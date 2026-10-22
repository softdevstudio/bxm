# Changelog

## 0.1.0

First release of both tools.

BXM Canon
- Format version 1: null, booleans, 64-bit ints, finite floats, bytes,
  strings, arrays and maps, each with one valid encoding.
- `bxm-canon` command: encode, decode, fmt, check, hash, diff.
- Rust library and a C API (static and shared library plus `bxm_canon.h`).
- Conformance vectors in `spec/vectors`.

BXM Memo
- `bxm-memo run`: restore outputs on a hit, run and store on a miss.
- Inputs as files, directories and patterns with `**`; declared outputs and
  environment variables.
- `explain`, `list`, `gc` and `clear`.
- Miss reasons, all-or-nothing restores, safe sharing between processes.
