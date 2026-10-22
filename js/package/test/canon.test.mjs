import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { load, CanonError } from "../index.js";

const vectors = (name) =>
  JSON.parse(readFileSync(new URL(`../../../spec/vectors/${name}`, import.meta.url), "utf8"));
const hex = (s) => Uint8Array.from(s.match(/../g) ?? [], (h) => parseInt(h, 16));
const toHex = (b) => Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");

const canon = await load();

test("version matches the repository VERSION", () => {
  const v = readFileSync(new URL("../../../VERSION", import.meta.url), "utf8").trim();
  assert.equal(canon.version, v);
});

test("load returns the same instance", async () => {
  assert.equal(await load(), canon);
});

test("valid vectors encode to their exact bytes and back", () => {
  const cases = vectors("valid.json");
  assert.ok(cases.length >= 30);
  for (const c of cases) {
    assert.equal(toHex(canon.fromText(c.text)), c.hex, c.text);
    const back = canon.toText(hex(c.hex));
    assert.equal(toHex(canon.fromText(back)), c.hex, `round trip ${c.text}`);
    assert.ok(canon.isCanonical(hex(c.hex)));
  }
});

test("invalid vectors are rejected with the right code", () => {
  const cases = vectors("invalid.json");
  assert.ok(cases.length >= 30);
  for (const c of cases) {
    assert.equal(canon.isCanonical(hex(c.hex)), false, c.hex);
    assert.throws(
      () => canon.check(hex(c.hex)),
      (e) => e instanceof CanonError && e.code === c.error && e.offset >= 0,
      c.hex,
    );
  }
});

test("key order does not change bytes or hash", () => {
  const a = '{"b":1,"a":[true,h"ff"]}';
  const b = '{ "a": [true, h"ff"], "b": 1 }';
  assert.deepEqual(canon.fromText(a), canon.fromText(b));
  assert.equal(canon.hashText(a), canon.hashText(b));
  assert.equal(canon.hash(canon.fromText(a)), canon.hashText(a));
  // The same hash the bxm-canon command prints for this value.
  assert.equal(
    canon.hashText('{"b":1,"a":2}'),
    "e70e7c7b89c3a8b38fe00713290413936c9f976266c102b35aa91556dcc5233a",
  );
});

test("text errors", () => {
  assert.throws(() => canon.fromText('{"a":1,"a":2}'), (e) => e.code === "text" && /twice/.test(e.message));
  assert.throws(() => canon.fromText("9223372036854775808"), CanonError);
  assert.throws(() => canon.fromText('"\\u+041"'), CanonError);
  assert.throws(() => canon.fromText(42), TypeError);
});

test("fmt, pretty and diff", () => {
  assert.equal(canon.fmt('{ "b": 1, "a": [1, 2.50] }'), '{"a":[1,2.5],"b":1}');
  assert.match(canon.toText(canon.fromText('{"a":1}'), { pretty: true }), /\n  "a": 1\n/);
  assert.deepEqual(canon.diffText('{"a":1,"b":[1,2]}', '{"a":2,"b":[1]}'), ["~ $.a 1 -> 2", "- $.b[1] 2"]);
  assert.deepEqual(canon.diffText("[1]", "[1]"), []);
});

test("byte inputs: ArrayBuffer, typed arrays, empty", () => {
  const b = canon.fromText("[1,2,3]");
  assert.ok(canon.isCanonical(b.buffer.slice(b.byteOffset, b.byteOffset + b.byteLength)));
  assert.ok(canon.isCanonical(new DataView(b.buffer, b.byteOffset, b.byteLength)));
  assert.equal(canon.isCanonical(new Uint8Array(0)), false);
  assert.throws(() => canon.check("not bytes"), TypeError);
});

test("large values and many calls do not leak or break", () => {
  const big = JSON.stringify(Array.from({ length: 20000 }, (_, i) => ({ id: i, name: `r${i}`, ok: i % 2 === 0 })));
  const bytes = canon.fromText(big);
  assert.equal(canon.toText(bytes), canon.fmt(big));
  for (let i = 0; i < 5000; i++) canon.hashText(`{"i":${i}}`);
  assert.equal(canon.hash(bytes), canon.hashText(big));
});
