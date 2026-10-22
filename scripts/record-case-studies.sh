#!/usr/bin/env bash
# Runs the case studies in examples/case-studies and writes their output to
# test/results/case-studies. Case study 4 runs in GitHub Actions; its file is
# written by examples/case-studies/04-github-actions/fetch-results.sh.
# Needs: bxm-canon and bxm-memo on PATH, python3 with bxm_canon (or PYTHON),
# node with bxm-canon installed next to case study 1, cargo, go, git,
# python3.12 and python3.13.
set -euo pipefail
cd "$(dirname "$0")/.."
out=test/results/case-studies
mkdir -p "$out"
cs=examples/case-studies
PY=${PYTHON:-python3}
{
  date -u +"recorded %Y-%m-%d %H:%M UTC"
  uname -srm
  echo "cpus: $(nproc)"
  bxm-canon version; bxm-memo version
  echo "python: $($PY --version 2>&1), bxm_canon $($PY -c 'import bxm_canon; print(bxm_canon.__version__)')"
  echo "node: $(node --version)"
  cargo --version; go version
} > "$out/environment.txt"
PYTHON=$PY "$cs/01-three-languages/run.sh"   > "$out/01-three-languages.txt"
PYTHON=$PY "$cs/02-config-diff/run.sh"       > "$out/02-config-diff.txt"
$PY "$cs/03-duplicates/run.py"               > "$out/03-duplicates.txt"
"$cs/05-tests-across-commits/run.sh"         > "$out/05-tests-across-commits.txt"
"$cs/06-miss-reasons/run.sh"                 > "$out/06-miss-reasons.txt"
echo "wrote $out"
