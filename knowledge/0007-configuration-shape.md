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
  (`internal/config/config.go:61-67`). This differs from the sibling services
  (HA-Services-Deployer / HA-Services-MaintenanceView), which continue
  running with defaults.
- Migration chain (`internal/config/migrations.go:18-76`):
  - `"" → 1`: baseline stamp.
  - `1 → 2`: remove `hrpauth.ops_token`, add empty `client_id`/`client_secret`
    (matches commit `af4155b`).
  - `2 → 3`: add `build.template_git_url`.
  - `3 → 4`: add `build.template_poll_interval_seconds` (commit `39e4e70`).
  - A backup `config.yaml.bak.<version>` is written before each step
    (`internal/config/config.go:156-172`, `migrations.go:92-95`).
- Required values: `hrpauth.client_id`, `hrpauth.client_secret`,
  `build.template_dir` (`internal/config/config.go:213-226`).
- Relative paths are resolved against the config file's directory, not the
  process cwd (`internal/config/config.go:233-244`).
- Defaults in code (`CreateDefault`, `internal/config/config.go:109-137`):
  listen `:2704`, maintenance `:2705`, poll 60s, template
  `./HRPAuth-WebUI` (git URL `https://github.com/CoreMatch/HRPAuth-WebUI.git`),
  work `./frontend-build`, **artifacts `./dist`, current `./dist`**, state
  `./state`, `serve_artifacts: false`.

## Consequences (including recorded drift)

- **Example file vs code defaults drift.** `config.example.yaml` ships
  `artifacts_dir: ./artifacts`, `current_link: ./current` and **no `version`
  field** (`config.example.yaml:18-33`), while `CreateDefault` uses
  `./dist`/`./dist`. Copying the example triggers the full migration chain
  `"" → 4` on first start (with backups), because the version is missing.
- **Compose template drift.** The platform compose template
  (HA-Services-Deployer `deploy/config/sdkhandler.yaml.tmpl`) also uses
  `./artifacts`/`./current` and has no `version` field, so containerized
  deployments migrate on first start too. The Dockerfile pre-creates
  `/app/artifacts` and `/app/current` (`Dockerfile:17`), consistent with the
  compose template but not with the code defaults.
- `current_link` is vestigial for publishing (see 0004) but still part of the
  config surface.
- `build_timeout_seconds` exists in code (`internal/config/config.go:54`,
  default 20 minutes via `applyDefaults:205-207`) but is absent from both the
  example file and the compose template.

## Evidence

- `internal/config/config.go:16` (version), `:61-67` (create-and-exit), `:109-137` (CreateDefault), `:174-211` (applyDefaults), `:213-226` (required fields), `:233-244` (path resolution).
- `internal/config/migrations.go:18-76` — migration chain.
- `config.example.yaml:18-33` — example values without `version`.
- HA-Services-Deployer repo `deploy/config/sdkhandler.yaml.tmpl` — compose values.
- Commits: `ef9550f` (config migration and defaults), `39e4e70` (version bump to 4), `af4155b` (OAuth2, migration 1→2), `6fa0acf` (template polling).
