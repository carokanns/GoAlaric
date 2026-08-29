from __future__ import annotations

import json
import shutil
import sqlite3
import subprocess
import tempfile
import unittest
from pathlib import Path

from goalaric_optimizer.dashboard import DashboardReader, final_report
from goalaric_optimizer.database import Database
from goalaric_optimizer.optimization import run_optimization


class Phase26BayesianCompletionTest(unittest.TestCase):
    def test_bounded_search_defers_restartable_confirmation_and_reports_candidate(self) -> None:
        with tempfile.TemporaryDirectory(prefix="goalaric-phase26-fake-") as raw_dir:
            root = Path(raw_dir)
            data_dir = root / "campaigns"
            registry = root / "registry.json"
            registry.write_text(
                json.dumps(
                    {
                        "schema_version": 1,
                        "registry": "phase26-completion-v1",
                        "parameters": [
                            {"name": "x", "value": 1, "min": 0, "max": 2, "step": 1, "min_step": 1},
                            {"name": "unchanged", "value": 7, "min": 5, "max": 9, "step": 1, "min_step": 1},
                        ],
                    }
                ),
                encoding="utf-8",
            )
            campaign = root / "campaign.json"
            campaign.write_text(
                json.dumps(
                    {
                        "schema_version": 1,
                        "campaign_id": "phase26-completion",
                        "name": "Phase 26 Bayesian completion",
                        "mode": "fake",
                        "registry": str(registry),
                        "baseline": {
                            "engine_id": "fake-engine",
                            "parameters": {
                                "schema_version": 1,
                                "registry": "phase26-completion-v1",
                                "parameters": [
                                    {"name": "x", "value": 1},
                                    {"name": "unchanged", "value": 9},
                                ],
                            },
                        },
                        "master_seed": 26,
                        "partitions": {"search": {"name": "search"}},
                        "goals": {
                            "max_games": 256,
                            "max_evaluations": 1,
                            "optimizer": {
                                "algorithm": "finite-noise-aware-bo-v1",
                                "parameters": ["x"],
                                "initial_points": 2,
                                "pairs_per_evaluation": 128,
                            },
                            "fake_match": {"optimum": {"x": 0}},
                            "confirmation": {
                                "enabled": True,
                                "games": 4,
                                "seed": 27,
                                "confidence": 0.95,
                                "fake_result": {"wins": 0, "draws": 4, "losses": 0},
                            },
                        },
                    }
                ),
                encoding="utf-8",
            )

            search = run_optimization(campaign, data_dir, invocation_limit=1)
            self.assertEqual(search["phase"], "completed")
            self.assertNotIn("confirmation", search)
            database = Database(data_dir / "phase26-completion" / "campaign.db")
            self.assertIsNone(database.confirmation("phase26-completion"))
            state = database.optimizer_state("phase26-completion")["state"]
            self.assertEqual(state["bayesian_best_parameters"]["parameters"], [
                {"name": "x", "value": 0},
                {"name": "unchanged", "value": 9},
            ])

            first_pair = run_optimization(campaign, data_dir, invocation_limit=1)
            self.assertEqual(first_pair["confirmation"]["status"], "running")
            self.assertEqual(first_pair["confirmation"]["games"], 2)
            snapshot = DashboardReader(data_dir, "phase26-completion").snapshot()
            self.assertEqual(snapshot["campaign"]["status"], "confirming")
            self.assertEqual(snapshot["final_anchor"]["source"], "bayesian_checkpoint_candidate")
            self.assertEqual(snapshot["final_anchor"]["values"], {"x": 0, "unchanged": 9})

            last_pair = run_optimization(campaign, data_dir, invocation_limit=1)
            self.assertEqual(last_pair["confirmation"]["status"], "running")
            self.assertEqual(last_pair["confirmation"]["games"], 4)
            completed = run_optimization(campaign, data_dir, invocation_limit=1)
            self.assertEqual(completed["confirmation"]["status"], "completed")
            self.assertEqual(completed["confirmation"]["outcome"], "inconclusive")
            report, _ = final_report(data_dir, "phase26-completion", report_format="json")
            self.assertEqual(report["final_anchor"]["values"], {"x": 0, "unchanged": 9})
            self.assertEqual(report["parameter_differences"], [
                {"name": "x", "baseline": 1, "best": 0, "delta": -1, "changed": True},
                {"name": "unchanged", "baseline": 9, "best": 9, "delta": 0, "changed": False},
            ])
            self.assertEqual(report["search_games"], 256)
            self.assertEqual(report["confirmation_games"], 4)
            self.assertEqual(report["total_games"], 260)
            with database._read() as connection:
                counts = (
                    connection.execute("SELECT COUNT(*) FROM bayesian_observations").fetchone()[0],
                    connection.execute("SELECT COUNT(*) FROM confirmation_blocks").fetchone()[0],
                    connection.execute("SELECT COUNT(*) FROM confirmation_games").fetchone()[0],
                )
            replay = run_optimization(campaign, data_dir, invocation_limit=1)
            self.assertEqual(replay["confirmation"]["status"], "completed")
            with database._read() as connection:
                replayed_counts = (
                    connection.execute("SELECT COUNT(*) FROM bayesian_observations").fetchone()[0],
                    connection.execute("SELECT COUNT(*) FROM confirmation_blocks").fetchone()[0],
                    connection.execute("SELECT COUNT(*) FROM confirmation_games").fetchone()[0],
                )
            self.assertEqual(replayed_counts, counts)


class Phase26MinimalRealBayesianTest(unittest.TestCase):
    """Run and restart two real Bayesian evaluations through Fastchess."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.repo_root = Path(__file__).parents[2].resolve()
        cls.go = shutil.which("go")
        cls.fastchess = cls.repo_root.parent / "GoAlaric" / ".tools" / "fastchess" / "bin" / "fastchess"
        if cls.go is None or not cls.fastchess.exists():
            raise unittest.SkipTest("Go and the local Fastchess binary are required")
        cls.build_dir = Path(tempfile.mkdtemp(prefix="goalaric-phase26-build-"))
        cls.testmonitor = cls.build_dir / "testmonitor"
        cls.engine = cls.build_dir / "goalaric"
        subprocess.run(
            [cls.go, "build", "-o", str(cls.testmonitor), "./cmd/testmonitor"],
            cwd=cls.repo_root,
            check=True,
        )
        subprocess.run(
            [cls.go, "build", "-o", str(cls.engine), "."],
            cwd=cls.repo_root,
            check=True,
        )

    @classmethod
    def tearDownClass(cls) -> None:
        shutil.rmtree(cls.build_dir, ignore_errors=True)

    def test_two_candidates_resume_with_exact_budget_and_node_transport(self) -> None:
        with tempfile.TemporaryDirectory(prefix="goalaric-phase26-real-") as raw_dir:
            root = Path(raw_dir)
            data_dir = root / "campaigns"
            openings = root / "openings.epd"
            openings.write_text(
                "".join(
                    f"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1 id phase26-{index}\n"
                    for index in range(100)
                ),
                encoding="utf-8",
            )
            campaign = root / "campaign.json"
            campaign.write_text(
                json.dumps(
                    {
                        "schema_version": 1,
                        "campaign_id": "phase26-real",
                        "name": "Phase 26 real Bayesian",
                        "mode": "real",
                        "registry": str(
                            self.repo_root / "optimizer" / "registries" / "search-hpo-v1.json"
                        ),
                        "baseline": {"engine_id": str(self.engine)},
                        "master_seed": 26026,
                        "partitions": {"search": {"name": "search"}},
                        "goals": {
                            "max_games": 8,
                            "max_evaluations": 2,
                            "optimizer": {
                                "algorithm": "finite-noise-aware-bo-v1",
                                "parameters": ["lmr_divisor_x100", "lmp_move_multiplier"],
                                "initial_points": 3,
                                "pairs_per_evaluation": 2,
                                "profile": "node-smoke",
                            },
                            "real": {
                                "testmonitor_command": [str(self.testmonitor)],
                                "fastchess": str(self.fastchess),
                                "opening_book": str(openings),
                                "workdir": str(self.repo_root),
                                "tc": "0.2+0.01",
                                "profiles": {"node-smoke": {"nodes": 20000}},
                                "concurrency": 1,
                                "hash_mb": 16,
                                "threads": 1,
                                "syzygy_path": "off",
                            },
                        },
                    }
                ),
                encoding="utf-8",
            )

            first = run_optimization(campaign, data_dir, invocation_limit=1)
            self.assertEqual(first["phase"], "bayesian")
            self.assertEqual(first["result_count"], 1)
            final = run_optimization(campaign, data_dir, invocation_limit=1)
            self.assertEqual(final["phase"], "completed")
            self.assertEqual(final["result_count"], 2)
            snapshot = DashboardReader(data_dir, "phase26-real").snapshot()
            self.assertTrue(snapshot["final_anchor"]["values"])
            self.assertEqual(snapshot["final_anchor"]["source"], "bayesian_checkpoint_candidate")

            database_path = data_dir / "phase26-real" / "campaign.db"
            with sqlite3.connect(database_path) as connection:
                self.assertEqual(connection.execute("SELECT COUNT(*) FROM bayesian_proposals").fetchone()[0], 2)
                self.assertEqual(connection.execute("SELECT COUNT(*) FROM bayesian_observations").fetchone()[0], 2)
                self.assertEqual(connection.execute("SELECT COUNT(*) FROM match_blocks").fetchone()[0], 4)
                self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 8)
                self.assertEqual(
                    connection.execute("SELECT COUNT(DISTINCT block_id || ':' || game_index) FROM games").fetchone()[0],
                    8,
                )
                self.assertEqual(
                    connection.execute("SELECT COUNT(*) FROM match_blocks WHERE status='running'").fetchone()[0],
                    0,
                )
                self.assertEqual(connection.execute("SELECT MAX(attempt) FROM match_blocks").fetchone()[0], 1)

            run_root = data_dir / "phase26-real" / "runs"
            monitor_configs = sorted(run_root.glob("*/monitor-config.json"))
            self.assertEqual(len(monitor_configs), 4)
            for path in monitor_configs:
                document = json.loads(path.read_text(encoding="utf-8"))
                self.assertEqual(document["nodes"], 20000)
                self.assertNotIn("time_control", document)

            before = (
                final["checkpoint"]["revision"],
                len(list(run_root.glob("*/monitor-config.json"))),
            )
            replay = run_optimization(campaign, data_dir, invocation_limit=1)
            after = (
                replay["checkpoint"]["revision"],
                len(list(run_root.glob("*/monitor-config.json"))),
            )
            self.assertEqual(after, before)


if __name__ == "__main__":
    unittest.main()
