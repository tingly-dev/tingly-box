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

## 3. 决策:`[1m]` 进 `rule.RequestModel`,作为单一真源

把 `[1m]` 持久化进 `rule.RequestModel`,让它成为**唯一真源**;所有 cc 配置面
都从它派生:

```
            ┌───────────────── rule.RequestModel (SoT) ─────────────┐
            │   built-in-cc-sonnet  /  profile pN 的 sonnet rule      │
            │   request_model = "tingly/cc-sonnet[1m]"                │
            └───────┬───────────────────────────────────┬───────────┘
       派生 env      │                                    │  派生 env
   (默认场景)        ▼                                    ▼   (profile)
   prefs.ToEnv ─► ...[1m]                       generateCCEnv ─► ...[1m]
                    │ 写 settings.json                    │ 写 <profile>.json
                    ▼                                    ▼
              Claude Code 读 env、按名字感知 [1m] → 发 context-1m beta
                    │
                    ▼ 回到 tingly:入站 model == rule.RequestModel(都带 [1m])
              精确匹配命中(不动匹配路径)
                    │
                    ▼ ModelHeader 开关 / Quick Config 开关 / profile 启动
              全部从同一个 rule.RequestModel 投影 → 自动联动
```

**为什么选后缀进 request_model,而不是加独立布尔字段**(取舍):

| 选项 | 采纳 | 理由 |
|---|---|---|
| `[1m]` 进 `request_model` | ✅ | 单一字符串真源:它本就是 §5.4 "rule = env = wire = 匹配键"的同一个串。`[1m]` 顺着它流到 env、流回 wire、精确匹配自动命中——**不需要在入口 strip**。前端 `has1M`/`with1M` 已认后缀,几乎零改动反映。 |
| Rule 加独立布尔 `Context1M` | ❌ | 模型身份与上下文窗口分两个真源,每个生成器要"组合"二者;且 env 带 `[1m]`、rule 干净 → 入口必须 strip `[1m]` 才能匹配。联动天然更弱。 |
| RuleFlags 里加 flag | ❌ | rule flag 是"请求期链路行为"(见 `rule-flags.md`),这里是"env 生成输入",语义不符;且 `generateCCEnv` / `ToEnv` 不走请求期 flag 解析。 |

代价:`request_model` 不再是纯净模型名。但它本是入站别名(真实上游模型在
service 上,计费按 service 走),`[1m]` 漏到日志 / 用量里基本是装饰性、甚至
能区分 1M 用量。影响面审计见 §6。

---

## 4. 后端改动

### 4a. `generateCCEnv` 从 rule 派生(`internal/command/cc_command.go`)

- 新增可测 helper `resolveCCModelSlots(cfg, scenario, profileID, unified, isProfile) map[string]string`:
  - 读取该 scenario 的 rules(默认场景 = `claude_code` 的 `built-in-cc*`;
    profile = `ProfiledScenarioName(claude_code, profileID)` 的短名 rule)。
  - 按槽位匹配 rule(默认场景按 `built-in-cc-<variant>` UUID;profile 按
    `request_model` 的 **base 名**——比较时 strip `[1m]`),输出该 rule 的
    **完整 `request_model`(含 `[1m]`)**。
  - rule 缺失时回退到现有硬编码默认串(保持当前行为)。
- `generateCCEnv` 签名改为接收 `slots map[string]string`,只负责拼装 env;
  rule 读取逻辑下沉到 helper。call site(`cc_command.go:135`)已有
  `globalConfig`。
- unified:单条 `cc` / `tingly/cc` rule 的 `[1m]` 复制到全部 5 槽。

### 4b. rule 更新 API(已存在,无需新增)

复用 `POST /api/v1/rule/{uuid}`(`config.UpdateRule`,`config.go:644`;前端
`api.updateRule`,`api.ts:469`)。1M 写回**不需要**新端点 / 新 swagger。

> 后端不新增字段、不动 routing、不动 transform。`[1m]` 只是 `request_model`
> 的一部分,沿用既有精确匹配。

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

## 6. `[1m]` 流经 request_model 的影响面审计

| 位置 | 行为 | 结论 |
|---|---|---|
| `config.go` 各 `RequestModel ==` 查找(733/746/772/785/798/821/865/895) | 两侧都带 `[1m]`,一致 | ✅ 命中 |
| `handlers.go:292/350` 路由入口匹配 | 同上 | ✅ |
| `migration.go:427/448` `== "*"` 通配 | `[1m]` ≠ `*` | ✅ 不误伤 |
| `configapply/handler.go:654`、`tb_client.go:191`(用 RequestModel 构建 catalog/env) | 会带出 `[1m]` | ⚠️ 审计:Codex catalog / 状态行是否容忍;必要时这两处 strip |
| 计费 / 用量(`model_request_handler.go`、`claude.models.json` 查价) | 计费按 **service 上游模型**,非 request_model | ⚠️ 确认查价不读 request_model;日志带 `[1m]` 视为可接受 |

---

## 7. 兼容 / 迁移

- 老用户此前靠 settings.json 里临时 `[1m]` 的,rule 无后缀 → 升级后需**重新
  开一次** 1M 开关(无法从 rule 反推,不做破坏性迁移)。release note 注明。
- `init.go` / profile 种子规则继续不带 `[1m]`;后缀由用户开关添加,零迁移。

---

## 8. ⚠️ 明确不在本次范围:上游真正激活(beta 转发)

让**上游真正切到 1M** 还需 round tripper 转发 client 的
`context-1m-2025-08-07` beta:`claude_round_tripper.go:386-421` 现在用硬编码
常量 `anthropicBeta` 重建 header、把 client 发来的 beta 清掉
(`406-408` 的合并逻辑被注释)。

本设计只解决 **表示 + 联动 + 路由命中**。在补上 beta 转发(把"覆盖"改成
"合并")之前,OAuth → `api.anthropic.com` 这条链路上游不会真正生效;三方
Anthropic 兼容 provider(非 OAuth 分支)可能本就透传。此项作为后续独立改动。

---

## 9. 测试点

- `resolveCCModelSlots`:默认 / unified / separate / profile 四态;rule 带 /
  不带 `[1m]`;rule 缺失回退默认。
- `generateCCEnv`:给定 slots 正确产出 env(含 `[1m]`)。
- 前端:`has1M` / `with1M` 幂等(已测);ModelHeader 1M 开关调 `updateRule`
  的 payload 正确(加 / 去后缀、不动其余字段);"重新应用"按更新后 rules
  重新 derive。
- 回归:`tingly/cc-sonnet[1m]` 入站 → 精确命中该 rule(路由层单测)。

---

## 9b. 实现落地状态(首版)

已实现:

- **真源助手**:`internal/typ/model_tag.go`(`ContextWindow1MTag` /
  `HasContextWindow1M` / `StripContextWindow1M` / `WithContextWindow1M`)+
  前端 `frontend/src/components/rule-card/utils.ts`(`ONE_M_SUFFIX` /
  `hasOneM` / `stripOneM` / `withOneM` / `isClaudeCodeScenario`)。
  `ClaudeCodeQuickConfig.tsx` 改为复用前端助手(别名 `has1M`/`with1M`),
  消除重复实现。
- **CLI / Profile env**:`cc_command.go` 新增 `resolveCCModelSlots`,
  `generateCCEnv` 改为消费它。默认场景按 `built-in-cc[-variant]` UUID 取,
  profile 按短名 base(strip `[1m]`)取,输出完整 `request_model`。覆盖
  四态单测 `cc_command_test.go`。
- **模型节点开关**:`ModelRequestHeader` 增 `oneM` prop(`UnifiedRoutingGraph`
  透传),`RuleCard` 计算 `show/on/onToggle`,门控 `claude_code` 系 + 非
  通配。toggle 走既有 `updateField('requestModel', withOneM(...))` 自动落库。
- **Quick Config 联动**:`ClaudeCodeConfigModal.handleApply` 新增
  `syncOneMToRules` —— Apply 时把每个槽位的 1M 状态(仅 `[1m]` 这一位)同步
  到对应 built-in 规则的 `request_model`(先同步规则,再写 settings.json),
  避免"env 带 `[1m]`、rule 不带 → 精确匹配失配"。模型名本身仍与 rule 解耦。
- **re-config 弹窗**:`rule-card/OneMReconfigDialog.tsx`,默认/profile 双话术
  + sessionStorage "本次不再提示"。先翻转后提示。

暂缺(后续):

- 弹窗的"一键重新应用"(`onReapply`)未接线——当前弹窗为信息提示型。接线
  需 `UseClaudeCodePage` 提供"按更新后 rules 重新 derive + applyClaudeConfig"
  回调,prop 传到 `RuleCard` → `OneMReconfigDialog`。
- §8 的 beta 转发(上游真正激活)仍未做。

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
