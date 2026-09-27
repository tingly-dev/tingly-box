# Custom / Embed

本章介绍两个通用兜底场景：Custom 通用兜底场景（原「OpenClaw」）与 Embedding API 代理。

> Team 和 Image 曾经也在本章介绍。它们现在都已升级为 Activity Bar 中的独立一级入口，各自拥有专属页面——详见 [Team](./07-team.md) 与 [Image](./07-image.md)。

---

## Custom

路径：`/agent/custom`

![Custom 场景](../images/custom-scenario.png)

Custom 场景（侧边栏曾用名「OpenClaw」/「Claw Agent」）是一个通用兜底场景：可自定义请求模型名，为任意自定义 Agent 框架提供标准化的 API 端点。默认在侧边栏中隐藏（如需使用可在 [场景总览](./02-scenario-overview.md) 中取消隐藏）。

### 页面结构

1. **Provider 配置卡**：
   - **Base URL**：Agent 接口地址（含复制按钮）
   - **API Key**：访问凭证（含复制按钮）
2. **Model Rules**（可折叠）：配置 Agent 请求的路由规则

### 使用场景

- 自定义 Agent 框架需要统一的 API 端点
- 多个 Agent 需要共享同一组 Provider 凭证
- 需要为 Agent 访问配置独立的路由规则

---

## Embed（Embedding API）

路径：`/agent/embed`

代理 Embedding API 请求，适用于文本向量化应用。

### 页面结构

1. **Embed API 配置卡**：展示代理地址和 Key
2. **Embedding 模型与转发规则**（可折叠）：专门针对 Embedding 模型配置路由

### 使用场景

- RAG（检索增强生成）应用的文本向量化
- 语义搜索系统
- 文本相似度计算

### 接入方式

```python
from openai import OpenAI
client = OpenAI(
    base_url="<tingly-box-embed-url>",
    api_key="<tingly-box-api-key>",
)
response = client.embeddings.create(
    model="text-embedding-3-small",
    input="your text here",
)
```

---

## 相关页面

- [场景总览](./02-scenario-overview.md)
- [Team](./07-team.md)
- [Image](./07-image.md)
- [凭证管理](./08-credentials.md)
