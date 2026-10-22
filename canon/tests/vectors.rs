//! Runs spec/vectors/*.json. Any Canon implementation should pass these files.

use bxm_canon::{decode, encode, text, Value};

fn load(name: &str) -> Value {
    let path = format!("{}/../spec/vectors/{name}", env!("CARGO_MANIFEST_DIR"));
    let s = std::fs::read_to_string(&path).unwrap_or_else(|e| panic!("{path}: {e}"));
    text::parse(&s).unwrap_or_else(|e| panic!("{path}: {e}"))
}

fn hex(s: &str) -> Vec<u8> {
    (0..s.len())
        .step_by(2)
        .map(|i| u8::from_str_radix(&s[i..i + 2], 16).unwrap())
        .collect()
}

fn field<'a>(v: &'a Value, k: &str) -> &'a str {
    match v {
        Value::Map(m) => match m.get(k) {
            Some(Value::String(s)) => s,
            other => panic!("{k}: {other:?}"),
        },
        _ => panic!("not a map"),
    }
}

#[test]
fn valid_vectors() {
    let Value::Array(cases) = load("valid.json") else {
        panic!()
    };
    assert!(cases.len() >= 30);
    for c in &cases {
        let t = field(c, "text");
        let want = hex(field(c, "hex"));
        let v = text::parse(t).unwrap_or_else(|e| panic!("{t}: {e}"));
        assert_eq!(encode(&v).unwrap(), want, "encode {t}");
        assert_eq!(decode(&want).unwrap(), v, "decode {t}");
        // Text written back reads as the same value.
        assert_eq!(text::parse(&text::to_string(&v)).unwrap(), v, "text {t}");
    }
}

#[test]
fn invalid_vectors() {
    let Value::Array(cases) = load("invalid.json") else {
        panic!()
    };
    assert!(cases.len() >= 28);
    for c in &cases {
        let h = field(c, "hex");
        let want = field(c, "error");
        match decode(&hex(h)) {
            Ok(v) => panic!("{h} decoded to {v:?}, want {want}"),
            Err(e) => assert_eq!(e.kind.code(), want, "{h}"),
        }
    }
}
