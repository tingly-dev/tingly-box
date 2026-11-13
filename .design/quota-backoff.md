# Quota 请求失败退避

> 适用对象：改 `ai/quota/{manager,refresher}.go`、`internal/server/module/providerquota`、
> `frontend/src/components/credential/QuotaCell.tsx` 的贡献者。
> 结构：现状（§1）→ 设计（§2–§5）→ 刻意不做的（§6）→ 通用性（§7）→ 实施（§8）→ 待定（§9）。
> 状态：**设计提案，尚未实现**。

---

## 1. 现状（代码里实际是什么样）

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

`Manager` 里一张内存 `map[uuid]*backoffState{failures int, next time.Time}`，互斥锁保护（5 个 worker 并发）。
**只放内存，不持久化**——最简实现。代价：进程重启后窗口清零（启动时本来就会立即全量刷一次）。
这与"只在成功时清零"有一处出入，仅在崩溃循环/频繁重启时才有意义，见 §9 Q2。

删除 provider 时丢弃其状态。

## 5. UI

原则（`ux-principles.md` §6/§12）：不加开关，不加常驻元素。

- `ProviderUsage` 增加可选字段 `backoff { failures, next_attempt_at }`（仅有窗口时才有），需要 `task codegen`。
- `QuotaCell` 提示层在现有错误行下追加一行："下次自动重试 14:35"。
- "点击单元格即刷新"保持不变，且它就是强制刷新——用户永远有一条立即重试的路，无需新按钮。
- 不向用户暴露"退避 / backoff"字样；文案走 i18n（en/zh）。

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

一个小 PR 即可：

1. `backoff.go` + 表驱动单测（第 n 次间隔、封顶、jitter 边界、整数倍 tick 的松弛）。
2. `Manager.Refresh` 加参数区分自动/主动（ticker 传自动，handler 与 CLI 传主动）；`fetchProviderQuota` 末尾按规则更新状态。
   单测用假 fetcher + 假时钟：自动失败翻倍、自动跳过未到点、主动强制请求、主动失败不动窗口、成功清零。
3. 删除死配置 `RetryOnFailure/MaxRetries`。
4. （可拆出）API 字段 + codegen + `QuotaCell` 一行文案 + i18n。

被跳过的 provider 不进入本 tick 返回的 usage 列表，`AutoRedeem` 只处理实际得到的读数，无影响。
`ErrProviderUnsupported`（无 fetcher / OAuth-only 的 API key）不发上游请求，也不进退避。

## 9. 待决问题

1. （已定）统一封顶 6h，不分类别。
2. **重启是否清零**：内存方案下重启即清零，崩溃循环时退避形同虚设。要不要把 `failures / next_attempt_at` 存进
   `provider_usage` 行（加两列、启动时加载）？建议先不做，出现问题再加。
3. **失败是否保留上次成功的读数**（现状：整行覆盖，windows 丢失）。退避让失败状态持续更久，这个问题会更显眼。
   保留（标注过期）对展示友好，但 `service_quota` 智能路由会读到陈旧窗口。需要产品决定，本设计不擅自改。
4. 是否需要在长期失败后**主动提示**用户，还是只在 `QuotaCell` 被动展示？
