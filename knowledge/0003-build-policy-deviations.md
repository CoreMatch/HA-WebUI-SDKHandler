# 0003 — Build Policy Deviations: No `npm install`, `run_scripts` Inert

## Status

Accepted (records broken promises as-is; deviations are still unfixed at HEAD)

## Context

The original build policy promised a pipeline of `npm install → tsc -b →
vite build`, with `npm install` defaulting to `--ignore-scripts` and
per-package `run_scripts: true` declarations lifting that restriction for
individual SDKs. The package doc of the build module still describes this
policy (`internal/build/build.go:L1-L8 fp=24eeb2b5 ~ Package build 编排 npm install → tsc -b → vite bui`). HA-Contract describes the actual
behavior (`docs/dev/HRPAuth/sdk-package.md` "Build Policy" and the
`run_scripts` row of the Field Reference).

## Decision

Record the two deviations honestly:

### 1. Builds do not install dependencies

Builds deliberately have no `npm install` step; dependency installation
belongs to template setup (commit `dfae95b`). Today:

- `npm install` runs only in the **template directory**, once after the
  initial clone and once after each `git pull` that changed the template
  (`main.go::ensureTemplateRepo`, `main.go::pullLatestTemplate`, `main.go::npmInstallTemplate`).
- Every build copies the template **including `node_modules`** into the work
  directory (`internal/inject/copy.go::copyExcluded`).
- `build.Run` contains no install step: blacklist precheck, then either
  `npm run build` or `tsc -b` + `vite build`
  (`internal/build/build.go::Run`).

Consequence: SDK `manifest.dependencies` are merged into `package.json`
(0002), but they are only resolvable if the template's `node_modules` already
contains them. A package that needs a dependency the template does not have
fails the build; the previous published output is kept (this failure mode is
also the contract's conflict policy, sdk-package.md "Conflict Policy").

### 2. `run_scripts` declarations have no effect

The value travels through the whole pipeline — list metadata
(`internal/broker/broker.go::RunScripts`), orchestrator (`main.go::runOnce`), injection
result (`internal/inject/inject.go::Inject`) — and is passed
to the build as `Options.RunScripts` (`internal/build/build.go::RunScripts`). But
`build.Run` never reads `RunScripts` or `IgnoreScriptsDefault`; the fields
exist only in the struct definition. There is no per-package script control
because there is no `npm install` at build time at all.

The generated `sdk-manifest.ts` still reports `runScripts` per package
(`internal/inject/generate.go::sdkManifestRunScripts`), which is informational only.

## Consequences (known defects)

- The promise "each build installs dependencies with `--ignore-scripts`,
  honoring `run_scripts: true`" does **not** hold; dependencies instead come
  preinstalled from the template.
- SDK authors cannot add new npm dependencies through the manifest alone;
  someone must first add and install them in the HRPAuth-WebUI template.
  There is no in-repo mechanism or documented runbook for that. Gap recorded
  as-is.
- `internal/build/build.go:L1-L8 fp=24eeb2b5 ~ Package build 编排 npm install → tsc -b → vite bui` is a stale doc comment describing a pipeline
  that no longer exists.
- `ignore_scripts_default` config still exists and is validated, but only
  affects the template-setup `npm install` (`main.go::npmInstallTemplate`), not builds.

## Evidence

- Commit `dfae95b8c7b5b2cc6144e7a1fbc3dc4365673517` — message: "remove the outdated build-time npm install step"; touches `internal/build/build.go` (-29 lines), `internal/inject/copy.go`, `main.go` (+31).
- `internal/build/build.go::Run` — `Run` has no install step; `RunScripts`/`IgnoreScriptsDefault` appear only as `Options` struct fields.
- `main.go::npmInstallTemplate` — called from `main.go::ensureTemplateRepo` and `main.go::pullLatestTemplate`.
- `internal/inject/copy.go::copyExcluded` — comment: node_modules is copied with the template.
- HA-Contract `docs/dev/HRPAuth/sdk-package.md` — Build Policy ("there is no per-build `npm install`") and Field Reference `run_scripts` row ("the current build pipeline does not read it").
