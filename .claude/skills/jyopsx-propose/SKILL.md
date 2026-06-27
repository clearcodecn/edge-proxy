---
name: jyopsx-propose
description: 一步生成所有变更工件（Artifacts）。适用于当你清楚要构建什么，并希望快速生成 Proposal、Design 和 Tasks 时。
license: MIT
compatibility: Requires jy-openspec CLI.
metadata:
  author: jy-openspec
  version: "1.0"
  generatedBy: "1.2.0"
---

【强制使用中文】作为资深架构师，你需要全程使用简体中文。

一步完成新变更的立项：创建变更目录并生成所有必要的工件。

我将为你创建包含以下文件的变更：
- proposal.md (初衷与目标)
- design.md (技术设计)
- tasks.md (实施步骤清单)

当一切准备就绪，运行 `/jyopsx-apply` 开始编码。

---

**输入**: 用户的请求应包含变更名称（kebab-case）或对要构建内容的描述。

**执行步骤**

1. **若输入不明确，询问具体需求**
   使用 **AskUserQuestion tool** 提问：
   > "您想进行什么变更？请描述您要构建或修复的内容。"
   根据描述推导出一个 kebab-case 名称（例如："增加用户认证" → `add-user-auth`）。

2. **创建变更目录**
   ```bash
   jy-openspec new change "<name>"
   ```
   这将在 `openspec/changes/<name>/` 创建基础结构。

3. **获取工件构建顺序**
   ```bash
   jy-openspec status --change "<name>" --json
   ```
   解析 JSON 获取 `applyRequires` 数组和工件列表。

4. **按顺序生成工件，直到达到可实施状态**
   使用 **TodoWrite tool** 追踪进度。
   循环处理处于 `ready` 状态的工件：
   
   a. **针对每个就绪的工件**:
      - 获取指令：
        ```bash
        jy-openspec instructions <artifact-id> --change "<name>" --json
        ```
      - 读取依赖文件获取上下文。
      - **核心规则**：
        - 如果正在生成 **design.md**，必须先加载使用 `writing-plans` 技能，并严格遵循其设计心法。
        - 必须使用简体中文。
      - 创建工件文件。
      - 提示进度：“已创建 <artifact-id>”。

   b. **重复直至所有 `applyRequires` 里的工件状态均为 `done`**。

5. **展示最终状态**
   ```bash
   jy-openspec status --change "<name>"
   ```

**输出要求**
完成后汇总：
- 变更名称与路径
- 已创建的工件清单
- 结束语：“所有工件已生成！准备好进入实施阶段。”
- 引导：“运行 `/jyopsx-apply` 开始执行任务。”

**守则 (Guardrails)**
- 严禁在工件中复制 `<context>` 或 `<rules>` 代码块。
- 必须读取前置工件后再写新工件。
- 验证每个文件是否真实写入磁盘。
