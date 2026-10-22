// The same order as a JavaScript service builds it: a different key order,
// JSON.stringify defaults (no spaces, non-ASCII kept as is). It checks the
// Python service's two signatures against what it can rebuild itself.
import { createHash, createHmac } from "node:crypto";
import { readFileSync } from "node:fs";
import { load } from "bxm-canon";

const KEY = "case-study-shared-key";
const fromPython = JSON.parse(readFileSync(process.argv[2], "utf8"));
const canon = await load();

const order = {
  currency: "CHF",
  paid: true,
  id: "ord-20261022-0042",
  lines: [
    { price: 19.95, qty: 3, sku: "BX-100" },
    { price: 249.0, qty: 1, sku: "BX-200" },
  ],
  customer: { city: "Zürich", name: "Anna Müller" },
};

const text = JSON.stringify(order);
const plain = Buffer.from(text, "utf8");
// JavaScript has one number type, so 249.0 arrives as 249 and would become
// an int. The record's prices are floats, so this producer writes them as
// Canon text with a decimal point, the way the schema says.
const canonText = text.replace(/"price":(\d+)(?=[,}])/g, '"price":$1.0');
const bytes = canon.fromText(canonText);
const hmac = (b) => createHmac("sha256", KEY).update(b).digest("hex");
const sha = (b) => createHash("sha256").update(b).digest("hex");

console.log(JSON.stringify({
  plain_sha256: sha(plain),
  plain_bytes: plain.length,
  canon_sha256: canon.hash(bytes),
  canon_bytes: bytes.length,
  plain_signature_ok: hmac(plain) === fromPython.plain_hmac,
  canon_signature_ok: hmac(bytes) === fromPython.canon_hmac,
}));
