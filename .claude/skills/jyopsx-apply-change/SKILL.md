---
name: jyopsx-apply-change
description: 从 OpenSpec 变更中实施任务。适用于开始编码、继续实施或按步骤完成任务时。
license: MIT
compatibility: Requires jy-openspec CLI.
metadata:
  author: jy-openspec
  version: "1.0"
  generatedBy: "1.2.0"
---

【强制使用中文】作为资深研发助手，你需要全程使用简体中文。

从 OpenSpec 变更中实施任务。

**输入**: 可选指定变更名称。如果省略，检查是否可以从对话上下文中推断。如果模糊不清，必须提示用户选择。

**执行步骤**

1. **选择变更**
   如果提供了名称，则使用它。否则：
   - 从上下文推断。
   - 如果只有一个活动变更，自动选择。
   - 否则运行 `jy-openspec list --json` 并请用户选择。
   
   始终声明：“当前变更：<name>”，并告知如何切换（如 `/jyopsx-apply <其他>`）。

2. **检查状态以了解 Schema**
   ```bash
   jy-openspec status --change "<name>" --json
   ```

3. **获取实施指令**
   ```bash
   jy-openspec instructions apply --change "<name>" --json
   ```
   根据状态处理：
   - 若状态为 `blocked` (缺少工件): 提示使用 `/jyopsx-continue`。
   - 若状态为 `all_done`: 祝贺并建议归档。
   - 否则：继续实施。

4. **读取上下文文件**
   读取指令输出中 `contextFiles` 列表的文件（如 proposal, specs, design, tasks）。

5. **显示当前进度**
   展示已完成任务数和剩余任务概览。

6. **实施任务（循环直至完成或阻塞）**
   针对每个待办任务：
   - 说明正在处理哪个任务。
   - 进行代码更改（保持简洁聚焦）。
   - 完成后立即更新任务文件：`- [ ]` → `- [x]`。
   - 遇到不明确或设计问题时，停下来请示。

7. **结束或暂停时展示状态**
   汇总本次会话完成的任务及总体进度。

**守则 (Guardrails)**
- 实施前必须读取上下文文件。
- 严禁猜测，不明确时必须询问。
- 每完成一个任务，必须立刻更新任务勾选框。
- 保持代码修改的原子性和聚焦性。
