#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tool="$repo_root/artifacts/tools/testmonitor"
state="$repo_root/artifacts/automation/active-campaign.json"

if [[ ! -x "$tool" ]]; then
  echo "Campaign tool is not built: $tool" >&2
  exit 1
fi

"$tool" campaign-status --state "$state"
systemctl --user --no-pager --full status goalaric-candidate-campaign.service || true
