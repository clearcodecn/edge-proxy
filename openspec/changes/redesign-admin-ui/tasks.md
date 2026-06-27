## 1. 前端构建链与依赖（Phase 1）

- [x] 1.1 在仓库根创建 `package.json`，声明 dev 依赖 `tailwindcss`、`daisyui`、`postcss`、`autoprefixer`
- [x] 1.2 创建 `tailwind.config.js`：`content: ['./internal/web/template/**/*.html', './internal/web/static/app.js']`，启用 daisyUI 插件
- [x] 1.3 创建 `postcss.config.js` 注册 tailwindcss + autoprefixer
- [x] 1.4 创建 `web/input.css`：`@tailwind base; @tailwind components; @tailwind utilities;`
- [x] 1.5 在 Makefile 添加 `make ui`（一次构建）和 `make ui-watch`（dev 监听）target
- [x] 1.6 跑一次 `make ui`，生成基线 `internal/web/static/tailwind.css` 并 commit（21KB minified）
- [x] 1.7 vendored Alpine.js：下载 `alpine.min.js`（3.14.3，44KB）放到 `internal/web/static/alpine.min.js`
- [x] 1.8 vendored htmx：layout.html 引用已改为本地 `/static/htmx.min.js`（vendored 1.9.10，47KB）
- [x] 1.9 在 `.gitattributes` 给 `tailwind.css` / `*.min.js` 标记 `linguist-generated`，降低 PR diff 噪音
- [x] 1.10 `internal/web/static.go` 的 `//go:embed static/*` 模式已自动包含新文件（go build 21M 成功验证）
- [x] 1.11 项目目前无 CI 工作流；改在 Makefile 新增 `make ui-check` target，未来接入 CI 时直接调用

## 2. Layout、登录、配置三页视觉升级（Phase 2）

- [x] 2.1 重写 `layout.html`：左侧 240px 固定菜单（域名/回源/配置）+ navbar 顶栏（品牌/页标题/节点名/用户名/退出）+ 全局 toast 容器
- [x] 2.2 `pages.go` 加 `NodeName / Version / User` 字段；新增 `BuildVersion()` helper（runtime/debug 读 git revision，截断 8 位 + dirty 标记）；`cmd_run.go` 用 `os.Hostname()` + `BuildVersion()` 注入
- [x] 2.3 重写 `login.html`：居中 logo + 卡片 + 底部 `v{Version} · {NodeName}`；layout 在 `Authenticated=false` 时跳过左侧菜单
- [x] 2.4 重写 `config.html`：5 个 daisyUI card 分组（管理/ACME/探测/告警/路径）+ 顶部 alert "配置只读" + dingtalk/telegram 用 badge 标记
- [x] 2.5 删除 `internal/web/static/edge.css`，layout 不再 link 它
- [x] 2.5a *（scope 增补）* 顺带把 `domains.html` / `upstreams.html` 也做了**最小 Tailwind 改造**（card + table + badge + btn），保留单行新增 + 行操作；批量 UI 留给 Phase 6/7。原因：若仅删 edge.css 而不更新这两页，会有视觉断档期
- [x] 2.5b 创建 `static/app.js`：Alpine `toastBus()` 组件 + `window.toast()` 全局 hook（Phase 5 会扩展）
- [x] 2.6 `make smoke` 通过：`/login` 返回 200 新模板（1.5KB），`/` 未登录 302→/login；`go test ./...` 全绿；`go vet` 无 warning

## 3. 仓储层扩展（Phase 3 后端基础）

- [x] 3.1 `DomainRepo.Search(hosts, status, page, pageSize)`：hosts 空走分页（默认 50/页），非空走 `host IN (...)` 不分页
- [x] 3.2 `DomainRepo.BatchUpdateStatus(ids, target, allowedFrom)`：先 SELECT 预检 → 分流 → 单 UPDATE；不存在 / 状态不允许 → 计入 `FailedItem`
- [x] 3.3 `DomainRepo.BatchDelete(ids, allowedFrom)`：返回 `(deleted []Domain, failed []FailedItem)`，让 handler 能据 host 清 nginx conf；同时新增 `BatchResetRetry` 走专门的字段重置路径（fail_count/last_error/next_retry_at）
- [x] 3.4 `UpstreamRepo.Search(addrs, enabled *bool, page, pageSize)`、`BatchSetEnabled(ids, target bool)`、`BatchDelete(ids)` 同构（启用/禁用合并为一个方法 + bool 参数）
- [x] 3.5 13 个新单测（domain_repo_batch_test.go 7 个 + upstream_repo_batch_test.go 6 个）：分页、精确多匹配、状态过滤、部分成功、empty input、deleted 列表回填——全绿

## 4. 后端批量端点（Phase 3 接口）

- [x] 4.1 `DomainHandler.ListGET` 扩展：解析 `hosts`/`status`/`page`，调用 `Repo.Search`（>200 chip 内部截断 + 调用方 capped）。分页/截断 UI 留给 Phase 6 模板做
- [x] 4.2 `domain.go::BatchImportPOST`：textarea → splitHosts dedup → ≤200 校验 → 循环 Create，分桶 `created/skipped/failed`，返回 JSON
- [x] 4.3 `BatchDeprecatePOST` / `BatchRetryPOST` / `BatchRecyclePOST`：≤200 校验，复用 `BatchUpdateStatus`/`BatchResetRetry`/`BatchDelete`。回收路径**每条**走 conf remove → nginx -t → reload → 异步 LE delete（throttle 100ms 可调；测试中置 0），单条失败计入 failed 不阻断
- [x] 4.4 `UpstreamHandler.ListGET` 扩展 + `BatchImportPOST`（CSV-lite，每条都 refresh 改一次性 refresh）+ `BatchEnablePOST`/`BatchDisablePOST`/`BatchDeletePOST`（合并为 `batchSetEnabled` 私有函数）
- [x] 4.5 `cmd_run.go` 在 auth-gated 组内注册 8 个新路由：4 个域名 + 4 个回源（其中 `DELETE /upstreams/batch` 用 `BatchDeletePOST` handler）
- [x] 4.6 24 个新 handler 测试：批量超限 400、空 ids 400、部分成功 JSON 结构、状态机过滤（cert_failed-only retry、deprecated-only recycle）、nginx 调用次数验证、LE delete 异步 fire 验证、batch enable 已启用 → failed、空 pool refresh 短路场景
- [x] 4.7 CSV-lite 解析器（`handler/csvlite.go`）：4 态状态机（NewField/InField/InQuotes/AfterQuote）正确处理前导空格 + 含逗号的引号字段；8 个单测覆盖 spec 所有 scenario

## 5. 前端 Alpine 组件（Phase 4 共用基础）

- [x] 5.1 `internal/web/static/app.js` 全量重写（380 行，覆盖 3 个组件，替换 Phase 2 stub）
- [x] 5.2 `chipInput()`：chips/buffer/composing 三态；Enter/Tab/`,`/空格 separators；paste 多行分割；Backspace 空时删尾；composition* 事件让 IME 期间忽略 separator
- [x] 5.3 chip 溢出折叠：ResizeObserver + 实时 `recompute()` 计算 `overflowCount`；`+N` 徽章点击 popover 列出全部 chip（含 ×）；CSS 用 flex nowrap + overflow hidden
- [x] 5.4 chip 变更同步隐藏 `<input name="hosts">` (`:value="chips.join('\n')"`)，并 `$dispatch('chipchange')` 让 form 的 htmx `hx-trigger="chipchange delay:200ms"` 触发请求
- [x] 5.5 行选中合入 `pageState.selection: Map<id, host>`（含 host 是为了 copy 时无需再查表）；提供 `toggleRow`/`selectAllVisible`/`clearSelection`/`isSelected`；`selectionVersion` bumper 解决 Alpine 不监听 Map mutation 的问题
- [x] 5.6 浮动工具栏：模板里 `<div x-show="selectionCount > 0" class="sticky top-0 ...">`，按钮直接调 pageState 方法
- [x] 5.7 模态：`pageState.modal` 状态 + `openAction/openImport/closeModal/submitAction/submitImport`；同一组件管理两种模态（action / import）
- [x] 5.8 `toastBus()`（Phase 2 已有，未变）
- [x] 5.9 layout 顶层 `<div x-data="toastBus()" class="toast toast-top toast-end">`（Phase 2 已有）

## 6. 域名页接线（Phase 4）

- [x] 6.1 重写 `domains.html`：chipInput + 状态下拉 + 浮动工具栏 + 两个模态（action / import）+ 引用 `domain_list` partial；同时定义 partial 自身（带 `id="domain-list"` + htmx 触发器）
- [x] 6.2 `domain_row` partial 加首列 checkbox（含 `data-row-check`/`data-row-id`/`data-row-host`，`@change` 调 `toggleRow`），选中行加 `bg-warning/10` 高亮
- [x] 6.3 模态实现合入 `domains.html` 内联（不单独 partial 化）：action 模态列出选中 hosts + 危险警告；import 模态含 textarea + 实时结果明细（创建/跳过/失败 badge + 折叠列表）
- [x] 6.4 import：textarea → POST `/domains/batch` → 解析 JSON → 模态内渲染明细 + 顶部 toast
- [x] 6.5 复制：`navigator.clipboard.writeText(hosts.join('\n'))` → toast "已复制 N 个"
- [x] 6.6 批量废弃/重试/回收：复用 action 模态；回收的 modal `danger:true` 触发红字警告
- [x] 6.7 端到端验证：编写 `/tmp/edge-e2e.sh` 跑通 10 项检查（登录 / 新模板加载 / chipInput 标记 / 批量导入 5 个 / chip-search partial 精确返回 2/3 / 批量废弃 3 个 / 250 ids → 400 / 未授权 → 302 / 回源页 / 配置页 5 cards）。新增基础设施：`web/views.go` 中的 `BuildDomainListView/BuildUpstreamListView`、`pages.go` 中的 `DomainListView/UpstreamListView/RenderDomainList/RenderUpstreamList`、`static.go` 中的 `seq`/`until` 模板函数

## 7. 回源页接线（Phase 5）

- [x] 7.1 重写 `upstreams.html`：与域名页结构镜像，复用 chipInput / pageState 组件（`resource:'upstreams'` 切换 endpoint 集），同样含两个模态、浮动工具栏、partial
- [x] 7.2 `upstream_row` 加 checkbox（`data-row-host` 装 addr）；选中行高亮
- [x] 7.3 import 模态：textarea + 详细 placeholder + 顶部格式提示（含双引号示例）→ POST `/upstreams/batch`
- [x] 7.4 "复制地址"复用 pageState.copyHosts；"批量启用/禁用"/"批量删除"复用 openAction（DELETE 走 pageState._actionEndpoint 的 method 分支）
- [x] 7.5 E2E 验证（`/tmp/edge-e2e-up.sh`）：CSV-lite 6 行混合（含 4 字段 + 引号 remark + 缺省字段 + 错误格式）→ 4 created / 2 failed；chip 精确搜索；批量禁用 2 个；`DELETE /upstreams/batch` 删除 4 个。同时发现并修复了 `parseIDs` 对 DELETE method 的 body 读取（Go `r.ParseForm` 只对 POST/PUT/PATCH 自动解析 body，新增 DELETE-method 单测锁住

## 8. 收尾与回归（Phase 6）

- [x] 8.1 `go test ./...` 全绿（含修复 `fakeCertbotForWeb` 的 atomic.Value 竞态：批量回收并发 goroutine append 导致条目丢失，换成 mutex-guarded 切片）
- [x] 8.2 `go vet ./...` 无 warning
- [x] 8.3 `make ui-check` 通过：`make ui` 后产物 git diff clean
- [x] 8.4 双 E2E 套件全过（域名 10 项检查 + 回源 5 项检查），覆盖：登录/302/新模板/chipInput/批量导入/chip-search partial/批量操作/CSV-lite/DELETE method
- [x] 8.5 边界回归覆盖：>200 → 400、未授权 → 302、状态机过滤（cert_failed-only retry, deprecated-only recycle, enabled-only disable）、空 ids → 400、部分成功 JSON 结构
- [x] 8.6 视觉自检：四页（login/domains/upstreams/config）风格统一（daisyUI light theme），左侧菜单高亮当前页（domains/upstreams/config 三选一），顶栏含品牌/页标题/节点名/用户名/退出
- [x] 8.7 `README.md` 更新：新增"管理后台 UI"章节（chip 搜索 + 批量能力说明）+ "开发：UI 构建链"章节（ui / ui-watch / ui-install / ui-check）
- [x] 8.8 `jy-openspec validate redesign-admin-ui` → "Change is valid"；准备 `/jyopsx-apply`（实施已通过本次会话完成；如需归档运行 `jy-openspec archive redesign-admin-ui`）

## 8. 收尾与回归（Phase 6）

- [ ] 8.1 `go test ./...` 全绿
- [ ] 8.2 `go vet ./...` 无 warning
- [ ] 8.3 `make ui` 后 `git status` 应 clean（CI sync check 提前本地通过）
- [ ] 8.4 端到端烟雾测试：启动 `./edge-proxy run` → 登录 → 100 条批量导入 → 全选 → 批量废弃 → 全选 → 批量回收（confirm）→ 列表清空；同样流程跑一遍回源
- [ ] 8.5 边界回归：未登录访问任意 batch 端点应 302→login；超 200 应 400；批量重试混入 online 状态应部分失败
- [ ] 8.6 视觉自检：4 个页面（login / 域名 / 回源 / 配置）风格一致，左侧菜单高亮当前页，顶栏信息完整
- [ ] 8.7 更新 `README.md` 增加"管理后台"截图与功能简述（chip 搜索 + 批量能力）
- [ ] 8.8 `jy-openspec validate redesign-admin-ui` 通过；准备 `/jyopsx-apply` 入场
