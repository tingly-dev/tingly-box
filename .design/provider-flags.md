# Provider / Model / Rule Flags

> 状态：**分三段交付**（api_key-only 发布范围）：
> ① rule 级 `extra_headers`（含前端）—— 已合并 (#1589)；
> ② provider & model 级后端（Provider.Flags/ModelFlags、db 列、三级合并、provider API）—— 本分支；
> ③ provider & model 级前端（Plugins 区块、per-model popover）—— 独立 PR，可后续升级。
>
> ②③ 拆开的原因：后端能力（含 API 与 registry endpoint）可独立落地并被
> CLI / 直接调用 API 的使用者消费；UI 是纯增量，晚一步不影响功能可用性。
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

分层原则：

> **provider/model 级 flag 是"如何抵达这个上游"的属性，与 base URL、auth
> 一样属于 `ai.Provider`；rule/scenario 级 flag 是网关的产品行为，留在
> `internal/`。**

最初规划过一个不透明扩展容器（"对 ai module 而言是任意的"），实现后按
评审意见塌缩为 typed 字段——理由与代价见 §2。校验、注册表、合并语义仍全部
在 `internal/`，ai 只持有数据。

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
│  ai module（公共 API）                                        │
│                                                             │
│  ai.Provider {                                              │
│      ...                                                    │
│      Flags      ProviderFlags            `json:"flags"`      │
│      ModelFlags map[string]ProviderFlags `json:"model_flags"`│
│  }                                                          │
│                                                             │
│  type ProviderFlags struct {                                │
│      ExtraHeaders map[string]string                         │
│      CustomUserAgent / BlockTools / ThinkingEffort  …        │
│      UseMaxTokens / UseMaxCompletionTokens / SkipUsage       │
│      ClaudeCodeCompat / CleanHeader / Cursor* / Context1M    │
│      // 字段集是 RuleFlags 的供给侧镜像（见下）                  │
│  }                                                          │
└──────────────────────────┬──────────────────────────────────┘
                           │ typ.ProviderFlags = ai.ProviderFlags（别名）
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  tingly-box 服务（internal/）                                 │
│                                                             │
│  internal/typ:                                              │
│      RuleFlags 追加同形态字段：                                │
│          ExtraHeaders map[string]string `json:"extra_headers"`│
│      SupplyExtraHeaders(p, model)   // provider ∪ model（§3.3）│
│      PruneModelFlags(m)             // 清空的 model 不留空对象  │
│                                                             │
│  internal/typ/provider_flag_registry.go:                    │
│      ProviderFlagRegistry() []FlagSpec  // provider/model 级可信源（§4）│
│  internal/typ/flag_registry.go:                             │
│      RuleFlagRegistry() 追加 extra_headers 条目（rule 级）      │
└─────────────────────────────────────────────────────────────┘
```

访问方式与 rule flags 完全一致：`p.Flags.ExtraHeaders` / `rule.Flags.ExtraHeaders`，
没有访问器、没有容器、没有 well-known key。

### 为什么字段直接放在 ai（而不是不透明扩展容器）

最初的规划是给 `ai.Provider` 一个 `Extensions map[string]json.RawMessage`
不透明容器，服务在两个 well-known key 下放自己的 typed schema——理由是
"对 ai module 而言扩展字段是任意的"。实现出来后评审判定这是过度设计，
改为 typed 字段，依据是：

1. **容器只有一个消费者**。除 provider flags 外没有任何代码读写它；为
   "保护别人的 key"而做的读-改-写和它的测试（测试里的 key 字面就叫
   `someone_elses_key`）保护的是一个假想消费者——与被删掉的
   `Scope`/`MergeMode` 是同一种投机，只是体量更大。
2. **ai 的依赖独立性不受影响**。`ProviderFlags` 内部只是
   `map[string]string`，不引入任何新 import，ai 仍然零依赖 `internal/`。
3. **语义上并不越界**。ai 自述的职责就是"provider + 如何抵达这个上游"，
   它已经装着 `OpenAIEndpointMode`、`VModelDetail`、`CredentialBundle`、
   `Issuer`；`extra_headers`（OpenRouter 的 `HTTP-Referer`、网关租户头）
   正属于这一类，而不是网关的产品行为。

塌缩省掉约 135 行：5 个访问器 + `setExtension` 的读-改-写、db 的
`cloneExtensions`、以及三个纯容器测试（外键保序 / 畸形 JSON 兜底 /
extensions wire shape）。

**代价与护栏**：ai 从此认识 flag schema，新增 provider flag 要动公共
module；真正的风险是先例——一个开放的 `Flags` 结构会招揽产品语义。

护栏最初写成一条准入规则（"ai 只接受描述如何抵达上游的字段"）。实施到
一半发现它把 provider flags 卡死在了一个 flag 上：按字面执行，
`use_max_tokens`（老 provider 不认新字段名）、`claude_code_compat`
（第三方 Anthropic 兼容端拒绝 system role）这类**明明是上游属性、描述
里自己就写着 "providers" / "model family"** 的 flag 也被挡在门外，用户
只能在每一条 rule 上重复配置同一个上游的怪癖。于是护栏换成一条更强也更
好执行的规则：

> **`ProviderFlags` 的字段集是消费者请求侧 flag 集（`typ.RuleFlags`）的
> 供给侧镜像。同一个 knob 在哪一级都是同一个意思，只有"作用范围"不同；
> 不为供给侧发明新词汇。**

这条规则同时解决了准入和命名两个问题：能不能加，看请求侧有没有这个
knob；叫什么，就叫请求侧那个名字。它把"ai 认识产品语义"的风险从"任意
膨胀"收窄成"跟随一个已存在的、被评审过的集合"。

**没有镜像的三类**（`ProviderFlagRegistry` 的注释里同样列着，
`TestProviderFlagRegistry_ExcludesRequestOnlyFlags` 钉住）：

| flag | 不镜像的理由 |
|------|--------------|
| `session_affinity` / `vision_proxy_service` | 在**选中上游之前**就已消费（负载均衡 / 入站改写），挂在上游身上的值永远读不到 |
| `openai_endpoint_override` | provider 已有一等字段 `OpenAIEndpointMode` 表达同一件事，再开一个控件就是重复 mode picker（ux-principles） |
| `claude_org_id` | Claude OAuth 专用，而 provider flags 首版仅 api_key |

Rule 级不受影响：RuleFlags 本来就是服务域内的 typed struct（rule-flags.md
§3），直接加字段即可。

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

### 3.3 三级合并语义

```
供给侧（客户端构造期，按 (provider, model) 缓存）：
    SupplyExtraHeaders(p, model) = provider 级 ∪ model 级，model 胜

请求侧（每次 RoundTrip）：
    GetRuleFlags(ctx).ExtraHeaders                 ← rule 级

最终优先级 = transport 的写入顺序：
    provider  <  model  <  rule
  （最泛化）              （最贴近本次请求）
```

优先级理由：粒度越贴近"这一次请求"越应胜出。provider 级是全局默认；
model 级是供给侧对默认的细化；rule 级表达"这一类客户端/用途"的显式
意图，是三者中最具体的，故最后写入。

**headers 没有"三级合并函数"**：供给侧两级在构造期合并一次，rule 级在写
header 时叠加，`req.Header.Set` 的 canonical 化保证同名（含大小写不同）
覆盖。详见 §5.1。

**其余 flag 走一次显式合并**：它们不是 header，注入点在 transform 链和
SDK 层（rule-flags.md 的 Type 1b-pre / 1b-post / Type 2），没有"写入
顺序"可借。合并放在 `typ.ApplyProviderFlags(flags, provider, model)`，
由 `ResolveRuleFlagsWithScenario` 这一个点调用——即 rule/scenario 合并
之后、`applyRuleFlags` 写进 ctx 之前：

```
rule.Flags ──┬─ + scenario 继承 ──┬─ + ApplyProviderFlags(provider, model) ──┬─ ctx
             │  （既有）           │   （供给侧，最低优先级）                    │
             └────────────────────┴──────────────────────────────────────────┘
                                        ↓
                        RulePreBaseTransforms / RulePreVendorTransforms /
                        outbound clients / response 处理 —— 全部免费拿到
```

合并语义按**值的种类**统一，不按 flag 声明：

| 种类 | 语义 |
|------|------|
| bool | 三级 OR（任一级打开即生效） |
| 标量（string / enum / int） | 取最窄的非零值：rule → model → provider |
| map（headers） | 逐 key 合并，窄的一级赢同名 key（走 §5.1 的写入顺序） |

因此 `FlagSpec` 上不需要 `Scope` / `MergeMode` 两个轴（曾经加过又删掉：
唯一的 flag 两级都有、只有一种合法合并，无任何生产读取方）。bool 用 OR
而不是"窄级可以关掉宽级"，与既有的 scenario→rule `InheritanceMode: "or"`
一致；真出现需要"下级关闭上级"的 flag 时再引入三态，届时有真实约束可依。

**为什么合并在 dispatch 侧而不是各注入点**：`ResolveRuleFlagsWithScenario`
已经是"本次请求的有效 flag"的唯一收敛点（scenario 继承、cursor 自动检测、
CleanHeader 自动应用与 OAuth 抑制都在这里），供给侧插在同一处，下游没有
任何一个消费点需要知道 flag 是从哪一级来的。代价是该函数多了一个 `model`
形参（四个 attempt 入口各自把已解析的 provider 侧 model ID 传进来）。

---

## 4. Flag Registry — 复用 rule flags 的唯一可信源模式

### Provider/Model 级：新增 `internal/typ/provider_flag_registry.go`

既然字段集是 RuleFlags 的镜像（§2），registry 也不重抄一遍：provider
registry **由 rule registry 派生**，本文件里只写一张 `key → 供给侧文案`
的表，其余（Label / Type / Category / Placeholder / Options /
Suggestions）全部取自同 key 的 rule spec：

```go
// providerFlagDescriptions：按展示顺序列出可在 provider/model 级配置的
// flag + 它的供给侧描述；不在表里的 key 就是"没有供给侧语义"（§2 三类）。
func ProviderFlagRegistry() []FlagSpec {
    // 取同 key 的 rule spec → 换掉 Description → 清掉 scenario 轴
    // （Shared / InheritanceMode 属于 rule 轴）
}
```

这样两个面渲染的控件天生一致（同一个 knob 不会一边是下拉一边是文本框），
新增 rule flag 时若要开到供给侧，只需在表里加一行文案。
`TestProviderFlagRegistry_SharesRuleControlShape` 钉住"控件同形、文案不同"，
`TestProviderFlagRegistry_KeysExistInRuleRegistry` 钉住表里不会出现 rule
registry 没有的 key（否则会被静默丢弃）。

**每个 spec 都是 provider + model 两级可配**（合并语义见 §3.3）。不做
per-spec 的 scope 声明：两个 UI 面都渲染整个 registry。

### Rule 级：`RuleFlagRegistry()` 追加一条

```go
{
    Key:         "extra_headers",
    Label:       "Custom Headers",
    Description: "Append custom HTTP headers to outbound requests for this rule ...",
    Type:        FlagValueHeaders,
    Category:    FlagCategoryRequest,
    // 无 Shared / InheritanceMode：不做 scenario 级（首版无此需求）。
    // 与 provider/model 级的叠加发生在出站 transport（§5.1），
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
- `headers` 类型不允许 Options/Suggestions。

Registry 通过新 endpoint 透出给前端（§7），前端 registry-driven 渲染，
**不为任何 flag 写 per-flag switch/case**（复用 rule flags 已验证的
架构，见 rule-flags.md §9）。

---

## 5. `extra_headers` 的注入与安全边界

### 5.1 注入点：统一的 `ruleFlagTransport`，供给侧静态 + rule 级随 ctx

上游把出站 rule flag 收敛成一个 ctx key + 一个 transport
（`internal/client/rule_flag_transport.go`，见 rule-flags.md §5 Type 2）：
pass-through 构造器（openai / anthropic / google）显式挂载 `wrapWithRuleFlags`，
vendor 链一律不挂。三级 headers **复用这一层**，不新增 transport：

```
wrapWithRuleFlags(base, provider, model, resolveUA)
   ├── 构造期：supplyHeaders = typ.SupplyExtraHeaders(provider, model)   ← 供给侧两级
   └── RoundTrip：先写 supplyHeaders，再写 GetRuleFlags(ctx).ExtraHeaders ← rule 级
```

关键点：

- **供给侧（provider ∪ model）在构造期解出**。client 本就按
  `(provider, model)` 缓存（`ClientKey`），三个挂载点全都持有 `model`，
  所以模型级 headers 不需要经过 dispatch——`typ.SupplyExtraHeaders` 在
  wrap 时算一次，之后每个 RoundTrip 直接用，JSON 不重复解码。
  不依赖 ctx 也意味着 probe、model-list 拉取、visionproxy 等不经
  protocol dispatch 的路径同样带上上游配置的 header。
- **rule 级仍是 rule flag**，随 `typ.WithRuleFlags` 整包挂 ctx，transport
  读 `typ.GetRuleFlags(ctx).ExtraHeaders`。**不把三级合并塞进这个 ctx
  值**：它的契约是"本次请求解析出的 RuleFlags"，混入供给侧配置会让
  `X-Tingly-Applied-Flags` 之类的诊断把 provider 配置报成 rule flag。
- **优先级由写入顺序产生，没有第四方合并函数**：supply 先写、rule 后写、
  UA 最后写，`req.Header.Set` 自带 canonical 化，于是
  `provider < model < rule < UA 解析 < 内层链` 自然成立。
- **api_key 守卫沿用同一处**：`wrapWithRuleFlags` 在 wrap 时判定
  `provider.IsAPIKey()`，false 则 supply headers 根本不装载（且当该链也
  不需要 UA 解析时直接返回 inner）。发布范围由这一个判断收口，配置入口
  另在 API 层拦截（§5.4）。

### 5.2 顺序不变量：vendor pin 永远胜出

vendor 链（Claude OAuth / Codex / Kimi / Gemini / Antigravity）**根本不挂
这一层**——上游重构后这条边界由结构保证，不再依赖注释里的不变量，因此
任何级别的 headers 都到不了 vendor 握手。

在挂载了该层的 pass-through 链上，内层（更靠近 wire）仍然后写后胜：

```
ruleFlagTransport             ← 外层：supply → rule → UA
  SDK / inner round-tripper   ← 内层：后写者胜
    wire
```

`.design/user-agent.md` 的结论同样成立：握手 / 指纹绑定的 header
（UA、`x-stainless-*`、`X-Msh-*`、`X-ChatGPT-Account-ID`、session header）
不可被用户配置覆盖。护栏：`rule_flag_transport_test.go` 里的
`TestRuleFlagTransport_VendorPinWins`（stub inner 断言最终值）+
**vendor 链内部永远不得挂 `wrapWithRuleFlags`**。

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
| vmodel | no-op（无出站 HTTP）。UI 不展示入口 |
| builtin providers | 沿用既有规则：builtin 不可 mutate（只许 toggle Enabled），flags 同样锁定 |

---

## 6. 持久化

Provider / model 级沿用 `credential` / `vmodel_detail` 的既定先例
（`internal/db/provider_store.go`）：

- `ProviderRecord` 新增 `flags` / `model_flags` 两个 TEXT 列
  （`serializer:json`，与 `credential` / `vmodel_detail` 同模式）。
- GORM `AutoMigrate` 增列即生效，**无需 migration 脚本、无需回填**
  （零值 = 未配置）。
- `toProvider` / `toRecord` / `updateRecordFromProvider` 三处映射各加两行
  clone（与其他 JSON 列一样做缓存隔离）。

Provider export / import（`/provider-export`、`/provider-import`）随
Provider JSON 自然携带这两个字段；import 侧对 `flags` / `model_flags`
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

- handler 侧写入路径：读出 provider → 赋 `p.Flags` / `p.ModelFlags` → 保存。
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
   即时校验(非法字符 / 大小写不敏感重复,直接标红并给出原因——仅结构
   校验,无 denylist,与后端 `ValidateExtraHeaders` 镜像,见 §5.3)。
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
   来自 `GET /provider/flags/registry`)。
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
| registry 护栏 | ProviderFlagRegistry Key ↔ `ProviderFlags` json tag 同步；rule 级由既有 KeysMatchStructFields 覆盖 |
| 合并语义 | `SupplyExtraHeaders`：仅 provider / 仅 model / model 不匹配 / 大小写碰撞 / 都为空；transport 侧再验三级同名覆盖顺序（provider < model < rule）|
| 校验 | 仅结构性:token/值合法性、大小写不敏感重名、canonical 化(无 denylist/上限,§5.3);非 api_key provider 拒绝;三个写入口都过同一校验 |
| transport | headers 逐字落到出站请求(无过滤);ctx 叠加覆盖 provider 级;**非 api_key no-op 守卫**;**vendor pin 胜出的顺序回归**(为未来放开保底);vmodel 路径 no-op |
| db | flags / model_flags 列 round-trip；缓存隔离（调用方改动不回灌 store）|
| API | partial update 语义（nil 不动 / 非 nil 替换）；builtin 拒绝；rule 保存路径校验生效；export→import round-trip 含校验 |
| e2e（后补） | 三级各配一部分 header → mock upstream 断言合并结果（依赖 mock provider fixture，同 rule flags 的待办） |

---

## 10. 实施步骤（按序，勿乱序）

```
1. ai/provider.go
   └─ Provider 增加 Flags / ModelFlags + ProviderFlags struct（含准入规则注释）

2. internal/constant
   └─ ProviderExtKeyFlags / ProviderExtKeyModelFlags 常量

3. internal/typ
   ├─ provider_flags.go：ProviderFlags struct + Get/Set 帮助函数
   │   （Set 系列负责整 map 读-改-写）+ SupplyExtraHeaders（供给侧合并）
   ├─ type.go：RuleFlags 加 ExtraHeaders 字段（json/yaml tag: extra_headers）
   ├─ flag_registry.go：FlagValueType 增加 "headers"；RuleFlagRegistry()
   │   追加 extra_headers（FlagSpec 本身不加任何 provider 专用字段）
   ├─ provider_flag_registry.go：ProviderFlagRegistry()
   └─ （ctx 侧无需新增：rule 级 headers 已随 typ.WithRuleFlags 整包传递）

4. internal/db/provider_store.go
   └─ flags / model_flags 两列 + 三处映射（rule 侧零改动）

5. internal/client
   ├─ rule_flag_transport.go：wrapWithRuleFlags 增 model 入参
   │   （含 IsAPIKey 守卫;逐字应用,无过滤）
   └─ 在 wrapWithLogging 接入点外侧统一挂载（一处改动覆盖全部构造器）

6. protocol dispatch —— 无需改动
   └─ 旧规划让 dispatch 计算三级合并再写 ctx；上游把出站 rule flag 收敛成
      单一 ctx key 后，供给侧改在 client 构造期解出，`ResolveRuleFlagsWithScenario`
      与 4 个 handler 的签名都不用动

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
| provider flag 的存放位置 | `ai.Provider` 上的 typed 字段 | `Extensions map[string]json.RawMessage` 不透明容器 + 服务侧 well-known key | 容器只有一个消费者，"保护别人 key"的读-改-写保护的是假想消费者（评审意见）。塌缩省 ~135 行；ai 的依赖独立性不变（不引入新 import），语义上 headers 本就属于"如何抵达上游"。代价是 ai 认识 flag schema，用准入规则约束（§2）|
| provider flag 的字段集 | RuleFlags 的供给侧镜像（12 个），排除三类无供给侧语义的 | 只做 `extra_headers` / 自定义一套供给侧词汇 | 只做 headers 把"上游怪癖"留在了 rule 上重复配置——`use_max_tokens`、`claude_code_compat` 的描述里自己就写着 providers / model family（评审意见：整个 provider flag 系统没实现完）。镜像同时解决准入与命名：能不能加看请求侧有没有，叫什么就叫请求侧的名字 |
| provider registry 的来源 | 由 `RuleFlagRegistry()` 派生，只覆写 Description | 手写 12 条完整 spec | 同一 knob 两个面必须渲染同一控件；派生让"漂移"在结构上不可能发生，新增一条只写一行文案 |
| 非 header flag 的合并 | dispatch 侧一次 `ApplyProviderFlags`，在 `ResolveRuleFlagsWithScenario` 内 | 各注入点自行读三级 / 像 headers 一样靠写入顺序 | 它们注入在 transform 链与 SDK 层，没有"写入顺序"可借；而请求的有效 flag 已有唯一收敛点，插在那里下游零改动。代价：该函数多一个 `model` 形参 |
| bool 的三级语义 | 三级 OR | 三态（`*bool`，窄级可关掉宽级） | 与既有 scenario→rule `InheritanceMode: "or"` 一致，零新概念；三态要给每个 bool 加指针 + UI 加"继承/开/关"三档，为一个尚未出现的需求付全额成本 |
| 服务侧 schema 位置 | internal/typ + registry | 散落各消费点 | 复刻 rule flags 的"唯一可信源"模式，前端零 switch/case，已被验证 |
| rule 级是否同步支持 | ✅ 三级统一 | 仅 provider/model | headers 三级各有真实语义（网关固有 / 模型灰度 / 客户端标记）；rule 级复用既有 RuleFlags 机制近乎零成本，一次做齐避免二次开口 |
| 三级合并顺序 | provider < model < rule | rule < model < provider | 粒度越贴近单次请求越应胜出；rule 是显式的请求侧意图 |
| 首版发布范围 | 仅 api_key | 全 auth type | vendor 特种链有握手/指纹边界、SigV4 有签名敏感性，验证成本高；api_key 覆盖绝大多数真实需求（OpenRouter/网关/自建端点）。范围由 UI 隐藏 + API 校验 + transport 守卫三道闸收口，放开时逐道解除 |
| extra_headers 形态 | `map[string]string` | `[]HeaderKV`（有序可重复） | 配置型 header 不需要重复/顺序；与 probe headers 先例一致；UI 简单；三级同形态合并函数唯一 |
| model flag 合并 | 层级属性：model 级同名覆盖 provider 级 | 每个 flag 用 registry 的 `MergeMode` 声明 | 曾按 flag 声明（Scope/MergeMode 两个轴），但唯一的 flag 两级都有、只有 merge 合法，无任何生产读取方——纯预留结构，已删。有语义不同的 flag 时再加，届时有真实约束 |
| 注入点 | transport 层（wrapWithLogging 旁，一处） | ClientPool 逐构造器传 option | option 类型按 SDK 分裂（openai/anthropic/google 各一套），transport 一处覆盖所有 auth type 与 style |
| 供给侧 headers 的解析时机 | client 构造期（wrap 时按 (provider, model) 解出） | dispatch 侧算好三级合并写进 ctx | client 本就按 provider+model 缓存，构造期解一次即可，JSON 不重复解码；且 rule flags 的 ctx key 契约保持"只装 RuleFlags"，诊断不会把 provider 配置报成 rule flag。代价是优先级由写入顺序表达，需在 transport 注释与测试中写清 |
| vendor pin 冲突 | 物理顺序保证 pin 胜出 | 逐 header 判断 | v1 不触发（api_key only），但顺序不变量零维护成本，为放开 OAuth 保底 |
| denylist / 上限 | **不做**（用户主导，配错自负） | 拒绝 Authorization/UA/传输头 + 数量尺寸上限 | 评审定调：编排器必须可任意配置，"custom" 即特殊需求，过度限制是过度设计。与网关自管 header 的冲突交给 transport 链顺序（pin 在内层后写后胜），不做过滤 |
| 校验时机 | 保存时拒绝（仅结构合法性） | 发请求时静默失败 | 非法 token/重名在保存时报错比 net/http 发送期报错清晰；结构校验不是限制而是"配置无定义" |
| API 形态 | typed `flags`/`model_flags` 字段 | —— | 与存储、与 ai 的字段同形，DTO 不再需要编解码 |
| 持久化 | `flags` / `model_flags` 两个 JSON 列 | 每 flag 一列 | credential/vmodel_detail 先例；增 flag 零 DDL |
| model key 匹配 | 精确匹配 | 通配/前缀 | 首版从简；registry 与数据形态不阻碍后续加 pattern |
| UI 命名 | 复用 "Plugins" | 新词（Custom Headers 等） | 词汇全局统一：同一交互心智（registry 目录 + 卡片）应同名；scope 差异由所在 surface（provider 编辑框 vs rule 卡片）表达 |
| 非 api_key 的 UI 呈现 | 隐藏入口 | 灰置 + 提示 | 减少视觉噪音：不解释一个当前用不了的功能；放开时再出现 |

---

## 12. 已决事项与遗留

**已决**（本次规划确认）：

- ✅ rule 级 `extra_headers` 同步加入，三级统一（provider < model < rule）。
- ✅ 首版仅释放 api_key auth type；OAuth / 多字段凭证 / vmodel 不释放，
  由 UI + API 校验 + transport 守卫三道闸收口。
- ✅ builtin provider 沿用"不可 mutate"规则，flags 锁定。
- ✅ `ProviderFlags` 的字段集是 `RuleFlags` 的供给侧镜像，而非仅
  `extra_headers`；准入规则从"只接受如何抵达上游"改为"镜像请求侧集合"
  （§2）。三类无供给侧语义的 flag 显式排除并有测试钉住。
- ✅ 非 header flag 的三级合并落在 `typ.ApplyProviderFlags`，由
  `ResolveRuleFlagsWithScenario` 单点调用；bool 三级 OR，标量取最窄非零值。

**遗留**（不阻塞实施）：

1. OAuth vendor 链放开时机与形态（依赖真实需求；机制与顺序不变量已就位）。
2. model key 通配/前缀匹配（依赖真实需求）。
3. scenario 级 extra_headers（当前无需求；若加，走 FlagSpec.Shared +
   InheritanceMode 的既有 scenario 继承链，与三级纵向合并正交）。
4. 供给侧 flag 的"生效范围"提示。镜像集合里有几个 flag 只在特定形态的
   上游上有意义（`context_1m` 只对 Anthropic、`use_max_*` 只对 OpenAI、
   `cursor_compat*` 取决于入站客户端）。当前**不加门禁**：配在用不上的
   地方就是静默无效，与 rule 级同一行为，也符合"编排器可任意配置、配错
   自负"的定调。若后续要提示，registry 已有 `Category`（如
   `request_openai` / `request_anthropic`）可直接驱动 UI 分组或灰置，
   不需要新的数据结构。
5. 前端 Plugins 目录当前只渲染 headers 控件（PR #1591 的范围）；registry
   现在返回 bool / string / enum 三种 Type，前端需补齐对应控件才能把这
   一批 flag 露出——后端契约已就位，属纯 UI 工作。
