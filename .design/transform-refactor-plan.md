# protocol/transform 重构规划

> 目标：把 `internal/protocol/transform/` 收敛到统一的 **thin Transform shell + ops 原语** 模式，
> 让所有协议变换都遵循同一个抽象层级，便于复用、单测和扩展。

---

## 1. 现状盘点

### 1.1 已经"干净"的 Transform（thin shell + ops 委托）✅

| 文件 | 行数 | ops 调用 |
|------|------|---------|
| `tool_block.go` | ~70 | `ops.ApplyToolBlock*`（5 个 shape）|
| `claude_code_compat.go` | ~35 | `ops.ApplyClaudeCodeCompatRoleRewrite` / `Beta` |
| `openai_cursor_compat.go` | ~40 | `ops.ApplyCursorCompatContentNormalization` |
| `openai_max_tokens_rewrite.go` | ~50 | `ops.ApplyMax{Completion,}TokensRewrite` |

**模式特征**：构造接受配置 → `Apply()` type-switch ctx.Request → 调对应 op。Transform 本身无业务逻辑。

### 1.2 内联实现的 Transform（业务逻辑直接在 Apply 里）⚠️

| 文件 | 行数 | 痛点 |
|------|------|------|
| `base.go` | 305 | 协议转换大开关；监听 + 转换混在一起，没法独立单测某个方向 |
| `consistency.go` | 711 | 16 个内联方法（4 shape × {tool schema / scenario flag / message / validate}）|
| `rule_thinking.go` | 144 | thinking budget 缩放、disable/enable 逻辑全内联，但已 export 给 server-domain 复用 |
| `vendor.go` | 279 | 大部分委托给 ops，但 Responses 路径仍有内联（Codex 调用前的字段处理）|

### 1.3 Server-domain Transform（`internal/server/transform_*.go`）⚠️

| 文件 | 痛点 |
|------|------|
| `transform_max_tokens.go` | inline：max_allowed 上限 + thinking budget 联动缩放，逻辑可拆 op |
| `transform_clean_header.go` | inline：从 system messages 中过滤 billing header 块 |
| `transform_thinking_mode.go` | 已是薄壳——委托给 `protocoltransform.ApplyThinkingEffort`，无需重构 |
| `transform_mcp_*.go`、`transform_native_websearch_strip.go` | inline，依赖 `*runtime.Runtime`；不容易完全 ops 化（运行时副作用） |

---

## 2. 重构目标与原则

### 2.1 目标分级

1. **L1（必做）**：把"纯字段改写"类逻辑从 `consistency.go` / `base.go` / `vendor.go` 抽到 ops，达到与 `tool_block` 同样的薄壳化程度。
2. **L2（推荐）**：把 server-domain Transform 中可纯函数化的部分抽到 ops，server-domain 文件只保留 "组合 op + 读 ScenarioConfig" 的胶水。
3. **L3（可选）**：`base.go` 的协议转换是大块逻辑，**不强制拆 op**——它本质上是状态机式的双向映射，强行拆反而碎片化。但可以拆出**字段级辅助函数**（thinking 解析、stop sequences 映射等）放到独立文件，让主开关读起来更清晰。

### 2.2 设计原则

**Transform 文件**（`transform/` 包）：实现 Transform 接口；构造期接受配置；`Apply()` 里 type-switch `ctx.Request`；是唯一感知 `ctx` / 链路位置的层。

**纯函数辅助**：mutation 逻辑不需要 `ctx`、`*typ.ScenarioConfig`、`*runtime.Runtime` 时，写成 package-level 函数。放置位置由**复用需求**决定：

```
需要被 server-domain 或其他包复用  →  ops/ 包（ApplyXxx 导出函数）
仅被本 Transform 文件使用          →  同文件内的 unexported 函数
```

**判断边界**：
- 函数只被一个 Transform 调用 → 放在同文件（unexported）。
- 函数被 `internal/server/` 或 2+ 个 Transform 调用 → 放到 `ops/`（exported）。

这条原则避免了"每个 Transform 都配一个 ops 文件"的无意义间接层：`ops/` 是真正的对外共享 API 层，不是 Transform 的内部实现存放处。

---

## 3. 分阶段重构方案

### Phase 1：拆分 `consistency.go`

**问题**：711 行的单文件聚集了 4 个 shape × 4 类操作（tool schema / scenario flag / message align / validate），新加一个 shape 或一类校验要在 4 处改。

**目标拆分**：

```
internal/protocol/transform/
  consistency.go                  ← 仍是 Transform 入口；Apply() 按 ctx.TargetAPI dispatch
  consistency_openai_chat.go      ← 4 个方法移入：normalize/scenario_flag/align/validate
  consistency_openai_responses.go ← 同
  consistency_anthropic_v1.go     ← 同
  consistency_anthropic_beta.go   ← 同

internal/protocol/transform/ops/
  consistency_tool_schema.go      ← per-shape 的 tool schema 归一化（4 个 op）
  consistency_message_align.go    ← AlignToolMessagesForOpenAI 已 export，移过来
  consistency_validate.go         ← validate*，返回 *ValidationError
```

**关键决策**：
- 不强行让每个 normalize 都拆 op——只有可以**纯函数化**（不读 `*typ.ScenarioConfig` 之外的状态）的拆出去。
- `applyScenarioFlags` 系列**保留**在 Transform 文件里（需要读 scenario flag），但内部调用的 mutation 部分尽量走 op。
- `ValidationError` 类型保留在 `consistency.go` 顶层（跨文件共用）。

**收益**：单个文件 100~200 行内；新增 shape 只动一个文件；validate / tool schema 等可被 server-domain 复用。

---

### Phase 2：`vendor.go` 完全 ops 化

**问题**：`vendor.go` 主体已委托给 `ops.ApplyProviderTransforms` 等，但 Responses 路径下有内联的 Codex 字段处理。

**目标**：
- 把 Responses 路径的内联部分移到 `ops/request_openai_codex.go`（该文件已存在）。
- `vendor.go` 只剩 type-switch + ops 调用，目标行数 < 100。

**风险**：低；本质是把已有内联块挪个位置。

---

### Phase 3：`base.go` 字段辅助拆分

**问题**：协议转换主体（Anthropic↔OpenAI↔Responses↔Google）的大开关**不该拆 op**——它是状态机式的多向映射，强拆反而碎。但里面混了字段级解析逻辑（thinking config 解析、stop sequences 映射、tool 系统消息拼接等）。

**目标**：
- 抽出 `base_thinking.go`、`base_stop.go`、`base_tool_system.go` 等**辅助文件**到 `protocol/transform/` 下（不进 ops，因为它们是协议转换的内部细节，不对外复用）。
- 主 `base.go` 只保留转换骨架 + 调辅助函数。
- 目标：`base.go` 从 305 行降到 ~150 行。

**风险**：中；要小心 thinking 字段在 Anthropic→OpenAI 方向的多次重赋值，拆分时要保持顺序。先用 `e2e_test.go` 锁住行为。

---

### Phase 4：`rule_thinking.go` 整理

**现状**：已经 export 了 `ApplyThinkingEffort` 等给 server-domain 复用，模式正确，**只缺一个 ops 文件位置**。

**目标**：
- 把 `disableThinking` / `enableThinking` / `stripOpenAIThinkingExtra` 三个方法从 receiver 方法改为 `ops/request_thinking.go` 里的纯函数。
- `RuleThinkingTransform` 变成纯薄壳：`Apply()` → type-switch → 调 ops。
- server-domain 的 `ThinkingEffortTransform` 也直接调 ops（不再需要 import protocol/transform 包级 helper）。

**收益**：消除 protocol/transform 包级 export 与 server-domain 包之间的"间接复用"耦合。

---

### Phase 5：Server-domain 局部 ops 化（可选）

**目标**：
- `transform_clean_header.go` → 抽 `ops.ApplyCleanBillingHeader{V1,Beta}` 纯函数。
- `transform_max_tokens.go` → 抽 `ops.ApplyMaxTokensBounding{V1,Beta}(req, defaultMax, allowedMax, isStreaming)` 纯函数；Transform 本体读 `ScenarioConfig` 后传参。
- MCP 系列**不拆**——`*runtime.Runtime` 是运行时状态，op 没法纯函数化。

**收益**：scenario 级的核心字段改写也能复用、独立测试。

---

## 4. 不在范围内的事

- **不重命名** Transform 接口或 `chain.go` 的结构——抽象本身没问题。
- **不合并** `protocol/transform/` 和 `internal/server/transform_*.go` 两个包——分层（SDK-only vs server-domain）有意义，不打破。
- **不为 MCP / runtime 依赖** 的 Transform 强行 ops 化——纯函数语义不成立。
- **不引入新 Transform 类型**（如"OpTransform 自动包装"）——抽象层级现在已经足够，再加一层 generic shell 反而绕。

---

## 5. 推进顺序与里程碑

| 阶段 | 风险 | 工作量（人日） | 顺序原因 |
|------|------|---------------|---------|
| Phase 2 | 低 | 0.5 | 暖身；验证 ops 抽取流程 |
| Phase 4 | 低 | 0.5 | 解决跨包间接复用 |
| Phase 1 | 中 | 2~3 | 收益最大但工作量最大；先做 2/4 树立模板 |
| Phase 3 | 中 | 1~2 | 独立模块；不阻塞其他阶段 |
| Phase 5 | 低 | 1 | 可选；做不做都不影响 protocol 层 |

每个阶段独立可合并、不互相阻塞。建议每阶段一个 PR。

---

## 6. 验收标准

- **单测**：每个新 op 至少 1 个 case；`fullchain_test.go` / `e2e_test.go` 保持绿。
- **行数**：`consistency.go` ≤ 150 行（只剩 dispatch）；`base.go` ≤ 200 行；每个 shape 的 consistency 文件 ≤ 200 行。
- **依赖方向**：`ops/` 包**不 import** `internal/typ` 之外的 server 包；`protocol/transform/` 不 import `internal/server/...`。go vet + 视线检查双重保证。
- **重构无行为变化**：所有现存 `*_test.go` 不需要修改即可通过。新增 op 单测覆盖新拆出的纯函数。
