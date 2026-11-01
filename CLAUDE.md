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
- PyPI and npm uploads can't be replaced. Once a version is on either, any
  further change, even a fix to the release itself, ships as a new version.
- GitHub job logs can't be downloaded from these sessions, but annotations can
  (`gh api repos/softdevstudio/bxm/check-runs/<job id>/annotations`). Scripts
  print `::error::` lines for that reason.

## In progress: BXM 0.4 Phase A (softdev-plan item 36, approved by YK 2026-11-01)

What and why: Memo's main use is CI, but the README only says "point it at a cache"; and two runs writing the same outputs at once can store a half-rewritten file (README states it). npm/PyPI publishing is not in this phase: it needs YK's registry accounts and he hasn't decided.

Design decisions:
- Output locks: before the cache lookup, `bxm-memo run` takes an exclusive OS lock for each declared output, in sorted order (no deadlock between runs), and holds them through restore or run-and-store. Lock files live in the cache under `locks/`, one per output path (SHA-256 of the absolute path, with the parent's symlinks resolved), so nothing is written into the project. flock on Linux, macOS and the BSDs, LockFileEx on Windows, stdlib only (syscall, kernel32 via NewLazyDLL); the OS drops a lock when its process dies, SIGKILL included. A second identical run waits, then gets a hit instead of running twice. While waiting it prints one "waiting for another run writing X" line. Runs sharing outputs but using different caches are not serialised; the README says so. `explain` takes no locks.
- Memo GitHub Action at `memo/action.yml` (`uses: softdevstudio/bxm/memo@v0.4.0`): composite, downloads the BXM release for the runner (pinned to the action's tag, else latest), checks SHA256SUMS, puts bxm-memo on PATH, sets BXM_MEMO_CACHE, and restores and saves that folder with actions/cache (key per OS, arch, a user key and the commit, restore-keys on the prefix). Later steps call bxm-memo themselves. Tested by `.github/workflows/memo-action-test.yml` on Linux, macOS and Windows (second run in the job gets a hit), and on the published binaries after each release.
- Upkeep: memo/go.mod to 1.26; README Python link to latest; README CI line names the Windows Python job.

Steps:
1. [x] Output locks (memo/internal/memo/lock*.go), with tests: lock_test.go (exclusion and wait callback, two spellings share a lock, unrelated outputs don't block, 8 goroutines locking in opposite orders don't deadlock, nothing written into the project) and three end-to-end tests (two different runs sharing an output store only finished files, four identical runs run the command once, a SIGKILLed holder doesn't block the next run). With locking switched off those tests fail. memo/go.mod raised to 1.26 here. CI gains a memo-windows job for LockFileEx.
2. [x] Memo Action (memo/action.yml) and .github/workflows/memo-action-test.yml: fill job (miss then hit in one job, bad `cache` input refused) and restore job (cache saved by the fill job's post step gives a hit, command not run) on 3 OS; download job after each release, gated on the newest release being for that commit, pinned and latest. Install step simulated locally against the real v0.3.0 release (download, SHA256SUMS, unpack).
3. [ ] Upkeep, README, CHANGELOG.
4. [ ] Independent review, fixes, re-record test/results.
5. [ ] Version 0.4.0 in the four places, release, verify downloads and the Action on published binaries.
6. [ ] bxm.net post, softdevstudio.com note, softdev-plan.md status.

Local note: this cloud session has Go 1.24 only, so local Memo tests run with a scratch go.mod; CI on stable Go is the real check.

## State

0.3.0: Canon adds the Python package (python/), PyO3 bindings to the core.
0.2.0: Canon adds the JavaScript package (js/), built from the core as WebAssembly.
0.1.0: both tools built, tested, recorded; Canon has the Rust library, CLI and C API.

## Next (approved Oct 22 2026; the owner left these calls to Claude)

1. Done in 0.2.0: Canon JavaScript package (npm publish waits for an NPM_TOKEN secret).
2. Done in 0.3.0: Canon Python package (python/; PyPI upload waits for a PYPI_TOKEN secret).
3. Dropped Oct 22 2026 (owner's call): Memo remote cache on S3/R2. Sharing
   already works through CI caches (actions/cache on BXM_MEMO_CACHE) and
   shared folders (--cache on a network drive). If cross-machine sharing is
   ever wanted, the preferred design is a plain-HTTP `bxm-memo serve`, not S3.

The approved plan is complete. Further work only on the owner's request.

Decided against: switching Memo's manifests to Canon. It would need cgo or a
second Canon implementation in Go, and either one breaks a promise (Memo as
one static binary, Canon as one implementation). Memo keeps sorted JSON.
