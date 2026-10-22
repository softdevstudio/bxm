# BXM

Two small tools that share one idea: same input, same bytes, same result.

- **BXM Canon** is a binary encoding with exactly one valid encoding for every
  value. Hash it, sign it, compare it, and the answer never depends on who
  wrote the bytes. Written in Rust, with a C API so other languages can use the
  one implementation.
- **BXM Memo** skips a command when its inputs have not changed. Put
  `bxm-memo run` in front of a build step, say what it reads and writes, and
  the second run restores the outputs instead of doing the work again. Written
  in Go, one binary, no setup.

Site: [bxm.net](https://bxm.net). License: Apache 2.0. Made by
[SoftDevStudio.com](https://softdevstudio.com).

## Install

Download the archive for your platform from the
[latest release](https://github.com/softdevstudio/bxm/releases/latest). Each
archive holds both tools, the Canon library and header, the spec and the
licenses.

| Platform       | Archive                                                                                                   |
|----------------|-----------------------------------------------------------------------------------------------------------|
| Linux x86-64   | [bxm-linux-amd64.tar.gz](https://github.com/softdevstudio/bxm/releases/latest/download/bxm-linux-amd64.tar.gz)   |
| Linux ARM64    | [bxm-linux-arm64.tar.gz](https://github.com/softdevstudio/bxm/releases/latest/download/bxm-linux-arm64.tar.gz)   |
| macOS (Apple)  | [bxm-darwin-arm64.tar.gz](https://github.com/softdevstudio/bxm/releases/latest/download/bxm-darwin-arm64.tar.gz) |
| macOS (Intel)  | [bxm-darwin-amd64.tar.gz](https://github.com/softdevstudio/bxm/releases/latest/download/bxm-darwin-amd64.tar.gz) |
| Windows x86-64 | [bxm-windows-amd64.zip](https://github.com/softdevstudio/bxm/releases/latest/download/bxm-windows-amd64.zip)     |

`SHA256SUMS` sits next to them. To build from source you need Rust (stable)
and Go 1.22 or later:

```sh
cd canon && cargo build --release      # target/release/bxm-canon
cd memo && go build ./cmd/bxm-memo     # ./bxm-memo
```

---

## BXM Canon

### The problem

Sign a JSON object on one machine and check the signature on another, and it
can fail even though nothing changed. One side wrote the keys in a different
order. Or `1.0` came out as `1`. Or a string used `é` where the other
wrote `é`. Same data, different bytes, broken hash.

The usual fix is a canonicalization step bolted on after the fact, and every
language has to get every rule exactly right. Most bugs in this area are two
implementations disagreeing on one corner case.

### What Canon does

Canon has one correct byte form for every value, and the decoder refuses
everything else. Key order, number spelling and padding are not choices. If a
byte string decodes at all, re-encoding the value gives back exactly the same
bytes. So "are these the same value?" becomes "are these bytes equal?", and a
hash or signature stays valid wherever the data goes.

Values are what JSON has, plus raw bytes, with integers and floats kept apart:
null, true, false, 64-bit integers, 64-bit floats, bytes, strings, arrays and
maps with string keys. NaN, infinity and negative zero are not values. The
whole format fits on two pages: [spec/CANON.md](spec/CANON.md).

### Command line

```sh
# JSON (Canon text) in, canonical bytes out
echo '{"name":"bxm","tags":["a","b"],"n":3}' | bxm-canon encode -o v.bxm

# bytes back to text
bxm-canon decode --pretty v.bxm

# the same value always hashes the same, whatever the key order
echo '{"b":1,"a":2}' | bxm-canon hash --text
echo '{ "a": 2, "b": 1 }' | bxm-canon hash --text

# is this file canonical? (exit 1 if not, with the reason and byte offset)
bxm-canon check v.bxm

# what changed between two values
bxm-canon diff --text old.json new.json
#   ~ $.version 3 -> 4
#   + $.tags[2] "c"

# rewrite text in its one normal form (sorted keys, no spaces)
bxm-canon fmt messy.json
```

Canon text is JSON with three rules: a number without `.` or `e` is an
integer and must fit in 64 bits; a repeated key is an error, not "last one
wins"; and bytes are written `h"00ff"`.

Exit codes: 0 fine, 1 for `check` or `diff` finding a problem, 2 for errors.

### Library

Rust:

```rust
use bxm_canon::{text, encode, decode, hash};
let v = text::parse(r#"{"b":1,"a":[true,h"ff"]}"#)?;
let bytes = encode(&v)?;           // canonical bytes
let same = decode(&bytes)?;        // rejects anything non-canonical
let h = hash(&v)?;                 // SHA-256 hex of the bytes
```

C, and anything with a C FFI: link `lib/libbxm_canon.a` (or the shared
library) and include `include/bxm_canon.h`.

```c
uint8_t *out; size_t n; char *err = NULL;
if (bxm_canon_from_text((const uint8_t *)text, len, &out, &n, &err) != BXM_OK) {
    fprintf(stderr, "%s\n", err);
    bxm_canon_free_str(err);
    return 1;
}
char hex[65];
bxm_canon_hash(out, n, hex, NULL);
bxm_canon_free(out, n);
```

JavaScript (WebAssembly) and Python packages built on the same core are next
on the list.

### Conformance

[spec/vectors/valid.json](spec/vectors/valid.json) lists values and their
exact bytes. [spec/vectors/invalid.json](spec/vectors/invalid.json) lists byte
strings every decoder must reject, each with its reason. Another
implementation that passes both files agrees with this one.

---

## BXM Memo

### The problem

Build and CI pipelines redo work whose inputs did not change: code
generation, asset processing, test fixtures, site builds, data preparation.
`make` decides by file timestamps, which break on a fresh clone or in CI.
Bazel decides properly but asks you to restructure the project around it.
Most teams sit in between and pay the time on every run.

### What Memo does

```sh
bxm-memo run -i 'proto/**/*.proto' -i gen.sh -o gen/ -- ./gen.sh
```

Memo hashes the declared inputs (file contents, not timestamps), the command
line and any declared environment variables. If it has seen that exact
combination before, it puts the outputs back and replays what the command
printed. If not, it runs the command, and when the command succeeds it keeps
the outputs for next time.

On a miss it tells you why:

```
bxm-memo: miss gen.sh: input proto/user.proto changed
```

### Flags

| Flag              | Meaning                                                              |
|-------------------|----------------------------------------------------------------------|
| `-i, --in PATH`   | an input: a file, a directory (all files under it) or a pattern with `*`, `?`, `[..]`, `**`. Repeat for more. |
| `-o, --out PATH`  | an output file or directory the command writes. Repeat for more.     |
| `-e, --env NAME`  | an environment variable that changes the result                      |
| `-n, --name NAME` | a name for the job, used when explaining misses                      |
| `--salt TEXT`     | extra text in the key, to throw away old results on purpose         |
| `--cache DIR`     | where results live; default `$BXM_MEMO_CACHE` or the user cache dir |
| `-q, --quiet`     | no hit or miss lines                                                 |

Other commands:

```sh
bxm-memo explain -i src -o out -- ./build.sh   # hit or miss, and why; runs nothing
bxm-memo list                                   # cached results, newest first
bxm-memo gc --older-than 30d --max-size 2G      # trim the cache
bxm-memo clear                                  # delete the whole cache
```

### Rules worth knowing

- A command that fails is never cached. Its exit code passes through.
  `bxm-memo` uses exit code 125 for its own errors.
- An input pattern that matches no files is an error, so a typo cannot
  silently make every run a hit.
- A directory output is restored exactly: files the cached run did not
  produce are removed.
- Restores are all or nothing. Every file is checked against its hash before
  any output is replaced, so a damaged cache leads to a normal run, never to
  half-restored outputs.
- If an input changes while the command runs, the result is not stored.
- Results are keyed per OS and CPU type.
- Many processes can share one cache at the same time.
- Outputs must be paths inside the working directory: not `.`, not `..` and
  not absolute, since a hit replaces them.
- Symlinks inside input directories are followed. A broken link is an error,
  so an input can't silently drop out of the key.
- Patterns match dotfiles too, unlike a shell. A file name containing `*`,
  `?` or `[` is read as a pattern.
- On a hit, stdout is replayed, then stderr. Their original interleaving is
  not kept.
- `gc` leaves files younger than an hour alone, since another run may be
  storing them right now.

### In CI

Point `BXM_MEMO_CACHE` at a directory your CI keeps between runs (for example
with `actions/cache`) and wrap the slow steps.

---

## How it was tested

`scripts/record-results.sh` runs every test and measurement and writes the
output to [test/results](test/results). The numbers on bxm.net come from those
files.

- Canon: the spec's test vectors; a million random and mutated byte strings,
  checking that every one the decoder accepts re-encodes to exactly the same
  bytes; random values round-tripped through bytes and text; a C program
  linked against the library.
- Memo: hits, misses and their reasons; failing commands; missing outputs;
  damaged cache objects; inputs edited mid-run; 16 processes sharing a cache;
  a run killed with SIGKILL; garbage collection.

CI runs both on Linux and macOS on every push.

## License

Apache 2.0. See [LICENSE](LICENSE), [NOTICE](NOTICE) and
[THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).
