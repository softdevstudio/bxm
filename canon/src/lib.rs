//! BXM Canon: a binary encoding with exactly one valid encoding for every value.
//!
//! Same value in, same bytes out. The decoder rejects every byte string that is
//! not the canonical encoding of some value, so "are these the same?" becomes
//! "are these bytes equal?". The format is described in `spec/CANON.md`.
//!
//! ```
//! use bxm_canon::{Value, encode, decode};
//! let v = Value::map([("b", Value::Int(2)), ("a", Value::Int(1))]);
//! let bytes = encode(&v).unwrap();
//! assert_eq!(decode(&bytes).unwrap(), v);
//! ```

use std::collections::BTreeMap;
use std::fmt;

pub mod diff;
pub mod ffi;
pub mod text;

/// Maximum nesting of arrays and maps.
pub const MAX_DEPTH: usize = 512;

/// Format version described by `spec/CANON.md`.
pub const FORMAT_VERSION: u32 = 1;

pub const TAG_NULL: u8 = 0x00;
pub const TAG_FALSE: u8 = 0x01;
pub const TAG_TRUE: u8 = 0x02;
pub const TAG_INT: u8 = 0x03;
pub const TAG_FLOAT: u8 = 0x04;
pub const TAG_BYTES: u8 = 0x05;
pub const TAG_STRING: u8 = 0x06;
pub const TAG_ARRAY: u8 = 0x07;
pub const TAG_MAP: u8 = 0x08;

/// A Canon value.
///
/// Maps are kept in a `BTreeMap`, whose order is the byte order of the keys,
/// which is the order Canon requires.
#[derive(Clone, Debug, PartialEq)]
pub enum Value {
    Null,
    Bool(bool),
    Int(i64),
    Float(f64),
    Bytes(Vec<u8>),
    String(String),
    Array(Vec<Value>),
    Map(BTreeMap<String, Value>),
}

impl Value {
    /// Builds a map from key/value pairs. A later duplicate key replaces an
    /// earlier one; use [`text::parse`] when duplicates must be an error.
    pub fn map<K: Into<String>, I: IntoIterator<Item = (K, Value)>>(entries: I) -> Value {
        Value::Map(entries.into_iter().map(|(k, v)| (k.into(), v)).collect())
    }

    /// Short name of the value's type, as used in error messages and diffs.
    pub fn type_name(&self) -> &'static str {
        match self {
            Value::Null => "null",
            Value::Bool(_) => "bool",
            Value::Int(_) => "int",
            Value::Float(_) => "float",
            Value::Bytes(_) => "bytes",
            Value::String(_) => "string",
            Value::Array(_) => "array",
            Value::Map(_) => "map",
        }
    }
}

/// What went wrong while encoding or decoding.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum ErrorKind {
    UnexpectedEnd,
    UnknownTag(u8),
    OverlongVarint,
    VarintTooLarge,
    IntOutOfRange,
    NaN,
    Infinity,
    NegativeZero,
    InvalidUtf8,
    LengthTooLarge,
    KeysOutOfOrder,
    DuplicateKey,
    TooDeep,
    TrailingBytes,
}

impl ErrorKind {
    /// Stable short code, used by the CLI, the C API and the test vectors.
    pub fn code(&self) -> &'static str {
        match self {
            ErrorKind::UnexpectedEnd => "unexpected-end",
            ErrorKind::UnknownTag(_) => "unknown-tag",
            ErrorKind::OverlongVarint => "overlong-varint",
            ErrorKind::VarintTooLarge => "varint-too-large",
            ErrorKind::IntOutOfRange => "int-out-of-range",
            ErrorKind::NaN => "nan",
            ErrorKind::Infinity => "infinity",
            ErrorKind::NegativeZero => "negative-zero",
            ErrorKind::InvalidUtf8 => "invalid-utf8",
            ErrorKind::LengthTooLarge => "length-too-large",
            ErrorKind::KeysOutOfOrder => "keys-out-of-order",
            ErrorKind::DuplicateKey => "duplicate-key",
            ErrorKind::TooDeep => "too-deep",
            ErrorKind::TrailingBytes => "trailing-bytes",
        }
    }
}

/// An encoding or decoding error, with the byte offset where it was found.
/// For encoding errors the offset is the position in the output so far.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Error {
    pub kind: ErrorKind,
    pub offset: usize,
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let what = match &self.kind {
            ErrorKind::UnexpectedEnd => "input ends in the middle of a value".to_string(),
            ErrorKind::UnknownTag(t) => format!("unknown tag byte 0x{t:02x}"),
            ErrorKind::OverlongVarint => "varint is not in its shortest form".to_string(),
            ErrorKind::VarintTooLarge => "varint does not fit in 64 bits".to_string(),
            ErrorKind::IntOutOfRange => "integer does not fit in 64 bits".to_string(),
            ErrorKind::NaN => "NaN is not a Canon value".to_string(),
            ErrorKind::Infinity => "infinity is not a Canon value".to_string(),
            ErrorKind::NegativeZero => "negative zero is not a Canon value".to_string(),
            ErrorKind::InvalidUtf8 => "string is not valid UTF-8".to_string(),
            ErrorKind::LengthTooLarge => "declared length is longer than the input".to_string(),
            ErrorKind::KeysOutOfOrder => "map keys are not in byte order".to_string(),
            ErrorKind::DuplicateKey => "map has the same key twice".to_string(),
            ErrorKind::TooDeep => format!("nesting deeper than {MAX_DEPTH}"),
            ErrorKind::TrailingBytes => "bytes left over after the value".to_string(),
        };
        write!(f, "{} at byte {}: {}", self.kind.code(), self.offset, what)
    }
}

impl std::error::Error for Error {}

fn check_float(f: f64, offset: usize) -> Result<(), Error> {
    let kind = if f.is_nan() {
        ErrorKind::NaN
    } else if f.is_infinite() {
        ErrorKind::Infinity
    } else if f == 0.0 && f.is_sign_negative() {
        ErrorKind::NegativeZero
    } else {
        return Ok(());
    };
    Err(Error { kind, offset })
}

fn put_varint(out: &mut Vec<u8>, mut v: u64) {
    loop {
        let byte = (v & 0x7f) as u8;
        v >>= 7;
        if v == 0 {
            out.push(byte);
            return;
        }
        out.push(byte | 0x80);
    }
}

fn zigzag(i: i64) -> u64 {
    ((i << 1) ^ (i >> 63)) as u64
}

fn unzigzag(u: u64) -> i64 {
    ((u >> 1) as i64) ^ -((u & 1) as i64)
}

/// Encodes a value. Fails only on floats that are not Canon values
/// (NaN, infinity, negative zero) or on nesting deeper than [`MAX_DEPTH`].
pub fn encode(v: &Value) -> Result<Vec<u8>, Error> {
    let mut out = Vec::new();
    encode_into(v, &mut out, 0)?;
    Ok(out)
}

fn encode_into(v: &Value, out: &mut Vec<u8>, depth: usize) -> Result<(), Error> {
    match v {
        Value::Null => out.push(TAG_NULL),
        Value::Bool(false) => out.push(TAG_FALSE),
        Value::Bool(true) => out.push(TAG_TRUE),
        Value::Int(i) => {
            out.push(TAG_INT);
            put_varint(out, zigzag(*i));
        }
        Value::Float(f) => {
            check_float(*f, out.len())?;
            out.push(TAG_FLOAT);
            out.extend_from_slice(&f.to_bits().to_be_bytes());
        }
        Value::Bytes(b) => {
            out.push(TAG_BYTES);
            put_varint(out, b.len() as u64);
            out.extend_from_slice(b);
        }
        Value::String(s) => {
            out.push(TAG_STRING);
            put_varint(out, s.len() as u64);
            out.extend_from_slice(s.as_bytes());
        }
        Value::Array(items) => {
            if depth >= MAX_DEPTH {
                return Err(Error {
                    kind: ErrorKind::TooDeep,
                    offset: out.len(),
                });
            }
            out.push(TAG_ARRAY);
            put_varint(out, items.len() as u64);
            for item in items {
                encode_into(item, out, depth + 1)?;
            }
        }
        Value::Map(entries) => {
            if depth >= MAX_DEPTH {
                return Err(Error {
                    kind: ErrorKind::TooDeep,
                    offset: out.len(),
                });
            }
            out.push(TAG_MAP);
            put_varint(out, entries.len() as u64);
            for (k, val) in entries {
                put_varint(out, k.len() as u64);
                out.extend_from_slice(k.as_bytes());
                encode_into(val, out, depth + 1)?;
            }
        }
    }
    Ok(())
}

/// Decodes one canonical value and requires that it uses the whole input.
pub fn decode(input: &[u8]) -> Result<Value, Error> {
    let mut d = Decoder {
        input,
        pos: 0,
        build: true,
    };
    let v = d.value(0)?;
    if d.pos != input.len() {
        return Err(Error {
            kind: ErrorKind::TrailingBytes,
            offset: d.pos,
        });
    }
    Ok(v)
}

/// Checks that the input is exactly one canonical value without building it.
/// Same rules as [`decode`].
pub fn check(input: &[u8]) -> Result<(), Error> {
    let mut d = Decoder {
        input,
        pos: 0,
        build: false,
    };
    d.value(0)?;
    if d.pos != input.len() {
        return Err(Error {
            kind: ErrorKind::TrailingBytes,
            offset: d.pos,
        });
    }
    Ok(())
}

/// Lowercase hex SHA-256 of the canonical encoding of `v`.
pub fn hash(v: &Value) -> Result<String, Error> {
    Ok(hash_bytes(&encode(v)?))
}

/// Lowercase hex SHA-256 of bytes that are already canonical.
pub fn hash_bytes(b: &[u8]) -> String {
    use sha2::{Digest, Sha256};
    to_hex(&Sha256::digest(b))
}

pub(crate) fn to_hex(b: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut s = String::with_capacity(b.len() * 2);
    for &x in b {
        s.push(HEX[(x >> 4) as usize] as char);
        s.push(HEX[(x & 15) as usize] as char);
    }
    s
}

/// One decoder for both `decode` and `check`, so the two can never disagree
/// on what is canonical. With `build` off it checks everything the same way
/// but allocates nothing and returns `Null` placeholders.
struct Decoder<'a> {
    input: &'a [u8],
    pos: usize,
    build: bool,
}

impl<'a> Decoder<'a> {
    fn err(&self, kind: ErrorKind, offset: usize) -> Error {
        Error { kind, offset }
    }

    fn byte(&mut self) -> Result<u8, Error> {
        match self.input.get(self.pos) {
            Some(&b) => {
                self.pos += 1;
                Ok(b)
            }
            None => Err(self.err(ErrorKind::UnexpectedEnd, self.pos)),
        }
    }

    fn varint(&mut self) -> Result<u64, Error> {
        let start = self.pos;
        let mut result: u64 = 0;
        let mut shift = 0u32;
        loop {
            let b = self.byte()?;
            let group = (b & 0x7f) as u64;
            if shift == 63 && group > 1 {
                return Err(self.err(ErrorKind::VarintTooLarge, start));
            }
            result |= group << shift;
            if b & 0x80 == 0 {
                if b == 0 && self.pos - start > 1 {
                    return Err(self.err(ErrorKind::OverlongVarint, start));
                }
                return Ok(result);
            }
            shift += 7;
            if shift > 63 {
                return Err(self.err(ErrorKind::VarintTooLarge, start));
            }
        }
    }

    fn length(&mut self) -> Result<usize, Error> {
        let start = self.pos;
        let n = self.varint()?;
        let left = (self.input.len() - self.pos) as u64;
        if n > left {
            return Err(self.err(ErrorKind::LengthTooLarge, start));
        }
        Ok(n as usize)
    }

    fn take(&mut self, n: usize) -> &'a [u8] {
        let s = &self.input[self.pos..self.pos + n];
        self.pos += n;
        s
    }

    fn str_slice(&mut self) -> Result<&'a str, Error> {
        let n = self.length()?;
        let start = self.pos;
        let raw = self.take(n);
        std::str::from_utf8(raw)
            .map_err(|e| self.err(ErrorKind::InvalidUtf8, start + e.valid_up_to()))
    }

    fn value(&mut self, depth: usize) -> Result<Value, Error> {
        let at = self.pos;
        let tag = self.byte()?;
        match tag {
            TAG_NULL => Ok(Value::Null),
            TAG_FALSE => Ok(Value::Bool(false)),
            TAG_TRUE => Ok(Value::Bool(true)),
            TAG_INT => Ok(Value::Int(unzigzag(self.varint()?))),
            TAG_FLOAT => {
                if self.input.len() - self.pos < 8 {
                    return Err(self.err(ErrorKind::UnexpectedEnd, self.input.len()));
                }
                let mut b = [0u8; 8];
                b.copy_from_slice(self.take(8));
                let f = f64::from_bits(u64::from_be_bytes(b));
                check_float(f, at)?;
                Ok(Value::Float(f))
            }
            TAG_BYTES => {
                let n = self.length()?;
                let b = self.take(n);
                Ok(if self.build {
                    Value::Bytes(b.to_vec())
                } else {
                    Value::Null
                })
            }
            TAG_STRING => {
                let s = self.str_slice()?;
                Ok(if self.build {
                    Value::String(s.to_owned())
                } else {
                    Value::Null
                })
            }
            TAG_ARRAY => {
                if depth >= MAX_DEPTH {
                    return Err(self.err(ErrorKind::TooDeep, at));
                }
                // Every item takes at least one byte, so the count is bounded
                // by the input left and `length` can check it.
                let n = self.length()?;
                let mut items = Vec::with_capacity(if self.build { n.min(4096) } else { 0 });
                for _ in 0..n {
                    let item = self.value(depth + 1)?;
                    if self.build {
                        items.push(item);
                    }
                }
                Ok(Value::Array(items))
            }
            TAG_MAP => {
                if depth >= MAX_DEPTH {
                    return Err(self.err(ErrorKind::TooDeep, at));
                }
                // Every entry takes at least two bytes (key length, value tag).
                let count_at = self.pos;
                let n = self.varint()?;
                if n > ((self.input.len() - self.pos) / 2) as u64 {
                    return Err(self.err(ErrorKind::LengthTooLarge, count_at));
                }
                let mut entries = BTreeMap::new();
                let mut prev: Option<&'a str> = None;
                for _ in 0..n {
                    let key_at = self.pos;
                    let key = self.str_slice()?;
                    if let Some(p) = prev {
                        match p.as_bytes().cmp(key.as_bytes()) {
                            std::cmp::Ordering::Less => {}
                            std::cmp::Ordering::Equal => {
                                return Err(self.err(ErrorKind::DuplicateKey, key_at))
                            }
                            std::cmp::Ordering::Greater => {
                                return Err(self.err(ErrorKind::KeysOutOfOrder, key_at))
                            }
                        }
                    }
                    let val = self.value(depth + 1)?;
                    prev = Some(key);
                    if self.build {
                        entries.insert(key.to_owned(), val);
                    }
                }
                Ok(Value::Map(entries))
            }
            other => Err(self.err(ErrorKind::UnknownTag(other), at)),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn varint_round_trip_and_zigzag() {
        for i in [0i64, 1, -1, 63, -64, 64, 1 << 40, i64::MAX, i64::MIN] {
            let b = encode(&Value::Int(i)).unwrap();
            assert_eq!(decode(&b).unwrap(), Value::Int(i), "{i}");
        }
        assert_eq!(encode(&Value::Int(-1)).unwrap(), vec![TAG_INT, 1]);
        assert_eq!(encode(&Value::Int(1)).unwrap(), vec![TAG_INT, 2]);
        assert_eq!(encode(&Value::Int(i64::MIN)).unwrap().len(), 11);
    }

    #[test]
    fn rejects_bad_floats_both_ways() {
        for f in [f64::NAN, f64::INFINITY, f64::NEG_INFINITY, -0.0] {
            assert!(encode(&Value::Float(f)).is_err());
            let mut b = vec![TAG_FLOAT];
            b.extend_from_slice(&f.to_bits().to_be_bytes());
            assert!(decode(&b).is_err());
        }
    }

    #[test]
    fn map_order_is_byte_order() {
        let v = Value::map([("b", Value::Null), ("a", Value::Null), ("ab", Value::Null)]);
        let b = encode(&v).unwrap();
        assert_eq!(
            b,
            vec![TAG_MAP, 3, 1, b'a', 0, 2, b'a', b'b', 0, 1, b'b', 0]
        );
    }

    #[test]
    fn huge_declared_lengths_do_not_allocate() {
        // A string claiming 2^63 bytes.
        let b = [
            TAG_STRING, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01,
        ];
        assert_eq!(decode(&b).unwrap_err().kind, ErrorKind::LengthTooLarge);
        let b = [TAG_ARRAY, 0xff, 0xff, 0xff, 0x0f];
        assert_eq!(decode(&b).unwrap_err().kind, ErrorKind::LengthTooLarge);
        let b = [TAG_MAP, 0xff, 0xff, 0xff, 0x0f];
        assert_eq!(decode(&b).unwrap_err().kind, ErrorKind::LengthTooLarge);
    }

    #[test]
    fn depth_limit() {
        let mut v = Value::Null;
        for _ in 0..MAX_DEPTH {
            v = Value::Array(vec![v]);
        }
        let b = encode(&v).unwrap();
        assert!(decode(&b).is_ok());
        let deeper = Value::Array(vec![v]);
        assert_eq!(encode(&deeper).unwrap_err().kind, ErrorKind::TooDeep);
        let mut b2 = vec![TAG_ARRAY, 1];
        b2.extend_from_slice(&b);
        assert_eq!(decode(&b2).unwrap_err().kind, ErrorKind::TooDeep);
    }
}
