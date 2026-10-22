#!/usr/bin/env bash
# Case study 2: two copies of one config, different formatting and key
# order, two real changes. A text diff against bxm-canon diff.
# Needs: python3, diff, bxm-canon on PATH.
set -euo pipefail
cd "$(dirname "$0")"
PY=${PYTHON:-python3}
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
$PY make_configs.py
lines() { wc -l < "$1" | tr -d ' '; }
echo "deployed.json: $(lines deployed.json) lines; edited.json: $(lines edited.json) lines"
changed=$(diff -u deployed.json edited.json | grep -c '^[-+][^-+]' || true)
echo "diff -u: $changed changed lines"
# The usual fix: normalise both with sorted keys and one indent, then diff.
for f in deployed edited; do $PY -c 'import json,sys; print(json.dumps(json.load(open(sys.argv[1])),indent=2,sort_keys=True))' "$f.json" > "$tmp/$f.json"; done
norm=$(diff -u $tmp/deployed.json $tmp/edited.json | grep -c '^[-+][^-+]' || true)
echo "diff -u after sorting keys and re-indenting: $norm changed lines"
{ diff -u $tmp/deployed.json $tmp/edited.json || true; } | grep '^[-+][^-+]' | sed 's/^/  /'
echo "bxm-canon diff --text deployed.json edited.json:"
{ bxm-canon diff --text deployed.json edited.json || true; } | sed 's/^/  /'
echo "bxm-canon hash --text: $(bxm-canon hash --text deployed.json | cut -c1-16) vs $(bxm-canon hash --text edited.json | cut -c1-16)"
