from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.cli import main
from goalaric_optimizer.optimization import run_fake_optimization


class Phase12FakeOptimizationTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry = self.root / "registry.json"
        self.registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "phase12-registry-v1",
                    "parameters": [
                        {"name": "a", "value": 4, "min": 0, "max": 16, "step": 8, "min_step": 2},
                        {"name": "b", "value": 8, "min": 0, "max": 16, "step": 8, "min_step": 2},
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
                    "campaign_id": "fake-phase12",
                    "name": "Fake autonomous optimization",
                    "mode": "fake",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 12012,
                    "partitions": {"adaptive": {"name": "adaptive"}},
                    "goals": {
                        "max_games": 1000,
                        "optimizer": {"parameters": ["a", "b"], "max_passes": 20},
                        "adaptive": {"min_blocks": 1, "max_blocks": 2, "weak_upper_score": 45.0},
                        "fake_match": {"optimum": {"a": 6, "b": 10}},
                    },
                }
            ),
            encoding="utf-8",
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def test_one_command_resumes_search_and_never_duplicates_match_data(self) -> None:
        first = run_fake_optimization(self.campaign, self.data_dir, invocation_limit=2)
        self.assertEqual(first["phase"], "coordinate")
        self.assertIsNone(first["stop_reason"])
        second = run_fake_optimization(self.campaign, self.data_dir, invocation_limit=2)
        self.assertGreater(second["result_count"], first["result_count"])
        final = run_fake_optimization(self.campaign, self.data_dir)

        self.assertEqual(final["phase"], "completed")
        self.assertEqual(final["best"]["parameters"]["parameters"][0]["value"], 6)
        self.assertEqual(final["best"]["parameters"]["parameters"][1]["value"], 10)
        self.assertEqual(final["campaign"]["status"], "completed")
        database_path = self.data_dir / "fake-phase12" / "campaign.db"
        import sqlite3

        with sqlite3.connect(database_path) as connection:
            parameter_hashes = [row[0] for row in connection.execute("SELECT parameter_hash FROM parameter_sets")]
            block_counts = connection.execute(
                "SELECT COUNT(*), COALESCE(SUM(wins+draws+losses),0) FROM match_blocks WHERE status='completed'"
            ).fetchone()
            game_count = connection.execute("SELECT COUNT(*) FROM games").fetchone()[0]
        self.assertEqual(len(parameter_hashes), len(set(parameter_hashes)))
        self.assertEqual(block_counts[1], game_count)
        self.assertGreater(len(list((self.data_dir / "fake-phase12" / "candidates").glob("*.json"))), 1)

    def test_match_budget_stops_autonomously_and_is_idempotent(self) -> None:
        report = run_fake_optimization(self.campaign, self.data_dir, max_games_override=10)
        self.assertEqual(report["phase"], "completed")
        self.assertEqual(report["stop_reason"], "match_budget_exhausted")
        self.assertLessEqual(report["campaign"]["games"], 10)
        before_revision = report["checkpoint"]["revision"]
        repeated = run_fake_optimization(self.campaign, self.data_dir, max_games_override=10)
        self.assertEqual(repeated["stop_reason"], "match_budget_exhausted")
        self.assertEqual(repeated["checkpoint"]["revision"], before_revision)

    def test_optimize_command_uses_campaign_file_and_work_quota(self) -> None:
        result = main(
            [
                "optimize",
                str(self.campaign),
                "--data-dir",
                str(self.data_dir),
                "--max-results",
                "1",
            ]
        )
        self.assertEqual(result, 0)


if __name__ == "__main__":
    unittest.main()
