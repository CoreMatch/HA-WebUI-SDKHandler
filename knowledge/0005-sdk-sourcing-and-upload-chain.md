# 0005 — SDK Sourcing and the Upload Chain (HRPAuth Broker, Yggdrasil-API)

## Status

Accepted (records current state)

## Date

2026-11-04

## Context

SDK packages must reach this handler somehow. The contract makes HRPAuth the
single trusted broker: satellite services **upload** packages to HRPAuth, and
this handler **pulls** them from HRPAuth only (HA-Contract
`docs/dev/HRPAuth/sdk-package.md` "Design Positioning";
`docs/dev/HRPAuth/microservices.md` endpoint table).

## Decision

Record the verified shape of the chain:

### Pull side (this repo)

- List: `GET {base}/services/sdk-packages` with OAuth2 Bearer token;
  change detection uses a client-side fingerprint (sha256 over sorted
  `name`/`version`/`sha256` of the list) sent as `If-None-Match`; a 304 or an
  unchanged fingerprint skips the build (`internal/broker/broker.go:117-161`,
  `broker.go:87-113`).
- Auth: OAuth2 client-credentials against `{base}/oauth/token` with
  `client_id`/`client_secret` (`internal/broker/broker.go:225-265`). This
  replaced a static ops token in commit `af4155b` (2026-10-12); the config
  migration v1→v2 deletes `ops_token`
  (`internal/config/migrations.go:28-45`).
- Download: `GET {base}/services/sdk-packages/{name}/download`; the archive
  type is sniffed from magic bytes (`internal/broker/broker.go:314-359`,
  `broker.go:390-398`).
- The last successful build's fingerprint is restored on startup to avoid an
  immediate rebuild (`main.go:525-529`).

### Upload side (cross-repo, verified in HRPAuth-Yggdrasil-API)

- HRPAuth-Yggdrasil-API ships its WebUI SDK inside its own repo (`sdk/`
  with `manifest.json`; `sdk/README.md` states it "is designed to be
  aggregated by HA-WebUI-SDKHandler"), patches profile-card priorities from
  its own config, packs the archive, and uploads it on startup:
  `controllers/sdk_upload.go:130-158` →
  `clients/core_client.go:164-165` (`POST {base}/services/sdk-packages`,
  multipart), with delete-then-reupload on conflict
  (`clients/core_client.go:205`). Verified against
  HRPAuth-Yggdrasil-API checkout at commit `68feb97`.
- Upload/list/download/delete endpoints are ops (level 2) endpoints on
  HRPAuth (HA-Contract `docs/dev/HRPAuth/microservices.md:189-193`).

### Relationship statement

This handler is the **consumer end** of the upload chain: Yggdrasil-API (and
any future satellite) uploads to HRPAuth; HRPAuth validates and stores; this
handler polls and injects. It never talks to satellites, and satellites never
talk to this handler.

## Consequences (including an honest gap)

- **List sha256 is never verified against downloaded bytes.** The list
  metadata carries `sha256` per package, but the orchestrator records the
  hash computed locally during download (`main.go:103-108`,
  `internal/broker/broker.go:347-358`); `pkg.SHA256` from the list is only
  printed for diagnostics (`main.go:744-750`). Integrity therefore relies on
  TLS plus trust in HRPAuth; a mismatch between list metadata and delivered
  bytes would go unnoticed. Recorded as a gap, not verified to be intentional
  — *pending author confirmation*.
- Ops verification for the maintenance port reuses the same endpoint: a token
  is considered "ops" if it can successfully list SDK packages
  (`internal/broker/broker.go:367-388`).
- ETag/fingerprint state lives only in the state store; losing `state/`
  causes one redundant full rebuild after restart.

## Evidence

- `internal/broker/broker.go:117-161` (list + If-None-Match), `:225-265` (OAuth2), `:314-359` (download + local sha256), `:367-388` (ops check).
- `main.go:101-110` (hashes taken from Download), `main.go:525-529` (ETag restore).
- `internal/config/migrations.go:28-45` (ops_token removal).
- Cross-repo (HRPAuth-Yggdrasil-API @ `68feb97`): `controllers/sdk_upload.go:130-158`, `clients/core_client.go:164-165`, `clients/core_client.go:205`, `sdk/README.md`.
- Contract: HA-Contract `docs/dev/HRPAuth/microservices.md:189-193`; `docs/dev/HRPAuth/sdk-package.md` Design Positioning / Data Flow.
- Commits: `af4155b` (OAuth2 replacement), `2c65776` (ETag handling), `b7e3705` (peek/logging/maintenance API).
