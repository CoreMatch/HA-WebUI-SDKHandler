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
  claim carries evidence (`file:line` and/or commit hash). Claims that could
  not be verified are marked `not verified`. Gaps and broken promises are
  recorded as-is, without sugar-coating.

## Entries

| ID | Title | One-line claim |
|---|---|---|
| [0001](0001-scope-and-positioning.md) | Scope and positioning | Build-time aggregator: polls SDK packages from HRPAuth only, injects them into the HRPAuth-WebUI template, compiles, and publishes static assets. |
| [0002](0002-injection-overwrites-src-generated.md) | Injection overwrites `src/generated/` | Every build re-injects all SDKs and regenerates six glue files consumed by five host mount points. |
| [0003](0003-build-policy-deviations.md) | Build policy deviations (broken promises) | The build pipeline runs no `npm install` and never reads `run_scripts`; dependencies must already exist in the template's `node_modules`. |
| [0004](0004-publish-fixed-directory-non-atomic.md) | Publishing to a fixed directory is not atomic | Publish removes the artifacts directory and then renames a temp copy into place; there is a window with no artifacts and no version history. |
| [0005](0005-sdk-sourcing-and-upload-chain.md) | SDK sourcing and the upload chain | SDKs arrive via satellite uploads to HRPAuth (e.g. HRPAuth-Yggdrasil-API); this handler only ever pulls from HRPAuth, and never verifies list metadata sha256 against downloaded bytes. |
| [0006](0006-state-package-untracked-defect.md) | `internal/state` is not tracked in git (defect) | `.gitignore` excludes `state/` everywhere, so the `internal/state` package was never committed and a fresh clone cannot compile. |
| [0007](0007-configuration-shape.md) | Configuration shape | `config.yaml` at version `"4"` with a migration chain; defaults, the example file, and the compose template disagree on artifact paths. |
| [0008](0008-http-surface-and-auth.md) | HTTP surface and authentication | The main API on `:2704` (including build trigger) is unauthenticated; only the maintenance port `:2705` checks an ops credential via HRPAuth. |

## Maintenance notes

- Records are written against `main` at commit `989a9d7` (2026-11-04).
- Do not edit these entries to make the service look better than it is; record
  reality and link the evidence.
