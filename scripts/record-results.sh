#!/usr/bin/env bash
# Runs every test and measurement and records the output in test/results.
# Every number on bxm.net comes from these files.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
res="$root/test/results"
mkdir -p "$res"
cd "$root"

now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }

{
  echo "recorded: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "machine: $(uname -srm)"
  echo "cpus: $(getconf _NPROCESSORS_ONLN)"
  echo "rust: $(rustc --version)"
  echo "go: $(go version)"
  echo "version: $(cat VERSION)"
} > "$res/environment.txt"

echo "== Canon tests"
(cd canon && BXM_PROP_ITERS=1000000 cargo test --release 2>&1) | tee "$res/canon-tests.txt" | grep -E "^test result" || true
grep -q "FAILED" "$res/canon-tests.txt" && { echo "canon tests failed"; exit 1; }

echo "== Canon C API"
canon/tests/c/run.sh 2>&1 | tee "$res/canon-c-api.txt"

echo "== Canon stats"
(cd canon && cargo run -q --release --example stats -- 2000000) | tee "$res/canon-stats.json"

echo "== Memo tests"
(cd memo && go test -count=1 -race -v ./... 2>&1) | tee "$res/memo-tests.txt" | grep -E "^(--- |ok|FAIL)" || true
grep -qE "^(--- FAIL|FAIL)" "$res/memo-tests.txt" && { echo "memo tests failed"; exit 1; }

echo "== Memo bench"
bin="$(mktemp -d)/bxm-memo"
(cd memo && go build -o "$bin" ./cmd/bxm-memo)
work="$(mktemp -d)"
export BXM_MEMO_CACHE="$work/cache"
mkdir -p "$work/src"
# 1000 input files of 1 KiB, and a step that takes about 2 seconds and writes
# 200 files of 50 KiB (about 10 MB) from them.
python3 - "$work/src" <<'PY'
import os, sys
for i in range(1000):
    with open(os.path.join(sys.argv[1], f"f{i:04}.txt"), "wb") as f:
        f.write(os.urandom(1024))
PY
cat > "$work/step.sh" <<'SH'
#!/bin/sh
sleep 2
mkdir -p out
i=0
while [ $i -lt 200 ]; do
  cat src/f00*.txt | head -c 51200 > out/part$i.bin
  i=$((i+1))
done
SH
chmod +x "$work/step.sh"
cd "$work"
args=(-i src -i step.sh -o out -- ./step.sh)
t0=$(now_ms); "$bin" run -q "${args[@]}"; t1=$(now_ms)
rm -rf out
t2=$(now_ms); "$bin" run -q "${args[@]}"; t3=$(now_ms)
t4=$(now_ms); "$bin" explain "${args[@]}" > /dev/null; t5=$(now_ms)
echo changed > src/f0500.txt
t6=$(now_ms); "$bin" explain "${args[@]}" > "$work/why.txt" || true; t7=$(now_ms)
outbytes=$(du -sk out | cut -f1)
cat > "$res/memo-bench.json" <<JSON
{
  "inputs": 1000,
  "input_kib_each": 1,
  "outputs": 200,
  "output_kib_total": $outbytes,
  "miss_ms": $((t1 - t0)),
  "hit_ms": $((t3 - t2)),
  "explain_hit_ms": $((t5 - t4)),
  "explain_after_edit_ms": $((t7 - t6)),
  "explain_after_edit_says": "$(grep -m1 'input ' "$work/why.txt" | sed 's/^ *//')"
}
JSON
cat "$res/memo-bench.json"
cd "$root"
rm -rf "$work" "$(dirname "$bin")"
echo "recorded in $res"
