# 0008 — HTTP Surface and Authentication

## Status

Accepted (records current state; unauthenticated main API is a known risk)

## Date

2026-11-04

## Context

The service exposes two HTTP listeners with different trust assumptions: the
main API (`:2704`) and the maintenance API (`:2705`).

## Decision

Record the verified surface:

### Main API, `server.listen_addr` (default `:2704`) — no authentication

Routes registered in `main.go:555-567`:

- `GET /health`
- `GET /api/build/status` — latest build record (`main.go:669-683`)
- `POST /api/build/trigger` — run a build synchronously; 409 when busy
  (`main.go:686-693`); concurrency guarded by a mutex (`main.go:475-479`)
- `GET /api/revision` — current published revision info (`main.go:696-703`)
- optional static hosting of the published artifacts when
  `serve_artifacts: true` (`main.go:563-567`, default `false`)

No middleware authenticates these routes.

### Maintenance API, `server.maintenance_addr` (default `:2705`) — ops-checked

Routes registered in `main.go:575-584`:

- `POST /api/maintenance/template/pull` — `git pull` the template, then
  trigger a forced build in the background (`main.go:645-666`)
- `GET /api/maintenance/sdk/list` — detailed SDK list (`main.go:725-736`)

Guarded by `opsAuthMiddleware` (`main.go:613-642`): `Authorization: Bearer
<token>` is forwarded to HRPAuth by attempting
`GET /services/sdk-packages` with it; success (200) means ops-level
(`internal/broker/broker.go:367-388`).

## Consequences (known defects / operational notes)

- **Anyone who can reach `:2704` can trigger builds** (`POST
  /api/build/trigger`) and read build status. Builds are expensive (full
  frontend compile). In self-hosted deployments the port must be firewalled;
  there is no in-code protection. Recorded as-is.
- The ops check is only as fine-grained as HRPAuth's authorization of
  `GET /services/sdk-packages` (any token able to list packages passes).
- The maintenance pull endpoint runs `git checkout .` before `git pull`,
  discarding local template changes by design ("fully trust upstream",
  `main.go:326-331`, commit `de9d527`).
- `serve_artifacts` defaults to false; see 0004 for the open question of who
  serves the artifacts.

## Evidence

- `main.go:551-567` — main routes, no auth middleware.
- `main.go:575-590` — maintenance server with `opsAuthMiddleware`.
- `main.go:613-642` — middleware implementation.
- `internal/broker/broker.go:367-388` — `VerifyOpsCredential`.
- `main.go:326-331` — discard local template changes before pull.
- Commits: `6e37c48` (maintenance service and ops endpoints), `b7e3705` (SDK peek/logging/maintenance API), `de9d527` (discard local changes before pull).
