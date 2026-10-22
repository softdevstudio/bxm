// Builds index.js (ES module) and index.cjs (CommonJS) from canon.template.js
// and the compiled WebAssembly file given as the first argument.
import { readFileSync, writeFileSync } from "node:fs";

const wasmPath = process.argv[2];
if (!wasmPath) {
  console.error("usage: node build.mjs path/to/bxm_canon_wasm.wasm");
  process.exit(2);
}
const b64 = readFileSync(wasmPath).toString("base64");
const template = readFileSync(new URL("./canon.template.js", import.meta.url), "utf8");
const body = template.replace("/*WASM*/", `const WASM_BASE64 = "${b64}";`);
writeFileSync(new URL("./index.js", import.meta.url), body + "\nexport { load, CanonError };\n");
writeFileSync(new URL("./index.cjs", import.meta.url), '"use strict";\n' + body + "\nmodule.exports = { load, CanonError };\n");
console.log(`index.js and index.cjs written, wasm ${readFileSync(wasmPath).length} bytes`);
