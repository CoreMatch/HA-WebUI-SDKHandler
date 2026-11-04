# 0006 — `internal/state` Package Tracking (Fixed) and Record Retention Behavior

## Status

Accepted — the missing-package defect was **fixed on 2026-11-04** (commit
`3214e8d`, merged as `c585a7b`); the record-retention section records
current behavior and its impacts, not a defect.

## Date

2026-11-04 (original record and fix)

## Context

The service persists build records in the `internal/state` package
(`state/builds.json` store). Both `main.go` and `internal/publish/publish.go`
import it.

## The Tracking Defect (history)

As originally recorded:

- `.gitignore:L6` contained the unanchored pattern `state/`, which matched
  **any** directory named `state` at any depth, including `internal/state/`.
- `internal/state/state.go` therefore was **never committed**
  (`git log --all -- internal/state/` was empty), while `main.go:L33` and
  `internal/publish/publish.go:L18` imported the package.
- Consequence: a fresh clone could not compile; only working trees that
  happened to hold the local file could build.

## Resolution (fix of record)

- Commit `3214e8d` ("fix: track internal/state package so clean clones
  compile", merged via `c585a7b`, pushed to `origin/main`) added a precise
  negation to `.gitignore`:

  ```gitignore
  state/
  !internal/state/
  ```

  Only `internal/state/` is re-included. Precision verified with
  `git check-ignore -v`:
  - `internal/state/state.go` → not ignored (exit 1);
  - `state/builds.json` → still ignored by `.gitignore:6:state/`;
  - `internal/foo/state/x.go` → still ignored by `.gitignore:6:state/`
    (no other `state/` path was accidentally re-included).
- `internal/state/state.go` is now tracked (`git ls-files internal/state/`).
- **Clean-clone verification performed:** cloned
  `https://github.com/CoreMatch/HA-WebUI-SDKHandler.git` into a temporary
  directory at HEAD `c585a7b`; `go build ./...` → OK; `go test ./...` →
  `ok ha-webui-sdkhandler/internal/inject` (all other packages: no test
  files); `go vet ./...` → OK. Toolchain: go1.26.4 linux/amd64
  (module requires go 1.25.0).

## Record Retention: Current Behavior and Impacts

This is **how the store works today** (code logic intentionally unchanged —
no authorization to modify it), not a defect classification:

- `Store.Record` keeps **only the most recent record**: after any insert or
  update the slice is truncated to length 1
  (`internal/state/state.go::Record`).
- The doc comment above `Record` describes a different policy (keep the
  latest success plus the current in-flight/failed record,
  `internal/state/state.go::Record`, doc comment); the comment does not match the
  implemented behavior. Recorded as a documentation drift within the file.

Operational impacts of the one-record policy:

- After a failed build, the previous successful record is replaced;
  `/api/build/status` reports the failure (which is intended), but the last
  success is no longer queryable.
- `/api/revision` resolves build metadata from the latest success record
  (`internal/publish/publish.go::LoadRevision`); when the most recent record is a
  failure, revision info degrades to "exists, but no build metadata".
- On restart, the ETag restore (`main.go::SetETag`) looks for the latest
  success record; if the only stored record is a failure, no ETag is
  restored and the next startup performs one redundant full rebuild.
- The published artifacts themselves are unaffected by all of the above: the
  last good `dist` stays on disk regardless of record retention.

## Evidence

- Fix: commit `3214e8d` / merge `c585a7b`; `.gitignore:L6-L9` (pattern + negation).
- `git check-ignore -v` outputs as listed in Resolution.
- Clean-clone build/test/vet results as listed in Resolution (HEAD `c585a7b`).
- `internal/state/state.go::Record` — one-record truncation; the doc comment above `Record` describes a different policy.
- `internal/publish/publish.go::LoadRevision`, `main.go::SetETag` — consumers of the retained record.
