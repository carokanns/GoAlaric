from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.dashboard import DashboardReader
from goalaric_optimizer.database import Database, DatabaseError
from goalaric_optimizer.real_integration import RealTestmonitorConfig, RealTestmonitorScheduler, SchedulerError
from goalaric_optimizer.service import init_campaign


class LiveBlockProgressTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-live-progress-")
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        registry = self.root / "registry.json"
        registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "live-progress-v1",
                    "parameters": [{"name": "p", "value": 1}],
                }
            ),
            encoding="utf-8",
        )
        campaign = self.root / "campaign.json"
        campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "live-progress",
                    "name": "Live progress",
                    "mode": "fake",
                    "registry": str(registry),
                    "baseline": {"engine_id": "fake"},
                    "master_seed": 1,
                    "partitions": {"fake": {"name": "fake"}},
                }
            ),
            encoding="utf-8",
        )
        init_campaign(campaign, self.data_dir)
        self.database = Database(self.data_dir / "live-progress" / "campaign.db")
        self.trial_id, self.block_ids = self.database.ensure_fake_schedule("live-progress", 1, 3)
        self.database.transition_campaign("live-progress", "running")
        self.database.transition_trial("live-progress", self.trial_id, "running")
        self.block = self.database.claim_next_block("live-progress")
        assert self.block is not None

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def test_progress_is_idempotent_visible_and_reset_on_retry(self) -> None:
        block_id = str(self.block["block_id"])
        first = self.database.update_running_block_progress("live-progress", block_id, 1, 2, 1)
        second = self.database.update_running_block_progress("live-progress", block_id, 1, 2, 1)
        self.assertEqual((first["wins"], first["draws"], first["losses"]), (1, 2, 1))
        self.assertEqual((second["wins"], second["draws"], second["losses"]), (1, 2, 1))

        snapshot = DashboardReader(self.data_dir, "live-progress").snapshot()
        self.assertEqual(snapshot["current_trial"]["metrics"]["games"], 4)
        self.assertEqual(snapshot["current_trial"]["metrics"]["score_percent"], 50.0)
        self.assertEqual(snapshot["search_games"], 4)
        self.assertIsNotNone(snapshot["current_trial"]["elapsed_seconds"])
        self.assertIsNotNone(snapshot["current_trial"]["estimated_remaining_seconds"])

        with self.assertRaises(DatabaseError):
            self.database.update_running_block_progress("live-progress", block_id, 1, 1, 1)
        with self.assertRaises(DatabaseError):
            self.database.update_running_block_progress("live-progress", block_id, 7, 0, 0)

        self.database.interrupt_block("live-progress", block_id, "test retry")
        retried = self.database.claim_next_block("live-progress", block_id)
        assert retried is not None
        self.assertEqual((retried["wins"], retried["draws"], retried["losses"], retried["score"]), (0, 0, 0, None))
        self.assertEqual(retried["attempt"], 2)
        self.assertEqual(DashboardReader(self.data_dir, "live-progress").snapshot()["search_games"], 0)

    def test_real_scheduler_transports_status_json_into_sqlite(self) -> None:
        run_dir = self.root / "run"
        run_dir.mkdir()
        (run_dir / "status.json").write_text(
            json.dumps(
                {
                    "state": "running",
                    "target_games": 6,
                    "games": 4,
                    "wins": 2,
                    "draws": 1,
                    "losses": 1,
                }
            ),
            encoding="utf-8",
        )
        config = RealTestmonitorConfig(
            testmonitor_command=("testmonitor",),
            fastchess=Path("fastchess"),
            baseline=Path("engine"),
            candidate=Path("engine"),
            baseline_parameter_file=Path("baseline.json"),
            candidate_parameter_file=Path("candidate.json"),
            opening_book=Path("book.pgn"),
            opening_block_file=Path("block.epd"),
            profile_mode="nodes",
            tc=None,
            nodes=100000,
        )
        scheduler = RealTestmonitorScheduler(self.data_dir, "live-progress", config)
        scheduler._poll_progress(self.database, self.block, run_dir)
        row = self.database.running_block_processes("live-progress")[0]
        self.assertEqual((row["wins"], row["draws"], row["losses"]), (2, 1, 1))
        self.assertEqual(DashboardReader(self.data_dir, "live-progress").snapshot()["search_games"], 4)

    def test_real_report_accepts_one_decimal_score_rounding(self) -> None:
        run_dir = self.root / "rounded-report"
        run_dir.mkdir()
        opening_block = run_dir / "openings.epd"
        opening_block.write_text("startpos\n", encoding="utf-8")
        baseline_file = run_dir / "baseline.json"
        candidate_file = run_dir / "candidate.json"
        baseline_file.write_text(json.dumps({"parameters": [{"name": "p", "value": 1}]}), encoding="utf-8")
        candidate_file.write_text(json.dumps({"parameters": [{"name": "p", "value": 2}]}), encoding="utf-8")
        config = RealTestmonitorConfig(
            testmonitor_command=("testmonitor",),
            fastchess=Path("fastchess"),
            baseline=Path("engine"),
            candidate=Path("engine"),
            baseline_parameter_file=baseline_file,
            candidate_parameter_file=candidate_file,
            opening_book=run_dir / "book.pgn",
            opening_block_file=opening_block,
            profile_mode="nodes",
            tc=None,
            nodes=100000,
            profile_name="node-search",
            profile_hash="profile-hash",
        )
        # 507-977-516 = 49.775%, reported by testmonitor as 49.8%.
        report = {
            "schema_version": 1,
            "state": "completed",
            "valid": True,
            "counted": True,
            "run_dir": str(run_dir),
            "opening_block_size": 1000,
            "target_games": 2000,
            "games": 2000,
            "wins": 507,
            "draws": 977,
            "losses": 516,
            "score_percent": 49.8,
            "node_budget": 100000,
            "opening_block_sha256": __import__("hashlib").sha256(opening_block.read_bytes()).hexdigest(),
            "color_swap": True,
        }
        status = {
            "state": "completed",
            "games": 2000,
            "wins": 507,
            "draws": 977,
            "losses": 516,
            "baseline_identity": {"sha256": "engine", "parameter_sha256": "baseline", "parameter_register_version": 1},
            "candidate_identity": {"sha256": "engine", "parameter_sha256": "candidate", "parameter_register_version": 1},
        }
        monitor_config = {
            "baseline_parameter_file": str(baseline_file),
            "candidate_parameter_file": str(candidate_file),
            "optimizer_mode": True,
            "nodes": 100000,
            "time_control": None,
        }
        (run_dir / "block-report.json").write_text(json.dumps(report), encoding="utf-8")
        (run_dir / "status.json").write_text(json.dumps(status), encoding="utf-8")
        (run_dir / "monitor-config.json").write_text(json.dumps(monitor_config), encoding="utf-8")
        scheduler = RealTestmonitorScheduler(self.data_dir, "live-progress", config)
        result = scheduler._read_result(run_dir / "result.json", 1000)
        self.assertEqual((result["wins"], result["draws"], result["losses"]), (507, 977, 516))

        report["score_percent"] = 49.7
        (run_dir / "block-report.json").write_text(json.dumps(report), encoding="utf-8")
        with self.assertRaises(SchedulerError):
            scheduler._read_result(run_dir / "result.json", 1000)


if __name__ == "__main__":
    unittest.main()
