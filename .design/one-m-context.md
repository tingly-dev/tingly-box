# 1M 上下文窗口:以 `request_model` 后缀为真源,联动 CC 配置与 Profile

> 适用对象:tingly-box 前后端贡献者,涉及 Claude Code 1M 上下文配置、规则卡
> 模型节点、profile 启动 env。
> 本文记录:为什么 1M 现在"配了不生效 / profile 不能切",决定把 `[1m]`
> 钉在 `rule.request_model` 上作为单一真源,以及由此推导的后端 / 前端 / UX
> 改动与取舍。

---

## 1. 背景与症状

用户反馈两个痛点:

1. **难独立配置 1M**:Claude Code / Codex 这类主场景,1M 是"一次性"写进
   `~/.claude/settings.json` 的 env 字符串,和规则脱钩,难以按槽位独立、可
   反复地配置。
2. **Profile 不能切换 1M**:profile 启动走另一条 env 生成路径,根本没有 1M
   入口。

更本质的一句话:**1M 是客户端(Claude Code)基于 env + 模型名 `[1m]` 感知
的**,不是服务端"发起"的;但今天 `[1m]` 在系统里没有一个"家",所以谈不上
联动。

---

## 2. 现状诊断:1M 在系统里没有真源

| 路径 | 怎么生成 model env | 1M 怎么处理 | rule 是真源吗 |
|---|---|---|---|
| Quick Config(默认场景)`prefs.ToEnv`(`internal/agent/prefs.go`) | 从 rules 的 `request_model` 派生(`derivePrefsFromRules`) | 表单里一个**临时开关**(`with1M`,`ClaudeCodeQuickConfig.tsx`)给 env 串拼 `[1m]`,Apply 写进 settings.json,**不回写 rule**(见 `claude-code-config.md` §5.5 的故意解耦) | ❌ |
| Profile `generateCCEnv`(`internal/command/cc_command.go:351`) | **完全硬编码**(`"tingly/cc-sonnet"` / `"sonnet"` / `"cc"`),**根本不读 rules** | **没有 1M 概念** | ❌ |

连带后果:

- 默认场景:1M 开关 transient,重开表单从 rule 重推 → 状态丢失("一次性")。
- 路由:routing 是 `rule.RequestModel == 入站 model` 的**精确匹配**
  (`internal/server/config/config.go` 多处 + `handlers.go:292/350`)。client
  开 1M 时发 `tingly/cc-sonnet[1m]`,但 rule 的 `request_model` 是干净串
  → **精确匹配失败** → 请求落空。这是"配了不生效"的直接原因。
- Profile:`generateCCEnv` 硬编码、不读 rule → profile 无任何 1M 入口。

历史遗留(本设计不依赖):`internal/client/claude_round_tripper.go:399-402`
和 `claude_client.go:76` 注入 `context-1m-2025-08-07` beta 的代码被注释,
`supportsContext1M` 为死代码。详见 §8。

---

## 3. 决策:`Rule.Context1M` 布尔为真源,`[1m]` 只是 env 与 wire 上的投影

`rule.request_model` 保持**干净的模型名**(`"tingly/cc-sonnet"`),1M 状态用
**独立布尔字段 `Rule.Context1M`** 持久化。所有 cc 配置面都从这两块组合派生;
server 入口在匹配时反向解析 `[1m]` 后缀,把它映射回干净 rule。

```
            ┌──────────── Rule (SoT) ────────────┐
            │  RequestModel: "tingly/cc-sonnet"   │ ← 始终干净
            │  Context1M:    true                 │ ← 唯一真源
            └────────┬──────────────────┬─────────┘
   派生 env (默认)     │                  │  派生 env (profile)
        ▼              │                  │       ▼
   prefs.ToEnv         │                  │   generateCCEnv
   "...sonnet[1m]"     │                  │   "...sonnet[1m]"
        │ 写 settings.json                │  写 <profile>.json
        ▼                                  ▼
   Claude Code 读 env、按名字感知 [1m] → 发 context-1m beta
        │
        ▼  回到 tingly:入站 model = "tingly/cc-sonnet[1m]"
   MatchRuleByModelAndScenario:
     exact("tingly/cc-sonnet[1m]") ✗
     → strip [1m] → exact("tingly/cc-sonnet") ✓ 命中干净 rule
        │
        ▼  Round tripper(上游 #1157 merge_beta_flags + 白名单):
   client 发的 context-1m beta 被允许转发给 Anthropic
        │
        ▼  ModelHeader 开关 / Quick Config 开关 / profile 启动
   全部读 rule.Context1M(干净 model + 干净开关)→ 自动联动
```

**为什么选独立布尔(改了!)**:

| 选项 | 采纳 | 理由 |
|---|---|---|
| `Rule.Context1M bool` 独立字段 | ✅ | **`[1m]` 是配置展示效果,不是模型身份。**模型名保持干净 → 按 request_model 分组的统计/日志/dashboards 不被切成 `sonnet` vs `sonnet[1m]` 两份;`tb_client.resolveClaudeCodeModels` 等下游消费者拿到的也是干净串。两个字段的组合在 env 生成处一次完成(`WithContextWindow1M(model, flag)`),不会"半同步"。 |
| `[1m]` 进 `request_model`(前版) | ❌ 已弃 | 看似单串真源,但污染 model identity;且把"路由匹配"与"上下文窗口语义"硬绑在同一个字符串里,任何按 request_model 分组的下游全要打补丁。 |
| `RuleFlags` 里加 flag | ❌ | `RuleFlags` 是请求期 transform/ctx 注入(见 `rule-flags.md`),Context1M 是 env 生成+入口解析的 **config-time 数据**,不走那条流水线。 |

代价:server 入口要做一次 `[1m]` strip 才能匹配 — 1 处代码(`MatchRuleByModelAndScenario`),覆盖整个站。换来 model identity 干净 + 双向投影解耦。

---

## 4. 后端改动

### 4a. 新增 `Rule.Context1M` 字段(`internal/typ/type.go`)

```go
type Rule struct {
    ...
    SmartEnabled bool `json:"smart_enabled"`
    // Context1M is config-time data: env materialization adds [1m]; server
    // entry strips [1m] for matching. RequestModel stays clean.
    Context1M    bool `json:"context_1m,omitempty" yaml:"context_1m,omitempty"`
    ...
}
```
同步 `Rule.ToJSON()` 输出 `"context_1m"`。

### 4b. 入口反向解析 `[1m]`(`internal/server/config/config.go`)

`MatchRuleByModelAndScenario` 增加一层"strip 后再 exact"匹配:

```
priority: exact(literal) > exact(strip [1m]) > wildcard
```

`exact(literal)` 在前是**防御层**——任何手工编辑或老配置里仍带 `[1m]` 的
rule 也能匹配。canonical 路径(干净 rule + Context1M)走第二层。

### 4c. `generateCCEnv` 派生(`internal/command/cc_command.go`)

`resolveCCModelSlots` 用规则的干净 `request_model` + `Context1M` 组合:

```go
return typ.WithContextWindow1M(r.RequestModel, r.Context1M)
```

profile 匹配也回到直接 `r.RequestModel == shortName`(不再需要 strip)。

### 4d. 迁移(`migration.go::migrate20260612`)

幂等地把任何**仍在 `request_model` 里带 `[1m]`** 的旧 rule(来自本特性
早期分支的中间形态)平移到新形态:

```go
if HasContextWindow1M(rule.RequestModel) {
    rule.RequestModel = StripContextWindow1M(rule.RequestModel)
    rule.Context1M = true
}
```

### 4e. rule 更新 API(已存在)

复用 `POST /api/v1/rule/{uuid}`(`config.UpdateRule`)。`Rule` 加字段后,
`ShouldBindJSON(&rule)` 自动接受 `context_1m`,**无需新 swagger**。

---

## 5. 前端改动(UX 优先)

### 5a. 模型节点上的 1M 开关 —— `ModelRequestHeader`

规则卡的模型节点是 **`ModelRequestHeader`**(`RuleCard.tsx` →
`ModelRequestHeader.tsx`),它已有 **`extraActions` 插槽**(L81)。

- 在 `extraActions` 放一个紧凑 `1M` Switch。写入走现成
  `onModelChange` → `onUpdateRecord('requestModel', with1M(modelName, on))`,
  规则卡本就 autosave `request_model`。
- **门控**:
  - 仅 `claude_code` 系场景显示(`rule.scenario` 在 RuleCard 可得)。`[1m]`
    是 Claude Code 客户端约定;通用 openai/anthropic 规则加后缀只会让精确
    匹配失配。
  - 通配规则(`*` / `[any]`)不显示。
- **白送 profile 入口**:profile 规则用同一规则卡 UI 渲染 → 这个开关同时覆盖
  默认 built-in-cc 规则和 profile 规则,profile 的 1M 入口无需单独控件。

### 5b. 与 Quick Config 自动联动

两者都读写同一个 `rule.request_model`(SoT),是**同一真源的两个视图**:

- ModelHeader 开 1M → 写 rule;Quick Config 下次打开 `derivePrefsFromRules`
  读到 `[1m]` → 1M 开关自动点亮。反之亦然。
- `ClaudeCodeQuickConfig.tsx` 的 `derivePrefsFromRules`(L443)无需改;
  `has1M`/`with1M`(L427-431)已认后缀。
- Quick Config 内联 1M 开关(L594)若也要写回 rule(而非仅 env),按同一
  `with1M` + `api.updateRule` 路径,与 ModelHeader 共用逻辑。

### 5c. 点击 1M 弹"重新配置"提示

**为什么需要**:rule 是 SoT(routing 立刻匹配),但 client 只通过**已落盘的
env** 才看得到 `[1m]`,而 tingly-box 的 materialize 不是自动的。

| 场景 | rule 改后 | client 何时看到 `[1m]` |
|---|---|---|
| 默认场景 | routing 立即匹配 | settings.json 是 Quick Config **Apply 一次性写的** → 现在是旧的 → **必须重新 Apply** |
| Profile | routing 立即匹配 | env 在 `tingly-box cc --profile` **启动时**生成 → **重启 profile 会话**生效,运行中不会热生效 |

弹窗设计(**先翻转、再提示** —— 乐观更新写 rule,再弹 CTA):

- **默认场景**:"1M 已写入路由规则。Claude Code 还在用旧的 settings.json,
  需要重新应用配置才会带 `[1m]`。" → 主按钮 **【立即重新应用】**:用**更新后
  的 rules** 重新 `derivePrefsFromRules` 再 `api.applyClaudeConfig`(顺序不能
  反:先写 rule → 再 derive 才拿得到 `[1m]`)。
- **Profile**:"该 profile 的 env 在启动时生成。重启
  `tingly-box cc --profile <id>` 生效。"(给命令,无一键)
- 两者补一句:**env 是进程启动时读的,运行中的 Claude Code 会话即使重新
  Apply 也需重启会话**才认新 `[1m]`。
- 加 **【本次不再提示】**,避免连开多槽位被反复打断。

轻量备选:改成规则卡上常驻 banner("自上次应用后配置已变更 · 重新应用")。
更激进备选:默认场景下开关直接静默重写 settings.json(更接近 cc-switch 的
"切了就 live"),但会在每次点击动用户的 `~/.claude/settings.json`,侵入性高,
不采纳为默认。

---

## 6. 影响面审计(新方案:`request_model` 永远干净)

| 位置 | 行为 | 结论 |
|---|---|---|
| `config.go` 各 `RequestModel ==` 查找(733/746/772/785/798/821/865/895) | rule 干净,入口的 `[1m]` 在 `MatchRuleByModelAndScenario` 里被 strip;其他 callsite 由 handlers 上游已通过 Match 解析过的 rule 对象传入,不会拿到带 `[1m]` 的串 | ✅ 一处 strip,全站受益 |
| `handlers.go:292/350` 路由入口匹配 | 走 `MatchRuleByModelAndScenario` | ✅ |
| `migration.go:427/448` `== "*"` 通配 | rule 永远不会是 `[1m]`,通配不受影响 | ✅ |
| `configapply/handler.go:654`、`tb_client.go:191`(catalog / 状态行) | 拿到的是干净 `request_model` | ✅ 干净 → catalog/状态行不再泄露 `[1m]` |
| 计费 / 用量、按 request_model 分组的统计 | 干净 → `sonnet` 不会被切成 `sonnet` vs `sonnet[1m]` | ✅ |
| `migration.go::migrate20260612` | 一次性把旧形态 (`request_model` 带 `[1m]`) 平移到 `Context1M` 字段 | ✅ 幂等 |

---

## 7. 兼容 / 迁移

- 来自本特性早期分支(`[1m]` 在 `request_model` 里)的旧配置:
  `migrate20260612` 一次性平移到 `Context1M` + 干净 `request_model`,**用户
  无感**。迁移幂等;后续 1M 开关只读写 `Context1M`。
- 来自 main 的全新配置:`Context1M` 默认 false,行为不变。
- 入口匹配:**双层** exact (literal → strip-1m) 保证手工编辑/老导出文件即使
  仍带 `[1m]` 也能匹配。

---

## 8. 上游激活(✅ 由 upstream #1157 闭合)

让上游真正切到 1M 需要 round tripper 把 client 发来的
`context-1m-2025-08-07` beta 转发给 Anthropic。这部分由 **upstream `43a113c`
(#1157)** 完成:`internal/client/claude_round_tripper.go` 现在用
`mergeBetaFlags(...)` 合并 client 发来的 beta,并通过
`claudeCodeAllowedUpstreamBetas` 白名单**显式包含** `context-1m-2025-08-07`
作为指纹安全的 flag。

端到端链路因此完整:
```
rule.Context1M=true → env 含 [1m] → CC 客户端按名感知 → 发 context-1m beta
  → tingly round tripper mergeBetaFlags 放行 → Anthropic 收到 → 1M 上下文激活
```

---

## 9. 测试点

后端(`internal/command/cc_command_test.go`):
- `resolveCCModelSlots`:默认 / unified / separate / profile 四态;开关 `Context1M` 后
  - 对应 slot 输出 `xxx[1m]`,其他 slot 干净
  - rule.RequestModel **没有**被弄脏(始终干净)
- `MatchRuleByModelAndScenario_StripsContext1M`:`tingly/cc-sonnet[1m]` 入站 →
  命中 `built-in-cc-sonnet`(clean rule);clean 入站也命中。

前端(`ruleUpdatePayload.test.ts`):
- `context_1m` 在 payload 里(snake_case),默认 false,显式 true 时为 true。

待补:Race 1/2 的并发测试(toggle → re-apply 紧邻;syncOneMToRules 部分失败)。

---

## 9b. 实现落地状态(v2 — Context1M 字段)

后端:
- `Rule.Context1M bool`(`internal/typ/type.go`)+ `ToJSON` 输出。
- `MatchRuleByModelAndScenario` 新增 strip-1m exact 层(`config.go`)。
- `resolveCCModelSlots` 用 `WithContextWindow1M(model, flag)` 组合
  (`cc_command.go`)。
- `migrate20260612` 平移旧形态(`migration.go`)。
- 单测覆盖匹配+四态 env 派生(`cc_command_test.go`)。

前端:
- `ConfigRecord.context1M` / `Rule.context_1m` 类型字段
  (`RoutingGraphTypes.ts`)。
- `ruleToConfigRecord` 读 `rule.context_1m`(`utils.ts`)。
- `buildRuleUpdatePayload` 写 `context_1m`(`ruleUpdatePayload.ts`),
  `useRuleCardHooks.autoSave` 回写父态时一并 echo。
- `RuleCard.handleToggleOneM` 改 `context1M` 字段;**await** `updateField`
  → 修复 Race 1;失败时不弹 dialog。
- `RuleCard.showOneM` 只看 scenario,不再因通配模型隐藏(开关与名字解耦,
  通配规则也能配 1M)。
- `ClaudeCodeQuickConfig.derivePrefsFromRules` 改为 `rule.request_model +
  (rule.context_1m ? '[1m]' : '')` 组合。
- `ClaudeCodeConfigModal.syncOneMToRules` 写 `context_1m` 字段;**并行
  Promise.allSettled + 失败抛出 + Apply 主动 abort** → 修复 Race 2;不再
  默默"non-fatal continue"。
- `OneMReconfigDialog` + 一键 Re-apply 链路保留,行为不变(`onReapply` 走
  `getRules → derive → apply`;Race 1 修复后此处不再读到 stale rule)。

测试:
- 后端 `TestMatchRuleByModelAndScenario_StripsContext1M`、`TestResolveCCModelSlots_*` 全绿。
- 前端 `ruleUpdatePayload.test.ts` 加 `context_1m` 往返断言。
- TODO:Race 1/2 的端到端并发回归(需 Playwright + 模拟网络失败注入)。

## 10. cc-switch 借鉴

参考桌面 all-in-one 配置切换器 [farion1231/cc-switch](https://github.com/farion1231/cc-switch)
([用户手册](https://github.com/farion1231/cc-switch/blob/main/docs/user-manual/en/README.md)):

- 它用 SQLite 作 **单一真源(SSOT)**,切换时 **materialize 到 live
  settings.json**(原子写:临时文件 + rename)。→ 印证我们 rule=SoT、多视图
  投影的选择。
- 它明确:多数工具切换后要**重启终端 / CLI** 才生效;Claude Code 例外、支持
  provider 数据热切换。→ 印证我们的 re-config 弹窗,并提醒 env 变量仍需重启
  会话。
- 差异:cc-switch 自动写 live 文件;我们用显式的一键 Apply 保留可控性。
