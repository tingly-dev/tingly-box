# Protocol Stage v2 — Guardrails × MCP 统一与重新落地

> Status: **Plan**（尚未开始编码）
> 前身：`origin/expr/protocol_stage`、`origin/feat/protocol-stage-hardening-port`
> （原设计文档 `protocol-stage-chain.md` / `protocol-stage-tool-loop.md` 保留在那两个分支上）。

---

## 0. 约束（本轮确定）

| # | 约束 | 含义 |
|---|---|---|
| C1 | **核心目标是统一 Guardrails 与 MCP** | 协议 Stage/Bridge 是手段，不是目标；先解决两者重复、互相漏检的问题 |
| C2 | **Harness 先行** | 每一步行为变化之前，harness 已经能钉住当前行为（包括已知缺陷） |
| C3 | **不带录制** | recording 不作为本重构的约束或交付；不移植 `internal/record`，也不为它改接口。录制调用点原样保留、原样搬运 |
| C4 | **尽量少挪文件、少改函数接口** | 在现有包里演进（`internal/toolengine`、`internal/guardrails`、`internal/protocolserver`）；不改包名、不搬目录；接口只做"加字段/加方法"级别的变化，每次改动在 PR 里列出 |
| C5 | **不保留平行路径** | 没有 `--stage` 之类的开关；某条路径迁到新机制后，旧实现在同一个分支里删除，回滚靠 revert 分支 |
| C6 | **小分支堆叠** | 每个分支可独立 review、独立 revert，合入时都是绿的 |

---

## 1. 现状：Guardrails 和 MCP 是同一件事的两份实现

两者都在回答同一个问题：**模型这一轮产生的每个 `tool_use`，该怎么处理？**

| 问题 | Guardrails 的实现 | MCP（toolengine）的实现 |
|---|---|---|
| 流式 tool_use 组装 | `guardrails/adapter/stream.go` `StreamAccumulator`（5 种格式） | `generic_stream_interceptor.go` `accumulateRoundEvent` / `accumulateOpenAIChunk` + 各 adapter `ClassifyEvent` / `ExtractToolFromEvent` |
| 暂扣 / 替换 tool_use 块 | `GuardrailsStreamState.AnthropicToolEvents` + `RewriteAnthropicToolUseEvent` | `suppressedBlockIndices` + `ShouldSuppressEvent` |
| stop_reason 处理 | 改写 `message_delta` 的 `tool_use → end_turn` | 暂扣 `message_delta` / `message_stop` 到循环结束 |
| 从响应取 tool 调用 | `commandFromAnthropicV1Blocks` / `...Beta` / `BuildCommand` | `FormatAdapter.ExtractTools`、`hasOnlyMCPToolUses*` |
| 历史消息提取 | `AdaptMessagesFromAnthropicV1/Beta` | `message_extract.go`、`mcp_hooks.go` `Extract*Messages` |
| tool_result 读写 | `ExtractToolResultText*` / `ReplaceToolResultContent*` | `AppendToolResults` / `BuildToolMessage` |
| 协议适配 | `guardrails/adapter/*`、`guardrails/mutate/*`（V1、Beta 各一份） | `toolengine/*_adapter.go`（V1、Beta、Chat） |

两者的交接只有 `InterceptorConfig.EnableGuardrails` 和 `OnBeforeRound → ReattachGuardrailsHooks`，
所以出现了下面这些**已确认的缺口**（G1 已人工核对代码）：

| # | 缺口 | 位置 |
|---|---|---|
| G1 | **流式 block 在 toolengine 路径上只评估不执行**：hook 记录了 block，但 `RewriteAnthropicToolUseEvent` 只在 `stream/anthropic_passthrough.go` 调用；Anthropic V1 流式**无论是否开 MCP**都走 `StreamAnthropicV1 → GenericStreamInterceptor`，被 block 的 tool_use 照样发给客户端 | `protocol_passthrough.go:107-176`、`generic_stream_interceptor.go:249` |
| G2 | **server 自有工具执行前不过 Guardrails**：流式时 verdict 被记录但 `handlePureVirtual` / `handleMixed` 不看；非流式 `GenericLoopProcessor` 完全不调 Guardrails，只检查最终响应的第一个 tool_use | `generic_stream_interceptor.go:595,628`、`generic_loop_processor.go` |
| G3 | **循环内回灌的 tool_result 不过 Guardrails**：`AppendToolResults` 与 continuation 在请求侧 Guardrails 之后，不做 mask、不做 tool_result 评估；传给 server 工具的参数里可能是 alias token | `toolengine` 循环内 |
| G4 | toolengine 流式路径不做 alias 还原（只有 `RebuildBufferedAnthropicToolUseEvents` 会做） | `mutate/anthropic_stream.go:266` |
| G5 | 后续轮次的 Guardrails 用的是第一轮的历史（`messages` 只捕获一次） | `ReattachGuardrailsHooks` 调用点 |
| G6 | 三套 MCP 循环：toolengine、Chat→Anthropic 的 `ErrMCPStreamContinue` 循环、Beta→Chat 的 hooks；后两者没有 Guardrails | `openai_mcp.go`、`mcp_stream_anthropic_to_openai.go` |
| G7 | 非流式只检查第一个 tool_use | `guardrails/adapter/anthropic_v1.go:59` |

---

## 2. 统一模型：一个 Tool Round，一个决策点

```text
provider stream/response (目标协议)
        │
        ▼
  toolengine 组装本轮 ──► 对每个 tool_use 依次决策：
        │                   1. Gate（Guardrails）   → Block(msg)：替换成文本块，不执行、不外发
        │                   2. Ownership（MCP 注册表）→ Execute：执行，结果先过 Gate 再回灌
        │                   3. 其余                   → Pass：原样交给客户端
        ▼
  本轮结论：NoTools / PureVirtual / PureExternal / Mixed（沿用现有 ResponseDecision）
```

要点：

1. **Guardrails 变成 toolengine 的一个依赖（Gate），不再是挂在流上的 hook。**
   组装只做一次（toolengine 的），Guardrails 只负责"给定一个 tool 调用 / 一段 tool_result，给出 verdict"。
   评估逻辑（`guardrails/evaluate`、`core`、`pipeline` 里的策略求值）不动。
2. **决策顺序固定为 Gate → Ownership。** server 自有工具在执行前被检查（修 G2），
   这和旧设计 `Guardrail(ToolLoop(Provider))` 不同：旧设计里外层 Guardrail 看不到内部工具调用，
   只能另设 `ToolPolicy`，等于两个决策点。v2 只有一个。
3. **toolengine 在目标（provider）协议上运行**，而不是旧设计的"统一转到 Anthropic Beta"。
   理由：当前 transform chain 在 dispatch 之前就已经把请求转成目标协议，MCP 工具注入也发生在
   Base 转换之后——引擎直接吃目标协议请求，不需要新的请求侧 Bridge；跨协议时只需要把引擎的
   **输出**经现有的 stream converter 转回源协议（这些 converter 已在 main 上）。
   代价：每个目标协议需要一个 adapter（V1 / Beta / Chat 已有，Responses 目标后续补）。
4. **没有 MCP 时，引擎退化为"透传 + Gate"。** 注册表为空 → 永远 Pass/Block，这样
   Anthropic 所有路径（包括现在单独的 beta passthrough）可以只走一条代码路径。

最小接口变化（C4）：

```go
// internal/toolengine/format_adapter.go —— 新增，其余不变
type ToolGate interface {
    // 本轮开始；messages 是当前轮真实历史（修 G5）
    BeginRound(ctx context.Context, round int, req any)
    // 对单个 tool 调用给 verdict；args 中的 alias 由 gate 负责还原（修 G4）
    CheckTool(ctx context.Context, tool Tool) GateVerdict
    // 回灌前处理 server 工具结果：mask + tool_result 评估（修 G3）
    CheckToolResult(ctx context.Context, result *ToolExecutionResult)
}

type InterceptorConfig struct {
    MaxRounds     int
    Gate          ToolGate // 替代 EnableGuardrails + OnBeforeRound
    DisableUsage  bool
    ResponseModel string
}

// FormatAdapter 新增 1 个方法：把被 block 的 tool_use 换成文本（流式事件 / 非流式响应各一处）
BlockTool(response any, toolID, message string) (any, error)
```

Guardrails 侧的实现放在现有的 `protocolserver/guardrails_runtime_ai.go`（复用
`BuildGuardrailsBaseInput`、`pipeline`、`mutate` 现有函数），不新建包。

---

## 3. Protocol Stage 在 v2 里的位置

旧设计的 Endpoint / Stage / Bridge 概念保留，但**不新建一套类型**，而是从 toolengine 已有的接口长出来：

| 旧概念 | v2 对应（已存在于 toolengine） |
|---|---|
| `Endpoint` | `Forwarder{ForwardStream, ForwardNonStream}` + pull 式 `StreamHandle` |
| `Stage`（同协议包装） | 一个包装 `Forwarder` 的 `Forwarder`（引擎本身就是第一个） |
| `Bridge`（跨协议） | 请求侧：现有 transform chain 的 Base 转换；响应侧：现有 `stream.New*Converter` / `nonstream.Convert*` |
| HTTP Adapter | 同协议：`FormatAdapter.SendEvent`；跨协议：converter + 现有 writer |

因此"把分发改成注册表驱动的 Stage 链"放到最后阶段，并且是在 Guardrails × MCP 已经统一、
引擎已经能作为 `StreamHandle` 产出事件之后才做，届时改动面很小。

---

## 4. Harness 先行

现状（摘要）：`internal/protocoltest` + `vmodel/benchmark` 能在进程内起真实 HTTP 网关 + 假上游，
覆盖 12 个协议对 × 12 场景 × 流/非流 × 多种 client；但：

- Guardrails 在真实链路上**零覆盖**；MCP 只有 `server/mcp_path_matrix_e2e_test.go`（绕过 HTTP、断言松）；两者组合零覆盖；
- 假上游是无状态的，不能按"请求里有没有 tool_result"切换回复，多轮脚本很别扭；
- 全矩阵没有 `go test` 入口；没有 golden 快照。

hardening-port 分支里可以几乎原样搬的：`protocoltest/guardrails.go`（allow-only runtime）、
`mcp_matrix.go`（echo servertool provider + 多轮 fixture）、`TestEnv` 的 guardrails / servertool
选项、tool_loop / guardrail 组合测试的 helper（去掉 stage 相关断言）、Responses tool-call-only
流的 `assembleFromEvents` 修复。`bridge_matrix.go` 依赖 stage 包，不搬。

**已知缺口登记表**：对 G1–G7，harness 里写的是"期望的正确行为"。当前 main 上失败的用例
登记在一张表里（`knownGaps`，带缺口编号），运行时报告为 known-gap 而不是失败；修复分支
**必须**同时把对应条目从表里删掉。这样 harness 分支是绿的，修复分支的效果也是可见的。

---

## 5. 堆叠分支

| # | 分支 | 内容 | 生产行为 |
|---|---|---|---|
| 0 | `claude/lucid-heisenberg-ppa3kj`（本分支） | 本文档 | 无 |
| H1 | `harness/1-infra` | `TestEnv` 加 guardrails / servertool 选项（server 侧新增 `WithServertoolProviders`，仅测试使用）；假上游支持按请求内容切换回复（`NonStreamFor(req)` / 序列 helper）；全矩阵 `go test` 入口；client 可见输出 + 上游请求的 golden 快照（`-update`，id/时间戳归一化） | 无 |
| H2 | `harness/2-guardrails-mcp` | 真实 HTTP 链路上的用例：Guardrails（block 文本 / block tool_use 且不泄漏 / 请求侧 tool_result block / credential mask 往返）× 协议对 × 流/非流；MCP（owned 循环、mixed continuation、max rounds、工具报错）；二者组合（owned 执行后外部工具被 block；owned 工具本身被 block）。G1–G7 登记为 known gap。现有协议对的 golden 快照首次生成 | 无 |
| U1 | `unify/1-gate` | `ToolGate` + `InterceptorConfig.Gate` + `FormatAdapter.BlockTool`；Guardrails 实现 Gate；流式引擎与非流式 `GenericLoopProcessor` 在分类前调用 Gate。删除 `EnableGuardrails`、`OnBeforeRound`、`ReattachGuardrailsHooks`。修 G1 G2 G4 G5 G7 | Anthropic V1 / Beta 的 toolengine 路径：Guardrails 真正生效 |
| U2 | `unify/2-tool-results` | 引擎回灌 tool_result 与应用 continuation 时调用 `Gate.CheckToolResult`。修 G3 | server 工具结果受 mask / 评估 |
| U3 | `unify/3-one-anthropic-path` | beta passthrough 改走引擎（空注册表 = 透传 + Gate）；删除 `stream.HandleAnthropic/HandleAnthropicBeta` 里的 Guardrails 改写分支与 `AttachGuardrailsHooks`、`HandleContext.Guardrails.Stream`。Anthropic 目标只剩一条路径 | 行为不变（golden 保证） |
| U4 | `unify/4-engine-stream` | 引擎输出从直接 `adapter.SendEvent(c, …)` 改为产出事件（引擎实现 `StreamHandle`，同协议时由一个薄 writer 写出）。同协议 golden 必须逐字节不变 | 行为不变 |
| U5 | `unify/5-cross-protocol-mcp` | Chat→Anthropic、Beta→Chat 的 MCP 改为"目标协议引擎 + 现有 converter 转回源协议"；删除 `openai_mcp.go` 的 `ErrMCPStreamContinue` 循环、`mcp_stream_anthropic_to_openai.go`、`mcp_hooks.go` 中对应 hooks。修 G6 | 跨协议 MCP 统一；Guardrails 覆盖随之到达（仍受现有 scenario gate 约束） |
| S1 | `stage/1-chain`（后续单独细化） | 把 `protocol_dispatch.go` 的两层 switch 逐个协议对替换为 Forwarder 链（Provider → 引擎 → converter → writer），每个 pair 一个小分支，删除对应 leaf；补 Responses 目标 adapter | 分发收敛 |

独立小修（不依赖上面任何分支，可随时合）：Chat → Google 目标可选中但 `dispatchGoogle` 无 Chat 分支
（静默无响应）；Responses 入口从不设置 `reqCtx.ResponseModel`。

依赖：H1 → H2 → U1 → U2 → U3 → U4 → U5 → S1…，严格线性。

---

## 6. 每个分支的验收

- `go build ./... && go vet ./...`，改动包单测，`go test ./internal/protocoltest/...` 全绿。
- `U*` 分支：PR 描述列出 (a) 删除的 known-gap 条目，(b) golden 快照的变化及原因——
  除 known-gap 对应的行为外，**快照不允许变化**。
- 接口变化清单：列出每个被改签名的函数 / 接口方法；超过预期（§2 所列）需要在 PR 中说明。
- 文件：不移动、不重命名现有文件；新增文件仅限测试与 `ToolGate` 实现所需。

---

## 7. 待讨论

1. OpenAI 入口（Chat / Responses 源）是否启用 Guardrails：U5 之后技术上"免费"获得，但是否打开
   是产品决策；在此之前保持 `GuardrailsSupportedScenarios` 不变。
2. Gate 对非 tool 的**文本**是否也做流式评估（今天只有非流式评估文本）。v2 暂不扩展。
3. Chat → Google：补齐还是显式不支持。
