# 线程权限：项目级 GitHub 开关与 `request_permission`

> 状态：**§2 已实现**（#2740 后端，用户确认了分组与默认值；§2.6 界面 #2742）；**§3 `request_permission` 已实现**（#2741，见 §3.7 与提案的差异），**§3.8 改为“批准后直接执行”**（用户反馈：批了还得再调、还可能再被拦；取代 §3.1–§3.7 中与之冲突的部分）。
> 建在 #2734（工具自己弹审批卡：`approval.go`、`Links.authorize`）、#2736（`close`）、#2737（`merge` 的 `head` 与 merge commit）之上。
> 相关：[github-tools.md](github-tools.md)（工具与边界）、[mcp-tools.md](mcp-tools.md)（工具粒度）、[mcp-tools-coverage.md](mcp-tools-coverage.md)（场景卡点）、[`../../.research/agentboot-stop-and-permissions.md`](../../.research/agentboot-stop-and-permissions.md) §2（permission mode 本身的实时下发、收紧与放宽的边界）、项目记忆 `ask-dont-refuse-beyond-boundary`、`mcp-tool-granularity`。

## 1 问题

用户原话：“这些 GitHub 的功能开关都可以在项目配置里面决策，而不是默认做卡点……默认先不开启自动放行。希望自动化的用户可以开启自动化，不希望自动化的用户可以弹出卡片来决定是否执行。”以及：“再增加一个工具用来索要权限，因为原生的这个权限并不一定能够生效，且每次都弹出。”

现状的“要不要问”散落在四处，规则写死在代码里：

| 动作 | 今天谁决定问不问 | 怎么问 |
|---|---|---|
| `create_issue` `create_pr` `update`（自己的） `comment` | 项目设置 `thread_github_access`（默认 allow，#2579） | Claude Code 的 ask 规则 |
| `merge` | 写死 `alwaysAskTools` | Claude Code 的 ask 规则（卡上只有参数） |
| `close` | 写死：每次都问（#2736） | 工具自己弹卡（写明谁开的） |
| 越界（他人分支、默认/保护分支、他人 PR 的 `update`、没挣到的强推） | 写死（#2734） | 工具自己弹卡 |
| 自有分支 lease 强推 | 写死：不问（#2734） | — |

Claude Code 原生的权限提示又不可靠：auto 模式分类器会静默拒绝（本项目里 `task codegen`、运行下载的二进制、子代理 worktree 里 `git rebase` 都被拦过），`bypassPermissions` 可能跳过 ask 规则（未探针），而且每次都弹。

## 2 设计：一个闸门，项目里五个开关

### 2.1 五个开关（能力组）

按“用户会怎么想这件事”分组，不按工具分（遵循工具粒度原则：开关少、每个开关一句话说得清）：

| 开关 | 覆盖的动作 | 为什么归一组 |
|---|---|---|
| `publish` 发布 | `create_issue`；`comment`；`create_pr` 开 PR、再推送本线程分支、改本线程 PR 的标题/正文/base；`update` 本线程开的 issue/PR；`merge` 的 `merge_request`（只开 PR） | 日常交付，都是“把本线程的工作公开出去”，都可撤回或可再改 |
| `rewrite` 改写自有分支 | `create_pr(force=true)`：对本线程建的、或本线程开过 PR 的分支做 `--force-with-lease` | 改写已推送的历史，风险高于普通推送，但只动自己的东西，lease 保证不覆盖别人的提交 |
| `others` 动别人的 | `update` 别人开的 issue/PR；普通推送另一个线程的分支、为它开 PR（堆叠） | 越出本线程的边界，但不改写历史 |
| `close` 关闭 | `close` 任何 issue/PR（本线程的、别的线程的、GitHub 上别人开的） | 对外可见、会通知别人，但 GitHub 上可重开 |
| `merge` 合并 | `merge` 的 `squash` / `merge`（merge commit），本线程分支或堆叠分支 | 不可撤销 |

组合规则：一个动作同时属于两组时两组都要开（例如合并另一个线程的分支 = `merge` + `others`），否则弹卡。

### 2.2 默认值与已有项目

- **新项目：五个全关。** 每个写 GitHub 的动作都弹卡，用户按需打开。这是用户的要求，也是对 #2579（Projects 工具默认免卡）在 GitHub 写操作上的有意回调：#2579 解决的是“卡片太多、在 auto 模式下被分类器拒”，那个机制（服务端整体 allow + 提示兜底）保留，只是“问不问”改由工具自己按开关决定。
- **已有项目：保持原行为。** 迁移一次：`thread_github_access` 为空或 `allow` → `publish` 开，其余关；为 `ask` → 全关。理由：已有项目的用户已经习惯日常发布不弹卡，静默改成每次弹卡是回归；而 `merge`、`close`、越界今天本来就弹卡，关着正好不变。唯一的行为变化是 #2734 新增的“自有分支 lease 强推不问”变成默认要问（`rewrite` 默认关）——#2734 尚未合并，没有用户依赖它。
- **文件夹会话**：没有项目设置界面可改，新建时给 `["publish"]`（与今天一样日常发布不弹卡），其余照常弹卡。
- `thread_github_access` 字段读一次用于迁移，之后由新字段取代；API 仍接受旧字段一个版本（映射到 `publish`），UI 只用新字段。

### 2.3 永远不能自动放行的动作（开关开着也弹卡）

| 动作 | 为什么 |
|---|---|
| 以仓库默认分支或 GitHub 保护分支为 head 推送 / 开 PR | 直接改主干；保护规则本身就表达了“这里要人看” |
| 强推一个本线程既没建、也没开过 PR 的分支（别的线程的、仓库里碰到的） | lease 只防“没见过的提交”，防不了“把别人的分支改写成我的” |

这两类每次都是动作自己的卡，卡上写明分支和归属；“对本线程允许”记住的是**这个确切目标**（今天 #2734 的 Key），不是整组。`request_permission` 的授权也不覆盖它们（§3.4）。

仍然是错误、谁也放行不了的（不变）：head 等于 base、head 不是本地分支、base 不在 GitHub、lease 不成立。

### 2.4 闸门：全部在服务端，由工具自己执行

```
gate(thread, groups, approval):
    if 动作属于 §2.3            → 弹卡（Key = 确切目标）
    if groups 全部开启          → 放行
    if 本线程“对本线程允许”过这组 → 放行
    否则                         → 弹卡（Key = 组名，“对本线程允许”记住整组）
```

- 卡就是 #2734 的审批卡（网页和 IM 都能答），由工具在调用里自己弹，调用在用户回答后返回。
- **Claude Code 一侧不再有 GitHub 工具的 ask 规则**：`alwaysAskTools` 与 `thread_github_access` 生成的 ask 规则都删掉，服务端整体 allow + 提示兜底（#2579）保持。所以“always-ask 列表”不再是写死的规则表，而是开关本身，在闸门里求值。好处：卡上能写明目标（规则只能显示参数）；与 Claude Code 的权限模式无关（`bypassPermissions`、auto 分类器都碰不到它）；只有一张卡。
- 自动放行不另记一条 system 事件（每次发布都记一条太吵）：工具调用本身就在线程记录里；经卡片批准的才有审批事件，所以“项目放行”和“用户点了”仍分得开。

### 2.5 数据

- `projects.github_auto`：JSON 列，开着的组名数组（`["publish"]`）。`NULL` = 旧项目：`Open` 时的迁移按 §2.2 补上（列名注意：GORM 把 `ThreadGitHubAccess` 存为 `thread_git_hub_access`）；新项目创建时写 `[]`。不加新表（项目记忆 `prefer-column-over-new-table`）。
- 线程的授权：`threads.grants` JSON 列（§3.3），只由一个事务方法写，`SaveThread` 不写它（同 `Links`）。

### 2.6 UI

项目设置对话框把“Issues and PRs: allowed / ask each time”下拉换成一个小节“GitHub：自动放行”，五个开关，每个一行：名称、一句说明、开关；小节底部一行灰字：“关闭时每次弹卡询问；推送到默认/保护分支、强推别人的分支总是询问。”en / zh / ru 三语。截图用 mock 模式、隔离 HOME。

**已实现**（`ProjectSettingsDialog.tsx` 的 General 页）：开关按 `GITHUB_GROUPS`（与 `project.GitHubGroups` 同序）排列，行内是名称、一句说明、MUI `Switch`；状态来自 `project.github_auto`（缺失或空 = 全关），与其他字段一样由页内“保存”统一提交，只发 `github_auto`（不再发 `thread_github_access`），改动前后按分组顺序比较来判断是否 dirty。手机壳（`?ui=mobile`）里同一对话框以底部抽屉呈现，说明文字自动折行，可用。mock 项目带 `github_auto`（`p-latency` 为 `["publish"]`）。API 类型由 `projects/openapi.json` 重新生成。

## 3 `request_permission`：线程明确地索要权限

### 3.1 工具

线程专用，一个写工具（粒度原则：一个动词；`group` / `rule` 是目标，不是动作选择器）：

| 输入 | 必填 | 含义 |
|---|---|---|
| `action` | 是 | 要做的具体事，如 “force-push branch tp/a with lease”、“run task codegen with an isolated HOME” |
| `reason` | 是 | 为什么需要 |
| `group` | 否 | §2.1 的一个开关名：授权由 GitHub 闸门直接兑现 |
| `rule` | 否 | 一条 Claude Code 权限规则，如 `Bash(task codegen)`、`Bash(./bin/tingly-project *)`：授权写进本线程的设置 |
| `scope` | 否 | `once`（默认）或 `thread`（直到线程关闭或到期） |
| `minutes` | 否 | `thread` 授权的有效期 |

`group` 与 `rule` 至多给一个；都不给 = 只记录（§3.3）。

### 3.2 卡片

走现有提示器（`threadPrompter.wait`，`EvApproval`），网页和 IM 同一张卡：“线程请求权限：<action>”，原因、范围、有效期、确切的规则或组名。**这张卡永远由用户回答**：提示器只对带建议答案的提问自动作答，从不自动批准审批卡（已读 `autoDeadline`：只对 `EvAsk`）；没有任何开关覆盖 `request_permission` 本身；“对本线程允许”在这张卡上不提供（否则一次点击就变成以后所有请求都放行）。

调用在用户回答后返回：`granted (scope thread, until …; request a1b2)` 或 `denied: <用户的话>`。

### 3.3 授权做什么（如实说明能强制什么）

| 授权对象 | 效果 | 能强制吗 |
|---|---|---|
| `group`（GitHub） | 存进 `threads.grants`；闸门（§2.4）在开关关着时认它：`once` 被下一次该组的调用消耗，`thread` 直到线程关闭 / 归档或到期 | **能**，完全在服务端 |
| `rule`（Claude Code 原生，Bash 等） | 只支持 `scope: thread`：把规则加进本线程设置文件的 `permissions.allow`；agentboot 在下一轮开始时把权限规则**在线下发**给常驻进程（`applyLive` → `ApplySettings`，不重启，满足“不重启”原则）；当前这一轮内是否能立即生效需要探针（见 §3.5）；到期或关闭时从文件删除、在线撤回 | **部分**：<br>· 规则对 Claude Code 自己的权限检查有效（含 `default`/`acceptEdits`；auto 模式下 allow 规则是否先于分类器——对 MCP 工具已实测成立，对 Bash **待探针**）<br>· 用户 profile 里的 deny 规则仍然优先<br>· 操作系统级沙箱（bwrap / seatbelt）不是权限规则，授权解不开<br>· 进程在授权期间重启过的话，规则成了启动时的基线，到期后要等下次重启才真正撤回（`liveDelta` 的已知限制） |
| `once` + `rule` | **拒绝**并说明：一条规则按模式匹配，无法做到“只这一次”；一次性的请用 Claude Code 自己的提示 | — |
| 都不给 | 只记录：卡片和用户的回答留在线程记录里，成为系统证明的授权（可被引用），不强制任何东西 | 否，但可核验 |

### 3.4 防止越权

- 授权不覆盖 §2.3 的动作：它们照样弹动作自己的卡（卡上是确切目标；授权卡上的是线程自述的意图，两者不能互相替代）。
- `rule` 拒绝：`mcp__tingly-projects…`（我们的工具由闸门管，不靠规则）；没有模式的整工具规则（`Bash`、`Write`、`Edit`、`Bash(*)`）——卡片上会出现“放开整个 Bash”这种用户很难评估的东西；拒绝时告诉线程写具体命令前缀。
- `request_permission` 自身没有自动放行；用户拒绝后线程不应重复请求（工具描述写明；同一 `action` 在一轮内第二次请求直接返回上次的拒绝）。

### 3.5 记录与可见性

- 线程记录里：审批卡（含用户的回答、谁在哪答的：网页 / IM）+ 一条 system 事件 “The user granted: <action> (scope …, until …)” 或 “The user denied: …”。payload 带 `user_answer` 事实：`request_id`、`action`、`group|rule`、`scope`、`expires_at`、`answer`、`answered_via`、`at`——与“改进线程直接指令的协调提示词”设计里“用户的回答作为 `user_answer` 事实出现”对齐（那份设计不在本分支，字段名在实现时与它核对）。
- 协调者：`get_thread` 能看到；线程的报告里应写明“已获用户授权（request a1b2）”。
- 授权到期 / 线程关闭：system 事件 “Permission expired: …”。

### 3.6 待探针（实现时做，不预设结论）

1. auto 模式下，线程设置文件里的 `Bash(...)` allow 规则是否让分类器不再拦截该命令（MCP 工具已证实 allow 生效）。
2. `ApplySettings` 能否在一轮进行中下发（若能，`rule` 授权当轮生效；否则下一轮生效，工具返回里如实说“从下一轮起生效”）。
3. `bypassPermissions` 下 GitHub 闸门的卡照常出现（闸门在服务端，预期与模式无关，补一条测试）。

### 3.7 实现说明（与上文的差异）

- 授权存在 `threads.grants`（`db.ThreadGrant`：`id`、`action`、`group|rule`、`scope`、`granted_at`、`expires_at`），只由 `Store.UpdateThreadGrants` 写；`SaveThread` 不写它。线程关闭时清空。
- 闸门（`Links.gate`）在开关没开时认授权：每个没开的组都要有一条有效授权；优先用线程范围的，一次性的在此被删掉。§2.3 的动作不看授权。
- 卡片是普通审批卡（`threadPrompter.wait`），不设“对本线程允许”的 Key；用户在卡上点“对本线程允许”时，**只把这一条请求**升级为线程范围（不会让以后的请求免卡）。
- `rule` 授权：写进线程设置文件的 `permissions.allow`（`grantRules`，过期的不写），然后调用新的 `engine.RefreshSettings` 立即把权限规则下发给常驻进程（agentboot 的 `ApplySettings` 注释写明“从下一次工具调用起生效，不重启”）；没有常驻进程或下发失败时，下一轮开始时照常下发，工具返回里如实说明。**未用真实 Claude Code 探针**：一轮进行中下发是否真的在当轮生效、auto 模式分类器是否让位于 allow 规则，仍是 §3.6 的探针 1、2。
- 文件夹会话拒绝 `rule`（它的设置来自 profile，不经 `writeSettings`）。
- 线程跑在 profile 下时（`workerSettings` 每轮由 profile 的设置加项目权限拼出），`rule` 授权不写项目设置文件、不即时下发，从下一轮生效（拼设置时 `grantProjectAccess` → `grantRules` 带上它）。
- `rule` 还拒绝：以 `gh`、`git push` 开头的 Bash 规则（这些由 GitHub 工具按项目设置管）、以 shell 或包装命令（`bash`、`sh`、`env`、`xargs`、`sudo` 等）开头的（等于整个 Bash），以及前缀后只剩通配的（`WebFetch(domain:*)`）。
- 过期在下一轮生效：常驻进程在当前这一轮里还拿着规则；工具返回里写明。
- 用户回答时线程已被关闭的，不记授权（关闭清空了其他授权，这条不能留下）。
- `engine.RefreshSettings` 有单元测试（只发变化的部分、失败下次重发）；它与一轮开始时的 `applyLive` 不互斥，两者同时发时可能有一次是多余的下发，不会丢规则（下一轮按文件重算）。
- “同一请求被拒后不重复弹卡”只写在工具描述里，没有在服务端强制。
- 记录：拒绝 / 同意各一条 system 事件（“The user denied: …” / “The user granted: … (scope; request xxxx)”），payload 是 `{"user_answer": {request_id, action, group, rule, scope, allowed, answer, answered_via}}`。

### 3.8 批准后直接执行（取代上文冲突处）

用户：“ask 了、同意了，就能直接执行”，“申明了权限就很确定了，就是可以执行了”；以及“环境分开了会影响 claude 的执行，看能否把权限直接注入给 claude 实例”。验收：一次 `request_permission` + 一次同意 → 动作在线程自己的 Claude Code 里执行；不弹第二张卡、不需要额外手动步骤、之后不会被分类器静默拦截；授权与执行都有记录。

**探针**（真实 Claude Code 2.1.259 + 脚本化模型，分类器一律拦截；方法与数据见仓库根 `.research/permission-grant-probe.md`）：

1. `apply_flag_settings` 在一轮进行中下发的 allow 规则，从下一次工具调用起生效，auto 模式下分类器不再被调用（不重启）。§3.6 探针 1、2 由此回答。
2. 已被拦的调用，规则下发后再调一次即通过。
3. auto 模式**丢弃解释器的通配规则**（`Bash(python3 -c *)`、`Bash(node -e *)` 下发后照拦），同一命令的精确规则生效。
4. 管道 `a | b` 只给整行规则不够，再给每一段的精确规则就通过。
5. 旧实现里 profile 线程的规则要等下一轮，所以“批了这一轮还被拦”。
6. auto 模式下被拦的调用不经过 `can_use_tool`，没有挂起的原生提示可以去“回答”；所以授权在调用之前（或被拦之后），模型自己的那次调用就是执行本身。

**决定**：

- **执行留在线程自己的 Claude Code 里**：Projects 不代为运行命令（环境、cwd、shell 状态、工具上下文都会分家，用户明确不要）。授权写进线程实际运行的设置文件并即时下发给常驻进程；线程紧接着用自己的 Bash 执行。
- **`command`（新）**：一条确切的命令行。同意后写入 `Bash(<整行>)` 及每一段的精确规则（`commandRules`），即时下发；工具返回“现在就用你的 Bash 按原样运行它；之前被拦的，再运行一次即可通过”。拒绝含 `*` 的命令（规则里 `*` 是通配，会超出卡上写的范围）和 `gh` / `git push`（归 GitHub 工具）。
- **`rule`** 保留给要多次运行的模式（`Bash(npm run build *)`、`WebFetch(domain:…)`）；新增拒绝以解释器开头的规则（`python`、`python3`、`node`、`deno`、`bun`、`ruby`、`perl`、`php`、`npx`、`bunx`）：等于放开任意命令，且 auto 模式会丢掉它，批了也没用——改用 `command`。
- **去掉 `group`**：GitHub 工具本来就自己弹卡、同意后当场执行（§2.4），`request_permission(group)` 再加一次是重叠路径，而且先授权再调用正是“批一次、再调一次”的来源；合并他人分支（`merge` + `others`）或 §2.3 的动作还会再弹第二张卡。现在工具描述直接说：GitHub 工具直接调用，它们自己问。闸门不再读授权（`grantsCover` 删除）；库里已有的组授权不再生效（线程关闭时本来也会清空）。
- **作用域**：`once`（默认）= 本轮；命令授权被一次无错误的运行用掉；返回错误的那次（命令失败，或被授权解不开的东西拦下）不用掉，留给本轮重试，记录里引用错误的第一行；剩下的在本轮结束时删除，从没被调用过的在线程里写明“没有运行”。规则从运行中的进程撤回发生在下一轮开始（`applyLive`），不在本轮中途撤（中途撤可能与该调用的权限检查竞态）。`thread` = 直到线程关闭或 `minutes` 到期。`rule` 现在也可以是 `once`。
- **profile 线程**：授权时用 `Launcher.LaunchEnv` 取 profile 的设置，`workerSettings` 重建会话设置文件，再 `RefreshSettings` 即时下发——与非 profile 线程一样当轮生效。下发失败时如实说“从下一轮起”，并让线程结束本轮而不是去跑注定被拦的命令。
- **文件夹会话**（IM 会话就是它）：原先拒绝 `command` / `rule`，现在支持——`sessionSettings` 每轮在宿主（profile）设置之上拼出会话设置，授权一并写入（`grantRules`），授权时即时重建并下发。
- **记录**：卡片和 “The user granted/denied …”（payload `user_answer`，多了 `command`）照旧；线程用 Bash 运行被授权的命令后，紧跟在结果下面记一条 “Ran with the user's grant (request xxxx): <命令>; it succeeded.” 或 “…; it returned an error: <第一行>.”（payload `granted_run`：`request_id`、`command`、`tool_use_id`、`failed`）。协调者在线程记录里能看到授权、调用、结果三者连在一起。

**端到端验证**（`.research/permission-grant-probe/e2e.py`，真实 Projects + 真实 Claude Code，项目线程 auto 模式）：项目线程先请求再运行——1 张卡，命令执行，分类器 0 次；先被拦再请求——1 张卡，重跑通过；拒绝——工具返回“不要运行”，命令照拦；文件夹会话先请求再运行——1 张卡，命令执行，分类器 0 次。

仍不能保证的（如实）：用户 profile 的 deny 规则、线程自己的 ask 规则（如强推）、操作系统沙箱（bwrap / seatbelt）照样优先；同一轮里同一条 `once` 命令跑第二次也会通过（规则在进程里留到下一轮开始）。

## 4 实现与拆分

| PR | 内容 | 叠在 |
|---|---|---|
| P1 闸门与开关（后端） | `github_auto` 列与迁移；`gate`；`create_pr` / `update` / `comment` / `create_issue` / `close` / `merge` 全部经闸门；删 `alwaysAskTools` 与 `thread_github_access` 生成的 ask 规则；`PATCH /projects/{id}` 接受 `github_auto`（旧字段映射）；工具描述；golden；闸门的变异检查 | #2737 |
| P2 设置界面 | 五个开关的小节，en/zh/ru，截图；需要 codegen（projects 的 openapi + `pnpm gen:api`），被拦时先放占位类型并说明 | P1 |
| P3 `request_permission` | 工具、`threads.grants` 列、闸门认授权、`rule` 写设置文件与在线下发、到期；§3.6 的探针 | P1 |

测试要点：每组开 / 关各走一遍；组合（`merge` + `others`）；§2.3 的动作开关全开仍弹卡；授权 `once` 被消耗、`thread` 到期与关闭时失效；被拒绝的请求不重复弹；旧项目迁移（allow → `publish`、ask → 全关、空 → `publish`）；golden 工具列表；变异检查：去掉闸门里的开关判断、去掉 §2.3 判断、去掉 once 消耗，各有测试失败。

## 5 需要用户确认

1. 五个开关的分组（`publish` / `rewrite` / `others` / `close` / `merge`）是否合适，`merge_request` 归 `publish` 是否同意。
2. 已有项目“保持原行为”（`publish` 随旧设置，其余关），新项目全关。
3. §2.3 两类动作永远弹卡。
4. `request_permission` 的 `rule` 只支持线程范围、拒绝整工具规则。
