from __future__ import annotations

import json
import unittest
from pathlib import Path

from goalaric_optimizer.registry import default_parameter_document, load_registry


class SearchLMPRegistryTest(unittest.TestCase):
    def test_lmp_campaign_registry_has_baseline_and_candidate_range(self) -> None:
        path = Path(__file__).resolve().parents[1] / "registries" / "search-lmp-v1.json"
        registry = load_registry(path)

        self.assertEqual(registry.schema_version, 1)
        self.assertEqual(registry.name, "search-lmp-v1")
        self.assertEqual(
            registry.parameters,
            (
                {
                    "name": "lmp_move_multiplier",
                    "value": 3,
                    "min": 3,
                    "max": 5,
                    "step": 1,
                    "min_step": 1,
                },
            ),
        )
        self.assertEqual(
            default_parameter_document(registry),
            {
                "schema_version": 1,
                "registry": "search-lmp-v1",
                "parameters": [{"name": "lmp_move_multiplier", "value": 3}],
            },
        )


class SearchAspirationRegistryTest(unittest.TestCase):
    def test_aspiration_campaign_registry_has_baseline_and_candidate_range(self) -> None:
        path = Path(__file__).resolve().parents[1] / "registries" / "search-aspiration-v1.json"
        registry = load_registry(path)

        self.assertEqual(registry.schema_version, 1)
        self.assertEqual(registry.name, "search-aspiration-v1")
        self.assertEqual(
            registry.parameters,
            (
                {
                    "name": "aspiration_initial_margin_cp",
                    "value": 10,
                    "min": 5,
                    "max": 30,
                    "step": 5,
                    "min_step": 5,
                },
            ),
        )
        self.assertEqual(
            default_parameter_document(registry),
            {
                "schema_version": 1,
                "registry": "search-aspiration-v1",
                "parameters": [{"name": "aspiration_initial_margin_cp", "value": 10}],
            },
        )


class SearchHPOBaselineRegistryTest(unittest.TestCase):
    def test_combined_registry_uses_confirmed_v124_baseline(self) -> None:
        root = Path(__file__).resolve().parents[1] / "registries"
        registry = load_registry(root / "search-hpo-v1.json")
        expected = [
            {"name": "lmr_divisor_x100", "value": 225},
            {"name": "lmp_move_multiplier", "value": 3},
            {"name": "aspiration_initial_margin_cp", "value": 10},
            {"name": "aspiration_min_depth", "value": 5},
        ]

        self.assertEqual(default_parameter_document(registry)["parameters"], expected)
        checked_in = json.loads((root / "search-hpo-v1-default.json").read_text(encoding="utf-8"))
        self.assertEqual(checked_in["parameters"], expected)


if __name__ == "__main__":
    unittest.main()
