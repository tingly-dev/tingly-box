# Tingly-Box 用户指南

Tingly-Box 是一个 AI 智能体编排平台，提供 LLM 网关、远程控制和安全防护能力。本指南按功能模块分章节说明 Web UI 的完整使用方法（桌面应用展示的是同一个产品）。

---

## 从这里开始

第一次使用 Tingly-Box？五步，大约十分钟。每一步都链接到对应页面的**快速开始**；本指南的每个页面开头都有一个。

1. **安装并启动**：`npm install -g tingly-box@latest`，然后 `tb start`（或者直接 `npx -y tingly-box@latest`）。→ [快速开始（首次使用）](./01-getting-started.md#快速开始)
2. **连接 Provider**：**Credential → 连接 AI**，粘贴 API Key 或用 OAuth 登录。→ [凭证管理](./08-credentials.md#快速开始)
3. **选一个 Agent 并一键配置**：[Claude Code](./03-scenario-claude-code.md#快速开始) · [Codex](./04-scenario-codex.md#快速开始) · [OpenCode、Pi、VS Code 等](./04-scenario-coding-agents.md#快速开始) · [Cursor](./04-scenario-cursor.md#快速开始) · [在自己的代码里使用 OpenAI / Anthropic SDK](./05-scenario-sdk-proxy.md#快速开始)
4. **在仪表盘里验证**：Agent 页面的状态行显示「已接入」，请求出现在[用量面板](./11-dashboard.md#快速开始)里。
5. **按需开启的功能**：[Team](./07-team.md#快速开始)（分享给一个小组）· [Image](./07-image.md#快速开始) · [Remote](./12-remote-control.md#快速开始)（在聊天软件里用 Claude Code）· [Projects](./22-projects.md#快速开始)（在浏览器里用 Claude Code）· [MCP 与工具](./16-mcp-tools.md#快速开始)

> 界面语言为中文时，按钮显示中文；本指南中的英文标签（如 **Connect AI**、**Auto Config**）对应英文界面，快速开始里会在括号中给出对照。

---

## 目录

### 一、快速上手
- [初始化与 Provider 接入](./01-getting-started.md) — 标准的首次使用流程：安装、连接 Provider、一键配置、验证

### 二、Agent 场景

Agent 场景是 Tingly-Box 的核心功能，将各类 AI 编程工具的 API 请求统一代理到你配置的 Provider。所有 Agent 页面共用一种布局（状态行、可折叠的连接卡、Plugins 卡、Quick Start、Model Rules），在 Claude Code 一章里统一说明。

- [场景总览](./02-scenario-overview.md) — 导航栏、Agent 侧边栏（置顶/隐藏 Agent）和 Power-ups 菜单
- [Claude Code](./03-scenario-claude-code.md) — 主力场景：页面结构、按槽位的规则、Profile、Auto Config
- [Cursor](./04-scenario-cursor.md) — Cursor 代理，附公网可达性说明（默认隐藏）
- [Codex](./04-scenario-codex.md) — OpenAI Codex CLI 代理：Auto Config、多模型 New Model、按模型自动压缩
- [其他编程 Agent](./04-scenario-coding-agents.md) — OpenCode、Pi、WorkBuddy、DeepSeek Harness、VS Code、Xcode、Claude Desktop
- [OpenAI / Anthropic SDK 代理与 Decisions](./05-scenario-sdk-proxy.md) — OpenAI 兼容接口、Anthropic 原生接口，以及 Decisions API
- [Custom / Embed](./06-scenario-special.md) — Custom 通用兜底场景（原 OpenClaw）与 Embedding API 代理

### 三、Team

Team 是导航栏中独立的一级入口，默认显示。

- [Team](./07-team.md) — 多团队隔离工作区：各自的路由规则、Team Keys，以及跨团队的 Team Keys 总览页

### 四、Image

Image 同样是导航栏中独立的一级入口，默认显示。

- [Image](./07-image.md) — Playground（生成、编辑、蒙版、草图、参考图、总览）、Profile、Library，以及 Image API 页

### 五、Projects 与 Remote

- [Projects](./22-projects.md) — 在网页里使用 Claude Code：工作文件夹、带协调者和线程的项目、回答提问、IM 页面 *（Beta Power-up）*
- [Remote](./12-remote-control.md) — IM 聊天经由 Projects 与 Claude Code 对话：Bots、Remote Control（访问与 Profile）、IM Notify

### 六、配置主链路

Provider 和凭证管理是所有场景正常工作的前提。

- [凭证管理](./08-credentials.md) — API Key、OAuth、Provider 配置、Quota 列
- [虚拟模型](./09-virtual-models.md) — 内置合成模型，用于演示与测试（一个 Power-up）

> 原「API Tokens」（`/tingly-box-token`，侧边栏曾用名「Sharing」）页面已停用，功能并入 [Team · Team Keys](./07-team.md#team-keys-总览)（`/agent/team/keys`）；旧地址会自动重定向。

### 七、其他主入口

- [用量面板](./11-dashboard.md) — 一条筛选工具栏、状态卡片、Summary / By Request / Activity、Team usage、Quota history
- [提示与帮助](./13-help.md) — 桌面快捷方式、Provider 目录、路由/Tier/Team 引导，集中在一个手风琴页面
- [Prompt 管理](./14-prompt-management.md) — 用户录制与 Skills *（Power-up）*
- [防护栏（Guardrails）](./15-guardrails.md) — 策略导入/导出、规则管理、Secrets、历史审计 *（Power-up）*
- [MCP 与工具](./16-mcp-tools.md) — 接入 MCP 工具服务、通过 MCP Profile 把工具交给 Agent，以及由网关执行的 Server Tools
- [桌面应用与托盘面板](./23-desktop-app.md) — 桌面窗口、托盘 Hub 面板和 macOS 菜单栏

### 八、系统设置

- [系统设置](./17-system-settings.md) — 齿轮与 Preferences 菜单、语言、主题、代理、版本/更新、Develop、日志
- [访问控制](./18-access-control.md) — 用户令牌与模型令牌管理

### 九、实验性功能

- [实验性功能](./19-experimental.md) — 可选开关：Skills、Guardrails、MCP、Bench、Projects

### 十、高阶特性

- [路由规则与插件标记](./20-routing-rules.md) — 规则头部、直接路由（Tier/熔断器）、智能路由（SmartOp 条件）、规则插件标记、探测面板
- [模型选择](./21-model-select.md) — 为路由规则指定 Provider 与模型的交互入口

---

## 可选功能

本指南中的一切在所有构建（包括桌面应用）中都可用——不再有版本拆分。部分功能是**可选**的，在 **Power-ups** 菜单（导航栏齿轮 → Preferences → Power-ups）或「实验性功能」页面里打开后才会出现：

- Projects 与 IM（Beta）、Bench（Beta）
- Guardrails、User prompts、Skills（Experimental）
- MCP Tools（开启 MCP 执行；MCP 页面始终在 Power-ups 侧边栏里）
- Team、Image 和 Virtual Models 默认开启，也可以在那里关闭
