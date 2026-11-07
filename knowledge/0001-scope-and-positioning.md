# 0001 — Scope and Positioning

## Status

Accepted (records current state)

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
  services directly (`internal/broker/broker.go:L1-L9 ~ Package broker 实现从 HRPAuth 主服务拉取 SDK 包的客户端。`).
- Poll on an interval (`hrpauth.poll_interval_seconds`, default 60) and build
  when the package list fingerprint changes; manual builds via
  `POST /api/build/trigger` (`main.go::pollLoop`, `main.go::handleBuildTrigger`).
- Optionally poll and `git pull` the frontend template
  (`build.template_poll_interval_seconds`, default 0 = off)
  (`main.go::templatePollLoop`).
- Pipeline per build: copy template → inject SDKs → compile (`npm run build`
  or `tsc -b` + `vite build`) → publish (`main.go::execute`).
- Build state is persisted in `state/builds.json` (`internal/state/state.go:L1-L6 ~ Package state 负责构建历史状态的持久化与查询。`).
- HTTP surface: `:2704` API, `:2705` maintenance operations
  (`config.example.yaml (field: listen_addr)`, `config.example.yaml (field: maintenance_addr)`).
- Runtime needs `node`, `npm`, and `git` in the image because compilation
  happens at runtime (`Dockerfile:L12-L18`, commit `d307473`).

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

- `main.go:L1-L7 ~ HA-WebUI-SDKHandler 是前端编译期 SDK 聚合器。` — package doc stating the responsibility.
- `main.go::execute` — the copy → inject → compile → publish pipeline.
- `internal/broker/broker.go:L1-L9 ~ Package broker 实现从 HRPAuth 主服务拉取 SDK 包的客户端。` — "only pull from HRPAuth" rule.
- `config.example.yaml (field: listen_addr)`, `config.example.yaml (field: base_url)` — ports and upstream config.
- `Dockerfile:L12-L18` — runtime image ships node/npm/git (commit `d307473`, 2026-11-03).
- Commits: `1479a3d` (initial service, 2026-10-09), `6e37c48` (maintenance service), `d307473` (Docker).
- Contract: HA-Contract `docs/dev/HRPAuth/sdk-package.md` "Design Positioning" and "Data Flow" sections.
