# 0008 — HTTP Surface, Authentication, and the Implemented Internal-Key Hardening

## Status

Accepted — records the surveyed surface and the internal-key hardening
**implemented on 2026-11-04** (branch `feat/internal-key-auth`), following
the organization's existing internal-key convention.

## Date

2026-11-04 (survey and implementation)

## Context

The service exposes two HTTP listeners: the main API (`:2704`) and the
maintenance API (`:2705`). A full survey (below) found the main API entirely
unauthenticated. The author decided SDKHandler must not be unauthenticated:
its only intended caller was believed to be the HA main program (HRPAuth) —
this premise was later corrected; see "Author Decisions (2026-11-05)" — and
the mechanism must be the organization's existing **Internal Key**
convention, not a new one.

## Author Decisions (2026-11-05)

The author closed the four open items from the implementation report. These
decisions are final for the current design; do not re-open them without new
input from the author.

1. **The main program does not call SDKHandler.** No HRPAuth code that
   triggers or otherwise calls this service will be added, and no further
   accommodation for a "who is the caller" question is needed. The current
   state is kept as-is.
2. **The `/health` exemption stays** exactly as implemented.
3. **The static shared key stays.** No switch to the dynamic
   presence/`auth-key` mode.
4. **No constant-time comparison.** The direct string comparison is kept,
   and no service in the organization is changed for this reason.

Two facts must be read together with these decisions, to prevent future
misreading:

- **No service calls the protected `:2704` endpoints today, and the main
  program does not plan to.** The internal key protects against **local
  operations/manual scripts and any unauthorized access** — it is *not* an
  inter-service call credential.
- **The key is a static shared secret with no rotation mechanism**
  (following the organization's static precedent, e.g. `manage.token`), and
  **the comparison is not constant-time**. Both are recorded as **known,
  deliberate trade-offs accepted by the author — not defects awaiting a
  fix**.

## Organization Convention (verified before implementing)

Evidence from sibling repos, as of 2026-11-04:

- **Header name** is uniformly `X-Internal-Key`
  (HRPAuth `registry/heartbeat.go:64`, `controllers/hook_runtime.go:135`;
  HRPAuth-Yggdrasil-API `controllers/internal_controller.go:23`, `:32`).
- **Verification pattern**: the receiver compares the header against its
  configured key; empty configured key rejects everything (fail-closed);
  failure returns **401** with code `invalid_credentials` and message
  "invalid internal key" inside the unified envelope
  (HRPAuth-Yggdrasil-API `controllers/internal_controller.go:23-27`,
  `controllers/api_response.go:109-128`).
- **Comparison** is a direct string comparison in all existing services; no
  service uses `subtle.ConstantTimeCompare`.
- **Key generation** in HRPAuth uses `utils.GenerateRandomToken(n)`
  (crypto/rand, hex-encoded): 64 bytes → 128 hex chars for dynamic internal
  keys (HRPAuth `registry/presence.go:92`), 32 bytes → 64 hex chars for the
  Manage Token (HRPAuth `controllers/startup_controller.go:226-229`).
- **Static shared-key precedent**: `manage.token` ← env `MANAGE_TOKEN`
  (HA-Services-Deployer `deploy/config/hrpauth.yaml.tmpl`,
  `deploy/.env.example`); env vars use per-service prefixes
  (`SDKHANDLER_*` for this service).
- **Health endpoints are exempt elsewhere** (HA-Services-Deployer and
  HA-Services-MaintenanceView expose unauthenticated `/healthz`).
- No rotation for static keys (manual); dynamic keys rotate on every
  `POST /services/auth-key` fetch and are revoked on heartbeat failure
  (HRPAuth `registry/heartbeat.go:66-70`). No service logs key values; no
  rate limiting exists on internal endpoints anywhere.

## Decision (implemented)

`internalKeyAuthMiddleware` (`main.go`, added next to `opsAuthMiddleware`)
protects the main API's business routes:

- Protected: `GET /api/build/status`, `POST /api/build/trigger`,
  `GET /api/revision` (`main.go:559-563`).
- **Exempt: `/health`** — liveness probes should not depend on the shared
  secret; matches the org pattern of open health endpoints. (Flagged for the
  author: if `:2704` is firewalled anyway, the exemption is harmless; if a
  stricter posture is wanted, this is a one-line change.)
- **Exempt: static artifact hosting** (`serve_artifacts: true`, off by
  default) — those routes serve the built frontend to browsers, which cannot
  carry an internal key.
- `:2705` maintenance auth (ops credential via HRPAuth) is **unchanged**.

Contract of the implemented check:

| Item | Value |
|---|---|
| Header | `X-Internal-Key` |
| Config field | `hrpauth.internal_key` (config.yaml) |
| Env var (compose) | `SDKHANDLER_INTERNAL_KEY` |
| Failure status | `401` for missing, malformed, or wrong key |
| Failure body | `{"success":false,"code":"invalid_credentials","message":"invalid internal key"}` |
| Key not configured | **fail-closed** — all protected routes return 401 (org convention per Yggdrasil `internal_controller.go:24` and Manage Token `auth_service.go:202`) |
| Comparison | direct string comparison (org convention) |
| Logging | rejections are logged with method+path only; the key value is never logged |

Tests: `main_internal_key_test.go` covers no-header 401, wrong-key 401,
correct-key pass-through, and unconfigured fail-closed.

## Consequences

- **Impact on existing keyless callers:** any previous consumer of
  `/api/build/trigger`, `/api/build/status`, `/api/revision` that does not
  send the key now gets 401. At implementation time no service calls these
  endpoints (verified: HRPAuth contains no client for SDKHandler's API; the
  only real-world callers were manual operator requests), so the blast radius
  is operator scripts, which must now set the header.
- **Key consistency** is a deployment duty only where a caller exists: the
  value lives in SDKHandler's `hrpauth.internal_key` (compose: one `.env`
  value, `SDKHANDLER_INTERNAL_KEY`). Since no service calls these endpoints
  (see "Author Decisions (2026-11-05)"), today the key is only shared with
  authorized operator tooling/scripts.
- Remaining exposure notes from the survey still apply: no rate limiting
  (only the global build mutex), builds are not cancelled by client
  disconnect, compose maps the ports on all host interfaces by default.
  Loopback binding / firewalling remains the cheapest additional layer.
- **Rate limiting / HTTP timeouts are deliberately NOT added** (2026-11-05,
  recorded only): the only existing limits are the global build mutex (busy
  → 409) and the build timeout (default 1200s,
  `internal/config/config.go` `applyDefaults`). The `http.Server` instances
  set no read/write/idle timeouts. Introducing rate limits or server
  timeouts requires an author decision on values; none was imposed.

## HRPAuth-side work (closed — nothing to dispatch)

- Verified on 2026-11-04: **HRPAuth contains no code that calls SDKHandler**
  (`config/config.go` `SDKPackagesConfig` is broker storage config only;
  `controllers/sdk_package_controller.go:22` documents the flow as
  "SDKHandler pulls from HRPAuth").
- Author decision 2026-11-05: it stays that way. No HRPAuth caller will be
  built, so there is no HRPAuth-side change to dispatch and no key to keep in
  sync with a main-program config item. This section is kept only as the
  record of why the protected endpoints currently have no service consumer.

## Survey (kept from the original entry)

### Endpoint inventory — main API, `server.listen_addr` (default `:2704`)

Registered in `main.go:551-567` on a bare `gin.New()` with only
`gin.Recovery()`.

| Method | Path | Auth (after this change) | Handler / effect |
|---|---|---|---|
| GET | `/health` | none (exempt, see above) | `{"status":"ok"}` (`main.go:555-557`) |
| GET | `/api/build/status` | `X-Internal-Key` | latest build record (`main.go` handleBuildStatus) |
| POST | `/api/build/trigger` | `X-Internal-Key` | runs a full build synchronously; 409 when busy |
| GET | `/api/revision` | `X-Internal-Key` | published revision info |
| GET | `/` | none (browser-facing; only when `serve_artifacts: true`) | serves `index.html` of current artifacts |
| * | NoRoute | none (browser-facing; only when `serve_artifacts: true`) | static files from the artifacts dir |

### `POST /api/build/trigger` — call chain (unchanged by this work)

1. `handleBuildTrigger` → `triggerBuild(ctx, force=false)`:
   `buildMu.TryLock()` — busy → 409 (the only throttle).
2. `preRunCheck` → `ensureTemplateRepo`: if the template directory is
   empty/missing and a git URL is configured, `git clone` + `npm install`
   run before any build (note: this also blocks HTTP startup on first run).
3. `runOnce`: `broker.List()` with ETag — unchanged list skips the build;
   otherwise download all packages → copy template → inject → compile
   (default 20-minute timeout, own context from `context.Background()` in
   `internal/build/build.go:70`, so **client disconnect does not cancel the
   build**) → publish (remove-then-rename window, see 0004).

### Maintenance API, `:2705` (unchanged)

- `POST /api/maintenance/template/pull`, `GET /api/maintenance/sdk/list`,
  both behind `opsAuthMiddleware` (`main.go`): `Authorization: Bearer
  <token>` verified by calling HRPAuth `GET /services/sdk-packages` with the
  caller's token (`internal/broker/broker.go:367-388`). Missing/malformed
  header → 401; rejected token → 403; HRPAuth error → 500. One HRPAuth
  round-trip per request (no cache); maintenance auth is down when HRPAuth
  is down.

## Evidence

- Implementation: branch `feat/internal-key-auth` commits (middleware + config field + tests; example/README; this entry).
- `main.go` — `internalKeyAuthMiddleware` and route wiring (`/api/*` protected, `/health` exempt).
- `internal/config/config.go` — `HRPAuthConfig.InternalKey` (`yaml:"internal_key"`).
- `main_internal_key_test.go` — four middleware cases.
- `config.example.yaml` — `hrpauth.internal_key` with fail-closed note.
- Convention sources: HRPAuth-Yggdrasil-API `controllers/internal_controller.go:23-35`, `controllers/api_response.go:109-128`; HRPAuth `registry/presence.go:83-98`, `registry/heartbeat.go:38-70`, `services/auth_service.go:200-203`, `controllers/startup_controller.go:226-229`; HA-Contract `agents.md` Unified Response Format; HA-Services-Deployer `deploy/config/hrpauth.yaml.tmpl` (`manage.token` ← `MANAGE_TOKEN`).
