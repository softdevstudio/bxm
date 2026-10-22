//! The WebAssembly face of BXM Canon. The JavaScript wrapper copies its input
//! into memory from `bxm_alloc`, calls one operation, then reads the answer
//! (bytes, text, or an error message) from `bxm_result_ptr`/`bxm_result_len`.
//! Every operation returns 0 on success and 1 on error. All the Canon rules
//! live in the bxm-canon crate; this file only moves bytes across.

use bxm_canon::{check, decode, encode, hash_bytes, text};
use std::cell::RefCell;

thread_local! {
    static RESULT: RefCell<Vec<u8>> = const { RefCell::new(Vec::new()) };
}

fn set(v: Vec<u8>) {
    RESULT.with(|r| *r.borrow_mut() = v);
}

fn done(r: Result<Vec<u8>, String>) -> i32 {
    match r {
        Ok(v) => {
            set(v);
            0
        }
        Err(e) => {
            set(e.into_bytes());
            1
        }
    }
}

unsafe fn input<'a>(p: *const u8, len: usize) -> &'a [u8] {
    if len == 0 {
        &[]
    } else {
        std::slice::from_raw_parts(p, len)
    }
}

fn parse_text(raw: &[u8]) -> Result<bxm_canon::Value, String> {
    let s = std::str::from_utf8(raw).map_err(|_| "text is not valid UTF-8".to_string())?;
    text::parse(s).map_err(|e| e.to_string())
}

#[no_mangle]
pub extern "C" fn bxm_alloc(len: usize) -> *mut u8 {
    let mut v = Vec::<u8>::with_capacity(len.max(1));
    let p = v.as_mut_ptr();
    std::mem::forget(v);
    p
}

/// # Safety
/// `p` and `len` must come from `bxm_alloc`.
#[no_mangle]
pub unsafe extern "C" fn bxm_dealloc(p: *mut u8, len: usize) {
    drop(Vec::from_raw_parts(p, 0, len.max(1)));
}

#[no_mangle]
pub extern "C" fn bxm_result_ptr() -> *const u8 {
    RESULT.with(|r| r.borrow().as_ptr())
}

#[no_mangle]
pub extern "C" fn bxm_result_len() -> usize {
    RESULT.with(|r| r.borrow().len())
}

/// Canon text in, canonical bytes out.
///
/// # Safety
/// `p` must point to `len` readable bytes.
#[no_mangle]
pub unsafe extern "C" fn bxm_from_text(p: *const u8, len: usize) -> i32 {
    done(parse_text(input(p, len)).and_then(|v| encode(&v).map_err(|e| e.to_string())))
}

/// Canonical bytes in, text out (indented when `pretty` is not 0).
///
/// # Safety
/// `p` must point to `len` readable bytes.
#[no_mangle]
pub unsafe extern "C" fn bxm_to_text(p: *const u8, len: usize, pretty: i32) -> i32 {
    done(
        decode(input(p, len))
            .map(|v| {
                if pretty != 0 {
                    text::to_string_pretty(&v)
                } else {
                    text::to_string(&v)
                }
                .into_bytes()
            })
            .map_err(|e| e.to_string()),
    )
}

/// Text in, the one normal compact text form out.
///
/// # Safety
/// `p` must point to `len` readable bytes.
#[no_mangle]
pub unsafe extern "C" fn bxm_fmt(p: *const u8, len: usize, pretty: i32) -> i32 {
    done(parse_text(input(p, len)).map(|v| {
        if pretty != 0 {
            text::to_string_pretty(&v)
        } else {
            text::to_string(&v)
        }
        .into_bytes()
    }))
}

/// 0 when the bytes are exactly one canonical value.
///
/// # Safety
/// `p` must point to `len` readable bytes.
#[no_mangle]
pub unsafe extern "C" fn bxm_check(p: *const u8, len: usize) -> i32 {
    done(
        check(input(p, len))
            .map(|_| Vec::new())
            .map_err(|e| e.to_string()),
    )
}

/// SHA-256 hex of canonical bytes (checked first).
///
/// # Safety
/// `p` must point to `len` readable bytes.
#[no_mangle]
pub unsafe extern "C" fn bxm_hash(p: *const u8, len: usize) -> i32 {
    let raw = input(p, len);
    done(
        check(raw)
            .map(|_| hash_bytes(raw).into_bytes())
            .map_err(|e| e.to_string()),
    )
}

/// SHA-256 hex of the canonical encoding of a text value.
///
/// # Safety
/// `p` must point to `len` readable bytes.
#[no_mangle]
pub unsafe extern "C" fn bxm_hash_text(p: *const u8, len: usize) -> i32 {
    done(parse_text(input(p, len)).and_then(|v| {
        encode(&v)
            .map(|b| hash_bytes(&b).into_bytes())
            .map_err(|e| e.to_string())
    }))
}

/// Differences between two text values, one change per line.
///
/// # Safety
/// `a` and `b` must point to `alen` and `blen` readable bytes.
#[no_mangle]
pub unsafe extern "C" fn bxm_diff_text(
    a: *const u8,
    alen: usize,
    b: *const u8,
    blen: usize,
) -> i32 {
    let x = match parse_text(input(a, alen)) {
        Ok(v) => v,
        Err(e) => return done(Err(e)),
    };
    let y = match parse_text(input(b, blen)) {
        Ok(v) => v,
        Err(e) => return done(Err(e)),
    };
    let lines: Vec<String> = bxm_canon::diff::diff(&x, &y)
        .iter()
        .map(|c| c.line())
        .collect();
    done(Ok(lines.join("\n").into_bytes()))
}

#[no_mangle]
pub extern "C" fn bxm_version() -> i32 {
    done(Ok(bxm_canon::VERSION.as_bytes().to_vec()))
}
