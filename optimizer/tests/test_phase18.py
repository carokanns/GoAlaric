from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from dataclasses import replace
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.canonical import sha256_json
from goalaric_optimizer.dashboard import DashboardReader, final_report
from goalaric_optimizer.database import CampaignConflict, Database
from goalaric_optimizer.optimization import _real_config, run_optimization
from goalaric_optimizer.profiles import resolve_profile
from goalaric_optimizer.real_integration import run_real_testmonitor
from goalaric_optimizer.service import init_campaign


class Phase18FakeProfileTest(unittest.TestCase):
    """Verify named profile selection and immutable SQLite identities without engines."""

    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-phase18-fake-")
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        self.registry = self.root / "registry.json"
        self.registry.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "registry": "phase18-profile-registry-v1",
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
                    "campaign_id": "phase18-fake-profiles",
                    "name": "Phase 18 fake profile plumbing",
                    "mode": "fake",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": "fake-engine"},
                    "master_seed": 20260814,
                    "partitions": {"optimization": {"name": "optimization"}},
                    "goals": {
                        "max_evaluations": 3,
                        "max_passes": 3,
                        "optimizer": {"parameters": ["p"], "profile": "long-search"},
                        "adaptive": {"min_blocks": 1, "max_blocks": 1},
                        "fake_match": {"optimum": {"p": 1}},
                        "real": {
                            "tc": "0.2+0.01",
                            "profiles": {
                                "long-search": {"tc": "1+0.02"},
                                "long-confirmation": {"tc": "2+0.02"},
                            },
                        },
                        "confirmation": {
                            "enabled": True,
                            "games": 4,
                            "seed": 20260930,
                            "confidence": 0.95,
                            "profile": "long-confirmation",
                            "fake_result": {"wins": 2, "draws": 2, "losses": 0},
                        },
                    },
                }
            ),
            encoding="utf-8",
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def test_fake_profile_flow_and_restart_validation(self) -> None:
        run_optimization(self.campaign, self.data_dir)
        database = Database(self.data_dir / "phase18-fake-profiles" / "campaign.db")
        search = resolve_profile({"tc": "0.2+0.01", "profiles": {"long-search": {"tc": "1+0.02"}}}, "long-search")
        confirmation = resolve_profile(
            {
                "tc": "0.2+0.01",
                "profiles": {"long-confirmation": {"tc": "2+0.02"}},
            },
            "long-confirmation",
            "confirmation",
        )
        state = database.optimizer_state("phase18-fake-profiles")["state"]
        self.assertEqual(state["search_profile"], search.as_dict())
        with database._read() as connection:
            trials = connection.execute(
                "SELECT profile_name,profile_hash,profile_tc,result_json FROM trials "
                "WHERE campaign_id=? ORDER BY trial_id",
                ("phase18-fake-profiles",),
            ).fetchall()
            self.assertTrue(trials)
            for row in trials:
                self.assertEqual((row["profile_name"], row["profile_hash"], row["profile_tc"]),
                                 (search.name, search.hash, search.tc))
                self.assertEqual(json.loads(row["result_json"])["profile"], search.as_dict())

        stored_confirmation = database.confirmation("phase18-fake-profiles")
        self.assertIsNotNone(stored_confirmation)
        assert stored_confirmation is not None
        self.assertEqual(
            (stored_confirmation["profile_name"], stored_confirmation["profile_hash"], stored_confirmation["profile_tc"]),
            (confirmation.name, confirmation.hash, confirmation.tc),
        )
        snapshot = DashboardReader(self.data_dir, "phase18-fake-profiles").snapshot()
        self.assertEqual(snapshot["campaign"]["status"], "completed")
        self.assertEqual(snapshot["search_profile"], search.as_dict())
        self.assertEqual(snapshot["confirmation_profile"]["name"], confirmation.name)
        self.assertEqual(snapshot["confirmation_profile"]["tc"], confirmation.tc)
        self.assertEqual(snapshot["confirmation"]["profile"]["hash"], confirmation.hash)
        self.assertEqual(snapshot["current_trial"]["profile"], search.as_dict())
        status = database.status_snapshot("phase18-fake-profiles")
        self.assertEqual(status["search_profile"], search.as_dict())
        self.assertEqual(status["confirmation_profile"]["tc"], confirmation.tc)
        report, _ = final_report(self.data_dir, "phase18-fake-profiles", "json")
        self.assertEqual(report["final_anchor"]["profile"], search.as_dict())
        self.assertEqual(report["confirmation_profile"]["tc"], confirmation.tc)

        with self.assertRaises(CampaignConflict):
            database.bind_optimizer_profile(
                "phase18-fake-profiles",
                "search",
                resolve_profile({"tc": "9+0.1", "profiles": {"other": {"tc": "9+0.1"}}}, "other").as_dict(),
            )

    def test_legacy_real_tc_is_the_default_profile(self) -> None:
        profile = resolve_profile({"tc": "0.2+0.01"})
        self.assertEqual(profile.name, "default")
        self.assertEqual(profile.tc, "0.2+0.01")
        self.assertEqual(profile.source, "real.tc")


class Phase18MinimalRealProfileTest(unittest.TestCase):
    """Run one unchanged candidate through Fastchess at two named time controls."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.repo_root = Path(__file__).parents[2].resolve()
        cls.go = shutil.which("go")
        cls.fastchess = cls.repo_root.parent / "GoAlaric" / ".tools" / "fastchess" / "bin" / "fastchess"
        if cls.go is None or not cls.fastchess.exists():
            raise unittest.SkipTest("Go and the local Fastchess binary are required")
        cls.build_dir = Path(tempfile.mkdtemp(prefix="goalaric-phase18-real-build-"))
        cls.testmonitor = cls.build_dir / "testmonitor"
        cls.engine = cls.build_dir / "goalaric"
        subprocess.run([cls.go, "build", "-o", str(cls.testmonitor), "./cmd/testmonitor"], cwd=cls.repo_root, check=True)
        subprocess.run([cls.go, "build", "-o", str(cls.engine), "."], cwd=cls.repo_root, check=True)

    @classmethod
    def tearDownClass(cls) -> None:
        shutil.rmtree(cls.build_dir, ignore_errors=True)

    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory(prefix="goalaric-phase18-real-")
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        source_registry = self.repo_root / "optimizer" / "registries" / "eval-pilot-v1-default.json"
        registry = json.loads(source_registry.read_text(encoding="utf-8"))
        registry["parameters"][0].update({"min": 0, "max": 64, "step": 1, "min_step": 1})
        self.registry = self.root / "registry.json"
        self.registry.write_text(json.dumps(registry), encoding="utf-8")
        self.opening_book = self.root / "opening-book.epd"
        self.opening_book.write_text(
            "".join(
                f"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1 id phase18-{index}\n"
                for index in range(100)
            ),
            encoding="utf-8",
        )
        baseline = json.loads((self.repo_root / "optimizer" / "registries" / "eval-pilot-v1-default.json").read_text())
        candidate = json.loads(json.dumps(baseline))
        candidate["parameters"][0]["value"] += 1
        self.candidate = candidate

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _run_profile(self, campaign_id: str, profile_name: str, tc: str) -> dict[str, object]:
        campaign = self.root / f"{campaign_id}.json"
        campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": campaign_id,
                    "name": f"Phase 18 {profile_name}",
                    "mode": "real",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": str(self.engine)},
                    "master_seed": 20260814,
                    "partitions": {"real-e2e": {"name": "real-e2e"}},
                    "goals": {
                        "optimizer": {"parameters": ["mobility_weight"], "profile": profile_name},
                        "real": {
                            "testmonitor_command": [str(self.testmonitor)],
                            "fastchess": str(self.fastchess),
                            "opening_book": str(self.opening_book),
                            "tc": "0.2+0.01",
                            "profiles": {profile_name: {"tc": tc}},
                            "hash_mb": 16,
                            "threads": 1,
                            "syzygy_path": "off",
                            "workdir": str(self.repo_root),
                        },
                    },
                }
            ),
            encoding="utf-8",
        )
        definition, _, _ = init_campaign(campaign, self.data_dir)
        profile = resolve_profile(definition.config["goals"]["real"], profile_name)
        config = _real_config(campaign, self.data_dir, definition, profile)
        candidate_path = self.root / f"{campaign_id}-candidate.json"
        candidate_path.write_text(json.dumps(self.candidate), encoding="utf-8")
        config = replace(config, candidate_parameter_file=candidate_path)
        run_real_testmonitor(
            self.data_dir,
            campaign_id,
            config,
            self.candidate,
            registry=definition.registry,
            poll_interval=0.02,
            stop_grace_seconds=1.0,
        )
        database = Database(self.data_dir / campaign_id / "campaign.db")
        with database._read() as connection:
            row = connection.execute(
                "SELECT result_json FROM match_blocks WHERE campaign_id=? AND status='completed'",
                (campaign_id,),
            ).fetchone()
        self.assertIsNotNone(row)
        assert row is not None
        block_result = json.loads(row["result_json"])
        self.assertEqual(
            {key: block_result["profile"][key] for key in ("name", "hash", "tc")},
            {key: profile.as_dict()[key] for key in ("name", "hash", "tc")},
        )
        return {
            "status": database.status_snapshot(campaign_id),
            "block_result": block_result,
            "profile": profile,
            "campaign_id": campaign_id,
        }

    def test_same_candidate_reaches_fastchess_with_both_profile_tcs(self) -> None:
        short = self._run_profile("phase18-real-short", "short-evaluation", "0.2+0.01")
        long = self._run_profile("phase18-real-long", "long-search", "1+0.02")
        for item in (short, long):
            status = item["status"]
            result = item["block_result"]
            profile = item["profile"]
            self.assertEqual(status["status"], "completed")
            self.assertEqual(status["games"], 2)
            database = Database(self.data_dir / str(item["campaign_id"]) / "campaign.db")
            with database._read() as connection:
                block = connection.execute(
                    "SELECT result_json FROM match_blocks WHERE campaign_id=? AND status='completed'",
                    (item["campaign_id"],),
                ).fetchone()
            self.assertIsNotNone(block)
            result_json = json.loads(block["result_json"])
            self.assertEqual(result_json["profile"]["name"], profile.name)
            self.assertEqual(result_json["profile"]["tc"], profile.tc)
            self.assertEqual(result_json["monitor_config"]["time_control"], profile.tc)
            self.assertEqual(result_json["block_report"]["target_games"], 2)
            self.assertEqual(result_json["candidate_parameter_hash"], sha256_json(self.candidate))
            self.assertEqual(database.running_block_processes(str(item["campaign_id"])), [])

        self.assertEqual(
            short["block_result"]["candidate_parameter_hash"],
            long["block_result"]["candidate_parameter_hash"],
        )


if __name__ == "__main__":
    unittest.main()
