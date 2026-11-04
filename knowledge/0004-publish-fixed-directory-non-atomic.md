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
is no longer switched (`internal/publish/publish.go::Publish`). `current_link`
survives only as the path read by `/api/revision` and the optional static
hosting (`internal/publish/publish.go::ResolveCurrent`, `main.go::handleRevision`,
`main.go::handleStatic`).

## Consequences (known defects)

- **Non-atomic window.** Between the `RemoveAll` (step 2) and the `Rename`
  (step 3) the artifacts directory does not exist. Any web server serving
  that path returns 404/500 during the window. The rename itself is atomic on
  Linux, but the remove-then-rename sequence is not. The package doc still
  claims "atomic switch of the current pointer" via symlink
  (`internal/publish/publish.go:L1-L9`) — stale comment, recorded as-is.
- **README overstates atomicity.** `README.md:L3-L5 @ 76c5199` says the service
  "publishes the resulting static assets atomically" (added in `76c5199`).
  That is not what the code guarantees.
- **No rollback history.** A broken-but-successful publish cannot be rolled
  back from disk; only the state record remembers the build
  (`internal/state/state.go::BuildRecord`), and only the latest record is kept (see
  0006).
- **Artifact consumer in the platform compose: none.** See the dedicated
  investigation section below ("Artifact Consumer Investigation") — the
  intended consumer is an operator-managed external web server, and the
  compose template wires no serving side at all.
- HA-Contract now describes this behavior accurately (sdk-package.md "Build
  Policy": "between the removal and the rename the artifacts directory is
  absent"), so there is no contract drift — only the in-repo comments and
  README are stale.

## Artifact Consumer Investigation (2026-11-04)

Question from the author: who is supposed to consume the published
artifacts, and does publishing to disk have any point if nobody reads it?

### Findings (verified by ecosystem-wide grep)

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
- **No ecosystem service reads the artifacts.** Grep across all repos under
  the HA workspace for `sdkhandler-artifacts`, `serve_artifacts`,
  `artifacts_dir`, `/app/artifacts` matches only: this repo (code/config),
  the compose template, and wiki pages. In the compose template, the only
  mount of `sdkhandler-artifacts` is the sdkhandler container itself
  [xref] (HA-Services-Deployer `deploy/docker-compose.yml:184`, volume
  declaration `:234`). No nginx/caddy/traefik or any other serving service
  exists in the template. HRPAuth's `NoRoute` is a JSON 404, not static
  hosting [xref] (HRPAuth `main.go:246-256`).
- **Why `serve_artifacts: false` in compose:** consistent with the design
  intent above — the handler is a build service; hosting is delegated out.
  The compose template simply never wires the delegated side.
- **Does publishing to disk still make sense?** Yes for the assumed
  self-host shape: the operator's own web server (e.g. the nginx-style
  hosting implied by the `/www/wwwroot` deploy-root convention used on the
  author's boxes) serves `artifacts_dir`. In a pure `docker compose`
  deployment, nothing serves the volume — the compose path is a real gap,
  not a misunderstanding of this entry.

### Options (author to decide; compose intentionally untouched)

1. **Keep as-is + document:** compose users must add their own static/reverse
   proxy service mounting `sdkhandler-artifacts` (read-only) with SPA
   fallback. Cheapest; keeps the template minimal.
2. **Add a small nginx service to the compose template** mounting the volume
   read-only with SPA fallback to `index.html`. One new service block; makes
   compose deployments self-contained.
3. **Set `serve_artifacts: true`** in the compose config template and use
   `:2704` as the site entry. The former blocker — the built-in static
   handler had **no SPA fallback** — was fixed on 2026-11-05 (see below).
   Remaining caveat: it widens exposure of the API listener (see 0008), so
   the port should be firewalled or proxied.
4. **Drop the volume** from the compose template if composed deployments are
   not expected to serve the frontend at all.

### Static hosting fix (2026-11-05)

- `handleStatic` now serves real files when present and falls back to
  `index.html` for missing non-`/api` paths (SPA routing); unknown `/api/*`
  paths keep a JSON 404, and out-of-bounds paths are rejected
  (`main.go::handleStatic`; tests in `main_static_test.go`).
- Implementation note: the previous handler read `c.Param("filepath")`,
  which is always empty in a gin `NoRoute` handler, so before this fix the
  static hosting never served any file other than `/` — the gap was larger
  than "no SPA fallback". Recorded for accuracy.

## Evidence

- Commit `28c7afb0a876a362c65892e9d6c1844942cd0512` — "Replace versioned artifact storage and current link switching with direct publishes to the configured dist directory".
- `internal/publish/publish.go::Publish` — fixed-dir publish, tmp copy, RemoveAll→Rename.
- `internal/publish/publish.go:L1-L9` — stale package doc (still mentions `artifacts/<build-id>/dist` and current-pointer switching).
- `README.md:L3-L5 @ 76c5199` — "publishes ... atomically" claim (commit `76c5199`).
- `internal/config/config.go::CreateDefault` — code defaults `artifacts_dir`/`current_link` both `./dist`.
- [xref] HA-Services-Deployer repo: `deploy/docker-compose.yml:184`, `:234` (`sdkhandler-artifacts` volume, mounted only by the sdkhandler service), `deploy/config/sdkhandler.yaml.tmpl` (`serve_artifacts: false`).
- [xref] HRPAuth repo `main.go:246-256` — `NoRoute` returns a JSON 404 (no static hosting).
- HRPAuth-WebUI template repo `README.md` "Build and deploy" — "serve it with any web server and configure an SPA fallback"; `htaccess.example` present.
- SamuelWiki `docs/projects/ha-webui-sdkhandler/index.md` — describes `serve_artifacts` as "off by default" (note: that page predates the publish rewrite and still describes the old atomic-symlink behavior; treat as stale). **Staleness handed off to the Wiki maintenance task (2026-11-05); do not fix the Wiki from this repo.**
- Contract: HA-Contract `docs/dev/HRPAuth/sdk-package.md` Build Policy (aligned in HA-Contract commit `4baf9a7`).
