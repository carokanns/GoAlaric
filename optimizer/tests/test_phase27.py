from __future__ import annotations

import json
import os
import socket
import subprocess
import sys
import tempfile
import time
import unittest
import urllib.error
import urllib.request
from pathlib import Path

from goalaric_optimizer.dashboard import DashboardReader, final_report
from goalaric_optimizer.database import Database
from goalaric_optimizer.optimization import run_optimization
from goalaric_optimizer.profiles import MatchProfile
from goalaric_optimizer.service import init_campaign


class Phase27BayesianRestartStressTest(unittest.TestCase):
    """Stress fixed-pair Bayesian search and confirmation on one SQLite DB."""

    CAMPAIGN_ID = "phase27-bayesian-stress"
    EVALUATIONS = 24
    PAIRS_PER_EVALUATION = 6
    KILLED_RUNS = 50
    CONFIRMATION_GAMES = 40

    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-phase27-")
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry_path = self.root / "registry.json"
        self.registry_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "phase27-bayesian-stress-v1",
                    "parameters": [
                        {"name": "p1", "value": 2, "min": 0, "max": 4, "step": 1, "min_step": 1},
                        {"name": "p2", "value": 4, "min": 0, "max": 8, "step": 2, "min_step": 2},
                        {"name": "p3", "value": 6, "min": 2, "max": 10, "step": 2, "min_step": 2},
                        {"name": "p4", "value": 8, "min": 4, "max": 12, "step": 2, "min_step": 2},
                    ],
                }
            ),
            encoding="utf-8",
        )
        self.campaign_path = self.root / "campaign.json"
        self.campaign_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": self.CAMPAIGN_ID,
                    "name": "Phase 27 Bayesian restart stress",
                    "mode": "fake",
                    "registry": str(self.registry_path),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 27027,
                    "partitions": {"stress": {"name": "stress"}},
                    "goals": {
                        "max_games": self.EVALUATIONS * self.PAIRS_PER_EVALUATION * 2,
                        "max_evaluations": self.EVALUATIONS,
                        "optimizer": {
                            "algorithm": "finite-noise-aware-bo-v1",
                            "parameters": ["p1", "p2", "p3", "p4"],
                            "initial_points": 5,
                            "pairs_per_evaluation": self.PAIRS_PER_EVALUATION,
                            "profile": "node-stress",
                        },
                        "fake_match": {"optimum": {"p1": 1, "p2": 6, "p3": 8, "p4": 10}},
                        "real": {
                            "tc": "0.2+0.01",
                            "profiles": {
                                "node-stress": {"nodes": 1000},
                                "node-confirmation": {"nodes": 2000},
                            },
                        },
                        "confirmation": {
                            "enabled": True,
                            "games": self.CONFIRMATION_GAMES,
                            "seed": 27028,
                            "confidence": 0.95,
                            "profile": "node-confirmation",
                            "fake_result": {
                                "wins": 0,
                                "draws": self.CONFIRMATION_GAMES,
                                "losses": 0,
                            },
                        },
                    },
                }
            ),
            encoding="utf-8",
        )
        self.definition, _, self.database_path = init_campaign(
            self.campaign_path, self.data_dir
        )
        self.profile = MatchProfile.create(
            "node-stress", source="real.profiles.node-stress", nodes=1000
        )
        Database(self.database_path).bind_optimizer_profile(
            self.CAMPAIGN_ID, "search", self.profile.as_dict()
        )
        self.repo_root = Path(__file__).parents[2].resolve()
        self.optimizer = self.repo_root / "optimizer" / ".venv" / "bin" / "optimizer"
        self.environment = os.environ.copy()
        source_path = str(Path(__file__).parents[1] / "src")
        self.environment["PYTHONPATH"] = os.pathsep.join(
            item for item in (source_path, self.environment.get("PYTHONPATH")) if item
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _start_optimizer(self, hold_after_claim: bool) -> subprocess.Popen[str]:
        command = [
            sys.executable,
            str(Path(__file__).with_name("phase27_worker.py")),
            "--database",
            str(self.database_path),
            "--registry",
            str(self.registry_path),
            "--campaign-id",
            self.CAMPAIGN_ID,
        ]
        if hold_after_claim:
            command.append("--hold-after-claim")
        return subprocess.Popen(
            command,
            cwd=self.repo_root,
            env=self.environment,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            text=True,
        )

    def _attempts(self, database: Database) -> dict[str, int]:
        with database._read() as connection:
            rows = connection.execute(
                "SELECT block_id,attempt FROM match_blocks WHERE campaign_id=?",
                (self.CAMPAIGN_ID,),
            ).fetchall()
        return {str(row["block_id"]): int(row["attempt"]) for row in rows}

    def _wait_for_new_running_block(
        self, database: Database, previous: dict[str, int], timeout: float = 30.0
    ) -> None:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            running = database.running_block_processes(self.CAMPAIGN_ID)
            if running:
                block = running[0]
                block_id = str(block["block_id"])
                if block_id not in previous or int(block["attempt"]) > previous[block_id]:
                    return
            time.sleep(0.01)
        self.fail("Bayesian optimizer did not expose a newly claimed block")

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
        deadline = time.monotonic() + 10.0
        while time.monotonic() < deadline:
            try:
                with urllib.request.urlopen(url, timeout=0.5) as response:
                    if response.status == 200:
                        return process, url
            except (OSError, urllib.error.URLError):
                if process.poll() is not None:
                    break
                time.sleep(0.05)
        output = process.stdout.read() if process.stdout is not None else ""
        self._stop_process(process)
        self.fail(f"dashboard did not start: {output}")

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
        if process.stdout is not None:
            process.stdout.close()

    def test_fifty_process_deaths_and_confirmation_restarts_are_exact(self) -> None:
        database = Database(self.database_path)
        dashboard: subprocess.Popen[str] | None = None
        status_watch: subprocess.Popen[str] | None = None
        try:
            dashboard, dashboard_url = self._start_dashboard()
            killed_runs = 0
            for evaluation in range(self.EVALUATIONS):
                kills_this_evaluation = 3 if evaluation < 2 else 2
                for _ in range(kills_this_evaluation):
                    previous = self._attempts(database)
                    process = self._start_optimizer(hold_after_claim=True)
                    self._wait_for_new_running_block(database, previous)
                    if status_watch is None:
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
                            live = json.load(response)
                        self.assertTrue(live["read_only"])
                        self.assertEqual(live["campaign"]["status"], "running")
                    process.kill()
                    process.wait(timeout=10)
                    self.assertEqual(process.returncode, -9)
                    if process.stderr is not None:
                        process.stderr.close()
                    killed_runs += 1
                process = self._start_optimizer(hold_after_claim=False)
                _, errors = process.communicate(timeout=120)
                self.assertEqual(process.returncode, 0, errors)

            self.assertEqual(killed_runs, self.KILLED_RUNS)
            assert status_watch is not None
            status_output, _ = status_watch.communicate(timeout=10)
            self.assertEqual(status_watch.returncode, 0)
            self.assertGreaterEqual(status_output.count(f'"campaign_id": "{self.CAMPAIGN_ID}"'), 6)
            self.assertIn('"status": "running"', status_output)
            status_watch = None

            search_state = database.optimizer_state(self.CAMPAIGN_ID)["state"]
            self.assertEqual(search_state["phase"], "completed")
            self.assertEqual(search_state["result_count"], self.EVALUATIONS)
            self.assertEqual(
                search_state["consumed_games"],
                self.EVALUATIONS * self.PAIRS_PER_EVALUATION * 2,
            )

            for _ in range(self.CONFIRMATION_GAMES // 2):
                result = run_optimization(self.campaign_path, self.data_dir, invocation_limit=1)
                self.assertEqual(result["confirmation"]["status"], "running")
            result = run_optimization(self.campaign_path, self.data_dir, invocation_limit=1)
            self.assertEqual(result["confirmation"]["status"], "completed")
            self.assertEqual(result["confirmation"]["outcome"], "inconclusive")
            self.assertIsNone(result["confirmation"]["recommendation_parameter_hash"])

            snapshot = DashboardReader(self.data_dir, self.CAMPAIGN_ID).snapshot()
            self.assertTrue(snapshot["read_only"])
            self.assertEqual(snapshot["campaign"]["status"], "completed")
            self.assertEqual(snapshot["search_games"], 288)
            self.assertEqual(snapshot["confirmation_games"], 40)
            self.assertEqual(snapshot["total_games"], 328)
            self.assertEqual(snapshot["final_anchor"]["source"], "bayesian_checkpoint_candidate")
            report, _ = final_report(self.data_dir, self.CAMPAIGN_ID, report_format="json")
            self.assertEqual(report["total_games"], 328)
            self.assertIsNone(report["confirmation"]["recommendation"])

            with database._read() as connection:
                proposals = connection.execute(
                    "SELECT COUNT(*),COUNT(DISTINCT parameter_hash) FROM bayesian_proposals"
                ).fetchone()
                observations = connection.execute(
                    "SELECT COUNT(*),COUNT(DISTINCT proposal_id),COALESCE(SUM(games),0) "
                    "FROM bayesian_observations"
                ).fetchone()
                blocks = connection.execute(
                    "SELECT COUNT(*),COUNT(DISTINCT materialized_openings_sha256),"
                    "COALESCE(SUM(wins+draws+losses),0),COALESCE(SUM(attempt),0),"
                    "SUM(status!='completed') FROM match_blocks"
                ).fetchone()
                games = connection.execute(
                    "SELECT COUNT(*),COUNT(DISTINCT game_id),"
                    "COUNT(DISTINCT block_id || ':' || game_index) FROM games"
                ).fetchone()
                recovered = connection.execute(
                    "SELECT COUNT(*) FROM events WHERE event_type='abandoned_job_recovered'"
                ).fetchone()[0]
                confirmation = connection.execute(
                    "SELECT COUNT(*),COUNT(DISTINCT block_index),SUM(status!='completed') "
                    "FROM confirmation_blocks"
                ).fetchone()
                confirmation_games = connection.execute(
                    "SELECT COUNT(*),COUNT(DISTINCT game_id),"
                    "COUNT(DISTINCT block_id || ':' || game_index) FROM confirmation_games"
                ).fetchone()
            expected_blocks = self.EVALUATIONS * self.PAIRS_PER_EVALUATION
            self.assertEqual(tuple(proposals), (self.EVALUATIONS, self.EVALUATIONS))
            self.assertEqual(tuple(observations), (self.EVALUATIONS, self.EVALUATIONS, 288))
            self.assertEqual(
                tuple(blocks),
                (expected_blocks, expected_blocks, 288, expected_blocks + self.KILLED_RUNS, 0),
            )
            self.assertEqual(tuple(games), (288, 288, 288))
            self.assertGreaterEqual(recovered, self.KILLED_RUNS)
            self.assertEqual(tuple(confirmation), (20, 20, 0))
            self.assertEqual(tuple(confirmation_games), (40, 40, 40))
            self.assertEqual(database.running_block_processes(self.CAMPAIGN_ID), [])
            self.assertEqual(database.running_confirmation_block_processes(self.CAMPAIGN_ID), [])

            terminal_counts = (tuple(proposals), tuple(observations), tuple(blocks), tuple(games))
            replay = run_optimization(self.campaign_path, self.data_dir, invocation_limit=1)
            self.assertEqual(replay["confirmation"]["status"], "completed")
            with database._read() as connection:
                replay_counts = (
                    tuple(connection.execute(
                        "SELECT COUNT(*),COUNT(DISTINCT parameter_hash) FROM bayesian_proposals"
                    ).fetchone()),
                    tuple(connection.execute(
                        "SELECT COUNT(*),COUNT(DISTINCT proposal_id),COALESCE(SUM(games),0) "
                        "FROM bayesian_observations"
                    ).fetchone()),
                    tuple(connection.execute(
                        "SELECT COUNT(*),COUNT(DISTINCT materialized_openings_sha256),"
                        "COALESCE(SUM(wins+draws+losses),0),COALESCE(SUM(attempt),0),"
                        "SUM(status!='completed') FROM match_blocks"
                    ).fetchone()),
                    tuple(connection.execute(
                        "SELECT COUNT(*),COUNT(DISTINCT game_id),"
                        "COUNT(DISTINCT block_id || ':' || game_index) FROM games"
                    ).fetchone()),
                )
            self.assertEqual(replay_counts, terminal_counts)

            database_mtime = self.database_path.stat().st_mtime_ns
            for _ in range(10):
                with urllib.request.urlopen(dashboard_url, timeout=2) as response:
                    self.assertEqual(json.load(response)["total_games"], 328)
            self.assertEqual(self.database_path.stat().st_mtime_ns, database_mtime)
        finally:
            self._stop_process(status_watch)
            self._stop_process(dashboard)


if __name__ == "__main__":
    unittest.main()
