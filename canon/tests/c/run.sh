#!/bin/sh
# Builds the static library and links a small C program against it.
set -eu
cd "$(dirname "$0")/../.."
cargo build --release --lib -q
cc -O2 -Iinclude tests/c/smoke.c target/release/libbxm_canon.a -lpthread -ldl -lm -o target/c-smoke
./target/c-smoke
