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

### 4f. Codex catalog context window(`apply_config.go`)

Codex 没有 `[1m]` 这类 wire 信号——1M 纯粹是 catalog 里声明的预算。所以
Codex 的 1M = 生成 `~/.codex/tingly-model-catalog.json` 时,对开了
`Context1M` 的模型把 `context_window` / `max_context_window` 写成
`codex1MContextWindow = 1_000_000`(否则维持 200k)。

- `CollectCodexContext1M(cfg) map[string]bool`:codex 场景规则的
  `request_model` → `Context1M`(同名 OR 合并)。
- `RenderCodexModelCatalog(models, context1M)` 按 map 决定每个 entry 的窗口。
- `ApplyCodexConfig(..., context1M)` 透传;两个 caller(HTTP handler
  `ApplyCodexConfigFromState`、agent path `CodexParams.Context1M` ←
  `rule_bridge`)都喂 `CollectCodexContext1M`。
- Codex slug **保持干净**(catalog 用干净 slug),server 入口的 `[1m]` strip
  对 codex 无副作用(codex 不发后缀)。

---

## 5. 前端改动(精简后 — 一个开关 + 派生)

核心:**唯一的 1M 控件是规则卡模型头(`ModelRequestHeader`)上的开关**,直接读写
`rule.context_1m`。没有跨面同步、没有重配弹窗、没有一键 Re-apply——那些都是
之前过度设计的产物,已删。

### 5a. 模型节点上的 1M 开关 —— `ModelRequestHeader`

- `ModelRequestHeader` 加 `oneM` prop(`UnifiedRoutingGraph` 透传),
  `RuleCard` 计算 `show / on / onToggle`。
- `on = configRecord.context1M`;`onToggle` → `updateField('context1M', on)`
  (规则卡既有 autosave 路径,失败自动回滚 + toast)。
- **门控**:`isClaudeCodeScenario(scenario) || isCodexScenario(scenario)`。
  通配规则也能配(1M 是独立 flag,不是模型串,不影响匹配)。
- **白送 profile + codex 入口**:profile 规则、codex 规则都用同一规则卡渲染,
  开关自动覆盖,无需各自做控件。

### 5b. 与 Quick Config 的关系(单向派生,无同步)

- `ConfigRecord.context1M` ← `rule.context_1m`(`ruleToConfigRecord`);
  `buildRuleUpdatePayload` 写 `context_1m`;`autoSave` echo 回父态。
- `ClaudeCodeQuickConfig.derivePrefsFromRules` 从 `request_model +
  (context_1m ? '[1m]' : '')` 组合 env 串 → Apply 写 settings.json。
- Quick Config modal **不再有内联 1M 开关**(避免双写 + 双源)。模型行显示
  **干净模型名**(剥掉 `[1m]`,符合"1M 是展示效果不是模型名"),旁边一个
  只读 `1M` 角标表示该槽位已开;开关动作在规则卡完成。

### 5c. 为什么删掉重配弹窗 / 一键 Re-apply

1M 现在是**配置生成时单向投影**的效果(§4c CC env / §4f codex catalog)。
改 `rule.context_1m` 后,用户本来就要走既有的 Apply(modal / 一键)或重启
`tingly-box cc` 才生效——这是 agent 配置的固有节奏,不需要 1M 专属弹窗。
保留 autosave 的成功 toast 作为反馈即可。要更强提醒可后续加一条规则卡
banner,但默认不做。

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

- Codex catalog(`apply_config_test.go::TestRenderCodexModelCatalog_Context1M`):
  1M-flagged 模型 `context_window=1_000_000`,其余 `200000`。

前端(`ruleUpdatePayload.test.ts`):
- `context_1m` 在 payload 里(snake_case),默认 false,显式 true 时为 true。

---

## 9b. 实现落地状态(v3 — 精简 + Codex)

相对 v2 的变化:**删掉所有跨面同步与重配 UI**(它们是过度设计 + race 源),
**加上 Codex catalog**。

后端:
- `Rule.Context1M bool` + `ToJSON`;`MatchRuleByModelAndScenario` strip-1m 层;
  `resolveCCModelSlots` 用 `WithContextWindow1M`;`migrate20260612` 平移旧形态。
- **Codex**:`CollectCodexContext1M` + `RenderCodexModelCatalog(models, ctx1m)`
  + `ApplyCodexConfig(..., ctx1m)`,两个 caller(handler / agent path)透传。
- 测试:`TestResolveCCModelSlots_*`、`TestMatchRuleByModelAndScenario_StripsContext1M`、
  `TestRenderCodexModelCatalog_Context1M`。

前端:
- 类型 + 映射:`ConfigRecord.context1M` / `Rule.context_1m` /
  `ruleToConfigRecord` / `buildRuleUpdatePayload` / `autoSave` echo。
- **唯一开关**:`ModelRequestHeader.oneM`(`UnifiedRoutingGraph` 透传,
  `RuleCard` 计算),写 `context1M`;门控 cc ∪ codex 场景。
- `derivePrefsFromRules` 单向派生 env 串(读 `context_1m`)。
- Quick Config modal:**移除内联 1M 开关**,模型行显示干净名 + 只读 `1M` 角标。
- **删除**:`OneMReconfigDialog`(整文件)、`ClaudeCodeConfigModal.syncOneMToRules`
  + Apply abort、`onReapplyConfig` 透传链(TemplatePage / RuleCard /
  UseClaudeCodePage)、`UseClaudeCodePage.handleReapplyConfig`。

被删代码消解的问题:
- **Race 1**(toggle→re-apply 读 stale):没有 re-apply 按钮了,消失。
- **Race 2**(syncOneMToRules 半失败):没有 Apply 时反向批量同步了,消失。
- 单向投影(rule flag → config 生成)结构上不可能 env/rule 失配。

仍未做(诚实):
- `internal/server/config` 整个包测试**当前在 main 上就编译失败**
  (`migration_test.go` 引用未定义的 `migrate20260609`),所以
  `TestRenderCodexModelCatalog_Context1M` 现在跑不起来;其逻辑已用仓库内
  throwaway `main` 运行验证(1M→1000000 / 其余→200000)。等 main 修了那个
  migration,此测试自动生效。
- `isClaudeCodeScenario` / `isCodexScenario` 仍是 Go + TS 双份常量。
- Codex profile 场景(`codex:pN`)未纳入 `CollectCodexContext1M`(沿用既有
  collector 的 base-codex-only 口径)。

## 10. cc-switch 借鉴

参考桌面 all-in-one 配置切换器 [farion1231/cc-switch](https://github.com/farion1231/cc-switch)
([用户手册](https://github.com/farion1231/cc-switch/blob/main/docs/user-manual/en/README.md)):

- 它用 SQLite 作 **单一真源(SSOT)**,切换时 **materialize 到 live
  settings.json**(原子写:临时文件 + rename)。→ 印证我们 `rule.Context1M`
  作真源、config 生成时单向 materialize 的选择。
- 它明确:多数工具切换后要**重启终端 / CLI** 才生效;Claude Code 例外、支持
  provider 数据热切换。→ 印证我们不做 1M 专属弹窗:改 flag 后走既有 Apply /
  重启即可。
- 差异:cc-switch 自动写 live 文件;我们用显式的 Apply / `tingly-box cc`
  启动来 materialize,保留可控性。
