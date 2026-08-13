from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.adaptive import (
    AdaptiveCampaign,
    AdaptiveCoordinateEvaluator,
    AdaptivePolicy,
    FakeBlockRunner,
    run_real_adaptive_campaign,
)
from goalaric_optimizer.coordinate import CoordinateSearch
from goalaric_optimizer.database import Database
from goalaric_optimizer.registry import load_registry
from goalaric_optimizer.registry import default_parameter_document, load_parameter_file
from goalaric_optimizer.real_integration import RealTestmonitorConfig
from goalaric_optimizer.service import init_campaign


class Phase9Test(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry_path = self.root / "registry.json"
        self.registry_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "adaptive-registry-v1",
                    "parameters": [{"name": "a", "value": 1, "min": 0, "max": 4, "step": 1}],
                }
            ),
            encoding="utf-8",
        )
        self.campaign_path = self.root / "campaign.json"
        self.campaign_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "fake-phase9",
                    "name": "Fake phase 9",
                    "mode": "fake",
                    "registry": str(self.registry_path),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 9001,
                    "partitions": {"training": {"name": "training"}},
                }
            ),
            encoding="utf-8",
        )
        init_campaign(self.campaign_path, self.data_dir)
        self.database = Database(self.data_dir / "fake-phase9" / "campaign.db")
        self.registry = load_registry(self.registry_path)

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    @staticmethod
    def _candidate(value: int) -> dict[str, object]:
        return {"schema_version": 1, "registry": "adaptive-registry-v1", "parameters": [{"name": "a", "value": value}]}

    def test_clearly_weak_candidate_stops_after_first_complete_pair(self) -> None:
        candidate = self._candidate(2)
        controller = AdaptiveCampaign(
            self.database,
            "fake-phase9",
            candidate,
            AdaptivePolicy(min_blocks=1, max_blocks=3, weak_upper_score=45.0),
            FakeBlockRunner(self.database, "fake-phase9", [{"wins": 0, "draws": 0, "losses": 2}]),
            seed=9001,
        )
        result = controller.run()
        self.assertEqual(result["decision"], "reject_early")
        self.assertEqual(result["statistics"]["games"], 2)
        self.assertLess(result["statistics"]["elo_ci_high"], -1500.0)
        trial = self.database.list_trials("fake-phase9", 10)[0]
        self.assertEqual(trial["status"], "rejected")
        with self.database._read() as connection:
            statuses = connection.execute(
                "SELECT status,COUNT(*) FROM match_blocks GROUP BY status ORDER BY status"
            ).fetchall()
            self.assertEqual([(row[0], row[1]) for row in statuses], [("completed", 1), ("rejected", 2)])
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 2)

    def test_restart_resumes_next_block_and_preserves_budget_evidence(self) -> None:
        candidate = self._candidate(2)
        policy = AdaptivePolicy(min_blocks=1, max_blocks=3, weak_upper_score=20.0)
        results = [
            {"wins": 0, "draws": 2, "losses": 0},
            {"wins": 2, "draws": 0, "losses": 0},
            {"wins": 2, "draws": 0, "losses": 0},
        ]
        first = AdaptiveCampaign(
            self.database,
            "fake-phase9",
            candidate,
            policy,
            FakeBlockRunner(self.database, "fake-phase9", results),
            seed=9001,
        )
        checkpoint = first.run(max_blocks=1)
        self.assertEqual(checkpoint["decision"], "continue")
        self.assertEqual(checkpoint["next_block_index"], 1)
        self.assertEqual(checkpoint["statistics"]["games"], 2)

        resumed = AdaptiveCampaign(
            self.database,
            "fake-phase9",
            candidate,
            policy,
            FakeBlockRunner(self.database, "fake-phase9", results),
            seed=9001,
        )
        checkpoint = resumed.run(max_blocks=1)
        self.assertEqual(checkpoint["next_block_index"], 2)
        final = resumed.run()
        self.assertEqual(final["decision"], "accept")
        self.assertEqual(final["statistics"]["blocks_completed"], 3)
        self.assertEqual(final["statistics"]["games"], 6)
        with self.database._read() as connection:
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM match_blocks WHERE status='completed'").fetchone()[0], 3)
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 6)
            self.assertEqual(
                connection.execute("SELECT COUNT(*) FROM events WHERE event_type='match_block_completed'").fetchone()[0],
                3,
            )
        stored = json.loads(self.database.list_trials("fake-phase9", 10)[0]["result_json"])
        self.assertEqual(stored["decision"], "accept")
        self.assertEqual(stored["statistics"]["block_ids"], final["statistics"]["block_ids"])

    def test_coordinate_search_accepts_adaptive_final_result_and_creates_next_candidate(self) -> None:
        policy = AdaptivePolicy(min_blocks=1, max_blocks=1, weak_upper_score=20.0)

        def runner_factory(candidate: dict[str, object], seed: int):
            del seed
            runner = FakeBlockRunner(
                self.database,
                "fake-phase9",
                [{"wins": 2, "draws": 0, "losses": 0}],
            )
            parameter_hash = __import__("goalaric_optimizer.canonical", fromlist=["sha256_json"]).sha256_json(candidate)
            return runner, lambda index: ("book-phase9", f"block-{parameter_hash}-{index}")

        evaluator = AdaptiveCoordinateEvaluator(
            self.database,
            "fake-phase9",
            policy,
            lambda parameters, seed: {"wins": 1, "draws": 0, "losses": 1, "score": 50.0, "uncertainty": 1.0},
            runner_factory,
        )
        report = CoordinateSearch(
            self.database,
            "fake-phase9",
            self.registry,
            evaluator,
            max_passes=1,
        ).run(max_results=4)
        self.assertEqual(report["result_count"], 3)  # baseline and both directions
        self.assertEqual(report["last_result"]["classification"], "win")
        self.assertEqual(report["best"]["parameters"]["parameters"][0]["value"], 2)
        with self.database._read() as connection:
            hashes = [
                row["parameter_hash"]
                for row in connection.execute(
                    "SELECT parameter_hash FROM parameter_sets WHERE campaign_id=?", ("fake-phase9",)
                ).fetchall()
            ]
        self.assertEqual(len(hashes), len(set(hashes)))
        self.assertGreaterEqual(len(hashes), 3)


if __name__ == "__main__":
    unittest.main()


class Phase9RealIntegrationTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.repo_root = Path(__file__).parents[2].resolve()
        cls.go = shutil.which("go")
        cls.fastchess = cls.repo_root.parent / "GoAlaric" / ".tools" / "fastchess" / "bin" / "fastchess"
        if cls.go is None or not cls.fastchess.exists():
            raise unittest.SkipTest("Go and the local Fastchess binary are required")
        cls.build_dir = Path(tempfile.mkdtemp(prefix="goalaric-phase9-build-"))
        cls.testmonitor = cls.build_dir / "testmonitor"
        cls.engine = cls.build_dir / "goalaric"
        subprocess.run(
            [cls.go, "build", "-o", str(cls.testmonitor), "./cmd/testmonitor"],
            cwd=cls.repo_root,
            check=True,
        )
        subprocess.run([cls.go, "build", "-o", str(cls.engine), "."], cwd=cls.repo_root, check=True)

    @classmethod
    def tearDownClass(cls) -> None:
        shutil.rmtree(cls.build_dir, ignore_errors=True)

    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry_path = self.repo_root / "optimizer" / "registries" / "eval-pilot-v1-default.json"
        self.registry = load_registry(self.registry_path)
        baseline = default_parameter_document(self.registry)
        candidate = json.loads(json.dumps(baseline))
        candidate["parameters"][0]["value"] += 1
        self.candidate = candidate
        self.baseline_file = self.root / "baseline.json"
        self.candidate_file = self.root / "candidate.json"
        self.baseline_file.write_text(json.dumps(baseline), encoding="utf-8")
        self.candidate_file.write_text(json.dumps(candidate), encoding="utf-8")
        self.book = self.root / "book.epd"
        self.book.write_text(
            "".join(
                f"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1 id phase9-{index}\n"
                for index in range(100)
            ),
            encoding="utf-8",
        )
        self.campaign_path = self.root / "campaign.json"
        self.campaign_path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "real-phase9",
                    "name": "Real phase 9",
                    "mode": "real",
                    "registry": str(self.registry_path),
                    "baseline": {"engine_id": str(self.engine)},
                    "master_seed": 20260909,
                    "partitions": {"adaptive": {"name": "adaptive"}},
                }
            ),
            encoding="utf-8",
        )
        init_campaign(self.campaign_path, self.data_dir)
        self.database = Database(self.data_dir / "real-phase9" / "campaign.db")
        self.config = RealTestmonitorConfig(
            testmonitor_command=(str(self.testmonitor),),
            fastchess=self.fastchess,
            baseline=self.engine,
            candidate=self.engine,
            baseline_parameter_file=self.baseline_file,
            candidate_parameter_file=self.candidate_file,
            opening_book=self.book,
            opening_block_file=self.root / "placeholder.epd",
            tc="0.2+0.01",
            seed=20260909,
            hash_mb=16,
            threads=1,
            syzygy_path="off",
            workdir=self.repo_root,
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def test_two_block_real_campaign_reaches_terminal_evidence(self) -> None:
        result = run_real_adaptive_campaign(
            self.data_dir,
            "real-phase9",
            self.config,
            self.candidate,
            AdaptivePolicy(min_blocks=1, max_blocks=2, weak_upper_score=0.0),
        )
        self.assertIn(result["decision"], {"accept", "reject", "uncertain"})
        self.assertEqual(result["statistics"]["blocks_completed"], 2)
        self.assertEqual(result["statistics"]["games"], 4)
        self.assertIn("elo_estimate", result["statistics"])
        self.assertIn("score_ci_low", result["statistics"])
        self.assertIn("score_ci_high", result["statistics"])
        self.assertEqual(self.database.running_block_processes("real-phase9"), [])
        with self.database._read() as connection:
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM match_blocks WHERE status='completed'").fetchone()[0], 2)
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 4)
            stored = json.loads(connection.execute("SELECT result_json FROM trials").fetchone()[0])
        self.assertEqual(stored["decision"], result["decision"])
        self.assertEqual(stored["statistics"]["block_ids"], result["statistics"]["block_ids"])
