"""Tests for the bxm-canon Python package. Run: python -m unittest discover -s tests"""

import json
import math
import pathlib
import unittest

import bxm_canon as canon
from bxm_canon import CanonError

ROOT = pathlib.Path(__file__).resolve().parents[2]
VECTORS = ROOT / "spec" / "vectors"
KNOWN = "e70e7c7b89c3a8b38fe00713290413936c9f976266c102b35aa91556dcc5233a"  # {"a":2,"b":1}


class Vectors(unittest.TestCase):
    def test_version_matches_repository(self):
        self.assertEqual(canon.__version__, (ROOT / "VERSION").read_text(encoding="utf-8").strip())

    def test_valid_vectors(self):
        cases = json.loads((VECTORS / "valid.json").read_text(encoding="utf-8"))
        self.assertGreaterEqual(len(cases), 30)
        for c in cases:
            with self.subTest(text=c["text"]):
                b = canon.from_text(c["text"])
                self.assertEqual(b.hex(), c["hex"])
                self.assertTrue(canon.is_canonical(b))
                self.assertEqual(canon.encode(canon.decode(b)), b)
                self.assertEqual(canon.from_text(canon.to_text(b)), b)

    def test_invalid_vectors(self):
        cases = json.loads((VECTORS / "invalid.json").read_text(encoding="utf-8"))
        self.assertGreaterEqual(len(cases), 30)
        for c in cases:
            with self.subTest(hex=c["hex"][:40]):
                data = bytes.fromhex(c["hex"])
                self.assertFalse(canon.is_canonical(data))
                with self.assertRaises(CanonError) as cm:
                    canon.check(data)
                self.assertEqual(cm.exception.code, c["error"])
                self.assertGreaterEqual(cm.exception.offset, 0)
                with self.assertRaises(CanonError):
                    canon.decode(data)


class PythonValues(unittest.TestCase):
    def test_round_trip(self):
        v = {"none": None, "t": True, "f": False, "i": -3, "big": 2**63 - 1, "small": -(2**63),
             "x": 0.1, "s": "é😀", "b": b"\x00\xff", "l": [1, [2, {"k": []}]], "e": {}}
        self.assertEqual(canon.decode(canon.encode(v)), v)

    def test_python_types_map_exactly(self):
        out = canon.decode(canon.encode([1, 1.0, True, (1, 2), bytearray(b"a")]))
        self.assertEqual([type(x) for x in out], [int, float, bool, list, bytes])
        self.assertNotEqual(canon.encode(1), canon.encode(1.0))
        self.assertNotEqual(canon.encode(True), canon.encode(1))

    def test_key_order_does_not_matter(self):
        self.assertEqual(canon.encode({"b": 1, "a": 2}), canon.encode({"a": 2, "b": 1}))
        self.assertEqual(canon.hash_value({"b": 1, "a": 2}), KNOWN)
        self.assertEqual(canon.hash_text('{ "a": 2, "b": 1 }'), KNOWN)
        self.assertEqual(canon.hash(canon.from_text('{"b":1,"a":2}')), KNOWN)

    def test_values_that_are_not_canon(self):
        for f, code in [(math.nan, "nan"), (math.inf, "infinity"), (-math.inf, "infinity"), (-0.0, "negative-zero")]:
            with self.subTest(f=f):
                with self.assertRaises(CanonError) as cm:
                    canon.encode(f)
                self.assertEqual(cm.exception.code, code)
        with self.assertRaises(OverflowError):
            canon.encode(2**63)
        with self.assertRaises(TypeError):
            canon.encode({1: "x"})
        with self.assertRaises(TypeError):
            canon.encode({1, 2})
        with self.assertRaises(CanonError):
            canon.encode("\ud800")

    def test_nesting_limit(self):
        def nest(n):
            v = None
            for _ in range(n):
                v = [v]
            return v
        b = canon.encode(nest(512))
        self.assertTrue(canon.is_canonical(b))
        self.assertEqual(canon.encode(canon.decode(b)), b)
        with self.assertRaises(CanonError) as cm:
            canon.encode(nest(513))
        self.assertEqual(cm.exception.code, "too-deep")

    def test_byte_inputs(self):
        b = canon.from_text("[1,2,3]")
        self.assertTrue(canon.is_canonical(bytearray(b)))
        self.assertTrue(canon.is_canonical(memoryview(b)))
        self.assertFalse(canon.is_canonical(b""))
        with self.assertRaises(TypeError):
            canon.check("not bytes")


class TextFunctions(unittest.TestCase):
    def test_fmt_pretty_diff(self):
        self.assertEqual(canon.fmt('{ "b": 1, "a": [1, 2.50] }'), '{"a":[1,2.5],"b":1}')
        self.assertIn('\n  "a": 1\n', canon.to_text(canon.from_text('{"a":1}'), pretty=True))
        self.assertEqual(canon.diff_text('{"a":1,"b":[1,2]}', '{"a":2,"b":[1]}'), ["~ $.a 1 -> 2", "- $.b[1] 2"])
        self.assertEqual(canon.diff_text("[1]", "[1]"), [])

    def test_text_errors(self):
        with self.assertRaises(CanonError) as cm:
            canon.from_text('{"a":1,"a":2}')
        self.assertEqual(cm.exception.code, "text")
        self.assertIn("twice", str(cm.exception))
        self.assertIsInstance(cm.exception, ValueError)


if __name__ == "__main__":
    unittest.main()
