# Quota 语义归一

> 适用对象：改 `ai/quota/**`、`internal/smart_routing/**`、`internal/server/module/statusline`、
> `frontend/src/types/quota.ts` 的贡献者。

---

## 1. 用户在问什么

配额这件事，用户只问两个问题：

> **「还剩多少？」** 和 **「什么时候回来？」**

`tokens` / `requests` / `credits` / `$` 都是这两个答案的**说法**，不是答案本身。

`ai/quota` 把 14 个 fetcher 归一成了同一个 `UsageWindow` 结构，但**结构相同不等于含义相同**——
同一组字段在不同 provider 里回答的是不同的问题，所以谁也没法拿它做判定。

本文档要做的事就一句话：**让每个 provider 都能回答这两个问题，用同一种说法。**

> 下文凡说「原先」，指的是本次改动之前的行为。四个真实上游返回的归一结果固化在
> `ai/quota/fetcher/taskfile_samples_test.go`，样本本身在 `build/Taskfile.quota.yml`。

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
| **额度** `limit` | 会 | **等一会** | Anthropic 5h/7d、Codex 周窗口、MiniMax 区间/周、Zai token 限额 |
| **资源** `resource` | 不会 | **得充钱** | OpenRouter 余额、KimiK2 credits、Kimi booster、Codex 重置券 |

这就是你说的"附加的资源 quota"。它俩的区别只有一条：**耗尽后会不会自愈**。
所以 `ResetsAt` 对资源天然是 `nil`——不需要额外的枚举来表达。

---

## 3. 聚合：取最紧

> **一个 provider 有多个窗口时，代表值取最紧的那个（max）。**

卡住你的永远是最紧的那个窗口，所以它就是这个 provider 的用量。

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
`AddWindow(key, tier, ...)` 的 tier 是各 fetcher 手填的，Codex 甚至用 `len(usage.Windows)`
（`fetcher/codex.go`，同一个窗口的 tier 取决于前面碰巧有几个窗口），跨 provider 根本不可比。
**排序改用窗口时长，`Tier` 字段随之删除**——本次改动前它就只写不读。

### 5h / 1w / 1m 怎么统一

**归一成一个数，不是一个枚举**：`WindowMinutes`（5h=300，1w=10080，1m=43200）。

枚举名不可比——`session` 在 Anthropic 是 5 小时、在 Codex 是上游给的任意
`limit_window_seconds`、在 Zai 是 N 小时、在 KimiCode 是"小于 24 小时就叫 session"
（`fetcher/kimi_code.go`）。四家的 `session` 是四个长度。
`Type` 枚举保留，但降级为纯展示。

这个数按可信度分三档取：

| 来源 | provider |
|---|---|
| 上游明说 | Codex（`LimitWindowSeconds / 60`） |
| 从时间戳算 | MiniMax（`EndTime - StartTime`；每个模型不一样，general 5h、video 24h） |
| 按契约写死 | Anthropic 300/10080/43200、Zai `Number × 单位`、Gemini 1440、OpenRouter 43200 |

**周期只做两件事**：排序，和 `Tightest()` 的平手判定（用量相同取短的）。

**不做的事：不参与 `Pct()` 加权。**5h 的 96% 和 1w 的 96% 都是"离耗尽还有 4%"，
本来就可比。加权要引入一个没有正确答案的旋钮（"三天后恢复"该打几折），
还会让这个数字不再只回答一个问题。

**周期的差别体现在 `RecoversAt()`，不在用量里**：

```
5h  96%  →  18:00 恢复        （两小时后没事）
1w  96%  →  周日 03:00 恢复   （得等三天）
```

用量一样紧，难受程度差 36 倍。所以 `Tightest()` 返回的是**哪个窗口**而不只是数字——
statusline 才能说"卡在 7d 窗口，周日 03:00 恢复"。

---

## 4. 三个必须掰开的地方

这三个都是当前代码里真实的 bug，也是"结构归一但语义没归一"的具体体现。每个配一个例子。

### 4.1 不可知 ≠ 0%

```go
// fetcher/anthropic.go   溢价额度，上游返回 utilization = null
} else {
    used  = 0
    limit = 100          // → 前端显示「0% 已用」
}
```

上游说"我不知道"，我们对用户说"一点没用，随便刷"。方向反了。

Copilot / Cursor / VertexAI 更直接——返回空 `Windows` + `LastError`
（`fetcher/copilot.go`），消费方看到的和"额度充足"一模一样。

**改法**：`Pct` 可空。不可知的条目不参与 max、不显示百分比、也不谎报 0。
一个 `unknown` 布尔字段就够，不需要"证据等级"那套东西。

**不要造占位行**。「读不到额度」这件事，`Pct()` 返回 `ok=false` 就已经表达了——
没有窗口，自然没有 countable 的。再补一条 "Quota unavailable" 的空窗口只是给每个界面
加一行说不出任何可操作信息的噪声（ux-principles #9）。原因记在 `LastError` 里，
需要它的界面（CLI 的 `Status: Error:`、前端的原始响应）本来就会读。

**没有信息就什么都不显示。**但**有部分信息**的窗口要留：溢价额度（上游说开了但不肯说用量）、
OpenRouter 的月度花费（$8.10，没有上限）、Codex 的余额（$12.40）——它们有真东西可说，
只是没有"百分比"。这类窗口显示自己的数值，不画进度条、不报百分数。

### 4.2 占比 ≠ 耗尽

```go
// fetcher/zai_shared.go
modelPercent = (detail.Usage / lim.CurrentValue) * 100   // 该模型占「已消耗量」的比例
...
UsedPercent: modelPercent,                               // 写进了「耗尽程度」字段
```

```
本周实际用掉 32%，其中 glm-4.6 占了 85%

原先：glm-4.6 那条 used_percent = 85  →  UI 染红（≥80 染红）
改后：本周窗口 32%；glm-4.6 按占**额度**的比例算（27.2%），占消耗的比例只进 Description
```

**改法**：一条规则——`UsedPercent` 只准表示"离耗尽还有多远"。
构成占比不是配额，不写进这个字段。

### 4.3 平均掩盖耗尽

```go
// fetcher/gemini.go
avgUsedPercent := totalUsedPercent / float64(len(quotaResp.Buckets))
```

```
gemini-2.5-pro    100%  （已经打不通了）
gemini-2.5-flash    0%

原先：平均 50%  →  "还有一半呢"  →  路由过去  →  429
改后：报最紧的那个 bucket = 100%，标签就写 `gemini-2.5-pro`
```

**改法**：account 级窗口从"平均"改成"最紧的那个 bucket"，并用**该模型的名字**当标签——
"Average Usage 50%" 指不出该停用哪个模型，`gemini-2.5-pro 100%` 指得出（ux-principles #5）。

---

## 5. 顺带修掉的两个小歧义

**`Limit == 0` 三义**。`types.go` 的注释写"0 means unlimited"，实际有三种来源：
真·无限制（OpenRouter 没设 key limit，`fetcher/openrouter.go`）、
不可知（OpenAI 没有 limit API）、无额度。
消费方一律按 `Limit > 0` 过滤（`statusline/handler.go`），于是"随便用"和"别乱用"都被静默丢掉。
→ 用 `unlimited` 和 `unknown` 两个布尔显式表达，不再靠 0 猜。

**作用域窗口误 gate**。Codex 的 `model_*`、`code_review`，Zai 的 MCP `TIME_LIMIT`（`zai_shared.go` 的 `classifyZaiLimit`），
MiniMax 套餐里捆的 `video` / `speech` / `image` / `music`
和账户级窗口平铺在同一个 `Windows` 数组里，导致"这个 provider 还有没有额度"被无关窗口污染。
判断标准很简单：**gateway 会不会往这里发请求**。不会的，就不该替账户回答。
→ 这些窗口移进已有的 `Breakdowns`（本来就是干这个的），`Windows` 只留账户级。零新概念。

---

## 6. 数据模型改动

不新建类型，不动 fetcher 输出契约。`UsageWindow` 加三个字段：

```go
type UsageWindow struct {
    // ... 现有字段全部保留 ...

    Kind      WindowKind `json:"kind,omitempty"`      // "limit"（会重置）| "resource"（要充钱）
    Unknown   bool       `json:"unknown,omitempty"`   // Pct 不可知，不参与聚合
    Unlimited bool       `json:"unlimited,omitempty"` // 真·无限制，区别于 Limit==0 的其他两义
}
```

`WindowMinutes` 保留，并成为排序键（原先 Anthropic 的 `seven_day`、Zai、Gemini 都没填）。
不参与比较的条目豁免——不可知 / 无上限的窗口既不排序也不平手判定，周期本来也无从得知。
`Type`（session/daily/weekly/...）降级为纯展示，判定一律不读。
`Tier` **已删除**——它只写不读（Go 侧只有 `AddWindow` 写入，前端也从不读），
排序改用周期之后它就没有消费方了，`AddWindow(key, window)` 因此少一个参数。

判定 API 就三个函数：

```go
func (p *ProviderUsage) Pct() (float64, bool)     // 所有已知条目取 max；bool=false 表示整个 provider 不可知
                                                  // 窗口自己的百分比是 (*UsageWindow).Percent()
func (p *ProviderUsage) Tightest() *UsageWindow   // Pct 最大的那条，用来说明「卡在哪」
func (p *ProviderUsage) RecoversAt() *time.Time   // Tightest 是额度 → ResetsAt；是资源 → nil
```

外加窗口上的两个小助手：`EffectiveKind()`（缺省为额度，兼容旧数据）和
`Countable()`（是否带可比的用量）：

```go
func (w *UsageWindow) Countable() bool {
    return w != nil && !w.Unknown && !w.Unlimited && w.Limit > 0
}
```

`Limit > 0` 是兜底：一个报了花费却没上限、又忘了打标记的窗口，否则会贡献一个伪造的 0%，
而这个 0% 会排在真窗口前面、并在平手时赢下 `Tightest()`。OpenRouter 的月度花费就踩过。

**能比大小就够了。**跨 provider 的聚合（池子怎么算）等到真有消费方时再定，
现在定了也是猜——见 §8。

`Tightest()` 的价值：UI 和 trace 能直接说**"卡在 7 天窗口，周日 03:00 恢复"**，
而不是今天那一排看不出重点的进度条。

---

## 7. 各 provider 要改什么

| Provider | 改动 |
|---|---|
| anthropic | `seven_day` 补 `WindowMinutes=10080`；`extra_usage` 的 `utilization==null` → `Unknown=true`（不再填 0） |
| codex | `model_*` / `code_review` 移进 `Breakdowns`；重置券 `Kind=resource`；`Cost.Limit=balance` 的错位改成资源条目（`fetcher/codex.go`） |
| gemini | average 窗口换成**最紧的那个 bucket**，并用模型名当标签（"Average Usage 50%" 指不出该停用哪个模型）；每个 bucket 进 `Breakdowns` |
| zai | `usageDetails` 的 `modelPercent` 不再写进 `UsedPercent`；MCP `TIME_LIMIT` 移进 `Breakdowns`；`Number×unit` → `WindowMinutes` |
| minimax | 只有**文本模型**（`general` / `MiniMax-M*`）答账户级；套餐捆的 `video` / `speech-*` / `image-*` / `music-*` / `Hailuo-*` 是 gateway 从不请求的媒体生成，移进 `Breakdowns` 的 `feature` 组（否则一个打满的视频额度会让整个 provider 看起来没额度——和 Codex code_review、Zai MCP 同一类）。每个条目出**两个窗口**：自己的区间（general 5h / video 24h）+ 共享周窗口。counts 为 `0/0` 时用 `*_remaining_percent`（真实返回常是这种形态，只按 counts 读会让整个 provider 变"不可知"）；两者都没有 → 该窗口不产出。只有一个文本模型时不再额外出 per-model 行，那只是把账户级窗口重复一遍 |
| kimi_code | `booster` → `Kind=resource`；`weekly` 补 7d 周期（套餐周期是明确的）。上游**没给时长**的 limit 保持无周期 + `Type=custom`，不编数（编出来会顺带被推成 `weekly`）。币种没提成字段——Description 和 `Cost.CurrencyCode` 已经带了，没有消费方要读它 |
| kimik2 | `credits` → `Kind=resource` |
| openrouter | 余额 → `Kind=resource`；`monthly` 的 `Limit=0` → `Unlimited=true`；去掉重复的第二个 monthly 窗口 |
| openai | 有花费没上限 → `spend` 窗口标 `Unknown`（数值仍可见）；404 时只记 `LastError` |
| copilot / cursor / vertex_ai | 只记 `LastError`，不产出窗口——`Pct()` 已经答"不可知"；三份重复兜底合成 `unreadableUsage` |

新增 provider 必须填 `Kind` + `WindowMinutes`，靠单测的不变量断言兜住（§9）。

---

## 8. quota 怎么参与路由（范围外，只记决定）

**结论：quota 只做减法，不做排序。**摘掉事实上不能用的候选，剩下的交给现有 tactic
（轮询 / tier / affinity）——它们已经表达了用户的意图，quota 不该越权。

**为什么不是"选余量最高的"。**三个 Anthropic 账号挂同一条 rule，贪心选最空的那个：

```
配额刷新间隔 15 min，两次刷新之间数字不动。

t=0     A 0%   B 0%   C 0%    → 选 A → 之后 15 分钟所有请求都打 A
t=15    A 25%  B 0%   C 0%    → 选 B → 之后 15 分钟所有请求都打 B
 ...
t=3h    A 95%  B 95%  C 95%   → 三个同时见底，一个能兜底的都没有
```

- **拉平 = 一起见底**：花三份钱买的"轮流用"的冗余，正好被拉平消灭掉。
- **它不是负载均衡**：刷新间隔内余量是常数，"选最高的"是 15 分钟粒度的轮流冲刷。
- **prompt cache 全废**：cache 按账号维度，来回换账号吃不到 cache read 折扣。

而且"拉平"和"顺序耗尽"本来就是两种买法，**项目里已经各有一个 tactic**：
总量不够用 → `loadbalance` 加权轮询；怕被卡住 → `Service.Tier` + failover。
quota 不该发明第三种。

**真做的时候挂在哪。**`internal/server/routing/stage_health.go` 已经是一个"摘除候选"的
stage，跑在 smart routing / affinity / LB 之前，且已经写好了需要的降级规则
（"全都不健康就不过滤"那条降级分支）。而 `loadbalance/health_monitor.go` 里的
`RateLimitedUntil = now.Add(RecoveryTimeout)` 是个**猜的固定超时**。于是：

```
今天（反应式）：吃一个 429 → 标 unhealthy → RateLimitedUntil = now + 固定超时   ← 猜的
有 quota（前瞻）：窗口 100%  → 标 unhealthy → RateLimitedUntil = RecoversAt()   ← 上游真值
```

**quota 不是新的选择维度，是 health 的前瞻版**——不用先吃 429，恢复时间从猜变成准，
出口 / 降级 / 日志全部复用，零新概念。

保守方向写死：`Unknown` 或数据过期 → **不摘**（误摘健康账号的代价远大于不摘耗尽账号）。

**smart op 不做**。它管的是"不想用"（本周超 85% 就省着点切便宜池子），
和摘除管的"不能用"不重叠。等真有人要再加；需要的数据 §6 已经够，不用回头改。

---

## 9. 端到端示例

一条 rule 挂四个 provider（示意数字；真实上游返回的归一结果见
`ai/quota/fetcher/taskfile_samples_test.go`）：

```
                     窗口                              Pct     恢复
  ─────────────────────────────────────────────────────────────────────────
  Anthropic          5h    12%  （刚重置）
                     7d    96%  ← 最紧                  96%    周日 03:00
                     溢价额度   不可知（上游不肯说）
  ─────────────────────────────────────────────────────────────────────────
  MiniMax            区间(5h)  60% ← 最紧               60%    18:00
                     周        12%
                     ·video 100%（feature，不替账户回答）
  ─────────────────────────────────────────────────────────────────────────
  OpenRouter         余额  $8.10/$20 = 40% ← 最紧       40%    nil（要充钱）
                     月度花费  无上限（不算百分比）
  ─────────────────────────────────────────────────────────────────────────
  Copilot            无配额 API                         不可知  —
  ─────────────────────────────────────────────────────────────────────────

  statusline：Anthropic 96% · 卡在 7d 窗口 · 周日 03:00 恢复
```

四个 provider 的用量第一次能放在一起比大小，且每个数字都是同一个意思。

**改之前**同一组数据是这样的：Anthropic 因为 `tier=0` 报 5h 的 **12%**（"还早呢"）；
Gemini 类的 provider 报跨模型平均值；MiniMax 把编码模型和视频额度加总，
一个打满的 video 会让整个 provider 看起来耗尽；OpenRouter 的余额和无上限的月度花费
画成一样的进度条，后者还显示 "$8.10 / $0.00"；Copilot 静默消失，
和"额度充足"无法区分。

---

## 10. 落地顺序

1. ~~**加字段 + 三个函数**~~ ✅ 纯新增，现有行为不变。另加 `checkInvariants` 共享断言，
   新 provider 漏填语义会在测试里挂掉。
2. ~~**按 §7 逐个 provider 归一**~~ ✅ 一个提交一个 provider，用现有 fetcher fixture 断言。
3. ~~**接展示消费方**~~ ✅ statusline / Summary / CLI 换成三个函数，
   窗口排序从 tier 改成周期，前端同步（排序 + 不可知窗口不画百分比）。
   **仍需跑 `task codegen`**：`kind` / `unknown` / `unlimited` 三个字段还没进
   `openapi.json`，删掉的 `tier` 也还在里面（它是 optional，没有客户端读它）。
   前端暂时用 `QuotaWindow` 这个交集类型自行声明，codegen 之后这个类型就可以整个去掉。

到这里 quota 就已经"说人话"了，且没有任何行为风险——以上都只影响显示。

往后是路由接入，**不在本次范围**，按 §8 的方向单独评估：

4. **quota → health 前瞻摘除**。改动集中在给 `HealthMonitor` 加一个
   "按 quota 预标记 + 用 `RecoversAt()` 覆盖 `RateLimitedUntil`" 的入口，`HealthStage` 不动。
5. **smart op `quota` position**。等有人真的要"省着点用"时再加。

不变量单测（跨所有 provider 统一跑，新增 provider 自动被兜住）：

- `Countable()` 且 `Type` **点明了周期**（session / daily / weekly / monthly）⇒ `WindowMinutes > 0`。
  `custom` 豁免——它的意思就是"上游没给任何可归类的信息"，那里编一个长度比留空更糟
  （Zai 有一种形态把 unit 写成字符串 `"tokens"`，根本没给时长）
- `Type=balance` ⇒ `Kind=resource`（能抓到遗漏的那个方向；`AddWindow` 也会自动补）
- `Unknown` ⇒ 不参与 `Pct()`，且前端不显示百分比
- `Unlimited` ⇒ 不参与 `Pct()`
- `Kind=resource` ⇒ `ResetsAt == nil`
- Gemini 的 `pro=100% / flash=0%` fixture ⇒ `Pct() == 100`（原先是 50）
- 不可知 / 无上限的条目 ⇒ `UsedPercent == 0`（不许既说"没有数"又带一个数）

---

## 11. 待定

1. **数据过期怎么办**。刷新间隔 15min，但各 fetcher 自己硬编码 `ExpiresAt`（5min / 10min / 1h），
   和 `Config.CacheTTL` 各行其是。倾向：TTL 归 Manager 所有，超过 4×TTL 的数据 `Pct()` 返回不可知。
   展示阶段无所谓，**但 §8 的摘除依赖它**——不能拿一小时前的快照摘账号。做 §8 之前必须先解决。
2. **Anthropic 溢价额度**：主额度耗尽后落到 extra usage，`Pct` 该报 100 还是报溢价额度的用量？
   倾向报溢价额度的用量，但 `Tightest().Label` 要说明"已进入溢价"——用户需要知道现在开始花钱了。
   （对 §8 也有影响：进了溢价的账号不该被摘，但也不该和没进溢价的一样优先。）
3. **平手判定只比用量，不比恢复距离**。`5h 96%（两小时后恢复）` 会盖过
   `1w 95%（三天后恢复）`，于是 `RecoversAt()` 报的是乐观的那个——18:00 之后 5h 回满，
   但你仍贴着 95% 的周窗口。对"现在能不能发"是对的（卡住下一个请求的确实是 96%），
   对"还能撑多久"偏乐观。修它要引入"紧迫度 = 用量 × 恢复距离"的加权，
   那个旋钮没有显然正确的值。**先保持一个口径**，等真有消费方问第二个问题时再说。
4. **同一个上游账号被配成多个 provider 记录**时（不同 APIBase / 不同模型集合），
   会有多份 quota 行指向同一份上游额度，摘除时可能不一致。
   `UsageAccount.ID` / `Email` 已经有了（Codex / OpenRouter / KimiCode 都填了，Zai 只填 tier），
   可以用它识别"共享配额组"。等 §8 真做的时候再看，展示阶段不受影响。
