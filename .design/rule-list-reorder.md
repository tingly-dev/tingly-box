# Rule 列表硬排序（可持久化重排）实现方案

状态：提案（2026-11-07，待评审）。目标：让 rule list 在所有出现场景里支持「硬排序」
——一条规则能真正移动到列表不同位置并持久生效，且顺序严格影响规则对模型列表的
应用（`/v1/models` 模型列表顺序、路由匹配优先级、客户端配置生成的模型顺序），
而不只是显示效果。

结论先行：**推荐方案 A ——「scenario 级整段顺序提交」端点 + RuleCard 置顶/上移/下移按钮**。
存储层（`position` 列）、写穿（`SyncAll`）、内存数组序到四个应用面的贯通**已经全部存在**，
缺的只是「一个改顺序的写入口」和「UI 控件」。零 schema 变更、零迁移、零应用侧改动。

---

## 1. 现状盘点（附代码证据）

### 1.1 rule list 出现在哪些场景/组件

Web UI 的 rule list 全部经由共享组件 `TemplatePage`（`frontend/src/pages/scenario/components/TemplatePage.tsx`）
渲染 `RuleCard` 列表（TemplatePage.tsx:403-439）：

| 场景 | 入口 | 显示顺序来源 |
|---|---|---|
| 直连路由页（AgentPage 描述符驱动）：`/agent/openai`、`anthropic`、`codex`、`claude_desktop`、`custom`、`opencode`、`pi`、`workbuddy`、`dsh`、`xcode`、`vscode`、`cursor`、`decisions`、`embed`（路由表 frontend/src/routes/appRoutes.tsx:118-139） | AgentPage.tsx:351-374 → TemplatePage（`internal.rules`） | 后端数组顺序 |
| Team 页 | UseTeamPage.tsx:284 直接用 TemplatePage | 后端数组顺序 |
| Image API 页 | frontend/src/pages/image/ImageApiPage.tsx:107 | 后端数组顺序 |
| Claude Code 槽位页（slotRouting）：`/agent/claude_code`、`/agent/claude_code/profile/:profileId` | UseClaudeCodePage.tsx:87、ClaudeCodeProfilePage.tsx:456（AgentPage.tsx:170 `slots.rules`） | **槽位定义顺序**（useSlotRouting.tsx:105-107 客户端按槽位排序），非后端数组顺序 |

排序控件现状：`TitleIconButtons.tsx:35,47-49` 的排序切换按钮 → `useRuleSort`（见 1.4）。
产品里已有的同模式交互先例：智能路由 op 节点的上移/下移按钮（`frontend/src/components/nodes/SmartOpNode.tsx:128-140`，
i18n key `common.moveUp`/`common.moveDown` 已存在于 frontend/src/i18n/locales/zh/app.ts:84-85）。

### 1.2 顺序如何存储

- 领域模型 `typ.Rule` **没有** position 字段（internal/typ/type.go:650-665）；
  顺序 = 内存切片 `Config.Rules` 的数组顺序（internal/config/config.go:30-35）。
- 持久化：SQLite `rules` 表已有 `position` 列（internal/db/rule_store.go:38）；
  `SyncAll` 落库时 `position = i`（rule_store.go:234），`List()` 按 `ORDER BY position`
  读回（rule_store.go:160），启动 hydrate 按 store 顺序灌回 `c.Rules`
  （internal/config/rule_storage.go:59-77）。
- 写路径收口：每次规则变更经 `Save() → syncRulesToStore()` 同步全量
  （config.go:531、rule_storage.go:118）。
- 设计文档定位：`.design/rule-storage.md` §6 明确「顺序……**只服务展示排序**」。

### 1.3 应用顺序语义（顺序真正被消费的地方）

1. **`/v1/models` 模型列表顺序**：`anthropic_model.go:109-119`、`openai_model.go:59-68`
   按数组序迭代 rules → 客户端模型选择器看到的顺序 = rule 顺序
   （Claude Desktop 直接从 /v1/models 拉模型列表，见 config/rule.go:147-149 注释）。
2. **路由匹配优先级**：`MatchRuleByModelAndScenario`（config/rule.go:308-354）分四层
   固定顺序：精确 → `[1m]` 归一 → 历史别名 → 通配；**同层内数组序先命中先赢**。
3. **跨 scenario 兜底**（企业上下文）：protocol_handler.go:249 按数组序。
4. **客户端配置生成**：`collectCodex/OpenCode/DshRuleModels` 按声明顺序
   （"in declaration order"，internal/agent/rule_bridge.go:215-245）→ 写入
   Codex/OpenCode/dsh 客户端配置文件的模型列表顺序。
5. **CLI/TUI 展示**：`rule list`（internal/command/rule_command.go:26,111）、
   TUI（internal/command/tui/rule_mode.go:53,186）按数组序渲染。

### 1.4 现状是否真的只是显示效果 —— 是

- `useRuleSort`（frontend/src/pages/scenario/hooks/useRuleSort.ts:12-31）是纯本地
  state，`original`/`name` 两档，从不回写后端；TemplatePage.tsx:154-157 注释明说
  "Display-only ordering ... doesn't touch `rules` itself"。
- **任何写入面都没有重排入口**：HTTP 只有 GET /rules、GET/POST/DELETE /rule、
  GET /rule/flags/registry（internal/server/module/rule/routes.go:11-57）；
  `AddRule` 永远 append 到末尾（config/rule.go:117），`UpdateRule` 原位替换
  （rule.go:168-172），`DeleteRule` 原位删除（rule.go:471-491）。
  规则创建后顺序永远不变；用户唯一能"重排"的方式是按期望顺序删掉重建。
- 存储层 `position` 列已存在且被每次 Save 正确维护——只是没有任何写路径能改变它。

---

## 2. 候选方案

### 方案 A（推荐）：scenario 级整段顺序提交 + 上移/下移按钮

**交互**：RuleCard 头部加三个操作：**置顶**（用 `@tingly/ui` 已预定义的 `Pin`/`PinFilled`
图标，frontend/packages/ui/src/icons/index.tsx:242-243；已是首条时隐藏）、**上移/下移**
↑/↓（同 SmartOpNode 先例；首条隐藏 ↑、末条隐藏 ↓）。置顶一步到位，服务长列表的快速
操作；↑/↓ 负责逐位微调。拖拽（如 @dnd-kit）可作为后续增强——端点形状不变，交互可
独立演进。不引入拖拽库，改造成本最低、键盘可达。

**API**（新增一个端点）：

```
POST /api/v1/rules/reorder
{ "scenario": "openai", "ordered_uuids": ["u1", "u2", ...] }
→ 200 { success: true, data: [ <重排后的该 scenario 全部规则> ] }
```

语义：
- 把该 scenario 的规则重排为 `ordered_uuids` 顺序；scenario 内未列出的 uuid
  （页面加载后其他端新建的规则）保持当前相对顺序**追加在队尾**（容忍合并，
  不丢规则）；属于其他 scenario 或不存在的 uuid → 400。
- 返回重排后的完整 scenario 子集，客户端直接替换本地 state。
- **置顶复用同一端点**：置顶 = 提交「目标 uuid 在首位」的整段顺序，与普通重排语义完全
  一致，不需要专门 API（同一机制将来也免费支持置底）。

**数据模型**：零变更。`typ.Rule` 不动，`position` 列不动，数组顺序即权威顺序。

**后端改动**：`Config` 新增一个方法（internal/config/rule.go）：

```go
// ReorderRules 把 scenario 的规则重排为 orderedUUIDs 顺序（未列出的规则
// 保持相对顺序追加在队尾），经 saveLocked() 写穿 position 列。
func (c *Config) ReorderRules(scenario typ.RuleScenario, orderedUUIDs []string) ([]typ.Rule, error)
```

实现：`c.mu` 内做稳定分区重排（scenario 块内按给定序，其余规则不动），然后
`saveLocked()`。Sort/过滤/其他 getter 全部自动跟随，因为它们读同一个数组。

**边界情况**：
- 新建规则：`AddRule` append 到末尾——与现状一致（新增在底部 + scroll-to-new-rule），
  无需改。
- 删除：splice 已保序（rule.go:489），无需改。
- 通配/匹配层级：层级判断固定在 `MatchRuleByModelAndScenario` 内部，重排不改变
  「精确 > 通配」优先级，只改变同层平局顺序与展示顺序——语义安全。
- 并发/多端：整段提交 = last-writer-wins，与现有 CRUD 并发语义一致（全仓库无乐观锁）；
  容忍性追加保证不会覆盖别的端刚建的规则。
- 旧数据迁移：零迁移（position 列已在、SyncAll 已维护）。
- 写放大：一次移动重写该 scenario 所有行的 position → SyncAll 全行 diff；
  几十条规模毫秒级，是 `.design/rule-storage.md` §4 已文档化的取舍。

### 方案 B：每条规则独立 sort key（fractional indexing）

**交互**：同上（拖拽/按钮），但每次移动只提交一条记录。
**数据模型**：`position` 改为可中点插入的键（float 或 lexicographic string），
移动 = 取相邻两键中点，只写一行。
**后端**：单条 move 端点 + 键生成/再平衡逻辑；`SyncAll` 的全量 diff-sync 语义
被打破，需要拆出单行写路径——正是 rule-storage.md §8 明确留到「rule 上万条」
才做的增量方向；schema 变更 + 存量行重键迁移。
**边界**：中点耗尽需再平衡；并发移动到同一区间可能产生重复键。
**取舍**：对几十条规则的列表，用迁移 + 再平衡 + 双写路径换「少写几十行」，
纯属过度设计。不推荐。

### 方案 C：前端本地持久化顺序（localStorage）

把 `useRuleSort` 的顺序存 localStorage，按浏览器记忆。
**不满足硬排序定义**：顺序到不了 `/v1/models`、匹配优先级、客户端配置生成；
多端/多浏览器不一致。仅列出以说明排除理由。

---

## 3. 推荐：方案 A

理由（以「简单有效」为第一标准）：

1. **难点已经不存在**。position 列、`SyncAll`、内存数组序 → 四个应用面
   （模型列表 / 匹配 / 客户端配置 / CLI·TUI）的贯通已全部在位，且因为所有消费者
   读同一个数组，硬排序对它们**自动全部生效**，应用侧零改动。
2. **改动面最小**：一个 Config 方法 + 一个 handler + 一条路由 + swagger；
   前端一个 api 函数 + RuleCard 两个按钮 + TemplatePage 一段处理。
3. **与现有架构同一套语义**：Save() 收口、last-writer-wins、全量 diff-sync，
   零新概念、零迁移。
4. **交互可演进**：端点收整段顺序，将来换拖拽不动后端；先置顶/上移/下移按钮最快见效
   （置顶图标已预定义、moveUp/moveDown i18n key 已有、SmartOpNode 先例已有）。

方案 B 是为「万条规则、高频增量写」预设计的形态，与 rule-storage.md §8 的
演进方向冲突（那个方向留给未来需要时再做）；方案 C 直接不满足需求。

---

## 4. 落地清单

### 后端（粗估 0.5 天，含 codegen 与测试）

| # | 文件 | 改动 |
|---|---|---|
| 1 | `internal/config/rule.go` | 新增 `ReorderRules(scenario, orderedUUIDs)`（稳定分区重排 + saveLocked） |
| 2 | `internal/config/rule_test.go` | 测试：基本重排 / 容忍追加 / 跨 scenario uuid 拒绝 / 无 store 轻量 config |
| 3 | `internal/server/module/rule/handler.go` | `ReorderRules` handler（bind + apierr，返回重排后子集） |
| 4 | `internal/server/module/rule/types.go` | `ReorderRulesRequest` / `ReorderRulesResponse` |
| 5 | `internal/server/module/rule/routes.go` | `POST /rules/reorder` + swagger 注解（WithRequestModel/WithResponseModel） |
| 6 | `internal/server/module/rule/handler_test.go` | handler 层测试 |
| 7 | `task codegen` | 重新生成 openapi.json 与前端 `pnpm gen:api` SDK（按 CLAUDE.md：codegen 未跑前前端先留占位函数） |

### 前端（粗估 1 天）

| # | 文件 | 改动 |
|---|---|---|
| 8 | `frontend/src/services/rulesApi.ts` | `reorderRules(scenario, orderedUuids)`（SDK 就绪前占位） |
| 9 | `frontend/src/pages/scenario/components/RuleCard.tsx` | 卡片头部 ↑/↓ + 置顶按钮（新 `allowReorder` prop；默认 false；首条隐藏 ↑/置顶、末条隐藏 ↓；置顶用已预定义的 `Pin`/`PinFilled` 图标） |
| 10 | `frontend/src/pages/scenario/components/TemplatePage.tsx` + `TemplatePage.types.ts` | 透传 `allowReorder`；`handleReorder`：乐观本地重排（`onRulesChange`）→ 调 API → 失败 toast + `loadRules` 回滚；**name 排序视图下隐藏移动按钮**（该视图是 view-only，避免"移了没保存"误解） |
| 11 | slot 页不开启：`AgentPage.tsx`（slotRouting 分支）、`UseClaudeCodePage.tsx`、`ClaudeCodeProfilePage.tsx` | 不传 `allowReorder`（槽位顺序是产品定义的固定优先级，见 1.1） |
| 12 | 直连页开启：`AgentPage.tsx` 直连分支、`UseTeamPage.tsx`、`ImageApiPage.tsx` | 传 `allowReorder` |
| 13 | `frontend/src/mocks/handlers.ts` | mock `POST /api/v1/rules/reorder`（仿 :2663-2825 既有 rule handlers） |
| 14 | i18n `en`/`zh`/`ru` 三份 locale | rule 专属 tooltip（上移/下移/置顶）；`common.moveUp/moveDown` 已存在，置顶 key 新增、需三语言齐（`localeParity.test.ts` 把关） |

### 风险点

- **排序切换视图与硬排序并存的心智负担**：`useRuleSort` 的 name 视图保留为
  view-only、隐藏移动控件；tooltip 说明。若评审觉得多余，可直接移除该切换。
- **槽位页顺序不可重排**是产品语义（cc/sonnet/…槽位优先级固定），文档化以防
  后续误加控件；若未来想让槽位顺序也可调，需另行定义槽位顺序的存储。
- **写放大**：每次移动重写 scenario 全部行（payloadEqual 对 position 敏感），
  几十条规模毫秒级，已在 rule-storage.md §4 文档化，不新增风险。
- 顺带发现（不在本任务范围，建议另行处理）：`internal/usecase/rule.go:55`
  的 `List()` 直接读 `uc.cfg.Rules` 未持 `c.mu`，与并发写存在 data race
  （其他读路径都走 `GetRequestConfigs` 的 RLock）。

## 5. 实施步骤（精简版）

按顺序执行，每步可独立验证；1–3 是后端，4–6 是前端，7 是端到端确认：

1. **Config 方法**：`internal/config/rule.go` 新增 `ReorderRules(scenario, orderedUUIDs)`
   （稳定分区重排 + `saveLocked()`），同文件测试补重排/容忍追加/跨 scenario 拒绝/无 store 四个用例。
   验证：`go test ./internal/config -run Reorder`。
2. **HTTP 端点**：`internal/server/module/rule/` 的 `types.go`（请求/响应结构）、
   `handler.go`（handler）、`routes.go`（`POST /rules/reorder` + swagger 注解）+ handler 测试。
   验证：`go test ./internal/server/module/rule`；起服务后 `curl -X POST .../api/v1/rules/reorder` 重排一次。
3. **代码生成**：`task codegen`（openapi.json + `pnpm gen:api` 前端 SDK）。
   验证：`git diff openapi.json` 只含 reorder 一个端点；生成的 client 里出现 reorder 调用。
4. **前端 API 封装**：`frontend/src/services/rulesApi.ts` 加 `reorderRules(scenario, orderedUuids)`
   （codegen 未跑前先按 CLAUDE.md 留占位函数）。
   验证：`pnpm typecheck`。
5. **UI 控件**：`RuleCard.tsx` 头部 ↑/↓ + 置顶（`Pin`/`PinFilled` 图标；新 `allowReorder` prop，
   首条隐藏 ↑/置顶、末条隐藏 ↓）；`TemplatePage.tsx` 的 `handleReorder`（乐观本地重排 → 调 API →
   失败 toast + `loadRules` 回滚，name 排序视图下隐藏按钮；置顶 = 把目标 uuid 排到首位走同一提交）；
   直连页/Team/ImageApi 开启，slot 页（claude_code/profile）不传。
   验证：`pnpm typecheck && pnpm lint && pnpm test`。
6. **mock 与 i18n**：`frontend/src/mocks/handlers.ts` 加 `POST /api/v1/rules/reorder` mock；
   en/zh/ru 三份 locale 补 tooltip key（`common.moveUp/moveDown` 已存在直接复用，置顶 key 新增）。
   验证：`pnpm test`（`localeParity.test.ts` 把关三语言齐全）。
7. **端到端确认**：`pnpm dev:mock` 截图核对交互；真机验证四件事——重排/置顶后刷新与重启顺序保持、
   置顶后 `/v1/models` 返回顺序跟随（该模型在列表首位）、CLI `rule list` 顺序一致、
   name 视图无移动/置顶按钮。
