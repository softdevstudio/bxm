use bxm_canon::{check, decode, diff, encode, hash_bytes, text, Value};
use std::io::{self, Read, Write};
use std::process::ExitCode;

const USAGE: &str = "bxm-canon: one value, one encoding

usage:
  bxm-canon encode [FILE] [-o OUT]     Canon text (JSON) in, canonical bytes out
  bxm-canon decode [FILE] [--pretty]   canonical bytes in, Canon text out
  bxm-canon fmt [FILE] [--pretty]      text in, the one normal text form out
  bxm-canon check FILE...              exit 0 only if every file is canonical bytes
  bxm-canon hash [FILE] [--text]       SHA-256 of the canonical bytes
  bxm-canon diff A B [--text]          paths that differ between two values
  bxm-canon version

FILE defaults to standard input. --text means the inputs are Canon text
rather than canonical bytes. Text is JSON plus h\"hex\" for bytes; numbers
without . or e are 64-bit ints; a repeated key is an error.";

struct Args {
    positional: Vec<String>,
    pretty: bool,
    text: bool,
    out: Option<String>,
}

fn parse_args(raw: &[String]) -> Result<Args, String> {
    let mut a = Args {
        positional: vec![],
        pretty: false,
        text: false,
        out: None,
    };
    let mut i = 0;
    while i < raw.len() {
        match raw[i].as_str() {
            "--pretty" => a.pretty = true,
            "--text" => a.text = true,
            "-o" | "--out" => {
                i += 1;
                a.out = Some(raw.get(i).ok_or("-o needs a file name")?.clone());
            }
            "-" => a.positional.push("-".into()),
            s if s.starts_with('-') => return Err(format!("unknown option {s}")),
            s => a.positional.push(s.into()),
        }
        i += 1;
    }
    Ok(a)
}

fn read_input(name: Option<&String>) -> Result<Vec<u8>, String> {
    match name.map(|s| s.as_str()) {
        None | Some("-") => {
            let mut b = Vec::new();
            io::stdin()
                .read_to_end(&mut b)
                .map_err(|e| format!("reading stdin: {e}"))?;
            Ok(b)
        }
        Some(path) => std::fs::read(path).map_err(|e| format!("{path}: {e}")),
    }
}

fn label(name: Option<&String>) -> String {
    name.cloned().unwrap_or_else(|| "stdin".into())
}

fn value_from(raw: &[u8], as_text: bool, name: &str) -> Result<Value, String> {
    if as_text {
        let s = std::str::from_utf8(raw).map_err(|_| format!("{name}: text is not valid UTF-8"))?;
        text::parse(s).map_err(|e| format!("{name}: {e}"))
    } else {
        decode(raw).map_err(|e| format!("{name}: {e}"))
    }
}

fn write_out(bytes: &[u8], out: Option<&String>) -> Result<(), String> {
    match out {
        Some(path) => std::fs::write(path, bytes).map_err(|e| format!("{path}: {e}")),
        None => {
            let mut o = io::stdout().lock();
            o.write_all(bytes)
                .and_then(|_| o.flush())
                .map_err(|e| format!("writing: {e}"))
        }
    }
}

fn render(v: &Value, pretty: bool) -> String {
    let mut s = if pretty {
        text::to_string_pretty(v)
    } else {
        text::to_string(v)
    };
    s.push('\n');
    s
}

fn run(raw: Vec<String>) -> Result<ExitCode, String> {
    let Some(cmd) = raw.first() else {
        println!("{USAGE}");
        return Ok(ExitCode::from(2));
    };
    let a = parse_args(&raw[1..])?;
    let first = a.positional.first();
    match cmd.as_str() {
        "encode" => {
            let v = value_from(&read_input(first)?, true, &label(first))?;
            let b = encode(&v).map_err(|e| e.to_string())?;
            write_out(&b, a.out.as_ref())?;
        }
        "decode" => {
            let v = value_from(&read_input(first)?, false, &label(first))?;
            write_out(render(&v, a.pretty).as_bytes(), a.out.as_ref())?;
        }
        "fmt" => {
            let v = value_from(&read_input(first)?, true, &label(first))?;
            write_out(render(&v, a.pretty).as_bytes(), a.out.as_ref())?;
        }
        "check" => {
            if a.positional.is_empty() {
                return Err("check needs at least one file (use - for stdin)".into());
            }
            let mut bad = 0;
            for f in &a.positional {
                match read_input(Some(f)).and_then(|b| check(&b).map_err(|e| e.to_string())) {
                    Ok(_) => println!("ok {f}"),
                    Err(e) => {
                        bad += 1;
                        println!("bad {f}: {e}");
                    }
                }
            }
            if bad > 0 {
                return Ok(ExitCode::from(1));
            }
        }
        "hash" => {
            let raw_in = read_input(first)?;
            let h = if a.text {
                let v = value_from(&raw_in, true, &label(first))?;
                hash_bytes(&encode(&v).map_err(|e| e.to_string())?)
            } else {
                check(&raw_in).map_err(|e| format!("{}: {e}", label(first)))?;
                hash_bytes(&raw_in)
            };
            println!("{h}");
        }
        "diff" => {
            if a.positional.len() != 2 {
                return Err("diff needs two files".into());
            }
            let x = value_from(
                &read_input(Some(&a.positional[0]))?,
                a.text,
                &a.positional[0],
            )?;
            let y = value_from(
                &read_input(Some(&a.positional[1]))?,
                a.text,
                &a.positional[1],
            )?;
            let changes = diff::diff(&x, &y);
            for c in &changes {
                println!("{}", c.line());
            }
            if !changes.is_empty() {
                return Ok(ExitCode::from(1));
            }
        }
        "version" | "--version" | "-V" => {
            println!(
                "bxm-canon {} (format {})",
                env!("CARGO_PKG_VERSION"),
                bxm_canon::FORMAT_VERSION
            );
        }
        "help" | "--help" | "-h" => println!("{USAGE}"),
        other => return Err(format!("unknown command {other:?}; run bxm-canon help")),
    }
    Ok(ExitCode::SUCCESS)
}

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    match run(args) {
        Ok(code) => code,
        Err(e) => {
            eprintln!("bxm-canon: {e}");
            ExitCode::from(2)
        }
    }
}
