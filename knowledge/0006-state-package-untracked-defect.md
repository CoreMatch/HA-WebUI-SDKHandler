# 0006 — `internal/state` Package Is Not Tracked in Git (Defect)

## Status

Accepted (records a known defect; unfixed at HEAD)

## Date

2026-11-04

## Context

The service persists build records in the `internal/state` package
(`state/builds.json` store). Both `main.go` and `internal/publish/publish.go`
import it.

## Decision

Record the defect precisely:

- `.gitignore:6` contains the pattern `state/`, which matches **any**
  directory named `state` at any depth. Verified:
  `git check-ignore -v internal/state/state.go` →
  `.gitignore:6:state/ internal/state/state.go`.
- `internal/state/state.go` exists in the working tree but has **never been
  committed**: `git log --all -- internal/state/` returns nothing, and
  `git ls-files` lists no file under `internal/state/`.
- `main.go:33` and `internal/publish/publish.go:18` import
  `ha-webui-sdkhandler/internal/state`.

Consequence: **a fresh clone of this repository cannot compile** (`go build`
fails on the missing package). The service only builds in working trees where
`internal/state/state.go` happens to exist locally. The Docker build
(`Dockerfile:9-10`, `COPY . .`) would fail the same way in a clean checkout.

## Consequences (known defects)

- Fresh clone / CI / Docker build from a clean checkout: broken. This is the
  most severe known defect in the repo at HEAD.
- The obvious fix (anchor the ignore pattern to the repo-root runtime state
  directory, e.g. `/state/`, and commit `internal/state/state.go`) has not
  been applied. Not applied here either, per the knowledge-base-only scope of
  this change.
- Secondary defect in the same package: `Store.Record` keeps **only the most
  recent record** (`internal/state/state.go:114-117`), while its doc comment
  claims it keeps the latest success plus the current in-flight/failed record
  (`internal/state/state.go:96-97`). Effects:
  - after a failed build, the previous successful record is gone;
  - `/api/revision` then reports no build metadata
    (`internal/publish/publish.go:146-164`);
  - on restart, the ETag restore (`main.go:525-529`) finds no success record
    and the next startup triggers a redundant full rebuild.

## Evidence

- `.gitignore:6` — `state/`.
- `git check-ignore -v internal/state/state.go` → matched by `.gitignore:6`.
- `git log --all -- internal/state/` → empty (never tracked).
- `main.go:33`, `internal/publish/publish.go:18` — imports of the untracked package.
- `internal/state/state.go:114-117` — truncation to one record; `:96-97` — contradicting doc comment.
- Note: the "fresh clone cannot compile" conclusion follows deterministically
  from the tracking evidence above; it was not re-verified by performing an
  actual clean-clone build (marked accordingly, though the tracking facts
  leave no room for doubt).
