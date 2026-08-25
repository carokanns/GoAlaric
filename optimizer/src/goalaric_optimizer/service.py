from __future__ import annotations

import os
import secrets
import fcntl
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Iterator

from .canonical import atomic_write_json, sha256_bytes
from .config import CampaignDefinition, baseline_parameter_bytes, load_campaign_definition
from .database import (
    CampaignBusy,
    Database,
    DatabaseError,
    InvalidTransition,
)


class ServiceError(RuntimeError):
    pass


def default_data_dir() -> Path:
    cwd = Path.cwd()
    if (cwd / "optimizer").is_dir():
        return (cwd / "optimizer" / "campaigns").resolve()
    return (cwd / "campaigns").resolve()


def campaign_dir(data_dir: Path, campaign_id: str) -> Path:
    return data_dir.resolve() / campaign_id


def database_for(data_dir: Path, campaign_id: str) -> Database:
    return Database(campaign_dir(data_dir, campaign_id) / "campaign.db")


def load_database(data_dir: Path, campaign_id: str) -> Database:
    database = database_for(data_dir, campaign_id)
    if not database.path.exists():
        raise ServiceError(f"campaign is not initialized: {campaign_id}")
    return database


@contextmanager
def campaign_lock(data_dir: Path, campaign_id: str) -> Iterator[None]:
    """Hold an OS lock for one control operation.

    SQLite remains the source of truth. The lock only prevents two live CLI
    processes from issuing conflicting control operations at once, and the OS
    releases it if a process dies.
    """
    directory = campaign_dir(data_dir, campaign_id)
    directory.mkdir(parents=True, exist_ok=True)
    lock_path = directory / "campaign.lock"
    with lock_path.open("a+", encoding="utf-8") as stream:
        try:
            fcntl.flock(stream.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            raise ServiceError(f"campaign {campaign_id} is busy") from exc
        try:
            yield
        finally:
            fcntl.flock(stream.fileno(), fcntl.LOCK_UN)


def init_campaign(campaign_path: Path, data_dir: Path) -> tuple[CampaignDefinition, bool, Path]:
    definition = load_campaign_definition(campaign_path)
    directory = campaign_dir(data_dir, definition.campaign_id)
    baseline_path = directory / "baseline-parameters.json"
    if baseline_path.exists():
        existing = baseline_path.read_bytes()
        expected = baseline_parameter_bytes(definition)
        if existing != expected:
            raise ServiceError("existing baseline parameter artifact differs from campaign configuration")
    else:
        from .canonical import atomic_write_bytes

        atomic_write_bytes(baseline_path, baseline_parameter_bytes(definition))
    database = Database(directory / "campaign.db")
    created = database.initialize_campaign(definition, baseline_path)
    return definition, created, database.path


def _owner_token() -> str:
    return f"optimizer-{os.getpid()}-{secrets.token_hex(8)}"


def run_campaign(data_dir: Path, campaign_id: str, fake: bool = False) -> dict[str, Any]:
    with campaign_lock(data_dir, campaign_id):
        database = load_database(data_dir, campaign_id)
        token = _owner_token()
        try:
            database.claim_campaign(campaign_id, token, takeover=True)
            database.recover_abandoned_jobs(campaign_id)
            campaign = database.campaign(campaign_id)
            if campaign["status"] == "pending":
                database.transition_campaign(campaign_id, "running", "run command")
            elif campaign["status"] == "paused":
                database.transition_campaign(campaign_id, "running", "run command")
            elif campaign["status"] == "interrupted":
                database.transition_campaign(campaign_id, "running", "run command after recovery")
            elif campaign["status"] in {"completed", "failed", "rejected"}:
                raise ServiceError(f"campaign is already terminal: {campaign['status']}")
            elif campaign["status"] != "running":
                raise ServiceError(f"campaign cannot be run from state {campaign['status']}")
            if not fake:
                raise ServiceError("real campaign execution is deferred until scheduler/match phases")
            return database.status_snapshot(campaign_id)
        finally:
            try:
                database.release_campaign(campaign_id, token)
            except (DatabaseError, CampaignBusy):
                pass


def pause_campaign(data_dir: Path, campaign_id: str) -> dict[str, Any]:
    # Pause and stop are asynchronous control operations. They must remain
    # available while a long-running optimize invocation owns campaign.lock.
    # SQLite transactions serialize their state changes, and terminating a
    # recorded process group is idempotent.
    database = load_database(data_dir, campaign_id)
    from .scheduler import terminate_active_blocks

    campaign = database.campaign(campaign_id)
    if campaign["status"] not in {"paused", "completed", "failed", "rejected", "interrupted"}:
        database.transition_campaign(campaign_id, "paused", "pause command")
    terminate_active_blocks(data_dir, campaign_id, "pause command")
    return database.campaign(campaign_id)


def resume_campaign(data_dir: Path, campaign_id: str) -> dict[str, Any]:
    with campaign_lock(data_dir, campaign_id):
        database = load_database(data_dir, campaign_id)
        from .scheduler import terminate_active_blocks

        terminate_active_blocks(data_dir, campaign_id, "resume command cleanup")
        return database.transition_campaign(campaign_id, "running", "resume command")


def stop_campaign(data_dir: Path, campaign_id: str) -> dict[str, Any]:
    database = load_database(data_dir, campaign_id)
    from .scheduler import terminate_active_blocks

    campaign = database.campaign(campaign_id)
    if campaign["status"] not in {"completed", "failed", "rejected", "interrupted"}:
        database.transition_campaign(campaign_id, "interrupted", "stop command")
    terminate_active_blocks(data_dir, campaign_id, "stop command")
    return database.campaign(campaign_id)
