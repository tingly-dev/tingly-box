# Thread 清理与 worktree 回收

> 状态：**核心已实施（2026-11-17），其余待做。** 已落地：`cleanTmp` 只读修复、
> 线程 Archive/Restore/Delete 后端（`projects/internal/project/thread_cleanup.go`）与 API
> （`/threads/:id/archive|restore|cleanup`、`DELETE /threads/:id`）、线程页菜单与确认对话框、
> Overview 的 Archived 分组、**按项目共享的工具缓存 `library/.cache`（仅提示词引导，见 §3.10）**；安全检查取 P1（有风险即拒绝），
> 远端分支删除、快照（P2）、批量清理、`auto_archive_days` 自动策略和积压迁移**尚未实施**（见 §4 的 PR 4–6）。
> 相关：[workspace.md](../projects/.design/workspace.md)（worktree 位置）、
> [projects.md](../projects/.design/projects.md)（线程生命周期）、
> [thread-card-requests.md](../projects/.design/thread-card-requests.md)。

## 1. 问题

Projects 能开线程（每个线程一个 worktree 和一个 `tp/*` 分支），但**没有任何一条路径会把它们收回来**：

- 项目线程没有 archive；`/threads/:id/archive` 只接受文件夹会话（`handler.go` 的 `folderSession`），UI 里也只有 `session && …` 才出现。
- `RemoveWorktree`（`project/git.go`）写好了，**没有任何调用方**。`projects.md` 里写的“删除项目：移除 worktree，保留分支”也没实现：项目的 Delete 只是 `SetArchived`，什么都不删。
- 线程没有 delete；只有项目级 `DeleteProjectCascade`（数据库行），且没有 HTTP 入口在用它清盘。
- Resolve 会清线程的 TMPDIR（`cleanTmp`），但**对大目录经常失败**（见 §2.3）。

**范围决定（用户，2026-11-17）：清理 = 只回收 worktree 本身；library（uploads、outputs、memory）不动，分支保留。** 线程的 TMPDIR（`library/.tmp/<thread>`）属于 library，但它是“resolved 后即删”的既定约定，不是清理新增的动作；它此前不可见也被判定为问题——`.tmp` 子树已在 Library 树里可见、可下载、可删除（`origin: "scratch"`，见 `projects/.design/library-tree.md`），所以 `cleanTmp` 的只读目录 bug 修复（PR 1）不再是“替用户删看不见的东西”。Archive / Delete 的全部安全检查照旧。

**补充决定（用户，2026-11-17）：**
- **Archive / Delete 同时删 `.tmp`**：它是 Resolve 既定契约的一部分（先修只读 bug：`makeWritable` 先补 owner 写位再 `RemoveAll`），Restore 后下一个 turn 会重建。`outputs/`、`uploads/` 仍然永远不删。
- **共享工具缓存按项目共享**：放 `library/.cache`（点目录，Library 树、fork、zip 根目录本来就跳过点目录）。**只靠线程提示词引导**——告诉线程把 `GOMODCACHE`/`GOCACHE`/`npm_config_cache`/`PIP_CACHE_DIR` 指过去、项目内共享；不预设启动 env，也不在库的读写逻辑里为 `.cache` 加特例（**2026-11-18 简化决定**，取代先前的“启动 env 显式指定”，见 §3.10）。fork 不带缓存/临时文件是现成行为（`copyTree` 跳过一切隐藏目录），fork 带的是 memory/ + library 非隐藏文件。

## 2. 现状（只读调查，2026-11-17，本机）

### 2.1 每个动作实际删了什么

| 动作 | 停进程 | 撤销工具入口 | TMPDIR | worktree 目录 | `git worktree` 登记 | 本地分支 | 远端分支 | DB 行/事件 | Claude 会话文件 |
|---|---|---|---|---|---|---|---|---|---|
| Resolve（含合并后的 resolve） | 是（Evict） | 是 | 尝试删（`os.RemoveAll`，失败只记 warn） | 留 | 留 | 留 | 留 | 留 | 留 |
| Archive（仅文件夹会话） | 是 | 是 | 不删 | n/a（用户自己的目录） | n/a | n/a | n/a | 留，打 `archived_at` | 留 |
| 项目 Delete（= 项目 archive） | 停全部线程 | 是 | 不删 | 留 | 留 | 留 | 留 | 留 | 留 |
| 线程 delete | **不存在** | | | | | | | | |
| `RemoveWorktree` | 无人调用 | | | | | | | | |

线程行里有 `MergedAt`、`ResolvedAt`、`Links`（PR/issue，含 `state`、`head`），可用来判断“已合并”，但：本机 DB 的 135 个 resolved 线程里 **`merged_at` 一个都没有**（合并多半在 GitHub 上完成，没走旧的本地合并），只有 15 个的 `links` 里带 `merged`（Links 功能较新）。所以“已合并”不能只信 DB（见 §3.3）。

### 2.2 本机实测的积压

读 `git worktree list`、`du`、`gh pr list`、`/proc/*/cwd`、SQLite（均只读）：

| 项 | 数值 |
|---|---|
| `<repo>/.claude/worktrees/` 下的 worktree | 118 个（115 个有分支，3 个 detached），共 **15.9 GB**；中位 82 MB，均值 135 MB，最大 837 MB |
| 其中 PR 已合并（按 `gh pr list --state merged` 的 head 名）的分支 | 85 个，**12.7 GB** |
| 其中 PR 未合并的 | 30 个，2.9 GB；其中 18 个有本地提交、没推送、没 PR（**这些是唯一一份**），4 个有未提交改动 |
| 本地 `tp/*` 分支 / 远端分支 | 160 / 588 |
| 其中是 `main` 祖先的分支 | 只有 8 / 115 —— 仓库用 squash merge，**祖先检查几乎没用** |
| 带初始化 submodule（`libs/*`）的 worktree | 36 个 |
| 有 `node_modules` 的 worktree | 63 个（单个前端 `node_modules` 约 750 MB，实际占大头；Go 构建缓存不在 worktree 里） |
| 仍有进程把 cwd 放在 worktree 里 | 12 个 `vite` dev server，加上若干在 worktree 里跑的 Claude 会话 |
| 线程 TMPDIR（`library/.tmp/<thread>`） | 33 个目录，**14.6 GB**；其中 **7 个已 resolved 线程**的目录还在，最大 4.8 GB |
| `~/.claude/projects/*worktrees*`（Claude 会话转录） | 159 个目录，约 0.9 GB |
| `tingly-project.db` | 172 MB，12.3 万个事件；线程 175 个（resolved 135、ready 18、idle 18、waiting 4、working 2） |

“一个典型已解决线程”的成本：**约 80–140 MB worktree + 一个本地分支（几乎不占空间）+ 事件行（约 0.7 KB × 数百）**；磁盘大头是 `node_modules`（装过依赖的 worktree 约 +750 MB）和线程里跑 Go 构建/测试时落在 TMPDIR 里的东西（4.8 GB 那个）。

### 2.3 为什么现在清不掉（失败模式）

1. **没有入口**：archive/delete/回收 worktree 根本没接线（§1）。
2. **TMPDIR 清理被只读文件卡住**：已 resolved 的 `9a83b1c6…` 目录里是 Go 模块缓存（`go/pkg/mod/**`，Go 把目录设成 0555、文件 0444）。`os.RemoveAll` 删不动只读目录，只打一行 warn，GB 级残留永远留下。这是**一个已存在的 bug**，与新功能无关，应先修（先 `chmod -R u+w` 再删）。
3. **并发/进程**：vite、`go test`、Claude 子进程的 cwd 在 worktree 里；直接删目录会让它们报错，或在原路径又长出文件。Resolve 只回收 agent 会话进程，不管线程自己起的后台命令。
4. **脏 worktree**：`git worktree remove --force`（`RemoveWorktree` 现在的写法）会连未提交文件一起丢，没有任何检查。不带 `--force` 则被 untracked 文件或 submodule 拒绝（36 个 worktree 有 submodule；`--force` 才能删）。
5. **未合并/只在本地的提交**：18 个线程的提交只存在于本地分支。**删分支 = 丢工作**；分支是否已合并必须看 PR 状态，不能靠 `merge-base --is-ancestor`（squash）。我试了 `git merge-tree --write-tree main <branch>` 是否为空操作：只有 29/115 命中（`main` 之后又改了同样的文件），不可靠，只能当补充线索。
6. **分支被占用**：同一分支被另一个 worktree 检出时 `git branch -d` 失败；worktree 目录被手工 `rm -rf` 后留下 prunable 登记（需要 `git worktree prune`）。
7. **数据目录里的 worktree**：旧线程（`<data>/projects/<p>/threads/<id>`）或回退路径，和仓库内的 `.claude/worktrees/` 位置不同，清理要按 `t.Cwd` 判断，而不是按约定路径拼。
8. **权限/分类器拒绝**：删除类命令在受限环境里会被拒（本线程被要求不删任何东西）。清理必须是产品里受控的动作（后端调用 git），而不是让 agent 自己去 `rm`。
9. **其他会留下的东西**：Claude 的会话转录（`~/.claude/projects/<cwd 编码>/`）、`<data>/sessions/settings/<thread>.json`、`<project>/threads/<id>.settings.json`、library 下 `outputs/<slug>/`（线程交付物，**不该随线程删**）。

## 3. 设计

### 3.1 词汇：三层，不要混

| 层 | 用户说的 | 做什么 | 能撤销吗 |
|---|---|---|---|
| Resolve（已有） | “做完了” | 停进程、收工具入口、清 TMPDIR；worktree 和分支留着 | Reopen |
| **Archive**（新，项目线程） | “收起来，不用再看” | 同 Resolve，再 **释放 worktree**（目录没了、`git worktree` 登记没了），**保留分支、记录和转录**；从看板消失，进 Archived | **Restore**：在原路径重建 worktree |
| **Delete**（新） | “永久不要了” | 只能从 Archived 里点：删分支（有条件）、可选删远端分支、删 DB 行和事件、设置文件 | 否（有备份或 trash ref 时可找回，见 3.3 / 3.4） |

Archive 与 Delete 分开，是 [ux-principles](./ux-principles.md) §10（完成 ≠ 锁死）的直接结果：绝大多数人想要的是“释放磁盘”，不是“丢掉”。

数据：线程已有 `ArchivedAt`。新增 `WorktreeFreedAt *time.Time`（worktree 已释放，`Cwd` 仍记着原路径供 Restore 用）和 `BackupRef string`（脏 worktree 的快照 ref，见 3.2 P2）。**不另加 status**：archived 是与 status 正交的标志（原则 §4），Resolve 里的“已合并/未合并”状态保持不变。

### 3.2 方案对比

Archive 的语义：

| | 做法 | 优点 | 缺点 |
|---|---|---|---|
| A | 只隐藏（打 `archived_at`），不删任何东西 | 最小、零风险 | 解决不了磁盘：现在的问题正是占盘 |
| **B（推荐）** | **隐藏 + 释放 worktree**，保留分支；Restore 重建 | 一个动作解决两件事；分支在 = 工作不丢；Restore 在**同一路径**重建，Claude 按 cwd 找转录，`--resume` 照常 | Restore 要重新装依赖（node_modules 不随分支走）；需要安全检查 |
| C | 隐藏和释放分成两个独立开关 | 灵活 | 多一个概念，用户要自己判断两个开关的组合 |

Delete 的语义：D1 硬删（推荐，只允许从 Archived 进入，等于两步确认）；D2 软删/回收站（再多一层状态，磁盘本来就不占，价值低，不做）。

“未合并工作”怎么保护（核心约束）：

| | 做法 | 评价 |
|---|---|---|
| P1 | 有风险就拒绝，列出原因，让用户先处理（推送 / 开 PR / 提交） | v1，最简单，不会丢东西 |
| **P2（推荐，PR 5）** | 脏 worktree 拒绝为默认；用户明确选“保存快照并归档”时，把改动（含 untracked，不含 ignored）做成一个提交挂在 `refs/tingly/backup/<thread>`，**不移动分支**，Restore 时还原成未提交改动 | 不污染分支/PR，也不丢；多一点实现 |
| P3 | 自动把脏改动提交到分支（WIP commit） | 简单，但会改动用户的 PR 分支，且 `git add -A` 可能把不该提交的东西提交上去，不选 |

### 3.3 安全检查：一个函数，预览和执行共用

`Threads.CleanupReport(threadID) → {size_bytes, blockers[], warnings[], kept[]}`；`GET` 预览与 `POST` 执行都调它，执行时**重新检查**（预览之后状态可能变）。

| 检查 | 怎么测 | Archive | Delete |
|---|---|---|---|
| 线程在跑 / 有后台任务 / 有排队消息 | `m.Running`、`RunningTasks`、held queue | 阻止（给“停止并归档”选项，沿用现有 `guarded('archive')` 确认） | 同 |
| 未提交改动 | `git status --porcelain`（含 untracked） | 阻止；可选快照（P2） | 不适用（worktree 已释放；有快照则一并删） |
| 本地提交没推送 / 没 PR | 远端分支是否存在且 SHA 相同；Links 里 PR 状态 | **只警告**（分支保留，不丢） | **阻止**，除非强制 |
| 分支是否已合并 | ① Links 里 `state=merged` 且 `head==branch`（权威）；② 没有 Links 记录时用 GitHub 查 head 分支的 PR（积压迁移用）；③ `merge-base --is-ancestor main branch`（只在非 squash 时有效，不能单独依赖）；`merge-tree` 空操作只当提示（实测命中 29/115） | 不影响 | 已合并才允许删分支；未合并要“强制”并输入分支名确认，同时在 `refs/tingly/trash/<branch>` 留指针 |
| PR 仍 open | Links | 警告（“归档后 PR 仍在 GitHub 上”） | 阻止删远端分支 |
| 路径安全 | 目标必须是 `git worktree list --porcelain` 登记的、位于 `t.RepoPath/.claude/worktrees/` 或 `<data>/projects/<p>/threads/` 下的目录，且不是仓库根 | 否则拒绝，**绝不对任意路径 `rm -rf`** | 同 |
| 目录被别的进程占用 | 线程自己登记的进程；Linux/macOS 上可选 `lsof +D` 尽力检测（见开放问题） | 警告；删除失败时返回“目录忙”，不重试不强杀 | 同 |
| 分支被别的 worktree 检出 | `git worktree list` | 不影响 | 阻止并说明在哪个目录 |

### 3.4 执行顺序与并发

Archive（持有该线程的 `lifecycleLock`，与 Resolve / Merge / Reopen 互斥，沿用 `threads.go` 的写法）：

1. 重新做安全检查，有 blocker 就返回（HTTP 409，body 带原因列表）。
2. 停当前 turn、`engine.Evict`、停线程登记的后台任务、`revokeTools`（和 Resolve 相同；archive 未 resolve 的线程时先 resolve）。
3. 脏且用户选了快照：生成快照 ref（临时 index：`GIT_INDEX_FILE=… git add -A && write-tree && commit-tree -p HEAD`），记 `BackupRef`。
4. `git worktree remove --force <cwd>`（`--force` 是因为 36 个 worktree 有 submodule、且 ignored 的 `node_modules` 不算脏；脏的已在第 1/3 步处理），再 `git worktree prune`。
5. 清 TMPDIR（先修只读目录 bug：遍历 `chmod u+w` 再删，见 §2.3）。
6. 最后写 `ArchivedAt`、`WorktreeFreedAt`，发布事件，记一条 system 事件 “Archived; branch tp/x kept”。

顺序保证崩溃安全：每一步幂等；DB 标志最后写，中途失败下次重试能续上；删除失败（目录忙）保持原状态并报错。

Restore：`git worktree add <原 Cwd> <branch>`（不带 `-b`，需要新增 `AddWorktreeExisting`）；分支已不存在 → 线程变成“仅记录”，说明原因；目录已存在且不是我们的 → 拒绝；有 `BackupRef` → 还原改动。清 `ArchivedAt` / `WorktreeFreedAt`；不自动装依赖。

发给已归档项目线程的消息：沿用 `ErrThreadArchived`（现在只对文件夹会话生效，扩展到所有线程）；IM 聊天绑定在 archive 时解绑到父线程（与 Resolve 的 `releaseChats` 一致）。

Delete（只接受已归档线程）：

1. 检查（3.3）；2. 删本地分支（已合并用 `-d`，强制用 `-D` 前写 `refs/tingly/trash/<branch>`）；3. 可选删远端分支（仅当 PR 已合并、head 一致；`git push origin --delete`，远端已被 GitHub 自动删除视为成功）；4. 删 `<project>/threads/<id>.settings.json`、`<data>/sessions/settings/<id>.json`、快照 ref；5. 可选删 `~/.claude/projects/<cwd 编码>/`（Claude 的转录；默认保留，因为占得很小，且是“记录”的一部分）；6. 删 DB 行：线程、事件、Links、channel bindings。
**永远不删** `library/outputs/<slug>/`（线程交付物，属于项目资料，不是线程私有）。

### 3.5 批量清理

`POST /projects/:id/cleanup/preview?scope=resolved_merged` 返回候选线程列表和总大小（`du` 在后台算并缓存，预览先返回估算，不阻塞）：

- **默认候选** = resolved ∧ 已合并 ∧ worktree 干净 ∧ 未在运行。预览里按“可释放 N 个，共 X GB；有 M 个被拦下，原因……”展示。
- 执行对每个线程**单独**走 Archive（有锁），一个失败不影响其他，结果逐条回报。
- 不提供“批量 Delete”：删除只能逐个，避免一次手滑。

### 3.6 自动策略（可选，默认关）

项目设置 `auto_archive_days`（0 = 关）。复用 `service.go:129` 已有的每小时 ticker：对 resolved 且已合并（Links 里 `merged`）超过 N 天、干净、不在运行的线程，自动 Archive，并在线程里记 “Archived automatically”。**只归档，永不删除**；任何 blocker 都跳过。

### 3.7 API

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/threads/:id/archive` | 现有，去掉“仅文件夹会话”限制；body `{snapshot?: bool}`；409 带 `blockers` |
| POST | `/threads/:id/restore` | 现有，项目线程重建 worktree |
| GET | `/threads/:id/cleanup` | 预览：`size_bytes`、`blockers`、`warnings`、`kept`（将保留什么） |
| DELETE | `/threads/:id` | 永久删除；body `{remote_branch?, force?, confirm_branch?}`；仅 Archived |
| POST | `/projects/:id/cleanup` | 批量归档 `{thread_ids}` 或 `{scope}`；`?dry_run=1` = 预览 |
| PUT | `/projects/:id` | 新增 `auto_archive_days` |

按仓库约定：后端模型 + swagger 定义，`task codegen` 生成前端客户端；codegen 前用占位函数。

### 3.8 UI 位置（沿用现有模式）

- **线程页 ⋯ 菜单**（`ThreadPanel.tsx`，现在只有 Resolve/Reopen 和会话的 Archive）：已 resolved 时加 **Archive**（项目线程也有）；已归档时换成 **Restore** 和 **Delete permanently…**。用 `ConfirmDialog`，内容是预览的结果：“释放约 X MB；分支 `tp/…` 和对话保留，随时 Restore”，blocker 以列表显示在对话框里而不是 toast；点按钮才请求，预览是只读的。
- **看板卡片菜单**：resolved 线程加 “Archive”（一键预览同一对话框）。已归档线程不再出现在看板，进项目的 **Archived** 分组（文件夹会话已有 `session.group.archived` 折叠组，复用）。
- **项目设置**（General / Environment 旁新增 *Storage*）：磁盘用量摘要（线程数、worktree 总大小、可释放大小）、“清理已合并线程…”（批量预览→确认）、`auto_archive_days`。
- 文案不用后端词（不出现 “worktree prune”）；“Archive” 的说明里直接写“保留分支，随时恢复”。

### 3.9 积压迁移（本机 118 个 worktree，15.9 GB）

做成**批量清理的第一次使用**，不写一次性脚本：

1. 老线程在 DB 里有 `Cwd`/`Branch`/`RepoPath`，直接可用；没有对应线程的 worktree（如 `agent-*`、手工的、`library/.tmp/prassets-wt3` 这种放在 TMPDIR 里的）不归 Projects 管，**只在预览里列为“未知来源”，不自动动**。
2. 老线程没有 Links/`merged_at`，预览时按 GitHub 查 head 分支的 PR 状态补上（一次 `gh pr list --state merged` 即可，本次调查就是这样得到 85 个）。
3. 预览分四类：已合并且干净（约 12.7 GB，可一次释放）；未合并但已推送；未合并且只在本地（18 个，必须人看）；脏的（4 个）。只有第一类默认勾选。
4. 先修 TMPDIR 只读 bug，再提供“清理已 resolved 线程的 TMPDIR”（本机立刻能释放约 14 GB 的大头，7 个已 resolved 线程）。
5. 本地分支（160 个）本身不占空间，积压里**不主动删**；用户在 Archived 里逐个 Delete 即可。

### 3.10 共享工具缓存（提示词引导，2026-11-17；2026-11-18 简化为仅提示）

**问题**：线程各自跑 `go test`、`npm install`、`pip install`，缓存落在自己的 HOME（或 agent 为测试自建的 isolated HOME）里，随线程 scratch 一起被删、又随下一个线程重新下载。实测一个线程的 `.tmp` 里留着 **8 份 isolated-home 的 Go 模块缓存，每份 702 MB，约 5.6 GB 纯重复**。

**决定**：缓存按**项目**共享，位置 `library/.cache`；**实现只是线程系统提示词里的一句话**——「工具/构建缓存用 `<library>/.cache`，不用默认目录」。四个变量只是例子（`GOMODCACHE`、`GOCACHE`、`npm_config_cache`、`PIP_CACHE_DIR`；yarn/pnpm/cargo/uv 等其它工具的下载、构建缓存同理），提示词不逐一列举，也不预设任何启动 env、不为 `.cache` 增加逻辑。

- **为什么够了**：线程都以同一个 OS 用户运行，路径可写；这些工具默认从 `HOME`/`XDG_CACHE_HOME` 推目录，agent 按提示把四个变量指到 `.cache` 下的子目录即可共享，不需要产品替它设。缓存是内容寻址存储（Go 模块/构建缓存、npm `_cacache`、pip HTTP 缓存），并发读写是设计内的，线程之间无需互斥。
- **不做**：不设启动 env；不加 `Layout.CacheDir`/大小统计；不在 Library 树、下载、zip、fork、coordinator 的工具里给 `.cache` 加特例——它就是普通点目录，现有的“跳过点目录”规则本来就不列出、不服务、不复制它。也不清理：Archive/Delete/Resolve 的 `cleanTmp` 只删 `.tmp/<thread>`。
- **代价**：依赖 agent 遵守提示；不遵守时缓存仍落在 HOME/scratch，只是没有收益，不会出错。若将来确需强制共享，再回到“启动 env 显式指定”。
- **测试**：提示词测试断言四个变量名与 `.cache` 路径；`seedLibrary` 加一个 `.cache` 文件，断言 Library 树、zip、fork 都不带它；Archive/Delete 测试断言它不被删除。

## 4. 推荐方案与 PR 拆分

选：**Archive = B（隐藏 + 释放 worktree，保留分支）；Delete 只在 Archived 内、分支删除受 PR 合并状态保护；安全检查 P1 起步，P2 快照后加。**

| PR | 内容 | 验证 |
|---|---|---|
| 1 | 修 `cleanTmp`（只读目录）+ 调查已 resolved 线程的残留 TMPDIR 的一次性清理入口；加测试（构造 0555 目录）。`.tmp` 在 Library 里可见/可下载/可删除已随本提案先期落地 | Go 单测；本机实测 4.8 GB 目录能删 |
| 2 | 后端 Archive/Restore（项目线程）：`CleanupReport`、`AddWorktreeExisting`、`WorktreeFreedAt`、路径安全、`ErrThreadArchived` 扩展；API + swagger | 真 git 仓库的集成测试：干净 / 脏 / submodule / 分支被占用 / 运行中 / 崩溃重试；Restore 后 `--resume` |
| 3 | 前端：线程页和卡片的 Archive / Restore 对话框，Archived 分组；截图（单语言单主题） | `pnpm typecheck/lint/test/build`，mock 模式截图 |
| 4 | 批量清理 + 项目设置 *Storage*（用量摘要、预览、执行），含积压迁移所需的 GitHub 合并状态补全 | 后端单测 + 本机 dry-run 对照 §2.2 的数字 |
| 5 | Delete（分支保护、trash ref、可选远端分支）+ 快照（P2） | 集成测试：未合并拒绝、强制、trash ref 可找回 |
| 6 | 自动策略 `auto_archive_days`（默认关） | ticker 单测，时钟注入 |

PR 1 单独有价值（立刻释放 GB 级空间，且是已有 bug），应先做；2→3 才是用户可见的“能归档”；4 解决积压；5、6 可延后。

## 5. 开放问题（需要你决定）

1. **Archive 默认就释放 worktree 吗**（推荐 B）？还是先只隐藏、另给“释放空间”按钮（C）？
2. **脏 worktree**：v1 直接拒绝（P1）够不够，还是第一版就要“保存快照并归档”（P2）？
3. **Delete 要不要做**？还是 Archive + 批量清理已经够，分支留着（本地分支不占空间）？
4. **远端分支**：Delete 时默认勾选“同时删远端分支（仅已合并）”，还是默认不勾？GitHub 仓库是否已开启合并后自动删分支（588 个远端分支说明多半没开）？
5. **自动策略** N 默认多少天；默认关是否可以？
6. **“目录被其他进程占用”**（本机有 12 个 vite 的 cwd 在 worktree 里）：是否值得在 Linux/macOS 用 `lsof` 尽力检测并警告，还是只依赖线程自己登记的进程，失败时报“目录忙”？
7. **Claude 转录**（`~/.claude/projects/…`，约 0.9 GB / 159 个目录）：Delete 时默认保留还是一起删？
8. **未知来源的 worktree**（`agent-*`、TMPDIR 里的）是否也纳入 Storage 页的“可手动清理”清单，还是明确不管？
