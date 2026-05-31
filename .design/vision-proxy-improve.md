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

## 2. 现状关键事实(调研所得)

### 2.1 两条流出口

| 协议 | 流出口 | 当前 hook 能力 |
|------|------|------|
| Anthropic v1 | `HandleAnthropic` → `ProcessStream` | ✅ `OnStreamEventHooks`(typed event) |
| Anthropic Beta | `HandleAnthropicBeta` → `ProcessStream` | ✅ `OnStreamEventHooks`(typed event) |
| OpenAI Chat | `HandleOpenAIChatStream` → `StreamLoop` | ❌ 无 hook |
| OpenAI Responses | `HandleOpenAIResponsesStream` → `StreamLoop` | ❌ 无 hook |

`StreamLoop`(`internal/protocol/stream/loop.go`)和 `ProcessStream`
(`internal/protocol/context.go`)都通过 `CommitFirstChunk()` 与 failover
gate 协作,但只有 `ProcessStream` 在事件经过时给出 hook。

### 2.2 非流式

四种协议非流式都是 `hc.GinContext.JSON(http.StatusOK, resp)`,在 JSON
化**之前**对 response 结构做就地修改即可。无现成 hook,但也无需新机制。

### 2.3 vision proxy 当前状态

- `applyVisionProxy`(`internal/server/vision_proxy.go`)在 `SelectService`
  之前跑,把请求里的 image block **原地改写**成 text。
- 描述只进了"发给下游的请求"和日志;**不进响应**。
- `ProcessorContext` 没有回传描述列表的字段。

---

## 3. 设计

### 3.1 统一 hook 抽象 —— `StreamLoop` 补齐 hook 能力

让 `StreamLoop` 接受一个可选的 per-step transform 回调,签名与
`ProcessStream` 的 hook 对齐(都是"事件经过时被调",返回 error 则中断)。

```go
// internal/protocol/stream/loop.go
type StepFunc func(w io.Writer) bool

type StreamLoopOption func(*streamLoopOptions)

type streamLoopOptions struct {
    onEvent func(eventBytes []byte) ([]byte, error)
}

// WithOnEvent runs once per emitted event, allowing the bytes to be
// rewritten in place. Returning a non-nil error stops the loop.
func WithOnEvent(fn func([]byte) ([]byte, error)) StreamLoopOption { ... }

func StreamLoop(c *gin.Context, step StepFunc, opts ...StreamLoopOption) bool {
    // ... 现有逻辑 ...
    // 在 step(w) 之后 / Flush 之前,把刚写出的字节交给 onEvent 重写
}
```

> StreamLoop 内 step 直接写 `w io.Writer`,要拿到刚写出的字节做改写,
> 有两种实现选项:
> - **(a)** step 写到一个内部 `bytes.Buffer`,onEvent 改完后再 flush 到真
>   writer。语义清晰,但每事件多一次拷贝。
> - **(b)** 暴露一个 "send raw bytes" 接口替代 step 的 `Write`,内部
>   先经过 onEvent 再落到底层 writer。改 API 形态。
>
> 倾向 **(a)** —— vision 描述前置场景下 onEvent 几乎总是返回原样
> (只在第一个 text 事件改一次),拷贝代价可接受。

`ProcessStream` 已有 `OnStreamEventHooks`,**不动**。两条路径的 hook
形态有差异(typed event vs raw bytes),由 `ResponseInjector`(§3.2)各自
适配。

### 3.2 `ResponseInjector` —— 协议无关的承接对象

抽象一个对象,**每个协议一个实现**,内部维护"已注入"状态:

```go
// internal/server/responseinjector/injector.go (新包)
type Injector interface {
    // OnAnthropicEvent rewrites a typed Anthropic event in place.
    // Returns true if the event was modified (caller may re-marshal).
    OnAnthropicEvent(event any) bool

    // OnOpenAIEventBytes rewrites raw SSE event bytes (OpenAI path uses
    // StreamLoop with byte hooks).
    OnOpenAIEventBytes(eventBytes []byte) ([]byte, bool)

    // OnNonStreamResponse rewrites a fully-formed response before JSON
    // serialization. resp is one of: *openai.ChatCompletion, *responses.Response,
    // *anthropic.Message, *anthropic.BetaMessage.
    OnNonStreamResponse(resp any) bool
}
```

> 三个方法对应:Anthropic typed event 流、OpenAI byte 流、所有非流。
> 实际实现可能用一个内部状态机 +「protocol kind」字段统一,但接口
> 上分三个方法更清楚。

#### 3.2.1 Vision 描述注入实现

```go
// internal/server/responseinjector/vision_text_prefix.go
type VisionTextPrefix struct {
    descriptions []string
    injected     bool
}

func NewVisionTextPrefix(descs []string) *VisionTextPrefix { ... }

// prefixText returns the "[Vision: a; b]\n\n" string once; "" thereafter.
func (v *VisionTextPrefix) prefixText() string {
    if v.injected || len(v.descriptions) == 0 {
        return ""
    }
    v.injected = true
    return "[Vision: " + strings.Join(v.descriptions, "; ") + "]\n\n"
}
```

各协议方法用 `prefixText()`:
- **Anthropic event**:看到第一个 `ContentBlockDeltaEvent` 且其 delta
  是 text → 在 delta.text 前 prepend。
- **OpenAI Chat 流字节**:解析 SSE 行的 JSON,看到第一个
  `choices[0].delta.content` 非空 → prepend。用 `sjson.SetBytes` 改字段。
- **OpenAI Responses 流字节**:同样按 SSE event 解析,找到第一个
  `response.output[i].content[j].text` 类型的 delta → prepend。
- **非流响应**:在 `Content[]` / `choices[0].message.content` /
  `Output[i].Content[j].Text` 第一个 text 位置 prepend;若数组里没有
  text(纯 tool_use)→ §4.4 兜底。

### 3.3 注入器的生命周期

#### (a) Processor 收集描述

`describe()` 返回描述时,**额外**把成功结果追加到一份列表(失败和
historical marker 不进列表)。列表通过 `ProcessorContext` 回传:

```go
// internal/smart_routing/processor.go
type ProcessorContext struct {
    // ... 现有字段 ...
    Descriptions []string  // collected by processors that produce user-visible text
}
```

> 字段命名故意泛化(`Descriptions`),为未来的整 message / tool 注入留口
> ——同一个字段可以承载"vision 给出的视觉描述",未来扩展时可能演化
> 成 `[]InjectionContent` 这种带类型的形态;现在 string 够用。

`processBeta` / `processV1` / `processOpenAI` 在 latest 消息每张图调
`describe` 成功时,把描述 append 到 `pctx.Descriptions`。

#### (b) Helper stash 到 gin.Context

`applyVisionProxy`(`internal/server/vision_proxy.go`)Process 完成后:

```go
if len(pctx.Descriptions) > 0 {
    c.Set(VisionDescriptionsKey, pctx.Descriptions)
}
```

常量 `VisionDescriptionsKey = "vision_descriptions"`。

#### (c) Handler 安装注入器

每条 handler 路径 dispatch 进入响应前判断:

```go
if descsAny, ok := c.Get(VisionDescriptionsKey); ok {
    injector := responseinjector.NewVisionTextPrefix(descsAny.([]string))
    // Anthropic: hc.WithOnStreamEvent(func(e any) error {
    //     injector.OnAnthropicEvent(e); return nil
    // })
    // OpenAI:   pass stream.WithOnEvent(injector.OnOpenAIEventBytes) into HandleOpenAI...Stream
    // Non-stream: in the response massaging block, call injector.OnNonStreamResponse(resp)
}
```

具体接入点(只 4 个 handler 文件,而非分散的 8 个):

| 文件 | 流式接入 | 非流式接入 |
|------|------|------|
| `internal/server/anthropic_message_v1.go` | ProcessStream hook | 非流响应处理处 |
| `internal/server/anthropic_message_beta.go` | ProcessStream hook | 非流响应处理处 |
| `internal/server/openai_chat.go` | 传 StreamLoop opt | `responseMap` 写出前 |
| `internal/server/openai_responses.go` | 传 StreamLoop opt | response struct JSON 化前 |

### 3.4 顺序与 failover

`firstChunkGate`(byte 缓冲)和 `ResponseInjector`(内容感知)分层不冲突:

```
upstream stream
   ↓ (typed events / raw bytes)
ResponseInjector  ← 这层做内容前置
   ↓
StreamLoop / ProcessStream  ← 调 CommitFirstChunk()
   ↓
firstChunkGate  ← 字节缓冲
   ↓
gin.ResponseWriter → client
```

如果在第一个 text 事件抵达**之前** failover 切换(上游 5xx),injector
本来就还没注入(没看到 text),切到下一个上游后正常生效。如果已经
注入并 commit,failover 不会再切换。无新冲突。

---

## 4. 边界

### 4.1 vision proxy 未启用

`c.Get(VisionDescriptionsKey)` 返回 (nil, false) → 不创建 injector →
现有路径零改动、零开销。

### 4.2 下游回应纯 tool_use 无 text

模型直接发 tool_use 没有任何 text content。injector 等不到"第一个 text
事件"。**第一版接受不注入**——描述仍在请求里给下游看过了,只是客户端
没收到 `[Vision: ...]` 前缀。Follow-up 可考虑在 tool_use 之前注入一个
独立 text block(Anthropic 协议允许;OpenAI 中 content + tool_calls 可
共存)。

### 4.3 多张图 / 历史图

- latest 消息里的每张成功描述都进 `pctx.Descriptions`,前缀里用 `; `
  拼接,形如 `[Vision: a red apple; a screenshot of a terminal]`。
- 历史消息的图不调上游(只打 marker),**不进**列表——它们对当前轮
  不是新信息,前置会吵。

### 4.4 描述包含特殊字符

描述里若含 `[`、`]`、换行,如实拼进前缀。客户端是文本消费方,基本
不会出问题。Markdown 转义不属于本 PR 责任。

### 4.5 OpenAI Responses 流的 SSE 解析复杂度

调研显示该路径是 raw JSON event(用 gjson/sjson 处理)。injector 的
`OnOpenAIEventBytes` 内部用 `sjson.SetBytes` 改字段,不需要完整 SDK
反序列化。可以接受。

---

## 5. 后端改动清单

| 文件 | 改动 |
|------|------|
| `internal/protocol/stream/loop.go` | 加 `StreamLoopOption` / `WithOnEvent`,内部缓冲一拷贝并交 hook 改写 |
| `internal/protocol/stream/loop_test.go` | 单测:hook 看到每事件,改写生效,无 hook 时零开销路径不变 |
| `internal/server/responseinjector/` (新包) | `Injector` 接口 + `VisionTextPrefix` 实现 |
| `internal/server/responseinjector/*_test.go` | 单测:首次注入、后续 no-op、空 descriptions no-op、各协议事件识别 |
| `internal/smart_routing/processor.go` | `ProcessorContext` 加 `Descriptions []string` |
| `internal/server/processor/vision_proxy.go` | `describe` 成功时把描述 append 到 ctx 上(需要从 ProcessorContext 拿到容器——signature 加一个 `*[]string` 或改 describe 返回 (text, success bool)) |
| `internal/server/vision_proxy.go` | Process 完后 `c.Set(VisionDescriptionsKey, pctx.Descriptions)`;新增常量导出 |
| `internal/server/anthropic_message_v1.go` / `anthropic_message_beta.go` | 流式注册 ProcessStream hook;非流式响应处理处调 injector |
| `internal/server/openai_chat.go` / `openai_responses.go` | 传 `stream.WithOnEvent` 给 StreamLoop;非流式响应 map / struct 改写处调 injector |

不增加新依赖(`sjson` 项目已用)。

---

## 6. 测试矩阵

| 用例 | 期望 |
|----|------|
| `VisionTextPrefix.prefixText` | 首次返回 `[Vision: ...]\n\n`,后续返回 `""` |
| Anthropic ContentBlockDelta 改写 | 第一个 text delta 被前置;text 事件之外的事件原样;后续 text delta 原样 |
| OpenAI Chat SSE 字节改写 | 第一个 `delta.content` 非空 chunk 被前置;后续原样;非 content chunk 原样 |
| OpenAI Responses SSE 字节改写 | 第一个 text-bearing output delta 被前置 |
| 非流 Anthropic Message | `Content[0]` 若是 text → 前置;若不是 text → 不前置(§4.2) |
| 非流 OpenAI Chat | `choices[0].message.content` 前置 |
| 端到端 Anthropic Beta | stub 流给出 text delta → 客户端 SSE 第一个 text 事件含前缀 |
| 端到端 OpenAI Chat | 同上 |
| vision proxy 关闭 | 无 descriptions → injector 不安装 → 流原样,零开销 |
| 多张图 | 多个成功描述 → 前缀里以 `; ` 拼接,只前置一次 |

---

## 7. 不做

- 多图 describe 并发(下个 PR)
- 描述结果 LRU 缓存(下个 PR)
- 远景的"整 message 注入" / "tool 注入"——本 PR 抽象留口,实现等
  用例落地再说
- describe 失败重试 / 超时控制
- 视觉描述计入 usage
