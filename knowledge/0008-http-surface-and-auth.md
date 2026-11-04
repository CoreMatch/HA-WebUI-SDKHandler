# 0008 — HTTP Surface and Authentication (Full Survey)

## Status

Accepted (read-only survey of the current state, 2026-11-04; the
unauthenticated main API is a known risk, no code changed)

## Date

2026-11-04

## Context

The service exposes two HTTP listeners with different trust assumptions: the
main API (`:2704`) and the maintenance API (`:2705`). The author requested a
full current-state survey of both surfaces before deciding on any changes.
This entry records that survey. Nothing in it is implemented; it is a
faithful description plus minimal hardening options.

## Decision

Record the verified surface.

### Endpoint inventory — main API, `server.listen_addr` (default `:2704`)

Registered in `main.go:551-567` on a bare `gin.New()` with only
`gin.Recovery()`. **No authentication, no rate limiting, no origin checks on
any of them.**

| Method | Path | Validation | Handler / effect |
|---|---|---|---|
| GET | `/health` | none | returns `{"status":"ok"}` (`main.go:555-557`) |
| GET | `/api/build/status` | none | latest build record incl. error text and artifact path (`main.go:669-683`) |
| POST | `/api/build/trigger` | none (only a concurrency lock) | runs a full build synchronously (`main.go:686-693`) |
| GET | `/api/revision` | none | published revision info (`main.go:696-703`) |
| GET | `/` | none | only when `serve_artifacts: true` — serves `index.html` of current artifacts (`main.go:565`, `705-712`) |
| * | NoRoute | none | only when `serve_artifacts: true` — serves static files from the artifacts dir (`main.go:566`, `714-722`) |

`serve_artifacts` defaults to `false` (`config.example.yaml:35-37`), so the
last two rows are off by default.

### `POST /api/build/trigger` — complete call chain

1. **Who can trigger:** anyone who can open a TCP connection to `:2704`. No
   credential, header, or source address is checked (`main.go:686-693`).
2. `handleBuildTrigger` → `triggerBuild(ctx, force=false)`
   (`main.go:475-489`):
   - `buildMu.TryLock()` — if a build is already running, returns "busy" and
     the handler answers **409** (`main.go:476-478`, `686-689`). This is the
     only throttle in the whole chain: a single global mutex, no per-client
     or per-time-window rate limit.
3. `preRunCheck` (`main.go:236-244`) → `ensureTemplateRepo`
   (`main.go:247-284`): if the template directory is empty/missing and
   `build.template_git_url` is set, it runs `git clone` **plus
   `npm install`** in the template directory before any build starts.
4. `runOnce(force=false)` (`main.go:84-136`):
   - `broker.List()` against HRPAuth with ETag; if the SDK list is unchanged
     the request ends with "no change" (this makes repeated spam cheap **only
     while the list is unchanged**);
   - otherwise downloads every SDK package, records a `building` state, then
     `execute` (`main.go:139-234`): copy template → inject → compile →
     publish.
5. Compilation (`internal/build/build.go:62-127`) runs with its **own**
   timeout context created from `context.Background()`
   (`internal/build/build.go:70`), default 20 minutes
   (`internal/build/build.go:59`, `internal/config/config.go:205-207`).
   Consequence: **an HTTP client that disconnects does not cancel the
   build** — the request context is not propagated into the build steps.
6. Publish removes and replaces the artifacts directory (see 0004), so a
   triggered build also produces the non-atomic serving window.

Summary of limits: no authentication, no rate limit, no payload (the POST
body is ignored), one global build-at-a-time lock, 20-minute default build
timeout.

### Maintenance API, `server.maintenance_addr` (default `:2705`) — ops-checked

Routes (`main.go:575-584`), both behind `opsAuthMiddleware`
(`main.go:576-577`, applied to the whole router):

| Method | Path | Effect |
|---|---|---|
| POST | `/api/maintenance/template/pull` | `git checkout .` + `git pull` on the template; on change triggers a forced build in a background goroutine (`main.go:645-666`, `326-331`) |
| GET | `/api/maintenance/sdk/list` | detailed SDK package list (`main.go:725-736`) |

**Authentication mechanics** (`opsAuthMiddleware`, `main.go:613-642`):

- Fields checked: the `Authorization` header must be exactly two tokens with
  scheme `bearer` (case-insensitive) — `Bearer <token>`; nothing else in the
  request is validated.
- Failure behavior:
  - header missing → **401** `Authorization header required`;
  - malformed (not `Bearer <token>`) → **401** `Invalid Authorization
    format`;
  - HRPAuth verification errors (unreachable / unexpected status) → **500**
    `Failed to verify credential` (also logged);
  - token rejected by HRPAuth (401/403 there) → **403** `Insufficient auth
    level`.
- Credential source and check: the token is verified by
  `VerifyOpsCredential` (`internal/broker/broker.go:367-388`), which calls
  HRPAuth `GET /services/sdk-packages` **with the caller's token**; a 200
  means the token has ops (level 2) rights on that endpoint. So any
  HRPAuth-issued token authorized for the SDK-package list passes; there is
  no separate credential store, no token cache (one HRPAuth round-trip per
  maintenance request), and maintenance auth is unavailable whenever HRPAuth
  is down (500).

## Consequences (exposure analysis, recorded as-is)

- **`:2704` on a reachable network:** anyone reaching the port can
  - spam `POST /api/build/trigger` to occupy the build slot (CPU/disk/node
    work up to 20 minutes per build; further triggers get 409 but the slot
    stays occupied) — a cheap resource-exhaustion vector;
  - repeatedly force the remove-then-rename publish window (0004) on the
    served frontend;
  - read `/api/build/status` and `/api/revision`, which disclose SDK names,
    error strings (may contain filesystem paths), and artifact locations.
  The trigger cannot inject attacker-controlled SDK content (sources come
  only from HRPAuth under the service's own credentials), so the risk is
  DoS/disclosure, not content tampering.
- **`:2705` on a reachable network:** protected, but only by HRPAuth's
  authorization of one endpoint; compromise or over-grant of any ops token
  grants template-pull (which discards local template edits and can trigger
  forced builds).
- **Compose exposure:** the platform compose template publishes both ports
  on the host without loopback binding
  (HA-Services-Deployer `deploy/docker-compose.yml:168-169`:
  `"${SDKHANDLER_PORT:-2704}:2704"`, `"${SDKHANDLER_MAINTENANCE_PORT:-2705}:2705"`),
  so a default `docker compose up` exposes both on all host interfaces.

### Minimal hardening options (not implemented; each with its price)

1. **Bind to loopback / firewall the ports** (compose
   `127.0.0.1:2704:2704` etc., or host firewall). Price: zero code change;
   remote operators lose direct access (need SSH tunnel or a proxy); does
   not help if the trigger caller legitimately lives on another host.
2. **Reuse the existing ops check on sensitive main routes** — apply
   `opsAuthMiddleware` to `/api/build/trigger` (and optionally
   `/api/build/status`, `/api/revision`), keep `/health` open. Price: small
   code change; every trigger needs an ops token; one extra HRPAuth
   round-trip per request (no caching today); trigger becomes unavailable
   when HRPAuth is down.
3. **Shared static secret** (config item, checked as a header on the
   trigger endpoint). Price: small code change; no per-caller identity,
   manual rotation, another secret to protect in config/backups.
4. **Reverse proxy with authentication** (e.g. nginx/OAuth2 proxy) in front
   of `:2704`. Price: operational only, no code; one more component to run.

Options can be combined (1 is the cheapest baseline, 2 reuses existing
machinery). Decision belongs to the author; nothing was changed.

## Evidence

- `main.go:551-567` — main routes, only `gin.Recovery()` middleware.
- `main.go:686-693` — trigger handler; `main.go:475-489` — mutex guard.
- `main.go:236-284` — preRunCheck / ensureTemplateRepo (clone + npm install path).
- `main.go:84-136`, `139-234` — runOnce / execute pipeline.
- `internal/build/build.go:59`, `:62-127`, `:70` — build timeout, detached context.
- `main.go:575-590`, `:613-642` — maintenance server and middleware.
- `internal/broker/broker.go:367-388` — `VerifyOpsCredential`.
- `config.example.yaml:35-37` — `serve_artifacts: false` default.
- HA-Services-Deployer `deploy/docker-compose.yml:168-169` — host port mapping.
- Commits: `6e37c48` (maintenance service and ops endpoints), `b7e3705` (SDK peek/logging/maintenance API), `de9d527` (discard local changes before pull).
