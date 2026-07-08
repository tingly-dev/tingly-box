# smart_compact 设计

> 范围:`internal/smart_compact/`(压缩变换)+ `vmodel/anthropic/cc_compact.go`、`defaults.go`(Claude Code 入口)+ `internal/server/protocol_transform.go`(代理链路接入)。
> 配套图示:`.design/smart-compact-pencil.md`。实现 spec:`.sdlc/docs/smart-compact-key-tool-preserve-20260708.spec.md`。

---

## 1. 核心心智模型:信息平坦化(最高原则)

`claude-code-compact` 作为**适配 Claude Code 的压缩辅助**,它的输出**不沿用标准 Anthropic 消息结构**。本质上做的是**信息平坦化(information flattening)**:

- 把多轮、多角色、带 tool_use/tool_result 配对的标准对话,**重组**成一段模型当上下文读的**叙述**。
- XML(`<conversation>/<user>/<assistant>/...`)是承载格式,本质是「把结构化对话拍平成线性信息流」。
- **标准协议约束在这里不适用**:不需要 `tool_use↔tool_result` 严格配对 / `tool_use_id`,不需要 user/assistant 严格角色交替,不存在「配对破坏→上游 400」——**输出是给 CC 当上下文读的叙述,不是发给上游校验的标准结构**。
- **对 tool,唯一要紧的是「位置对」**:调用与其结果在叙述里**相对位置正确**(调用在前、结果紧随),让模型能还原「先做了 A→得到 X,再做 B→得到 Y」的因果时序。

真正要守的只有三条约束:**信息保真(FIDELITY)、位置正确(POSITION)、归属正确(ATTRIBUTION)**。

---

## 2. 两条上线通路

```
通路 A:scenario flag `SmartCompact`(真代理,Anthropic only)
   rule 的 scenario config.Flags.SmartCompact == true
   → 在 Anthropic V1/Beta 请求的 transform chain 最前面 prepend
     smart_compact.NewCompactTransform(2)   // 只删历史 round 的 thinking
   (OpenAI Chat/Responses 路径无此分支)

通路 B:vmodel 虚拟模型(进程内)
   客户端把 model 指向 claude-code-compact / compact-* / claude-code-strategy
   → dispatcher 短路到 in-process handler,跑模型自带的 chain
   Claude Code 的 /compact 实际命中 claude-code-compact
```

---

## 3. Round 与 Grouper

定义在 [`internal/protocol/round.go`](../internal/protocol/round.go)。

**Round = 从一条「纯 user 消息」开始,到下一条「纯 user 消息」为止(不含)。**

- 「纯 user 消息」= `role=="user"` 且 content 里没有 `OfToolResult` block。
- 一个 round:1 条纯 user + N 条 assistant(可带 tool_use)+ M 条 tool_result user。
- 最后一个 round 标记 `IsCurrentRound=true`,是当前请求,压缩一律保留。
- `RoundStats`:`UserMessageCount / AssistantCount / ToolResultCount / TotalMessages / HasThinking`。
- Guard(`ShouldCompactRound`,[`compact_helpers.go`](../internal/smart_compact/compact_helpers.go)):`UserMessageCount==1 && AssistantCount>=1` 才允许压缩,异常 round 跳过。

---

## 4. 变换清单

| 变换 (`Name()`) | 作用 | 触发 | 上线通路 |
|---|---|---|---|
| `compact_thinking` | 删历史 round 的 thinking,留其余 | 无 gate | A(flag)+ vmodel `compact-thinking` |
| `round_only` | 历史只留 user/assistant 文本 | 无 gate | vmodel `compact-round-only` |
| `round_files` | 历史留文本 + 文件路径注入虚拟 read_file tool | 无 gate | vmodel `compact-round-files` |
| `xml_compact` | 整段历史压成一条 assistant XML 叙述(见 §5) | 无 gate(本身);被 CC 包装层加 gate | vmodel `claude-code-compact`(经 `ClaudeCodeCompactTransform`) |
| `claude_code_compact` | XML 压缩的条件包装 | 最后一条 user 含 "compact" 子串 | vmodel `claude-code-compact` |
| `deduplication` | 同签名 tool 只留最后 output | 无 gate,仅 Beta | vmodel `claude-code-strategy` |
| `purge_errors` | 出错 tool 超期换 input 占位 | 无 gate,仅 Beta | vmodel `claude-code-strategy` |

> 仅测试覆盖、无生产调用:`conversation-replay`、`conversation-document`、原生 compaction block(`xml-compaction`)、`NewCompactTransformWithConfig`、legacy `CompactTransformer`。`xml_compact_transform.go` 注释提到的 `ConditionalWrapper` 类型不存在,实际条件逻辑在 `ClaudeCodeCompactTransform`。

---

## 5. XML 平坦化叙述(claude-code-compact 的产物)

`XMLCompactTransform` 把**所有历史消息**替换成**一条 assistant 文本**,内容是 `<analysis/>` + `<summary><conversation>...</conversation></summary>`。`<conversation>` 由 [`xml_builder.go`](../internal/smart_compact/xml_builder.go) 单遍、按原消息顺序拼出。

### 5.1 tool 处理(信息平坦化 + 关键 tool 保留)

单遍遍历,维护「`tool_use_id → result 文本`」索引。每个 assistant turn 内:

1. 输出 assistant 文本(仅 `OfText`,**不含** tool_result)。
2. 对该 turn 的每个 `tool_use`,按出现顺序:
   - **关键 tool**(命中 `keyToolsPreserve` 且值为 `true`):内联成
     ```
     <tool name="Task">请求(JSON)</tool>
     <tool_result>结论</tool_result>
     ```
     调用在前、结果紧随(POSITION ✓)。`is_error` 结论带 `[error]` 前缀。
   - **非关键 tool**:抽文件路径汇入**逐 turn** 的 `<tool_calls>`(ATTRIBUTION ✓,修掉旧的全局只输出一次 bug)。
3. user 消息:`tool_result` 文本**不再平铺进 `<user>`**,只通过对应 tool 的内联/摘要路径出现一次(FIDELITY ✓);真正的 user 文本照常输出 `<user>`。

### 5.2 关键 tool 白名单(内部固化,不透出用户侧)

`internal/smart_compact/xml_builder.go` 包级 `keyToolsPreserve` map,**map value 即开关**:`true` 内联保留,`false` 走非关键摘要。

**开启(`true`)** —— 这些 tool 的调用请求与结论是压缩后模型仍需知道、无法从环境重获的承重事实:

| Tool | 保留理由 |
|---|---|
| `Task` | subagent 结论,独立推理产物 |
| `AskUserQuestion` | 用户决策/选择,约束后续方向 |
| `WebFetch` | 外部抓取信息,昂贵/易变,不可重获 |

**列入但暂不开启(`false`)** —— 决策文档化,后续开启是一行翻转:

| Tool | 关闭理由 |
|---|---|
| `WebSearch` | 结果可重跑;`WebFetch` 已覆盖外部事实场景 |
| `TodoWrite`/`TaskCreate`/`TaskUpdate` | 意图/进度,CC 通常镜像进实时任务清单供重读;观测丢进度再开 |
| `Bash` | 输出可重跑;命令文本虽可能承重,但环境状态更适合后续 Read 重探 |
| `ExitPlanMode` | 批准的计划会镜像进 `TodoWrite`,被覆盖 |

未列入的 tool(`Read`/`Glob`/`Grep`/`LS`/`Edit`/`Write`/`NotebookEdit`/`Skill`/`Workflow`/MCP 等)按非关键处理——纯检索或执行,产物在文件系统/环境,摘要掉是对的。

### 5.3 触发与 gate

- `ClaudeCodeCompactTransform`(`vmodel/anthropic/cc_compact.go`):**仅当最后一条 user 消息含 "compact" 子串(忽略大小写)时压缩**。无「必须有 Tools」约束——纯文本长对话命中 `/compact` 也压缩。
- `XMLCompactTransform` 本身无 gate;`claude-code-strategy` 的 dedup/purge 无 gate、每请求跑。

> `smart_routing` 已能精确判定 CC 请求种类(`main/subagent/compact`,用系统提示词标记,见 [`internal/smart_routing/agent_detect.go`](../internal/smart_routing/agent_detect.go)),但压缩 gate **未使用**它,本期保持现状子串触发。

---

## 附:文件索引

- 概念:[`internal/protocol/round.go`](../internal/protocol/round.go)(Grouper/Round/RoundStats)
- 变换包:[`internal/smart_compact/`](../internal/smart_compact/)——核心为 [`xml_builder.go`](../internal/smart_compact/xml_builder.go)(平坦化叙述 + 关键 tool 白名单)
- CC 入口:[`vmodel/anthropic/cc_compact.go`](../vmodel/anthropic/cc_compact.go)、[`vmodel/anthropic/defaults.go`](../vmodel/anthropic/defaults.go)
- 代理接入:[`internal/server/protocol_transform.go`](../internal/server/protocol_transform.go)
- flag:[`internal/typ/type.go`](../internal/typ/type.go)、[`internal/server/config/flag_keys.go`](../internal/server/config/flag_keys.go)
- 链路:[`internal/protocol/transform/chain.go`](../internal/protocol/transform/chain.go)
- 分层约定:[`.design/transform-refactor-plan.md`](transform-refactor-plan.md)
