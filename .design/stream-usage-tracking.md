# Usage & Token Tracking

> 适用对象：tingly-box 后端贡献者，特别是改 `ai/protocol.go`（canonical type）、`internal/protocol/usage/`（normalization）、`internal/protocol/token/`（streaming counter）、`internal/protocol/stream/` + `internal/protocol/nonstream/`（converter）、`internal/protocol/assembler/`、`internal/server/usage_tracking.go` + `recording_hooks.go`、`internal/data/db/usage_record.go`、`internal/server/module/usage/`（API），或在 `vmodel/` 内增 mock 的人。
>
> 这份文档覆盖**整条 usage 通路**，不只 stream。原始 PR #1063（stream drain 修复）的 rationale 保留在 §8 历史。

---

## 0. 一张图看懂

```
        provider wire (OpenAI / Anthropic / Google)
                          │
        ┌─────────────────┴──────────────────┐
        │  非流式 nonstream          流式 stream │
        │  usage.From*(resp.Usage)            │
        │                    ┌────────────────┤
        │                    │ Anthropic 上游 │ OpenAI 上游
        │                    │ usage.Anthropic│ token.StreamTokenCounter
        │                    │   Accumulator  │   (增量 tiktoken + 上游 usage chunk)
        └────────┬───────────┴───────┬────────┘
                 ▼                    ▼
        ┌────────────────────────────────────┐
        │  canonical  *ai.TokenUsage          │  ← 全链路流通货币
        │  {Input, Output, CacheInput,        │
        │   Reasoning, System}                │
        └───────┬───────────────────┬─────────┘
                │ (录制路径)         │ (计费/observability 路径)
                ▼                    ▼
   assembler.AnthropicStreamAssembler   server.trackUsageWithTokenUsage(c, usage, err)
   .SetUsageFromTokenUsage(u)            ├─ updateServiceStats        (内存 stats)
   → streamRecorder.Finish(model, u)     ├─ tokenTracker.RecordUsage  (OTel)
   → assembled anthropic.Message         ├─ recordDetailedUsage...    (DB UsageRecord)
                                         └─ reportHealthStatus / 429 hook
                                                    │
                                                    ▼
                              internal/data/db  →  GET /api/v1/usage/{stats,timeseries,records}
```

核心原则：**所有 provider 的 usage 先归一化成 `*ai.TokenUsage`，再往下游分发。** 任何一段没拿全，下游记录就缺字段。

---

## 1. Canonical type：`ai.TokenUsage`

定义：`ai/protocol.go:61`。这是全链路唯一的流通货币——converter / recorder / tracker / DB 之间都传它，避免「每加一个字段就改一圈 `(int, int)` 签名」。

```go
type TokenUsage struct {
    InputTokens      int `json:"input_tokens"`                 // 输入/prompt，已扣除 cache
    OutputTokens     int `json:"output_tokens"`                // 输出/completion
    CacheInputTokens int `json:"cache_input_tokens,omitempty"` // cache creation + cache read 合并
    ReasoningTokens  int `json:"reasoning_tokens,omitempty"`   // o1/o3 reasoning，是 OutputTokens 的子集
    SystemTokens     int `json:"system_tokens,omitempty"`      // 模板/系统指令/框架开销
}
```

方法 / 工厂：

| 名称 | 作用 |
|---|---|
| `TotalTokens()` | `Input + Output`（**不含 cache**；cache 单独算成本） |
| `HasUsage()` | 任一 Input/Output/Cache/System > 0 |
| `HasCacheUsage()` | `CacheInputTokens > 0` |
| `NewTokenUsage(in, out)` | 基础 |
| `NewTokenUsageWithCache(in, out, cache)` | + cache |
| `NewTokenUsageFull(in, out, cache, reasoning)` | + reasoning（OpenAI 路径用） |
| `ZeroTokenUsage()` | 零值，用于「无 usage」回退 |

> ⚠️ 注意 `CacheInputTokens` 是 **creation + read 合并**后的单字段（见 §2 归一化）。早期版本曾分 `cache_creation` / `cache_read` 两列，已合并（DB 迁移见 §6.2）。

---

## 2. 归一化层：`internal/protocol/usage/`

各 provider 上报 token 的语义互不兼容，必须先归一化，否则前端 cache-hit 公式算不对：

```
cache_hit_ratio = CacheInputTokens / (InputTokens + CacheInputTokens)
```

| Provider | wire 语义 | 归一化后 InputTokens | 归一化后 CacheInputTokens |
|---|---|---|---|
| **OpenAI Chat / Responses** | `prompt_tokens` = 总数（含 cached） | `prompt_tokens − cached_tokens` | `cached_tokens` |
| **Anthropic** | `input_tokens` = 仅未命中；`cache_creation_input_tokens` = 写入成本 | `input_tokens + cache_creation_input_tokens` | `cache_read_input_tokens` |

> **为什么把 `cache_creation` 加进 input？** creation 按写入价计费，属于「本次 prompt 总花费」，要进分母；read 命中是省下来的，单独放 cache。

完整 usage shape（哪个字段从哪来）：

| 维度 | OpenAI Chat | OpenAI Responses | Anthropic |
|---|---|---|---|
| Prompt | `usage.prompt_tokens` | `usage.input_tokens` | `usage.input_tokens` |
| Completion | `usage.completion_tokens` | `usage.output_tokens` | `usage.output_tokens` |
| Cache read | `usage.prompt_tokens_details.cached_tokens` | `usage.input_tokens_details.cached_tokens` | `usage.cache_read_input_tokens` |
| Cache creation | —（无对应） | —（无对应） | `usage.cache_creation_input_tokens` |
| Reasoning | `usage.completion_tokens_details.reasoning_tokens` | `usage.output_tokens_details.reasoning_tokens` | —（计入 output_tokens） |

### 2.1 非流式：纯函数（`extract.go`）

```go
usage.FromOpenAIChatCompletion(resp.Usage) // openai.CompletionUsage
usage.FromOpenAIResponses(resp.Usage)      // responses.ResponseUsage
usage.FromAnthropicMessage(resp.Usage)     // anthropic.Usage  (v1)
usage.FromAnthropicBetaMessage(resp.Usage) // anthropic.BetaUsage
```

OpenAI 侧：`InputTokens = prompt − cached`，cache/reasoning 直接读 details。
Anthropic 侧：`InputTokens = input + cache_creation`，`CacheInputTokens = cache_read`，无 reasoning。

反向（`*TokenUsage` → wire）：

```go
usage.ChatUsage(u) // → openai.CompletionUsage：PromptTokens = Input + Cache（还原成总数），
                   //   CachedTokens / ReasoningTokens 填回 details
```

### 2.2 流式 Anthropic：`AnthropicAccumulator`（`accumulator.go`）

Anthropic 把 usage 拆在两个事件里：

- `message_start` → `input_tokens` / `cache_creation_input_tokens` / `cache_read_input_tokens`
- `message_delta` → `output_tokens`（个别非标准 provider 也在这里塞 `input_tokens`）

```go
acc := usage.NewAnthropicAccumulator()
// 事件循环里，每个事件都喂（只有 usage-carrying 事件有效）：
acc.Consume(&evt)     // MessageStreamEventUnion（非 beta）
acc.ConsumeBeta(&evt) // BetaRawMessageStreamEventUnion（beta）
// 收尾：
if acc.HasUsage() { return acc.Result(), nil }
return protocol.ZeroTokenUsage(), nil
```

优先级（`consumeRaw`）：

- **Input**：`message_start` 优先，回退 `message_delta`；每个来源还有 SDK 字段 + gjson raw 两条路（非标准 provider 兜底，仅非 beta 需要 gjson，beta 的 SDK 字段可靠）
- **Output**：只看 delta
- **Cache read**：`message_start` 优先，回退 delta，单独存 `cacheTokens`
- **Cache creation**：直接 `+=` 进 `inputTokens`（归一化，见上）

### 2.3 流式 OpenAI：`StreamTokenCounter`（`internal/protocol/token/`）

OpenAI 流不像 Anthropic 那样在 `message_start` 给 input，所以用**增量 tiktoken 估算 + 尾 usage chunk 校正**双轨：

```go
type StreamTokenCounter struct {
    inputTokens, outputTokens           int   // 本地估算
    upstreamInputTokens, ...Output      int64 // 来自尾 usage chunk
    upstreamCacheTokens                 int64 // prompt_tokens_details.cached_tokens
    upstreamReasoning                   int64 // completion_tokens_details.reasoning_tokens
}
```

`ConsumeOpenAIChunk(chunk)`：
- chunk 带 usage（通常是 `stream_options.include_usage=true` 的**尾 usage-only chunk**，`choices` 为空）→ 抓权威 input/output/cache/reasoning，**覆盖**本地估算
  - SDK 的 `JSON.Usage.Valid()` 会漏掉部分合法情况，所以同时接受 `PromptTokens>0 || CompletionTokens>0` 作为「usage 存在」的证据
- 否则 → 对每个 delta（content / refusal / tool_call name+args / 旧式 function_call）增量 tiktoken 累加 output

取数：
- `GetCounts() → (input, output)`：有上游就用上游，`input = upstreamInput − upstreamCache`（归一化成仅未命中）
- `GetUpstreamDetails() → (cache, reasoning)`：上游 usage chunk 里抓到的 cache/reasoning（没有则 0）

tiktoken：默认 `O200kBase`；`EstimateInputTokens(req)` 在流前预置 input；`countTokens` 失败回退 `len(text)/4`。Anthropic 侧另有 `EstimateAnthropicTokens`。

---

## 3. Converter 覆盖矩阵

> 新增 converter 时，对照这张表确认 usage 通路接上了，并按 §10 的「提前 return / 字段映射不全」两条标准自查。

### 3.1 `internal/protocol/nonstream/`

| Handler | 提取器 |
|---|---|
| `HandleOpenAIChatNonStream` | `usage.FromOpenAIChatCompletion` |
| `HandleOpenAIResponsesNonStream` | `usage.FromOpenAIResponses` |
| `HandleAnthropicV1NonStream` | `usage.FromAnthropicMessage` |
| `HandleAnthropicV1BetaNonStream` | `usage.FromAnthropicBetaMessage` |
| `nonstream/anthropic_to_openai.go` | inline（返回 wire `map[string]interface{}`，不是 `*TokenUsage`） |
| `nonstream/openai_to_anthropic.go` | inline（同上）；reasoning 在 Anthropic 无对等字段 |

### 3.2 `internal/protocol/stream/`

| Handler / converter | 机制 |
|---|---|
| `HandleAnthropic` | `AnthropicAccumulator.Consume` |
| `HandleAnthropicBeta` | `AnthropicAccumulator.ConsumeBeta` |
| `AnthropicToOpenAIStreamWithMCPHooks`（`anthropic_to_openai*.go`） | `AnthropicAccumulator.ConsumeBeta`；`Usage()` 返回 `acc.Result()` |
| `HandleAnthropicBetaToOpenAIResponsesStream` | `AnthropicAccumulator.ConsumeBeta` |
| `openAIToAnthropicConverter`（`openai_to_anthropic_converter.go` + `_beta`） | `StreamTokenCounter`；`Usage()` 返回 `NewTokenUsageFull(in, out, cache, reasoning)` |
| `openai_passthrough.go` | inline：每 chunk 累加 + 估算 fallback 注入 |
| `openai_{chat,responses}_to_*.go` | inline：`state` 字段双用（同时拼 wire body） |
| `google_to_any.go` | inline：Google SDK 无结构化 cache 子字段 |

OpenAI→Anthropic converter 的终态收口在 `emitTerminalEvents()`：从 counter 同步 `GetCounts()` + `GetUpstreamDetails()` 写进 `state`，再发 `message_delta` / `message_stop`，并打一条 **Debug** 总览日志（见 §10）。

### 3.3 `internal/server/`（dispatch 层）

| 代码点 | 提取器 |
|---|---|
| `protocol_dispatch` — Anthropic Beta 非流式（×2） | `FromAnthropicBetaMessage` |
| `protocol_dispatch` — Responses → Anthropic Beta | `FromAnthropicBetaMessage` |
| `protocol_dispatch` — OpenAI Chat 非流式（×2） | `FromOpenAIChatCompletion` |
| `protocol_dispatch` — OpenAI Responses 非流式（×2） | `FromOpenAIResponses` |
| `anthropic_message_v1` — Responses → Anthropic v1 | `FromOpenAIResponses` |
| `anthropic_message_beta` — Responses → Anthropic Beta | `FromOpenAIResponses` |
| `protocol_dispatch` — Google 非流式 | inline（Google schema 无 cached） |

---

## 4. 录制链：Assembler + Recorder

录制路径（PR 回放 / 日志）和计费路径并行，同样以 `*ai.TokenUsage` 为货币。

### 4.1 `AnthropicStreamAssembler`（`internal/protocol/assembler/anthropic_assembler.go`）

把流式事件攒成一个完整的 `anthropic.Message`。

- `RecordV1Event` / `RecordV1BetaEvent`：处理 `message_start`（msgID/role）、`content_block_*`（攒 text/thinking/tool input）、`message_delta`（stop_reason + 若带 usage 则存 `usageData`，**含 `CacheReadInputTokens`**）
- `SetUsage(in, out)`：原始计数（简单入口，优先用下面那个）
- `SetUsageFromTokenUsage(u *ai.TokenUsage)`：canonical 入口
  - `InputTokens → anthropic.Usage.InputTokens`
  - `OutputTokens → anthropic.Usage.OutputTokens`
  - `CacheInputTokens → anthropic.Usage.CacheReadInputTokens`
  - `ReasoningTokens` 丢弃（Anthropic 无对应字段，已计入 output）
- `Finish(model, in, out) → *anthropic.Message`：有 `SetUsage*` 数据就用它，否则回退入参

### 4.2 `streamRecorder`（`internal/server/recording_hooks.go`）

```go
type streamRecorder struct {
    recorder        *ProtocolRecorder
    assembler       *assembler.AnthropicStreamAssembler
    inputTokens, outputTokens, cacheReadTokens int  // RecordRawMapEvent 兜底用
    hasUsage        bool
}

func (sr *streamRecorder) Finish(model string, usage *protocol.TokenUsage)
```

- `Finish(model, usage)`：usage 非空就 `assembler.SetUsageFromTokenUsage(usage)` + `assembler.Finish(model, usage.Input, usage.Output)`；usage 为 nil/zero 但 `RecordRawMapEvent` 攒到过 → 用内部兜底计数
- `RecordRawMapEvent(type, event)`：把 SSE 事件喂给 assembler + recorder chunk log；遇 `message_delta` 抽 input/output/cache_read 更新内部计数，置 `hasUsage`
- `AttachRecorderHooks(...)`：把 `ProtocolRecorder` 接进原生 Anthropic 流，装 `WithOnStreamEvent`（镜像到 recorder + assembler）/ `WithOnStreamComplete`（`asm.Finish` + `recorder.RecordResponse`）/ `WithOnStreamError`

---

## 5. 计费 / observability 层：`internal/server/usage_tracking.go`

两个入口：

### 5.1 `trackUsageWithTokenUsage(c, usage *TokenUsage, err)` —— 首选

完整字段（cache / reasoning / system）都走这条。流程：

1. `GetTrackingContext(c)` 取 rule / provider / model / requestModel / scenario / streamed / startTime（任一缺失或 `usage==nil` 直接 return）
2. 算 latency、status（success / error / canceled）、errorCode
3. 打一条 **Debug** `"trackUsage: token usage recorded"`，带 `input/output/cache/reasoning/system/total_tokens` + status/streamed/latency
4. `detectCacheHit(usage)` → `SetCacheHit(c, …)`；算 `TTFT` / `TPS`
5. 分发：
   - `updateServiceStats(rule, provider, model, MetricsData{...})`（内存 stats）
   - `tokenTracker.RecordUsage(ctx, UsageOptions{... CacheInputTokens, SystemTokens ...})`（OTel）
   - `recordDetailedUsageWithTokenUsage(...)`（写 DB，见 §6）
   - `reportHealthStatus(...)`；429 时 enterprise 限流告警 hook

`MetricsData`：`InputTokens / OutputTokens / LatencyMs / TTFTMs / CacheHit / TPS`。

### 5.2 `trackUsageFromContext(c, inputTokens, outputTokens, err)` —— 旧式 2-int 入口

只有 input/output 的简化路径（cache/reasoning/system 会丢）。新代码尽量用 5.1。

> 日志层级：入口诊断 = **Trace**；usage 总览 = **Debug**；health / 429 = **Warn**。（早期文档把 stream 总览写成 Info —— 已统一降到 Debug，见 §8.4 / §10。）

---

## 6. 持久化：`internal/data/db/usage_record.go`

### 7.1 模型

`UsageRecord`（表 `usage_records`，逐条记录）：

| 列 | 说明 |
|---|---|
| `provider_uuid` / `provider_name` / `model` / `request_model` | 路由维度 |
| `scenario` / `rule_uuid` | 场景 / 规则 |
| `user_id` | 多租户（`not null; default ''`，迁移见下） |
| `timestamp` | 索引（含 `idx_timestamp_scenario`） |
| `input_tokens` / `output_tokens` / `total_tokens` | `total = input + output` |
| `cache_input_tokens` | **合并** cache creation + read（`default 0`） |
| `system_tokens` | 框架开销（`default 0`） |
| `status` / `error_code` | success / error / partial / canceled |
| `latency_ms` / `ttft_ms` / `streamed` | 性能 |

聚合表：`UsageDailyRecord`（`usage_daily`）、`UsageMonthlyRecord`（`usage_monthly`）——`RequestCount / TotalTokens / Input / Output / CacheInputTokens / SystemTokens / ErrorCount`，按 `date(timestamp)` 或 `year/month` SUM。

### 7.2 Schema 迁移（`ensureUsageRecordSchema`）

dev-stage 破坏式清理，按存在性条件执行：

1. **删 `department_id`**：废弃维度
2. **合并 cache 列**：`cache_input_tokens = COALESCE(cache_creation_input_tokens,0) + COALESCE(cache_read_input_tokens,0)`，然后 DROP 掉那两列 —— 这就是 §1 里「合并单字段」的来历
3. **回填 `user_id`**：空 / NULL → `DefaultAdminUserID`（`"admin"`），兼容多租户之前的旧记录

### 7.3 查询

- `GetAggregatedStats(UsageStatsQuery)`：`groupBy ∈ {model, provider, scenario, rule, user, daily, hourly}`
- `GetTimeSeries(interval ∈ {minute, hour, day, week}, start, end, filters)`
- `GetRecords(start, end, filters, limit, offset)`：分页逐条

---

## 7. 对外 API：`internal/server/module/usage/`

| 路由 | 用途 |
|---|---|
| `GET /api/v1/usage/stats` | 聚合统计；`groupBy` + `filterBy`（provider/model/scenario/rule_uuid/user_id/status）+ `sortBy`（total_tokens/request_count/avg_latency） |
| `GET /api/v1/usage/timeseries` | 时间序列；`interval` + 同款 filter |
| `GET /api/v1/usage/records` | 逐条记录（分页） |
| `DELETE /api/v1/usage/records` | 删 `older_than_days` 之前的记录 |

响应模型（`types.go`）：`UsageStatsResponse{Meta, []AggregatedStat}` / `TimeSeriesResponse{Meta, []TimeSeriesData}` / `UsageRecordsResponse{Meta, []UsageRecordResponse}` / `DeleteOldRecordsResponse{deleted_count, cutoff_date}`。

`AggregatedStat`：`Key / Provider* / Model / Scenario / UserID / RequestCount / TotalTokens / Input / Output / CacheInputTokens / AvgInput / AvgOutput / AvgLatencyMs / ErrorCount / ErrorRate / StreamedCount / StreamedRate`。

---

## 8. 历史：PR #1063 的 stream drain 修复

> **Status: shipped** in PR #1063 on `claude/keen-ramanujan-qUaXP`。下面是当时修的几个 bug 与设计取舍——大部分代码后来被重构进 §2/§3 的 converter 抽象，但 rationale 仍有参考价值。

### 8.1 改动前的 bug

- **OpenAI 尾 usage chunk 被丢**：旧 `handleOpenAIToAnthropicStreamResponse` 收到 `finish_reason` 立刻 `return false`，而 OpenAI 的 usage-only chunk（`choices:[]`）**晚于** finish chunk 到达，于是权威 input/output/cache/reasoning 全丢，只剩本地 tiktoken。
- **反向只搬基础字段**：`anthropic_to_openai` 抽 `message_delta.usage` 只读 input/output，`cache_read` 静默丢。
- **streamRecorder 截胡 cache**：`Finish(model, in, out)` 只接 input/output，assembled message 的 `CacheReadInputTokens=0`。
- **缺 Info 级总览**：当时想加一条每请求一次的 usage 总览。

### 8.2 设计：drain 到底再发终态

`finish_reason` chunk 不再立刻 `return false`，只记 `pendingFinishReason`；`choices` 为空的尾 usage chunk 喂进 token counter；流自然结束后在 post-loop 读最终 counter，发 stop / message_delta / message_stop。

> 这套逻辑现在落在 `openAIToAnthropicConverter.processChunk` + `emitTerminalEvents`（§3.2），不再是单个大函数。

### 8.3 当时引入、现已成为基础设施的部分

- `StreamTokenCounter.GetUpstreamDetails()`（cache + reasoning）→ §2.3
- `anthropic_to_openai` 映射 `cache_read → prompt_tokens_details.cached_tokens` → §3.2
- `streamRecorder.Finish(model, *TokenUsage)` + `assembler.SetUsageFromTokenUsage` → §4

### 8.4 ⚠️ 与当前实现的差异

原 PR 设计的「**Info 级** `OpenAI->Anthropic stream usage` 总览」——当前代码是 **Debug 级**（`openai_to_anthropic_converter.go:301`，`emitTerminalEvents` 里 `logrus.Debugf`）。`trackUsage` 总览同为 Debug。线上看 token 分布需开 Debug，不是 Info。

实际日志：

```
level=debug msg="OpenAI->Anthropic stream usage: model=... in=42 out=17 cache=11 reasoning=9 stop=stop"
```

---

## 9. vmodel 测试基建

PR #1063 顺手补的端到端 usage 开关，沿用至今。

### 9.1 `MockUsage`（`vmodel/defaults_shared.go`）

```go
type MockUsage struct {
    PromptTokens             int64
    CompletionTokens         int64
    CachedInputTokens        int64 // OpenAI cached_tokens / Anthropic cache_read
    CacheCreationInputTokens int64 // Anthropic only
    ReasoningTokens          int64 // OpenAI only
}
```

两个协议的 `MockModelConfig` 都加 `Usage *vmodel.MockUsage`。

### 9.2 `UsageEvent` + virtualserver 渲染

`vmodel/openai/stream.go` / `vmodel/anthropic/stream.go` 各加 `UsageEvent{Usage MockUsage}`，在 `DoneEvent` 前 emit。virtualserver 渲染：

- **OpenAI**：`finish_reason` chunk 后、`[DONE]` 前发尾 usage-only chunk，填 `PromptTokensDetails.CachedTokens` / `CompletionTokensDetails.ReasoningTokens`
- **Anthropic**：`message_stop` 前发 `message_delta`，带 `input/output/cache_read/cache_creation/reasoning`

### 9.3 opt-in 注册（故意不进 `RegisterDefaults`）

```go
openaivm.RegisterStreamTestMocks(svc.GetOpenAIRegistry())
anthropicvm.RegisterStreamTestMocks(svc.GetAnthropicRegistry())
```

生产 registry / 用户面 demo 列表保持干净（见 `defaults_shared.go` doc-comment：「Test-only fixtures must NOT be added to SharedDefaultMocks.」）。

| ID | 类型 | 用途 |
|---|---|---|
| `virtual-stream-test` | static text | 完整 usage shape on text 路径 |
| `virtual-stream-test-tool` | tool_call | 完整 usage shape on tool 路径 + `stop_reason=tool_use` |

固定数值（Prompt=42, Completion=17, Cached=11, CacheCreation=5, Reasoning=9），断言端硬编码。

### 9.4 测试落点

| 测试 | 覆盖 |
|---|---|
| `vmodel/virtualserver/stream_test_mocks_test.go` | 两协议 wire 格式（OpenAI 尾 usage chunk、Anthropic message_delta.usage），static + tool |
| `stream/openai_to_anthropic_vmodel_e2e_test.go::TestOpenAIToAnthropicStream_VModelFullUsage` | OpenAI→Anthropic 完整链路：vmodel 上游 + converter + 终端 `ai.TokenUsage` 四字段 + `stop_reason=tool_use` |
| `stream/anthropic_to_openai_vmodel_e2e_test.go::TestAnthropicToOpenAIStream_VModelFullUsage` | 反向：上游 cache_read 落到下游 `prompt_tokens_details.cached_tokens` |
| `assembler/anthropic_assembler_test.go::TestAnthropicStreamAssembler_SetUsageFromTokenUsage_CarriesCacheRead` | 单测：cache_read 经 assembler 进 assembled response |
| `internal/protocol/usage/usage_test.go` | `From*` / `AnthropicAccumulator` / `ChatUsage` 归一化单测 |

回滚验证：把 converter 那侧 fix `git stash`，E2E 会失败（OutputTokens=0 / cached_tokens=0），证明测试真在抓 bug。

---

## 10. 日志策略

每 chunk 一条 Debug 是当年追 finish_reason→usage drop bug 的产物。落地标准：

**保留**：
- 每请求 ≤1 次的 Start / Finish 边界
- 每请求 1 次的终态 Debug `OpenAI->Anthropic stream usage` 总览
- 每 block 1 次的 `Initializing thinking block` / `Thinking block done`
- 异常路径（panic / client disconnect / stream error）
- 状态事件（in_progress / completed / generating / searching，每请求顶多几条）

**删**：
- 任何「每 chunk / 每 delta」的 Debug（content / thinking / annotation / audio / code-interpreter / output_item.added）
- 任何 dump 完整 RawJSON / marshalled message 的 Debug（大 payload 本身就是 HTTP body，重复存档无意义）
- 只服务于这些日志的本地变量（`chunkCount` / `eventCount` / `hasValidUsage` / `hasNonZeroUsage` / `preview`）

清理范围限引入或受影响的文件，不动 stream package 其他历史日志。

---

## 11. 新增 converter 自查清单

按「**提前 return**」与「**字段映射不全**」两条标准核对（PR #1063 用 Explore agent 扫过 `stream/` + `nonstream/`）：

| 文件 | 状态 | 备注 |
|---|---|---|
| `stream/openai_to_anthropic_converter.go` + `_beta` | ✅ | drain 到底；走 `StreamTokenCounter` |
| `stream/anthropic_to_openai*.go` | ✅ | `AnthropicAccumulator`，cache_read 全 |
| `stream/anthropic_beta_to_openai_responses*.go` | ✅ | 已抽 cache_read；`cache_creation` 目标协议无字段 |
| `stream/openai_chat_to_responses*.go` | OK | usage 持续抽取，无早退 |
| `stream/openai_responses_to_chat*.go` | OK | 完整抽 input/output/cache/reasoning |
| `nonstream/openai_to_anthropic.go` | OK | reasoning 在 Anthropic 无对等；其余完整 |
| `nonstream/anthropic_to_openai.go` | OK | cache_read 已映射；Anthropic 不发 reasoning |
| `nonstream/openai_responses_to_chat.go` | OK | 完整 |
| `stream/google_to_any.go` + `nonstream/google_to.go` | skip | Google SDK 无结构化 cache 子字段 |

新增 stream / nonstream converter 时：① 确认 `Usage()`（或等价提取）接到 §2 的归一化函数；② cache_creation / cache_read / reasoning 三个易丢字段逐一核对；③ 流式注意「上游终态 chunk 是否晚于 finish」别提前 return。
