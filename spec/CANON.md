# BXM Canon format, version 1

BXM Canon is a binary encoding with exactly one valid encoding for every value.
Same value in, same bytes out, on any machine and in any language. A decoder
that follows this document rejects every byte string that is not the one
canonical encoding of some value.

This document is the whole format. It is short on purpose.

## Values

A Canon value is one of eight types:

| Tag    | Type   | Payload                                                     |
|--------|--------|-------------------------------------------------------------|
| `0x00` | null   | none                                                        |
| `0x01` | false  | none                                                        |
| `0x02` | true   | none                                                        |
| `0x03` | int    | zigzag-encoded signed 64-bit integer, as a varint           |
| `0x04` | float  | 8 bytes, IEEE 754 binary64, big-endian                      |
| `0x05` | bytes  | varint length, then that many bytes                         |
| `0x06` | string | varint length, then that many bytes of valid UTF-8          |
| `0x07` | array  | varint item count, then each item                           |
| `0x08` | map    | varint entry count, then each key (a string) and its value  |

Any other tag byte is an error.

An encoded document is exactly one value. Bytes after the value are an error.

## Varints

Lengths, counts and integers use unsigned LEB128: seven bits per byte, least
significant group first, high bit set on every byte except the last.

The encoding must be minimal. A varint whose last byte is `0x00` and that is
longer than one byte is an error (it has a redundant zero group). A varint
longer than ten bytes, or one whose value does not fit in 64 bits, is an error.

## Integers

An int is a signed 64-bit integer. It is zigzag-mapped to an unsigned value
(`0 → 0, -1 → 1, 1 → 2, -2 → 3, …`) and written as a varint.

Integers and floats are different types. `1` and `1.0` are different values
with different encodings.

## Floats

A float is written as its 8 IEEE 754 bytes, most significant byte first.

Three kinds of float are not values and must be rejected by both encoders and
decoders:

- NaN, in any of its bit patterns
- positive and negative infinity
- negative zero

Every other binary64 bit pattern, subnormals included, is a valid float.

## Strings and bytes

A string is UTF-8. Invalid UTF-8, including encoded surrogates and overlong
forms, is an error. Canon does not apply Unicode normalization: two strings
that look alike but differ in code points are different values. If your data
needs normalization, normalize before encoding.

Bytes are opaque.

## Arrays

Items keep their order.

## Maps

Keys are strings. Entries are sorted by the key's UTF-8 bytes, compared as
unsigned bytes, shorter key first when one key is a prefix of the other.

A map with keys out of order, or with the same key twice, is an error.

## Limits

Decoders must reject nesting deeper than 512 arrays and maps. A declared length
or count larger than the bytes left in the input is an error, so a decoder
never allocates for data that is not there.

## Text form

For humans and for JSON tools, Canon has a text form. It is JSON with two
rules and one addition:

- A number with no `.`, `e` or `E` is an int and must fit in 64 bits. Any
  other number is a float.
- A JSON object with the same key twice is an error rather than "last one
  wins".
- Bytes are written as `h"` followed by lowercase hex digits and `"`, for
  example `h"00ff"`. This is the one place the text form is not JSON.

Text is only a view. The bytes are the value, and hashes are taken over the
bytes.

## Hash

The Canon hash of a value is the SHA-256 of its encoding, written as
lowercase hex.

## Test vectors

`spec/vectors/valid.json` lists values with their encodings.
`spec/vectors/invalid.json` lists byte strings that every decoder must reject,
with the reason. An implementation is conforming when it passes both files.
