# 0002 — Injection Overwrites `src/generated/` (Five Mount Points)

## Status

Accepted (records current state)

## Date

2026-11-04

## Context

The handler's core job is to turn N SDK source archives plus the read-only
frontend template into one compilable frontend. The contract defines the
generated modules and their host consumers (HA-Contract
`docs/dev/HRPAuth/sdk-package.md`, "Generated Frontend Contract").

## Decision

Every build performs a **full, non-incremental injection**:

1. The template directory is copied in full into a fresh work directory
   (`internal/inject/copy.go::CopyTemplate`). `node_modules` is deliberately copied
   too (`internal/inject/copy.go::copyExcluded` — see 0003).
2. Each SDK archive is extracted to
   `work_dir/src/generated/services/<name>/` after removing any previous
   directory for that name (`internal/inject/inject.go::Inject`), i.e. all SDK
   packages are re-injected on every build.
3. Archive extraction rejects path traversal, absolute paths, symlinks, and
   hardlinks (`internal/inject/extract.go::sanitizeArchivePath`,
   `internal/inject/extract.go::extractTarGz`, `internal/inject/extract.go::extractZip`).
4. Host-relative imports inside SDK sources are rewritten so files outside
   the package that live in the host `src/` tree resolve
   (`internal/inject/rewrite.go::RewriteHostImports`, commit `21c6891`).
5. Each package's `manifest.json` is validated against schema version 1
   (`internal/inject/manifest.go::ValidateManifest`).
6. All `manifest.dependencies` are merged into the work directory's
   `package.json`; template dependencies fill the rest
   (`internal/inject/inject.go::writePackageJSON`).
7. Six glue files are (re)generated under `src/generated/`
   (`internal/inject/generate.go::writeSDKRoutes` and five sibling `writeSDK*` writers):

| Generated file | Host mount point (consumer) |
|---|---|
| `sdk-routes.tsx` | `App.tsx` route tree |
| `sdk-navbar.tsx` | `Navbar.tsx` |
| `sdk-dashboard.tsx` | `Dashboard.tsx` |
| `sdk-critical-operations.ts` | `AccountSecurity.tsx` |
| `sdk-profile-cards.tsx` | `Profile.tsx` |
| `sdk-manifest.ts` | diagnostics/metadata (not a mount point) |

The five mount points are the five host components above; `sdk-manifest.ts`
is metadata only. This matches the contract table (HA-Contract
`docs/dev/HRPAuth/sdk-package.md`, "Generated Frontend Contract"). Type
contracts come from `src/types/generated-sdk.ts` in the template repo
(CoreMatch/HRPAuth-WebUI; verified present in the local checkout named
`HRPAuth-Web`).

## Consequences

- Hand-editing anything under `src/generated/` in the template is pointless:
  the files are overwritten on every build.
- An empty SDK list still produces a build: all six files are generated with
  empty arrays (commit `7461931`).
- Mount-point additions beyond the five above require changes in both the
  handler (new writer) and the template (new consumer), plus a contract
  update. Profile cards (`dc0926e`) and critical operations (`958961c`) were
  added this way.
- The handler's local manifest validation is slightly looser than the
  contract's upload-time rule: it accepts a package when at least one of
  `routes`/`profile_cards` is non-empty
  (`internal/inject/manifest.go::ValidateManifest`), while the contract says HRPAuth
  rejects uploads with empty `routes` regardless of `profile_cards`
  (sdk-package.md, Field Reference for `routes`). The handler never sees such
  packages if HRPAuth enforces its rule; the difference is recorded as-is.

## Evidence

- `internal/inject/inject.go::Inject` — full injection flow.
- `internal/inject/generate.go::writeSDKRoutes` (+5 sibling `writeSDK*` writers) — the six writers and file names.
- `internal/inject/copy.go::copyExcluded` — copy exclusion set (no `node_modules`).
- `internal/inject/extract.go::sanitizeArchivePath` — archive path sanitization.
- `internal/inject/rewrite.go::RewriteHostImports` — host import rewriting.
- Commits: `21c6891` (host import rewriting), `dc0926e` (profile cards), `958961c` (critical operations), `7461931` (empty list handling), `917950e` (flexible menu/dashboard entries).
- Contract: HA-Contract `docs/dev/HRPAuth/sdk-package.md`, Generated Frontend Contract table.
