# 日志系统：架构与上游调用分类

> 适用对象：tingly-box 后端贡献者。
> 本文档分两部分:**架构**(第 1 节——日志怎么分源、怎么按 `request_id` 关联、
> 前端怎么展示)和**内容**(第 2 节起——一条日志该带什么级别、什么字段)。
> 内容部分目前只覆盖**日志本身**;把这套分类结果进一步用于下游 API 的响应
> 状态码/消息(而不只是日志)是后续一个独立改动的范围,会在那次改动里扩展
> 本文档。

---

# 第一部分:架构——关联的 Model-Request 追踪

**Status: shipped** on `base/logging-system`.

Route 选择:**轻量 logrus 关联**——给请求 context 挂一个 `request_id`,通过既有
的 `MultiLogger.WriteEntry` hook 把条目路由到专门的 `model_request` sink,不引入
新的 logging API。
前端目标:**两个视图(Requests / System)**,smart-routing 折叠进单条请求的
展开详情里。

## UI

**Requests** ——一行一个请求(scenario、路由到的 model、provider、状态、延迟)。
Scenario / provider / status 过滤条。自动刷新,固定住的自动滚动。

![Requests list](images/logs-requests.png)

**展开时间线**——一个请求的完整 pipeline:`smartrouting`(rule 匹配)→
`model_request`(转换阶段)→ `upstream`(provider 调用)→ `http`(access log),
用 `request_id` 关联:

![Request timeline](images/logs-timeline.png)

**System Logs** ——只有真正的系统级条目(启动、配置、job),带级别过滤 chip:

![System logs](images/logs-system.png)

## 为什么要重做

之前的 Logs 页面把日志拆成三个 tab——**Model / System / Smart**——这个拆分
适得其反:

- **"Model Requests" 其实不是以 model 为中心的。** 它和 "System Logs" 调用
  的是同一个 endpoint `/api/v1/system/logs`,唯一区别只是前端一个
  `pathPrefix="/tingly/"` 的过滤。展示的其实是 HTTP access log(状态/延迟/
  路径),不是 model 语义。
- **Protocol 和 client 的日志都漏进了 "System"。** 两个包都直接调用全局
  `logrus.*`,不带 context,`WriteEntry` 默认把它们归到 `LogSourceSystem`。
  转换警告、client 错误、重试——全都进错了 tab,和请求本身脱节。
- **没有关联 id。** 一个请求散落在四个地方,彼此没有任何关联。recording
  pipeline 的 `RequestID` 是在 emit 时才现生成的,没有真正串联起来。

## 怎么做的

### 核心思路

一个 "model request" 是一条**关联 trace**:一个 `request_id` 贯穿整条 pipeline;
日志按 **scope + stage** 分类,而不是按传输路径分类。

- scope `model_request` → 所有挂了 `request_id` 的
- scope `system` → 真正的非请求日志(启动、配置、job)
- stage ∈ `inbound | routing | transform | upstream`

### 关键实现决策

**`logrus.WithContext(ctx)`,而不是 `obs.LogFromContext(ctx)`。**
曾经考虑过做一个 `obs.LogFromContext` helper,但因为太具侵入性被否决了——它会
改变每个调用点的 import 和签名。标准 logrus API 被保留下来:下游代码只是从
`logrus.Info(...)` 换成 `logrus.WithContext(ctx).Info(...)`。集中的路由逻辑在
`WriteEntry` 里读 `entry.Context`,取出 `request_id`,把条目路由到
`model_request` sink。调用点侧零新概念。

**统一的 `loggingRoundTripper`。**
一个包住每个 provider transport 的 wrapper,每次 upstream 调用打一行日志——
provider / proxy / status / latency——而不是每个 client 各写各的。通过请求
context 关联,让 upstream 结果落进同一条时间线。代理凭证永远不打日志;脱敏
成 `scheme://***@host`(不是直接砍成 `scheme://host`,那样会让人看不出其实
配了认证)。`HTTP_PROXY`/`HTTPS_PROXY` 在请求时才 resolve,所以 `direct` 只在
真的是直连时才会被打出来。

**System 页只展示真正的系统条目。**
`ReadJSONLogsBySource` 把 System 视图过滤到只剩 `system / action / unknown`
来源,替换掉之前脆弱的路径前缀匹配。

**共享组件,两个入口。**
`LogExplorer`(Requests + System 两个 tab)同时被主 Logs 页面和按 scenario 快开
的对话框复用;对话框传 `lockedScenario` 做预设过滤——不需要特殊 UI。

### 和最初设计的出入

| 设计 | 实际 |
|---|---|
| `obs.LogFromContext(ctx)` helper | `logrus.WithContext(ctx)` + `WriteEntry` 里集中路由 |
| 每个 client 各自 ad-hoc 地把 Debug 提到 Info | 单一的 `loggingRoundTripper` 包住所有 provider transport |
| `X-Request-Id` 头作为主 ID 来源 | UUID 在中间件里现生成,同时存进 gin context 和 `request.Context()` |

### 和其他可观测性系统的关系

| 系统 | 位置 | 记录什么 | 展示在哪 |
|---|---|---|---|
| A. logrus 日志(`pkg/obs.MultiLogger`) | `internal/obs/multi_logger.go` | text/json/memory,按来源分桶 | **Logs 页面** ← 本次重做 |
| B. 请求录制(`ProtocolRecorder`) | `internal/server/protocol_recording.go` | 原始→转换后的请求/响应、流式 chunk | Prompt recording 页面;按 scenario opt-in |
| C. 用量统计(`UsageTracker`) | `internal/server/tracking.go` | tokens、provider、model、延迟 | Dashboard / DB |

本次重做修的是 (A)。(A) 的 `request_id` 现在和 (B) 的 `RequestID` 对齐了,为以后
收敛到单一数据源留了余地。

### 架构层面尚未做的事

- `GetSystemLogStats` 还在读未过滤的数据(`ReadJSONLogs`);应该和
  `GetSystemLogs` 的来源过滤对齐。
- `GET /api/v1/requests` 和 `GET /api/v1/requests/:id` 的 `openapi.json` 还没
  重新生成;前端用的是占位 client。

---

# 第二部分:内容——上游调用日志的分类与严重度

## 0. 背景

`upstream call failed via direct` ——这条日志本身没写错什么,它只是把 Go
`http.RoundTripper` 返回的原始 `error` 原样打了出来。问题是**这个原始错误
从头到尾都没被分类过**:`err` 通常是 `*url.Error` 包着 `*net.OpError`
(DNS 失败、连接拒绝)、`context.DeadlineExceeded`(超时)、TLS 证书错误或
`io.EOF`。典型样子是 `Post "https://api.openai.com/...": dial tcp: lookup
api.openai.com: no such host`。这些错误没有归类,排障时只能肉眼 grep 关键词猜。

另外,provider 真的返回了 HTTP 响应但状态码是 4xx/5xx 时,`RoundTrip` 并不
返回 `error`——这条日志过去一律打 Info,一个 500 的 provider 响应和一个 200
在严重度上没有任何区别,过滤日志时容易漏看。

以下两节修的是这两个问题。

## 1. 分类：`ClassifyTransportError`

**位置**：`internal/protocol/transport_error.go`

区分"**传输层失败**"(provider 根本没返回 HTTP 响应——DNS、TCP connect、TLS
握手、超时、请求被取消)——这类错误连状态码都没有,过去一律被拍扁成 500,和真正
的网关内部 bug 长得一模一样(状态码这部分目前只影响日志字段;把它也用来决定
下游 API 响应的状态码是后续改动的范围)。

```go
func ClassifyTransportError(err error) (reason TransportFailureReason, ok bool)
```

`reason` 取值(`internal/protocol/transport_error.go:19-26`):

| reason | 触发条件 |
|---|---|
| `dns_error` | `*net.DNSError` |
| `connection_refused` | `errors.Is(err, syscall.ECONNREFUSED)` |
| `tls_error` | 错误文本包含 `x509:` 或 `tls:`（Go 的 TLS/x509 错误没有统一类型可 `errors.As`，只能按标准库固定的文本子串匹配） |
| `timeout` | `context.DeadlineExceeded`，或匹配到的 `net.Error.Timeout() == true` |
| `canceled` | `context.Canceled` |
| `network_error` | 其余任何 `net.Error`（兜底） |
| `ok == false` | 都不匹配——包括 `err == nil`，以及已经是 SDK 类型化错误的情况 |

只返回 `reason`,不返回人类可读消息——消息是 `reason → sentence` 的纯派生
(`transportFailureMessages` 表,同文件)。目前唯一的消费者是
`logging_roundtripper`(用于 `fail_reason` 日志字段,只要 `reason`/`ok`,
不要消息),所以 `transportFailureMessages` 暂时没有被读到——它是为了下一个
改动(把分类结果也用在下游响应消息上)预留的,那时才会真正被消费。

## 2. 日志严重度：`obs.LevelForStatus`

**位置**：`internal/obs/level.go`

```go
func LevelForStatus(statusCode int) logrus.Level  // >=500 Error, >=400 Warn, 其余 Info
```

这条规则原本在两处重复定义:HTTP access log 中间件
(`internal/middleware/memory_log.go`)和 upstream 调用日志
(`internal/client/logging_roundtripper.go`,后者本次新加了按状态码分级的逻辑,
之前只有 Info 一档)。提出来共享一份,两处都改成调用它。

**`internal/client/logging_roundtripper.go`** 里两条分支现在是:

- **传输层失败**(`RoundTrip` 直接返回 `err != nil`,provider 连响应都没
  返回)→ `Error` 级别,并带 `fail_reason` 字段(来自 `ClassifyTransportError`),
  完整 `err` 走 `WithError`。
- **provider 真的返回了响应**(`err == nil`,但 `resp.StatusCode` 可能是
  4xx/5xx)→ 用 `entry.WithField("status", ...).Logf(obs.LevelForStatus(...), ...)`
  按状态码分级。

用 `Entry.Logf`(而不是先 `fmt.Sprintf` 出字符串再选级别调用)是因为 logrus
的 `Logf` 内部会先查 `IsLevelEnabled` 再格式化——这样级别被过滤掉时,连
`Sprintf` 都不会执行,和上面 `Errorf` 分支的惰性求值行为一致。

**这条日志只按状态码分类,不读响应体**——响应体的解析/转成类型化错误
(`openai.Error` 等)仍然是 SDK 客户端那一层的职责,`logging_roundtripper`
在那之前就已经把日志打完了。
