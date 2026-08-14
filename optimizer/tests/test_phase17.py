from __future__ import annotations

import hashlib
import json
import sys
import unittest
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.dashboard import DashboardReader, final_report


class Phase17LegacyReportRegressionTest(unittest.TestCase):
    """Read the archived v1.1.1 campaign without changing its SQLite database."""

    CAMPAIGN_ID = "activity-overnight-v1-1-1"
    EXPECTED_ANCHOR = {
        "mobility_weight": 18,
        "mobility_shift": 9,
        "activity_bias": 10,
        "activity_shift": 2,
        "activity_knight_weight": 6,
        "activity_bishop_weight": 2,
        "activity_rook_weight": 7,
        "activity_queen_weight": 2,
    }

    @classmethod
    def setUpClass(cls) -> None:
        cls.repo_root = Path(__file__).parents[2].resolve()
        cls.data_dir = cls.repo_root / "artifacts/v1.1/activity-overnight/campaigns"
        cls.database_path = cls.data_dir / cls.CAMPAIGN_ID / "campaign.db"
        if not cls.database_path.is_file():
            raise AssertionError(f"missing archived v1.1.1 database: {cls.database_path}")

    @staticmethod
    def _sha256(path: Path) -> str:
        digest = hashlib.sha256()
        with path.open("rb") as source:
            for chunk in iter(lambda: source.read(1024 * 1024), b""):
                digest.update(chunk)
        return digest.hexdigest()

    def _assert_compact(self, value: Any) -> None:
        if isinstance(value, dict):
            self.assertNotIn("blocks", value)
            self.assertNotIn("block_ids", value)
            for nested in value.values():
                self._assert_compact(nested)
        elif isinstance(value, list):
            for nested in value:
                self._assert_compact(nested)

    def test_archived_v1_1_1_report_is_read_only_and_semantically_correct(self) -> None:
        before = self._sha256(self.database_path)

        snapshot = DashboardReader(self.data_dir, self.CAMPAIGN_ID).snapshot()
        self.assertTrue(snapshot["read_only"])
        self.assertEqual(snapshot["campaign"]["status"], "completed")
        self.assertTrue(snapshot["campaign"]["finished"])
        self.assertEqual(snapshot["final_anchor"]["source"], "optimizer_checkpoint")
        self.assertEqual(snapshot["final_anchor"]["values"], self.EXPECTED_ANCHOR)
        self.assertEqual(snapshot["highest_local_trial"]["trial_id"], "trial-000020")
        self.assertEqual(snapshot["highest_local_trial"]["metrics"]["score_percent"], 61.71875)

        confirmation = snapshot["confirmation"]
        self.assertIsNotNone(confirmation)
        assert confirmation is not None
        self.assertEqual(confirmation["candidate_parameter_hash"], snapshot["final_anchor"]["parameter_hash"])
        self.assertEqual(confirmation["candidate_values"], self.EXPECTED_ANCHOR)
        self.assertEqual(
            (confirmation["wins"], confirmation["draws"], confirmation["losses"]),
            (255, 295, 250),
        )
        self.assertEqual(confirmation["metrics"]["games"], 800)
        self.assertEqual(confirmation["outcome"], "inconclusive")
        self.assertIsNone(confirmation["recommendation"])
        self.assertIsNone(confirmation["recommendation_parameter_file"])
        self.assertEqual((snapshot["search_games"], snapshot["confirmation_games"], snapshot["total_games"]), (6336, 800, 7136))
        self.assertLess(snapshot["times"]["search_finished_at"], snapshot["times"]["confirmation_finished_at"])

        report, content = final_report(self.data_dir, self.CAMPAIGN_ID, "json")
        self.assertEqual(json.loads(content), report)
        self.assertEqual(report["parameter_differences"], report["confirmation"]["parameter_differences"])
        self._assert_compact(report)

        detail_report, _ = final_report(self.data_dir, self.CAMPAIGN_ID, "json", detail=True)
        self.assertIn("blocks", detail_report["confirmation"])
        self.assertIn("block_ids", detail_report["campaign_metrics"])
        self.assertEqual(self._sha256(self.database_path), before)


if __name__ == "__main__":
    unittest.main()
