# 快速开始

本章带你从全新安装走到一个能工作的 Agent：连接 Provider、让编码工具指向网关，并看到第一条请求到达。

---

## 首次启动

没有单独的引导向导。全新安装会落在某个 **Agent 页面**——你上次所在的页面，首次运行时就是 **Claude Code**——并且侧边栏是**展开**的，让你看到其他 Agent。页面上的 **Quick Start** 卡片负责引导：

![全新安装时的 Quick Start](../images/claude-code-quickstart.png)

1. **Connect AI Provider**：点击 **Connect AI**。Provider 选择器会*就地*打开（不会跳转页面）；添加完 Provider 后卡片直接继续
2. **Select a Model**：**Choose Model** 会滚动到 **Model Rules** 并高亮 **＋ Add model**；在 [Model Select](./21-model-select.md) 对话框里选 Provider 和模型（或 **Skip**）
3. **安装**工具：复制安装命令（Claude Code：`npm install -g @anthropic-ai/claude-code`），或点 **I've installed it**
4. **Auto Config**：一键写入工具的配置，让它连到 Tingly-Box（Claude Code 还会安装状态栏）；**Config** 可先检查再应用

工具发出第一条请求后，页面标题下的状态行会从 `No requests yet · finish the setup below` 变成 `Connected`。同样的卡片也出现在 Codex、OpenCode、Pi、WorkBuddy、DeepSeek 和 VS Code 上——见[场景总览](./02-scenario-overview.md)和 [Claude Code](./03-scenario-claude-code.md#4-quick-start)。

> 旧的行为——把没有 Provider 的安装送去 **Tips & Help** 页面——已取消；`/onboarding` 仍重定向到 Help，Provider 目录在那里依然可用。

---

## 连接你的第一个 Provider

无论是从 Quick Start、Model Rules 工具栏（**Connect AI**）、[凭证管理](./08-credentials.md)页面，还是[提示与帮助](./13-help.md)上的 **Provider Catalog** 卡片进入，都是同一个选择器：

![Connect AI 选择器](../images/connect-ai.png)

顶部是搜索框，下面是：

- **Custom**：**Custom endpoint**（任意 OpenAI/Anthropic 兼容的 Base URL）、**Import**（从文件或剪贴板）和 **Paste & detect**（粘贴 `.env`、curl 命令或 JSON 片段——自动提取 URL 和凭证）
- **OAuth sign-in**：用订阅登录的 Provider（Claude Code、Google Gemini CLI、Antigravity、Codex、Kimi Code、xAI Grok、ZCode、WorkBuddy 等）。点击后直接发起 OAuth 流程——无需 API Key——授权后令牌自动保存
- **Self-hosted**、**Cloud** 和 **API key** Provider，按地区和协议（OpenAI · Anthropic）分组

选择任何非 OAuth 的 Provider 都会打开配置表单——所有字段见[凭证管理 · 添加 Provider](./08-credentials.md#添加-providerconnect-ai-流程)。

![Provider 配置表单](../images/connect-ai-form.png)

---

## 下一步

- [场景总览](./02-scenario-overview.md)——导航栏、Agent 侧边栏和 Power-ups
- [Claude Code](./03-scenario-claude-code.md)——主力场景的完整说明
- [凭证管理](./08-credentials.md)——管理 Provider 并查看其配额
