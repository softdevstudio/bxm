#!/usr/bin/env bash
# Case study 1: one order, three producers (Python, JavaScript, a file
# written by hand and hashed with the bxm-canon command).
# Needs: python3 with bxm_canon, node with bxm-canon, bxm-canon on PATH.
set -euo pipefail
cd "$(dirname "$0")"
PY=${PYTHON:-python3}
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

$PY producer.py > "$tmp/py.json"
node consumer.mjs "$tmp/py.json" > "$tmp/js.json"
file_plain=$(sha256sum order.json | cut -d' ' -f1)
file_canon=$(bxm-canon hash --text order.json)

$PY - "$tmp/py.json" "$tmp/js.json" "$file_plain" "$file_canon" <<'PY'
import json, sys
py = json.load(open(sys.argv[1])); js = json.load(open(sys.argv[2]))
fp, fc = sys.argv[3], sys.argv[4]
print("producer      plain JSON sha256 (first 16)   Canon sha256 (first 16)")
print(f"python        {py['plain_sha256'][:16]}               {py['canon_sha256'][:16]}")
print(f"javascript    {js['plain_sha256'][:16]}               {js['canon_sha256'][:16]}")
print(f"file + CLI    {fp[:16]}               {fc[:16]}")
plain = {py['plain_sha256'], js['plain_sha256'], fp}
canon = {py['canon_sha256'], js['canon_sha256'], fc}
print(f"distinct plain JSON hashes: {len(plain)}")
print(f"distinct Canon hashes: {len(canon)}")
print(f"canon bytes: python {py['canon_bytes']}, javascript {js['canon_bytes']}; plain JSON bytes: python {py['plain_bytes']}, javascript {js['plain_bytes']}")
print(f"python's HMAC over plain JSON verified in javascript: {js['plain_signature_ok']}")
print(f"python's HMAC over Canon bytes verified in javascript: {js['canon_signature_ok']}")
PY
