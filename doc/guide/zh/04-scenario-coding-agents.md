# 其他编程 Agent 场景

本章介绍除 Claude Code 之外的编程工具代理场景，包括 Codex、OpenCode、VS Code、Xcode 和 Claude Desktop。这些场景的配置结构与 Claude Code 类似。

---

## Codex

路径：`/agent/codex`

将 OpenAI Codex CLI 的 API 请求代理到你配置的 Provider。

### 页面结构

1. **Codex 配置卡**：展示代理 Base URL 和 API Key
2. **Agent 设置卡**：
   - 安装命令（一键复制）
   - **Auto Config** 按钮：自动将代理配置写入 Codex 配置文件
3. **模型与转发规则**（可折叠）：管理 Codex 场景的路由规则

### 配置流程

1. 安装 Codex CLI（见安装命令）
2. 点击 **Config** 获取代理地址
3. 点击 **Auto Config** 自动写入配置，或手动设置 `OPENAI_BASE_URL` 和 `OPENAI_API_KEY`
4. 在终端中使用 Codex CLI

---

## OpenCode

路径：`/agent/opencode`

代理 OpenCode CLI 的请求。页面结构与 Codex 完全一致：

- 配置卡 + 代理地址/Key
- Agent 设置 + 安装引导
- 转发规则管理

---

## VS Code

路径：`/agent/vscode`

代理 VS Code AI 扩展（如 GitHub Copilot Chat、Continue 等）的 API 请求。

### 说明

VS Code 扩展通常通过 `baseURL` 环境变量或扩展设置指定 API 端点，将其指向 Tingly-Box 提供的代理地址即可。

---

## Xcode

路径：`/agent/xcode`

代理 Apple Xcode AI 功能（Xcode Intelligence）的 API 请求。配置方式与 VS Code 类似，将 API 端点指向 Tingly-Box 提供的代理地址。

---

## Claude Desktop

路径：`/agent/claude_desktop`

代理 Claude 桌面客户端（Desktop App）的 API 请求。

### 页面结构

1. **Claude Desktop 配置卡**：展示代理地址和 API Key
2. **Config 模态框**：提供完整的 `claude_desktop_config.json` 配置片段，可一键复制并粘贴到 Claude Desktop 的配置文件中
3. **模型与转发规则**（可折叠）

### 配置流程

1. 点击 **Config** 打开配置模态框
2. 复制 JSON 配置片段
3. 打开 Claude Desktop 设置文件，粘贴配置
4. 重启 Claude Desktop

---

## Zen 模式

以上所有场景均支持 Zen 模式（全屏沉浸视图）：

| 场景 | Zen 路径 |
|------|---------|
| Codex | `/zen/codex` |
| OpenCode | `/zen/opencode` |
| VS Code | `/zen/vscode` |
| Xcode | `/zen/xcode` |
| Claude Desktop | — |

---

## 场景可见性

在 [场景总览](./02-scenario-overview.md) 页面，可通过卡片底部的开关将不常用的场景从侧边栏隐藏。

---

## 相关页面

- [Claude Code 场景](./03-scenario-claude-code.md)
- [场景总览](./02-scenario-overview.md)
- [凭证管理](./08-credentials.md)
