"""Autonomous optimizer orchestration with a deterministic fake match runner."""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .adaptive import AdaptiveCampaign, AdaptiveError, AdaptivePolicy
from .canonical import atomic_write_json, sha256_bytes, sha256_json
from .coordinate import MultiResolutionCoordinateSearch
from .database import CampaignBusy, Database, DatabaseError
from .registry import Registry
from .service import campaign_dir, campaign_lock, init_campaign, load_database


class OptimizationError(RuntimeError):
    """The autonomous optimization controller cannot continue safely."""


class OptimizationBudgetExhausted(AdaptiveError):
    """A fake runner refused to start a block beyond the configured budget."""


def _integer(value: Any, name: str, minimum: int = 0) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or value < minimum:
        raise OptimizationError(f"{name} must be an integer >= {minimum}")
    return value


@dataclass(frozen=True)
class OptimizeSettings:
    max_games: int
    max_evaluations: int
    max_passes: int
    parameter_names: tuple[str, ...] | None
    adaptive: AdaptivePolicy
    fake_optimum: dict[str, int]


def _settings(database: Database, campaign_id: str, registry: Registry) -> OptimizeSettings:
    campaign = database.campaign(campaign_id)
    config = json.loads(campaign["config_json"])
    goals = config.get("goals", {})
    if not isinstance(goals, dict):
        raise OptimizationError("campaign goals must be an object")
    optimizer_goals = goals.get("optimizer", {})
    if not isinstance(optimizer_goals, dict):
        raise OptimizationError("goals.optimizer must be an object")
    adaptive_goals = goals.get("adaptive", {})
    if not isinstance(adaptive_goals, dict):
        raise OptimizationError("goals.adaptive must be an object")
    fake_goals = goals.get("fake_match", {})
    if not isinstance(fake_goals, dict):
        raise OptimizationError("goals.fake_match must be an object")

    max_games = _integer(
        goals.get("max_games", optimizer_goals.get("max_games", 0)), "goals.max_games"
    )
    max_evaluations = _integer(
        goals.get("max_evaluations", optimizer_goals.get("max_evaluations", 0)),
        "goals.max_evaluations",
    )
    max_passes = _integer(
        goals.get("max_passes", optimizer_goals.get("max_passes", 100)), "goals.max_passes", 1
    )
    raw_names = optimizer_goals.get("parameters", goals.get("parameters"))
    parameter_names: tuple[str, ...] | None
    if raw_names is None:
        parameter_names = None
    elif isinstance(raw_names, list) and all(isinstance(name, str) and name for name in raw_names):
        if len(set(raw_names)) != len(raw_names):
            raise OptimizationError("optimizer parameter names must be unique")
        parameter_names = tuple(raw_names)
    else:
        raise OptimizationError("goals.optimizer.parameters must be a list of names")

    adaptive = AdaptivePolicy(
        min_blocks=_integer(adaptive_goals.get("min_blocks", 1), "adaptive.min_blocks", 1),
        max_blocks=_integer(adaptive_goals.get("max_blocks", 2), "adaptive.max_blocks", 1),
        weak_upper_score=float(adaptive_goals.get("weak_upper_score", 45.0)),
        target_score=float(adaptive_goals.get("target_score", 50.0)),
    )
    optimum = fake_goals.get("optimum", goals.get("fake_optimum"))
    if not isinstance(optimum, dict):
        raise OptimizationError("goals.fake_match.optimum is required for fake optimization")
    baseline = {
        str(item["name"]): int(item["value"])
        for item in registry.parameters
    }
    fake_optimum = dict(baseline)
    for name, value in optimum.items():
        if not isinstance(name, str) or not isinstance(value, int) or isinstance(value, bool):
            raise OptimizationError("fake optimum values must be integers")
        if name not in baseline:
            raise OptimizationError(f"fake optimum contains unknown parameter: {name}")
        fake_optimum[name] = value
    return OptimizeSettings(
        max_games=max_games,
        max_evaluations=max_evaluations,
        max_passes=max_passes,
        parameter_names=parameter_names,
        adaptive=adaptive,
        fake_optimum=fake_optimum,
    )


def _parameter_values(document: dict[str, Any]) -> dict[str, int]:
    return {str(item["name"]): int(item["value"]) for item in document["parameters"]}


class FakeAdaptiveMatchRunner:
    """Run one deterministic paired block without starting an engine process."""

    def __init__(
        self,
        database: Database,
        campaign_id: str,
        candidate: dict[str, Any],
        reference: dict[str, Any],
        optimum: dict[str, int],
        max_games: int,
    ) -> None:
        self.database = database
        self.campaign_id = campaign_id
        self.candidate = candidate
        self.reference = reference
        self.optimum = optimum
        self.max_games = max_games

    def _objective(self, document: dict[str, Any]) -> int:
        values = _parameter_values(document)
        return -sum((values[name] - self.optimum[name]) ** 2 for name in values)

    def run(self, block: dict[str, Any]) -> dict[str, Any]:
        pairs = int(block["pairs_per_block"])
        expected_games = pairs * 2
        if self.max_games:
            games = self.database.status_snapshot(self.campaign_id)["games"]
            if games + expected_games > self.max_games:
                raise OptimizationBudgetExhausted(
                    f"match budget exhausted before block {block['block_id']}"
                )
        claimed = self.database.claim_next_block(self.campaign_id)
        if claimed is None or claimed["block_id"] != block["block_id"]:
            raise OptimizationError("fake runner claimed a different block")
        trial_id = str(block["trial_id"])
        trial = self.database.trial(self.campaign_id, trial_id)
        if trial["status"] in {"pending", "interrupted"}:
            self.database.transition_trial(self.campaign_id, trial_id, "running", result={"phase": "running"})

        candidate_score = self._objective(self.candidate)
        reference_score = self._objective(self.reference)
        if candidate_score > reference_score:
            wins, draws, losses = expected_games, 0, 0
            game_result = "1-0"
        elif candidate_score < reference_score:
            wins, draws, losses = 0, 0, expected_games
            game_result = "0-1"
        else:
            wins, draws, losses = 0, expected_games, 0
            game_result = "1/2-1/2"
        result = {
            "wins": wins,
            "draws": draws,
            "losses": losses,
            "score": (wins + draws / 2.0) / expected_games * 100.0,
            "games": [game_result] * expected_games,
            "runner": "fake-adaptive-v1",
            "candidate_parameter_hash": sha256_json(self.candidate),
            "reference_parameter_hash": sha256_json(self.reference),
            "candidate_objective": candidate_score,
            "reference_objective": reference_score,
            "block_index": int(block["block_index"]),
        }
        checkpoint = self.database.optimizer_state(self.campaign_id)["state"]
        self.database.complete_block_atomically(
            self.campaign_id,
            str(block["block_id"]),
            wins,
            draws,
            losses,
            float(result["score"]),
            result,
            checkpoint,
        )
        return result


def _materialize_candidate(
    database: Database, data_dir: Path, campaign_id: str, document: dict[str, Any]
) -> Path:
    parameter_hash = sha256_json(document)
    path = campaign_dir(data_dir, campaign_id) / "candidates" / f"{parameter_hash}.json"
    expected = (json.dumps(document, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n").encode()
    if path.exists() and path.read_bytes() != expected:
        raise OptimizationError(f"candidate parameter artifact differs: {path}")
    if not path.exists():
        atomic_write_json(path, document)
    database.record_artifact(campaign_id, "candidate_parameters", str(path.resolve()), sha256_bytes(expected))
    return path


class FakeAdaptiveEvaluator:
    """Use AdaptiveCampaign as the evaluator behind multi-resolution search."""

    def __init__(
        self,
        database: Database,
        data_dir: Path,
        campaign_id: str,
        policy: AdaptivePolicy,
        optimum: dict[str, int],
        max_games: int,
    ) -> None:
        self.database = database
        self.data_dir = data_dir
        self.campaign_id = campaign_id
        self.policy = policy
        self.optimum = optimum
        self.max_games = max_games

    def __call__(self, candidate: dict[str, Any], seed: int) -> dict[str, Any]:
        _materialize_candidate(self.database, self.data_dir, self.campaign_id, candidate)
        state = self.database.optimizer_state(self.campaign_id)["state"]
        reference = state.get("coordinate_base_parameters") or state.get("anchor_parameters")
        if not isinstance(reference, dict):
            raise OptimizationError("coordinate checkpoint has no current best parameter set")
        runner = FakeAdaptiveMatchRunner(
            self.database,
            self.campaign_id,
            candidate,
            reference,
            self.optimum,
            self.max_games,
        )
        controller = AdaptiveCampaign(
            self.database,
            self.campaign_id,
            candidate,
            self.policy,
            runner,
            seed,
            complete_trial=False,
        )
        return controller.run()


class AutonomousFakeOptimizer:
    """Drive one candidate at a time until search or campaign budget ends."""

    def __init__(
        self,
        database: Database,
        data_dir: Path,
        campaign_id: str,
        registry: Registry,
        settings: OptimizeSettings,
        invocation_limit: int = 0,
    ) -> None:
        self.database = database
        self.data_dir = data_dir
        self.campaign_id = campaign_id
        self.settings = settings
        self.invocation_limit = _integer(invocation_limit, "max_results")
        evaluator = FakeAdaptiveEvaluator(
            database, data_dir, campaign_id, settings.adaptive, settings.fake_optimum, settings.max_games
        )
        self.search = MultiResolutionCoordinateSearch(
            database,
            campaign_id,
            registry,
            evaluator,
            max_passes=settings.max_passes,
            parameter_names=list(settings.parameter_names) if settings.parameter_names is not None else None,
        )

    def run(self) -> dict[str, Any]:
        processed = 0
        while True:
            state = self.database.optimizer_state(self.campaign_id)["state"]
            if state.get("phase") == "completed":
                return self.search.report()
            if self.settings.max_evaluations and int(state.get("result_count", 0)) >= self.settings.max_evaluations:
                return self.search.stop("evaluation_budget_exhausted")
            if self.settings.max_games:
                consumed = int(self.database.status_snapshot(self.campaign_id)["games"])
                worst_candidate_games = self.settings.adaptive.max_blocks * 2
                if consumed + worst_candidate_games > self.settings.max_games:
                    return self.search.stop("match_budget_exhausted")
            before = int(state.get("result_count", 0))
            report = self.search.run(max_results=1)
            after = int(report.get("result_count", before))
            processed += max(0, after - before)
            if report.get("phase") == "completed":
                return report
            if self.invocation_limit and processed >= self.invocation_limit:
                return report


def run_fake_optimization(
    campaign_path: Path,
    data_dir: Path,
    invocation_limit: int = 0,
    max_games_override: int | None = None,
    max_evaluations_override: int | None = None,
) -> dict[str, Any]:
    """Initialize or resume a fake autonomous optimization campaign."""
    definition, _, _ = init_campaign(campaign_path, data_dir)
    if definition.mode != "fake":
        raise OptimizationError("optimize currently requires campaign mode=fake; real runner is not connected")
    database = load_database(data_dir, definition.campaign_id)
    settings = _settings(database, definition.campaign_id, definition.registry)
    if max_games_override is not None:
        settings = OptimizeSettings(
            **{**settings.__dict__, "max_games": _integer(max_games_override, "max_games")}
        )
    if max_evaluations_override is not None:
        settings = OptimizeSettings(
            **{**settings.__dict__, "max_evaluations": _integer(max_evaluations_override, "max_evaluations")}
        )

    with campaign_lock(data_dir, definition.campaign_id):
        token = f"optimizer-{os.getpid()}"
        try:
            database.claim_campaign(definition.campaign_id, token, takeover=True)
            database.recover_abandoned_jobs(definition.campaign_id)
            controller = AutonomousFakeOptimizer(
                database,
                data_dir,
                definition.campaign_id,
                definition.registry,
                settings,
                invocation_limit=invocation_limit,
            )
            return controller.run()
        finally:
            try:
                database.release_campaign(definition.campaign_id, token)
            except (DatabaseError, CampaignBusy):
                pass
