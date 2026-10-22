# BXM: where the next session starts

This repo holds two products under one roof, bxm.net:

- `canon/` BXM Canon, Rust. Canonical binary encoding. Spec in `spec/CANON.md`,
  conformance vectors in `spec/vectors/`. Library + `bxm-canon` CLI + C API
  (`canon/include/bxm_canon.h`).
- `memo/` BXM Memo, Go, stdlib only. Command memoizer, `bxm-memo` CLI.

Site repo: github.com/photozeitgeist/bxm.net (Hugo, Launch template, content
and config values only; never touch the theme).

## Workflow (low maintenance)

- The owner does not publish releases. Raise the version in `VERSION`,
  `canon/Cargo.toml` and `const Version` in `memo/cmd/bxm-memo/main.go`, add a
  `CHANGELOG.md` section, push to main. When CI is green, `release.yml` builds
  Linux/macOS/Windows archives, smoke-tests them, tags `vX.Y.Z` and publishes.
  Then watch the run, check the release files and links, run a downloaded
  binary. `scripts/check-version.sh` keeps the four places in step.
- Every number on the site comes from `test/results/`, written by
  `scripts/record-results.sh`. Re-record after any change that could move a
  number, and update the site from the new files.
- An independent review agent looks at code and site before a release is
  called done.
- main is the default branch. No side branches.

## State

0.1.0: both tools built, tested, recorded. Canon has the Rust library, CLI
and C API.

## Next (proposed; waiting for the owner to approve)

1. Canon JavaScript package: compile the core to WebAssembly, publish to npm.
2. Canon Python package: PyO3 wheels, publish to PyPI.
3. Memo: use Canon for its manifests once a Go binding exists without cgo
   (or keep JSON; decide with the owner).
4. Memo remote cache (S3/R2 compatible) so CI and laptops share results.
