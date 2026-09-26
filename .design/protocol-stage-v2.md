# Protocol Stage v2 — Guardrails × MCP 统一与重新落地

> Status: **进行中**（P1–P4 已推送，未接入请求路径；§4 已采纳）
> 前身：`origin/expr/protocol_stage`、`origin/feat/protocol-stage-hardening-port`
> （原设计文档 `protocol-stage-chain.md` / `protocol-stage-tool-loop.md` 保留在那两个分支上）。

---

## 0. 约束（本轮确定）

| # | 约束 | 含义 |
|---|---|---|
| C1 | `claude/lucid-heisenberg-ppa3kj-c1`（**已推送**，叠在 h3 上） | 第一次切流：Anthropic Beta 客户端 → Anthropic provider 全部请求走 Stage 管线（Stage 仅在 MCP / Guardrails 生效时插入）；删除 `passthroughAnthropicBeta`、Beta 的 generic MCP dispatch、`StreamAnthropicBeta`；保留工具执行期间的 `: keep-alive`（`stage.Heartbeat`）；G2 Beta→Beta 移出 known-gap；golden 不变，harness CLI 1132 例 0 失败 | Beta→Beta |
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
| G8 | 跨协议路径（Anthropic 客户端 → OpenAI provider）没有任何响应侧 Guardrails | `protocol_dispatch.go` / `protocol_cross.go` 各 leaf |
| M1 | Responses 源：server 工具被注入上游但调用不被拦截，直接泄漏给客户端 | Responses 入口 |
| ~~M2~~ | ~~Chat→Chat 流式工具循环：最终答案以空 `data:` 帧到达客户端~~（已由热修复 `-chat-mcp-stream` 修复） | toolengine |
| M3 | OpenAI Responses 目标：根本不向模型提供 server 工具 | transform chain |
| M4 | Anthropic 客户端 → Chat provider 流式循环：到达轮数上限时整个请求 500，其他路径均正常结束 | `mcp_stream_anthropic_to_openai.go` |
| M5 | Chat 客户端 → Anthropic provider 流式：mixed 轮次的 server 工具结果不会拼回后续请求 | `openai_mcp.go` |

G1（含非流式 block / alias 还原被 `WriteAnthropicMessage` 的 RawJSON 丢弃）已在
`claude/lucid-heisenberg-ppa3kj-g1` 修复；其余缺口在 harness 中以 `knownGaps` 登记（见 §5）。
另修复（独立分支 `claude/lucid-heisenberg-ppa3kj-mcp-init`）：全新配置首次启动时 MCP runtime 为 nil（`NewRuntime` 早于 `RegisterBuiltinTools`）。

独立热修复原则：不依赖重构的 bug 修复各自基于 main 开分支，可单独合入；harness / 重构分支通过 merge 引入它们，不重复提交。

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

### 2.3 Tool Round Stage 的行为决定（P4 落地）

以"切流时客户端可见形态不变，只修 known-gap"为原则：

| 场景 | 行为 | 与旧路径的差异 |
|---|---|---|
| 某轮有 tool_use 被 Gate 拦截 | 该轮终止：被拦截的块替换为 block 文本，其余 tool_use（owned / client）都不执行、不外发，`stop_reason=end_turn` | 旧流式只替换被拦截块、保留其他 client tool；旧非流式整段替换且只看第一个 tool_use（G7） |
| 流式 | text / thinking 实时外发；只有 tool_use 块暂扣到本轮决策完成（首字延迟不变） | 旧路径开 Guardrails 时暂扣到单个块结束；差别只在同一轮多个工具时 |
| 多轮流式 | 合成一条客户端消息：message_start 一次，块 index 连续，最后一轮的 message_delta / stop | 旧路径不重排 index |
| 多轮非流式 | 返回最后一轮（同旧路径） | 无 |
| 轮数上限 | 结束本轮：去掉 owned 调用，`end_turn`（有 client 调用则保留并 `tool_use`） | 旧非流式返回空消息；旧流式保留 `tool_use` stop_reason 但无工具块 |
| usage | Response / StreamResult 汇总所有轮次；客户端可见的 usage 字段保持每轮原值 | 旧路径只统计最后一轮 |
| 执行过 server 工具后出错 | `stage.CommittedError`，failover 不得重试 | 新增 |
| mixed continuation | 无会话不存；只有携带对应 tool_result 的 follow-up 才会取用 | 旧路径无会话时共用一个键，任何同会话请求都会取走 |
| 工具注入 | 仍由 transform 链的 `MCPToolInjectionTransform` 负责；M3（Responses 目标不注入）在切流时随 transform 修 | 无 |
| Guardrails 失败 | fail-open，同旧路径 | 无 |

给 P5 的备注：Chat / Responses → Beta bridge 产出的消息没有 RawJSON，直接 `json.Marshal` 会带出 SDK 所有零值字段；HTTP Adapter 需要按 wire 形态序列化。

### 2.4 何时进入 IR（Beta）

Beta 是 Tool Round Stage 的 IR。Stage 按请求插入：本请求有 server 工具注入（含 mixed 续接）或 Guardrails 对该场景生效时
插入，否则拓扑里没有它；这仍是同一条管线由 `BuildTopology` 决定形态，不保留旧 leaf，不违反 C5。

| 源 → 目标 | 无 MCP / Guardrails | 开 MCP（或 Guardrails） | 多出往返 |
|---|---|---|---|
| Beta / V1 → Anthropic | 直连（V1 边缘无损升降级） | Stage 在 Beta 上 | 否 |
| Beta / V1 → Chat / Responses | Beta→目标 1 次 | 同左 | 否 |
| Chat / Responses → Anthropic | 源→Beta 1 次 | 同左 | 否 |
| **Chat → Chat、Responses → Responses** | 直连 0 次 | 源→Beta→Stage→Beta→目标 2 次 | **是** |
| **Chat → Responses、Responses → Chat** | 直连 Bridge 1 次 | 经 Beta 2 次 | **多 1 次** |

往返是阶段性妥协：Stage 只实现在 IR 上。IR 覆盖不到的事实经 `stage.ProtocolState` 这类有类型的通道携带、由对端 Bridge 还原；
H3 对加粗的四个协议对（开 MCP 时）逐一比对请求 / 非流式响应 / 流式响应在往返前后与直连的差异，损失登记为 known-gap，
OpenAI 源的切流在其清零或被明确接受后进行。Guardrails 目前只对 Anthropic 场景启用（§8.2），所以现阶段触发往返的只有 MCP。

#### 2.4.0 支持策略（已决定）

Guardrails 与 MCP 只在"经 IR 往返能稳定作用"的协议对上支持。不支持的协议对即使开启了 Guardrails / MCP，也按原协议处理：
不注入 server 工具、不插 Stage、不做 Guardrails，原协议直通（不注入是为了避免重演 M1：工具发给了模型、调用泄漏给客户端）。
不做"部分支持"或原协议放行层。跳过时给出可见信号（debug routing 头与日志），让用户知道本请求未启用的原因。

| 协议对 | Guardrails / MCP | 依据 |
|---|---|---|
| Beta / V1 → 任意目标 | 支持 | 源协议即 IR（V1 边缘升降级无损，已验证） |
| Chat / Responses → Anthropic | 支持 | 转换本来就要做，Stage 不增加往返 |
| Chat→Chat、Responses→Responses、Chat→Responses、Responses→Chat | **不支持**，直到该协议对的 H3 known-gap 清零 | 往返有损（§2.4.1） |

注意：旧路径今天 Chat→Chat 的 MCP 是可用的（toolengine OpenAI Chat 循环）。该协议对切流时若 H3 仍未清零，MCP 会随之关闭——这是按本策略接受的变化，届时在切流 PR 中列明。

录制不在本重构范围内（C3）：新路径不做逐轮录制，旧 MCP 循环的逐轮录制随切流消失，另行解决。

#### 2.4.1 H3 当前登记的 IR 损失（开 MCP 时，OpenAI 源切流前需清零或明确接受）

| ID | 损失 | 影响的协议对 |
|---|---|---|
| I1 | IR 注入客户端未要求的输出上限（`max_tokens` / `max_output_tokens` 4096），并覆盖客户端的 `max_completion_tokens` | Chat→Chat、Responses→Responses |
| I2 | 采样与请求元数据丢失：`temperature`、`top_p`、`stop`、`seed`、presence/frequency penalty、`user` | 四对均有 |
| I3 | 结构化输出（`response_format` / `text.format` json_schema）丢失 | Chat→Chat、Responses→Responses |
| I4 | 推理强度（`reasoning_effort` / `reasoning.effort`）丢失 | Chat→Chat、Chat→Responses、Responses→Responses |
| I5 | `developer` 消息丢失 | Chat→Chat |
| I6 | 输入图片的 `detail` 丢失 | Responses→Responses |
| R1 | 响应、输出 item、function call 的 ID 被重新生成，而非沿用 provider 的 | Chat→Chat、Responses→Responses |
| R2 | Responses 的 `object` 被规范化为 `"response"`，未沿用 provider 的值 | Responses→Responses |
| R3 | 流式的长度截断（`finish_reason: length`）变成 `stop` | Chat→Chat、Chat→Responses |
| R4 | provider 流式未报 usage 时，IR 的估算与直连不同 | Responses→Chat |

另：IR 降到 Chat 时会给上游 assistant 消息加非标准字段 `x_thinking`（空值时被比较忽略），切流前需确认对各 Chat provider 无害。

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

### 4.4 决定（已采纳）

- 采纳；Provider 边缘**做** downgrade。
- **不单独做清理步骤**：新协议层从一开始就没有 V1 这个协议——客户端边缘 upgrade/downgrade 在
  HTTP Adapter（P5），provider 边缘 downgrade 在 Provider Endpoint（P3）；旧 V1 代码随 V1 源
  协议对切流一并删除，不预先重构即将被替换的代码。
- "Beta 是超集"的前提在 harness 中提前验证：V1 请求经 upgrade → Beta → downgrade 与现行 V1 路径
  逐字节比对（上游请求 + 客户端响应），在搭协议层之前暴露问题，不动生产代码。

---

## 5. Harness 先行

现状：`internal/protocoltest` + `vmodel/benchmark` 能在进程内起真实 HTTP 网关 + 假上游，
覆盖 12 个协议对 × 12 场景 × 流/非流 × 多种 client；但：

- Guardrails 在真实链路上**零覆盖**；MCP 只有 `server/mcp_path_matrix_e2e_test.go`（绕过 HTTP、断言松）；两者组合零覆盖；
- 假上游是无状态的，不能按"请求里有没有 tool_result"切换回复；
- 全矩阵没有 `go test` 入口；没有 golden 快照；V1 → Anthropic V1 目标没有独立 pair。

hardening-port 分支可几乎原样搬的：`protocoltest/guardrails.go`、`mcp_matrix.go`、`TestEnv` 的
guardrails / servertool 选项、tool loop / guardrail 组合测试 helper（去掉 stage 断言）、Responses
tool-call-only 流的 `assembleFromEvents` 修复。`bridge_matrix.go` 不搬（以 V1 为中心、自带 fixture）；P2 改为复用 vmodel 场景 fixture 与断言的紧凑内存矩阵。

**已知缺口登记表**：用例写"期望的正确行为"；当前失败的登记在 `knownGaps`（带 G 编号），报告为
known-gap 而非失败。修复分支必须同时删除对应条目。

---

## 6. 堆叠分支（严格线性）

| # | 分支 | 内容 | 生产行为 |
|---|---|---|---|
| 0 | `claude/lucid-heisenberg-ppa3kj` | 本文档 | 无 |
| F | `claude/lucid-heisenberg-ppa3kj-g1`（**已合入 #1843**） | G1 热修复：toolengine 流式路径执行 block 改写；非流式 block / alias 还原写回 RawJSON | 修安全缺口 |
| F2 | `claude/lucid-heisenberg-ppa3kj-mcp-init`（**已合入 #1844**，基于 main） | 独立热修复：全新配置首次启动时 MCP runtime 为 nil；附回归测试 | 修首启 MCP |
| H1 | `claude/lucid-heisenberg-ppa3kj-h1`（**已推送**，热修复合入后 rebase 到 main；只含 harness 提交） | 假上游按请求内容回复；`WithServertoolProviders` + echo 工具；`knownGaps` 登记；MCP 12 对 × 流/非流；Guardrails（Anthropic 源 × 3 目标）及与 MCP 的组合。登记 G2 G8 M1 M2 M3 | 无 |
| F3 | `claude/lucid-heisenberg-ppa3kj-model-leak`（**已合入 #1845**，基于 main） | 独立热修复：客户端响应报告上游 model id（Anthropic 非流式、拦截器流式、Responses 入口从不设 `ResponseModel`） | 修信息泄漏 |
| F4 | `claude/lucid-heisenberg-ppa3kj-chat-mcp-stream`（**已合入 #1846**，基于 main） | 独立热修复：M2 | 修 Chat MCP 流式 |
| F5 | `claude/lucid-heisenberg-ppa3kj-chat-google`（**已合入 #1847**，基于 main） | 独立热修复：Chat → Google 目标返回空 200；接上现有 converter，未处理的源显式报错 | 修 Chat→Google |
| H2 | 并入 `-h1`（**已推送**） | 每个协议对 × 12 场景 × 流/非流的 golden wire 快照（上游请求 + 客户端响应，`-update`）；全矩阵与 idempotent 的 `go test` 入口；V1 ⇄ Beta 请求 wire 逐字节等价（§4 前提成立）；MCP 工具报错 / 轮数上限 / mixed 续接。登记 M4 M5，删除 M2 | 无 |
| P1 | `claude/lucid-heisenberg-ppa3kj-p1`（**已推送**，叠在 h1 上） | `internal/protocol/stage` 契约 + 单测：Endpoint / Stage / Compose / Bridge / 精确配对 Registry / BuildTopology；链内拒绝 `anthropic_v1`（只在边缘处理）；去掉隐式 identity 回退与反射 nil 检查 | 无 |
| P2 | `claude/lucid-heisenberg-ppa3kj-p2`（**已推送**，叠在 p1 上） | 6 个 Beta-only Bridge（Beta⇄Chat、Beta⇄Responses、Chat⇄Responses），包装现有 converter，不含 V1 / identity bridge；`TestBridgeMatrix`：每个 bridge × 成功场景 × 流/非流，在内存中复用 HTTP 矩阵的场景 fixture 与断言（错误场景属 HTTP 状态映射，归 P5） | 无 |
| P3 | `claude/lucid-heisenberg-ppa3kj-p3`（**已推送**，叠在 p2 上） | `stage/upstream` 终端 endpoint：Anthropic（Beta）/ Chat / Responses，只包装 `forwarding.Forward*`，不做请求准备；`AnthropicWireV1` 在 provider 边缘 downgrade 请求、upgrade 响应与流事件；`request.ConvertAnthropicBetaToV1Request` 拒绝（而非丢弃）V1 无法表达的内容。测试：每个 endpoint × 流/非流发往 provider 的 method / path / query / beta 头 / body 与旧路径直接调用 forwarding 逐一相同（V1 覆盖 upgrade → 链 → downgrade 全程）；两种 Anthropic wire 结果一致；6 个 bridge 叠在真实 endpoint 上跑通 | 无 |
| P4a | `claude/lucid-heisenberg-ppa3kj-p4`（**已推送**，叠在 p3 上） | `stage/toolround`：Tool Round Stage（Beta），Gate 接口 + MCP Ownership；`toolengine.AnthropicBetaOwner` 复用 registry / ToolExecutor / continuation store（键格式不变）；`stage.CommittedError`；continuation 限定会话并与 follow-up 关联。内存测试：真实 MCP runtime + servertool pipeline + H1 fixture，经 P2 bridge 覆盖 Beta / Chat / Responses 目标（含 M4、M5） | 无 |
| P4b | `claude/lucid-heisenberg-ppa3kj-p4b`（**已推送**，叠在 p4 上） | `guardrailspipeline.ToolRoundGate`：复用现有 Guardrails 管线实现 Gate（每请求一份 mask 状态，fail-open）。内存测试覆盖 G2 G3 G4 G5 G7 G8 | 无 |
| F6 | `claude/lucid-heisenberg-ppa3kj-continuation-scope`（**已推送**，基于 main） | 独立热修复：continuation 无会话时共用一个键（跨会话串入）；同会话任意请求都会取走存储轮次。改为无会话不存、只有携带对应 tool_result 的 follow-up 才取用（V1 / Beta / Chat） | 修跨会话泄漏 |
| P5 | `claude/lucid-heisenberg-ppa3kj-p5`（**已推送**，叠在 p4b 上） | Anthropic 客户端（V1 / Beta）的 HTTP Adapter：`sdkstream` 把 EventStream 呈现为 SDK stream，直接复用现有写出器（SSE、首块提交、model 改写、错误事件、usage）；V1 在边缘 downgrade（同 wire 字节，V1 无法表达的内容显式报错）；执行过 server 工具后出错时提交 failover gate，不重试。验收：对照 golden，Beta→Beta 24 例逐字节一致（上游请求 + 客户端响应）；V1 24 例除预期统一外一致 | 无 |
| H3 | `claude/lucid-heisenberg-ppa3kj-h3`（**已推送**，叠在 p5 上） | IR 往返保真度 harness（§2.4 加粗四对，开 MCP 时）：Chat↔Beta↔Chat、Responses↔Beta↔Responses、Chat→Beta→Responses、Responses→Beta→Chat 的请求 / 非流式响应 / 流式响应，与直连对比，损失登记为 known-gap | 无 |
| P5b | 待定 | OpenAI 客户端（Chat / Responses）的 HTTP Adapter；前提是 H3（§8.5） | 无 |
| C1 | `claude/lucid-heisenberg-ppa3kj-c1`（**已推送**，叠在 h3 上） | 第一次切流：Anthropic Beta 客户端 → Anthropic provider 全部请求走 Stage 管线（Stage 仅在 MCP / Guardrails 生效时插入）；删除 `passthroughAnthropicBeta`、Beta 的 generic MCP dispatch、`StreamAnthropicBeta`；保留工具执行期间的 `: keep-alive`（`stage.Heartbeat`）；G2 Beta→Beta 移出 known-gap；golden 不变，harness CLI 1132 例 0 失败 | Beta→Beta |
| C2… | `stage/7-cut-*` | 逐个协议对切流（Beta→Chat/Responses，Chat→*，Responses→*），每对一个分支，删对应 leaf 与跨协议 MCP 循环 | 逐对 |
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

1. ~~§4：是否采纳"Anthropic 内部只保留 Beta"~~——已决定，见 §4.4。
2. OpenAI 入口（Chat / Responses 源）是否启用 Guardrails：切流后技术上直接可得，是产品决策；此前保持 `GuardrailsSupportedScenarios` 不变。
3. Gate 是否对非 tool 的**文本**做流式评估（今天只有非流式评估文本）。
4. Chat → Google：补齐还是显式不支持。
5. ~~**OpenAI 源经 Beta 中转**~~——已决定（见 §2.4）：Beta 是 Stage 的 IR，但只有 Stage 有事可做的请求才进入 IR；
   其余请求按协议对直连，同协议不插 Bridge。往返只出现在 OpenAI 源 × OpenAI 目标且本轮启用 MCP（或将来对 OpenAI 源启用
   Guardrails）时，由 H3 钉住保真度。
6. **V1 客户端的预期差异**（V1 切流时 golden 会变）：SSE 帧统一为 Beta 写出器格式（`event:X` / `data:...`，旧 V1 走拦截器为
   `event: X` 带空格）；非流式转发失败的错误文案由 "Failed to create streaming request" 改为 "Failed to forward request"（状态码不变）；
   截断流的错误事件由 `upstream_truncated` 统一为 `incomplete_stream`。
