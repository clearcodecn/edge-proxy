## ADDED Requirements

### Requirement: 域名列表分页

系统 SHALL 默认按每页 50 条分页显示域名列表，提供页码导航。当用户激活"批量精确搜索"（搜索框含 ≥1 个 chip）时，分页 MUST 自动禁用并一次性展示全部匹配结果。

#### Scenario: 无搜索时分页
- **WHEN** 用户访问 `/domains` 且未输入任何搜索 chip
- **THEN** 后端按 `?page=N` 返回该页 50 条，页脚渲染页码导航

#### Scenario: 搜索激活时禁用分页
- **WHEN** 用户在搜索框输入 ≥1 个 chip
- **THEN** 分页控件 MUST 隐藏，列表展示全部匹配结果（受下一项的截断上限约束）

### Requirement: 域名 chip 式精确搜索

系统 SHALL 提供 chip 风格的域名搜索输入框：用户输入 / 粘贴 / 回车 / 逗号分隔的每个域名转为一个独立 chip，每个 chip 可通过 × 单独删除。匹配规则 MUST 为**精确等值**（SQL `host IN (...)`），不做模糊匹配。搜索结果数 MUST 受 200 条上限保护；超出时返回前 200 条并展示截断警告。

#### Scenario: 单条输入转 chip
- **WHEN** 用户在输入框键入 `a.example.com` 并按下 Enter / Tab / `,` / 空格
- **THEN** 输入框 MUST 把该字符串转换为一个 chip 元素，输入光标移至 chip 右侧

#### Scenario: 多行粘贴批量转 chip
- **WHEN** 用户从剪贴板粘贴含换行符的多个域名（例如 `a.com\nb.com\nc.com`）
- **THEN** 输入框 MUST 一次性生成三个 chip，每个对应一行

#### Scenario: chip 删除
- **WHEN** 用户点击某个 chip 上的 × 按钮
- **THEN** 该 chip MUST 从输入框移除，列表 MUST 重新触发搜索

#### Scenario: 空输入位 Backspace 删除最后 chip
- **WHEN** 输入框文本为空且至少存在 1 个 chip，用户按 Backspace
- **THEN** 最后一个 chip MUST 被删除

#### Scenario: chip 溢出折叠
- **WHEN** chip 总宽超出输入框可视宽度
- **THEN** 输入框 MUST NOT 换行；超出部分以 `+N` 徽章替代，点击徽章 MUST 弹出 popover 列出所有溢出 chip，每个仍可通过 × 删除

#### Scenario: 精确匹配查询
- **WHEN** 输入框含 chip `[a.com, b.com, c.com]`
- **THEN** 后端 SQL 必须等价于 `WHERE host IN ('a.com', 'b.com', 'c.com')`，MUST NOT 使用 LIKE 模糊匹配

#### Scenario: 搜索结果超限
- **WHEN** 匹配结果数 > 200
- **THEN** 响应 MUST 返回前 200 条，且页面顶部 MUST 渲染警告条"匹配过多，已截断为 200 条"

#### Scenario: 中文输入法兼容
- **WHEN** 用户处于中文输入法 composition 状态（拼音未上屏）
- **THEN** 输入框 MUST NOT 把未上屏字符当作 chip 内容，需等待 compositionend 事件后再判断分隔符

### Requirement: 域名状态筛选

系统 SHALL 在搜索框旁提供状态下拉筛选（全部 / pending / cert_applying / cert_failed / online / degraded / deprecated），与 chip 搜索可叠加生效。

#### Scenario: 状态叠加搜索
- **WHEN** 用户选择"cert_failed" + chip `[a.com, b.com]`
- **THEN** 列表 MUST 仅展示状态为 cert_failed 且 host 在 `[a.com, b.com]` 内的域名

### Requirement: 域名行勾选与全选

系统 SHALL 在域名列表每行最左侧渲染 checkbox，表头 checkbox 实现"全选当前可见行"。翻页或修改搜索条件时，勾选状态 MUST 全部清空（不跨视图保留）。

#### Scenario: 单行勾选
- **WHEN** 用户点击某行 checkbox
- **THEN** 该行 `data-selected` 状态翻转，浮动批量工具栏更新已选数量

#### Scenario: 全选当前页
- **WHEN** 用户点击表头 checkbox
- **THEN** 当前页所有可见行 MUST 同步勾选；再次点击 MUST 全部取消

#### Scenario: 翻页清空选中
- **WHEN** 用户在已勾选 5 行的状态下点击页码切换
- **THEN** 切换后的页面 MUST 不保留任何勾选

### Requirement: 浮动批量工具栏

系统 SHALL 在用户勾选 ≥1 行后，在列表顶部显示**粘性浮动工具栏**，左侧展示"已选 N 项"与"清空选择"按钮，右侧展示批量操作按钮。勾选数为 0 时 MUST 隐藏。

#### Scenario: 工具栏出现
- **WHEN** 用户勾选第一行
- **THEN** 粘性工具栏 MUST 从无到有渲染（CSS `position: sticky; top: 0`），并显示已选数 = 1

#### Scenario: 工具栏隐藏
- **WHEN** 用户取消最后一个勾选或点击"清空选择"
- **THEN** 工具栏 MUST 立即隐藏

### Requirement: 域名批量导入

系统 SHALL 提供"批量导入域名"入口（按钮触发模态框），模态内含 textarea 用于多行粘贴域名。单次提交 MUST 限制 ≤ 200 个域名；超过 MUST 返回 HTTP 400 并提示。后端 MUST 返回结构化结果区分**已创建 / 已跳过（已存在）/ 失败（格式错误）**。

#### Scenario: 批量导入成功
- **WHEN** 用户粘贴 100 个有效且不重复的域名并提交
- **THEN** 后端 MUST 返回 `{created: [...100 ids], skipped: [], failed: []}`，列表 MUST 自动刷新展示新条目

#### Scenario: 部分跳过 + 部分失败
- **WHEN** 用户粘贴的 100 行中含 3 个已存在域名、2 个格式错误
- **THEN** 后端 MUST 返回 `created: 95, skipped: 3, failed: 2`，模态 MUST 展示明细列表（每条含 host + 原因）

#### Scenario: 超过单次上限
- **WHEN** 用户提交 250 行域名
- **THEN** 后端 MUST 返回 HTTP 400 + "单次导入不能超过 200 条"

#### Scenario: 默认跳过已存在
- **WHEN** 批量导入中包含已存在域名
- **THEN** 默认行为 MUST 为"跳过且不报错"，计入 `skipped` 而非 `failed`

### Requirement: 域名批量复制到剪贴板

系统 SHALL 在浮动工具栏提供"复制域名列表"按钮，点击后将所有选中行的 host 以换行分隔的纯文本写入系统剪贴板，并展示成功 toast。

#### Scenario: 复制成功
- **WHEN** 用户勾选 5 行后点击"复制域名列表"
- **THEN** 系统剪贴板 MUST 包含 5 行域名（每行一个 host），右下角弹出 toast "已复制 5 个域名"

### Requirement: 域名批量废弃

系统 SHALL 在浮动工具栏提供"批量废弃"按钮，点击后弹出模态列出所有选中域名并要求确认。后端 MUST 采用**部分成功**语义：返回 `{succeeded: [ids], failed: [{id, reason}]}`，已是 deprecated 的域名 MUST 跳过并计入 failed 而非整批回滚。

#### Scenario: 全部成功
- **WHEN** 用户勾选 5 个 online 域名并确认批量废弃
- **THEN** 5 个域名状态 MUST 变为 deprecated，响应 `succeeded: [...5 ids], failed: []`

#### Scenario: 含已废弃域名
- **WHEN** 选中的 5 个里有 1 个已是 deprecated
- **THEN** 该域名 MUST 出现在 `failed` 数组中（reason="已废弃"），其余 4 个仍然成功

### Requirement: 域名批量重试

系统 SHALL 在浮动工具栏提供"批量重试"按钮，仅对状态为 `cert_failed` 的域名生效，其余 ID MUST 计入 failed 并标注原因，不整批回滚。

#### Scenario: 重试失败的证书申请
- **WHEN** 用户勾选 3 个 cert_failed 域名并确认批量重试
- **THEN** 3 个域名 MUST 被重置为 pending、fail_count=0、last_error 清空

#### Scenario: 混入非失败状态
- **WHEN** 选中含 1 个 online 域名
- **THEN** 该 online 域名 MUST 出现在 failed 中（reason="只能重试失败的申请"）

### Requirement: 域名批量回收

系统 SHALL 在浮动工具栏提供"批量回收"按钮，仅对 `deprecated` 状态的域名生效。点击后 MUST 弹出确认模态列出待回收的全部域名 + 红字警告"含 nginx conf + LE 证书将一并删除，不可恢复"，仅需用户点击"确认"按钮（**不**要求用户输入额外文本）。后端 MUST 串行处理每个域名（删 DB → 删 nginx conf → reload → 异步删 LE 证书），单条失败不影响其它条目。

#### Scenario: 确认弹窗
- **WHEN** 用户勾选 10 个 deprecated 域名后点击"批量回收"
- **THEN** 模态 MUST 列出 10 个待回收域名 + 红字警告 + 一个标红的"确认回收"按钮，无需输入文本

#### Scenario: 单条 nginx reload 失败
- **WHEN** 批量回收过程中某个域名的 nginx -t 失败
- **THEN** 该域名 MUST 计入 failed（reason 含 nginx 错误信息），后续域名 MUST 继续执行，前面已成功的 MUST NOT 回滚

#### Scenario: 非 deprecated 状态拒绝
- **WHEN** 选中含 1 个 online 域名
- **THEN** 该 online 域名 MUST 出现在 failed 中（reason="只有已废弃域名可回收"）

### Requirement: 回源列表与批量能力（与域名对齐）

系统 SHALL 为回源列表提供与域名列表**结构同构**的能力：chip 式精确搜索（按 `addr` 匹配）、状态筛选（全部/启用/禁用）、分页（每页 50，搜索时禁用）、行勾选与全选、粘性浮动工具栏。

#### Scenario: 回源同构 UI
- **WHEN** 用户访问 `/upstreams`
- **THEN** 页面 MUST 渲染与 `/domains` 同构的 chip 搜索 + 状态筛选 + 分页 + 行勾选 + 浮动工具栏

### Requirement: 回源批量导入（CSV-lite）

系统 SHALL 提供"批量导入回源"入口，textarea 每行解析为 `addr[,weight][,backup|main][,remark]` 四字段：缺省 `weight=1`、`backup=false`、`remark=空`；含逗号的 remark MUST 通过双引号包裹。单次提交 MUST ≤ 200 条。后端 MUST 返回 `{created, failed: [{line, reason}]}`。

#### Scenario: 仅 addr 行
- **WHEN** 粘贴 `10.0.0.5:80`
- **THEN** 后端创建一条 upstream，weight=1、backup=false、remark=""

#### Scenario: 完整四字段
- **WHEN** 粘贴 `10.0.0.7:80, 2, backup, "rack-A 主力"`
- **THEN** 后端创建一条 upstream，weight=2、backup=true、remark="rack-A 主力"

#### Scenario: 中间字段留空
- **WHEN** 粘贴 `10.0.0.8:8080, , , "rack-B"`
- **THEN** 后端使用默认 weight=1、backup=false、remark="rack-B"

#### Scenario: 行格式错误
- **WHEN** 粘贴 `bad-format-line`（缺失端口）
- **THEN** 该行 MUST 出现在 `failed` 中并附带原因，其余合法行 MUST 正常创建

### Requirement: 回源批量启用 / 禁用 / 删除

系统 SHALL 在回源浮动工具栏提供三个批量操作：批量启用（仅对 `enabled=false` 生效）、批量禁用（仅对 `enabled=true` 生效）、批量删除（弹 confirm 后直接删除）。三者均采用**部分成功**语义。

#### Scenario: 批量启用
- **WHEN** 用户勾选 3 个 disabled upstream 并确认批量启用
- **THEN** 3 个 upstream 的 `enabled` MUST 设为 true，响应 `succeeded: [...3], failed: []`

#### Scenario: 批量禁用混入已禁用
- **WHEN** 选中含 1 个已 disabled
- **THEN** 该项 MUST 计入 failed（reason="已禁用"），其它正常禁用

#### Scenario: 批量删除确认
- **WHEN** 用户勾选 5 个 upstream 点击"批量删除"
- **THEN** MUST 弹 confirm 模态列出待删除条目，仅需点击"确认"按钮，无需输入

### Requirement: 批量端点的部分成功响应格式

系统 SHALL 让所有批量操作端点（domains/batch/{deprecate,retry,recycle}、upstreams/batch/{enable,disable}、DELETE /upstreams/batch）使用统一响应格式 `{"succeeded": [int64], "failed": [{"id": int64, "reason": string}]}`，HTTP 状态码 MUST 为 200（部分失败不算请求失败）。

#### Scenario: 全部成功
- **WHEN** 批量端点处理的所有 id 均成功
- **THEN** 响应 MUST 为 `{"succeeded": [...], "failed": []}`，HTTP 200

#### Scenario: 全部失败
- **WHEN** 批量端点处理的所有 id 均失败
- **THEN** 响应 MUST 为 `{"succeeded": [], "failed": [...]}`，HTTP 200（前端通过 failed 长度判断）

#### Scenario: 鉴权失败
- **WHEN** 未认证用户请求任意 `/domains/batch/*` 或 `/upstreams/batch*` 端点
- **THEN** 响应 MUST 为 HTTP 401 / 302（与现有 session 中间件一致），不进入业务处理

### Requirement: 批量端点的输入上限

系统 SHALL 对所有 batch 端点的 `ids` 数组或 `lines` 数组大小施加 ≤ 200 上限；超过 MUST 返回 HTTP 400 + 中文错误消息。

#### Scenario: 超限拒绝
- **WHEN** 客户端 POST `/domains/batch/recycle` 含 250 个 id
- **THEN** 响应 MUST 为 HTTP 400 + 消息"单次批量操作不能超过 200 条"
