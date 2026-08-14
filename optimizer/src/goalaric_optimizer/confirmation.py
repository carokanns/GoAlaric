"""Fixed, non-adaptive confirmation matches after coordinate search."""

from __future__ import annotations

import math
import os
import subprocess
import time
from dataclasses import dataclass, replace
from pathlib import Path
from statistics import NormalDist
from typing import Any, Protocol

from .canonical import sha256_bytes, sha256_json
from .database import Database, DatabaseError, InvalidTransition
from .process import terminate_process_group
from .real_integration import RealTestmonitorConfig, _materialize_real_block, RealTestmonitorScheduler
from .scheduler import SchedulerError
from .service import campaign_dir


class ConfirmationError(RuntimeError):
    """The fixed confirmation campaign cannot continue safely."""


@dataclass(frozen=True)
class ConfirmationSettings:
    enabled: bool
    games: int
    seed: int
    confidence: float
    fake_results: tuple[str, ...]


def _validate_game(value: Any) -> str:
    if value not in {"1-0", "0-1", "1/2-1/2"}:
        raise ConfirmationError(f"invalid confirmation game result: {value!r}")
    return str(value)


def _fake_results(raw: Any, games: int) -> tuple[str, ...]:
    if raw is None:
        return tuple("1/2-1/2" for _ in range(games))
    if isinstance(raw, list):
        if len(raw) != games:
            raise ConfirmationError("confirmation.fake_results must contain exactly confirmation.games results")
        return tuple(_validate_game(value) for value in raw)
    if isinstance(raw, dict):
        counts = {name: raw.get(name, 0) for name in ("wins", "draws", "losses")}
        if any(not isinstance(value, int) or isinstance(value, bool) or value < 0 for value in counts.values()):
            raise ConfirmationError("confirmation.fake_result W-D-L must be non-negative integers")
        if sum(counts.values()) != games:
            raise ConfirmationError("confirmation.fake_result W-D-L must add up to confirmation.games")
        return tuple(
            ["1-0"] * counts["wins"]
            + ["1/2-1/2"] * counts["draws"]
            + ["0-1"] * counts["losses"]
        )
    raise ConfirmationError("confirmation.fake_results must be a result list or W-D-L object")


def parse_confirmation_settings(goals: dict[str, Any], master_seed: int) -> ConfirmationSettings:
    raw = goals.get("confirmation", {})
    if raw is None:
        raw = {}
    if not isinstance(raw, dict):
        raise ConfirmationError("goals.confirmation must be an object")
    enabled = raw.get("enabled", False)
    if not isinstance(enabled, bool):
        raise ConfirmationError("confirmation.enabled must be boolean")
    if not enabled:
        return ConfirmationSettings(False, 0, master_seed, 0.95, ())
    games = raw.get("games")
    if not isinstance(games, int) or isinstance(games, bool) or games < 2 or games % 2:
        raise ConfirmationError("confirmation.games must be an even integer >= 2")
    seed = raw.get("seed")
    if not isinstance(seed, int) or isinstance(seed, bool) or seed < 0:
        raise ConfirmationError("confirmation.seed must be a non-negative integer")
    if seed == master_seed:
        raise ConfirmationError("confirmation.seed must differ from the optimization master_seed")
    confidence = raw.get("confidence", 0.95)
    if not isinstance(confidence, (int, float)) or isinstance(confidence, bool) or not 0 < float(confidence) < 1:
        raise ConfirmationError("confirmation.confidence must be between 0 and 1")
    fake_raw = raw.get("fake_results", raw.get("fake_result"))
    return ConfirmationSettings(
        True,
        games,
        seed,
        float(confidence),
        _fake_results(fake_raw, games),
    )


class ConfirmationBlockRunner(Protocol):
    def prepare_block(self, block_index: int, pairs_per_block: int) -> tuple[str, str]: ...

    def run(self, block: dict[str, Any]) -> dict[str, Any]: ...


def _summary(blocks: list[dict[str, Any]], confidence: float) -> dict[str, Any]:
    completed = [block for block in blocks if block.get("status") == "completed"]
    wins = sum(int(block["wins"]) for block in completed)
    draws = sum(int(block["draws"]) for block in completed)
    losses = sum(int(block["losses"]) for block in completed)
    games = wins + draws + losses
    if not games:
        low, high, score = 0.0, 100.0, 0.0
    else:
        points = wins + draws / 2.0
        score = points / games
        variance = (
            wins * (1.0 - score) ** 2
            + draws * (0.5 - score) ** 2
            + losses * score**2
        ) / games
        z = NormalDist().inv_cdf(0.5 + confidence / 2.0)
        half_width = z * math.sqrt(variance / games)
        low = max(0.0, score - half_width)
        high = min(1.0, score + half_width)
    score_percent = round(score * 100.0, 8)
    low_percent = round(low * 100.0, 8)
    high_percent = round(high * 100.0, 8)
    if low_percent > 50.0:
        outcome = "confirmed"
    elif high_percent < 50.0:
        outcome = "rejected"
    else:
        outcome = "inconclusive"
    return {
        "blocks_completed": len(completed),
        "games": games,
        "wins": wins,
        "draws": draws,
        "losses": losses,
        "score": score_percent,
        "score_percent": score_percent,
        "score_ci_low": low_percent,
        "score_ci_high": high_percent,
        "confidence": confidence,
        "outcome": outcome,
        "block_ids": [str(block["block_id"]) for block in completed],
    }


class FakeConfirmationRunner:
    def __init__(self, database: Database, campaign_id: str, results: tuple[str, ...]) -> None:
        self.database = database
        self.campaign_id = campaign_id
        self.results = results

    def prepare_block(self, block_index: int, pairs_per_block: int) -> tuple[str, str]:
        return (
            sha256_json(["confirmation-fake-book-v1", self.campaign_id]),
            sha256_json(["confirmation-fake-openings-v1", self.campaign_id, block_index, pairs_per_block]),
        )

    def run(self, block: dict[str, Any]) -> dict[str, Any]:
        start = int(block["block_index"]) * int(block["pairs_per_block"]) * 2
        count = int(block["pairs_per_block"]) * 2
        games = list(self.results[start : start + count])
        if len(games) != count:
            raise ConfirmationError("fake confirmation result schedule is shorter than the block")
        wins = games.count("1-0")
        draws = games.count("1/2-1/2")
        losses = games.count("0-1")
        result = {
            "wins": wins,
            "draws": draws,
            "losses": losses,
            "score": (wins + draws / 2.0) * 100.0 / count,
            "games": games,
            "runner": "fake-confirmation-v1",
            "block_index": int(block["block_index"]),
        }
        self.database.complete_confirmation_block_atomically(
            self.campaign_id,
            str(block["block_id"]),
            wins,
            draws,
            losses,
            float(result["score"]),
            result,
        )
        return result


class RealConfirmationRunner:
    def __init__(
        self,
        database: Database,
        data_dir: Path,
        campaign_id: str,
        config: RealTestmonitorConfig,
        candidate_parameter_file: Path,
    ) -> None:
        self.database = database
        self.data_dir = data_dir.resolve()
        self.campaign_id = campaign_id
        self.config = replace(
            config,
            candidate_parameter_file=candidate_parameter_file.resolve(),
            opening_block_file=(campaign_dir(data_dir, campaign_id) / "confirmation" / "openings" / "placeholder.epd"),
        )
        self._process: subprocess.Popen[str] | None = None
        self._process_group_id: int | None = None

    def _opening_path(self, block_index: int) -> Path:
        return campaign_dir(self.data_dir, self.campaign_id) / "confirmation" / "openings" / f"block-{block_index:06d}.epd"

    def prepare_block(self, block_index: int, pairs_per_block: int) -> tuple[str, str]:
        path = self._opening_path(block_index)
        effective = replace(self.config, opening_block_file=path)
        path.parent.mkdir(parents=True, exist_ok=True)
        if not path.exists():
            _materialize_real_block(effective, self.config.seed, block_index=block_index, pairs=pairs_per_block)
        return sha256_bytes(effective.opening_book.read_bytes()), sha256_bytes(path.read_bytes())

    def run(self, block: dict[str, Any]) -> dict[str, Any]:
        block_index = int(block["block_index"])
        opening_path = self._opening_path(block_index)
        effective = replace(self.config, opening_block_file=opening_path, seed=int(block["master_seed"]))
        helper = RealTestmonitorScheduler(
            self.data_dir,
            self.campaign_id,
            effective,
            poll_interval=0.05,
            stop_grace_seconds=1.0,
        )
        run_dir = campaign_dir(self.data_dir, self.campaign_id) / "confirmation" / "runs" / (
            f"{block['block_id']}-attempt-{int(block['attempt']):04d}"
        )
        run_dir.mkdir(parents=True, exist_ok=True)
        result_path = run_dir / "result.json"
        log_path = run_dir / "monitor.log"
        command = helper._command(block, run_dir, result_path)
        process: subprocess.Popen[str] | None = None
        process_group_id: int | None = None
        try:
            with log_path.open("w", encoding="utf-8") as log:
                process = subprocess.Popen(
                    command,
                    stdin=subprocess.DEVNULL,
                    stdout=log,
                    stderr=subprocess.STDOUT,
                    text=True,
                    start_new_session=True,
                    cwd=effective.workdir,
                )
            process_group_id = os.getpgid(process.pid)
            self._process = process
            self._process_group_id = process_group_id
            self.database.set_confirmation_block_process(
                self.campaign_id,
                str(block["block_id"]),
                process.pid,
                process_group_id,
                str(run_dir),
                command,
            )
            while process.poll() is None:
                time.sleep(0.05)
            return_code = process.returncode
            terminate_process_group(process_group_id, 1.0)
            if return_code != 0:
                self.database.interrupt_confirmation_block(
                    self.campaign_id,
                    str(block["block_id"]),
                    f"testmonitor exited with status {return_code}",
                )
                raise ConfirmationError(f"confirmation testmonitor exited with status {return_code}")
            result = helper._read_result(result_path, int(block["pairs_per_block"]))
            self.database.complete_confirmation_block_atomically(
                self.campaign_id,
                str(block["block_id"]),
                int(result["wins"]),
                int(result["draws"]),
                int(result["losses"]),
                float(result["score"]),
                result,
            )
            return result
        except (InvalidTransition, DatabaseError):
            raise
        except Exception as exc:
            if process_group_id is not None:
                terminate_process_group(process_group_id, 1.0)
            try:
                self.database.interrupt_confirmation_block(self.campaign_id, str(block["block_id"]), str(exc))
            except DatabaseError:
                pass
            if isinstance(exc, ConfirmationError):
                raise
            if isinstance(exc, SchedulerError):
                raise ConfirmationError(str(exc)) from exc
            raise ConfirmationError(f"confirmation block failed: {exc}") from exc
        finally:
            self._process = None
            self._process_group_id = None


def terminate_active_confirmation_blocks(database: Database, campaign_id: str, reason: str) -> None:
    """Kill recorded confirmation process groups before replaying them."""
    for block in database.running_confirmation_block_processes(campaign_id):
        group_id = block.get("process_group_id")
        pid = block.get("pid")
        if group_id:
            terminate_process_group(int(group_id), 1.0)
        elif pid:
            try:
                os.kill(int(pid), 15)
            except ProcessLookupError:
                pass
        database.interrupt_confirmation_block(campaign_id, str(block["block_id"]), reason)


class ConfirmationCampaign:
    """Run all predeclared opening pairs without adaptive decisions."""

    def __init__(
        self,
        database: Database,
        campaign_id: str,
        settings: ConfirmationSettings,
        runner: ConfirmationBlockRunner,
    ) -> None:
        self.database = database
        self.campaign_id = campaign_id
        self.settings = settings
        self.runner = runner

    def _ensure_schedule(self) -> dict[str, Any]:
        campaign = self.database.campaign(self.campaign_id)
        existing = self.database.confirmation(self.campaign_id)
        if existing is None:
            raise ConfirmationError("confirmation record was not initialized")
        used_seeds: set[int] = {int(campaign["master_seed"])}
        with self.database._read() as connection:
            used_seeds.update(
                int(row["master_seed"])
                for row in connection.execute(
                    "SELECT master_seed FROM match_blocks WHERE campaign_id=?", (self.campaign_id,)
                ).fetchall()
            )
        if self.settings.seed in used_seeds:
            raise ConfirmationError("confirmation seed was already used by optimization")
        blocks = self.database.confirmation_blocks(self.campaign_id, str(existing["confirmation_id"]))
        expected = self.settings.games // 2
        if len(blocks) > expected:
            raise ConfirmationError("confirmation database contains too many blocks")
        for block_index in range(len(blocks), expected):
            book_hash, openings_hash = self.runner.prepare_block(block_index, 1)
            if openings_hash in {
                str(row["materialized_openings_sha256"])
                for row in self.database.confirmation_blocks(self.campaign_id, str(existing["confirmation_id"]))
            }:
                # An identical opening block is never a new confirmation pair.
                raise ConfirmationError("confirmation opening block was duplicated")
            self.database.create_confirmation_block(
                str(existing["confirmation_id"]),
                block_index,
                1,
                self.settings.seed,
                book_hash,
                openings_hash,
            )
        return self.database.confirmation_snapshot(self.campaign_id) or existing

    def run(self, max_blocks: int = 0) -> dict[str, Any]:
        if max_blocks < 0:
            raise ConfirmationError("max confirmation blocks cannot be negative")
        existing = self.database.confirmation(self.campaign_id)
        if existing is None:
            raise ConfirmationError("confirmation record was not initialized")
        if existing["candidate_parameter_hash"] == existing["baseline_parameter_hash"]:
            # A real runner must never be asked to play a self-match. There is
            # no changed candidate to confirm; the evidence is explicitly
            # inconclusive and carries no recommendation.
            result = {
                "blocks_completed": 0,
                "games": 0,
                "wins": 0,
                "draws": 0,
                "losses": 0,
                "score": 0.0,
                "score_percent": 0.0,
                "score_ci_low": 0.0,
                "score_ci_high": 100.0,
                "confidence": self.settings.confidence,
                "outcome": "inconclusive",
                "block_ids": [],
                "recommendation_parameter_hash": None,
                "recommendation": None,
                "automatic_promotion": False,
            }
            return self.database.finalize_confirmation(self.campaign_id, result)
        self._ensure_schedule()
        terminate_active_confirmation_blocks(
            self.database, self.campaign_id, "confirmation startup recovered abandoned job"
        )
        self.database.recover_abandoned_confirmation_jobs(self.campaign_id)
        completed_this_run = 0
        while True:
            snapshot = self.database.confirmation_snapshot(self.campaign_id)
            if snapshot is None:
                raise ConfirmationError("confirmation disappeared during execution")
            if snapshot["status"] == "completed":
                return snapshot
            block = self.database.claim_next_confirmation_block(self.campaign_id)
            if block is None:
                snapshot = self.database.confirmation_snapshot(self.campaign_id)
                if snapshot is None:
                    raise ConfirmationError("confirmation disappeared before finalization")
                blocks = snapshot["blocks"]
                if blocks and all(row["status"] == "completed" for row in blocks):
                    result = _summary(blocks, self.settings.confidence)
                    candidate_hash = str(snapshot["candidate_parameter_hash"])
                    if result["outcome"] == "confirmed":
                        result["recommendation_parameter_hash"] = candidate_hash
                        result["recommendation"] = "candidate"
                    else:
                        result["recommendation_parameter_hash"] = None
                        result["recommendation"] = None
                    result["automatic_promotion"] = False
                    return self.database.finalize_confirmation(self.campaign_id, result)
                raise ConfirmationError("confirmation has no runnable block but is not complete")
            self.runner.run(block)
            completed_this_run += 1
            if max_blocks and completed_this_run >= max_blocks:
                return self.database.confirmation_snapshot(self.campaign_id) or {}


def initialize_confirmation(
    database: Database,
    campaign_id: str,
    candidate_document: dict[str, Any],
    baseline_document: dict[str, Any],
    settings: ConfirmationSettings,
) -> str:
    if not settings.enabled:
        raise ConfirmationError("confirmation is disabled")
    return database.create_confirmation(
        campaign_id,
        candidate_document,
        baseline_document,
        settings.games,
        settings.seed,
        settings.confidence,
    )
