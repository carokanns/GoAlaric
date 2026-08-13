from __future__ import annotations

import json
import sqlite3
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Iterator

from .canonical import canonical_json, sha256_json, utc_now
from .config import CampaignDefinition


SCHEMA_VERSION = 2
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
"""


def _row(row: sqlite3.Row | None) -> dict[str, Any] | None:
    return dict(row) if row is not None else None


def _json(value: Any) -> str:
    return canonical_json(value)


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
                version = int(existing["value"])
                if version == 1:
                    connection.execute("ALTER TABLE match_blocks ADD COLUMN process_group_id INTEGER")
                    connection.execute("ALTER TABLE match_blocks ADD COLUMN attempt INTEGER NOT NULL DEFAULT 0")
                    connection.execute("ALTER TABLE match_blocks ADD COLUMN run_dir TEXT")
                    connection.execute("ALTER TABLE match_blocks ADD COLUMN command_json TEXT")
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
        row = connection.execute("SELECT * FROM campaigns WHERE campaign_id=?", (campaign_id,)).fetchone()
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

    def create_trial(self, campaign_id: str, parameter_set_id: str, algorithm: str, seed: int) -> str:
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
                "SELECT trial_id FROM trials WHERE campaign_id=? AND parameter_set_id=?",
                (campaign_id, parameter_set_id),
            ).fetchone()
            if existing is not None:
                return str(existing["trial_id"])
            number = connection.execute(
                "SELECT COUNT(*) FROM trials WHERE campaign_id=?", (campaign_id,)
            ).fetchone()[0] + 1
            trial_id = f"trial-{number:06d}"
            connection.execute(
                "INSERT INTO trials(trial_id,campaign_id,parameter_set_id,status,algorithm,seed,created_at,updated_at) "
                "VALUES(?,?,?,?,?,?,?,?)",
                (trial_id, campaign_id, parameter_set_id, "pending", algorithm, seed, now, now),
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
                "SELECT revision FROM optimizer_state WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if state_row is None:
                raise DatabaseError(f"optimizer state is missing for campaign: {campaign_id}")
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

    def claim_next_block(self, campaign_id: str) -> dict[str, Any] | None:
        """Reserve the first missing block, enforcing one running block."""
        with self._transaction() as connection:
            campaign = self._campaign(connection, campaign_id)
            if campaign["status"] != "running":
                return None
            running = connection.execute(
                "SELECT COUNT(*) FROM match_blocks WHERE campaign_id=? AND status='running'", (campaign_id,)
            ).fetchone()[0]
            if running:
                raise CampaignBusy(f"campaign {campaign_id} already has a running match block")
            row = connection.execute(
                "SELECT * FROM match_blocks WHERE campaign_id=? AND status IN ('pending','interrupted') "
                "ORDER BY block_index,created_at,block_id LIMIT 1",
                (campaign_id,),
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

    def checkpoint(self, campaign_id: str, state: dict[str, Any], event_type: str = "checkpoint") -> tuple[int, str]:
        if not isinstance(state, dict):
            raise DatabaseError("checkpoint state must be an object")
        with self._transaction() as connection:
            self._campaign(connection, campaign_id)
            row = connection.execute(
                "SELECT revision FROM optimizer_state WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if row is None:
                raise DatabaseError(f"optimizer state is missing for campaign: {campaign_id}")
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
                "SELECT revision FROM optimizer_state WHERE campaign_id=?", (campaign_id,)
            ).fetchone()
            if state_row is None:
                raise DatabaseError(f"optimizer state is missing for campaign: {campaign_id}")
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
                "SELECT revision,checkpoint_hash,updated_at FROM optimizer_state WHERE campaign_id=?",
                (campaign_id,),
            ).fetchone()
            event_count = connection.execute(
                "SELECT COUNT(*) AS count FROM events WHERE campaign_id=?", (campaign_id,)
            ).fetchone()["count"]
            return {
                "campaign_id": campaign["campaign_id"],
                "name": campaign["name"],
                "status": campaign["status"],
                "config_hash": campaign["config_hash"],
                "baseline_parameter_hash": campaign["baseline_parameter_hash"],
                "master_seed": campaign["master_seed"],
                "created_at": campaign["created_at"],
                "updated_at": campaign["updated_at"],
                "finished_at": campaign["finished_at"],
                "trials": {row["status"]: row["count"] for row in trial_rows},
                "blocks": {row["status"]: row["count"] for row in block_rows},
                "games": int(games),
                "checkpoint": dict(checkpoint) if checkpoint else None,
                "event_count": int(event_count),
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
