# Quota 请求失败退避

> 适用对象：改 `ai/quota/{manager,refresher}.go`、`internal/server/module/providerquota`、
> `frontend/src/components/credential/QuotaCell.tsx` 的贡献者。
> 结构：现状（§1）→ 设计（§2–§5）→ 刻意不做的（§6）→ 通用性（§7）→ 实施（§8）→ 待定（§9）。
> 状态：**已实现**（退避、失败保留上次读数、"下次自动重试"一行）。

---

## 1. 实现前的现状（代码里原先是什么样）

| 项 | 现状 | 位置 |
|---|---|---|
| 调度 | **一个全局 ticker**，`Config.RefreshInterval`（默认 5 分钟），启动时立即刷一次。每个 tick 对所有 enabled 且有 fetcher 的 provider 各发一次请求（最多 5 路并发） | `refresher.go` `run`、`manager.go` `Refresh` |
| 每 provider 周期 | **不存在**。`ProviderUsageConfig` 只有 `Enabled/Priority/CustomEndpoint/FetcherType`；所有 provider 共用 5 分钟 | `types.go` |
| 失败处理 | 失败只被写成一条 `LastError` 记录，**下个 tick 原样再来**，永远不变慢。`Config.RetryOnFailure` / `MaxRetries` 是**无人读取的死配置** | `manager.go` `fetchProviderQuota` |
| 失败的副作用 | 失败记录整行覆盖 `provider_usage`（上一次成功的 windows 丢失），并向 `provider_usage_history` 追加一行 —— 每 5 分钟一行，无休止 | `store.go` `Save` |
| 触发来源 | 自动：ticker → `Manager.Refresh`。手动：`POST /provider-quota/:uuid/refresh`、`POST /provider-quota/refresh`、CLI `internal/command/quota.go`；其中"全部刷新"与 ticker **共用 `Manager.Refresh`**，目前无法区分 | `providerquota/handler.go` |
| 可复用的退避 | **没有可直接用的**：`mcp/runtime.ExponentialBackoffStrategy`、`task.BackoffFor`、`loadbalance` 熔断器各自独立，量级与语义不同 | — |

结论：一个持续 403 的 provider（key 过期、套餐无权限、上游拒绝）会**每 5 分钟打一次上游，永远如此**，
同时每天向历史表写 288 行无信息量的失败记录。

## 2. 核心规则（一句话版）

> **自动触发会不断扩大窗口，减少无效请求；主动触发是强制刷新。窗口只在成功之后清零。**

| 触发 | 行为 |
|---|---|
| 自动（ticker） | 没到 `next_attempt` 就跳过；请求失败 → 窗口翻倍；成功 → 窗口清零 |
| 主动（用户点单个 provider、"全部刷新"、CLI） | **总是真的去请求**，无视窗口。成功 → 窗口清零；失败 → 窗口**原样不动**（不扩大、不清零） |

除"成功"外**没有任何其他清零条件**：不因改凭据、不因启用/禁用、不因 OAuth token 轮换而清零。
用户改了凭据想立刻验证，点一下刷新即可（主动触发总会真请求，成功即清零）。
这样不需要凭据指纹、不需要 provider 更新钩子，规则只有一条。

## 3. 退避公式

- `base = RefreshInterval`（5 分钟）。连续 *n* 次自动失败后，下次自动请求的间隔 = `min(cap, base × 2^n)`：
  10m、20m、40m、80m、160m …… 封顶。
- **统一封顶 `cap = 6h`，所有失败同等对待**，不按错误类型各配各的。5 分钟起步，间隔依次是
  10m、20m、40m、80m、160m、320m、360m(封顶)，第 7 次失败起稳定在每 6 小时一次：持续失败的 provider 每天请求从 288 次降到 4 次。
  翻倍序列总会到顶；恢复靠两条路——自动请求某次成功即清零，或用户主动刷新（强制请求，成功即清零），不再为"更快恢复"加任何机制。
- **Jitter**：间隔乘 `[1.0, 1.1)`（只往后延）。目的只有一个：避免同一时刻一起失败的一批 provider 在同一时刻一起重试。
  时钟与随机源可注入，单测不依赖真随机。

### 3.1 到点判断要留松弛量（易踩的坑）

ticker 按 5 分钟整数倍触发，而失败发生在 tick 之后若干秒。`next = failAt + 10m` 在下一个 tick 到 `+10m` 时
**永远差几秒**，于是被推迟到 `+15m`，每一级都被多推一个 tick。
所以到点判断用 `now >= next - slack`，`slack = RefreshInterval / 4`，单测要覆盖"恰好整数倍 tick"。

## 4. 状态存放

**per-provider（以 provider UUID 为键）**，不是 per fetcher 类型：同一类型的两个账号互不连坐。

`Manager` 里一张内存 `map[uuid]*backoffEntry{failures int, next time.Time}`（`backoffBook`），互斥锁保护（5 个 worker 并发）。
**只放内存，不持久化**（已决定）：进程重启后窗口清零，启动时本来就会立即全量刷一次。
与"只在成功时清零"的唯一出入就是重启，这是有意为之——重启时用户通常想看到最新状态，失败本身会照常展示。

provider 变为不支持（无 fetcher）时丢弃其状态。

## 5. 失败时保留上次读数，与 UI

**最近一次要么是成功、要么是失败，二者明确可辨；失败不再抹掉上次成功的读数。**

- `GormStore.Save`：一次失败（`LastError` 非空且没有任何读数）**不覆盖**当前行里的 windows / breakdowns / cost，只换上
  `LastError` / `LastErrorAt`；`FetchedAt` 仍是上次成功的时间。于是 `LastErrorAt > FetchedAt` 即"最新一次失败了，读数是旧的"。
  之前从未成功过的 provider 仍然只有一条错误。成功一次则整行换新、错误清空。
- **历史表照旧记录这次失败**（无读数的失败行）：历史是尝试的日志，`QuotaHistoryView` 也靠它标出失败点；
  不能用带旧读数的行冒充一次新的采样。
- `Manager.fetchProviderQuota` 失败时返回**落库后的那条记录**（旧读数 + 错误），所以手动刷新的响应与之后的读取一致。
- **只展示，不用于决策**：带 `LastError` 的记录里的读数可能已是数小时前的，所以智能路由（`service_quota`）、状态栏、
  `Summary` 的"接近用尽"计数都把它当作**未知**——与以前失败即无读数时的行为完全一致。展示面（Credentials 列表、
  `QuotaCell`、`ProviderQuotaContext`）显示旧读数，并把它变暗（与过期快照同一种处理），tooltip 里仍显示"Updated N ago"
  （取自上次成功时间）和错误原文。点击单元格 = 强制刷新，不变。
- 前端 `ProviderQuotaContext.refresh`：响应里带 `last_error` 但仍有 windows 时，用它替换旧记录并标记 failed；
  只有"从未成功过"（无 windows）时才保留旧快照不动。

**"下次自动重试"**：`ProviderUsage.backoff{failures,next_attempt_at}`（运行时状态，不入库）在 `GetQuota` / `ListQuota` /
刷新返回时由 `Manager` 按内存窗口补上；窗口已过期或已清零则不带。`QuotaCell` tooltip 在错误原文下加一行
"Next automatic retry at 14:35"（非当天带日期），不提"退避"。点击单元格仍是立即刷新。
Mock 模式有两个状态可看：Anthropic（从未读到，错误 + 重试时间）与 Kimi（成功后被拒：旧读数变暗、`Updated 3h ago`、错误与重试时间）。

## 6. 刻意不做的（先保持简单，需要时再加）

- **不按错误类型分类、不按类型配不同封顶**（401/403/429/5xx/超时/解析失败一视同仁，同一个 6h 封顶）。因此不需要在 transport 层采集状态码，14 个 fetcher 一行不改。
  若线上发现某一类需要不同处理，再加：最自然的第一步是 429 的 `Retry-After`（`loggingTransport` 是唯一汇聚点，可记入 ctx）。
- **不持久化**窗口（§4）。
- **不因凭据变化 / 启用状态变化清零**（§2）。
- **不做每 provider 的基准周期**；全局 5 分钟不变。
- 不动推理路径的熔断（`internal/loadbalance`）。

## 7. 通用性：要不要做成可复用组件

结论：**不做**。目前唯一的周期性上游请求就是 quota；规则本身只有十几行（翻倍、封顶、jitter、到点判断）。
为此另建包或调度器是过度抽象。写成 `ai/quota/backoff.go` 里一个**不依赖 quota 类型**的小结构体
（`Delay(failures) time.Duration`，注入时钟/随机源），将来出现第二个调用方时只需移动文件。
（候选调用方如 `catalog` 模板刷新、`ResolveProviderModels` 的模型列表刷新，**未核实它们是否真有失败重试的痛点**。）

## 8. 实施

已实现（一个 PR）：

1. `ai/quota/backoff.go`：`backoffDelay`（`base×2^n`、封顶、只往后延的 jitter）与每 provider 的内存状态 `backoffBook`
   （`due` 带松弛量、`failed(auto)`、`succeeded`），时钟与随机源可注入。
2. `Manager.RefreshDue`（后台 ticker 用，跳过未到点的 provider）与 `Manager.Refresh` / `RefreshProvider` /
   兑换后的刷新（都是主动触发，强制请求）。`Config.MaxBackoff`（默认 6h）取代两个无人读取的死配置 `RetryOnFailure/MaxRetries`。
3. `GormStore.Save` 保留上次读数（§5）；路由 / 状态栏 / `Summary` 把带错误的记录当未知；前端两处小改。
4. `ProviderUsage.backoff` + `openapi.json` + `QuotaCell` 一行 + i18n（en/zh/ru）+ mock 状态。

被跳过的 provider 不进入本 tick 返回的 usage 列表，`AutoRedeem` 只处理实际得到的读数，无影响。
`ErrProviderUnsupported`（无 fetcher / OAuth-only 的 API key）不发上游请求，也不进退避。

## 9. 已定的取舍与待决

已定：统一封顶 6h；不分错误类型；只放内存，重启清零；"全部刷新"是主动触发，强制请求；失败保留上次读数（只展示，不用于决策）。

待决：长期失败后是否要**主动提示**用户（如 key 疑似失效的通知），还是只在 `QuotaCell` 被动展示？目前是后者。
