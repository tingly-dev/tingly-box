# Stream Usage Tracking

> 适用对象：tingly-box 后端贡献者，特别是改 `internal/protocol/stream/`、`internal/protocol/token/`、`internal/protocol/assembler/`、`internal/server/recording_hooks.go`，或在 `vmodel/` 内增 mock 的人。

**Status: shipped** in PR #1063 on `claude/keen-ramanujan-qUaXP`。

---

## 1. 问题域

OpenAI ↔ Anthropic 互转的 stream handler 要把上游 usage 一路带到：

1. 客户端看到的 Anthropic `message_delta.usage`（输出协议层）
2. 返回给调用方的 `*ai.TokenUsage`（in-process API 层）
3. 计费 / 记录用的 `trackUsageWithTokenUsage`（observability 层）
4. 录制下来的 assembled response（PR/日志回放层）

完整 usage shape 不只有 input/output，还有：

| 维度 | OpenAI Chat | OpenAI Responses | Anthropic |
|---|---|---|---|
| Prompt | `usage.prompt_tokens` | `usage.input_tokens` | `usage.input_tokens` |
| Completion | `usage.completion_tokens` | `usage.output_tokens` | `usage.output_tokens` |
| Cache read | `usage.prompt_tokens_details.cached_tokens` | `usage.input_tokens_details.cached_tokens` | `usage.cache_read_input_tokens` |
| Cache creation | — | — | `usage.cache_creation_input_tokens` |
| Reasoning | `usage.completion_tokens_details.reasoning_tokens` | `usage.output_tokens_details.reasoning_tokens` | —（计入 output_tokens） |

任何一段没拿全，下游记录就缺字段。

---

## 2. 改动前的三个 bug

### 2.1 OpenAI 流的尾 usage chunk 被丢

OpenAI 在 `stream_options.include_usage=true` 时（tingly-box 在 Anthropic→OpenAI 请求转换里强制开启），最后一个 chunk 形态为：

```
{ "choices": [],   "usage": {"prompt_tokens": ..., "completion_tokens": ..., ...} }
```

它 **晚于** 携带 `finish_reason` 的 chunk。旧版 `handleOpenAIToAnthropicStreamResponse` 在收到 `finish_reason` 立刻 `return false`，于是这条 usage-only chunk 永远读不到 —— `state.cacheTokens` / `state.reasoningTokens` / 甚至权威的 `input_tokens` / `output_tokens` 都丢了，只剩本地 tiktoken 估算。

### 2.2 Anthropic→OpenAI 反向只搬基础字段

`anthropic_to_openai.go` 抽 `message_delta.usage` 时只读 `InputTokens` / `OutputTokens`，`CacheReadInputTokens` 静默丢弃。Anthropic 没有 reasoning_tokens（thinking tokens 已计入 output），这一侧 N/A。

### 2.3 streamRecorder 截胡 cache

`streamRecorder.Finish(model, in, out)` 只接 input/output；`AnthropicStreamAssembler.SetUsage(in, out)` 也只存 input/output；assembled `anthropic.Message.Usage.CacheReadInputTokens = 0`。converter 那侧拼了命算出来的 cache 进 recorder 前就被丢了。

### 2.4 缺一个能落到 Info 日志的 usage 总览

`trackUsageWithTokenUsage` 仅在 `Trace` 级别打全字段（默认不可见）；converter 本身一行总览都没有。线上想看一次请求的 token 分布就只能开 trace。

---

## 3. 设计

### 3.1 Stream 转换：drain 到底再发终态

`handleOpenAIToAnthropicStreamResponse` / `handleOpenAIToAnthropicBetaStream` 改为：

- `finish_reason` chunk 不再立刻 `return false`，只记录 `pendingFinishReason` + `finishSeen = true` 然后继续循环
- `len(choices) == 0` 的 chunk（包含尾 usage-only chunk）落入既有的 token-counter 喂入分支
- 流自然结束（`stream.Next()` 返回 false）后，在 post-loop 里读最终 counter，发 stop / message_delta / message_stop

```go
StreamLoop(c, func(w io.Writer) bool {
    // ... process content / tool_calls deltas ...

    if len(chunk.Choices) == 0 {
        tokenCounter.ConsumeOpenAIChunk(&chunk) // 尾 usage chunk 走这里
        return true
    }

    if choice.FinishReason != "" {
        pendingFinishReason = choice.FinishReason
        finishSeen = true
        // 不返回 false，继续 drain
    }
    return true
})

if finishSeen && hookErr == nil {
    // 从 counter 同步最终 input/output/cache/reasoning
    // 发 message_delta + message_stop
    // 打一条 Info "OpenAI->Anthropic stream usage"
}
```

### 3.2 StreamTokenCounter 扩展 cache + reasoning

`StreamTokenCounter` 之前只抽 `chunk.Usage.PromptTokens` / `CompletionTokens`。新增：

```go
// 读 chunk.Usage.PromptTokensDetails.CachedTokens
// 读 chunk.Usage.CompletionTokensDetails.ReasoningTokens
func (c *StreamTokenCounter) GetUpstreamDetails() (cacheTokens, reasoningTokens int)
```

post-finish 块从这里取，写进 `state.cacheTokens` / `state.reasoningTokens`，最终走 `sendMessageDelta`（已支持 `cache_read_input_tokens`）和返回的 `ai.TokenUsage`。

### 3.3 反向流：anthropic_to_openai 映射 cache_read

```go
chunk.Usage = openai.CompletionUsage{
    PromptTokens:     usage.InputTokens,
    CompletionTokens: usage.OutputTokens,
    TotalTokens:      usage.InputTokens + usage.OutputTokens,
}
if usage.CacheReadInputTokens > 0 {
    chunk.Usage.PromptTokensDetails.CachedTokens = usage.CacheReadInputTokens
}
```

Anthropic 的 `CacheCreationInputTokens` 在 OpenAI Chat 协议里没对应字段，物理上不能搬。reasoning N/A。

### 3.4 Recorder 链：以 `*ai.TokenUsage` 为流通货币

把 `(int, int)` 这种到处散落的字段对替换成 canonical type，避免每加一个字段就改一圈签名：

```go
// 旧：streamRecorder.Finish(model, inputTokens, outputTokens)
// 新：
func (sr *streamRecorder) Finish(model string, usage *protocol.TokenUsage)

// 旧：assembler.SetUsage(inputTokens, outputTokens)
// 新增（旧保留作为简单入口）：
func (a *AnthropicStreamAssembler) SetUsageFromTokenUsage(u *ai.TokenUsage)
```

`SetUsageFromTokenUsage` 把 `CacheInputTokens` 落到 `anthropic.Usage.CacheReadInputTokens`。reasoning 在 Anthropic 没有对应字段，丢弃（与协议一致）。

同时修补一个独立的丢字段：`AnthropicStreamAssembler.RecordV1Event` / `RecordV1BetaEvent` 处理 `message_delta` 时本来只复制 input/output，新版把 `event.Usage.CacheReadInputTokens` 也带上 —— 上游事件里就有，但被 assembler 截胡的话本 PR 后半段的 cache_read 通路在某些路径上其实通不到底。

### 3.5 Info 级 usage 总览

v1 + beta 流终态各打一条：

```
level=info msg="OpenAI->Anthropic stream usage"
  model=virtual-stream-test
  input_tokens=42  output_tokens=17
  cache_tokens=11  reasoning_tokens=9
  stop_reason=end_turn
```

`trackUsage` 旁路日志从 Trace 升到 Debug，并补 `reasoning_tokens` 字段。

---

## 4. vmodel 测试基建

PR 顺手补了一个能在 in-process 测端到端 usage 的开关。

### 4.1 MockUsage

`vmodel/defaults_shared.go`：

```go
type MockUsage struct {
    PromptTokens             int64
    CompletionTokens         int64
    CachedInputTokens        int64 // OpenAI cached_tokens / Anthropic cache_read
    CacheCreationInputTokens int64 // Anthropic only
    ReasoningTokens          int64 // OpenAI only
}
```

写进 SharedMockSpec（仅协议无关字段），两个协议的 `MockModelConfig` 都加 `Usage *vmodel.MockUsage` 字段。

### 4.2 UsageEvent

`vmodel/openai/stream.go` 和 `vmodel/anthropic/stream.go` 各加：

```go
type UsageEvent struct { Usage vmodel.MockUsage }
```

`MockModel.HandleOpenAIChatStream` / `HandleAnthropicStream` 在 `DoneEvent` 之前 emit。

### 4.3 virtualserver 渲染成 wire 格式

- **OpenAI**：`req.StreamOptions.IncludeUsage.Value || explicitUsage != nil` 时，在 `finish_reason` chunk 后、`[DONE]` 前发尾 usage-only chunk，`PromptTokensDetails.CachedTokens` / `CompletionTokensDetails.ReasoningTokens` 都填。
- **Anthropic**：在 `message_stop` 前发 `message_delta`，usage 块带 `input_tokens` / `output_tokens` / `cache_read_input_tokens` / `cache_creation_input_tokens` / `reasoning_tokens`。

### 4.4 opt-in 注册

```go
openaivm.RegisterStreamTestMocks(svc.GetOpenAIRegistry())
anthropicvm.RegisterStreamTestMocks(svc.GetAnthropicRegistry())
```

故意不进 `RegisterDefaults` —— 生产 registry / 用户面 demo 列表保持干净。规范见 `defaults_shared.go` 的 doc-comment：「Test-only fixtures must NOT be added to SharedDefaultMocks.」

两个 spec：

| ID | 类型 | 用途 |
|---|---|---|
| `virtual-stream-test` | static text | 验完整 usage shape 在 text 路径上 |
| `virtual-stream-test-tool` | tool_call | 验完整 usage shape 在 tool path + `stop_reason=tool_use` 映射 |

usage 数值固定（PromptTokens=42, CompletionTokens=17, CachedInputTokens=11, CacheCreationInputTokens=5, ReasoningTokens=9），断言端可硬编码。

### 4.5 测试落点

| 测试 | 覆盖 |
|---|---|
| `vmodel/virtualserver/stream_test_mocks_test.go` | 两个协议的 wire 格式（OpenAI 尾 usage chunk 字段、Anthropic message_delta.usage 字段），static + tool 两种变体 |
| `internal/protocol/stream/openai_to_anthropic_vmodel_e2e_test.go::TestOpenAIToAnthropicStream_VModelFullUsage` | OpenAI→Anthropic 完整链路：vmodel 上游 + converter + 终端 `ai.TokenUsage` 四字段 + `stop_reason=tool_use` 映射 |
| `internal/protocol/stream/anthropic_to_openai_vmodel_e2e_test.go::TestAnthropicToOpenAIStream_VModelFullUsage` | 反向链路：上游 cache_read 落到下游 prompt_tokens_details.cached_tokens |
| `internal/protocol/assembler/anthropic_assembler_test.go::TestAnthropicStreamAssembler_SetUsageFromTokenUsage_CarriesCacheRead` | 单测：cache_read 经 assembler 进 assembled response |

回滚验证：把 converter 那侧的 fix `git stash` 掉，上述 E2E 测试会失败（OutputTokens=0 / cached_tokens=0），证明测试真在抓 bug。

---

## 5. 日志策略

每 chunk 一条 Debug 是这个 PR 在追那个 finish_reason→usage drop bug 时的产物。落地后清理标准：

**保留**：
- 每请求 ≤ 1 次的 Start / Finish 边界
- 每请求 1 次的终态 Info `OpenAI->Anthropic stream usage`（前述总览）
- 每 block 1 次的 `Initializing thinking block` / `Thinking block done`
- 异常路径（panic, client disconnect, stream error）
- 状态事件（in_progress / completed / generating / searching 等，每请求顶多几条）

**删**：
- 任何"每 chunk"或"每 delta"的 Debug（content、thinking、annotation、audio、code-interpreter、output_item.added 等）
- 任何 dump 完整 RawJSON 或 marshalled message 的 Debug（前 N chunk dump、Assemble response 等）—— 大 payload 本身就是 HTTP body，重复存档没意义
- 只服务于这些日志的本地变量（`chunkCount`、`eventCount`、`hasValidUsage`、`hasNonZeroUsage`、`preview` 等）

清理范围限本 PR 引入或受影响的文件，不动 stream package 里其他历史日志。

---

## 6. 类似 bug 的扫描清单

PR 期间用 Explore agent 扫了 `internal/protocol/stream/` + `internal/protocol/nonstream/`，按"提前 return"与"字段映射不全"两条标准查同类 bug。

| 文件 | 状态 | 备注 |
|---|---|---|
| `stream/openai_to_anthropic.go` + `_beta.go` | ✅ 修 | 本 PR 主题 |
| `stream/anthropic_to_openai.go` | ✅ 修 | cache_read 字段补全 |
| `stream/anthropic_beta_to_openai_responses.go` | OK | 已抽 cache_read；`CacheCreationInputTokens` 在目标协议里没字段 |
| `stream/openai_chat_to_responses.go` | OK | usage 持续抽取，无早退 |
| `stream/openai_responses_to_chat.go` | OK | 完整抽 input/output/cache/reasoning |
| `nonstream/openai_to_anthropic.go` | OK | reasoning 在 Anthropic 无对等字段；其余完整 |
| `nonstream/anthropic_to_openai.go` | OK | cache_read 已映射；Anthropic 不发 reasoning |
| `nonstream/openai_responses_to_chat.go` | OK | 完整 |
| `stream/google_to_any.go` + `nonstream/google_to.go` | skip | 当前不投入 |

新增 stream converter 时按这个清单核对一遍。
