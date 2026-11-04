# 0001 — Scope and Positioning

## Status

Accepted (records current state)

## Date

2026-11-04

## Context

HA-WebUI-SDKHandler is a build service, first added in commit `1479a3d`
(2026-10-09). The HRPAuth platform moved from a runtime JS SDK model to a
compile-time aggregation model: satellite services ship SDK **source
packages**, and this service compiles them into one aggregated frontend. The
format and pipeline are contracted in HA-Contract
`docs/dev/HRPAuth/sdk-package.md`.

## Decision

This service is a **build-time SDK aggregator** with exactly one upstream and
one template:

- Pull SDK packages **only** from the HRPAuth main service
  (`GET /services/sdk-packages` + `.../download`); never contact satellite
  services directly (`internal/broker/broker.go:1-9`).
- Poll on an interval (`hrpauth.poll_interval_seconds`, default 60) and build
  when the package list fingerprint changes; manual builds via
  `POST /api/build/trigger` (`main.go:448-471`, `main.go:561-563`).
- Optionally poll and `git pull` the frontend template
  (`build.template_poll_interval_seconds`, default 0 = off)
  (`main.go:402-445`).
- Pipeline per build: copy template → inject SDKs → compile (`npm run build`
  or `tsc -b` + `vite build`) → publish (`main.go:139-234`).
- Build state is persisted in `state/builds.json` (`internal/state/state.go:1-6`).
- HTTP surface: `:2704` API, `:2705` maintenance operations
  (`config.example.yaml:10-12`).
- Runtime needs `node`, `npm`, and `git` in the image because compilation
  happens at runtime (`Dockerfile:12-18`, commit `d307473`).

## Consequences

- The service is stateless with respect to SDK content: every build starts
  from a fresh copy of the template (see 0002).
- It is tightly coupled to three external pieces: a reachable HRPAuth
  instance, a usable HRPAuth-WebUI template checkout (or its Git URL), and a
  Node.js toolchain at runtime.
- Known defects and broken promises are recorded separately in 0003, 0004,
  0006, and 0008; they are real limitations of the current implementation,
  not roadmap promises.

## Evidence

- `main.go:1-7` — package doc stating the responsibility.
- `main.go:139-234` — the copy → inject → compile → publish pipeline.
- `internal/broker/broker.go:1-9` — "only pull from HRPAuth" rule.
- `config.example.yaml:10-12`, `config.example.yaml:14-24` — ports and upstream config.
- `Dockerfile:12-18` — runtime image ships node/npm/git (commit `d307473`, 2026-11-03).
- Commits: `1479a3d` (initial service, 2026-10-09), `6e37c48` (maintenance service), `d307473` (Docker).
- Contract: HA-Contract `docs/dev/HRPAuth/sdk-package.md` "Design Positioning" and "Data Flow" sections.
