# Vision Proxy 改进 —— 响应注入(承接式)

> 接续 `.design/vision-proxy.md`。本文档只描述**一项改进**:把 vision
> 描述前置注入到响应中,让客户端实时可见。多图并发、缓存等先不做。

---

## 1. 立意 ——「承接」

项目里已有一次「承接」:`firstChunkGate`(`internal/server/failover_dispatch.go`)
做字节级缓冲,在第一个真实 chunk 抵达前允许 failover 切换下一个上游。
它确立了一个好范式:**用 wrapper 拦截响应流,在合适的时机决策/动作**。

本设计沿用这个范式做**内容感知的承接**:在响应流出口接一层
**响应注入(response transformer)**,在第一个真正带 text content 的事件
抵达时,前置一段文本,然后变透传。Vision 图片描述是它的第一个用例。

> 远景:同一中间层未来可支撑
> - **单 message 内容注入**(本 PR:vision 描述前置)
> - **整 message 注入**(在模型回答前额外发一条 system/assistant 消息)
> - **tool 注入**(增加 / 修改响应里的 tool_use 块)
>
> 本 PR 不实现远景形态,但抽象要为它们留口。

---

## 2. 现状关键事实(决定实现形态)

### 2.1 所有协议的流出口都发「原始字节」,不是 typed 字段

| 协议 | 流出口 | 实际发送的内容 |
|------|------|------|
| Anthropic v1 | `HandleAnthropic` → `ProcessStream` | `evt.RawJSON()`(原始 JSON) |
| Anthropic Beta | `HandleAnthropicBeta` → `ProcessStream` | `evt.RawJSON()` |
| OpenAI Chat | `HandleOpenAIChatStream` → `ProcessStream` | `json.Marshal(chunkMap)` |
| OpenAI Responses | `HandleOpenAIResponsesStream` → `StreamLoop` | `eventRaw`(gjson/sjson 处理的 raw) |

**关键**:`ProcessStream` 虽有 `OnStreamEventHooks`(typed event),但
passthrough handler 最终发的是 `evt.RawJSON()` / marshaled bytes —— **改
typed event 的字段不会反映到 wire**。所以注入必须在「即将写出的字节」上
做,而不是 typed event 上。

→ 结论:注入统一走**字节 hook**,对 4 协议对称。typed `OnStreamEventHooks`
保留给只读副作用(recording、TTFT、accumulation),不背改写责任。

### 2.2 非流式

四种协议非流式都是 `hc.GinContext.JSON(resp)`,在 JSON 化**之前**对
response 结构(struct 或 map)就地修改即可。

### 2.3 起点

- `applyVisionProxy`(`internal/server/vision_proxy.go`)在 `SelectService`
  之前跑,原地改写 image block 成 text。
- 描述只进了请求和日志;**不进响应**。
- `ProcessorContext` 没有回传描述列表的字段。

---

## 3. 实现

### 3.1 协议层:两条 hook 链(`HandleContext`)

`internal/protocol/context.go` 加两条与现有 `OnStreamEventHooks` 平行的链:

```go
// 流式:在「即将写出的字节」上改写
OnStreamRawEventHooks []func(eventType string, eventRaw []byte) ([]byte, error)
// 非流式:在 c.JSON 之前改写 resp(struct 或 map)
OnNonStreamResponseHooks []func(resp any)
```

配套方法:`WithOnStreamRawEvent` / `WithOnNonStreamResponse` 注册,
`RunStreamRawEventHooks` / `RunNonStreamResponseHooks` 执行。现有
`OnStreamEventHooks` **不动**(Guardrails / MCP / Recording 零冲击)。

### 3.2 协议层:四个 send 点调用 raw hook

每个流式 handler 在写出前把字节交给 `RunStreamRawEventHooks`:

- `anthropic_passthrough.go`(v1 + Beta):组装出 `outBytes`(含 message_start
  的 model 改写)后,过 raw hooks,再 `SSEvent`。
- `openai_passthrough.go` Chat:`json.Marshal(chunkMap)` 后过 raw hooks,
  再 `OpenAISSE`。
- `openai_passthrough.go` Responses:`eventRaw` 过 raw hooks,再
  `OpenAIResponsesEvent`。

非流式 send 点(`nonstream/*.go`)在 `c.JSON` 前调
`RunNonStreamResponseHooks(resp)`。

### 3.3 协议层:`outputinjector` 包(协议无关)

`internal/protocol/outputinjector/injector.go`:

```go
type OutputInjector interface {
    PrefixText() string  // 首次返回前缀,之后返回 ""
}

// 一次 attach 注册两条链上的 hook,覆盖 4 协议流式 + 4 协议非流式
func AttachToHandleContext(hc *protocol.HandleContext, inj OutputInjector)

// 非流式响应改写(也被 Attach 内部用)
func PrependToNonStreamResponse(inj OutputInjector, resp any) bool
```

`prependToStreamEvent`(内部)按 `eventType` 路由,用 `gjson/sjson` 在
字节上改对应字段:

| eventType | 改写字段 | 协议 |
|-----------|---------|------|
| `content_block_delta`(delta.type==text_delta) | `delta.text` | Anthropic v1/Beta |
| `chat.completion.chunk`(choices.0.delta.content 非空) | `choices.0.delta.content` | OpenAI Chat |
| `response.output_text.delta`(delta 非空) | `delta` | OpenAI Responses |

非 text-bearing 的事件原样透传,且**不消费** `PrefixText()`(injected 标志
不被翻转),保证前缀落到真正的第一段文本上。

`PrependToNonStreamResponse` 按 resp 具体类型(`*anthropic.Message` /
`*anthropic.BetaMessage` / `*openai.ChatCompletion` / `map`(Chat passthrough)
/ `*responses.Response`)找第一个 text 块前置。

> **接口只有 `PrefixText()`**:协议层不理解「描述是什么、怎么拼」,业务
> 收敛到一个字符串。injector 自己用 bool 字段保证只前置一次。与 Guardrails
> 的分层(`guardrails/mutate` 通用改写 + `server/guardrails_runtime` 业务挂载)
> 同型。

### 3.4 业务层:`server/output.VisionTextPrefix`

`internal/server/output/vision_text_prefix.go` 实现 `OutputInjector`:
持有 `descs []string` + `injected bool`,`PrefixText()` 首次返回
`"[Vision: a; b]\n\n"`,之后返回 `""`。

### 3.5 数据流(Processor → gin.Context → Handler)

- `internal/smart_routing/processor.go`:`ProcessorContext` 加
  `Descriptions []string`(命名泛化,留口给未来 injection 类型)。
- `internal/server/processor/vision_proxy.go`:`describe(...)` 改返回
  `(replacement, rawDesc string)` —— 成功时 rawDesc 是上游原始描述,失败
  为 `""`。walker 链多带一个 `sink *[]string`,成功描述经 `recordDesc`
  追加(`""` 不进)。
- `internal/server/vision_proxy.go`:`applyVisionProxy` 用本地
  `ProcessorContext` 收集 `Descriptions`,非空时
  `c.Set(VisionDescriptionsKey, descs)`。

### 3.6 集成层:handler 接入

- **流式**:每个构造 `HandleContext` 的点调
  `attachVisionStreamInjector(c, hc)`(server/vision_proxy.go 的薄封装,
  内部 `newVisionOutputInjector(c)` + `outputinjector.AttachToHandleContext`)。
  覆盖 8 个 `NewHandleContext` 站点(openai_chat、anthropic_message_beta、
  protocol_dispatch ×3、mcp_anthropic_v1_helper ×3)。
- **非流式**:模型响应的 `c.JSON(http.StatusOK, resp)` 统一换成
  `sendNonStreamModelResponse(c, resp)`(先
  `prependVisionToNonStreamResponse` 再 `c.JSON`)。16 个站点。

`newVisionOutputInjector(c)` 在 key 缺失 / descs 空时返回 nil,所有
helper 都 nil-safe → **vision proxy 未启用时零开销**。

---

## 4. 边界

- **vision proxy 未启用**:`c.Get(VisionDescriptionsKey)` miss → injector
  nil → 流式不接 hook、非流式 no-op。
- **下游纯 tool_use 无 text**:没有 text-bearing 事件 → `PrefixText()` 不
  被消费 → 不注入。描述仍在请求里给下游看过了,只是客户端没收到
  `[Vision:]` 前缀。**第一版接受**,follow-up 可考虑独立 text block。
- **历史消息里的图**:不调上游、不进 `Descriptions`(对当前轮不是新信息)。
- **failover 重试**:`firstChunkGate` 在 injector 下游;若上游在第一个
  text 事件之前 5xx 切换,injector 还没注入,切到下一个上游正常生效。
- **raw hook err 传播**:与 `OnStreamEventHooks` 对齐 —— hook 返回 err 时
  中断流并记日志。注入场景不应 err,但机制对称。

---

## 5. 后端改动清单

**新增**
- `internal/protocol/outputinjector/injector.go` + `injector_test.go`
- `internal/server/output/vision_text_prefix.go` + `_test.go`

**修改**
- `internal/protocol/context.go` —— 两条 hook 链 + 方法
- `internal/protocol/stream/{anthropic,openai}_passthrough.go` —— send 前调
  raw hook
- `internal/protocol/nonstream/{anthropic,openai}_passthrough.go` —— JSON 前
  调非流 hook
- `internal/smart_routing/processor.go` —— `ProcessorContext.Descriptions`
- `internal/server/processor/vision_proxy.go` —— describe 双返回值,walker
  收集
- `internal/server/vision_proxy.go` —— `VisionDescriptionsKey`、
  `newVisionOutputInjector` / `attachVisionStreamInjector` /
  `prependVisionToNonStreamResponse` / `sendNonStreamModelResponse`
- handler:`openai_chat.go`、`anthropic_message_beta.go`、
  `protocol_dispatch.go`、`mcp_anthropic_v1_helper.go` —— attach + 非流式
  改用 `sendNonStreamModelResponse`

**依赖**:`tidwall/gjson`、`tidwall/sjson`(已在依赖)。

---

## 6. 测试

- `outputinjector/injector_test.go`:三协议 stream 事件路由(text-delta /
  非 text / 无 content)、只注入一次、`AttachToHandleContext` 两链注册、
  `PrependToNonStreamResponse` 五种 resp 类型 + tool-only 不注入 + nil 安全。
- `server/output/vision_text_prefix_test.go`:首次前缀/再调空、单描述/多描述
  拼接、空 descs、nil 接收者。
- `server/processor/vision_proxy_test.go`:`Descriptions` 按序收集、失败不
  收集。
- `server/vision_proxy_test.go`:`applyVisionProxy` 有/无 service 的 stash
  行为、`newVisionOutputInjector` 三态。

---

## 7. 不做(本 PR 范围外)

- 多图 describe 并发(下个 PR)
- 描述结果缓存(下个 PR)
- 远景的「整 message 注入」/「tool 注入」——抽象留口,实现等用例落地
- describe 失败重试 / 超时控制
- 视觉描述计入 usage
