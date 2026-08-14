from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.cli import main
from goalaric_optimizer.canonical import sha256_json
from goalaric_optimizer.database import Database
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
