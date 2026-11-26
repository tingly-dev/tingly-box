# 协调者的线程状态工具与 reopen（讨论稿）

> 状态：**讨论稿，未实施**（2026-11-26）。本稿只提方案，不改代码；要用户拍板的问题见 §8。
> 落地前先按本稿结论改 [thread-lifecycle.md](thread-lifecycle.md)（原则 4、§6.1、§11 的“暂缓”），再改代码。
>
> 相关：[thread-lifecycle.md](thread-lifecycle.md)（状态机现行设计）、[mcp-tools.md](mcp-tools.md)（Step 4 早已提出 `reopen_thread`，并否决了 `set_thread_state`）、[`../../.design/thread-cleanup.md`](../../.design/thread-cleanup.md)（Archive / Restore / Delete）、`cite.go`（`quoteUser`）。

## 1 起因

协调者给已 resolved 的线程发 `message_thread`，得到 `ErrResolved`（“the thread is resolved; reopen it first”，`threads.go:31`、`:884`），但它没有 reopen 工具，建议无法执行，只能另开新线程、丢掉原线程的上下文（实例：用户要去掉 #2774 加的标题圆点，原线程已 resolved）。同时 `resolve_thread` 的“只在用户要求时”只写在描述里，工具不检查；用户后来口头授权“已合并、没有剩余工作的线程交接后可直接 resolve”，这条授权也没有落在代码里。

## 2 现状

### 2.1 状态与切换

`Status` 六值：working / waiting / failed / ready / idle / resolved（`thread-lifecycle.md` §2.1）；`ArchivedAt`、`WorktreeFreedAt` 与状态正交。

| 切换 | 自动？ | 触发 | 代码 |
|---|---|---|---|
| working ↔ waiting / failed / 歇着（ready·idle） | 自动 | 一轮开始 / 结束 / 提问 / 出错 / 后台任务 / 重启恢复 | `settle`、`threadPrompter.wait`、`Recover`（T1–T12） |
| 歇着 → working | 消息触发 | 网页、IM、协调者 `message_thread` | `sendWith` |
| 任意 → resolved | **从不自动** | 网页 Resolve（单个 / 看板批量）、Archive、协调者 `resolve_thread` | `Threads.Resolve` `threads.go:1650` → `endProcess` + `close` |
| resolved → 歇着 | 从不自动 | 网页 `POST /threads/:id/reopen` | `Threads.Reopen` `threads.go:1749` |
| 加 / 去 `ArchivedAt` | 从不自动 | 网页 Archive / Restore | `Threads.Archive` / `Restore` `thread_cleanup.go:240`、`:298` |

合并、PR 关闭、空闲时间都不改状态（`thread-lifecycle.md` §6.1）。

### 2.2 协调者现有的状态相关工具（`coordinator_tools.go`）

| 工具 | 约束 | 备注 |
|---|---|---|
| `stop_thread` | `Strict`；`cite` 必填且经 `quoteUser` 校验必须是用户自己最近的消息（`cite.go:25`）；只对在跑的线程生效，否则 “not running” 不算错 | 唯一一个**代码强制**用户授权的状态工具 |
| `resolve_thread` | 只有 `thread_id`；**描述里**写“只在用户要求时”，代码不查（`:83`、`:300`）；对 working / waiting 线程同样生效，等于先 Stop 再关 | 授权全凭协调者自觉 |
| `message_thread` | `cite` 可选（给线程转述用户的批准）；resolved 或 archived 的线程直接报错 | 错误只有一句建议，没有对应工具 |
| `rename_thread` / `get_thread` / `list_threads` | 无状态影响 / 只读 | `list_threads` 可按 status 过滤，包含已归档 |
| （无）reopen / archive / restore / delete | — | `thread-lifecycle.md` §6.1 写明“重开只有用户能做”，§11 把 G6 列为暂缓 |

所有协调者工具都是 `CoordinatorOnly`，`findThread` 只找本项目的 worker 线程，不会碰主线程或别的项目（`mcp-tools.md` Step 2）。

### 2.3 网页端能做什么

`frontend/packages/projects/src/components/project/threadOps.ts`：`finish`（Resolve）、`reopen`、`archiveSession`、`restoreSession`、cleanup 预览、delete。看板批量（`ThreadsView.tsx` 的 select mode）只有 **Resolve 和 Archive**，没有批量 Reopen / Restore；已归档的线程不可勾选。

### 2.4 关闭 / 归档释放什么，重开能不能恢复

| 资源 | Resolve（`close`） | Archive（再加） | Reopen | Restore |
|---|---|---|---|---|
| 当前一轮、会话进程 | 停并回收（`endProcess`） | 同 | 下一轮重新拉起，`--resume` 找回会话 | — |
| 工具 URL | 收回（`revokeTools`） | 同 | 下一轮重建 | — |
| 线程的授权（grants） | **清空**（`UpdateThreadGrants → nil`，`close`） | 同 | **不恢复**，需要用户重新批准 | — |
| IM 绑定 | 交还父线程（`releaseChats`） | 同 | **不迁回**；`#N` 仍可直达 | — |
| scratch（`library/.tmp/<thread>`） | 删（`cleanTmp`） | 同 | 下一轮重建 | — |
| worktree | **保留** | **删除**（`git worktree remove --force`，脏的或在跑的拒绝） | 不涉及 | 在原路径从分支重建；分支没了则只恢复记录并说明 |
| 分支、对话记录、Claude 会话文件、Links（PR）、library 产出 | 保留 | 保留 | 原样可用 | 原样可用 |

**已核实的缺口**：`Reopen` 只清 `ResolvedAt`、重选歇着状态（`threads.go:1749`），**不碰 `ArchivedAt` / `WorktreeFreedAt`**；`Restore` 只清这两个标志，**不改 resolved**。所以已归档的线程要“能说话”必须先 Restore 再 Reopen 两步；只 Reopen 的话状态变回 idle，但 `sendWith` 仍以 `ErrThreadArchived`（“restore it first”）拒绝，且 worktree 还没有。按 §4.5，这种线程协调者不可见、不可操作，不需要出路；网页用户要点两次，见 §6。

## 3 要不要 reopen

**要。** 理由是 §1 的死胡同：错误信息指向一个协调者没有的动作。重开是可撤销的（再 Resolve 即可）、不丢任何东西、线程里留一条 “Reopened by the coordinator.”，风险低于现在的替代做法（新开线程 + 重述上下文，既丢上下文又多一个分支）。

### 3.1 真实场景

| 场景 | 重开原线程 | 新开线程（带旧线程摘要） |
|---|---|---|
| A 用户误点 / 协调者误 Resolve | **唯一合理**：什么都没变 | 荒谬：为一次误操作多一条线程 |
| B Resolve 了但 PR 还开着，评审意见要改 | **合适**：同一分支、同一 PR（Links）、线程记得评审前的实现 | 要接手别人的分支，易冲突，PR 归属混乱 |
| C PR 已合并，用户又提跟进（#2774 圆点） | 上下文最好；但 squash 合并后分支已落后 main，要先 merge/rebase main，再走 Merge 对话框的“合并后新提交 → 新 PR”路径（`thread-lifecycle.md` §6.4） | 干净：从 main 新分支，没有历史包袱；代价是丢上下文，协调者得写摘要，线程可用 `get_thread` 读不到别线程的完整转录 |
| D 合并后发现回归 | 线程最了解实现，适合直接修；同 C 的分支问题 | 修复和原功能解耦，PR 更好审；上下文靠摘要 |
| E 已归档的线程要继续 | 需要 Restore（重建 worktree，可能失败：分支被删、目录被占）+ Reopen；依赖要重装 | 免去重建，但原分支还要保留给新线程当 base 时才有用 |
| F 无关的新需求 | 不该重开：污染原线程上下文 | 正确 |

### 3.2 结论

不是二选一。**重开解决“同一件事还没完”（A、B、E，C/D 的上下文优势）；新线程解决“新的一件事”（F，以及 C/D 想要干净 base 的时候）。** 重开该做的是便宜、明确，让协调者在两者之间判断，而不是为协调者省掉判断。分支和 PR 的差异就是判断依据：PR 已 merged 且用户要的是增量功能 → 倾向新线程；PR 还开着或没合并 → 重开。这条判断写进 `reopen_thread` 的描述，不写进代码。

另一个独立缺口：新线程目前没有“承接旧线程上下文”的一等方式（只能协调者手写摘要，`start_thread` 有 `base` / `branch`，没有 `from_thread`）。列为阶段 3 的可选项，不是 reopen 的前提。

## 4 工具设计

### 4.1 两种方案

**方案 U（统一）**：`set_thread_status(thread_id, to, cite?)`，`to ∈ {resolved, open, …}`。
**方案 S（专用）**：沿用 `stop_thread`、`resolve_thread`，新增 `reopen_thread`；archive / restore / delete 不给协调者。

| 维度 | U：`set_thread_status` | S：多个专用工具 |
|---|---|---|
| 误用风险 | `to` 是个模式选择器，模型少写一个字段、选错枚举值就换了一种操作；`stop`、`resolve` 的副作用不同（Stop 丢排队消息并让待答的卡过期；Resolve 还回收进程、交还 IM、清 scratch、清授权），被同一个名字掩盖 | 动词即意图，描述里可以写各自的后果和时机，模型调错的代价是“工具不存在”而不是“枚举选错” |
| 批准（cite） | `cite` 是否必填取决于 `to`；JSON schema 表达不了条件必填，`Strict` 的 schema 只能全部可选，校验推给运行时，错误只有试了才知道 | 每个工具各自的 schema 写明：`stop_thread` 必填、`resolve_thread` 按 §4.3、`reopen_thread` 没有 |
| 不允许的切换 | `to` 的枚举要么暴露 working / waiting / failed（谁都不该手动设，因为它们是一轮的事实，`thread-lifecycle.md` 原则 1），要么得说明哪些值不可选 | 不存在就不能调；archive / delete 不给协调者是“没有这个工具”，最硬的禁止 |
| 出错提示 | 一个工具要为每个 (from, to) 组合写提示 | 每个工具只管自己的前置条件，提示固定且短，并能指向下一个工具（“已归档：用 reopen_thread”） |
| 与网页一致 | 网页是 resolve / reopen / archive / restore 各一个路由；U 需要再造一层映射 | 一对一：`resolve_thread` ↔ `POST /resolve`，`reopen_thread` ↔ `POST /reopen`，都落到同一个 `Threads` 方法，沿用 `mcp-tools.md` 的“动作是 Threads 方法，handler 和工具只解析输入”，并已有“同一夹具经服务、HTTP、MCP 状态和事件一致”的对等测试要求 |
| 工具列表成本 | 少一个工具 | 多一个工具；协调者工具列表有 12000 字节预算（`mcp-tools.md` Step 3，已用约 10.7 KB），`reopen_thread` 约 300 字节，放得下 |
| 审计 | 事件文本要带 `to` | 事件天然区分：“Stopped / Reopened by the coordinator.” |

`mcp-tools.md` 在 Step 1 已经比较过 `set_thread_state` 并否决，这次多出来的理由是上表“条件必填 + 枚举暴露自动状态”两条。**推荐 S。** U 的唯一优势是少一个工具，但状态只有 resolved⇄open 一对可以由协调者切换，不值得为一对开关造枚举。

### 4.2 `message_thread` 遇到 resolved / archived

| 做法 | 评价 |
|---|---|
| (a) 报错并点名工具：`the thread is resolved; call reopen_thread first`（archived：`…reopen_thread restores it`） | 一步显式、协调者要多一次调用；保留“resolved 是用户的决定”这道闸，也防止协调者把 PR 事件等转述自动推给已完成的线程 |
| (b) `message_thread(…, reopen=true)` 一次完成 | **不做**（用户决定，2026-11-26） |
| (c) 自动重开 | **不做**：resolved 是用户的验收结论（`thread-lifecycle.md` 原则 4），消息不应悄悄推翻它；网页 / IM 对 resolved 线程同样拒绝，三个入口保持一致 |

**采用 (a)**。归档的线程对协调者不可见（§4.5），所以 (a) 的提示只有 resolved 一种。

### 4.3 各切换的批准要求

原则：**降低用户可见性、停掉在做的事、或释放用户磁盘的动作需要用户授权；撤销这些的动作不需要。**

| 工具 | 授权 | 实现 |
|---|---|---|
| `stop_thread` | 用户明说 | 不变：`cite` 必填 |
| `resolve_thread` | 用户明说，**或**系统验证“已交接完成” | 见下 |
| `reopen_thread` | 不需要；`cite` **可选**（用户决定） | 可撤销、不丢东西、线程里留痕。给了 `cite` 就经 `quoteUser` 校验并和 `message_thread` 一样引用给线程；不给不影响。描述里写判断依据（§3.2） |
| archive / restore / delete | 协调者**没有这些工具，也看不到已归档的线程**（§4.5） | archive 会删用户磁盘上的 worktree，delete 不可撤销，是用户管理磁盘的事 |

**`resolve_thread` 的授权落到代码。** 现在的规则只在描述里，而用户已经给过一个窄授权（已合并、没有剩余工作 → 交接后可直接 resolve），应让工具检查事实，而不是靠协调者自觉：

```
resolve_thread(thread_id, cite?)
  cite 给出（经 quoteUser 校验）        → 允许，任意状态；对 working / waiting 的线程相当于先停再关，结果里说明
  cite 没有，且同时满足下面全部          → 允许（“handoff done”）
      线程不在跑、不排队、无后台任务（status ∈ ready / idle）
      无未提交改动（Uncommitted == false）
      Links 里有 head == 本线程分支的 merged PR，且没有开着的 PR，且合并后没有新提交
  其他                                  → 拒绝，信息写明缺哪一条、以及“引用用户的话或让用户决定”
```

这是该授权的**保守子集**：口头授权里“没有剩余工作”无法由系统完全验证，系统只验证能验证的几项；用户若对别的线程（例如从未开 PR 的调研线程）说“收掉”，走 `cite`。`Resolve` 的服务层方法保持无条件（网页用户永远可以），检查放在协调者工具里，与 `stop_thread` 的 cite 校验同层。

### 4.4 `reopen_thread` 的行为

```
reopen_thread(thread_id, cite?)
  resolved             → Reopen；歇着（ready / idle）；“reopened; the thread is now idle”
  已是 open            → “already open”，不算错（与 stop_thread 的无事可做一致）
  已归档 / 主线程 / 别的项目 → 找不到（findThread，§4.5）
  项目暂停 / 归档       → 与其他工具同闸，拒绝
```

重开后线程的授权保持清空、IM 绑定不迁回：这是线程关闭的既有行为（`close`），**不为它加新的记录或提示**（用户决定）。需要时用户重新批准即可。

### 4.5 已归档的线程对协调者不可见

用户决定：归档的线程协调者不能操作，理论上也不应看到。现状：`findThread`（`coordinator.go:575`）不排除已归档的线程，`list_threads` 的描述还特意写了“包括已归档的”。改为：

- `findThread` 跳过 `ArchivedAt != nil` 的线程，按编号或 id 都是 “no thread … in this project”，所有协调者工具（`get_thread`、`message_thread`、`stop_thread`、`resolve_thread`、`reopen_thread`、`rename_thread`）一致；
- `list_threads` 不再返回已归档的线程，描述同步改；
- 因此 `message_thread` 的 `ErrThreadArchived` 提示对协调者不会出现，`Threads.Reopen` 也不需要处理归档（网页上用户仍是先 Restore 再 Reopen，见 §6）。

需注意：被归档的线程仍可能出现在协调者历史里的旧消息或报告中（`#N`）；工具对它统一回答“找不到”，不泄漏它被归档了。

## 5 状态切换表

“允许”指协调者工具；网页列出来是为了看一致性。

| 切换 | 网页 | 协调者 | 批准 | 备注 |
|---|---|---|---|---|
| 歇着 / waiting / failed → resolved | Resolve（单个、批量） | `resolve_thread` | 网页：无；协调者：`cite`，或系统验证交接完成（§4.3） | working / waiting 的线程需要 `cite` |
| working → 歇着（Stop） | Stop | `stop_thread` | `cite` | 不变 |
| resolved → 歇着 | Reopen | `reopen_thread`（新） | 无（`cite` 可选） | 已归档的线程协调者看不到 |
| resolved → 归档（释放 worktree） | Archive（单个、批量） | **不提供** | — | 有未提交改动 / 在跑时拒绝；归档后协调者看不到 |
| 归档 → 未归档（重建 worktree） | Restore | **不提供** | — | 归档的线程协调者不可见 |
| 删除 | Delete（仅已归档） | **不提供** | 用户确认框 | 不可撤销 |
| 任意 → working / waiting / failed | — | **不能直接设** | — | 只由一轮产生（原则 1） |
| 主线程的任何切换 | 不可 | 不可 | — | `ErrNotWorker` / `findThread` |

## 6 与网页

网页行为不变：已归档线程先 Restore 再 Reopen（两步），是用户自己的磁盘管理动作，不为协调者改 `Threads.Reopen`。看板批量仍只有 Resolve / Archive；是否加批量 Reopen 看实际需求，不阻塞本方案。

## 7 分阶段

| 阶段 | 内容 | 风险 |
|---|---|---|
| 1 | 加 `reopen_thread(thread_id, cite?)`；`ErrResolved` 的协调者提示点名工具；`findThread` 和 `list_threads` 排除已归档线程（§4.5）；改 `thread-lifecycle.md` 原则 4 / §6.1 / §11（重开“用户或协调者”）、`mcp-tools.md`；对等测试（服务 / HTTP / MCP 同一夹具）、`coordtest` 的 resolve → reopen → message 流程；工具列表 golden 和 12000 字节上限测试 | 低：增加一个可撤销的工具；隐藏归档线程是行为收紧，要看协调者提示词里有没有依赖归档线程的写法 |
| 2 | `resolve_thread` 的授权落到代码（§4.3）：加 `cite`，无 `cite` 时校验“交接完成”；改描述；补测试（缺一条就拒绝，各条件做变异检查） | 中：会拒绝以前能过的调用，提示词里“用户授权后直接 resolve”的写法要同步更新 |
| 可选 | `start_thread` 的 `from_thread`（给新线程一个指向旧线程转录的入口）；网页批量 Reopen | 视使用情况；`message_thread` 的 `reopen=true` 已决定不做 |

## 8 已定与待定

用户 2026-11-26 的决定：

1. `reopen_thread` 的 `cite` 可选。
2. 归档的线程协调者不能操作，也不应可见；重开不处理归档，不自动 Restore。
3. `message_thread` 不加 `reopen=true`，也不自动重开。
4. 重开后授权清空、IM 绑定不迁回：保持现状，不加新记录。

仍待确认：

- `resolve_thread` 的授权：本稿仍按“`cite`，或系统验证交接完成”写（§4.3），即没有 `cite` 也能 resolve 已合并、无剩余工作的线程；如果你的意思是 `resolve_thread` 也完全不检查，请说一声。
