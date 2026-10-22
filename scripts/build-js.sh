#!/usr/bin/env bash
# Builds the bxm-canon npm package: the Canon core compiled to WebAssembly,
# wrapped in index.js (ES module) and index.cjs (CommonJS). Then runs its tests.
# Usage: scripts/build-js.sh [--pack]   (--pack also writes dist/bxm-canon-js.tgz)
set -euo pipefail
cd "$(dirname "$0")/.."
rustup target add wasm32-unknown-unknown >/dev/null 2>&1 || true
(cd js/wasm && cargo build --release --target wasm32-unknown-unknown)
wasm=js/wasm/target/wasm32-unknown-unknown/release/bxm_canon_wasm.wasm
node js/package/build.mjs "$wasm"
cp LICENSE NOTICE js/package/
(cd js/package && node --test test/*.test.mjs test/*.test.cjs)
if [ "${1:-}" = "--pack" ]; then
  mkdir -p dist
  (cd js/package && npm pack --silent --pack-destination ../../dist >/dev/null)
  mv dist/bxm-canon-*.tgz dist/bxm-canon-js.tgz
  echo "dist/bxm-canon-js.tgz"
fi
