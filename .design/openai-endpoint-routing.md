# OpenAI Endpoint Routing 设计

> 适用对象：tingly-box 后端贡献者，特别是改 `internal/server/endpoint_resolution.go` 或 provider/template 类型时。
> 本文档描述「客户端发请求 → gateway 选 OpenAI 上游 endpoint」的最终设计。

---

## 1. 问题域

OpenAI 兼容生态里有两种 endpoint 形态：

| Endpoint | 提供方 |
|---|---|
| `/v1/chat/completions` | 几乎所有 OpenAI-compat 厂商（Qwen、Deepseek、Mistral、GLM、MiniMax、xAI、本地 vLLM/llama.cpp 等）+ OpenAI 官方 |
| `/v1/responses` | 仅 OpenAI 官方（gpt-5、o-series 等）+ Codex |

Provider 实际能力组合**只有三种**：

| 类型 | 例子 | Chat | Responses |
|---|---|:---:|:---:|
| Chat-only | Qwen / Deepseek / Mistral / 本地模型 / 绝大多数厂商 | ✅ | ❌ |
| Responses-only | Codex | ❌ | ✅ |
| Both | OpenAI 官方 | ✅ | ✅ |

Gateway 收到客户端的 request 时（无论入站协议是 OpenAI Chat / OpenAI Responses / Anthropic Messages 经转换后等价的 OpenAI 形态），**必须知道**：上游用哪一个？

---

## 2. 历史与教训

### 2.1 Adaptive 时代（已移除，PR #976）

`AdaptiveProbe` 在 cold-start 时同时探两个 endpoint，缓存结果，运行时按缓存路由。

代价：
- 首次请求阻塞 10s
- 每次 probe 烧真实 token
- 单次失败即标"不可用"，不可重试
- 永远不可能 100% 准确（速率限制、临时 5xx 都会污染缓存）
- 整体黑盒，故障难诊断

最早就引入了 deprecated 注释（"use per-request routing decisions instead"）。

### 2.2 失败的过渡：负向声明（dead-end）

PR #976 中间一版试过这个补丁：

```go
// 错误设计
type Provider struct {
    ResponsesOnly bool  // 标记 Codex
    ChatOnly bool       // 补丁：标记常见 Chat-only 厂商
}
```

`ChatOnly` 是补丁，因为默认行为还是错的——**默认 mirror 入站**，本质上沿袭了 Adaptive 的乐观假设「上游也支持客户端发的协议」。这导致：
- Codex 客户端发 `/v1/responses` → 路由到任意非-Codex 上游 → mirror 到 Responses → 404
- 用户必须显式 set 一个否定标志才能避免 bug

根本错误：**正确语义应该是 positive declaration**——provider 显式声明"我支持 Responses"，没声明就默认 Chat。

### 2.3 当前设计：单一 enum + Chat 默认

```go
type OpenAIEndpointMode string

const (
    EndpointModeUnknown   OpenAIEndpointMode = ""           // 未声明，按 Chat 处理
    EndpointModeChat      OpenAIEndpointMode = "chat"       // 绝大多数厂商
    EndpointModeResponses OpenAIEndpointMode = "responses"  // Codex
    EndpointModeBoth      OpenAIEndpointMode = "both"       // OpenAI 官方
)
```

三种状态 1:1 映射上文表格的三类 provider。

---

## 3. 关注点分层（partition）

Endpoint 路由相关的状态/决策被刻意拆成两层，每层承担一个 well-defined 的责任。**不要**把任何一层的事情塞到另一层去做——这是 Adaptive 时代失败的根因（把"结构事实"埋进了运行时缓存）。

```
┌─────────────────────────────────────────────────────────────┐
│  Layer 2  Rule flag (extension)  —  per-rule customization   │
│           openai_endpoint_override = auto/chat/responses    │
│           其他 rule flag：cursor_compat、skip_usage、…       │
├─────────────────────────────────────────────────────────────┤
│  Layer 1  Provider mode  —  structural fact                  │
│           Provider.OpenAIEndpointMode = chat/responses/both  │
│           来自 template 快照 / OAuth 实例化（用户不可编辑）  │
└─────────────────────────────────────────────────────────────┘
```

「Request-shape guard」**不是**第三层。Responses-only 字段（`previous_response_id`、`include`、`reasoning` 等）在路由到 Chat 上游时由转换函数 `ConvertOpenAIResponsesToChat` 静默丢弃，与 Anthropic→Chat 降级时静默丢弃 thinking blocks / vision details 的行为完全一致。用户在 provider mode 上的选择已经承担了这个 trade-off，gateway 不再做二次裁决。

### Layer 1: Provider 模式 —— 结构性事实

回答的问题：**这个 provider 实际上能听懂什么 endpoint？**

- 来源：template 预设（实例化时快照）或 OAuth issuer 推断（Codex）
- **不是用户可编辑字段**——它描述的是上游 API 的客观能力，不是用户偏好
- 单一来源、单一字段（`Provider.OpenAIEndpointMode`），无 ambiguity

判定属于这一层的标志：**改变这个值需要 provider API 本身发生改变**（厂商升级 API、用户换 provider）。

### Layer 2: Rule flag —— per-rule 定制（extension）

回答的问题：**这条 rule 想要什么特殊行为？**

- 实现：`typ.RuleFlags` 结构 + `internal/typ/flag_registry.go` 的 catalog
- 当前关于 endpoint 的 flag：`openai_endpoint_override`（auto / chat / responses）
- 其他 rule flag 与 endpoint 无关（`cursor_compat`、`skip_usage`、`use_max_completion_tokens` …）但**走相同的 extension 机制**

判定属于这一层的标志：**只对某条 rule / 某类客户端 / 某种调试场景有意义**，做成 provider 字段会污染默认，做成 scenario flag 又过于粗粒度。详见 `.design/rule-flags.md`。

### 两层冲突时的优先级

| 场景 | 谁赢 | 理由 |
|---|---|---|
| Rule flag 指定 `chat`，Provider mode 是 `responses` | **Rule flag 赢** | override 是 per-rule 显式选择，用于调试或特殊客户端适配 |
| Rule flag 指定 `responses`，Provider mode 是 `chat` / 未声明 | **Rule flag 赢** | 同上；调用方显式接受上游不支持时可能产生的错误 |
| Provider mode 是 `both`、入站 Chat | mirror（Chat） | 入站协议是默认 tie-breaker |
| Provider mode 是 `both`、入站 Responses | mirror（Responses） | 同上 |

具体决策表见 §4.2。

### 何时应当新增一个 rule flag

如果未来出现某种"针对部分 rule 的 endpoint-routing 微调"需求（例如：某条 rule 强制走 streaming-only 通道、某条 rule 启用 Responses 的特殊参数），**首选** 把它做成新 rule flag：

1. 在 `typ.RuleFlags` 加字段
2. 在 `RuleFlagRegistry()` 注册 metadata（label / description / category）
3. 在相应的 transform / handler 消费

**不应当** 把这种需求塞进 `OpenAIEndpointMode`——那是 structural layer，扩张它会复活 Adaptive 时代的混乱。

---

## 4. Resolver 行为

`ResolveOpenAIEndpoint(provider, ruleFlags, incoming) → (protocol.APIType, error)` 定义在 `internal/server/endpoint_resolution.go`。**纯函数**：不读 Server 状态、不发 I/O。

### 4.1 precedence（高 → 低）

1. **Rule flag `OpenAIEndpointOverride`**（Layer 2，用户每条 rule 可设）
   - `""` / `"auto"` / 未知值 → 当作未设置
   - `"chat"` 或 `"responses"` → 显式 override
2. **Provider 声明 `OpenAIEndpointMode`**（Layer 1，来自 template 快照 / OAuth 实例化）

Provider 未声明或显式声明 `EndpointModeUnknown` 时按 Chat 处理。Rule override 优先于 provider mode；`OpenAIEndpointOverride` 与 provider mode 冲突时不再降级为 provider mode，也不记录 ignored warn。

### 4.2 决策表

| Override | Provider mode | 结果 | 备注 |
|---|---|---|---|
| 无 | `""` / unknown | **Chat** | 入站是 Responses 也降级 |
| 无 | `"chat"` | Chat | 入站是 Responses 也降级 |
| 无 | `"responses"` | Responses | Codex |
| 无 | `"both"` (chat 入站) | Chat | mirror |
| 无 | `"both"` (responses 入站) | Responses | mirror |
| `=chat` | 任意 | Chat | override 生效 |
| `=responses` | 任意 | Responses | override 生效 |

### 4.3 为什么默认是 Chat

- 生态现实：绝大多数 OpenAI-compat 厂商只实现 `/chat/completions`
- Mirror 入站等于继续相信「上游也支持客户端发的协议」——这就是 Adaptive 时代的乐观假设
- Chat 默认 + 显式 opt-in Responses 是 safe failure mode：未知 provider 永远走通用 endpoint，不会 404

---

## 5. Codex 的处理

Codex 是 OAuth-only 接入路径（Web `oauth/handler.go` 和 CLI `command/oauth.go`），实例化时通过 `ai.OpenAIEndpointModeForIssuer(issuer)` 直接填入 Provider 结构体——目前该 helper 只对 `IssuerCodex` 返回 `EndpointModeResponses`，其他 issuer 返回 `EndpointModeUnknown`，由 resolver 按 Chat 处理。两条 OAuth 路径共用同一 mapping。

无需用户配置。OAuth 完成即正确。

存量 Codex provider（PR #976 之前已 OAuth 完成的）由 `migrate20260518` backfill。Idempotent。

`ai.Provider.IsCodexProvider()` 方法**保留**——它仍被 client、UA pin、system message 注入等非路由代码消费。本文档讨论的 endpoint mode 只与路由相关。

---

## 6. Template 与 Provider 字段

Template 是用户实例化 provider 的预设入口。Template 里的 `openai_endpoint_mode` 在实例化时快照到 Provider 同名字段。

`providers.json` 现状：
| Template | mode |
|---|---|
| `openai-com` | `"both"` |
| `codex` | `"responses"` |
| 其他（Qwen / Deepseek / GLM / ...）| 未设（= `""` / unknown，resolver 按 Chat 处理）|

用户**不在 provider 编辑 UI 里改 mode**——它从 template 来。后续若发现某个 Chat-only template 应该是 `"both"`，在 `providers.json` 修，新装机生效；存量 provider 需手动 edit config.json 或重建 provider。

---

## 7. 客户端协议 → 上游 endpoint 全链路

完整端到端转换矩阵（仅 OpenAI-API-style provider；Anthropic / Google provider 走各自原生路径）：

| 客户端入站 | Provider mode | 上游 | 入站→上游 转换 | 上游→客户端 转换 |
|---|---|---|---|---|
| OpenAI Chat | chat | Chat | passthrough | passthrough |
| OpenAI Chat | responses | Responses | `ConvertOpenAIChatToResponses`（建中）* | `buildChatPayloadFromResponses` |
| OpenAI Responses | chat | Chat | `ConvertOpenAIResponsesToChat` | `buildResponsesPayloadFromChat` |
| OpenAI Responses | responses | Responses | passthrough | passthrough |
| OpenAI Responses | both（mirror）| Responses | passthrough | passthrough |
| Anthropic Messages | chat | Chat | `ConvertAnthropicToOpenAIRequest` | `ConvertOpenAIToAnthropicResponse` |
| Anthropic Messages | responses | Responses | Anthropic→Responses 直转 | `streamResponsesToAnthropic*` |
| Anthropic Messages | both（mirror）| Responses | （同上）| （同上）|

(*) Chat-in / Responses-out 路径在 `protocol_dispatch.go:streamOpenAIChatToResponses` / `nonstreamOpenAIChatToResponses`。注意：当前 dispatch 里这两个函数被命名为 `streamResponsesToChat` / `nonstreamResponsesToChat`，方向写反了，pre-existing issue。

---

## 8. 关键文件

- `ai/provider.go` —— `OpenAIEndpointMode` 类型 + 常量 + `Provider.OpenAIEndpointMode` 字段
- `internal/data/provider_template.go` —— `ProviderTemplate.OpenAIEndpointMode`（plain string）
- `internal/data/providers.json` —— 出厂 template 的 mode 声明
- `internal/server/endpoint_resolution.go` —— `ResolveOpenAIEndpoint` 纯函数
- `internal/server/endpoint_override.go` —— `EndpointOverride` 枚举与 `ParseEndpointOverride`
- `internal/server/openai_responses.go` —— Responses 入站的路由调用点
- `internal/server/module/oauth/handler.go`、`internal/command/oauth.go` —— Codex OAuth 实例化打 mode
- `internal/server/config/migration_codex_endpoint_mode.go` —— 存量 Codex backfill 迁移

---

## 9. 升级与兼容性

PR #976 引入此设计。涉及行为变更的两个点：

1. **默认从 mirror 变 Chat**：手搓的 OpenAI-proper provider（没用 `openai-com` template）若依赖 `/v1/responses` 直通上游，需手动加 `"openai_endpoint_mode": "both"` 到 config.json。Migration 不能自动处理这种情况（provider 没有 template_id 痕迹），文档化警告即可。
2. **Codex 存量**：`migrate20260518` 自动 backfill，无感知。

后续若新增 provider 类型，原则不变：默认 Chat，需要 Responses 就显式声明。

---

## 10. 不在本文档范围

- Anthropic / Google provider 的路由（走各自原生 endpoint，不进 OpenAI resolver）
- Smart routing / load balance 选哪个 service（在 endpoint 选择之前）
- vmodel loopback（独立处理）
- `IsCodexProvider()` 在 client 层的用法（UA pin、system message 注入等 quirk）
