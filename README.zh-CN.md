<div align="center">
  <h1>Armorial WebUI SDK Handler</h1>
  <p><strong>汇集每个 Armorial 服务，构建同一个 WebUI。</strong></p>
  <p><a href="./README.md">English</a></p>
</div>

Armorial WebUI SDK Handler 将 Armorial 托管的 SDK 包构建为供现有 HRPAuth-Web 仓库使用的单一 WebUI。

它整合各服务页面，在包指纹变化时重新构建，并完整重新注入整套 SDK。

## 为什么选择 Armorial WebUI SDK Handler

- **聚合生态页面。** 收集由 Armorial 托管的 SDK 源码包。
- **变化即重建。** 检测包指纹变化，生成全新的前端产物。
- **完整重新注入。** 替换整套已注入 SDK，而非叠加局部更新。
- **默认拒绝。** 共享内部密钥为空时，拒绝所有内部 API 请求。
- **发布静态产物。** 将编译后的 WebUI 交付到固定产物目录。

## 进一步了解

- [Docker 镜像](https://hub.docker.com/r/limeight/ha-webui-sdkhandler) — 已发布的容器镜像
- [Armorial Wiki](https://wiki.mcnb.dev/) — 生态文档
- [Armorial API Contracts](https://github.com/CoreMatch/HA-Contract) — 生态 API 契约

## 发布限制

发布采用先删除再重命名的替换方式：过程并非原子操作，也不保留可回滚版本。

## 许可证与来源

[AGPL-3.0](LICENSE)。来源：[CoreMatch/HA-WebUI-SDKHandler](https://github.com/CoreMatch/HA-WebUI-SDKHandler)。
