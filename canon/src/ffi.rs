//! C API, so any language with a C FFI can use the one Canon implementation.
//! The header is `canon/include/bxm_canon.h`.
//!
//! Every function returns 0 on success and a positive error number otherwise.
//! Output buffers are allocated here and must be released with
//! `bxm_canon_free`. On error, `*err_out` (when not null) receives a
//! NUL-terminated message that must also be released with `bxm_canon_free_str`.

use crate::{check, decode, encode, hash_bytes, text};
use std::ffi::{c_char, CString};
use std::ptr;
use std::slice;

pub const BXM_OK: i32 = 0;
pub const BXM_ERR_TEXT: i32 = 1;
pub const BXM_ERR_ENCODE: i32 = 2;
pub const BXM_ERR_DECODE: i32 = 3;
pub const BXM_ERR_ARG: i32 = 4;

unsafe fn input<'a>(p: *const u8, len: usize) -> Option<&'a [u8]> {
    if p.is_null() {
        if len == 0 {
            Some(&[])
        } else {
            None
        }
    } else {
        Some(slice::from_raw_parts(p, len))
    }
}

unsafe fn set_err(err_out: *mut *mut c_char, msg: String) {
    if !err_out.is_null() {
        let c = CString::new(msg.replace('\0', " ")).unwrap_or_default();
        *err_out = c.into_raw();
    }
}

unsafe fn give(bytes: Vec<u8>, out: *mut *mut u8, out_len: *mut usize) {
    let boxed = bytes.into_boxed_slice();
    *out_len = boxed.len();
    *out = Box::into_raw(boxed) as *mut u8;
}

/// Text in, canonical bytes out.
///
/// # Safety
/// `text` must point to `text_len` readable bytes; `out` and `out_len` must be
/// valid pointers.
#[no_mangle]
pub unsafe extern "C" fn bxm_canon_from_text(
    text_ptr: *const u8,
    text_len: usize,
    out: *mut *mut u8,
    out_len: *mut usize,
    err_out: *mut *mut c_char,
) -> i32 {
    if out.is_null() || out_len.is_null() {
        return BXM_ERR_ARG;
    }
    let Some(raw) = input(text_ptr, text_len) else {
        return BXM_ERR_ARG;
    };
    let s = match std::str::from_utf8(raw) {
        Ok(s) => s,
        Err(_) => {
            set_err(err_out, "text is not valid UTF-8".into());
            return BXM_ERR_TEXT;
        }
    };
    let v = match text::parse(s) {
        Ok(v) => v,
        Err(e) => {
            set_err(err_out, e.to_string());
            return BXM_ERR_TEXT;
        }
    };
    match encode(&v) {
        Ok(b) => {
            give(b, out, out_len);
            BXM_OK
        }
        Err(e) => {
            set_err(err_out, e.to_string());
            BXM_ERR_ENCODE
        }
    }
}

/// Canonical bytes in, compact text out (not NUL-terminated; use `out_len`).
///
/// # Safety
/// Same as `bxm_canon_from_text`.
#[no_mangle]
pub unsafe extern "C" fn bxm_canon_to_text(
    data: *const u8,
    data_len: usize,
    out: *mut *mut u8,
    out_len: *mut usize,
    err_out: *mut *mut c_char,
) -> i32 {
    if out.is_null() || out_len.is_null() {
        return BXM_ERR_ARG;
    }
    let Some(raw) = input(data, data_len) else {
        return BXM_ERR_ARG;
    };
    match decode(raw) {
        Ok(v) => {
            give(text::to_string(&v).into_bytes(), out, out_len);
            BXM_OK
        }
        Err(e) => {
            set_err(err_out, e.to_string());
            BXM_ERR_DECODE
        }
    }
}

/// Checks that the bytes are exactly one canonical value.
///
/// # Safety
/// `data` must point to `data_len` readable bytes.
#[no_mangle]
pub unsafe extern "C" fn bxm_canon_check(
    data: *const u8,
    data_len: usize,
    err_out: *mut *mut c_char,
) -> i32 {
    let Some(raw) = input(data, data_len) else {
        return BXM_ERR_ARG;
    };
    match check(raw) {
        Ok(()) => BXM_OK,
        Err(e) => {
            set_err(err_out, e.to_string());
            BXM_ERR_DECODE
        }
    }
}

/// Checks the bytes, then writes the 64 hex characters of their SHA-256 and a
/// NUL into `hex_out`, which must have room for 65 bytes.
///
/// # Safety
/// `data` must point to `data_len` readable bytes; `hex_out` to 65 writable bytes.
#[no_mangle]
pub unsafe extern "C" fn bxm_canon_hash(
    data: *const u8,
    data_len: usize,
    hex_out: *mut c_char,
    err_out: *mut *mut c_char,
) -> i32 {
    if hex_out.is_null() {
        return BXM_ERR_ARG;
    }
    let Some(raw) = input(data, data_len) else {
        return BXM_ERR_ARG;
    };
    if let Err(e) = check(raw) {
        set_err(err_out, e.to_string());
        return BXM_ERR_DECODE;
    }
    let h = hash_bytes(raw);
    ptr::copy_nonoverlapping(h.as_ptr(), hex_out as *mut u8, 64);
    *hex_out.add(64) = 0;
    BXM_OK
}

/// Releases a buffer returned by this library.
///
/// # Safety
/// `p` and `len` must come from one of this library's outputs, once.
#[no_mangle]
pub unsafe extern "C" fn bxm_canon_free(p: *mut u8, len: usize) {
    if !p.is_null() {
        drop(Box::from_raw(ptr::slice_from_raw_parts_mut(p, len)));
    }
}

/// Releases an error message.
///
/// # Safety
/// `p` must come from an `err_out` of this library, once.
#[no_mangle]
pub unsafe extern "C" fn bxm_canon_free_str(p: *mut c_char) {
    if !p.is_null() {
        drop(CString::from_raw(p));
    }
}

/// Library version as a static NUL-terminated string.
#[no_mangle]
pub extern "C" fn bxm_canon_version() -> *const c_char {
    concat!(env!("CARGO_PKG_VERSION"), "\0").as_ptr() as *const c_char
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn c_api_round_trip() {
        unsafe {
            let t = br#"{"b":1,"a":[true,h"0a"]}"#;
            let mut out = ptr::null_mut();
            let mut n = 0usize;
            let mut err = ptr::null_mut();
            assert_eq!(
                bxm_canon_from_text(t.as_ptr(), t.len(), &mut out, &mut n, &mut err),
                0
            );
            assert_eq!(bxm_canon_check(out, n, &mut err), 0);
            let mut hex = [0 as c_char; 65];
            assert_eq!(bxm_canon_hash(out, n, hex.as_mut_ptr(), &mut err), 0);
            let mut tout = ptr::null_mut();
            let mut tn = 0usize;
            assert_eq!(bxm_canon_to_text(out, n, &mut tout, &mut tn, &mut err), 0);
            let back = std::str::from_utf8(slice::from_raw_parts(tout, tn)).unwrap();
            assert_eq!(back, r#"{"a":[true,h"0a"],"b":1}"#);
            bxm_canon_free(out, n);
            bxm_canon_free(tout, tn);

            let bad = [0x09u8];
            assert_eq!(bxm_canon_check(bad.as_ptr(), 1, &mut err), BXM_ERR_DECODE);
            assert!(!err.is_null());
            bxm_canon_free_str(err);
        }
    }
}
