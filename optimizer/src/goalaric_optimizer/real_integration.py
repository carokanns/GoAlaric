"""Phase-8 bridge from the SQLite scheduler to the real Go testmonitor."""

from __future__ import annotations

import json
import math
import subprocess
from dataclasses import dataclass, replace
from pathlib import Path
from typing import Any, Sequence

from .canonical import sha256_bytes, sha256_json
from .database import Database, DatabaseError
from .registry import Registry
from .scheduler import Scheduler, SchedulerError
from .service import load_database


@dataclass(frozen=True)
class RealTestmonitorConfig:
    """The complete, explicit input set for one real opening block."""

    testmonitor_command: Sequence[str]
    fastchess: Path
    baseline: Path
    candidate: Path
    baseline_parameter_file: Path
    candidate_parameter_file: Path
    opening_book: Path
    opening_block_file: Path
    tc: str = "10+0.1"
    seed: int = 0
    concurrency: int = 1
    hash_mb: int = 16
    threads: int = 1
    syzygy_path: str = "off"
    workdir: Path | None = None


def _read_object(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise SchedulerError(f"could not read JSON object {path}: {exc}") from exc
    if not isinstance(value, dict):
        raise SchedulerError(f"JSON artifact is not an object: {path}")
    return value


def _require_int(value: Any, name: str, minimum: int = 0) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or value < minimum:
        raise SchedulerError(f"{name} must be an integer >= {minimum}")
    return value


def _parameter_values(document: dict[str, Any]) -> dict[str, int]:
    values = document.get("parameters")
    if not isinstance(values, list):
        raise SchedulerError("parameter document has no parameters list")
    result: dict[str, int] = {}
    for item in values:
        if not isinstance(item, dict) or not isinstance(item.get("name"), str) or not isinstance(item.get("value"), int):
            raise SchedulerError("parameter document contains an invalid parameter")
        result[item["name"]] = item["value"]
    return result


def validate_single_parameter_step(
    baseline: dict[str, Any], candidate: dict[str, Any], registry: Registry | None = None
) -> None:
    """Require the phase-8 candidate to be baseline plus one registry step."""

    base_values = _parameter_values(baseline)
    candidate_values = _parameter_values(candidate)
    if set(base_values) != set(candidate_values):
        raise SchedulerError("baseline and candidate parameter names differ")
    changed = [(name, candidate_values[name] - value) for name, value in base_values.items() if candidate_values[name] != value]
    if len(changed) != 1:
        raise SchedulerError(f"phase 8 requires exactly one changed parameter, got {len(changed)}")
    name, delta = changed[0]
    if registry is not None:
        descriptor = next((item for item in registry.parameters if item["name"] == name), None)
        if descriptor is None:
            raise SchedulerError(f"candidate parameter is not in the registry: {name}")
        # The checked-in Go registry deliberately contains only stable
        # names/defaults; its pilot registry step is one when metadata is not
        # exported alongside the Python registry.
        step = descriptor.get("step", 1)
        if isinstance(step, int) and abs(delta) != step:
            raise SchedulerError(f"parameter {name} changed by {delta}; expected one step of {step}")


def _ensure_real_schedule(
    database: Database,
    campaign_id: str,
    candidate_document: dict[str, Any],
    seed: int,
    opening_book_sha256: str,
    materialized_openings_sha256: str,
) -> tuple[str, str]:
    campaign = database.campaign(campaign_id)
    config = json.loads(campaign["config_json"])
    if config.get("mode") != "real":
        raise DatabaseError("the real testmonitor requires a campaign with mode=real")
    candidate_id = database.add_parameter_set(campaign_id, candidate_document, group_name="candidate")
    if database.parameter_set(candidate_id, campaign_id)["parameter_hash"] == campaign["baseline_parameter_hash"]:
        raise SchedulerError("candidate parameter hash is identical to the baseline")
    trial_id = database.create_trial(campaign_id, candidate_id, "real-testmonitor", seed)
    block_id = database.create_match_block(
        campaign_id,
        trial_id,
        "real-e2e",
        0,
        1,
        seed,
        opening_book_sha256,
        materialized_openings_sha256,
    )
    return trial_id, block_id


def _materialize_real_block(
    config: RealTestmonitorConfig, seed: int, block_index: int = 0, pairs: int = 1
) -> None:
    command = [
        *[str(item) for item in config.testmonitor_command],
        "materialize-openings",
        "--openings",
        str(config.opening_book.resolve()),
        "--seed",
        str(seed),
        "--block-index",
        str(block_index),
        "--pairs",
        str(pairs),
        "--output",
        str(config.opening_block_file.resolve()),
    ]
    completed = subprocess.run(
        command,
        cwd=config.workdir.resolve() if config.workdir is not None else None,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        check=False,
    )
    if completed.returncode != 0:
        detail = completed.stdout.strip()
        raise SchedulerError(f"testmonitor could not materialize the phase-8 block: {detail}")


class RealTestmonitorScheduler(Scheduler):
    """Run the Go testmonitor while retaining phase-6 process guarantees."""

    def __init__(
        self,
        data_dir: Path,
        campaign_id: str,
        config: RealTestmonitorConfig,
        poll_interval: float = 0.05,
        stop_grace_seconds: float = 1.0,
        preserve_optimizer_state: bool = False,
    ) -> None:
        super().__init__(
            data_dir,
            campaign_id,
            config.testmonitor_command,
            poll_interval=poll_interval,
            stop_grace_seconds=stop_grace_seconds,
            workdir=config.workdir,
            preserve_optimizer_state=preserve_optimizer_state,
        )
        if not config.testmonitor_command:
            raise SchedulerError("testmonitor command cannot be empty")
        if config.concurrency != 1:
            raise SchedulerError("phase 8 requires concurrency=1")
        if config.hash_mb < 16 or config.threads < 1:
            raise SchedulerError("phase 8 requires hash >= 16 and positive threads")
        self.config = replace(
            config,
            fastchess=config.fastchess.resolve(),
            baseline=config.baseline.resolve(),
            candidate=config.candidate.resolve(),
            baseline_parameter_file=config.baseline_parameter_file.resolve(),
            candidate_parameter_file=config.candidate_parameter_file.resolve(),
            opening_book=config.opening_book.resolve(),
            opening_block_file=config.opening_block_file.resolve(),
            workdir=config.workdir.resolve() if config.workdir is not None else None,
        )

    def _command(self, block: dict[str, Any], run_dir: Path, result_path: Path) -> list[str]:
        games = int(block["pairs_per_block"]) * 2
        return [
            *self.monitor_command,
            "run-match",
            "--fastchess",
            str(self.config.fastchess),
            "--baseline",
            str(self.config.baseline),
            "--candidate",
            str(self.config.candidate),
            "--baseline-parameter-file",
            str(self.config.baseline_parameter_file),
            "--candidate-parameter-file",
            str(self.config.candidate_parameter_file),
            "--optimizer-mode",
            "--openings",
            str(self.config.opening_book),
            "--opening-block-file",
            str(self.config.opening_block_file),
            "--block-index",
            str(block["block_index"]),
            "--block-size",
            str(block["pairs_per_block"]),
            "--seed",
            str(block["master_seed"]),
            "--games",
            str(games),
            "--tc",
            self.config.tc,
            "--concurrency",
            str(self.config.concurrency),
            "--progress-games",
            "1",
            "--progress-interval",
            "0",
            "--hash",
            str(self.config.hash_mb),
            "--threads",
            str(self.config.threads),
            "--syzygy-path",
            self.config.syzygy_path,
            "--run-dir",
            str(run_dir),
        ]

    def _read_result(self, path: Path, pairs_per_block: int) -> dict[str, Any]:
        run_dir = path.parent
        report = _read_object(run_dir / "block-report.json")
        status = _read_object(run_dir / "status.json")
        monitor_config = _read_object(run_dir / "monitor-config.json")
        expected_games = pairs_per_block * 2

        if report.get("schema_version") != 1 or report.get("state") != "completed":
            raise SchedulerError("real testmonitor block-report is not a completed phase-8 report")
        if report.get("valid") is not True or report.get("counted") is not True:
            raise SchedulerError("real testmonitor block-report is not valid and counted")
        if Path(str(report.get("run_dir", ""))).resolve() != run_dir.resolve():
            raise SchedulerError("block-report run directory does not match the scheduler run")
        if _require_int(report.get("opening_block_size"), "report opening_block_size", 1) != pairs_per_block:
            raise SchedulerError("block-report opening block size does not match SQLite")
        if _require_int(report.get("target_games"), "report target_games", 1) != expected_games:
            raise SchedulerError("block-report target games do not match SQLite")
        if _require_int(report.get("games"), "report games") != expected_games:
            raise SchedulerError("block-report game count is incomplete")

        counts = [_require_int(report.get(name), f"report {name}") for name in ("wins", "draws", "losses")]
        if sum(counts) != expected_games:
            raise SchedulerError("block-report W-D-L does not add up to the block size")
        score = report.get("score_percent")
        if not isinstance(score, (int, float)) or isinstance(score, bool) or not 0 <= float(score) <= 100:
            raise SchedulerError("block-report score must be a percentage from 0 to 100")
        expected_score = (counts[0] + counts[1] / 2) * 100 / expected_games
        if not math.isclose(float(score), expected_score, abs_tol=0.01):
            raise SchedulerError("block-report score does not match W-D-L")

        if status.get("state") != "completed" or _require_int(status.get("games"), "status games") != expected_games:
            raise SchedulerError("status.json is not a completed full block")
        for name, expected in zip(("wins", "draws", "losses"), counts):
            if _require_int(status.get(name), f"status {name}") != expected:
                raise SchedulerError("status.json W-D-L differs from block-report")
        baseline_identity = status.get("baseline_identity")
        candidate_identity = status.get("candidate_identity")
        if not isinstance(baseline_identity, dict) or not isinstance(candidate_identity, dict):
            raise SchedulerError("status.json is missing baseline/candidate identities")
        if baseline_identity.get("sha256") != candidate_identity.get("sha256"):
            raise SchedulerError("phase-8 engines are not the same binary")
        if not baseline_identity.get("parameter_sha256") or not candidate_identity.get("parameter_sha256"):
            raise SchedulerError("status.json is missing parameter-file identities")
        if baseline_identity["parameter_sha256"] == candidate_identity["parameter_sha256"]:
            raise SchedulerError("phase-8 baseline and candidate parameter identities are identical")
        if baseline_identity.get("parameter_register_version") != candidate_identity.get("parameter_register_version"):
            raise SchedulerError("baseline and candidate parameter registry versions differ")

        configured_paths = {
            "baseline_parameter_file": self.config.baseline_parameter_file,
            "candidate_parameter_file": self.config.candidate_parameter_file,
        }
        for field, expected in configured_paths.items():
            actual = monitor_config.get(field)
            if not isinstance(actual, str) or Path(actual).resolve() != expected:
                raise SchedulerError(f"monitor-config.json has the wrong {field}")
        if monitor_config.get("optimizer_mode") is not True:
            raise SchedulerError("real phase-8 match did not run in optimizer mode")
        block_hash = sha256_bytes(self.config.opening_block_file.read_bytes())
        if report.get("opening_block_sha256") != block_hash:
            raise SchedulerError("block-report opening identity differs from the supplied block")
        if report.get("color_swap") is not True:
            raise SchedulerError("real phase-8 block was not marked as color-swapped")

        game_results = ["1-0"] * counts[0] + ["1/2-1/2"] * counts[1] + ["0-1"] * counts[2]
        baseline_parameter_hash = sha256_json(_read_object(self.config.baseline_parameter_file))
        candidate_parameter_hash = sha256_json(_read_object(self.config.candidate_parameter_file))
        return {
            "wins": counts[0],
            "draws": counts[1],
            "losses": counts[2],
            "score": float(score),
            "games": game_results,
            "block_report": report,
            "status": status,
            "monitor_config": monitor_config,
            "identities": {"baseline": baseline_identity, "candidate": candidate_identity},
            "runner": "real-testmonitor-v1",
            "reference_parameter_hash": baseline_parameter_hash,
            "candidate_parameter_hash": candidate_parameter_hash,
        }


def run_real_testmonitor(
    data_dir: Path,
    campaign_id: str,
    config: RealTestmonitorConfig,
    candidate_document: dict[str, Any],
    registry: Registry | None = None,
    poll_interval: float = 0.05,
    stop_grace_seconds: float = 1.0,
) -> dict[str, Any]:
    """Create one real trial/block and run it through the SQLite scheduler."""

    database = load_database(data_dir, campaign_id)
    with database._read() as connection:
        row = connection.execute(
            "SELECT document_json FROM parameter_sets WHERE campaign_id=? AND group_name='baseline' ORDER BY created_at LIMIT 1",
            (campaign_id,),
        ).fetchone()
    if row is None:
        raise DatabaseError("baseline parameter set is missing")
    baseline_document = json.loads(row["document_json"])
    validate_single_parameter_step(baseline_document, candidate_document, registry)
    campaign = database.campaign(campaign_id)
    seed = config.seed or int(campaign["master_seed"])
    effective = replace(
        config,
        seed=seed,
        fastchess=config.fastchess.resolve(),
        baseline=config.baseline.resolve(),
        candidate=config.candidate.resolve(),
        baseline_parameter_file=config.baseline_parameter_file.resolve(),
        candidate_parameter_file=config.candidate_parameter_file.resolve(),
        opening_book=config.opening_book.resolve(),
        opening_block_file=config.opening_block_file.resolve(),
    )
    if not effective.opening_book.exists():
        raise SchedulerError(f"opening book does not exist: {effective.opening_book}")
    _materialize_real_block(effective, seed)
    opening_book_hash = sha256_bytes(effective.opening_book.read_bytes())
    block_hash = sha256_bytes(effective.opening_block_file.read_bytes())
    _ensure_real_schedule(database, campaign_id, candidate_document, seed, opening_book_hash, block_hash)
    if database.campaign(campaign_id)["status"] == "completed":
        return database.status_snapshot(campaign_id)
    return RealTestmonitorScheduler(
        data_dir,
        campaign_id,
        effective,
        poll_interval=poll_interval,
        stop_grace_seconds=stop_grace_seconds,
    ).run()
