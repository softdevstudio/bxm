//! The main promise, tested on many inputs: for any byte string, either the
//! decoder rejects it, or encoding the decoded value gives back exactly those
//! bytes. If that holds, no value has two accepted encodings.
//!
//! BXM_PROP_ITERS sets the number of cases per test (default 200000).

use bxm_canon::{decode, encode, text, Value};
use std::collections::BTreeMap;

struct Rng(u64);

impl Rng {
    fn next(&mut self) -> u64 {
        // xorshift64*
        self.0 ^= self.0 >> 12;
        self.0 ^= self.0 << 25;
        self.0 ^= self.0 >> 27;
        self.0.wrapping_mul(0x2545F4914F6CDD1D)
    }
    fn below(&mut self, n: u64) -> u64 {
        self.next() % n
    }
}

fn iters() -> u64 {
    std::env::var("BXM_PROP_ITERS")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(200_000)
}

fn random_string(r: &mut Rng) -> String {
    let pool = [
        "", "a", "b", "ab", "é", "z", "😀", "key", "\u{0}", "\"", "\\",
    ];
    let mut s = String::new();
    for _ in 0..r.below(3) {
        s.push_str(pool[r.below(pool.len() as u64) as usize]);
    }
    s
}

fn random_float(r: &mut Rng) -> f64 {
    loop {
        let f = match r.below(4) {
            0 => f64::from_bits(r.next()),
            1 => (r.below(2000) as f64 - 1000.0) / 8.0,
            2 => f64::from_bits(r.below(16)), // subnormals
            _ => [0.1, 1e300, -1e-300, 0.5][r.below(4) as usize],
        };
        if f.is_finite() && !(f == 0.0 && f.is_sign_negative()) {
            return f;
        }
    }
}

fn random_value(r: &mut Rng, depth: u32) -> Value {
    let top = if depth >= 4 { 6 } else { 8 };
    match r.below(top) {
        0 => Value::Null,
        1 => Value::Bool(r.below(2) == 1),
        2 => Value::Int(match r.below(3) {
            0 => r.below(300) as i64 - 150,
            1 => r.next() as i64,
            _ => [i64::MIN, i64::MAX, 0, -1][r.below(4) as usize],
        }),
        3 => Value::Float(random_float(r)),
        4 => Value::Bytes((0..r.below(6)).map(|_| r.next() as u8).collect()),
        5 => Value::String(random_string(r)),
        6 => Value::Array(
            (0..r.below(4))
                .map(|_| random_value(r, depth + 1))
                .collect(),
        ),
        _ => {
            let mut m = BTreeMap::new();
            for _ in 0..r.below(4) {
                m.insert(random_string(r), random_value(r, depth + 1));
            }
            Value::Map(m)
        }
    }
}

#[test]
fn values_round_trip_through_bytes_and_text() {
    let mut r = Rng(0x9E3779B97F4A7C15);
    for _ in 0..iters() {
        let v = random_value(&mut r, 0);
        let b = encode(&v).unwrap();
        assert_eq!(decode(&b).unwrap(), v);
        assert_eq!(text::parse(&text::to_string(&v)).unwrap(), v);
    }
}

#[test]
fn accepted_bytes_are_the_only_encoding() {
    let mut r = Rng(0xD1B54A32D192ED03);
    let mut accepted = 0u64;
    for i in 0..iters() {
        // Half random noise, half small mutations of a valid encoding.
        let input: Vec<u8> = if i % 2 == 0 {
            (0..r.below(12))
                .map(|_| r.below(12) as u8 | if r.below(4) == 0 { 0x80 } else { 0 })
                .collect()
        } else {
            let mut b = encode(&random_value(&mut r, 0)).unwrap();
            for _ in 0..=r.below(3) {
                if b.is_empty() {
                    break;
                }
                let at = r.below(b.len() as u64) as usize;
                match r.below(3) {
                    0 => b[at] ^= 1 << r.below(8),
                    1 => b[at] = r.next() as u8,
                    _ => {
                        b.remove(at);
                    }
                }
            }
            b
        };
        let decoded = decode(&input);
        assert_eq!(
            decoded.is_ok(),
            bxm_canon::check(&input).is_ok(),
            "check and decode disagree"
        );
        if let Ok(v) = decoded {
            accepted += 1;
            assert_eq!(
                encode(&v).unwrap(),
                input,
                "accepted a non-canonical encoding"
            );
        }
    }
    // Make sure the test is not vacuous.
    assert!(accepted > iters() / 50, "only {accepted} inputs accepted");
}

#[test]
fn key_order_in_text_does_not_change_the_bytes() {
    let a = text::parse(r#"{"z":1,"a":{"y":[1,2],"b":null},"m":"x"}"#).unwrap();
    let b = text::parse(r#"{"m":"x","a":{"b":null,"y":[1,2]},"z":1}"#).unwrap();
    assert_eq!(encode(&a).unwrap(), encode(&b).unwrap());
}
