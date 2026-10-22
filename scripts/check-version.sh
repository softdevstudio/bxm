#!/usr/bin/env bash
# VERSION is the one place the version is set; the code must agree with it.
set -euo pipefail
cd "$(dirname "$0")/.."
v=$(tr -d '[:space:]' < VERSION)
cargo_v=$(sed -n 's/^version = "\(.*\)"/\1/p' canon/Cargo.toml | head -1)
go_v=$(sed -n 's/^const Version = "\(.*\)"/\1/p' memo/cmd/bxm-memo/main.go)
js_v=$(sed -n 's/^  "version": "\(.*\)",/\1/p' js/package/package.json | head -1)
wasm_v=$(sed -n 's/^version = "\(.*\)"/\1/p' js/wasm/Cargo.toml | head -1)
fail=0
[ "$js_v" = "$v" ] || { echo "js/package/package.json has $js_v, VERSION has $v"; fail=1; }
[ "$wasm_v" = "$v" ] || { echo "js/wasm/Cargo.toml has $wasm_v, VERSION has $v"; fail=1; }
[ "$cargo_v" = "$v" ] || { echo "canon/Cargo.toml has $cargo_v, VERSION has $v"; fail=1; }
[ "$go_v" = "$v" ] || { echo "memo main.go has $go_v, VERSION has $v"; fail=1; }
grep -q "^## $v" CHANGELOG.md || { echo "CHANGELOG.md has no section for $v"; fail=1; }
[ $fail = 0 ] && echo "version $v everywhere"
exit $fail
