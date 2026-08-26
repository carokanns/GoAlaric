from __future__ import annotations

import json
import sqlite3
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.canonical import sha256_json
from goalaric_optimizer.dashboard import DashboardReader, final_report
from goalaric_optimizer.database import CampaignConflict, Database
from goalaric_optimizer.optimization import run_optimization
from goalaric_optimizer.profiles import MatchProfile, ProfileError, resolve_profile
from goalaric_optimizer.real_integration import RealTestmonitorConfig, RealTestmonitorScheduler


class Phase22NodeProfileTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-phase22-node-")
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry = self.root / "registry.json"
        self.registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "phase22-node-profile-v1",
                    "parameters": [
                        {"name": "p", "value": 0, "min": 0, "max": 2, "step": 1, "min_step": 1}
                    ],
                }
            ),
            encoding="utf-8",
        )
        self.campaign = self.root / "campaign.json"
        self.campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "phase22-node-profile",
                    "name": "Phase 22 node profile plumbing",
                    "mode": "fake",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 20261030,
                    "partitions": {"optimization": {"name": "optimization"}},
                    "goals": {
                        "max_evaluations": 2,
                        "max_passes": 2,
                        "optimizer": {"parameters": ["p"], "profile": "node-search"},
                        "adaptive": {"min_blocks": 1, "max_blocks": 1},
                        "fake_match": {"optimum": {"p": 1}},
                        "real": {
                            "tc": "0.2+0.01",
                            "profiles": {
                                "node-search": {"nodes": 100000},
                                "node-confirmation": {"nodes": 250000},
                            },
                        },
                        "confirmation": {
                            "enabled": True,
                            "games": 4,
                            "seed": 20261031,
                            "confidence": 0.95,
                            "profile": "node-confirmation",
                            "fake_result": {"wins": 2, "draws": 2, "losses": 0},
                        },
                    },
                }
            ),
            encoding="utf-8",
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def test_profile_resolution_and_hashes(self) -> None:
        time_profile = MatchProfile.create("long-search", "1+0.02", "test")
        self.assertEqual(time_profile.hash, sha256_json({"name": "long-search", "tc": "1+0.02"}))
        node_profile = resolve_profile(
            {"tc": "0.2+0.01", "profiles": {"node-search": {"nodes": 100000}}},
            "node-search",
        )
        self.assertEqual(node_profile.mode, "nodes")
        self.assertIsNone(node_profile.tc)
        self.assertEqual(node_profile.nodes, 100000)
        self.assertEqual(
            node_profile.hash,
            sha256_json({"name": "node-search", "mode": "nodes", "nodes": 100000}),
        )
        self.assertEqual(node_profile.as_dict()["mode"], "nodes")
        self.assertEqual(node_profile.as_dict()["nodes"], 100000)
        with self.assertRaises(ProfileError):
            resolve_profile({"profiles": {"bad": {"tc": "1+0.02", "nodes": 100}}}, "bad")
        with self.assertRaises(ProfileError):
            resolve_profile({"profiles": {"bad": {"nodes": 0}}}, "bad")
        with self.assertRaises(ProfileError):
            resolve_profile({"profiles": {"bad": {}}}, "bad")

    def test_fake_flow_persists_node_profile_and_exposes_it(self) -> None:
        run_optimization(self.campaign, self.data_dir)
        database = Database(self.data_dir / "phase22-node-profile" / "campaign.db")
        search = resolve_profile(
            {"tc": "0.2+0.01", "profiles": {"node-search": {"nodes": 100000}}}, "node-search"
        )
        confirmation = resolve_profile(
            {"tc": "0.2+0.01", "profiles": {"node-confirmation": {"nodes": 250000}}},
            "node-confirmation",
            "confirmation",
        )
        with database._read() as connection:
            trial = connection.execute(
                "SELECT profile_name,profile_hash,profile_tc,profile_mode,profile_nodes "
                "FROM trials WHERE campaign_id=? LIMIT 1",
                ("phase22-node-profile",),
            ).fetchone()
            self.assertIsNotNone(trial)
            assert trial is not None
            self.assertEqual(
                tuple(trial), (search.name, search.hash, None, "nodes", search.nodes)
            )
            row = connection.execute(
                "SELECT profile_name,profile_hash,profile_tc,profile_mode,profile_nodes "
                "FROM confirmations WHERE campaign_id=?",
                ("phase22-node-profile",),
            ).fetchone()
            self.assertIsNotNone(row)
            assert row is not None
            self.assertEqual(
                tuple(row), (confirmation.name, confirmation.hash, None, "nodes", confirmation.nodes)
            )

        snapshot = DashboardReader(self.data_dir, "phase22-node-profile").snapshot()
        self.assertEqual(snapshot["search_profile"]["nodes"], 100000)
        self.assertEqual(snapshot["confirmation_profile"]["nodes"], 250000)
        self.assertIn("100000 nodes/move", final_report(self.data_dir, "phase22-node-profile", "html")[1])
        self.assertEqual(snapshot["current_trial"]["profile"]["mode"], "nodes")

        with self.assertRaises(CampaignConflict):
            database.bind_optimizer_profile(
                "phase22-node-profile",
                "search",
                resolve_profile(
                    {"profiles": {"node-search": {"nodes": 125000}}}, "node-search"
                ).as_dict(),
            )

    def test_real_builder_uses_nodes_without_tc(self) -> None:
        profile = resolve_profile(
            {"tc": "0.2+0.01", "profiles": {"node-search": {"nodes": 100000}}}, "node-search"
        )
        config = RealTestmonitorConfig(
            testmonitor_command=("testmonitor",),
            fastchess=Path("fastchess"),
            baseline=Path("baseline"),
            candidate=Path("candidate"),
            baseline_parameter_file=Path("baseline.json"),
            candidate_parameter_file=Path("candidate.json"),
            opening_book=Path("book.epd"),
            opening_block_file=Path("block.epd"),
            tc=profile.tc,
            profile_name=profile.name,
            profile_hash=profile.hash,
            profile_mode=profile.mode,
            nodes=profile.nodes,
        )
        scheduler = RealTestmonitorScheduler(self.root, "phase22-node-profile", config)
        command = scheduler._command({"pairs_per_block": 1, "block_index": 0, "master_seed": 1}, self.root, self.root / "result.json")
        self.assertIn("--nodes", command)
        self.assertIn("100000", command)
        self.assertNotIn("--tc", command)

        time_profile = resolve_profile({"tc": "0.2+0.01"})
        self.assertEqual(time_profile.mode, "time")
        self.assertEqual(time_profile.tc, "0.2+0.01")
        self.assertIsNone(time_profile.nodes)

    def test_legacy_schema_profile_migration_is_idempotent(self) -> None:
        path = self.root / "legacy-schema.db"
        connection = sqlite3.connect(path)
        connection.execute("CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)")
        connection.execute("INSERT INTO schema_meta(key, value) VALUES('schema_version', '2')")
        connection.execute(
            "CREATE TABLE trials ("
            "trial_id TEXT PRIMARY KEY, campaign_id TEXT, parameter_set_id TEXT, "
            "status TEXT, algorithm TEXT, seed INTEGER, result_json TEXT, error TEXT, "
            "pid INTEGER, created_at TEXT, started_at TEXT, finished_at TEXT, updated_at TEXT"
            ")"
        )
        connection.commit()
        connection.close()

        database = Database(path)
        database.initialize()
        database.initialize()
        with database._read() as connection:
            self.assertEqual(
                connection.execute(
                    "SELECT value FROM schema_meta WHERE key='schema_version'"
                ).fetchone()[0],
                "5",
            )
            for table in ("trials", "confirmations"):
                columns = {
                    row[1] for row in connection.execute(f"PRAGMA table_info({table})").fetchall()
                }
                self.assertTrue(
                    {"profile_name", "profile_hash", "profile_tc", "profile_mode", "profile_nodes"}
                    <= columns
                )


class Phase22MinimalRealNodeProfileTest(unittest.TestCase):
    """Exercise one real two-game block with Fastchess nodes=N transport."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.repo_root = Path(__file__).parents[2].resolve()
        cls.go = shutil.which("go")
        cls.fastchess = cls.repo_root.parent / "GoAlaric" / ".tools" / "fastchess" / "bin" / "fastchess"
        if cls.go is None or not cls.fastchess.exists():
            raise unittest.SkipTest("Go and the local Fastchess binary are required")
        cls.build_dir = Path(tempfile.mkdtemp(prefix="goalaric-phase22-real-build-"))
        cls.testmonitor = cls.build_dir / "testmonitor"
        cls.engine = cls.build_dir / "goalaric"
        subprocess.run([cls.go, "build", "-o", str(cls.testmonitor), "./cmd/testmonitor"], cwd=cls.repo_root, check=True)
        subprocess.run([cls.go, "build", "-o", str(cls.engine), "."], cwd=cls.repo_root, check=True)

    @classmethod
    def tearDownClass(cls) -> None:
        shutil.rmtree(cls.build_dir, ignore_errors=True)

    def test_two_real_games_use_node_profile(self) -> None:
        with tempfile.TemporaryDirectory(prefix="goalaric-phase22-real-") as raw_dir:
            root = Path(raw_dir)
            baseline = json.loads((self.repo_root / "optimizer" / "docs" / "phase11-v1-recommended-parameters.json").read_text())
            candidate = json.loads(json.dumps(baseline))
            baseline["parameters"][0]["value"] = 18
            candidate["parameters"][0]["value"] = 19
            baseline_path = root / "baseline.json"
            candidate_path = root / "candidate.json"
            baseline_path.write_text(json.dumps(baseline), encoding="utf-8")
            candidate_path.write_text(json.dumps(candidate), encoding="utf-8")
            openings = root / "openings.epd"
            openings.write_text(
                "".join(
                    f"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1 id phase22-{ix}\n"
                    for ix in range(100)
                ),
                encoding="utf-8",
            )
            run_dir = root / "run"
            completed = subprocess.run(
                [
                    str(self.testmonitor),
                    "run-match",
                    "--fastchess",
                    str(self.fastchess),
                    "--baseline",
                    str(self.engine),
                    "--candidate",
                    str(self.engine),
                    "--baseline-parameter-file",
                    str(baseline_path),
                    "--candidate-parameter-file",
                    str(candidate_path),
                    "--optimizer-mode",
                    "--openings",
                    str(openings),
                    "--games",
                    "2",
                    "--nodes",
                    "100000",
                    "--concurrency",
                    "1",
                    "--hash",
                    "16",
                    "--threads",
                    "1",
                    "--syzygy-path",
                    "off",
                    "--run-dir",
                    str(run_dir),
                ],
                cwd=self.repo_root,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                timeout=120,
                check=False,
            )
            self.assertEqual(completed.returncode, 0, completed.stdout)
            status = json.loads((run_dir / "status.json").read_text(encoding="utf-8"))
            monitor_config = json.loads((run_dir / "monitor-config.json").read_text(encoding="utf-8"))
            block_report = json.loads((run_dir / "block-report.json").read_text(encoding="utf-8"))
            self.assertEqual(status["state"], "completed")
            self.assertEqual(status["games"], 2)
            self.assertEqual(status["node_budget"], 100000)
            self.assertEqual(monitor_config["nodes"], 100000)
            self.assertNotIn("time_control", monitor_config)
            self.assertEqual(block_report["node_budget"], 100000)
            pgn = (run_dir / "games.pgn").read_text(encoding="utf-8")
            self.assertIn("n=100", pgn)
            self.assertIn("sd=", pgn)


if __name__ == "__main__":
    unittest.main()
