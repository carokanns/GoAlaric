from __future__ import annotations

import argparse
import json
import shlex
import sys
import time
from pathlib import Path
from typing import Sequence

from .database import DatabaseError, InvalidTransition
from .service import (
    ServiceError,
    database_for,
    default_data_dir,
    init_campaign,
    load_database,
    pause_campaign,
    resume_campaign,
    run_campaign,
    stop_campaign,
)
from .scheduler import SchedulerError, run_fake_scheduler
from .coordinate import (
    CoordinateSearchError,
    run_synthetic_coordinate_search,
    run_synthetic_multiresolution_search,
)
from .real_integration import RealTestmonitorConfig, run_real_testmonitor
from .registry import load_parameter_file, load_registry
from .adaptive import AdaptiveError, AdaptivePolicy, run_real_adaptive_campaign
from .dashboard import DashboardError, final_report, serve_dashboard
from .optimization import OptimizationError, run_fake_optimization


def _data_dir(value: str | None) -> Path:
    return Path(value).expanduser().resolve() if value else default_data_dir()


def _add_data_dir(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--data-dir", help="campaign storage directory; defaults to optimizer/campaigns")


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="goalaric_optimizer")
    commands = parser.add_subparsers(dest="command", required=True)

    init = commands.add_parser("init", help="validate and initialize a campaign")
    init.add_argument("campaign", type=Path)
    _add_data_dir(init)

    run = commands.add_parser("run", help="enter the fake campaign running state")
    run.add_argument("campaign_id")
    run.add_argument("--fake", action="store_true", help="explicitly select the phase-5 fake execution path")
    run.add_argument(
        "--monitor-command",
        help="phase-6 fake testmonitor command as one shell-like string; it must write the supplied result JSON",
    )
    run.add_argument("--blocks", type=int, default=1, help="number of deterministic fake blocks")
    run.add_argument("--pairs-per-block", type=int, default=1)
    run.add_argument("--poll-interval", type=float, default=0.05)
    _add_data_dir(run)

    coordinate = commands.add_parser("coordinate", help="run the phase-7 synthetic coordinate search")
    coordinate.add_argument("campaign_id")
    coordinate.add_argument("--registry", type=Path, required=True)
    coordinate.add_argument("--fake-optimum", required=True, help="JSON object mapping parameter names to optimum values")
    coordinate.add_argument("--max-results", type=int, default=0)
    coordinate.add_argument("--max-passes", type=int, default=100)
    coordinate.add_argument(
        "--uncertain-values",
        default="[]",
        help="JSON list of [parameter, value] pairs that return an uncertain result",
    )
    _add_data_dir(coordinate)

    multires = commands.add_parser(
        "coordinate-multires",
        aliases=["multires-coordinate"],
        help="run the phase-11 synthetic multi-resolution coordinate search",
    )
    multires.add_argument("campaign_id")
    multires.add_argument("--registry", type=Path, required=True)
    multires.add_argument("--fake-optimum", required=True, help="JSON object mapping parameter names to optimum values")
    multires.add_argument("--max-results", type=int, default=0)
    multires.add_argument("--max-passes", type=int, default=100)
    multires.add_argument(
        "--parameters",
        default="",
        help="comma-separated registry parameter names; defaults to all parameters",
    )
    multires.add_argument(
        "--uncertain-values",
        default="[]",
        help="JSON list of [parameter, value] pairs that return an uncertain result",
    )
    _add_data_dir(multires)

    optimize = commands.add_parser(
        "optimize",
        help="run or resume autonomous multi-resolution optimization with the fake match runner",
    )
    optimize.add_argument("campaign", type=Path)
    optimize.add_argument(
        "--max-results",
        type=int,
        default=0,
        help="work quota for this invocation; 0 runs until a terminal search or campaign budget",
    )
    optimize.add_argument("--max-games", type=int, help="override the campaign-wide fake match budget")
    optimize.add_argument(
        "--max-evaluations", type=int, help="override the campaign-wide candidate evaluation budget"
    )
    _add_data_dir(optimize)

    real = commands.add_parser(
        "real-run",
        aliases=["run-real"],
        help="run one real testmonitor opening block through the SQLite scheduler",
    )
    real.add_argument("campaign_id")
    real.add_argument("--registry", type=Path, required=True)
    real.add_argument("--testmonitor-command", required=True, help="testmonitor executable command")
    real.add_argument("--fastchess", type=Path, required=True)
    real.add_argument("--baseline", type=Path, required=True)
    real.add_argument("--candidate", type=Path, required=True)
    real.add_argument("--baseline-parameter-file", type=Path)
    real.add_argument("--candidate-parameter-file", type=Path, required=True)
    real.add_argument("--opening-book", type=Path, required=True)
    real.add_argument("--opening-block-file", type=Path, required=True)
    real.add_argument("--tc", default="10+0.1")
    real.add_argument("--seed", type=int, default=0)
    real.add_argument("--hash", dest="hash_mb", type=int, default=16)
    real.add_argument("--threads", type=int, default=1)
    real.add_argument("--workdir", type=Path)
    real.add_argument("--poll-interval", type=float, default=0.05)
    real.add_argument("--stop-grace-seconds", type=float, default=1.0)
    _add_data_dir(real)

    adaptive = commands.add_parser(
        "adaptive-real",
        aliases=["real-adaptive"],
        help="run one real candidate through deterministic adaptive blocks",
    )
    adaptive.add_argument("campaign_id")
    adaptive.add_argument("--registry", type=Path, required=True)
    adaptive.add_argument("--testmonitor-command", required=True)
    adaptive.add_argument("--fastchess", type=Path, required=True)
    adaptive.add_argument("--baseline", type=Path, required=True)
    adaptive.add_argument("--candidate", type=Path, required=True)
    adaptive.add_argument("--baseline-parameter-file", type=Path)
    adaptive.add_argument("--candidate-parameter-file", type=Path, required=True)
    adaptive.add_argument("--opening-book", type=Path, required=True)
    adaptive.add_argument("--tc", default="10+0.1")
    adaptive.add_argument("--seed", type=int, default=0)
    adaptive.add_argument("--hash", dest="hash_mb", type=int, default=16)
    adaptive.add_argument("--threads", type=int, default=1)
    adaptive.add_argument("--workdir", type=Path)
    adaptive.add_argument("--min-blocks", type=int, default=1)
    adaptive.add_argument("--max-blocks", type=int, default=4)
    adaptive.add_argument("--weak-upper-score", type=float, default=45.0)
    adaptive.add_argument("--target-score", type=float, default=50.0)
    _add_data_dir(adaptive)

    dashboard = commands.add_parser(
        "dashboard", help="serve a local read-only campaign dashboard on 127.0.0.1"
    )
    dashboard.add_argument("campaign_id")
    dashboard.add_argument("--listen", default="127.0.0.1:8787")
    dashboard.add_argument("--refresh-ms", type=int, default=2000)
    _add_data_dir(dashboard)

    report = commands.add_parser(
        "dashboard-report", aliases=["report"], help="write a final report for a finished campaign"
    )
    report.add_argument("campaign_id")
    report.add_argument("--format", choices=("html", "json"), default="html")
    report.add_argument("--output", type=Path)
    _add_data_dir(report)

    status = commands.add_parser("status", help="read campaign status")
    status.add_argument("campaign_id")
    status.add_argument("--watch", action="store_true")
    status.add_argument("--interval", type=float, default=1.0)
    status.add_argument("--iterations", type=int, default=0, help="watch iterations; 0 means until interrupted")
    _add_data_dir(status)

    for name, function in (("pause", "pause"), ("resume", "resume"), ("stop", "stop")):
        command = commands.add_parser(name, help=f"{function} a campaign")
        command.add_argument("campaign_id")
        _add_data_dir(command)

    best = commands.add_parser("best", help="read the latest completed trial")
    best.add_argument("campaign_id")
    _add_data_dir(best)

    trials = commands.add_parser("trials", help="read recent trials")
    trials.add_argument("campaign_id")
    trials.add_argument("--last", type=int, default=20)
    _add_data_dir(trials)

    show = commands.add_parser("show", help="read a trial")
    show.add_argument("campaign_id")
    show.add_argument("trial_id")
    _add_data_dir(show)
    return parser


def _print(value: object) -> None:
    print(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True))


def _status_once(data_dir: Path, campaign_id: str) -> dict[str, object]:
    return load_database(data_dir, campaign_id).status_snapshot(campaign_id)


def main(argv: Sequence[str] | None = None) -> int:
    args = _parser().parse_args(argv)
    try:
        data_dir = _data_dir(getattr(args, "data_dir", None))
        if args.command == "init":
            definition, created, db_path = init_campaign(args.campaign, data_dir)
            _print(
                {
                    "campaign_id": definition.campaign_id,
                    "created": created,
                    "database": str(db_path),
                    "config_hash": definition.config_hash,
                    "baseline_parameter_hash": definition.baseline_parameter_hash,
                    "registry": definition.registry.name,
                    "journal_mode": "wal",
                }
            )
            return 0
        if args.command == "run":
            if args.monitor_command:
                if not args.fake:
                    raise ServiceError("--monitor-command is only available with --fake in phase 6")
                _print(
                    run_fake_scheduler(
                        data_dir,
                        args.campaign_id,
                        shlex.split(args.monitor_command),
                        block_count=args.blocks,
                        pairs_per_block=args.pairs_per_block,
                        poll_interval=args.poll_interval,
                    )
                )
            else:
                _print(run_campaign(data_dir, args.campaign_id, fake=True))
            return 0
        if args.command == "optimize":
            _print(
                run_fake_optimization(
                    args.campaign,
                    data_dir,
                    invocation_limit=args.max_results,
                    max_games_override=args.max_games,
                    max_evaluations_override=args.max_evaluations,
                )
            )
            return 0
        if args.command == "coordinate":
            optimum = json.loads(args.fake_optimum)
            if not isinstance(optimum, dict):
                raise ServiceError("--fake-optimum must be a JSON object")
            if any(not isinstance(value, int) or isinstance(value, bool) for value in optimum.values()):
                raise ServiceError("--fake-optimum values must be integers")
            uncertain_raw = json.loads(args.uncertain_values)
            if not isinstance(uncertain_raw, list):
                raise ServiceError("--uncertain-values must be a JSON list")
            uncertain_values: set[tuple[str, int]] = set()
            for item in uncertain_raw:
                if (
                    not isinstance(item, list)
                    or len(item) != 2
                    or not isinstance(item[0], str)
                    or not isinstance(item[1], int)
                    or isinstance(item[1], bool)
                ):
                    raise ServiceError("each uncertain value must be [parameter, integer]")
                uncertain_values.add((item[0], item[1]))
            registry = load_registry(args.registry.resolve())
            _print(
                run_synthetic_coordinate_search(
                    data_dir,
                    args.campaign_id,
                    registry,
                    {str(name): int(value) for name, value in optimum.items()},
                    max_results=args.max_results,
                    max_passes=args.max_passes,
                    uncertain_values=uncertain_values,
                )
            )
            return 0
        if args.command in {"coordinate-multires", "multires-coordinate"}:
            optimum = json.loads(args.fake_optimum)
            if not isinstance(optimum, dict):
                raise ServiceError("--fake-optimum must be a JSON object")
            if any(not isinstance(value, int) or isinstance(value, bool) for value in optimum.values()):
                raise ServiceError("--fake-optimum values must be integers")
            uncertain_raw = json.loads(args.uncertain_values)
            if not isinstance(uncertain_raw, list):
                raise ServiceError("--uncertain-values must be a JSON list")
            uncertain_values: set[tuple[str, int]] = set()
            for item in uncertain_raw:
                if (
                    not isinstance(item, list)
                    or len(item) != 2
                    or not isinstance(item[0], str)
                    or not isinstance(item[1], int)
                    or isinstance(item[1], bool)
                ):
                    raise ServiceError("each uncertain value must be [parameter, integer]")
                uncertain_values.add((item[0], item[1]))
            registry = load_registry(args.registry.resolve())
            parameter_names = [item.strip() for item in args.parameters.split(",") if item.strip()] or None
            _print(
                run_synthetic_multiresolution_search(
                    data_dir,
                    args.campaign_id,
                    registry,
                    {str(name): int(value) for name, value in optimum.items()},
                    max_results=args.max_results,
                    max_passes=args.max_passes,
                    uncertain_values=uncertain_values,
                    parameter_names=parameter_names,
                )
            )
            return 0
        if args.command in {"real-run", "run-real"}:
            registry = load_registry(args.registry.resolve())
            candidate_file = args.candidate_parameter_file.resolve()
            candidate_document, _ = load_parameter_file(candidate_file, registry)
            baseline_file = (
                args.baseline_parameter_file.resolve()
                if args.baseline_parameter_file is not None
                else (data_dir / args.campaign_id / "baseline-parameters.json").resolve()
            )
            baseline_document, _ = load_parameter_file(baseline_file, registry)
            config = RealTestmonitorConfig(
                testmonitor_command=shlex.split(args.testmonitor_command),
                fastchess=args.fastchess.resolve(),
                baseline=args.baseline.resolve(),
                candidate=args.candidate.resolve(),
                baseline_parameter_file=baseline_file,
                candidate_parameter_file=candidate_file,
                opening_book=args.opening_book.resolve(),
                opening_block_file=args.opening_block_file.resolve(),
                tc=args.tc,
                seed=args.seed,
                hash_mb=args.hash_mb,
                threads=args.threads,
                workdir=args.workdir.resolve() if args.workdir is not None else None,
            )
            if baseline_document.get("registry") != candidate_document.get("registry"):
                raise ServiceError("baseline and candidate parameter files use different registries")
            _print(
                run_real_testmonitor(
                    data_dir,
                    args.campaign_id,
                    config,
                    candidate_document,
                    registry=registry,
                    poll_interval=args.poll_interval,
                    stop_grace_seconds=args.stop_grace_seconds,
                )
            )
            return 0
        if args.command in {"adaptive-real", "real-adaptive"}:
            registry = load_registry(args.registry.resolve())
            candidate_file = args.candidate_parameter_file.resolve()
            candidate_document, _ = load_parameter_file(candidate_file, registry)
            baseline_file = (
                args.baseline_parameter_file.resolve()
                if args.baseline_parameter_file is not None
                else (data_dir / args.campaign_id / "baseline-parameters.json").resolve()
            )
            load_parameter_file(baseline_file, registry)
            config = RealTestmonitorConfig(
                testmonitor_command=shlex.split(args.testmonitor_command),
                fastchess=args.fastchess.resolve(),
                baseline=args.baseline.resolve(),
                candidate=args.candidate.resolve(),
                baseline_parameter_file=baseline_file,
                candidate_parameter_file=candidate_file,
                opening_book=args.opening_book.resolve(),
                opening_block_file=(data_dir / args.campaign_id / "adaptive-opening-block.epd").resolve(),
                tc=args.tc,
                seed=args.seed,
                hash_mb=args.hash_mb,
                threads=args.threads,
                workdir=args.workdir.resolve() if args.workdir is not None else None,
            )
            _print(
                run_real_adaptive_campaign(
                    data_dir,
                    args.campaign_id,
                    config,
                    candidate_document,
                    AdaptivePolicy(
                        min_blocks=args.min_blocks,
                        max_blocks=args.max_blocks,
                        weak_upper_score=args.weak_upper_score,
                        target_score=args.target_score,
                    ),
                )
            )
            return 0
        if args.command == "dashboard":
            serve_dashboard(data_dir, args.campaign_id, args.listen, args.refresh_ms)
            return 0
        if args.command in {"dashboard-report", "report"}:
            snapshot, content = final_report(data_dir, args.campaign_id, args.format)
            if args.output is None:
                print(content, end="")
            else:
                args.output.parent.mkdir(parents=True, exist_ok=True)
                args.output.write_text(content, encoding="utf-8")
                print(json.dumps({"campaign_id": snapshot["campaign"]["campaign_id"], "format": args.format, "output": str(args.output)}))
            return 0
        if args.command == "status":
            if not args.watch:
                _print(_status_once(data_dir, args.campaign_id))
                return 0
            if args.interval <= 0 or args.iterations < 0:
                raise ServiceError("status --interval must be positive and --iterations cannot be negative")
            count = 0
            while args.iterations == 0 or count < args.iterations:
                _print(_status_once(data_dir, args.campaign_id))
                count += 1
                if args.iterations == 0 or count < args.iterations:
                    time.sleep(args.interval)
            return 0
        if args.command == "pause":
            _print(pause_campaign(data_dir, args.campaign_id))
            return 0
        if args.command == "resume":
            _print(resume_campaign(data_dir, args.campaign_id))
            return 0
        if args.command == "stop":
            _print(stop_campaign(data_dir, args.campaign_id))
            return 0
        database = load_database(data_dir, args.campaign_id)
        if args.command == "best":
            _print(database.best_trial(args.campaign_id))
            return 0
        if args.command == "trials":
            _print(database.list_trials(args.campaign_id, args.last))
            return 0
        if args.command == "show":
            trials = [item for item in database.list_trials(args.campaign_id, 10000) if item["trial_id"] == args.trial_id]
            if not trials:
                raise ServiceError(f"unknown trial: {args.trial_id}")
            _print(trials[0])
            return 0
        raise ServiceError(f"unsupported command: {args.command}")
    except (
        DatabaseError,
        InvalidTransition,
        ServiceError,
        SchedulerError,
        AdaptiveError,
        CoordinateSearchError,
        DashboardError,
        OptimizationError,
        ValueError,
    ) as exc:
        print(f"goalaric_optimizer: {exc}", file=sys.stderr)
        return 1
