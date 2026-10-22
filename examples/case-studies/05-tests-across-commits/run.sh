#!/usr/bin/env bash
# Case study 5: the test suites of two packages in one repository, run at
# every commit of the repository's own history, with and without bxm-memo.
#
# The packages are Canon (cargo test) and Memo (go test). Each pass starts
# from a fresh clone and checks out each commit in turn. There are two
# settings, each run plain and through Memo:
#   fresh: the Rust target dir and Go build cache are emptied before every
#          commit, as on a CI runner that keeps nothing but Memo's own small
#          cache folder
#   warm:  the Rust target dir and Go build cache are kept between commits,
#          as on a developer machine or a CI runner that caches them
# Go has its own test cache; every pass turns it off with -count=1 so the
# numbers show what running the suites costs.
#
# Needs: git, cargo, go, bxm-memo on PATH. Usage: run.sh [REPO_URL] [LAST_COMMIT]
set -euo pipefail
repo=${1:-https://github.com/softdevstudio/bxm}
last=${2:-a32af04}
work=$(mktemp -d); trap 'rm -rf "$work"' EXIT
git clone -q "$repo" "$work/src"
commits=$(git -C "$work/src" rev-list --reverse "$last")
now() { date +%s.%N; }
# A failing suite is recorded, not fatal: CI would show it red and move on,
# and Memo never caches a failed run, so the next commit runs it again.
fail() {
  eval "$1=FAILED"
  { echo "== $1 tests failed at $c ($mode $setting)"; grep -E -- '^(--- FAIL|FAIL|test result: FAILED|panicked| +main_test|thread )' "logs/$1.txt" | head -10; } >> "$work/failures.txt"
}
# Download the Rust dependencies once, so no pass pays for the network.
cargo fetch -q --manifest-path "$work/src/canon/Cargo.toml"

pass() { # $1 = plain | memo, $2 = fresh | warm
  local mode=$1 setting=$2 dir="$work/$1-$2"
  git clone -q "$work/src" "$dir/repo"
  export CARGO_TARGET_DIR="$dir/target" GOCACHE="$dir/gocache" BXM_MEMO_CACHE="$dir/memo"
  local total=0 n=0
  for c in $commits; do
    n=$((n + 1))
    git -C "$dir/repo" checkout -q --detach "$c"
    if [ "$setting" = fresh ]; then rm -rf "$CARGO_TARGET_DIR" "$GOCACHE"; fi
    cd "$dir/repo"; mkdir -p logs
    local t0 t1 canon memo
    t0=$(now)
    if [ "$mode" = plain ]; then
      canon=ran; cargo test -q --manifest-path canon/Cargo.toml > logs/canon.txt 2>&1 || fail canon
      memo=ran; (cd memo && env -u BXM_MEMO_CACHE go test -count=1 ./... > ../logs/memo.txt 2>&1) || fail memo
    else
      canon=ran; out=$(bxm-memo run -n canon-tests -i 'canon/**' -i 'spec/vectors/**' -o logs/canon.txt \
        -- sh -c 'cargo test -q --manifest-path canon/Cargo.toml > logs/canon.txt 2>&1' 2>&1) || fail canon
      case "$out" in *"hit "*) canon=skipped;; esac
      memo=ran; out=$(bxm-memo run -n memo-tests -i 'memo/**' -o logs/memo.txt \
        -- sh -c 'cd memo && env -u BXM_MEMO_CACHE go test -count=1 ./... > ../logs/memo.txt 2>&1' 2>&1) || fail memo
      case "$out" in *"hit "*) memo=skipped;; esac
    fi
    local t2; t2=$(now)
    local secs; secs=$(echo "$t2 - $t0" | bc)
    total=$(echo "$total + $secs" | bc)
    printf '%-6s %-5s %2d %s  canon %-7s memo %-7s %6.1f s  %s\n' "$mode" "$setting" "$n" "${c:0:7}" "$canon" "$memo" "$secs" \
      "$(git log -1 --format=%s "$c" | cut -c1-48)"
    cd "$work"
  done
  printf '%-6s %-5s total %.1f s over %d commits\n' "$mode" "$setting" "$total" "$n"
}

: > "$work/failures.txt"
pass plain fresh
pass memo fresh
pass plain warm
pass memo warm
echo "test failures:"
if [ -s "$work/failures.txt" ]; then sed 's/^/  /' "$work/failures.txt"; else echo "  none"; fi
