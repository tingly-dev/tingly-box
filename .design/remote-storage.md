# Remote 存储重设计

Remote 子系统（IM bot 远程控制）的状态持久化方案。记录现状问题、目标 schema、
分阶段迁移路径。

目录：

1. 现状盘点
2. 问题清单（带证据）
3. 设计原则
4. 目标 schema
5. 抽象与依赖方向
6. 数据迁移
7. 分阶段落地
8. 待决策事项

---

## 1. 现状盘点

Remote 的状态散落在四种载体上，只有 bot 设置进了主库：

| 数据 | 载体 | 位置 | 并发/持久性 |
|---|---|---|---|
| Chat（项目绑定、配对、白名单、bash cwd、当前 agent、agent state） | `pkg/jsonstore` 整文件 | `~/.tingly-box/bot_chats.json` | 多实例各持内存副本，整文件覆盖 |
| Session（执行会话 + 消息历史） | `pkg/jsonstore` 整文件 | `~/.tingly-box/bot_sessions.json` | 更新只置 dirty，不落盘 |
| SmartGuide 对话历史 | 裸 `os.WriteFile` | `<dataDir>/sessions/<chatID>-smartguide.json` | 每 chat 一个文件，0644 |
| Chat.AgentState / Session.Context | 随宿主结构 | 同上 | **零调用方，已删除** |
| Bot 设置 | SQLite / GORM ✅ | `db/tingly.db` → `imbot_settings` | 单连接 + WAL |
| Scenario bindings | SQLite 里的 JSON text 列 | `imbot_settings.scenarios` | 读-改-写整个 blob |
| Pairing code | 纯内存 | `imbot/security` | 重启即失 |
| Audit | 纯内存 | `remote/audit.Logger` | 从不自动落盘 |

也就是说：**主库已经在那儿了，remote 是唯一还在用 JSON 文件当数据库的子系统。**

## 2. 问题清单

### P0-1 多实例并发写整文件 → 静默丢数据

`runBotWithSettings` 里每个 bot 各自 `NewChatStoreJSON(dataPath)`
（`internal/remote_control/bot/manager.go:32`），而 `dataPath` 是全局共享的
`bot_chats.json`（`internal/server/module/imbot/manager.go:123`）。
`Manager.ChatStore()`（`manager.go:281`）给 CLI 又开一个。

`jsonstore` 只在 `New()` 时 `load()` 一次，之后各实例各持一份内存 map，
每次写都是 `MarshalIndent` 整个 map + rename 覆盖
（`pkg/jsonstore/store.go:162`）。结果：

> 开了 telegram 和 feishu 两个 bot。telegram 侧用户 `/cd` 绑了项目 →
> 写入文件。之后 feishu 侧任何一次写（配对、切 agent）都会用它启动时的
> 旧快照整体覆盖，telegram 那条绑定凭空消失。

跨进程（server + CLI 同时操作）同理，且没有文件锁。

### P0-2 Session 的更新根本不落盘

`Manager.Update()` → `store.Set()` 只把 `dirty` 置 true
（`remote/session/manager.go:288`, `pkg/jsonstore/store.go:231`）。真正写盘只发生在：

- `Create` / `CreateWithID`（显式 `ForceSave`）
- `Manager.Stop()` → `store.Close()`

也就是说状态流转（running → completed）、response、每一条 message，
在下一次「新建会话」或「优雅关闭」之前都只在内存里。进程被 kill / 崩溃 →
全部丢失，重启后看到的是一堆停在 `pending` 的僵尸会话。

附带：manager 靠 `if jsonStore, ok := m.store.(*SessionStoreJSON)` 类型断言
来触发落盘（`manager.go:136`, `manager.go:179`）—— 接口没抽干净，
换实现就悄悄失去持久化。

### P0-3 chatID 直接拼进文件名

`smart_guide.SessionStore.path()` = `filepath.Join(s.dir, chatID+"-smartguide.json")`
（`internal/remote_control/smart_guide/session_store.go:37`），chatID 直接来自平台。
Telegram 是纯数字，但飞书 `open_chat_id`、WhatsApp JID 含 `@`、部分平台含 `/`。
没有任何 sanitize → 路径穿越 + 文件名冲突。同一目录还是 0644（其他状态文件是 0600）。

### P1-1 全表扫描代替索引

所有查询都是 O(n) 线性遍历：`FindByChatAgentProject`、`ListByChat`
（`remote/session/json_store.go:84,111`）、`ListChatsByOwner`、`IsWhitelisted`、
`ListWhitelistedGroups`（`chat_store.go:446,483,559`）。

更糟的是写放大：`Session.Messages []Message` 内联在会话里，
append 一条消息要重新 marshal 整个 sessions 文件 → 消息数增长下是 O(n²)。

### P1-2 没有 schema，也没有迁移路径

`jsonstore` 的 `version` 只做「文件版本 > 代码版本就报错」，没有升级钩子
（`store.go:139`）。字段增删全靠 `omitempty` 碰运气。

`Session` 结构体连 json tag 都没有（`remote/session/manager.go:38-53`），
持久化的 key 是 `"ID"` / `"ChatID"` 这种 Go 字段名 —— 任何一次字段重命名
都是一次静默的数据丢失。

### P1-3 同一份状态有两个家

- `Chat.AgentState []byte`（chats.json 里的 blob，`chat_store.go:124`）和
  `smart_guide.SessionStore`（每 chat 一个文件）都在存会话/handoff 状态。
- `bot.BotSetting`（`chat_store.go:22`）和 `db.Settings`
  （`internal/data/db/imbot_settings_store.go:21`）是两个手工同步的结构体，
  已经漂移：`BotSetting` 有 `Token`/`Verbose` 而 `Settings` 没有 `Verbose`，
  `ImBotSettingsRecord` 有 `Debug` 而两个 DTO 都没有。
- `Chat.ProjectPath` + `Chat.ProjectHistory []string` 是同一个事实的两份表示，
  靠 `pushProjectHistory` 手工维持一致（`chat_store.go:387`）。

### P1-4 Scenario bindings 是 SQLite 里的 JSON blob

`imbot_settings.scenarios` 存一整个 binding 列表的 JSON 文本。
后果是 `remote/binding/binding.go` 里要手写 `rawBinding map[string]json.RawMessage`
解析器（`binding.go:194`）来保住未知字段，`SetScenarioEnabled` 要在原始
`[]map[string]json.RawMessage` 上做读-改-写（`binding.go:161`）才不丢字段，
`Resolver.Resolve` 要遍历所有 bot、逐个 parse blob 才能匹配一个
(scenario, event)（`binding.go:124`）。查询、约束、部分更新全都做不了。

### P2-1 审计不落盘 —— 已解决：删掉，不是修

`remote/audit.Logger` 只有内存环形缓冲（`logger.go:41`），
唯一的落盘入口是手动调用的、从未被生产代码调用的 `ExportJSONToFile`
（`logger.go:195`）。读侧（`GetEntries`/`GetLogs`/`GetEntriesByUser`/...）
同样零生产调用方——排查下来 `remote/audit` 整包只有写入，没有任何地方
读回来给用户看。而 pairing code reveal 这类「每次都审计」的安全事件
重启即蒸发。

原计划是给它建 `remote_audit` 表（见下方 schema）换取持久化，但那等于
维护第二套日志系统去解决"没有持久化"——而 `pkg/obs.MultiLogger` 已经是
持久化的（JSON 文件 + rotation + retention，见 `.design/logging-redesign.md`），
且已经有 `GET /api/v1/system/logs` 之类的出口。`remote/audit` 唯一自己的动作
`logToConsole`（`logger.go:219`）本来就是转发到 logrus。

所以实际落地是**删除 `remote/audit` 整个包**，全部约 15 处调用点
（pairing 成功/失败/锁定、bot panic、bot 交互 API、scenario 插件生命周期）
改成直接的结构化 `logrus.WithFields(...).Info/Warn(...)` 调用——这些日志
本来就通过 `logToConsole` 落进 logrus，现在只是去掉中间那层从不被读的
环形缓冲。`imbot/security.PairingAuditor` 接口保持不变（`NewPairingManager`
的注入点），生产实现从 `*audit.Logger` 换成新的 `security.LogAuditor`。

### P2-2 UX：这些状态在产品里不可见

`internal/server/module/imbot/routes.go` 全部是 bot 设置的 CRUD，
**没有任何接口能列出 chat、session、绑定历史或配对状态**。
用户在 UI 上看不到「这个 bot 现在配对了谁、绑在哪个项目、有哪些活跃会话」，
只能去 shell 里翻 JSON 文件。

对照 `.design/ux-principles.md`：违反「surface the artifact for the next action」
（配对/绑定这些产物是下一步操作的入口，却没有出口）和
「diagnostics must traverse the real path」（诊断「为什么 bot 不回我」时
真实路径上的状态全是黑箱）。这条是这次重设计真正的产品动机 ——
存储换成表之后，这些出口才做得出来。

## 3. 设计原则

**最重要的一条：按访问模式选介质，不是「全进 SQLite」。**

原来的缺陷不是「用了文件」，而是「**一个**文件装所有会话，并且每次写整体重写」。
per-session 的追加文件没有任何一条这样的毛病。所以分界线是：

| | 去 SQLite | 去文件 |
|---|---|---|
| 形态 | 小、有界、字段固定 | 大、无界、只追加 |
| 访问 | 按条件查询、排序、join、扫表清理 | 写一次、整体读、从不按内容查 |
| 例子 | 会话**索引**（绑定、状态、时间戳）、chat 状态 | 会话 **transcript**、SmartGuide 对话历史 |

把 transcript 塞进 SQLite 的代价是实打实的：每次追加变成一次对全产品共享库的事务，
库文件随对话文本无界膨胀（而没有任何查询用得上这些文本），
用户也失去了 `tail` / `grep` / 附到 bug 报告里的能力。
而 tingly-box 恰恰要和 Claude Code 的 on-disk session 对齐 ——
`Manager.CreateWithID` 就是为了让 remote 会话 id 和 Claude 的 session_id 一致，
从而支持 `--resume`。两半对话理应是同一种产物。

其余原则：

1. **单库单连接。** 需要进库的部分并入已有的 `internal/data/db.StoreManager`
   （单 `*gorm.DB` + WAL + busy_timeout + AutoMigrate）。
2. **一份状态一个家。** 同一个事实不要有两处表示。
3. **SQLite 支持 JSON，不必强行展平。** 真正开放或只整体读写的字段
   （binding options、chat 的 project history）用 JSON 列就好 ——
   拆成表只有在真的要按它查询时才划算。
4. **每张表都要有出口。** schema 落地的同时给 API + UI，否则只是把黑箱换了个格式。
5. **死代码直接删，不要给它搬家。** 迁移时逐个确认调用方；发现零调用方的字段
   （`Chat.AgentState`、`Session.Context`）就删掉，而不是费力给它找新介质 ——
   否则等于把历史包袱重新实现一遍。

## 4. 目标 schema

```
remote_chats
  chat_id            TEXT PK
  platform           TEXT
  project_path       TEXT
  owner_id           TEXT          idx(owner_id, platform)
  is_paired          BOOL
  paired_bot_uuid    TEXT          idx
  paired_sender_id   TEXT
  paired_at          DATETIME
  is_whitelisted     BOOL          idx
  whitelisted_by     TEXT
  bash_cwd           TEXT
  current_agent      TEXT
  verbose            BOOL NULL     -- 三态保留
  created_at / updated_at

remote_chat_projects              -- 取代 Chat.ProjectHistory []string
  chat_id            TEXT          idx(chat_id, last_used_at DESC)
  project_path       TEXT
  last_used_at       DATETIME
  PK(chat_id, project_path)        -- 去重变成主键，上限变成 DELETE

remote_sessions
  id                 TEXT PK
  chat_id            TEXT
  agent              TEXT
  project            TEXT
  status             TEXT          idx(status)
  request / response / error  TEXT
  permission_mode    TEXT
  created_at / last_activity / expires_at
  idx(chat_id, agent, project, last_activity DESC)   -- FindByChatAgentProject 的冷路径

remote_bindings                   -- 取代 imbot_settings.scenarios JSON 列
  bot_uuid           TEXT          idx(bot_uuid)
  scenario           TEXT          idx(scenario)
  chat_id            TEXT
  enabled            BOOL NULL     -- 三态保留（nil = 兼容旧行为「视为开」）
  events             JSON
  options            JSON          -- 真正开放的部分，保持 JSON
  PK(bot_uuid, scenario)

```

`remote_audit` 表**不做**——见上方 P2-1：`remote/audit` 整包已删除，
安全/审计事件走常规 logrus（`pkg/obs.MultiLogger`，已持久化），
不需要第二套日志系统。

`Resolver.Resolve` 从「遍历所有 bot × parse blob」变成
`WHERE scenario = ? AND enabled IS NOT FALSE`。

### 不进库的部分

```
<configDir>/remote/transcripts/<session-id>.jsonl   -- 会话 transcript
  每行一条 {"Role":...,"Content":...,"Summary":...,"Timestamp":...}
  O_APPEND 追加；会话间零争用；可 tail/grep
  session-id 不安全时哈希（/resume 的 id 来自用户输入）

<dataDir>/sessions/<chatID>-smartguide.json         -- SmartGuide 对话历史
  anthropic message 数组，单 chat 可能 MB 级
  P0 已修掉路径穿越与 0644 权限
```

两者都是「写一次、整体读、从不按内容查」的无界数据，留在文件里是正确的，
不是待办。数据库里只保留能定位到它们的索引。

## 5. 抽象与依赖方向

- `remote/store`（新包）只定义领域接口：`ChatStore` / `SessionStore` /
  `AgentStateStore` / `BindingStore` / `AuditStore`。
- 实现落在 `internal/data/db`，由 `StoreManager` 统一初始化并注入。
- **`remote/*` 不 import gorm，也不再接受文件路径参数** —— 现在
  `NewChatStoreJSON(filePath)` / `NewSessionStoreJSON(filePath)` 这种签名
  把存储介质泄漏给了调用方，是「per-bot 各开一个实例」的根因。
- 删掉 `manager.go` 里的 `*SessionStoreJSON` 类型断言：写入即事务提交，
  `ForceSave` 这个概念消失。
- `bot.BotSetting` 与 `db.Settings` 合并成一个类型，消除手工同步。
- 现有 `ChatStoreInterface`（`chat_store.go:135`）方法签名基本可以原样保留，
  这让 P1 能做到「换实现不改调用方」。

## 6. 数据迁移

复用 `internal/data/db/migrations/` 的既有模式（见 `migrate_imbot_credentials.go`）：

- 启动时一次性 importer：`bot_chats.json` / `bot_sessions.json` /
  `<dataDir>/sessions/*.json` → 建表 → 逐条 upsert。
- 幂等：以 migration marker 行判定，重复启动不重复导入。
- 旧文件**重命名**为 `.migrated` 而非删除，留一个回滚窗口。
- 导入失败不阻塞启动：记 error 日志 + 保留原文件，让用户能拿到数据。
- `Session` 没有 json tag，导入时按现有的大写 key（`"ID"`/`"ChatID"`…）读，
  这段读逻辑随迁移代码一起在若干版本后删除。

## 7. 分阶段落地

**P0 · 止血**（不改 schema，可独立合入、独立发版）✅ 已完成
- `Manager` 持有唯一 `ChatStore` 实例，`runBotWithSettings` 接收而非新建 → 修 P0-1
- session 写入后立即持久化；顺带修好从未被调用的 `sessionMgr.Stop()` → 修 P0-2
- `smart_guide.SessionStore` 文件名 sanitize/hash + 0600 → 修 P0-3

P0 修不掉的部分（留给 P1）：**跨进程**的整文件覆盖。共享实例只在单进程内有效，
CLI 的 `remote run`（`internal/command/remote.go` 的 standalone 路径，单进程单 bot）
与 server 同时操作同一份 `bot_chats.json` 时仍会互相覆盖 —— jsonstore 没有文件锁，
也没有写前重读。这个只有换到 SQLite（WAL + busy_timeout）才真正解决。

**P1 · 落表** ✅ 已完成
- `remote_chats` / `remote_sessions` 两张表 + AutoMigrate，
  并入 `StoreManager`（`RemoteChats()` / `RemoteSessions()`）
- 一次性 importer（`migrations.ImportRemoteJSONStores`），旧文件重命名 `.migrated`
- `ChatStoreInterface` 签名不变；`Manager` 改为**注入** store，不再持有文件路径 ——
  P0 的「共享单实例」从此是结构性的，不是要靠人记住的约定
- 消息**不进库**：改为 per-session 的 append-only JSONL
  （`<configDir>/remote/transcripts/<id>.jsonl`），O(1) 追加、按需读取；
  `Session` 不再内联 `Messages`，`List()` 预热 manager 时不会拖出全部对话文本
- CLI 两条路径（`remote pair revoke`、standalone bot）也接到同一个库上，
  **跨进程覆盖随之消失**（P0 遗留问题在此关闭）
- `pkg/jsonstore` 与两个 JSON store 实现全部删除（零调用方）

收敛（评审后）：
- 删掉 `Chat.AgentState` 和 `Session.Context` —— 逐个确认后都是**零生产调用方**。
  前者是「同一类状态三个家」里的一个，后者只装 `project_path`，
  而 `Project` 已经是独立列。删掉比搬家正确。
- 文件名净化收敛成 `pkg/fs.SafeFileKey`，原来 transcript 和 SmartGuide 各写了一份。
- 删掉 `remote_open.go` 这套平行开库路径，CLI 与测试统一走已有的
  `db.NewStoreManager`；两个 store 的 `owned` 所有权标志随之消失，`Close()` 变成纯 no-op。
- 修正一处过度包装：session 索引的价值**不是**「一次索引查询」——
  `Manager.FindBy` 先扫内存 map，store 查询只是冷路径。真实理由是
  小的可变状态 + 跨进程并发写 + P3 需要列表。

代码评审后的修正：
- **迁移下沉到 `StoreManager.initRemoteStores`。** 原来挂在 `NewBotManager` 上，
  于是只有 server 路径会迁移 —— 用户在启动 server 之前跑
  `tingly-box remote pair revoke`，会读到一张空表而 `bot_chats.json` 就在旁边。
  存储生命周期的拥有者才该触发迁移。（这也是 importer 从 `migrations` 包
  移进 `db` 包的原因：`migrations` import `db`，反向会成环。）
- **迁移对崩溃可重放。** 先写 transcript、最后写索引行 —— 索引行才是
  「已导入」的标记，中途挂掉就没有行，下次整体重做；反过来会永久留下
  一个被标记为已导入、但历史被截断的会话。
- **四处「GetOrCreate 然后 Update」折叠成一个事务**（`mutate`）。原来每次绑定/
  配对/白名单/切 agent 都是两个事务、读两遍行，新建时还写两遍（第一次立刻被覆盖）。
- **启动预热加了边界**：`List()` 跳过 closed/expired。`Manager.Close` 会留下
  closed 行且永不删除，原来每次启动都要全部载入。行为不变 —— `FindBy` 本就
  忽略这两种状态，`GetOrLoad` 仍能按 id 取到任何会话。
- `ImportChat` 保留原 `UpdatedAt`（与 session 的 `Import` 对称），
  否则迁移后所有 chat 在新的列表 UI 里显示为同一时刻活跃。
- 删除：`ChatStoreInterface.Close()`（两个实现都是 no-op，所有权靠注释维持）、
  `ListWhitelistedGroups`（无生产调用，匿名 struct 声明了三遍）、
  `ErrStoreNotInitialized` / `ErrChatNotFound`（随 JSON store 一起死了）、
  `HealthCheck` 漏掉的两个新 store（`TotalStores` 改为从被检查集合推导）。

**已知取舍（评审提出，故意不改）**：单条入站消息现在会对同一 chat 行发出
约 12 次 SELECT（旧的 jsonstore 从内存 map 直接读）。绝对开销在本地 SQLite +
消息级流量下可忽略。两种修法都不可取：加缓存会破坏这次刚修好的跨进程可见性，
把 `*Chat` 一路传下去要改十来个 diff 之外的文件。留待 P3 做 API 时一并处理。
- GORM 的 struct `Updates` 会跳过零值 —— `RemoveFromWhitelist` / `ClearPaired`
  这类「把标志位关掉」的写入会静默失败。统一改走 `OnConflict{UpdateAll}` upsert。
- `Set` 会刷新 `LastActivity`，直接用于导入会把所有休眠会话变成「刚活跃」，
  打乱 `FindByChatAgentProject` 的排序和 retention。另开不改时间戳的 `Import` 路径。

**P2 · 收尾**
- `scenarios` JSON blob → `remote_bindings` 表。这个值得拆，
  因为 `Resolver.Resolve` 真的要按 (scenario, event) 跨 bot 查询。
- `BotSetting` 与 `db.Settings` 合并（消除已漂移的重复结构体）
- **`Chat` 移到中立包（评审提出的最深一条）。** 现在 `Chat` 住在 `db` 里、
  `bot.Chat` 是别名，纯粹为了破环。而 session 那半边做对了：`session.Session`
  留在 `remote/session`，由 `db` import 它 —— 实现依赖领域。chat 之所以反了，
  只因为 `bot` 已经 import `db`，而那个 import 又只是为了 `db.Settings`
  的类型断言（正是上面这条要消灭的东西）。代价已经具体：`DefaultChatAgent`
  与 `PushProjectHistory` 这些领域逻辑住进了存储包。
  正解是本文档 §5 说的 `remote/store` 中立包，与 `remote/session` 对称。

**明确不做**（按第 3 节的分界线，这些留在原地是对的，不是待办）：
- `ProjectHistory` 拆表 —— 只整体读写、上限 20 条，JSON 列足够
- session transcript / SmartGuide 历史进库 —— 无界、只追加、从不按内容查

**P3 · 出口**（真正兑现 UX 动机）
- `GET /api/v1/remote/chats`、`/remote/chats/:id`、`/remote/sessions`
  + swagger 定义 + `task codegen`
- UI：bot 详情里的「当前状态」面板 —— 配对了谁、绑到哪个项目、活跃会话、最近安全事件
  （安全事件走常规日志页，不是独立的 audit 出口 —— 见 P2-1）

**已解决，不再是待办**：
- **`/clear` 的语义统一。** @cc/@mock 的 `/clear`
  （`internal/remote_control/bot/bot_command.go:handleClearCommand`）本来就是
  软删除：`sessionMgr.Close` 把旧 session 标 `closed` 后持久化、逐出内存，
  transcript 文件不动，下一条消息 lazily 建一个新 session（新 UUID）——
  「关闭旧的、不是抹掉」的语义已经成立。@tb（SmartGuide）不是：
  `tbSessionStore.Delete(chatID)` 直接删文件。已改为
  `SessionStore.Clear`：把活跃的 `<chatID>-smartguide.json`
  rename 成带时间戳后缀的归档文件而不是删除，画风和 @cc 对齐——
  「clear = 停用当前 session、旧的留作日志」，不需要额外建审计层，
  这份归档文件本身就是日志。SmartGuide 历史进不进库仍是待决策事项 #2，
  与这次的改动无关（还是文件，只是不再是"唯一一份、可被清空"）。
- audit 落盘 —— 见 P2-1，决定是删除 `remote/audit` 而不是给它建表。

## 8. 待决策事项

1. **范围**：只做 P0+P1，还是一路做到 P3？
2. **SmartGuide 历史进不进库**：单 chat 的 anthropic message 数组可能到 MB 级。
   倾向进 `remote_agent_state` + retention；另一选择是留文件但只修 sanitize。
3. **迁移策略**：静默自动迁移，还是保留旧文件只读回退窗口 + 显式提示？
4. **`pkg/jsonstore` 是否直接废弃**：全迁完就只剩零调用方。
