# Quota 语义归一

> 适用对象：改 `ai/quota/**`、`internal/server/module/statusline`、`internal/command/quota.go`、
> `frontend/src/types/quota.ts` 的贡献者。
> 结构：调研（§1–§2）→ 设计（§3–§4）→ 实现（§5–§6）→ 结果（§7）→ 范围外决定（§8）→ 待定（§9）。
> 下文凡说「原先」，指本次改动之前的行为。

---

## 1. 问题

配额这件事，用户只问两个问题：

> **「还剩多少？」** 和 **「什么时候回来？」**

`tokens` / `requests` / `credits` / `$` 都是这两个答案的**说法**，不是答案本身。

`ai/quota` 把 14 个 fetcher 归一成了同一个 `UsageWindow` 结构，但**结构相同不等于含义相同**：
同一组字段在不同 provider 里回答的是不同的问题，于是没有任何跨 provider 的判定成立——
展示会误导，未来想做的路由判定更无从谈起。

---

## 2. 调研

### 2.1 各上游到底返回什么

对 14 个 fetcher 的上游响应逐一核对（真实样本在 `build/Taskfile.quota.yml`，
其中 anthropic / codex / minimax / glm 是线上抓包）：

| 上游 | 给什么 | 不给什么 |
|---|---|---|
| Anthropic | 5h / 7d 两个窗口的利用率百分比；溢价额度（**利用率可为 null**） | 绝对量 |
| Codex | primary / secondary 窗口的 `used_percent` + `limit_window_seconds`（**free 套餐的 primary 是 604800s，即一周**）；模型专属限额；code review 限额；重置券；余额（**只给余额，不给原始总额**） | — |
| Gemini | 每模型 `remainingFraction` + 日重置时间 | 账户级汇总 |
| Z.ai / GLM | 编码单位制（3=时 5=月 6=周）的 token 限额，或字符串单位制（**不含时长**）；MCP 限额；每模型消耗明细（**是占消耗的比例，不是占额度的**） | — |
| MiniMax | 每模型两个区间（自己的 interval + 共享周），**counts 常为 0/0**，此时只有 `*_remaining_percent`；套餐捆绑视频/语音/图片/音乐 | — |
| Kimi Code | 周额度 + N 个 limit（**部分不给时长**）；booster 钱包 | — |
| Kimi K2 | `consumed` / `remaining` | 原始总额（只能合成） |
| OpenRouter | key 终身限额（可为 null）；日/周/月花费（**月花费永远无上限**——key limit 管终身不管当月） | — |
| OpenCode | Go 订阅三个窗口（5h 滚动 / 周 / 月）的百分比 + 重置时间 + `status` | 限额绝对值、窗口长度、**按量付费余额（无公开端点）**、免费额度（按 IP 计） |
| OpenAI | 花费时序（`/v1/usage` 常 404） | 上限 |
| Copilot / Cursor / VertexAI | 无配额 API | 一切 |

### 2.2 归一失败的具体形态

结构归一掩盖了七类语义分歧，每类都在产出用户可见的误读：

1. **不可知读作 0%。** Anthropic 溢价额度 `utilization: null` 被填成 `0/100`；
   Copilot 等返回空 `Windows`；MiniMax 按 counts 读 `0/0` 得不出上限——
   三种"读不到"全都渲染成"一点没用，随便刷"。方向是反的。
2. **占比冒充耗尽。** Z.ai 把"某模型占已消耗量的比例"写进 `UsedPercent`：
   本周只用了 32%，glm-4.6 占其中 85%，UI 把它染红、80% 阈值被误触发。
3. **平均掩盖耗尽。** Gemini 对模型 bucket 取平均：pro 100% + flash 0% 报 50%，
   路由过去就 429。MiniMax 的求和是同病：打满的编码额度被未动的媒体额度稀释成 27%。
4. **作用域窗口误 gate。** Codex 的 `model_*` / `code_review`、Z.ai 的 MCP、
   MiniMax 捆绑的视频/语音额度和账户级窗口平铺在一个数组里，
   一个耗尽的 code review 额度让整个账号看起来没额度——而普通请求根本不经过它。
5. **`Limit == 0` 三义。** 真·无限制 / 不可知 / 无额度共用一个 0，
   消费方按 `Limit > 0` 过滤，三种情况全被静默丢弃。
6. **周期名不可比。** `session` 在 Anthropic 是 5 小时、在 Codex 是任意
   `limit_window_seconds`、在 Z.ai 是 N 小时、在 Kimi Code 是"小于 24h"。
   四家的同一个词是四个长度。
7. **`Tier` 是假排序键。** 各 fetcher 手填（Codex 传 `len(usage.Windows)`），
   跨 provider 不可比，且全仓库只写不读。

---

## 3. 设计

### 3.1 归一目标：一个百分比 + 一个重置时间

```
Pct        0-100，已用比例。唯一参与判定的量。
ResetsAt   什么时候回满。nil = 不会自己回来（要充钱）
```

绝对值（`340/500 requests`、`$8.10/$20`）全部保留，但**只给人看**。
不做单位换算、不折算汇率——比例本身就是公共货币，换算只会制造虚假精度。

### 3.2 两类条目

| | 会重置吗 | 耗尽意味着 | 例子 |
|---|---|---|---|
| **额度** `limit` | 会 | **等一会** | Anthropic 5h/7d、Codex 周窗口、MiniMax 区间/周 |
| **资源** `resource` | 不会 | **得充钱** | OpenRouter key 余额、KimiK2 credits、booster、重置券 |

区别只有一条：**耗尽后会不会自愈**。资源天然没有 `ResetsAt`，不需要更多枚举。

### 3.3 聚合：取最紧

一个 provider 有多个窗口时，代表值取**最紧**的那个（max）——卡住下一个请求的就是它。

不取最短：周五晚上 5h 刚重置（12%）而 7d 到 96%，取最短会报"还早呢"，下一个请求就 429。
不取平均：见 §2.2-3。同分平手取周期短的（变化快，更值得看）。

**跨模型同理**：Gemini / MiniMax 的账户级窗口取最紧的那个模型，并把**模型名**写进标签——
"Average Usage 50%" 指不出该停用哪个模型，`gemini-2.5-pro 100%` 指得出。

### 3.4 周期：一个数，不是一个枚举

`WindowMinutes`（5h=300，1w=10080，1m=43200）取代枚举参与排序与平手判定。
取值按可信度分三档：**上游明说**（Codex 的 `limit_window_seconds`）＞
**从时间戳算**（MiniMax 的 `EndTime - StartTime`，每模型不同）＞
**按套餐契约写死**（Anthropic 300/10080）。上游什么都没给 → 保持 0 + `Type=custom`，不编数。

周期**只做**排序和平手判定，刻意不参与 `Pct` 加权——5h 的 96% 和 1w 的 96%
都是"离耗尽还有 4%"，本来就可比；加权需要一个没有正确答案的旋钮。
周期的差别体现在 `RecoversAt()`：同样 96%，一个两小时后没事，一个要等三天。

### 3.5 作用域：gateway 会不会往这里发请求

会 → 账户级 `Windows`；不会 → `Breakdowns`（`feature` / `model` 组），照常展示但不替账户回答。
Codex code review、Z.ai MCP、MiniMax 的视频/语音/图片/音乐额度都属于后者。

### 3.6 没有信息就什么都不显示

「读不到额度」由 `Pct()` 返回 `ok=false` 表达——没有窗口自然没有 countable 的，
**不造 "Quota unavailable" 占位行**（每个界面多一行说不出任何可操作信息的噪声）。
原因记在 `LastError`，要看的界面（CLI 的 `Status: Error:`、前端原始响应入口）本来就读它。

但**有部分信息**的窗口要留：溢价额度（开了但上游不肯说用量）、无上限的月度花费（$8.10）、
只报余额的 Codex credits——显示自己的数值，不画进度条、不报百分数。

### 3.7 刻意不做的

- **单位/货币换算**、**周期加权**：制造虚假精度或不可解释的旋钮。
- **跨 provider 的池聚合**：没有消费方，现在定口径就是猜（见 §8）。
- **证据分级 / 四态可用性 / burn rate**：早期设计稿有过，被"能比大小就够了"砍掉。

---

## 4. 数据模型

`UsageWindow` 增三个字段、删一个：

```go
Kind      WindowKind `json:"kind,omitempty"`      // "unknown"(空串，缺省) | "limit"（会重置） | "resource"（要充钱）
Unknown   bool       `json:"unknown,omitempty"`   // 上游没说；不等于 0%
Unlimited bool       `json:"unlimited,omitempty"` // 真·无限制
// Tier 已删除：全仓库只写不读，排序改用周期后没有任何消费方
```

`Unknown` / `Unlimited` 合起来消掉 `Limit == 0` 的三义。`Windows` 走 JSON 持久化，
新字段自动 round-trip，**不需要 DB 迁移**；旧行缺字段按 false 解码，行为不变。

`Kind` 三态：`WindowKindUnknown`（缺省值，空串）/ `WindowKindLimit` / `WindowKindResource`。
`Unknown` 在 `RecoversAt`、排序、`Pct(WindowKindLimit)` 里一律按"不会自己恢复"处理。

判定 API（`ai/quota/semantic.go`）：`kinds` 留空回答展示问题（不分 `Kind`，含资源型）；
传 `WindowKindLimit` 回答自动化判定问题（§8.1 的 `service_quota`，只认显式标了 `limit` 的窗口）。

```go
func (p *ProviderUsage) Pct(kinds ...WindowKind) (float64, bool)   // 最紧窗口的百分比；kinds 留空 = 不分 Kind，传 WindowKindLimit = 只看显式打了这个标签的窗口
func (p *ProviderUsage) Tightest(kinds ...WindowKind) *UsageWindow // Pct 的来源窗口（能说"卡在哪"），kinds 语义同上
func (p *ProviderUsage) RecoversAt() *time.Time                    // Tightest() 本身 Kind==limit → ResetsAt；否则 → nil

func (w *UsageWindow) Percent() float64            // 窗口自己的百分比
func (w *UsageWindow) Countable() bool             // 是否带可比用量
```

`Countable` 是整个模型的门闩：

```go
return w != nil && !w.Unknown && !w.Unlimited && w.Limit > 0
```

`Limit > 0` 是兜底——一个报了花费却没上限、又忘了打标记的窗口，否则会贡献一个伪造的 0%，
排在真窗口前面并在平手时赢下 `Tightest()`（OpenRouter 月度花费踩过）。

共享层自动推导（`applyWindowSemantics`，`AddWindow` / `AddBreakdown` / 排序都过它）：
`Type=balance` 未填 `Kind` → 自动补 `resource`（漏填会产出一个"声称会回满"的余额）；
不可比窗口的 `UsedPercent` 强制归零（跳过标记的读者不该看到一个像样的数）。

**排序**（`NormalizeWindows`）：额度按周期升序 → 资源 → 无数值，取代原先的 tier。
无周期窗口在排序和平手判定里共用一条规则（`periodRank`），不会"最不紧急却排最前"。

---

## 5. 实现

### 5.1 代码地图

| 职责 | 位置 |
|---|---|
| 判定 API、排序、`Unreadable` | `ai/quota/semantic.go` |
| 字段定义、`AddWindow` / `AddBreakdown`、语义推导 | `ai/quota/types.go` |
| 15 个 fetcher | `ai/quota/fetcher/*.go` |
| 共享助手（`endpoint` / `calcPercent` / `windowTypeForMinutes` / `unreadableUsage`） | `ai/quota/fetcher/helpers.go` |
| 不变量断言（每个 provider 测试调用） | `ai/quota/fetcher/invariants_test.go` |
| 真实样本固化 | `ai/quota/fetcher/taskfile_samples_test.go` + `build/Taskfile.quota.yml` |
| 消费方 | `statusline/handler.go`（`Tightest`）、`manager.go` Summary（`Pct`）、`internal/command/quota.go`、前端 `types/quota.ts` |

### 5.2 各 provider 的归一

| Provider | 语义修正 |
|---|---|
| anthropic | `seven_day` 补周期；溢价额度 `utilization==null` → `Unknown`（不再填 0） |
| codex | `model_*` / `code_review` → `Breakdowns`；重置券 → `resource` 且去掉"恢复时间"（券的过期是失去不是回血）；余额从 `Cost.Limit` 错位改成 `Unknown` 资源窗口（原先渲染 "$0.00 / $12.40"）；窗口类型按 `limit_window_seconds` 长度推（free 套餐的 primary 实为一周） |
| gemini | 平均 → 最紧 bucket + 模型名标签；空 bucket → `MarkUnreadable`（保留 RawResponse 与 5min TTL） |
| zai / glm | 模型明细改按**占额度**算，占消耗的比例只进 Description；MCP → `feature` breakdown（明细保留）；编码单位 → 周期，字符串单位 → `custom` 不编数 |
| minimax | 只有文本模型答账户级，视频/语音/图片/音乐 → `feature`（判据 §3.5）；每模型两个窗口（自己的 interval + 共享周）；counts 0/0 时读 `*_remaining_percent`；两者皆无 → 不产出该窗口 |
| kimi_code | booster → `resource`；weekly 补 7d；无时长 limit 保持 `custom` |
| kimik2 | credits → `resource` |
| openrouter | key 余额 → `resource`；月度花费两个分支都标 `Unlimited`（key limit 管终身不管当月）；去掉重复的第二个 monthly 窗口 |
| opencode | 三个窗口都是账户级 gate（网关任一打满即拒），交给 `Tightest()`；只有百分比 → 0-100 标度；滚动窗口长度上游不给，按其超限文案的 "5 hour" 取 300min；无 Go 订阅的 403 `EntitlementError` → `MarkUnreadable`（好 key，不是错误，更不是 0%）。详见 `.design/opencode-quota.md` |
| openai | 有花费无上限 → `Unknown` 窗口（数值可见）；404 → 只记 `LastError` |
| copilot / cursor / vertex_ai | 只记 `LastError`，不产出窗口 |

### 5.3 不变量（`checkInvariants`，新 provider 漏填语义在测试里挂掉）

- `Countable()` 且 `Type` 点明周期（session/daily/weekly/monthly）⇒ `WindowMinutes > 0`；`custom` 豁免
- `Type=balance` ⇒ `Kind=resource`（能抓到遗漏的方向；`AddWindow` 也自动补）
- `Kind=resource` ⇒ `ResetsAt == nil`
- `Unknown` 与 `Unlimited` 互斥；不可比窗口 ⇒ `UsedPercent == 0`
- `Percent()` ∈ [0,100]

### 5.4 消费方

- **statusline**：`Tightest()` 供 `TBQuota*` 字段；`Usage:` 段只列 `Countable()` 窗口。
  原先按 tier 取第一个窗口——tier 各家规则不同，5h 刚重置时会盖住 96% 的周窗口。
- **Summary**：`Pct() >= 80` 计 warning，不可知不再计入"未用"。
- **CLI**：不可比窗口打 `· <label>: <description>` 一行，不画伪 0% 进度条。
- **前端**：`isCountable` 同款门闩；无比例的窗口显示数值不画条；排序同后端
  （`kind`/`unknown`/`unlimited` 以 `QuotaWindow` 交集类型声明，等 codegen 后收编）；
  mock（`frontend/src/mocks/handlers.ts`）按 taskfile 真实样本重写，覆盖全部渲染态。

---

## 6. 验证

- **真实样本固化**：`taskfile_samples_test.go` 把四份线上抓包（anthropic / codex 限流 /
  minimax 0-0 counts / glm）+ 文档样本的归一结果钉死。这些样本覆盖手写 fixture 想不到的形态：
  null 利用率、一周长的 "primary" 窗口、captureed 0/0 counts。
- **review 流程**：/simplify（4 视角）+ correctness（共享层 / fetcher / 消费方 3 agent 并行），
  各自抓到并修掉本分支引入的缺陷——最重要的两个：`Countable` 缺 `Limit > 0` 兜底、
  前端把占位窗口渲染成满绿 0% 条（后来占位行整个删除）。
- 全量 `go build ./...` + `go test ./ai/... ./internal/...` 通过。

---

## 7. 结果

同一条 rule 下四个 provider，改动前后（示意数字，真实结果见样本测试）：

```
                     窗口                              Pct     恢复
  ─────────────────────────────────────────────────────────────────────────
  Anthropic          5h 12%（刚重置） / 7d 96% ←最紧    96%    周日 03:00
                     溢价额度：不可知（上游不肯说）
  MiniMax            区间(5h) 60% ←最紧 / 周 12%        60%    18:00
                     ·video 100%（feature，不替账户回答）
  OpenRouter         余额 $8.10/$20 = 40% ←最紧         40%    nil（要充钱）
                     月度花费：无上限（不算百分比）
  Copilot            无配额 API                         不可知  —
```

statusline：`Anthropic 96% · 卡在 7d 窗口 · 周日 03:00 恢复`。
四个 provider 的用量第一次能放在一起比大小，且每个数字都是同一个意思。

**改动前**同一组数据：Anthropic 报 5h 的 12%（"还早呢"）；Gemini 类报平均值；
MiniMax 打满的视频额度让全账号显得耗尽；OpenRouter 的月度花费显示 "$8.10 / $0.00"
并排在真窗口前面；Copilot 静默消失，与"额度充足"无法区分。

**遗留**：`task codegen` 未跑——`kind`/`unknown`/`unlimited` 未进 `openapi.json`，
删掉的 `tier` 还在里面（optional，无消费方）；前端的 `QuotaWindow` 交集类型是过渡。

---

## 8. quota 怎么参与路由

quota 参与路由分两条线，管的是两件不重叠的事：smart op 管**"不想用"**（主动避让），
`stage_health` 管**"不能用"**（事后摘除）。前者已实现（本节 §8.1），后者仍是范围外决定，
只记结论未实现（§8.2）。

### 8.1 smart op：`service_quota`（已实现）

新增 `SmartOp` position `service_quota`，操作符 `pct_le` / `pct_ge` / `pct_lt` / `pct_gt`，
值是 0-100 的百分比阈值。代码见 `internal/smart_routing`（`op.go` / `routing.go`
/ `context.go`）与 `internal/routing/stage_smart_routing.go`；用法示例见
`internal/smart_routing/README.md` "Switch to a cheaper pool once quota runs hot"。

- **数据来源**：`ai/quota` 本地缓存（`Manager.GetQuotaNoCache`），按 `Service.Provider`
  （provider UUID）查 `ProviderUsage.Pct(quota.WindowKindLimit)`（§4）。纯读库，路由
  热路径不发起线上配额请求；刷新节奏由 `Manager` 后台 refresher 决定（§9.1 未处理）。
- **只算 `Kind=limit`，不算 `Kind=resource`**：余额耗尽要充钱，不会自己恢复，不该驱动
  这种本该是临时避让的判定。
- **聚合方式：跨 service 取最紧（max），不取平均**——一个池子里任何一个 service 吃紧
  就判定整个池吃紧。分级降级用 `Service.Tier` + `TacticTier` 或多条规则，不用这个 op
  在多个 service 间取平均；UI 建议每条规则只配一个 service。
- **未知不计入判定**：查不到配额的 service 从 `ServiceQuota` 里剔除，不当 0%；一条规则
  所有 service 都没有配额数据时直接放行（`Matched=true`），不阻塞路由。
- **和 §8.2 的边界**：smart op 是主动的、阈值可配的"不想用"，在真吃到 429 之前就切走；
  `stage_health` 是被动的、reactive 的"不能用"，两者数据源相同（`ai/quota`）但不是
  同一个机制，互不替代。

### 8.2 `stage_health` 摘除（范围外决定，未实现）

**quota 只做减法，不做排序**——摘掉事实上不能用的候选，选择交给现有 tactic。

**为什么不是"选余量最高的"**：配额 15 分钟才刷新一次，窗口内余量是常数，
"选最高"退化成 15 分钟粒度的轮流冲刷；三个账号被拉平后**同时**见底，
花三份钱买的冗余正好被消灭；账号间来回切换让 prompt cache 全废。
"拉平"和"顺序耗尽"两种意图项目里已各有 tactic（加权轮询 / `Service.Tier` + failover），
quota 不该发明第三种。

**真做时挂在哪**：`routing/stage_health.go` 已是现成的摘除点（含"全不健康就不过滤"降级）；
`health_monitor.go` 的 `RateLimitedUntil = now + 固定超时` 是猜的，`RecoversAt()` 是上游真值——
quota 是 health 的**前瞻版**：不用先吃 429，恢复时间从猜变准，出口/降级/日志全复用。
保守方向写死：`Unknown` 或数据过期 → 不摘。

---

## 9. 待定

1. **数据新鲜度**：各 fetcher 硬编码 `ExpiresAt`（5min/1h）与 `Config.CacheTTL` 各行其是。
   展示无所谓，但 §8 的摘除不能拿一小时前的快照做——做 §8 前必须先收敛。
2. **MiniMax `*_status` 字段**（观察到 1 / 3）语义未知，未解析。若 3 表示"本套餐无此额度"，
   video 的 0% 窗口应整个不出而非显示 0%。
3. **Anthropic 溢价额度**：主额度耗尽落到 extra usage 后，`Pct` 报 100 还是报溢价用量？
   倾向后者 + 标签注明"已进入溢价"（开始花钱了，用户需要知道）。
4. **平手判定只比用量不比恢复距离**：`5h 96%（2h 后恢复）` 盖过 `1w 95%（3 天后恢复）`，
   `RecoversAt()` 报的是乐观的那个。对"现在能不能发"是对的，对"还能撑多久"偏乐观。
   修它需要"用量 × 恢复距离"的加权，旋钮没有显然正确的值——保持单一口径，等有消费方问第二个问题再说。
5. **同一上游账号配成多个 provider 记录**时会有多份 quota 指向同一份额度；
   `UsageAccount.ID/Email` 可识别"共享配额组"，做 §8 时再处理。
