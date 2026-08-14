from __future__ import annotations

import json
import os
import socket
import subprocess
import tempfile
import time
import unittest
import urllib.request
from pathlib import Path

import sys

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.canonical import sha256_json
from goalaric_optimizer.database import Database


class Phase15FinalVerificationTest(unittest.TestCase):
    """Verify v1.1 through the installed terminal command and one SQLite DB."""

    CAMPAIGN_ID = "phase15-v1-1-final"

    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-phase15-")
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry = self.root / "registry.json"
        self.registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "phase15-final-registry-v1",
                    "parameters": [
                        {"name": "p1", "value": 10, "min": 0, "max": 24, "step": 8, "min_step": 2},
                        {"name": "p2", "value": 20, "min": 0, "max": 40, "step": 16, "min_step": 4},
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
                    "name": "Phase 15 full v1.1 verification",
                    "mode": "fake",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 20260814,
                    "partitions": {"optimization": {"name": "optimization"}},
                    "goals": {
                        "max_games": 200,
                        "max_evaluations": 100,
                        "max_passes": 20,
                        "optimizer": {"parameters": ["p1", "p2"]},
                        "adaptive": {"min_blocks": 1, "max_blocks": 1},
                        "fake_match": {"optimum": {"p1": 14, "p2": 28}},
                        "confirmation": {
                            "enabled": True,
                            "games": 8,
                            "seed": 20260830,
                            "confidence": 0.95,
                            "fake_result": {"wins": 8, "draws": 0, "losses": 0},
                        },
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
        self.assertTrue(self.optimizer.exists(), f"missing installed optimizer command: {self.optimizer}")

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _run(self, *arguments: str, timeout: float = 30.0) -> dict[str, object]:
        result = subprocess.run(
            [str(self.optimizer), *arguments],
            cwd=self.repo_root,
            env=self.environment,
            capture_output=True,
            text=True,
            timeout=timeout,
        )
        self.assertEqual(
            result.returncode,
            0,
            msg=f"command failed: {arguments}\nstdout:\n{result.stdout}\nstderr:\n{result.stderr}",
        )
        if not result.stdout.strip():
            return {}
        return json.loads(result.stdout)

    def _database(self) -> Database:
        return Database(self.data_dir / self.CAMPAIGN_ID / "campaign.db")

    @staticmethod
    def _free_port() -> int:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
            probe.bind(("127.0.0.1", 0))
            return int(probe.getsockname()[1])

    def _start_dashboard(self) -> tuple[subprocess.Popen[str], str]:
        port = self._free_port()
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
            stdout=subprocess.DEVNULL,
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
            except OSError:
                if process.poll() is not None:
                    break
                time.sleep(0.05)
        if process.poll() is None:
            process.terminate()
            process.wait(timeout=5)
        self.fail("dashboard did not start")

    @staticmethod
    def _stop_process(process: subprocess.Popen[str] | None) -> None:
        if process is None:
            return
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)

    def test_terminal_campaign_search_confirmation_report_and_resume(self) -> None:
        dashboard: subprocess.Popen[str] | None = None
        status_watch: subprocess.Popen[str] | None = None
        try:
            self._run("init", str(self.campaign), "--data-dir", str(self.data_dir))
            database = self._database()

            # A quota-limited ordinary CLI invocation is a planned stop during search.
            first = self._run(
                "optimize",
                str(self.campaign),
                "--data-dir",
                str(self.data_dir),
                "--max-results",
                "1",
            )
            self.assertEqual(first["phase"], "coordinate")
            self.assertEqual(database.optimizer_state(self.CAMPAIGN_ID)["state"]["result_count"], 1)

            dashboard, dashboard_url = self._start_dashboard()
            status_watch = subprocess.Popen(
                [
                    str(self.optimizer),
                    "status",
                    self.CAMPAIGN_ID,
                    "--data-dir",
                    str(self.data_dir),
                    "--watch",
                    "--interval",
                    "0.01",
                    "--iterations",
                    "4",
                ],
                cwd=self.repo_root,
                env=self.environment,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
            )

            confirmation = None
            for _ in range(60):
                self._run(
                    "optimize",
                    str(self.campaign),
                    "--data-dir",
                    str(self.data_dir),
                    "--max-results",
                    "1",
                )
                confirmation = database.confirmation_snapshot(self.CAMPAIGN_ID)
                if confirmation is not None:
                    break
            self.assertIsNotNone(confirmation, "search did not reach automatic confirmation")
            assert confirmation is not None
            self.assertEqual(database.optimizer_state(self.CAMPAIGN_ID)["state"]["phase"], "completed")
            self.assertEqual(confirmation["status"], "running")
            completed_before_resume = sum(block["status"] == "completed" for block in confirmation["blocks"])
            self.assertGreaterEqual(completed_before_resume, 1)
            self.assertLess(completed_before_resume, 4)

            with urllib.request.urlopen(dashboard_url, timeout=2) as response:
                dashboard_snapshot = json.load(response)
            self.assertTrue(dashboard_snapshot["read_only"])
            self.assertEqual(dashboard_snapshot["confirmation"]["status"], "running")
            self.assertFalse(dashboard_snapshot["campaign"]["finished"])

            # Stop again while the fixed confirmation is in progress, then resume.
            self._run(
                "optimize",
                str(self.campaign),
                "--data-dir",
                str(self.data_dir),
                "--max-results",
                "1",
            )
            stopped_confirmation = database.confirmation_snapshot(self.CAMPAIGN_ID)
            assert stopped_confirmation is not None
            self.assertEqual(stopped_confirmation["status"], "running")
            self.assertLess(
                sum(block["status"] == "completed" for block in stopped_confirmation["blocks"]),
                4,
            )

            self._run("optimize", str(self.campaign), "--data-dir", str(self.data_dir), timeout=60)
            final = database.confirmation_snapshot(self.CAMPAIGN_ID)
            self.assertIsNotNone(final)
            assert final is not None
            self.assertEqual(final["status"], "completed")
            self.assertEqual(final["outcome"], "confirmed")
            self.assertEqual(final["recommendation"], "candidate")
            self.assertFalse(final["result"]["automatic_promotion"])
            self.assertEqual(len(final["blocks"]), 4)
            self.assertEqual(sum(block["status"] == "completed" for block in final["blocks"]), 4)

            state = database.optimizer_state(self.CAMPAIGN_ID)["state"]
            self.assertEqual(state["phase"], "completed")
            self.assertEqual(state["parameter_names"], ["p1", "p2"])
            self.assertGreater(len(state["step_history"]), 1)
            self.assertEqual(len(state["evaluated_parameter_hashes"]), len(set(state["evaluated_parameter_hashes"])))
            self.assertNotEqual(final["candidate_parameter_hash"], final["baseline_parameter_hash"])
            self.assertEqual(final["recommendation_parameter_hash"], final["candidate_parameter_hash"])

            recommendation_path = Path(final["recommendation_parameter_file"])
            self.assertTrue(recommendation_path.is_file())
            recommendation = json.loads(recommendation_path.read_text(encoding="utf-8"))
            self.assertEqual(sha256_json(recommendation), final["recommendation_parameter_hash"])

            report_path = self.root / "final-report.json"
            self._run(
                "report",
                self.CAMPAIGN_ID,
                "--data-dir",
                str(self.data_dir),
                "--format",
                "json",
                "--output",
                str(report_path),
            )
            report = json.loads(report_path.read_text(encoding="utf-8"))
            self.assertTrue(report["campaign"]["finished"])
            self.assertEqual(report["campaign"]["status"], "completed")
            self.assertEqual(report["final_anchor"]["source"], "optimizer_checkpoint")
            self.assertEqual(report["highest_local_trial"]["source"], "highest_local_trial")
            self.assertEqual(
                report["confirmation"]["candidate_parameter_hash"],
                report["final_anchor"]["parameter_hash"],
            )
            self.assertEqual(
                report["parameter_differences"],
                report["confirmation"]["parameter_differences"],
            )
            self.assertEqual(
                report["total_games"],
                report["search_games"] + report["confirmation_games"],
            )
            self.assertNotIn("blocks", report)
            self.assertNotIn("block_ids", report)
            self.assertEqual(report["confirmation"]["outcome"], "confirmed")
            self.assertEqual(report["confirmation"]["recommendation_parameter_file"], str(recommendation_path))

            with database._read() as connection:
                search_games = connection.execute(
                    "SELECT COUNT(*) FROM games WHERE campaign_id=?", (self.CAMPAIGN_ID,)
                ).fetchone()[0]
                distinct_search_games = connection.execute(
                    "SELECT COUNT(DISTINCT game_id) FROM games WHERE campaign_id=?", (self.CAMPAIGN_ID,)
                ).fetchone()[0]
                confirmation_games = connection.execute(
                    "SELECT COUNT(*) FROM confirmation_games WHERE confirmation_id=?",
                    (final["confirmation_id"],),
                ).fetchone()[0]
                distinct_confirmation_games = connection.execute(
                    "SELECT COUNT(DISTINCT game_id) FROM confirmation_games WHERE confirmation_id=?",
                    (final["confirmation_id"],),
                ).fetchone()[0]
            self.assertEqual(search_games, distinct_search_games)
            self.assertEqual(search_games, database.status_snapshot(self.CAMPAIGN_ID)["games"])
            self.assertEqual(confirmation_games, 8)
            self.assertEqual(confirmation_games, distinct_confirmation_games)
            self.assertEqual(database.running_block_processes(self.CAMPAIGN_ID), [])
            self.assertEqual(database.running_confirmation_block_processes(self.CAMPAIGN_ID), [])

            status_output, _ = status_watch.communicate(timeout=5)
            self.assertEqual(status_watch.returncode, 0)
            self.assertGreaterEqual(status_output.count(self.CAMPAIGN_ID), 4)
        finally:
            self._stop_process(status_watch)
            self._stop_process(dashboard)


if __name__ == "__main__":
    unittest.main()
