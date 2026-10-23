# Prompt 管理

路径：`/prompt/user`、`/prompt/skill`（`/prompt/command` 会重定向到 Skills）

Prompt 管理提供 IDE 录制浏览、Skills 管理等功能，帮助团队积累和复用 AI 编程知识。

> **注意**：Prompt 管理是一对可选的 Power-up——**User prompts** 和 **Skills**——需要在 [Power-ups 菜单](./02-scenario-overview.md#power-ups齿轮背后的菜单)或[实验性功能](./19-experimental.md)里打开。它们作为 **User Request** 和 **Skills** 出现在 **Power-ups** 导航项下。（不再有版本拆分；桌面应用同样有这两项。）

---

## 用户录制（User Requests）

路径：`/prompt/user`

### 功能说明

该页面（标题 **User Recordings**——*Browse and manage your IDE recordings*）浏览和管理从 Claude Code IDE 会话中录制的交互记录，适合：
- 回顾历史 AI 辅助决策过程
- 提取成功的提示词模板
- 团队经验分享

![用户录制](../images/prompt-user.png)

### 布局

顶部一条筛选栏——**Search recordings…**、**User**、**Workspace**（录制时 IDE 打开的工作区；以前叫 *Project*）和 **Type**（code-review / debug / refactor / test）——下面是三列：

**左列：日历**——按录制条数着色的月历（Less → More），带范围按钮 **Today / 7D / 30D / 90D**

**中列：录制列表**——标题是所选日期及其条数（`10/23/2026 (0)`）；每条录制显示标题、类型徽章和时间；当天没有时显示 *No recordings found*

**右列：Recording Details**——摘要、元数据（用户、工作区、类型、时长、模型、时间戳）和完整对话

---

## Skills 管理

![Skills 管理](../images/prompt-skills.png)

路径：`/prompt/skill`

### 功能说明

该页面（标题 **Skill Management**——*Manage your AI skill locations from various IDEs and tools*）管理从 IDE 配置（如 `.claude/skills/` 目录）同步的可复用 Prompt 片段，支持：
- 从多个 IDE 来源自动发现 Skills
- 分组浏览和搜索
- 查看 Skill 的 Markdown/原始内容

### 三列布局

**左列：Skill 位置（Locations）**

每个 Skill 位置（Location）对应一个目录来源：
- 位置名称
- 路径
- IDE 来源徽章（如 claude_code）
- Skill 数量
- 操作：刷新、编辑、删除

顶部按钮：
- **Add Location**：手动添加 Skill 目录（显示名称、路径和 IDE 来源）
- **Auto Discover**：扫描主目录里受支持的 IDE（Claude Code、OpenCode、VS Code、Cursor、Codex、Antigravity、Gemini CLI、GitHub Copilot、Windsurf 等）并导入它们的 Skills——在对话框里选择要导入哪些

**中列：Skills 列表**

选中位置后展示该位置下的所有 Skills：
- **分组/平铺** 视图切换（列标题里的图标）
- 搜索过滤

**右列：Skill 内容**

选中 Skill 后展示：
- 文件元数据（路径、大小、修改时间）
- **Markdown 渲染**视图（默认）
- **Raw** 原始文本视图
- 复制按钮

---

## 相关页面

- [实验性功能](./19-experimental.md)
- [Claude Code 场景](./03-scenario-claude-code.md)
