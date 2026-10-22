# The order as a Python service builds it: keys in the order the code fills
# them, json.dumps defaults (spaces after separators, non-ASCII escaped).
import hashlib, hmac, json, sys
import bxm_canon

KEY = b"case-study-shared-key"

order = {
    "id": "ord-20261022-0042",
    "customer": {"name": "Anna Müller", "city": "Zürich"},
    "lines": [
        {"sku": "BX-100", "qty": 3, "price": 19.95},
        {"sku": "BX-200", "qty": 1, "price": 249.0},
    ],
    "currency": "CHF",
    "paid": True,
}

plain = json.dumps(order).encode()
canon = bxm_canon.encode(order)
out = {
    "plain_sha256": hashlib.sha256(plain).hexdigest(),
    "plain_bytes": len(plain),
    "canon_sha256": bxm_canon.hash(canon),
    "canon_bytes": len(canon),
    "plain_hmac": hmac.new(KEY, plain, hashlib.sha256).hexdigest(),
    "canon_hmac": hmac.new(KEY, canon, hashlib.sha256).hexdigest(),
}
json.dump(out, sys.stdout)
