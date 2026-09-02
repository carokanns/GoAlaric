#!/usr/bin/env python3
import json
from pathlib import Path

ROOT = Path("/home/peter/Projekt/GoAlaric")
RUNS = ROOT / "artifacts/experiments/aspiration-budget-profile-20260902/runs"

print("nodes\tmargin\tgames\tscore\tmedian_depth\tfail_%\toverhead_%\tretries\tfull_window")
for path in sorted(RUNS.glob("nodes-*-margin-*/status.json")):
    status = json.loads(path.read_text(encoding="utf-8"))
    profile_path = path.parent / "aspiration-profile.json"
    profile = json.loads(profile_path.read_text(encoding="utf-8")) if profile_path.exists() else {}
    parts = path.parent.name.split("-")
    print(
        f"{parts[1]}\t{int(parts[3])}\t{status.get('games', 0)}\t"
        f"{status.get('score_percent', 0):.2f}\t{profile.get('median_depth', 0)}\t"
        f"{profile.get('aspiration_failure_percent', 0):.2f}\t"
        f"{profile.get('aspiration_overhead_percent', 0):.2f}\t"
        f"{profile.get('aspiration_retries', 0)}\t{profile.get('aspiration_full_window', 0)}"
    )
