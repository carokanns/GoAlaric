from __future__ import annotations

import json
import sqlite3
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Iterator

from .canonical import canonical_json, sha256_json, utc_now
from .config import CampaignDefinition
from .profiles import profile_identity, profiles_equivalent
from .statistics import aggregate_wdl


SCHEMA_VERSION = 5
CAMPAIGN_STATES = {"pending", "running", "completed", "failed", "interrupted", "paused", "rejected"}
TRIAL_STATES = CAMPAIGN_STATES
BLOCK_STATES = {"pending", "running", "completed", "failed", "interrupted", "rejected"}

CAMPAIGN_TRANSITIONS = {
    "pending": {"running", "paused", "interrupted", "failed", "rejected", "completed"},
    "running": {"paused", "interrupted", "failed", "rejected", "completed"},
    "paused": {"running", "interrupted", "failed", "rejected", "completed"},
    "interrupted": {"running", "failed", "rejected", "completed"},
    "failed": set(),
    "rejected": set(),
    "completed": set(),
}

TRIAL_TRANSITIONS = {
    "pending": {"running", "interrupted", "failed", "rejected", "completed"},
    "running": {"completed", "interrupted", "failed", "rejected"},
    "interrupted": {"running", "failed", "rejected", "completed"},
    "failed": set(),
    "rejected": set(),
    "completed": set(),
}

BLOCK_TRANSITIONS = {
    "pending": {"running", "interrupted", "failed", "rejected"},
    "running": {"completed", "interrupted", "failed", "rejected"},
    "interrupted": {"running", "failed", "rejected"},
    "failed": set(),
    "rejected": set(),
    "completed": set(),
}


class DatabaseError(RuntimeError):
    pass


class CampaignConflict(DatabaseError):
    pass


class InvalidTransition(DatabaseError):
    pass


class CampaignBusy(DatabaseError):
    pass


SCHEMA = """
CREATE TABLE IF NOT EXISTS schema_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS campaigns (
    campaign_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','running','completed','failed','interrupted','paused','rejected')),
    config_hash TEXT NOT NULL,
    config_json TEXT NOT NULL,
    baseline_engine_id TEXT NOT NULL,
    baseline_parameter_hash TEXT NOT NULL,
    registry_name TEXT NOT NULL,
    registry_version INTEGER NOT NULL,
    master_seed INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    finished_at TEXT,
    owner_token TEXT,
    owner_acquired_at TEXT,
    revision INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS parameter_sets (
    parameter_set_id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaigns(campaign_id),
    parameter_hash TEXT NOT NULL,
    document_json TEXT NOT NULL,
    group_name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (campaign_id, parameter_hash)
);

CREATE TABLE IF NOT EXISTS trials (
    trial_id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaigns(campaign_id),
    parameter_set_id TEXT NOT NULL REFERENCES parameter_sets(parameter_set_id),
    status TEXT NOT NULL CHECK (status IN ('pending','running','completed','failed','interrupted','paused','rejected')),
    algorithm TEXT NOT NULL,
    seed INTEGER NOT NULL,
    profile_name TEXT,
    profile_hash TEXT,
    profile_tc TEXT,
    profile_mode TEXT,
    profile_nodes INTEGER,
    result_json TEXT,
    error TEXT,
    pid INTEGER,
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE (campaign_id, parameter_set_id)
);

CREATE TABLE IF NOT EXISTS match_blocks (
    block_id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaigns(campaign_id),
    trial_id TEXT NOT NULL REFERENCES trials(trial_id),
    partition_name TEXT NOT NULL,
    block_index INTEGER NOT NULL,
    pairs_per_block INTEGER NOT NULL CHECK (pairs_per_block > 0),
    master_seed INTEGER NOT NULL,
    opening_book_sha256 TEXT NOT NULL,
    materialized_openings_sha256 TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','running','completed','failed','interrupted','rejected')),
    pid INTEGER,
    process_group_id INTEGER,
    attempt INTEGER NOT NULL DEFAULT 0,
    run_dir TEXT,
    command_json TEXT,
    wins INTEGER NOT NULL DEFAULT 0,
    draws INTEGER NOT NULL DEFAULT 0,
    losses INTEGER NOT NULL DEFAULT 0,
    score REAL,
    result_json TEXT,
    error TEXT,
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE (campaign_id, trial_id, partition_name, block_index, pairs_per_block, master_seed, opening_book_sha256, materialized_openings_sha256)
);

CREATE TABLE IF NOT EXISTS games (
    game_id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaigns(campaign_id),
    block_id TEXT NOT NULL REFERENCES match_blocks(block_id),
    game_index INTEGER NOT NULL,
    result TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (block_id, game_index)
);

CREATE TABLE IF NOT EXISTS optimizer_state (
    campaign_id TEXT PRIMARY KEY REFERENCES campaigns(campaign_id),
    revision INTEGER NOT NULL,
    state_json TEXT NOT NULL,
    checkpoint_hash TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS artifacts (
    artifact_id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaigns(campaign_id),
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    sha256 TEXT,
    created_at TEXT NOT NULL,
    UNIQUE (campaign_id, kind, path)
);

CREATE TABLE IF NOT EXISTS events (
    event_id INTEGER PRIMARY KEY AUTOINCREMENT,
    campaign_id TEXT NOT NULL REFERENCES campaigns(campaign_id),
    event_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    from_status TEXT,
    to_status TEXT,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS events_campaign_idx ON events(campaign_id, event_id);
CREATE INDEX IF NOT EXISTS trials_campaign_status_idx ON trials(campaign_id, status);
CREATE INDEX IF NOT EXISTS blocks_campaign_status_idx ON match_blocks(campaign_id, status);

-- Confirmation is deliberately isolated from optimizer trials, blocks and
-- checkpoints.  It is evidence for the final recommendation, never search
-- feedback.
CREATE TABLE IF NOT EXISTS confirmations (
    confirmation_id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaigns(campaign_id),
    status TEXT NOT NULL CHECK (status IN ('pending','running','completed','failed','interrupted')),
    candidate_parameter_hash TEXT NOT NULL,
    baseline_parameter_hash TEXT NOT NULL,
    candidate_document_json TEXT NOT NULL,
    baseline_document_json TEXT NOT NULL,
    games_target INTEGER NOT NULL CHECK (games_target > 0),
    seed INTEGER NOT NULL,
    confidence REAL NOT NULL,
    profile_name TEXT,
    profile_hash TEXT,
    profile_tc TEXT,
    profile_mode TEXT,
    profile_nodes INTEGER,
    outcome TEXT CHECK (outcome IN ('confirmed','rejected','inconclusive')),
    wins INTEGER NOT NULL DEFAULT 0,
    draws INTEGER NOT NULL DEFAULT 0,
    losses INTEGER NOT NULL DEFAULT 0,
    score REAL,
    score_ci_low REAL,
    score_ci_high REAL,
    recommendation_parameter_hash TEXT,
    result_json TEXT,
    error TEXT,
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE (campaign_id)
);

CREATE TABLE IF NOT EXISTS confirmation_blocks (
    block_id TEXT PRIMARY KEY,
    confirmation_id TEXT NOT NULL REFERENCES confirmations(confirmation_id),
    campaign_id TEXT NOT NULL REFERENCES campaigns(campaign_id),
    block_index INTEGER NOT NULL,
    pairs_per_block INTEGER NOT NULL CHECK (pairs_per_block > 0),
    master_seed INTEGER NOT NULL,
    opening_book_sha256 TEXT NOT NULL,
    materialized_openings_sha256 TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','running','completed','failed','interrupted','rejected')),
    pid INTEGER,
    process_group_id INTEGER,
    attempt INTEGER NOT NULL DEFAULT 0,
    run_dir TEXT,
    command_json TEXT,
    wins INTEGER NOT NULL DEFAULT 0,
    draws INTEGER NOT NULL DEFAULT 0,
    losses INTEGER NOT NULL DEFAULT 0,
    score REAL,
    result_json TEXT,
    error TEXT,
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE (confirmation_id, block_index, pairs_per_block, master_seed,
            opening_book_sha256, materialized_openings_sha256)
);

CREATE TABLE IF NOT EXISTS confirmation_games (
    game_id TEXT PRIMARY KEY,
    confirmation_id TEXT NOT NULL REFERENCES confirmations(confirmation_id),
    block_id TEXT NOT NULL REFERENCES confirmation_blocks(block_id),
    game_index INTEGER NOT NULL,
    result TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (block_id, game_index)
);

CREATE INDEX IF NOT EXISTS confirmations_campaign_idx ON confirmations(campaign_id);
CREATE INDEX IF NOT EXISTS confirmation_blocks_status_idx ON confirmation_blocks(confirmation_id, status);
"""


def _row(row: sqlite3.Row | None) -> dict[str, Any] | None:
    return dict(row) if row is not None else None


def _json(value: Any) -> str:
    return canonical_json(value)


def _confirmation_metrics(
    confirmation: dict[str, Any], blocks: list[dict[str, Any]]
) -> dict[str, Any]:
    """Derive live confirmation evidence exclusively from completed SQLite blocks."""
    metrics = aggregate_wdl(blocks, confidence=float(confirmation["confidence"]))
    completed = [block for block in blocks if block.get("status") == "completed"]
    pairs_completed = sum(int(block["pairs_per_block"]) for block in completed)
    pairs_target = int(confirmation["games_target"]) // 2
    metrics.update(
        {
            "pairs_completed": pairs_completed,
            "pairs_target": pairs_target,
            "games_completed": int(metrics["games"]),
            "games_target": int(confirmation["games_target"]),
        }
    )
    return metrics


class Database:
    def __init__(self, path: Path):
        self.path = Path(path).resolve()

    def _connect(self, readonly: bool = False) -> sqlite3.Connection:
        if readonly:
            if not self.path.exists():
                raise DatabaseError(f"database does not exist: {self.path}")
            uri = f"file:{self.path}?mode=ro"
            connection = sqlite3.connect(uri, uri=True, timeout=5)
        else:
            self.path.parent.mkdir(parents=True, exist_ok=True)
            connection = sqlite3.connect(self.path, timeout=5, isolation_level=None)
            journal_mode = connection.execute("PRAGMA journal_mode=WAL").fetchone()[0]
            if str(journal_mode).lower() != "wal":
                connection.close()
                raise DatabaseError(f"SQLite WAL mode could not be enabled: {journal_mode}")
            connection.execute("PRAGMA synchronous=NORMAL")
        connection.row_factory = sqlite3.Row
        connection.execute("PRAGMA foreign_keys=ON")
        connection.execute("PRAGMA busy_timeout=5000")
        if readonly:
            connection.execute("PRAGMA query_only=ON")
        return connection

    @contextmanager
    def _transaction(self) -> Iterator[sqlite3.Connection]:
        connection = self._connect()
        try:
            connection.execute("BEGIN IMMEDIATE")
            yield connection
            connection.commit()
        except Exception:
            connection.rollback()
            raise
        finally:
            connection.close()

    @contextmanager
    def _read(self) -> Iterator[sqlite3.Connection]:
        connection = self._connect(readonly=True)
        try:
            yield connection
        finally:
            connection.close()

    def initialize(self) -> None:
        with self._transaction() as connection:
            connection.executescript(SCHEMA)
            existing = connection.execute(
                "SELECT value FROM schema_meta WHERE key='schema_version'"
            ).fetchone()
            if existing is not None:
                def add_missing_columns(table: str, definitions: tuple[tuple[str, str], ...]) -> None:
                    columns = {
                        row["name"]
                        for row in connection.execute(f"PRAGMA table_info({table})").fetchall()
                    }
                    for column, definition in definitions:
                        if column not in columns:
                            connection.execute(f"ALTER TABLE {table} ADD COLUMN {column} {definition}")

                profile_columns = (
                    ("profile_name", "TEXT"),
                    ("profile_hash", "TEXT"),
                    ("profile_tc", "TEXT"),
                    ("profile_mode", "TEXT"),
                    ("profile_nodes", "INTEGER"),
                )
                version = int(existing["value"])
                if version == 1:
                    add_missing_columns(
                        "match_blocks",
                        (
                            ("process_group_id", "INTEGER"),
                            ("attempt", "INTEGER NOT NULL DEFAULT 0"),
                            ("run_dir", "TEXT"),
                            ("command_json", "TEXT"),
                        ),
                    )
                    add_missing_columns("trials", profile_columns)
                    add_missing_columns("confirmations", profile_columns)
                    connection.execute(
                        "UPDATE schema_meta SET value=? WHERE key='schema_version'", (str(SCHEMA_VERSION),)
                    )
                elif version == 2:
                    add_missing_columns("trials", profile_columns)
                    add_missing_columns("confirmations", profile_columns)
                    connection.execute(
                        "UPDATE schema_meta SET value=? WHERE key='schema_version'", (str(SCHEMA_VERSION),)
                    )
                elif version == 3:
                    for table in ("trials", "confirmations"):
                        add_missing_columns(table, profile_columns)
                    connection.execute(
                        "UPDATE schema_meta SET value=? WHERE key='schema_version'", (str(SCHEMA_VERSION),)
                    )
                elif version == 4:
                    for table in ("trials", "confirmations"):
                        add_missing_columns(
                            table,
                            (("profile_mode", "TEXT"), ("profile_nodes", "INTEGER")),
                        )
                    connection.execute(
                        "UPDATE schema_meta SET value=? WHERE key='schema_version'", (str(SCHEMA_VERSION),)
                    )
                elif version != SCHEMA_VERSION:
                    raise DatabaseError(
                        f"unsupported database schema version: {existing['value']} (expected {SCHEMA_VERSION})"
                    )
            connection.execute(
                "INSERT OR IGNORE INTO schema_meta(key, value) VALUES('schema_version', ?)",
                (str(SCHEMA_VERSION),),
            )

    def journal_mode(self) -> str:
        with self._read() as connection:
            return str(connection.execute("PRAGMA journal_mode").fetchone()[0]).lower()

    def _event(
        self,
        connection: sqlite3.Connection,
        campaign_id: str,
        event_type: str,
        entity_type: str,
        entity_id: str,
        payload: Any,
        from_status: str | None = None,
        to_status: str | None = None,
    ) -> None:
        connection.execute(
            "INSERT INTO events(campaign_id,event_type,entity_type,entity_id,from_status,to_status,payload_json,created_at) "
            "VALUES(?,?,?,?,?,?,?,?)",
            (campaign_id, event_type, entity_type, entity_id, from_status, to_status, _json(payload), utc_now()),
        )

    def _campaign(self, connection: sqlite3.Connection, campaign_id: str) -> sqlite3.Row:
        try:
            row = connection.execute("SELECT * FROM campaigns WHERE campaign_id=?", (campaign_id,)).fetchone()
        except sqlite3.OperationalError as exc:
            # A newly created SQLite file becomes visible before the creator
            # has finished executing the schema script. Callers that poll for
            # the file can safely retry this transient state.
            if "no such table" in str(exc):
                raise DatabaseError("database schema is still initializing") from exc
            raise
        if row is None:
            raise DatabaseError(f"unknown campaign: {campaign_id}")
        return row

    def initialize_campaign(self, definition: CampaignDefinition, baseline_artifact_path: Path) -> bool:
        self.initialize()
        now = utc_now()
        with self._transaction() as connection:
            existing = connection.execute(
                "SELECT config_hash FROM campaigns WHERE campaign_id=?", (definition.campaign_id,)
            ).fetchone()
            if existing is not None:
                if existing["config_hash"] != definition.config_hash:
                    raise CampaignConflict(
                        f"campaign {definition.campaign_id} already exists with a different configuration"
                    )
                return False
            connection.execute(
                "INSERT INTO campaigns(campaign_id,name,status,config_hash,config_json,baseline_engine_id,"
                "baseline_parameter_hash,registry_name,registry_version,master_seed,created_at,updated_at) "
                "VALUES(?,?,?,?,?,?,?,?,?,?,?,?)",
                (
                    definition.campaign_id,
                    definition.name,
                    "pending",
                    definition.config_hash,
                    _json(definition.config),
                    definition.baseline_engine_id,
                    definition.baseline_parameter_hash,
                    definition.registry.name,
                    definition.registry.schema_version,
                    definition.master_seed,
                    now,
                    now,
                ),
            )
            parameter_set_id = self._insert_parameter_set(
                connection,
                definition.campaign_id,
                definition.baseline_parameters,
                "baseline",
                now,
            )
            state = {"version": 1, "next_trial": 1, "next_block": 0, "last_event": "initialized"}
            connection.execute(
                "INSERT INTO optimizer_state(campaign_id,revision,state_json,checkpoint_hash,updated_at) VALUES(?,?,?,?,?)",
                (definition.campaign_id, 0, _json(state), sha256_json(state), now),
            )
            self._event(
                connection,
                definition.campaign_id,
                "campaign_initialized",
                "campaign",
                definition.campaign_id,
                {"config_hash": definition.config_hash, "baseline_parameter_set_id": parameter_set_id},
                to_status="pending",
            )
            self._event(
                connection,
                definition.campaign_id,
                "checkpoint",
                "optimizer_state",
                definition.campaign_id,
                {"revision": 0, "checkpoint_hash": sha256_json(state)},
            )
            self._insert_artifact(
                connection,
                definition.campaign_id,
                "baseline_parameters",
                str(baseline_artifact_path),
                None,
                now,
            )
        return True

    def _insert_artifact(
        self,
        connection: sqlite3.Connection,
        campaign_id: str,
        kind: str,
        path: str,
        digest: str | None,
        now: str,
    ) -> None:
        connection.execute(
            "INSERT INTO artifacts(artifact_id,campaign_id,kind,path,sha256,created_at) VALUES(?,?,?,?,?,?) "
            "ON CONFLICT(campaign_id,kind,path) DO UPDATE SET sha256=excluded.sha256",
            (f"artifact-{sha256_json([campaign_id, kind, path])[:20]}", campaign_id, kind, path, digest, now),
        )

    def _insert_parameter_set(
        self,
        connection: sqlite3.Connection,
        campaign_id: str,
        document: dict[str, Any],
        group_name: str,
        now: str,
    ) -> str:
        parameter_hash = sha256_json(document)
        existing = connection.execute(
            "SELECT parameter_set_id, document_json FROM parameter_sets WHERE campaign_id=? AND parameter_hash=?",
            (campaign_id, parameter_hash),
        ).fetchone()
        if existing is not None:
            if existing["document_json"] != _json(document):
                raise CampaignConflict("parameter hash collision with different canonical content")
            return str(existing["parameter_set_id"])
        parameter_set_id = f"parameter-{parameter_hash[:20]}"
        connection.execute(
            "INSERT INTO parameter_sets(parameter_set_id,campaign_id,parameter_hash,document_json,group_name,created_at) "
            "VALUES(?,?,?,?,?,?)",
            (parameter_set_id, campaign_id, parameter_hash, _json(document), group_name, now),
        )
        return parameter_set_id

    def add_parameter_set(self, campaign_id: str, document: dict[str, Any], group_name: str = "candidate") -> str:
        now = utc_now()
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            parameter_set_id = self._insert_parameter_set(connection, campaign_id, document, group_name, now)
            return parameter_set_id

    def record_artifact(self, campaign_id: str, kind: str, path: str, digest: str | None = None) -> None:
        """Register a reproducible file artifact without changing optimizer state."""
        if not kind or not path:
            raise DatabaseError("artifact kind and path must be non-empty")
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            self._insert_artifact(connection, campaign_id, kind, path, digest, utc_now())

    def artifacts(self, campaign_id: str) -> list[dict[str, Any]]:
        with self._read() as connection:
            self._campaign(connection, campaign_id)
            rows = connection.execute(
                "SELECT * FROM artifacts WHERE campaign_id=? ORDER BY created_at,artifact_id", (campaign_id,)
            ).fetchall()
            return [dict(row) for row in rows]

    def parameter_set(self, parameter_set_id: str, campaign_id: str) -> dict[str, Any]:
        with self._read() as connection:
            row = connection.execute(
                "SELECT * FROM parameter_sets WHERE parameter_set_id=? AND campaign_id=?",
                (parameter_set_id, campaign_id),
            ).fetchone()
            if row is None:
                raise DatabaseError(f"unknown parameter set: {parameter_set_id}")
            result = dict(row)
            result["document"] = json.loads(result.pop("document_json"))
            return result

    def parameter_set_by_hash(self, campaign_id: str, parameter_hash: str) -> dict[str, Any] | None:
        with self._read() as connection:
            row = connection.execute(
                "SELECT * FROM parameter_sets WHERE campaign_id=? AND parameter_hash=?",
                (campaign_id, parameter_hash),
            ).fetchone()
            if row is None:
                return None
            result = dict(row)
            result["document"] = json.loads(result.pop("document_json"))
            return result

    def create_trial(
        self,
        campaign_id: str,
        parameter_set_id: str,
        algorithm: str,
        seed: int,
        profile_name: str | None = None,
        profile_hash: str | None = None,
        profile_tc: str | None = None,
        profile_mode: str | None = None,
        profile_nodes: int | None = None,
    ) -> str:
        now = utc_now()
        with self._transaction() as connection:
            campaign = self._campaign(connection, campaign_id)
            if campaign["status"] in {"completed", "failed", "rejected"}:
                raise DatabaseError(f"cannot create a trial in campaign state {campaign['status']}")
            parameter = connection.execute(
                "SELECT * FROM parameter_sets WHERE parameter_set_id=? AND campaign_id=?",
                (parameter_set_id, campaign_id),
            ).fetchone()
            if parameter is None:
                raise DatabaseError(f"unknown parameter set for campaign: {parameter_set_id}")
            existing = connection.execute(
                "SELECT * FROM trials WHERE campaign_id=? AND parameter_set_id=?",
                (campaign_id, parameter_set_id),
            ).fetchone()
            if existing is not None:
                existing_profile = {
                    "name": existing["profile_name"],
                    "hash": existing["profile_hash"],
                    "tc": existing["profile_tc"],
                    "mode": existing["profile_mode"],
                    "nodes": existing["profile_nodes"],
                }
                requested_profile = {
                    "name": profile_name,
                    "hash": profile_hash,
                    "tc": profile_tc,
                    "mode": profile_mode,
                    "nodes": profile_nodes,
                }
                if (
                    any(value is not None for value in existing_profile.values())
                    and any(value is not None for value in requested_profile.values())
                    and not profiles_equivalent(existing_profile, requested_profile)
                ):
                    raise CampaignConflict("trial already exists with a different profile")
                return str(existing["trial_id"])
            number = connection.execute(
                "SELECT COUNT(*) FROM trials WHERE campaign_id=?", (campaign_id,)
            ).fetchone()[0] + 1
            trial_id = f"trial-{number:06d}"
            connection.execute(
                "INSERT INTO trials(trial_id,campaign_id,parameter_set_id,status,algorithm,seed,profile_name,"
                "profile_hash,profile_tc,profile_mode,profile_nodes,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)",
                (
                    trial_id,
                    campaign_id,
                    parameter_set_id,
                    "pending",
                    algorithm,
                    seed,
                    profile_name,
                    profile_hash,
                    profile_tc,
                    profile_mode,
                    profile_nodes,
                    now,
                    now,
                ),
            )
            self._event(
                connection,
                campaign_id,
                "trial_created",
                "trial",
                trial_id,
                {"parameter_set_id": parameter_set_id, "algorithm": algorithm, "seed": seed},
                to_status="pending",
            )
            return trial_id

    def bind_trial_profile(
        self,
        campaign_id: str,
        trial_id: str,
        profile_name: str,
        profile_hash: str,
        profile_tc: str | None,
        profile_mode: str = "time",
        profile_nodes: int | None = None,
    ) -> dict[str, Any]:
        """Bind or validate an immutable profile identity for a trial."""
        if (
            not isinstance(profile_name, str)
            or not profile_name
            or not isinstance(profile_hash, str)
            or not profile_hash
            or profile_mode not in {"time", "nodes"}
            or (profile_mode == "time" and (not isinstance(profile_tc, str) or not profile_tc))
            or (profile_mode == "nodes" and (not isinstance(profile_nodes, int) or profile_nodes <= 0))
        ):
            raise DatabaseError("trial profile identity is invalid")
        values = (profile_name, profile_hash, profile_tc, profile_mode, profile_nodes)
        with self._transaction() as connection:
            row = connection.execute(
                "SELECT * FROM trials WHERE campaign_id=? AND trial_id=?", (campaign_id, trial_id)
            ).fetchone()
            if row is None:
                raise DatabaseError(f"unknown trial: {trial_id}")
            existing_profile = {
                "name": row["profile_name"],
                "hash": row["profile_hash"],
                "tc": row["profile_tc"],
                "mode": row["profile_mode"],
                "nodes": row["profile_nodes"],
            }
            requested_profile = {
                "name": profile_name,
                "hash": profile_hash,
                "tc": profile_tc,
                "mode": profile_mode,
                "nodes": profile_nodes,
            }
            if any(value is not None for value in existing_profile.values()) and not profiles_equivalent(
                existing_profile, requested_profile
            ):
                raise CampaignConflict(f"trial {trial_id} already uses a different profile")
            if profiles_equivalent(existing_profile, requested_profile):
                return dict(row)
            now = utc_now()
            connection.execute(
                "UPDATE trials SET profile_name=?,profile_hash=?,profile_tc=?,profile_mode=?,profile_nodes=?,updated_at=? "
                "WHERE campaign_id=? AND trial_id=?",
                (*values, now, campaign_id, trial_id),
            )
            self._event(
                connection,
                campaign_id,
                "trial_profile_bound",
                "trial",
                trial_id,
                {
                    "profile_name": profile_name,
                    "profile_hash": profile_hash,
                    "profile_tc": profile_tc,
                    "profile_mode": profile_mode,
                    "profile_nodes": profile_nodes,
                },
            )
            return dict(connection.execute("SELECT * FROM trials WHERE trial_id=?", (trial_id,)).fetchone())

    def trial(self, campaign_id: str, trial_id: str) -> dict[str, Any]:
        with self._read() as connection:
            row = connection.execute(
                "SELECT * FROM trials WHERE campaign_id=? AND trial_id=?", (campaign_id, trial_id)
            ).fetchone()
            if row is None:
                raise DatabaseError(f"unknown trial: {trial_id}")
            return dict(row)

    def trial_for_parameter_hash(self, campaign_id: str, parameter_hash: str) -> dict[str, Any] | None:
        with self._read() as connection:
            row = connection.execute(
                "SELECT trials.* FROM trials JOIN parameter_sets ON parameter_sets.parameter_set_id=trials.parameter_set_id "
                "WHERE trials.campaign_id=? AND parameter_sets.parameter_hash=?",
                (campaign_id, parameter_hash),
            ).fetchone()
            return dict(row) if row is not None else None

    def record_coordinate_result_atomically(
        self,
        campaign_id: str,
        trial_id: str,
        result: dict[str, Any],
        state: dict[str, Any],
    ) -> tuple[dict[str, Any], tuple[int, str]]:
        """Commit one coordinate result and the following RNG checkpoint together."""
        if not isinstance(result, dict) or not isinstance(state, dict):
            raise DatabaseError("coordinate result and state must be objects")
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            trial = connection.execute(
                "SELECT * FROM trials WHERE campaign_id=? AND trial_id=?", (campaign_id, trial_id)
            ).fetchone()
            if trial is None:
                raise DatabaseError(f"unknown trial: {trial_id}")
            now = utc_now()
            existing_result = trial["result_json"]
            if trial["status"] in {"pending", "running", "interrupted"}:
                connection.execute(
                    "UPDATE trials SET status='completed',result_json=?,error=NULL,pid=NULL,finished_at=?,updated_at=? "
                    "WHERE campaign_id=? AND trial_id=?",
                    (_json(result), now, now, campaign_id, trial_id),
                )
                self._event(
                    connection,
                    campaign_id,
                    "coordinate_result_recorded",
                    "trial",
                    trial_id,
                    {"parameter_set_id": trial["parameter_set_id"], "classification": result.get("classification")},
                    from_status=trial["status"],
                    to_status="completed",
                )
            elif trial["status"] == "completed":
                if existing_result != _json(result):
                    raise CampaignConflict(f"completed coordinate trial has different result: {trial_id}")
            else:
                raise InvalidTransition(f"cannot record coordinate result for trial {trial_id} from {trial['status']}")

            state_row = connection.execute(
                "SELECT revision,state_json FROM optimizer_state WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if state_row is None:
                raise DatabaseError(f"optimizer state is missing for campaign: {campaign_id}")
            stored_state = json.loads(state_row["state_json"])
            if not isinstance(stored_state, dict):
                raise DatabaseError("optimizer checkpoint state is not an object")
            state = dict(state)
            stored_profile = stored_state.get("search_profile")
            requested_profile = state.get("search_profile")
            if (
                stored_profile is not None
                and requested_profile is not None
                and not profiles_equivalent(stored_profile, requested_profile)
            ):
                raise CampaignConflict("checkpoint already uses a different search profile")
            if stored_profile is not None and requested_profile is None:
                state["search_profile"] = stored_profile
            revision = int(state_row["revision"]) + 1
            checkpoint_hash = sha256_json(state)
            connection.execute(
                "UPDATE optimizer_state SET revision=?,state_json=?,checkpoint_hash=?,updated_at=? WHERE campaign_id=?",
                (revision, _json(state), checkpoint_hash, now, campaign_id),
            )
            self._event(
                connection,
                campaign_id,
                "coordinate_checkpoint",
                "optimizer_state",
                campaign_id,
                {"revision": revision, "checkpoint_hash": checkpoint_hash, "trial_id": trial_id},
            )
            updated = dict(connection.execute("SELECT * FROM trials WHERE trial_id=?", (trial_id,)).fetchone())
            return updated, (revision, checkpoint_hash)

    def create_match_block(
        self,
        campaign_id: str,
        trial_id: str,
        partition_name: str,
        block_index: int,
        pairs_per_block: int,
        master_seed: int,
        opening_book_sha256: str,
        materialized_openings_sha256: str,
    ) -> str:
        if block_index < 0 or pairs_per_block < 1:
            raise DatabaseError("block index must be non-negative and pairs_per_block positive")
        now = utc_now()
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            trial = connection.execute(
                "SELECT * FROM trials WHERE trial_id=? AND campaign_id=?", (trial_id, campaign_id)
            ).fetchone()
            if trial is None:
                raise DatabaseError(f"unknown trial for campaign: {trial_id}")
            existing = connection.execute(
                "SELECT block_id FROM match_blocks WHERE campaign_id=? AND trial_id=? AND partition_name=? "
                "AND block_index=? AND pairs_per_block=? AND master_seed=? AND opening_book_sha256=? "
                "AND materialized_openings_sha256=?",
                (
                    campaign_id,
                    trial_id,
                    partition_name,
                    block_index,
                    pairs_per_block,
                    master_seed,
                    opening_book_sha256,
                    materialized_openings_sha256,
                ),
            ).fetchone()
            if existing is not None:
                return str(existing["block_id"])
            identity = [
                campaign_id,
                trial_id,
                partition_name,
                block_index,
                pairs_per_block,
                master_seed,
                opening_book_sha256,
                materialized_openings_sha256,
            ]
            block_id = f"block-{sha256_json(identity)[:20]}"
            connection.execute(
                "INSERT INTO match_blocks(block_id,campaign_id,trial_id,partition_name,block_index,pairs_per_block,"
                "master_seed,opening_book_sha256,materialized_openings_sha256,status,created_at,updated_at) "
                "VALUES(?,?,?,?,?,?,?,?,?,?,?,?)",
                (
                    block_id,
                    campaign_id,
                    trial_id,
                    partition_name,
                    block_index,
                    pairs_per_block,
                    master_seed,
                    opening_book_sha256,
                    materialized_openings_sha256,
                    "pending",
                    now,
                    now,
                ),
            )
            self._event(
                connection,
                campaign_id,
                "block_created",
                "match_block",
                block_id,
                {"trial_id": trial_id, "block_index": block_index},
                to_status="pending",
            )
            return block_id

    def ensure_fake_schedule(
        self,
        campaign_id: str,
        block_count: int,
        pairs_per_block: int,
        partition_name: str = "fake",
    ) -> tuple[str, list[str]]:
        """Create an idempotent fake trial and deterministic fake blocks.

        This is deliberately a test-only schedule. It never invokes an engine
        and gives the phase-6 scheduler stable identities to replay.
        """
        if block_count < 1 or pairs_per_block < 1:
            raise DatabaseError("block_count and pairs_per_block must be positive")
        now = utc_now()
        with self._transaction() as connection:
            campaign = self._campaign(connection, campaign_id)
            config = json.loads(campaign["config_json"])
            if config.get("mode") != "fake":
                raise DatabaseError("the fake scheduler requires a campaign with mode=fake")
            baseline = connection.execute(
                "SELECT parameter_set_id FROM parameter_sets WHERE campaign_id=? AND group_name='baseline' "
                "ORDER BY created_at LIMIT 1",
                (campaign_id,),
            ).fetchone()
            if baseline is None:
                raise DatabaseError("baseline parameter set is missing")
            existing_trial = connection.execute(
                "SELECT trial_id FROM trials WHERE campaign_id=? AND parameter_set_id=?",
                (campaign_id, baseline["parameter_set_id"]),
            ).fetchone()
            if existing_trial is None:
                number = connection.execute(
                    "SELECT COUNT(*) FROM trials WHERE campaign_id=?", (campaign_id,)
                ).fetchone()[0] + 1
                trial_id = f"trial-{number:06d}"
                connection.execute(
                    "INSERT INTO trials(trial_id,campaign_id,parameter_set_id,status,algorithm,seed,created_at,updated_at) "
                    "VALUES(?,?,?,?,?,?,?,?)",
                    (trial_id, campaign_id, baseline["parameter_set_id"], "pending", "fake", campaign["master_seed"], now, now),
                )
                self._event(
                    connection,
                    campaign_id,
                    "trial_created",
                    "trial",
                    trial_id,
                    {"parameter_set_id": baseline["parameter_set_id"], "algorithm": "fake", "seed": campaign["master_seed"]},
                    to_status="pending",
                )
            else:
                trial_id = str(existing_trial["trial_id"])

            book_hash = sha256_json(["fake-opening-book-v1", campaign_id, partition_name])
            block_ids: list[str] = []
            for block_index in range(block_count):
                openings_hash = sha256_json(
                    ["fake-opening-block-v1", campaign_id, partition_name, block_index, campaign["master_seed"]]
                )
                identity = [
                    campaign_id,
                    trial_id,
                    partition_name,
                    block_index,
                    pairs_per_block,
                    campaign["master_seed"],
                    book_hash,
                    openings_hash,
                ]
                block_id = f"block-{sha256_json(identity)[:20]}"
                existing_block = connection.execute(
                    "SELECT block_id FROM match_blocks WHERE block_id=?", (block_id,)
                ).fetchone()
                if existing_block is None:
                    connection.execute(
                        "INSERT INTO match_blocks(block_id,campaign_id,trial_id,partition_name,block_index,pairs_per_block,"
                        "master_seed,opening_book_sha256,materialized_openings_sha256,status,created_at,updated_at) "
                        "VALUES(?,?,?,?,?,?,?,?,?,?,?,?)",
                        (
                            block_id,
                            campaign_id,
                            trial_id,
                            partition_name,
                            block_index,
                            pairs_per_block,
                            campaign["master_seed"],
                            book_hash,
                            openings_hash,
                            "pending",
                            now,
                            now,
                        ),
                    )
                    self._event(
                        connection,
                        campaign_id,
                        "block_created",
                        "match_block",
                        block_id,
                        {"trial_id": trial_id, "block_index": block_index, "fake": True},
                        to_status="pending",
                    )
                block_ids.append(block_id)
            return trial_id, block_ids

    def claim_next_block(self, campaign_id: str, expected_block_id: str | None = None) -> dict[str, Any] | None:
        """Reserve one missing block, optionally requiring an exact identity."""
        with self._transaction() as connection:
            campaign = self._campaign(connection, campaign_id)
            if campaign["status"] != "running":
                return None
            running = connection.execute(
                "SELECT COUNT(*) FROM match_blocks WHERE campaign_id=? AND status='running'", (campaign_id,)
            ).fetchone()[0]
            if running:
                raise CampaignBusy(f"campaign {campaign_id} already has a running match block")
            if expected_block_id is None:
                row = connection.execute(
                    "SELECT * FROM match_blocks WHERE campaign_id=? AND status IN ('pending','interrupted') "
                    "ORDER BY block_index,created_at,block_id LIMIT 1",
                    (campaign_id,),
                ).fetchone()
            else:
                row = connection.execute(
                    "SELECT * FROM match_blocks WHERE campaign_id=? AND block_id=? "
                    "AND status IN ('pending','interrupted')",
                    (campaign_id, expected_block_id),
                ).fetchone()
            if row is None:
                return None
            now = utc_now()
            connection.execute(
                "UPDATE match_blocks SET status='running',attempt=attempt+1,pid=NULL,process_group_id=NULL,"
                "run_dir=NULL,command_json=NULL,error=NULL,started_at=?,finished_at=NULL,updated_at=? WHERE block_id=?",
                (now, now, row["block_id"]),
            )
            self._event(
                connection,
                campaign_id,
                "match_block_started",
                "match_block",
                row["block_id"],
                {"attempt": int(row["attempt"]) + 1},
                from_status=row["status"],
                to_status="running",
            )
            return dict(connection.execute("SELECT * FROM match_blocks WHERE block_id=?", (row["block_id"],)).fetchone())

    def set_block_process(
        self,
        campaign_id: str,
        block_id: str,
        pid: int,
        process_group_id: int,
        run_dir: str,
        command: list[str],
    ) -> dict[str, Any]:
        if pid < 1 or process_group_id < 1:
            raise DatabaseError("process identifiers must be positive")
        with self._transaction() as connection:
            block = connection.execute(
                "SELECT status FROM match_blocks WHERE block_id=? AND campaign_id=?",
                (block_id, campaign_id),
            ).fetchone()
            if block is None:
                raise DatabaseError(f"unknown match block: {block_id}")
            if block["status"] != "running":
                raise InvalidTransition(f"cannot attach a process to block {block_id} from {block['status']}")
            connection.execute(
                "UPDATE match_blocks SET pid=?,process_group_id=?,run_dir=?,command_json=?,updated_at=? "
                "WHERE block_id=? AND campaign_id=?",
                (pid, process_group_id, run_dir, _json(command), utc_now(), block_id, campaign_id),
            )
            return dict(connection.execute("SELECT * FROM match_blocks WHERE block_id=?", (block_id,)).fetchone())

    def running_block_processes(self, campaign_id: str) -> list[dict[str, Any]]:
        with self._read() as connection:
            self._campaign(connection, campaign_id)
            rows = connection.execute(
                "SELECT * FROM match_blocks WHERE campaign_id=? AND status='running' ORDER BY block_index",
                (campaign_id,),
            ).fetchall()
            return [dict(row) for row in rows]

    def interrupt_block(self, campaign_id: str, block_id: str, reason: str) -> dict[str, Any]:
        with self._transaction() as connection:
            block = connection.execute(
                "SELECT * FROM match_blocks WHERE block_id=? AND campaign_id=?", (block_id, campaign_id)
            ).fetchone()
            if block is None:
                raise DatabaseError(f"unknown match block: {block_id}")
            if block["status"] != "running":
                return dict(block)
            now = utc_now()
            connection.execute(
                "UPDATE match_blocks SET status='interrupted',pid=NULL,process_group_id=NULL,run_dir=NULL,"
                "command_json=NULL,error=?,finished_at=?,updated_at=? WHERE block_id=? AND campaign_id=?",
                (reason, now, now, block_id, campaign_id),
            )
            self._event(
                connection,
                campaign_id,
                "match_block_interrupted",
                "match_block",
                block_id,
                {"reason": reason, "attempt": block["attempt"]},
                from_status="running",
                to_status="interrupted",
            )
            return dict(connection.execute("SELECT * FROM match_blocks WHERE block_id=?", (block_id,)).fetchone())

    def finish_completed_work(self, campaign_id: str) -> dict[str, int | bool]:
        """Finish trials/campaign only after every planned block is complete."""
        with self._transaction() as connection:
            campaign = self._campaign(connection, campaign_id)
            trial_rows = connection.execute(
                "SELECT trial_id,status FROM trials WHERE campaign_id=? ORDER BY created_at", (campaign_id,)
            ).fetchall()
            completed_trials = 0
            for trial in trial_rows:
                counts = connection.execute(
                    "SELECT COUNT(*) AS total, COALESCE(SUM(status='completed'),0) AS completed "
                    "FROM match_blocks WHERE campaign_id=? AND trial_id=?",
                    (campaign_id, trial["trial_id"]),
                ).fetchone()
                if counts["total"] and counts["total"] == counts["completed"] and trial["status"] != "completed":
                    now = utc_now()
                    connection.execute(
                        "UPDATE trials SET status='completed',finished_at=?,updated_at=? WHERE trial_id=?",
                        (now, now, trial["trial_id"]),
                    )
                    self._event(
                        connection,
                        campaign_id,
                        "trial_status_changed",
                        "trial",
                        trial["trial_id"],
                        {"reason": "all match blocks completed"},
                        from_status=trial["status"],
                        to_status="completed",
                    )
                    completed_trials += 1
            total_blocks = connection.execute(
                "SELECT COUNT(*) FROM match_blocks WHERE campaign_id=?", (campaign_id,)
            ).fetchone()[0]
            incomplete_blocks = connection.execute(
                "SELECT COUNT(*) FROM match_blocks WHERE campaign_id=? AND status!='completed'", (campaign_id,)
            ).fetchone()[0]
            completed = bool(
                campaign["status"] == "running" and total_blocks and incomplete_blocks == 0
            )
            if completed:
                now = utc_now()
                connection.execute(
                    "UPDATE campaigns SET status='completed',finished_at=?,updated_at=?,revision=revision+1 "
                    "WHERE campaign_id=?",
                    (now, now, campaign_id),
                )
                self._event(
                    connection,
                    campaign_id,
                    "campaign_status_changed",
                    "campaign",
                    campaign_id,
                    {"reason": "all match blocks completed"},
                    from_status="running",
                    to_status="completed",
                )
            return {"completed_trials": completed_trials, "campaign_completed": completed}

    def _transition(
        self,
        connection: sqlite3.Connection,
        table: str,
        identifier_column: str,
        identifier: str,
        campaign_id: str,
        new_status: str,
        transitions: dict[str, set[str]],
        entity_type: str,
        error: str | None = None,
        result: Any = None,
    ) -> dict[str, Any]:
        row = connection.execute(
            f"SELECT * FROM {table} WHERE {identifier_column}=? AND campaign_id=?", (identifier, campaign_id)
        ).fetchone()
        if row is None:
            raise DatabaseError(f"unknown {entity_type}: {identifier}")
        old_status = str(row["status"])
        if old_status == new_status:
            return dict(row)
        if new_status not in transitions.get(old_status, set()):
            raise InvalidTransition(f"cannot transition {entity_type} {identifier} from {old_status} to {new_status}")
        now = utc_now()
        assignments = ["status=?", "updated_at=?"]
        values: list[Any] = [new_status, now]
        if new_status == "running":
            assignments.append("started_at=COALESCE(started_at, ?)")
            values.append(now)
            assignments.append("finished_at=NULL")
            assignments.append("error=NULL")
        if new_status in {"completed", "failed", "interrupted", "rejected"}:
            assignments.append("finished_at=?")
            values.append(now)
        if error is not None:
            assignments.append("error=?")
            values.append(error)
        if result is not None:
            assignments.append("result_json=?")
            values.append(_json(result))
        values.extend([identifier, campaign_id])
        connection.execute(
            f"UPDATE {table} SET {', '.join(assignments)} WHERE {identifier_column}=? AND campaign_id=?",
            values,
        )
        self._event(
            connection,
            campaign_id,
            f"{entity_type}_status_changed",
            entity_type,
            identifier,
            {"error": error, "result": result} if error is not None or result is not None else {},
            from_status=old_status,
            to_status=new_status,
        )
        updated = connection.execute(
            f"SELECT * FROM {table} WHERE {identifier_column}=?", (identifier,)
        ).fetchone()
        return dict(updated)

    def transition_campaign(self, campaign_id: str, new_status: str, reason: str | None = None) -> dict[str, Any]:
        if new_status not in CAMPAIGN_STATES:
            raise InvalidTransition(f"unknown campaign status: {new_status}")
        with self._transaction() as connection:
            row = self._campaign(connection, campaign_id)
            old_status = str(row["status"])
            if old_status == new_status:
                return dict(row)
            if new_status not in CAMPAIGN_TRANSITIONS[old_status]:
                raise InvalidTransition(f"cannot transition campaign {campaign_id} from {old_status} to {new_status}")
            now = utc_now()
            connection.execute(
                "UPDATE campaigns SET status=?,updated_at=?,finished_at=?,revision=revision+1 WHERE campaign_id=?",
                (
                    new_status,
                    now,
                    now if new_status in {"completed", "failed", "interrupted", "rejected"} else None,
                    campaign_id,
                ),
            )
            self._event(
                connection,
                campaign_id,
                "campaign_status_changed",
                "campaign",
                campaign_id,
                {"reason": reason} if reason else {},
                from_status=old_status,
                to_status=new_status,
            )
            return dict(connection.execute("SELECT * FROM campaigns WHERE campaign_id=?", (campaign_id,)).fetchone())

    def begin_confirmation(self, campaign_id: str) -> dict[str, Any]:
        """Keep the persisted campaign non-terminal while confirmation runs."""
        with self._transaction() as connection:
            row = self._campaign(connection, campaign_id)
            if row["status"] != "completed":
                return dict(row)
            now = utc_now()
            connection.execute(
                "UPDATE campaigns SET status='running',updated_at=?,finished_at=NULL,revision=revision+1 "
                "WHERE campaign_id=?",
                (now, campaign_id),
            )
            self._event(
                connection,
                campaign_id,
                "campaign_status_changed",
                "campaign",
                campaign_id,
                {"reason": "fixed confirmation started"},
                from_status="completed",
                to_status="running",
            )
            return dict(connection.execute("SELECT * FROM campaigns WHERE campaign_id=?", (campaign_id,)).fetchone())

    def transition_trial(
        self, campaign_id: str, trial_id: str, new_status: str, error: str | None = None, result: Any = None
    ) -> dict[str, Any]:
        if new_status not in TRIAL_STATES:
            raise InvalidTransition(f"unknown trial status: {new_status}")
        with self._transaction() as connection:
            return self._transition(
                connection, "trials", "trial_id", trial_id, campaign_id, new_status, TRIAL_TRANSITIONS, "trial", error, result
            )

    def transition_block(
        self, campaign_id: str, block_id: str, new_status: str, error: str | None = None, result: Any = None
    ) -> dict[str, Any]:
        if new_status not in BLOCK_STATES:
            raise InvalidTransition(f"unknown block status: {new_status}")
        with self._transaction() as connection:
            return self._transition(
                connection,
                "match_blocks",
                "block_id",
                block_id,
                campaign_id,
                new_status,
                BLOCK_TRANSITIONS,
                "match_block",
                error,
                result,
            )

    def checkpoint_trial_result(self, campaign_id: str, trial_id: str, result: dict[str, Any]) -> dict[str, Any]:
        """Persist adaptive evidence without changing the trial status."""
        if not isinstance(result, dict):
            raise DatabaseError("trial checkpoint result must be an object")
        with self._transaction() as connection:
            row = connection.execute(
                "SELECT * FROM trials WHERE trial_id=? AND campaign_id=?", (trial_id, campaign_id)
            ).fetchone()
            if row is None:
                raise DatabaseError(f"unknown trial: {trial_id}")
            if row["status"] not in {"pending", "running", "interrupted"}:
                raise InvalidTransition(f"cannot checkpoint trial {trial_id} from {row['status']}")
            now = utc_now()
            connection.execute(
                "UPDATE trials SET result_json=?,updated_at=? WHERE trial_id=? AND campaign_id=?",
                (_json(result), now, trial_id, campaign_id),
            )
            self._event(
                connection,
                campaign_id,
                "trial_checkpoint",
                "trial",
                trial_id,
                {
                    "phase": result.get("phase"),
                    "decision": result.get("decision"),
                    "next_block_index": result.get("next_block_index"),
                },
                from_status=row["status"],
                to_status=row["status"],
            )
            return dict(connection.execute("SELECT * FROM trials WHERE trial_id=?", (trial_id,)).fetchone())

    def reject_pending_blocks(self, campaign_id: str, trial_id: str, reason: str) -> int:
        """Close unused budget blocks so an early decision cannot be replayed."""
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            rows = connection.execute(
                "SELECT block_id,attempt FROM match_blocks WHERE campaign_id=? AND trial_id=? AND status='pending' "
                "ORDER BY block_index",
                (campaign_id, trial_id),
            ).fetchall()
            now = utc_now()
            for row in rows:
                connection.execute(
                    "UPDATE match_blocks SET status='rejected',error=?,finished_at=?,updated_at=? "
                    "WHERE block_id=? AND campaign_id=?",
                    (reason, now, now, row["block_id"], campaign_id),
                )
                self._event(
                    connection,
                    campaign_id,
                    "match_block_budget_closed",
                    "match_block",
                    row["block_id"],
                    {"reason": reason, "attempt": row["attempt"]},
                    from_status="pending",
                    to_status="rejected",
                )
            return len(rows)

    def reconcile_terminal_trial_blocks(self, campaign_id: str, reason: str) -> int:
        """Close unused blocks left behind by an already-terminal trial."""
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            running = connection.execute(
                "SELECT COUNT(*) FROM match_blocks b JOIN trials t "
                "ON t.trial_id=b.trial_id AND t.campaign_id=b.campaign_id "
                "WHERE b.campaign_id=? AND t.status IN ('completed','rejected','failed') "
                "AND b.status='running'",
                (campaign_id,),
            ).fetchone()[0]
            if running:
                raise CampaignBusy("terminal trial still has a running match block")
            rows = connection.execute(
                "SELECT b.block_id,b.status FROM match_blocks b JOIN trials t "
                "ON t.trial_id=b.trial_id AND t.campaign_id=b.campaign_id "
                "WHERE b.campaign_id=? AND t.status IN ('completed','rejected','failed') "
                "AND b.status IN ('pending','interrupted') ORDER BY b.created_at,b.block_id",
                (campaign_id,),
            ).fetchall()
            now = utc_now()
            for row in rows:
                connection.execute(
                    "UPDATE match_blocks SET status='rejected',error=?,pid=NULL,process_group_id=NULL,"
                    "run_dir=NULL,command_json=NULL,finished_at=?,updated_at=? "
                    "WHERE block_id=? AND campaign_id=?",
                    (reason, now, now, row["block_id"], campaign_id),
                )
                self._event(
                    connection,
                    campaign_id,
                    "terminal_trial_block_reconciled",
                    "match_block",
                    row["block_id"],
                    {"reason": reason},
                    from_status=row["status"],
                    to_status="rejected",
                )
            return len(rows)

    def checkpoint(self, campaign_id: str, state: dict[str, Any], event_type: str = "checkpoint") -> tuple[int, str]:
        if not isinstance(state, dict):
            raise DatabaseError("checkpoint state must be an object")
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            row = connection.execute(
                "SELECT revision,state_json FROM optimizer_state WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if row is None:
                raise DatabaseError(f"optimizer state is missing for campaign: {campaign_id}")
            stored_state = json.loads(row["state_json"])
            if not isinstance(stored_state, dict):
                raise DatabaseError("optimizer checkpoint state is not an object")
            state = dict(state)
            for immutable_key in ("search_profile",):
                stored_profile = stored_state.get(immutable_key)
                requested_profile = state.get(immutable_key)
                if (
                    stored_profile is not None
                    and requested_profile is not None
                    and not profiles_equivalent(stored_profile, requested_profile)
                ):
                    raise CampaignConflict(f"checkpoint already uses a different {immutable_key}")
                if stored_profile is not None and requested_profile is None:
                    state[immutable_key] = stored_profile
            revision = int(row["revision"]) + 1
            checkpoint_hash = sha256_json(state)
            now = utc_now()
            connection.execute(
                "UPDATE optimizer_state SET revision=?,state_json=?,checkpoint_hash=?,updated_at=? WHERE campaign_id=?",
                (revision, _json(state), checkpoint_hash, now, campaign_id),
            )
            self._event(
                connection,
                campaign_id,
                event_type,
                "optimizer_state",
                campaign_id,
                {"revision": revision, "checkpoint_hash": checkpoint_hash},
            )
            return revision, checkpoint_hash

    def bind_optimizer_profile(
        self, campaign_id: str, role: str, profile: dict[str, Any]
    ) -> dict[str, Any]:
        """Persist and validate the immutable profile used by a search role."""
        if role not in {"search"}:
            raise DatabaseError(f"unsupported optimizer profile role: {role}")
        if not isinstance(profile, dict):
            raise DatabaseError("optimizer profile must be an object")
        identity = {
            "name": profile.get("name"),
            "hash": profile.get("hash"),
            "mode": profile.get("mode"),
            "tc": profile.get("tc"),
            "nodes": profile.get("nodes"),
        }
        if profile.get("mode") is None and isinstance(profile.get("tc"), str):
            identity["mode"] = "time"
        if profile_identity(identity) is None:
            raise DatabaseError("optimizer profile must contain a valid name, hash and tc/nodes")
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            row = connection.execute(
                "SELECT revision,state_json FROM optimizer_state WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if row is None:
                raise DatabaseError(f"optimizer state is missing for campaign: {campaign_id}")
            state = json.loads(row["state_json"])
            key = f"{role}_profile"
            existing = state.get(key)
            if existing is not None and not profiles_equivalent(existing, profile):
                raise CampaignConflict(f"campaign already uses a different {role} profile")
            if existing is not None and profiles_equivalent(existing, profile):
                return state
            state[key] = dict(profile)
            revision = int(row["revision"]) + 1
            checkpoint_hash = sha256_json(state)
            now = utc_now()
            connection.execute(
                "UPDATE optimizer_state SET revision=?,state_json=?,checkpoint_hash=?,updated_at=? "
                "WHERE campaign_id=?",
                (revision, _json(state), checkpoint_hash, now, campaign_id),
            )
            self._event(
                connection,
                campaign_id,
                "optimizer_profile_bound",
                "optimizer_state",
                campaign_id,
                {"role": role, "profile": profile, "revision": revision},
            )
            return state

    def complete_block_atomically(
        self,
        campaign_id: str,
        block_id: str,
        wins: int,
        draws: int,
        losses: int,
        score: float,
        result: dict[str, Any],
        checkpoint_state: dict[str, Any],
    ) -> tuple[dict[str, Any], tuple[int, str]]:
        if min(wins, draws, losses) < 0:
            raise DatabaseError("block result counts cannot be negative")
        game_results = result.get("games") if isinstance(result, dict) else None
        if game_results is not None:
            if not isinstance(game_results, list):
                raise DatabaseError("block result games must be a list")
            if len(game_results) != wins + draws + losses:
                raise DatabaseError("block result game count does not match W-D-L")
            for game in game_results:
                value = game.get("result") if isinstance(game, dict) else game
                if value not in {"1-0", "0-1", "1/2-1/2"}:
                    raise DatabaseError(f"unsupported fake game result: {value}")
        with self._transaction() as connection:
            campaign = self._campaign(connection, campaign_id)
            if campaign["status"] != "running":
                raise InvalidTransition(
                    f"cannot complete block {block_id} while campaign is {campaign['status']}"
                )
            block = connection.execute(
                "SELECT * FROM match_blocks WHERE block_id=? AND campaign_id=?", (block_id, campaign_id)
            ).fetchone()
            if block is None:
                raise DatabaseError(f"unknown match block: {block_id}")
            if block["status"] != "running":
                raise InvalidTransition(f"cannot complete block {block_id} from {block['status']}")
            now = utc_now()
            connection.execute(
                "UPDATE match_blocks SET status='completed',wins=?,draws=?,losses=?,score=?,result_json=?,pid=NULL,"
                "process_group_id=NULL,run_dir=NULL,command_json=NULL,finished_at=?,updated_at=? "
                "WHERE block_id=? AND campaign_id=?",
                (wins, draws, losses, score, _json(result), now, now, block_id, campaign_id),
            )
            self._event(
                connection,
                campaign_id,
                "match_block_completed",
                "match_block",
                block_id,
                {"wins": wins, "draws": draws, "losses": losses, "score": score},
                from_status="running",
                to_status="completed",
            )
            if game_results is not None:
                for game_index, game in enumerate(game_results):
                    value = game.get("result") if isinstance(game, dict) else game
                    game_id = f"{block_id}-game-{game_index:04d}"
                    connection.execute(
                        "INSERT INTO games(game_id,campaign_id,block_id,game_index,result,created_at) "
                        "VALUES(?,?,?,?,?,?)",
                        (game_id, campaign_id, block_id, game_index, value, now),
                    )
            state_row = connection.execute(
                "SELECT revision,state_json FROM optimizer_state WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if state_row is None:
                raise DatabaseError(f"optimizer state is missing for campaign: {campaign_id}")
            stored_state = json.loads(state_row["state_json"])
            if not isinstance(stored_state, dict):
                raise DatabaseError("optimizer checkpoint state is not an object")
            checkpoint_state = dict(checkpoint_state)
            stored_profile = stored_state.get("search_profile")
            requested_profile = checkpoint_state.get("search_profile")
            if (
                stored_profile is not None
                and requested_profile is not None
                and not profiles_equivalent(stored_profile, requested_profile)
            ):
                raise CampaignConflict("checkpoint already uses a different search profile")
            if stored_profile is not None and requested_profile is None:
                checkpoint_state["search_profile"] = stored_profile
            revision = int(state_row["revision"]) + 1
            checkpoint_hash = sha256_json(checkpoint_state)
            connection.execute(
                "UPDATE optimizer_state SET revision=?,state_json=?,checkpoint_hash=?,updated_at=? WHERE campaign_id=?",
                (revision, _json(checkpoint_state), checkpoint_hash, now, campaign_id),
            )
            self._event(
                connection,
                campaign_id,
                "checkpoint",
                "optimizer_state",
                campaign_id,
                {"revision": revision, "checkpoint_hash": checkpoint_hash, "block_id": block_id},
            )
            updated = dict(connection.execute("SELECT * FROM match_blocks WHERE block_id=?", (block_id,)).fetchone())
            return updated, (revision, checkpoint_hash)

    def create_confirmation(
        self,
        campaign_id: str,
        candidate_document: dict[str, Any],
        baseline_document: dict[str, Any],
        games_target: int,
        seed: int,
        confidence: float,
        profile_name: str | None = None,
        profile_hash: str | None = None,
        profile_tc: str | None = None,
        profile_mode: str | None = None,
        profile_nodes: int | None = None,
    ) -> str:
        """Create the immutable confirmation record, independently of search state."""
        if games_target < 2 or games_target % 2:
            raise DatabaseError("confirmation games must be a positive even number >= 2")
        if seed < 0 or not 0 < confidence < 1:
            raise DatabaseError("confirmation seed/confidence is invalid")
        candidate_hash = sha256_json(candidate_document)
        baseline_hash = sha256_json(baseline_document)
        confirmation_id = f"confirmation-{sha256_json([campaign_id, candidate_hash, baseline_hash, seed])[:20]}"
        now = utc_now()
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            existing = connection.execute(
                "SELECT * FROM confirmations WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if existing is not None:
                if (
                    existing["candidate_parameter_hash"] != candidate_hash
                    or existing["baseline_parameter_hash"] != baseline_hash
                    or int(existing["games_target"]) != games_target
                    or int(existing["seed"]) != seed
                    or float(existing["confidence"]) != float(confidence)
                ):
                    raise CampaignConflict("confirmation already exists with different inputs")
                existing_profile = {
                    "name": existing["profile_name"],
                    "hash": existing["profile_hash"],
                    "tc": existing["profile_tc"],
                    "mode": existing["profile_mode"],
                    "nodes": existing["profile_nodes"],
                }
                requested_profile = {
                    "name": profile_name,
                    "hash": profile_hash,
                    "tc": profile_tc,
                    "mode": profile_mode,
                    "nodes": profile_nodes,
                }
                if any(value is not None for value in existing_profile.values()) and not profiles_equivalent(
                    existing_profile, requested_profile
                ):
                    raise CampaignConflict("confirmation already exists with a different profile")
                if any(value is not None for value in requested_profile.values()) and not profiles_equivalent(
                    existing_profile, requested_profile
                ):
                    connection.execute(
                        "UPDATE confirmations SET profile_name=?,profile_hash=?,profile_tc=?,profile_mode=?,profile_nodes=?,updated_at=? "
                        "WHERE confirmation_id=?",
                        (
                            profile_name,
                            profile_hash,
                            profile_tc,
                            profile_mode,
                            profile_nodes,
                            now,
                            existing["confirmation_id"],
                        ),
                    )
                return str(existing["confirmation_id"])
            connection.execute(
                "INSERT INTO confirmations(confirmation_id,campaign_id,status,candidate_parameter_hash,"
                "baseline_parameter_hash,candidate_document_json,baseline_document_json,games_target,seed,"
                "confidence,profile_name,profile_hash,profile_tc,profile_mode,profile_nodes,created_at,updated_at) "
                "VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
                (
                    confirmation_id,
                    campaign_id,
                    "pending",
                    candidate_hash,
                    baseline_hash,
                    _json(candidate_document),
                    _json(baseline_document),
                    games_target,
                    seed,
                    float(confidence),
                    profile_name,
                    profile_hash,
                    profile_tc,
                    profile_mode,
                    profile_nodes,
                    now,
                    now,
                ),
            )
            self._event(
                connection,
                campaign_id,
                "confirmation_created",
                "confirmation",
                confirmation_id,
                {
                    "candidate_parameter_hash": candidate_hash,
                    "baseline_parameter_hash": baseline_hash,
                    "games": games_target,
                    "seed": seed,
                    "confidence": confidence,
                    "profile_name": profile_name,
                    "profile_hash": profile_hash,
                    "profile_tc": profile_tc,
                    "profile_mode": profile_mode,
                    "profile_nodes": profile_nodes,
                },
                to_status="pending",
            )
            return confirmation_id

    def confirmation(self, campaign_id: str) -> dict[str, Any] | None:
        with self._read() as connection:
            self._campaign(connection, campaign_id)
            row = connection.execute(
                "SELECT * FROM confirmations WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if row is None:
                return None
            result = dict(row)
            result["candidate_document"] = json.loads(result.pop("candidate_document_json"))
            result["baseline_document"] = json.loads(result.pop("baseline_document_json"))
            if result.get("result_json"):
                result["result"] = json.loads(result.pop("result_json"))
            else:
                result.pop("result_json", None)
            return result

    def create_confirmation_block(
        self,
        confirmation_id: str,
        block_index: int,
        pairs_per_block: int,
        master_seed: int,
        opening_book_sha256: str,
        materialized_openings_sha256: str,
    ) -> str:
        if block_index < 0 or pairs_per_block < 1:
            raise DatabaseError("confirmation block index/pairs are invalid")
        now = utc_now()
        with self._transaction() as connection:
            confirmation = connection.execute(
                "SELECT * FROM confirmations WHERE confirmation_id=?", (confirmation_id,)
            ).fetchone()
            if confirmation is None:
                raise DatabaseError(f"unknown confirmation: {confirmation_id}")
            existing = connection.execute(
                "SELECT block_id FROM confirmation_blocks WHERE confirmation_id=? AND block_index=? "
                "AND pairs_per_block=? AND master_seed=? AND opening_book_sha256=? "
                "AND materialized_openings_sha256=?",
                (
                    confirmation_id,
                    block_index,
                    pairs_per_block,
                    master_seed,
                    opening_book_sha256,
                    materialized_openings_sha256,
                ),
            ).fetchone()
            if existing is not None:
                return str(existing["block_id"])
            identity = [
                confirmation_id,
                block_index,
                pairs_per_block,
                master_seed,
                opening_book_sha256,
                materialized_openings_sha256,
            ]
            block_id = f"confirmation-block-{sha256_json(identity)[:20]}"
            connection.execute(
                "INSERT INTO confirmation_blocks(block_id,confirmation_id,campaign_id,block_index,"
                "pairs_per_block,master_seed,opening_book_sha256,materialized_openings_sha256,status,"
                "created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
                (
                    block_id,
                    confirmation_id,
                    confirmation["campaign_id"],
                    block_index,
                    pairs_per_block,
                    master_seed,
                    opening_book_sha256,
                    materialized_openings_sha256,
                    "pending",
                    now,
                    now,
                ),
            )
            self._event(
                connection,
                confirmation["campaign_id"],
                "confirmation_block_created",
                "confirmation_block",
                block_id,
                {"confirmation_id": confirmation_id, "block_index": block_index},
                to_status="pending",
            )
            return block_id

    def confirmation_blocks(self, campaign_id: str, confirmation_id: str | None = None) -> list[dict[str, Any]]:
        with self._read() as connection:
            self._campaign(connection, campaign_id)
            if confirmation_id is None:
                row = connection.execute(
                    "SELECT confirmation_id FROM confirmations WHERE campaign_id=?", (campaign_id,)
                ).fetchone()
                if row is None:
                    return []
                confirmation_id = str(row["confirmation_id"])
            rows = connection.execute(
                "SELECT * FROM confirmation_blocks WHERE campaign_id=? AND confirmation_id=? "
                "ORDER BY block_index,block_id",
                (campaign_id, confirmation_id),
            ).fetchall()
            return [dict(row) for row in rows]

    def claim_next_confirmation_block(self, campaign_id: str) -> dict[str, Any] | None:
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            confirmation = connection.execute(
                "SELECT * FROM confirmations WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if confirmation is None or confirmation["status"] == "completed":
                return None
            running = connection.execute(
                "SELECT COUNT(*) FROM confirmation_blocks WHERE campaign_id=? AND status='running'",
                (campaign_id,),
            ).fetchone()[0]
            if running:
                raise CampaignBusy(f"campaign {campaign_id} already has a running confirmation block")
            row = connection.execute(
                "SELECT * FROM confirmation_blocks WHERE campaign_id=? AND confirmation_id=? "
                "AND status IN ('pending','interrupted') ORDER BY block_index,block_id LIMIT 1",
                (campaign_id, confirmation["confirmation_id"]),
            ).fetchone()
            if row is None:
                return None
            now = utc_now()
            connection.execute(
                "UPDATE confirmation_blocks SET status='running',attempt=attempt+1,pid=NULL,"
                "process_group_id=NULL,run_dir=NULL,command_json=NULL,error=NULL,started_at=?,"
                "finished_at=NULL,updated_at=? WHERE block_id=?",
                (now, now, row["block_id"]),
            )
            connection.execute(
                "UPDATE confirmations SET status='running',started_at=COALESCE(started_at,?),updated_at=? "
                "WHERE confirmation_id=? AND status IN ('pending','interrupted')",
                (now, now, confirmation["confirmation_id"]),
            )
            self._event(
                connection,
                campaign_id,
                "confirmation_block_started",
                "confirmation_block",
                row["block_id"],
                {"confirmation_id": confirmation["confirmation_id"], "attempt": int(row["attempt"]) + 1},
                from_status=row["status"],
                to_status="running",
            )
            return dict(connection.execute(
                "SELECT * FROM confirmation_blocks WHERE block_id=?", (row["block_id"],)
            ).fetchone())

    def set_confirmation_block_process(
        self,
        campaign_id: str,
        block_id: str,
        pid: int,
        process_group_id: int,
        run_dir: str,
        command: list[str],
    ) -> dict[str, Any]:
        if pid < 1 or process_group_id < 1:
            raise DatabaseError("confirmation process identifiers must be positive")
        with self._transaction() as connection:
            block = connection.execute(
                "SELECT status FROM confirmation_blocks WHERE block_id=? AND campaign_id=?",
                (block_id, campaign_id),
            ).fetchone()
            if block is None:
                raise DatabaseError(f"unknown confirmation block: {block_id}")
            if block["status"] != "running":
                raise InvalidTransition(f"cannot attach a process to confirmation block {block_id} from {block['status']}")
            connection.execute(
                "UPDATE confirmation_blocks SET pid=?,process_group_id=?,run_dir=?,command_json=?,updated_at=? "
                "WHERE block_id=? AND campaign_id=?",
                (pid, process_group_id, run_dir, _json(command), utc_now(), block_id, campaign_id),
            )
            return dict(connection.execute(
                "SELECT * FROM confirmation_blocks WHERE block_id=?", (block_id,)
            ).fetchone())

    def running_confirmation_block_processes(self, campaign_id: str) -> list[dict[str, Any]]:
        with self._read() as connection:
            self._campaign(connection, campaign_id)
            rows = connection.execute(
                "SELECT * FROM confirmation_blocks WHERE campaign_id=? AND status='running' ORDER BY block_index",
                (campaign_id,),
            ).fetchall()
            return [dict(row) for row in rows]

    def interrupt_confirmation_block(self, campaign_id: str, block_id: str, reason: str) -> dict[str, Any]:
        with self._transaction() as connection:
            block = connection.execute(
                "SELECT * FROM confirmation_blocks WHERE block_id=? AND campaign_id=?", (block_id, campaign_id)
            ).fetchone()
            if block is None:
                raise DatabaseError(f"unknown confirmation block: {block_id}")
            if block["status"] != "running":
                return dict(block)
            now = utc_now()
            connection.execute(
                "UPDATE confirmation_blocks SET status='interrupted',pid=NULL,process_group_id=NULL,"
                "run_dir=NULL,command_json=NULL,error=?,finished_at=?,updated_at=? WHERE block_id=?",
                (reason, now, now, block_id),
            )
            connection.execute(
                "UPDATE confirmations SET status='interrupted',error=?,updated_at=? "
                "WHERE confirmation_id=? AND status='running'",
                (reason, now, block["confirmation_id"]),
            )
            self._event(
                connection,
                campaign_id,
                "confirmation_block_interrupted",
                "confirmation_block",
                block_id,
                {"reason": reason, "attempt": block["attempt"]},
                from_status="running",
                to_status="interrupted",
            )
            return dict(connection.execute(
                "SELECT * FROM confirmation_blocks WHERE block_id=?", (block_id,)
            ).fetchone())

    def complete_confirmation_block_atomically(
        self,
        campaign_id: str,
        block_id: str,
        wins: int,
        draws: int,
        losses: int,
        score: float,
        result: dict[str, Any],
    ) -> dict[str, Any]:
        if min(wins, draws, losses) < 0:
            raise DatabaseError("confirmation result counts cannot be negative")
        game_results = result.get("games") if isinstance(result, dict) else None
        if not isinstance(game_results, list) or len(game_results) != wins + draws + losses:
            raise DatabaseError("confirmation result games do not match W-D-L")
        if any(value not in {"1-0", "0-1", "1/2-1/2"} for value in game_results):
            raise DatabaseError("unsupported confirmation game result")
        with self._transaction() as connection:
            block = connection.execute(
                "SELECT * FROM confirmation_blocks WHERE block_id=? AND campaign_id=?",
                (block_id, campaign_id),
            ).fetchone()
            if block is None:
                raise DatabaseError(f"unknown confirmation block: {block_id}")
            if block["status"] != "running":
                raise InvalidTransition(f"cannot complete confirmation block {block_id} from {block['status']}")
            expected_games = int(block["pairs_per_block"]) * 2
            if len(game_results) != expected_games:
                raise DatabaseError("confirmation block result does not contain a complete opening pair set")
            now = utc_now()
            connection.execute(
                "UPDATE confirmation_blocks SET status='completed',wins=?,draws=?,losses=?,score=?,"
                "result_json=?,pid=NULL,process_group_id=NULL,run_dir=NULL,command_json=NULL,"
                "finished_at=?,updated_at=? WHERE block_id=?",
                (wins, draws, losses, score, _json(result), now, now, block_id),
            )
            for game_index, value in enumerate(game_results):
                connection.execute(
                    "INSERT INTO confirmation_games(game_id,confirmation_id,block_id,game_index,result,created_at) "
                    "VALUES(?,?,?,?,?,?)",
                    (
                        f"{block_id}-game-{game_index:04d}",
                        block["confirmation_id"],
                        block_id,
                        game_index,
                        value,
                        now,
                    ),
                )
            self._event(
                connection,
                campaign_id,
                "confirmation_block_completed",
                "confirmation_block",
                block_id,
                {"wins": wins, "draws": draws, "losses": losses, "score": score},
                from_status="running",
                to_status="completed",
            )
            return dict(connection.execute(
                "SELECT * FROM confirmation_blocks WHERE block_id=?", (block_id,)
            ).fetchone())

    def recover_abandoned_confirmation_jobs(
        self, campaign_id: str, reason: str = "confirmation job has no trusted owner"
    ) -> dict[str, int]:
        recovered = {"blocks": 0}
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            now = utc_now()
            rows = connection.execute(
                "SELECT block_id,confirmation_id FROM confirmation_blocks WHERE campaign_id=? AND status='running'",
                (campaign_id,),
            ).fetchall()
            for row in rows:
                connection.execute(
                    "UPDATE confirmation_blocks SET status='interrupted',pid=NULL,process_group_id=NULL,"
                    "run_dir=NULL,command_json=NULL,error=?,finished_at=?,updated_at=? WHERE block_id=?",
                    (reason, now, now, row["block_id"]),
                )
                self._event(
                    connection,
                    campaign_id,
                    "abandoned_confirmation_recovered",
                    "confirmation_block",
                    row["block_id"],
                    {"reason": reason},
                    from_status="running",
                    to_status="interrupted",
                )
                recovered["blocks"] += 1
                connection.execute(
                    "UPDATE confirmations SET status='interrupted',error=?,updated_at=? WHERE confirmation_id=?",
                    (reason, now, row["confirmation_id"]),
                )
        return recovered

    def finalize_confirmation(self, campaign_id: str, result: dict[str, Any]) -> dict[str, Any]:
        outcome = result.get("outcome")
        if outcome not in {"confirmed", "rejected", "inconclusive"}:
            raise DatabaseError("invalid confirmation outcome")
        with self._transaction() as connection:
            confirmation = connection.execute(
                "SELECT * FROM confirmations WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if confirmation is None:
                raise DatabaseError(f"no confirmation for campaign: {campaign_id}")
            blocks = connection.execute(
                "SELECT status FROM confirmation_blocks WHERE confirmation_id=?", (confirmation["confirmation_id"],)
            ).fetchall()
            if not blocks:
                if not (
                    confirmation["candidate_parameter_hash"] == confirmation["baseline_parameter_hash"]
                    and outcome == "inconclusive"
                    and int(result.get("games", 0)) == 0
                ):
                    raise DatabaseError("cannot finalize confirmation before every block is complete")
            elif any(row["status"] != "completed" for row in blocks):
                raise DatabaseError("cannot finalize confirmation before every block is complete")
            expected_games = sum(
                int(row["pairs_per_block"]) * 2
                for row in connection.execute(
                    "SELECT pairs_per_block FROM confirmation_blocks WHERE confirmation_id=?",
                    (confirmation["confirmation_id"],),
                ).fetchall()
            )
            if blocks and (expected_games != int(confirmation["games_target"]) or int(result.get("games", 0)) != expected_games):
                raise DatabaseError("confirmation result does not cover the configured fixed game count")
            now = utc_now()
            recommendation_hash = result.get("recommendation_parameter_hash")
            connection.execute(
                "UPDATE confirmations SET status='completed',outcome=?,wins=?,draws=?,losses=?,score=?,"
                "score_ci_low=?,score_ci_high=?,recommendation_parameter_hash=?,result_json=?,error=NULL,"
                "finished_at=?,updated_at=? WHERE confirmation_id=?",
                (
                    outcome,
                    int(result.get("wins", 0)),
                    int(result.get("draws", 0)),
                    int(result.get("losses", 0)),
                    float(result.get("score", 0.0)),
                    float(result.get("score_ci_low", 0.0)),
                    float(result.get("score_ci_high", 100.0)),
                    recommendation_hash,
                    _json(result),
                    now,
                    now,
                    confirmation["confirmation_id"],
                ),
            )
            campaign = self._campaign(connection, campaign_id)
            if campaign["status"] != "completed":
                connection.execute(
                    "UPDATE campaigns SET status='completed',finished_at=?,updated_at=?,revision=revision+1 "
                    "WHERE campaign_id=?",
                    (now, now, campaign_id),
                )
                self._event(
                    connection,
                    campaign_id,
                    "campaign_status_changed",
                    "campaign",
                    campaign_id,
                    {"reason": "fixed confirmation completed"},
                    from_status=campaign["status"],
                    to_status="completed",
                )
            self._event(
                connection,
                campaign_id,
                "confirmation_finalized",
                "confirmation",
                confirmation["confirmation_id"],
                {"outcome": outcome, "recommendation_parameter_hash": recommendation_hash},
                from_status=confirmation["status"],
                to_status="completed",
            )
            row = connection.execute(
                "SELECT * FROM confirmations WHERE confirmation_id=?", (confirmation["confirmation_id"],)
            ).fetchone()
            final = dict(row)
            final["candidate_document"] = json.loads(final.pop("candidate_document_json"))
            final["baseline_document"] = json.loads(final.pop("baseline_document_json"))
            final["result"] = json.loads(final.pop("result_json"))
            return final

    def confirmation_snapshot(self, campaign_id: str) -> dict[str, Any] | None:
        record = self.confirmation(campaign_id)
        if record is None:
            return None
        record["blocks"] = self.confirmation_blocks(campaign_id, str(record["confirmation_id"]))
        metrics = _confirmation_metrics(record, record["blocks"])
        # The confirmation row is finalized only once.  During a running
        # campaign its counters are intentionally stale, so all live values
        # come from completed confirmation blocks instead.
        for key in (
            "blocks_completed",
            "games",
            "games_completed",
            "wins",
            "draws",
            "losses",
            "score",
            "score_percent",
            "score_ci_low",
            "score_ci_high",
            "elo_estimate",
            "elo_ci_low",
            "elo_ci_high",
            "uncertainty",
            "block_ids",
            "pairs_completed",
            "pairs_target",
            "games_target",
        ):
            if key in metrics:
                record[key] = metrics[key]
        record["metrics"] = metrics
        if isinstance(record.get("result"), dict):
            record["recommendation"] = record["result"].get("recommendation")
            record["automatic_promotion"] = record["result"].get("automatic_promotion")
        with self._read() as connection:
            artifact = connection.execute(
                "SELECT path,sha256 FROM artifacts WHERE campaign_id=? AND kind='recommended_parameters' "
                "ORDER BY created_at DESC LIMIT 1",
                (campaign_id,),
            ).fetchone()
        if artifact is not None:
            record["recommendation_parameter_file"] = artifact["path"]
            record["recommendation_parameter_file_sha256"] = artifact["sha256"]
        return record

    def recover_abandoned_jobs(self, campaign_id: str, reason: str = "running job has no trusted owner") -> dict[str, int]:
        recovered = {"trials": 0, "blocks": 0}
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            now = utc_now()
            blocks = connection.execute(
                "SELECT block_id FROM match_blocks WHERE campaign_id=? AND status='running'", (campaign_id,)
            ).fetchall()
            for row in blocks:
                connection.execute(
                    "UPDATE match_blocks SET status='interrupted',pid=NULL,process_group_id=NULL,run_dir=NULL,"
                    "command_json=NULL,error=?,finished_at=?,updated_at=? WHERE block_id=?",
                    (reason, now, now, row["block_id"]),
                )
                self._event(
                    connection,
                    campaign_id,
                    "abandoned_job_recovered",
                    "match_block",
                    row["block_id"],
                    {"reason": reason},
                    from_status="running",
                    to_status="interrupted",
                )
                recovered["blocks"] += 1
            trials = connection.execute(
                "SELECT trial_id FROM trials WHERE campaign_id=? AND status='running'", (campaign_id,)
            ).fetchall()
            for row in trials:
                connection.execute(
                    "UPDATE trials SET status='interrupted',error=?,finished_at=?,updated_at=? WHERE trial_id=?",
                    (reason, now, now, row["trial_id"]),
                )
                self._event(
                    connection,
                    campaign_id,
                    "abandoned_job_recovered",
                    "trial",
                    row["trial_id"],
                    {"reason": reason},
                    from_status="running",
                    to_status="interrupted",
                )
                recovered["trials"] += 1
        return recovered

    def claim_campaign(self, campaign_id: str, owner_token: str, takeover: bool = False) -> None:
        if not owner_token:
            raise CampaignBusy("owner token must be non-empty")
        with self._transaction() as connection:
            row = self._campaign(connection, campaign_id)
            current = row["owner_token"]
            if current is not None and current != owner_token and not takeover:
                raise CampaignBusy(f"campaign {campaign_id} is owned by another optimizer process")
            connection.execute(
                "UPDATE campaigns SET owner_token=?,owner_acquired_at=?,updated_at=? WHERE campaign_id=?",
                (owner_token, utc_now(), utc_now(), campaign_id),
            )
            self._event(connection, campaign_id, "campaign_claimed", "campaign", campaign_id, {"owner_token": owner_token})

    def release_campaign(self, campaign_id: str, owner_token: str) -> None:
        with self._transaction() as connection:
            row = self._campaign(connection, campaign_id)
            if row["owner_token"] not in {None, owner_token}:
                raise CampaignBusy(f"campaign {campaign_id} is owned by another optimizer process")
            connection.execute(
                "UPDATE campaigns SET owner_token=NULL,owner_acquired_at=NULL,updated_at=? WHERE campaign_id=?",
                (utc_now(), campaign_id),
            )
            self._event(connection, campaign_id, "campaign_released", "campaign", campaign_id, {})

    def campaign(self, campaign_id: str) -> dict[str, Any]:
        with self._read() as connection:
            row = connection.execute("SELECT * FROM campaigns WHERE campaign_id=?", (campaign_id,)).fetchone()
            if row is None:
                raise DatabaseError(f"unknown campaign: {campaign_id}")
            return dict(row)

    def status_snapshot(self, campaign_id: str) -> dict[str, Any]:
        with self._read() as connection:
            campaign = connection.execute("SELECT * FROM campaigns WHERE campaign_id=?", (campaign_id,)).fetchone()
            if campaign is None:
                raise DatabaseError(f"unknown campaign: {campaign_id}")
            trial_rows = connection.execute(
                "SELECT status,COUNT(*) AS count FROM trials WHERE campaign_id=? GROUP BY status", (campaign_id,)
            ).fetchall()
            block_rows = connection.execute(
                "SELECT status,COUNT(*) AS count FROM match_blocks WHERE campaign_id=? GROUP BY status", (campaign_id,)
            ).fetchall()
            games = connection.execute(
                "SELECT COALESCE(SUM(wins+draws+losses),0) AS games FROM match_blocks WHERE campaign_id=?",
                (campaign_id,),
            ).fetchone()["games"]
            checkpoint = connection.execute(
                "SELECT revision,checkpoint_hash,updated_at,state_json FROM optimizer_state WHERE campaign_id=?",
                (campaign_id,),
            ).fetchone()
            event_count = connection.execute(
                "SELECT COUNT(*) AS count FROM events WHERE campaign_id=?", (campaign_id,)
            ).fetchone()["count"]
            confirmation_row = connection.execute(
                "SELECT * FROM confirmations "
                "WHERE campaign_id=?",
                (campaign_id,),
            ).fetchone()
            confirmation = None
            if confirmation_row is not None:
                confirmation = dict(confirmation_row)
                confirmation_blocks = [
                    dict(row)
                    for row in connection.execute(
                        "SELECT * FROM confirmation_blocks WHERE campaign_id=? AND confirmation_id=? "
                        "ORDER BY block_index,block_id",
                        (campaign_id, confirmation["confirmation_id"]),
                    ).fetchall()
                ]
                live_metrics = _confirmation_metrics(confirmation, confirmation_blocks)
                confirmation = {
                    key: confirmation.get(key)
                    for key in (
                        "confirmation_id",
                        "status",
                        "outcome",
                        "games_target",
                        "wins",
                        "draws",
                        "losses",
                        "score",
                        "score_ci_low",
                        "score_ci_high",
                        "recommendation_parameter_hash",
                        "candidate_parameter_hash",
                        "baseline_parameter_hash",
                        "profile_name",
                        "profile_hash",
                        "profile_tc",
                        "profile_mode",
                        "profile_nodes",
                        "started_at",
                        "finished_at",
                        "updated_at",
                    )
                }
                confirmation.update(live_metrics)
            raw_status = str(campaign["status"])
            display_status = raw_status
            if confirmation is not None and confirmation["status"] != "completed" and raw_status in {
                "pending",
                "running",
                "paused",
                "interrupted",
                "completed",
            }:
                display_status = "confirming"
            checkpoint_dict = dict(checkpoint) if checkpoint else None
            checkpoint_state: dict[str, Any] = {}
            if checkpoint_dict is not None:
                try:
                    decoded_state = json.loads(checkpoint_dict.pop("state_json"))
                except (TypeError, json.JSONDecodeError):
                    decoded_state = {}
                if isinstance(decoded_state, dict):
                    checkpoint_state = decoded_state
            confirmation_profile = None
            if confirmation is not None and confirmation.get("profile_name"):
                confirmation_profile = {
                    "name": confirmation.get("profile_name"),
                    "hash": confirmation.get("profile_hash"),
                    "mode": confirmation.get("profile_mode") or "time",
                }
                if confirmation_profile["mode"] == "nodes":
                    confirmation_profile["nodes"] = confirmation.get("profile_nodes")
                else:
                    confirmation_profile["tc"] = confirmation.get("profile_tc")
            return {
                "campaign_id": campaign["campaign_id"],
                "name": campaign["name"],
                "status": display_status,
                "raw_status": raw_status,
                "config_hash": campaign["config_hash"],
                "baseline_parameter_hash": campaign["baseline_parameter_hash"],
                "master_seed": campaign["master_seed"],
                "created_at": campaign["created_at"],
                "updated_at": campaign["updated_at"],
                "finished_at": campaign["finished_at"],
                "trials": {row["status"]: row["count"] for row in trial_rows},
                "blocks": {row["status"]: row["count"] for row in block_rows},
                "games": int(games),
                "checkpoint": checkpoint_dict,
                "search_profile": checkpoint_state.get("search_profile"),
                "confirmation_profile": confirmation_profile,
                "event_count": int(event_count),
                "confirmation": confirmation,
                "journal_mode": "wal",
            }

    def list_trials(self, campaign_id: str, limit: int = 20) -> list[dict[str, Any]]:
        if limit < 1:
            raise DatabaseError("trial limit must be positive")
        with self._read() as connection:
            self._campaign(connection, campaign_id)
            rows = connection.execute(
                "SELECT * FROM trials WHERE campaign_id=? ORDER BY created_at DESC LIMIT ?", (campaign_id, limit)
            ).fetchall()
            return [dict(row) for row in rows]

    def best_trial(self, campaign_id: str) -> dict[str, Any] | None:
        with self._read() as connection:
            self._campaign(connection, campaign_id)
            row = connection.execute(
                "SELECT * FROM trials WHERE campaign_id=? AND status='completed' ORDER BY created_at DESC LIMIT 1",
                (campaign_id,),
            ).fetchone()
            return dict(row) if row else None

    def events(self, campaign_id: str) -> list[dict[str, Any]]:
        with self._read() as connection:
            self._campaign(connection, campaign_id)
            rows = connection.execute(
                "SELECT * FROM events WHERE campaign_id=? ORDER BY event_id", (campaign_id,)
            ).fetchall()
            return [dict(row) for row in rows]

    def optimizer_state(self, campaign_id: str) -> dict[str, Any]:
        with self._read() as connection:
            self._campaign(connection, campaign_id)
            row = connection.execute(
                "SELECT * FROM optimizer_state WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if row is None:
                raise DatabaseError(f"optimizer state is missing for campaign: {campaign_id}")
            result = dict(row)
            result["state"] = json.loads(result.pop("state_json"))
            return result
