from __future__ import annotations

import json
import os
import signal
import shutil
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.cli import main
from goalaric_optimizer.canonical import sha256_json
from goalaric_optimizer.database import Database
from goalaric_optimizer.dashboard import DashboardReader
from goalaric_optimizer.registry import load_registry
from goalaric_optimizer.service import init_campaign


class ConfirmationFakeOutcomesTest(unittest.TestCase):
    def _campaign(self, root: Path, campaign_id: str, result: dict[str, int], master_seed: int) -> tuple[Path, Path]:
        registry = root / f"{campaign_id}-registry.json"
        registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "confirmation-fake-registry-v1",
                    "parameters": [{"name": "p", "value": 0, "min": 0, "max": 2, "step": 1, "min_step": 1}],
                }
            ),
            encoding="utf-8",
        )
        campaign = root / f"{campaign_id}.json"
        campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": campaign_id,
                    "name": campaign_id,
                    "mode": "fake",
                    "registry": str(registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": master_seed,
                    "partitions": {"optimization": {"name": "optimization"}},
                    "goals": {
                        "max_games": 8,
                        "max_evaluations": 3,
                        "max_passes": 2,
                        "optimizer": {"parameters": ["p"]},
                        "adaptive": {"min_blocks": 1, "max_blocks": 1},
                        "fake_match": {"optimum": {"p": 2}},
                        "confirmation": {
                            "enabled": True,
                            "games": 20,
                            "seed": master_seed + 100,
                            "confidence": 0.95,
                            "fake_result": result,
                        },
                    },
                }
            ),
            encoding="utf-8",
        )
        return campaign, registry

    def test_all_three_outcomes_are_automatic_and_non_promoting(self) -> None:
        cases = {
            "confirmed": {"wins": 18, "draws": 1, "losses": 1},
            "rejected": {"wins": 1, "draws": 1, "losses": 18},
            "inconclusive": {"wins": 10, "draws": 0, "losses": 10},
        }
        with tempfile.TemporaryDirectory(prefix="goalaric-confirmation-fake-") as temp:
            root = Path(temp)
            data_dir = root / "campaigns"
            for index, (expected, result) in enumerate(cases.items()):
                campaign, _ = self._campaign(root, f"confirmation-{expected}", result, 5000 + index)
                self.assertEqual(main(["optimize", str(campaign), "--data-dir", str(data_dir)]), 0)
                database = Database(data_dir / f"confirmation-{expected}" / "campaign.db")
                report = database.confirmation_snapshot(f"confirmation-{expected}")
                self.assertIsNotNone(report)
                assert report is not None
                self.assertEqual(report["status"], "completed")
                self.assertEqual(report["outcome"], expected)
                self.assertEqual(report["games_target"], 20)
                self.assertEqual(report["result"]["automatic_promotion"], False)
                self.assertEqual(
                    report["recommendation_parameter_hash"],
                    report["candidate_parameter_hash"] if expected == "confirmed" else None,
                )
                self.assertNotEqual(report["candidate_parameter_hash"], report["baseline_parameter_hash"])
                self.assertEqual(
                    report["recommendation"], "candidate" if expected == "confirmed" else None
                )
                self.assertEqual(
                    "recommendation_parameter_file" in report,
                    expected == "confirmed",
                )
                self.assertEqual(len(report["blocks"]), 10)
                self.assertEqual(sum(block["status"] == "completed" for block in report["blocks"]), 10)
                with database._read() as connection:
                    self.assertEqual(
                        connection.execute(
                            "SELECT COUNT(*) FROM confirmation_games WHERE confirmation_id=?",
                            (report["confirmation_id"],),
                        ).fetchone()[0],
                        20,
                    )
                    self.assertEqual(
                        connection.execute("SELECT COUNT(*) FROM games").fetchone()[0],
                        6,
                    )

    def test_confirmation_resumes_without_duplicate_blocks_or_search_feedback(self) -> None:
        with tempfile.TemporaryDirectory(prefix="goalaric-confirmation-resume-") as temp:
            root = Path(temp)
            data_dir = root / "campaigns"
            campaign, _ = self._campaign(
                root,
                "confirmation-resume",
                {"wins": 10, "draws": 0, "losses": 10},
                9000,
            )
            self.assertEqual(
                main(["optimize", str(campaign), "--data-dir", str(data_dir), "--max-results", "3"]),
                0,
            )
            database = Database(data_dir / "confirmation-resume" / "campaign.db")
            self.assertIsNone(database.confirmation("confirmation-resume"))
            self.assertEqual(
                main(["optimize", str(campaign), "--data-dir", str(data_dir), "--max-results", "2"]),
                0,
            )
            pending = database.confirmation_snapshot("confirmation-resume")
            self.assertIsNotNone(pending)
            assert pending is not None
            self.assertEqual(pending["status"], "running")
            self.assertEqual(sum(block["status"] == "completed" for block in pending["blocks"]), 2)
            checkpoint_before = database.optimizer_state("confirmation-resume")

            self.assertEqual(main(["optimize", str(campaign), "--data-dir", str(data_dir)]), 0)
            finished = database.confirmation_snapshot("confirmation-resume")
            self.assertIsNotNone(finished)
            assert finished is not None
            self.assertEqual(finished["status"], "completed")
            self.assertEqual(finished["outcome"], "inconclusive")
            self.assertEqual(len(finished["blocks"]), 10)
            self.assertEqual(
                database.optimizer_state("confirmation-resume")["revision"],
                checkpoint_before["revision"],
            )
            self.assertEqual(
                main(["optimize", str(campaign), "--data-dir", str(data_dir)]),
                0,
            )
            with database._read() as connection:
                self.assertEqual(connection.execute("SELECT COUNT(*) FROM confirmation_blocks").fetchone()[0], 10)
                self.assertEqual(connection.execute("SELECT COUNT(*) FROM confirmation_games").fetchone()[0], 20)

    def test_live_confirmation_metrics_are_read_from_completed_blocks(self) -> None:
        """The read-only views must expose partial confirmation progress."""
        with tempfile.TemporaryDirectory(prefix="goalaric-confirmation-dashboard-") as temp:
            root = Path(temp)
            data_dir = root / "campaigns"
            campaign, _ = self._campaign(
                root,
                "confirmation-dashboard",
                {"wins": 10, "draws": 0, "losses": 10},
                9101,
            )
            self.assertEqual(
                main(["optimize", str(campaign), "--data-dir", str(data_dir), "--max-results", "3"]),
                0,
            )
            self.assertEqual(
                main(["optimize", str(campaign), "--data-dir", str(data_dir), "--max-results", "2"]),
                0,
            )

            database = Database(data_dir / "confirmation-dashboard" / "campaign.db")
            confirmation = database.confirmation_snapshot("confirmation-dashboard")
            self.assertIsNotNone(confirmation)
            assert confirmation is not None
            self.assertEqual(confirmation["status"], "running")
            self.assertEqual(confirmation["blocks_completed"], 2)
            self.assertEqual(confirmation["pairs_completed"], 2)
            self.assertEqual(confirmation["pairs_target"], 10)
            self.assertEqual(confirmation["games"], 4)
            self.assertEqual(confirmation["wins"], 4)
            self.assertEqual(confirmation["draws"], 0)
            self.assertEqual(confirmation["losses"], 0)
            self.assertEqual(confirmation["score_percent"], 100.0)
            self.assertEqual(confirmation["metrics"]["games"], 4)

            status = database.status_snapshot("confirmation-dashboard")
            self.assertEqual(status["status"], "confirming")
            self.assertEqual(status["raw_status"], "running")
            self.assertEqual(status["confirmation"]["games"], 4)
            self.assertEqual(status["confirmation"]["wins"], 4)

            dashboard = DashboardReader(data_dir, "confirmation-dashboard")
            first = dashboard.snapshot()
            second = dashboard.snapshot()
            self.assertTrue(first["read_only"])
            self.assertEqual(first["campaign"]["status"], "confirming")
            self.assertFalse(first["campaign"]["finished"])
            self.assertEqual(first["confirmation"]["status"], "running")
            self.assertEqual(first["confirmation"]["metrics"]["games"], 4)
            self.assertEqual(first["confirmation"]["metrics"], second["confirmation"]["metrics"])
            self.assertEqual(first["confirmation"]["candidate_values"]["p"], 2)
            self.assertEqual(first["confirmation"]["baseline_values"]["p"], 0)
            self.assertEqual(first["confirmation"]["parameter_differences"][0]["delta"], 2)

            # A v1.1.1 database is read directly; no schema migration or
            # match-file result is needed to display the live confirmation.
            with database._read() as connection:
                self.assertEqual(connection.execute("PRAGMA query_only").fetchone()[0], 1)
                self.assertEqual(
                    connection.execute("SELECT COUNT(*) FROM confirmation_games").fetchone()[0], 4
                )

            environment = os.environ.copy()
            source_path = str(Path(__file__).parents[1] / "src")
            environment["PYTHONPATH"] = os.pathsep.join(
                item for item in (source_path, environment.get("PYTHONPATH")) if item
            )
            watcher = subprocess.Popen(
                [
                    sys.executable,
                    "-u",
                    "-m",
                    "goalaric_optimizer",
                    "status",
                    "confirmation-dashboard",
                    "--data-dir",
                    str(data_dir),
                    "--watch",
                    "--interval",
                    "0.01",
                ],
                env=environment,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
            try:
                assert watcher.stdout is not None
                self.assertEqual(watcher.stdout.readline().strip(), "{")
                watcher.send_signal(signal.SIGINT)
                _, stderr = watcher.communicate(timeout=5)
            finally:
                if watcher.poll() is None:
                    watcher.kill()
                    watcher.wait(timeout=5)
            self.assertEqual(watcher.returncode, 0)
            self.assertNotIn("Traceback", stderr)


class ConfirmationMinimalRealTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.repo_root = Path(__file__).parents[2].resolve()
        cls.go = shutil.which("go")
        cls.fastchess = cls.repo_root.parent / "GoAlaric" / ".tools" / "fastchess" / "bin" / "fastchess"
        if cls.go is None or not cls.fastchess.exists():
            raise unittest.SkipTest("Go and Fastchess are required for the real confirmation test")
        cls.build_dir = Path(tempfile.mkdtemp(prefix="goalaric-confirmation-real-build-"))
        cls.testmonitor = cls.build_dir / "testmonitor"
        cls.engine = cls.build_dir / "goalaric"
        subprocess.run([cls.go, "build", "-o", str(cls.testmonitor), "./cmd/testmonitor"], cwd=cls.repo_root, check=True)
        subprocess.run([cls.go, "build", "-o", str(cls.engine), "."], cwd=cls.repo_root, check=True)

    @classmethod
    def tearDownClass(cls) -> None:
        shutil.rmtree(cls.build_dir, ignore_errors=True)

    def test_tiny_real_confirmation_uses_new_openings_and_restarts_idempotently(self) -> None:
        with tempfile.TemporaryDirectory(prefix="goalaric-confirmation-real-") as temp:
            root = Path(temp)
            data_dir = root / "campaigns"
            registry_document = json.loads(
                (self.repo_root / "optimizer" / "registries" / "eval-pilot-v1-default.json").read_text()
            )
            registry_document["parameters"][0].update({"min": 0, "max": 64, "step": 1, "min_step": 1})
            registry = root / "registry.json"
            registry.write_text(json.dumps(registry_document), encoding="utf-8")
            opening_book = root / "openings.epd"
            opening_book.write_text(
                "".join(f"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1 id c-{i}\n" for i in range(120)),
                encoding="utf-8",
            )
            campaign = root / "campaign.json"
            campaign.write_text(
                json.dumps(
                    {
                        "schema_version": 1,
                        "campaign_id": "confirmation-real",
                        "name": "Tiny real confirmation",
                        "mode": "real",
                        "registry": str(registry),
                        "baseline": {"engine_id": str(self.engine)},
                        "master_seed": 20260814,
                        "partitions": {"optimization": {"name": "optimization"}},
                        "goals": {
                            "max_games": 6,
                            "max_evaluations": 4,
                            "max_passes": 1,
                            "optimizer": {"parameters": ["mobility_weight"]},
                            "adaptive": {"min_blocks": 1, "max_blocks": 1, "weak_upper_score": 0.0, "target_score": 0.0},
                            "real": {
                                "testmonitor_command": [str(self.testmonitor)],
                                "fastchess": str(self.fastchess),
                                "opening_book": str(opening_book),
                                "tc": "0.1+0.01",
                                "hash_mb": 16,
                                "threads": 1,
                                "workdir": str(self.repo_root),
                            },
                            "confirmation": {
                                "enabled": True,
                                "games": 4,
                                "seed": 20260830,
                                "confidence": 0.95,
                            },
                        },
                    }
                ),
                encoding="utf-8",
            )
            init_campaign(campaign, data_dir)
            database = Database(data_dir / "confirmation-real" / "campaign.db")
            baseline = database.parameter_set_by_hash("confirmation-real", database.campaign("confirmation-real")["baseline_parameter_hash"])
            assert baseline is not None
            candidate = json.loads(json.dumps(baseline["document"]))
            candidate["parameters"][0]["value"] = 19
            candidate_hash = sha256_json(candidate)
            state = database.optimizer_state("confirmation-real")["state"]
            state.update(
                {
                    "algorithm": "coordinate-multires-v1",
                    "phase": "completed",
                    "registry_sha256": load_registry(registry).sha256,
                    "parameter_names": ["mobility_weight"],
                    "max_passes": 1,
                    "anchor_parameters": candidate,
                    "anchor_hash": candidate_hash,
                    "coordinate_base_parameters": candidate,
                    "coordinate_base_hash": candidate_hash,
                    "result_count": 1,
                    "stop_reason": "test_search_completed",
                }
            )
            database.checkpoint("confirmation-real", state, event_type="test_search_completed")
            database.transition_campaign("confirmation-real", "completed", "test search completed")
            self.assertEqual(main(["optimize", str(campaign), "--data-dir", str(data_dir)]), 0)
            report = database.confirmation_snapshot("confirmation-real")
            self.assertIsNotNone(report)
            assert report is not None
            self.assertEqual(report["status"], "completed")
            self.assertEqual(sum(block["wins"] + block["draws"] + block["losses"] for block in report["blocks"]), 4)
            with database._read() as connection:
                optimization_hashes = {
                    row["materialized_openings_sha256"]
                    for row in connection.execute(
                        "SELECT materialized_openings_sha256 FROM match_blocks"
                    ).fetchall()
                }
                confirmation_hashes = {
                    row["materialized_openings_sha256"]
                    for row in connection.execute(
                        "SELECT materialized_openings_sha256 FROM confirmation_blocks"
                    ).fetchall()
                }
                self.assertTrue(confirmation_hashes.isdisjoint(optimization_hashes))
                self.assertEqual(connection.execute("SELECT COUNT(*) FROM confirmation_games").fetchone()[0], 4)
            self.assertEqual(main(["optimize", str(campaign), "--data-dir", str(data_dir)]), 0)
            with database._read() as connection:
                self.assertEqual(connection.execute("SELECT COUNT(*) FROM confirmation_games").fetchone()[0], 4)


if __name__ == "__main__":
    unittest.main()
