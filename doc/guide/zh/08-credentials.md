# 凭证管理

路径：`/credentials`

凭证管理页面是 Tingly-Box 的配置主链路核心，所有 Provider 的 API Key 和 OAuth 凭证均在此集中管理。

---

![凭证管理](../images/credentials.png)

## 页面概览

页面顶部显示当前凭证总数（如 `Managing 5 credentials`），顶部操作栏包含：

| 按钮 | 功能 |
|------|------|
| **Connect AI** | 打开统一 Provider 选择器，添加新凭证（与 Onboarding 流程相同） |
| **Import** | 批量导入 Provider 配置（JSON/YAML 格式） |
| **Providers** | 跳转到 Onboarding 页面（浏览全部 Provider） |

---

## 凭证类型

### API Keys 表格

展示所有通过 API Key 方式接入的 Provider：

| 列 | 说明 |
|----|------|
| Provider | Provider 名称与图标 |
| Base URL | API 端点地址 |
| Token | API Key（脱敏显示） |
| Status | 启用/禁用状态 |
| Quota | 已知配额信息（可点击刷新） |
| Actions | 编辑、删除、启用/禁用 |

### OAuth 表格

展示所有通过 OAuth 授权接入的 Provider（如 Claude.ai）：

| 列 | 说明 |
|----|------|
| Provider | Provider 名称 |
| Status | 授权状态 |
| Expiry | Token 过期时间 |
| Actions | 刷新 Token、编辑、删除、启用/禁用 |

---

## 添加 Provider

点击 **Connect AI** 打开 Provider 选择对话框：

1. 搜索或浏览 Provider 列表
2. 选择目标 Provider（如 Anthropic、OpenAI、DeepSeek、本地 Ollama 等）
3. 填写配置表单：
   - **Name**：显示名称（可自定义）
   - **API Base**：API 端点（已预填，可修改）
   - **API Style**：`openai` 或 `anthropic`
   - **Token**：API Key
   - **Proxy URL**（可选）：为该 Provider 单独设置 HTTP 代理
   - **User Agent**（可选）：自定义请求头
4. 确认保存

对于 OAuth Provider，步骤 3 会自动发起授权流程，完成后自动保存 Token。

---

## 批量导入

点击 **Import** 按钮：

1. 选择文件（JSON 或 YAML 格式）或直接粘贴配置内容
2. 支持的格式示例：
   ```yaml
   providers:
     - name: "My OpenAI"
       api_base: "https://api.openai.com/v1"
       api_style: "openai"
       token: "sk-..."
   ```
3. 点击 **Import** 确认导入
4. 如有重复 Provider，系统提示是否强制覆盖（Force Add）

---

## 编辑 Provider

点击 Provider 行右侧的编辑图标，打开编辑表单，可修改：
- 名称
- API Base URL
- API Key/Token
- 代理设置
- 启用/禁用状态

---

## 启用 / 禁用 Provider

每个 Provider 行都有一个开关，用于快速启用或禁用。禁用的 Provider 不会接受新的路由请求，但配置保留。

---

## 相关页面

- [虚拟模型](./09-virtual-models.md)
- [API Tokens](./10-api-tokens.md)
- [快速上手](./01-getting-started.md)
