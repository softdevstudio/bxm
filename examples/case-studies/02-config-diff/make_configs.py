# Writes the two copies of one service config used in case study 2.
# deployed.json is how the deploy tool wrote it: two-space indent, keys in
# the order the template lists them. edited.json is the same config after
# someone opened it in another editor that saves with four spaces and sorted
# keys, escapes non-ASCII, and changed two values on the way.
import copy, json

config = {
    "service": "checkout",
    "owner": "payments-team",
    "region": "eu-central",
    "description": "Checkout API für Zahlungen",
    "server": {
        "host": "0.0.0.0", "port": 8443, "read_timeout_ms": 5000,
        "write_timeout_ms": 10000, "max_body_bytes": 1048576,
        "tls": {"cert": "/etc/checkout/tls.crt", "key": "/etc/checkout/tls.key", "min_version": "1.2"},
    },
    "database": {
        "url": "postgres://checkout@db.internal:5432/checkout",
        "pool": {"min": 4, "max": 32, "timeout_ms": 30000, "idle_ms": 600000},
        "statement_timeout_ms": 15000,
        "read_replicas": ["db-r1.internal", "db-r2.internal"],
    },
    "cache": {"backend": "redis", "url": "redis://cache.internal:6379/2", "ttl_s": 300, "max_items": 50000},
    "limits": {"requests_per_minute": 1200, "burst": 200, "retries": 3, "backoff_factor": 1.5},
    "features": {"new_tax_engine": False, "apple_pay": True, "invoice_pdf": True, "fraud_score_threshold": 0.82},
    "logging": {"level": "info", "format": "json", "sample_rate": 0.1, "redact": ["card_number", "cvv", "iban"]},
    "deploy": {"replicas": 3, "max_surge": 1, "max_unavailable": 0, "probe_path": "/healthz"},
}

with open("deployed.json", "w", encoding="utf-8") as f:
    json.dump(config, f, indent=2, ensure_ascii=False)
    f.write("\n")

edited = copy.deepcopy(config)
edited["database"]["pool"]["timeout_ms"] = 3000   # the change someone meant to make, or didn't
edited["deploy"]["replicas"] = 3.0                # a tool turned an int into a float
with open("edited.json", "w", encoding="utf-8") as f:
    json.dump(edited, f, indent=4, sort_keys=True, ensure_ascii=True)
    f.write("\n")
