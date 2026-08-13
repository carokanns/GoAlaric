from __future__ import annotations

import json
import sys
import tempfile
import textwrap
import threading
import time
import unittest
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.dashboard import DashboardError, DashboardReader, create_dashboard_server, final_report
from goalaric_optimizer.database import Database
from goalaric_optimizer.scheduler import Scheduler
from goalaric_optimizer.service import init_campaign


class Phase10Test(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        registry = self.root / "registry.json"
        registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "dashboard-registry-v1",
                    "parameters": [{"name": "a", "value": 1}],
                }
            ),
            encoding="utf-8",
        )
        campaign = self.root / "campaign.json"
        campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "dashboard-phase10",
                    "name": "Dashboard phase 10",
                    "mode": "fake",
                    "registry": str(registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 20261010,
                    "partitions": {"training": {"name": "training"}},
                }
            ),
            encoding="utf-8",
        )
        init_campaign(campaign, self.data_dir)
        self.database = Database(self.data_dir / "dashboard-phase10" / "campaign.db")
        self.monitor = self.root / "monitor.py"
        self.monitor.write_text(
            textwrap.dedent(
                """
                import argparse
                import json
                import time

                parser = argparse.ArgumentParser()
                parser.add_argument("--result-path", required=True)
                parser.add_argument("--pairs-per-block", type=int, required=True)
                args, _ = parser.parse_known_args()
                time.sleep(0.45)
                with open(args.result_path, "w", encoding="utf-8") as stream:
                    json.dump({"wins": 1, "draws": 1, "losses": 0, "score": 75.0, "games": ["1-0", "1/2-1/2"]}, stream)
                """
            ),
            encoding="utf-8",
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def test_dashboard_is_read_only_and_can_stop_during_scheduler(self) -> None:
        self.database.ensure_fake_schedule("dashboard-phase10", block_count=1, pairs_per_block=1)
        reader = DashboardReader(self.data_dir, "dashboard-phase10")
        mtime_before = reader.database_path.stat().st_mtime_ns
        reader.snapshot()
        self.assertEqual(reader.database_path.stat().st_mtime_ns, mtime_before)
        scheduler_errors: list[BaseException] = []

        def run_scheduler() -> None:
            try:
                Scheduler(
                    self.data_dir,
                    "dashboard-phase10",
                    [sys.executable, str(self.monitor)],
                    poll_interval=0.01,
                    stop_grace_seconds=0.1,
                ).run()
            except BaseException as exc:
                scheduler_errors.append(exc)

        scheduler_thread = threading.Thread(target=run_scheduler)
        scheduler_thread.start()
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline and not self.database.running_block_processes("dashboard-phase10"):
            time.sleep(0.01)
        self.assertTrue(self.database.running_block_processes("dashboard-phase10"))

        server = create_dashboard_server(self.data_dir, "dashboard-phase10", "127.0.0.1:0", refresh_ms=300)
        server_thread = threading.Thread(target=server.serve_forever)
        server_thread.start()
        try:
            host, port = server.server_address[:2]
            with urllib.request.urlopen(f"http://{host}:{port}/api/dashboard", timeout=2) as response:
                payload = json.load(response)
            self.assertTrue(payload["read_only"])
            self.assertEqual(payload["campaign"]["campaign_id"], "dashboard-phase10")
        finally:
            server.shutdown()
            server.server_close()
            server_thread.join(timeout=3)

        self.assertFalse(server_thread.is_alive())
        scheduler_thread.join(timeout=5)
        self.assertFalse(scheduler_thread.is_alive())
        self.assertFalse(scheduler_errors)
        self.assertEqual(self.database.status_snapshot("dashboard-phase10")["status"], "completed")
        self.assertEqual(self.database.status_snapshot("dashboard-phase10")["games"], 2)
        mtime_after_scheduler = reader.database_path.stat().st_mtime_ns
        reader.snapshot()
        self.assertEqual(reader.database_path.stat().st_mtime_ns, mtime_after_scheduler)

    def test_final_report_is_available_only_after_completion_and_has_both_formats(self) -> None:
        reader = DashboardReader(self.data_dir, "dashboard-phase10")
        with self.assertRaises(DashboardError):
            final_report(self.data_dir, "dashboard-phase10", "json")
        with self.database._transaction() as connection:
            connection.execute(
                "UPDATE campaigns SET status='completed',finished_at=updated_at WHERE campaign_id=?",
                ("dashboard-phase10",),
            )
        snapshot, json_report = final_report(self.data_dir, "dashboard-phase10", "json")
        _, html_report = final_report(self.data_dir, "dashboard-phase10", "html")
        self.assertTrue(snapshot["campaign"]["finished"])
        self.assertIn('"read_only": true', json_report)
        self.assertIn("GoAlaric optimizer report", html_report)
        self.assertEqual(reader.snapshot()["consumed_games"], 0)


if __name__ == "__main__":
    unittest.main()
