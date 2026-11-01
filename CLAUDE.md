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
3. [x] Upkeep, README, CHANGELOG. GitHub serves no "latest" page of release files for pip (expanded_assets/latest is 404), so the pip line keeps naming a version and check-version.sh now fails when it doesn't name VERSION. README: locks under Rules worth knowing, the Action under In CI, Go 1.26, CI line names the Windows jobs.
4. [x] Independent review, two rounds; round 2 found one regression (a nested run on a sibling output was refused because shared locks were treated as conflicts), fixed by recording each held lock's mode in BXM_MEMO_HELD (key:x / key:s) and refusing only when one side is exclusive, with a unit and an end-to-end test. Fresh test/results recorded locally (Go 1.24 via scratch go.mod; CI runs stable Go).
5. [x] BXM 0.4.0 released by the workflow (commit 2ece11c, 12 assets). The five archives match SHA256SUMS at the tag and latest; expanded_assets/v0.4.0 lists the five wheels for the README's pip line. The downloaded Linux bxm-memo: three parallel identical runs ran the command once (two waited, then hit); a nested run on its parent's output exited 125 at once. Memo action test download job passed on Linux, macOS and Windows, pinned and latest (run 41287342149).
6. [ ] bxm.net post, softdevstudio.com note, softdev-plan.md status.

CI note (2026-11-01): ubuntu-latest became Ubuntu 26.04, which has no Python 3.9, so the python (ubuntu, 3.9) job couldn't start. That row now runs on ubuntu-24.04; the wheels still promise 3.9. When 24.04 runners go away, raising the floor to 3.10 is YK's call.

Review round 1 (2026-11-01, independent agent), all fixed with tests:
- Blocker: overlapping outputs (`out` and `out/r.txt`) took different locks, so a hit could restore another run's file. Now each output locks itself exclusively and every folder above it (inside the working directory) shared; siblings still run together.
- Blocker: a command wrapping its own output with bxm-memo waited forever. Now the held lock keys go to the child in BXM_MEMO_HELD and a nested run on them fails at once; every wait is capped (`--lock-wait`, default 1h, a reminder each minute). Locks are now polled with non-blocking attempts.
- Should-fix: any lock setup error failed the run; now it runs without locks and says so. `clear` deleted locks in use; now it keeps locks/. The Action's pin came from github.action_ref (unreliable in composite actions, wrong for SHA pins); now it reads its own ../VERSION. A relative cache-dir differed between steps; now it is made absolute from the workspace.
- Nits fixed: symlinks resolved in the longest existing part of an output path; `latest` resolved to one tag before downloading. Left: a background child keeping the command's pipes open extends the run (pre-0.4); lock files are never pruned (tiny, one per output path); macOS lowercases paths for lock keys (over-locks on case-sensitive volumes, harmless).

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
