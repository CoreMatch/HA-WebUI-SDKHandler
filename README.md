<div align="center">
  <h1>Armorial WebUI SDK Handler</h1>
  <p><strong>One WebUI, assembled from every Armorial service.</strong></p>
  <p><a href="./README.zh-CN.md">简体中文</a></p>
</div>

Armorial WebUI SDK Handler turns Armorial-hosted SDK packages into a single WebUI build for the existing HRPAuth-Web repository.

It combines service pages, rebuilds when package fingerprints change, and reinjects the complete SDK set.

## Why Armorial WebUI SDK Handler

- **Aggregate ecosystem pages.** Collect SDK source packages hosted by Armorial.
- **Rebuild on change.** Detect package fingerprint changes and produce a fresh frontend.
- **Reinject completely.** Replace the injected SDK set rather than layer partial updates.
- **Fail closed.** Reject internal API requests whenever the shared internal key is empty.
- **Publish static output.** Deliver the compiled WebUI to a fixed artifacts directory.

## Learn more

- [Docker image](https://hub.docker.com/r/limeight/ha-webui-sdkhandler) — published container image
- [Armorial Wiki](https://wiki.mcnb.dev/) — ecosystem documentation
- [Armorial API Contracts](https://github.com/CoreMatch/HA-Contract) — ecosystem API contracts

## Publication limit

Publication uses remove-then-rename replacement: it is not atomic and keeps no retained rollback.

## License & provenance

[AGPL-3.0](LICENSE). Source: [CoreMatch/HA-WebUI-SDKHandler](https://github.com/CoreMatch/HA-WebUI-SDKHandler).
