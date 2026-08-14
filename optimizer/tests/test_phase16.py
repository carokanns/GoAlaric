from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.coordinate import CoordinateSearchError, MultiResolutionCoordinateSearch
from goalaric_optimizer.database import Database
from goalaric_optimizer.optimization import _settings
from goalaric_optimizer.registry import load_registry
from goalaric_optimizer.service import init_campaign


class Phase16ExploratorySearchTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-phase16-")
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry_path = self.root / "registry.json"
        self.registry_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "phase16-exploratory-registry-v1",
                    "parameters": [
                        {"name": "p", "value": 0, "min": 0, "max": 2, "step": 1, "min_step": 1}
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
                    "campaign_id": "phase16-exploratory",
                    "name": "Phase 16 exploratory search",
                    "mode": "fake",
                    "registry": str(self.registry_path),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 1616,
                    "partitions": {"optimization": {"name": "optimization"}},
                    "goals": {
                        "optimizer": {
                            "parameters": ["p"],
                            "exploratory": {"enabled": True, "min_score": 51.0},
                        }
                    },
                }
            ),
            encoding="utf-8",
        )
        init_campaign(self.campaign_path, self.data_dir)
        self.database = Database(self.data_dir / "phase16-exploratory" / "campaign.db")
        self.registry = load_registry(self.registry_path)

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    @staticmethod
    def _point_evaluator(score: float):
        def evaluate(parameters: dict[str, object], seed: int) -> dict[str, object]:
            del seed
            value = int(parameters["parameters"][0]["value"])  # type: ignore[index]
            if value == 0:
                wins, draws, losses = 50, 0, 50
            else:
                wins, draws, losses = int(score), 0, 100 - int(score)
            return {
                "wins": wins,
                "draws": draws,
                "losses": losses,
                "score": score if value else 50.0,
                "uncertainty": 10.0,
                "uncertain": True,
                "decision": "uncertain",
            }

        return evaluate

    def _search(self, score: float, exploratory: bool) -> MultiResolutionCoordinateSearch:
        return MultiResolutionCoordinateSearch(
            self.database,
            "phase16-exploratory",
            self.registry,
            self._point_evaluator(score),
            max_passes=1,
            parameter_names=["p"],
            exploratory=exploratory,
            exploratory_min_score=51.0,
        )

    def test_exploratory_accepts_point_score_after_max_budget_and_resumes(self) -> None:
        search = self._search(52.0, exploratory=True)
        first = search.run(max_results=1)
        self.assertEqual(first["result_count"], 1)
        self.assertEqual(first["phase"], "coordinate")

        resumed = self._search(52.0, exploratory=True)
        resumed.run(max_results=1)
        final = resumed.run()

        self.assertEqual(final["phase"], "completed")
        self.assertEqual(final["search_mode"], "exploratory")
        self.assertEqual(final["exploratory"], {"enabled": True, "min_score": 51.0})
        self.assertEqual(final["best"]["parameters"]["parameters"][0]["value"], 1)
        self.assertEqual(final["best"]["result"]["decision"], "accept_exploratory")
        self.assertTrue(final["best"]["result"]["exploratory"])
        self.assertEqual(final["best"]["result"]["exploratory_threshold"], 51.0)
        self.assertEqual(len(final["evaluated_parameter_hashes"]), 2)
        self.assertEqual(len(set(final["evaluated_parameter_hashes"])), 2)

        stored_row = next(
            row
            for row in self.database.list_trials("phase16-exploratory", 10)
            if json.loads(row["result_json"])["parameter"]["parameters"][0]["value"] == 1
        )
        stored = json.loads(stored_row["result_json"])
        self.assertEqual(stored["decision"], "accept_exploratory")
        self.assertTrue(stored["exploratory"])

    def test_exploratory_rejects_score_at_or_below_threshold(self) -> None:
        final = self._search(50.5, exploratory=True).run()
        self.assertEqual(final["best"]["parameters"]["parameters"][0]["value"], 0)
        self.assertEqual(final["last_result"]["decision"], "reject_exploratory")
        self.assertEqual(final["last_result"]["classification"], "loss")
        self.assertTrue(final["last_result"]["exploratory"])

    def test_strict_mode_keeps_uncertain_result_out_of_anchor(self) -> None:
        final = self._search(52.0, exploratory=False).run()
        self.assertEqual(final["search_mode"], "strict")
        self.assertEqual(final["best"]["parameters"]["parameters"][0]["value"], 0)
        self.assertEqual(final["last_result"]["decision"], "uncertain")
        self.assertEqual(final["last_result"]["classification"], "uncertain")
        self.assertNotIn("exploratory", final["last_result"])

    def test_policy_is_checkpointed_and_cannot_change_on_resume(self) -> None:
        self._search(52.0, exploratory=True).run(max_results=1)
        with self.assertRaises(CoordinateSearchError):
            self._search(52.0, exploratory=False).run(max_results=1)

    def test_optimizer_config_parses_exploratory_mode(self) -> None:
        definition, _, _ = init_campaign(self.campaign_path, self.data_dir)
        settings = _settings(self.database, definition.campaign_id, self.registry)
        self.assertTrue(settings.exploratory)
        self.assertEqual(settings.exploratory_min_score, 51.0)


if __name__ == "__main__":
    unittest.main()
