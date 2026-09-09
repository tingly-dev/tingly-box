# 日志与上游错误分类

> 适用对象：tingly-box 后端贡献者。
> 本文档聚焦**日志/报错的内容与严重度**——"这条日志该打什么级别、带什么字段"
> "返回给下游调用方的错误消息该长什么样"。日志的**架构**（Requests/System 两个
> 视图、`request_id` 关联、`WriteEntry` 路由）已经在 `.design/logging-redesign.md`
> 里讲过，本文不重复，只在涉及处引用。

---

## 0. 一句话模型

一次上游调用失败,有**三个读者**,他们该看到的内容完全不同:

- **服务端日志**：看什么都不藏——原始 Go error、provider、host、base_url、
  分类原因,全量结构化字段,供排障用。
- **下游调用方**(调用 tingly-box 网关 API 的 client)：看状态码 + 一条干净、
  可读、不泄漏内部实现细节的消息。
- **主动调试的下游调用方**(带 `X-Tingly-Debug-Routing` 头的探针请求)：额外
  看到真实的 upstream URL、匹配的 rule、应用的 flags——这是**唯一**应该暴露
  内部路由细节的地方(`internal/protocolserver/protocol_dispatch.go:179-209`
  `setProbeUpstreamHeaders`)。

**核心原则：这三者永远不共享同一个字符串。** 把"内部实现细节"和"给用户看的
错误说明"混在一起,是本文档要修的问题的根源。

---

## 1. 背景：从一条难读的日志开始

`upstream call failed via direct` ——这条日志本身没写错什么,它只是把 Go
`http.RoundTripper` 返回的原始 `error` 原样打了出来。问题出在**这个原始错误
从头到尾都没被分类/翻译过**,一路裸奔到了两个地方:

1. **日志**：`err` 通常是 `*url.Error` 包着 `*net.OpError`(DNS 失败、连接
   拒绝)、`context.DeadlineExceeded`(超时)、TLS 证书错误或 `io.EOF`。典型
   样子是 `Post "https://api.openai.com/...": dial tcp: lookup api.openai.com:
   no such host`。这些错误没有归类,排障时只能肉眼 grep 关键词猜。
2. **下游响应体**：更严重的是,`openai.Error` / `anthropic.Error`(vendor SDK
   在收到 HTTP 响应后构造的类型化错误)的 `Error()` 方法会把**完整的出站请求
   行**(method + 实际调用的 upstream URL,包括自定义/内部 API base)和 provider
   原始错误体拼在一起。这段文本被原样塞进了返回给下游调用方的 JSON `message`
   字段——相当于把网关的内部路由细节,在**每一次报错**时无差别地泄漏出去。

修复思路分两条线,对应下面两节。

---

## 2. 分类：`ClassifyTransportError`

**位置**：`internal/protocol/transport_error.go`

区分"**传输层失败**"(provider 根本没返回 HTTP 响应——DNS、TCP connect、TLS
握手、超时、请求被取消)——这类错误连状态码都没有,过去一律被拍扁成 500,和真正
的网关内部 bug 长得一模一样。

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
(`transportFailureMessages` 表,同文件),只有 `UpstreamMessage` 一处真正需要
那句话,`UpstreamStatus` 和 `logging_roundtripper` 只要 `reason`/`ok`,让它们
各自去拼消息只会白算一遍还被丢掉。

---

## 3. 状态码与消息：`UpstreamStatus` / `UpstreamMessage`

**位置**：`internal/protocol/upstream_error.go`

### `UpstreamStatus(err, fallback) int`

优先级(先到先得):

1. SDK 类型化错误自带的真实状态码——`openai.Error.StatusCode` /
   `anthropic.Error.StatusCode` / `genai.APIError.Code`。
2. `ClassifyTransportError` 命中 → **502 Bad Gateway**(比笼统的 500 准确:
   请求根本没到达 provider,不是网关自己的 bug)。
3. 都不命中 → `fallback`(调用点几乎总传 `http.StatusInternalServerError`)。

502 同时也在 `internal/protocolserver/failover_dispatch.go:90-98` 的
`retryableUpstreamStatuses` 里,所以传输层失败依然会触发 priority failover
换下一个 provider,行为不变。

### `UpstreamMessage(err) string`

给下游调用方看的、**干净且不泄漏内部信息**的一句话:

- **传输层失败** → `"<reason>: <人类可读句子>"`,例如
  `dns_error: could not resolve the upstream provider's hostname`。不回显
  原始 Go dial/DNS 字符串——那是服务端日志该看的细节,不是 API 调用方需要的。
- **SDK 类型化错误**(`openai.Error` / `anthropic.Error`)→ **委托给 SDK 自己
  的 `Error()`**,只是先把 `Request` 换成一个 URL 被替换成固定占位符
  `"REDACTED"` 的副本(`redactedRequest`,同文件)。**不是**手动挑字段重新拼——
  最初的实现挑了 `StatusCode` + `RawJSON()` + `RequestID`,上线当天就已经漏了
  `anthropic.Error` 还会打印的 `WorkspaceID`。委托给真实 `Error()` 意味着
  vendor SDK 以后再加字段,这里自动跟上,不会重新腐化。
- **`genai.APIError`** → 原样透传,它的 `Error()` 本来就不含 URL。
- 都不是 → 原样透传 `err.Error()`(未识别的普通 Go error,没有已知的泄漏源)。

**为什么要在返回给下游前"洗"这个字符串**:`openai.Error`/`anthropic.Error`
的 `Error()` 会把**出站请求的完整 URL**打印出来——如果 provider 配置的是自定义
/内部 API base,这就是一次不必要的内部实现细节泄漏,而且每次报错都会发生,不是
边缘情况。真正需要看这个 URL 的场景(排查路由是否配对)已经有专门通道,见
下面第 6 节。

---

## 4. 日志严重度：`obs.LevelForStatus`

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
  按状态码分级。**这条分支在改动前一律打 Info**,一个 500/429 的 provider
  响应和 200 在日志严重度上没有任何区别——过滤日志时容易漏看。

用 `Entry.Logf`(而不是先 `fmt.Sprintf` 出字符串再选级别调用)是因为 logrus
的 `Logf` 内部会先查 `IsLevelEnabled` 再格式化——这样级别被过滤掉时,连
`Sprintf` 都不会执行,和上面 `Errorf` 分支的惰性求值行为一致。

**这条日志只按状态码分类,不读响应体**——响应体的解析/转成类型化错误
(`openai.Error` 等)仍然是 SDK 客户端那一层的职责,`logging_roundtripper`
在那之前就已经把日志打完了。

---

## 5. 把 `UpstreamMessage` 接到下游响应的每个出口

`err.Error()` 直接进 client-facing JSON/SSE 是本文档要根治的模式。凡是"报告
一次失败的上游调用"的出口,现在都过 `protocol.UpstreamMessage(err)`,而不是
原样 `err.Error()`:

| 出口 | 位置 | 场景 |
|---|---|---|
| `SendErrorResponse` | `internal/protocolserver/error_response.go:76` | 非流式转发失败的统一出口 |
| `respondMCPError` | `internal/protocolserver/error_response.go:61` | MCP 工具调用失败 |
| `FailAttemptSetup` | `internal/protocolserver/failover_dispatch.go:64` | attempt 建立阶段失败（failover 会重试下一档） |
| `SendStreamingError` / `SendForwardingError` | `internal/protocol/stream/anthropic_helper.go:70,82` | 流式请求建立/转发失败（尚未开始吐 SSE 帧） |
| 三处 "Failed to forward request" | `openai_embeddings.go` / `openai_image.go` / `openai_image_edit.go` | embeddings / 图片生成 / 图片编辑的直接转发失败 |
| `failEmptyAssembly` | `internal/protocol/stream/openai_responses_to_anthropic_assembly.go` | 上游流没有产出任何内容块 |
| ~7 处 mid-stream SSE error 帧 | `google_to_any.go` / `openai_to_anthropic{,_beta}.go` / `openai_chat_to_responses.go` / `openai_passthrough.go`（2 处，OpenAI 原生 error chunk 形状，不套 `BuildErrorEvent`） | SSE 已经开始吐帧之后，upstream 流中途失败 |

其中 Anthropic 形状(`{"type":"error","error":{...}}`)的 5 处 mid-stream 站点
统一通过已有的 `BuildErrorEvent(message, errorType, code)` 构造 payload
(`internal/protocol/stream/anthropic_helper.go:28`),而不是各自手写一份相同
的 map 字面量——这样 SSE error 帧的形状只在一处定义。**注意**：`openai_passthrough.go`
里两处走的是 OpenAI 原生 chat/responses 的裸 `{"error": {...}}` 形状(没有外层
`"type":"error"` 包裹),`BuildErrorEvent` 的形状对不上,所以那两处保留手写字面量,
只换了 message 的来源——**给这类 error 帧加字段前,先确认它是 Anthropic 形状
还是 OpenAI 形状,两者不能共用一个 builder**。

新增失败出口时,检查清单:

1. 这个 `err` 有没有可能是 `openai.Error` / `anthropic.Error` / `genai.APIError`
   或一次真正的传输层失败(DNS/TLS/超时/连接拒绝)？如果有,消息字段用
   `protocol.UpstreamMessage(err)`,不要用 `err.Error()`。
2. 这个失败有没有已知的 HTTP 状态码？状态码用 `protocol.UpstreamStatus(err, fallback)`。
3. 纯本地校验错误(读 body 失败、JSON 解析失败、`req.MarshalJSON()` 失败,
   这些从不经过任何 SDK 调用)不需要也不应该套 `UpstreamMessage`——它对普通
   `errors.New(...)` 是无害的直通,但语义上这类错误压根不是"upstream 失败",
   保持 `err.Error()` 更直接。

---

## 6. 真正想看 upstream URL 时怎么办

`UpstreamMessage` 默认策略是"藏住 URL"。如果需要确认某次请求实际打到了哪个
endpoint(排查路由/rule 匹配是否正确),走**专门的调试通道**,而不是从错误消息
里意外看到:

请求带上 `X-Tingly-Debug-Routing: 1`,响应会额外带上
(`internal/protocolserver/protocol_dispatch.go:179-209` `setProbeUpstreamHeaders`):

- `X-Tingly-Upstream-URL` —— 实际转发到的完整 endpoint
  (`upstreamURLFor(provider, reqCtx.TargetAPI)`)
- `X-Tingly-Upstream-API` —— 目标 API 类型
- `X-Tingly-Matched-Rule` / `X-Tingly-Matched-Rule-Desc` —— 命中的 rule
- `X-Tingly-Applied-Flags` —— 生效的 rule flags

这套机制是显式 opt-in、只在探针/调试请求上生效,production 流量不受影响。
这也是为什么剥离 `UpstreamMessage` 里的 URL 不算"丢失排障能力"——真正的排障
路径本来就没打算走错误消息字符串。

---

## 7. 有意不做的事

- **`UpstreamStatus` 和 `UpstreamMessage` 各自独立跑一遍分类逻辑**,同一个
  `err` 在两者都被调用的站点(`errors.As` 链 + `ClassifyTransportError`)会
  走两遍。这是纯错误路径(不是吞吐热路径),两遍的开销是几次 `errors.As`/
  `errors.Is` 判断,可忽略;把两者合并成一个返回 `(status, message)` 的函数
  需要改遍所有调用点的取值结构,为了这点开销不值得。如果以后调用点数量继续
  增长到这两个函数经常成对出现,再考虑合并。
- **`count_tokens` 端点、纯请求体/参数校验失败**没有接 `UpstreamMessage`——
  它们从不经过任何 upstream SDK 调用,谈不上"泄漏 upstream URL",套上去只是
  给普通 `errors.New(...)` 多绕一层无意义的分类判断。只有真正调用了 vendor
  SDK 的路径才需要这层处理。

---

## 8. 相关文档

- `.design/logging-redesign.md` —— 日志的**架构**:Requests/System 两个视图、
  `request_id` 关联、`stage`/`scope` 分类、`loggingRoundTripper` 的落地位置。
  本文档只讲**内容**（分类/严重度/消息），架构层面以那篇为准。
