# HA-WebUI-SDKHandler Knowledge Base

## Positioning

This directory is **the project's own notebook** — ADR-style records kept for
self-hosting and maintenance of this service.

- It does **not** define external behavior. The normative contracts live in
  [HA-Contract](https://github.com/CoreMatch/HA-Contract), in particular
  `docs/dev/HRPAuth/sdk-package.md` (SDK package format, generated frontend
  contract, build policy) and `docs/dev/HRPAuth/microservices.md`
  (`/services/sdk-packages` endpoints).
- When an entry here conflicts with HA-Contract, **fix the entry, not the
  contract**.
- Entries describe the verified current state at the time of writing. Every
  claim carries evidence (a stable anchor — `file::Symbol`,
  `file (field: key)`, or `file:Lx-Ly [@ commit]` — and/or commit hash;
  cross-repo citations are marked `[xref]`). Claims that could not be
  verified are marked `not verified`. Gaps and broken promises are recorded
  as-is, without sugar-coating.

## Entries

| ID | Title | One-line claim |
|---|---|---|
| [0001](0001-scope-and-positioning.md) | Scope and positioning | Build-time aggregator: polls SDK packages from HRPAuth only, injects them into the HRPAuth-WebUI template, compiles, and publishes static assets. |
| [0002](0002-injection-overwrites-src-generated.md) | Injection overwrites `src/generated/` | Every build re-injects all SDKs and regenerates six glue files consumed by five host mount points. |
| [0003](0003-build-policy-deviations.md) | Build policy deviations (broken promises) | The build pipeline runs no `npm install` and never reads `run_scripts`; dependencies must already exist in the template's `node_modules`. |
| [0004](0004-publish-fixed-directory-non-atomic.md) | Publishing to a fixed directory is not atomic | Publish removes the artifacts directory and then renames a temp copy into place; there is a window with no artifacts and no version history. Intended consumer is an external operator web server; the compose template has an optional profile-gated nginx consumer (off by default). Built-in static hosting has an SPA fallback. |
| [0005](0005-sdk-sourcing-and-upload-chain.md) | SDK sourcing and the upload chain | SDKs arrive via satellite uploads to HRPAuth (e.g. HRPAuth-Yggdrasil-API); this handler only ever pulls from HRPAuth, and never verifies list metadata sha256 against downloaded bytes. |
| [0006](0006-state-package-untracked-defect.md) | `internal/state` tracking and record retention | `internal/state` is tracked so fresh clones compile; the one-record retention policy is recorded as current behavior with impacts. |
| [0007](0007-configuration-shape.md) | Configuration shape | `config.yaml` at version `"4"` with a migration chain; defaults, the example file, and the compose template disagree on artifact paths. |
| [0008](0008-http-surface-and-auth.md) | HTTP surface, authentication, and the internal-key hardening | `/api/*` on `:2704` requires the org-standard `X-Internal-Key` (fail-closed when unconfigured; `/health` exempt); `:2705` uses ops auth; full endpoint survey included. |

## Maintenance notes

- Records are written against `main` at commit `c585a7b` (2026-11-04).
  Author decisions of 2026-11-05 (no HRPAuth caller, `/health` exempt, static
  key without rotation, no constant-time comparison) are recorded in 0008.
- Do not edit these entries to make the service look better than it is; record
  reality and link the evidence.
