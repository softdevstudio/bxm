# Third-party licenses

## BXM Canon (Rust)

BXM Canon is compiled with these crates. Each one is offered under the MIT
license, the Apache License 2.0, or either at your choice, as noted. Their
full license texts are in their source packages on crates.io.

| Crate         | Version | License            | Source                                  |
|---------------|---------|--------------------|-----------------------------------------|
| sha2          | 0.10.9  | MIT OR Apache-2.0  | https://github.com/RustCrypto/hashes    |
| digest        | 0.10.7  | MIT OR Apache-2.0  | https://github.com/RustCrypto/traits    |
| crypto-common | 0.1.7   | MIT OR Apache-2.0  | https://github.com/RustCrypto/traits    |
| block-buffer  | 0.10.4  | MIT OR Apache-2.0  | https://github.com/RustCrypto/utils     |
| cpufeatures   | 0.2.17  | MIT OR Apache-2.0  | https://github.com/RustCrypto/utils     |
| generic-array | 0.14.7  | MIT                | https://github.com/fizyk20/generic-array |
| typenum       | 1.20.1  | MIT OR Apache-2.0  | https://github.com/paholg/typenum       |
| cfg-if        | 1.0.5   | MIT OR Apache-2.0  | https://github.com/rust-lang/cfg-if     |

The binaries also contain the Rust standard library (MIT OR Apache-2.0).

## BXM Canon for JavaScript

The npm package contains the Canon crate and the crates listed above,
compiled to WebAssembly. It has no JavaScript dependencies.

## BXM Canon for Python

The Python wheels contain the Canon crate and the crates listed above, plus:

| Crate     | Version | License           | Source                              |
|-----------|---------|-------------------|-------------------------------------|
| pyo3      | 0.30.1  | MIT OR Apache-2.0 | https://github.com/PyO3/pyo3        |
| pyo3-ffi  | 0.30.1  | MIT OR Apache-2.0 | https://github.com/PyO3/pyo3        |
| once_cell | 1.21.4  | MIT OR Apache-2.0 | https://github.com/matklad/once_cell |
| libc      | 0.2.191 | MIT OR Apache-2.0 | https://github.com/rust-lang/libc   |

PyO3's procedural macros and build helpers run only at build time and are not
part of the wheels.

## BXM Memo (Go)

BXM Memo uses only the Go standard library, which is under the BSD 3-Clause
license: https://go.dev/LICENSE
