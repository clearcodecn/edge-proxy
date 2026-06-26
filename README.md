# edge-proxy

自建反向代理边缘节点：Go 单 binary + 嵌入式 Web UI，负责 443 SSL 终结、自动申请 / 续签 Let's Encrypt 证书、反代到内部 upstream 池。

定位为通用反代工具，独立于上游业务系统（如 `im-api`）部署，每台节点完全自治、互不感知。

## 部署

详见 [`docs/deploy.md`](docs/deploy.md)。

## 设计文档

设计与规约来源于 OpenSpec 变更 `add-edge-proxy-mvp`（在上游 `im-api` 仓库内归档）。
