# 0006 — `internal/state` Package Tracking and Record Retention Behavior

## Status

Accepted — records the current `internal/state` tracking and the
record-retention behavior; retention is current behavior and its impacts, not
a defect.

## Context

The service persists build records in the `internal/state` package
(`state/builds.json` store). Both `main.go` and `internal/publish/publish.go`
import it.

## Tracking

`internal/state` is tracked in git. The `.gitignore` excludes runtime state
data but re-includes the package source:

```gitignore
state/
!internal/state/
```

Only `internal/state/` is re-included; the negation does not re-include any
other `state/` path. Verified with `git check-ignore -v`:

- `internal/state/state.go` → not ignored (exit 1);
- `state/builds.json` → ignored by `.gitignore:6 fp=14188a31:state/`;
- `internal/foo/state/x.go` → ignored by `.gitignore:6 fp=14188a31:state/`.

`internal/state/state.go` is tracked (`git ls-files internal/state/`), and a
fresh clone compiles: `go build ./...` → OK; `go test ./...` →
`ok ha-webui-sdkhandler/internal/inject` (all other packages: no test files);
`go vet ./...` → OK.

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

- `.gitignore:L6-L9 fp=5d1f7308` (pattern + negation).
- `git check-ignore -v` outputs as listed in Tracking.
- Clean-clone build/test/vet pass as listed in Tracking.
- `internal/state/state.go::Record` — one-record truncation; the doc comment above `Record` describes a different policy.
- `internal/publish/publish.go::LoadRevision`, `main.go::SetETag` — consumers of the retained record.
