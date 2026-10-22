# bxm-canon

BXM Canon for JavaScript: a binary encoding with exactly one valid encoding for
every value. Same data in, same bytes out, on any machine. The decoder refuses
everything else, so "are these the same?" becomes "are these bytes equal?".

This package runs the Canon core as WebAssembly, compiled from the same Rust
code as the `bxm-canon` command and the C library. It gives the same bytes and
the same hashes, because it is the same code. It works in Node 18+ and in
browsers, and has no dependencies.

```sh
npm install bxm-canon
# or straight from the latest GitHub release:
npm install https://github.com/softdevstudio/bxm/releases/latest/download/bxm-canon-js.tgz
```

```js
import { load } from "bxm-canon";       // or: const { load } = require("bxm-canon")

const canon = await load();

const a = canon.hashText('{"b":1,"a":2}');
const b = canon.hashText('{ "a": 2, "b": 1 }');
a === b; // true: same value, same hash

const bytes = canon.fromText('{"name":"bxm","raw":h"cafe"}'); // Uint8Array
canon.toText(bytes);                     // '{"name":"bxm","raw":h"cafe"}'
canon.isCanonical(bytes);                // true
canon.hash(bytes);                       // SHA-256 hex of the bytes

canon.fmt('{ "b": 1, "a": [1, 2.50] }'); // '{"a":[1,2.5],"b":1}'
canon.diffText('{"v":3}', '{"v":4}');    // ['~ $.v 3 -> 4']
```

## API

`load()` loads the WebAssembly module once and resolves to an object with:

| Function | What it does |
|---|---|
| `fromText(text)` | Canon text to canonical bytes (`Uint8Array`) |
| `toText(bytes, { pretty })` | canonical bytes to text; throws if they aren't canonical |
| `fmt(text, { pretty })` | text to its one normal form |
| `check(bytes)` | throws a `CanonError` unless the bytes are exactly one canonical value |
| `isCanonical(bytes)` | the same check, as `true` or `false` |
| `hash(bytes)` | SHA-256 hex of canonical bytes |
| `hashText(text)` | SHA-256 hex of the canonical encoding of a text value |
| `diffText(a, b)` | the differences between two values, one line each |
| `version` | version of the Canon core |

Byte arguments accept a `Uint8Array`, an `ArrayBuffer` or any typed array.

A `CanonError` has a `code` (for example `overlong-varint`,
`keys-out-of-order`, or `text` for text that doesn't parse) and the byte
`offset` where the problem was found.

## Canon text

Canon text is JSON with three rules: a number without `.` or `e` is a 64-bit
integer, a repeated key is an error, and bytes are written `h"00ff"`.

To hash a JavaScript object, pass `JSON.stringify(obj)` to `hashText`. Keep
in mind how JavaScript writes numbers. `1.0` comes out as `1`, so whole
numbers become Canon integers. Whole numbers from 2^63 up to 1e21 come out as
long digit strings, which don't fit in 64 bits and are refused. From 1e21 up
they're written with `e`, so they become floats. And integers beyond 2^53
have already lost precision in JavaScript before Canon sees them. If a value
must be a float, or must be exact, write the text yourself.

Strings must be valid Unicode: a lone surrogate (half of an emoji, say) is
refused rather than quietly replaced.

## Errors

`instanceof CanonError` works within one module system. If an app mixes
`import` and `require`, the two copies have separate classes, so check
`e.name === "CanonError"` instead.

## Browsers

The package works in any browser with WebAssembly. A page with a Content
Security Policy needs `'wasm-unsafe-eval'` in `script-src`. CI tests it in
Node; browser use is not tested in CI yet.

## More

- Format spec and conformance vectors: https://github.com/softdevstudio/bxm/tree/main/spec
- Site: https://bxm.net/canon/

License: Apache 2.0. A [SoftDevStudio.com](https://softdevstudio.com) project.
