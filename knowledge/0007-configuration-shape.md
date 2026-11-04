# 0007 — Configuration Shape

## Status

Accepted (records current state)

## Date

2026-11-04

## Context

The service reads a YAML config file. Over several revisions the config
gained versioning and migrations, and three sources of "default values"
diverged (code defaults, the example file, and the platform compose
template).

## Decision

Record the verified shape:

- Path: `./config.yaml`, overridable via `SDKHANDLER_CONFIG`
  (`main.go:492-496`).
- Current config version: `"4"` (`internal/config/config.go:16`).
- Missing file: `Load` writes a default file via `CreateDefault` and then
  **exits with an error asking the operator to edit and restart**
  (`internal/config/config.go:62-71`). This differs from the sibling services
  (HA-Services-Deployer / HA-Services-MaintenanceView), which continue
  running with defaults.
- Migration chain (`internal/config/migrations.go:18-76`):
  - `"" → 1`: baseline stamp.
  - `1 → 2`: remove `hrpauth.ops_token`, add empty `client_id`/`client_secret`
    (matches commit `af4155b`).
  - `2 → 3`: add `build.template_git_url`.
  - `3 → 4`: add `build.template_poll_interval_seconds` (commit `39e4e70`).
  - A backup `config.yaml.bak.<version>` is written before each step
    (`internal/config/config.go:160-176`, `migrations.go:92-95`).
- Required values: `hrpauth.client_id`, `hrpauth.client_secret`,
  `build.template_dir` (`internal/config/config.go:217-230`).
- Relative paths are resolved against the config file's directory, not the
  process cwd (`internal/config/config.go:237-248`).
- Defaults in code (`CreateDefault`, `internal/config/config.go:112-141`):
  listen `:2704`, maintenance `:2705`, poll 60s, template
  `./HRPAuth-WebUI` (git URL `https://github.com/CoreMatch/HRPAuth-WebUI.git`),
  work `./frontend-build`, **artifacts `./dist`, current `./dist`**, state
  `./state`, `serve_artifacts: false`.

## Consequences (including recorded drift)

- **Example file vs code defaults drift.** `config.example.yaml` ships
  `artifacts_dir: ./artifacts`, `current_link: ./current`
  (`config.example.yaml:32-33`), while `CreateDefault` uses
  `./dist`/`./dist`. (The example's missing `version` field was fixed on
  2026-11-05: it now ships `version: "4"`, so copying it no longer triggers
  the migration chain.)
- **Compose template drift.** The platform compose template
  (HA-Services-Deployer `deploy/config/sdkhandler.yaml.tmpl`) also uses
  `./artifacts`/`./current` (its missing `version` field was likewise fixed
  to `"4"` on 2026-11-05). The Dockerfile pre-creates
  `/app/artifacts` and `/app/current` (`Dockerfile:17`), consistent with the
  compose template but not with the code defaults.
- `current_link` is vestigial for publishing (see 0004) but still part of the
  config surface.
- `build_timeout_seconds` exists in code (`internal/config/config.go:58`,
  default 20 minutes via `applyDefaults:209-211`) but is absent from both the
  example file and the compose template.

## Evidence

- `internal/config/config.go:16` (version), `:62-71` (create-and-exit), `:112-141` (CreateDefault), `:178-215` (applyDefaults), `:217-230` (required fields), `:237-248` (path resolution).
- `internal/config/migrations.go:18-76` — migration chain.
- `config.example.yaml:7-45` — example values (`version: "4"` at `:8`).
- HA-Services-Deployer repo `deploy/config/sdkhandler.yaml.tmpl` — compose values.
- Commits: `ef9550f` (config migration and defaults), `39e4e70` (version bump to 4), `af4155b` (OAuth2, migration 1→2), `6fa0acf` (template polling).
