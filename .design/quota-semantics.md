# Quota 语义归一

> 适用对象：改 `ai/quota/**`、`internal/smart_routing/**`、`internal/server/module/statusline`、
> `frontend/src/types/quota.ts` 的贡献者。

---

## 1. 用户在问什么

配额这件事，用户只问两个问题：

> **「还剩多少？」** 和 **「什么时候回来？」**

`tokens` / `requests` / `credits` / `$` 都是这两个答案的**说法**，不是答案本身。
今天 `ai/quota` 把 15 个 provider 归一成了同一个 `UsageWindow` 结构，但没归一成同一个**答案**——
所以谁也没法拿它做判定。

本文档要做的事就一句话：**让每个 provider 都能回答这两个问题，用同一种说法。**

---

## 2. 归一到什么

### 一个百分比 + 一个重置时间

```
Pct        0-100，已用比例。唯一的归一量。
ResetsAt   什么时候回满。nil = 不会自己回来（要充钱）
```

绝对值（`340/500 requests`、`$8.10/$20`）全部保留，但**只给人看**，不参与任何判定。
不做单位换算，不折算汇率——比例本身就是公共货币。

### 两类条目

| | 会重置吗 | 耗尽意味着 | 例子 |
|---|---|---|---|
| **额度** `limit` | 会 | **等一会** | Anthropic 5h/7d、Codex 周窗口、MiniMax 日/周、Zai token 限额 |
| **资源** `resource` | 不会 | **得充钱** | OpenRouter 余额、KimiK2 credits、Kimi booster、Codex 重置券 |

这就是你说的"附加的资源 quota"。它俩的区别只有一条：**耗尽后会不会自愈**。
所以 `ResetsAt` 对资源天然是 `nil`——不需要额外的枚举来表达。

---

## 3. 聚合：两条规则

> **窗口之间取最紧（max）；成员之间取最松（min）。**

一个 provider 有多个窗口时，卡住你的是**最紧**的那个。
一个池子有多个 provider 时，救你的是**最松**的那个。

### 为什么代表值不能固定取 5h

你提到"5h 先于 1w"。**排序**上对——短窗口变化快，用户天天盯的是它，所以列表按窗口时长升序排。
但**代表值**不能固定取最短的：

```
周五晚上的 Anthropic：
    5h  12%   （刚重置）
    7d  96%

固定取 5h  →  报 12%  →  规则 "used_le 80" 匹配  →  路由过去  →  下一个请求就 429
取最紧     →  报 96%  →  规则不匹配             →  正确落到下一档
```

好在"取最紧"和"按时长排序"不冲突，而且比"手写优先级"更省事——
今天 `AddWindow(key, tier, ...)` 的 tier 是各 fetcher 手填的，Codex 甚至用 `len(usage.Windows)`
（`fetcher/codex.go:248`，同一个窗口的 tier 取决于前面碰巧有几个窗口），跨 provider 根本不可比。
**排序改用窗口时长，tier 这个概念可以整个去掉。**

### 池子取最松

```
一条 rule 挂 4 个 provider：

    Anthropic    5h 62% / 7d 88%       →  Pct 88
    MiniMax      daily 340/500          →  Pct 68
    OpenRouter   $8.10 / $20 余额       →  Pct 40
    Copilot      无配额 API             →  Pct 不可知

池子的余量 = min = 40%（OpenRouter）
"used_le 80" 匹配到的成员 = [MiniMax, OpenRouter]
```

**不能用平均**：Anthropic 88% 不妨碍 MiniMax 干活，平均成 65% 是在描述一个不存在的东西。

---

## 4. 三个必须掰开的地方

这三个都是当前代码里真实的 bug，也是"结构归一但语义没归一"的具体体现。每个配一个例子。

### 4.1 不可知 ≠ 0%

```go
// fetcher/anthropic.go:169-178   溢价额度，上游返回 utilization = null
} else {
    used  = 0
    limit = 100          // → 前端显示「0% 已用」
}
```

上游说"我不知道"，我们对用户说"一点没用，随便刷"。方向反了。

Copilot / Cursor / VertexAI 更直接——返回空 `Windows` + `LastError`
（`fetcher/copilot.go:58`），消费方看到的和"额度充足"一模一样。

**改法**：`Pct` 可空。不可知的条目不参与 max、不显示百分比、也不谎报 0。
一个 `unknown` 布尔字段就够，不需要"证据等级"那套东西。

### 4.2 占比 ≠ 耗尽

```go
// fetcher/zai_shared.go:246-255
modelPercent = (detail.Usage / lim.CurrentValue) * 100   // 该模型占「已消耗量」的比例
...
UsedPercent: modelPercent,                               // 写进了「耗尽程度」字段
```

```
本周实际用掉 32%，其中 glm-4.6 占了 85%

现在：glm-4.6 那条显示 used_percent = 85  →  UI 染红（QuotaBarItem 对 ≥80 染红）
      规则 "used_ge 80" 被误触发
应该：本周窗口 32%；glm-4.6 只是 Used 的一个明细，不产生自己的 Pct
```

**改法**：一条规则——`UsedPercent` 只准表示"离耗尽还有多远"。
构成占比不是配额，不写进这个字段。

### 4.3 平均掩盖耗尽

```go
// fetcher/gemini.go:124
avgUsedPercent := totalUsedPercent / float64(len(quotaResp.Buckets))
```

```
gemini-2.5-pro    100%  （已经打不通了）
gemini-2.5-flash    0%

现在：平均 50%  →  "还有一半呢"  →  路由过去  →  429
改后：max 100%  →  这个 provider 的 pro 额度已耗尽
```

**改法**：删掉 average 窗口，套 §3 的 max。改动是一行。

---

## 5. 顺带修掉的两个小歧义

**`Limit == 0` 三义**。`types.go:111` 注释写"0 means unlimited"，实际有三种来源：
真·无限制（OpenRouter 没设 key limit，`fetcher/openrouter.go:134`）、
不可知（OpenAI 没有 limit API）、无额度。
消费方一律按 `Limit > 0` 过滤（`statusline/handler.go:378`），于是"随便用"和"别乱用"都被静默丢掉。
→ 用 `unlimited` 和 `unknown` 两个布尔显式表达，不再靠 0 猜。

**作用域窗口误 gate**。Codex 的 `model_*`、`code_review`，Zai 的 MCP `TIME_LIMIT`（`zai_shared.go` 的 `classifyZaiLimit`）
和账户级窗口平铺在同一个 `Windows` 数组里，导致"这个 provider 还有没有额度"被无关窗口污染。
→ 这些窗口移进已有的 `Breakdowns`（本来就是干这个的），`Windows` 只留账户级。零新概念。

---

## 6. 数据模型改动

不新建类型，不动 fetcher 输出契约。`UsageWindow` 加三个字段：

```go
type UsageWindow struct {
    // ... 现有字段全部保留 ...

    Kind      WindowKind `json:"kind"`                // "limit"（会重置）| "resource"（要充钱）
    Unknown   bool       `json:"unknown,omitempty"`   // Pct 不可知，不参与聚合
    Unlimited bool       `json:"unlimited,omitempty"` // 真·无限制，区别于 Limit==0 的其他两义
}
```

`WindowMinutes` 保留但**要求所有 fetcher 填满**（今天 Anthropic 的 `seven_day` 就没填，
Zai / Gemini 都没填）——它现在是排序键，不能空着。
`Type`（session/daily/weekly/...）和 `Tier` 降级为纯展示，判定一律不读。

判定 API 就三个函数：

```go
func (p *ProviderUsage) Pct() (float64, bool)     // 所有已知条目取 max；bool=false 表示整个 provider 不可知
func (p *ProviderUsage) Tightest() *UsageWindow   // Pct 最大的那条，用来说明「卡在哪」
func (p *ProviderUsage) RecoversAt() *time.Time   // Tightest 是额度 → ResetsAt；是资源 → nil

func Loosest(usages []*ProviderUsage) *ProviderUsage  // 池子取 min
```

`Tightest()` 的价值：UI 和 trace 能直接说**"卡在 7 天窗口，周日 03:00 恢复"**，
而不是今天那一排看不出重点的进度条。

---

## 7. 各 provider 要改什么

| Provider | 改动 |
|---|---|
| anthropic | `seven_day` 补 `WindowMinutes=10080`；`extra_usage` 的 `utilization==null` → `Unknown=true`（不再填 0） |
| codex | `model_*` / `code_review` 移进 `Breakdowns`；重置券 `Kind=resource`；`Cost.Limit=balance` 的错位改成资源条目（`fetcher/codex.go:324`） |
| gemini | 删掉 average 窗口；每个 bucket 进 `Breakdowns`；`Pct` 走 max |
| zai | `usageDetails` 的 `modelPercent` 不再写进 `UsedPercent`；MCP `TIME_LIMIT` 移进 `Breakdowns`；`Number×unit` → `WindowMinutes` |
| minimax | 补 `WindowMinutes`（daily=1440 / weekly=10080） |
| kimi_code | `booster` → `Kind=resource`；补币种（现在只在 Description 里，`fetcher/kimi_code.go:389`） |
| kimik2 | `credits` → `Kind=resource` |
| openrouter | 余额 → `Kind=resource`；`monthly` 的 `Limit=0` → `Unlimited=true`；去掉重复的第二个 monthly 窗口 |
| openai | 无 limit API → `Unknown=true` |
| copilot / cursor / vertex_ai | 产出一条 `Unknown=true` 的占位条目，让"不可观测"能被看见、被判定 |

新增 provider 必须填 `Kind` + `WindowMinutes`，靠单测的不变量断言兜住（§9）。

---

## 8. 怎么用

### Smart op：两个 op

```
quota.used_ge   <0-100>    池里最松的成员用量 ≥ N%
quota.used_le   <0-100>    池里最松的成员用量 ≤ N%
```

求值方式和已有的 `service_ttft` / `service_capacity` 同构：
`SmartRoutingStage` 预填候选 service 的配额，`evaluateRule` 里按 rule 的 services 过滤后套 `Loosest()`。
`Unknown` 的成员不参与——**不参与就是"不匹配"**，规则自然落到下一档，
不需要 `optimistic / conservative` 之类的全局开关。

一条典型规则：

```yaml
# 本周额度快见底了，切到备用池
- ops: [{position: quota, operation: used_ge, value: "85"}]
  services: [{provider: openrouter, model: ...}]
```

### 其他消费方

- `statusline/handler.go:343` 的 `selectBestQuotaWindow` 拿 tier 最小的当"最重要窗口"——
  换成 `Tightest()`，顺带修掉 §3 提到的 tier 不可比。
- `manager.go:194` 的硬编码 `UsedPercent >= 80` 换成 `Pct()`。
- 前端展示分两区：**额度**（进度条，按窗口时长排序）+ **资源**（"3 张重置券 · $12.40"），
  重置券不该和额度画成一样的进度条。

---

## 9. 端到端示例

一条 rule 挂 4 个 provider，周五晚上：

```
                     窗口                          Pct     恢复
  ─────────────────────────────────────────────────────────────────────
  Anthropic          5h    12%   （刚重置）
                     7d    96%   ← 最紧              96%    周日 03:00
  ─────────────────────────────────────────────────────────────────────
  MiniMax            daily  340/500  = 68% ← 最紧    68%    明天 00:00
                     weekly 1200/3500 = 34%
  ─────────────────────────────────────────────────────────────────────
  OpenRouter         余额  $8.10/$20 = 60% ← 最紧    60%    nil（要充钱）
  ─────────────────────────────────────────────────────────────────────
  Copilot            无配额 API                      不可知  —
  ─────────────────────────────────────────────────────────────────────

  池子（取最松）= OpenRouter 60%

  规则 "used_le 70"  →  匹配 [MiniMax 68%, OpenRouter 60%]
  规则 "used_ge 85"  →  只有 Anthropic 96%，但池子取最松是 60% → 不匹配（正确：池子还很健康）

  statusline 显示：Anthropic 96% · 卡在 7d 窗口 · 周日 03:00 恢复
```

对照今天的行为：Anthropic 会因为 tier=0 报出 5h 的 **12%**（"还早呢"），
Gemini 类的 provider 会报平均值，Copilot 会静默消失，OpenRouter 的余额和周期额度画成一样的条。

---

## 10. 落地顺序

1. **加字段 + 三个函数**（`Kind` / `Unknown` / `Unlimited` + `Pct()` / `Tightest()` / `RecoversAt()`）。
   纯新增，现有行为不变。
2. **按 §7 逐个 provider 归一**，一个 PR 一个 provider，用现有 fetcher 测试的 fixture 断言。
3. **接消费方**：statusline 换 `Tightest()`、Summary 换 `Pct()`、smart op 加 `quota` position。
4. **前端分区展示**（需要 `task codegen` 重新生成 openapi + client sdk）。

不变量单测（跨所有 provider 统一跑，新增 provider 自动被兜住）：

- `Kind=limit` ⇒ `WindowMinutes > 0`
- `Unknown` ⇒ 不参与 `Pct()`，且前端不显示百分比
- `Unlimited` ⇒ 不参与 `Pct()`
- `Kind=resource` ⇒ `ResetsAt == nil`
- Gemini 的 `pro=100% / flash=0%` fixture ⇒ `Pct() == 100`（今天是 50）

---

## 11. 待定

1. **数据过期怎么办**。刷新间隔 15min，但各 fetcher 自己硬编码 `ExpiresAt`（5min / 10min / 1h），
   和 `Config.CacheTTL` 各行其是。倾向：TTL 归 Manager 所有，超过 4×TTL 的数据 `Pct()` 返回不可知。
   独立于本次改动，可以后做。
2. **Anthropic 溢价额度**：主额度耗尽后落到 extra usage，`Pct` 该报 100 还是报溢价额度的用量？
   倾向报溢价额度的用量，但 `Tightest().Label` 要说明"已进入溢价"——用户需要知道现在开始花钱了。
