from __future__ import annotations

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
                    "value": 4,
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
                "parameters": [{"name": "lmp_move_multiplier", "value": 4}],
            },
        )


if __name__ == "__main__":
    unittest.main()
