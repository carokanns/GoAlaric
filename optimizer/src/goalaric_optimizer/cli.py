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
    except (DatabaseError, InvalidTransition, ServiceError, SchedulerError, ValueError) as exc:
        print(f"goalaric_optimizer: {exc}", file=sys.stderr)
        return 1
