//! Python bindings for BXM Canon. The Canon rules live in the bxm-canon crate;
//! this file converts between Python objects and Canon values and maps errors
//! to `bxm_canon.CanonError`.

use bxm_canon::{
    check as canon_check, decode as canon_decode, encode as canon_encode, hash_bytes, text, Value,
    MAX_DEPTH,
};
use pyo3::create_exception;
use pyo3::exceptions::{PyOverflowError, PyTypeError, PyValueError};
use pyo3::prelude::*;
use pyo3::types::{
    PyBool, PyByteArray, PyBytes, PyDict, PyFloat, PyInt, PyList, PyString, PyTuple,
};
use std::collections::BTreeMap;

create_exception!(
    _core,
    CanonError,
    PyValueError,
    "An error from Canon, with `code` and `offset`."
);

fn canon_err(py: Python<'_>, message: String) -> PyErr {
    let (code, offset) = parse_message(&message);
    let err = CanonError::new_err(message);
    let v = err.value(py);
    let _ = v.setattr("code", code);
    let _ = v.setattr("offset", offset);
    err
}

/// "keys-out-of-order at byte 7: ..." -> ("keys-out-of-order", 7); "text at byte 3: ..." -> ("text", 3)
fn parse_message(m: &str) -> (String, i64) {
    if let Some((head, _)) = m.split_once(": ") {
        if let Some((code, at)) = head.split_once(" at byte ") {
            if let Ok(n) = at.parse::<i64>() {
                return (code.to_string(), n);
            }
        }
    }
    ("error".to_string(), -1)
}

fn bytes_of(data: &Bound<'_, PyAny>) -> PyResult<Vec<u8>> {
    if let Ok(b) = data.cast::<PyBytes>() {
        return Ok(b.as_bytes().to_vec());
    }
    if data.is_instance_of::<PyByteArray>() || data.get_type().name()? == "memoryview" {
        let b = data.py().get_type::<PyBytes>().call1((data,))?;
        return Ok(b.cast::<PyBytes>()?.as_bytes().to_vec());
    }
    Err(PyTypeError::new_err(
        "data must be bytes, bytearray or memoryview",
    ))
}

fn to_value(obj: &Bound<'_, PyAny>, depth: usize) -> PyResult<Value> {
    let py = obj.py();
    if obj.is_none() {
        return Ok(Value::Null);
    }
    if let Ok(b) = obj.cast::<PyBool>() {
        return Ok(Value::Bool(b.is_true()));
    }
    if obj.is_instance_of::<PyInt>() {
        return obj
            .extract::<i64>()
            .map(Value::Int)
            .map_err(|_| PyOverflowError::new_err("integer does not fit in 64 bits"));
    }
    if let Ok(f) = obj.cast::<PyFloat>() {
        return Ok(Value::Float(f.value()));
    }
    if let Ok(s) = obj.cast::<PyString>() {
        return s
            .to_cow()
            .map(|s| Value::String(s.into_owned()))
            .map_err(|_| {
                canon_err(
                    py,
                    "text at byte 0: the string is not valid Unicode (it has a lone surrogate)"
                        .into(),
                )
            });
    }
    if obj.is_instance_of::<PyBytes>()
        || obj.is_instance_of::<PyByteArray>()
        || obj.get_type().name()? == "memoryview"
    {
        return Ok(Value::Bytes(bytes_of(obj)?));
    }
    if obj.is_instance_of::<PyList>() || obj.is_instance_of::<PyTuple>() {
        if depth >= MAX_DEPTH {
            return Err(canon_err(
                py,
                format!("too-deep at byte 0: nesting deeper than {MAX_DEPTH}"),
            ));
        }
        let mut items = Vec::new();
        for item in obj.try_iter()? {
            items.push(to_value(&item?, depth + 1)?);
        }
        return Ok(Value::Array(items));
    }
    if let Ok(d) = obj.cast::<PyDict>() {
        if depth >= MAX_DEPTH {
            return Err(canon_err(
                py,
                format!("too-deep at byte 0: nesting deeper than {MAX_DEPTH}"),
            ));
        }
        let mut map = BTreeMap::new();
        for (k, v) in d.iter() {
            let key = k
                .cast::<PyString>()
                .map_err(|_| PyTypeError::new_err("map keys must be str"))?
                .to_cow()
                .map_err(|_| {
                    canon_err(
                        py,
                        "text at byte 0: a key is not valid Unicode (it has a lone surrogate)"
                            .into(),
                    )
                })?
                .into_owned();
            map.insert(key, to_value(&v, depth + 1)?);
        }
        return Ok(Value::Map(map));
    }
    Err(PyTypeError::new_err(format!(
        "cannot encode a {} (use None, bool, int, float, str, bytes, list, tuple or dict)",
        obj.get_type().name()?
    )))
}

fn from_value<'py>(py: Python<'py>, v: &Value) -> PyResult<Bound<'py, PyAny>> {
    Ok(match v {
        Value::Null => py.None().into_bound(py),
        Value::Bool(b) => PyBool::new(py, *b).to_owned().into_any(),
        Value::Int(i) => i.into_pyobject(py)?.into_any(),
        Value::Float(f) => PyFloat::new(py, *f).into_any(),
        Value::Bytes(b) => PyBytes::new(py, b).into_any(),
        Value::String(s) => PyString::new(py, s).into_any(),
        Value::Array(items) => {
            let list = PyList::empty(py);
            for item in items {
                list.append(from_value(py, item)?)?;
            }
            list.into_any()
        }
        Value::Map(m) => {
            let d = PyDict::new(py);
            for (k, val) in m {
                d.set_item(k, from_value(py, val)?)?;
            }
            d.into_any()
        }
    })
}

fn parse(py: Python<'_>, t: &str) -> PyResult<Value> {
    text::parse(t).map_err(|e| canon_err(py, e.to_string()))
}

fn enc(py: Python<'_>, v: &Value) -> PyResult<Vec<u8>> {
    canon_encode(v).map_err(|e| canon_err(py, e.to_string()))
}

/// Encode a Python value as canonical bytes.
#[pyfunction]
fn encode<'py>(py: Python<'py>, obj: &Bound<'py, PyAny>) -> PyResult<Bound<'py, PyBytes>> {
    let v = to_value(obj, 0)?;
    Ok(PyBytes::new(py, &enc(py, &v)?))
}

/// Decode canonical bytes into a Python value. Raises CanonError if the bytes are not canonical.
#[pyfunction]
fn decode<'py>(py: Python<'py>, data: &Bound<'py, PyAny>) -> PyResult<Bound<'py, PyAny>> {
    let b = bytes_of(data)?;
    let v = canon_decode(&b).map_err(|e| canon_err(py, e.to_string()))?;
    from_value(py, &v)
}

/// Raise CanonError unless the bytes are exactly one canonical value.
#[pyfunction]
fn check(py: Python<'_>, data: &Bound<'_, PyAny>) -> PyResult<()> {
    let b = bytes_of(data)?;
    canon_check(&b).map_err(|e| canon_err(py, e.to_string()))
}

/// True when the bytes are exactly one canonical value.
#[pyfunction]
fn is_canonical(data: &Bound<'_, PyAny>) -> PyResult<bool> {
    Ok(canon_check(&bytes_of(data)?).is_ok())
}

/// Lowercase hex SHA-256 of canonical bytes (checked first).
#[pyfunction]
fn hash(py: Python<'_>, data: &Bound<'_, PyAny>) -> PyResult<String> {
    let b = bytes_of(data)?;
    canon_check(&b).map_err(|e| canon_err(py, e.to_string()))?;
    Ok(hash_bytes(&b))
}

/// Lowercase hex SHA-256 of the canonical encoding of a Python value.
#[pyfunction]
fn hash_value(py: Python<'_>, obj: &Bound<'_, PyAny>) -> PyResult<String> {
    let v = to_value(obj, 0)?;
    Ok(hash_bytes(&enc(py, &v)?))
}

/// Canon text (JSON plus h"hex" for bytes) to canonical bytes.
#[pyfunction]
fn from_text<'py>(py: Python<'py>, t: &str) -> PyResult<Bound<'py, PyBytes>> {
    let v = parse(py, t)?;
    Ok(PyBytes::new(py, &enc(py, &v)?))
}

/// Canonical bytes to Canon text.
#[pyfunction]
#[pyo3(signature = (data, pretty=false))]
fn to_text(py: Python<'_>, data: &Bound<'_, PyAny>, pretty: bool) -> PyResult<String> {
    let v = canon_decode(&bytes_of(data)?).map_err(|e| canon_err(py, e.to_string()))?;
    Ok(if pretty {
        text::to_string_pretty(&v)
    } else {
        text::to_string(&v)
    })
}

/// Text to its one normal form: keys sorted, no spaces (indented with pretty).
#[pyfunction]
#[pyo3(signature = (t, pretty=false))]
fn fmt(py: Python<'_>, t: &str, pretty: bool) -> PyResult<String> {
    let v = parse(py, t)?;
    Ok(if pretty {
        text::to_string_pretty(&v)
    } else {
        text::to_string(&v)
    })
}

/// Lowercase hex SHA-256 of the canonical encoding of a text value.
#[pyfunction]
fn hash_text(py: Python<'_>, t: &str) -> PyResult<String> {
    let v = parse(py, t)?;
    Ok(hash_bytes(&enc(py, &v)?))
}

/// Differences between two text values, one change per line.
#[pyfunction]
fn diff_text(py: Python<'_>, a: &str, b: &str) -> PyResult<Vec<String>> {
    let x = parse(py, a)?;
    let y = parse(py, b)?;
    Ok(bxm_canon::diff::diff(&x, &y)
        .iter()
        .map(|c| c.line())
        .collect())
}

#[pymodule]
fn _core(m: &Bound<'_, PyModule>) -> PyResult<()> {
    m.add("CanonError", m.py().get_type::<CanonError>())?;
    m.add("__version__", bxm_canon::VERSION)?;
    m.add_function(wrap_pyfunction!(encode, m)?)?;
    m.add_function(wrap_pyfunction!(decode, m)?)?;
    m.add_function(wrap_pyfunction!(check, m)?)?;
    m.add_function(wrap_pyfunction!(is_canonical, m)?)?;
    m.add_function(wrap_pyfunction!(hash, m)?)?;
    m.add_function(wrap_pyfunction!(hash_value, m)?)?;
    m.add_function(wrap_pyfunction!(from_text, m)?)?;
    m.add_function(wrap_pyfunction!(to_text, m)?)?;
    m.add_function(wrap_pyfunction!(fmt, m)?)?;
    m.add_function(wrap_pyfunction!(hash_text, m)?)?;
    m.add_function(wrap_pyfunction!(diff_text, m)?)?;
    Ok(())
}
