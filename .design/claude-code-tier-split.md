# Claude Code：统一配置 + 单独拆出某个 Tier（以 Subagent 为典型）

> 状态：**设计提案，未实施**。
> 背景文档：`claude-code-config.md`（env 线形、规则 ↔ slot 映射）、`ux-principles.md`（P1–P12）。

---

## 1. 问题

Claude Code 目前只有两种模式，由 scenario flag `unified` / `separate` 二选一：

| 模式 | 规则 | env |
|---|---|---|
| Unified | 只有 `builtin:claude_code:cc` 生效 | 6 个 slot 全部写 `tingly/cc` |
| Separate | `default/haiku/sonnet/opus/fable/subagent` 6 条规则生效，`cc` 被停用 | 每个 slot 各写自己的 `tingly/cc-<tier>` |

实际使用中最常见的诉求是第三种：**绝大多数 tier 统一走一个模型，只有 subagent 要单独分出去**（换便宜/快的模型跑 Task 子任务，或者反过来给 subagent 上更强的模型）。现在只能：

1. 切 Separate，然后把 5 条规则配成一模一样，只改 subagent —— 改一次主模型要改 5 处，极易漂移；
2. 在 `cc` 规则上写 smart routing `agent.claude_code == subagent` —— 能用，但它是 prompt 指纹启发式（`agent_detect.go`），藏在高级功能里，env 里看不出来，也不进 tier 视图。

根因是**模型错了**（P4）：一个开关同时在控制"有几条规则"和"每个 tier 走哪条规则"两件事。"统一"和"分开"并不是两种模式，而是同一个问题的两个极端：

> **每个 tier 是跟随主规则，还是有自己的规则？**

---

## 2. 核心模型：主规则 + 按 tier 拆出

```
           ┌─ default  ── 跟随 ─┐
           ├─ haiku    ── 跟随 ─┤
tier slot ─┼─ sonnet   ── 跟随 ─┼──▶  主规则 builtin:claude_code:cc   (tingly/cc)
           ├─ opus     ── 跟随 ─┤
           ├─ fable    ── 跟随 ─┘
           └─ subagent ── 拆出 ────▶  builtin:claude_code:subagent   (tingly/cc-subagent)
```

- **主规则**：始终是 `cc`。它是所有"跟随"tier 的落点。
- **拆出（split）**：某个 tier 拥有并使用自己的规则 `builtin:claude_code:<tier>`。
- **跟随（follow）**：tier 没有被拆出，请求落到主规则。

于是：

| 旧概念 | 新模型下的表达 |
|---|---|
| Unified | 0 个 tier 拆出 |
| Separate | 6 个 tier 全部拆出（主规则此时只是兜底，没有 tier 落到它） |
| **新：Unified + Subagent** | 只拆出 `subagent` |

旧的两个模式退化为两个预设，不再是需要先选的"模式"（P2）。

### 2.1 真相源：用 tier 规则的 `Active` 表达"是否拆出"

不新增 flag 字段，**tier 规则 Active = 拆出，Inactive = 跟随主规则**。

理由：

- 现有代码已经在用这个语义的一半：Separate 模式下 fable 规则不存在/停用时，`GenerateCCEnv` 和 `derivePrefsFromRules` 都让 fable 回落到 default。这里只是把"回落目标"统一成主规则，并推广到所有 tier。
- 规则表本身就是用户看得见的东西；另起一个 `split_tiers` 列表会和 `Active` 形成两个真相源（`claude-code-config.md` §5.6 已经踩过"两个表面都能写同一件事"的坑）。
- `setClaudeCodeModeRulesActiveLocked` 已经是"按模式批量设置 Active"，只需要拆成"按 tier 设置 Active"。

`scenario.flags.unified / separate` 保留但降级为**派生值**（读取时由规则状态算出：0 拆出 → unified，全拆出 → separate，其他 → 都为 false），仅为兼容旧前端 / CLI / TUI quickstart 读取。写入这两个 flag 等价于"全部合并 / 全部拆出"。

> 备选：在 `ScenarioConfig.Extensions` 里存 `cc_split_tiers: ["subagent"]`。优点是"停用规则"和"合并回主规则"语义分离；缺点是双真相源。若评审认为"临时停用某条 tier 规则"是一个必须保留的独立操作，再退回这个方案。

---

## 3. 各层改动

### 3.1 配置层（`internal/config`）

- 新增 `SetClaudeCodeTierSplit(scenario, tier string, split bool)`：
  - `split=true`：确保 `builtin:<scenario>:<tier>` 存在（不存在则按 `newCCProfileRules` 的模板创建），**首次创建时从主规则复制 services / flags / LB tactic**，这样拆出的瞬间行为不变，用户再去改 subagent 的模型即可；置 Active。
  - `split=false`：置 Inactive（保留规则内容，再次拆出时恢复原配置 —— P10 可逆）。
  - 主规则 `cc` 永远保持 Active（Separate 预设下也一样，作为兜底）。
- `syncClaudeCodeRuleModeLocked` / `SetScenarioFlag(unified|separate)` 改为调用上面的函数批量操作，行为与现在一致，只是不再停用 `cc`。
- 迁移：现有 Separate 配置把 `cc` 重新置 Active（它只是兜底，不会抢请求，因为每个 tier 都有自己的 request_model）。

### 3.2 env 生成（`GenerateCCEnv` / `derivePrefsFromRules`）

统一为一个规则，替代现在的 `if unified {...} else {...}`：

```go
main := tierModel("cc", RuleUUIDBuiltinCC, "tingly/cc")
for tier, envKey := range tierEnvKeys {           // default/haiku/sonnet/opus/fable/subagent
    env[envKey] = ruleModel(main, BuiltinRuleUUID(scenario, tier))  // 拆出→自己的 request_model；跟随→主规则
}
```

`ruleModel` 已经只取 Active 规则，所以跟随的 tier 自然落到 `main`。前端 `derivePrefsFromRules` 做同样的事，删除 `mode` 参数。1M 后缀按"该 tier 最终落到的那条规则"的 flag 计算，逻辑不变。

`ClaudeCodeTiersFromEnv`（desk 的 tier 选择）不用改：它本来就是按 env 值是否相同来判断 unified，混合状态下会正确地列出 `default` 和 `subagent` 两个不同模型。

### 3.3 路由兜底（`protocol_handler.go` 的规则查找）

现状：命中的规则 `!rule.Active` 时直接报 `provider or model not configured`。这意味着用户"合并回主规则"后，如果没重新 Apply，旧 `settings.json` 里的 `tingly/cc-subagent` 会直接 404。

改为：**claude_code 场景（含 profile）下，命中一条停用的 tier 规则时，回落到该 scenario 的主规则 `cc`**。合并方向因此无需重新 Apply 就立即生效；只有"拆出"方向需要 env 变化（env 必须写出不同的 model 名，Claude Code 才会发出可区分的请求）。

### 3.4 Profile

`ProfileMeta.Unified` 目前在创建时固定、且"切换模式需删除重建"（`UpdateProfile` 注释）。新模型下这条限制可以去掉：

- profile 统一创建 `cc` 规则；`Unified` 字段同样降级为派生值。
- profile 页面的 Model Rules 区使用同一个"拆出 / 合并"交互，调用同一个 `SetClaudeCodeTierSplit(claude_code:<pid>, tier, …)`。
- 已有的 separate profile（只有 tier 规则、没有 `cc`）：迁移时补一条 `cc` 规则，内容复制 `default` 规则 —— 行为不变。

### 3.5 API

```
PUT /api/v1/scenario/:scenario/tiers/:tier/split   { "split": true }
```

返回更新后的 tier 映射（每个 tier → 落到的 rule uuid + request_model），前端直接用它刷新，不再单独 GET 规则列表再按模式过滤（`useSlotRouting` 的两套加载逻辑合并）。按项目约定先定义 Go model + swagger，再 `task codegen`。

---

## 4. UI

### 4.1 Model Rules 区（Claude Code 页 / profile 页）

去掉 Unified / Separate 切换 + 确认弹窗，改为以主规则为中心的一张图（P1：用户在问的是"我每种请求走哪个模型"）：

```
┌ 主规则  tingly/cc ─────────────────────────────── [规则卡片：services…] ┐
│ 跟随此规则：  default · haiku · sonnet · opus · fable       [拆出 ▾]     │
└──────────────────────────────────────────────────────────────────────────┘
┌ Subagent  tingly/cc-subagent ─────────── [规则卡片：services…] [合并回主规则] ┐
└──────────────────────────────────────────────────────────────────────────┘
```

- 每个 tier 以真实的 env 值展示（P5：`tingly/cc-subagent`，而不是"subagent 模式"）。
- "拆出 ▾" 菜单列出仍在跟随的 tier；**subagent 放第一位**（P6：覆盖最常见的场景）。菜单底部保留"全部拆出 / 全部合并"作为旧两种模式的快捷预设。
- 拆出后新卡片直接展开并聚焦到它的模型选择（P11：把下一步要操作的东西递到手上）。
- 合并回主规则不弹确认（可逆，规则内容保留，P10）；"全部合并"才确认。

### 4.2 Auto Config 弹窗 & Apply 提示

- 5（6）个 model slot 行为不变，值来自新的 `derivePrefsFromRules`，跟随的 slot 显示一个淡化的"跟随主规则"标注。
- **拆出操作会改变 env**，所以拆出成功后在同一个视野内出现 Apply 提示（复用 `Context1MChangeBanner` 的模式："Subagent 已拆出，需重新应用 Claude Code 配置后生效 [Apply]"）。合并方向因为有 §3.3 的路由兜底，不需要提示。

---

## 5. 与 smart routing `agent.claude_code == subagent` 的关系

两者并存，各管一件事：

| | tier 拆出（本设计） | smart routing 指纹 |
|---|---|---|
| 依据 | Claude Code 发出的 model 名（`CLAUDE_CODE_SUBAGENT_MODEL`） | system prompt 启发式 |
| 确定性 | 确定 | 依赖 Claude Code 版本的 prompt 文本 |
| 可见性 | env、tier 视图、desk 都能看到 | 只在规则的 smart routing 里 |
| 适用 | "subagent 用另一个模型" 这一主路径 | 区分 compact、或旧版 Claude Code 不认 env 时的兜底 |

文档和 smart rule catalog 里在 `agent.claude_code` 处加一句提示：只想把 subagent 分出去时，优先用"拆出 Subagent"。

---

## 6. 实施顺序

1. 配置层 `SetClaudeCodeTierSplit` + 迁移（`cc` 常驻 Active、separate profile 补 `cc`）+ 单测。
2. `GenerateCCEnv` / `derivePrefsFromRules` 改为"tier → 自己 or 主规则"，删掉 mode 分支；更新 `cc_settings` 与 `claudeCodePrefsState` 的测试。
3. 路由兜底（停用 tier 规则 → 主规则）。
4. API + swagger + `task codegen`。
5. 前端 Model Rules 区 & profile 页交互，mock handler，ui-preview 截图。
6. 更新 `claude-code-config.md` §5 的映射表与本文件状态。

## 7. 待确认的问题

- "停用一条 tier 规则" 和 "合并回主规则" 合并成同一个动作是否可接受（§2.1 备选方案）。
- Separate 预设下 `cc` 保持 Active 作为兜底，会让 `/v1/models` 多列出一个 `tingly/cc`，是否需要在列表中隐藏没有 tier 落到的主规则。
- TUI quickstart 的 "Use unified mode?" 是否改为 "Split subagent to its own model?" 三选一（统一 / 统一 + subagent / 全部分开）。
