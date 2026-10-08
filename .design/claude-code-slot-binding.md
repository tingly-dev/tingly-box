# Claude Code：主规则 + 槽位

> 状态：**已实施**。背景：`claude-code-config.md`（env 线形、规则 ↔ slot 映射）。

![Slots 行](images/claude-code-slots.png)

## 问题

原来只有两种模式：Unified（所有槽位走 `cc`）和 Separate（每个槽位走自己的规则，`ANTHROPIC_MODEL` 走 `default` 规则）。最常见的需求却是"统一，只把 subagent 分出去"，只能切 Separate 再把 5 条规则配成一样。

## 模型

没有模式。每个 Claude Code scenario（主场景和每个 profile）只有：

- **主规则 `builtin:<scenario>:cc`**：就是 default 槽位（`ANTHROPIC_MODEL`），一直存在、一直启用。
- **5 条可选的槽位规则** `builtin:<scenario>:<slot>`（haiku / sonnet / opus / fable / subagent）：启用就用自己的 `request_model`，否则跟随主规则。

全部点亮 = 原来的 Separate，全部熄灭 = 原来的 Unified。路由不变：rule 就是 request，`request_model` 仍是路由键。

## 界面

- Base URL / API Key 下面一行 **Slots**：`default` 常亮不可点；其余每个槽位一个 chip。点一下加规则（第一次创建时复制主规则的路由，所以行为不变），再点一下停用（不删除，再点亮时恢复原配置）。
- Model Rules 只显示启用的规则，主规则在最前。
- 新建 profile 只问名字，从主规则开始。
- 没有 Unified / Separate 开关、确认弹窗或模式标签。
- 改了槽位后 env 会变：主场景由 Client Config 状态芯片提示 Reapply；profile 的 settings 在启动时重建。

## 迁移（`20261008-claude-code-slots`，每个配置执行一次）

把旧 `default` 规则并进 `cc`，**保留它原来的 `request_model`**，已写入 `settings.json` 的 env 不用改、迁移当下不断流量：

| 现状 | 动作 | 迁移后 env |
|---|---|---|
| 主场景 unified（`cc` 启用） | 删除停用的 `default` 规则 | 不变（`tingly/cc`） |
| 主场景 separate（`cc` 停用，`default` 启用） | `default` 的 services / flags / 负载均衡 / smart routing 和 `request_model`（`tingly/cc-default`）搬进 `cc`，启用 `cc`，删除 `default`；槽位规则保持启用 | 不变 |
| unified profile | 不动 | 不变 |
| separate profile（无 `cc`） | 同上，用 `default` 规则建出 `cc`（`request_model` 保持 `default`），删除 `default` | 不变 |

同时清掉 scenario 的 `flags.unified/separate` 和 `ProfileMeta.Unified`。新安装不再预置 `default` 规则。

## 兼容

- 旧的模式写入（`SetScenarioFlag`、`POST /scenario/:scenario` 带 unified/separate）= 全部点亮 / 全部熄灭，模式本身不落盘。
- `POST /scenario/claude_code/profiles` 的 `unified:false`（旧的 separate）= 新建后全部点亮；省略或 `true` = 只有主规则。
- `tingly-box agent apply claude-code --unified`：隐藏并忽略，env 一律按槽位生成。TUI quickstart / agent 不再询问模式。

## 实现

- `internal/config/cc_slots.go`：`SetClaudeCodeSlot`、迁移 `collapseClaudeCodeMainRuleOnce`。
- `agent.ClaudeCodeSlotModels`：settings 文件（`GenerateCCEnv`）、tbclient env、`agent apply` 共用的槽位解析。
- API：`PUT /api/v1/scenario/:scenario/claude-code/slots/:slot {enabled}`。
- 前端：`useSlotRouting` 提供 Slots 行和启用的规则；`derivePrefsFromRules({ rules })` 按同样规则推导 env。
