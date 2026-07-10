# Rule Flags 设计与实操

> 适用对象：tingly-box 后端 / 前端贡献者。
> 本文档描述**当前**的 rule flag 系统的最终设计与实操。

---

## 1. 为什么需要 rule 级 flag

三层语义：

| 维度 | 粒度 | 例子 |
|------|------|------|
| Provider | provider 实例 | `user_agent`、`api_base`、`proxy_url`、`timeout` |
| Scenario flags | scenario | `skip_usage`（场景级默认）、`smart_compact`、`recording_v2` 等 |
| **Rule flags** | **单条 rule** | **`cursor_compat`、`skip_usage`、`use_max_completion_tokens`、`use_max_tokens`、`custom_user_agent` …** |

很多行为只对**某类客户端 / 某类模型 / 某种调试目的**有意义。塞到 Provider
配置里会污染默认；做成 Scenario flag 又过于粗粒度。Rule flag 是这类
"局部、可选、可叠加开关"的归宿。

设计原则：

1. **可发现**：UI 列出所有可选 flag 及其语义。
2. **可叠加**：同一条 rule 可同时启用多个 flag。
3. **可扩展**：新 flag 只在**一处**注册元数据（`flag_registry.go`），后端
   行为代码 + 前端 UI 不应硬编码 flag 列表。
4. **不污染默认**：未启用时必须是 zero-cost no-op。

---

## 2. 架构图

```
              ┌───────────────────────────────────────────────────┐
              │                    Frontend                        │
              │  FlagCatalogDialog  ◄── GET /rule/flags/registry  │
              │       │                                            │
              │       ▼ Registry-driven (no per-flag switch/case) │
              │  RulePluginsCard  ─── POST /rule/:uuid (flags)     │
              └─────────────────────┬─────────────────────────────┘
                                    │
              ┌─────────────────────▼─────────────────────────────┐
              │                    Backend                          │
              │                                                     │
              │  internal/typ/flag_registry.go                      │
              │      RuleFlagRegistry() []FlagSpec  ◄── 唯一可信源 │
              │                                                     │
              │  internal/typ/type.go                               │
              │      Rule.Flags = RuleFlags{ ...typed fields... }   │
              │                                                     │
              │  Persisted as JSON column on the rule row.          │
              └─────────────────────┬───────────────────────────────┘
                                    │ rule resolved at request time
                                    ▼
   ┌───────────────────────────────────────────────────────────────┐
   │  inbound handler (openai.go / openai_responses.go /            │
   │                   anthropic_v1.go / anthropic_beta.go)         │
   │                                                                │
   │   flags := resolveRuleFlags(c, rule)                           │
   │   ├─ WithCustomUserAgent(ctx, flags.CustomUserAgent)           │  Type 2
   │   ├─ reqCtx.Extra["skip_usage"] = flags.SkipUsage              │  Type 3
   │   ├─ preBase   := rulePreBaseTransforms(flags)                 │  Type 1b-pre
   │   │   → [transform.OpenAICursorCompatTransform{}]              │
   │   └─ preVendor := rulePreVendorTransforms(flags)              │  Type 1b-post
   │       → [transform.OpenAIMaxTokensRewriteTransform{...}]       │
   └───────────────────────────┬───────────────────────────────────┘
                               │
                               ▼
   ┌───────────────────────────────────────────────────────────────┐
   │   transformXxxx(..., preBase, preVendor...) :                  │
   │     chain := BuildTransformChain(..., preBase, preVendor)      │
   │     chain.Execute(ctx)                                         │
   │                                                                │
   │   preBase → Base → MCP → Consistency → preVendor → Vendor → ▸rec│
   │   └ preBase slot                       └ preVendor slot        │
   │                              Vendor + 录制 = 不可逾越的尾段     │
   └───────────────────────────┬───────────────────────────────────┘
                               ▼
                       upstream provider
```

两个动态插入位置本质都是"在某步之前"：**preBase**（Base 之前，看见入站形态）
与 **preVendor**（Vendor 之前，看见目标形态）。

**不变式：除录制外，没有任何阶段在 `Vendor` 之后运行。** `Vendor` 直面
provider、做最终且不可逆的改写（model alias、metadata、billing header、
DeepSeek thinking patch 等），必须是最后一个 mutation；因此 rule 的 preVendor
transforms 装在 `Consistency` 之后、`Vendor` **之前**。这同时修掉了一个隐患：
StagePost 录制现在落在 `Vendor` 之后，抓到的是真正发出的请求（此前这些 transform
跑在录制之后，录到的"最终请求"与实际出站不一致）。

---

## 3. Flag Registry — 唯一可信源

```go
// internal/typ/flag_registry.go
type FlagSpec struct {
    Key             string        // 与 RuleFlags 上的 json tag 完全对应
    Label           string        // UI 展示用人话
    Description     string        // hover 详细说明
    Type            FlagValueType // bool | string | enum | int | service_ref
    Category        FlagCategory  // compatibility | request | response | routing | app
    Placeholder     string        // string 类型的输入框 hint
    Options         []FlagOption  // enum 类型的可选值，按显示顺序
    Suggestions     []string      // string 类型的快选建议（如 DefaultUserAgents()）
    Shared          bool          // 是否同时存在 scenario 和 rule 级
    InheritanceMode string        // shared flag 的继承语义："or" | "override"
}

type FlagOption struct {
    Value string // 持久化到 RuleFlags 的字符串
    Label string // UI 展示
}

func RuleFlagRegistry() []FlagSpec { … }
```

**约束**（`flag_registry_test.go` 强制）：

- 每个 `FlagSpec.Key` **必须**对应 `RuleFlags` struct 上的某个 json
  tag。这条测试挡住"加了 spec 忘了加字段"或反过来。
- `Label` / `Description` 非空。
- `Type` 必须在已知枚举内（`bool`、`string`、`enum`、`int`、`service_ref`）。
- `enum` 类型必须声明 ≥ 2 个 `Options`，每个 Option 的 `Value` / `Label`
  非空且不重复。首个 Option 视为默认值（UI 显示为"未启用"状态）。
- `Shared == true` 的 flag **必须**声明 `InheritanceMode`（`"or"` 或 `"override"`）。
- `Shared == false` 的 flag **不得**声明 `InheritanceMode`。

---

## 4. 当前已注册 flag（14 个）

| Key | Type | 类别 | Shared | 继承 | 作用 | 注入点 |
|-----|------|------|--------|------|------|--------|
| `custom_user_agent` | string | request | **yes** | override | 覆盖出站 User-Agent header。registry 通过 `Suggestions`（`typ.DefaultUserAgents()`）透出几个常见 CLI/agent 的 UA 预设供快选。特殊值 `none`（`typ.UserAgentNone`）= 完全去掉 User-Agent header。| `customUserAgentTransport` + `applyCustomUserAgent(c, flags)` → `WithCustomUserAgent(ctx, ...)`（Type 2）|
| `openai_endpoint_override` | enum (`auto`/`chat`/`responses`) | request | — | — | 强制单条 rule 的 OpenAI 出口走 Chat 或 Responses；与 provider 声明的 `OpenAIEndpointMode` 冲突时 provider 赢（见 `.design/openai-endpoint-routing.md`）| `ParseEndpointOverride` → `ResolveOpenAIEndpoint`（Type 4：路由层决策）|
| `use_max_completion_tokens` | bool | request | — | — | 把 `max_tokens` 字段名重写为 `max_completion_tokens`（OpenAI o1/o3/gpt-5 系列必需） | `transform.OpenAIMaxTokensRewriteTransform` → `ops.ApplyMaxCompletionTokensRewrite`（Type 1b-post）|
| `use_max_tokens` | bool | request | — | — | 反向：把 `max_completion_tokens` 写回旧字段 `max_tokens`（用于拒绝新字段的 provider/模型）| 同上 → `ops.ApplyMaxTokensRewrite`（Type 1b-post）|
| `block_tools` | string (逗号分隔) | request | — | — | 按名字从请求 tool list 中剔除指定工具（发出前），跨 OpenAI Chat / Responses / Anthropic / Google 入站形态生效 | `transform.ToolBlockTransform` → `ops.ApplyToolBlock*`（Type 1b-pre）|
| `skip_usage` | bool | response | **yes** | or | 剥离响应中的 `usage`（流式 + 非流式 + Anthropic 转 OpenAI 路径） | `shouldStripUsage(reqCtx.Extra)`（Type 3）|
| `thinking_effort` | enum (`""`/`off`/`low`/`medium`/`high`/`max`) | reasoning | **yes** | override | 统一控制 extended thinking。映射为 Anthropic budget_tokens（low 1K / medium 5K / high 20K / max 32K）或 OpenAI reasoning_effort。空 = "By Client"（透传客户端参数）。| `ThinkingModeTransform`（Type 1b，server-domain Transform）|
| `vision_proxy_service` | service_ref | vision | — | — | 通过视觉代理模型描述图片，让纯文本下游模型能处理图片输入。rule 级优先于 scenario 级。| VisionProxy 中间件（Type 1b-pre）|
| `session_affinity` | int (seconds) | routing | — | — | 会话亲和 TTL（秒），0=禁用，>0=启用。Pin 会话到服务以提升缓存命中率。Session ID 解析优先级：Anthropic metadata.user_id > X-Tingly-Session-ID header > 客户端 IP。**rule-only**（已从 scenario plugin 移除——无 scenario 级继承）。**built-in CC / Desktop / Codex rule 默认 1800s**（`init.go` 种子 + `migrate20260610` 存量），其余 rule 不设即禁用，可在 Plugins 卡片按 rule 调整。| `ProviderResolver.PostProcess()` → `Config.GetEffectiveAffinity(rule)`（Type 5，仅读 `rule.Flags.SessionAffinity`）|
| `cursor_compat` | bool | app | — | — | Cursor IDE 内容归一化 + stream usage 抑制 | `transform.OpenAICursorCompatTransform` → `ops.ApplyCursorCompatContentNormalization`（Type 1b-pre）|
| `cursor_compat_auto` | bool | app | — | — | 通过请求头识别 Cursor，自动折叠进 `cursor_compat` | `resolveRuleFlags(c, rule)` 在 handler 入口合并 |
| `claude_code_compat` | bool | app | **yes** | or | 归一化 Claude Code 的会话中段 `role == "system"` 消息。Claude Code 在 messages 中写入 system role（非标准扩展，对应 Anthropic `mid-conversation-system` beta）；三方 Anthropic-compatible provider 拒绝该 role。**位置感知**：把每条 system 消息**就地并入相邻 user turn**，方向由左右邻居唯一决定（不是自由选择）——前邻是 user 则**向后并**入它；前邻是 assistant/开头则**向前并**入下一条 user；两侧都不是 user 才独立成一个 user turn。决策需先知道 next 的角色，故实现用 pending 缓冲，到下一条非 system 消息（或数组末尾）才落位。这样既保住位置、又避免产生连续 user 消息（严格 provider 同样会以 "roles must alternate" 拒绝）。**不做 hoist**：messages 里的 system 按 beta 契约必是中段消息（不能是 `messages[0]`），全局 system prompt 已在顶层 `system` 字段；hoist 会把"截至第 N 轮"的指令重排为全局、并击穿 prompt cache。**built-in CC rule 默认开**（`init.go::ccRule` + `newCCProfileRules` 种子 / `migrate20260610` 存量），可在 Plugins 卡片按 rule 关闭以保真原生 Anthropic。| `transform.ClaudeCodeCompatTransform` → `ops.ApplyClaudeCodeCompatRoleRewrite`（Type 1b-pre，仅 Anthropic 入站形态）|
| `clean_header` | bool | app | — | — | 剥离 system messages 中的 x-anthropic-billing-header 块。Claude Code 注入该 header 仅供自家计费，绝不能泄漏给三方 provider。**rule-only**（已从 scenario plugin 移除 —— 不再有 `ScenarioFlags.CleanHeader` / OR 注入）。**built-in CC rule 默认开**（`init.go::ccRule` + `newCCProfileRules` 种子 / `migrate20260610` 存量），可按 rule 关闭以保真原生 Anthropic。**Claude OAuth provider 自动抑制**：OAuth 订阅走原生 Anthropic，其计费后端要消费这个 header，故 `resolveRuleFlagsWithScenario` 在解析末尾对 `provider.IsClaudeCodeProvider()` 命中的请求清掉该 flag。claude_desktop 仍靠 `autoSetCleanHeaderFlag` 在协议转换时自动启用（claude_desktop 未做 rule 级默认）。| `CleanHeaderTransform`（Type 1b-pre，server-domain Transform）|

---

## 5. 五种 flag 注入手法

```
Type 1b-pre  Request body, preBase slot（pre-Base Transform）
         作为 Transform 接口的实现装在 chain 最前（BaseTransform
         之前）运行；type-switch 决定是否对 inbound request 形态生效。
         例：OpenAICursorCompatTransform —— 在 Base 把 OpenAI Chat 转
         成其他形态之前 flatten 富文本内容。聚合点：
         `internal/server/rule_flags.go::rulePreBaseTransforms`。

Type 1b-post Request body, preVendor slot（post-Base Transform，推荐用于跨协议 rewrite）
         作为 Transform 接口的实现装在 `Consistency` 之后、`Vendor`
         **之前**，在 BaseTransform 完成协议转换之后运行；type-switch
         决定是否对目标 request 形态生效。**绝不在 Vendor 之后**——Vendor
         是直面 provider 的最终步骤，不可逾越。例：
         OpenAIMaxTokensRewriteTransform、RuleThinkingTransform。聚合点：
         `internal/server/rule_flags.go::rulePreVendorTransforms`。

Type 2   Per-request context hint
         handler 把 c.Request 的 ctx 替换成带 hint 的；transport /
         round-tripper 等深层组件读 ctx。例：custom_user_agent。

Type 3   Response post-processing
         handler 把 flag 写进 reqCtx.Extra；protocol_dispatch.go 的派
         发分支用 shouldStripUsage 这类统一判定来决定剥/改字段。
         例：skip_usage。

Type 4   Routing-decision input
         handler 把 flag 解析后传给 ResolveOpenAIEndpoint；路由层在
         Transform chain 构造**之前**就决定走哪条出口。Provider 声明
         的 OpenAIEndpointMode 优先于 rule flag 冲突时（详见
         .design/openai-endpoint-routing.md）。不进 ExtraFields、不进
         ctx。例：openai_endpoint_override。

Type 5   Routing behavior (service selection)
         路由层在 PostProcess() 时读取 flag 影响服务选择逻辑。例：
         session_affinity 通过 Config.GetEffectiveAffinity(rule) 解析
         继承链（rule explicit > scenario default > disabled），然后在
         ProviderResolver.PostProcess() 中应用会话亲和策略。
```

任何新 flag 都应归入这几类之一。pre-Base 和 post-Base Transform 都是同一
个 `Transform` 接口的实现，区别只在 chain 中的位置：pre 装在 preBase slot
（BaseTransform 之前，看见 inbound 形态），post 装在 preVendor slot
（Consistency 之后、Vendor 之前，看见目标形态）。聚合点
`rulePreBaseTransforms` / `rulePreVendorTransforms` 决定 flag 装哪边。两个 slot
之外，chain 的骨架（StagePre 录制 → Base → MCP → Consistency → Vendor →
StagePost 录制）由 `BuildTransformChain` 固定，**Vendor + 录制是不可逾越的
尾段**。

---

## 6. op vs Transform —— Type 1b 内部的分层

Type 1b（pre-Base 与 post-Base 同形）涉及两层抽象，**职责必须分开**：

| 层 | 位置 | 职责 | 例子 |
|----|------|------|------|
| **op（操作原语）** | `internal/protocol/transform/ops/` | 纯函数。对某个具体 request 类型做无副作用的字段改写。**不感知链路、不感知 rule、不感知 ctx**。 | `ops.ApplyMaxCompletionTokensRewrite(*openai.ChatCompletionNewParams)`、`ops.ApplyCursorCompatContentNormalization(*openai.ChatCompletionNewParams)` |
| **Transform（链路阶段，协议层）** | `internal/protocol/transform/` | 实现 `Transform` 接口。构造期接受配置；`Apply()` 里 type-switch `ctx.Request`，匹配目标类型时调 op。**仅依赖协议层 / SDK 类型**。pre/post 之分由聚合点（`rulePreBaseTransforms` / `rulePreVendorTransforms`）决定，与 Transform 实现本身无关。 | `transform.OpenAIMaxTokensRewriteTransform`、`transform.OpenAICursorCompatTransform` |
| **Transform（链路阶段，server-domain）** | `internal/server/transform_*.go` | 同 Transform 接口，但需要 server-domain 类型（如 `*typ.ScenarioConfig`）。 | `ThinkingModeTransform`、`MaxTokensTransform`（Anthropic 上限）、`CleanHeaderTransform` |

**为什么必须分两层？**

- `BaseTransform` 负责跨协议转换（Anthropic ↔ OpenAI / Responses /
  Google）。如果在链外直接改 inbound 请求，遇到 "Anthropic inbound →
  OpenAI Chat target" 的路径，改的还是 Anthropic 形态，rewrite 在
  Base 转换之后就丢失。
- 把 rewrite 包成一个 Transform 并加在 Base **之后**（post-Base），它
  看见的是已经转换好的 `*openai.ChatCompletionNewParams`——无论 inbound
  是什么形态，目标是 OpenAI Chat 时就能命中。`max_tokens` 走这条。
- 反过来，如果 mutation 的语义就是"在 inbound 形态上做归一化"，则放
  Base **之前**（pre-Base）。`cursor_compat` 走这条：cursor IDE 只发
  OpenAI Chat，归一化只在 inbound=Chat 时有意义；放 post-Base 反而要
  对每个目标形态各写一遍 op。
- op 是纯函数：可以独立单测（含 wire 序列化）、可以被多个 Transform
  复用、可以被多端调用而不被 chain 绑死。

**为什么再分协议层 vs server-domain？**

- 协议层 Transform 只依赖 SDK 类型，可以放在 `protocol/transform/`，与
  `BaseTransform`、`ConsistencyTransform`、`VendorTransform` 同包。
- 一旦 Transform 需要读 `*typ.ScenarioConfig`、`*typ.Rule` 等 server
  类型，就放到 `internal/server/transform_*.go`，避免反向依赖。

---

## 7. 链路 wiring

```
handler                            transformXxxx                       chain
  │                                     │                                │
  │ flags  := resolveRuleFlags(c, rule) │                                │
  │ preBase  := rulePreBaseTransforms(  │                                │
  │              flags)                 │                                │
  │  (e.g. [OpenAICursorCompat])        │                                │
  │ preVendor:= rulePreVendorTransforms(│                                │
  │              flags)                 │                                │
  │  (e.g. [OpenAIMaxTokensRewrite])    │                                │
  ├─────── preBase, preVendor... ──────►│                                │
  │                                     │  chain := BuildTransformChain( │
  │                                     │      ..., preBase, preVendor)   │
  │                                     ├──────────────────────────────►│
  │                                     │  chain.Execute(ctx)            │
  │                                     ├──────────────────────────────►│
```

关键约束：

- `BuildTransformChain` 是唯一的 chain 装配点。它接受 scenario / provider
  / recording，外加 `preBase []transform.Transform` 与
  `preVendor []transform.Transform` 两个 slot 入参，把它们插进固定骨架的对应
  位置（preBase 在最前的 preBase slot，preVendor 在 Consistency 之后、Vendor
  之前的 preVendor slot）。它自身不解析 rule flag——具体装哪些 Transform 仍由
  handler 通过聚合点决定，但**插哪个位置由 builder 统一保证**，handler 不再
  各自 prepend / append。
- 所有 4 个 `transformXxxx`（`transformAnthropicV1`、
  `transformAnthropicBeta`、`transformOpenAIChat`、
  `transformOpenAIResponses`）都接受一个 `preBaseTransforms []transform.Transform`
  位置参数 + 一个 `preVendorTransforms []transform.Transform`，直接透传给
  `BuildTransformChain`。（`smart_compact` 仍在 Anthropic V1 / Beta 两条
  handler 内单独 prepend 到最前，行为不变。）
- `rulePreBaseTransforms(flags)` 和 `rulePreVendorTransforms(flags)`
  （`internal/server/rule_flags.go`）是 rule→Transform 的两个聚合点。
  新增 rule-driven Transform 时，按"作用于 inbound 形态" / "作用于目标
  形态"二选一，往对应聚合点追加 case；handler 端不需要改动。
- chain 顺序的回归护栏在
  `internal/server/protocol_chain_builder_test.go`：`preVendor` 必须落在
  `consistency_normalize` 之后、`vendor_adjust` 之前。

---

## 8. UA 链层 — 实操中最易踩坑的部分

User-Agent 优先级（每一层都是"set then delegate"，**innermost wins**）：

```
   ┌────────────────────────────────────┐
   │  outer adapter (e.g. claudeRT)     │ ← vendor 硬编码 UA，先 set
   │     │  set "claude-cli/2.1.86"     │
   │     ▼ delegates to                 │
   │  wrapWithUserAgent(provider)       │ ← provider.UserAgent（调试 override）
   │     │  if non-empty: set provider  │   非空时覆盖 vendor 硬编码
   │     ▼                              │
   │  customUserAgentTransport          │ ← rule-level custom_user_agent
   │     │  if ctx has UA: set rule UA  │   非空时覆盖一切（innermost wins）
   │     ▼                              │
   │  base http.Transport (sends wire)  │
   └────────────────────────────────────┘
```

最终结果：

| 场景 | rule UA | provider UA | vendor 硬编码 | 实际发出 |
|------|---------|-------------|--------------|---------|
| 默认 | 空 | 空 | claude-cli/… | claude-cli/… |
| Provider 配了 | 空 | "MyOrg/1.0" | claude-cli/… | MyOrg/1.0 |
| Rule 配了 | "Bench/1" | "MyOrg/1.0" | claude-cli/… | Bench/1 |
| Rule 配了 | "Bench/1" | 空 | — | Bench/1 |

⚠️ `provider.UserAgent` 一旦设置就会覆盖 vendor 硬编码——这是**有意为之**
的调试通道（见 `ai/provider.go` 该字段的 doc comment）。Claude Code OAuth
等端点对 UA 有真实校验，错配会让 OAuth 直接被拒；设置该字段时要清楚自己
在干嘛。

并非所有 client 都接入这条链：

| Client | Provider UA | Rule UA |
|--------|-------------|---------|
| 通用 OpenAI (`client/openai.go`) | ✅ | ✅ |
| 通用 Anthropic 非-OAuth (`client/anthropic.go` else 分支) | ✅ | ✅ |
| Claude Code OAuth (`claudeRoundTripper`) | ✅ | ❌ |
| Codex / Kimi / Gemini / Google | ✅ | ❌ |

vendor-specialized 路径不接 rule UA：它们 UA 跟整个 OAuth/握手协议绑
定，rule 级覆盖反而会破坏 vendor 校验。这条边界写进了
`flag_registry.go` 中 `custom_user_agent` 的 description 里。

⚠️ **重要不变量**：`applyCustomUserAgent`（由 `resolveRuleFlagsWithScenario`
统一调用）把 UA 写进 `c.Request.Context()`（对**所有**请求，无论
scenario/provider），但 UA 只在 transport 链里
**有** `customUserAgentTransport` 的 client 上生效——它读 `req.Context()`。
`customUserAgentTransport` 只在两处接入：通用 `NewOpenAIClient`（openai.go）
与通用非-OAuth Anthropic 分支（anthropic.go else）。vendor 路径
（Codex / Kimi 等）虽然内部复用 `NewOpenAIClient`，但用 `extraOptions` 里
自带的 `WithHTTPClient`（含 `codexRoundTripper` / `kimiRoundTripper`，
**不含** `customUserAgentTransport`）在 SDK option 末尾覆盖掉通用 httpClient
（"extra 最后应用"）；Claude OAuth 走 `NewClaudeClient` 自建链。所以即便
ctx 里带着 UA，vendor client 也没有任何 transport 去读它——握手 UA 不会被
flag 污染。**给 vendor 链新增 transport 时切勿引入 `customUserAgentTransport`。**

---

## 9. 前端 UI

### 统一命名：Plugins

前端统一使用 **"Plugins"** 命名（之前混用 "Plugin" / "Rule Extensions"）。
相关组件：`RulePluginsCard`（右侧卡片）、`FlagCatalogDialog`（弹窗标题
"Rule Plugins"）、`PluginFeatures`（场景级 Plugin 控制）。

### Registry-driven 架构

前端不再硬编码任何 per-flag switch/case。核心工具集中在
`frontend/src/components/rule-card/flagHelpers.ts`：

| Helper | 作用 |
|--------|------|
| `snakeToCamel` / `camelToSnake` | `snake_case` ↔ `camelCase` key 转换 |
| `getFlagValue(flags, key)` | 用 snake_case key 动态读取 camelCase flag |
| `setFlagValue(flags, key, value)` | 用 snake_case key 动态写入，返回新 flags |
| `apiToFlags(api)` / `flagsToApi(flags)` | 整体转换 `RuleFlagsApi` ↔ `RuleFlags` |
| `isFlagActive(flags, spec)` | 根据 spec.Type 判断 flag 是否启用 |
| `flagDefault(spec)` | 从 spec 取默认值（bool→false, enum→Options[0], etc.） |
| `enumInactive(spec)` | 取 enum 的"未启用"值（首个 Option.Value） |

这意味着**新增 flag 只需后端 `RuleFlagRegistry()` + `RuleFlags` struct 加字段 + 前端 `RuleFlags`/`RuleFlagsApi` 类型加字段**，无需改动任何 switch/case 或 UI 组件逻辑。

### 卡片位置

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Rule Card                                            [⚙ settings]       │
│                                                                         │
│  ┌──────────────────────────────── horizontal scroll ──→  ┐ ┌────────┐  │
│  │ ModelNode → ArrowNode → ProviderNode → ProviderNode … │ │Plugins │  │
│  │                                                        │ │ Card   │  │
│  └────────────────────────────────────────────────────────┘ └────────┘  │
│           ↑                                                    ↑        │
│   随 provider 增多可横向滚动                          常驻右侧不滚动    │
└─────────────────────────────────────────────────────────────────────────┘
```

关键 CSS 决策：

- 外层 `display: flex`，graph 在 `flexGrow:1, minWidth:0` 的 box 里启用
  `overflowX:auto`；`minWidth:0` 是让 flex item 能收缩到内容宽以下的关
  键，否则会把 Plugins Card 推出视口。
- Plugins Card `flexShrink:0`——滚动条出现时它**不缩**。
- Card 高度 = `PROVIDER_NODE_STYLES.height`（72px），视觉与 provider 行对
  齐；内容溢出时 card 内 `overflowY:auto`。

### Catalog 弹窗

按 `FlagSpec.Category` 分组。所有渲染逻辑 registry-driven——根据
`spec.Type` 选择控件，无需为每个 flag 写 case：

- `type: bool` → `Switch`。
- `type: string` → `Switch` + `TextField`（空字符串 = 未启用）。
- `type: enum` → `Switch` + `Select`（首个 Option = 未启用状态）。
- `type: int` → `Switch` + `TextField` (number)（0 = 未启用）。
- `type: service_ref` → `Switch` + provider/model picker。
- 已启用的 flag 在边框 / 背景上有高亮。

### 路由图齿轮菜单

齿轮菜单只保留通用项（Test Probe、Export、Activate/Deactivate、Edit
flag、Delete）。**Cursor 专用菜单项已删除**——所有 flag 都通过右侧
Plugins Card 操作。

---

## 10. 新增一个 flag — 操作手册

按以下顺序，**勿乱序**：

```
1. internal/typ/type.go
   ├─ 给 RuleFlags struct 加一个字段（snake_case json tag）
   └─ yaml tag 与 json 保持一致

2. internal/typ/flag_registry.go
   └─ 在 RuleFlagRegistry() 追加一个 FlagSpec，
      key 必须与上一步的 json tag 一字不差
      ├─ 如果 flag 同时存在 scenario 和 rule 级：
      │   设 Shared: true，并声明 InheritanceMode ("or" / "override")
      └─ 如果 flag 是 enum 类型，首个 Option 是"未启用"默认值

3. 选定 §5 的注入类型，落地行为：

   ┌─────────────────────────────────────────────────────────────┐
   │ Type 1b (Transform — 推荐)                                   │
   │   ① internal/protocol/transform/ops/<xxx>.go：写 op 原语，    │
   │      签名形如 ApplyXxx(*openai.ChatCompletionNewParams) 或    │
   │      其他具体 request 类型。op 必须纯函数、无 rule 感知。     │
   │   ② internal/protocol/transform/<xxx>.go：写 Transform，      │
   │      构造期接受配置，Apply() 里 type-switch ctx.Request，     │
   │      匹配目标类型时调 op。如需 server-domain 类型才放到       │
   │      internal/server/ 下。                                    │
   │   ③ pre-Base / post-Base 二选一：                             │
   │      • 作用于 inbound 形态（cursor_compat 类）→               │
   │        internal/server/rule_flags.go::rulePreBaseTransforms   │
   │      • 作用于目标形态（max_tokens 类）→                       │
   │        internal/server/rule_flags.go::rulePreVendorTransforms │
   │   ④ handler 端无需改动——4 个 handler 都已调                  │
   │      rulePreBaseTransforms(flags) / rulePreVendorTransforms(flags)│
   │                                                              │
   │ Type 2 (context-passed hint)                                 │
   │   ① 在 internal/typ/id.go 加 contextKey + 一对                │
   │      WithXxx / GetXxx helper。                                │
   │   ② handler 入口 c.Request = c.Request.WithContext(WithXxx)。 │
   │   ③ 消费方（transport / round-tripper）读 GetXxx。            │
   │                                                              │
   │ Type 3 (response 后置加工)                                    │
   │   ① handler 把 flag 值写进 reqCtx.Extra。                    │
   │   ② internal/server/protocol_dispatch.go：在派发分支调用      │
   │      shouldStripUsage(...) 这类聚合判定。                     │
   └─────────────────────────────────────────────────────────────┘

4. 测试位置随注入类型走：
   ├─ op 单元测试 → 与 op 同包 (`internal/protocol/transform/ops/`)
   ├─ Transform 行为测试 → 与 Transform 同包
   │     必备 case：在目标 request 类型上启用 / 在其他类型上 no-op /
   │              chain 中配合 stub BaseTransform 验证位置正确：
   │              • post-Base：跟在 stub Base 之后，验证目标形态生效
   │                （max_tokens 模式）。
   │              • pre-Base：跟在 stub Base 之前，验证 inbound 形态
   │                生效（cursor_compat 模式）。
   └─ wire 序列化测试（仅对改字段名的 op 有意义）：marshal 前后断言
       JSON 顶层 key 出现/消失，挡 SDK omitzero tag 失效。

5. frontend/src/components/RoutingGraphTypes.ts
   ├─ RuleFlags 接口加 camelCase 字段
   └─ RuleFlagsApi 接口加 snake_case 字段（与后端 json tag 对齐）

6. (完毕 — 无需改动前端 UI 组件)
   flagHelpers.ts 的 registry-driven 工具（getFlagValue / setFlagValue /
   isFlagActive / apiToFlags / flagsToApi）通过 snakeToCamel key 动态
   访问 RuleFlags 字段。RulePluginsCard、FlagCatalogDialog、
   useRuleCardHooks 均无需 per-flag switch/case 改动。
```

`TestRuleFlagRegistry_KeysMatchStructFields` 会在第 1、2 步任一步漏改
时立即红——这是链路最稳的安全网。

---

## 11. 设计取舍

| 选项 | 已采纳 | 备择 | 取舍理由 |
|------|--------|------|----------|
| `RuleFlags` 用 typed struct vs `map[string]any` | typed struct | map | 编译期检查；JSON 兼容性靠 `omitempty` 与零值 |
| Registry 由后端 owner | ✅ | 前端硬编码 + 后端硬编码两边对 | 单一可信源，新 flag 只动一处元数据 |
| 卡片放路由图内 vs 固定右侧 | 固定右侧 | 卡片随路由图滚动 | flag 是 rule 级而非 provider 级，常驻可见更符合心智模型 |
| string flag 的"启用"语义 | 空 = 未启用 | 独立 enable Switch + 文本 | 一个字段一个状态，UI 更简单；权衡是无法区分"空字符串"和"未配置" |
| UA 链 vendor pin 是否不可覆盖 | 否，可被 provider/rule 覆盖 | vendor pin 强制 | 把 `provider.UserAgent` 当调试 override 更灵活；用 doc comment 规约 |
| 请求字段重写：handler pre-chain mutate vs post-base Transform | post-base Transform | handler 链外直改 | 链外直改在跨协议路径（Anthropic→OpenAI）失效；Transform 在 Base 之后看到的是最终形态，所有 inbound 类型都能命中 |
| preVendor transforms 的 chain 位置 | Consistency 之后、**Vendor 之前** | append 到 chain 末尾（Vendor 之后） | Vendor 直面 provider 做不可逆改写，必须是最后一个 mutation；preVendor 跑在 Vendor 之后会破坏"vendor 最终态"且让 StagePost 录制抓不到真实出站请求 |
| op vs Transform 是否合并 | 分两层 | 把 op 直接做成 Transform | op 是纯函数原语（可独立测、可复用、可多端调用）；Transform 才感知 rule 与链路位置。合并会让原语难复用 |
| Transform 放协议层包 vs server 包 | 视依赖而定 | 全部塞 server | 只依赖 SDK / 协议类型的放 `internal/protocol/transform/`；需要 server-domain 类型的放 `internal/server/`。避免反向依赖 |
| `BuildTransformChain` 是否感知 rule | 否（只收已构造好的 Transform） | 把 ruleFlags 传入 | chain builder 不解析 flag，但作为唯一装配点接收 `preBase` / `preVendor` 两个 slot 入参并保证插入位置（preBase / preVendor slot）；rule→Transform 的决策仍在 handler 聚合点。早期版本由各 handler 自行 prepend/append，导致插入位置分散且 preVendor transforms 误落在 Vendor 之后 |
| 取消路由图齿轮菜单中的 Cursor 专用项 | ✅ | 同时保留菜单项和 Plugins 卡片 | 两套入口对同一字段会引发混淆 |
| 前端 registry-driven vs per-flag switch/case | registry-driven | 每个 flag 一个 case | registry-driven 消除 ~293 行重复代码；新增 flag 零前端 UI 改动。代价是需要 snakeToCamel 动态映射（类型不够严格），但 `TestRuleFlagRegistry_KeysMatchStructFields` 保证 key 与 struct 同步 |
| `Shared` / `InheritanceMode` 嵌入 FlagSpec | ✅ | 前端单独维护共享清单 | 继承语义是 flag 固有属性，放在 registry 可信源里前端零维护 |

---

## 12. Scenario-level Flags（场景级 Flag）

ScenarioFlags 是 RuleFlags 的特殊子集：Scenario flag 设置场景级默认值，
rule flag 在请求级覆盖或继承它。`resolveRuleFlagsWithScenario`（`internal/server/rule_flags.go`）
是这条继承链的唯一落地点。

| Flag | Rule级 | Scenario级 | 继承行为 |
|------|--------|------------|----------|
| `skip_usage` | ✅ | ✅ | Scenario 启用时自动 OR 进 rule 的 `SkipUsage`（rule 未禁用即生效）|
| `thinking_effort` | ✅ | ✅ | Rule 显式设置 > Scenario 默认 > By Client（空字符串）|
| `claude_code_compat` | ✅ | ✅ | Scenario 启用时自动 OR 进 rule 的 `ClaudeCodeCompat` |
| `custom_user_agent` | ✅ | ✅ | Rule 显式设置（非空）> Scenario 默认 > 不覆盖（空）。注入点 `applyCustomUserAgent`（Type 2，写入 c.Request.Context()）|

**Scenario-only flags**（无 rule 级对应，只存在于 ScenarioFlags）：

| Flag | 作用 |
|------|------|
| `smart_compact` | 从会话历史中移除 thinking blocks 以减少 context（实现：`internal/server/transform.ThinkingCompactTransform`） |
| `recording_v2` | 控制请求/响应录制模式（off / request / request_response / staged） |
| `unified` / `separate` / `smart` | 路由模式开关 |

**Rule-only flags（曾是 scenario 级、已下放为纯 rule 级）**：

- `clean_header` 原本是 scenario-shared（OR 继承），现已从 scenario plugin 移除——`ScenarioFlags.CleanHeader` 字段、`GetScenarioFlag`/`SetScenarioFlag` 的 `clean_header` 分支、`FlagCleanHeader` 常量、以及 `resolveRuleFlagsWithScenario` 里的 OR 注入全部删除。改为 built-in CC rule 默认开（见上方主表与 §built-in 默认），UI 上只在 Plugins 卡片按 rule 呈现真实值，不再有 scenario 级开关。
- `session_affinity` 原本是 scenario-shared（`>0` override 继承），现已下放为纯 rule 级（参考 `clean_header`）——`ScenarioFlags.SessionAffinity` 字段、`GetScenarioIntFlag`/`SetScenarioIntFlag` 的 `session_affinity` 分支、`FlagSessionAffinity` 常量、`GetEffectiveAffinity` 的 scenario fallback、以及 `resolveRuleFlagsWithScenario` 里的注入全部删除。改为 built-in **Claude Code / Claude Desktop / Codex** rule 默认 1800s（`init.go` 种子 + `migrate20260610` 存量回填），其余 rule 不设即禁用，UI 上只在 Plugins 卡片按 rule 调整。`GetScenarioIntFlag`/`SetScenarioIntFlag` 与其 HTTP int-flag endpoint 当前无任何注册 key，但作为**通用 infra 保留**（scenario 级 int flag 的读写骨架）——未来新增 scenario int flag 只需在 `flag_keys.go` 加 key 常量 + 在 `config.go` 的 switch 里加一个 case。`migrate20260606` 不再种 scenario 级 affinity，仅保留 Xcode `SkipUsage` 默认。

> **Migration 整合**：上述 `claude_code_compat` / `clean_header` / `session_affinity` 三个 rule 级默认值，曾分散在 `migrate20260608`/`_2`/`_3`/`_4` 与 `migrate20260609`/`_2`/`_3` 共 7 个 migration 中，现已统一重组为单个 **`migrate20260610`**（一次性 marker 门控，按 rule scenario 分支：CC / Desktop → 三个 flag 全开，Codex → 仅 affinity），与 `init.go` 的内建 rule 种子一一对应。

**继承逻辑实现**：

```go
// resolveRuleFlagsWithScenario 的继承顺序（internal/server/rule_flags.go）
func resolveRuleFlagsWithScenario(...) typ.RuleFlags {
    flags := resolveRuleFlags(c, rule) // 含 cursor_compat_auto 折叠

    if scenarioConfig != nil {
        // ThinkingEffort：rule 未设置时继承 scenario 值
        if flags.ThinkingEffort == "" && scenarioConfig.Flags.ThinkingEffort != "" {
            flags.ThinkingEffort = scenarioConfig.Flags.ThinkingEffort
        }
        // ClaudeCodeCompat / SkipUsage：OR 语义（任一启用即生效）
        // 注意：clean_header 已不在此 —— 它是 rule-only，无 scenario 级 OR 注入。
        flags.ClaudeCodeCompat = flags.ClaudeCodeCompat || scenarioConfig.Flags.ClaudeCodeCompat
        flags.SkipUsage        = flags.SkipUsage || scenarioConfig.Flags.SkipUsage
        // CustomUserAgent：rule 非空时优先，否则继承 scenario 默认
        if flags.CustomUserAgent == "" && scenarioConfig.Flags.CustomUserAgent != "" {
            flags.CustomUserAgent = scenarioConfig.Flags.CustomUserAgent
        }
        // 注意：session_affinity 已不在此 —— 它是 rule-only，无 scenario 级注入。
    }
    // claude_desktop 等 billing 场景：协议转换时自动启用 CleanHeader（claude_code 已 rule 级默认开）
    flags = autoSetCleanHeaderFlag(flags, sourceAPI, targetAPI, scenarioType)
    // Claude OAuth provider：原生 Anthropic 计费后端要消费 billing header，抑制 CleanHeader
    if flags.CleanHeader && provider.IsClaudeCodeProvider() {
        flags.CleanHeader = false
    }
    return flags
}

// session_affinity 路由层解析顺序（Config.GetEffectiveAffinity）—— rule-only，
// 无 scenario fallback。built-in CC/Desktop/Codex rule 已种 1800s；其余 rule
// 不设即禁用。
func (c *Config) GetEffectiveAffinity(rule *typ.Rule) time.Duration {
    if rule.Flags.SessionAffinity > 0 {
        return time.Duration(rule.Flags.SessionAffinity) * time.Second
    }
    return 0 // 禁用
}
```

**默认值策略（session_affinity，rule 级）**：

- built-in **Claude Code / Claude Desktop / Codex** rule：默认 1800 秒（30 分钟），由 `init.go` 种子（新装）+ `migrate20260610` 回填（存量）。
- 其余场景 / 用户自建 rule：不设即禁用（0）。需要时在 Plugins 卡片按 rule 开启。
- Profile（如 claude_code:p1）：migration 通过 `ParseScenarioProfile` 按 base scenario 匹配，profile rule 同样回填。

**持久化语义**：

- Migration 一次性回填（marker 门控）：用户事后把某 rule 的 affinity 设为 0（禁用）后，重启不会被重新打开。
- Rule 的 `0` 值现在直接表示"禁用"（无 scenario 继承可回退）。
- Profile 删除时清理独立的 scenario config（防止遗留）

---

## 13. 未做 / 后续可做

- **前端**：Scenario-level flag 的 `PluginFeatures.tsx` 仍硬编码
  per-flag 状态管理。下一步应建立 `ScenarioFlagRegistry`（或复用 rule
  registry 的 `Shared` 子集），让场景级 UI 也走 registry-driven 路径，
  消除 `PluginFeatures.tsx` 中的 switch/case。
- **UI**：string flag 加独立 enable Switch，让"空"与"未启用"可区分。
- **UI**：Catalog 加搜索框 / category collapse（flag 数量超过 ~8 个时
  会拥挤）。
- ~~**后端**：`cursor_compat` 内容归一化目前是 Type 1a（pre-chain）~~。
  **已完成**：cursor_compat 现在是 Type 1b-pre（pre-Base Transform，
  `transform.OpenAICursorCompatTransform`），通过
  `rulePreBaseTransforms` prepend 到 chain 最前；handler 入口的 pre-chain
  mutation 已移除。
- ~~**后端**：部分 ScenarioFlags 可下沉成 rule flag（`disable_stream_usage`
  与 `skip_usage` 高度重叠）~~。**已完成**：`ScenarioFlags.DisableStreamUsage`
  已移除并统一为 `SkipUsage`（json: `skip_usage`）。Xcode 场景的默认值
  通过 migration 改为 `SkipUsage: true`；继承逻辑在
  `resolveRuleFlagsWithScenario` 中 OR 合并。`ScenarioFlags` 与 `RuleFlags`
  共享 flag 时统一通过该函数注入，不再双轨维护。
- ~~**前端**：`PluginFeatures.tsx` 混合了多个 flag 的状态管理+渲染~~。
  **已完成**：拆分为独立子组件（`ThinkingEffortControl`、`RecordingV2Control`、
  `VisionProxyControl`、`PluginToggleButton`），均放在
  `frontend/src/components/flags/`。`PluginFeatures` 自身缩减为薄编排层，
  一次 `Promise.all` 加载后将数据+回调传给各子组件。
- ~~**前端**：Rule flag UI 各组件（RuleExtensionsCard、FlagCatalogDialog、
  useRuleCardHooks、utils）硬编码 per-flag switch/case~~。**已完成**：
  统一为 registry-driven 架构。核心工具集中在 `flagHelpers.ts`
  （`getFlagValue`/`setFlagValue`/`apiToFlags`/`flagsToApi`/`isFlagActive`
  等）。新增 flag 零前端 UI 代码改动。组件重命名为 `RulePluginsCard`，
  统一使用 "Plugins" 命名。
- **后端**：`internal/client/openai.go` 通用 OpenAI client 用
  `http.DefaultTransport` 而非 `createSessionBoundTransport`，没有走
  transport pool / session 隔离——独立问题，不在 rule flag 范畴，但
  补齐时记得保留 §8 的两层 UA 包装顺序。
- **测试**：当前覆盖 op / Transform / handler helper / wire 序列化四
  层，缺一个 "rule 配了 skip_usage → 真实 HTTP 响应里没有 usage" 的端
  到端 case，待 mock provider fixture 成熟后补。
