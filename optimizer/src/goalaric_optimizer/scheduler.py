from __future__ import annotations

import json
import os
import secrets
import signal
import subprocess
import time
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Iterator, Sequence

import fcntl

from .database import CampaignBusy, Database, DatabaseError, InvalidTransition
from .process import ProcessGroupError, terminate_process, terminate_process_group
from .service import campaign_dir, load_database


class SchedulerError(RuntimeError):
    pass


@contextmanager
def scheduler_lock(data_dir: Path, campaign_id: str) -> Iterator[None]:
    """Keep control commands usable while allowing only one scheduler."""
    directory = campaign_dir(data_dir, campaign_id)
    directory.mkdir(parents=True, exist_ok=True)
    path = directory / "scheduler.lock"
    with path.open("a+", encoding="utf-8") as stream:
        try:
            fcntl.flock(stream.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            raise SchedulerError(f"scheduler for campaign {campaign_id} is already running") from exc
        try:
            yield
        finally:
            fcntl.flock(stream.fileno(), fcntl.LOCK_UN)


def _owner_token() -> str:
    return f"scheduler-{os.getpid()}-{secrets.token_hex(8)}"


class Scheduler:
    """Run one deterministic match block at a time.

    The command is intentionally an external fake-monitor command in phase 6.
    It receives a result path and block identity, and must write a JSON result
    only after the complete block has finished.
    """

    def __init__(
        self,
        data_dir: Path,
        campaign_id: str,
        monitor_command: Sequence[str],
        poll_interval: float = 0.05,
        stop_grace_seconds: float = 0.5,
        workdir: Path | None = None,
        preserve_optimizer_state: bool = False,
        embedded_campaign: bool = False,
    ) -> None:
        if not monitor_command:
            raise SchedulerError("monitor command cannot be empty")
        if poll_interval <= 0:
            raise SchedulerError("scheduler poll interval must be positive")
        self.data_dir = data_dir.resolve()
        self.campaign_id = campaign_id
        self.monitor_command = tuple(str(item) for item in monitor_command)
        self.poll_interval = poll_interval
        self.stop_grace_seconds = stop_grace_seconds
        self.workdir = workdir.resolve() if workdir is not None else None
        self.preserve_optimizer_state = preserve_optimizer_state
        self.embedded_campaign = embedded_campaign
        self._process: subprocess.Popen[str] | None = None
        self._process_group_id: int | None = None

    def run(
        self,
        max_completed_blocks: int = 0,
        finish_work: bool = True,
        expected_block_id: str | None = None,
    ) -> dict[str, Any]:
        if max_completed_blocks < 0:
            raise SchedulerError("max_completed_blocks cannot be negative")
        if expected_block_id is not None and max_completed_blocks != 1:
            raise SchedulerError("an expected block requires max_completed_blocks=1")
        with scheduler_lock(self.data_dir, self.campaign_id):
            database = load_database(self.data_dir, self.campaign_id)
            owner: str | None = None
            completed_blocks = 0
            if self.embedded_campaign:
                campaign = database.campaign(self.campaign_id)
                if campaign["status"] != "running" or not campaign["owner_token"]:
                    raise SchedulerError("embedded scheduler requires a running, optimizer-owned campaign")
            else:
                owner = _owner_token()
                database.recover_abandoned_jobs(self.campaign_id, "scheduler startup recovered abandoned job")
                database.claim_campaign(self.campaign_id, owner, takeover=True)
            try:
                if not self.embedded_campaign:
                    self._enter_running_state(database)
                while True:
                    campaign = database.campaign(self.campaign_id)
                    status = str(campaign["status"])
                    if status == "paused":
                        time.sleep(self.poll_interval)
                        continue
                    if status in {"completed", "failed", "rejected", "interrupted"}:
                        return database.status_snapshot(self.campaign_id)
                    if status != "running":
                        raise SchedulerError(f"scheduler cannot run campaign from state {status}")

                    block = database.claim_next_block(self.campaign_id, expected_block_id)
                    if block is None:
                        if expected_block_id is not None:
                            raise SchedulerError(f"expected block is not runnable: {expected_block_id}")
                        if finish_work:
                            finished = database.finish_completed_work(self.campaign_id)
                            if finished["campaign_completed"]:
                                return database.status_snapshot(self.campaign_id)
                        raise SchedulerError("running campaign has no pending or completed match blocks")

                    self._ensure_trial_running(database, str(block["trial_id"]))
                    outcome = self._run_block(database, block)
                    if outcome == "completed":
                        completed_blocks += 1
                        if finish_work:
                            database.finish_completed_work(self.campaign_id)
                        if max_completed_blocks and completed_blocks >= max_completed_blocks:
                            return database.status_snapshot(self.campaign_id)
                        continue
                    current = database.campaign(self.campaign_id)
                    if outcome == "control" or current["status"] in {"paused", "interrupted"}:
                        continue
                    # A dead or malformed monitor is made restartable but does
                    # not loop forever and silently create repeated attempts.
                    return database.status_snapshot(self.campaign_id)
            finally:
                self._terminate_current_process()
                if owner is not None:
                    try:
                        database.release_campaign(self.campaign_id, owner)
                    except (DatabaseError, CampaignBusy):
                        pass

    def _enter_running_state(self, database: Database) -> None:
        campaign = database.campaign(self.campaign_id)
        status = str(campaign["status"])
        if status in {"pending", "paused", "interrupted"}:
            database.transition_campaign(self.campaign_id, "running", "scheduler start/resume")
        elif status in {"completed", "failed", "rejected"}:
            raise SchedulerError(f"campaign is already terminal: {status}")
        elif status != "running":
            raise SchedulerError(f"campaign cannot be scheduled from state {status}")

    def _ensure_trial_running(self, database: Database, trial_id: str) -> None:
        trials = database.list_trials(self.campaign_id, 10000)
        trial = next((item for item in trials if item["trial_id"] == trial_id), None)
        if trial is None:
            raise SchedulerError(f"unknown trial for block: {trial_id}")
        if trial["status"] in {"pending", "interrupted"}:
            database.transition_trial(self.campaign_id, trial_id, "running", result={"scheduler": "phase6"})
        elif trial["status"] != "running":
            raise SchedulerError(f"trial {trial_id} cannot run from state {trial['status']}")

    def _command(self, block: dict[str, Any], run_dir: Path, result_path: Path) -> list[str]:
        return [
            *self.monitor_command,
            "--campaign-id",
            self.campaign_id,
            "--block-id",
            str(block["block_id"]),
            "--run-dir",
            str(run_dir),
            "--result-path",
            str(result_path),
            "--pairs-per-block",
            str(block["pairs_per_block"]),
            "--attempt",
            str(block["attempt"]),
        ]

    def _run_block(self, database: Database, block: dict[str, Any]) -> str:
        run_dir = campaign_dir(self.data_dir, self.campaign_id) / "runs" / (
            f"{block['block_id']}-attempt-{block['attempt']:04d}"
        )
        run_dir.mkdir(parents=True, exist_ok=True)
        result_path = run_dir / "result.json"
        log_path = run_dir / "monitor.log"
        command = self._command(block, run_dir, result_path)
        process: subprocess.Popen[str] | None = None
        process_group_id: int | None = None
        try:
            log = log_path.open("w", encoding="utf-8")
            try:
                process = subprocess.Popen(
                    command,
                    stdin=subprocess.DEVNULL,
                    stdout=log,
                    stderr=subprocess.STDOUT,
                    text=True,
                    start_new_session=True,
                    cwd=self.workdir,
                )
            finally:
                log.close()
            process_group_id = os.getpgid(process.pid)
            self._process = process
            self._process_group_id = process_group_id
            database.set_block_process(
                self.campaign_id,
                str(block["block_id"]),
                process.pid,
                process_group_id,
                str(run_dir),
                command,
            )

            while process.poll() is None:
                self._poll_progress(database, block, run_dir)
                if database.campaign(self.campaign_id)["status"] != "running":
                    terminate_process(process, self.stop_grace_seconds)
                    database.interrupt_block(
                        self.campaign_id,
                        str(block["block_id"]),
                        "campaign control requested before block completion",
                    )
                    return "control"
                time.sleep(self.poll_interval)
            return_code = process.returncode
            # A well-behaved monitor waits for Fastchess, but terminate any
            # accidental daemonized child before the block can be committed.
            terminate_process_group(process_group_id, self.stop_grace_seconds)
            if return_code != 0:
                if database.campaign(self.campaign_id)["status"] != "running":
                    database.interrupt_block(
                        self.campaign_id,
                        str(block["block_id"]),
                        "campaign control interrupted monitor",
                    )
                    return "control"
                database.interrupt_block(
                    self.campaign_id,
                    str(block["block_id"]),
                    f"monitor exited with status {return_code}",
                )
                return "dead"
            if database.campaign(self.campaign_id)["status"] != "running":
                database.interrupt_block(
                    self.campaign_id,
                    str(block["block_id"]),
                    "campaign control requested before result commit",
                )
                return "control"
            result = self._read_result(result_path, int(block["pairs_per_block"]))
            if self.preserve_optimizer_state:
                checkpoint_state = database.optimizer_state(self.campaign_id)["state"]
            else:
                checkpoint_state = {
                    "next_block": int(block["block_index"]) + 1,
                    "last_block": block["block_id"],
                    "attempt": block["attempt"],
                }
            try:
                database.complete_block_atomically(
                    self.campaign_id,
                    str(block["block_id"]),
                    int(result["wins"]),
                    int(result["draws"]),
                    int(result["losses"]),
                    float(result["score"]),
                    result,
                    checkpoint_state,
                )
            except InvalidTransition:
                database.interrupt_block(
                    self.campaign_id,
                    str(block["block_id"]),
                    "block was no longer running at result commit",
                )
                return "dead"
            return "completed"
        except (OSError, subprocess.SubprocessError, DatabaseError, ProcessGroupError, ValueError, SchedulerError) as exc:
            if process is not None and process.poll() is None:
                terminate_process(process, self.stop_grace_seconds)
            database.interrupt_block(self.campaign_id, str(block["block_id"]), f"scheduler error: {exc}")
            return "dead"
        finally:
            self._process = None
            self._process_group_id = None
            if process is not None:
                try:
                    process.wait(timeout=0.1)
                except subprocess.TimeoutExpired:
                    terminate_process(process, self.stop_grace_seconds)

    def _poll_progress(self, database: Database, block: dict[str, Any], run_dir: Path) -> None:
        """Allow monitor-specific schedulers to checkpoint non-final progress."""

    @staticmethod
    def _read_result(path: Path, pairs_per_block: int) -> dict[str, Any]:
        if not path.exists():
            raise SchedulerError(f"monitor did not produce a result: {path}")
        value = json.loads(path.read_text(encoding="utf-8"))
        if not isinstance(value, dict):
            raise SchedulerError("monitor result must be a JSON object")
        counts = [value.get(name) for name in ("wins", "draws", "losses")]
        if any(not isinstance(item, int) or isinstance(item, bool) or item < 0 for item in counts):
            raise SchedulerError("monitor result W-D-L must be non-negative integers")
        expected_games = pairs_per_block * 2
        if sum(counts) != expected_games:
            raise SchedulerError(f"monitor result contains {sum(counts)} games; expected {expected_games}")
        score = value.get("score")
        if not isinstance(score, (int, float)) or isinstance(score, bool) or not 0 <= float(score) <= 100:
            raise SchedulerError("monitor result score must be a percentage from 0 to 100")
        games = value.get("games")
        if games is not None and (not isinstance(games, list) or len(games) != expected_games):
            raise SchedulerError("monitor result games must contain every completed game")
        return value

    def _terminate_current_process(self) -> None:
        process = self._process
        group_id = self._process_group_id
        if process is not None:
            terminate_process(process, self.stop_grace_seconds)
        elif group_id is not None:
            terminate_process_group(group_id, self.stop_grace_seconds)


def run_fake_scheduler(
    data_dir: Path,
    campaign_id: str,
    monitor_command: Sequence[str],
    block_count: int = 1,
    pairs_per_block: int = 1,
    poll_interval: float = 0.05,
) -> dict[str, Any]:
    database = load_database(data_dir, campaign_id)
    database.ensure_fake_schedule(campaign_id, block_count, pairs_per_block)
    return Scheduler(data_dir, campaign_id, monitor_command, poll_interval=poll_interval).run()


def terminate_active_blocks(data_dir: Path, campaign_id: str, reason: str) -> dict[str, int]:
    """Stop all recorded child groups before control status is changed."""
    database = load_database(data_dir, campaign_id)
    running = database.running_block_processes(campaign_id)
    for block in running:
        process_group_id = block.get("process_group_id") or block.get("pid")
        terminate_process_group(process_group_id, grace_seconds=0.5)
    return database.recover_abandoned_jobs(campaign_id, reason)
