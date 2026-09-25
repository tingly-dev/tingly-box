# Protocol Stage v2 — Guardrails × MCP 统一与重新落地

> Status: **Plan**（尚未开始编码；§4 为讨论中的提案）
> 前身：`origin/expr/protocol_stage`、`origin/feat/protocol-stage-hardening-port`
> （原设计文档 `protocol-stage-chain.md` / `protocol-stage-tool-loop.md` 保留在那两个分支上）。

---

## 0. 约束（本轮确定）

| # | 约束 | 含义 |
|---|---|---|
| C1 | **核心目标是统一 Guardrails 与 MCP** | 两者合成**一个**特性 Stage，只在**一个**工作协议上实现一次 |
| C1' | **先协议，后应用** | 先立协议边界（Endpoint / Bridge），再把 Guardrails × MCP 落在其上；否则应用层会被迫理解 N 个协议，协议层改造时又要返工 |
| C2 | **Harness 先行** | 每一步行为变化之前，harness 已经能钉住当前行为（包括已知缺陷） |
| C3 | **不带录制** | recording 不作为本重构的约束或交付；不移植 `internal/record`，也不为它改接口。录制调用点原样保留、原样搬运 |
| C4 | **尽量少挪文件、少改函数接口** | 协议层是**新增**文件（`internal/protocol/stage`）；特性 Stage 复用 `toolengine` / `guardrails` 现有实现，不搬目录、不改包名；旧代码在切流时整段删除，而不是搬移或改签名 |
| C5 | **不保留平行路径（方案 A）** | 没有 `--stage` 开关，也不按"是否开 MCP / Guardrails"分流：一个协议对只有在完整链路（协议层 + 统一特性 Stage）就绪后才切，所有请求一起切，对应旧 leaf 在同一分支删除；回滚靠 revert |
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

## 2. 目标形态

```text
client (V1 / Beta / Chat / Responses)
  │
HTTP Adapter（唯一解析 HTTP、写 JSON/SSE、提交首块的地方）
  │
Ingress Bridge            client 协议 ⇄ anthropic_beta
  │
Tool Round Stage [anthropic_beta]   ← Guardrails（Gate）+ MCP（Ownership）统一在这里
  │
Provider Bridge           anthropic_beta ⇄ provider 协议
  │
Provider Endpoint         包装 forwarding.Forward*
```

请求向内；complete 响应、stream 事件、错误、usage、副作用标记向外。相邻层协议相同则不插 Bridge。

### 2.1 协议层契约（沿用旧设计，按 main 现状精简）

`Endpoint{Protocol; Complete; Stream}`、pull 式 `EventStream{Next(ctx); Close; Result}`、
`Stage{Name; Protocol; Wrap(next)}`、`Bridge{Source; Target; Open(call, op) → BridgeSession}`、
`BuildTopology`（缺能力即构建失败）。Bridge 只包装 main 上已有的 `request.*` /
`nonstream.Convert*` / `stream.New*Converter`，不写第二份转换逻辑。

### 2.2 Tool Round Stage：一个决策点

对模型每一轮产生的每个 tool 调用，按固定顺序决策：

1. **Gate（Guardrails）** → Block：替换为文本块，不执行、不外发（修 G1 G2 G7）
2. **Ownership（MCP 注册表）** → Execute：执行；结果**先过 Gate**（mask + tool_result 评估）再回灌（修 G3）
3. 其余 → Pass：交给客户端

每轮用当轮真实历史调用 Gate（修 G5），Gate 负责 alias 还原（修 G4）。没有 MCP 时注册表为空，
Stage 退化为"透传 + Gate"。跨协议 MCP 不再需要单独的循环（修 G6）——所有源 / 目标都经 Bridge 到 Beta。

与旧设计的区别：旧设计是 `Guardrail(ToolLoop(Provider))` 两个 Stage，外层 Guardrail 看不到被
内部消化的 tool 调用，只能另设 `ToolPolicy`；v2 合成一个 Stage、一个决策点。

复用而非重写：toolengine 的 Beta adapter、continuation store、`ServerToolExecutor`；Guardrails 的
`evaluate` / `core` / `pipeline` 求值与 mask 逻辑。流式按旧设计缓冲一轮（可见前缀无法证明本轮没有内部 tool_use）。

---

## 3. 为什么先协议后应用

上一版计划曾让 toolengine 在目标协议上运行、先统一 Guardrails × MCP 再改协议层，被否决：

- 那样 Gate 要在 V1 / Beta / Chat / Responses 每个目标协议上各实现一次 block 改写与组装，正是 Stage 要消除的"特性理解 N 个协议"；
- "引擎产出事件""跨协议输出经 converter 转回"本质是 Endpoint / Bridge 的工作，塞进 toolengine 后，协议层改造时还要返工，违背 C4。

唯一例外是 G1：它是正在生效的安全缺口，先用一个很小的热修复堵住，不等重构。

---

## 4. 提案（讨论中）：Anthropic 内部只保留 Beta

### 4.1 事实

- **wire 层 Beta 是 V1 的严格超集**：同一个 `/v1/messages`，Beta 只多可选字段 / 块和 `anthropic-beta` 头。
  `request/anthropic_v1_to_beta.go` 已依赖这一点，用 JSON 往返做无损 V1→Beta。
- SDK 的 V1 / Beta 区分是 Go 类型层面的；Beta 调用走 `v1/messages?beta=true`（`libs/anthropic-sdk-go/betamessage.go:72`）。
- **目前只有"V1 源 → Anthropic provider"使用 V1 目标**；Beta、Chat、Responses 源打到 Anthropic 风格 provider 时
  已经全部走 Beta（`anthropic_message.go:380`、`openai_chat.go:222`）。第三方 Anthropic 兼容 provider 早已在大量接收 Beta 调用。
- V1 专有代码面：`TypeAnthropicV1` 58 处引用；guardrails adapter/mutate、toolengine adapter、各 converter 都有 V1 / Beta 两份。

### 4.2 提案

内部协议集合从 `{v1, beta, chat, responses, google}` 缩为 `{anthropic(beta), chat, responses, google}`。
V1 不再是链路中的协议，只在两个边缘处理：

| 边缘 | 方向 | 做法 |
|---|---|---|
| 客户端边缘（V1 客户端） | upgrade 请求 | 现有 JSON 往返，无损 |
| | downgrade 响应 / 事件 | Beta → V1 JSON；Beta 专有块 / 字段按**显式清单**处理（丢弃或报错），不静默 |
| Provider 边缘 | downgrade 请求（可选） | 请求中没有 Beta 专有内容时用 V1 路径（无 `?beta=true`、无 beta 头）发出；有则走 Beta。保证原来走 V1 的 provider 看到的 wire 请求不变 |

收益：Bridge 数量、特性 Stage 的适配面、Guardrails / toolengine 的 V1 副本全部减半；V1 客户端自动获得统一特性链路。

### 4.3 风险与验证（harness 负责）

1. **上游请求字节变化**：V1 类型 → Beta 类型序列化可能在零值 / omitempty 上不同。用 harness 的上游请求 golden 快照逐字节比对 V1→V1 路径。
2. **V1 客户端看到 Beta 专有块**：只可能来自我们自己注入的内容（如 MCP 工具，已被 Stage 隐藏）；downgrade 清单 + harness 断言兜底。
3. **只接过 V1 调用的 provider**：由 Provider 边缘 downgrade 保证 wire 不变，而不是赌它们接受 `?beta=true`。
4. count_tokens 同样存在 V1 / Beta 两份，一并处理。

### 4.4 若采纳，对计划的影响

在 P1 之前插入一步 "V1 → 边缘"：V1 源请求在入口 upgrade，走现有 Beta 路径（目标 `TypeAnthropicBeta`），
响应在出口 downgrade；随后删除 V1 专有 leaf 与 adapter。之后 P1 只需实现 Beta / Chat / Responses 之间的 Bridge。

---

## 5. Harness 先行

现状：`internal/protocoltest` + `vmodel/benchmark` 能在进程内起真实 HTTP 网关 + 假上游，
覆盖 12 个协议对 × 12 场景 × 流/非流 × 多种 client；但：

- Guardrails 在真实链路上**零覆盖**；MCP 只有 `server/mcp_path_matrix_e2e_test.go`（绕过 HTTP、断言松）；两者组合零覆盖；
- 假上游是无状态的，不能按"请求里有没有 tool_result"切换回复；
- 全矩阵没有 `go test` 入口；没有 golden 快照；V1 → Anthropic V1 目标没有独立 pair。

hardening-port 分支可几乎原样搬的：`protocoltest/guardrails.go`、`mcp_matrix.go`、`TestEnv` 的
guardrails / servertool 选项、tool loop / guardrail 组合测试 helper（去掉 stage 断言）、Responses
tool-call-only 流的 `assembleFromEvents` 修复。`bridge_matrix.go` 留到 P1 与 stage 包一起搬。

**已知缺口登记表**：用例写"期望的正确行为"；当前失败的登记在 `knownGaps`（带 G 编号），报告为
known-gap 而非失败。修复分支必须同时删除对应条目。

---

## 6. 堆叠分支（严格线性）

| # | 分支 | 内容 | 生产行为 |
|---|---|---|---|
| 0 | `claude/lucid-heisenberg-ppa3kj` | 本文档 | 无 |
| F | `fix/g1-stream-block` | G1 热修复：toolengine 流式路径执行 Guardrails 的 block 改写（复用 `RewriteAnthropicToolUseEvent`） | 修安全缺口 |
| H1 | `harness/1-infra` | `TestEnv` 加 guardrails / servertool 选项；假上游按请求内容切换回复；全矩阵 `go test` 入口；client 输出 + 上游请求 golden 快照（`-update`，id / 时间戳归一化）；补 V1→V1 pair | 无 |
| H2 | `harness/2-guardrails-mcp` | 真实 HTTP 用例：Guardrails（block 文本 / block tool_use 不泄漏 / 请求侧 tool_result block / mask 往返）、MCP（owned 循环 / mixed continuation / max rounds / 工具报错）、二者组合；× 协议对 × 流/非流。G2–G7 登记 known gap；生成首批 golden | 无 |
| V | `stage/0-v1-edge`（取决于 §4） | V1 upgrade / downgrade 边缘，V1 源改走 Beta；删除 V1 专有 leaf / adapter | V1 源路径变化（golden 逐字节验证上游请求） |
| P1 | `stage/1-contracts` | `internal/protocol/stage` 契约 + identity + 单测 | 无 |
| P2 | `stage/2-bridges` | Bridge（包装现有 converter）+ Provider Endpoint + in-memory bridge 矩阵（搬 `bridge_matrix.go`） | 无 |
| P3 | `stage/3-tool-round` | Tool Round Stage（Gate + Ownership，Beta），复用 toolengine / guardrails；用 H2 的 fixture 在内存中验证 | 无 |
| P4 | `stage/4-http-adapter` | 各客户端协议的 HTTP Adapter（JSON / SSE / 首块提交 / model 改写 / usage / affinity） | 无 |
| C1 | `stage/5-cut-beta` | 第一次切流：Beta→Beta 全部请求（含 MCP / Guardrails）；删除对应 leaf、`AttachGuardrailsHooks`、passthrough 改写分支 | Beta→Beta |
| C2… | `stage/6-cut-*` | 逐个协议对切流（Beta→Chat/Responses，Chat→*，Responses→*），每对一个分支，删对应 leaf 与跨协议 MCP 循环 | 逐对 |
| Z | `stage/9-cleanup` | 删除 `HandleContext` stream hooks、`ErrMCPStreamContinue`、toolengine `FormatAdapter.SendEvent` 等遗留；Google 目标去留 | 收尾 |

独立小修（随时可合）：Chat → Google 目标可选中但无处理分支（静默无响应）；Responses 入口从不设置 `reqCtx.ResponseModel`。

---

## 7. 每个分支的验收

- `go build ./... && go vet ./...`，改动包单测，`go test ./internal/protocoltest/...` 全绿。
- 切流分支：PR 列出迁移的协议对、删除的旧函数、删除的 known-gap 条目、golden 快照变化及原因——
  除 known-gap 对应行为外，快照不允许变化。
- 接口：列出每个被改签名的既有函数 / 接口方法；新增文件限于 stage 包、测试、Stage 实现。

---

## 8. 待讨论

1. §4：是否采纳"Anthropic 内部只保留 Beta"，以及 Provider 边缘 downgrade 是否需要（还是直接统一发 Beta）。
2. OpenAI 入口（Chat / Responses 源）是否启用 Guardrails：切流后技术上直接可得，是产品决策；此前保持 `GuardrailsSupportedScenarios` 不变。
3. Gate 是否对非 tool 的**文本**做流式评估（今天只有非流式评估文本）。
4. Chat → Google：补齐还是显式不支持。
