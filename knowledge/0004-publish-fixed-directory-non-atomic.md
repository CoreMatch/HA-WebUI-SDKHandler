# 0004 — Publishing to a Fixed Directory Is Not Atomic

## Status

Accepted (records current state; the non-atomic window is a known defect)

## Date

2026-11-04

## Context

The publish step was originally designed around versioned artifact
directories (`artifacts/<build-id>/dist`) plus an atomically switched
`current` pointer (symlink + temp name + rename). Commit `28c7afb`
(2026-10-12, "refactor: rewrite publish logic and update paths") replaced
that with direct publishing into one fixed directory.

## Decision

Record what publishing actually does today:

1. `dist/` from the build work directory is copied to
   `<artifacts_dir>.tmp` (`internal/publish/publish.go:50-55`).
2. The existing artifacts directory is **removed**
   (`internal/publish/publish.go:59`).
3. The temp directory is renamed into place
   (`internal/publish/publish.go:60`).

`Publish` writes to the configured `build.artifacts_dir` directly; the build
ID is not part of the path, there is no versioned history, and `current_link`
is no longer switched (`internal/publish/publish.go:38-43`). `current_link`
survives only as the path read by `/api/revision` and the optional static
hosting (`internal/publish/publish.go:126-164`, `main.go:705-722`).

## Consequences (known defects)

- **Non-atomic window.** Between the `RemoveAll` (step 2) and the `Rename`
  (step 3) the artifacts directory does not exist. Any web server serving
  that path returns 404/500 during the window. The rename itself is atomic on
  Linux, but the remove-then-rename sequence is not. The package doc still
  claims "atomic switch of the current pointer" via symlink
  (`internal/publish/publish.go:1-9`) — stale comment, recorded as-is.
- **README overstates atomicity.** `README.md:3-5` says the service
  "publishes the resulting static assets atomically" (added in `76c5199`).
  That is not what the code guarantees.
- **No rollback history.** A broken-but-successful publish cannot be rolled
  back from disk; only the state record remembers the build
  (`internal/state/state.go:29-39`), and only the latest record is kept (see
  0006).
- **Artifact consumer is undefined in the platform compose.** The compose
  template (in HA-Services-Deployer `deploy/docker-compose.yml`) mounts a
  `sdkhandler-artifacts` volume at `/app/artifacts`, while the rendered
  config sets `serve_artifacts: false`
  (HA-Services-Deployer `deploy/config/sdkhandler.yaml.tmpl`). Nothing in the
  compose template serves the built frontend. *Pending author confirmation:*
  which web server is supposed to serve `artifacts_dir` in self-hosted
  deployments — the intent is not documented anywhere verified.
- HA-Contract now describes this behavior accurately (sdk-package.md "Build
  Policy": "between the removal and the rename the artifacts directory is
  absent"), so there is no contract drift — only the in-repo comments and
  README are stale.

## Evidence

- Commit `28c7afb0a876a362c65892e9d6c1844942cd0512` — "Replace versioned artifact storage and current link switching with direct publishes to the configured dist directory".
- `internal/publish/publish.go:38-77` — fixed-dir publish, tmp copy, RemoveAll→Rename.
- `internal/publish/publish.go:1-9` — stale package doc (still mentions `artifacts/<build-id>/dist` and current-pointer switching).
- `README.md:3-5` — "publishes ... atomically" claim (commit `76c5199`).
- `internal/config/config.go:126-127` — code defaults `artifacts_dir`/`current_link` both `./dist`.
- HA-Services-Deployer repo: `deploy/docker-compose.yml` (`sdkhandler-artifacts` volume), `deploy/config/sdkhandler.yaml.tmpl` (`serve_artifacts: false`).
- Contract: HA-Contract `docs/dev/HRPAuth/sdk-package.md` Build Policy (aligned in HA-Contract commit `4baf9a7`).
