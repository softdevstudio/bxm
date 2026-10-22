# Case studies

Six worked examples, each a script you can run again. Their recorded output
is in `test/results/case-studies`, and the posts on bxm.net quote those files.

| # | Folder | Tool | What it shows |
|---|---|---|---|
| 1 | `01-three-languages` | Canon | One order built in Python, in JavaScript and by hand: three plain JSON hashes, one Canon hash, and a signature made in Python that checks out in JavaScript |
| 2 | `02-config-diff` | Canon | Two copies of a service config with different formatting and key order: a text diff against `bxm-canon diff` |
| 3 | `03-duplicates` | Canon | 100,000 records with 10,000 resends written by other producers: duplicates found by line hash, by Canon hash and by sorted-key JSON |
| 4 | `04-github-actions` | Memo | A release build of bxm-canon in GitHub Actions with the Memo cache kept by actions/cache: cold, warm, a docs edit and a source edit |
| 5 | `05-tests-across-commits` | Memo | The Canon and Memo test suites at every commit of this repository's history, with and without Memo |
| 6 | `06-miss-reasons` | Memo | What Memo says when it misses: an env var, the Python version, an edited, a new and a deleted data file, then everything undone |

Record 1, 2, 3, 5 and 6 with `scripts/record-case-studies.sh`. Case study 4
runs in GitHub Actions (`.github/workflows/case-study-memo.yml`, started by
hand); `04-github-actions/fetch-results.sh RUN_ID` writes its results file.

Case study 1 needs the JavaScript package installed next to it:
`npm install https://github.com/softdevstudio/bxm/releases/latest/download/bxm-canon-js.tgz`
in `01-three-languages`.
