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
   │   ├─ preBase := rulePreBaseTransforms(flags)                   │  Type 1b-pre
   │   │   → [transform.OpenAICursorCompatTransform{}]              │
   │   └─ extras := ruleExtraTransforms(flags)                      │  Type 1b-post
   │       → [transform.OpenAIMaxTokensRewriteTransform{...}]       │
   └───────────────────────────┬───────────────────────────────────┘
                               │
                               ▼
   ┌───────────────────────────────────────────────────────────────┐
   │   transformXxxx(..., preBase, extras...) :                     │
   │     chain := BuildTransformChain(...)                          │
   │     prependPreBaseTransforms(chain, preBase)                   │
   │     appendExtraTransforms(chain, extras)                       │
   │     chain.Execute(ctx)                                         │
   │                                                                │
   │     preBase → Base → MCP → Consistency → Vendor → extras       │
   │   (pre-base stages)                       (post-base stages)   │
   └───────────────────────────┬───────────────────────────────────┘
                               ▼
                       upstream provider
```

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
| `session_affinity` | int (seconds) | routing | **yes** | override | 会话亲和 TTL（秒），0=禁用，>0=启用。Pin 会话到服务以提升缓存命中率。Session ID 解析优先级：Anthropic metadata.user_id > X-Tingly-Session-ID header > 客户端 IP。| `ProviderResolver.PostProcess()` → `Config.GetEffectiveAffinity(rule)`（Type 5）|
| `cursor_compat` | bool | app | — | — | Cursor IDE 内容归一化 + stream usage 抑制 | `transform.OpenAICursorCompatTransform` → `ops.ApplyCursorCompatContentNormalization`（Type 1b-pre）|
| `cursor_compat_auto` | bool | app | — | — | 通过请求头识别 Cursor，自动折叠进 `cursor_compat` | `resolveRuleFlags(c, rule)` 在 handler 入口合并 |
| `claude_code_compat` | bool | app | **yes** | or | 把 messages 中 `role == "system"` 重写为 `"user"`。Claude Code 在 messages 中写入 system role（非标准扩展）；此 flag 在转发前归一化。| `transform.ClaudeCodeCompatTransform` → `ops.ApplyClaudeCodeCompatRoleRewrite`（Type 1b-pre，仅 Anthropic 入站形态）|
| `clean_header` | bool | app | **yes** | or | 剥离 system messages 中的 x-anthropic-billing-header 块。billing 场景（claude_code, claude_desktop）协议转换时自动启用；也可手动强制启用。| `CleanHeaderTransform`（Type 1b-pre，server-domain Transform）|

---

## 5. 五种 flag 注入手法

```
Type 1b-pre  Request body, pre-Base Transform
         作为 Transform 接口的实现 prepend 到 chain 最前（BaseTransform
         之前）运行；type-switch 决定是否对 inbound request 形态生效。
         例：OpenAICursorCompatTransform —— 在 Base 把 OpenAI Chat 转
         成其他形态之前 flatten 富文本内容。聚合点：
         `internal/server/rule_flags.go::rulePreBaseTransforms`。

Type 1b-post Request body, post-Base Transform（推荐用于跨协议 rewrite）
         作为 Transform 接口的实现 append 到 chain 末尾，在 BaseTransform
         完成协议转换之后运行；type-switch 决定是否对目标 request 形态
         生效。例：OpenAIMaxTokensRewriteTransform。聚合点：
         `internal/server/rule_flags.go::ruleExtraTransforms`。

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
个 `Transform` 接口的实现，区别只在 chain 中的位置：pre 在 BaseTransform
之前（看见 inbound 形态），post 在之后（看见目标形态）。聚合点
`rulePreBaseTransforms` / `ruleExtraTransforms` 决定 flag 装哪边。

---

## 6. op vs Transform —— Type 1b 内部的分层

Type 1b（pre-Base 与 post-Base 同形）涉及两层抽象，**职责必须分开**：

| 层 | 位置 | 职责 | 例子 |
|----|------|------|------|
| **op（操作原语）** | `internal/protocol/transform/ops/` | 纯函数。对某个具体 request 类型做无副作用的字段改写。**不感知链路、不感知 rule、不感知 ctx**。 | `ops.ApplyMaxCompletionTokensRewrite(*openai.ChatCompletionNewParams)`、`ops.ApplyCursorCompatContentNormalization(*openai.ChatCompletionNewParams)` |
| **Transform（链路阶段，协议层）** | `internal/protocol/transform/` | 实现 `Transform` 接口。构造期接受配置；`Apply()` 里 type-switch `ctx.Request`，匹配目标类型时调 op。**仅依赖协议层 / SDK 类型**。pre/post 之分由聚合点（`rulePreBaseTransforms` / `ruleExtraTransforms`）决定，与 Transform 实现本身无关。 | `transform.OpenAIMaxTokensRewriteTransform`、`transform.OpenAICursorCompatTransform` |
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
  │ preBase:= rulePreBaseTransforms(    │                                │
  │             flags)                  │                                │
  │  (e.g. [OpenAICursorCompat])        │                                │
  │ extras := ruleExtraTransforms(      │                                │
  │             flags)                  │                                │
  │  (e.g. [OpenAIMaxTokensRewrite])    │                                │
  ├──────── preBase, extras... ────────►│                                │
  │                                     │  chain := BuildTransformChain  │
  │                                     │           (...)                │
  │                                     ├──────────────────────────────►│
  │                                     │  prependPreBaseTransforms(     │
  │                                     │      chain, preBase)           │
  │                                     │  appendExtraTransforms(        │
  │                                     │      chain, extraTransforms)   │
  │                                     │  chain.Execute(ctx)            │
  │                                     ├──────────────────────────────►│
```

关键约束：

- `BuildTransformChain` 不感知 rule flag。它的输入是 scenario / provider
  / recording——把哪些 rule Transform 要 prepend / append 的决策留给
  handler。
- 所有 4 个 `transformXxxx`（`transformAnthropicV1`、
  `transformAnthropicBeta`、`transformOpenAIChat`、
  `transformOpenAIResponses`）都接受一个 `preBaseTransforms []transform.Transform`
  位置参数 + variadic `extraTransforms ...transform.Transform`，分别用
  `prependPreBaseTransforms(chain, preBase)` 和
  `appendExtraTransforms(chain, extras)` 装到 chain 两端。
- `rulePreBaseTransforms(flags)` 和 `ruleExtraTransforms(flags)`
  （`internal/server/rule_flags.go`）是 rule→Transform 的两个聚合点。
  新增 rule-driven Transform 时，按"作用于 inbound 形态" / "作用于目标
  形态"二选一，往对应聚合点追加 case；handler 端不需要改动。

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
   │        internal/server/rule_flags.go::ruleExtraTransforms     │
   │   ④ handler 端无需改动——4 个 handler 都已调                  │
   │      rulePreBaseTransforms(flags) / ruleExtraTransforms(flags)│
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
| op vs Transform 是否合并 | 分两层 | 把 op 直接做成 Transform | op 是纯函数原语（可独立测、可复用、可多端调用）；Transform 才感知 rule 与链路位置。合并会让原语难复用 |
| Transform 放协议层包 vs server 包 | 视依赖而定 | 全部塞 server | 只依赖 SDK / 协议类型的放 `internal/protocol/transform/`；需要 server-domain 类型的放 `internal/server/`。避免反向依赖 |
| `BuildTransformChain` 是否感知 rule | 否 | 把 ruleFlags 传入 | chain builder 只懂 scenario/provider/recording；rule 级关心点放在 handler，通过 `extraTransforms ...transform.Transform` variadic 注入 |
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
| `clean_header` | ✅ | ✅ | Scenario 启用时自动 OR 进 rule 的 `CleanHeader`；billing 场景协议转换时自动启用 |
| `claude_code_compat` | ✅ | ✅ | Scenario 启用时自动 OR 进 rule 的 `ClaudeCodeCompat` |
| `custom_user_agent` | ✅ | ✅ | Rule 显式设置（非空）> Scenario 默认 > 不覆盖（空）。注入点 `applyCustomUserAgent`（Type 2，写入 c.Request.Context()）|
| `session_affinity` | ✅ | ✅ | Rule 显式设置（>0）> Scenario 默认 > 禁用（0）|

**Scenario-only flags**（无 rule 级对应，只存在于 ScenarioFlags）：

| Flag | 作用 |
|------|------|
| `smart_compact` | 从会话历史中移除 thinking blocks 以减少 context |
| `recording_v2` | 控制请求/响应录制模式（off / request / request_response / staged） |
| `unified` / `separate` / `smart` | 路由模式开关 |

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
        // CleanHeader / ClaudeCodeCompat / SkipUsage：OR 语义（任一启用即生效）
        flags.CleanHeader      = flags.CleanHeader || scenarioConfig.Flags.CleanHeader
        flags.ClaudeCodeCompat = flags.ClaudeCodeCompat || scenarioConfig.Flags.ClaudeCodeCompat
        flags.SkipUsage        = flags.SkipUsage || scenarioConfig.Flags.SkipUsage
        // CustomUserAgent：rule 非空时优先，否则继承 scenario 默认
        if flags.CustomUserAgent == "" && scenarioConfig.Flags.CustomUserAgent != "" {
            flags.CustomUserAgent = scenarioConfig.Flags.CustomUserAgent
        }
        // SessionAffinity：rule 显式设置（>0）优先
        if flags.SessionAffinity == 0 && scenarioConfig.Flags.SessionAffinity > 0 {
            flags.SessionAffinity = scenarioConfig.Flags.SessionAffinity
        }
    }
    // billing 场景协议转换时自动启用 CleanHeader
    flags = autoSetCleanHeaderFlag(flags, sourceAPI, targetAPI, scenarioType)
    return flags
}

// session_affinity 路由层解析顺序（Config.GetEffectiveAffinity）
func (c *Config) GetEffectiveAffinity(rule *typ.Rule) time.Duration {
    // 1. Rule 显式设置
    if rule.Flags.SessionAffinity > 0 {
        return time.Duration(rule.Flags.SessionAffinity) * time.Second
    }

    // 2. Scenario 默认（profiled scenario 先找自己，没有则 fallback 到 base）
    scenarioConfig := c.scenarioConfigLocked(rule.GetScenario())
    if scenarioConfig != nil && scenarioConfig.Flags.SessionAffinity > 0 {
        return time.Duration(scenarioConfig.Flags.SessionAffinity) * time.Second
    }

    // 3. 禁用
    return 0
}

// scenarioConfigLocked 的查找顺序
func (c *Config) scenarioConfigLocked(scenario typ.RuleScenario) *typ.ScenarioConfig {
    // 1. 精确匹配 profiled scenario（如 claude_code:p1）
    for i := range c.Scenarios {
        if c.Scenarios[i].Scenario == scenario {
            return &c.Scenarios[i]
        }
    }

    // 2. Profile fallback 到 base scenario
    baseScenario, profileID := typ.ParseScenarioProfile(scenario)
    if profileID != "" {
        for i := range c.Scenarios {
            if c.Scenarios[i].Scenario == baseScenario {
                return &c.Scenarios[i]
            }
        }
    }

    return nil
}
```

**默认值策略**：

- IDE/Agent 场景（claude_code, claude_desktop, vscode, xcode, agent, codex, opencode）：默认 1800 秒（30 分钟）
- API 场景（openai, anthropic, embed, imagegen）：默认禁用（0）
- Profile（如 claude_code:p1）：继承 base scenario 默认，可独立配置覆盖

**持久化语义**：

- Scenario 配置**只在不存在时创建默认**，用户配置后不会被覆盖
- Rule 的 `0` 值表示"未设置"而非"禁用"，通过继承链决定实际值
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
