# Tingly-Box 用户指南

Tingly-Box 是一个 AI 智能体编排平台，提供 LLM 网关、远程控制和安全防护能力。本指南按功能模块分章节说明 Web UI 的完整使用方法。

---

## 目录

### 一、快速上手
- [初始化与 Provider 接入](./01-getting-started.md)

### 二、Agent 场景

Agent 场景是 Tingly-Box 的核心功能，将各类 AI 编程工具的 API 请求统一代理到你配置的 Provider。

- [场景总览](./02-scenario-overview.md) — 场景导航与可见性管理
- [Claude Code](./03-scenario-claude-code.md) — 主力场景，支持 Profile、统一/分离模型、转发规则
- [Codex](./04-scenario-codex.md) — OpenAI Codex CLI 代理，自动配置支持
- [其他编程 Agent](./04-scenario-coding-agents.md) — OpenCode、VS Code、Xcode、Claude Desktop
- [OpenAI / Anthropic SDK 代理](./05-scenario-sdk-proxy.md) — OpenAI 兼容接口与 Anthropic 原生接口
- [Claw Agent / Embed / ImageGen](./06-scenario-special.md) — OpenClaw、Embedding、图像生成
- [Playground（图像生成测试台）](./07-scenario-playground.md)

### 三、配置主链路

Provider 和凭证管理是所有场景正常工作的前提。

- [凭证管理](./08-credentials.md) — API Key、OAuth、导入导出、Provider 配置
- [虚拟模型](./09-virtual-models.md) — 内置合成模型，用于演示与测试
- [API Tokens](./10-api-tokens.md) — 管理外部客户端访问令牌

### 四、其他主入口

- [用量看板](./11-dashboard.md) — 请求统计、Token 消耗、缓存命中率
- [远程控制](./12-remote-control.md) — 通过 IM 平台（微信、Telegram、飞书等）远程操控 Claude Code
- [Remote Coder](./13-remote-coder.md) — Web Chat 与会话管理
- [Prompt 管理](./14-prompt-management.md) — 用户录制、Skill、Command（Full Edition）
- [防护栏（Guardrails）](./15-guardrails.md) — 策略导入/导出、规则管理、历史审计
- [MCP 与工具](./16-mcp-tools.md) — MCP 服务器注册与本地模式

### 五、系统设置

- [系统设置](./17-system-settings.md) — 代理、语言、主题、版本信息、日志
- [访问控制](./18-access-control.md) — 用户令牌与模型令牌管理

### 六、实验性功能

- [实验性功能](./19-experimental.md) — Skills IDE、Guardrails、MCP 开关

### 七、高阶特性

- [路由规则与扩展标记](./20-routing-rules.md) — 直接路由（Tier/熔断器）、智能路由（SmartOp 条件）、规则扩展标记
- [模型选择](./21-model-select.md) — 为路由规则指定 Provider 与模型的交互入口

---

## 版本说明

部分功能仅在 **Full Edition** 中提供：
- Prompt 管理（用户录制、Skills）
- 远程控制（IM Bot）
- Remote Coder

部分功能需在「实验性功能」页面手动开启后方可在侧边栏看到：
- Guardrails（防护栏）
- MCP 工具
