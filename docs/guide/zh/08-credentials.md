# 凭证管理

路径：`/credentials`

凭证管理页面是 Tingly-Box 的配置主链路核心，所有 Provider 的 API Key 和 OAuth 凭证均在此集中管理。

---

![凭证管理](../images/credentials.png)

## 页面概览

侧边栏中该页面所在分组标签为 **Credential**，下含三个子页面：**Model Key**（本页）、**Sharing**（见 [API Tokens](./10-api-tokens.md)）、**VModel**（Virtual Models 的缩写，见 [虚拟模型](./09-virtual-models.md)）。

页面顶部显示当前凭证总数（如 `Managing 15 credentials`），顶部操作栏包含：

| 按钮 | 功能 |
|------|------|
| **Providers** | 跳转到 Onboarding 页面（浏览全部 Provider） |
| **Connect AI** | 打开统一 Provider 选择器，添加新凭证（与 Onboarding 流程相同） |

> 凭证配置已不再提供页面级的批量导入/导出——请通过 Connect AI 逐个接入 Provider。（Guardrails 的 Protected Credentials 页面仍保留独立的「Import from Credentials」快捷方式，详见 [防护栏](./15-guardrails.md)。）

---

## 凭证类型

### OAuth 表格

展示所有通过 OAuth 授权接入的 Provider（如 Claude Code、Codex、Gemini CLI）：

| 列 | 说明 |
|----|------|
| Status | 启用/禁用开关 |
| Name | Provider 显示名称 |
| API Style | 协议标签（OpenAI / Anthropic） |
| Provider | 底层 Provider 标识 |
| Expires At | Token 过期日期 |
| Proxy | 该凭证单独设置的代理（如有） |
| Actions | 编辑、Quota、Models、更多（⋮） |

有用量/套餐数据的 Provider 行下方会显示实时**配额条**——例如 Claude Code OAuth 显示 *5-Hour Window* 和 *7-Day Window* 用量条及 Extra Usage 状态；Codex 显示 *Current Window* 和 *Weekly* 用量条；Gemini/Kimi/Qwen 则显示各自套餐特有的量表。每条配额条旁还有 **Details** 链接和相对刷新时间（`just now`）。

### API Keys 表格

展示所有通过 API Key 方式接入的 Provider：

| 列 | 说明 |
|----|------|
| Status | 启用/禁用开关 |
| Name | Provider 显示名称 |
| API Style | 协议标签（OpenAI / Anthropic——融合 Provider 显示双标签） |
| API Base URL | 端点地址 |
| API Key | 脱敏显示，附眼睛图标可查看明文 |
| Proxy | 该凭证单独设置的代理（如有） |
| Actions | 编辑、Quota、Models、更多（⋮） |

有配额 API 的 Provider 行下方显示与 OAuth 凭证相同的实时用量条；没有配额 API 的则显示提示文字（如 `quota API not available — see the OpenAI dashboard`）。

---

## 添加 Provider（Connect AI 流程）

点击 **Connect AI** 打开 Provider 选择器。这是接入任何 AI 服务的统一入口，分两步完成：**先选类型，再填配置**。

### 第一步：选择 Provider

![Connect AI 选择器](../images/connect-ai.png)

顶部是搜索框（按名称过滤），下方按接入方式分区展示，每张卡片右上角有彩色标签标明类型：

| 分区 | 说明 | 选中后 |
|------|------|--------|
| **Custom** | `Custom endpoint`（自带任意 Base URL）、`Import`（从文件/剪贴板导入）、`Paste & detect`（粘贴 `.env`、curl 命令或 JSON 片段，Tingly 自动提取 Provider 配置） | 打开空白配置表单 / 导入对话框 / 粘贴识别流程 |
| **OAuth sign-in** | 支持 OAuth 授权的 Provider（Claude Code、Google Gemini CLI、Codex 等） | **直接发起 OAuth 授权**，无需填 API Key |
| **Self-hosted** | 本地自托管服务（如 Ollama），卡片显示 `localhost:端口` | 打开配置表单，Base URL 已预填但**可编辑**（按你的主机/端口调整） |
| **API key providers** | 通过 API Key 接入的云端 Provider，按区域分组（CN / Global），卡片标注协议（OpenAI · Anthropic） | 打开配置表单，名称和 Base URL 已预填 |

> 大多数 Provider 都已内置，只需提供它们各自需要的信息。列表里没有？选 **Custom endpoint** 手动填任意端点。

### 第二步：填写配置表单

![Provider 配置表单](../images/connect-ai-form.png)

选中非 OAuth 的 Provider 后弹出配置表单：

- **Base URL**（必填）：API 端点。预置 Provider 已预填；Custom / Self-hosted 可自由编辑
- **API Key**（必填）：访问令牌；若是本地无鉴权服务，打开 **No API Key Required** 开关即可免填
- **API Style（协议）**：
  - **OpenAI Compatible**（推荐）：大多数端点都兼容 OpenAI 协议，不确定时选它
  - **Anthropic**：原生 Anthropic 协议
  - 两者可同时启用（融合 Provider），让同一凭证同时服务 OpenAI 和 Anthropic 两种入站协议
- **Proxy URL**（可选，展开高级）：为该 Provider 单独走 HTTP 代理
- **User Agent**（可选，展开高级）：自定义请求头

填好后可点 **Test** 验证连通性，再 **Save** 保存。

> **OAuth Provider 例外**：在第一步选中 OAuth 卡片后直接跳转授权页，无需第二步表单，授权完成自动保存 Token。

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
