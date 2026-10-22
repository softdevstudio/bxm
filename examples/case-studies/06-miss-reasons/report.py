# A small report step: sums sales per region from data/*.csv into
# out/report.txt. REPORT_CURRENCY changes the output, so Memo has to know it.
import csv, glob, os, sys
cur = os.environ.get("REPORT_CURRENCY", "EUR")
totals = {}
for path in sorted(glob.glob("data/*.csv")):
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            totals[row["region"]] = totals.get(row["region"], 0.0) + float(row["amount"])
os.makedirs("out", exist_ok=True)
with open("out/report.txt", "w") as f:
    f.write(f"built with Python {sys.version.split()[0]}\n")
    for region, total in sorted(totals.items()):
        f.write(f"{region:<8} {total:>12,.2f} {cur}\n")
print(f"report: {len(totals)} regions")
