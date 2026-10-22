from typing import Any, Dict, List, Tuple, Union

Bytes = Union[bytes, bytearray, memoryview]
Value = Union[None, bool, int, float, str, bytes, List["Value"], Tuple["Value", ...], Dict[str, "Value"]]

__version__: str

class CanonError(ValueError):
    """An error from Canon."""
    code: str
    """Stable short name, for example "overlong-varint", "keys-out-of-order", or "text"."""
    offset: int
    """Byte offset where the problem was found, or -1."""

def encode(obj: Value) -> bytes:
    """Encode a Python value as canonical bytes."""
def decode(data: Bytes) -> Any:
    """Decode canonical bytes into a Python value. Raises CanonError if they are not canonical."""
def check(data: Bytes) -> None:
    """Raise CanonError unless the bytes are exactly one canonical value."""
def is_canonical(data: Bytes) -> bool:
    """True when the bytes are exactly one canonical value."""
def hash(data: Bytes) -> str:
    """Lowercase hex SHA-256 of canonical bytes (checked first)."""
def hash_value(obj: Value) -> str:
    """Lowercase hex SHA-256 of the canonical encoding of a Python value."""
def from_text(t: str) -> bytes:
    """Canon text (JSON plus h"hex" for bytes) to canonical bytes."""
def to_text(data: Bytes, pretty: bool = False) -> str:
    """Canonical bytes to Canon text."""
def fmt(t: str, pretty: bool = False) -> str:
    """Text to its one normal form: keys sorted, no spaces (indented with pretty)."""
def hash_text(t: str) -> str:
    """Lowercase hex SHA-256 of the canonical encoding of a text value."""
def diff_text(a: str, b: str) -> List[str]:
    """Differences between two text values, one change per line."""
