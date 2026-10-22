"""Case study 3: finding duplicate records sent by different producers.

Builds a stream of 100,000 JSON lines from a fixed seed. 90,000 are distinct
events. 10,000 are resends of earlier events by another producer, which
writes the same data with its keys in another order, other spacing and other
escaping. 100 of those resends come from a producer that writes the integer
qty as a float (2.0 instead of 2), which makes them different values.

Then it counts distinct records two ways: SHA-256 of each line as received,
the Canon hash of each parsed line, and, for comparison, SHA-256 of the line
re-serialized by Python with sorted keys.
"""
import hashlib, json, random, time
import bxm_canon

SEED, EVENTS, RESENDS, FLOAT_RESENDS = 20261022, 90_000, 10_000, 100
rng = random.Random(SEED)
cities = ["Zürich", "Köln", "São Paulo", "Kraków", "Tel Aviv", "Lyon", "Malmö", "Austin"]

def event(i):
    return {
        "event_id": f"evt-{i:06d}",
        "user": f"u{rng.randrange(20000):05d}",
        "sku": f"SKU-{rng.randrange(500):03d}",
        "qty": rng.randrange(1, 6),
        "price": round(rng.uniform(1, 300), 2),
        "city": rng.choice(cities),
        "tags": rng.sample(["gift", "promo", "repeat", "mobile", "web"], k=rng.randrange(0, 3)),
        "ts": 1792000000 + i * 7,
    }

def producer_a(e):   # compact, keys as built, UTF-8 kept
    return json.dumps(e, separators=(",", ":"), ensure_ascii=False)

def producer_b(e):   # sorted keys, default spacing, non-ASCII escaped
    return json.dumps(e, sort_keys=True, ensure_ascii=True)

def producer_c(e):   # reversed key order, spaced, UTF-8 kept
    return json.dumps(dict(reversed(list(e.items()))), ensure_ascii=False)

events = [event(i) for i in range(EVENTS)]
lines = [producer_a(e) for e in events]
resent = rng.sample(range(EVENTS), RESENDS)
for n, i in enumerate(resent):
    e = dict(events[i])
    if n < FLOAT_RESENDS:
        e["qty"] = float(e["qty"])
    lines.append((producer_b if n % 2 else producer_c)(e))
rng.shuffle(lines)

t = time.perf_counter()
plain = {hashlib.sha256(l.encode()).hexdigest() for l in lines}
t_plain = time.perf_counter() - t

t = time.perf_counter()
canon = {bxm_canon.hash_value(json.loads(l)) for l in lines}
t_canon = time.perf_counter() - t

t = time.perf_counter()
sorted_json = {hashlib.sha256(json.dumps(json.loads(l), sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()).hexdigest() for l in lines}
t_sorted = time.perf_counter() - t

total_bytes = sum(len(l.encode()) for l in lines)
print(f"lines: {len(lines)} ({total_bytes / 1e6:.1f} MB), distinct events: {EVENTS}, resends: {RESENDS} ({FLOAT_RESENDS} with qty written as a float)")
print(f"distinct by SHA-256 of the line: {len(plain)} (duplicates found: {len(lines) - len(plain)})")
print(f"distinct by Canon hash: {len(canon)} (duplicates found: {len(lines) - len(canon)})")
print(f"distinct by SHA-256 of Python's sorted-key JSON: {len(sorted_json)} (duplicates found: {len(lines) - len(sorted_json)})")
print(f"left as different values by Canon: {len(canon) - EVENTS} (the float-qty resends)")
print(f"time, SHA-256 of lines: {t_plain:.2f} s; parse with json.loads + Canon hash: {t_canon:.2f} s ({len(lines) / t_canon:,.0f} records/s); sorted-key JSON: {t_sorted:.2f} s; one run")
