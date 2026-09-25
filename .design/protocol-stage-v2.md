# Protocol Stage v2 — 重新落地计划

> Status: **Plan**（尚未开始编码）。
> 前身：`origin/expr/protocol_stage`（78 commits，+22k）与
> `origin/feat/protocol-stage-hardening-port`（8 commits，把前者 port 到
> 2026-08-09 的 main）。两者的设计文档（`protocol-stage-chain.md`、
> `protocol-stage-tool-loop.md`、`protocol-recording-redesign.md`）仍保留在
> 那两个分支上，本文只引用，不复制。
>
> 本文回答三个问题：旧设计哪些**保留**、为什么当时**搁置**、现在按什么顺序用
> **堆叠分支**逐步落地。

---

## 1. 旧设计的核心（保留）

一句话：把"协议绑定的特性"（Guardrails、MCP/servertool 工具循环、recording）
从 handler 的各个 leaf 里抽出来，变成**有序的、全双工的 in-process 中间层**。

```text
client ─► HTTP Adapter ─► Ingress Bridge ─► Guardrail Stage ─► Tool Loop Stage ─► Provider Bridge ─► Provider Endpoint
                          (src ⇄ beta)      [anthropic_beta]   [anthropic_beta]    (beta ⇄ tgt)
          请求向内 ───────────────────────────────────────────────────────────────►
          ◄─────────────────────────────── complete 响应 / stream 事件 / 错误 / usage / side-effect 向外
```

保留的契约（`internal/protocol/stage`）：

| 概念 | 含义 |
|---|---|
| `Endpoint` | 一个具体协议的 `Complete` + `Stream`（pull 模型 `EventStream.Next(ctx)`） |
| `Stage` | 同协议的具名 wrapper：`Wrap(next Endpoint) Endpoint` |
| `Bridge` / `BridgeSession` | 两协议之间的**双向**适配；per-call session 持有转换状态 |
| `BuildTopology` | 从 provider 往外组装，相邻协议不同时自动插 Bridge；缺能力则构建失败 |
| `Response` / `StreamResult` | usage、model、`SideEffectsCommitted` 走结构化结果，不走 gin ctx |

保留的不变式：Stage 不碰 Gin / 不写 SSE；只有 HTTP Adapter 驱动最外层 stream
并提交字节；per-call 状态；协议变化只发生在具名 Bridge；Stage 不做重试，只报告
"输出已提交 / 副作用已提交"两个边界，由 failover 决定能否换 provider。

保留的关键决策：Tool Loop 和 Guardrail 的工作协议选 `anthropic_beta`（不造中立
AST）；MCP 与 servertool 不拆成两个 Stage（它们争夺同一个 tool_use 循环）；
Beta 流式工具循环必须缓冲一轮（可见前缀无法证明本轮没有内部 tool_use）。

---

## 2. 为什么当时搁置（推断）

1. **做成了"第二套平行管线"**：`--stage` 进程级开关，12 条路由各有一份
   `protocol_stage_*.go` glue（~3k 行）与 legacy 并存。legacy 之后又演进了
   556 个 commit（protocolserver 拆包、recording 重做、Responses 修复、thinking
   ladder、错误脱敏……），stage 路径需要双份追平，默认值始终没翻过来。
2. **一次性范围太大**：路由、Guardrail、Tool Loop、recording 新模型、harness 矩阵
   同时推进，单个分支 +22k，难以 review，也难以局部合入。
3. **自带一套 recording（`internal/record`）**，与 main 后来的
   `internal/recording`（capture-point + rule flag，#1644/#1656/#1649）方向分叉。

v2 的改进方向都是针对这三点。

---

## 3. main 现状（2026-09-25）与旧分支的差距

**已经在 main 上的（旧分支里可独立合入的部分已合）**：
- 所有 transport-neutral 转换器：`nonstream.Convert*`、`stream.New*Converter`、
  typed `wire.*`、`assembler.NewStreamAssembler`（4721cf6c、#1503）。
- `StreamConverter{ Next(); Usage() }` pull 模型（`.design/stream-converter-pipeline.md`），
  转换器已不写 SSE —— Bridge 可以做得很薄。

**main 上没有的**：`internal/protocol/stage`、Bridge 注册表、Provider Endpoint、
统一 HTTP Adapter、Stage 化 Guardrail / Tool Loop、side-effect 提交边界。
`internal/mcpserver` 已更名 `internal/toolengine`（#1782）。

**main 当前的结构性问题**（Stage 化后"按构造"消失的那一类）：

| # | 问题 | 位置 |
|---|---|---|
| P1 | 协议对分发是两层手写 `switch`（Target → Source → stream?），~25 个 leaf 函数各自重复 forward → 错误处理 → usage → 转换 → affinity → record → 写出 | `protocol_dispatch.go`、`protocol_cross.go`、`protocol_passthrough.go` |
| P2 | 4 个入口各自复制 prologue 与 `switch provider.APIStyle` 目标选择 | `anthropic_message.go`、`openai_chat.go`、`openai_responses.go` |
| P3 | 三套 MCP 循环：toolengine generic loop、OpenAI→Anthropic `ErrMCPStreamContinue` 循环、Anthropic→OpenAI hooks | `internal/toolengine`、`openai_mcp.go`、`mcp_stream_anthropic_to_openai.go` |
| P4 | Guardrails 仅 Anthropic passthrough / generic MCP 路径生效，cross-protocol 路径没有 response 检查 | `guardrails_runtime_ai.go` 调用点 |
| P5 | recording 覆盖不均：`streamOpenAIChatToResponses`、`streamAnthropicBetaToResponses`、`DispatchGenericOpenAIChatNonStream` 不记录；`upstream_response` capture point 无实现 | `.design/recording.md` §3.5 |
| P6 | toolengine interceptor 绕开 `HandleContext`，不调 `CommitFirstChunk`（多 service rule 下 V1 流式可能被 failover gate 缓冲到结束 —— 待验证） | `generic_stream_interceptor.go` |
| P7 | 已确认的小 bug：Chat→Google 可被选中但 `dispatchGoogle` 无 Chat 分支（静默不写响应）；Responses 入口从不设置 `reqCtx.ResponseModel` | `openai_chat.go:224` + `protocol_dispatch.go:485`；`openai_responses.go:159,253` |

---

## 4. v2 的改进原则

1. **收敛，而不是并行。** 不再有进程级 `--stage`。每个协议对是一个迁移单元：
   Bridge + parity 测试就位后，`DispatchChainResult` 对该 pair 直接走 Stage，
   对应 legacy leaf **在同一分支或紧随的下一个分支里删除**。同一时间只有一条
   生产路径需要维护，legacy 的后续修复不必双写。
   回滚手段是 git revert 单个小分支，而不是运行时开关（UX 原则：消解模式选择）。
2. **分发改为注册表驱动。** `(source, target) → Route{Bridges, Capabilities}` 取代
   两层 switch；未注册的 pair 在注册表里显式标记（例如 Chat→Google 变成明确的
   "unsupported" 错误而不是静默）。
3. **复用 main 的 recording，不引入 `internal/record`。** Stage 链上的两个天然位置
   ——provider Endpoint 装饰器、HTTP Adapter 出口——正好对应 recording.md 里未实现的
   `upstream_response` / `final_response` capture point。Stage 只是让它们有地方挂。
4. **transform chain 按协议边界切开，而不是整体包成 Stage。** 旧分支把 chain 包成
   `client_prepare` / `provider_finalize` 两个 Stage；v2 进一步明确：
   - preBase rule transforms → `client_prepare`（源协议 Stage）
   - `BaseTransform`（协议转换）→ **由 Bridge 取代**
   - MCP 注入 / strip-guard → 移入 Tool Loop Stage
   - Consistency + preVendor + Vendor → `provider_finalize`（目标协议 Stage）
   - recording StagePre/Post → 观察点，不再是 transform
5. **先 plain 路径，再特性。** 无 MCP、无 Guardrail 的请求先全部迁完；特性 Stage
   后加，届时它们天然覆盖所有 pair（解决 P4/P5），且只在工作协议 Beta 上实现一次。
6. **每个分支可独立 review、可独立 revert，每步都绿。** 目标单分支 ≤ ~1.5k 行
   非测试代码。

---

## 5. 堆叠分支计划

每一层基于上一层；括号内是主要来源（可从 hardening-port 分支搬运后按 main 签名调整）。

| # | 分支（建议名） | 内容 | 生产流量 |
|---|---|---|---|
| 0 | `fix/protocol-dispatch-gaps` | 独立小修：P7 两个 bug（Chat→Google 显式报错或补齐；Responses 设置 `ResponseModel`）。不依赖 Stage，可先合 | 修 bug |
| 1 | `stage/1-core` | `internal/protocol/stage`：Endpoint / Stage / Bridge / Compose / Registry / Topology / identity + 单测（来源：`stage/*.go`，~740 行）。v2 调整：`Call.State` 保持显式字段；删 reflect `isNil`；`APIType` 用 main 的 `protocol.APIType` | 无 |
| 2 | `stage/2-bridges` | 具体 Bridge，**只包装 main 现有转换器**：Beta⇄Chat、Beta⇄Responses、Chat⇄Responses、V1→Beta/Chat/Responses；in-memory bridge 矩阵测试（来源：`anthropicbridge/`、`openaibridge/`、`responsesbridge/`，~2k 行） | 无 |
| 3 | `stage/3-edges` | 两端：Provider Endpoint（包 `forwarding.Forward*`，含 stream prime 以保 pre-stream failover）；每个客户端协议一个 HTTP Adapter（JSON/SSE 写出、`RunLoop` + `CommitFirstChunk`、public model 改写、usage tracking、affinity、recording 出口）。`transform` 链切为 `client_prepare` / `provider_finalize` Stage。httptest 单测 | 无 |
| 4 | `stage/4-identity-routes` | 第一次切流：Chat→Chat、Responses→Responses、Beta→Beta 的 **plain（无 MCP / Guardrail）** 请求走注册表 + Stage；删除对应 passthrough leaf。protocoltest 矩阵作为 parity 证据 | 3 个同协议 pair |
| 5 | `stage/5-cross-routes` | 跨协议 plain 路径：Chat⇄Beta、Chat⇄Responses、Beta→Responses、Responses→Beta（可按需拆成 5a/5b）；删对应 `protocol_cross.go` / `protocol_dispatch.go` leaf。V1 目标与 Google 暂留 legacy | 跨协议 plain |
| 6 | `stage/6-recording` | provider Endpoint 观察者装饰器 → 实现 `upstream_response`（含多轮/failover 多次 exchange）；HTTP Adapter 出口 → `final_response`。接入现有 `ProtocolRecorder` 与 rule flag，不新增存储格式 | recording 覆盖补齐 |
| 7 | `stage/7-guardrail` | Beta-native Guardrail Stage（来源：`stage/guardrail/anthropic_beta.go`），替换 Anthropic 路径的 hook 式 guardrails。是否扩到 OpenAI 入口是**产品决策**，本分支只保持现有 scenario gate 语义不变，另开分支决定 | Anthropic 入口 guardrails |
| 8 | `stage/8-toolloop` | Beta-native Tool Loop Stage 取代三套 MCP 循环（P3）。带上 hardening 分支的修复：`CanStash` fail-closed、`maxRounds` 语义对齐 legacy、`Dispatched` 标记副作用、trailing-assistant merge。failover 读 `SideEffectsCommitted` | MCP 路径 |
| 9 | `stage/9-legacy-removal` | V1 目标、Google、Codex assemble 迁移或显式保留；删除 `HandleContext` stream hooks、`AttachGuardrailsHooks`、`ErrMCPStreamContinue`、toolengine `FormatAdapter.SendEvent` 等遗留机制；统一 4 个入口 prologue（P2） | 收尾 |

依赖关系：0 独立；1→2→3→4→5 为主干；6、7 只依赖 5（可并行开发，合入时按序 rebase）；
8 依赖 7（Guardrail 必须位于 Tool Loop 外层）；9 最后。

---

## 6. 每层的验收标准

- `go build ./... && go vet ./...` 与改动包单测通过；切流分支额外要求
  `go test ./internal/protocoltest` 全绿，且对被删除的 leaf 有等价用例覆盖。
- 切流分支在 PR 描述中列出：迁移的 pair、删除的 legacy 函数、行为差异（如有，
  必须是有意的并写明理由——例如 recording 从"不记录"变为"记录"）。
- 流式：首个客户端可见事件才提交；pre-stream 错误仍可 failover；
  Tool Loop 执行过 server 工具后不再 failover。
- `X-Tingly-Debug-Routing: 1` 输出实际 Stage 链（如
  `openai_chat → anthropic_beta → guardrail → tool_loop → openai_responses → provider`），
  满足"诊断必须走真实链路"。

---

## 7. 待讨论

1. 第 4 步是否需要一个**仅内部**的逃生口（例如 env `TINGLY_PROTOCOL_LEGACY=1`）保留一个
   release？本文倾向不要——分支足够小，revert 即回滚。
2. Chat→Google：补齐（需要 Google→Chat 转换器）还是显式不支持？
3. OpenAI 入口是否启用 Guardrails（第 7 步之后的产品决策）。
4. harness 矩阵（`cli/harness matrix --stage`）：v2 没有 `--stage`，矩阵直接跑生产链路即可，
   旧分支的 `bridge_matrix.go` 可在第 2 步作为 in-memory 测试搬入。
