# Provider Extensions & Provider/Model/Rule Flags

> 状态：**rule 级已实现；provider/model 级为未实施提案**（api_key-only 发布范围）：
> PR1 — rule 级 `extra_headers`（含前端，已实现）；
> PR2 — **待实施**：provider & model 级改造（Extensions 容器、db 列、provider API 与前端）。
> 下文三级合并、provider API/UI 与 extensions schema 均描述目标状态，不能作为当前接口使用。
> 适用对象：tingly-box 后端 / 前端贡献者。
> 相关文档：`.design/rule-flags.md`（rule/scenario 级 flag 机制，本设计大量复用其模式）、
> `.design/user-agent.md`（vendor pin 不可污染的边界，本设计必须尊重）、
> `.design/dual-provider.md`。

---

## 1. 目标与动机

为 Provider 增加**扩展字段**，并以第一个 flag `extra_headers` 打通
**三级统一控制**：

1. **Provider 级 flag** —— 作用于该 provider 的所有出站请求；
2. **Model 级 flag** —— 只作用于该 provider 下某个具体 model 的请求；
3. **Rule 级 flag** —— 作用于命中该 rule 的请求（复用既有 RuleFlags 机制）。

第一个落地的 flag：**`extra_headers`** —— 为出站请求追加 N 个自定义
HTTP header（典型场景：OpenRouter 的 `HTTP-Referer`/`X-Title`、
Cloudflare AI Gateway 的网关 header、企业内网网关的租户/审计 header、
自建推理服务的自定义路由 header）。三级各自维护一个 header map，
请求时合并（§3.3），一次性覆盖"上游固有 / 按模型 / 按客户端来源"
三种粒度的诉求。

**首版发布范围：仅 `api_key` auth type 的 provider**（含空 auth type 的
向后兼容语义与 dual provider）。OAuth / vendor 特种链、多字段凭证
（aws_sigv4 / azure_key / gcp_sa）、vmodel 首版不释放（§5.4）。

分层原则（本设计的核心约束）：

> **对 `ai` module 而言，扩展字段是任意的（opaque）；
> 对 tingly-box 服务而言，扩展字段有明确定义的 schema 与语义。**

`ai/` 是独立 go module、对外公共 API。它不应该知道 tingly-box 服务定义了
哪些 flag——它只提供一个无 schema 的扩展容器。所有类型化、注册表、校验、
合并语义都在 `internal/` 落地。

现有 flag 语义由此补全为四层（rule-flags.md §1 的表 + 本设计的前两行）：

| 维度 | 粒度 | 归属 | 例子 |
|------|------|------|------|
| Provider flags（**本设计**） | provider 实例 | 供给侧（对上游） | `extra_headers` |
| Model flags（**本设计**） | provider × model | 供给侧（对上游） | `extra_headers`（model 覆写） |
| Scenario flags | scenario | 请求侧（对客户端） | `skip_usage`、`smart_compact` |
| Rule flags | 单条 rule | 请求侧（对客户端） | `cursor_compat`、**`extra_headers`（本设计新增）** |

判断一个新 flag 归哪层："这个行为是**这个上游 provider/model 的固有属性**
（无论哪个客户端、哪条 rule 打过来都成立）"→ provider/model 级；
"这个行为取决于**是谁在请求、怎么请求**"→ rule/scenario 级。
`extra_headers` 是少数三级都有合理语义的 flag（网关 header 是 provider
固有；模型灰度 header 是 model 维度；按客户端打审计标是 rule 维度），
所以三级同名同形态、统一合并。但**同一概念多级并存是例外不是常态**——
UA 的教训（`provider.UserAgent` 移除史，user-agent.md §5）仍然成立：
新 flag 默认只落一层，三级并存需要像本节这样逐级说清语义。

---

## 2. 分层模型

```
┌─────────────────────────────────────────────────────────────┐
│  ai module（公共 API，opaque）                                │
│                                                             │
│  ai.Provider {                                              │
│      ...                                                    │
│      Extensions map[string]json.RawMessage `json:"extensions,omitempty"` │
│  }                                                          │
│                                                             │
│  ai 包不解释内容；任何消费方可用自己的 key 存放任意 JSON。       │
└──────────────────────────┬──────────────────────────────────┘
                           │ well-known keys（服务侧独占）
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  tingly-box 服务（internal/，typed）                          │
│                                                             │
│  internal/constant:                                         │
│      ProviderExtKeyFlags      = "provider_flags"  // ProviderFlags │
│      ProviderExtKeyModelFlags = "model_flags"  // map[model]ProviderFlags │
│                                                             │
│  internal/typ:                                              │
│      type ProviderFlags struct {                            │
│          ExtraHeaders map[string]string `json:"extra_headers,omitempty"` │
│          // 后续 flag 在此追加 typed 字段                       │
│      }                                                      │
│      RuleFlags 追加同形态字段：                                │
│          ExtraHeaders map[string]string `json:"extra_headers,omitempty"` │
│      GetProviderFlags(p)            // 解 "flags" key         │
│      GetModelFlags(p, model)        // 解 "model_flags"[model]│
│      EffectiveExtraHeaders(p, model, ruleFlags) // 三级合并（§3.3）│
│                                                             │
│  internal/typ/provider_flag_registry.go:                    │
│      ProviderFlagRegistry() []FlagSpec  // provider/model 级可信源（§4）│
│  internal/typ/flag_registry.go:                             │
│      RuleFlagRegistry() 追加 extra_headers 条目（rule 级）      │
└─────────────────────────────────────────────────────────────┘
```

Rule 级不走 Extensions 容器——RuleFlags 本来就是服务域内的 typed struct
（rule-flags.md §3），直接加字段即可；Extensions 只解决"公共 module 不能
认识服务 schema"的问题，Rule 类型不存在这个问题。

### 为什么是 `map[string]json.RawMessage` 而不是 `map[string]any`

- **无损**：RawMessage 原样保存字节，不经历 `any` 的 float64 化 /
  key 顺序丢失；ai module "任意"的承诺才真正成立。
- **强制分层**：ai 包物理上无法"顺手"读懂内容，schema 只能在服务侧定义，
  防止类型定义渗回公共 module。
- 服务侧解码是一次小 JSON unmarshal，请求路径成本可忽略（provider 对象
  在 ClientPool / dispatch 中已被缓存与克隆，必要时可在解析处 memoize，
  首版不做）。
- 先例差异说明：`ScenarioConfig.Extensions` 用了 `map[string]interface{}`，
  但它本身就在 internal/typ（服务域内），没有跨 module 分层需求；
  ai.Provider 是公共 API，取 RawMessage 更严格。

`ResolveStyle` / dual-provider 的 shallow clone 语义不受影响：Extensions
是引用共享的 map，clone 不深拷贝，所有读取方**只读**（写入只发生在
usecase 保存路径）。

---

## 3. 数据模型与合并语义

### 3.1 ProviderFlags / RuleFlags.ExtraHeaders

```go
// internal/typ —— provider 级与 model 级共用
type ProviderFlags struct {
    // ExtraHeaders 追加到发往该 provider 的出站请求。
    // key = header 名（保存时规范化为 canonical form），value = 字面值。
    ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
}

// internal/typ/type.go —— RuleFlags 追加同名同形态字段
//     ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
```

`map[string]string` vs `[]HeaderKV` 的取舍：map 无法表达重复 header 与
顺序，但配置型 header 几乎不需要重复（重复 header 的典型是 Cookie/Via，
不是配置场景）；map 与 probe 的既有形态
（`internal/client/http.go` `WithProbeHeaders` 的 `map[string]string`）
一致，UI 也天然是 N 行 KV。选 map。三级同一形态，合并函数只写一个。

### 3.2 Model flags 的 key

`"model_flags"` 是 `map[string]ProviderFlags`，key 为 **provider 侧
model ID 精确匹配**（即经过 rule 映射后、实际发给该 provider 的 model
名，与 `Provider.Models` 缓存列表同一命名空间）。理由：provider/model
flag 是供给侧配置，只应认识 provider 自己的模型词汇，不认识客户端别名。
通配/前缀匹配（如 `gpt-5*`）留作后续扩展，registry 机制不受影响。

### 3.3 三级合并语义（Effective headers）

```go
// EffectiveExtraHeaders(p, model, ruleFlags):
//   provider 级 ∪ model 级 ∪ rule 级，同名时后者胜出：
//
//       provider  <  model  <  rule
//     （最泛化）              （最贴近本次请求）
```

优先级理由：粒度越贴近"这一次请求"越应胜出。provider 级是全局默认；
model 级是供给侧对默认的细化；rule 级表达"这一类客户端/用途"的显式
意图，是三者中最具体的，故最后写入。

对未来的非 map 型 flag（bool/enum），合并模式由 registry 按 flag 声明
（`MergeMode`：`merge` / `override`），不是全局规则；`extra_headers`
声明为 `merge`。这与 rule flags 的 `InheritanceMode`（scenario→rule 的
or/override）同构，但轴不同：`MergeMode` 描述 provider→model→rule 的
纵向叠加，命名分开以免混淆。

---

## 4. Flag Registry — 复用 rule flags 的唯一可信源模式

### Provider/Model 级：新增 `internal/typ/provider_flag_registry.go`

```go
// 复用 FlagSpec / FlagValueType / FlagOption（flag_registry.go）
// FlagSpec 增加两个字段（rule flags 不使用，零值忽略）：
//   Scope     FlagScope // "provider" | "model" | "both"
//   MergeMode string    // Scope == "both" 时必填："override" | "merge"

func ProviderFlagRegistry() []FlagSpec {
    return []FlagSpec{
        {
            Key:         "extra_headers",
            Label:       "Custom Headers",
            Description: "Append custom HTTP headers to outbound requests ...",
            Type:        FlagValueHeaders, // 新增的 value type，见下
            Category:    FlagCategoryRequest,
            Scope:       "both",
            MergeMode:   "merge",
        },
    }
}
```

### Rule 级：`RuleFlagRegistry()` 追加一条

```go
{
    Key:         "extra_headers",
    Label:       "Custom Headers",
    Description: "Append custom HTTP headers to outbound requests for this rule ...",
    Type:        FlagValueHeaders,
    Category:    FlagCategoryRequest,
    // 无 Shared / InheritanceMode：不做 scenario 级（首版无此需求），
    // 与 provider/model 级的合并发生在 EffectiveExtraHeaders（§3.3），
    // 不走 resolveRuleFlagsWithScenario 的 scenario 继承链。
}
```

新增 `FlagValueType`：`FlagValueHeaders = "headers"`（值形态
`map[string]string`，UI 渲染为可增删的 KV 行编辑器）。该控件类型是
一次性投入，rule / provider / model 三处 UI 共用同一组件。

**约束测试**（镜像 `flag_registry_test.go` 的既有护栏）：

- ProviderFlagRegistry 每个 Key 必须对应 `ProviderFlags` struct 的某个
  json tag（`TestProviderFlagRegistry_KeysMatchStructFields`）；
  rule 级 `extra_headers` 由既有 `TestRuleFlagRegistry_KeysMatchStructFields`
  自动覆盖。
- `Scope` 必须在枚举内；`Scope == "both"` 必须声明 `MergeMode`，
  其他 Scope 不得声明。
- `headers` 类型不允许 Options/Suggestions。

Registry 通过新 endpoint 透出给前端（§7），前端 registry-driven 渲染，
**不为任何 flag 写 per-flag switch/case**（复用 rule flags 已验证的
架构，见 rule-flags.md §9）。

---

## 5. `extra_headers` 的注入与安全边界

### 5.1 注入点：transport 层

rule 级 `extra_headers` 已由统一的 `ruleFlagTransport`
（`internal/client/rule_flag_transport.go`，见 rule-flags.md §5 Type 2）
承载：pass-through 构造器（openai / anthropic / google）显式挂载，
vendor 链不挂。provider 级 headers（本文档规划部分）沿同一策略加一层：

```
wrapWithLogging( providerHeadersTransport( ruleFlagTransport( base ) ) )
                 └── 新增：读 provider 级 headers（构造期解出，不依赖 ctx）
```

`providerHeadersTransport`：

- **api_key 守卫（首版范围）**：`!provider.IsAPIKey()` 时该 transport
  为纯透传（no-op）。发布范围由这一个判断收口（配置入口另在 API/UI 层
  拦截，见 §5.4），后续放开 = 调整这一处守卫 + 对应校验。
- **provider 级 headers**：构造时从 provider 解出（`GetProviderFlags`），
  每个 RoundTrip 应用。不依赖 ctx —— 因此 probe、model-list 拉取、
  visionproxy 等不经 protocol dispatch 的路径也自动生效。
- **model 级 + rule 级 headers**：dispatch 阶段（rule→provider→model
  已定型）计算 `EffectiveExtraHeaders(p, model, ruleFlags)` 与 provider
  级的差集，经 Type 2 手法写入 request
  ctx——rule 级 headers 是 RuleFlags 的字段，随 `typ.WithRuleFlags`
  整包挂 ctx，transport 读 `typ.GetRuleFlags(ctx).ExtraHeaders`；transport 读到则叠加（合并优先级已在 dispatch 侧算好，transport
  只做"provider 级打底、ctx 覆盖"两步）。ctx 缺失时退化为仅 provider
  级 —— 这是非 dispatch 路径（无 rule、无 model 语境）的预期行为。

rule 级选择与 model 级共用同一个 ctx key 而不是单独一个：transport 只
认"本次请求的增量 header"一个概念，三级优先级是 dispatch 侧
`EffectiveExtraHeaders` 的职责——合并逻辑集中一处，transport 保持哑。

### 5.2 顺序不变量：vendor pin 永远胜出（对首版是防御，For 未来是边界）

首版只释放 api_key，generic 链上没有 vendor pin，本节在 v1 实际不触发；
但机制上 `providerHeadersTransport` 挂在所有构造器的统一包装点，**顺序
不变量从第一天就要成立**，为后续放开 OAuth 特种链保底：

```
providerHeadersTransport      ← 外层：先写入用户 extra headers
  vendorRoundTripper / SDK    ← 内层（更靠近 wire）：后写入 vendor pin
    wire                          同名时后写者胜 → vendor pin 决定性
```

`.design/user-agent.md` 的结论在这里同样成立：vendor 特种链上由握手 /
指纹协议绑定的 header（UA、`x-stainless-*`、`X-Msh-*`、
`X-ChatGPT-Account-ID`、session header 等）不可被任何用户配置覆盖。
护栏：给该顺序写回归测试（stub inner 断言最终 header 值）；并延续
rule-flags.md §8 的警告——**vendor 链内部永远不得引入
providerHeadersTransport**。

### 5.3 Header 校验 —— 用户主导，只查结构

Extra headers 是**纯用户主导**的编排配置："custom" 就意味着我们无法预判的
特殊需求，所以**没有 denylist、没有数量/尺寸上限**——`Authorization`、
`User-Agent`、`anthropic-version`……任何 header 都可以配，配错自负
（评审定调：把 tingly-box 当编排器，可任意配置，不过度设计）。

保存时（API 层）仍拒绝两类**结构性**问题——它们不是限制，而是"这个配置
本身无定义"：

- 名字必须是合法 RFC 7230 token、值必须是合法 field value（否则
  net/http 在发送时才失败，报错更晦涩）；保存时规范化为 canonical 形式
  （`textproto.CanonicalMIMEHeaderKey`）。
- 大小写不敏感的重名拒绝（HTTP header 名大小写不敏感，一个 map 里同名
  两个拼写没有确定胜者）。

与网关自管 header 的冲突**由 transport 链顺序决定，而不是过滤**：
vendor pin 与 UA 链在更内层（更靠近 wire）后写后胜（§5.2）；通用链上
用户配置的 `Authorization` 等则会覆盖网关默认——这正是用户显式表达的
意图。

### 5.4 各 provider 形态的行为（首版发布范围）

**首版只对 `api_key` auth type 释放**（含空 auth type 的向后兼容语义）。
范围由三道闸共同保证，各司其职：

1. **UI**：非 api_key provider 的编辑框不渲染 headers 入口（不是灰置——
   减少视觉噪音，不解释一个用不了的功能）；rule 级入口始终可见（rule
   不绑定 provider，同一 rule 可能路由到混合类型的 provider，见下表）。
2. **API 校验**：对非 api_key provider 保存 `flags` / `model_flags` 中的
   `extra_headers` → 拒绝并说明原因（防直接调 API / import 绕过 UI）。
3. **transport 守卫**：`!IsAPIKey()` 时 no-op（§5.1，防旧数据/竞态）。

| Provider 形态 | v1 行为 |
|---|---|
| generic api_key（OpenAI/Anthropic style） | 完整生效（provider + model + rule 级） |
| dual provider（api_key 的子集） | 两个 endpoint 同一份 headers（同一凭证同一供应商，不按 style 拆分；有真实需求再谈） |
| `no_key_required` 的自建端点 | 属 api_key 语义路径，生效 |
| OAuth / vendor 特种链（Claude Code、Codex、Kimi、Gemini、Antigravity） | **不释放**：无配置入口 + 校验拒绝 + transport no-op。rule 级 headers 命中此类 provider 时同样被 transport 守卫拦下（对用户的语义即"该 flag 仅对 API-key provider 生效"，写进 rule 级 flag 的 Description） |
| aws_sigv4 / azure_key / gcp_sa | **不释放**（SigV4 对参与签名的 header 敏感，注入未签名 header 可能直接 403；放开需单独验证） |
| vmodel | no-op（auth type 不在 api_key 发布范围）。实际已经 SDK + 私有 HTTP transport 调度，见 `vmodel-transport.md`；不能再用“无 HTTP”作为理由。UI 不展示入口 |
| builtin providers | 沿用既有规则：builtin 不可 mutate（只许 toggle Enabled），flags 同样锁定 |

---

## 6. 持久化

Provider / model 级沿用 `credential` / `vmodel_detail` 的既定先例
（`internal/db/provider_store.go`）：

- `ProviderRecord` 新增一个 `extensions` TEXT 列，内容为
  `ai.Provider.Extensions` 整个 map 的 JSON。
- GORM `AutoMigrate` 增列即生效，**无需 migration 脚本、无需回填**
  （零值 = 无扩展）。
- `toProvider` / `toRecord` / `updateRecordFromProvider` 三处映射函数
  各加一段 marshal/unmarshal（与 credential 的处理完全同形）。
- 服务侧不认识的 key（其他消费方存的任意扩展）随整个 map 原样存取，
  **update 时必须整 map 读-改-写，不得只写服务自己的两个 key**（否则
  丢别人的数据）。

Rule 级零持久化改动：RuleFlags 本就以 JSON 列随 rule 行存储
（rule-flags.md §2），加字段即可。

Provider export / import（`/provider-export`、`/provider-import`）随
Provider JSON 自然携带 extensions；import 侧对 `flags` / `model_flags`
两个 well-known key 执行与 API 相同的校验（§5.3 + §5.4 的 auth type
门），非法拒绝导入并报明确错误。

---

## 7. API 层

对外 API 是**服务的 surface**，暴露 typed 形态而非 opaque blob（opaque
容器是 ai module 与存储之间的事，不是 REST 契约）：

```
GET  /api/v1/provider/flags/registry     // ProviderFlagRegistry() → 前端渲染元数据
                                         // （与既有 GET /rule/flags/registry 同构）

// ProviderResponse 新增：
//   flags        *ProviderFlags                `json:"flags,omitempty"`
//   model_flags  map[string]ProviderFlags      `json:"model_flags,omitempty"`

// UpdateProviderRequest 新增（延续全 pointer 的 partial 语义）：
//   Flags       *ProviderFlags               `json:"flags,omitempty"`
//   ModelFlags  *map[string]ProviderFlags    `json:"model_flags,omitempty"`
//   —— nil = 不动；非 nil = 整体替换对应 well-known key（key 内部不做深合并，
//      语义与前端"整卡片保存"一致，避免 map 深 patch 的歧义）

// CreateProviderRequest 同步新增两个可选字段。
```

- handler 侧写入路径：读出 provider → 改 `Extensions["flags"]` /
  `Extensions["model_flags"]` → 保存（整 map 读-改-写，见 §6）。
- 校验集中在 `ValidateExtraHeaders`（§5.3）+ auth type 门（§5.4），
  Create / Update / Import 三处共用。
- `model_flags` 的 model key 不强校验存在于 `Provider.Models`（模型列表
  是缓存、可过期），但 UI 侧用列表辅助选择；对完全未知的 key 仅
  warning 不拒绝。
- **Rule 级零新增 endpoint**：`extra_headers` 走既有 rule 更新路径
  （`POST /rule/:uuid` 携带 flags），仅在 rule handler 的保存路径挂上
  同一个 `ValidateExtraHeaders`。registry 由既有
  `GET /rule/flags/registry` 自然透出（FlagSpec 新增的 `headers` 类型
  会出现在响应里，前端按类型渲染新控件）。
- swagger 定义补齐后 `task codegen` 重新生成 openapi.json 与前端 SDK。

---

## 8. 前端

复用 rule flags 的 registry-driven 架构（rule-flags.md §9），零 per-flag
switch/case。实现落点：

1. **新控件类型 `headers`（一次性投入，三处共用）**：
   `components/flags/HeadersEditor.tsx` —— 可增删的 KV 行编辑器,行内
   即时校验(denylist / 非法字符 / 大小写不敏感重复,直接标红并给出
   原因,教育内嵌于产品;denylist 与后端 `ValidateExtraHeaders` 镜像)。
   接入 `FlagCatalogDialog` 的类型分发(`spec.Type` 驱动,与 bool/
   string/enum/int/service_ref 并列新增一个分支)。
2. **Rule 级入口**：零布局改动——`extra_headers` 出现在既有
   `RulePluginsCard` / `FlagCatalogDialog`（新增通用 `request` 分类,
   排在分类栏 App 之后）。`RuleFlags` / `RuleFlagsApi` 加
   `extraHeaders` / `extra_headers`;`flagHelpers.ts` 补 `headers` 的
   isActive(map 非空)/ default(undefined)判定 + `headersValue`。
3. **Provider 级入口**：`ProviderFormDialog` 既有 **Advanced accordion**
   内(edit 模式)渲染 `ProviderPluginsBlock` —— 与 rule 侧同构的
   "Plugins 卡片 + 目录弹窗"交互:折叠卡片列出 active flag 与具体值,
   点击打开共享的 `FlagCatalogDialog`(标题 "Provider Plugins",registry
   来自 `GET /provider/flags/registry`,按 `Scope` 含 provider 过滤)。
   该编辑框本身只服务 api_key 域(oauth/cloud 走别的对话框),天然满足
   §5.4 的 UI 门。
4. **Model 级入口**：`ModelCard`(模型管理面)hover 工具条加
   `ModelHeadersTrigger`,点开锚定 **Popover**(`ModelHeaders.tsx`)
   内嵌 HeadersEditor 就地编辑保存;保存走"重新拉取 provider →
   读-改-写整个 model_flags map → PUT"防同会话兄弟模型互踩。有覆写的
   model 常驻左下 badge(tooltip 列出具体 header 名)——"surface the
   artifact"。`canEditModelHeaders` 按 api_key 门控。
5. **展示具体值原则**：所有折叠态显示真实 header 名列表（如
   `HTTP-Referer, X-Title`），不显示 "2 headers configured" 这类别名式
   摘要。
6. `useProviderEditDialog`:seed `flags: apiToFlags(provider.flags)`,
   `buildEditProviderPayload` 附带 `flags: flagsToApi(...)`(整对象
   替换语义与后端一致);`api.getProviderFlagRegistry` 镜像 rule 侧;
   MSW mock 补 provider registry endpoint 与 rule mock 的
   `extra_headers` 条目。

---

## 9. 测试计划

| 层 | 内容 |
|---|---|
| registry 护栏 | ProviderFlagRegistry Key ↔ `ProviderFlags` json tag 同步；rule 级由既有 KeysMatchStructFields 覆盖；Scope/MergeMode 约束 |
| 合并语义 | `EffectiveExtraHeaders`：仅 provider / 仅 model / 仅 rule / 三级同名覆盖顺序（provider < model < rule）/ 都为空 |
| 校验 | denylist、canonical 化、token 合法性、数量/尺寸上限；非 api_key provider 拒绝；三个写入口都过同一校验 |
| transport | headers 落到出站请求；ctx 叠加覆盖 provider 级；**非 api_key no-op 守卫**；**vendor pin 胜出的顺序回归**（为未来放开保底）；denylist 第二道防御；vmodel 路径 no-op |
| db | extensions 列 round-trip；含未知 key 的整 map 读-改-写不丢数据 |
| API | partial update 语义（nil 不动 / 非 nil 替换）；builtin 拒绝；rule 保存路径校验生效；export→import round-trip 含校验 |
| e2e（后补） | 三级各配一部分 header → mock upstream 断言合并结果（依赖 mock provider fixture，同 rule flags 的待办） |

---

## 10. 实施步骤（按序，勿乱序）

```
1. ai/provider.go
   └─ Provider 增加 Extensions map[string]json.RawMessage（含只读语义注释）

2. internal/constant
   └─ ProviderExtKeyFlags / ProviderExtKeyModelFlags 常量

3. internal/typ
   ├─ provider_flags.go：ProviderFlags struct + Get/Set 帮助函数
   │   （Set 系列负责整 map 读-改-写）+ EffectiveExtraHeaders（三级合并）
   ├─ type.go：RuleFlags 加 ExtraHeaders 字段（json/yaml tag: extra_headers）
   ├─ flag_registry.go：FlagSpec 增加 Scope / MergeMode 字段；
   │   FlagValueType 增加 "headers"；RuleFlagRegistry() 追加 extra_headers
   ├─ provider_flag_registry.go：ProviderFlagRegistry()
   └─ id.go：rule 级 headers 随 WithRuleFlags / GetRuleFlags 整包传递（Type 2）

4. internal/db/provider_store.go
   └─ extensions 列 + 三处映射（rule 侧零改动）

5. internal/client
   ├─ provider_headers_transport.go：providerHeadersTransport
   │   （含 IsAPIKey 守卫；校验仅查结构，不设 denylist，见 §5.3）
   └─ 挂载策略同 rule 级 ruleFlagTransport：pass-through 构造器显式挂载，vendor 链不挂

6. protocol dispatch
   └─ provider+model+rule 定型处调用 EffectiveExtraHeaders，
      与 provider 级的差集写入 ctx

7. internal/server/module/provider + module/rule
   ├─ provider/types.go / handler.go：请求响应模型 + ValidateExtraHeaders
   │   + api_key 门
   ├─ provider/routes.go：GET /provider/flags/registry + swagger 定义
   ├─ rule handler 保存路径挂 ValidateExtraHeaders
   └─ import/export 路径接同一校验

8. task codegen（openapi.json + 前端 SDK）

9. frontend
   ├─ headers KV 控件（FlagCatalogDialog 类型分发 + flagHelpers 扩展）
   ├─ RuleFlags / RuleFlagsApi 类型加字段（rule 级即完成）
   ├─ Provider 编辑框 Plugins 区块（registry-driven，仅 api_key 渲染）
   ├─ 模型列表 per-model flag 入口 + badge
   └─ buildEditProviderPayload 扩展

10. 测试随各层同 PR 落地（§9）；本文档随实现校正后去掉"规划稿"标记
```

---

## 11. 设计取舍

| 选项 | 已采纳 | 备择 | 取舍理由 |
|------|--------|------|----------|
| ai.Provider 扩展容器形态 | `map[string]json.RawMessage` | typed struct / `map[string]any` | ai 是公共 module，必须对内容不知情；RawMessage 无损且物理隔离 schema。typed struct 会把服务语义泄进公共 API |
| 服务侧 schema 位置 | internal/typ + registry | 散落各消费点 | 复刻 rule flags 的"唯一可信源"模式，前端零 switch/case，已被验证 |
| rule 级是否同步支持 | ✅ 三级统一 | 仅 provider/model | headers 三级各有真实语义（网关固有 / 模型灰度 / 客户端标记）；rule 级复用既有 RuleFlags 机制近乎零成本，一次做齐避免二次开口 |
| rule 级走 Extensions 还是 RuleFlags | RuleFlags 加字段 | Rule 也做 opaque 容器 | Rule 类型在服务域内，不存在跨 module schema 问题；opaque 容器只为公共 module 存在 |
| 三级合并顺序 | provider < model < rule | rule < model < provider | 粒度越贴近单次请求越应胜出；rule 是显式的请求侧意图 |
| 首版发布范围 | 仅 api_key | 全 auth type | vendor 特种链有握手/指纹边界、SigV4 有签名敏感性，验证成本高；api_key 覆盖绝大多数真实需求（OpenRouter/网关/自建端点）。范围由 UI 隐藏 + API 校验 + transport 守卫三道闸收口，放开时逐道解除 |
| extra_headers 形态 | `map[string]string` | `[]HeaderKV`（有序可重复） | 配置型 header 不需要重复/顺序；与 probe headers 先例一致；UI 简单；三级同形态合并函数唯一 |
| model flag 合并 | 按 flag 声明 MergeMode（headers=merge） | 一律 override | headers 的自然语义是叠加覆写；未来 bool flag 需要 override——语义属于 flag 本身，进 registry |
| 注入点 | transport 层（ruleFlagTransport，一处） | ClientPool 逐构造器传 option | option 类型按 SDK 分裂（openai/anthropic/google 各一套），transport 一处覆盖所有 auth type 与 style |
| model/rule 级 ctx 传递 | 单一 ctx key（dispatch 侧合并好） | 每级一个 ctx key | transport 保持哑，合并逻辑集中在 EffectiveExtraHeaders 一处 |
| vendor pin 冲突 | 物理顺序保证 pin 胜出 | 逐 header 判断 | v1 不触发（api_key only），但顺序不变量零维护成本，为放开 OAuth 保底 |
| denylist / 上限 | **不做**（用户主导，配错自负） | 拒绝 Authorization/UA/传输头 + 数量尺寸上限 | 评审定调：编排器必须可任意配置，"custom" 即特殊需求，过度限制是过度设计。与网关自管 header 的冲突交给 transport 链顺序（pin 在内层后写后胜），不做过滤 |
| 校验时机 | 保存时拒绝（仅结构合法性） | 发请求时静默失败 | 非法 token/重名在保存时报错比 net/http 发送期报错清晰；结构校验不是限制而是"配置无定义" |
| API 形态 | typed `flags`/`model_flags` 字段 | 直接暴露 opaque extensions | REST 契约是服务 surface，必须 typed；opaque 容器只是存储与公共 module 之间的运输形态 |
| 持久化 | 单 `extensions` JSON 列 | 每 flag 一列 | credential/vmodel_detail 先例；增 flag 零 DDL；provider 行不因 flag 增长而加宽 |
| model key 匹配 | 精确匹配 | 通配/前缀 | 首版从简；registry 与数据形态不阻碍后续加 pattern |
| UI 命名 | 复用 "Plugins" | 新词（Extensions/Custom Headers） | 词汇全局统一：同一交互心智（registry 目录 + 卡片）应同名；scope 差异由所在 surface（provider 编辑框 vs rule 卡片）表达 |
| 非 api_key 的 UI 呈现 | 隐藏入口 | 灰置 + 提示 | 减少视觉噪音：不解释一个当前用不了的功能；放开时再出现 |

---

## 12. 已决事项与遗留

**已决**（本次规划确认）：

- ✅ rule 级 `extra_headers` 同步加入，三级统一（provider < model < rule）。
- ✅ 首版仅释放 api_key auth type；OAuth / 多字段凭证 / vmodel 不释放，
  由 UI + API 校验 + transport 守卫三道闸收口。
- ✅ builtin provider 沿用"不可 mutate"规则，flags 锁定。

**遗留**（不阻塞实施）：

1. OAuth vendor 链放开时机与形态（依赖真实需求；机制与顺序不变量已就位）。
2. model key 通配/前缀匹配（依赖真实需求）。
3. scenario 级 extra_headers（当前无需求；若加，走 FlagSpec.Shared +
   InheritanceMode 的既有 scenario 继承链，与三级纵向合并正交）。
