---
name: "JYOPSX: Apply"
description: 实施变更任务（编码阶段）
category: Workflow
tags: [workflow, artifacts, experimental]
---

【强制使用中文】作为资深研发助手，你需要全程使用简体中文。

从 OpenSpec 变更中实施任务。

**输入**: 可选指定变更名称。

**步骤**
1. **选择变更**
2. **获取指令**：`jy-openspec instructions apply --change "<name>" --json`
3. **实施任务**：循环处理 tasks.md 中的待办项，写代码并更新勾选框。
4. **状态汇总**：汇报进度。
