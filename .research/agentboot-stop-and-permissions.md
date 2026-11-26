# agentboot 会话停止 / 恢复，以及 workspace 权限的实时生效

> 状态：分析（2026-11-26）。A（停止）以方案为主，但**A3 任务台账**已实现；B（权限）的“改完不生效”已定位到两个根因，其中**线程级权限在运行中回合不生效**、**规则撤销的可见性**已做最小修复并有测试；**项目级权限只在新线程生效**属于产品决策（§2.6）。
> 验证：Claude Code 2.1.259，离线脚本化 Anthropic mock + 隔离 HOME（`.research/agentboot-stop/stop_probe.py`、`mode_probe.py` 及各自的 `.txt` 运行记录）；Go 单测 `agentboot/engine/permission_live_test.go`。
> 相关：[`.design/agentboot-live-control.md`](../.design/agentboot-live-control.md)（注入、模型/模式的在线控制）、[`.design/claude-code-session.md`](../.design/claude-code-session.md)（常驻进程、resume）、[`projects/.design/thread-lifecycle.md`](../projects/.design/thread-lifecycle.md)（线程状态机）、[`projects/.design/background-tasks.md`](../projects/.design/background-tasks.md)（后台任务）、[`projects/.design/thread-permissions.md`](../projects/.design/thread-permissions.md)（`request_permission` 与 GitHub 闸门）。

## 0 结论摘要

**A. 停止**

- 代码里的“停止回合”**已经**是优雅的：取消 ctx → `interrupt` 控制请求 → 回合以 `error_during_execution` 结束 → **进程和后台任务都活着**。实测（§1.2）证明后台任务在 interrupt 后照常跑完并发出 `task_notification`。
- 真正丢东西的是另外几条路：① interrupt 不被响应时，15 秒后 `Close()`（关 stdin + 5 秒后 Kill）；② “停止会话”级操作（Restart / Resolve / Archive / Handoff / 暂停项目 / 退出应用）总是关进程，后台任务随之结束；③ **正在前台跑的 subagent / 工具调用**在 interrupt 时被中止，这部分任何方案都救不回来。
- 因此 A 的问题不是“只能杀进程”，而是**没有“保留/可续”的中间态**：后台任务只在进程活着时存在，而能关进程的操作又都不先给它们一个收尾。

**B. 权限**

- “workspace permission” 指线程的 permission mode（线程详情栏“权限”），另有项目默认 `thread_permission_mode`。
- 根因 1（项目级）：项目设置里的“线程权限”在**建线程时被复制**进每个线程，`threadPermissionMode` 优先用线程自己的值；改项目设置只给协调者发一条 system 事件，**已有线程完全不变**，UI 也没有说明（对比“线程默认模型”的帮助文字明确写了“已有线程保持各自的模型”）。
- 根因 2（线程级）：模式只在**下一轮“有消息”的回合开始时**由 `applyLive` 下发；运行中的回合、以及后台任务结束触发的**非请求回合**（unsolicited turn）都拿不到。`bypassPermissions` 的自动批准包装器也是在回合开始时按快照选择的，收紧后本回合仍会继续自动批准。
- 根因 3（规则级与方向性）：`set_permission_mode` 对运行中回合立即生效（实测切到 `plan` 当场拦下本回合的下一个命令），但**放宽到 `bypassPermissions` 会被 CLI 拒绝**（未以 `--dangerously-skip-permissions` 启动），只能下一轮重启生效；`apply_flag_settings` 只能**加**规则，进程启动设置里的 allow 规则必须重启才能撤销。所以“收紧模式”能实时，“规则收紧”和“放宽到 bypass”做不到实时。
- 已修：线程级模式变更现在即时 `set_permission_mode` 下发（不重启），失败则记一条事件、下一轮重试；自动批准改为每次批准时读取当前模式。安全边界与残余风险见 §2.5。

## 1 问题 A：停止会话

### 1.1 现状链路（谁发信号、发什么）

按“用户点 Stop”走一遍：

| 步骤 | 位置 | 行为 |
|---|---|---|
| 前端按钮 | `frontend/packages/projects/src/components/project/threadOps.ts:43` | `POST /api/v1/threads/{id}/stop` |
| 路由 | `projects/internal/server/module/thread/handler.go:630` | `Threads.Stop(cur.ID, rt.Source{})` |
| 线程层 | `projects/internal/project/threads.go:1500` → `1541` `stop` | 先 `stopQueued`（`1547`）：标记 `stopped/stoppedBy`、**丢弃排队消息**、取消“等并发槽位”的 ctx；再 `engine.Stop(session)` |
| 引擎层 | `agentboot/engine/engine.go:239` | 标记 `stopped`、取消**回合 ctx**；非请求回合（Claude 自己在后台任务结束后发起的）额外直接 `Interrupt`（`:248-254`） |
| 常驻进程层 | `agentboot/engine/turn.go:292` `res.c.RunTurn(ctx,…)` → `agentboot/conductor.go:188` `wait` | ctx 结束时发 `interrupt` 控制请求（`:200`），等 `SessionCloseTimeout`=15s（`agentboot/options.go:15`） |
| 兜底 | `agentboot/conductor.go:209-212` | 15s 内没结束 → `session.Close()` |
| 关进程 | `agentboot/persistent_session.go:404` / `425` | 关 stdin → 5s（`agentboot/runner.go:30`）→ `Kill()`（`agentboot/process/osexec.go:140`） |
| 落盘 | 事件逐条 `Store.Append`（`agentboot/engine/resident.go:64-88`）；状态 `SetInterrupted` + “interrupted; send a message to resume”（`agentboot/engine/turn.go:425-429`） | 线程回到歇着（`RestingStatus`，见 thread-lifecycle §3.1） |

“停止会话”的其它入口（都会关闭进程，因而结束后台任务）：

| 入口 | 位置 | 效果 |
|---|---|---|
| Restart | `projects/internal/project/restart.go:17` → `engine.Restart`（`engine.go:411`） | 停回合 + `Close` + `pool.Remove`，下一步 `--resume` |
| Resolve / Archive | `threads.go:1661` `endProcess` → `engine.Evict`（`engine.go:475`）, `1661-1700` | 停回合、`Close`、收回工具 URL |
| Handoff（交给终端） | `projects/internal/project/launch.go:197` → `engine.Release`（`engine.go:370`） | `Close` 后返回 `claude --resume <sid>` 命令 |
| 暂停 / 归档项目 | `projects/internal/project/service_ops.go` `SetPaused`/`SetArchived` → `Threads.ReleaseProject`（`threads.go:1605`） | 对每个线程 `stop` + `Evict` |
| 退出服务 | `projects/internal/project/service.go:148` → `Threads.Shutdown`（`threads.go:356`）→ `Engine.Shutdown`（`engine.go:489`）→ `pool.Shutdown` | 关闭所有常驻进程 |
| 空闲回收 / 容量 | `agentboot/pool/pool.go`（IdleTimeout 65 分钟、MaxSessions） | 静默 `Close` |
| 启动参数变了 | `agentboot/engine/turn.go:248-255`（`launchSignature` 不匹配）→ `Evict` | 静默重启 |

重启后的恢复：启动时 `Threads.Recover()`（`threads.go:1938`）把 working 改成 waiting，原因 “Interrupted when tingly-project stopped. Send a message to continue.”（`threads.go:1932`）；下一条消息以 `resume = t.Turns > 0` 启动（`threads.go:1109`），用 `--resume` 续上同一个 Claude 会话。崩溃（非主动 Stop）还有一层：`CLAUDE_CODE_RESUME_INTERRUPTED_TURN` 让 CLI 自己重放被打断的回合（`agentboot/engine/turn.go:347-364`）。

### 1.2 实测：interrupt 到底丢什么（离线、真实 CLI）

`stop_probe.py` 用脚本化 Anthropic mock（127.0.0.1）+ 隔离 HOME 跑真实 `claude`（2.1.259）：第一轮让模型用 `run_in_background` 起一个后台任务，第二轮跑一个 30 秒的前台命令并在 2.5 秒后发 agentboot 同款的 `interrupt`。完整日志见 `stop_probe.txt`，要点：

```
3.81 RECV control_response {"subtype":"success","request_id":"probe-interrupt","response":{"still_queued":[]}}
3.83 RECV user [{"tool_result","content":"The user doesn't want to proceed with this tool use… STOP what you are doing…"}]
3.83 RECV user [{"text":"[Request interrupted by user for tool use]"}]
3.83 RECV result #2 subtype=error_during_execution is_error=True
5.80 进程仍活着；set_model 控制请求 success
7.32 新用户消息照常进入模型（main#4）
8.58 task_notification status=completed （第一轮的后台任务跑完了）
10.5 bg.txt 存在
```

结论：

- `interrupt` 立刻（~30ms）结束回合，返回 `error_during_execution`；**进程不退出**，还能接控制请求与下一条消息。
- **后台任务不受影响**，照常完成并上报。
- 正在执行的前台工具调用（subagent 同理）被中止，工具结果变成“用户拒绝”，**这部分工作不可恢复**（CLI 不会把它算成可重放）。
- 所以代码里的 Stop 本身已经是“优雅停止”。如果实际观察到“点 Stop 后进程没了”，能解释它的只有两条：§1.1 表中那些**会话级**入口（Restart/Archive/退出等），或 interrupt 15 秒不响时的兜底 `Close`——这两条之外，代码路径不会因为 Stop 关进程。

> UNKNOWN：真实 CLI 若正在跑一个**前台 subagent** 而不是前台 Bash，interrupt 的时序是否同样在 15s 内结束（subagent 内部还有一层模型调用）。从 CLI 语义（等价 Esc/Ctrl+C）推断应该会，但未实测。

### 1.3 什么会丢，什么是可恢复的

| 内容 | interrupt（Stop 回合） | 关进程（Restart/Resolve/Archive/Pause/退出/15s 兜底） |
|---|---|---|
| 对话记录（含已流式输出的文本/工具结果） | 保留 | 保留：引擎逐事件写库；CLI 自己的会话文件也在，`--resume` 续上 |
| 本轮尚未输出的收尾文本 | 丢失（回合被中止） | 丢失 |
| 正在执行的前台工具 / subagent | **丢失，不可恢复** | 丢失，不可恢复 |
| 后台任务（`run_in_background` bash、后台子 agent） | **保留并继续**，结束后正常 `task_notification` | **随进程结束**；输出文件可能还在，任务本身不可续 |
| 待答审批 / 提问 | 过期 | 过期（重启后 `Recover` 把卡片标 expired） |
| 排队消息 | 丢弃（`stopQueued`） | 丢弃 |

### 1.4 已有但没接上的机制

- `Interrupt` / `StopTask` 已有、已接线（`agentboot/persistent_session.go:276-298`），但没有“先停回合、再等后台任务收尾、然后才关进程”的组合。
- `--resume` 与 `CLAUDE_CODE_RESUME_INTERRUPTED_TURN` 已有，用于崩溃恢复（`turn.go:347`），**没有**用于“用户主动停止会话后的续跑”。
- `pool.Hold`（`agentboot/pool/pool.go`）能让某个会话不被空闲回收，说明“进程可以比线程活得久”在机制上可行，只是目前没有“无主常驻”的概念。
- `Handoff` 已经是“保留而非停止”的雏形：关进程 + 给一条 `--resume` 命令，但要求用户去终端接手。
- 后台任务的输出文件、`task` 事件都已落盘并能通过 `GET /threads/{id}/tasks/{id}/output` 读取（`projects/internal/project/sessions.go:259`），所以“丢了什么”至少是可展示的。

### 1.5 方案对比

| 方案 | 做法 | 内容不丢 | 可恢复 | 代价 | 放弃理由 |
|---|---|---|---|---|---|
| **A1 优雅收尾的会话停止**（推荐） | 关进程前：`Interrupt` → 等回合结果（有上限）→ 对每个运行中的后台任务 `StopTask` → 等 `task_notification` → `Close`。用于 Restart/Archive/Handoff/暂停/退出。 | 对话、后台任务输出（文件）保留；前台工具仍丢 | 是（`--resume`） | 中：`Threads.endProcess`/`ReleaseProject` 增加一段有界等待；要处理“任务不响应 stop”的超时 | 不解决“进程必须关”的场景本身，只让收尾干净、可解释 |
| **A2 保留而非停止（detach / 托管常驻）** | Resolve/Archive 时不 `Evict`：进程留在 pool，用 `Hold` 保护，线程记录“后台运行中”；Reopen 时重新挂上 Conductor | **后台任务继续跑** | 是 | 高：无主进程的寿命/泄漏、与终端 `--resume` 的会话文件冲突、退出应用仍需全关、UI 要能看管 | 仅对“用户主动关线程但想让它跑完”有意义；需要显式开关，不能默认 |
| **A3 任务台账 + 重开提示**（推荐，和 A1 一起；**已实现** `agentboot/engine/tasks.go` `clearTasks`/`taskLedgerNote`） | 关进程时把当时运行中的任务（id、类型、描述）写成一条 system 事件，并附原有的空 `background_tasks_changed`；重开后能看到“这些任务随进程结束、已写入的输出仍在磁盘” | 不保留工作，只保留**事实** | 可查看，不可续 | 低：一处快照 + 前端一行 | 不能替代 A1（没有收尾），但保证“丢得明白” |
| **A4 检查点 / 周期持久化** | 周期性把 transcript、运行任务清单落盘，崩溃/被杀后可恢复 | 与现有 `--resume` 基本相同 | 是 | 低（已有）；增量很小 | 现有 CLI 会话文件已经是“检查点”；重复造轮子收益低，除非要支持跨机器 |
| **A5 后台任务与会话解耦** | 后台命令跑在独立进程/沙箱里，可重连 | 后台任务**不丢** | 是 | 很高：改变 Claude Code 的后台任务模型，结果回灌、权限、输出都要重做 | 与产品语义冲突（后台任务的目的是把结果喂回同一会话），不建议 |

推荐组合：**A1 + A3**（所有会话级关闭都先优雅收尾并留台账），**A2 作为显式选项**只给“关线程但让它跑完”。A5 不做；A4 已经由 CLI 提供。A3 已实现（见上表）：任何进程结束（Restart/Resolve/Archive/暂停/退出/空闲回收/崩溃）时，若有后台任务仍在跑，对话里会留下那一行。

实现代价排序：A3（~半天，已完成） < A1（~1-2 天，含测试） < A2（~3-5 天，含进程寿命/冲突处理） < A5（不建议）。

## 2 问题 B：workspace permission 为什么改了不生效

### 2.1 定义与三层存储

- 可选值：`default / acceptEdits / auto / plan / dontAsk / bypassPermissions`（`projects/internal/project/sessions.go:23`）。
- 两个设置：项目的 `thread_permission_mode`（项目设置“线程权限”）和线程的 `permission_mode`（线程详情栏“权限”）。
- 三处状态：**DB**（线程/项目行）、**CLI 进程**（启动参数 `--permission-mode`，之后可被 `set_permission_mode` 改）、**引擎缓存** `e.launch[id].mode`（`agentboot/engine/turn.go:478-487`）。

### 2.2 下发链路

```
项目设置 PATCH /projects/{id} ── SaveSettings ── 只给协调者一条 system 事件（service_ops.go:123-130）
线程设置 PUT /threads/{id}/permission-mode（handler.go:893）
   └─ Threads.SetPermissionMode（sessions.go:174）── 写 DB（+ 本分支新增：立即在线下发，§2.5）
下一次“有消息”的回合：
   turnLaunch（threads.go:949）→ threadPermissionMode(p,t)（threads.go:421）
   └─ opts.PermissionMode → runPersistentTurn（turn.go:228）
        └─ 与 e.launch[id] 比较 → applyLive（turn.go:505）
             └─ set_permission_mode 控制请求（claude/transport.go:140）→ 进程
```

`launchSignature` 明确**不包含** model / permission mode（`turn.go:717-724`），所以模式变化不会重启进程——只会在下一轮开始时发控制请求。

### 2.3 根因

**B1 项目级：创建时复制，之后不跟随。**
`sessions.go:150`、`threads.go:443` 在建线程时把项目当时的模式复制进 `t.PermissionMode`；`threadPermissionMode`（`threads.go:421-424`）优先用线程自己的值。项目设置改变后，`SaveSettings` 只发一条提示（`service_ops.go:127`），不碰任何线程。UI 的“线程权限”一行（`frontend/packages/projects/src/i18n/locales/zh/project.ts:186`）没有像“线程默认模型”（`:182` 明确写“已有线程保持各自的模型不受影响”）那样的说明。

**B2 线程级：只在下一轮“有消息”的回合生效。**
`applyLive` 只在 `runPersistentTurn` 里调用（`turn.go:233-256`）。因此：
- 正在跑的回合继续用旧模式，直到回合结束（可能几小时）。
- 后台任务结束触发的**非请求回合**不经过 `applyLive`（`agentboot/engine/resident.go:93-118`）；引擎这边批准时用 `currentMode`（`resident.go:109/117`）读的是**新模式**，CLI 进程却还是**旧模式**。对“从 bypassPermissions 收紧”来说，这是危险的不一致：CLI 根本不问，引擎也就没有批准可拦。

**B3 模式可在线收紧，但“放宽到 bypassPermissions”不行；规则不行。**
`set_permission_mode` 立即生效，而且**对运行中的回合也立即生效**——实测（`mode_probe.py tighten`）：default 模式下第一个命令跑着时切到 `plan`，同一回合的第二个命令直接被拦（`{"mode":"plan"}` success）。但 `--settings` 文件只在进程启动时读一次，`apply_flag_settings` 只能**叠加**：启动时就在文件里的 allow 规则，**重启前撤不掉**（`agentboot/claude/ref/live-control.md` “Settings” 一节；`turn.go:693-697` `liveDelta` 的注释）。另外实测（`mode_probe.py widen`）：在 `--permission-mode default` 启动的进程上切到 `bypassPermissions` 会被拒绝——`Cannot set permission mode to bypassPermissions because the session was not launched with --dangerously-skip-permissions`（agentboot 只有在 `SetSkipPermissions` 打开时才加这个 flag，`claude/driver.go:303-311`，产品代码无人调用）。这类“放宽到 bypass”只能走下一轮的 `applyLive` 失败回退：重启，用 `--permission-mode bypassPermissions` 重新启动。所以：
- 模式收紧（→ plan/default/acceptEdits）：可以实时，连运行中回合也算。
- 模式放宽到 bypassPermissions：**不能**实时；下一轮重启后生效（会结束后台任务）。
- 规则（allow 列表）收紧：**不能**实时；只能“重启后”。

**B4 自动批准的快照（已修）。**
修复前 `prompterFor`（`engine.go:452-462`）在回合开始时按 `opts.PermissionMode` 决定是否套 `autoApprovePrompter`，所以从 `bypassPermissions` 收紧后，本回合剩下的审批仍会被引擎自动批准。

### 2.4 收紧 vs 放宽，以及各层次受影响的方式

| 层次 | 收紧（bypass→default、auto→plan…） | 放宽（default→bypass…） | 说明 |
|---|---|---|---|
| 线程的模式 | 应当**立即**生效：运行中回合的下一次工具调用起（实测 plan 立即拦截，`mode_probe.py tighten`） | 模式本身可立即，但**放宽到 `bypassPermissions` 被 CLI 拒绝**（未以 `--dangerously-skip-permissions` 启动）；退回下一轮重启生效（`mode_probe.py widen`） | 已实现（§2.5） |
| 线程的 allow 规则 | **做不到**实时（启动规则撤不掉） | 可以实时（`apply_flag_settings` 叠加） | 需在 UI/事件里说明“重启后撤销” |
| 运行中的回合 | 下一次工具调用按新模式下权限检查 | 同左 | CLI 的 `set_permission_mode` 即时 |
| 正在跑的前台工具 / 后台任务 | **不受影响**：权限模式管的是“工具调用”，不是已启动的进程 | 同左 | 要停只能 `StopTask` / 杀进程；UNKNOWN：已在运行的 subagent 是否按新模式检查后续调用（未探针） |
| 项目的 `thread_permission_mode` | 只影响之后新建的线程 | 同左 | B1，需产品决策（§2.6） |

要不要分策略：**要，但只在“失败时怎么办”上分**。
- **收紧**：用户按下开关时期望“现在起就不许做”。模式可以立即（实测），但允许规则撤不掉、已运行的进程停不下。所以收紧的兜底应该是**响亮**的：下发失败要记录/告警（已做），规则收紧要提示“重启后生效”；如果用户要“立刻停手”，那是 Stop/`StopTask` 的职责，不是权限。
- **放宽**：用户已经明确同意更多权力，晚一点生效没有安全问题；除了 `bypassPermissions`（只能重启）外，其余模式也立即生效即可。
- **subagent**：和主会话共用同一个 permission mode，所以对**之后**的工具调用应当跟随；正在运行中的 subagent 是否会在内部下一步就拿到新模式，**UNKNOWN**（未单独探针）。
- **后台任务**：权限模式是“调用闸门”，不回收已启动的进程；收紧不会停掉已经在跑的命令。

### 2.5 已实现的最小修复（线程级实时）

代码：

- `agentboot/engine/engine.go:306` `Engine.SetPermissionMode(id, mode) (bool, error)`：有常驻进程时立即 `set_permission_mode`，成功才更新 `e.launch[id].mode`；进程拒绝则返回错误且**不记住**（下一轮 `applyLive` 重试）；`auto` 不可用时与轮间路径一致地落到 `acceptEdits` 并写系统说明（`:325` `setModeLive`）。
- `agentboot/engine/engine.go:514` `prompterFor` + `agentboot/engine/prompter.go:105`：`autoApprovePrompter` 改为**每次批准时**读取当前模式（有 host 的 `PermissionMode` 函数就用它，否则用回合启动模式）。收紧后本回合的后续审批不再被引擎自动放行。
- `projects/internal/project/sessions.go:174/204`：保存后调用引擎在线下发；被拒绝时记一条 system 事件“the running process did not take the new permission mode; it applies from the next turn: …”。
- **规则收紧的可见性**（后补）：`launchState.startup` 记住进程启动时的规则；`applyLive`/`refreshLive` 成功下发后，如果被移除的规则**启动时就在**设置里，写一条 system 事件“permission rules removed: … — they were in the settings when this process started, so Claude Code keeps them until it restarts”（`turn.go` `noteStartupRuleRemovals`/`startupRuleRemovals`）。只移除“之前推上去的”规则不会有这条，因为那种撤销是即时的。
- **文案**：暂停/归档项目的帮助文字补上“后台任务也随进程结束”（en/zh/ru）；composer 的停止按钮提示改为“停止当前轮次（后台任务继续运行）”。

测试（`agentboot/engine/permission_live_test.go`、`tasks_test.go`）：

- 模式在**回合进行中**下发：进程收到 `set_permission_mode plan`，且不重启；下一轮不再重复下发。
- 拒绝时**不**记住，下一次仍会尝试；`auto` 不可用落到 `acceptEdits`。
- 收紧后本回合的下一次审批**不再自动批准**（`AwaitingInput` 变为真，等用户回答）。
- 移除启动时就在的 allow 规则会写一条“重启后生效”事件；移除只推过的规则不写。
- 进程带着后台任务结束时，台账行列出任务描述与 id（`TestClearTasks_KeepsALedgerOfWhatTheProcessTookWithIt`）。

安全影响（如实）：

- **变好的**：从 `bypassPermissions` 收紧后，运行中回合的下一次审批就不会再被自动批准；进程也会拿到新模式，且**对运行中回合立即生效**（实测 plan 在同回合拦下下一个命令）。这是“收紧应立即生效”能做到的部分。
- **仍然做不到的**：① 已被批准的、**正在运行**的命令/后台任务不会因此停下（权限模式不回收已启动的进程）；② 启动设置里的 allow 规则撤不掉（§2.3 B3）；③ 进程拒绝新模式时（实测只有“放宽到 `bypassPermissions`”会被拒；`auto` 不可用会落到 `acceptEdits`），如果旧模式是 `bypassPermissions`，CLI 仍会跳过所有询问——引擎侧已经不再自动批准，但 CLI 不问。对“放宽到 bypass”本身，回退是下一轮重启（§2.3 B3），可以接受；对“收紧失败”，**建议的决策**是是否允许破例重启进程（会结束后台任务），或至少在 UI 上升级成需要用户确认的告警。
- **放宽**：模式本身可立即下发，但放宽到 `bypassPermissions` 会被 CLI 拒绝（§2.3），于是返回错误、下一轮重启生效；其余放宽（如 plan→acceptEdits）立即生效。放宽是用户的显式动作，但它同样改变**正在进行的回合**（剩下步骤按新模式跑）。

### 2.6 未修 / 需要决策

1. **项目级“线程权限”是否应影响已有线程**（B1）。现在是“创建时复制”。三种选项：(a) 保持复制，但 UI 明说“新线程生效”；(b) 改为“跟随项目，除非线程自己覆盖过”；(c) 改项目时批量更新并在线下发（等于把项目设置当成批量操作）。需要用户选。
2. **收紧失败时是否破例重启**（§2.5 残余风险 ③）。
3. **规则撤销**要不要在设置界面/事件里明确提示“重启后撤销”；以及是否提供“重启以收紧”按钮。

## 3 验证

| 结论 | 方式 |
|---|---|
| interrupt 只结束回合，进程与后台任务存活 | `.research/agentboot-stop/stop_probe.py` + `stop_probe.txt`（真实 CLI 2.1.259、离线 mock、隔离 HOME） |
| 15s 不响应会 `Close` 关进程 | `agentboot/conductor.go:198-212`；`agentboot/engine/engine_test.go:134` 的“CLI 不读输入”用例 |
| 停止链路与落盘 | 代码路径（§1.1）+ `projects/internal/project/stop_thread_test.go`、`thread_lifecycle_test.go` |
| 权限模式只在下一轮下发（修复前） | `agentboot/engine/live_test.go:190`、`:260` 两个既有用例的时序；`applyLive` 的 `e.launch` 比较 |
| 修复后运行中即时下发、拒绝不记住、收紧后不再自动批准 | `agentboot/engine/permission_live_test.go`（新增 4 个用例） |
| `set_permission_mode` 对**运行中回合**立即生效（plan 拦下同回合的下一个命令） | `mode_probe.py tighten` + `mode_probe_tighten.txt`（真实 CLI） |
| 放宽到 `bypassPermissions` 被 CLI 拒绝（未以 `--dangerously-skip-permissions` 启动） | `mode_probe.py widen` + `mode_probe_widen.txt` |
| 对照：不切模式时两个命令都各自要审批 | `mode_probe.py control` + `mode_probe_control.txt` |
| `apply_flag_settings` 不能撤销启动规则 | `agentboot/claude/ref/live-control.md` “Settings” 一节 |
| 台账行与“撤销要重启”提示 | `agentboot/engine/tasks_test.go`、`permission_live_test.go`（本轮追加） |
| 暂停/归档帮助文字 | 截图 [`../.design/images/pause-archive-help.png`](../.design/images/pause-archive-help.png)（mock 模式、zh） |

## 4 下一步（可执行）

1. ~~**A3 任务台账**~~：**已实现**（`agentboot/engine/tasks.go` `clearTasks`/`taskLedgerNote`，进程结束时若还有后台任务就写一行，含描述与 id）。
2. **A1 优雅收尾**：`Threads.endProcess` / `ReleaseProject` 等入口，先 `Interrupt` 并等回合结果（有界），再对运行中任务 `StopTask` 并等 `task_notification`，最后 `Close`；超时保留现有行为。
3. **A2 决策**：是否为 Resolve/Archive 提供“保持后台运行”（进程留在 pool + `Hold`，UI 显示“后台运行中”）。
4. **B1 决策 + 实现**：按 §2.6 选定项目级语义；若选 (c)，把 `applyPermissionMode` 复用成批量下发。
5. 剩余可见性/文案：~~allow 规则撤销的 system 事件~~（已实现）；~~暂停/归档帮助文字、停止按钮提示~~（已实现）；仍待办的是 composer 的 `session.nextTurn` 提示（en/zh/ru）现在对“权限”不准确（它说“启动设置下一次轮次生效”），应改为“模型/推理强度下一轮生效、权限立即”，并补一个前台 subagent 的 interrupt/模式探针（§1.2、§2.4 的 UNKNOWN）。
