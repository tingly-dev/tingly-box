# Protocol Stage

`internal/protocol/stage` 把"一次模型调用"抽象为可组合的层：provider 调用、同协议内的横切逻辑、协议转换，各自是独立、可测试的一层。
目标是让 Guardrails、server tool（MCP）循环这类横切能力**只在一种协议上实现一次**，而不是在每个"客户端协议 × provider 协议"的组合里各写一遍。

本文说明这一层的术语、契约与取舍。代码入口：

| 包 | 内容 |
|---|---|
| `internal/protocol/stage` | 契约：`Endpoint`、`EventStream`、`Stage`、`Compose`、`Bridge`、`Adapt` |
| `internal/protocol/stage/anthropicbridge` | Anthropic Beta → OpenAI Chat / Responses 的 Bridge |
| `internal/protocol/stage/upstream` | 终端 Endpoint：Anthropic（Beta 或 V1 wire）、OpenAI Chat、OpenAI Responses |
| `internal/protocol/stage/toolround` | Tool Round Stage：Guardrails 与 MCP 对工具调用的统一决策 |

---

## 术语

| 术语 | 含义 |
|---|---|
| **协议（protocol）** | `protocol.APIType`：`anthropic_beta`、`openai_chat`、`openai_responses` 等。一层只讲一种协议。 |
| **工作协议** | 横切逻辑实现所用的协议，即 **Anthropic Beta**。其它协议在边缘或 Bridge 处与它互转。 |
| **Endpoint** | 一种协议的完整调用实现：`Complete`（非流式）与 `Stream`（流式）。不处理 HTTP、响应头、SSE 分帧。 |
| **Call** | 一次调用的输入：`Request`（该协议的 SDK 原生请求）+ `State`（跨协议仍需保留的少量类型化事实）。 |
| **Response** | 非流式结果：`Value`（原生响应）+ `Usage`（协议无关的 token 用量）。 |
| **EventStream** | 拉取式（pull）流：`Next` 逐个取事件，`io.EOF` 表示正常结束；`Result()` 给出当前用量汇总；`Close` 必须恰好调用一次。 |
| **终端 Endpoint（upstream）** | 链的最内层，真正把请求发给 provider，复用现有 `internal/forwarding`。 |
| **Stage** | 同协议的包装层：`Wrap(next) Endpoint`。可在请求上行、响应/事件下行时插入逻辑。 |
| **Compose** | 按"外 → 内"的顺序把多个 Stage 包在一个 Endpoint 外面，只做结构校验，不执行请求。 |
| **Bridge** | 协议边界：把 Source 协议的调用转成 Target 协议，再把 Target 的响应/流转回 Source。 |
| **BridgeSession** | Bridge 为**每次调用**打开的会话，持有该次调用的转换状态（如客户端可见 model、转换后的请求）。 |
| **Adapt** | 把一个 Target 协议的 Endpoint 经 Bridge 暴露为 Source 协议的 Endpoint。 |
| **Operation** | 会话是为 `Complete` 还是 `Stream` 打开的；请求转换可能依赖它（如 Chat 的 `stream_options`）。 |
| **边缘（edge）** | 链的两端：客户端侧（HTTP 解析与写回）与 provider 侧（wire 形态）。V1 只存在于边缘。 |
| **wire** | 发给 provider 的实际形态。Anthropic provider 可走 Beta wire 或 V1 wire。 |

---

## 结构

```
客户端（HTTP）
   │  边缘：解析请求；V1 → Beta 升级；写回响应
   ▼
Stage（Beta）          ← 横切逻辑：只在工作协议上实现一次
   │
Adapt(Bridge)          ← 需要时：Beta ⇄ OpenAI Chat / Responses，逐次调用转换
   │
Stage（目标协议）      ← 例如 provider 侧的请求整形
   │
终端 Endpoint          ← upstream：Beta / V1 wire、Chat、Responses
   ▼
provider
```

组合方式是显式的：

```go
providerSide, _ := stage.Compose(upstreamEndpoint, providerStages...)   // 目标协议
beta, _ := stage.Adapt(providerSide, anthropicbridge.NewBetaToOpenAIChat(opts))
endpoint, _ := stage.Compose(beta, betaStages...)                       // 工作协议
```

`Compose(terminal, s1, s2)` 得到 `s1(s2(terminal))`：请求先经过 s1，响应最后经过 s1。

---

## 关键决定

### 1. Anthropic Beta 是唯一的工作协议

横切逻辑需要稳定、表达力最强的消息模型（thinking、tool_use / tool_result、缓存控制）。Beta 是这些能力的超集，所以链内的 Anthropic 一律是 Beta。

- **V1 不进入链**：客户端边缘把 V1 请求升级为 Beta（无损，V1/Beta 等价语料钉住）；provider 需要 V1 wire 时，终端 Endpoint 在转发前才降级。`Compose`、`Adapt` 遇到 `anthropic_v1` 直接拒绝。
- **降级不静默丢字段**：`request.ConvertAnthropicBetaToV1Request` 遇到 `anthropic-beta` 头或 V1 无法表达的 Beta 字段时返回错误，而不是丢掉。

### 2. 协议转换只发生在 Bridge，且逐次调用

Bridge 本身不可变、可并发；每次调用 `Open` 一个 `BridgeSession`，会话持有这次调用的状态。
这样同一个 Bridge 可以被上层反复调用（例如一个 Stage 在一次客户端请求里发起多轮 provider 调用），每轮独立转换。

Bridge 只做协议转换，复用 `internal/protocol` 现有的请求转换函数与流转换器，不复制、不改写转换逻辑。

### 3. 拉取式流

`EventStream` 由调用方逐个拉取。Stage 可以读一个事件、决定放行 / 暂扣 / 替换，再决定何时读下一个——这对"必须看完整个 tool_use 块才能决定"的逻辑是必需的；推送式回调做不到"暂扣"。

### 4. 错误原样穿过

provider 返回的错误是 SDK 的类型化错误，携带 HTTP 状态；客户端边缘据此映射状态码并决定是否 failover。
因此 Bridge 与 Stage 不包装、不改写 target 的错误，原样返回。

**开流失败必须先于任何事件暴露**：provider SDK 的流是惰性的，429 这类错误在第一次读取时才出现。如果转换器先产出 `message_start` 再读上游，客户端会先收到 200 再收到错误，failover 也失去机会。所以 Responses Bridge 在转换器运行前先预读第一个上游事件；Chat 转换器本身先读后发。两者都有测试钉住。

### 5. 跨协议只保留有明确契约的事实

`Call.State` 与 `Response.Usage` 是仅有的"协议无关事实"，每一项都有明确的消费方：

| 事实 | 生产方 | 消费方 |
|---|---|---|
| `State.OpenAIChat`（`*protocol.OpenAIConfig`） | Beta → Chat Bridge 转换请求时 | Chat 侧的 vendor 整形（thinking / reasoning 相关） |
| `Response.Usage` / `StreamResult.Usage` | 终端 Endpoint、Bridge | 用量统计；Bridge 自身未报告用量时沿用 target 的 |

刻意**没有**开放式属性包，也没有"以后可能有用"的字段：新增一项必须同时说明谁写、谁读。

### 6. 组合是显式的，没有注册表

链由调用方用 `Compose` / `Adapt` 手写组装。没有 Bridge 注册表、自动拓扑推导或能力声明：
协议组合是有限且已知的，显式组装更容易读、也更容易在代码审查里看清一条请求实际经过了什么。

---

## 契约要点（实现 Endpoint / Stage / Bridge 时）

- `EventStream.Next` 必须尊重 `ctx` 取消；调用方对每个成功返回的流恰好调用一次 `Close`（现有实现都做成了幂等）。
- `BridgeSession.ConvertStream` 返回的流**拥有** target 流：由它的 `Close` 关闭 target。转换失败时由 `Adapt` 关闭 target。
- `Stage.Wrap` 不得执行请求；`Compose` 校验每层协议一致、名称非空、返回值非 nil。
- Bridge 不得把请求状态存在 Bridge 实例上，只能放在会话里。
- 客户端可见的 model 由边缘或 Bridge 选项（`ResponseModel`）决定，与发给 provider 的 model 相互独立。

---

## Tool Round Stage

`stage/toolround` 是模型产生的**每一个工具调用**的唯一决策点，工作在 Beta 上。Guardrails 与 MCP（server tool）都通过它生效，因此二者的组合语义只定义一次。

| 术语 | 含义 |
|---|---|
| **轮（round）** | 一次 provider 调用及其响应。模型调用 server tool 后，Stage 执行工具并发起下一轮。 |
| **Gate** | Guardrails 一侧：筛请求、判定每个 tool_use、筛 server tool 结果、还原凭证别名、检查最终响应。实现：`guardrailspipeline.ToolRoundGate`。 |
| **Owner** | MCP 一侧：认领 server tool、执行、在混合轮后暂存与续接。实现：`toolengine.AnthropicBetaOwner`。 |
| **混合轮** | 同一轮既有 server tool 又有客户端工具：server tool 执行后结果暂存，等客户端带着自己工具的结果回来时再拼回对话。 |

每一轮里，每个 tool_use 按固定顺序决策：

1. **Gate**：被拦下的调用结束本轮对话，替换为说明文本；该轮其它调用都不执行。
2. **Owner**：属于 server 的调用执行，结果经 Gate 筛查后进入下一轮；混合轮只执行 server 部分并暂存。
3. 其余调用原样交给客户端。

流式行为：

- 文本与 thinking **实时转发**；只有 tool_use 块在本轮决策完成前被暂扣。
- 跨多轮的回答对客户端是**一条消息**：只有一个 `message_start`，块索引连续。
- 达到轮数上限（执行过 `DefaultMaxRounds` 轮 server tool）后，再请求模型一次，让它基于已有结果作答；若它仍调用 server tool，则以 `end_turn` 结束且不暴露这些调用。现有工具循环在上限处返回空或失败的回答。
- 上游流在 `message_stop` 之前结束（截断）且本轮有暂扣的 tool_use 时，本轮不执行也不下发任何工具调用，流以错误结束。
- 用量覆盖所有轮。
- server tool 执行之后再出的错误标记为**已提交**（`stage.CommittedError`）：客户端边缘不会因此 failover 到另一个服务，避免工具被重复执行。

混合轮的续接只在有会话时进行，并且只由"回答了该轮某个客户端调用"的请求消费（`continuationStore.popAnswered`），因此既不会串到别的对话，也不会被无关请求提前取走。

Gate 的筛查时机：server tool 结果连同发起它的 assistant 轮一起筛查；非流式的最终回答先以别名形态经响应检查，通过后才还原凭证（与现有响应 guardrails 相同），流式回答在下发客户端工具调用时还原。

Gate 与 Owner 都是可选的：两者都没有时，Tool Round Stage 直接透传。

---

## 如何证明与现有路径等效

这一层替换的是已有的转换与转发代码，因此测试以**差分**为主：同一输入分别走现有路径与新层，输出（经 golden 归一化：排序键、编号 ID、抹去时间戳）必须一致。

| 对象 | 对照的现有路径 | 测试 |
|---|---|---|
| Bridge 发往 provider 的请求、Chat 的 `OpenAIConfig` | `transform.BaseTransform` | `protocoltest.TestBridgeRequestEquivalence` |
| 终端 Endpoint 发给 provider 的请求 | 现有 `forwarding.Forward*` 调用 | `upstream.TestUpstreamWireMatchesDirectForward` |

Bridge 返回方向（完整消息与逐事件的流）在路由切换前曾与现有非流式转换函数和流式 handler 做过同样的差分；这些 handler 随路由切换删除后，返回方向由 HTTP 级的 golden wire 快照固定。

差分测试无法覆盖的是**路由如何接入这一层**；那部分由 HTTP 级的 golden wire 快照（`protocoltest.TestGoldenWire`）在接入时证明。

---

## 扩展指引

- **新增 provider 协议**：实现一个终端 Endpoint（复用 `forwarding`），以及一对与 Beta 互转的 Bridge；再为二者各补一组差分测试。横切 Stage 无需改动。
- **新增横切能力**：实现一个 Beta Stage，用 `Compose` 放到合适的位置。不要在 Bridge 或终端 Endpoint 里加业务逻辑。
- **新增跨协议事实**：先确认它确实需要跨越协议边界，再在 `ProtocolState` 里加一个有类型、有注释、有明确生产方和消费方的字段。
