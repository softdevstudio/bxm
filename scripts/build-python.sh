#!/usr/bin/env bash
# Builds the bxm-canon wheel for this machine, installs it in a fresh virtual
# environment and runs the Python tests against the installed copy.
# Usage: scripts/build-python.sh [extra maturin build args...]
set -euo pipefail
cd "$(dirname "$0")/.."
root=$PWD
cp LICENSE NOTICE python/
py=${PYTHON:-python3}
"$py" -m pip install -q "maturin>=1.5,<2"
rm -rf dist/python && mkdir -p dist/python
(cd python && "$py" -m maturin build --release -o "$root/dist/python" "$@")
venv=$(mktemp -d)/venv
"$py" -m venv "$venv"
bin="$venv/bin"; [ -d "$bin" ] || bin="$venv/Scripts"
"$bin/python" -m pip install -q --no-index --find-links "$root/dist/python" bxm-canon
# Run from outside the source folder so the installed package is the one tested.
(cd "$(dirname "$venv")" && "$bin/python" -m unittest discover -s "$root/python/tests" -v 2>&1 | tail -4)
ls dist/python
