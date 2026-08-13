from __future__ import annotations

import json
import sqlite3
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.cli import main
from goalaric_optimizer.config import load_campaign_definition
from goalaric_optimizer.database import CampaignBusy, Database, InvalidTransition
from goalaric_optimizer.service import init_campaign


class Phase5Test(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.registry = self.root / "registry.json"
        self.registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "fake-registry-v1",
                    "parameters": [
                        {"name": "a", "value": 1},
                        {"name": "b", "value": 2},
                    ],
                }
            ),
            encoding="utf-8",
        )
        self.campaign_file = self.root / "campaign.json"
        self.campaign_file.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "fake-phase5",
                    "name": "Fake phase 5",
                    "mode": "fake",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 42,
                    "partitions": {"training": {"name": "training"}},
                    "goals": {"max_trials": 2},
                }
            ),
            encoding="utf-8",
        )
        self.data_dir = self.root / "campaigns"

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def init(self) -> Database:
        definition, created, db_path = init_campaign(self.campaign_file, self.data_dir)
        self.assertTrue(created)
        self.assertEqual(definition.campaign_id, "fake-phase5")
        return Database(db_path)

    def test_init_wal_baseline_and_idempotent_history(self) -> None:
        database = self.init()
        self.assertEqual(database.journal_mode(), "wal")
        baseline = self.data_dir / "fake-phase5" / "baseline-parameters.json"
        self.assertTrue(baseline.exists())
        first_events = len(database.events("fake-phase5"))

        _, created, _ = init_campaign(self.campaign_file, self.data_dir)
        self.assertFalse(created)
        self.assertEqual(len(database.events("fake-phase5")), first_events)
        self.assertEqual(database.status_snapshot("fake-phase5")["status"], "pending")

    def test_fake_cli_lifecycle_and_read_only_status(self) -> None:
        self.init()
        data = ["--data-dir", str(self.data_dir)]
        self.assertEqual(main(["run", "fake-phase5", *data]), 0)
        self.assertEqual(main(["pause", "fake-phase5", *data]), 0)
        self.assertEqual(main(["resume", "fake-phase5", *data]), 0)
        database = Database(self.data_dir / "fake-phase5" / "campaign.db")
        before = len(database.events("fake-phase5"))
        self.assertEqual(main(["status", "fake-phase5", *data]), 0)
        self.assertEqual(main(["status", "fake-phase5", "--watch", "--iterations", "1", *data]), 0)
        self.assertEqual(len(database.events("fake-phase5")), before)
        self.assertEqual(main(["stop", "fake-phase5", *data]), 0)
        snapshot = database.status_snapshot("fake-phase5")
        self.assertEqual(snapshot["status"], "interrupted")

    def test_config_change_is_rejected(self) -> None:
        self.init()
        changed = json.loads(self.campaign_file.read_text(encoding="utf-8"))
        changed["master_seed"] = 43
        self.campaign_file.write_text(json.dumps(changed), encoding="utf-8")
        with self.assertRaises(Exception):
            init_campaign(self.campaign_file, self.data_dir)

    def test_parameter_trials_blocks_and_safe_transitions(self) -> None:
        database = self.init()
        definition = load_campaign_definition(self.campaign_file)
        parameter_set = database.add_parameter_set("fake-phase5", definition.baseline_parameters)
        self.assertEqual(parameter_set, database.add_parameter_set("fake-phase5", definition.baseline_parameters))
        trial = database.create_trial("fake-phase5", parameter_set, "fake", 42)
        self.assertEqual(trial, database.create_trial("fake-phase5", parameter_set, "fake", 42))
        block = database.create_match_block("fake-phase5", trial, "training", 0, 2, 42, "book", "openings")
        self.assertEqual(block, database.create_match_block("fake-phase5", trial, "training", 0, 2, 42, "book", "openings"))
        database.transition_campaign("fake-phase5", "running")
        database.transition_trial("fake-phase5", trial, "running")
        database.transition_block("fake-phase5", block, "running")
        with self.assertRaises(InvalidTransition):
            database.transition_block("fake-phase5", block, "pending")
        completed, checkpoint = database.complete_block_atomically(
            "fake-phase5",
            block,
            1,
            1,
            0,
            75.0,
            {"source": "fake-fastchess"},
            {"next_block": 1, "last_block": block},
        )
        self.assertEqual(completed["status"], "completed")
        self.assertEqual(checkpoint[0], 1)
        self.assertEqual(database.optimizer_state("fake-phase5")["revision"], 1)

    def test_atomic_completion_failure_leaves_checkpoint_and_block_intact(self) -> None:
        database = self.init()
        definition = load_campaign_definition(self.campaign_file)
        parameter_set = database.add_parameter_set("fake-phase5", definition.baseline_parameters)
        trial = database.create_trial("fake-phase5", parameter_set, "fake", 42)
        block = database.create_match_block("fake-phase5", trial, "training", 0, 2, 42, "book", "openings")
        before = database.optimizer_state("fake-phase5")
        with self.assertRaises(InvalidTransition):
            database.complete_block_atomically(
                "fake-phase5", block, 0, 2, 0, 50.0, {}, {"next_block": 1}
            )
        self.assertEqual(database.optimizer_state("fake-phase5"), before)
        with sqlite3.connect(database.path) as connection:
            self.assertEqual(connection.execute("SELECT status FROM match_blocks WHERE block_id=?", (block,)).fetchone()[0], "pending")

    def test_running_jobs_are_recovered_once_as_interrupted(self) -> None:
        database = self.init()
        definition = load_campaign_definition(self.campaign_file)
        parameter_set = database.add_parameter_set("fake-phase5", definition.baseline_parameters)
        trial = database.create_trial("fake-phase5", parameter_set, "fake", 42)
        block = database.create_match_block("fake-phase5", trial, "training", 0, 2, 42, "book", "openings")
        database.transition_trial("fake-phase5", trial, "running")
        database.transition_block("fake-phase5", block, "running")
        recovered = database.recover_abandoned_jobs("fake-phase5")
        self.assertEqual(recovered, {"trials": 1, "blocks": 1})
        event_count = len(database.events("fake-phase5"))
        self.assertEqual(database.recover_abandoned_jobs("fake-phase5"), {"trials": 0, "blocks": 0})
        self.assertEqual(len(database.events("fake-phase5")), event_count)
        self.assertEqual(database.list_trials("fake-phase5")[0]["status"], "interrupted")

    def test_campaign_ownership_rejects_second_owner(self) -> None:
        database = self.init()
        database.claim_campaign("fake-phase5", "owner-a")
        with self.assertRaises(CampaignBusy):
            database.claim_campaign("fake-phase5", "owner-b")
        database.release_campaign("fake-phase5", "owner-a")


if __name__ == "__main__":
    unittest.main()
