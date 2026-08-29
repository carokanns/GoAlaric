"""Autonomous optimizer orchestration for fake and real match runners."""

from __future__ import annotations

import json
import math
import os
import shlex
from dataclasses import dataclass, replace
from pathlib import Path
from typing import Any, Callable, Sequence

from .adaptive import AdaptiveCampaign, AdaptiveError, AdaptivePolicy, RealAdaptiveBlockRunner
from .bayes import BayesianSearchError
from .bayesian_optimization import (
    BayesianOptimizationError,
    BayesianOptimizer,
    BayesianRunSettings,
    DeterministicFakePairRunner,
    dimensions_from_registry,
)
from .canonical import atomic_write_json, canonical_json, sha256_bytes, sha256_json
from .confirmation import (
    ConfirmationCampaign,
    ConfirmationSettings,
    FakeConfirmationRunner,
    RealConfirmationRunner,
    initialize_confirmation,
    parse_confirmation_settings,
)
from .coordinate import MultiResolutionCoordinateSearch
from .database import CampaignBusy, Database, DatabaseError
from .profiles import MatchProfile, ProfileError, resolve_profile
from .real_integration import RealTestmonitorConfig
from .registry import Registry
from .scheduler import terminate_active_blocks
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
    algorithm: str
    max_games: int
    max_evaluations: int
    max_passes: int
    parameter_names: tuple[str, ...] | None
    exploratory: bool
    exploratory_min_score: float
    adaptive: AdaptivePolicy
    fake_optimum: dict[str, int] | None
    search_profile: MatchProfile
    confirmation: ConfirmationSettings
    bayesian_initial_points: int
    bayesian_pairs_per_evaluation: int


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

    algorithm = optimizer_goals.get("algorithm", "coordinate-multires-v1")
    if algorithm not in {"coordinate-multires-v1", "finite-noise-aware-bo-v1"}:
        raise OptimizationError(f"unsupported optimizer algorithm: {algorithm}")

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

    raw_exploratory = optimizer_goals.get("exploratory", False)
    if isinstance(raw_exploratory, bool):
        exploratory = raw_exploratory
        exploratory_min_score = 51.0
    elif isinstance(raw_exploratory, dict):
        exploratory = raw_exploratory.get("enabled", True)
        exploratory_min_score = raw_exploratory.get("min_score", 51.0)
        if not isinstance(exploratory, bool):
            raise OptimizationError("goals.optimizer.exploratory.enabled must be boolean")
    else:
        raise OptimizationError("goals.optimizer.exploratory must be boolean or an object")
    if (
        isinstance(exploratory_min_score, bool)
        or not isinstance(exploratory_min_score, (int, float))
        or not math.isfinite(float(exploratory_min_score))
        or not 0.0 <= float(exploratory_min_score) <= 100.0
    ):
        raise OptimizationError("goals.optimizer.exploratory.min_score must be between 0 and 100")

    adaptive = AdaptivePolicy(
        min_blocks=_integer(adaptive_goals.get("min_blocks", 1), "adaptive.min_blocks", 1),
        max_blocks=_integer(adaptive_goals.get("max_blocks", 2), "adaptive.max_blocks", 1),
        weak_upper_score=float(adaptive_goals.get("weak_upper_score", 45.0)),
        target_score=float(adaptive_goals.get("target_score", 50.0)),
    )
    baseline = {
        str(item["name"]): int(item["value"])
        for item in registry.parameters
    }
    optimum = fake_goals.get("optimum", goals.get("fake_optimum"))
    fake_optimum: dict[str, int] | None = None
    if optimum is not None:
        if not isinstance(optimum, dict):
            raise OptimizationError("goals.fake_match.optimum must be an object")
        fake_optimum = dict(baseline)
        for name, value in optimum.items():
            if not isinstance(name, str) or not isinstance(value, int) or isinstance(value, bool):
                raise OptimizationError("fake optimum values must be integers")
            if name not in baseline:
                raise OptimizationError(f"fake optimum contains unknown parameter: {name}")
            fake_optimum[name] = value
    confirmation_source = goals if "confirmation" in goals else {"confirmation": config.get("confirmation", {})}
    real_goals = goals.get("real", {})
    if real_goals is None:
        real_goals = {}
    try:
        search_profile = resolve_profile(real_goals, optimizer_goals.get("profile"), "search")
        raw_confirmation = confirmation_source.get("confirmation", {})
        confirmation_profile = None
        if isinstance(raw_confirmation, dict) and raw_confirmation.get("enabled", False):
            confirmation_profile = resolve_profile(
                real_goals, raw_confirmation.get("profile"), "confirmation"
            )
    except ProfileError as exc:
        raise OptimizationError(str(exc)) from exc
    confirmation = parse_confirmation_settings(
        confirmation_source, int(campaign["master_seed"]), profile=confirmation_profile
    )
    return OptimizeSettings(
        algorithm=algorithm,
        max_games=max_games,
        max_evaluations=max_evaluations,
        max_passes=max_passes,
        parameter_names=parameter_names,
        exploratory=exploratory,
        exploratory_min_score=float(exploratory_min_score),
        adaptive=adaptive,
        fake_optimum=fake_optimum,
        search_profile=search_profile,
        confirmation=confirmation,
        bayesian_initial_points=(
            _integer(optimizer_goals.get("initial_points", 9), "optimizer.initial_points", 1)
            if algorithm == "finite-noise-aware-bo-v1"
            else 9
        ),
        bayesian_pairs_per_evaluation=(
            _integer(
                optimizer_goals.get("pairs_per_evaluation", 16),
                "optimizer.pairs_per_evaluation",
                2,
            )
            if algorithm == "finite-noise-aware-bo-v1"
            else 16
        ),
    )


def _runtime_path(value: Any, name: str, base_dir: Path) -> Path:
    if not isinstance(value, str) or not value:
        raise OptimizationError(f"goals.real.{name} must be a path")
    path = Path(value)
    if not path.is_absolute():
        path = base_dir / path
    return path.resolve()


def _real_config(
    campaign_path: Path,
    data_dir: Path,
    definition: Any,
    profile: MatchProfile | None = None,
) -> RealTestmonitorConfig:
    config = definition.config
    goals = config.get("goals", {})
    runtime = goals.get("real", {}) if isinstance(goals, dict) else {}
    if not isinstance(runtime, dict):
        raise OptimizationError("goals.real must be an object for mode=real")
    command = runtime.get("testmonitor_command")
    if isinstance(command, str):
        testmonitor_command: Sequence[str] = tuple(shlex.split(command))
    elif isinstance(command, list) and all(isinstance(item, str) and item for item in command):
        testmonitor_command = tuple(command)
    else:
        raise OptimizationError("goals.real.testmonitor_command must be a command string or list")
    if not testmonitor_command:
        raise OptimizationError("goals.real.testmonitor_command cannot be empty")

    base_dir = campaign_path.resolve().parent
    engine = _runtime_path(definition.baseline_engine_id, "engine", base_dir)
    fastchess = _runtime_path(runtime.get("fastchess"), "fastchess", base_dir)
    opening_book = _runtime_path(runtime.get("opening_book"), "opening_book", base_dir)
    workdir_value = runtime.get("workdir")
    workdir = _runtime_path(workdir_value, "workdir", base_dir) if workdir_value is not None else base_dir
    for name, path in (("engine", engine), ("fastchess", fastchess), ("opening_book", opening_book), ("workdir", workdir)):
        if not path.exists():
            raise OptimizationError(f"goals.real.{name} does not exist: {path}")
    selected_profile = profile or MatchProfile.create(
        "default", str(runtime.get("tc", "10+0.1")), "real.tc"
    )
    campaign_path_root = campaign_dir(data_dir, definition.campaign_id)
    baseline_parameter_file = campaign_path_root / "baseline-parameters.json"
    return RealTestmonitorConfig(
        testmonitor_command=testmonitor_command,
        fastchess=fastchess,
        baseline=engine,
        candidate=engine,
        baseline_parameter_file=baseline_parameter_file,
        candidate_parameter_file=baseline_parameter_file,
        opening_book=opening_book,
        opening_block_file=campaign_path_root / "adaptive-blocks" / "placeholder.epd",
        tc=selected_profile.tc,
        seed=_integer(runtime.get("seed", definition.master_seed), "real.seed"),
        concurrency=_integer(runtime.get("concurrency", 1), "real.concurrency", 1),
        hash_mb=_integer(runtime.get("hash_mb", 16), "real.hash_mb", 16),
        threads=_integer(runtime.get("threads", 1), "real.threads", 1),
        syzygy_path=str(runtime.get("syzygy_path", "off")),
        workdir=workdir,
        profile_name=selected_profile.name,
        profile_hash=selected_profile.hash,
        profile_mode=selected_profile.mode,
        nodes=selected_profile.nodes,
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
        profile: dict[str, Any] | None = None,
    ) -> None:
        self.database = database
        self.campaign_id = campaign_id
        self.candidate = candidate
        self.reference = reference
        self.optimum = optimum
        self.max_games = max_games
        self.profile = dict(profile) if profile is not None else None

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
        if self.profile is not None:
            result["profile"] = dict(self.profile)
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
        profile: MatchProfile | None = None,
    ) -> None:
        self.database = database
        self.data_dir = data_dir
        self.campaign_id = campaign_id
        self.policy = policy
        self.optimum = optimum
        self.max_games = max_games
        self.profile = profile

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
            self.profile.as_dict() if self.profile is not None else None,
        )
        controller = AdaptiveCampaign(
            self.database,
            self.campaign_id,
            candidate,
            self.policy,
            runner,
            seed,
            profile=self.profile.as_dict() if self.profile is not None else None,
            complete_trial=False,
        )
        result = controller.run()
        # This runner models a deterministic synthetic objective.  Its
        # adaptive evidence may contain a statistical decision, but the
        # synthetic objective is the intended comparison signal here.  Leave
        # decision absent so coordinate search uses its documented objective
        # fallback, just like older synthetic result records.
        result.pop("decision", None)
        result.pop("uncertain", None)
        if controller.trial_id is None:
            raise OptimizationError("fake adaptive evaluator completed without a trial id")
        stored = self.database.trial(self.campaign_id, controller.trial_id)
        stored_result = json.loads(stored["result_json"] or "{}")
        if "decision" in stored_result or "uncertain" in stored_result:
            self.database.checkpoint_trial_result(self.campaign_id, controller.trial_id, result)
        return result


def _reference_parameter_file(
    database: Database, data_dir: Path, campaign_id: str, document: dict[str, Any]
) -> Path:
    campaign = database.campaign(campaign_id)
    if sha256_json(document) == campaign["baseline_parameter_hash"]:
        return campaign_dir(data_dir, campaign_id) / "baseline-parameters.json"
    return _materialize_candidate(database, data_dir, campaign_id, document)


class RealAdaptiveEvaluator:
    """Run one candidate against the current best through testmonitor."""

    def __init__(
        self,
        database: Database,
        data_dir: Path,
        campaign_id: str,
        config: RealTestmonitorConfig,
        policy: AdaptivePolicy,
        profile: MatchProfile | None = None,
    ) -> None:
        self.database = database
        self.data_dir = data_dir
        self.campaign_id = campaign_id
        self.config = config
        self.policy = policy
        self.profile = profile

    def __call__(self, candidate: dict[str, Any], seed: int) -> dict[str, Any]:
        candidate_hash = sha256_json(candidate)
        _materialize_candidate(self.database, self.data_dir, self.campaign_id, candidate)
        campaign = self.database.campaign(self.campaign_id)
        if candidate_hash == campaign["baseline_parameter_hash"]:
            # The baseline is the reference point, not a self-match. Real
            # testmonitor deliberately rejects identical parameter identities.
            return {
                "wins": 0,
                "draws": 1,
                "losses": 0,
                "score": 50.0,
                "uncertainty": 0.0,
                "uncertain": False,
                "runner": "real-baseline-reference-v1",
                "candidate_parameter_hash": candidate_hash,
                "reference_parameter_hash": candidate_hash,
                "matchless_reference": True,
                "profile": self.profile.as_dict() if self.profile is not None else None,
            }
        state = self.database.optimizer_state(self.campaign_id)["state"]
        reference = state.get("coordinate_base_parameters") or state.get("anchor_parameters")
        if not isinstance(reference, dict):
            raise OptimizationError("coordinate checkpoint has no current best parameter set")
        reference_path = _reference_parameter_file(self.database, self.data_dir, self.campaign_id, reference)
        block_dir = campaign_dir(self.data_dir, self.campaign_id) / "adaptive-blocks" / candidate_hash[:20]
        effective = replace(
            self.config,
            seed=seed,
            baseline_parameter_file=reference_path,
            candidate_parameter_file=campaign_dir(self.data_dir, self.campaign_id)
            / "candidates"
            / f"{candidate_hash}.json",
            opening_block_file=block_dir / "placeholder.epd",
        )
        runner = RealAdaptiveBlockRunner(
            self.data_dir,
            self.campaign_id,
            effective,
            block_dir,
            embedded_campaign=True,
        )
        controller = AdaptiveCampaign(
            self.database,
            self.campaign_id,
            candidate,
            self.policy,
            runner,
            seed,
            block_hash_factory=runner.block_hashes,
            profile=self.profile.as_dict() if self.profile is not None else None,
            complete_trial=False,
        )
        return controller.run()


def _original_baseline(database: Database, campaign_id: str) -> dict[str, Any]:
    campaign = database.campaign(campaign_id)
    row = database.parameter_set_by_hash(campaign_id, str(campaign["baseline_parameter_hash"]))
    if row is None:
        raise OptimizationError("original baseline parameter set is missing")
    return dict(row["document"])


def _final_search_candidate(database: Database, campaign_id: str) -> dict[str, Any]:
    state = database.optimizer_state(campaign_id)["state"]
    candidate = state.get("anchor_parameters") or state.get("coordinate_base_parameters")
    if not isinstance(candidate, dict):
        raise OptimizationError("completed search has no final candidate parameter set")
    return dict(candidate)


def _materialize_recommendation(
    database: Database, data_dir: Path, campaign_id: str
) -> Path | None:
    """Write the reviewed recommendation without changing the engine baseline."""
    snapshot = database.confirmation_snapshot(campaign_id)
    if snapshot is None or snapshot.get("status") != "completed":
        return None
    recommendation_hash = snapshot.get("recommendation_parameter_hash")
    if recommendation_hash is None:
        return None
    if not isinstance(recommendation_hash, str) or not recommendation_hash:
        raise OptimizationError("completed confirmation has an invalid recommendation parameter hash")
    if recommendation_hash == snapshot.get("candidate_parameter_hash"):
        document = snapshot.get("candidate_document")
    elif recommendation_hash == snapshot.get("baseline_parameter_hash"):
        document = snapshot.get("baseline_document")
    else:
        raise OptimizationError("confirmation recommendation does not match candidate or baseline")
    if not isinstance(document, dict) or sha256_json(document) != recommendation_hash:
        raise OptimizationError("confirmation recommendation document has an invalid hash")
    path = campaign_dir(data_dir, campaign_id) / "recommended-parameters.json"
    expected = (canonical_json(document) + "\n").encode("utf-8")
    if path.exists() and path.read_bytes() != expected:
        raise OptimizationError(f"recommendation parameter artifact differs: {path}")
    if not path.exists():
        atomic_write_json(path, document)
    database.record_artifact(campaign_id, "recommended_parameters", str(path.resolve()), sha256_bytes(expected))
    return path


def _run_confirmation(
    database: Database,
    data_dir: Path,
    campaign_path: Path,
    definition: Any,
    settings: OptimizeSettings,
    invocation_limit: int,
    search_processed: int,
) -> dict[str, Any]:
    confirmation_settings = settings.confirmation
    if not confirmation_settings.enabled:
        return {}
    existing_confirmation = database.confirmation(definition.campaign_id)
    if existing_confirmation is None or existing_confirmation["status"] != "completed":
        database.begin_confirmation(definition.campaign_id)
    candidate = _final_search_candidate(database, definition.campaign_id)
    baseline = _original_baseline(database, definition.campaign_id)
    if database.confirmation(definition.campaign_id) is None:
        initialize_confirmation(
            database,
            definition.campaign_id,
            candidate,
            baseline,
            confirmation_settings,
            profile=(confirmation_settings.profile.as_dict() if confirmation_settings.profile else None),
        )

    if definition.mode == "fake":
        runner: Any = FakeConfirmationRunner(
            database,
            definition.campaign_id,
            confirmation_settings.fake_results,
            profile=(confirmation_settings.profile.as_dict() if confirmation_settings.profile else None),
        )
    else:
        runtime = _real_config(campaign_path, data_dir, definition, confirmation_settings.profile)
        candidate_path = _materialize_candidate(database, data_dir, definition.campaign_id, candidate)
        runtime = replace(
            runtime,
            seed=confirmation_settings.seed,
            baseline_parameter_file=campaign_dir(data_dir, definition.campaign_id) / "baseline-parameters.json",
            candidate_parameter_file=candidate_path,
        )
        runner = RealConfirmationRunner(
            database,
            data_dir,
            definition.campaign_id,
            runtime,
            candidate_path,
        )

    if invocation_limit and search_processed >= invocation_limit:
        snapshot = database.confirmation_snapshot(definition.campaign_id)
        return snapshot or {}
    remaining = max(0, invocation_limit - search_processed) if invocation_limit else 0
    confirmation = ConfirmationCampaign(
        database,
        definition.campaign_id,
        confirmation_settings,
        runner,
    )
    return confirmation.run(max_blocks=remaining)


class AutonomousOptimizer:
    """Drive one candidate at a time until search or campaign budget ends."""

    def __init__(
        self,
        database: Database,
        data_dir: Path,
        campaign_id: str,
        registry: Registry,
        settings: OptimizeSettings,
        evaluator: Callable[[dict[str, Any], int], dict[str, Any]],
        invocation_limit: int = 0,
    ) -> None:
        self.database = database
        self.data_dir = data_dir
        self.campaign_id = campaign_id
        self.settings = settings
        self.invocation_limit = _integer(invocation_limit, "max_results")
        self.search = MultiResolutionCoordinateSearch(
            database,
            campaign_id,
            registry,
            evaluator,
            max_passes=settings.max_passes,
            parameter_names=list(settings.parameter_names) if settings.parameter_names is not None else None,
            exploratory=settings.exploratory,
            exploratory_min_score=settings.exploratory_min_score,
        )
        self.processed = 0

    def run(self) -> dict[str, Any]:
        # Repair a checkpoint that contains completed coordinate directions but
        # whose selection step was interrupted before it could be committed.
        # This performs no evaluator call and is idempotent; the normal loop
        # can then resume with the repaired anchor.
        self.search.reconcile_checkpoint()
        processed = 0
        while True:
            state = self.database.optimizer_state(self.campaign_id)["state"]
            if state.get("phase") == "completed":
                self.processed = processed
                return self.search.report()
            if self.settings.max_evaluations and int(state.get("result_count", 0)) >= self.settings.max_evaluations:
                self.processed = processed
                return self.search.stop("evaluation_budget_exhausted")
            if self.settings.max_games:
                consumed = int(self.database.status_snapshot(self.campaign_id)["games"])
                worst_candidate_games = self.settings.adaptive.max_blocks * 2
                if consumed + worst_candidate_games > self.settings.max_games:
                    self.processed = processed
                    return self.search.stop("match_budget_exhausted")
            before = int(state.get("result_count", 0))
            report = self.search.run(max_results=1)
            after = int(report.get("result_count", before))
            processed += max(0, after - before)
            if report.get("phase") == "completed":
                self.processed = processed
                return report
            if self.invocation_limit and processed >= self.invocation_limit:
                self.processed = processed
                return report


def _run_fake_bayesian_optimization(
    database: Database,
    definition: Any,
    settings: OptimizeSettings,
    invocation_limit: int,
) -> dict[str, Any]:
    if settings.fake_optimum is None:
        raise OptimizationError("goals.fake_match.optimum is required for Bayesian fake mode")
    if settings.confirmation.enabled:
        raise OptimizationError("Bayesian confirmation is not implemented until the real-runner milestone")
    if settings.max_evaluations < 1:
        raise OptimizationError("Bayesian optimization requires goals.max_evaluations")
    dimensions = dimensions_from_registry(definition.registry, settings.parameter_names)
    optimum = tuple(settings.fake_optimum[item.name] for item in dimensions)

    def objective(values: tuple[int, ...]) -> float:
        distances = []
        for value, target, dimension in zip(values, optimum, dimensions, strict=True):
            span = dimension.values[-1] - dimension.values[0]
            distances.append(0.0 if span == 0 else ((value - target) / span) ** 2)
        mean_distance = sum(distances) / len(distances)
        return min(0.60, max(0.40, 0.56 - 0.12 * mean_distance))

    max_evaluations = settings.max_evaluations
    if settings.max_games:
        games_per_evaluation = settings.bayesian_pairs_per_evaluation * 2
        budgeted_evaluations = settings.max_games // games_per_evaluation
        if budgeted_evaluations < 1:
            raise OptimizationError("match budget cannot fund one Bayesian evaluation")
        max_evaluations = min(max_evaluations, budgeted_evaluations)
    controller = BayesianOptimizer(
        database,
        definition.campaign_id,
        definition.registry,
        BayesianRunSettings(
            seed=definition.master_seed,
            pairs_per_evaluation=settings.bayesian_pairs_per_evaluation,
            max_evaluations=max_evaluations,
            initial_points=settings.bayesian_initial_points,
            parameter_names=settings.parameter_names,
        ),
        DeterministicFakePairRunner(objective, seed=definition.master_seed),
    )
    return controller.run(max_results=invocation_limit)


def run_optimization(
    campaign_path: Path,
    data_dir: Path,
    invocation_limit: int = 0,
    max_games_override: int | None = None,
    max_evaluations_override: int | None = None,
    required_mode: str | None = None,
) -> dict[str, Any]:
    """Initialize or resume an autonomous fake or real optimization campaign."""
    definition, _, _ = init_campaign(campaign_path, data_dir)
    if required_mode is not None and definition.mode != required_mode:
        raise OptimizationError(f"optimization requires campaign mode={required_mode}")
    database = load_database(data_dir, definition.campaign_id)
    settings = _settings(database, definition.campaign_id, definition.registry)
    if max_games_override is not None:
        settings = replace(settings, max_games=_integer(max_games_override, "max_games"))
    if max_evaluations_override is not None:
        settings = replace(settings, max_evaluations=_integer(max_evaluations_override, "max_evaluations"))
    if settings.algorithm == "finite-noise-aware-bo-v1":
        if definition.mode != "fake":
            raise OptimizationError("Bayesian real mode is not implemented until the real-runner milestone")
        with campaign_lock(data_dir, definition.campaign_id):
            token = f"optimizer-{os.getpid()}"
            try:
                database.claim_campaign(definition.campaign_id, token, takeover=True)
                return _run_fake_bayesian_optimization(
                    database, definition, settings, invocation_limit
                )
            except (BayesianOptimizationError, BayesianSearchError) as exc:
                raise OptimizationError(str(exc)) from exc
            finally:
                try:
                    database.release_campaign(definition.campaign_id, token)
                except (DatabaseError, CampaignBusy):
                    pass
    if definition.mode == "fake":
        if settings.fake_optimum is None:
            raise OptimizationError("goals.fake_match.optimum is required for mode=fake")
        evaluator: Callable[[dict[str, Any], int], dict[str, Any]] = FakeAdaptiveEvaluator(
            database,
            data_dir,
            definition.campaign_id,
            settings.adaptive,
            settings.fake_optimum,
            settings.max_games,
            settings.search_profile,
        )
    elif definition.mode == "real":
        runtime = _real_config(campaign_path, data_dir, definition, settings.search_profile)
        evaluator = RealAdaptiveEvaluator(
            database,
            data_dir,
            definition.campaign_id,
            runtime,
            settings.adaptive,
            settings.search_profile,
        )
    else:
        raise OptimizationError(f"unsupported campaign mode: {definition.mode}")

    with campaign_lock(data_dir, definition.campaign_id):
        token = f"optimizer-{os.getpid()}"
        try:
            database.claim_campaign(definition.campaign_id, token, takeover=True)
            database.bind_optimizer_profile(
                definition.campaign_id, "search", settings.search_profile.as_dict()
            )
            # A previous optimizer may have died after handing a block to the
            # scheduler. Terminate that recorded process group before replay.
            terminate_active_blocks(data_dir, definition.campaign_id, "optimizer startup recovered abandoned job")
            database.recover_abandoned_jobs(definition.campaign_id)
            database.reconcile_terminal_trial_blocks(
                definition.campaign_id,
                "optimizer startup closed unused blocks from terminal trial",
            )
            controller = AutonomousOptimizer(
                database,
                data_dir,
                definition.campaign_id,
                definition.registry,
                settings,
                evaluator,
                invocation_limit=invocation_limit,
            )
            report = controller.run()
            # A bounded invocation owns only its requested search results. If
            # the last result also completed search, leave confirmation for
            # the next invocation rather than silently spending more work than
            # that quota. An invocation that starts with an already-completed
            # search may proceed directly into confirmation.
            quota_stopped_search = invocation_limit and controller.processed >= invocation_limit
            if (
                settings.confirmation.enabled
                and report.get("phase") == "completed"
                and (not quota_stopped_search or controller.processed == 0)
            ):
                confirmation_report = _run_confirmation(
                    database,
                    data_dir,
                    campaign_path,
                    definition,
                    settings,
                    invocation_limit,
                    controller.processed,
                )
                recommendation_path = _materialize_recommendation(
                    database, data_dir, definition.campaign_id
                )
                if recommendation_path is not None:
                    confirmation_report = database.confirmation_snapshot(definition.campaign_id) or confirmation_report
                    confirmation_report["recommendation_parameter_file"] = str(recommendation_path.resolve())
                report = dict(report)
                report["confirmation"] = confirmation_report
            return report
        finally:
            try:
                database.release_campaign(definition.campaign_id, token)
            except (DatabaseError, CampaignBusy):
                pass


def run_fake_optimization(
    campaign_path: Path,
    data_dir: Path,
    invocation_limit: int = 0,
    max_games_override: int | None = None,
    max_evaluations_override: int | None = None,
) -> dict[str, Any]:
    """Compatibility wrapper for the phase-12 fake runner tests."""
    return run_optimization(
        campaign_path,
        data_dir,
        invocation_limit=invocation_limit,
        max_games_override=max_games_override,
        max_evaluations_override=max_evaluations_override,
        required_mode="fake",
    )
