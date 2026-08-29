from __future__ import annotations

import json
import sqlite3
import tempfile
import unittest
from pathlib import Path

from goalaric_optimizer.bayesian_optimization import (
    BayesianOptimizer,
    BayesianRunSettings,
    DeterministicFakePairRunner,
)
from goalaric_optimizer.database import CampaignConflict, Database, SCHEMA_VERSION
from goalaric_optimizer.optimization import run_fake_optimization
from goalaric_optimizer.service import init_campaign


class Phase24BayesianAskTellTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-phase24-")
        self.root = Path(self.tempdir.name)
        self.registry = self.root / "registry.json"
        self.registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "phase24-grid-v1",
                    "parameters": [
                        {"name": "x", "value": 2, "min": 0, "max": 4, "step": 1, "min_step": 1},
                        {"name": "y", "value": 2, "min": 0, "max": 4, "step": 1, "min_step": 1},
                    ],
                }
            ),
            encoding="utf-8",
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _campaign(self, directory: str) -> tuple[Database, object]:
        campaign = self.root / directory / "campaign.json"
        campaign.parent.mkdir()
        campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "phase24-restart",
                    "name": "Phase 24 restart",
                    "mode": "fake",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 24024,
                    "partitions": {"fake": {"name": "fake"}},
                    "goals": {},
                }
            ),
            encoding="utf-8",
        )
        data_dir = self.root / directory / "data"
        definition, _, database_path = init_campaign(campaign, data_dir)
        return Database(database_path), definition

    @staticmethod
    def _objective(values: tuple[int, ...]) -> float:
        x, y = values
        return 0.55 - 0.008 * (x - 4) ** 2 - 0.01 * (y - 1) ** 2

    def _optimizer(self, database: Database, definition: object, **overrides: int) -> tuple[BayesianOptimizer, DeterministicFakePairRunner]:
        settings = BayesianRunSettings(
            seed=17,
            pairs_per_evaluation=overrides.get("pairs_per_evaluation", 8),
            max_evaluations=overrides.get("max_evaluations", 6),
            initial_points=overrides.get("initial_points", 3),
        )
        runner = DeterministicFakePairRunner(self._objective, seed=99)
        return BayesianOptimizer(database, definition.campaign_id, definition.registry, settings, runner), runner

    @staticmethod
    def _stable_report(report: dict[str, object]) -> dict[str, object]:
        return {
            "phase": report["phase"],
            "values": [item["values"] for item in report["proposals"]],
            "scores": [item["score"] for item in report["observations"]],
            "pair_points": [item["pair_points"] for item in report["observations"]],
            "state": report["checkpoint"]["state"],
            "checkpoint_hash": report["checkpoint"]["checkpoint_hash"],
        }

    def test_repeated_single_result_restarts_equal_uninterrupted_run(self) -> None:
        direct_db, direct_definition = self._campaign("direct")
        direct, _ = self._optimizer(direct_db, direct_definition)
        direct_report = direct.run()

        restart_db, restart_definition = self._campaign("restart")
        restart_calls = 0
        for _ in range(6):
            resumed, runner = self._optimizer(restart_db, restart_definition)
            report = resumed.run(max_results=1)
            restart_calls += runner.calls
        self.assertEqual(restart_calls, 6)
        self.assertEqual(self._stable_report(report), self._stable_report(direct_report))
        self.assertEqual(len({item["parameter_hash"] for item in report["proposals"]}), 6)
        self.assertEqual(sum(item["games"] for item in report["observations"]), 96)

    def test_interruption_after_ask_reuses_proposal_without_duplicate_result(self) -> None:
        database, definition = self._campaign("after-ask")
        optimizer, runner = self._optimizer(database, definition)
        pending = optimizer.ask()
        self.assertEqual(runner.calls, 0)

        resumed, resumed_runner = self._optimizer(database, definition)
        self.assertEqual(resumed.ask()["proposal_id"], pending["proposal_id"])
        report = resumed.run(max_results=1)
        self.assertEqual(resumed_runner.calls, 1)
        self.assertEqual(report["result_count"], 1)
        self.assertEqual(len(report["proposals"]), 1)
        self.assertEqual(len(report["observations"]), 1)
        self.assertEqual(report["proposals"][0]["status"], "completed")

        replay, replay_runner = self._optimizer(database, definition)
        replay.run(max_results=1)
        self.assertEqual(replay_runner.calls, 1)
        self.assertEqual(len(database.bayesian_observations(definition.campaign_id)), 2)

    def test_max_results_counts_only_after_atomic_tell_checkpoint(self) -> None:
        database, definition = self._campaign("quota")
        optimizer, runner = self._optimizer(database, definition)
        first = optimizer.run(max_results=1)
        self.assertEqual(first["processed"], 1)
        self.assertEqual(first["result_count"], 1)
        self.assertEqual(first["checkpoint"]["state"]["consumed_games"], 16)
        self.assertEqual(runner.calls, 1)
        self.assertIsNone(database.pending_bayesian_proposal(definition.campaign_id))

    def test_replayed_tell_is_idempotent_and_does_not_advance_checkpoint(self) -> None:
        database, definition = self._campaign("tell-replay")
        optimizer, runner = self._optimizer(database, definition)
        proposal = optimizer.ask()
        result = runner(proposal)
        optimizer.tell(proposal, result)
        first = database.optimizer_state(definition.campaign_id)
        optimizer.tell(proposal, result)
        replayed = database.optimizer_state(definition.campaign_id)
        self.assertEqual(replayed["revision"], first["revision"])
        self.assertEqual(replayed["checkpoint_hash"], first["checkpoint_hash"])
        self.assertEqual(len(database.bayesian_observations(definition.campaign_id)), 1)

    def test_restart_rejects_changed_model_identity(self) -> None:
        database, definition = self._campaign("identity")
        optimizer, _ = self._optimizer(database, definition)
        optimizer.run(max_results=1)
        with self.assertRaises(CampaignConflict):
            self._optimizer(database, definition, pairs_per_evaluation=10)

    def test_schema_five_database_migrates_without_losing_campaign(self) -> None:
        database, definition = self._campaign("migration")
        with sqlite3.connect(database.path) as connection:
            connection.execute("DROP TABLE bayesian_observations")
            connection.execute("DROP TABLE bayesian_proposals")
            connection.execute("UPDATE schema_meta SET value='5' WHERE key='schema_version'")
        legacy_snapshot = database.status_snapshot(definition.campaign_id)
        self.assertIsNone(legacy_snapshot["bayesian"])
        database.initialize()
        with sqlite3.connect(database.path) as connection:
            version = int(connection.execute(
                "SELECT value FROM schema_meta WHERE key='schema_version'"
            ).fetchone()[0])
            proposal_table = connection.execute(
                "SELECT name FROM sqlite_master WHERE type='table' AND name='bayesian_proposals'"
            ).fetchone()
        self.assertEqual(version, SCHEMA_VERSION)
        self.assertIsNotNone(proposal_table)
        self.assertEqual(database.campaign(definition.campaign_id)["name"], "Phase 24 restart")

    def test_existing_optimize_entrypoint_runs_and_resumes_fake_bayesian_mode(self) -> None:
        campaign = self.root / "cli-campaign.json"
        campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "phase24-cli",
                    "name": "Phase 24 CLI",
                    "mode": "fake",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 24124,
                    "partitions": {"fake": {"name": "fake"}},
                    "goals": {
                        "max_evaluations": 3,
                        "optimizer": {
                            "algorithm": "finite-noise-aware-bo-v1",
                            "parameters": ["x", "y"],
                            "initial_points": 2,
                            "pairs_per_evaluation": 8,
                        },
                        "fake_match": {"optimum": {"x": 4, "y": 1}},
                    },
                }
            ),
            encoding="utf-8",
        )
        data_dir = self.root / "cli-data"
        first = run_fake_optimization(campaign, data_dir, invocation_limit=1)
        self.assertEqual(first["processed"], 1)
        self.assertEqual(first["phase"], "bayesian")
        second = run_fake_optimization(campaign, data_dir, invocation_limit=1)
        self.assertEqual(second["result_count"], 2)
        final = run_fake_optimization(campaign, data_dir)
        self.assertEqual(final["phase"], "completed")
        self.assertEqual(final["result_count"], 3)
        database = Database(data_dir / "phase24-cli" / "campaign.db")
        self.assertEqual(database.campaign("phase24-cli")["status"], "completed")
        self.assertEqual(len(database.bayesian_proposals("phase24-cli")), 3)


if __name__ == "__main__":
    unittest.main()
