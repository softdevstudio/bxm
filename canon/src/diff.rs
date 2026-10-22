//! Structural differences between two values, as a list of paths.

use crate::text::to_string;
use crate::Value;

/// One difference between two values.
#[derive(Clone, Debug, PartialEq)]
pub enum Change {
    /// Present only on the right.
    Added { path: String, value: Value },
    /// Present only on the left.
    Removed { path: String, value: Value },
    /// Present on both sides with different values.
    Changed {
        path: String,
        from: Value,
        to: Value,
    },
}

impl Change {
    /// One line per change: `+ path value`, `- path value` or `~ path from -> to`.
    pub fn line(&self) -> String {
        match self {
            Change::Added { path, value } => format!("+ {path} {}", to_string(value)),
            Change::Removed { path, value } => format!("- {path} {}", to_string(value)),
            Change::Changed { path, from, to } => {
                format!("~ {path} {} -> {}", to_string(from), to_string(to))
            }
        }
    }
}

/// Lists the differences from `a` to `b`. Paths look like `$.key[3].other`;
/// keys that are not plain identifiers are quoted: `$["a key"]`.
pub fn diff(a: &Value, b: &Value) -> Vec<Change> {
    let mut out = Vec::new();
    walk(a, b, "$".to_string(), &mut out);
    out
}

fn key_path(base: &str, k: &str) -> String {
    let plain = !k.is_empty()
        && k.chars().all(|c| c.is_ascii_alphanumeric() || c == '_')
        && !k.chars().next().unwrap().is_ascii_digit();
    if plain {
        format!("{base}.{k}")
    } else {
        let mut q = String::new();
        crate::text::quote(k, &mut q);
        format!("{base}[{q}]")
    }
}

fn walk(a: &Value, b: &Value, path: String, out: &mut Vec<Change>) {
    match (a, b) {
        (Value::Map(ma), Value::Map(mb)) => {
            for (k, va) in ma {
                match mb.get(k) {
                    Some(vb) => walk(va, vb, key_path(&path, k), out),
                    None => out.push(Change::Removed {
                        path: key_path(&path, k),
                        value: va.clone(),
                    }),
                }
            }
            for (k, vb) in mb {
                if !ma.contains_key(k) {
                    out.push(Change::Added {
                        path: key_path(&path, k),
                        value: vb.clone(),
                    });
                }
            }
        }
        (Value::Array(xa), Value::Array(xb)) => {
            let common = xa.len().min(xb.len());
            for i in 0..common {
                walk(&xa[i], &xb[i], format!("{path}[{i}]"), out);
            }
            for (i, v) in xa.iter().enumerate().skip(common) {
                out.push(Change::Removed {
                    path: format!("{path}[{i}]"),
                    value: v.clone(),
                });
            }
            for (i, v) in xb.iter().enumerate().skip(common) {
                out.push(Change::Added {
                    path: format!("{path}[{i}]"),
                    value: v.clone(),
                });
            }
        }
        _ => {
            if a != b {
                out.push(Change::Changed {
                    path,
                    from: a.clone(),
                    to: b.clone(),
                });
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::text::parse;

    #[test]
    fn lists_paths() {
        let a = parse(r#"{"a":1,"b":[1,2],"c d":true}"#).unwrap();
        let b = parse(r#"{"a":1.0,"b":[1],"e":null}"#).unwrap();
        let lines: Vec<String> = diff(&a, &b).iter().map(|c| c.line()).collect();
        assert_eq!(
            lines,
            vec![
                "~ $.a 1 -> 1.0",
                "- $.b[1] 2",
                r#"- $["c d"] true"#,
                "+ $.e null"
            ]
        );
        assert!(diff(&a, &a).is_empty());
    }
}
