#!/usr/bin/env bash
# Case study 6: what bxm-memo says when it misses. One report step, run
# again after each kind of change: none, an env var, the Python version,
# an edited data file, a new data file, a deleted one, and all of it undone.
# Needs: bxm-memo on PATH, python3.12 and python3.13.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d); trap 'rm -rf "$work"' EXIT
cp "$here/report.py" "$work/"; cd "$work"
mkdir -p data shim
printf 'region,amount\nnorth,120.50\nsouth,80.00\n' > data/2026-09.csv
printf 'region,amount\nnorth,99.90\neast,45.25\n' > data/2026-10.csv
export BXM_MEMO_CACHE="$work/cache" REPORT_CURRENCY=EUR
export PATH="$work/shim:$PATH"
ln -sf "$(command -v python3.12)" shim/python3

step() {
  echo "== $1"
  bxm-memo explain -n report -i report.py -i 'data/*.csv' -o out -e REPORT_CURRENCY \
    --salt "$(python3 --version)" -- python3 report.py | grep -v '^key \|^inputs ' || true
  bxm-memo run -q -n report -i report.py -i 'data/*.csv' -o out -e REPORT_CURRENCY \
    --salt "$(python3 --version)" -- python3 report.py > /dev/null
}

step "first run"
step "nothing changed"
REPORT_CURRENCY=CHF step "REPORT_CURRENCY set to CHF"
ln -sf "$(command -v python3.13)" shim/python3
REPORT_CURRENCY=CHF step "python3 now points to $(python3 --version)"
printf 'region,amount\nnorth,120.50\nsouth,85.00\n' > data/2026-09.csv
REPORT_CURRENCY=CHF step "one amount in data/2026-09.csv corrected"
printf 'region,amount\nwest,300.00\n' > data/2026-11.csv
REPORT_CURRENCY=CHF step "data/2026-11.csv added"
rm data/2026-10.csv
REPORT_CURRENCY=CHF step "data/2026-10.csv deleted"
rm data/2026-11.csv
printf 'region,amount\nnorth,120.50\nsouth,80.00\n' > data/2026-09.csv
printf 'region,amount\nnorth,99.90\neast,45.25\n' > data/2026-10.csv
ln -sf "$(command -v python3.12)" shim/python3
step "every change undone: Python 3.12, EUR, the first two data files as they were"
echo "== final report"
cat out/report.txt
