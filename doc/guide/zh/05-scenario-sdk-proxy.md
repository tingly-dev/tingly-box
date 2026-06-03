# OpenAI / Anthropic SDK 代理

本章介绍 OpenAI 兼容接口代理和 Anthropic 原生接口代理两个场景，适用于在代码中直接调用 API 的应用程序。

---

## OpenAI 场景

路径：`/agent/openai`

将任何使用 OpenAI SDK 的应用程序的请求，透明代理到 Tingly-Box 管理的 Provider。

### 使用场景

- 自己开发的 Python/Node.js/Go 应用使用 `openai` 官方 SDK
- 第三方工具配置了 OpenAI 兼容端点（如 LangChain、LlamaIndex 等）
- 需要统一管理多个 OpenAI 兼容 Provider 的访问凭证

### 页面结构

1. **OpenAI 配置卡**：展示代理 Base URL 和 API Key
2. **模型与转发规则**（可折叠）：配置请求路由到哪个 Provider

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

## Zen 模式

| 场景 | Zen 路径 |
|------|---------|
| OpenAI | `/zen/openai` |
| Anthropic | `/zen/anthropic` |

---

## 与凭证管理的关系

这两个场景的转发规则决定了请求最终发往哪个 Provider。若尚未添加 Provider，请先前往 [凭证管理](./08-credentials.md) 完成配置。

---

## 相关页面

- [场景总览](./02-scenario-overview.md)
- [Claude Code 场景](./03-scenario-claude-code.md)
- [凭证管理](./08-credentials.md)
