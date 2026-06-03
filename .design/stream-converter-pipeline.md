# Stream Converter Pipeline

> 适用对象：改 `internal/protocol/stream/`、`internal/protocol/context.go`、`internal/server/` 里调用流式转换函数的贡献者。

**Status: shipped** across branches `claude/jolly-goldberg-s7WYH` (Phases 0–7).

---

## 1. 背景与问题

改造前，流式处理层存在两条并行路径：

| 路径 | 机制 | Hook 支持 |
|------|------|-----------|
| **透传 (passthrough)** | `ProcessStream(nextFunc, handleFunc)` | ✅ OnStreamEvent / OnStreamComplete / OnStreamError |
| **转换 (conversion)** | `StreamLoop` + 手动读写 | ❌ 跳过 hook，或 dispatch 错误协议的事件 |

结果：recording、guardrails、TTFT 等 hook 只在透传路径生效。转换路径的 `HandleContext` 是摆设。

---

## 2. 设计目标

```
Before:  upstream → [converter reads + transforms + writes SSE + dispatches hooks] → client
After:   upstream → [converter.Next() → target event] → ProcessStream → hooks → writer → client
```

**核心原则**：converter 是纯状态机迭代器，不接触 `gin.Context`，不写 SSE。`ProcessStream` 驱动循环、触发 hook、调用 writer。

---

## 3. 接口定义

```go
// internal/protocol/stream/converter.go
type StreamConverter interface {
    Next() (event interface{}, done bool, err error)
    Usage() *protocol.TokenUsage
}

func RunConverter(
    hc *protocol.HandleContext,
    conv StreamConverter,
    writer func(event interface{}) error,
) (*protocol.TokenUsage, error)
```

`RunConverter` 的实现：

```go
func RunConverter(hc *protocol.HandleContext, conv StreamConverter, writer func(event interface{}) error) (*protocol.TokenUsage, error) {
    hc.SetupSSEHeaders()
    err := hc.ProcessStream(
        func() (bool, error, interface{}) {
            event, done, err := conv.Next()
            if err != nil { return false, err, nil }
            if done    { return false, nil, nil  }
            return true, nil, event
        },
        writer,
    )
    return conv.Usage(), err
}
```

---

## 4. Converter 的内部结构

每个 converter 维护一个 `pending []interface{}` 队列，支持「一个上游事件 → 多个目标事件」。

```go
func (c *Converter) Next() (interface{}, bool, error) {
    // 1. 先消费 pending（上次 processEvent 遗留的多余事件）
    if len(c.pending) > 0 {
        evt := c.pending[0]
        c.pending = c.pending[1:]
        return evt, false, nil
    }
    if c.done { return nil, true, nil }

    // 2. 从上游读一个事件，驱动状态机
    for {
        if !c.stream.Next() { break }
        c.processEvent(c.stream.Current())

        if len(c.pending) > 0 {
            evt := c.pending[0]
            c.pending = c.pending[1:]
            return evt, false, nil
        }
        if c.done { return nil, true, nil }
    }

    // 3. 上游结束
    return nil, true, nil
}
```

`processEvent` 只操作内部状态和 `pending`，不写任何网络 IO。

---

## 5. 事件类型映射

| Converter 文件 | 上游 | 目标事件类型 |
|----------------|------|--------------|
| `openai_chat_to_responses.go` | `openai.ChatCompletionChunk` | Responses API event maps |
| `openai_responses_to_chat.go` | `responses.ResponseStreamEventUnion` | `map[string]interface{}` (OpenAI chunk) |
| `anthropic_beta_to_openai_responses.go` | `anthropic.BetaRawMessageStreamEventUnion` | `wire.ResponsesEvent` |
| `anthropic_to_openai.go` | `anthropic.BetaRawMessageStreamEventUnion` | `map[string]interface{}` (OpenAI chunk) |
| `openai_to_anthropic_converter.go` | `openai.ChatCompletionChunk` | `anthropicStreamEvent` |
| `openai_responses_to_anthropic_converter.go` | `responses.ResponseStreamEventUnion` | `anthropicStreamEvent` |

`anthropicStreamEvent` 是一个内部包装类型：

```go
type anthropicStreamEvent struct {
    eventType string
    data      map[string]interface{}
}
```

Writer 将其转换为 SSE，格式为 `event:<type>\ndata:<json>\n\n`（gin `c.SSEvent`，冒号后无空格）。

---

## 6. SSE Writer 规范

### Anthropic 格式
```go
// anthropicSSEWriter: 标准 anthropic SSE，同时回调 stream_event_recorder
func anthropicSSEWriter(c *gin.Context) func(interface{}) error

// anthropicSSEWriterWithFirstChunk: 在首个事件写出前调用 CommitFirstChunk
// 用于 Responses → Anthropic 路径（替代原 SendMessageStart 里的内联调用）
func anthropicSSEWriterWithFirstChunk(c *gin.Context) func(interface{}) error
```

### OpenAI Chat 格式
```go
func openaiChatSSEWriter(c *gin.Context) func(interface{}) error  // data: <json>\n\n
```

### OpenAI Responses 格式
```go
func responsesSSEWriter(c *gin.Context) func(interface{}) error   // event: <type>\ndata: <json>\n\n（冒号后有空格）
```

> **注意**：Anthropic 和 Responses 的 SSE 格式不同（冒号后是否有空格）。混用会导致客户端解析失败，已有测试覆盖。

---

## 7. MCP Hook 模式

MCP `OnToolCallsFinal` 需要在状态机内部（工具调用完整收集后）执行，不适合移到外层。Converter 内部处理：

```go
// 在 processEvent 的 message_stop 分支
if err := c.hooks.OnToolCallsFinal(c.pendingToolCalls); err != nil {
    c.hookErr = err   // 存储，不从 Next() 返回
    c.done = true
    return            // 不 emit 最终 chunk
}
```

外层通过 `conv.HookErr()` 读取：

```go
_, err := RunConverter(hc, conv, writer)
if hookErr := conv.HookErr(); errors.Is(hookErr, ErrMCPStreamContinue) {
    return in, out, hookErr  // 触发 MCP 重试循环
}
```

---

## 8. 错误处理约定

```
RunConverter 返回的 err
  ├── errors.Is(err, context.Canceled) → client 断开，返回 nil（正常）
  ├── 其他 err → ProcessStream 已调用 OnStreamErrorHooks
  │              直接 dispatch SSE 错误事件并返回 err
  └── nil → 检查 stream.Err()（基础设施错误，ProcessStream 未感知）
              → 需要手动 hc.DispatchStreamError(streamErr)

conv.HookErr() != nil
  → 协议级错误（如 response.failed）
  → SSE 错误事件已由 converter emit，调用方只需返回 error，不再发 SSE
```

---

## 9. 保留的旧机制

以下内容未迁移，仍使用旧模式：

| 函数 | 原因 |
|------|------|
| `HandleResponsesToAnthropicV1Assembly` | 非流式，内存聚合后一次性 JSON 响应 |
| `HandleResponsesToAnthropicBetaAssembly` | 同上 |
| `handlerResponsesToAnthropicStream`（v1 文件内） | 仅供 Assembly 使用的旧实现，共享状态机逻辑 |
| `StreamLoop` | `openai_chat.go` 和 `openai_passthrough.go` 透传路径仍在使用 |

---

## 10. 已移除

- `HandleContext.DispatchStreamEvent()`：Phase 0–6 完成后无任何调用方，已删除。所有 converter 路径通过 `ProcessStream` 自动 dispatch `OnStreamEventHooks`。

---

## 11. 新增文件速查

| 文件 | 内容 |
|------|------|
| `stream/converter.go` | `StreamConverter` 接口 + `RunConverter` |
| `stream/openai_chat_to_responses.go` | `chatToResponsesConverter` |
| `stream/openai_responses_to_chat.go` | `responsesToChatConverter` |
| `stream/anthropic_beta_to_openai_responses.go` | `anthropicBetaToResponsesConverter` |
| `stream/anthropic_to_openai.go` | `anthropicToOpenAIConverter` |
| `stream/openai_to_anthropic_converter.go` | `openAIToAnthropicConverter` + `anthropicStreamEvent` + writers |
| `stream/openai_responses_to_anthropic_converter.go` | `responsesToAnthropicConverter` |
