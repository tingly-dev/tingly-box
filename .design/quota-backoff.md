# Quota 请求失败退避

> 适用对象：改 `ai/quota/{manager,refresher,fetcher,httplog,store}.go`、
> `internal/server/module/providerquota`、`frontend/src/components/credential/QuotaCell.tsx` 的贡献者。
> 结构：现状（§1）→ 目标与非目标（§2）→ 设计（§3–§8）→ 通用性（§9）→ 实施顺序（§10）→ 待定（§11）。
> 状态：**设计提案，尚未实现**。

---

## 1. 现状（代码里实际是什么样）

| 项 | 现状 | 位置 |
|---|---|---|
| 调度 | **一个全局 ticker**，`Config.RefreshInterval`（默认 5 分钟），启动时立即刷一次。每个 tick 对所有 enabled 且有 fetcher 的 provider 各发一次请求（最多 5 路并发） | `refresher.go` `run`、`manager.go` `Refresh` |
| 每 provider 周期 | **不存在**。`ProviderUsageConfig` 只有 `Enabled/Priority/CustomEndpoint/FetcherType`；所有 provider 共用 5 分钟 | `types.go` |
| 失败处理 | 失败只被写成一条 `LastError` 记录，**下个 tick 原样再来**，永远不变慢。`Config.RetryOnFailure` / `MaxRetries` 是**无人读取的死配置** | `manager.go` `fetchProviderQuota` |
| 失败的副作用 | 失败记录整行覆盖 `provider_usage`（上一次成功的 windows 丢失），并向 `provider_usage_history` 追加一行 —— 每 5 分钟一行，无休止 | `store.go` `Save` |
| 错误形态 | fetcher 返回 `fmt.Errorf("status %d")` / `"unexpected status code: %d: …"` 之类**纯字符串**，状态码、`Retry-After` 都没有结构化出来 | `ai/quota/fetcher/*.go` |
| 唯一的汇聚点 | 所有 fetcher 都经 `quota.NewHTTPClient`，其 `loggingTransport` 看得到每个响应的状态码与头；所有请求都带 `ctx` | `fetcher.go`、`httplog.go` |
| 非失败的"失败" | 如 OpenCode 的 403 `EntitlementError`：fetcher 返回 `unreadableUsage(...), nil` —— 一个**确定性状态**，当前也每 5 分钟重问一次 | `fetcher/opencode.go` |
| 手动刷新 | `POST /provider-quota/:uuid/refresh`、`POST /provider-quota/refresh`、CLI `internal/command/quota.go`，都直接走 `Manager.Refresh*`，与 ticker 同一路径 | `providerquota/handler.go` |
| 可复用的退避 | **没有可直接用的**。仓库里有三份各自为政的实现：`mcp/runtime.ExponentialBackoffStrategy`（5s→60s，±25% jitter）、`task.BackoffFor`（5s·2^n，封顶 10m）、`loadbalance` 熔断器（推理请求路径，按 rule/service，含 half-open）。量级、语义、依赖都不同 | 见左 |

结论：一个持续 403 的 provider（key 过期、套餐无权限、上游对 quota 端点封了我们）会**每 5 分钟打一次上游，永远如此**，
同时每天向历史表写 288 行无信息量的失败记录。

## 2. 目标与非目标

目标：失败后拉长对**该 provider** 的下一次请求间隔，减轻上游压力；明确拒绝（401/403）退得最狠；
成功一次、用户动手（改凭据 / 手动刷新）立即恢复。

非目标（本设计不做）：

- 不改正常情况下的刷新周期，不引入每 provider 可配置周期（见 §11 Q5）。
- 不动推理请求路径的熔断（`internal/loadbalance`），那是另一个问题。
- 不做"所有 provider 同时失败 = 本机断网，不计罚"的全局判断（见 §4 末尾）。

## 3. 总体形态

**保留全局 ticker 作为心跳**，在 `Manager` 里加一张 per-provider 的退避状态表；
tick 到来时，没到点的 provider 直接跳过（不发请求、不写记录、不写历史）。

```
tick ─▶ for each enabled provider
          state := backoff[uuid]
          if !state.due(now)            → skip   (debug log: "in backoff until …")
          result := fetch(ctx)
          state.observe(result, now)    → success: reset; failure: failures++, next = now + delay(kind, failures)
```

最小化变更面：`Refresher` 不变；`Manager.Refresh` 加一个过滤，`fetchProviderQuota` 末尾加一次 `observe`。

### 3.1 到点判断要留松弛量（易踩的坑）

ticker 按 5 分钟整数倍触发，而失败发生在 tick 之后若干秒。`next = failAt + 10m` 在下一个 tick 到 `+10m` 时
**永远差几秒**，于是被推迟到 `+15m`，实际间隔变成 15m 而不是 10m，每一级都被多推一个 tick。
所以 `due` 用 `now >= next - slack`，`slack = RefreshInterval / 4`（并让实现的单测覆盖"恰好整数倍 tick"的情形）。

## 4. 错误分类与退避策略

分类发生在 manager，**不改 14 个 fetcher**：`loggingTransport` 把每次响应的状态码与 `Retry-After` 记进 ctx 里的
`*attemptTrace`（manager 在调用 `Fetch` 前放入）。`Fetch` 返回后：

1. `err == nil` 且 `usage.LastError == ""` → 成功；
2. `err == nil` 但 `usage.LastError != ""`（如 OpenCode 的 `EntitlementError`）→ 确定性状态，按 *refused* 处理；
3. 否则看 trace：最近一次响应 ≥400 → 按状态码；没有响应且 err 是 `net.Error` / `context.DeadlineExceeded` / 代理拨号失败 → *network*；
   最近一次响应是 2xx 但 `Fetch` 仍报错 → *parse*。

多请求的 fetcher（如 Codex 两次调用）取**最后一个失败响应**。fetcher 想显式表态时可返回
`quota.WithBackoff(err, kind, retryAfter)` 覆盖推断（可选，第一版不用）。

| 类别 | 判据 | 起步 | 封顶 | 说明 |
|---|---|---|---|---|
| **refused** | 401、403、其余 4xx（400/404/410…）、validation 失败、确定性 `LastError` | `2×base` | **6h** | 重试大概率得到同样的拒绝，且对上游最"可疑"。倍增最快到顶，凭据变化是主要的恢复手段（§5） |
| **rate-limited** | 429 | `max(Retry-After, 2×base)` | **24h** | 遵循 `Retry-After`（秒数或 HTTP 日期，两种都解析）；无头则按普通倍增。`Retry-After` 异常大时以封顶为准 |
| **server** | 500、502、503、504 | `2×base` | **1h** | 上游自己的问题，通常很快恢复，不宜退太远。503 带 `Retry-After` 同上遵循 |
| **network** | 超时、DNS、连接被拒、代理错误 | `2×base` | **30m** | 多半是本机/代理问题，恢复快；封顶低，别让一次断网把 provider 压到半天后 |
| **parse** | 2xx 但解码失败/结构变了 | `2×base` | **6h** | 上游格式变了，等得再久也不会自愈，等升级；不是拒绝，但重试同样无益 |

`base = RefreshInterval`。第 *n* 次**连续**失败后的间隔 = `min(cap, base × 2^n)`：5 分钟起步时，
refused 依次为 10m、20m、40m、80m、160m、320m、360m(封顶)；network 为 10m、20m、30m(封顶)。

**Jitter**：在算出的间隔上乘 `[1.0, 1.1)`（只往后延，不早于下限）。目的是把"同一时刻一起失败的一批 provider"
（断网恢复、上游事故）在时间上错开，避免恢复瞬间同时涌向各自上游。`Retry-After` 同样只做正向抖动。
随机源与时钟可注入，单测不依赖真随机。

**不做**"所有 provider 同时失败 ⇒ 本机断网 ⇒ 不计罚"的全局判断：network 封顶只有 30m，代价有限；
加这条会引入跨 provider 耦合，却没有已知的实际问题要它解决。

## 5. 何时重置

| 事件 | 行为 |
|---|---|
| 一次成功（含"成功但 windows 为空"，如 Copilot） | `failures=0`，`next = now + base`，立即恢复原周期 |
| 凭据 / 地址变化 | 立即重置。状态里存 `fingerprint = sha256(access token ‖ APIBase ‖ ProxyURL)`，tick 时与当前 provider 比较，不一致即重置，**不需要 provider 更新的钩子**；`ProviderUpdated` 处理器若想"保存即重试"可调一次 `Manager.ResetBackoff(uuid)` 把延迟缩到 0。OAuth token 轮换也会触发重置——可接受，且正合适（新 token 可能就修好了 401），频率受 token 寿命限制 |
| provider 被禁用再启用 / 删除 | 禁用时状态保留但不被调度；重新启用重置；删除时丢弃状态 |
| **手动刷新（单个 provider）** | **总是真的去请求**。成功 → 重置；失败 → **不累加 `failures`**，只把 `next` 从现在起重排一个当前级别的间隔。用户狂点刷新不应把 provider 推到封顶 |
| 手动"全部刷新" | 见 §11 Q1 |
| 进程重启 | 见 §6：状态持久化，重启后**延续**退避，不重置 |

## 6. 状态存放与持久化

**per-provider（以 provider UUID 为键）**，不是 per fetcher 类型：同一种类型的两个账号凭据各异，互不连坐。

内存里一张 `map[uuid]*backoffState`（互斥锁保护；5 个 worker 并发读写），字段：

```
failures      int          连续失败次数
kind          string       最近一次的错误类别（refused/rate_limited/server/network/parse）
status        int          最近一次的 HTTP 状态码（无响应为 0）
next_attempt  time.Time    下次允许请求的时间
fingerprint   string       凭据指纹（只存 hash，不存明文）
```

**持久化：建议做，落在现成的 `provider_usage` 行上**（新增列 `consecutive_failures / backoff_kind / last_status /
next_attempt_at`，`AutoMigrate` 自动加列）。理由：

- 失败本来就要写这一行（`LastError`），没有额外的写入路径；
- 不持久化的话，崩溃循环或频繁重启时每次启动都"立即全量刷新一遍"，退避形同虚设；
- 同一份数据直接随 `ProviderUsage` JSON 给到 UI（§7），不需要第二个来源。

只加在 `ProviderUsageRecord`（最新值表），**不加进共享的 `ProviderUsageRecordFields`**，否则历史表每行都背上这几列。
启动时（`Refresher.Start` 之前）从 `store.List` 把状态读进内存。`CleanupExpired` 目前无人调用，不会把带状态的行删掉；
若以后启用，需要跳过 `consecutive_failures > 0` 的行（实现时在代码里留注释）。

**顺带收益**：退避期间不再每 5 分钟写一行失败历史，`provider_usage_history` 的失败噪声按同一倍率衰减。

## 7. UI

原则（`ux-principles.md` §6/§12）：不增加新开关，不增加新的常驻元素；信息进现有的 `QuotaCell` 提示层。

- `ProviderUsage` 新增可选字段 `backoff { failures, kind, status, next_attempt_at }`（仅退避中才有）。需要 `task codegen`。
- `QuotaCell` 现有的 `lastError` 行下追加一行：`Retrying at 14:35 · attempt 4`（`status` 非 0 时前缀 `HTTP 403 ·`）；
  已有的"点击单元格即刷新"保持原样，且点击就是手动强制刷新——**用户永远有一条立即重试的路**，不需要额外按钮。
- 退避中的 provider 不改变单元格的主视觉（沿用现有的 stale 变暗）。
- 文案走 i18n（en/zh），时间用本地短格式；不向用户暴露"退避""backoff"这类内部词汇，写"下次重试时间"。

## 8. 与周边的交互

- **`AutoRedeem`**：只处理本 tick 实际得到的 usage；被跳过的 provider 不在其中，而失败的 provider 本就没有 breakdown，无影响。
- **`Unsupported`（无 fetcher / 仅 OAuth 的 API key）**：根本不发上游请求，也不进退避；保持现状（跳过并清理旧记录）。
- **tinglybox 中继**：边端的 `tingly_box` fetcher 请求的是中央 TB 的 `/quota`，该端点只读缓存、不触发上游刷新
  （`quota-relay.md`），所以中继链路上的失败退避只影响边→中央这一跳，不会向中央背后的真实上游传导压力。
- **失败覆盖上次成功读数**：现状如此（§1），退避让失败"持续更久"，这个问题会更显眼。
  是否改为"失败时保留上次成功的 windows 并标注过期"是独立决定，见 §11 Q3，**不在本设计内偷偷改**。

## 9. 通用性：要不要做成可复用组件

结论：**实现为 `ai/quota` 内的一个小的纯逻辑类型，不另建包，不抽象调度器。**

- 目前**唯一**的周期性上游请求是 quota。仓库里别处的退避（mcp 重连、task 重试、负载均衡熔断）语义与量级都不同，
  硬并成一个组件会得到一个满是参数的东西。
- 但把"策略"和"quota 的接线"分开写，使以后迁移成本低：
  - `backoff.go`（纯函数/小结构体，无 quota 依赖）：`Policy{Base, Multiplier, Cap, Jitter}`、`Policy.Delay(failures, retryAfter)`、
    `parseRetryAfter`、可注入的 `now`/`rand`（时钟沿用 `internal/clock` 的约定，便于测试）。
  - `manager.go`/`refresher.go`：分类（依赖 HTTP 与 quota 的错误形态）、状态表、持久化、due 判断。
- 将来若出现第二个调用方（候选，**未核实它们当前是否真有失败重试的痛点**：`catalog` 的 provider 模板刷新、
  `ResolveProviderModels` 的模型列表刷新），再把 `backoff.go` 提到共享位置，只需移动文件。到那时再决定要不要通用的调度器。

## 10. 实施顺序（建议拆成小 PR）

1. `backoff.go`（Policy + Retry-After 解析）+ 单测（表驱动：每类封顶、第 n 次间隔、jitter 边界、Retry-After 秒/日期/异常值）。
2. `loggingTransport` 记录 trace；manager 分类 + 状态表 + `due` 过滤 + 重置规则；删除死配置 `RetryOnFailure/MaxRetries`
   （或改为接上，见 §11 Q4）。单测用假 fetcher + 假时钟，覆盖"整数倍 tick 的松弛"、手动刷新不累加、指纹变化重置。
3. 持久化列 + 启动时加载 + `CleanupExpired` 注释。
4. API 字段 + `task codegen` + `QuotaCell` 一行文案 + i18n。

每步独立可合并；1+2 即解决上游压力问题，3、4 是体验与重启语义。

## 11. 待决问题

1. **"全部刷新"是否绕过退避？** 建议**不绕过**：只对未在退避的 provider 发请求，返回里带上被跳过的数量，
   行内仍有"下次重试时间"，单行点击可强制。绕过的话，一个 403 的 key 会被每次"全部刷新"重新打一遍，与目标相悖。
2. **封顶值**（refused 6h / rate-limited 24h / server 1h / network 30m / parse 6h）是否合适？这些是按"quota 只用于展示与软路由、
   用户可手动刷新"估的，属于猜测，没有线上数据支撑。
3. **失败是否保留上次成功的读数？** 现状整行覆盖。保留（标注过期）对展示更友好，
   但 `service_quota` 智能路由会读到陈旧窗口，可能误路由；不保留则退避期间长时间只有一条错误。需要产品决定。
4. `RetryOnFailure/MaxRetries` 是删除，还是改成"单次刷新内立刻重试一次"（对瞬时网络抖动有用，但与"减轻上游压力"相悖）？建议删除。
5. 是否需要 **per-provider 的基准周期**（目前所有 provider 共用 5 分钟）？退避公式按 `base` 写，所以将来加了它不用改公式。
   但这是另一个特性，建议等有具体 provider 需要（例如某上游对 quota 端点有严格限速）再做。
6. 长期失败后是否要**主动提示用户**（如 key 疑似失效的通知）？本设计只在 `QuotaCell` 里被动展示。
