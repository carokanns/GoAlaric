from __future__ import annotations

import unittest
from pathlib import Path

from goalaric_optimizer.registry import default_parameter_document, load_registry


class SearchAspirationDepthRegistryTest(unittest.TestCase):
    def test_depth_campaign_registry_has_baseline_and_candidate_range(self) -> None:
        path = (
            Path(__file__).resolve().parents[1]
            / "registries"
            / "search-aspiration-depth-v1.json"
        )
        registry = load_registry(path)

        self.assertEqual(registry.schema_version, 1)
        self.assertEqual(registry.name, "search-aspiration-depth-v1")
        self.assertEqual(
            registry.parameters,
            (
                {
                    "name": "aspiration_min_depth",
                    "value": 5,
                    "min": 5,
                    "max": 7,
                    "step": 1,
                    "min_step": 1,
                },
            ),
        )
        self.assertEqual(
            default_parameter_document(registry),
            {
                "schema_version": 1,
                "registry": "search-aspiration-depth-v1",
                "parameters": [{"name": "aspiration_min_depth", "value": 5}],
            },
        )


if __name__ == "__main__":
    unittest.main()
