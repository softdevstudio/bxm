//! Canon text form: JSON, plus `h"…"` for bytes, with ints and floats kept apart
//! and duplicate keys rejected. See `spec/CANON.md`.

use crate::{Value, MAX_DEPTH};
use std::collections::BTreeMap;
use std::fmt;

/// A text parse error, with the byte offset in the text.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ParseError {
    pub message: String,
    pub offset: usize,
}

impl fmt::Display for ParseError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "text at byte {}: {}", self.offset, self.message)
    }
}

impl std::error::Error for ParseError {}

/// Parses Canon text (JSON with the Canon rules) into a value.
pub fn parse(text: &str) -> Result<Value, ParseError> {
    let mut p = Parser {
        s: text.as_bytes(),
        pos: 0,
    };
    p.ws();
    let v = p.value(0)?;
    p.ws();
    if p.pos != p.s.len() {
        return Err(p.err("unexpected text after the value"));
    }
    Ok(v)
}

struct Parser<'a> {
    s: &'a [u8],
    pos: usize,
}

impl<'a> Parser<'a> {
    fn err(&self, m: &str) -> ParseError {
        ParseError {
            message: m.to_string(),
            offset: self.pos,
        }
    }

    fn ws(&mut self) {
        while let Some(&c) = self.s.get(self.pos) {
            if c == b' ' || c == b'\t' || c == b'\n' || c == b'\r' {
                self.pos += 1;
            } else {
                break;
            }
        }
    }

    fn peek(&self) -> Option<u8> {
        self.s.get(self.pos).copied()
    }

    fn lit(&mut self, word: &str, v: Value) -> Result<Value, ParseError> {
        if self.s[self.pos..].starts_with(word.as_bytes()) {
            self.pos += word.len();
            Ok(v)
        } else {
            Err(self.err("unexpected character"))
        }
    }

    fn value(&mut self, depth: usize) -> Result<Value, ParseError> {
        match self.peek() {
            None => Err(self.err("text ends where a value was expected")),
            Some(b'n') => self.lit("null", Value::Null),
            Some(b't') => self.lit("true", Value::Bool(true)),
            Some(b'f') => self.lit("false", Value::Bool(false)),
            Some(b'"') => Ok(Value::String(self.string()?)),
            Some(b'h') => self.hex_bytes(),
            Some(b'[') => {
                if depth >= MAX_DEPTH {
                    return Err(self.err("nesting is too deep"));
                }
                self.pos += 1;
                let mut items = Vec::new();
                self.ws();
                if self.peek() == Some(b']') {
                    self.pos += 1;
                    return Ok(Value::Array(items));
                }
                loop {
                    self.ws();
                    items.push(self.value(depth + 1)?);
                    self.ws();
                    match self.peek() {
                        Some(b',') => self.pos += 1,
                        Some(b']') => {
                            self.pos += 1;
                            return Ok(Value::Array(items));
                        }
                        _ => return Err(self.err("expected , or ]")),
                    }
                }
            }
            Some(b'{') => {
                if depth >= MAX_DEPTH {
                    return Err(self.err("nesting is too deep"));
                }
                self.pos += 1;
                let mut map = BTreeMap::new();
                self.ws();
                if self.peek() == Some(b'}') {
                    self.pos += 1;
                    return Ok(Value::Map(map));
                }
                loop {
                    self.ws();
                    if self.peek() != Some(b'"') {
                        return Err(self.err("map keys must be strings"));
                    }
                    let key_at = self.pos;
                    let k = self.string()?;
                    self.ws();
                    if self.peek() != Some(b':') {
                        return Err(self.err("expected :"));
                    }
                    self.pos += 1;
                    self.ws();
                    let v = self.value(depth + 1)?;
                    if map.contains_key(&k) {
                        return Err(ParseError {
                            message: format!("the key {:?} appears twice", k),
                            offset: key_at,
                        });
                    }
                    map.insert(k, v);
                    self.ws();
                    match self.peek() {
                        Some(b',') => self.pos += 1,
                        Some(b'}') => {
                            self.pos += 1;
                            return Ok(Value::Map(map));
                        }
                        _ => return Err(self.err("expected , or }")),
                    }
                }
            }
            Some(c) if c == b'-' || c.is_ascii_digit() => self.number(),
            Some(_) => Err(self.err("unexpected character")),
        }
    }

    fn hex_bytes(&mut self) -> Result<Value, ParseError> {
        self.pos += 1;
        if self.peek() != Some(b'"') {
            return Err(self.err("expected \" after h"));
        }
        self.pos += 1;
        let start = self.pos;
        while let Some(c) = self.peek() {
            if c == b'"' {
                break;
            }
            if !(c.is_ascii_digit() || (b'a'..=b'f').contains(&c)) {
                return Err(self.err("bytes must be lowercase hex digits"));
            }
            self.pos += 1;
        }
        if self.peek() != Some(b'"') {
            return Err(self.err("bytes are not closed with \""));
        }
        let hex = &self.s[start..self.pos];
        if hex.len() % 2 != 0 {
            return Err(self.err("bytes need an even number of hex digits"));
        }
        self.pos += 1;
        let val = |c: u8| {
            if c.is_ascii_digit() {
                c - b'0'
            } else {
                c - b'a' + 10
            }
        };
        Ok(Value::Bytes(
            hex.chunks(2).map(|p| val(p[0]) << 4 | val(p[1])).collect(),
        ))
    }

    fn number(&mut self) -> Result<Value, ParseError> {
        let start = self.pos;
        if self.peek() == Some(b'-') {
            self.pos += 1;
        }
        match self.peek() {
            Some(b'0') => {
                self.pos += 1;
                if matches!(self.peek(), Some(c) if c.is_ascii_digit()) {
                    return Err(self.err("numbers cannot have leading zeros"));
                }
            }
            Some(c) if c.is_ascii_digit() => {
                while matches!(self.peek(), Some(c) if c.is_ascii_digit()) {
                    self.pos += 1;
                }
            }
            _ => return Err(self.err("expected a digit")),
        }
        let mut is_float = false;
        if self.peek() == Some(b'.') {
            is_float = true;
            self.pos += 1;
            if !matches!(self.peek(), Some(c) if c.is_ascii_digit()) {
                return Err(self.err("expected a digit after ."));
            }
            while matches!(self.peek(), Some(c) if c.is_ascii_digit()) {
                self.pos += 1;
            }
        }
        if matches!(self.peek(), Some(b'e') | Some(b'E')) {
            is_float = true;
            self.pos += 1;
            if matches!(self.peek(), Some(b'+') | Some(b'-')) {
                self.pos += 1;
            }
            if !matches!(self.peek(), Some(c) if c.is_ascii_digit()) {
                return Err(self.err("expected a digit in the exponent"));
            }
            while matches!(self.peek(), Some(c) if c.is_ascii_digit()) {
                self.pos += 1;
            }
        }
        let text = std::str::from_utf8(&self.s[start..self.pos]).unwrap();
        if is_float {
            let f: f64 = text.parse().map_err(|_| ParseError {
                message: "not a number".into(),
                offset: start,
            })?;
            let bad = if f.is_infinite() {
                Some("float is too large (it would be infinity)")
            } else if f == 0.0 && f.is_sign_negative() {
                Some("negative zero is not a Canon value")
            } else {
                None
            };
            if let Some(m) = bad {
                return Err(ParseError {
                    message: m.into(),
                    offset: start,
                });
            }
            Ok(Value::Float(f))
        } else {
            text.parse::<i64>().map(Value::Int).map_err(|_| ParseError {
                message: "integer does not fit in 64 bits (write it with .0 for a float)".into(),
                offset: start,
            })
        }
    }

    fn hex4(&mut self) -> Result<u32, ParseError> {
        if self.pos + 4 > self.s.len() {
            return Err(self.err("short \\u escape"));
        }
        let digits = &self.s[self.pos..self.pos + 4];
        if !digits.iter().all(|c| c.is_ascii_hexdigit()) {
            return Err(self.err("bad \\u escape"));
        }
        let h = u32::from_str_radix(std::str::from_utf8(digits).unwrap(), 16).unwrap();
        self.pos += 4;
        Ok(h)
    }

    fn string(&mut self) -> Result<String, ParseError> {
        self.pos += 1; // opening quote
        let mut out: Vec<u8> = Vec::new();
        loop {
            let c = match self.peek() {
                None => return Err(self.err("string is not closed")),
                Some(c) => c,
            };
            self.pos += 1;
            match c {
                b'"' => break,
                b'\\' => {
                    let e = self
                        .peek()
                        .ok_or_else(|| self.err("string is not closed"))?;
                    self.pos += 1;
                    match e {
                        b'"' => out.push(b'"'),
                        b'\\' => out.push(b'\\'),
                        b'/' => out.push(b'/'),
                        b'b' => out.push(8),
                        b'f' => out.push(12),
                        b'n' => out.push(b'\n'),
                        b'r' => out.push(b'\r'),
                        b't' => out.push(b'\t'),
                        b'u' => {
                            let at = self.pos - 2;
                            let mut cp = self.hex4()?;
                            if (0xD800..0xDC00).contains(&cp) {
                                if self.s[self.pos..].starts_with(b"\\u") {
                                    self.pos += 2;
                                    let lo = self.hex4()?;
                                    if !(0xDC00..0xE000).contains(&lo) {
                                        return Err(ParseError {
                                            message: "unpaired surrogate in \\u escape".into(),
                                            offset: at,
                                        });
                                    }
                                    cp = 0x10000 + ((cp - 0xD800) << 10) + (lo - 0xDC00);
                                } else {
                                    return Err(ParseError {
                                        message: "unpaired surrogate in \\u escape".into(),
                                        offset: at,
                                    });
                                }
                            } else if (0xDC00..0xE000).contains(&cp) {
                                return Err(ParseError {
                                    message: "unpaired surrogate in \\u escape".into(),
                                    offset: at,
                                });
                            }
                            let ch = char::from_u32(cp).unwrap();
                            let mut buf = [0u8; 4];
                            out.extend_from_slice(ch.encode_utf8(&mut buf).as_bytes());
                        }
                        _ => return Err(self.err("unknown escape")),
                    }
                }
                c if c < 0x20 => return Err(self.err("control character in string")),
                c => out.push(c),
            }
        }
        // The input is &str, so raw bytes are valid UTF-8 and escapes produce
        // valid UTF-8; this cannot fail.
        Ok(String::from_utf8(out).expect("valid utf-8"))
    }
}

/// Writes a value as compact Canon text.
pub fn to_string(v: &Value) -> String {
    let mut s = String::new();
    write(v, &mut s, None, 0);
    s
}

/// Writes a value as indented Canon text.
pub fn to_string_pretty(v: &Value) -> String {
    let mut s = String::new();
    write(v, &mut s, Some(2), 0);
    s
}

/// Text for a float: Rust's shortest round-trip form, always with `.` or `e`
/// so it reads back as a float.
pub fn float_text(f: f64) -> String {
    let t = format!("{f:?}");
    if t.contains('.') || t.contains('e') || t.contains("inf") || t.contains("NaN") {
        t
    } else {
        format!("{t}.0")
    }
}

pub(crate) fn quote(s: &str, out: &mut String) {
    out.push('"');
    for ch in s.chars() {
        match ch {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
}

fn newline(out: &mut String, indent: Option<usize>, level: usize) {
    if let Some(n) = indent {
        out.push('\n');
        for _ in 0..n * level {
            out.push(' ');
        }
    }
}

fn write(v: &Value, out: &mut String, indent: Option<usize>, level: usize) {
    match v {
        Value::Null => out.push_str("null"),
        Value::Bool(b) => out.push_str(if *b { "true" } else { "false" }),
        Value::Int(i) => out.push_str(&i.to_string()),
        Value::Float(f) => out.push_str(&float_text(*f)),
        Value::Bytes(b) => {
            out.push_str("h\"");
            out.push_str(&crate::to_hex(b));
            out.push('"');
        }
        Value::String(s) => quote(s, out),
        Value::Array(items) => {
            out.push('[');
            for (i, item) in items.iter().enumerate() {
                if i > 0 {
                    out.push(',');
                }
                newline(out, indent, level + 1);
                write(item, out, indent, level + 1);
            }
            if !items.is_empty() {
                newline(out, indent, level);
            }
            out.push(']');
        }
        Value::Map(entries) => {
            out.push('{');
            for (i, (k, val)) in entries.iter().enumerate() {
                if i > 0 {
                    out.push(',');
                }
                newline(out, indent, level + 1);
                quote(k, out);
                out.push(':');
                if indent.is_some() {
                    out.push(' ');
                }
                write(val, out, indent, level + 1);
            }
            if !entries.is_empty() {
                newline(out, indent, level);
            }
            out.push('}');
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ints_and_floats_stay_apart() {
        assert_eq!(parse("1").unwrap(), Value::Int(1));
        assert_eq!(parse("1.0").unwrap(), Value::Float(1.0));
        assert_eq!(parse("1e2").unwrap(), Value::Float(100.0));
        assert!(parse("9223372036854775808").is_err());
        assert_eq!(parse("-9223372036854775808").unwrap(), Value::Int(i64::MIN));
    }

    #[test]
    fn duplicate_keys_are_errors() {
        let e = parse(r#"{"a":1,"a":2}"#).unwrap_err();
        assert!(e.message.contains("twice"));
    }

    #[test]
    fn bytes_and_round_trip() {
        let v = parse(r#"{"k":h"00ff","s":"é\n","f":0.1,"n":[null,true,-3]}"#).unwrap();
        let t = to_string(&v);
        assert_eq!(parse(&t).unwrap(), v);
        assert_eq!(parse(&to_string_pretty(&v)).unwrap(), v);
        assert!(t.contains(r#"h"00ff""#));
    }

    #[test]
    fn rejects_negative_zero_and_huge() {
        assert!(parse("-0.0").is_err());
        assert!(parse("1e999").is_err());
        assert_eq!(parse("-0").unwrap(), Value::Int(0));
    }

    #[test]
    fn surrogates() {
        assert_eq!(parse(r#""😀""#).unwrap(), Value::String("😀".into()));
        assert!(parse(r#""\ud83d""#).is_err());
        assert!(parse(r#""\ude00""#).is_err());
        assert!(parse(r#""\u+041""#).is_err());
    }
}
