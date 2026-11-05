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
   `<artifacts_dir>.tmp` (`internal/publish/publish.go::Publish`).
2. The existing artifacts directory is **removed**
   (`internal/publish/publish.go::Publish`).
3. The temp directory is renamed into place
   (`internal/publish/publish.go::Publish`).

`Publish` writes to the configured `build.artifacts_dir` directly; the build
ID is not part of the path, there is no versioned history, and `current_link`
is not switched (`internal/publish/publish.go::Publish`). `current_link`
serves only as the path read by `/api/revision` and the optional static
hosting (`internal/publish/publish.go::ResolveCurrent`, `main.go::handleRevision`,
`main.go::handleStatic`).

## Consequences (known defects)

- **Non-atomic window.** Between the `RemoveAll` (step 2) and the `Rename`
  (step 3) the artifacts directory does not exist. Any web server serving
  that path returns 404/500 during the window. The rename itself is atomic on
  Linux, but the remove-then-rename sequence is not. The package doc still
  claims "atomic switch of the current pointer" via symlink
  (`internal/publish/publish.go:L1-L9`) — stale comment, recorded as-is.
- **No rollback history.** A broken-but-successful publish cannot be rolled
  back from disk; only the state record remembers the build
  (`internal/state/state.go::BuildRecord`), and only the latest record is kept (see
  0006).
- **Artifact consumer in the platform compose: optional, profile-gated.**
  See "Artifact Consumer" below — the intended consumer is an
  operator-managed external web server, and the compose template wires an
  optional nginx consumer (`ha-webui-nginx`) behind the `webui` compose
  profile (off by default).
- **No contract drift.** HA-Contract describes this behavior accurately
  (sdk-package.md "Build Policy": "between the removal and the rename the
  artifacts directory is absent"), and the README describes the actual
  remove-then-rename behavior (`README.md:L3-L6`). The only stale description
  is the in-repo package comment.

## Artifact Consumer

Who consumes the published artifacts:

- **Intended consumer: an external, operator-managed web server.** Two
  documents state this:
  - `config.example.yaml (field: serve_artifacts)` (this repo): `serve_artifacts` defaults to
    false because "当前阶段只提供 API，前端产物由其他 Web 服务器托管"
    (at this stage only the API is provided; the frontend artifacts are
    hosted by another web server).
  - The frontend template repo's README (CoreMatch/HRPAuth-WebUI,
    `README.md` "Build and deploy"): the build output is "fully static —
    serve it with any web server and configure an SPA fallback to
    `index.html`", with `htaccess.example` provided for Apache.
- **The platform compose template wires an optional, profile-gated nginx
  consumer.** `ha-webui-nginx` is gated behind the `webui` compose profile
  (off by default); it mounts `sdkhandler-artifacts` read-only and serves it
  with SPA fallback to `index.html`
  [xref] (HA-Services-Deployer `deploy/docker-compose.yml:321-333`,
  `deploy/nginx-sdkhandler-artifacts.conf`). With the profile disabled — the
  default compose deployment — nothing serves the volume. The compose
  template sets `serve_artifacts: false`, consistent with the design intent
  that the handler is a build service and hosting is delegated out.
- **No other ecosystem service reads the artifacts.** Grep across all repos
  under the HA workspace for `sdkhandler-artifacts`, `serve_artifacts`,
  `artifacts_dir`, `/app/artifacts` matches only this repo (code/config),
  the compose template, and wiki pages. HRPAuth's `NoRoute` is a JSON 404,
  not static hosting [xref] (HRPAuth `main.go:246-256`).

Publishing to disk makes sense for the assumed self-host shape: the
operator's own web server (e.g. the nginx-style hosting implied by the
`/www/wwwroot` deploy-root convention used on the author's boxes) serves
`artifacts_dir`.

### Built-in static hosting

`handleStatic` serves real files when present and falls back to
`index.html` for missing non-`/api` paths (SPA routing); unknown `/api/*`
paths keep a JSON 404, and out-of-bounds paths are rejected
(`main.go::handleStatic`; tests in `main_static_test.go`).

## Evidence

- Commit `28c7afb0a876a362c65892e9d6c1844942cd0512` — "Replace versioned artifact storage and current link switching with direct publishes to the configured dist directory".
- `internal/publish/publish.go::Publish` — fixed-dir publish, tmp copy, RemoveAll→Rename.
- `internal/publish/publish.go:L1-L9` — stale package doc (still mentions `artifacts/<build-id>/dist` and current-pointer switching).
- `README.md:L3-L6` — describes the remove-then-rename publish behavior.
- `internal/config/config.go::CreateDefault` — code defaults `artifacts_dir`/`current_link` both `./dist`.
- [xref] HA-Services-Deployer repo: `deploy/docker-compose.yml:321-333` (`ha-webui-nginx`, `webui` profile, read-only mount of `sdkhandler-artifacts`), `deploy/nginx-sdkhandler-artifacts.conf` (SPA fallback), `deploy/config/sdkhandler.yaml.tmpl` (`serve_artifacts: false`).
- [xref] HRPAuth repo `main.go:246-256` — `NoRoute` returns a JSON 404 (no static hosting).
- HRPAuth-WebUI template repo `README.md` "Build and deploy" — "serve it with any web server and configure an SPA fallback"; `htaccess.example` present.
- SamuelWiki `docs/projects/ha-webui-sdkhandler/index.md` — describes `serve_artifacts` as "off by default" but still describes the old atomic-symlink publish behavior; treat that page as stale. **Staleness handed off to the Wiki maintenance task; do not fix the Wiki from this repo.**
- Contract: HA-Contract `docs/dev/HRPAuth/sdk-package.md` Build Policy.
