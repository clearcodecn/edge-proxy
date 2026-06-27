## ADDED Requirements

### Requirement: 左侧固定导航菜单

系统 SHALL 在所有已登录页面提供宽度 240px 的左侧固定导航菜单，包含三个一级菜单项：**域名**、**回源**、**配置**。菜单 MUST 始终展开（不提供折叠/抽屉模式），当前页对应菜单项 MUST 高亮。

#### Scenario: 已登录用户访问任意管理页面
- **WHEN** 已认证用户访问 `/`、`/upstreams`、`/config` 任意一页
- **THEN** 页面左侧渲染 240px 宽固定菜单，顶部为品牌区，下方为三个一级菜单项

#### Scenario: 当前页菜单高亮
- **WHEN** 用户访问 `/upstreams`
- **THEN** 菜单中"回源"项 MUST 呈高亮状态，其它两项保持默认态

#### Scenario: 未登录用户访问
- **WHEN** 未认证用户访问 `/`
- **THEN** 系统重定向到 `/login`，登录页 MUST NOT 渲染左侧菜单

### Requirement: 顶栏信息展示与登出

系统 SHALL 在所有已登录页面顶部渲染一条顶栏，显示：品牌名 `edge-proxy`、当前节点名（取自 hostname 或配置）、当前登录用户名、"退出"按钮。顶栏 MUST NOT 包含其它导航项或操作。

#### Scenario: 顶栏元素齐全
- **WHEN** 已认证用户访问任意管理页面
- **THEN** 顶栏从左到右依次显示：品牌名、节点名、用户名、退出按钮

#### Scenario: 点击退出
- **WHEN** 用户点击顶栏"退出"按钮
- **THEN** 系统清除 session 并重定向到 `/login`

### Requirement: 登录页视觉风格

系统 SHALL 提供**居中卡片式**登录页：上方品牌 logo + 应用名、下方账号/密码输入卡片、底部小字显示版本号与节点名。登录页 MUST NOT 渲染左侧菜单或顶栏。

#### Scenario: 渲染登录页
- **WHEN** 用户访问 `/login`
- **THEN** 页面在视口居中渲染：顶部品牌区、中部登录表单卡片、底部"vX.Y · <node-name>"小字

#### Scenario: 登录失败展示错误
- **WHEN** 用户提交错误的用户名或密码
- **THEN** 登录卡片内显示红色错误提示，表单字段值保留以便重试

### Requirement: 配置页视觉分组

系统 SHALL 把现有只读配置项按语义分组渲染为五个卡片：**管理（admin）**、**ACME**、**探测（probe）**、**告警（alert）**、**路径（paths）**，顶部 MUST 显示一条信息提示，说明配置只读及修改方式。

#### Scenario: 渲染配置页
- **WHEN** 用户访问 `/config`
- **THEN** 页面顶部渲染只读提示条，下方为五个 daisyUI card，每张卡片内为该分组的键值对列表

#### Scenario: 敏感字段脱敏
- **WHEN** 渲染配置页
- **THEN** `password_hash`、`dingtalk.secret`、`telegram.bot_token` 等敏感字段 MUST NOT 出现在响应中，仅以"已配置 / 未配置"徽章替代

### Requirement: 前端构建链与单 binary 交付

系统 SHALL 使用 Tailwind CSS + daisyUI 生成样式产物，使用 Alpine.js 提供前端交互状态。构建产物（`tailwind.css`、`alpine.min.js`）MUST 通过 `//go:embed` 嵌入 Go binary，最终交付物 MUST 仍为单一可执行文件。

#### Scenario: 生产构建产物嵌入
- **WHEN** 执行 `go build` 生成 `edge-proxy` binary
- **THEN** binary MUST 自包含全部前端静态资源，运行时无需额外文件即可在 `/static/*` 提供服务

#### Scenario: 开发期重生成样式
- **WHEN** 开发者修改任意 `.html` 模板或 Tailwind 配置后执行 `make ui`
- **THEN** 命令 MUST 通过本地 Node.js 工具链重新生成 `internal/web/static/tailwind.css`

#### Scenario: 无 Node.js 的构建机
- **WHEN** 在没有 Node.js 的环境（CI/纯 Go 构建机）执行 `go build`
- **THEN** 构建 MUST 成功，使用仓库中已 commit 的 `tailwind.css` 产物

### Requirement: 移除旧手写 CSS

系统 MUST 移除 `internal/web/static/edge.css`（约 200 行手写 CSS）以及 layout 模板中对它的 `<link>` 引用，避免与 Tailwind 产物并存导致样式冲突。

#### Scenario: 旧 CSS 不再被引用
- **WHEN** 访问 `/static/edge.css`
- **THEN** 服务 MUST 返回 404
