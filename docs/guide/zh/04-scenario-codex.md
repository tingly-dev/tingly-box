# Codex 场景

路径：`/agent/codex`

Codex 场景将 OpenAI Codex CLI 的 API 请求代理到你配置的 Provider，支持自动配置和灵活的转发规则。

---

![Codex 场景](../images/codex.png)

## 页面结构

页面由以下区域从上到下依次构成：

### 1. Codex 配置卡

展示当前场景的连接信息：
- **Base URL**：Codex CLI 应配置的代理地址（含复制按钮）
- **API Key**：供 CLI 使用的令牌（含复制/显示按钮）
- **Plugins** 行：与 Claude Code 相同的场景级插件下拉菜单（Thinking / Smart Compact / Vision Proxy / Record）——详见 [Claude Code · Plugin 插件开关](./03-scenario-claude-code.md#plugin-插件开关)

### 2. Quick Start 引导步骤

与 Claude Code 相同的 4 步模式（Connect AI Provider → Select a Model → Install Codex → Auto Config），附 **Reset progress** 和 **?**（How routing works）链接——详见 [Claude Code · Quick Start 引导步骤](./03-scenario-claude-code.md#2-quick-start-引导步骤)，该说明适用于所有编程 Agent 场景。

### 3. Model Rules（可折叠）

与 Claude Code 相同的路由图 UI——**Test All** / **Troubleshoot** / **Connect AI** / **New Rule** 工具栏；详见 [Claude Code · Model Rules](./03-scenario-claude-code.md#5-model-rules)。

---

## 配置流程

1. 在 [凭证管理](./08-credentials.md) 添加至少一个 Provider
2. 打开 Codex 场景页，确认 Base URL 和 API Key
3. 安装 Codex CLI（见安装命令）
4. 点击 **Auto Config** 自动写入代理配置，或手动设置：
   - `OPENAI_BASE_URL`：填写 Base URL
   - `OPENAI_API_KEY`：填写 API Key
5. 在终端中使用 Codex CLI

---

## 相关页面

- [Claude Code 场景](./03-scenario-claude-code.md)
- [其他编程 Agent](./05-scenario-coding-agents.md)
- [凭证管理](./08-credentials.md)
