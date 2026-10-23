# OpenAI / Anthropic SDK 代理

本章介绍 OpenAI 兼容接口代理、Anthropic 原生接口代理和 Decisions API 端点，适用于在代码中直接调用 API 的应用程序。它们都是标准的 Agent 页面（状态行、**Requests / Usage / Log** 按钮、可折叠的连接卡、Plugins 卡、Model Rules——见 [Claude Code · 页面结构](./03-scenario-claude-code.md#页面结构)），只是去掉了客户端相关的部分：没有 Auto Config 或 Setup Guide 按钮，也没有 Quick Start——因为不需要在客户端安装任何东西，只要把 SDK 指向 Base URL 即可。

---

## OpenAI 场景

路径：`/agent/openai`

将任何使用 OpenAI SDK 的应用程序的请求，透明代理到 Tingly-Box 管理的 Provider。

### 使用场景

- 自己开发的 Python/Node.js/Go 应用使用 `openai` 官方 SDK
- 第三方工具配置了 OpenAI 兼容端点（如 LangChain、LlamaIndex 等）
- 需要统一管理多个 OpenAI 兼容 Provider 的访问凭证

### 页面结构

1. **连接卡**：代理 Base URL（`/tingly/openai`）和 API Key
2. **Model Rules**（可折叠）：每个请求模型名路由到哪个 Provider——**New Model** 新增规则（也可一次选多个模型，每个模型一条规则），**Connect AI** 添加 Provider

### 接入方式

在你的应用程序中，将 OpenAI SDK 的 `baseURL` 指向 Tingly-Box 提供的代理地址：

```python
from openai import OpenAI
client = OpenAI(
    base_url="<tingly-box-base-url>",
    api_key="<tingly-box-api-key>",
)
```

```javascript
import OpenAI from 'openai';
const client = new OpenAI({
  baseURL: '<tingly-box-base-url>',
  apiKey: '<tingly-box-api-key>',
});
```

---

## Anthropic 场景

路径：`/agent/anthropic`

将使用 Anthropic 官方 SDK 的应用程序请求，代理到 Tingly-Box 管理的 Provider（包括非 Anthropic 的 Provider，只要支持 Anthropic 协议即可）。

### 使用场景

- 应用程序使用 `anthropic` 官方 SDK 直接调用 Claude API
- 需要在不改变代码的情况下切换底层 Provider
- 需要统计和审计 Anthropic API 调用

### 接入方式

将 Anthropic SDK 的 `base_url` 指向 Tingly-Box 提供的代理地址：

```python
import anthropic
client = anthropic.Anthropic(
    base_url="<tingly-box-base-url>",
    api_key="<tingly-box-api-key>",
)
```

---

## Decisions API

路径：`/agent/decisions`

Decisions API 会从**你定义的选项里挑出一个答案**（OpenAI 的 `/decisions` 端点；托管同一结构的厂商——TypeSafe、OpenRouter——也可通过这个场景访问）。请求体原样透传给 Provider；只有 `model` 会按你的规则路由（并以调用方请求的模型名回显）。同一个端点在 `openai` 和 `team` 场景上也可用；本页是一个专用入口，类似 Embedding。

- 连接卡：Base URL（`/tingly/decisions/v1`）和 API Key；Plugins 卡只显示 **Record**
- **Decisions Model Rules**：请求模型名到 Provider 的映射，与其他 Agent 页面一致
- **Setup Guide**（页头，实心）打开 **Decisions API Quick Start**——提供 **Python**、**TypeScript** 和 **curl** 三种可直接复制的示例，已填好你的 Base URL、API key 和第一条规则的模型：

![Decisions API Quick Start](../images/decisions-guide.png)

```bash
curl http://localhost:3000/tingly/decisions/v1/decisions \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"model": "my-decision-model", "state": "My invoice shows a charge I do not recognise.",
       "questions": {"team": {"type": "choice", "instructions": "Which team should handle this ticket?",
                               "criteria": {"billing": "Charges, invoices, refunds", "technical": "Bugs, errors, outages"}}}}'
```

![Decisions 页面](../images/decisions-scenario.png)

---

## 与凭证管理的关系

这两个场景的转发规则决定了请求最终发往哪个 Provider。若尚未添加 Provider，请先前往 [凭证管理](./08-credentials.md) 完成配置。

---

## 相关页面

- [场景总览](./02-scenario-overview.md)
- [Claude Code 场景](./03-scenario-claude-code.md)
- [凭证管理](./08-credentials.md)
