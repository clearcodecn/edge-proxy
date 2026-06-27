## Why

当前 edge-proxy 的内嵌 Web UI 只能"逐条"操作（新增 1 个域名、1 个回源），手写 200 行 CSS 视觉简陋，菜单挤在顶部黑条里。运维真实场景下一次往往要导入/废弃/重试**几十到上百个域名**——按现状每条点 3 次按钮、刷新等待，几乎不可用。

此次重构把界面升级为"管理后台"形态（左侧菜单 + Tailwind/daisyUI 组件），并把"批量"作为列表页的一等能力，让 100 条规模的操作从"一小时"压缩到"一分钟"。

## What Changes

- **Layout**：替换顶栏导航为**左侧 240px 固定菜单**（域名 / 回源 / 配置），顶栏只保留品牌、节点名、用户名、退出
- **登录页**：从极简卡片升级为带 logo + 节点版本号的居中卡片（风格 2）
- **域名列表**：
    - 引入 **chip 输入式精确搜索**（多行粘贴 → 自动转 chip → `host IN (...)` 精确匹配；可点 × 删除单条；溢出折叠为 `+N`）
    - 状态筛选下拉 + 分页（每页 50；搜索激活时禁用分页，结果 >200 截断 + 警告）
    - 行勾选 → **粘性浮动批量工具栏**：复制域名列表（剪贴板）/ 批量废弃 / 批量重试 / 批量回收（confirm 弹窗，无需输入）
    - 新增 **批量导入**入口：textarea 多行粘贴 → 验证报告（成功/跳过/失败明细）
- **回源列表**：与域名同构 UI；批量导入采用 **CSV-lite 行格式**（`addr[,weight][,backup][,remark]`）；批量操作为启用 / 禁用 / 删除
- **配置页**：保留只读语义不变，仅做视觉分组升级（5 个 card 按 admin/acme/probe/alert/paths 分块）
- **前端技术栈**：在 htmx 之上增加 **Tailwind CSS + daisyUI**（视觉）与 **Alpine.js**（chip 输入框、模态、toast、批量选中等纯前端状态）
- **构建链**：新增 `package.json`、`tailwind.config.js`、`make ui` target；产物 `internal/web/static/tailwind.css` 仍走 `//go:embed`，最终交付物依旧是**单 binary**
- **后端 API**：新增 9 个 batch/查询端点（详见 `specs/domain-upstream-batch-ops/spec.md`）；现有单条 API 保留不动

非目标（YAGNI 明确砍掉）：暗色主题、菜单折叠、面包屑、通知中心、跨页选中、CSV/JSON 多格式复制、配置页可写、nginx upstream 片段导出。

## Capabilities

### New Capabilities
- `admin-ui-redesign`：管理后台视觉与导航重构。覆盖左侧菜单 layout、登录页、配置页视觉、前端构建链（Tailwind/daisyUI/Alpine 三件套 embed 进 binary）。
- `domain-upstream-batch-ops`：域名与回源的批量能力。覆盖 chip 精确搜索、批量导入、行勾选 + 浮动工具栏、部分成功语义的批量端点。

### Modified Capabilities
- *（无）* 当前 `openspec/specs/` 为空，本次为首批 capability 落地。

## Impact

- **代码**：
    - `internal/web/template/*.html`：全部模板按新 layout 重写
    - `internal/web/static/`：新增 Tailwind 构建产物 `tailwind.css`、删除手写 `edge.css`
    - `internal/web/static.go`：embed 列表更新；考虑加入 `app.js`（Alpine 组件）
    - `internal/web/handler/domain.go`、`handler/upstream.go`：新增批量端点 + 查询参数解析
    - `internal/store/domain_repo.go`、`store/upstream_repo.go`：新增按 host/addr `IN (...)` 查询、按 ids 批量更新方法
    - `cmd/edge-proxy/cmd_run.go`：注册新路由
- **新增构建步骤**：仓库根新增 `package.json` / `tailwind.config.js` / `postcss.config.js` / `web/input.css`；Makefile 新增 `ui` target
- **运维**：产物 `tailwind.css` 提交进仓库（避免要求构建机上有 Node.js），开发机改 UI 时手动 `make ui` 重生成；CI 校验产物与源同步
- **依赖**：dev 依赖新增 `tailwindcss`、`daisyui`、`postcss`、`autoprefixer`；运行时无新 Go 依赖；浏览器侧新增 `alpine.js`（embed，约 15KB gz）
- **风险**：批量回收涉及 N 次 nginx reload + LE 证书删除，需节流；chip 输入框需兼容中文输入法的 composition 事件；详见 `design.md`
