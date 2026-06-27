---
name: "JYOPSX: Propose"
description: "一步生成所有变更工件（Proposal, Design, Tasks）"
category: Workflow
tags: [workflow, artifacts, experimental]
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

3. **获取工件构建顺序**
   ```bash
   jy-openspec status --change "<name>" --json
   ```

4. **按顺序生成工件**
   - **核心规则**：如果正在生成 **design.md**，必须先加载使用 `writing-plans` 技能。
   - 必须使用简体中文。

5. **完成汇总**
   - 引导：“运行 `/jyopsx-apply` 开始执行任务。”
