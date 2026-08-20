from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.canonical import sha256_json
from goalaric_optimizer.coordinate import CoordinateSearchError, MultiResolutionCoordinateSearch
from goalaric_optimizer.dashboard import DashboardReader, final_report
from goalaric_optimizer.database import Database
from goalaric_optimizer.optimization import AutonomousOptimizer, _settings, run_optimization
from goalaric_optimizer.registry import load_registry
from goalaric_optimizer.service import init_campaign


class Phase20CoordinateQuotaTest(unittest.TestCase):
    """Regression tests for quota-boundary coordinate state transitions."""

    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-phase20-")
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry_path = self.root / "registry.json"
        self.registry_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "phase20-coordinate-quota-v1",
                    "parameters": [
                        {"name": "p", "value": 0, "min": 0, "max": 1, "step": 1, "min_step": 1}
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
                    "campaign_id": "phase20-coordinate-quota",
                    "name": "Phase 20 coordinate quota",
                    "mode": "fake",
                    "registry": str(self.registry_path),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 20260820,
                    "partitions": {"optimization": {"name": "optimization"}},
                    "goals": {
                        "max_passes": 1,
                        "max_evaluations": 10,
                        "optimizer": {
                            "parameters": ["p"],
                            "exploratory": {"enabled": True, "min_score": 51.0},
                        },
                        "adaptive": {
                            "min_blocks": 1,
                            "max_blocks": 1,
                            "weak_upper_score": 0.0,
                            "target_score": 100.0,
                        },
                        "fake_match": {"optimum": {"p": 1}},
                    },
                }
            ),
            encoding="utf-8",
        )
        init_campaign(self.campaign_path, self.data_dir)
        self.registry = load_registry(self.registry_path)

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def test_fake_max_results_one_commits_exploratory_anchor_before_return(self) -> None:
        def fake_runner(parameters: dict[str, object], seed: int) -> dict[str, object]:
            del seed
            value = int(parameters["parameters"][0]["value"])  # type: ignore[index]
            if value == 0:
                return {
                    "wins": 1,
                    "draws": 0,
                    "losses": 1,
                    "score": 50.0,
                    "uncertainty": 0.0,
                    "uncertain": True,
                    "decision": "uncertain",
                }
            return {
                "wins": 2,
                "draws": 0,
                "losses": 0,
                "score": 100.0,
                "uncertainty": 0.0,
                "uncertain": True,
                "decision": "uncertain",
            }

        database = Database(self.data_dir / "phase20-coordinate-quota" / "campaign.db")
        settings = _settings(database, "phase20-coordinate-quota", self.registry)
        first = AutonomousOptimizer(
            database,
            self.data_dir,
            "phase20-coordinate-quota",
            self.registry,
            settings,
            fake_runner,
            invocation_limit=1,
        ).run()
        self.assertEqual(first["result_count"], 1)
        self.assertEqual(first["phase"], "coordinate")

        second = AutonomousOptimizer(
            database,
            self.data_dir,
            "phase20-coordinate-quota",
            self.registry,
            settings,
            fake_runner,
            invocation_limit=1,
        ).run()
        self.assertEqual(second["result_count"], 2)
        self.assertEqual(second["last_result"]["decision"], "accept_exploratory")
        self.assertEqual(second["last_result"]["classification"], "win")
        self.assertEqual(second["best"]["parameters"]["parameters"][0]["value"], 1)
        self.assertEqual(second["best"]["result"]["decision"], "accept_exploratory")
        self.assertEqual(second["phase"], "completed")

        database = Database(self.data_dir / "phase20-coordinate-quota" / "campaign.db")
        state_row = database.optimizer_state("phase20-coordinate-quota")
        state = state_row["state"]
        self.assertEqual(state["anchor_parameters"]["parameters"][0]["value"], 1)
        self.assertEqual(state["anchor_hash"], second["best"]["parameter_hash"])
        self.assertEqual(state_row["revision"], second["checkpoint"]["revision"])

        snapshot = DashboardReader(self.data_dir, "phase20-coordinate-quota").snapshot()
        self.assertEqual(snapshot["final_anchor"]["values"]["p"], 1)
        report, _ = final_report(self.data_dir, "phase20-coordinate-quota", "json")
        self.assertEqual(report["final_anchor"]["values"]["p"], 1)

    def test_budget_after_first_direction_commits_acceptance_idempotently(self) -> None:
        campaign_id = "phase20-partial-coordinate-budget"
        registry_path = self.root / f"{campaign_id}-registry.json"
        registry_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": f"{campaign_id}-v1",
                    "parameters": [
                        {"name": "p", "value": 0, "min": -1, "max": 1, "step": 1, "min_step": 1}
                    ],
                }
            ),
            encoding="utf-8",
        )
        campaign = self.root / f"{campaign_id}.json"
        campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": campaign_id,
                    "name": campaign_id,
                    "mode": "fake",
                    "registry": str(registry_path),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 20260820,
                    "partitions": {"optimization": {"name": "optimization"}},
                    "goals": {
                        "max_evaluations": 2,
                        "max_passes": 1,
                        "optimizer": {
                            "parameters": ["p"],
                            "exploratory": {"enabled": True, "min_score": 51.0},
                        },
                        "fake_match": {"optimum": {"p": 1}},
                    },
                }
            ),
            encoding="utf-8",
        )
        init_campaign(campaign, self.data_dir)
        registry = load_registry(registry_path)
        database = Database(self.data_dir / campaign_id / "campaign.db")
        settings = _settings(database, campaign_id, registry)
        calls: list[int] = []

        def fake_runner(parameters: dict[str, object], seed: int) -> dict[str, object]:
            del seed
            value = int(parameters["parameters"][0]["value"])  # type: ignore[index]
            calls.append(value)
            if value == 0:
                return {
                    "wins": 1,
                    "draws": 0,
                    "losses": 1,
                    "score": 50.0,
                    "uncertainty": 0.0,
                    "decision": "uncertain",
                }
            return {
                "wins": 2,
                "draws": 0,
                "losses": 0,
                "score": 100.0,
                "uncertainty": 0.0,
                "decision": "accept_exploratory",
            }

        first = AutonomousOptimizer(
            database,
            self.data_dir,
            campaign_id,
            registry,
            settings,
            fake_runner,
            invocation_limit=1,
        ).run()
        self.assertEqual(first["result_count"], 1)
        self.assertEqual(first["phase"], "coordinate")

        second = AutonomousOptimizer(
            database,
            self.data_dir,
            campaign_id,
            registry,
            settings,
            fake_runner,
            invocation_limit=1,
        ).run()
        self.assertEqual(second["result_count"], 2)
        self.assertEqual(second["phase"], "coordinate")
        self.assertEqual(second["last_result"]["decision"], "accept_exploratory")
        self.assertEqual(second["best"]["parameters"]["parameters"][0]["value"], 0)

        third = AutonomousOptimizer(
            database,
            self.data_dir,
            campaign_id,
            registry,
            settings,
            fake_runner,
        ).run()
        self.assertEqual(third["phase"], "completed")
        self.assertEqual(third["result_count"], 2)
        self.assertEqual(third["best"]["parameters"]["parameters"][0]["value"], 1)
        self.assertEqual(third["best"]["result"]["decision"], "accept_exploratory")
        self.assertEqual(calls, [0, 1])
        self.assertEqual(database.campaign(campaign_id)["status"], "completed")
        with database._read() as connection:
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM trials").fetchone()[0], 2)

        state_row = database.optimizer_state(campaign_id)
        state = state_row["state"]
        self.assertEqual(state["anchor_parameters"]["parameters"][0]["value"], 1)
        self.assertEqual(state["coordinate_results"], {})
        self.assertEqual(state["stop_reason"], "evaluation_budget_exhausted")

        repeated = AutonomousOptimizer(
            database,
            self.data_dir,
            campaign_id,
            registry,
            settings,
            fake_runner,
        ).run()
        repeated_row = database.optimizer_state(campaign_id)
        self.assertEqual(repeated["best"], third["best"])
        self.assertEqual(repeated_row["revision"], state_row["revision"])
        self.assertEqual(repeated_row["checkpoint_hash"], state_row["checkpoint_hash"])
        self.assertEqual(calls, [0, 1])

    def test_reconcile_pending_selection_is_idempotent_without_evaluation(self) -> None:
        calls: list[int] = []

        def fake_runner(parameters: dict[str, object], seed: int) -> dict[str, object]:
            del seed
            value = int(parameters["parameters"][0]["value"])  # type: ignore[index]
            calls.append(value)
            return {
                "wins": 2 if value else 1,
                "draws": 0,
                "losses": 0 if value else 1,
                "score": 100.0 if value else 50.0,
                "uncertainty": 0.0,
                "uncertain": True,
                "decision": "uncertain",
            }

        database = Database(self.data_dir / "phase20-coordinate-quota" / "campaign.db")
        search = MultiResolutionCoordinateSearch(
            database,
            "phase20-coordinate-quota",
            self.registry,
            fake_runner,
            max_passes=1,
            parameter_names=["p"],
            exploratory=True,
        )
        search.run(max_results=1)
        state = database.optimizer_state("phase20-coordinate-quota")["state"]
        state, produced = search._step(state)
        self.assertTrue(produced)
        state, produced = search._step(state)
        self.assertFalse(produced)
        self.assertEqual(state["direction_index"], 2)
        self.assertEqual(calls, [0, 1])

        first = search.reconcile_checkpoint()
        checkpoint = database.optimizer_state("phase20-coordinate-quota")
        self.assertEqual(first["best"]["parameters"]["parameters"][0]["value"], 1)
        self.assertEqual(checkpoint["state"]["result_count"], 2)
        self.assertEqual(calls, [0, 1])

        second = search.reconcile_checkpoint()
        repeated = database.optimizer_state("phase20-coordinate-quota")
        self.assertEqual(second["best"], first["best"])
        self.assertEqual(repeated["revision"], checkpoint["revision"])
        self.assertEqual(repeated["checkpoint_hash"], checkpoint["checkpoint_hash"])
        self.assertEqual(calls, [0, 1])

    def test_reconcile_after_last_atomic_result_completes_campaign(self) -> None:
        calls: list[int] = []

        def fake_runner(parameters: dict[str, object], seed: int) -> dict[str, object]:
            del seed
            value = int(parameters["parameters"][0]["value"])  # type: ignore[index]
            calls.append(value)
            return {
                "wins": 2 if value else 1,
                "draws": 0,
                "losses": 0 if value else 1,
                "score": 100.0 if value else 50.0,
                "uncertainty": 0.0,
                "decision": "uncertain",
            }

        database = Database(self.data_dir / "phase20-coordinate-quota" / "campaign.db")
        search = MultiResolutionCoordinateSearch(
            database,
            "phase20-coordinate-quota",
            self.registry,
            fake_runner,
            max_passes=1,
            parameter_names=["p"],
            exploratory=False,
        )
        search.run(max_results=1)
        state = database.optimizer_state("phase20-coordinate-quota")["state"]
        state, produced = search._step(state)
        self.assertTrue(produced)
        self.assertEqual(state["direction_index"], 1)
        self.assertEqual(state["result_count"], 2)
        self.assertEqual(calls, [0, 1])

        first = search.reconcile_checkpoint()
        checkpoint = database.optimizer_state("phase20-coordinate-quota")
        self.assertEqual(first["phase"], "completed")
        self.assertEqual(first["best"]["parameters"]["parameters"][0]["value"], 0)
        self.assertEqual(checkpoint["state"]["result_count"], 2)
        self.assertEqual(database.campaign("phase20-coordinate-quota")["status"], "completed")
        self.assertEqual(calls, [0, 1])

        second = search.reconcile_checkpoint()
        repeated = database.optimizer_state("phase20-coordinate-quota")
        self.assertEqual(second["phase"], "completed")
        self.assertEqual(repeated["revision"], checkpoint["revision"])
        self.assertEqual(repeated["checkpoint_hash"], checkpoint["checkpoint_hash"])
        self.assertEqual(calls, [0, 1])

    def test_bounded_final_search_result_defers_confirmation_until_next_invocation(self) -> None:
        campaign_id = "phase20-bounded-confirmation"
        campaign = self.root / f"{campaign_id}.json"
        campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": campaign_id,
                    "name": campaign_id,
                    "mode": "fake",
                    "registry": str(self.registry_path),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 20260820,
                    "partitions": {"optimization": {"name": "optimization"}},
                    "goals": {
                        "max_games": 20,
                        "max_evaluations": 2,
                        "max_passes": 1,
                        "optimizer": {"parameters": ["p"]},
                        "adaptive": {"min_blocks": 1, "max_blocks": 1},
                        "fake_match": {"optimum": {"p": 1}},
                        "confirmation": {
                            "enabled": True,
                            "games": 4,
                            "seed": 20260821,
                            "confidence": 0.95,
                            "fake_result": {"wins": 0, "draws": 4, "losses": 0},
                        },
                    },
                }
            ),
            encoding="utf-8",
        )
        init_campaign(campaign, self.data_dir)
        database = Database(self.data_dir / campaign_id / "campaign.db")

        first = run_optimization(campaign, self.data_dir, invocation_limit=1)
        self.assertEqual(first["phase"], "coordinate")
        self.assertEqual(first["result_count"], 1)
        self.assertIsNone(database.confirmation(campaign_id))
        self.assertEqual(database.campaign(campaign_id)["status"], "running")
        self.assertEqual(database.status_snapshot(campaign_id)["games"], 2)

        second = run_optimization(campaign, self.data_dir, invocation_limit=1)
        self.assertEqual(second["phase"], "completed")
        self.assertEqual(second["result_count"], 2)
        self.assertIsNone(second.get("confirmation"))
        self.assertEqual(database.confirmation(campaign_id), None)
        self.assertEqual(database.campaign(campaign_id)["status"], "completed")
        self.assertEqual(database.status_snapshot(campaign_id)["games"], 4)

        third = run_optimization(campaign, self.data_dir, invocation_limit=1)
        self.assertEqual(third["phase"], "completed")
        self.assertEqual(third["result_count"], 2)
        self.assertEqual(third["confirmation"]["status"], "running")
        self.assertEqual(third["confirmation"]["games"], 2)
        self.assertEqual(database.status_snapshot(campaign_id)["status"], "confirming")
        self.assertEqual(database.optimizer_state(campaign_id)["state"]["result_count"], 2)

        fourth = run_optimization(campaign, self.data_dir)
        self.assertEqual(fourth["phase"], "completed")
        self.assertEqual(fourth["result_count"], 2)
        self.assertEqual(fourth["confirmation"]["status"], "completed")
        finished_confirmation = database.confirmation_snapshot(campaign_id)
        self.assertIsNotNone(finished_confirmation)
        assert finished_confirmation is not None
        self.assertEqual(finished_confirmation["metrics"]["games"], 4)
        self.assertEqual(database.status_snapshot(campaign_id)["status"], "completed")
        with database._read() as connection:
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM trials").fetchone()[0], 2)
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 4)
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM confirmation_games").fetchone()[0], 4)

    def test_accept_and_reject_decisions_move_or_preserve_anchor(self) -> None:
        decisions = {
            "accept": True,
            "accept_exploratory": True,
            "reject": False,
            "reject_early": False,
            "reject_exploratory": False,
            "uncertain": False,
        }
        for decision, moves_anchor in decisions.items():
            with self.subTest(decision=decision):
                campaign_id = f"phase20-{decision.replace('_', '-')}"
                campaign = self.root / f"{campaign_id}.json"
                campaign.write_text(
                    json.dumps(
                        {
                            "schema_version": 1,
                            "campaign_id": campaign_id,
                            "name": campaign_id,
                            "mode": "fake",
                            "registry": str(self.registry_path),
                            "baseline": {"engine_id": "fake-engine"},
                            "master_seed": 20260820,
                            "partitions": {"optimization": {"name": "optimization"}},
                        }
                    ),
                    encoding="utf-8",
                )
                init_campaign(campaign, self.data_dir)
                database = Database(self.data_dir / campaign_id / "campaign.db")

                def evaluate(parameters: dict[str, object], seed: int) -> dict[str, object]:
                    del seed
                    value = int(parameters["parameters"][0]["value"])  # type: ignore[index]
                    if value == 0:
                        return {
                            "wins": 1,
                            "draws": 0,
                            "losses": 1,
                            "score": 50.0,
                            "uncertainty": 0.0,
                            "uncertain": True,
                            "decision": "uncertain",
                            "candidate_objective": 0,
                        }
                    score = 60.0 if moves_anchor else 40.0
                    return {
                        "wins": 6 if moves_anchor else 4,
                        "draws": 0,
                        "losses": 4 if moves_anchor else 6,
                        "score": score,
                        "uncertainty": 0.0,
                        "uncertain": True,
                        "decision": decision,
                        # Deliberately contradict the decision to prove that
                        # decision, not synthetic objective metadata, wins.
                        "candidate_objective": -1 if moves_anchor else 1,
                    }

                result = MultiResolutionCoordinateSearch(
                    database,
                    campaign_id,
                    self.registry,
                    evaluate,
                    max_passes=1,
                    parameter_names=["p"],
                    exploratory=False,
                ).run()
                self.assertEqual(
                    result["best"]["parameters"]["parameters"][0]["value"],
                    1 if moves_anchor else 0,
                )
                self.assertEqual(result["last_result"]["decision"], decision)
                self.assertEqual(
                    result["last_result"]["classification"],
                    "win" if moves_anchor else ("uncertain" if decision == "uncertain" else "loss"),
                )

    def test_candidate_objective_remains_fallback_without_decision(self) -> None:
        anchor = {
            "score": 50.0,
            "uncertainty": 5.0,
            "candidate_objective": 0,
            "parameter_hash": "anchor",
        }
        for objective, expected in ((1, "win"), (-1, "loss"), (0, "uncertain")):
            with self.subTest(objective=objective):
                result = MultiResolutionCoordinateSearch._classify(
                    {
                        "score": 50.0,
                        "uncertainty": 0.0,
                        "candidate_objective": objective,
                        "parameter_hash": "candidate",
                    },
                    anchor,
                    exploratory=False,
                )
                self.assertEqual(result["classification"], expected)

    def test_reused_decision_from_another_anchor_stays_uncertain(self) -> None:
        result = MultiResolutionCoordinateSearch._classify(
            {
                "score": 100.0,
                "uncertainty": 0.0,
                "decision": "accept",
                "reused": True,
                "reference_parameter_hash": "old-anchor",
            },
            {"score": 50.0, "uncertainty": 0.0, "parameter_hash": "new-anchor"},
            exploratory=False,
        )
        self.assertEqual(result["classification"], "uncertain")

    def test_interrupted_evaluation_is_not_counted_and_resumes_same_trial(self) -> None:
        database = Database(self.data_dir / "phase20-coordinate-quota" / "campaign.db")
        interrupted_once = False

        def evaluate(parameters: dict[str, object], seed: int) -> dict[str, object]:
            nonlocal interrupted_once
            del seed
            value = int(parameters["parameters"][0]["value"])  # type: ignore[index]
            if value == 0:
                return {
                    "wins": 1,
                    "draws": 0,
                    "losses": 1,
                    "score": 50.0,
                    "uncertainty": 0.0,
                    "decision": "uncertain",
                }
            parameter_hash = sha256_json(parameters)
            trial = database.trial_for_parameter_hash(
                "phase20-coordinate-quota", parameter_hash
            )
            self.assertIsNotNone(trial)
            assert trial is not None
            if not interrupted_once:
                interrupted_once = True
                database.transition_trial(
                    "phase20-coordinate-quota", trial["trial_id"], "running"
                )
                database.transition_trial(
                    "phase20-coordinate-quota",
                    trial["trial_id"],
                    "interrupted",
                    result={"phase": "interrupted", "decision": "interrupted"},
                )
                return {
                    "wins": 1,
                    "draws": 0,
                    "losses": 1,
                    "score": 50.0,
                    "uncertainty": 0.0,
                    "phase": "interrupted",
                    "decision": "interrupted",
                }
            self.assertEqual(trial["status"], "interrupted")
            database.transition_trial(
                "phase20-coordinate-quota", trial["trial_id"], "running"
            )
            return {
                "wins": 2,
                "draws": 0,
                "losses": 0,
                "score": 100.0,
                "uncertainty": 0.0,
                "phase": "terminal",
                "decision": "accept",
            }

        search = MultiResolutionCoordinateSearch(
            database,
            "phase20-coordinate-quota",
            self.registry,
            evaluate,
            max_passes=1,
            parameter_names=["p"],
        )
        baseline = search.run(max_results=1)
        self.assertEqual(baseline["result_count"], 1)
        with self.assertRaisesRegex(
            CoordinateSearchError, "did not reach a terminal decision"
        ):
            search.run(max_results=1)

        state = database.optimizer_state("phase20-coordinate-quota")["state"]
        self.assertEqual(state["result_count"], 1)
        self.assertEqual(state["anchor_parameters"]["parameters"][0]["value"], 0)
        candidate_hash = sha256_json(
            {
                "schema_version": 1,
                "registry": "phase20-coordinate-quota-v1",
                "parameters": [{"name": "p", "value": 1}],
            }
        )
        trial_before = database.trial_for_parameter_hash(
            "phase20-coordinate-quota", candidate_hash
        )
        self.assertIsNotNone(trial_before)
        assert trial_before is not None
        self.assertEqual(trial_before["status"], "interrupted")

        resumed = search.run(max_results=1)
        trial_after = database.trial_for_parameter_hash(
            "phase20-coordinate-quota", candidate_hash
        )
        self.assertIsNotNone(trial_after)
        assert trial_after is not None
        self.assertEqual(trial_after["trial_id"], trial_before["trial_id"])
        self.assertEqual(trial_after["status"], "completed")
        self.assertEqual(resumed["result_count"], 2)
        self.assertEqual(resumed["best"]["parameters"]["parameters"][0]["value"], 1)


if __name__ == "__main__":
    unittest.main()
