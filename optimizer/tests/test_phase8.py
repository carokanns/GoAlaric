from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from dataclasses import replace
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.config import load_campaign_definition
from goalaric_optimizer.database import Database
from goalaric_optimizer.real_integration import RealTestmonitorConfig, run_real_testmonitor
from goalaric_optimizer.registry import default_parameter_document, load_parameter_file, load_registry
from goalaric_optimizer.service import init_campaign, pause_campaign, stop_campaign


class Phase8RealIntegrationTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.repo_root = Path(__file__).parents[2].resolve()
        cls.go = shutil.which("go")
        if cls.go is None:
            raise unittest.SkipTest("Go is required for the real testmonitor integration")
        cls.fastchess = cls.repo_root.parent / "GoAlaric" / ".tools" / "fastchess" / "bin" / "fastchess"
        if not cls.fastchess.exists():
            raise unittest.SkipTest(f"real Fastchess binary is not available: {cls.fastchess}")
        cls.build_dir = Path(tempfile.mkdtemp(prefix="goalaric-phase8-build-"))
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
        self.baseline_file = self.root / "baseline-parameters.json"
        self.candidate_file = self.root / "candidate-parameters.json"
        baseline = default_parameter_document(self.registry)
        candidate = json.loads(json.dumps(baseline))
        candidate["parameters"][0]["value"] += 1
        self.baseline_file.write_text(json.dumps(baseline), encoding="utf-8")
        self.candidate_file.write_text(json.dumps(candidate), encoding="utf-8")
        self.opening_block = self.root / "opening-block.epd"
        self.opening_book = self.root / "opening-book.epd"
        self.opening_book.write_text(
            "".join(
                f"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1 id phase8-{index}\n"
                for index in range(100)
            ),
            encoding="utf-8",
        )
        self.campaign_file = self.root / "campaign.json"
        self.campaign_file.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "real-phase8",
                    "name": "Real phase 8 integration",
                    "mode": "real",
                    "registry": str(self.registry_path),
                    "baseline": {"engine_id": str(self.engine)},
                    "master_seed": 20260813,
                    "partitions": {"real-e2e": {"name": "real-e2e"}},
                }
            ),
            encoding="utf-8",
        )
        init_campaign(self.campaign_file, self.data_dir)
        self.database = Database(self.data_dir / "real-phase8" / "campaign.db")
        self.config = RealTestmonitorConfig(
            testmonitor_command=(str(self.testmonitor),),
            fastchess=self.fastchess,
            baseline=self.engine,
            candidate=self.engine,
            baseline_parameter_file=self.baseline_file,
            candidate_parameter_file=self.candidate_file,
            opening_book=self.opening_book,
            opening_block_file=self.opening_block,
            tc="0.2+0.01",
            seed=20260813,
            hash_mb=16,
            threads=1,
            syzygy_path="off",
            workdir=self.repo_root,
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def _run(self, config: RealTestmonitorConfig, errors: list[BaseException]) -> None:
        try:
            run_real_testmonitor(
                self.data_dir,
                "real-phase8",
                config,
                json.loads(self.candidate_file.read_text(encoding="utf-8")),
                registry=self.registry,
                poll_interval=0.01,
                stop_grace_seconds=1.0,
            )
        except BaseException as exc:  # surfaced by the test thread
            errors.append(exc)

    def _wait_for_running_block(self, errors: list[BaseException]) -> None:
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if errors:
                raise errors[0]
            if self.database.running_block_processes("real-phase8"):
                return
            time.sleep(0.02)
        self.fail(
            f"real scheduler did not start a testmonitor block: "
            f"snapshot={self.database.status_snapshot('real-phase8')} runs="
            f"{list((self.data_dir / 'real-phase8' / 'runs').glob('*')) if (self.data_dir / 'real-phase8' / 'runs').exists() else []}"
        )

    @staticmethod
    def _engine_or_fastchess_pids(paths: tuple[Path, ...]) -> list[int]:
        found: list[int] = []
        for proc in Path("/proc").glob("[0-9]*"):
            try:
                command_line = (proc / "cmdline").read_bytes().replace(b"\0", b" ").decode(errors="replace")
            except OSError:
                continue
            if any(str(path) in command_line for path in paths):
                found.append(int(proc.name))
        return found

    def test_real_testmonitor_report_is_atomic_and_resume_counts_once(self) -> None:
        slow = replace(self.config, tc="30+0.1")
        errors: list[BaseException] = []
        thread = threading.Thread(target=self._run, args=(slow, errors))
        thread.start()
        self._wait_for_running_block(errors)

        pause_campaign(self.data_dir, "real-phase8")
        stop_campaign(self.data_dir, "real-phase8")
        thread.join(timeout=10)
        self.assertFalse(thread.is_alive(), "scheduler did not stop after real run interruption")
        if errors:
            raise errors[0]
        self.assertEqual(self.database.status_snapshot("real-phase8")["status"], "interrupted")
        self.assertEqual(self.database.running_block_processes("real-phase8"), [])
        self.assertEqual(self._engine_or_fastchess_pids((self.engine, self.fastchess)), [])

        result = run_real_testmonitor(
            self.data_dir,
            "real-phase8",
            self.config,
            json.loads(self.candidate_file.read_text(encoding="utf-8")),
            registry=self.registry,
            poll_interval=0.01,
            stop_grace_seconds=1.0,
        )
        run_root = self.data_dir / "real-phase8" / "runs"
        run_logs = {
            str(path): path.read_text(encoding="utf-8", errors="replace")
            for path in run_root.glob("*/monitor.log")
        } if run_root.exists() else {}
        self.assertEqual(result["status"], "completed", f"runs={run_logs}")
        self.assertEqual(result["blocks"], {"completed": 1})
        self.assertEqual(result["games"], 2)
        self.assertEqual(self.database.running_block_processes("real-phase8"), [])
        self.assertEqual(self._engine_or_fastchess_pids((self.engine, self.fastchess)), [])

        with self.database._read() as connection:
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM match_blocks WHERE status='completed'").fetchone()[0], 1)
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 2)
            self.assertEqual(
                connection.execute(
                    "SELECT COUNT(*) FROM events WHERE campaign_id=? AND event_type='match_block_completed'",
                    ("real-phase8",),
                ).fetchone()[0],
                1,
            )
            row = connection.execute("SELECT result_json FROM match_blocks WHERE status='completed'").fetchone()
        stored = json.loads(row["result_json"])
        self.assertTrue(stored["block_report"]["valid"])
        self.assertTrue(stored["block_report"]["counted"])
        self.assertEqual(stored["block_report"]["games"], 2)
        self.assertEqual(
            stored["identities"]["baseline"]["sha256"], stored["identities"]["candidate"]["sha256"]
        )
        self.assertNotEqual(
            stored["identities"]["baseline"]["parameter_sha256"],
            stored["identities"]["candidate"]["parameter_sha256"],
        )


if __name__ == "__main__":
    unittest.main()
