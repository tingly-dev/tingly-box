# Protocol Stage 请求管线 (pencil)

`.design/protocol-stage-pipeline.md` 的图示，逐节对应：下面每个二级标题与正文同名，正文每节开头也指回这里。
图按设计目标画（偏差迁移全部落地后的形态）；还没落地的部分以「现状偏差与迁移」一节为准，
那里每条偏差都配了"以前 → 以后"的图。

图例：`║` 四个入口共用的一条路 · `┆` 各入口各写一份 · `✗` 偏差 / 出过的 bug ·
`▸` 录制点 · `⟳` Stage 路径上每轮 provider 调用都跑

---

## 一次请求经过的阶段

```
  run*Attempt(c, req, provider, model)                  ← 四个入口共用下面整条路
        ║
        ▼
  ┌────────────────────────────────────────────────────────────────────────────┐
  │ ① PLAN   planAttempt(…)   只读，不碰请求                    attempt_plan.go   │
  ├────────────────────────────────────────────────────────────────────────────┤
  │   provider   = ResolveStyle(客户端协议的风格)                                  │
  │   target     = resolveAttemptTarget(…)    ← 唯一一份 provider 风格表 + 端点路由 │
  │   flags      = ResolveRuleFlagsWithScenario(…)                               │
  │   preBase / preVendor = RulePre*Transforms(flags)                            │
  │   MaxAllowed = 模型输出上限     DefaultMaxTokens = 缺省 max_tokens            │
  └────────────────────────────────────────────────────────────────────────────┘
        ║   （Anthropic 客户端：请求 guardrails）
        ▼
  ┌────────────────────────────────────────────────────────────────────────────┐
  │ ② SOURCE 半段   客户端协议形态                          sourceTransforms      │
  ├────────────────────────────────────────────────────────────────────────────┤
  │   max_tokens_default ... 只补 Anthropic 必填的 max_tokens，不截断、不碰 budget │
  │   preBase 规则 ......... cursor_compat / clean_header / block_tools / …       │
  │   ▸ StagePre 录制 ....... 客户端请求                                           │
  └────────────────────────────────────────────────────────────────────────────┘
        ║
        ▼
  ┌────────────────────────────────────────────────────────────────────────────┐
  │ ③ 转换   两个协议之间的语义映射                                              │
  │   Stage 路径：Bridge（anthropicbridge / 边缘转 Beta）  ·  整链：BaseTransform │
  │   budget → effort 分档、reasoning.effort → reasoning_effort 都只在这里做       │
  └────────────────────────────────────────────────────────────────────────────┘
        ║
        ▼
  ┌────────────────────────────────────────────────────────────────────────────┐
  │ ④ TARGET 半段   provider 协议形态                ⟳  targetTransforms          │
  ├────────────────────────────────────────────────────────────────────────────┤
  │   output_limit ......... 按形态截断：Anthropic max_tokens(+budget)             │
  │                          / Chat max_tokens+max_completion_tokens              │
  │                          / Responses max_output_tokens / Google               │
  │   MCP → Consistency → preVendor 规则 → Vendor（最后一个改写）                  │
  │   ▸ StagePost 录制 ...... 真正发出的请求                                       │
  └────────────────────────────────────────────────────────────────────────────┘
        ║
        ▼
     provider
```

## 放置规则

```
            新的一步，它读的、改的是谁的东西？
                          │
      ┌───────────────────┼──────────────────────┬─────────────────────────┐
      ▼                   ▼                      ▼                         ▼
  provider / rule /   客户端协议自身         两个协议之间              provider 线路 /
  模型，不看请求      （与发往哪里无关）      的语义映射                模型限额 / vendor
      │                   │                      │                         │
      ▼                   ▼                      ▼                         ▼
   ① Plan           ② Source 半段           ③ 转换函数               ④ Target 半段
                     不做有损改写            Bridge 与 BaseTransform    按形态 type-switch 命中，
                                            共用一个产出函数            不带"target 是什么"的开关

  需要同时知道客户端形态和 target？ → 拆成两步：② 把意图归一，④ 按线路约束落地
```

| 步骤 | 段 | 以前在哪 |
|---|---|---|
| target 解析 | ① `resolveAttemptTarget` | 三个 handler 各一份 switch |
| rule flags → preBase / preVendor | ① 进 plan，只传 plan | 每个 handler 解析，preVendor 传两次 |
| `skip_usage` / `cursor_compat` 提示 | `transformRequest` 统一写 | 只有 OpenAI handler 手写 |
| Anthropic `max_tokens` 补齐 | ② `max_tokens_default` | pre-chain（chain 外） |
| 模型输出上限 / budget 截断 | ④ `output_limit` | pre-chain · handler 内联 · 没有 |
| budget → effort、Responses effort | ③ 转换函数 | 同左（#1899 已统一 Responses→Chat 的产出函数） |

## 装配

```
  transformRequest(plan)
        │
        ├── plan.servedByStage() == false   （同协议、OpenAI↔OpenAI、→Google）
        │
        │     buildTransformChain = [ ② source 半段 ] → BaseTransform → [ ④ target 半段 ]
        │     一次跑完 ─────────────────────────────────────► DispatchChainResult ─► provider
        │
        └── plan.servedByStage() == true    （Anthropic → OpenAI · OpenAI → Anthropic）

              [ ② source 半段 ]  跑一次
                    │
                    │  OpenAI 客户端：边缘转成 Beta
                    ▼
              Tool Round Stage（Beta）......... guardrails 判定 / server tool 多轮
                    │
                    │  Anthropic 客户端 → OpenAI provider：Bridge（Beta → Chat / Responses），逐轮转换
                    ▼
              targetTransformStage = [ ④ target 半段 ]  ⟳ 每轮 provider 调用一次
                    │
                    ▼
                provider
```

两条路径只有第 ③ 步不同；flag 与限额放在 ② 或 ④，就自动对两条路径都生效，不用各写一份。

## 现状偏差与迁移

### 偏差 1：四个入口各自解析 target 与 flags

```
  以前                                                                  以后
  Anthropic V1/Beta        OpenAI Chat              OpenAI Responses
  ┆ ResolveStyle           ┆ ResolveStyle            ┆ ResolveStyle           ║ planAttempt(…)
  ┆ switch APIStyle ✗ 副本1 ┆ switch APIStyle ✗ 副本2  ┆ switch APIStyle ✗ 副本3  ║   ResolveStyle
  ┆                        ┆  （先算一次 tempFlags）  ┆                        ║   resolveAttemptTarget   ← 一份表
  ┆ ResolveRuleFlags…      ┆ ResolveRuleFlags…       ┆ ResolveRuleFlags…      ║   ResolveRuleFlags…
  ┆                        ┆ Extra[skip_usage] 手写   ┆ Extra[skip_usage] 手写  ║   RulePre*Transforms
  ┆ serve*(… preVendor)    ┆ serve*(… preVendor)     ┆ serve*(… preVendor)    ║ Transform<Source>(plan)
  ┆   ✗ 第二次传            ┆   ✗ 第二次传             ┆   ✗ 第二次传            ║ serve*(plan)
```

### 偏差 2：输出上限分散在三处

```
  以前                                                  以后
  Anthropic:  ExecuteAnthropicPreChain（chain 外）       ② max_tokens_default   只补 Anthropic 必填
              补齐 + 截断 + budget 截断                  ④ output_limit         按上游形态截断
              ✗ 改客户端请求，按 Anthropic 规则动 budget                          budget 只在 Anthropic 形态上动
              ✗ #1899 加 KeepThinkingBudget 才躲开 OpenAI
  Chat:       handler 内联截断 max_tokens
              ✗ max_completion_tokens 不管
  Responses:  ✗ 不截断
```

| 客户端 → 字段 | 以前 | 以后（④ `output_limit`） |
|---|---|---|
| Anthropic `max_tokens` | pre-chain 补齐 + 截断（chain 外） | ② 补齐，④ 截断 |
| Anthropic `budget_tokens` | pre-chain 截断，OpenAI target 靠开关跳过 | 只在 Anthropic 形态上截断 |
| Chat `max_tokens` | handler 内联截断 | ④ 截断 |
| Chat `max_completion_tokens` | 不截断 | ④ 截断 ⚠ 行为变化 |
| Responses `max_output_tokens` | 不截断 | ④ 截断 ⚠ 行为变化 |
| 转成 Google 后的 `MaxOutputTokens` | 转换前在客户端形态上截断 | ④ 在 Google 形态上截断 |

同一个请求走三个版本（#1897 的场景：Claude Code → OpenAI 兼容 provider，模型不在 catalog，
`MaxAllowed` 退回 8192，客户端发 `max_tokens: 40000`、`budget_tokens: 10240`）：

```
  #1899 之前                         #1899（止血）                      以后
  ─────────────────────────         ─────────────────────────          ─────────────────────────
  pre-chain（chain 外，客户端请求）  pre-chain（chain 外）              ② source 半段
    max_tokens 40000 → 8192            max_tokens 40000 → 8192            max_tokens 已有，不动
    budget 10240 > 8192                budget：target 是 OpenAI，          budget 不动
      → max(1024, 819) = 1024 ✗          KeepThinkingBudget 跳过 ✓
        │                                  │                                  │
  ③ Bridge：EffortFromBudget(1024)   ③ Bridge：EffortFromBudget(10240)  ③ Bridge：EffortFromBudget(10240)
      = minimal                          = medium                           = medium
        │                                  │                                  │
  ④ Vendor genericEffortTiers         ④ Vendor genericEffortTiers         ④ output_limit（Chat 形态）
      minimal → low ✗                    medium → medium ✓                  max_tokens 40000 → 8192
        │                                  │                                   没有 budget 可动
        ▼                                  ▼                                ④ Vendor: medium → medium ✓
  reasoning_effort: "low"            reasoning_effort: "medium"               ▼
                                                                          reasoning_effort: "medium"
```

同一个请求换成 **Anthropic provider**：④ `output_limit` 看到的是 Anthropic 形态，`max_tokens` 截到 8192、
`budget_tokens` 截到 1024——这是 Anthropic 线路的硬约束，只在那里做。

### 偏差 3：Chat 形态的 thinking 意图有两个来源

```
  客户端 reasoning_effort ──► req.ReasoningEffort ─────────────┐   原样透传
                                                               ├──► ④ Vendor  ──► reasoning_effort
  ③ 转换推导的档位 ────────► OpenAIConfig.ReasoningEffort ─────┘   按 vendor 分档
                              ▲
                              └─ RuleThinkingTransform.syncConfig 两边同步
                                 （已删 #1918）buildOpenAIConfigFromRequest：带 thinking 扩展字段时猜 low
                                 现在：Chat 客户端的字段只留在请求上；DeepSeek transform 补自己的默认档 high

  以后（待定，见正文开放问题）：请求上一个 effort 字段 + "是否由网关推导"，③ 写一次，④ 只读这一处
```

### 偏差 4–5：组合校验发现的缺口（harness `thinking_limits`，TL2–TL3，已修 #1917）

```
  偏差 4（TL2）  ③ Anthropic → Responses
    client: thinking.budget_tokens 10240               rule thinking_effort = ""（按客户端）
      以前：Bridge 不产出 effort ──► reasoning.effort 缺失                     ✗
      现在：anthropicViewReasoningEffort ── EffortFromBudget(10240) = medium
              ──► reasoning.effort "medium"                                   （与 → Chat 同一个 helper）
            只认请求本身的 thinking（enabled / adaptive）；只在历史里有 thinking 块、或 disabled ──► 不带

  偏差 5（TL3）  ③ Chat / Responses → Anthropic
    client: reasoning_effort "high"                     rule thinking_effort = ""（按客户端）
      以前：边缘转 Beta 不产出 thinking ──► 上游没有 thinking                    ✗
      现在：③ applyOpenAIEffortAsThinking ── BudgetMapping[high] = 20480
              ──► thinking.enabled + output_config.effort "high"
                  并给回答留空间：没设上限 max_tokens 4096 + 20480；设了 32000 → budget ≤ 16000
              ──► ④ output_limit 截到模型上限 ──► ④ vendor ReconcileBetaThinkingWithRequest：
                    最后一条 assistant tool_use 消息没有 thinking 块？ ── 是 ──► thinking.disabled
                    tool_choice 强制用工具（any / tool）？         ── 是 ──► thinking.disabled
                    否则 temperature ≠ 1、top_k 丢掉，top_p 抬到 0.95

  以前两条都只在 rule = ""（按客户端）时出现：rule 设了档位时，④ 的 RuleThinkingTransform 会补上。
```

### 偏差 6–8：rule flag 在部分路径上不生效（harness `flag_paths`；FP1 已修、FP2 撤销 #1918，FP3 待定）

```
                         → Anthropic       → Chat           → Responses
  skip_usage / cursor_compat（Chat 客户端）
     以前：                 ✓                 ✓                ✗ FP1  usage 仍回给客户端
     现在：                 ✓                 ✓                ✓      三种 provider 的 Chat 回写都走 shouldStripChatUsage
  skip_usage（Responses / Anthropic 客户端）
                            保留 usage        保留 usage       保留 usage   ← 正确行为：用量提示是 Chat 客户端专用
  recording（Responses 客户端）
                            ✗ FP3 无记录      ✗ FP3 无记录     ✓

  ✓ 的格子与 ✗ 的格子走的是同一组 ② / ④；差别在 ⑤ 回写：读不读 Extra 里的用量提示、录不录。
  其余 flag（headers、block_tools、clean_header、claude_code_compat、vision、override、context_1m、
  affinity、claude_org_id）在每个适用组合、流式与非流式上都生效。
```
