from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from goalaric_optimizer.adaptive import AdaptiveError, FakeBlockRunner
from goalaric_optimizer.bayesian_optimization import BayesianOptimizer, BayesianRunSettings
from goalaric_optimizer.database import Database
from goalaric_optimizer.optimization import FixedPairBayesianEvaluator
from goalaric_optimizer.profiles import MatchProfile
from goalaric_optimizer.service import init_campaign


class _StopAfterFirstBlock:
    def __init__(self, delegate: FakeBlockRunner) -> None:
        self.delegate = delegate

    def run(self, block: dict[str, object]) -> dict[str, object]:
        if int(block["block_index"]) > 0:
            raise AdaptiveError("simulated process stop")
        return self.delegate.run(block)


class Phase25BayesianFixedPairTransportTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-phase25-")
        self.root = Path(self.tempdir.name)
        self.registry_path = self.root / "registry.json"
        self.registry_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "phase25-v1",
                    "parameters": [
                        {"name": "x", "value": 2, "min": 0, "max": 4, "step": 1, "min_step": 1}
                    ],
                }
            ),
            encoding="utf-8",
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _campaign(self, name: str) -> tuple[Database, object]:
        campaign = self.root / f"{name}.json"
        campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": name,
                    "name": name,
                    "mode": "fake",
                    "registry": str(self.registry_path),
                    "baseline": {
                        "engine_id": "fake-engine",
                        "parameters": {
                            "schema_version": 1,
                            "registry": "phase25-v1",
                            "parameters": [{"name": "x", "value": 3}],
                        },
                    },
                    "master_seed": 25025,
                    "partitions": {"search": {"name": "search"}},
                    "goals": {},
                }
            ),
            encoding="utf-8",
        )
        definition, _, path = init_campaign(campaign, self.root / "data")
        return Database(path), definition

    @staticmethod
    def _results() -> list[dict[str, object]]:
        return [
            {"wins": 2, "draws": 0, "losses": 0},
            {"wins": 0, "draws": 2, "losses": 0},
            {"wins": 0, "draws": 0, "losses": 2},
        ]

    def _controller(
        self,
        database: Database,
        definition: object,
        stop_after_first: bool = False,
    ) -> BayesianOptimizer:
        profile = MatchProfile.create("node-search", source="test", nodes=100000)

        def runner_factory(candidate, proposal):
            runner = FakeBlockRunner(database, definition.campaign_id, self._results())
            return (_StopAfterFirstBlock(runner) if stop_after_first else runner), None

        evaluator = FixedPairBayesianEvaluator(
            database,
            definition.campaign_id,
            3,
            25025,
            runner_factory,
            profile,
        )
        return BayesianOptimizer(
            database,
            definition.campaign_id,
            definition.registry,
            BayesianRunSettings(
                seed=25025,
                pairs_per_evaluation=3,
                max_evaluations=1,
                initial_points=2,
                exact_baseline_prior=True,
                exact_baseline_values=(3,),
            ),
            evaluator,
        )

    def test_exact_baseline_prior_skips_self_match_and_transports_fixed_pairs(self) -> None:
        database, definition = self._campaign("phase25-transport")
        report = self._controller(database, definition).run()
        self.assertEqual(report["phase"], "completed")
        self.assertEqual(report["result_count"], 1)
        self.assertNotEqual(
            report["proposals"][0]["parameter_hash"],
            database.campaign(definition.campaign_id)["baseline_parameter_hash"],
        )
        observation = report["observations"][0]
        self.assertEqual(observation["pair_points"], [2.0, 1.0, 0.0])
        self.assertEqual(observation["games"], 6)
        self.assertEqual(observation["result"]["profile"]["nodes"], 100000)
        self.assertEqual(len(database.list_trials(definition.campaign_id)), 1)
        snapshot = database.status_snapshot(definition.campaign_id)
        self.assertEqual(snapshot["games"], 6)
        self.assertEqual(snapshot["bayesian"]["games"], 6)

    def test_interrupted_fixed_pair_evaluation_resumes_without_duplicate_blocks(self) -> None:
        database, definition = self._campaign("phase25-restart")
        with self.assertRaisesRegex(AdaptiveError, "simulated process stop"):
            self._controller(database, definition, stop_after_first=True).run(max_results=1)
        self.assertIsNotNone(database.pending_bayesian_proposal(definition.campaign_id))

        report = self._controller(database, definition).run(max_results=1)
        self.assertEqual(report["result_count"], 1)
        with database._read() as connection:
            counts = connection.execute(
                "SELECT COUNT(*),COUNT(DISTINCT block_index),SUM(wins+draws+losses),MAX(attempt) "
                "FROM match_blocks WHERE campaign_id=?",
                (definition.campaign_id,),
            ).fetchone()
            games = connection.execute(
                "SELECT COUNT(*) FROM games WHERE campaign_id=?", (definition.campaign_id,)
            ).fetchone()[0]
        self.assertEqual(tuple(counts), (3, 3, 6, 1))
        self.assertEqual(games, 6)


if __name__ == "__main__":
    unittest.main()
