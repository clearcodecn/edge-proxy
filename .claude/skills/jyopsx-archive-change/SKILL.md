---
name: jyopsx-archive-change
description: 归档已完成的变更。这将把临时的工件（Proposal, Specs, Design）合并到项目的主规格库中。
license: MIT
compatibility: Requires jy-openspec CLI.
metadata:
  author: jy-openspec
  version: "1.0"
  generatedBy: "1.2.0"
---

【强制使用中文】作为资深研发助手，你需要全程使用简体中文。

归档已完成的变更并更新项目主规格。

**输入**: 可选指定变更名称。

**执行步骤**

1. **选择变更**
   推断或运行 `jy-openspec list --json` 请用户选择。

2. **验证状态**
   运行 `jy-openspec status --change "<name>" --json`。
   如果任务未全部完成，提示用户确认是否强制归档。

3. **执行归档**
   **重要**: 必须使用 `--yes` 参数跳过交互式确认，因为 AI 无法处理终端交互提示。
   ```bash
   jy-openspec archive "<name>" --yes
   ```
   如果遇到验证错误（如中文规格格式问题），可追加 `--no-validate`：
   ```bash
   jy-openspec archive "<name>" --yes --no-validate
   ```
   此操作会：
   - 更新 `openspec/specs/` 下的主规格文件。
   - 将变更目录移至 `openspec/changes/archive/`。

4. **确认结果**
   确认归档成功并告知用户主规格已更新。

**守则 (Guardrails)**
- 归档前确保用户没有未提交的代码更改（可选建议）。
- 执行归档命令时必须带上 `--yes` 参数，严禁使用不带参数的裸命令。
- 归档后引导用户查看更新后的主规格。
