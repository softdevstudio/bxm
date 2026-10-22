#!/usr/bin/env bash
# VERSION is the one place the version is set; the code must agree with it.
set -euo pipefail
cd "$(dirname "$0")/.."
v=$(tr -d '[:space:]' < VERSION)
cargo_v=$(sed -n 's/^version = "\(.*\)"/\1/p' canon/Cargo.toml | head -1)
go_v=$(sed -n 's/^const Version = "\(.*\)"/\1/p' memo/cmd/bxm-memo/main.go)
fail=0
[ "$cargo_v" = "$v" ] || { echo "canon/Cargo.toml has $cargo_v, VERSION has $v"; fail=1; }
[ "$go_v" = "$v" ] || { echo "memo main.go has $go_v, VERSION has $v"; fail=1; }
grep -q "^## $v" CHANGELOG.md || { echo "CHANGELOG.md has no section for $v"; fail=1; }
[ $fail = 0 ] && echo "version $v everywhere"
exit $fail
