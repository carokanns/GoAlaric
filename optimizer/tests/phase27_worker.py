"""Fresh-process worker used by the phase-27 restart stress test."""

from __future__ import annotations

import argparse
import os
import time
from pathlib import Path

from goalaric_optimizer.adaptive import FakeBlockRunner
from goalaric_optimizer.bayesian_optimization import BayesianOptimizer, BayesianRunSettings
from goalaric_optimizer.database import Database
from goalaric_optimizer.optimization import FixedPairBayesianEvaluator
from goalaric_optimizer.profiles import MatchProfile
from goalaric_optimizer.registry import load_registry


def _pair_result(selector: int) -> dict[str, int]:
    outcomes = (
        {"wins": 2, "draws": 0, "losses": 0},
        {"wins": 1, "draws": 1, "losses": 0},
        {"wins": 0, "draws": 2, "losses": 0},
        {"wins": 0, "draws": 1, "losses": 1},
        {"wins": 0, "draws": 0, "losses": 2},
    )
    return dict(outcomes[selector % len(outcomes)])


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--database", type=Path, required=True)
    parser.add_argument("--registry", type=Path, required=True)
    parser.add_argument("--campaign-id", required=True)
    parser.add_argument("--hold-after-claim", action="store_true")
    args = parser.parse_args()

    database = Database(args.database)
    database.recover_abandoned_jobs(args.campaign_id)
    profile = MatchProfile.create(
        "node-stress", source="real.profiles.node-stress", nodes=1000
    )
    registry = load_registry(args.registry)

    if args.hold_after_claim:
        original_claim = Database.claim_next_block

        def delayed_claim(instance: Database, campaign_id: str):
            block = original_claim(instance, campaign_id)
            if block is not None:
                time.sleep(30.0)
            return block

        Database.claim_next_block = delayed_claim  # type: ignore[method-assign]

    def runner_factory(candidate, proposal):
        digest = bytes.fromhex(str(proposal["parameter_hash"]))
        results = [_pair_result(digest[index % len(digest)] + index) for index in range(6)]
        return FakeBlockRunner(database, args.campaign_id, results), None

    evaluator = FixedPairBayesianEvaluator(
        database, args.campaign_id, 6, 27027, runner_factory, profile
    )
    controller = BayesianOptimizer(
        database,
        args.campaign_id,
        registry,
        BayesianRunSettings(
            seed=27027,
            pairs_per_evaluation=6,
            max_evaluations=24,
            initial_points=5,
            parameter_names=("p1", "p2", "p3", "p4"),
            exact_baseline_prior=True,
            exact_baseline_values=(2, 4, 6, 8),
        ),
        evaluator,
    )
    controller.run(max_results=1)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
