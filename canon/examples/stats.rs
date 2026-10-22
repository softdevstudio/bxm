//! Prints the numbers recorded in test/results/canon-stats.json:
//! how many random and mutated byte strings were tried, how many the decoder
//! accepted, whether each accepted one re-encoded to the same bytes, and
//! encode/decode speed on a mixed document.
//!
//! cargo run --release --example stats -- [CASES]

use bxm_canon::{decode, encode, text, Value};
use std::time::Instant;

struct Rng(u64);
impl Rng {
    fn next(&mut self) -> u64 {
        self.0 ^= self.0 >> 12;
        self.0 ^= self.0 << 25;
        self.0 ^= self.0 >> 27;
        self.0.wrapping_mul(0x2545F4914F6CDD1D)
    }
    fn below(&mut self, n: u64) -> u64 {
        self.next() % n
    }
}

fn value(r: &mut Rng, depth: u32) -> Value {
    let top = if depth >= 4 { 6 } else { 8 };
    match r.below(top) {
        0 => Value::Null,
        1 => Value::Bool(r.below(2) == 1),
        2 => Value::Int(r.next() as i64 >> r.below(64)),
        3 => loop {
            let f = f64::from_bits(r.next());
            if f.is_finite() && !(f == 0.0 && f.is_sign_negative()) {
                break Value::Float(f);
            }
        },
        4 => Value::Bytes((0..r.below(8)).map(|_| r.next() as u8).collect()),
        5 => Value::String(
            ["", "a", "é", "key", "😀"][r.below(5) as usize].repeat(r.below(3) as usize),
        ),
        6 => Value::Array((0..r.below(4)).map(|_| value(r, depth + 1)).collect()),
        _ => Value::Map(
            (0..r.below(4))
                .map(|i| (format!("k{i}{}", r.below(3)), value(r, depth + 1)))
                .collect(),
        ),
    }
}

fn main() {
    let cases: u64 = std::env::args()
        .nth(1)
        .and_then(|s| s.parse().ok())
        .unwrap_or(1_000_000);
    let mut r = Rng(0x0123456789ABCDEF);
    let (mut accepted, mut mismatched) = (0u64, 0u64);
    for i in 0..cases {
        let input: Vec<u8> = if i % 2 == 0 {
            (0..r.below(12))
                .map(|_| r.below(12) as u8 | if r.below(4) == 0 { 0x80 } else { 0 })
                .collect()
        } else {
            let mut b = encode(&value(&mut r, 0)).unwrap();
            if !b.is_empty() {
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
        if decoded.is_ok() != bxm_canon::check(&input).is_ok() {
            mismatched += 1; // check and decode must agree
        }
        if let Ok(v) = decoded {
            accepted += 1;
            if encode(&v).unwrap() != input {
                mismatched += 1;
            }
        }
    }

    // A mixed document of about 1 MB: records with strings, ints, floats, bytes.
    let records: Vec<Value> = (0..8000)
        .map(|i| {
            Value::map([
                ("id", Value::Int(i)),
                ("name", Value::String(format!("record number {i}"))),
                ("score", Value::Float(i as f64 / 7.0)),
                (
                    "tags",
                    Value::Array(vec![
                        Value::String("alpha".into()),
                        Value::String("beta".into()),
                    ]),
                ),
                ("blob", Value::Bytes(vec![(i % 251) as u8; 64])),
                ("ok", Value::Bool(i % 2 == 0)),
            ])
        })
        .collect();
    let doc = Value::Array(records);
    let bytes = encode(&doc).unwrap();
    let mb = bytes.len() as f64 / 1e6;
    let reps = 50;
    let t = Instant::now();
    for _ in 0..reps {
        std::hint::black_box(encode(&doc).unwrap());
    }
    let enc = mb * reps as f64 / t.elapsed().as_secs_f64();
    let t = Instant::now();
    for _ in 0..reps {
        std::hint::black_box(decode(&bytes).unwrap());
    }
    let dec = mb * reps as f64 / t.elapsed().as_secs_f64();
    let t = Instant::now();
    for _ in 0..reps {
        bxm_canon::check(std::hint::black_box(&bytes)).unwrap();
    }
    let chk = mb * reps as f64 / t.elapsed().as_secs_f64();
    let json_len = text::to_string(&doc).len();

    println!("{{");
    println!("  \"cases\": {cases},");
    println!("  \"accepted\": {accepted},");
    println!("  \"accepted_but_not_canonical\": {mismatched},");
    println!("  \"doc_bytes\": {},", bytes.len());
    println!("  \"doc_text_bytes\": {json_len},");
    println!("  \"encode_mb_per_s\": {enc:.0},");
    println!("  \"decode_mb_per_s\": {dec:.0},");
    println!("  \"check_mb_per_s\": {chk:.0}");
    println!("}}");
    if mismatched > 0 {
        std::process::exit(1);
    }
}
