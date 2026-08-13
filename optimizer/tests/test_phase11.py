from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.coordinate import MultiResolutionCoordinateSearch, synthetic_evaluator
from goalaric_optimizer.database import Database
from goalaric_optimizer.cli import main
from goalaric_optimizer.registry import load_registry
from goalaric_optimizer.service import init_campaign


class Phase11Test(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry_path = self.root / "registry.json"
        self.registry_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "coordinate-multires-registry-v1",
                    "parameters": [
                        {"name": "a", "value": 4, "min": 0, "max": 16, "step": 8, "min_step": 2},
                        {"name": "b", "value": 8, "min": 0, "max": 16, "step": 8, "min_step": 2},
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
                    "campaign_id": "fake-phase11",
                    "name": "Fake phase 11",
                    "mode": "fake",
                    "registry": str(self.registry_path),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 111,
                    "partitions": {"training": {"name": "training"}},
                }
            ),
            encoding="utf-8",
        )
        init_campaign(self.campaign_path, self.data_dir)
        self.database = Database(self.data_dir / "fake-phase11" / "campaign.db")
        self.registry = load_registry(self.registry_path)
        self.optimum = {"a": 6, "b": 10}

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _search(self) -> MultiResolutionCoordinateSearch:
        return MultiResolutionCoordinateSearch(
            self.database,
            "fake-phase11",
            self.registry,
            synthetic_evaluator(self.optimum),
            max_passes=20,
        )

    def test_large_steps_halve_to_min_step_and_find_optimum(self) -> None:
        report = self._search().run(max_results=1)
        self.assertEqual(report["result_count"], 1)

        for _ in range(80):
            report = self._search().run(max_results=1)
            if report["phase"] == "completed":
                break
        else:
            self.fail("multi-resolution search did not finish after repeated restarts")

        self.assertEqual(report["campaign"]["status"], "completed")
        self.assertEqual(report["algorithm"], "coordinate-multires-v1")
        self.assertEqual(report["best"]["parameters"]["parameters"], [
            {"name": "a", "value": 6},
            {"name": "b", "value": 10},
        ])
        self.assertEqual(report["step_history"], [
            {"a": 8, "b": 8},
            {"a": 4, "b": 4},
            {"a": 2, "b": 2},
        ])
        self.assertEqual(report["step_by_parameter"], {"a": 2, "b": 2})
        self.assertEqual(len(report["evaluated_parameter_hashes"]), len(set(report["evaluated_parameter_hashes"])))
        self.assertEqual(
            len(self.database.list_trials("fake-phase11", 1000)),
            len(report["evaluated_parameter_hashes"]),
        )
        self.assertEqual(report["best"]["result"]["score"], 95.0)

    def test_checkpoint_restart_is_idempotent_and_does_not_duplicate_trials(self) -> None:
        first = self._search().run(max_results=1)
        for _ in range(80):
            report = self._search().run(max_results=1)
            if report["phase"] == "completed":
                break
        else:
            self.fail("multi-resolution search did not finish")

        terminal = self._search().run()
        self.assertEqual(terminal["phase"], "completed")
        self.assertEqual(terminal["result_count"], report["result_count"])
        self.assertEqual(terminal["evaluated_parameter_hashes"], report["evaluated_parameter_hashes"])
        self.assertGreater(terminal["checkpoint"]["revision"], first["checkpoint"]["revision"])
        with self.database._read() as connection:
            self.assertEqual(
                connection.execute("SELECT COUNT(*) FROM trials WHERE status='completed'").fetchone()[0],
                len(terminal["evaluated_parameter_hashes"]),
            )
            self.assertEqual(
                connection.execute(
                    "SELECT COUNT(DISTINCT parameter_set_id) FROM trials WHERE campaign_id=?",
                    ("fake-phase11",),
                ).fetchone()[0],
                len(terminal["evaluated_parameter_hashes"]),
            )

    def test_selected_parameters_are_explicit(self) -> None:
        search = MultiResolutionCoordinateSearch(
            self.database,
            "fake-phase11",
            self.registry,
            synthetic_evaluator(self.optimum),
            parameter_names=["b"],
        )
        report = search.run()
        self.assertEqual(report["parameter_names"], ["b"])
        self.assertEqual(report["best"]["parameters"]["parameters"], [
            {"name": "a", "value": 4},
            {"name": "b", "value": 10},
        ])

    def test_multiresolution_search_is_available_through_cli(self) -> None:
        self.assertEqual(
            main(
                [
                    "coordinate-multires",
                    "fake-phase11",
                    "--registry",
                    str(self.registry_path),
                    "--fake-optimum",
                    json.dumps(self.optimum),
                    "--parameters",
                    "b",
                    "--max-results",
                    "1",
                    "--data-dir",
                    str(self.data_dir),
                ]
            ),
            0,
        )
        self.assertEqual(
            self.database.optimizer_state("fake-phase11")["state"]["algorithm"],
            "coordinate-multires-v1",
        )


if __name__ == "__main__":
    unittest.main()
