#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tool="$repo_root/artifacts/tools/testmonitor"
state="$repo_root/artifacts/automation/active-campaign.json"
unit="goalaric-candidate-campaign"

if systemctl --user is-active --quiet "$unit.service"; then
  echo "A GoAlaric candidate campaign is already running." >&2
  exit 1
fi

mkdir -p "$(dirname "$tool")"
cd "$repo_root"
go build -o "$tool" ./cmd/testmonitor

"$tool" campaign-init \
  --state "$state" \
  --repo-root "$repo_root" \
  "$@"

systemctl --user reset-failed "$unit.service" 2>/dev/null || true
systemd-run --user \
  --unit "$unit" \
  --collect \
  --description "GoAlaric automated candidate campaign" \
  --working-directory "$repo_root" \
  --property Restart=on-failure \
  --property RestartSec=30s \
  --property KillMode=process \
  "$tool" campaign-run --state "$state" --poll-interval 2m

echo "Campaign service started: $unit.service"
echo "Status: $repo_root/scripts/campaign_status.sh"
