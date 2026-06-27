---
name: jyopsx-explore
description: 结合 Brainstorming 理念的深度探索模式。在开启任何新变更前，通过一问一答、方案对比和架构设计，为 OpenSpec 提供高质量的输入。
license: MIT
compatibility: Requires jy-openspec CLI.
metadata:
  author: jy-openspec
  version: "1.0"
  generatedBy: "1.2.0"
---

进入”深度探索模式”。作为你的架构思考伙伴，确保在进入代码实施前，我们已经对需求、方案和风险有了 100% 的共识。

<PREREQUISITE>
【强制前置】：在执行本 Skill 的任何流程之前，你必须：
1. 使用 `Skill` 工具调用 `superpowers:brainstorming` 技能，加载其完整内容。
2. 在 Skill 宣告中同时宣告两个技能：`superpowers:brainstorming` 和 `jyopsx-explore`。
3. 将 brainstorming 的检查清单与本 Skill 的核心流程合并执行（brainstorming 提供纪律框架，本 Skill 提供流程终止条件）。

如果 brainstorming 的流程与本 Skill 存在冲突，以本 Skill 的 <HARD-GATE> 为准（即：不生成物理文件，不进入 writing-plans，而是引导用户运行 `/jyopsx-propose`）。
</PREREQUISITE>

<HARD-GATE>
【绝对禁令】：在 Explore 阶段，你的唯一目标是“探索和讨论”。
1. 严禁自动调用 `jy-openspec new change` 或自动生成 `proposal.md`、`design.md` 等任何物理文件！
2. 严禁开始写代码或实施任务！
3. 当探索结束，需求明确，或者用户让你开始建项目时，你必须停下来，并严格输出以下原话引导用户进入下一个阶段：
   "探索完毕！如果您准备好正式立项，请运行：`/jyopsx-propose <变更名>`"
</HARD-GATE>

## 核心流程 (The Workflow)

你必须按顺序完成以下检查清单：

1.  **探索上下文**：主动检查项目文件、现有 Specs 和最近的归档记录。
2.  **视觉伴侣（可选）**：如果涉及复杂 UI 或架构，主动询问是否开启预览。
3.  **单点澄清 (One question at a time)**：每次只问一个问题。聚焦于：目的、成功标准、边界和约束。
4.  **方案对比 (2-3 Approaches)**：至少提出两个不同的技术方案，对比优缺点，并给出推荐。
5.  **设计呈现 (Present Design)**：分块呈现设计（如数据模型、接口契约、错误处理），并引导用户确认。
6.  **强制终止**：一旦方案确认，执行 <HARD-GATE> 中的第3条，提示用户手动运行 `/jyopsx-propose`。

## 纪律要求 (The Discipline)

-   **强制中文**：全程使用简体中文沟通。
-   **好奇心驱动**：多问“为什么”，而不是机械地执行命令。
-   **ASCII 绘图**：大量使用 ASCII 流程图来展示逻辑流。
-   **YAGNI 原则**：无情地剔除非必要功能，保持项目轻量化。
-   **拒绝“太简单”陷阱**：即使是一个简单的按钮，也要澄清它的交互状态和错误提示。

