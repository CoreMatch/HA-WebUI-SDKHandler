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
its only intended caller is the HA main program (HRPAuth), and the mechanism
must be the organization's existing **Internal Key** convention, not a new
one.

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
- **Key consistency** is a deployment duty: the same value must be placed in
  SDKHandler's `hrpauth.internal_key` and in whatever the HRPAuth side will
  send (see "HRPAuth-side work" below). In compose this is one `.env` value
  (`SDKHANDLER_INTERNAL_KEY`).
- Remaining exposure notes from the survey still apply: no rate limiting
  (only the global build mutex), builds are not cancelled by client
  disconnect, compose maps the ports on all host interfaces by default.
  Loopback binding / firewalling remains the cheapest additional layer.

## HRPAuth-side work (not in this repo; to be dispatched separately)

- As of 2026-11-04, **HRPAuth contains no code that calls SDKHandler**
  (verified: `config/config.go` `SDKPackagesConfig` is broker storage config
  only; `controllers/sdk_package_controller.go:22` documents the flow as
  "SDKHandler pulls from HRPAuth"). The premise "the only caller is the main
  program" therefore describes the *intended* future caller, not existing
  code — the author should confirm what HRPAuth is supposed to call
  (e.g. trigger a build after `POST /services/sdk-packages` upload).
- That caller must send `X-Internal-Key: <key>` and read `<key>` from a
  HRPAuth config item kept consistent with SDKHandler's
  `hrpauth.internal_key`.

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
