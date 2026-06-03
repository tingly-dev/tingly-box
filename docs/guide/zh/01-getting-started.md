# 快速上手

本章引导你完成 Tingly-Box 的第一次启动与 Provider 接入，使后续所有 Agent 场景可用。

---

## 初次启动

首次访问 Tingly-Box Web UI 时，系统检测到尚无 Provider 配置，会自动跳转到 **Onboarding（初始化向导）** 页面，路径为 `/onboarding`。

---

## Onboarding 页面

页面标题：**Welcome to Tingly Box**

![Onboarding 页面](../images/onboarding.png)

提供两种方式添加第一个 AI Provider：

### 方式一：浏览并选择 Provider

切换到 **Browse providers** 标签页，Provider 分两类展示：

**Custom（自定义）**
- **Custom endpoint**：手动填写任意 OpenAI/Anthropic 兼容的 API 端点

**OAuth sign-in**
- 列出支持 OAuth 授权的 Provider（如 Claude Code、Google Gemini CLI、Codex 等）
- 点击后直接发起 OAuth 授权流程，无需手动输入 API Key

向下滚动可看到更多通过 API Key 接入的 Provider（Anthropic、OpenAI、DeepSeek 等），按协议风格（OpenAI / Anthropic）分组展示。

点击目标 Provider 后：
- **OAuth Provider**：自动弹出 OAuth 授权对话框，完成授权即保存
- **API Key Provider**：弹出配置表单，填写：
  - **Name**：Provider 显示名称
  - **API Base**：API 端点（通常已预填）
  - **API Style**：`openai` 或 `anthropic`
  - **Token**：API Key
  - **Proxy URL**（可选）：HTTP/HTTPS 代理地址

对于支持 OAuth 的 Provider（如 Claude.ai），系统会自动发起 OAuth 授权流程。

### 方式二：粘贴配置自动识别

切换到 **Paste & detect** 标签页：

1. 将 Provider 配置片段（JSON 或 YAML）粘贴到输入区
2. 系统自动解析并识别 Provider 类型和凭证信息
3. 确认后保存

### 完成 Onboarding

成功添加 Provider 后，弹出成功对话框，可选择：
- **Go to Agents** — 前往场景总览页，开始使用
- **Stay Here** — 继续添加更多 Provider

---

## 已有环境：从凭证页添加 Provider

如果已完成初始化，需要添加新 Provider，请访问 [凭证管理](./08-credentials.md) 页面（`/credentials`），点击 **Connect AI** 按钮，流程与 Onboarding 相同。

---

## 下一步

- 进入 [场景总览](./02-scenario-overview.md) 查看所有可用 Agent
- 查看 [Claude Code 配置](./03-scenario-claude-code.md) 开始主力场景
