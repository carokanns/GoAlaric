from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.cli import main
from goalaric_optimizer.database import Database, DatabaseError


class Phase13MinimalRealOptimizationTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.repo_root = Path(__file__).parents[2].resolve()
        cls.go = shutil.which("go")
        cls.fastchess = cls.repo_root.parent / "GoAlaric" / ".tools" / "fastchess" / "bin" / "fastchess"
        if cls.go is None:
            raise unittest.SkipTest("Go is required for the real optimize integration")
        if not cls.fastchess.exists():
            raise unittest.SkipTest(f"Fastchess is not available: {cls.fastchess}")
        cls.build_dir = Path(tempfile.mkdtemp(prefix="goalaric-phase13-build-"))
        cls.testmonitor = cls.build_dir / "testmonitor"
        cls.engine = cls.build_dir / "goalaric"
        subprocess.run([cls.go, "build", "-o", str(cls.testmonitor), "./cmd/testmonitor"], cwd=cls.repo_root, check=True)
        subprocess.run([cls.go, "build", "-o", str(cls.engine), "."], cwd=cls.repo_root, check=True)

    @classmethod
    def tearDownClass(cls) -> None:
        shutil.rmtree(cls.build_dir, ignore_errors=True)

    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.data_dir = self.root / "campaigns"
        source_registry = self.repo_root / "optimizer" / "registries" / "eval-pilot-v1-default.json"
        registry_document = json.loads(source_registry.read_text(encoding="utf-8"))
        registry_document["parameters"][0].update({"min": 0, "max": 64, "step": 1, "min_step": 1})
        self.registry = self.root / "registry.json"
        self.registry.write_text(json.dumps(registry_document), encoding="utf-8")
        self.opening_book = self.root / "opening-book.epd"
        self.opening_book.write_text(
            "".join(
                f"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1 id phase13-{index}\n"
                for index in range(100)
            ),
            encoding="utf-8",
        )
        self.campaign = self.root / "campaign.json"
        self.campaign.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "campaign_id": "real-phase13",
                    "name": "Minimal real autonomous optimization",
                    "mode": "real",
                    "registry": str(self.registry),
                    "baseline": {"engine_id": str(self.engine)},
                    "master_seed": 20260814,
                    "partitions": {"adaptive": {"name": "adaptive"}},
                    "goals": {
                        "max_games": 4,
                        "max_evaluations": 3,
                        "max_passes": 2,
                        "optimizer": {"parameters": ["mobility_weight"]},
                        "adaptive": {"min_blocks": 1, "max_blocks": 1, "weak_upper_score": 45.0},
                        "real": {
                            "testmonitor_command": [str(self.testmonitor)],
                            "fastchess": str(self.fastchess),
                            "opening_book": str(self.opening_book),
                            "tc": "0.2+0.01",
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

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    @staticmethod
    def _campaign_pids(paths: tuple[Path, ...]) -> list[int]:
        found: list[int] = []
        for proc in Path("/proc").glob("[0-9]*"):
            try:
                command_line = (proc / "cmdline").read_bytes().replace(b"\0", b" ").decode(errors="replace")
            except OSError:
                continue
            if any(str(path) in command_line for path in paths):
                found.append(int(proc.name))
        return found

    def _optimize(self, max_results: int = 0) -> int:
        return main(
            [
                "optimize",
                str(self.campaign),
                "--data-dir",
                str(self.data_dir),
                "--max-results",
                str(max_results),
            ]
        )

    def test_minimal_real_campaign_resumes_and_counts_each_game_once(self) -> None:
        self.assertEqual(self._optimize(max_results=2), 0)
        database = Database(self.data_dir / "real-phase13" / "campaign.db")
        self.assertEqual(database.campaign("real-phase13")["status"], "running")

        self.assertEqual(self._optimize(), 0)
        snapshot = database.status_snapshot("real-phase13")
        self.assertEqual(snapshot["status"], "completed")
        self.assertEqual(snapshot["games"], 4)
        self.assertEqual(snapshot["blocks"], {"completed": 2})
        self.assertEqual(database.running_block_processes("real-phase13"), [])
        self.assertEqual(self._campaign_pids((self.engine, self.fastchess, self.testmonitor)), [])

        with database._read() as connection:
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 4)
            self.assertEqual(
                connection.execute(
                    "SELECT COUNT(*) FROM events WHERE campaign_id=? AND event_type='match_block_completed'",
                    ("real-phase13",),
                ).fetchone()[0],
                2,
            )
            rows = connection.execute(
                "SELECT result_json FROM match_blocks WHERE status='completed' ORDER BY block_index"
            ).fetchall()
        for row in rows:
            result = json.loads(row["result_json"])
            self.assertEqual(result["runner"], "real-testmonitor-v1")
            self.assertNotEqual(
                result["identities"]["baseline"]["parameter_sha256"],
                result["identities"]["candidate"]["parameter_sha256"],
            )
            self.assertEqual(result["identities"]["baseline"]["sha256"], result["identities"]["candidate"]["sha256"])

    def test_real_campaign_recovers_after_optimizer_process_death(self) -> None:
        config = json.loads(self.campaign.read_text(encoding="utf-8"))
        config["goals"]["real"]["tc"] = "1+0.01"
        self.campaign.write_text(json.dumps(config), encoding="utf-8")
        command = [
            sys.executable,
            "-c",
            "import sys; from goalaric_optimizer.cli import main; raise SystemExit(main(sys.argv[1:]))",
            "optimize",
            str(self.campaign),
            "--data-dir",
            str(self.data_dir),
        ]
        environment = os.environ.copy()
        source_path = str(self.repo_root / "optimizer" / "src")
        environment["PYTHONPATH"] = os.pathsep.join(
            item for item in (source_path, environment.get("PYTHONPATH")) if item
        )
        process = subprocess.Popen(
            command,
            cwd=self.repo_root,
            env=environment,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        database_path = self.data_dir / "real-phase13" / "campaign.db"
        deadline = time.monotonic() + 10.0
        while not database_path.exists() and time.monotonic() < deadline and process.poll() is None:
            time.sleep(0.05)
        self.assertTrue(database_path.exists(), "optimizer exited before creating the SQLite campaign")
        database = Database(database_path)
        while time.monotonic() < deadline and process.poll() is None:
            try:
                if database.running_block_processes("real-phase13"):
                    break
            except DatabaseError:
                pass
            time.sleep(0.05)
        self.assertIsNone(process.poll(), "optimizer finished before the process-death checkpoint")
        self.assertTrue(database.running_block_processes("real-phase13"))
        process.kill()
        process.wait(timeout=5)
        if process.stdout is not None:
            process.stdout.close()

        self.assertEqual(self._optimize(), 0)
        snapshot = database.status_snapshot("real-phase13")
        self.assertEqual(snapshot["status"], "completed")
        self.assertEqual(snapshot["games"], 4)
        self.assertEqual(snapshot["blocks"].get("completed"), 2)
        self.assertEqual(snapshot["blocks"].get("running", 0), 0)
        self.assertEqual(snapshot["blocks"].get("pending", 0), 0)
        self.assertEqual(database.running_block_processes("real-phase13"), [])
        self.assertEqual(self._campaign_pids((self.engine, self.fastchess, self.testmonitor)), [])

        with database._read() as connection:
            self.assertEqual(connection.execute("SELECT COUNT(*) FROM games").fetchone()[0], 4)
            self.assertGreaterEqual(
                connection.execute(
                    "SELECT COUNT(*) FROM events WHERE campaign_id=? AND event_type='abandoned_job_recovered'",
                    ("real-phase13",),
                ).fetchone()[0],
                1,
            )


if __name__ == "__main__":
    unittest.main()
