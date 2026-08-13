from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
import urllib.request
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).parents[2] / "optimizer" / "src"))

from goalaric_optimizer.canonical import atomic_write_json, sha256_json
from goalaric_optimizer.config import load_campaign_definition
from goalaric_optimizer.dashboard import DashboardReader, final_report
from goalaric_optimizer.database import Database
from goalaric_optimizer.registry import default_parameter_document, load_registry
from goalaric_optimizer.service import init_campaign, pause_campaign, resume_campaign, stop_campaign


PARAMETER_BOUNDS = {
    "mobility_weight": (0, 64),
    "mobility_shift": (1, 16),
    "activity_bias": (0, 32),
    "activity_shift": (1, 8),
    "activity_knight_weight": (0, 16),
    "activity_bishop_weight": (0, 16),
    "activity_rook_weight": (0, 16),
    "activity_queen_weight": (0, 16),
}


def write_jsonl(path: Path, value: dict[str, Any]) -> None:
    with path.open("a", encoding="utf-8") as stream:
        stream.write(json.dumps(value, ensure_ascii=False, sort_keys=True) + "\n")
        stream.flush()
        os.fsync(stream.fileno())


def process_paths(paths: tuple[Path, ...]) -> list[int]:
    found: list[int] = []
    for proc in Path("/proc").glob("[0-9]*"):
        if int(proc.name) == os.getpid():
            continue
        try:
            command_line = (proc / "cmdline").read_bytes().replace(b"\0", b" ").decode(errors="replace")
        except OSError:
            continue
        if any(str(path) in command_line for path in paths):
            found.append(int(proc.name))
    return sorted(found)


def candidate_documents(registry_path: Path) -> list[tuple[str, dict[str, Any]]]:
    registry = load_registry(registry_path)
    baseline = default_parameter_document(registry)
    values = {item["name"]: item["value"] for item in baseline["parameters"]}
    candidates: list[tuple[str, dict[str, Any]]] = []
    for item in baseline["parameters"]:
        name = str(item["name"])
        minimum, maximum = PARAMETER_BOUNDS[name]
        for direction, label in ((-1, "minus"), (1, "plus")):
            value = values[name] + direction
            if not minimum <= value <= maximum:
                continue
            document = {
                "schema_version": baseline["schema_version"],
                "registry": baseline["registry"],
                "parameters": [
                    {"name": entry["name"], "value": value if entry["name"] == name else entry["value"]}
                    for entry in baseline["parameters"]
                ],
            }
            candidates.append((f"{len(candidates) + 1:02d}-{name}-{label}", document))
    return candidates


def build_command(args: argparse.Namespace, candidate_file: Path) -> list[str]:
    return [
        sys.executable,
        "-m",
        "goalaric_optimizer",
        "adaptive-real",
        args.campaign_id,
        "--data-dir",
        str(args.data_dir),
        "--registry",
        str(args.registry),
        "--testmonitor-command",
        str(args.testmonitor),
        "--fastchess",
        str(args.fastchess),
        "--baseline",
        str(args.engine),
        "--candidate",
        str(args.engine),
        "--baseline-parameter-file",
        str(args.baseline_file),
        "--candidate-parameter-file",
        str(candidate_file),
        "--opening-book",
        str(args.opening_book),
        "--tc",
        args.tc,
        "--seed",
        str(args.seed),
        "--hash",
        str(args.hash_mb),
        "--threads",
        str(args.threads),
        "--workdir",
        str(args.repo),
        "--min-blocks",
        "1",
        "--max-blocks",
        str(args.max_blocks),
        "--weak-upper-score",
        str(args.weak_upper_score),
        "--target-score",
        "50",
    ]


def dashboard_sample(reader: DashboardReader, samples_path: Path, label: str) -> dict[str, Any]:
    snapshot = reader.snapshot()
    write_jsonl(
        samples_path,
        {
            "label": label,
            "read_only": snapshot["read_only"],
            "status": snapshot["campaign"]["status"],
            "current_trial": (snapshot.get("current_trial") or {}).get("trial_id"),
            "current_block": ((snapshot.get("current_trial") or {}).get("current_block") or {}).get("block_index"),
            "consumed_games": snapshot["consumed_games"],
            "candidate_counts": snapshot["candidate_counts"],
            "checkpoint": snapshot.get("checkpoint"),
        },
    )
    return snapshot


def run_candidate(
    args: argparse.Namespace,
    reader: DashboardReader,
    candidate_file: Path,
    candidate_label: str,
    index: int,
    logs_dir: Path,
    samples_path: Path,
    controls_path: Path,
) -> None:
    env = dict(os.environ)
    env["PYTHONPATH"] = str(args.repo / "optimizer" / "src")
    attempt = 0
    control = {0: "pause", 7: "stop", 14: "pause"}.get(index)
    control_used = False
    started = time.monotonic()
    while True:
        attempt += 1
        log_path = logs_dir / f"{candidate_label}-attempt-{attempt:02d}.log"
        with log_path.open("w", encoding="utf-8") as log:
            process = subprocess.Popen(
                build_command(args, candidate_file),
                cwd=args.repo,
                env=env,
                stdout=log,
                stderr=subprocess.STDOUT,
                text=True,
            )
        while process.poll() is None:
            snapshot = dashboard_sample(reader, samples_path, f"{candidate_label}:attempt-{attempt}:poll")
            running_blocks = reader.database_path.exists() and Database(reader.database_path).running_block_processes(args.campaign_id)
            if control and not control_used and running_blocks:
                time.sleep(0.25)
                if control == "pause":
                    result = pause_campaign(args.data_dir, args.campaign_id)
                else:
                    result = stop_campaign(args.data_dir, args.campaign_id)
                control_used = True
                write_jsonl(
                    controls_path,
                    {
                        "candidate": candidate_label,
                        "action": control,
                        "attempt": attempt,
                        "status_after_action": result["status"],
                        "at": time.time(),
                    },
                )
                if control == "pause":
                    # The scheduler deliberately remains alive in paused state
                    # so that resume exercises the same checkpoint and process
                    # ownership path. Do not wait for it to exit while paused.
                    time.sleep(1.5)
                    dashboard_sample(reader, samples_path, f"{candidate_label}:paused")
                    resume_result = resume_campaign(args.data_dir, args.campaign_id)
                    write_jsonl(
                        controls_path,
                        {
                            "candidate": candidate_label,
                            "action": "resume",
                            "attempt": attempt,
                            "status_after_action": resume_result["status"],
                            "at": time.time(),
                        },
                    )
                    dashboard_sample(reader, samples_path, f"{candidate_label}:resumed")
                else:
                    process.wait(timeout=30)
                    resume_result = resume_campaign(args.data_dir, args.campaign_id)
                    write_jsonl(
                        controls_path,
                        {
                            "candidate": candidate_label,
                            "action": "resume",
                            "attempt": attempt,
                            "status_after_action": resume_result["status"],
                            "at": time.time(),
                        },
                    )
                    dashboard_sample(reader, samples_path, f"{candidate_label}:resumed")
                continue
            if time.monotonic() - started > args.candidate_timeout:
                if control:
                    stop_campaign(args.data_dir, args.campaign_id)
                process.wait(timeout=30)
                raise RuntimeError(f"candidate timed out: {candidate_label}")
            time.sleep(0.35)
        if process.poll() is None:
            continue
        return_code = process.wait()
        dashboard_sample(reader, samples_path, f"{candidate_label}:attempt-{attempt}:done")
        if return_code != 0:
            detail = log_path.read_text(encoding="utf-8", errors="replace")[-4000:]
            raise RuntimeError(f"candidate {candidate_label} failed with {return_code}:\n{detail}")
        if control_used and reader.database_path.exists():
            current = reader.snapshot().get("current_trial") or {}
            if current.get("status") in {"completed", "rejected"}:
                return
            continue
        return


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", type=Path, required=True)
    parser.add_argument("--data-dir", type=Path, required=True)
    parser.add_argument("--campaign-id", required=True)
    parser.add_argument("--registry", type=Path, required=True)
    parser.add_argument("--testmonitor", type=Path, required=True)
    parser.add_argument("--fastchess", type=Path, required=True)
    parser.add_argument("--engine", type=Path, required=True)
    parser.add_argument("--opening-book", type=Path, required=True)
    parser.add_argument("--seed", type=int, default=20260813)
    parser.add_argument("--tc", default="10+0.1")
    parser.add_argument("--hash-mb", type=int, default=128)
    parser.add_argument("--threads", type=int, default=1)
    parser.add_argument("--max-blocks", type=int, default=4)
    parser.add_argument("--weak-upper-score", type=float, default=40.0)
    parser.add_argument("--candidate-timeout", type=float, default=1800.0)
    args = parser.parse_args()
    for path in (args.repo, args.registry, args.testmonitor, args.fastchess, args.engine, args.opening_book):
        if not path.exists():
            raise SystemExit(f"missing pilot input: {path}")
    args.repo = args.repo.resolve()
    args.data_dir = args.data_dir.resolve()
    args.registry = args.registry.resolve()
    args.testmonitor = args.testmonitor.resolve()
    args.fastchess = args.fastchess.resolve()
    args.engine = args.engine.resolve()
    args.opening_book = args.opening_book.resolve()

    campaign_root = args.data_dir / args.campaign_id
    campaign_root.mkdir(parents=True, exist_ok=True)
    args.baseline_file = campaign_root / "baseline-parameters.json"
    campaign_file = campaign_root / "campaign.json"
    definition = {
        "schema_version": 1,
        "campaign_id": args.campaign_id,
        "name": "GoAlaric eval pilot v1 full adaptive verification",
        "mode": "real",
        "registry": str(args.registry),
        "baseline": {"engine_id": str(args.engine)},
        "master_seed": args.seed,
        "partitions": {"pilot": {"name": "eval-pilot-v1", "opening_seed": args.seed}},
        "goals": {
            "parameters": 8,
            "candidate_neighborhood": "legal +/- one-step",
            "max_blocks_per_candidate": args.max_blocks,
            "adaptive_weak_upper_score": args.weak_upper_score,
            "auto_promotion": False,
        },
    }
    atomic_write_json(campaign_file, definition)
    loaded = load_campaign_definition(campaign_file)
    _, created, _ = init_campaign(campaign_file, args.data_dir)
    if not created:
        existing = Database(campaign_root / "campaign.db").campaign(args.campaign_id)
        if existing["config_hash"] != loaded.config_hash:
            raise SystemExit(f"existing campaign configuration differs: {args.campaign_id}")
    if not args.baseline_file.exists():
        raise SystemExit("baseline parameter artifact was not created")

    candidates_dir = campaign_root / "candidates"
    logs_dir = campaign_root / "logs"
    candidates_dir.mkdir(exist_ok=True)
    logs_dir.mkdir(exist_ok=True)
    samples_path = campaign_root / "dashboard-samples.jsonl"
    controls_path = campaign_root / "control-events.jsonl"
    reader = DashboardReader(args.data_dir, args.campaign_id)
    atomic_write_json(campaign_root / "pilot-inputs.json", {
        "campaign_definition": loaded.config,
        "registry": str(args.registry),
        "engine": str(args.engine),
        "engine_sha256": __import__("hashlib").sha256(args.engine.read_bytes()).hexdigest(),
        "testmonitor": str(args.testmonitor),
        "fastchess": str(args.fastchess),
        "opening_book": str(args.opening_book),
        "opening_book_sha256": __import__("hashlib").sha256(args.opening_book.read_bytes()).hexdigest(),
        "tc": args.tc,
        "hash_mb": args.hash_mb,
        "threads": args.threads,
        "candidate_count": 0,
        "auto_promotion": False,
    })

    candidates = candidate_documents(args.registry)
    atomic_write_json(campaign_root / "pilot-inputs.json", {
        **json.loads((campaign_root / "pilot-inputs.json").read_text(encoding="utf-8")),
        "candidate_count": len(candidates),
        "candidate_labels": [label for label, _ in candidates],
    })
    for label, document in candidates:
        atomic_write_json(candidates_dir / f"{label}.json", document)

    print(json.dumps({"campaign_id": args.campaign_id, "candidate_count": len(candidates), "status": "started"}), flush=True)
    for index, (label, _) in enumerate(candidates):
        print(json.dumps({"candidate": label, "index": index + 1, "total": len(candidates)}), flush=True)
        run_candidate(
            args,
            reader,
            candidates_dir / f"{label}.json",
            label,
            index,
            logs_dir,
            samples_path,
            controls_path,
        )

    database = Database(campaign_root / "campaign.db")
    if database.running_block_processes(args.campaign_id):
        raise SystemExit("running block processes remain after pilot")
    if process_paths((args.engine, args.fastchess)):
        raise SystemExit("engine or fastchess process remains after pilot")
    with database._read() as connection:
        trials = connection.execute("SELECT trial_id,status,result_json FROM trials WHERE campaign_id=?", (args.campaign_id,)).fetchall()
        blocks = connection.execute("SELECT block_id,status,wins,draws,losses,attempt FROM match_blocks WHERE campaign_id=?", (args.campaign_id,)).fetchall()
        games = connection.execute("SELECT game_id,block_id,game_index FROM games WHERE campaign_id=?", (args.campaign_id,)).fetchall()
        completed_events = connection.execute("SELECT COUNT(*) FROM events WHERE campaign_id=? AND event_type='match_block_completed'", (args.campaign_id,)).fetchone()[0]
    if any(row["status"] not in {"completed", "rejected"} for row in trials):
        raise SystemExit("pilot has non-terminal trials")
    if any(row["status"] not in {"completed", "rejected"} for row in blocks):
        raise SystemExit("pilot has non-terminal blocks")
    completed_blocks = [row for row in blocks if row["status"] == "completed"]
    expected_games = sum(row["wins"] + row["draws"] + row["losses"] for row in completed_blocks)
    if len(games) != expected_games or len({row["game_id"] for row in games}) != len(games):
        raise SystemExit("game accounting is inconsistent or double-counted")
    if int(completed_events) != len(completed_blocks):
        raise SystemExit("completed block event count is inconsistent")
    checkpoint = database.optimizer_state(args.campaign_id)
    if int(checkpoint["revision"]) != len(completed_blocks):
        raise SystemExit("checkpoint revision does not match completed blocks")
    database.transition_campaign(args.campaign_id, "completed", "full pilot verification finished")
    snapshot, json_content = final_report(args.data_dir, args.campaign_id, "json")
    _, html_content = final_report(args.data_dir, args.campaign_id, "html")
    (campaign_root / "final-report.json").write_text(json_content, encoding="utf-8")
    (campaign_root / "final-report.html").write_text(html_content, encoding="utf-8")
    best = snapshot["best_parameters"]
    if best["values"]:
        recommended = {
            "schema_version": 1,
            "registry": snapshot["campaign"]["registry_name"],
            "parameters": [{"name": name, "value": value} for name, value in best["values"].items()],
        }
        atomic_write_json(campaign_root / "recommended-parameters.json", recommended)
        atomic_write_json(campaign_root / "recommendation.json", {
            "status": "manual_review_only",
            "auto_promotion": False,
            "source_trial_id": best["trial_id"],
            "source_parameter_hash": best["parameter_hash"],
            "metrics": best["metrics"],
            "parameter_file": str(campaign_root / "recommended-parameters.json"),
        })
    atomic_write_json(campaign_root / "verification.json", {
        "campaign_id": args.campaign_id,
        "status": snapshot["campaign"]["status"],
        "finished": snapshot["campaign"]["finished"],
        "candidate_count": len(candidates),
        "trials": len(trials),
        "blocks": len(blocks),
        "completed_blocks": len(completed_blocks),
        "rejected_blocks": sum(row["status"] == "rejected" for row in blocks),
        "games": len(games),
        "completed_block_events": int(completed_events),
        "checkpoint": checkpoint,
        "running_processes": process_paths((args.engine, args.fastchess)),
        "best_parameters": best,
        "auto_promotion": False,
    })
    print(json.dumps({"campaign_id": args.campaign_id, "status": "completed", "games": len(games), "best": best}, ensure_ascii=False), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
