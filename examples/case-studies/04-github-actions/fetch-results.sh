#!/usr/bin/env bash
# Collects the ::notice:: lines of one run of the case study workflow
# (.github/workflows/case-study-memo.yml) into
# test/results/case-studies/04-github-actions.txt.
# Needs: gh, logged in. Usage: fetch-results.sh RUN_ID [RUN_ID...]
set -euo pipefail
repo=softdevstudio/bxm
out="$(cd "$(dirname "$0")/../../.." && pwd)/test/results/case-studies/04-github-actions.txt"
mkdir -p "$(dirname "$out")"
for run in "$@"; do
  gh api "repos/$repo/actions/runs/$run" --jq '"run \(.id), \(.created_at), commit \(.head_sha[0:7]), \(.conclusion)\n\(.html_url)"'
  for job in plain cold warm docs source; do
    id=$(gh api "repos/$repo/actions/runs/$run/jobs" --jq ".jobs[] | select(.name == \"$job\") | .id")
    gh api "repos/$repo/actions/jobs/$id" --jq '"== \(.name): \(.conclusion), job \((((.completed_at | fromdate) - (.started_at | fromdate))) ) s"'
    gh api "repos/$repo/check-runs/$id/annotations" --jq '.[] | select(.annotation_level == "notice" and .title != "") | "  \(.title): \(.message)"'
  done
  echo
done > "$out"
cat "$out"
