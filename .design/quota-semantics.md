# Quota 语义归一（Semantic Normalization）

> 适用对象：改 `ai/quota/**`、`internal/smart_routing/**`、`internal/server/module/statusline`、
> `frontend/src/types/quota.ts` 的贡献者。
> 本文档描述「多 provider 配额从**结构归一**走向**语义归一**」的模型、归一规则、判定接口与迁移路径。

---

## 1. 问题：结构归一 ≠ 语义归一

`ai/quota` 已经把 15 个 provider 的配额塞进了同一个结构体 `UsageWindow{Used, Limit, UsedPercent, Unit, Type, ResetsAt}`。
结构是统一的，**含义不是**。每个 fetcher 都在用同一组字段回答不同的问题。

后果：任何跨 provider 的**判定**都不成立——

- **smart op 判定**：想写「这个 provider 还剩 20% 以上就走它」，但 `used_percent` 在 A 是"额度耗尽程度"、
  在 B 是"某模型占总消耗的比例"、在 C 是"跨模型的算术平均"。同一个阈值在三家意味着三件事。
- **混合资源池判定**：一个 rule 下挂 Anthropic(percent) + MiniMax(requests) + OpenRouter($) + Copilot(不可观测)，
  没有任何一个量能把它们放在一起比。
- **失败与空闲不可分**：Copilot / Cursor / VertexAI 返回空 `Windows` + `LastError`，
  消费方看到的和"额度充足、一点没用"完全一样。

**根因**：`UsageWindow` 是给"画进度条"设计的。判定需要的是另外三个问题的答案，而这三个问题从来没有被建模。

---

## 2. 现状证据

以下都是当前代码的真实行为，不是假设。

### 2.1 `Used` 至少有四种含义

| 位置 | `Used` 实际是什么 |
|---|---|
| `fetcher/anthropic.go:131` | **百分数本身**（`Used=utilization, Limit=100`） |
| `fetcher/minimax_shared.go:86` | `total - usage_count`，即从"剩余计数"反推的消耗 |
| `fetcher/kimik2.go:122` | 已消耗额度，`Limit` 是 `consumed+remaining` **合成**出来的总量 |
| `fetcher/codex.go:324` | `Cost.Limit = balance` —— **余额被写进了 Limit 字段**，而 Anthropic 的 `Cost.Limit` 是月度上限 |

任何对 `Used` 做求和、比较、阈值判断的代码都是错的。

### 2.2 `UsedPercent` 同时表示"耗尽程度"和"构成占比"

`fetcher/zai_shared.go:248`：
```go
modelPercent = (detail.Usage / lim.CurrentValue) * 100   // 该模型占「已消耗量」的比例
...
UsedPercent: modelPercent,                               // 写进了「耗尽程度」字段
```
前端 `QuotaBarItem` 对 `used_percent >= 80` 染红。于是"某模型占了本周消耗的 85%"会被渲染成"快耗尽了"。
这是最典型的**一个字段承载两个概念**（违反 ux-principles #3）。

### 2.3 跨 bucket 用算术平均，掩盖耗尽

`fetcher/gemini.go:124`：
```go
avgUsedPercent := totalUsedPercent / float64(len(quotaResp.Buckets))
```
`gemini-2.5-pro` 100% 耗尽 + `gemini-2.5-flash` 0% → 聚合显示 50%。
用它做路由判定会把一个**已经不能服务 pro 请求**的 provider 判成"半满"。

### 2.4 `Limit == 0` 三义

`types.go:111` 注释说"0 means unlimited"，但实际有三种来源：
- 真·无限制（OpenRouter 未设 key limit，`openrouter.go:134`）
- **不可知**（OpenAI 没有 limit API，`openai.go:137`）
- 无额度

消费方一律按 `Limit > 0` 过滤（`statusline/handler.go:378`、`350`），于是"无限制"和"不知道"都被静默丢弃。
判定层无法区分"随便用"和"别乱用"。

### 2.5 `WindowType` 是一个 overloaded 枚举

`types.go:35-44` 把三条正交的轴压进了一个枚举：

| 值 | 实际表达的轴 |
|---|---|
| `session` / `daily` / `weekly` / `monthly` / `custom` | 时间周期 |
| `balance` | **资源形态**（存量，不是时间窗口） |
| `model` / `code_review` | **作用域** |

而且周期名不可比：`session` 在 Anthropic 是 5 小时（`WindowMinutes=300`），
在 Codex 是上游给的任意 `limit_window_seconds`，在 Zai 是 `unit=3` 的 N-hour，
在 KimiCode 是"< 24h 就叫 session"（`kimi_code.go:301`）。
`WindowMinutes` 只有一部分 fetcher 填（Anthropic 的 `seven_day` 就没填），
所以"这个窗口多久重置"经常两条路都问不出来。

### 2.6 `Tier` 是显示顺序，却被当成业务优先级

`AddWindow(key, tier, ...)` 的 tier 语义是"渲染排序"（`types.go:102`）。
但 `statusline/handler.go:343` 的 `selectBestQuotaWindow` 直接拿 tier 最小的当"最重要的窗口"。
而各家给 tier 的规则完全不同：Anthropic 手写 0/1/2；
Codex 用 `len(usage.Windows)` 递增（`codex.go:248`，同一个模型窗口的 tier 取决于前面碰巧有几个窗口）；
Zai 有硬编码 map 且 default 落到 `len()`。**tier=0 在 A 是 5 小时会话窗，在 B 是余额**——不可比。

### 2.7 作用域缺失导致误 gate

Codex 的 `model_*`（某模型专属限流）、`code_review`，Zai 的 `TIME_LIMIT`（MCP 额度），
都和普通对话请求无关，但它们和账户级窗口平铺在同一个 `Windows` 数组里。
任何"看这个 provider 还有没有额度"的逻辑都会被这些无关窗口污染。

### 2.8 单位不可比，且 currency 无处安放

`UsageWindow` 有 `Unit=currency` 却**没有币种字段**。
`kimi_code.go:389` 只能把币种塞进 `Description` 字符串。
`credits` 这个单位在 KimiCode 是抽象额度、在 KimiK2 是余额、在 Codex reset credit 是"张数"（`Used=0/1`）。

### 2.9 新鲜度不参与判定

每个 fetcher 硬编码 `ExpiresAt`（5min / 10min / 1h），和 `Config.CacheTTL` 各行其是；
`manager.go` 只在错误路径用 `CacheTTL`。数据过期没有降级标记——
用一小时前的快照做路由判定，和用刚拉到的快照，在类型上完全一样。

`manager.go:194` 的 `UsedPercent >= 80` 是全局唯一的"健康"定义，硬编码在 Summary 里。

---

## 3. 设计目标

**换一个提问方式。** 不要再问 provider "你的 used 和 limit 是多少"（那是形状），
而是问它用户真正在问的三个问题（ux-principles #1）：

> 1. **现在还能不能服务这次请求？**
> 2. **还剩多少？**（一个跨 tokens / requests / $ / percent 都成立的量）
> 3. **不能的话，什么时候回血？**

`tokens` / `requests` / `credits` / `$` 是这三个答案的**证据**，属于展示层。
判定层只认这三个答案。

### 非目标

- **不做单位/货币换算**。不引入汇率、不把 tokens 折算成 $。异构资源之间用**比例**做公共货币，
  避免制造虚假精度（ux-principles #5 的反面用法：宁可不给数，也不给假数）。
- **不改 fetcher 现有输出契约**。15 个 fetcher 的 `UsageWindow` 输出、`openapi.json`、前端 codegen 全部保持不动。
- **不建全局 canonical model 表**。见 §5.3。

---

## 4. 分层模型

现在只有一层（结构层）。补两层：

```
  Layer 0  Raw            provider 原生 JSON（RawResponse，已有）
  Layer 1  Display        UsageWindow / UsageBreakdown（已有，保持不动）—— 画进度条
  Layer 2  Semantic       Signal                        （新）—— 单条资源的无歧义含义
  Layer 3  Decision       Assessment / PoolAssessment   （新）—— 判定用的答案
```

Layer 2/3 是**新增的旁路**，Layer 1 不受影响。前端与 API 在 Phase 3 之前完全无感。

---

## 5. 语义模型

### 5.1 三条正交轴，替代 `WindowType`

```go
// ai/quota/semantic.go

// Shape —— 资源形态。决定「耗尽」意味着什么。
type Shape string
const (
    ShapeFlow  Shape = "flow"  // 周期性重置的流量额度；耗尽 = 等到 ResetsAt 自愈
    ShapeStock Shape = "stock" // 存量余额；耗尽 = 必须充值，不会自愈
    ShapeGrant Shape = "grant" // 一次性凭证（Codex reset credit）；可数、有过期
)

// Scope —— 作用域。决定这个信号是否 gate 本次请求。
type ScopeKind string
const (
    ScopeAccount ScopeKind = "account"  // 账户级，gate 一切
    ScopeModel   ScopeKind = "model"    // 仅 gate 指定模型
    ScopeFeature ScopeKind = "feature"  // 仅 gate 指定能力（code_review / mcp / vision）
)
type Scope struct {
    Kind  ScopeKind
    Value string // model: provider 原生 model code；feature: 能力名
}

// 周期不再用枚举 —— 用数值。枚举永远追不上 5h / 7d / N-hour。
//   WindowSeconds + ResetsAt，缺一个可由另一个推
```

`Shape` 的价值在判定上是实的：**flow 耗尽只是"等一会"，stock 耗尽是"这个 provider 死了"**。
只看 headroom 无法区分这两件事，而它们对路由的含义完全相反。

### 5.2 证据等级 `Basis`

```go
type Basis string
const (
    BasisMeasured Basis = "measured" // used + limit 都是真实计量值（MiniMax / Zai / KimiCode）
    BasisRatio    Basis = "ratio"    // 只有比例（Anthropic utilization / Gemini remainingFraction / Codex used_percent）
    BasisDeclared Basis = "declared" // provider 直接声明 allowed / limit_reached（Codex）
    BasisNone     Basis = "none"     // 不可观测（Copilot / Cursor / VertexAI / OpenAI）
)
```

注意 `declared` 与 `measured/ratio` 是**互补而非等级**：
`declared` 是 serviceability 的最强证据但给不出 headroom；`ratio` 给得出 headroom 但说不准"到底还让不让发"。
所以 serviceability 与 headroom 各自带自己的 confidence。

### 5.3 Signal

```go
type Signal struct {
    Key   string // 沿用 UsageWindow.Key，保持可追溯
    Scope Scope
    Shape Shape
    Basis Basis

    // ── 判定量 ────────────────────────────────
    Headroom    NullableFloat64 // 还剩多少比例 [0,1]；Invalid = 不可知
    Serviceable Serviceability

    // ── 恢复语义 ──────────────────────────────
    ResetsAt      *time.Time
    WindowSeconds int
    NeverResets   bool // stock：不会自愈

    // ── 展示证据（不参与判定）──────────────────
    Consumed  NullableFloat64 // 真实计量值；ratio-only 时 Invalid，禁止写百分数
    Capacity  NullableFloat64
    Unlimited bool            // 显式「无限制」，区别于 Capacity.Invalid（「不可知」）
    Unit      UsageUnit
    Currency  string          // Unit=currency 时必填

    // ── 速率（Phase 2）─────────────────────────
    BurnPerHour   NullableFloat64
    RunwaySeconds NullableFloat64
}

type Serviceability string
const (
    ServiceableYes      Serviceability = "yes"
    ServiceableDegraded Serviceability = "degraded" // 逼近耗尽，或只剩溢价额度（Anthropic extra_usage）
    ServiceableNo       Serviceability = "no"
    ServiceableUnknown  Serviceability = "unknown"  // 不可观测 / 数据过期
)
```

**`Unknown` 既不是 `Yes` 也不是 `No`**，这是整个模型最重要的一条。
今天所有不可观测的 provider 都被静默当成"有额度"，判定不出问题只是因为根本没人判定。

`Headroom ∈ [0,1]` 是唯一跨异构可比的量，而且**每个 provider 都给得出**：
`measured` 用 `1 - consumed/capacity`；`ratio` 用 `1 - util/100` 或直接 `remainingFraction`；
`declared` 只在 `limit_reached=true` 时给 0；`none` 给 Invalid。

**关于 canonical model**：`Scope.Value` 用 **provider 原生 model code**，不做全局归一。
因为 `loadbalance.Service{Provider, Model}` 里的 `Model` 本来就是该 provider 的原生模型名，
匹配发生在**同一个 provider 内部**。只需要 provider 内的轻量归一（大小写、
`models/gemini-2.5-pro` 的前缀、`MiniMax-M2` vs `minimax-m2`），由各 provider 的 profile 提供
`NormalizeModelKey(string) string`。这比建全局映射表便宜一个数量级，且不会引入新的漂移源。

### 5.4 NormalizedUsage 与新鲜度

```go
type NormalizedUsage struct {
    ProviderUUID string
    ProviderType ProviderType
    Freshness    Freshness
    Signals      []Signal
    Grants       []Grant   // Codex reset credits 之类，不是配额窗口
    Observable   bool      // false = 该 provider 无配额 API（Copilot/Cursor/VertexAI）
}

type Freshness struct {
    FetchedAt time.Time
    Age       time.Duration
    TTL       time.Duration
    Stale     bool // Age > TTL
}
```

新鲜度参与判定，规则固定、不做成配置项（ux-principles #6）：

| Age | 影响 |
|---|---|
| `≤ TTL` | 原样 |
| `TTL ~ 2×TTL` | Headroom confidence 降为 `approximate` |
| `> 4×TTL` | `Serviceable` 一律降为 `Unknown` |

同时修掉 TTL 分裂：**TTL 归 Manager 所有**（`Config.CacheTTL`），fetcher 只声明
`MinRefreshInterval()`（保护上游限流），不再自己写 `ExpiresAt`。

---

## 6. 归一规则表

每个 provider 的语义知识集中在一处（`ai/quota/profile_*.go`），可审计、可 table-driven 测试
（现有的 `codex_test.go` / `kimi_code_test.go` / `minimax_test.go` fixture 直接复用）。

| Provider | window key | Shape | Scope | Basis | 需要修正的语义 |
|---|---|---|---|---|---|
| anthropic | `five_hour` | flow(300m) | account | ratio | `Consumed/Capacity` 置 Invalid（现在是百分数冒充计量值） |
| anthropic | `seven_day` | flow(7d) | account | ratio | 补 `WindowSeconds=604800`（现在缺失） |
| anthropic | `extra_usage` | stock | account | ratio / **none** | **`Utilization==nil` 时必须 Unknown**；现在 `anthropic.go:175` 填 0，等于宣称"完全没用" |
| codex | `current` / `weekly` | flow | account | ratio + declared | `allowed/limit_reached` 存在时以 declared 覆盖 ratio |
| codex | `model_*` | flow | **model** | ratio + declared | 现在是 account 级平铺，会误 gate 无关请求 |
| codex | `code_review` | flow | **feature:code_review** | declared | 同上 |
| codex | reset credits | **grant** | account | measured | 从 `Breakdowns` 移出（它不是配额窗口，`Used=0/1` 是"张数"） |
| codex | `Cost` | stock | account | measured | `Limit=balance` 是错位（`codex.go:324`）；余额入 `Capacity`，`Consumed` 不可知 → Headroom Invalid |
| gemini | per-bucket | flow(daily) | **model** | ratio | `Headroom = remainingFraction`，直接可用 |
| gemini | `average` | — | — | — | **判定层丢弃**；跨 bucket 用 min（见 §7） |
| zai | `TOKENS_LIMIT_*` | flow | account | measured / ratio | `Number × unit` → `WindowSeconds` |
| zai | `TIME_LIMIT` | flow | **feature:mcp** | measured | 现在是 account 级，MCP 额度会 gate 普通请求 |
| zai | `usageDetails` | — | **model** | measured | **不是 utilization**（`zai_shared.go:248`）；只作为 `Consumed` 明细，Headroom 继承父信号 |
| minimax | `daily` / `weekly` | flow | account | measured | 明确 `used = total - remaining_count` 的反推来源 |
| minimax | per-model | flow | **model** | measured | — |
| kimi_code | `weekly` / `limit_N` | flow | account | measured | `duration+timeUnit` 已可推 `WindowSeconds` |
| kimi_code | `booster` | stock | account | measured | 补 `Currency`（现在只在 Description 里，`kimi_code.go:389`） |
| kimik2 | `credits` | stock | account | measured(derived) | `Capacity` 是 `consumed+remaining` 合成，标注 derived |
| openrouter | `key_limit` | stock | account | measured | — |
| openrouter | `monthly_usage` / `monthly` | flow(30d) | account | measured | `Limit=0` → **`Unlimited=true`**，不是 unknown；两个窗口去重 |
| openai | `Cost` | stock | account | **none** | 无 limit API → Headroom Invalid，Serviceable Unknown |
| copilot / cursor / vertex_ai | — | — | account | none | `Observable=false`，Serviceable **Unknown**（现在是空数组，和"0% 已用"不可分） |

---

## 7. 聚合语义：禁止平均，木桶原则

### 7.1 单 provider 内

```go
func (n *NormalizedUsage) Assess(req RequestScope) Assessment
```

`RequestScope{Model string, Feature string}` 来自本次请求。步骤：

1. **按 scope 过滤**：只保留 `account` + `model:<req.Model>` + `feature:<req.Feature>` 的信号。
   这一步直接消掉 §2.7 的误 gate。
2. **Serviceable = AND**：任一 gating 信号为 `No` → `No`；有 `Unknown` 且无 `No` → `Unknown`。
3. **Headroom = min**（木桶）。**绝不用平均**——这正是 §2.3 Gemini 的 bug。
4. **Binding**：记录是哪个信号构成瓶颈（key），这样 UI 和 trace 能直接说"卡在 7-day 窗口"。
5. **RecoveryAt**：`No` 时取瓶颈信号的 `ResetsAt`；`NeverResets` 的 stock 返回 nil（**不会自愈**）。

```go
type Assessment struct {
    Serviceable Serviceability
    Headroom    NullableFloat64
    Binding     string          // 瓶颈信号的 Key
    BindingShape Shape          // flow → 等得到；stock → 等不到
    RecoveryAt  *time.Time
    Runway      NullableFloat64 // 秒，Phase 2
    Confidence  Confidence
}
```

### 7.2 混合资源池

```go
func AssessPool(members []loadbalance.Service, ...) PoolAssessment

type PoolAssessment struct {
    Serviceable, Degraded, Exhausted, Unknown []ServiceID
    Best           *ServiceID       // headroom 最大的可服务成员
    Headroom       NullableFloat64  // = Best 的 headroom，不是平均
    NextRecoveryAt *time.Time       // 全池耗尽时：最早回血时间（只算 flow 成员）
}
```

**池的余量 = 最好那个成员的余量**，不是平均。理由：失败转移池里有一个成员耗尽，
池子照样能服务；平均会把一个健康池判成半死。

> 顺带记一笔：`evaluateServiceCapacityOp`（`routing.go:517`）对 seat 利用率用的是 avg，
> 同样的质疑对它成立。不在本次范围内，列为 follow-up。

`NextRecoveryAt` 是用户真正在问的问题（"我现在全被卡住了，多久能继续"），
今天整个系统里没有任何地方能回答它。

---

## 8. Smart Op：新增 `quota` position

```go
PositionQuota SmartOpPosition = "quota"

OpQuotaHeadroomGe   = "headroom_ge"    // 值 0-100（%）
OpQuotaHeadroomLe   = "headroom_le"
OpQuotaServiceable  = "serviceable"    // 值 yes | degraded | no | unknown
OpQuotaResetsWithin = "resets_within"  // 值：秒。「快重置了，别切走」
OpQuotaRunwayLe     = "runway_le"      // 值：秒（Phase 2）
```

求值方式与 `service_ttft` / `service_capacity` 同构：
`SmartRoutingStage` 预填 `RequestContext.ServiceQuota`，`evaluateRule` 里按 rule 的 services 过滤
（复用现有的 `filterCapacityForRule` 模式），再对池聚合。
`RequestScope.Model` 取自候选 service 的 `Model`，`Feature` 由 scenario 推出。

**聚合口径用 §7.2 的 max headroom**，与 capacity op 的 avg 刻意不同，
需在 `SmartOpMeta.Description` 里写清楚（ux-principles #8：把教育嵌进产品）。

**Unknown 的处理不做全局开关**（ux-principles #6）：
`headroom_ge 20` 对 unknown 成员**不匹配**——不匹配就落到下一条 rule / 默认路径，
这本身就是"乐观"的正确表达。想保守的用户显式写 `serviceable = yes` 即可。
不需要 `optimistic|conservative` 配置项。

同时接入的其他消费方：
- `statusline/handler.go:343` `selectBestQuotaWindow` 改用 `Assess`，修掉"拿 tier 当业务优先级"（§2.6）；
- `manager.go:194` 的硬编码 `>= 80` 改成 `Assessment.Serviceable == Degraded`。

---

## 9. 迁移路径

**Phase 0 — 纯新增，零行为变更**
`ai/quota/semantic.go` 类型 + `ai/quota/profile_*.go` 归一规则 + `Normalize(*ProviderUsage) *NormalizedUsage`。
用现有 fetcher 测试的 fixture 做 table-driven 断言。不接任何消费方。

**Phase 1 — 判定接入**
`Assess` / `AssessPool`；smart_routing 新增 `quota` position；statusline 与 Summary 换用 Assessment。
此时 `UsageWindow` 依旧是 fetcher 的输出契约，语义层从它推导。

**Phase 2 — 速率**
Manager 保留最近 N 个快照（`quota_sample` 表，刷新间隔 15min → 每小时 4 个点），
差分出 `BurnPerHour` → `RunwaySeconds`。
flow 型信号的 runway 在 `ResetsIn` 处封顶（撑到重置就等于无限）。
**Runway 是比 headroom 更好的混合池排序键**：它把 tokens / requests / $ 统一成"秒"，
且天然把 flow 与 stock 的紧迫性差异算了进去。没有历史时降级为 Invalid，不阻塞 Phase 1。

**Phase 3 — 前端**
展示层用 `Binding` / `RecoveryAt` 讲"卡在哪个窗口、什么时候回血"，
而不是今天的一排无差别进度条；`Grants` 独立渲染（Codex reset credit 不该混在配额条里）。
需要 `task codegen` 重新生成 openapi + client sdk。

**Phase 4 — 收敛**
fetcher 直接产出 `Signal`（fetcher 最清楚上游 API 的语义，profile 退化为兜底），
`UsageWindow` 从"数据源"降级为**由 Signal 派生的展示投影**。
到这一步，`AddWindow` 的 tier 就只剩纯展示含义，§2.6 的歧义从结构上消失。

每个 Phase 都可独立发布、独立回滚。

---

## 10. 对照 ux-principles

| 原则 | 本设计的落点 |
|---|---|
| #1 IA 围绕用户问题 | 判定层只回答"能不能用 / 还剩多少 / 什么时候回血"，不外露后端字段分类 |
| #3 命名碰撞必须拆开 | `used` 四义、`balance` 二义、`used_percent` 二义、`tier` 二义 —— 全部拆开 |
| #4 正交维度分轴 | `WindowType` 拆成 shape × period × scope |
| #5 展示具体值 | 保留 `Consumed/Capacity/Unit/Currency` 作为证据；判定用比例，但**不拿比例冒充计量值** |
| #6 智能默认优于开关 | unknown 的处理靠"不匹配"表达，不加全局策略开关 |
| #8 把教育嵌进产品 | 聚合口径（max vs min vs avg）写进 `SmartOpMeta.Description` 与 eval trace 的 `Reason` |

---

## 11. 测试策略

- **归一规则**：每个 provider 一张 table，输入用现有 fetcher 测试的真实 fixture，
  断言 `(Shape, Scope, Basis, Headroom, Serviceable)`。新增 provider 必须补表，否则 profile 缺失测试失败。
- **不变量测试**（跨所有 provider 统一跑）：
  - `Basis=ratio` ⇒ `Consumed.Valid == false`（禁止百分数冒充计量值）
  - `Unlimited` ⇒ `Headroom` 为 1 且 `Serviceable=Yes`
  - `Observable=false` ⇒ 所有 `Serviceable=Unknown`
  - `Headroom.Valid` ⇒ `0 ≤ v ≤ 1`
  - `Shape=stock` ⇒ `NeverResets=true` 或 `ResetsAt=nil`
- **聚合**：Gemini 的 pro=0%/flash=100% fixture 必须断出 `Headroom=0`（今天是 50%）。
- **新鲜度**：注入时钟，断言 4×TTL 后降级为 Unknown。

---

## 12. 开放问题

1. **`degraded` 的阈值**从哪来？固定 15%？还是按 shape 分（flow 宽松、stock 严格）？
   倾向：flow 用 `headroom < 0.1`，stock 用 `headroom < 0.2`，写死不做配置。
2. **Anthropic `extra_usage`（溢价额度）**：主额度耗尽后落到 extra usage，
   `Serviceable` 应该是 `Yes` 还是 `Degraded`？倾向 `Degraded` —— 它能服务但要花钱，
   用户需要知道这个区别。
3. **Phase 2 的快照留存**：`quota_sample` 保留多久？倾向 24h 滚动，够算 burn 且不膨胀 SQLite。
4. **`Grant`（Codex reset credit）要不要参与判定**？它能"解锁一次重置"，
   理论上应该影响 `RecoveryAt`。倾向 Phase 1 先只展示，不进判定。
