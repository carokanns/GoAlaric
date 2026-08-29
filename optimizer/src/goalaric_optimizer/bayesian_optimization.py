"""Restartable SQLite ask/tell orchestration for noise-aware Bayesian search."""

from __future__ import annotations

import hashlib
import random
from dataclasses import dataclass
from importlib.metadata import PackageNotFoundError, version
from typing import Any, Callable, Sequence

from .bayes import (
    BayesianSearchError,
    Dimension,
    FiniteBayesianSearch,
    Observation,
    pentanomial_score_variance,
)
from .canonical import sha256_json
from .database import CampaignConflict, Database
from .registry import Registry


class BayesianOptimizationError(RuntimeError):
    pass


def _package_version(name: str) -> str:
    try:
        return version(name)
    except PackageNotFoundError:
        return "unavailable"


@dataclass(frozen=True)
class BayesianRunSettings:
    seed: int
    pairs_per_evaluation: int
    max_evaluations: int
    initial_points: int = 9
    parameter_names: tuple[str, ...] | None = None

    def __post_init__(self) -> None:
        if self.pairs_per_evaluation < 2:
            raise BayesianOptimizationError("pairs_per_evaluation must be at least two")
        if self.max_evaluations < 1:
            raise BayesianOptimizationError("max_evaluations must be positive")
        if self.initial_points < 1:
            raise BayesianOptimizationError("initial_points must be positive")


def dimensions_from_registry(
    registry: Registry, parameter_names: Sequence[str] | None = None
) -> tuple[Dimension, ...]:
    selected = tuple(parameter_names) if parameter_names is not None else tuple(
        str(item["name"]) for item in registry.parameters
    )
    if not selected or len(set(selected)) != len(selected):
        raise BayesianOptimizationError("Bayesian parameter names must be non-empty and unique")
    by_name = {str(item["name"]): item for item in registry.parameters}
    dimensions: list[Dimension] = []
    for name in selected:
        item = by_name.get(name)
        if item is None:
            raise BayesianOptimizationError(f"unknown Bayesian parameter: {name}")
        minimum = item.get("min")
        maximum = item.get("max")
        step = item.get("min_step", item.get("step"))
        if not all(isinstance(value, int) and not isinstance(value, bool) for value in (minimum, maximum, step)):
            raise BayesianOptimizationError(f"{name} needs integer min, max and min_step/step")
        if step <= 0 or minimum > maximum or (maximum - minimum) % step:
            raise BayesianOptimizationError(f"{name} has an invalid finite grid")
        values = tuple(range(minimum, maximum + 1, step))
        dimensions.append(Dimension(name, values, int(item["value"])))
    return tuple(dimensions)


class DeterministicFakePairRunner:
    """Generate paired synthetic outcomes without launching any subprocess."""

    def __init__(self, objective: Callable[[tuple[int, ...]], float], seed: int) -> None:
        self.objective = objective
        self.seed = int(seed)
        self.calls = 0

    def __call__(self, proposal: dict[str, Any]) -> dict[str, Any]:
        self.calls += 1
        values = tuple(int(value) for value in proposal["values"])
        probability = float(self.objective(values))
        if not 0.0 <= probability <= 1.0:
            raise BayesianOptimizationError("fake objective must return a score fraction")
        identity = f"{self.seed}:{proposal['proposal_id']}".encode("utf-8")
        sample_seed = int.from_bytes(hashlib.sha256(identity).digest()[:8], "big")
        generator = random.Random(sample_seed)
        pair_points = [
            float((generator.random() < probability) + (generator.random() < probability))
            for _ in range(int(proposal["pairs_requested"]))
        ]
        score, variance = pentanomial_score_variance(pair_points)
        return {
            "runner": "deterministic-fake-pairs-v1",
            "pair_points": pair_points,
            "score": score,
            "variance": variance,
            "pairs": len(pair_points),
            "games": len(pair_points) * 2,
            "sample_seed": sample_seed,
        }


class BayesianOptimizer:
    """Drive one durable ask/tell result at a time from SQLite evidence."""

    def __init__(
        self,
        database: Database,
        campaign_id: str,
        registry: Registry,
        settings: BayesianRunSettings,
        runner: Callable[[dict[str, Any]], dict[str, Any]],
    ) -> None:
        self.database = database
        self.campaign_id = campaign_id
        self.registry = registry
        self.settings = settings
        self.runner = runner
        self.dimensions = dimensions_from_registry(registry, settings.parameter_names)
        self.search = FiniteBayesianSearch(
            self.dimensions, seed=settings.seed, initial_points=settings.initial_points
        )
        self.identity = {
            "algorithm": self.search.ALGORITHM,
            "registry_sha256": registry.sha256,
            "seed": settings.seed,
            "initial_points": settings.initial_points,
            "pairs_per_evaluation": settings.pairs_per_evaluation,
            "max_evaluations": settings.max_evaluations,
            "dimensions": [
                {"name": item.name, "values": list(item.values), "default": item.default}
                for item in self.dimensions
            ],
            "packages": {
                "botorch": _package_version("botorch"),
                "torch": _package_version("torch"),
            },
        }
        self.identity_hash = sha256_json(self.identity)
        self._initialize_or_validate_state()

    def _initialize_or_validate_state(self) -> None:
        checkpoint = self.database.optimizer_state(self.campaign_id)
        state = dict(checkpoint["state"])
        stored = state.get("bayesian_identity")
        if stored is not None:
            if stored != self.identity or state.get("bayesian_identity_hash") != self.identity_hash:
                raise CampaignConflict("Bayesian optimizer identity differs from checkpoint")
            return
        if self.database.bayesian_proposals(self.campaign_id):
            raise CampaignConflict("Bayesian proposals exist without an optimizer identity")
        if self.database.list_trials(self.campaign_id, limit=1):
            raise CampaignConflict("coordinate trials already exist in this campaign")
        new_state = {
            "version": 1,
            "phase": "bayesian",
            "bayesian_identity": self.identity,
            "bayesian_identity_hash": self.identity_hash,
            "result_count": 0,
            "consumed_pairs": 0,
            "consumed_games": 0,
            "last_proposal_id": None,
            "stop_reason": None,
        }
        self.database.checkpoint(self.campaign_id, new_state, event_type="bayesian_initialized")

    def _observations(self) -> list[Observation]:
        return [
            Observation(
                tuple(int(value) for value in row["result"]["values"]),
                float(row["score"]),
                float(row["variance"]),
                int(row["pairs"]),
            )
            for row in self.database.bayesian_observations(self.campaign_id)
        ]

    def _document(self, values: tuple[int, ...]) -> dict[str, Any]:
        selected = dict(zip((item.name for item in self.dimensions), values, strict=True))
        return {
            "schema_version": self.registry.schema_version,
            "registry": self.registry.name,
            "parameters": [
                {"name": str(item["name"]), "value": selected.get(str(item["name"]), int(item["value"]))}
                for item in self.registry.parameters
            ],
        }

    def ask(self) -> dict[str, Any]:
        pending = self.database.pending_bayesian_proposal(self.campaign_id)
        if pending is not None:
            return pending
        observations = self._observations()
        if len(observations) >= self.settings.max_evaluations:
            raise BayesianSearchError("evaluation budget is exhausted")
        values = self.search.ask(observations)
        sequence = len(observations) + 1
        return self.database.create_bayesian_proposal(
            self.campaign_id,
            sequence,
            self._document(values),
            list(values),
            self.settings.pairs_per_evaluation,
            self.settings.seed,
            self.search.ALGORITHM,
        )

    def tell(self, proposal: dict[str, Any], result: dict[str, Any]) -> dict[str, Any]:
        pair_points = result.get("pair_points")
        if not isinstance(pair_points, list):
            raise BayesianOptimizationError("runner result has no pair_points list")
        score, variance = pentanomial_score_variance(pair_points)
        if abs(float(result.get("score", score)) - score) > 1e-12:
            raise BayesianOptimizationError("runner score differs from pair outcomes")
        observations_after = len(self.database.bayesian_observations(self.campaign_id)) + 1
        terminal = observations_after >= min(self.settings.max_evaluations, len(self.search.grid))
        stop_reason = None
        if terminal:
            stop_reason = (
                "search_space_exhausted"
                if len(self.search.grid) <= self.settings.max_evaluations
                else "evaluation_budget_exhausted"
            )
        state = {
            "version": 1,
            "phase": "completed" if terminal else "bayesian",
            "bayesian_identity": self.identity,
            "bayesian_identity_hash": self.identity_hash,
            "result_count": observations_after,
            "consumed_pairs": observations_after * self.settings.pairs_per_evaluation,
            "consumed_games": observations_after * self.settings.pairs_per_evaluation * 2,
            "last_proposal_id": proposal["proposal_id"],
            "stop_reason": stop_reason,
        }
        stored_result = dict(result)
        stored_result["values"] = list(proposal["values"])
        stored_result["parameter_hash"] = proposal["parameter_hash"]
        observation, checkpoint = self.database.record_bayesian_observation_atomically(
            self.campaign_id,
            str(proposal["proposal_id"]),
            [float(value) for value in pair_points],
            score,
            variance,
            stored_result,
            state,
        )
        persisted_state = self.database.optimizer_state(self.campaign_id)["state"]
        return {"observation": observation, "checkpoint": checkpoint, "state": persisted_state}

    def run(self, max_results: int = 0) -> dict[str, Any]:
        if max_results < 0:
            raise BayesianOptimizationError("max_results cannot be negative")
        campaign = self.database.campaign(self.campaign_id)
        if campaign["status"] in {"pending", "interrupted"}:
            self.database.transition_campaign(self.campaign_id, "running", "Bayesian optimizer started")
        processed = 0
        while not max_results or processed < max_results:
            state = self.database.optimizer_state(self.campaign_id)["state"]
            if state.get("phase") == "completed":
                break
            proposal = self.ask()
            result = self.runner(proposal)
            self.tell(proposal, result)
            processed += 1
        return self.report(processed)

    def report(self, processed: int = 0) -> dict[str, Any]:
        checkpoint = self.database.optimizer_state(self.campaign_id)
        observations = self.database.bayesian_observations(self.campaign_id)
        best = max(observations, key=lambda item: (float(item["score"]), item["parameter_hash"])) if observations else None
        return {
            "campaign_id": self.campaign_id,
            "phase": checkpoint["state"].get("phase"),
            "processed": processed,
            "result_count": len(observations),
            "proposals": self.database.bayesian_proposals(self.campaign_id),
            "observations": observations,
            "best": best,
            "checkpoint": {
                "revision": checkpoint["revision"],
                "checkpoint_hash": checkpoint["checkpoint_hash"],
                "state": checkpoint["state"],
            },
        }
