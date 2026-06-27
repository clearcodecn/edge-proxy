# edge-proxy

自建反向代理边缘节点：Go 单 binary + 嵌入式 Web UI，负责 443 SSL 终结、自动申请 / 续签 Let's Encrypt 证书、反代到内部 upstream 池。

定位为通用反代工具，独立于上游业务系统（如 `im-api`）部署，每台节点完全自治、互不感知。

## 管理后台 UI

内嵌的 admin UI（启动后访问 `https://<节点>:<admin.bind 端口>/`）是管理后台风格的单页面工具，左侧菜单 + 顶栏 + 卡片化内容，含：

- **域名**：chip 式精确搜索（多行粘贴 → 自动切分；× 单独删除；溢出 `+N` popover）+ 状态筛选 + 50/页分页（搜索激活时禁用）；
  批量导入（textarea，≤200）/ 批量复制域名到剪贴板 / 批量废弃 / 批量重试 / 批量回收（含 confirm 模态）。
- **回源**：与域名页同构；批量导入按 CSV-lite 行格式 `addr[,weight][,backup|main][,remark]`（含双引号 remark 支持）；
  批量启用 / 禁用 / 删除。
- **配置**：只读视图，5 个 daisyUI card 分组（管理 / ACME / 探测 / 告警 / 路径）。

技术栈：Go html/template + htmx（服务端片段渲染）+ Tailwind CSS + daisyUI（视觉系统）+ Alpine.js（chip 输入、模态、toast 等纯前端状态）。
全部前端资源 `//go:embed` 进 binary，部署仍是单文件。

## 开发：UI 构建链

修改 `internal/web/template/**/*.html` 或 Tailwind 配置后需要重新生成样式：

```
make ui-install   # 首次或 package.json 改动后执行 npm install
make ui           # 一次构建 → internal/web/static/tailwind.css
make ui-watch     # 开发期增量重建
make ui-check     # CI sync 检查：本地若忘了 commit 产物会失败
```

无 Node.js 的构建机直接 `make build`（或 `make release`）即可：`tailwind.css` 已 commit 进仓库，
`go build` 会通过 `//go:embed` 把它打进 binary。

## 部署

详见 [`docs/deploy.md`](docs/deploy.md)。

## 设计文档

- 原始 MVP 设计：OpenSpec 变更 `add-edge-proxy-mvp`（在上游 `im-api` 仓库内归档）。
- 管理后台 UI 重构（左侧菜单 + 批量能力）：`openspec/changes/redesign-admin-ui/`。
