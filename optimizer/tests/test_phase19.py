from __future__ import annotations

import unittest
from pathlib import Path

from goalaric_optimizer.registry import default_parameter_document, load_registry


class SearchLMRRegistryTest(unittest.TestCase):
    def test_first_lmr_campaign_registry_is_single_parameter(self) -> None:
        path = Path(__file__).resolve().parents[1] / "registries" / "search-lmr-v1.json"
        registry = load_registry(path)

        self.assertEqual(registry.schema_version, 1)
        self.assertEqual(registry.name, "search-lmr-v1")
        self.assertEqual(len(registry.parameters), 1)
        self.assertEqual(
            registry.parameters[0],
            {
                "name": "lmr_divisor_x100",
                "value": 225,
                "min": 175,
                "max": 275,
                "step": 25,
                "min_step": 25,
            },
        )
        self.assertEqual(
            default_parameter_document(registry),
            {
                "schema_version": 1,
                "registry": "search-lmr-v1",
                "parameters": [{"name": "lmr_divisor_x100", "value": 225}],
            },
        )


if __name__ == "__main__":
    unittest.main()
