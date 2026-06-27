## Context

edge-proxy 是部署在边缘节点的反向代理管理服务，定位 "单 binary、自治、零外部依赖"。当前 Web UI（commit `e694a56` 引入）由 Go html/template + htmx + 200 行手写 CSS 构成，足以演示功能，但运维侧反馈两个痛点：

1. **视觉粗糙**：单栏顶部导航 + 卡片堆叠，没有"管理后台"的信息密度；
2. **批量缺位**：导入/废弃/重试都是一次一条，单节点 100+ 域名几乎不可操作。

本次重构**只重做交互层**：后端模型、cron、ACME 流程、nginx render 等模块不动；只增量补充批量端点和查询参数。

**约束**：
- MUST 保持单 binary 交付（不能引入运行时 Node.js / 不能要求构建机有 Node.js）
- MUST 保持 htmx 主轴（不引入 SPA 框架）
- MUST 保持现有 chi 路由 + session 鉴权
- 操作员（受众）：DevOps / SRE，熟悉命令行，能接受"右上角 toast 报告部分成功"这种非 UX 决策

## Goals / Non-Goals

**Goals:**
- 视觉升级为"管理后台"形态（左侧菜单 + 卡片 + 表格 + 模态 + toast），统一 4 页风格
- 把"导入 / 选中 / 复制 / 废弃 / 重试 / 回收"等批量能力以**列表页能力**形式落地（不是独立菜单）
- 域名搜索改为 chip 式精确多匹配，让"粘贴一列异常域名 → 批量废弃"一气呵成
- 引入 Tailwind/daisyUI/Alpine 三件套，构建产物 embed，单 binary 不变

**Non-Goals:**
- 不做主题切换、暗色模式、菜单折叠、面包屑、通知中心
- 不做跨页选中保留（用搜索→全显示→全选覆盖此场景）
- 不让配置页可写（涉及热加载、字段校验、敏感字段，工程量过大且不在本次痛点）
- 不做 nginx upstream 片段导出、批量 CSV/JSON 多格式复制
- 不替换 htmx、不引入 SPA、不改后端数据模型

## Decisions

### D1. 前端栈：htmx + Tailwind + daisyUI + Alpine.js

- htmx 继续承担"服务端 fragment 渲染 + 局部更新"，与现有 handler 模式天然契合
- Tailwind + daisyUI 提供管理后台视觉系统：daisyUI 的 `drawer`、`menu`、`card`、`modal`、`badge`、`toast` 直接覆盖本次需要的 90% 组件
- Alpine.js 处理纯前端状态（chip 输入、勾选状态、浮动工具栏可见性、模态开关、toast 队列），与 htmx 同生态、官方互推

**备选方案**：
- (a) 纯手写 CSS + vanilla JS：chip 输入 + 中文输入法兼容 + popover 至少 200 行 JS，维护成本高
- (b) 改 Vue/React SPA + JSON API：破坏单 binary 定位、改造工作量数倍
- (c) Tagify（专用 chip 库）：30KB 但只解决一处问题

选 daisyUI + Alpine 的核心理由：**一处依赖、多处复用**（chip / modal / toast / 工具栏可见性 / 表单提交态都靠 Alpine）。

### D2. 构建链：commit 构建产物，避免 CI 依赖 Node.js

- 仓库根新增 `package.json`、`tailwind.config.js`、`postcss.config.js`、`web/input.css`
- Makefile 新增 `make ui` target（开发期手动跑，或加 `make ui-watch`）
- 产物 `internal/web/static/tailwind.css` **commit 进仓库**
- CI 加一道 check：`make ui && git diff --exit-code internal/web/static/tailwind.css`，防止源改了产物没跟着改

**备选**：
- (a) CI 装 Node.js 现场构建：增加 CI 复杂度 + 拉慢构建；与"单 binary、零依赖"精神冲突
- (b) 用 esbuild + tailwind-cli 的纯 Go 包装：方案不成熟，esbuild 有 Go 版但 tailwind 没有

### D3. chip 搜索：Alpine 组件 + `<textarea>`/`<input>` 同步隐藏字段

- Alpine 维护 `chips: []string` 状态数组
- 渲染时映射成 `<span class="badge">{{chip}} ×</span>`
- 实际提交字段通过隐藏 `<input name="hosts">` 同步，值为 `chips.join('\n')`
- htmx 监听该隐藏字段的 change 事件，触发 `hx-get="/domains" hx-vals="..."`

**中文输入法处理**：
- 监听 `compositionstart` / `compositionend`，composition 期间忽略 Enter/Space/逗号分隔符触发
- composition 结束后再判断当前 buffer 是否含分隔符

### D4. 搜索时禁用分页 vs 跨页选中保留

- 二选一，本设计采用"搜索时禁用分页 + 翻页清空选中"
- 理由：用户的真实工作流是 "粘贴 47 个域名 → 全选 → 批量操作"，分页和跨页选中都会破坏此流；保留分页会引出"已选 47 项（本页 30 项）"这种容易踩坑的语义
- 兜底：搜索匹配 > 200 时截断 + 警告，避免一次拉太多数据撑爆 DOM

### D5. 批量端点的部分成功语义

- 所有 `*/batch/*` 端点采用 `{succeeded, failed: [{id, reason}]}` + HTTP 200
- 原因：批量回收一旦整批回滚，意味着 100 个域名删了 50 个 nginx conf 失败要全部撤销，撤销逻辑本身就复杂且容易出错；让每条独立失败、单条 reason 透明，前端 toast "成功 N 失败 M（查看详情）" 是更可观测的语义
- 例外：超过 200 上限直接 HTTP 400（输入校验失败，不进入业务）

### D6. 批量回收的安全机制

- 只要求 confirm 弹窗（按用户原意），**不**要求输入确认文本
- 但 confirm 模态 MUST 列出所有待回收域名（不只是数量）+ 红字警告
- 后端串行处理：删 DB → 删 nginx conf → nginx -t → reload → 异步删 LE 证书（best-effort，原代码已有此模式）
- 单条 nginx -t 失败：该条 reason 含错误信息，继续处理后续条目；前面已成功的不回滚

### D7. 仍用 chi + form-encoded，不引入 JSON body

- 现有 handler 全部 `r.ParseForm()` 模式
- 批量端点用 `ids=1&ids=2&ids=3` 或 `ids=1,2,3` 形式，保持一致
- 响应仍可以是 JSON（toast 解析需要结构化数据），但请求体保持 form-encoded
- 避免引入 JSON encoder/decoder 的样板和错误处理分歧

### D8. 仓库布局

```
edge-proxy/
├── package.json                      # 新增
├── tailwind.config.js                # 新增
├── postcss.config.js                 # 新增
├── web/
│   └── input.css                     # 新增（@tailwind directives）
├── internal/web/
│   ├── static/
│   │   ├── tailwind.css              # 新增（构建产物，commit）
│   │   ├── app.js                    # 新增（Alpine 组件集合）
│   │   ├── alpine.min.js             # 新增（vendored，commit）
│   │   └── edge.css                  # 删除
│   ├── template/
│   │   ├── layout.html               # 重写：drawer 布局
│   │   ├── login.html                # 重写：居中卡片
│   │   ├── domains.html              # 重写：chip 搜索 + 浮动工具栏 + 模态
│   │   ├── upstreams.html            # 重写：同上
│   │   ├── config.html               # 重写：5 个 card 分组
│   │   └── partials/
│   │       ├── domain_row.html
│   │       ├── upstream_row.html
│   │       ├── batch_modal.html      # 通用批量确认模态
│   │       └── toast.html
│   ├── static.go                     # 更新 embed 清单
│   ├── pages.go                      # 新增 Render*Page 方法签名（含 q/status/page）
│   └── handler/
│       ├── domain.go                 # 新增 Batch* / ListGET 参数扩展
│       └── upstream.go               # 同上
├── internal/store/
│   ├── domain_repo.go                # 新增 ListByHosts / BatchUpdateStatus / Count
│   └── upstream_repo.go              # 新增 ListByAddrs / BatchEnable / BatchDelete
└── Makefile                          # 加 ui / ui-watch target
```

### D9. 数据访问层小幅扩展

`DomainRepo` 新增：
- `Search(hosts []string, status string, page, pageSize int) ([]*Domain, total int, err error)` —— hosts 为空走全表分页，非空走 `host IN (...)` 不分页
- `BatchUpdateStatus(ids []int64, target string, allowedFrom []string) (succeeded []int64, failed []FailedItem)` —— 用单 SQL `UPDATE ... WHERE id IN (...) AND status IN (allowedFrom)` + 二次 SELECT 找出未变更的
- `BatchDelete(ids []int64) ([]int64, []FailedItem)` —— 配合 recycle 用

`UpstreamRepo` 同构新增。

## Risks / Trade-offs

| 风险 | 缓解 |
|------|------|
| Tailwind 产物与源不同步（开发者改了 input.css 忘了 `make ui`） | CI check：`make ui && git diff --exit-code internal/web/static/tailwind.css`；本地 `make ui-watch` 文档化 |
| Alpine.js + htmx 组合在嵌套 fragment 替换时丢失 Alpine 状态 | 浮动工具栏、模态等关键 Alpine 状态放在 `<body>` 顶层，而非 htmx 替换的 fragment 内部 |
| 中文输入法 composition 事件兼容 | chip 组件 MUST 监听 `compositionstart`/`compositionend`，composition 期间禁用分隔符触发 |
| 批量回收 N 次 nginx reload 短时间高并发 | 串行处理（已有的实现是 best-effort 异步删证书+同步 reload），可加 throttle（≥100ms / 次）；上限 200 已是天花板 |
| 仓库 commit `tailwind.css` 产物导致 git diff 嘈杂 | 接受这个代价；好处是 CI 无 Node 依赖；可在 `.gitattributes` 标记为 `linguist-generated` |
| 删除 `edge.css` 时旧 layout 引用未清理导致 404 | 在 layout 重写同一 PR 内删除引用；构建后 smoke test 抓 `/static/edge.css` 应 404 |
| chip popover + sticky toolbar 在小屏（< 1024px）布局错乱 | 本工具受众是 SRE 桌面端，不做响应式保证；min-width 兜底为 1280px |
| 部分成功语义可能让前端误以为"已成功" | toast 文案明确区分 `succeeded.length === ids.length` vs 含 failed；failed 时 toast 类型为 warning 而非 success |
| LE 证书异步删除失败仅日志 | 沿用现状（与单条 recycle 一致）；运维通过日志/告警发现 |

## Migration Plan

由于只是 UI 重写 + 新增端点，无 DB schema 迁移、无破坏性 API 变更：

1. **Phase 1**：搭构建链（package.json / tailwind.config / Makefile / 提交一份基线 tailwind.css）
2. **Phase 2**：重写 layout.html + login.html + config.html（视觉升级，行为不变；现有 4 页面看上去全新但功能 100% 等价）
3. **Phase 3**：实现后端批量端点 + 仓储扩展 + 单元测试
4. **Phase 4**：实现前端 Alpine 组件（ChipInput / BatchToolbar / Modal / Toast），重写 domains.html
5. **Phase 5**：重写 upstreams.html（复用 Phase 4 的组件）
6. **Phase 6**：删除 edge.css 与对应引用、回归测试、CI 加 sync-check

**回滚**：每个 phase 一个独立 commit，phase 4/5 出问题可单独 revert；layout / login / config 的视觉升级即使 revert 也不影响 phase 3 的后端能力（后端能力对老 UI 也是 backward-compatible，只是不会被调用）。

## Open Questions

1. **是否需要前端单元测试基础设施**？Alpine 组件目前没有测试；可选方案：vitest + happy-dom，或不写测试（受众小、改动可控）。倾向**不写**，YAGNI。
2. **Alpine.js 的获取方式**：vendored（commit 一份 alpine.min.js）vs CDN？现状 htmx 走 unpkg CDN，建议统一改为 vendored（边缘节点常无外网，离线可用更重要）。需在 phase 1 同步处理。
3. **每页 50 还是动态可调（25/50/100）**？当前设计写死 50，若用户后续要求可加下拉，按 YAGNI 暂不做。
4. **chip 搜索是否支持通配（`*.example.com`）**？当前明确"只精确"。若后续有诉求，再加一个独立的 "wildcard" toggle，本次不做。
