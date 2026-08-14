from __future__ import annotations

import json
import os
import signal
import socket
import subprocess
import sys
import tempfile
import time
import unittest
import urllib.error
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.cli import main
from goalaric_optimizer.database import Database


class Phase14LongFakeStressTest(unittest.TestCase):
    """Exercise autonomous restart recovery against one persistent SQLite DB."""

    CAMPAIGN_ID = "phase14-fake-stress"
    KILLED_RUNS = 50
    SUCCESSFUL_RESTARTS = 21

    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-phase14-")
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry = self.root / "registry.json"
        self.registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "phase14-stress-registry-v1",
                    "parameters": [
                        {"name": "p1", "value": 10, "min": 0, "max": 40, "step": 8, "min_step": 2},
                        {"name": "p2", "value": 20, "min": 0, "max": 64, "step": 16, "min_step": 4},
                        {"name": "p3", "value": 30, "min": 0, "max": 90, "step": 30, "min_step": 5},
                    ],
                }
            ),
            encoding="utf-8",
        )
        self.campaign = self.root / "campaign.json"
        self.campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": self.CAMPAIGN_ID,
                    "name": "Phase 14 long fake stress",
                    "mode": "fake",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 20260814,
                    "partitions": {"stress": {"name": "stress"}},
                    "goals": {
                        "max_games": 42,
                        "max_evaluations": 21,
                        "max_passes": 100,
                        "optimizer": {"parameters": ["p1", "p2", "p3"]},
                        "adaptive": {"min_blocks": 1, "max_blocks": 1},
                        "fake_match": {"optimum": {"p1": 10, "p2": 20, "p3": 30}},
                    },
                }
            ),
            encoding="utf-8",
        )
        self.repo_root = Path(__file__).parents[2].resolve()
        self.optimizer = self.repo_root / "optimizer" / ".venv" / "bin" / "optimizer"
        self.environment = os.environ.copy()
        source_path = str(Path(__file__).parents[1] / "src")
        self.environment["PYTHONPATH"] = os.pathsep.join(
            item for item in (source_path, self.environment.get("PYTHONPATH")) if item
        )
        self.assertEqual(
            main(["init", str(self.campaign), "--data-dir", str(self.data_dir)]),
            0,
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _optimize_args(self) -> list[str]:
        return [
            "optimize",
            str(self.campaign),
            "--data-dir",
            str(self.data_dir),
            "--max-results",
            "1",
        ]

    def _fork_optimizer(self, hold_after_claim: bool) -> int:
        pid = os.fork()
        if pid == 0:
            if hold_after_claim:
                os.environ["GOALARIC_PHASE14_HOLD_AFTER_CLAIM"] = "1"
            else:
                os.environ.pop("GOALARIC_PHASE14_HOLD_AFTER_CLAIM", None)
            with open(os.devnull, "w", encoding="utf-8") as sink:
                os.dup2(sink.fileno(), sys.stdout.fileno())
                os.dup2(sink.fileno(), sys.stderr.fileno())
            try:
                exit_code = main(self._optimize_args())
            except BaseException:
                exit_code = 1
            os._exit(exit_code)
        return pid

    @staticmethod
    def _wait_child(pid: int) -> int:
        waited_pid, status = os.waitpid(pid, 0)
        if waited_pid != pid:
            raise AssertionError(f"waitpid returned {waited_pid}, expected {pid}")
        return status

    def _block_attempts(self, database: Database) -> dict[str, int]:
        with database._read() as connection:
            rows = connection.execute(
                "SELECT block_id,attempt FROM match_blocks WHERE campaign_id=?",
                (self.CAMPAIGN_ID,),
            ).fetchall()
            return {str(row["block_id"]): int(row["attempt"]) for row in rows}

    def _wait_for_running_block(
        self, database: Database, previous_attempts: dict[str, int] | None = None, timeout: float = 5.0
    ) -> None:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            running = database.running_block_processes(self.CAMPAIGN_ID)
            if running:
                block = running[0]
                if previous_attempts is None:
                    return
                block_id = str(block["block_id"])
                if block_id not in previous_attempts or int(block["attempt"]) > previous_attempts[block_id]:
                    return
            time.sleep(0.01)
        self.fail(f"optimizer did not expose a running block: {database.status_snapshot(self.CAMPAIGN_ID)}")

    @staticmethod
    def _free_local_port() -> int:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
            probe.bind(("127.0.0.1", 0))
            return int(probe.getsockname()[1])

    def _start_dashboard(self) -> tuple[subprocess.Popen[str], str]:
        port = self._free_local_port()
        process = subprocess.Popen(
            [
                str(self.optimizer),
                "dashboard",
                self.CAMPAIGN_ID,
                "--data-dir",
                str(self.data_dir),
                "--listen",
                f"127.0.0.1:{port}",
                "--refresh-ms",
                "250",
            ],
            cwd=self.repo_root,
            env=self.environment,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        url = f"http://127.0.0.1:{port}/api/dashboard"
        deadline = time.monotonic() + 5.0
        while time.monotonic() < deadline:
            try:
                with urllib.request.urlopen(url, timeout=0.5) as response:
                    if response.status == 200:
                        return process, url
            except (OSError, urllib.error.URLError):
                if process.poll() is not None:
                    break
                time.sleep(0.05)
        if process.poll() is None:
            process.terminate()
            process.wait(timeout=5)
        output = process.stdout.read() if process.stdout is not None else ""
        self.fail(f"dashboard did not start: {output}")

    def _stop_process(self, process: subprocess.Popen[str] | None) -> None:
        if process is None:
            return
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
        if process.stdout is not None:
            process.stdout.close()

    def test_fifty_process_deaths_resume_exactly_without_duplicates(self) -> None:
        database = Database(self.data_dir / self.CAMPAIGN_ID / "campaign.db")
        original_claim = Database.claim_next_block
        dashboard: subprocess.Popen[str] | None = None
        status_watch: subprocess.Popen[str] | None = None

        def delayed_claim(instance: Database, campaign_id: str) -> dict[str, object] | None:
            block = original_claim(instance, campaign_id)
            if block is not None and os.environ.get("GOALARIC_PHASE14_HOLD_AFTER_CLAIM") == "1":
                time.sleep(0.4)
            return block

        Database.claim_next_block = delayed_claim  # type: ignore[method-assign]
        try:
            dashboard, dashboard_url = self._start_dashboard()
            previous_attempts = self._block_attempts(database)
            first_kill_pid = self._fork_optimizer(hold_after_claim=True)
            self._wait_for_running_block(database, previous_attempts)

            status_watch = subprocess.Popen(
                [
                    str(self.optimizer),
                    "status",
                    self.CAMPAIGN_ID,
                    "--data-dir",
                    str(self.data_dir),
                    "--watch",
                    "--interval",
                    "0.05",
                    "--iterations",
                    "6",
                ],
                    cwd=self.repo_root,
                env=self.environment,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
            )
            with urllib.request.urlopen(dashboard_url, timeout=2) as response:
                dashboard_snapshot = json.load(response)
            self.assertTrue(dashboard_snapshot["read_only"])
            self.assertEqual(dashboard_snapshot["campaign"]["status"], "running")

            os.kill(first_kill_pid, signal.SIGKILL)
            first_status = self._wait_child(first_kill_pid)
            self.assertTrue(os.WIFSIGNALED(first_status))
            killed_runs = 1

            for round_index in range(self.SUCCESSFUL_RESTARTS):
                rounds_left = self.SUCCESSFUL_RESTARTS - round_index
                remaining_kills = self.KILLED_RUNS - killed_runs
                kills_this_round = min(remaining_kills, (remaining_kills + rounds_left - 1) // rounds_left)
                for _ in range(kills_this_round):
                    previous_attempts = self._block_attempts(database)
                    pid = self._fork_optimizer(hold_after_claim=True)
                    self._wait_for_running_block(database, previous_attempts)
                    os.kill(pid, signal.SIGKILL)
                    status = self._wait_child(pid)
                    self.assertTrue(os.WIFSIGNALED(status))
                    killed_runs += 1
                pid = self._fork_optimizer(hold_after_claim=False)
                status = self._wait_child(pid)
                self.assertTrue(os.WIFEXITED(status), f"optimizer restart did not exit normally: {status}")
                self.assertEqual(os.WEXITSTATUS(status), 0)

            self.assertEqual(killed_runs, self.KILLED_RUNS)
            final_pid = self._fork_optimizer(hold_after_claim=False)
            final_status = self._wait_child(final_pid)
            self.assertTrue(os.WIFEXITED(final_status))
            self.assertEqual(os.WEXITSTATUS(final_status), 0)
            status_output, _ = status_watch.communicate(timeout=5)
            self.assertEqual(status_watch.returncode, 0)
            self.assertGreaterEqual(status_output.count('"campaign_id": "phase14-fake-stress"'), 6)
            self.assertIn('"status": "running"', status_output)
            status_watch = None

            with urllib.request.urlopen(dashboard_url, timeout=2) as response:
                final_dashboard = json.load(response)
            self.assertTrue(final_dashboard["read_only"])
            self.assertEqual(final_dashboard["campaign"]["status"], "completed")
            self.assertEqual(final_dashboard["consumed_games"], 42)

            snapshot = database.status_snapshot(self.CAMPAIGN_ID)
            self.assertEqual(snapshot["status"], "completed")
            self.assertEqual(snapshot["games"], 42)
            self.assertEqual(snapshot["blocks"], {"completed": 21})
            self.assertEqual(snapshot["trials"], {"completed": 21})
            self.assertEqual(database.running_block_processes(self.CAMPAIGN_ID), [])

            state = database.optimizer_state(self.CAMPAIGN_ID)["state"]
            self.assertEqual(state["result_count"], 21)
            self.assertEqual(state["parameter_names"], ["p1", "p2", "p3"])
            self.assertEqual(
                state["step_history"],
                [
                    {"p1": 8, "p2": 16, "p3": 30},
                    {"p1": 4, "p2": 8, "p3": 15},
                    {"p1": 2, "p2": 4, "p3": 7},
                    {"p1": 2, "p2": 4, "p3": 5},
                ],
            )
            self.assertEqual(len(state["evaluated_parameter_hashes"]), 21)
            self.assertEqual(len(set(state["evaluated_parameter_hashes"])), 21)

            with database._read() as connection:
                self.assertEqual(
                    connection.execute(
                        "SELECT COUNT(*) FROM trials WHERE campaign_id=? AND algorithm='coordinate-multires-v1'",
                        (self.CAMPAIGN_ID,),
                    ).fetchone()[0],
                    21,
                )
                self.assertEqual(
                    connection.execute(
                        "SELECT COUNT(*) FROM trials WHERE campaign_id=? AND status!='completed'",
                        (self.CAMPAIGN_ID,),
                    ).fetchone()[0],
                    0,
                )
                self.assertEqual(
                    connection.execute(
                        "SELECT COUNT(*) FROM games WHERE campaign_id=?",
                        (self.CAMPAIGN_ID,),
                    ).fetchone()[0],
                    42,
                )
                self.assertEqual(
                    connection.execute(
                        "SELECT COUNT(DISTINCT game_id) FROM games WHERE campaign_id=?",
                        (self.CAMPAIGN_ID,),
                    ).fetchone()[0],
                    42,
                )
                self.assertEqual(
                    connection.execute(
                        "SELECT COUNT(*) FROM events WHERE campaign_id=? AND event_type='match_block_completed'",
                        (self.CAMPAIGN_ID,),
                    ).fetchone()[0],
                    21,
                )
                self.assertGreaterEqual(
                    connection.execute(
                        "SELECT COUNT(*) FROM events WHERE campaign_id=? AND event_type='abandoned_job_recovered'",
                        (self.CAMPAIGN_ID,),
                    ).fetchone()[0],
                    self.KILLED_RUNS,
                )
        finally:
            Database.claim_next_block = original_claim  # type: ignore[method-assign]
            self._stop_process(status_watch)
            self._stop_process(dashboard)


if __name__ == "__main__":
    unittest.main()
