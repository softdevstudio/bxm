# bxm-canon

BXM Canon for Python: a binary encoding with exactly one valid encoding for
every value. Same data in, same bytes out, on any machine. The decoder refuses
everything else, so "are these the same?" becomes "are these bytes equal?".

This package runs the Canon core compiled from the same Rust code as the
`bxm-canon` command, the C library and the JavaScript package. It gives the
same bytes and the same hashes, because it is the same code. Wheels for Linux,
macOS and Windows, Python 3.9 and later, no dependencies.

```python
import bxm_canon as canon

canon.hash_value({"b": 1, "a": 2}) == canon.hash_value({"a": 2, "b": 1})  # True

data = canon.encode({"name": "bxm", "raw": b"\xca\xfe", "n": 3, "x": 1.0})
canon.decode(data)        # {'n': 3, 'name': 'bxm', 'raw': b'\xca\xfe', 'x': 1.0}
canon.is_canonical(data)  # True
canon.to_text(data)       # '{"n":3,"name":"bxm","raw":h"cafe","x":1.0}'
```

## Python values

| Python | Canon |
|---|---|
| `None` | null |
| `bool` | true / false |
| `int` (64-bit) | integer; larger ones raise `OverflowError` |
| `float` | float; NaN, infinity and `-0.0` raise `CanonError` |
| `str` | string; a lone surrogate raises `CanonError` |
| `bytes`, `bytearray`, `memoryview` | bytes |
| `list`, `tuple` | array (decoded as `list`) |
| `dict` with `str` keys | map (decoded with keys in byte order) |

Python keeps `1` and `1.0` apart, and so does Canon, so they encode
differently. Anything else raises `TypeError`.

## Functions

| Function | What it does |
|---|---|
| `encode(obj)` | Python value to canonical bytes |
| `decode(data)` | canonical bytes to a Python value; raises `CanonError` if not canonical |
| `check(data)` | raises `CanonError` unless the bytes are one canonical value |
| `is_canonical(data)` | the same check as `True` or `False` |
| `hash(data)` | SHA-256 hex of canonical bytes |
| `hash_value(obj)` | SHA-256 hex of a Python value's canonical encoding |
| `from_text(text)` | Canon text (JSON plus `h"hex"` for bytes) to bytes |
| `to_text(data, pretty=False)` | canonical bytes to text |
| `fmt(text, pretty=False)` | text to its one normal form |
| `hash_text(text)` | SHA-256 hex of a text value's canonical encoding |
| `diff_text(a, b)` | the differences between two text values, one line each |

`CanonError` is a `ValueError` with `code` (for example `overlong-varint`,
`keys-out-of-order`, or `text`) and the byte `offset` where it was found.

## More

- Format spec and conformance vectors: https://github.com/softdevstudio/bxm/tree/main/spec
- Site: https://bxm.net/canon/

License: Apache 2.0. A [SoftDevStudio.com](https://softdevstudio.com) project.
