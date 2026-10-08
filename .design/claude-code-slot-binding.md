# Claude Code：槽位绑定（Slot Binding）

> 状态：**已实施**（路由层不变：rule 即 request，`request_model` 仍是路由键；本设计只处理 Claude Code 特有的槽位 → 规则映射）。

![槽位表](images/claude-code-slots.png)
> 背景文档：`claude-code-config.md`（env 线形、规则 ↔ slot 映射）、`ux-principles.md`（P1–P12）。

---

## 1. 问题：槽位只能"全绑一条"或"各绑各的"

Claude Code 通过 6 个 env 槽位决定每类请求发出的 model 名：

| 槽位 | env |
|---|---|
| default | `ANTHROPIC_MODEL` |
| haiku | `ANTHROPIC_DEFAULT_HAIKU_MODEL` |
| sonnet | `ANTHROPIC_DEFAULT_SONNET_MODEL` |
| opus | `ANTHROPIC_DEFAULT_OPUS_MODEL` |
| fable | `ANTHROPIC_DEFAULT_FABLE_MODEL` |
| subagent | `CLAUDE_CODE_SUBAGENT_MODEL` |

tb 用一个全局开关 `unified / separate` 决定槽位 → 规则的映射，只有两种形状：

- **Unified**：6 个槽位全部 → `cc`
- **Separate**：6 个槽位各自 → `cc-<slot>`

真实需求是两者之间的任意形状，例如：

- 全部走 `cc`，只有 **subagent** 走便宜模型；
- **haiku + subagent** 共用一个快模型，其余走主模型；
- **opus** 单独上最强模型，其余统一。

现在只能切 Separate 再把 N 条规则配成一样 —— 改一次主模型要改 N 处，必然漂移。

根因（P4）：一个开关控制了"每个槽位绑哪条规则"这 6 个独立的轴。**正确的模型是：每个槽位独立绑定一条规则。** Unified / Separate 只是这张表的两种特殊取值。

---

## 2. 核心模型

```
槽位绑定表（per scenario）            规则（per scenario）
  default  ─┐
  sonnet   ─┼──────────────▶  cc            request_model = tingly/cc
  opus     ─┤
  fable    ─┘
  haiku    ─┬──────────────▶  cc-fast       request_model = tingly/cc-fast
  subagent ─┘
```

- **槽位绑定**是一张 `slot → rule UUID` 的表，多对一。
- **未绑定的槽位落到 default 槽位的规则**；default 未绑定时落到 `cc`。所以空表 = 现在的 Unified。
- **env 值就是绑定规则的 `request_model`**（加 1M 后缀）。这沿用 `claude-code-config.md` §5.4 的约定 ——
  `request_model` 就是路由键 —— **路由层零改动**。
- 规则与槽位解耦：规则只管"这个 request_model 走哪些 service"，槽位绑定只管"哪个槽位用哪条规则"。
  规则的 `Active` 不再被模式切换批量改写，回归它本来的含义（这条规则是否启用）。

| 旧概念 | 绑定表 |
|---|---|
| Unified | `{}`（全部落到 `cc`） |
| Separate | `{default: cc-default, haiku: cc-haiku, …, subagent: cc-subagent}` |
| 统一 + subagent 单独 | `{subagent: cc-subagent}` |
| haiku + subagent 共用 | `{haiku: cc-fast, subagent: cc-fast}` |

### 2.1 存储

`ScenarioConfig` 新增类型化字段（不放 `Extensions`，避免 `interface{}` 反序列化与 GET-merge 丢字段问题）：

```go
type ScenarioConfig struct {
    // ...
    // SlotBindings maps a client model slot (default/haiku/sonnet/opus/fable/subagent)
    // to the UUID of the rule it requests. Missing slots follow the default slot.
    SlotBindings map[string]string `json:"slot_bindings,omitempty" yaml:"slot_bindings,omitempty"`
}
```

- **不做 profile → base 回落**：`scenarioConfigLocked` 对 profile 会回落到 base 配置，但绑定里是规则 UUID，跨 scenario 无意义。读取绑定只取 exact-match 的 scenario 配置。
- 绑定的规则必须属于同一 scenario；规则被删除 / 停用时，该槽位视为未绑定（落到 default 槽位），UI 上对这个槽位给出提示。
- `flags.unified / separate` 降级为**派生值**（空表 → unified；6 个槽位绑定到 6 条互不相同的规则 → separate；其余都为 false），只为兼容旧前端 / CLI / TUI。写这两个 flag 等价于套用对应预设。
- `ProfileMeta.Unified` 同样派生；"profile 模式创建后固定" 的限制（`UpdateProfile` 注释）随之取消。

### 2.2 迁移（一次性）

| 现状 | 迁移后的绑定表 | 规则 Active |
|---|---|---|
| main scenario，unified | `{}` | 全部置 Active（不再靠停用来表达模式） |
| main scenario，separate | 每个槽位 → `builtin:claude_code:<slot>`；fable 规则不存在 / 停用时省略（落到 default，与现行为一致） | 全部置 Active |
| profile，`Unified=true` | `{}` | 不变 |
| profile，`Unified=false` | 每个槽位 → `builtin:claude_code:<pid>:<slot>`；补一条 `cc` 规则（复制 default 规则），使"取消绑定"有落点 | 不变 |

迁移前后生成的 env 逐字节相同 —— 测试以此为准。

---

## 3. 各层改动

### 3.1 配置层（`internal/config`）

- `GetSlotBindings(scenario)` / `SetSlotBinding(scenario, slot, ruleUUID string)`（空串 = 取消绑定）。
- `CreateSlotRule(scenario, slot)`："为这个槽位新建规则"：创建 `builtin:<scenario>:<slot>`（已存在则复用），**从该槽位当前生效的规则复制 services / flags / LB tactic**，再绑定。新建那一刻行为不变，用户随后只改模型。
- `syncClaudeCodeRuleModeLocked` / `setClaudeCodeModeRulesActiveLocked` 删除；`SetScenarioFlag(unified|separate)` 改为套用预设。

### 3.2 env 生成（`GenerateCCEnv` / 前端 `derivePrefsFromRules`）

删除 `if unified {...} else {...}` 两套分支，统一为：

```go
fallback := resolve(bindings["default"], ccRule)
for slot, envKey := range slotEnvKeys {
    env[envKey] = modelOf(resolve(bindings[slot], fallback)) // 1M 后缀按最终规则的 flag
}
```

前端 `derivePrefsFromRules` 去掉 `mode` 参数，改为接收 `bindings`；`AgentPage` 的 one-click apply 与弹窗共用。
`ClaudeCodeTiersFromEnv`（desk tier 选择）不用改：它按 env 值是否相同判断，混合绑定下会正确列出不同的模型。

### 3.3 API

```
GET /api/v1/scenario/:scenario/slots
PUT /api/v1/scenario/:scenario/slots/:slot   { "rule_uuid": "builtin:claude_code:subagent" }   // "" = 跟随 default
POST /api/v1/scenario/:scenario/slots/:slot/rule                                             // 为该槽位新建规则并绑定
```

返回值统一为解析后的槽位表：每个槽位 → `{ rule_uuid, request_model, context_1m, bound: bool }`。
前端用它渲染，不再按模式分两套规则加载（`useSlotRouting` 的 unified / separate 分支合并）。
按项目约定先定义 Go model + swagger，再 `task codegen`。

---

## 4. UI

### 4.1 Model Rules 区（Claude Code 页 / profile 页）

去掉 Unified / Separate 切换和确认弹窗（P2），在规则卡片上方放一张**槽位表**（P1：用户的问题是"每类请求走哪个模型"）：

```
槽位            发出的 model             规则
default         tingly/cc               [cc            ▾]
haiku           tingly/cc               跟随 default
sonnet          tingly/cc               跟随 default
opus            tingly/cc               跟随 default
fable           tingly/cc               跟随 default
subagent        tingly/cc-subagent      [cc-subagent   ▾]
                                         预设：全部统一 · 全部分开
```

- "发出的 model" 列展示真实 env 值（P5）。
- 规则下拉：同 scenario 的规则 + "跟随 default" + "为此槽位新建规则…"。
- 预设只有两个按钮，替代旧开关；"全部分开"会为缺失的槽位新建规则（复制当前生效规则）。
- 下方的规则卡片照常展示，每张卡片标注"被哪些槽位使用"；没有槽位使用的规则淡化显示，但不隐藏（可能被别的客户端直接请求）。
- 新建槽位规则后，对应规则卡片滚动到视野并展开模型选择（P11）。

### 4.2 Apply 提示

绑定变化会改变 env，Claude Code 只在启动时读 `settings.json`。绑定保存后，在槽位表下方出现
"槽位已变更，需重新应用 Claude Code 配置 [Apply]"（复用 `Context1MChangeBanner` 的模式）。
旧 env 里的 request_model 对应的规则仍然启用，所以在 Apply 之前旧会话继续按旧绑定工作，不会 404。

### 4.3 Auto Config 弹窗

6 个 model 行保持不变，值来自新的 `derivePrefsFromRules`；跟随 default 的行显示淡化的"跟随 default"标注。

---

## 5. 与 smart routing `agent.claude_code == subagent` 的关系

两者并存：槽位绑定是**确定性的主路径**（依据 Claude Code 发出的 model 名，env / desk / 规则表都看得见）；
smart routing 的 prompt 指纹用于区分 compact，或旧版 Claude Code 不认 `CLAUDE_CODE_SUBAGENT_MODEL` 时兜底。
在 smart rule catalog 的 `agent.claude_code` 处加提示：只想让 subagent 用另一个模型时，用槽位绑定。

---

## 6. 实施顺序

1. `ScenarioConfig.SlotBindings` + 配置层读写 + 迁移；单测断言迁移前后 env 逐字节相同。
2. `GenerateCCEnv` / `derivePrefsFromRules` 改为按绑定解析，删除 mode 分支；更新 `cc_settings`、`claudeCodePrefsState` 测试。
3. API + swagger + `task codegen`。
4. 前端槽位表、预设、Apply 提示；mock handler；ui-preview 截图。
5. TUI quickstart 的 "Use unified mode?" 改为预设选择；更新 `claude-code-config.md` §5。

## 7. 决定

- **env 直写绑定规则的 `request_model`**，不做服务端槽位解析：路由图逻辑保持不变，rule 就是 request。改绑定后需要重新 Apply（主场景通过 Client Config 状态芯片的 "Reapply" 提示；profile 的 settings 在启动时重建，无需操作）。
- 预设不会停用任何规则，所以在重新 Apply 之前，仍在用旧 env 的 Claude Code 照常路由。
- 存储为 `ScenarioConfig.ClaudeCodeSlots`；从未写过绑定的 scenario 按旧的 unified/separate 标志（profile 为 `ProfileMeta.Unified`）惰性推导，不需要迁移。
- 实现位置：`internal/config/cc_slots.go`（解析 / 绑定 / 预设）、`internal/server/module/scenario/handler_slots.go`（API）、`frontend/src/pages/scenario/components/ClaudeCodeSlotsCard.tsx`（槽位表）。
- 未覆盖：`tingly-box agent apply claude-code --unified` 仍按显式参数写 env，不读槽位绑定。
