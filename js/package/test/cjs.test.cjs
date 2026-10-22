const { test } = require("node:test");
const assert = require("node:assert/strict");
const { load, CanonError } = require("../index.cjs");

test("CommonJS build works", async () => {
  const canon = await load();
  assert.equal(canon.hashText('{"a":2,"b":1}'), "e70e7c7b89c3a8b38fe00713290413936c9f976266c102b35aa91556dcc5233a");
  assert.throws(() => canon.check(Uint8Array.of(9)), (e) => e instanceof CanonError && e.code === "unknown-tag");
});
