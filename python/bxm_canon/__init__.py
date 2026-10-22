"""BXM Canon: a binary encoding with exactly one valid encoding for every value.

Same value in, same bytes out, on any machine and in any language. The rules
run in the Rust core, the same code as the bxm-canon command, the C library
and the JavaScript package, so every one of them gives the same bytes.

    >>> import bxm_canon as canon
    >>> canon.hash_value({"b": 1, "a": 2}) == canon.hash_value({"a": 2, "b": 1})
    True
"""

from ._core import (
    CanonError,
    __version__,
    check,
    decode,
    diff_text,
    encode,
    fmt,
    from_text,
    hash,
    hash_text,
    hash_value,
    is_canonical,
    to_text,
)

__all__ = [
    "CanonError",
    "__version__",
    "check",
    "decode",
    "diff_text",
    "encode",
    "fmt",
    "from_text",
    "hash",
    "hash_text",
    "hash_value",
    "is_canonical",
    "to_text",
]
