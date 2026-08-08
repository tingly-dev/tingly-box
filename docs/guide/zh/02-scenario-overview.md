# 场景总览

路径：`/agent`

---

![场景总览](../images/scenario-overview.png)

## 页面功能

**Agents** 是 Tingly-Box 的 Agent 场景导航中心，以卡片网格形式展示所有可用场景。页面副标题：「Pick a scenario to configure. Hide the ones you don't use to keep the sidebar tidy.」

### 场景卡片

每张卡片包含：
- **图标**：场景对应工具/平台的 Logo
- **名称**：场景名称（如 Claude Code、Codex、OpenCode 等）
- **描述**：两行截断的场景简介
- **状态行**：卡片的实时配置状态——若已有路由规则则显示规则数（如 `3 rules`），否则显示 `Not configured yet`，让页面一眼回答「我已经配置了什么」
- **Hidden 标记**：已隐藏的场景显示灰色 `Hidden` 徽章

### 可见性管理

卡片右上角有一个小巧的**眼睛图标**，用于控制该场景是否出现在左侧活动栏（Sidebar）中。默认隐藏，仅在悬停时出现（已隐藏的卡片则始终显示），避免和场景名称/描述抢占视觉焦点。

- 点击隐藏 → 场景从侧边栏隐藏，但仍可通过总览页直接访问（显示 `Hidden` 徽章和带斜杠的眼睛图标）
- 点击取消隐藏 → 场景重新显示在侧边栏

> 仅部分场景支持隐藏，Claude Code 始终显示在侧边栏。

---

## 全部场景列表

| 场景 | 路径 | 说明 |
|------|------|------|
| Claude Code | `/agent/claude_code` | 通过自定义 Profile 和分任务模型路由 Claude Code |
| Claude Desktop | `/agent/claude_desktop` | 通过 Tingly Box 将 Claude Desktop 接入为 MCP 客户端 |
| Codex | `/agent/codex` | 通过你的 Provider 密钥配置 Codex CLI |
| OpenCode | `/agent/opencode` | 由你的 Provider 驱动的开源编程 Agent |
| Xcode | `/agent/xcode` | 将你的模型接入 Xcode 的编程智能功能 |
| VS Code | `/agent/vscode` | 通过 Tingly Box 驱动 VS Code Copilot Chat |
| OpenAI SDK | `/agent/openai` | OpenAI 兼容 SDK 端点，即插即用 |
| Anthropic SDK | `/agent/anthropic` | Anthropic 兼容 SDK 端点，即插即用 |
| Embedding | `/agent/embed` | 将 Embedding 请求路由到你的 Provider |
| Image Gen | `/agent/imagegen` | 通过 Tingly Box 路由图像生成请求 |
| OpenClaw | `/agent/agent` | 通用 Agent 运行器（默认隐藏） |
| Team | `/agent/team` | 面向全团队的共享中央模型部署（默认隐藏） |
| Playground | `/agent/playground` | 图像生成交互测试台（不在卡片网格中，通过侧边栏进入） |

---

## 导航结构

左侧活动栏（Activity Bar）图标对应 **Scenarios** 分组，点击后在次级侧边栏展示所有可见场景的导航项。

- 每个场景导航项支持直接点击跳转到对应配置页
- Claude Code 支持多 Profile，每个 Profile 作为独立导航子项展示

---

## 相关页面

- [Claude Code 场景](./03-scenario-claude-code.md)
- [其他编程 Agent](./04-scenario-coding-agents.md)
- [OpenAI / Anthropic SDK 代理](./05-scenario-sdk-proxy.md)
- [Claw / Embed / ImageGen](./06-scenario-special.md)
- [Playground](./07-scenario-playground.md)
