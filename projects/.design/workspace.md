# 线程工作区：从原始目录出发，共享的东西放在一处

> 状态：草案，对应 TEP（见 PR 描述）。取代 [projects.md](./projects.md) §4.1 里“每个线程一个 worktree，放在 Projects 数据目录”的做法，
> 并回应 [#2084](https://github.com/tingly-dev/tingly-box/issues/2084)（每个线程都开 worktree 未必合适）。
> 相关：[../../.design/projects-sessions.md](../../.design/projects-sessions.md)（统一 Thread 模型）、
> [../../.design/projects-attachments.md](../../.design/projects-attachments.md)（上传）。
> 验证：2026-10-25，Claude Code 2.1.310，`claude -p`。

## 问题

每个项目线程启动时 `git worktree add -b tp/<slug>-<id6> <data>/projects/<p>/threads/<id>`，cwd 因此每个线程都不同。结果：

1. **共享信息被切碎。** 凡是以 cwd 为键的东西（Claude Code 的转录、非 git 目录的原生记忆、本地设置、被忽略的 `.env`）每个线程一份。
2. **上传不共享。** 文件夹项目的上传是 per-thread（`UploadOwner` 返回线程自己，`_sessions/<thread>/`），同一文件夹的线程互相看不到。
3. **worktree 离开了仓库。** 分支在用户的仓库里，工作目录却在另一个应用的数据目录里，难发现、难清理。

另外要为多容器留余地：到那时 cwd 和数据目录都是容器内的本地路径。

## 结论

线程的行为尽量简单，由三条组成：

1. **线程从项目的原始目录出发，cwd 在整个生命周期里不变。** 有仓库时，新线程默认在仓库的 `.claude/worktrees/<slug>-<id6>` 里（Projects 自己 `git worktree add`，分支名仍是 `tp/<slug>-<id6>`）；没有仓库时是项目专属的空目录。
2. **共享的信息放在仓库之外，启动时注入。** 记忆经 `--settings` 的 `autoMemoryDirectory` 指到项目 `memory/`；上传和产出在项目 `library/`；多个目录用 `additionalDirectories` 授权。
3. **上传共享。** 文件夹项目的上传改成 per-folder，和项目一样放在 `library/uploads/`。

不新增工具，不做写前拦截钩子。启动 prompt 里加一句话，告诉 agent 它的工作区在哪、共享目录在哪、需要隔离时可以自己用 `EnterWorktree`。

“默认用 worktree”先不做成设置项；以后需要“就地编辑”再加（见后续）。

## 验证与事实依据

| # | 事实 | 依据 |
|---|---|---|
| F1 | 原生 auto memory 在 `~/.claude/projects/<project>/memory/`。git 仓库的子目录、仓库外的 worktree、**建在 `repo/.claude/worktrees/` 下的 worktree** 都落到仓库根那一份；非 git 目录按 cwd | memory 文档；实测 |
| F2 | `--settings` 里的 `autoMemoryDirectory` 对 git 和非 git 目录都生效 | 实测 |
| F3 | `EnterWorktree` 在 `.claude/worktrees/<name>` 建 worktree，分支 `worktree-<name>`，转录迁到新 cwd 名下 | worktrees 文档；实测 |
| F4 | 对已进入 worktree 的会话 resume，从仓库根或从 worktree 路径启动都回到该 worktree，历史完整 | 实测 |
| F5 | 隔离时对**主 checkout 路径**的 Edit/Write 被拦，即使目录在 `additionalDirectories`；仓库外目录可写 | 实测（仓库内 `.tingly/` 被拒，仓库外共享目录可写） |
| F6 | 在 worktree 之外的路径调用 `EnterWorktree` 每次弹审批，仅 `bypassPermissions` 跳过 | worktrees 文档 |
| F7 | `PreToolUse` 钩子的拒绝文案可以引导 agent 自己调用 `EnterWorktree` | 实测（本方案不采用，见下） |
| F8 | 官方 Projects：项目记忆是独立文件，与本机 auto memory 分开；本机文件夹线程的 worktree 是每线程的可选项 | claude-projects 文档 |

实测方式：`claude -p`，用 `env -u` 去掉托管会话的标识，`CLAUDE_CODE_DISABLE_AUTO_MEMORY=0` 打开 auto memory，让模型报告系统提示里的记忆目录，用 `git worktree list` 和转录目录核对。**当时未测**的常驻流式进程，实现后已补测，见下一节。

### 实现后的验证（2026-10-25，#2252、#2253）

| 验证 | 方式 | 结果 |
|---|---|---|
| 新线程的 worktree 位置、分支名、`git status` | 真 git 仓库起两个线程；单元测试 `TestThreadWorktreeIsInTheRepository` | 在 `<repo>/.claude/worktrees/<slug>-<id6>`，分支 `tp/...`，`git status` 为空，exclude 只有一行 |
| 记忆目录 | 真 `claude` 2.1.310（`-p`）加载线程实际生成的 settings，从 worktree 1 写文件，换到 worktree 2 读 | 两个 worktree 都报告项目 `memory/`，文件互通 |
| 常驻流式进程下的 resume | 真 `claude`，`--input-format stream-json --output-format stream-json`，同一 worktree cwd，`--session-id` 起一轮，`--resume` 再来一轮 | 第二轮记得第一轮内容，记忆目录仍是 `autoMemoryDirectory`（取代上面“未测”） |
| 仓库只读时的回退 | `unshare -m` 里把仓库的 `.claude` 以只读 bind mount，真跑 `Threads.Start` | git 报 Read-only file system；线程落到数据目录并记一条系统事件；回退不撞分支名 |
| 失败后残留分支 | 同上 | `git worktree add -b` 失败会留下分支，`AddWorktree` 现在删掉它；单元测试 `TestThreadWorktreeFallsBackToDataDir` |
| 文件夹上传共享、旧位置回退 | HTTP 层 `TestFolderThreadsShareUploadsOverHTTP` | 同文件夹两个线程互相读到上传；旧 `_sessions/<id>` 里的文件只对该线程可读 |
| 真实进程池整条链路 | 一次性测试（未提交，需要本机已登录的 `claude`）：`ClaudeAgents()` 的常驻进程池，真仓库、真 `claude`。`Start` → 第二轮在常驻进程里读到第一轮写的文件 → `Resolve`（回收进程）→ `Reopen` → 再发一轮 → `Merge` | cwd 在 `.claude/worktrees/`；重开后的轮次记得最初的任务（resume 成功）；合并后主仓库有 feature.txt、`git status` 为空 |
| 整体 | `projects/` 下 `go test ./... -count=1` | 全过 |

没有验证、也无法在当前环境验证的：
- Windows：没有 Windows 环境。路径全部用 `filepath` 拼接，exclude 行固定写正斜杠（git 的格式），但未实跑。
- NFS / 容器共享卷上的 `git worktree`：环境里没有这类挂载。`git worktree` 在网络盘上的锁语义由 git 决定，放到多容器那一步（本设计之外）再验。
- 线程里 agent 自己调用 `EnterWorktree`：本设计不依赖它，只在提示里留了说明，未验。

## 讨论中放弃的方向

| 方向 | 为什么不 |
|---|---|
| 共享目录放仓库里（`.tingly/`） | F5：线程在 worktree 里写不进去 |
| worktree 放兄弟目录或 TB 目录 | F6：`EnterWorktree` 每次审批；本方案不用 `EnterWorktree`，但 `.claude/worktrees/` 是官方默认位置，工具和用户都认 |
| 写前拦截 + 按需 `EnterWorktree`（lazy worktree） | 能用（F3、F4、F7），但要钩子、新字段和流式模式验证，复杂度不匹配当前收益；留给“就地编辑”设置之后再看 |
| tingly-fuse 作为隔离机制 | 视图路径固定在它的状态目录，与 `.claude/worktrees/` 惯例不兼容；等它的“宿主可见”落地再看 |
| 项目数据做成 Server tool | 另线讨论；本方案的接口（逻辑地址）可以以后直接暴露 |
| 共享内容放 `.git/` 内 | 无仓库项目没有 `.git` |

## 与已有决定的关系

- **[TEP-2112](https://github.com/tingly-dev/tingly-box/issues/2112)**（已关）：把 Projects 数据根移到 `~/.tingly-project`，使线程 cwd 的上级目录不含 Tingly Box 的密钥；其中“把 worktree 放进项目目录”被否决，理由是污染仓库、且无仓库项目不适用。本设计改变了前半个理由：`.claude/worktrees/` 通过 `.git/info/exclude` 忽略，不出现在 `git status`，也不改用户的 `.gitignore`；后半个理由仍成立，所以**无仓库项目的 cwd 仍在 Projects 数据目录**。安全上的出发点（cwd 上级不含 TB 密钥）依然满足：worktree 的上级是用户自己的仓库。
- **[#2084](https://github.com/tingly-dev/tingly-box/issues/2084)**：提出的 A（lazy worktree）、B（`.worktreeinclude`）、C（就地编辑）。本设计第一步只做位置与共享；A 经本次验证可行但不进第一步；B、C 作为后续。
- **[projects-attachments.md](../../.design/projects-attachments.md)**：文件夹线程的上传“每线程一块、互相看不到”是当时有意的（也避免落进用户仓库）。现在改为 per-folder：仍在 Projects 数据目录里（不进用户仓库），但同一文件夹的线程共享。

## 设计

### 磁盘布局

```
~/.tingly-project/projects/<p>/          （仓库之外）
├── memory/                  项目记忆；autoMemoryDirectory 指向这里
├── library/uploads/         上传（项目与文件夹项目一致）
├── library/outputs/<slug>/  无仓库线程的产出
└── threads/<id>.settings.json

<repo>/.claude/worktrees/<slug>-<id6>/   新线程的 cwd（有仓库）
<data>/projects/<p>/threads/<id>/        无仓库线程的 cwd（空目录，与现在相同）
```

- 建 worktree 时，把 `/.claude/worktrees/` 写入 `<repo>/.git/info/exclude`（对所有 worktree 生效，不动用户的 `.gitignore`）。
- 仓库不可写或建失败时，回退到 `<data>/projects/<p>/threads/<id>/`（旧位置），并在线程里记一条系统事件。
- `.worktreeinclude` 由 Claude 自己创建的 worktree 才处理；我们自己建的需要自己复制，列为后续。
- 老线程的 `cwd` 不动。归档仍只归档不清理（后续单独处理）。

### 启动设置

`threads/<id>.settings.json` 现在只有 `permissions.additionalDirectories`。新增：

```json
{
  "autoMemoryDirectory": "<data>/projects/<p>/memory",
  "permissions": { "additionalDirectories": ["<library>", "<memory>", "<其他仓库>…"] }
}
```

`autoMemoryDirectory` 对所有项目线程设置（有仓库的也设），让一个项目只有一份记忆：Projects 自己的 `MEMORY.md` 机制与 Claude 的原生 auto memory 指向同一目录。

### 上传

`UploadOwner` 对所有项目返回 `t.ProjectID`，文件夹项目也一样；草稿区 `_sessions/_new` 不再需要（上传直接进文件夹项目的 library）。旧线程的 `_sessions/<thread>/` 里的文件继续可读：解析附件时先找新位置，找不到再找旧位置。

### 启动提示

线程 prompt 的规则里按情况加一条：

- 有 worktree：已有的“你在自己的 worktree 和分支上”。
- 无 worktree 但有仓库（后续的就地编辑）：说明当前在主 checkout；需要隔离时可以用 `EnterWorktree`，且共享目录在仓库之外。
- 共享目录：记忆、上传、产出的绝对路径（现在已有），并说明它们对同项目的所有线程相同。

## 影响面

- `projects/internal/project`：`Layout`（新增 worktree 路径）、`Threads.Start`、`writeSettings`、`AddWorktree` 周边（exclude、回退）、`prompts.go`、`attachment.go`、`sessions.go`、`bind.go`。
- `projects/internal/server/module`：附件接口读取旧位置的回退。
- 前端：无界面变化。
- 数据库：无变化。
- 文档：本文件；`projects.md` §4.1 加一行指向本文件；`projects-attachments.md` 更新文件夹上传一段。

## 分步

| 步骤 | 内容 | 验收 |
|---|---|---|
| 1 | 本设计与 TEP | 评审 |
| 2 | 共享层：`autoMemoryDirectory` 注入；上传 per-folder，旧位置回退 | 同项目两个线程读到同一份记忆；同文件夹两个线程互相看到上传；旧线程的旧上传仍能打开 |
| 3 | worktree 位置：`.claude/worktrees/`、exclude、回退、提示更新 | 新线程在仓库下建 worktree 且 `git status` 干净；仓库只读时回退；resume 正常 |
| 后续 | 就地编辑设置；`.worktreeinclude`；归档清理 worktree；常驻流式模式下的 `EnterWorktree` 验证 | — |

## 风险

- F1、F2、F4 最初只用 `claude -p` 验证；实现后用流式输入输出补测了 resume 与记忆目录，一致（见“实现后的验证”）。
- `autoMemoryDirectory` 需要 Claude Code 2.1.x 的较新版本，旧版本会忽略该键，退化为按仓库/ cwd 的原生记忆，Projects 自己的 `MEMORY.md` 机制不受影响。
- 仓库内的 `.claude/worktrees/` 对用户可见（`ls`），也会被 IDE 的文件监听扫到；这是与官方一致的取舍。
- 多个线程共用一个仓库的 `.git`：并发 `git worktree add` 由 git 的锁处理，已有行为不变。
