# HA-WebUI-SDKHandler

Build service that polls HRPAuth for SDK source packages, injects them into the
HRPAuth-WebUI frontend template, compiles the aggregated frontend, and publishes
the resulting static assets to a fixed artifacts directory (remove-then-rename
replacement; see `knowledge/0004` for the known serving gap).

## Build & Run

Requires Go 1.25+.

```bash
go build -o ./build/sdkhandler
./build/sdkhandler
```

The image is published on Docker Hub: [`limeight/ha-webui-sdkhandler`](https://hub.docker.com/r/limeight/ha-webui-sdkhandler)
(`linux/amd64` + `linux/arm64` multi-arch; tag `7195a69` alongside `latest`, effective
reference pinned by the manifest list digest in `deploy-manifest.yaml`).
The runtime image needs `node`, `npm`, and `git` because frontend builds happen at
runtime; these are included in the provided image. The multi-stage `Dockerfile` is kept
for local builds (`docker build -t ha-webui-sdkhandler .`).

The service listens on `:2704` (API, incl. `/health`, `/api/build/status`,
`/api/build/trigger`, `/api/revision`) and `:2705` (maintenance operations).

## Dependencies & Configuration

- Upstream dependency: an HRPAuth core instance (the only SDK package source, via
  `GET /services/sdk-packages`).
- Frontend template: a local HRPAuth-WebUI checkout, or the Git URL configured for
  automatic cloning.

Copy `config.example.yaml` to `config.yaml` and edit it; the file is read from the
working directory. Required values: `hrpauth.base_url`, `hrpauth.client_id`,
`hrpauth.client_secret` (OAuth2 service credentials for HRPAuth), and
`hrpauth.internal_key` (shared `X-Internal-Key`; while it is empty, all `/api/*`
endpoints reject requests with 401).

## Deployment facts

Authoritative deployment facts live in [`deploy-manifest.yaml`](deploy-manifest.yaml)
at the repo root (consumed by HA-Services-Deployer's orchestrator;
`manager: aapanel` means the process is panel-managed and the orchestrator
treats it as read-only):

- Listens on `:2704` (API) and `:2705` (maintenance), loopback-only by
  deployment default; health probe `GET /health` on `:2704`.
- Depends on HRPAuth (SDK package polling); no database, no Redis.
- Config file `config.yaml` (template version 4; `SDKHANDLER_CONFIG`
  overrides the path).
- Data dirs: `./state`, `./artifacts`, `./frontend-build`.

## Documentation

- SDK package format and aggregation pipeline: [HA-Contract — sdk-package.md](https://github.com/CoreMatch/HA-Contract/blob/main/docs/dev/HRPAuth/sdk-package.md)
- Microservice extension layer (presence / sdk-packages endpoints): [HA-Contract — microservices.md](https://github.com/CoreMatch/HA-Contract/blob/main/docs/dev/HRPAuth/microservices.md)
