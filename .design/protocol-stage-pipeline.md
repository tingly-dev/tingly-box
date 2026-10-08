# Protocol Stage 请求管线

protocolserver 如何把一次请求接到 Protocol Stage 上：每个 failover attempt 经过哪些阶段、
rule flag / 输出上限 / vendor 规则这类请求整形步骤各放在哪一段、两条执行路径怎么装配。
Stage / Bridge 自身的契约见 `.design/protocol-stage.md`；各 flag 的注入手法见 `.design/rule-flags.md`。
图示见 `.design/protocol-stage-pipeline.pencil.md`，与本文逐节对应。

Status: 规则已定；偏差 1、2 的迁移见 #1901、#1902，进度见文末"现状偏差与迁移"
Date: 2026-09-29
相关：`.design/protocol-stage.md`（Stage / Bridge 契约）、`.design/rule-flags.md`（flag 注入手法）、
`.design/openai-endpoint-routing.md`（target 解析）、`.design/tier-routing.md`（failover 的逐 attempt 管线）

## 动机

Protocol Stage 重构（#1880 / #1882）把跨协议请求搬进了 Stage 管线，但 rule flag、Anthropic pre-chain、
各类 Transform 的位置是沿用旧整链时"改到哪个 handler 就加在哪"留下来的，重构没有顺带整理。
四个入口各自解析 target 与 flags，同一类约束在不同入口出现在不同阶段。这在旧架构下就存在，
Stage 之后多了一条路径，问题更明显。

#1897（已由 #1899 修复）是一个典型后果：

- Anthropic pre-chain 在 target 还没确定时就按 Anthropic 的 `budget_tokens ≤ max_tokens` 规则截断 budget。
  请求发往 OpenAI Chat 时 budget 根本不上线路，只用来推 effort 档位，被截断后所有 budget 都落到 `low`。
- Responses → Chat 的 thinking 配置在 Stage Bridge 与 `BaseTransform` 两处各写死一份，两处都丢了 effort。

两个 bug 的共同点：步骤放错了阶段，或者同一个事实有多个生产者。本文给出判定规则，让新步骤有唯一的位置。

## 一次请求经过的阶段

> 图：`protocol-stage-pipeline.pencil.md`「一次请求经过的阶段」

```
handler 前段（每个请求一次，与 provider 无关）
  解析 → rule / 选路 → vision proxy → context-1m → 录制器 → 原始请求快照（failover 用）

每个 attempt（failover 的每个候选一次，见 tier-routing.md）
  ① Plan        解析 provider 风格 → target、rule flags、preBase / preVendor、模型输出上限
                 只读，不改请求
  ② Source 半段  客户端协议形态上运行：客户端协议的必填补齐（max_tokens_default）→ preBase 规则 → StagePre 录制
                 （Anthropic 客户端的请求 guardrails 也在这一侧）
  ③ 协议转换    跨协议 Stage 路径：Bridge（逐次调用）；其余路径：BaseTransform
  ④ Target 半段  provider 协议形态上运行，Stage 路径每轮 provider 调用各跑一次：
                 输出上限（output_limit）→ MCP → Consistency → preVendor 规则 → Vendor → StagePost 录制
  ⑤ 发送 / 回写
```

Source 半段与 Target 半段在两条路径上是**同一组** Transform（`sourceTransforms` / `targetTransforms`，
`internal/protocolserver/protocol_transform.go`），只有第 ③ 步不同：

| 客户端 → provider | 路径 | 转换 | Target 半段的运行方式 |
|---|---|---|---|
| Anthropic → OpenAI Chat / Responses | Stage（`serveAnthropicOnOpenAI`） | `anthropicbridge` | `targetTransformStage`，每轮一次 |
| OpenAI Chat / Responses → Anthropic | Stage（`serveOpenAIOnAnthropic`） | 边缘转成 Beta | `targetTransformStage`，每轮一次 |
| 其余（同协议、OpenAI ↔ OpenAI、→ Google） | 旧整链（`buildTransformChain`） | `BaseTransform` | 整链内一次 |

## 放置规则

> 图：`protocol-stage-pipeline.pencil.md`「放置规则」

判定时只问一句：**这一步读的、改的，是哪一方的东西？**

| 步骤的性质 | 放在 | 例子 |
|---|---|---|
| 只取决于 provider / rule / 模型，不看请求内容 | ① Plan | target 解析（含 `openai_endpoint_override`）、rule flags 与 scenario 继承、模型输出上限 |
| 客户端协议自身的语义，与发往哪里无关 | ② Source 半段 | Anthropic `max_tokens` 缺省补齐（该协议的必填字段）、`cursor_compat`、`clean_header`、`block_tools`、`claude_code_compat`、`smart_compact` |
| 两个协议之间的语义映射 | ③ 转换（Bridge / 转换函数） | budget → effort 分档（`thinking.EffortFromBudget`）、`reasoning.effort` → `reasoning_effort` |
| provider 线路的约束、模型限额、vendor 怪癖 | ④ Target 半段 | `max_tokens` 上限、`budget_tokens ≤ max_tokens`（仅 Anthropic 形态）、Consistency、`thinking_effort` 规则、`use_max_completion_tokens`、Vendor |

由此推出的约束：

1. **Source 半段不做有损改写。** 不得为了迁就某个 target 的约束去改客户端的意图。
   需要迁就 target 的，放到 Target 半段，在 target 形态上改。#1897 的 budget 截断就是反例。
2. **Target 约束靠形态命中，不靠开关。** Target 半段的 Transform 对 `ctx.Request` 做 type-switch：
   只有 Anthropic 形态才有 budget，所以 budget 截断天然只作用于 Anthropic target。
   不要给 Source 侧的步骤加"target 是什么"的参数（#1899 修 #1897 时给 Anthropic pre-chain 加的 `KeepThinkingBudget` 开关即是此类，已在 #1902 迁移输出上限时删除）。
3. **Plan 每个 attempt 只做一次，四个入口共用。** Handler 不再各写一份 provider 风格的 switch，
   也不再把同一个 preVendor 列表传两遍。
4. **跨协议事实在转换处产出一次。** Bridge 与 `BaseTransform` 必须调用同一个产出函数
   （例：`request.OpenAIConfigFromResponses`），不能各写一份字面量。
5. **Vendor 之后只有录制。** `Vendor` 直面 provider、做最终且不可逆的改写（model alias、metadata、
   billing header、DeepSeek thinking patch 等），必须是最后一个 mutation；rule 的 preVendor transforms
   因此装在 `Consistency` 之后、`Vendor` **之前**，StagePost 录制落在 `Vendor` 之后，抓到的是真正发出的请求。

新增一个步骤时，按上表选位置；如果它需要同时知道客户端形态和 target，通常说明它应该拆成两步：
Source 侧把意图归一，Target 侧按线路约束落地。

## 装配

> 图：`protocol-stage-pipeline.pencil.md`「装配」

```
run*Attempt（每个 failover attempt）
  │
  │ plan := planAttempt(c, rule, provider, model, source, …)      attempt_plan.go
  │   ├─ target    := resolveAttemptTarget(…)       provider 风格 + endpoint 路由
  │   ├─ flags     := ResolveRuleFlagsWithScenario(…)
  │   ├─ preBase   := RulePreBaseTransforms(flags)
  │   └─ preVendor := RulePreVendorTransforms(flags)
  │
  │ ctx := Transform<Source>(c, req, plan, …)  →  transformRequest
  │   ├─ 旧整链：buildTransformChain(preBase, preVendor) → Execute          一次
  │   └─ Stage（plan.servedByStage()）：只跑 sourceTransforms(preBase)      一次
  │
  │ Stage 路径：serve*(c, plan, ctx, …)
  │   → targetTransformStage(targetTransforms(plan.PreVendor))              每轮
  │ 旧整链：DispatchChainResult(c, ctx, …)
```

- `sourceTransforms` / `targetTransforms`（`internal/protocolserver/protocol_transform.go`）是 chain
  两半的唯一装配点。`buildTransformChain` 把它们与 `BaseTransform` 串成旧整链；Stage 路径
  （`serveAnthropicOnOpenAI` / `serveOpenAIOnAnthropic`）只在 source 侧跑前半段，后半段包成
  `targetTransformStage`（`stage_transform.go`），放在终端 Endpoint 外面，每轮 provider 调用前各跑一次。
  preBase / preVendor 在两条路径上落在同一位置，handler 不自行 prepend / append。
- 4 个入口（Anthropic V1 / Beta、OpenAI Chat / Responses）都先经 `planAttempt` 得到同一份
  attempt plan（target、flags、preBase / preVendor），再经 `transformRequest` 进入 chain；
  Stage 路径的 `serve*` 从 plan 取 preVendor，不再单独传参。（`smart_compact` 由 `transformRequest`
  在 Anthropic 入口按 scenario 默认 flag 单独 prepend 到最前。）
- chain 顺序的回归护栏：`internal/protocolserver/protocol_transform_test.go`。thinking 与输出上限在
  ① ② ③ ④ 各段的组合效果由 harness 的 `thinking_limits` 段逐组合校验（`.design/harness-matrix.md` §10.5）；
  其余 rule flag 在每个适用的 source → target 与流式组合上的效果由 `flag_paths` 段校验（§10.6）。
- 非 flag 的请求整形也装在这两半里：Anthropic `max_tokens` 缺省补齐在 source 半段最前
  （`MaxTokensDefaultTransform`），模型输出上限在 target 半段最前（`OutputLimitTransform`）。
  没有 handler 在 chain 之外改请求（原 `ExecuteAnthropicPreChain` 已删除）。

## 现状偏差与迁移

> 图：`protocol-stage-pipeline.pencil.md`「现状偏差与迁移」

| # | 偏差 | 目标 | 状态 |
|---|---|---|---|
| 1 | 四个 `run*Attempt` 各自解析 target（三份 provider 风格 switch，Chat 另有一次 `tempFlags`）；preVendor 列表同时传给 `transformRequest` 与 `serve*`；`skip_usage` / `cursor_compat` 提示只在 OpenAI 入口写入 `Extra` | 共用的 attempt plan（规则 3）：`internal/protocolserver/attempt_plan.go` 的 `planAttempt` 解析 provider、target、flags、preBase / preVendor 与输出上限；`transformRequest` 按 `servedByStage()` 决定跑整链还是只跑 source 半段，并统一写入用量提示 | 已完成（#1901） |
| 2 | 输出上限分散在三处且各不相同：Anthropic 入口的 `ExecuteAnthropicPreChain`（Source 侧，补齐 + 上限 + budget 截断）、Chat 入口 handler 内联截断 `max_tokens`、Responses 入口不截断 | Source 侧只补齐 Anthropic 必填的 `max_tokens`（`MaxTokensDefaultTransform`）；上限与 budget 截断移到 Target 半段的 `OutputLimitTransform`，按形态生效（规则 1、2）。删除 `ExecuteAnthropicPreChain`、handler 内联截断与 `KeepThinkingBudget`。统一后 Chat 的 `max_completion_tokens` 与 Responses 的 `max_output_tokens` 也按模型上限截断（此前不截） | 已完成（#1902） |
| 3 | Chat 形态的 thinking 意图有两个来源：`req.ReasoningEffort`（客户端原值，原样透传）与 `OpenAIConfig.ReasoningEffort`（网关推导值，按 vendor 分档），靠 `RuleThinkingTransform.syncConfig` 同步；~~`buildOpenAIConfigFromRequest` 在 Chat 客户端带 `thinking` 扩展字段时猜一个 `low`~~（已删，#1918） | 见下 | 部分完成（剩第一个开放问题） |
| 4 | ③ Anthropic → Responses 转换不把客户端的 thinking budget 带成 `reasoning.effort`（只有 rule 的档位能到） | 转换处按 `thinking.EffortFromBudget` 产出，与 → Chat 共用 `anthropicViewReasoningEffort`；只在请求本身开启 thinking（enabled / adaptive）时带，历史里的 thinking 块不算 | 已完成（#1917，TL2） |
| 5 | ③ Chat / Responses → Anthropic 转换不把客户端的 reasoning effort 带成 thinking（只有 rule 的档位能到） | 转换处按 `thinking.BudgetMapping` 产出 thinking 与 `output_config.effort`，并给回答留空间（客户端没设上限：`max_tokens` 加上 budget；设了：budget 至多占一半，放不下 1024 就不开），交给 ④ 与 vendor 按模型能力落地；④ vendor 对 Anthropic target 做 thinking 与请求其余部分的协调（`ReconcileBetaThinkingWithRequest`）：最后一条带 `tool_use` 的 assistant 消息不以 thinking 块开头、或 tool_choice 强制用工具时关闭 thinking，否则丢掉 thinking 不允许的采样参数（temperature ≠ 1、top_k，top_p 抬到 0.95）。**只对网关造出来的 thinking 生效**：非 Anthropic 客户端（OpenAI 的历史本来就没有 thinking 块），或 rule 的 `thinking_effort` 在客户端没开的请求上打开的 thinking（`RuleThinkingTransform` 在 `Extra` 里留标记，`thinkingGuardApplies` 读取）。Anthropic 客户端自己的 thinking 原样转发 | 已完成（#1917，TL3）；Anthropic 客户端的例外见下 |

> **Anthropic 客户端的 thinking 不动（缓存）。** #1917 最初对所有 Anthropic target 都跑这条保护，包括 Claude Code 自己的请求。上游某一轮不返回 thinking 块时，下一个工具回合的 `thinking` 被改成 disabled，回到新的用户回合又是 enabled，同一会话里 `thinking` 来回翻。provider 的缓存前缀在 thinking 设置变化时失效（Anthropic 官方明确如此），命中率因此在"生效"和"突然失效"之间跳。真实 Claude Code 会话回放可复现：请求 0 enabled、1–3 disabled、4–5 enabled、6–8 disabled。回归：`TestAnthropicClientThinkingReachesUpstreamAsSent`、`vendor_thinking_guard_test.go`。
| 6 | 响应整形只在部分路径上读用量提示：Chat 客户端 → Responses provider 时，`skip_usage` / `cursor_compat` 不剥 usage | 用量提示是 Chat 客户端专用的（Cursor 这类处理不了 usage chunk）：所有 Chat 客户端回写路径统一用 `shouldStripChatUsage` 判断；Responses / Anthropic 客户端保留 usage | 已完成（#1918，FP1） |
| 7 | ~~`skip_usage` 对 Responses 客户端不生效~~ | 不是偏差：用量提示只作用于 Chat 客户端，harness 改为断言其它客户端保留 usage | 撤销（#1918，原 FP2） |
| 8 | rule 级 recording：Responses 客户端 → Anthropic / Chat provider 时不出记录（→ Responses 正常） | 录制在两条路径上同样落盘 | 待定（FP3） |

偏差 3 需要先定语义再动代码，开放问题：

- 客户端直接给出的 `reasoning_effort`（例如 Chat 客户端的 `xhigh`）发往未验证的 OpenAI 兼容 host 时，
  要不要和网关推导值一样按 `genericEffortTiers` 收窄？现状是原样透传，只收窄推导值。
- ~~Chat 客户端带 DeepSeek 风格的 `thinking: {type: enabled}` 扩展字段、但没给 effort 时，
  是保留现状补一个 `low`，还是不补、交给 vendor 默认？~~ 已定（#1918）：这是 vendor 自己的开关，
  通用层不从 Chat 客户端的字段推导任何档位（`BaseTransform` 对 Chat 请求不再产出 `OpenAIConfig`）；
  DeepSeek 的 vendor transform 在开关打开且没有 `reasoning_effort` 时写出 DeepSeek 文档的默认档 `high`，
  其它 vendor 原样收到开关、走各自默认。

这两点定下来之后，`OpenAIConfig` 可以收敛成"请求上的 effort 字段 + 它是否由网关推导"这一个事实，
由 Bridge / `BaseTransform` 共用的产出函数写入，vendor 只读这一处。

## 与其它文档的关系

- `rule-flags.md` 描述每个 flag 用哪种注入手法（Type 1b-pre / 1b-post / 2 / 3 / 4 / 5）。
  本文决定这些手法在管线中的位置：Type 1b-pre = Source 半段，Type 1b-post = Target 半段，Type 4 = Plan。
  chain 的骨架与装配以本文为准，`rule-flags.md` 不再重复。
- `protocol-stage.md` 描述 Stage / Bridge 的契约；本文描述 protocolserver 如何把 flag 与 Transform 接到这条管线上。
