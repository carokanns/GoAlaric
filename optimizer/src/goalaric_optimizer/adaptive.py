"""Adaptive deterministic candidate gating over complete match blocks."""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any, Callable, Protocol, Sequence

from .canonical import sha256_json
from .database import Database, DatabaseError
from .real_integration import (
    RealTestmonitorConfig,
    RealTestmonitorScheduler,
    _materialize_real_block,
)
from .scheduler import Scheduler, SchedulerError
from .service import campaign_dir, load_database
from .statistics import aggregate_wdl


class AdaptiveError(RuntimeError):
    pass


@dataclass(frozen=True)
class AdaptivePolicy:
    min_blocks: int = 1
    max_blocks: int = 4
    weak_upper_score: float = 45.0
    target_score: float = 50.0

    def __post_init__(self) -> None:
        if self.min_blocks < 1 or self.max_blocks < self.min_blocks:
            raise AdaptiveError("adaptive block budget must satisfy 1 <= min_blocks <= max_blocks")
        if not 0.0 <= self.weak_upper_score <= 100.0:
            raise AdaptiveError("weak_upper_score must be between 0 and 100")
        if not 0.0 <= self.target_score <= 100.0:
            raise AdaptiveError("target_score must be between 0 and 100")


class BlockRunner(Protocol):
    def run(self, block: dict[str, Any]) -> dict[str, Any]:
        """Run exactly the supplied block and leave its SQLite row complete or interrupted."""


def _block_hashes(campaign_id: str, parameter_hash: str, block_index: int) -> tuple[str, str]:
    book_hash = sha256_json(["adaptive-book-v1", campaign_id])
    block_hash = sha256_json(["adaptive-block-v1", campaign_id, parameter_hash, block_index])
    return book_hash, block_hash


def _complete_block_rows(database: Database, campaign_id: str, trial_id: str) -> list[dict[str, Any]]:
    with database._read() as connection:
        rows = connection.execute(
            "SELECT * FROM match_blocks WHERE campaign_id=? AND trial_id=? AND status='completed' "
            "ORDER BY block_index,block_id",
            (campaign_id, trial_id),
        ).fetchall()
    return [dict(row) for row in rows]


def _next_block(database: Database, campaign_id: str, trial_id: str) -> dict[str, Any] | None:
    with database._read() as connection:
        row = connection.execute(
            "SELECT * FROM match_blocks WHERE campaign_id=? AND trial_id=? AND status IN ('pending','interrupted') "
            "ORDER BY block_index,created_at,block_id LIMIT 1",
            (campaign_id, trial_id),
        ).fetchone()
    return dict(row) if row is not None else None


def _normalise_block_result(result: dict[str, Any], pairs_per_block: int) -> dict[str, Any]:
    if not isinstance(result, dict):
        raise AdaptiveError("block runner result must be an object")
    counts = [result.get(name) for name in ("wins", "draws", "losses")]
    if any(not isinstance(value, int) or isinstance(value, bool) or value < 0 for value in counts):
        raise AdaptiveError("block W-D-L must be non-negative integers")
    expected_games = pairs_per_block * 2
    if sum(counts) != expected_games:
        raise AdaptiveError(f"block W-D-L contains {sum(counts)} games; expected {expected_games}")
    score = result.get("score")
    expected_score = (counts[0] + counts[1] / 2.0) / expected_games * 100.0
    if score is None:
        score = expected_score
    if not isinstance(score, (int, float)) or isinstance(score, bool) or not 0.0 <= float(score) <= 100.0:
        raise AdaptiveError("block score must be between 0 and 100")
    if abs(float(score) - expected_score) > 0.01:
        raise AdaptiveError("block score does not match W-D-L")
    games = result.get("games")
    if games is None:
        games = ["1-0"] * counts[0] + ["1/2-1/2"] * counts[1] + ["0-1"] * counts[2]
    if not isinstance(games, list) or len(games) != expected_games:
        raise AdaptiveError("block games must contain both games of every opening pair")
    normalised = dict(result)
    normalised.update(
        {
            "wins": counts[0],
            "draws": counts[1],
            "losses": counts[2],
            "games": games,
            "score": round(float(score), 8),
            "complete_opening_pairs": pairs_per_block,
        }
    )
    return normalised


def _evidence(
    trial_id: str,
    parameter_hash: str,
    policy: AdaptivePolicy,
    aggregate: dict[str, Any],
    decision: str,
    next_block_index: int | None,
    phase: str,
) -> dict[str, Any]:
    result = {
        "schema_version": 1,
        "algorithm": "adaptive-gating-v1",
        "trial_id": trial_id,
        "parameter_hash": parameter_hash,
        "phase": phase,
        "decision": decision,
        "next_block_index": next_block_index,
        "policy": asdict(policy),
        "statistics": aggregate,
    }
    # Keep the fields consumed by CoordinateSearch in the same final result.
    result.update(
        {
            "wins": aggregate["wins"],
            "draws": aggregate["draws"],
            "losses": aggregate["losses"],
            "games": aggregate["games"],
            "score": aggregate["score_percent"],
            "uncertainty": aggregate["uncertainty"],
            "uncertain": decision in {"continue", "uncertain", "interrupted"},
            "elo_estimate": aggregate["elo_estimate"],
            "elo_ci_low": aggregate["elo_ci_low"],
            "elo_ci_high": aggregate["elo_ci_high"],
        }
    )
    return result


def _attach_runner_metadata(evidence: dict[str, Any], block: dict[str, Any]) -> dict[str, Any]:
    """Carry deterministic runner identities through adaptive evidence."""
    raw = block.get("result_json")
    if not isinstance(raw, str):
        return evidence
    try:
        result = json.loads(raw)
    except json.JSONDecodeError:
        return evidence
    if not isinstance(result, dict):
        return evidence
    for key in (
        "runner",
        "candidate_parameter_hash",
        "reference_parameter_hash",
        "candidate_objective",
        "reference_objective",
    ):
        if key in result:
            evidence[key] = result[key]
    return evidence


class AdaptiveCampaign:
    """Run one candidate through a fixed budget of deterministic blocks."""

    ALGORITHM = "adaptive-gating-v1"

    def __init__(
        self,
        database: Database,
        campaign_id: str,
        candidate_document: dict[str, Any],
        policy: AdaptivePolicy,
        runner: BlockRunner,
        seed: int,
        block_hash_factory: Callable[[int], tuple[str, str]] | None = None,
        complete_trial: bool = True,
    ) -> None:
        self.database = database
        self.campaign_id = campaign_id
        self.candidate_document = candidate_document
        self.policy = policy
        self.runner = runner
        self.seed = seed
        self.parameter_hash = sha256_json(candidate_document)
        self.block_hash_factory = block_hash_factory or (
            lambda index: _block_hashes(self.campaign_id, self.parameter_hash, index)
        )
        self.complete_trial = complete_trial
        self.trial_id: str | None = None

    def prepare(self) -> str:
        parameter_set_id = self.database.add_parameter_set(
            self.campaign_id, self.candidate_document, group_name="adaptive"
        )
        self.trial_id = self.database.create_trial(
            self.campaign_id, parameter_set_id, self.ALGORITHM, self.seed
        )
        for index in range(self.policy.max_blocks):
            book_hash, block_hash = self.block_hash_factory(index)
            self.database.create_match_block(
                self.campaign_id,
                self.trial_id,
                "adaptive",
                index,
                1,
                self.seed,
                book_hash,
                block_hash,
            )
        return self.trial_id

    def run(self, max_blocks: int = 0) -> dict[str, Any]:
        if max_blocks < 0:
            raise AdaptiveError("max_blocks cannot be negative")
        trial_id = self.trial_id or self.prepare()
        campaign = self.database.campaign(self.campaign_id)
        if campaign["status"] in {"pending", "paused", "interrupted"}:
            self.database.transition_campaign(self.campaign_id, "running", "adaptive gating start/resume")
        elif campaign["status"] in {"completed", "failed", "rejected"}:
            raise AdaptiveError(f"campaign is terminal: {campaign['status']}")

        trial = self.database.trial(self.campaign_id, trial_id)
        if trial["status"] in {"completed", "rejected"} and trial["result_json"]:
            return json.loads(trial["result_json"])
        if trial["result_json"]:
            stored_result = json.loads(trial["result_json"])
            if (
                isinstance(stored_result, dict)
                and stored_result.get("phase") == "terminal"
                and stored_result.get("decision") in {
                    "accept",
                    "reject",
                    "reject_early",
                    "uncertain",
                    "accept_exploratory",
                    "reject_exploratory",
                }
            ):
                # A process may die after the adaptive controller has stored its
                # terminal evidence but before coordinate search records it.
                return stored_result
        if trial["status"] == "pending":
            self.database.transition_trial(self.campaign_id, trial_id, "running", result={"phase": "running"})

        processed_blocks = 0
        while True:
            completed = _complete_block_rows(self.database, self.campaign_id, trial_id)
            aggregate = aggregate_wdl(completed)
            next_block = _next_block(self.database, self.campaign_id, trial_id)
            if next_block is None:
                raise AdaptiveError("adaptive trial has no next block and no terminal result")
            try:
                self.runner.run(next_block)
            except (AdaptiveError, DatabaseError, SchedulerError):
                raise
            block = next((row for row in _complete_block_rows(self.database, self.campaign_id, trial_id) if row["block_id"] == next_block["block_id"]), None)
            if block is None:
                interrupted = _evidence(
                    trial_id,
                    self.parameter_hash,
                    self.policy,
                    aggregate,
                    "interrupted",
                    int(next_block["block_index"]),
                    "interrupted",
                )
                self.database.checkpoint_trial_result(self.campaign_id, trial_id, interrupted)
                return interrupted
            completed = _complete_block_rows(self.database, self.campaign_id, trial_id)
            aggregate = aggregate_wdl(completed)
            blocks_completed = int(aggregate["blocks_completed"])
            processed_blocks += 1
            if blocks_completed < self.policy.min_blocks:
                decision = "continue"
            elif aggregate["score_ci_high"] < self.policy.weak_upper_score:
                decision = "reject_early"
            elif blocks_completed < self.policy.max_blocks:
                decision = "continue"
            elif aggregate["score_ci_low"] > self.policy.target_score:
                decision = "accept"
            elif aggregate["score_ci_high"] < self.policy.target_score:
                decision = "reject"
            else:
                decision = "uncertain"
            next_index = blocks_completed if decision == "continue" else None
            phase = "running" if decision == "continue" else "terminal"
            evidence = _evidence(
                trial_id,
                self.parameter_hash,
                self.policy,
                aggregate,
                decision,
                next_index,
                phase,
            )
            _attach_runner_metadata(evidence, block)
            if decision == "continue":
                self.database.checkpoint_trial_result(self.campaign_id, trial_id, evidence)
                if max_blocks and processed_blocks >= max_blocks:
                    return evidence
                continue

            self.database.reject_pending_blocks(
                self.campaign_id, trial_id, f"adaptive decision {decision} closed unused match budget"
            )
            if self.complete_trial:
                terminal_status = "rejected" if decision in {"reject_early", "reject"} else "completed"
                self.database.transition_trial(
                    self.campaign_id, trial_id, terminal_status, result=evidence
                )
            else:
                self.database.checkpoint_trial_result(self.campaign_id, trial_id, evidence)
            return evidence


class FakeBlockRunner:
    """SQLite-backed fake match runner for deterministic controller tests."""

    def __init__(self, database: Database, campaign_id: str, results: Sequence[dict[str, Any]]) -> None:
        self.database = database
        self.campaign_id = campaign_id
        self.results = tuple(results)

    def run(self, block: dict[str, Any]) -> dict[str, Any]:
        claimed = self.database.claim_next_block(self.campaign_id)
        if claimed is None or claimed["block_id"] != block["block_id"]:
            raise AdaptiveError("fake runner claimed a different block")
        trial_id = str(block["trial_id"])
        trial = self.database.trial(self.campaign_id, trial_id)
        if trial["status"] in {"pending", "interrupted"}:
            self.database.transition_trial(self.campaign_id, trial_id, "running", result={"phase": "running"})
        index = int(block["block_index"])
        if index >= len(self.results):
            raise AdaptiveError(f"fake result missing for block {index}")
        result = _normalise_block_result(self.results[index], int(block["pairs_per_block"]))
        checkpoint = self.database.optimizer_state(self.campaign_id)["state"]
        self.database.complete_block_atomically(
            self.campaign_id,
            str(block["block_id"]),
            int(result["wins"]),
            int(result["draws"]),
            int(result["losses"]),
            float(result["score"]),
            result,
            checkpoint,
        )
        return result


class SchedulerBlockRunner:
    """Run one block through the existing fake-monitor scheduler."""

    def __init__(self, scheduler: Scheduler) -> None:
        self.scheduler = scheduler

    def run(self, block: dict[str, Any]) -> dict[str, Any]:
        self.scheduler.run(max_completed_blocks=1, finish_work=False)
        return block


class RealAdaptiveBlockRunner:
    """Materialize each block and execute it with the real Go testmonitor."""

    def __init__(self, data_dir: Path, campaign_id: str, config: RealTestmonitorConfig, block_dir: Path) -> None:
        self.data_dir = data_dir
        self.campaign_id = campaign_id
        self.config = config
        self.block_dir = block_dir
        self.block_dir.mkdir(parents=True, exist_ok=True)

    def block_hashes(self, index: int) -> tuple[str, str]:
        path = self.block_dir / f"opening-block-{index:06d}.epd"
        block_config = self.config.__class__(**{**asdict(self.config), "opening_block_file": path})
        _materialize_real_block(block_config, self.config.seed, block_index=index, pairs=1)
        from .canonical import sha256_bytes

        return sha256_bytes(self.config.opening_book.read_bytes()), sha256_bytes(path.read_bytes())

    def run(self, block: dict[str, Any]) -> dict[str, Any]:
        index = int(block["block_index"])
        path = self.block_dir / f"opening-block-{index:06d}.epd"
        block_config = self.config.__class__(**{**asdict(self.config), "opening_block_file": path})
        _materialize_real_block(
            block_config,
            int(block["master_seed"]),
            block_index=index,
            pairs=int(block["pairs_per_block"]),
        )
        scheduler = RealTestmonitorScheduler(
            self.data_dir,
            self.campaign_id,
            block_config,
            poll_interval=0.05,
            stop_grace_seconds=1.0,
            preserve_optimizer_state=True,
        )
        scheduler.run(max_completed_blocks=1, finish_work=False)
        return block


class AdaptiveCoordinateEvaluator:
    """Feed a completed adaptive candidate result back into CoordinateSearch."""

    def __init__(
        self,
        database: Database,
        campaign_id: str,
        policy: AdaptivePolicy,
        baseline_evaluator: Callable[[dict[str, Any], int], dict[str, Any]],
        runner_factory: Callable[
            [dict[str, Any], int], tuple[BlockRunner, Callable[[int], tuple[str, str]] | None]
        ],
    ) -> None:
        self.database = database
        self.campaign_id = campaign_id
        self.policy = policy
        self.baseline_evaluator = baseline_evaluator
        self.runner_factory = runner_factory

    def __call__(self, candidate_document: dict[str, Any], seed: int) -> dict[str, Any]:
        campaign = self.database.campaign(self.campaign_id)
        candidate_hash = sha256_json(candidate_document)
        if candidate_hash == campaign["baseline_parameter_hash"]:
            return self.baseline_evaluator(candidate_document, seed)
        runner, block_hash_factory = self.runner_factory(candidate_document, seed)
        controller = AdaptiveCampaign(
            self.database,
            self.campaign_id,
            candidate_document,
            self.policy,
            runner,
            seed,
            block_hash_factory=block_hash_factory,
            complete_trial=False,
        )
        return controller.run()


def run_real_adaptive_campaign(
    data_dir: Path,
    campaign_id: str,
    config: RealTestmonitorConfig,
    candidate_document: dict[str, Any],
    policy: AdaptivePolicy,
) -> dict[str, Any]:
    """Run a small real candidate campaign with a fixed adaptive budget."""
    database = load_database(data_dir, campaign_id)
    campaign = database.campaign(campaign_id)
    seed = config.seed or int(campaign["master_seed"])
    block_dir = campaign_dir(data_dir, campaign_id) / "adaptive-blocks" / sha256_json(candidate_document)[:20]
    effective = config.__class__(**{**asdict(config), "seed": seed})
    runner = RealAdaptiveBlockRunner(data_dir, campaign_id, effective, block_dir)
    controller = AdaptiveCampaign(
        database,
        campaign_id,
        candidate_document,
        policy,
        runner,
        seed,
        block_hash_factory=runner.block_hashes,
    )
    return controller.run()
