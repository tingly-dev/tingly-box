# Rule 存储迁移：config.json → SQLite

Rule（路由规则）从 config.json 的 JSON 数组迁入主库 `tingly.db` 的 `rules` 表。
记录动机、领域模型与持久化模型的分层、列 vs 胖字段的取舍、迁移路径与不变量。

目录：

1. 动机
2. 分层：领域模型 / 内存工作集 / 持久化
3. Schema：哪些是列，哪些是胖字段
4. 写路径收口：Save() 即同步
5. 一次性迁移与权威性规则
6. 顺序语义（position 与 DefaultRequestID）
7. 不变量与边界情况
8. 未来扩展

---

## 1. 动机

迁移前 rule 是 `Config.Rules []typ.Rule`，整个数组随 config.json 序列化：

- **整文件覆盖写**：任何一条 rule 的小改动都重写整个 config.json，与
  token、scenario、profile 等无关配置互相放大写冲突面。
- **无行级身份**：usage 记录、stats 已经在 SQLite 里按 rule UUID 关联，
  而 rule 本身却在文件里，跨存储 join 只能全量加载后在内存做。
- **手改文件与运行时互踩**：watcher 热加载会把手改的 rules 数组整个灌回
  内存，与 API 写入竞争，谁后写谁赢。
- **provider 已迁完**：providers 走过完全相同的路径（`migrateProvidersToDB`），
  rule 是 config.json 里最后一大块"实体型"数据。

## 2. 分层：领域模型 / 内存工作集 / 持久化

```
typ.Rule (领域模型，不变)
   ↑↓ 内存中直接使用
Config.Rules []typ.Rule (内存工作集，json:"-")
   ↑ 启动时 hydrate        ↓ Save() 写穿
db.RuleRecord / rules 表 (持久化权威)
```

三个决定：

**`typ.Rule` 不动。** API、swagger、前端、路由匹配全部继续使用领域模型；
持久化形态（`db.RuleRecord`）是独立的 DTO，转换函数在 store 内部。
未来改表结构不触碰领域模型，反之亦然。

**内存工作集必须保留，不能改成每次查库。** 两个硬理由：

- 热路径：每个请求都要 `MatchRuleByModelAndScenario`，内存 RLock 扫描远快于
  SQLite 往返（mattn/go-sqlite3 还有全局锁竞争）。
- 正确性：`loadbalance.Service.Stats` 是运行时统计对象，由
  `StatsStore.HydrateRules` 水合到**内存里的 Service 指针**上，负载均衡
  实时读写。若每次从库里重建 Rule 对象，stats 就丢了。

**数据库是重启后的唯一权威。** config.json 从此不含 rules（写 `"rules": null`），
手改文件不再能引入 rule（热加载时忽略并告警）。

## 3. Schema：哪些是列，哪些是胖字段

判断标准只有一条：**查询/排序/唯一性真正用到的才是列，其余进 JSON 胖字段。**

```
rules
├─ uuid           TEXT PK      -- 身份；与 usage/stats 的 rule_uuid 对齐
├─ position       INTEGER idx  -- 列表顺序（见 §6）
├─ scenario       TEXT ┐
├─ request_model  TEXT ┘ 复合索引 idx_rules_scenario_request_model
│                              -- 路由键：MatchRuleByModelAndScenario 的两个谓词
├─ response_model TEXT         -- 展示/改写用，轻查询
├─ description    TEXT
├─ active         BOOL         -- 列表过滤
├─ smart_enabled  BOOL         -- 列表过滤
├─ services       TEXT(JSON)   -- []*loadbalance.Service（serializer:json）
├─ flags          TEXT(JSON)   -- typ.RuleFlags（serializer:json）
├─ lb_tactic      TEXT(JSON)   -- typ.Tactic（serializer:json）
├─ smart_routing  TEXT(JSON)   -- []smartrouting.SmartRouting（serializer:json）
├─ created_at / updated_at
```

四个 JSON 列直接在 `RuleRecord` 上声明为**类型化字段 + `serializer:json`**
（跟随 internal/db 的 serializer 迁移，见 .design/db.md）：
编解码由 GORM serializer 完成，store 里没有手写 marshal/unmarshal。与
ProviderStore 不同，RuleStore 不做记录缓存——每次读取由 serializer 分配全新
值，天然无共享可变状态，因此也不需要 provider 那套 clone-at-boundary 助手。

胖字段的理由：

- **services / smart_routing**：深嵌套数组，只会随 rule 整体读写，永远不需要
  "查所有引用 provider X 的 service" 这种谓词——真需要时 SQLite 的
  `json_each` 也能兜底，不必现在拆表。拆成子表会引入外键、顺序列、
  两阶段写，换不来任何现有查询的收益。
- **flags**：字段每隔几周就加一个（cursor_compat、thinking_effort、
  session_affinity、vision_proxy…）。做成列意味着每个新 flag 都动 schema；
  JSON 胖字段让 flag 演进完全停留在 `typ.RuleFlags` 一处。
- **lb_tactic**：带 `Params interface{}` 的和类型，天然 JSON。

序列化格式与 config.json 时代逐字段一致（同一套 json tag），所以迁移是
无损重编码，零值的归一行为也与旧文件存储相同（如未设置的 tactic 序列化为
"random"——文档化的默认值）；`Service.Stats` 带 `json:"-"`，运行时统计
不会渗入持久化。变更检测（SyncAll 的逐行比较）用**往返规范化 JSON**
（marshal→unmarshal→marshal）：解码会归一的编码差异（`params: null` vs
`{}`）不会被误判为"变了"。

## 4. 写路径收口：Save() 即同步

rule 的变更入口非常多：AddRule / UpdateRule / DeleteRule / SetRequestConfigs /
profile 创建删除 / SmartGuide ensure / 十几个日期迁移……但它们有一个共同点：
**改完 `c.Rules` 之后全部调用 `c.Save()`**。

所以同步点就放在 `Save()` 里：

```go
func (c *Config) Save() error {
    // ... 序列化 config.json 内容（不含 rules）...
    if err := c.syncRulesToStore(); err != nil {  // 先库
        return err
    }
    return os.WriteFile(c.ConfigFile, out, 0644)  // 后文件
}
```

**顺序不变量：先写库，后写文件。** 一次性迁移时文件写入会把 rules 数组
替换为 null；若先清文件、后写库且写库失败，就出现"文件已清空 + 库是空的"
的丢数据窗口。先库后文件则两种失败都安全：写库失败 → 文件仍带旧 rules，
下次启动重试导入；写库成功但写文件失败 → 库已权威，下次启动清理陈旧 JSON。

- 不需要逐个改造调用点，也**不可能**出现"改了内存忘了落库"的路径——
  忘了调 Save() 的代码在旧世界同样丢数据，语义没有变差。
- `syncRulesToStore` 先做 JSON 快照比对（`lastSyncedRules`），rules 没变的
  Save()（改 token、scenario 等）完全跳过数据库。
- `RuleStore.SyncAll` 在单事务里做 upsert + 删除 + position 重排，且逐行
  与现有 payload 比对，没变的行不写——`updated_at` 只在真实变更时移动。

rule 数量级是几十条，全量 diff-sync 每次毫秒级；换来的是绝对的简单和
缓存/库不可能漂移。等将来 rule 上万条再谈增量（见 §8）。

## 5. 一次性迁移与权威性规则

完全复刻 provider 迁移的模式（`migrateProvidersToDB`）：

```go
// Config 结构体
Rules       []typ.Rule `yaml:"rules" json:"-"`     // 内存工作集
LegacyRules []typ.Rule `yaml:"-"     json:"rules"` // 只为加载旧文件而存在
```

启动序列（`NewConfig`）：

```
StoreManager 初始化（含 rules 表 AutoMigrate）
load() / CreateDefaultConfig()      -- 旧文件的 rules 进 LegacyRules
hydrateRulesFromStore():
    库里有 rule   → Rules = 库；LegacyRules 若非空则视为陈旧备份，清掉
    库空 + 有遗留 → Rules = LegacyRules；缺 UUID 的补随机 UUID；Save() 落库
    双空          → 全新安装，内建 rule 稍后由 InsertDefaultRule 走 AddRule 进来
Migrate(cfg)                        -- 日期迁移在真实规则集上跑，改动经 Save() 落库
InsertDefaultRule / RefreshStatsFromStore ...
```

关键点：

- **hydrate 在 Migrate 之前**。日期迁移（内建 rule 身份归一、smart-routing
  清理等）必须作用于库里的规则集，其结果又经 Save() 写回库。
- **hydrate 不受 `WithDisableMigration` 控制**——它是存储管道，不是配置迁移，
  和 `migrateProvidersToDB` 同一待遇。
- **`rulesHydrated` 门闩**：hydrate 之前的任何 Save()（如 CreateDefaultConfig）
  不会同步 rules。这保证了"config.json 被删但库还在"的场景不会用空列表
  把库清空。
- `LegacyRules` 的 json tag 故意**不带 omitempty**：清空后 Save() 写出
  `"rules": null`，压过 Save() 的 merge-unknown-keys 逻辑保留的旧值，
  旧数组从文件里彻底消失。
- 热加载（watcher → `load()`）时若文件里又出现 rules 数组：告警并忽略。
  文件不再是 rule 的输入面，UI/API 才是。

## 6. 顺序语义（position 与 DefaultRequestID）

`DefaultRequestID` 是**下标**语义的默认 rule 指针，且仍留在 config.json。
为了让它跨重启稳定，表里的 `position` 列忠实记录内存切片的顺序：
SyncAll 时 `position = i`，List 时 `ORDER BY position`。

追加、删除、`SetRequestConfigs` 整体重排都自然正确——顺序永远是"上次
Save() 时内存里的顺序"，与 config.json 时代逐字节等价。

（`DefaultRequestID` 用 UUID 取代下标是值得做的后续清理，但那是行为语义
变更，不塞进本次存储迁移。）

## 7. 不变量与边界情况

- **UUID 是主键**：修复策略只有一处——`ensureRuleUUIDs`（补空 UUID、
  给重复 UUID 重新赋值），`normalizeRuleBasics` 每次启动执行、一次性迁移
  在首次落库前执行。SyncAll 里的跳过+告警是纯防御，正常情况下永不触发。
- **腐坏的胖字段不阻断启动**：单字段解码失败记 warning、字段取零值，
  rule 仍可从 UI 修复。拒绝启动的服务器比缺了 tactic 的 rule 伤害大。
- **轻量 Config（测试直接构造，无 store）**：`ruleStore == nil` 时
  hydrate/sync 均为 no-op，退化为纯内存行为，存量测试不受影响。
- **多进程**：与其它 store 一致依赖 WAL + busy_timeout；rule 写本来就
  低频（人工操作），无新增风险面。
- **scenario+request_model 不做库级唯一约束**：应用层校验保留在
  AddRule/UpdateRule（wildcard、历史脏数据、跨 scenario 复名都有合法场景），
  硬约束只会让存量数据迁移失败。

## 8. 未来扩展

- **查询下推**：列已备好（scenario、request_model、active），当 rule 数量
  或调用方（企业版多租户）需要时，可加 `ListByScenario` 等谓词查询而不动
  schema。
- **增量写**：若 rule 规模增长到全量 diff-sync 不可接受，把 Save() 收口
  拆成显式的 store 粒度操作（AddRule → store.Save 单行）；RuleStore 的
  接口（Save/Delete/GetByUUID 语义）已按这个方向预留。
- **DefaultRequestID → default rule UUID**：摆脱下标语义，之后
  `position` 只服务展示排序。
- **flags 谓词查询**：真出现"找出所有开了 X flag 的 rule"需求时，用
  SQLite `json_extract` 表达式索引即可，无需拆列。
