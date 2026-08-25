from __future__ import annotations

import json
import os
import shlex
import signal
import sys
import tempfile
import textwrap
import threading
import time
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.database import Database
from goalaric_optimizer.cli import main
from goalaric_optimizer.scheduler import Scheduler
from goalaric_optimizer.service import (
    campaign_lock,
    init_campaign,
    pause_campaign,
    resume_campaign,
    stop_campaign,
)


class Phase6Test(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry = self.root / "registry.json"
        self.registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "fake-registry-v1",
                    "parameters": [{"name": "a", "value": 1}],
                }
            ),
            encoding="utf-8",
        )
        self.campaign_file = self.root / "campaign.json"
        self.campaign_file.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "fake-phase6",
                    "name": "Fake phase 6",
                    "mode": "fake",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 20260813,
                    "partitions": {"training": {"name": "training"}},
                }
            ),
            encoding="utf-8",
        )
        init_campaign(self.campaign_file, self.data_dir)
        self.database = Database(self.data_dir / "fake-phase6" / "campaign.db")
        self.fastchess = self.root / "fake_fastchess.py"
        self.monitor = self.root / "fake_testmonitor.py"
        self.pid_log = self.root / "pids.log"
        self._write_fake_processes()

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _write_fake_processes(self) -> None:
        self.fastchess.write_text(
            textwrap.dedent(
                """
                import argparse
                import os
                import time

                parser = argparse.ArgumentParser()
                parser.add_argument("--duration", type=float, required=True)
                parser.add_argument("--pid-log", required=True)
                args = parser.parse_args()
                with open(args.pid_log, "a", encoding="utf-8") as stream:
                    stream.write(f"{os.getpid()}\\n")
                time.sleep(args.duration)
                """
            ),
            encoding="utf-8",
        )
        self.monitor.write_text(
            textwrap.dedent(
                """
                import argparse
                import json
                import subprocess
                import sys

                parser = argparse.ArgumentParser()
                parser.add_argument("--fastchess", required=True)
                parser.add_argument("--duration", type=float, required=True)
                parser.add_argument("--pid-log", required=True)
                parser.add_argument("--result-path", required=True)
                parser.add_argument("--run-dir", required=True)
                parser.add_argument("--pairs-per-block", type=int, required=True)
                parser.add_argument("--mode", default="complete")
                args, _ = parser.parse_known_args()
                child = subprocess.Popen([
                    sys.executable, args.fastchess,
                    "--duration", str(args.duration),
                    "--pid-log", args.pid_log,
                ])
                if args.mode == "die":
                    child.kill()
                    child.wait()
                    raise SystemExit(17)
                code = child.wait()
                if code != 0:
                    raise SystemExit(code)
                with open(args.result_path, "w", encoding="utf-8") as stream:
                    json.dump({
                        "wins": 1,
                        "draws": 1,
                        "losses": 0,
                        "score": 75.0,
                        "games": ["1-0", "1/2-1/2"],
                    }, stream)
                """
            ),
            encoding="utf-8",
        )

    def _command(self, duration: float = 0.2, mode: str = "complete") -> list[str]:
        return [
            sys.executable,
            str(self.monitor),
            "--fastchess",
            str(self.fastchess),
            "--duration",
            str(duration),
            "--pid-log",
            str(self.pid_log),
            "--mode",
            mode,
        ]

    def _prepare(self, block_count: int = 1) -> None:
        self.database.ensure_fake_schedule("fake-phase6", block_count, 1)

    def _start(self, command: list[str], stop_grace_seconds: float = 0.1) -> tuple[threading.Thread, list[BaseException]]:
        errors: list[BaseException] = []

        def worker() -> None:
            try:
                Scheduler(
                    self.data_dir,
                    "fake-phase6",
                    command,
                    poll_interval=0.01,
                    stop_grace_seconds=stop_grace_seconds,
                ).run()
            except BaseException as exc:  # surfaced by the test thread below
                errors.append(exc)

        thread = threading.Thread(target=worker)
        thread.start()
        self._wait_until(
            lambda: bool(
                self.database.running_block_processes("fake-phase6")
                and self.database.running_block_processes("fake-phase6")[0]["process_group_id"]
            )
        )
        return thread, errors

    def _wait_until(self, predicate: object, timeout: float = 5.0) -> None:
        check = predicate  # keep the call site readable without weakening the timeout
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if check():
                return
            time.sleep(0.01)
        self.fail("timed out waiting for scheduler state")

    def _join_clean(self, thread: threading.Thread, errors: list[BaseException]) -> None:
        thread.join(timeout=5)
        self.assertFalse(thread.is_alive(), "scheduler did not stop")
        if errors:
            raise errors[0]

    def _assert_no_fake_processes_alive(self) -> None:
        if not self.pid_log.exists():
            return
        for raw_pid in self.pid_log.read_text(encoding="utf-8").splitlines():
            pid = int(raw_pid)
            proc_stat = Path(f"/proc/{pid}/stat")
            if not proc_stat.exists():
                continue
            fields = proc_stat.read_text(encoding="utf-8").split()
            self.assertNotEqual(fields[2], "Z", f"fake Fastchess PID {pid} is still a zombie")
            self.fail(f"fake Fastchess PID {pid} is still alive")

    def test_pause_resume_replays_interrupted_block_without_double_counting(self) -> None:
        self._prepare()
        thread, errors = self._start(self._command(duration=0.8))
        pause_campaign(self.data_dir, "fake-phase6")
        self.assertEqual(self.database.campaign("fake-phase6")["status"], "paused")
        block = self.database.running_block_processes("fake-phase6")
        self.assertEqual(block, [])
        self.assertEqual(self.database.status_snapshot("fake-phase6")["games"], 0)

        resume_campaign(self.data_dir, "fake-phase6")
        self._join_clean(thread, errors)
        snapshot = self.database.status_snapshot("fake-phase6")
        self.assertEqual(snapshot["status"], "completed")
        self.assertEqual(snapshot["games"], 2)
        self._assert_no_fake_processes_alive()
        row = self.database.list_trials("fake-phase6")[0]
        self.assertEqual(row["status"], "completed")
        with self.database._read() as connection:
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 2)
            self.assertEqual(connection.execute("SELECT attempt FROM match_blocks").fetchone()[0], 2)

    def test_dead_monitor_process_becomes_interrupted_and_can_resume(self) -> None:
        self._prepare()
        thread, errors = self._start(self._command(duration=2.0))
        process_group_id = self.database.running_block_processes("fake-phase6")[0]["process_group_id"]
        os.killpg(process_group_id, signal.SIGKILL)
        self._join_clean(thread, errors)
        self.assertEqual(self.database.status_snapshot("fake-phase6")["blocks"], {"interrupted": 1})
        self.assertEqual(self.database.status_snapshot("fake-phase6")["games"], 0)

        thread, errors = self._start(self._command(duration=0.03))
        self._join_clean(thread, errors)
        self.assertEqual(self.database.status_snapshot("fake-phase6")["status"], "completed")
        self.assertEqual(self.database.status_snapshot("fake-phase6")["games"], 2)
        self._assert_no_fake_processes_alive()

    def test_twenty_stop_restart_cycles_leave_no_duplicate_games_or_running_processes(self) -> None:
        self._prepare()
        for _ in range(20):
            thread, errors = self._start(self._command(duration=0.4), stop_grace_seconds=0.05)
            stop_campaign(self.data_dir, "fake-phase6")
            self._join_clean(thread, errors)
            self.assertEqual(self.database.running_block_processes("fake-phase6"), [])
            self.assertEqual(self.database.status_snapshot("fake-phase6")["games"], 0)
            self._assert_no_fake_processes_alive()
            resume = self.database.campaign("fake-phase6")
            self.assertEqual(resume["status"], "interrupted")

        thread, errors = self._start(self._command(duration=0.03))
        self._join_clean(thread, errors)
        snapshot = self.database.status_snapshot("fake-phase6")
        self.assertEqual(snapshot["status"], "completed")
        self.assertEqual(snapshot["games"], 2)
        self._assert_no_fake_processes_alive()
        with self.database._read() as connection:
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 2)
            self.assertEqual(connection.execute("SELECT attempt FROM match_blocks").fetchone()[0], 21)

    def test_stop_remains_available_while_optimize_lock_is_held(self) -> None:
        self._prepare()
        self.database.transition_campaign("fake-phase6", "running", "test optimize invocation")

        with campaign_lock(self.data_dir, "fake-phase6"):
            stopped = stop_campaign(self.data_dir, "fake-phase6")

        self.assertEqual(stopped["status"], "interrupted")
        self.assertEqual(self.database.status_snapshot("fake-phase6")["games"], 0)
        self.assertEqual(self.database.running_block_processes("fake-phase6"), [])

    def test_scheduler_runs_three_blocks_sequentially(self) -> None:
        self._prepare(block_count=3)
        thread, errors = self._start(self._command(duration=0.02))
        self._join_clean(thread, errors)
        snapshot = self.database.status_snapshot("fake-phase6")
        self.assertEqual(snapshot["status"], "completed")
        self.assertEqual(snapshot["blocks"], {"completed": 3})
        self.assertEqual(snapshot["games"], 6)
        with self.database._read() as connection:
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 6)
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM match_blocks WHERE status='running'").fetchone()[0], 0)

    def test_embedded_scheduler_preserves_optimizer_owner_and_active_trial(self) -> None:
        self._prepare(block_count=1)
        stale_trial = self.database.list_trials("fake-phase6")[0]
        self.database.transition_trial("fake-phase6", stale_trial["trial_id"], "completed")
        parameter_set_id = self.database.add_parameter_set(
            "fake-phase6",
            {
                "schema_version": 1,
                "registry": "fake-registry-v1",
                "parameters": [{"name": "a", "value": 2}],
            },
        )
        trial_id = self.database.create_trial("fake-phase6", parameter_set_id, "embedded-test", 7)
        block_ids = [
            self.database.create_match_block(
                "fake-phase6",
                trial_id,
                "training",
                index,
                1,
                20260813,
                "a" * 64,
                f"{index + 1:064x}",
            )
            for index in range(2)
        ]
        owner = "optimizer-regression-owner"
        self.database.claim_campaign("fake-phase6", owner)
        self.database.transition_campaign("fake-phase6", "running", "optimizer start")
        self.database.transition_trial("fake-phase6", trial_id, "running")

        scheduler = Scheduler(
            self.data_dir,
            "fake-phase6",
            self._command(duration=0.02),
            poll_interval=0.01,
            preserve_optimizer_state=True,
            embedded_campaign=True,
        )
        for block_id, expected_games in zip(block_ids, (2, 4), strict=True):
            scheduler.run(
                max_completed_blocks=1,
                finish_work=False,
                expected_block_id=block_id,
            )
            campaign = self.database.campaign("fake-phase6")
            self.assertEqual(campaign["owner_token"], owner)
            self.assertEqual(self.database.trial("fake-phase6", trial_id)["status"], "running")
            self.assertEqual(self.database.status_snapshot("fake-phase6")["games"], expected_games)

        events = self.database.events("fake-phase6")
        self.assertFalse(any(event["event_type"] == "abandoned_job_recovered" for event in events))
        with self.database._read() as connection:
            attempts = connection.execute(
                "SELECT attempt FROM match_blocks WHERE trial_id=? ORDER BY block_index",
                (trial_id,),
            ).fetchall()
            self.assertEqual([int(row[0]) for row in attempts], [1, 1])
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 4)
            self.assertEqual(
                connection.execute(
                    "SELECT status FROM match_blocks WHERE trial_id=?",
                    (stale_trial["trial_id"],),
                ).fetchone()[0],
                "pending",
            )
        self._assert_no_fake_processes_alive()

    def test_terminal_trial_block_reconciliation_is_idempotent(self) -> None:
        self._prepare(block_count=2)
        trial = self.database.list_trials("fake-phase6")[0]
        self.database.transition_trial("fake-phase6", trial["trial_id"], "completed")
        self.database.transition_campaign("fake-phase6", "running", "optimizer start")
        claimed = self.database.claim_next_block("fake-phase6")
        self.assertIsNotNone(claimed)
        self.database.recover_abandoned_jobs("fake-phase6", "simulated crash")

        self.assertEqual(
            self.database.reconcile_terminal_trial_blocks("fake-phase6", "terminal trial cleanup"),
            2,
        )
        self.assertEqual(
            self.database.reconcile_terminal_trial_blocks("fake-phase6", "terminal trial cleanup"),
            0,
        )
        with self.database._read() as connection:
            statuses = connection.execute(
                "SELECT status FROM match_blocks ORDER BY block_index"
            ).fetchall()
        self.assertEqual([row[0] for row in statuses], ["rejected", "rejected"])

    def test_phase6_fake_scheduler_is_available_through_cli(self) -> None:
        command = self._command(duration=0.02)
        self.assertEqual(
            main(
                [
                    "run",
                    "fake-phase6",
                    "--fake",
                    "--monitor-command",
                    shlex.join(command),
                    "--blocks",
                    "2",
                    "--data-dir",
                    str(self.data_dir),
                ]
            ),
            0,
        )
        self.assertEqual(self.database.status_snapshot("fake-phase6")["status"], "completed")
        self.assertEqual(self.database.status_snapshot("fake-phase6")["games"], 4)
        self._assert_no_fake_processes_alive()


if __name__ == "__main__":
    unittest.main()
