from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.coordinate import CoordinateSearch, synthetic_evaluator
from goalaric_optimizer.cli import main
from goalaric_optimizer.database import Database
from goalaric_optimizer.registry import load_registry
from goalaric_optimizer.service import init_campaign


class Phase7Test(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry_path = self.root / "registry.json"
        self.registry_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "coordinate-registry-v1",
                    "parameters": [
                        {"name": "a", "value": 1, "min": 0, "max": 4, "step": 1},
                        {"name": "b", "value": 2, "min": 0, "max": 4, "step": 1},
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
                    "campaign_id": "fake-phase7",
                    "name": "Fake phase 7",
                    "mode": "fake",
                    "registry": str(self.registry_path),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 777,
                    "partitions": {"training": {"name": "training"}},
                }
            ),
            encoding="utf-8",
        )
        init_campaign(self.campaign_path, self.data_dir)
        self.database = Database(self.data_dir / "fake-phase7" / "campaign.db")
        self.registry = load_registry(self.registry_path)
        self.optimum = {"a": 3, "b": 1}

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _search(self, uncertain_values: set[tuple[str, int]] | None = None) -> CoordinateSearch:
        return CoordinateSearch(
            self.database,
            "fake-phase7",
            self.registry,
            synthetic_evaluator(self.optimum, uncertain_values),
            max_passes=20,
        )

    def test_restarts_reach_known_optimum_without_duplicate_hashes(self) -> None:
        report = self._search().run(max_results=1)
        self.assertEqual(report["result_count"], 1)
        revisions = [report["checkpoint"]["revision"]]

        for _ in range(40):
            report = self._search().run(max_results=1)
            revisions.append(report["checkpoint"]["revision"])
            if report["phase"] == "completed":
                break
        else:
            self.fail("coordinate search did not finish after repeated restarts")

        self.assertEqual(report["campaign"]["status"], "completed")
        self.assertEqual(report["best"]["parameters"]["parameters"], [
            {"name": "a", "value": 3},
            {"name": "b", "value": 1},
        ])
        self.assertEqual(len(report["evaluated_parameter_hashes"]), len(set(report["evaluated_parameter_hashes"])))
        self.assertEqual(len(self.database.list_trials("fake-phase7", 100)), len(report["evaluated_parameter_hashes"]))
        self.assertEqual(report["best"]["result"]["wins"], 90)
        self.assertEqual(report["best"]["result"]["draws"], 10)
        self.assertEqual(report["best"]["result"]["score"], 95.0)
        self.assertTrue(all(later > earlier for earlier, later in zip(revisions, revisions[1:])))
        coordinate_checkpoints = [
            event for event in self.database.events("fake-phase7") if event["event_type"] == "coordinate_checkpoint"
        ]
        self.assertEqual(len(coordinate_checkpoints), report["result_count"])
        with self.database._read() as connection:
            self.assertEqual(
                connection.execute("SELECT COUNT(*) FROM trials WHERE status='completed'").fetchone()[0],
                len(report["evaluated_parameter_hashes"]),
            )

    def test_bounds_are_respected_and_uncertain_is_not_accepted(self) -> None:
        report = self._search({("a", 2)}).run()
        self.assertEqual(report["campaign"]["status"], "completed")
        self.assertEqual(report["best"]["parameters"]["parameters"], [
            {"name": "a", "value": 1},
            {"name": "b", "value": 1},
        ])
        results = [json.loads(item["result_json"]) for item in self.database.list_trials("fake-phase7", 100)]
        classifications = {item["classification"] for item in results}
        self.assertTrue({"baseline", "win", "loss", "uncertain"}.issubset(classifications))
        # Every materialized parameter document remains inside its registry bounds.
        with self.database._read() as connection:
            rows = connection.execute("SELECT document_json FROM parameter_sets").fetchall()
        for row in rows:
            document = json.loads(row["document_json"])
            for item, spec in zip(document["parameters"], self.registry.parameters):
                self.assertGreaterEqual(item["value"], spec["min"])
                self.assertLessEqual(item["value"], spec["max"])

    def test_classification_is_deterministic_for_win_loss_uncertain(self) -> None:
        anchor = {"score": 50.0, "uncertainty": 1.0}
        classify = CoordinateSearch._classify
        self.assertEqual(classify({"wins": 60, "draws": 0, "losses": 40, "score": 60, "uncertainty": 1}, anchor)["classification"], "win")
        self.assertEqual(classify({"wins": 40, "draws": 0, "losses": 60, "score": 40, "uncertainty": 1}, anchor)["classification"], "loss")
        self.assertEqual(classify({"wins": 51, "draws": 0, "losses": 49, "score": 50.5, "uncertainty": 1}, anchor)["classification"], "uncertain")

    def test_synthetic_coordinate_search_is_available_through_cli(self) -> None:
        self.assertEqual(
            main(
                [
                    "coordinate",
                    "fake-phase7",
                    "--registry",
                    str(self.registry_path),
                    "--fake-optimum",
                    json.dumps(self.optimum),
                    "--max-results",
                    "1",
                    "--data-dir",
                    str(self.data_dir),
                ]
            ),
            0,
        )
        self.assertEqual(self.database.optimizer_state("fake-phase7")["state"]["result_count"], 1)


if __name__ == "__main__":
    unittest.main()
