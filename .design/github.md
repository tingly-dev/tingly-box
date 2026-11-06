# GitHub 集成：凭证在宿主，agent 只读，写操作走 tb

> 读者：要给 Desk / @cc / Projects 线程接上 GitHub 的人。记录为什么凭证不交给
> agent、为什么每个用户各建一个 GitHub App，以及第一版要做什么、不做什么。
> 方案对比与评估过程见项目文档《GitHub 集成方案评估》（2026-10-15）；这里只写定下来的契约。

状态：提案，尚未实现。

---

## 1. 决定

1. **凭证只在 tb 宿主上。** agent（Claude Code 进程）拿到的只有一个 1 小时有效的
   **只读** token，供它自己的 `gh` 读 issue / PR / CI 日志。
2. **写 GitHub 的动作全部是 tb 的 MCP 工具**（`tb_push`、`tb_pr_create` …），由 tb
   用另一个带写权限的 token 执行，按动作分级（§5）。merge、force push、删分支、
   release、改 workflow 或仓库设置不提供，App 也不申请这些权限。
3. **每个用户（每个 tb 部署）各建一个自己的 GitHub App**，用 App Manifest 流程一键
   创建，私钥只存在这个 tb 里。我们**不**统一提供 App：共用一个 App 意味着要么把私钥
   随安装包发出去（任何人都能给所有安装铸 token），要么由我们托管代签服务（成为所有
   用户的安全关键依赖，和本地优先的定位冲突）。将来做云端版时再加一个统一 App 作为
   可选来源，tb 内部工具不变，只换 token 来源。
4. **兜底：细粒度 PAT。** 建不了 App 的用户可以粘贴 PAT，走同一套写工具；代价是没有
   bot 身份（PR 作者是本人，无法自己 approve），也没有只读/写分级，agent 的 `gh`
   不注入 token。
5. **agent 开的 PR 作者是 `<app-slug>[bot]`。** 这样用户能 review 和 approve；App
   token 推送会正常触发 CI（不同于 Actions 的 `GITHUB_TOKEN`）。

## 2. 本机原地模式的边界

Desk / @cc 在用户目录原地跑 Claude Code，进程跑在用户账号下，理论上能绕过任何环境变量
去用用户自己的 `gh` 登录或 ssh key。所以这里是**默认改向 + 策略 + 审计**三层，不是
硬隔离：

| 层 | 做法 |
|---|---|
| 默认改向 | 启动时注入 `GH_TOKEN`（只读）、`GH_CONFIG_DIR`（tb 自己的空目录）、`GH_PROMPT_DISABLED=1`、`GIT_TERMINAL_PROMPT=0` |
| 策略 | 生成的 `--settings` 里加 `permissions.deny`（§6）；guardrails 命令策略补 shell 变形；写 token 注册为受保护凭证，泄露也只以 `TINGLY_CRED_…` 别名进模型 |
| 审计 | 所有写操作从 tb 发出，进 transcript，在 GitHub 上是 bot 身份 |

硬保证需要没有用户凭证的运行时（Claude Code sandbox、容器）。那时本设计不用改，
只是从“默认如此”变成“只能如此”。

## 3. 连接 GitHub（Manifest 流程）

```
设置页「连接 GitHub」→ 选 安装到：个人账号 | 组织（填组织名）
  → 浏览器 POST manifest 到
      个人  https://github.com/settings/apps/new?state=<nonce>
      组织  https://github.com/organizations/<org>/settings/apps/new?state=<nonce>
  → 用户在 GitHub 确认（可改名）
  → GitHub 302 到 redirect_url?code=<code>&state=<nonce>
  → tb POST https://api.github.com/app-manifests/<code>/conversions（无需鉴权，code 1 小时有效）
      ← id, slug, client_id, client_secret, webhook_secret, pem, owner
  → tb 存下（§4），再把浏览器送到 https://github.com/apps/<slug>/installations/new
  → 用户选仓库安装 → GitHub 跳到 setup_url?installation_id=<id>&setup_action=install
  → tb 记录 installation
```

- **为什么要问个人还是组织**：私有 App 只能装在创建它的账号上。个人账号下建的 App
  装不到组织仓库；组织下建需要组织 owner 或被授权管理 App 的成员。这是唯一必须问的
  选择，其余全部预填。
- manifest 内容：`name` 默认 `tingly-box-<login 或主机名>`（全局唯一，冲突时用户在
  GitHub 页面上改）；`url` 为 tb 地址；`redirect_url`、`setup_url` 为 tb 回调；
  `public: false`；`hook_attributes: {url: <tb>/api/v1/github/webhook, active: false}`
  （第一版不收 webhook，§7）；`default_permissions` 见下。
- 回调用浏览器跳转，`localhost` 地址可用。`state` 用一次性 nonce 防 CSRF。
- 权限：`contents: write`、`pull_requests: write`、`issues: write`、
  `checks: read`、`actions: read`、`metadata: read`。不申请 `administration`、
  `workflows`、`secrets`。

## 4. 存储与 token

- 新表（SQLite，`internal/db`）：
  - `github_apps`：id, slug, owner, client_id, client_secret, webhook_secret,
    private_key（加密存储方式与现有 provider key 相同）, created_at。
  - `github_installations`：installation_id, app_id, account, repository_selection,
    updated_at。仓库列表不落库，按需用 `GET /installation/repositories` 查。
  - `github_pats`：兜底 PAT，结构同 provider credential。
- token 铸造：`POST /app/installations/{id}/access_tokens`，body 带
  `repositories: [<repo>]` 和 `permissions`，按用途铸两种：
  - **读 token**：`contents/pull_requests/issues/checks/actions/metadata: read`，限定当前仓库，给 agent。
  - **写 token**：§3 的写权限，限定当前仓库，只在 tb 进程内使用，用完即弃。
  - 缓存到过期前 5 分钟，按 (installation, repo, 用途) 作 key。
- 仓库识别：会话工作目录的 `git remote get-url origin` → `owner/repo` → 找覆盖它的
  installation。找不到就不注入 token，写工具返回明确错误（“这个仓库没装 App，去设置
  里加上”）。

## 5. 写操作工具（MCP）

tb 在服务端暴露一个 MCP-over-HTTP 端点，按会话签发的 bearer 鉴权（工具调用能归到会话、
目录和仓库）；agentboot 用已有的 `--mcp-config` 把它挂给 Claude Code。

| 工具 | 做什么 | 默认策略 |
|---|---|---|
| `tb_push` | 在宿主侧执行 `git push <origin-url> HEAD:refs/heads/tb/<session-short>`；写 token 经一次性 `GIT_ASKPASS` 传入，不写进目录的 git config。只推这一个 ref，拒绝 force。 | 自动，进 transcript |
| `tb_pr_create` | 以 `tb/<session-short>` 为 head 开 PR，base 默认仓库默认分支 | 自动；同一仓库第一次时走 Prompter 问一次 |
| `tb_pr_update` | 改本会话开的 PR 的标题 / 描述 | 自动 |
| `tb_pr_comment` | 在本会话开的 PR 上评论、回复 review | 自动 |
| merge / close / release / 删分支 / 改设置 | 不提供 | 用户在 GitHub 上自己做 |

- 只能操作**本会话开的 PR**：tb 记录 (session → repo, branch, PR number)。
- 推送分支固定为 `tb/<session-short>`，不依赖 worktree：agent 在原地目录的任何分支上
  commit 都行，推出去的永远是这个 ref。worktree / 会话分支是后续改进，不是前置条件。
- 工具结果给 agent 的是可直接行动的数据（PR URL、编号、推送的 commit），错误带原因
  和下一步。

## 6. 启动参数改动

接入点：`internal/desk/turn.go` `launchOptions`、`remote/control/remoteagent/executor_claude.go`、
Projects 的 `Host.ThreadEnv`，以及 profile settings 生成（`internal/agent/cc_settings.go`）。

- env：`GH_PROMPT_DISABLED=1`、`GIT_TERMINAL_PROMPT=0` 始终注入；连接了 App 且仓库
  可识别时再注入 `GH_TOKEN=<读 token>` 和 `GH_CONFIG_DIR=<tb 数据目录>/gh-empty`。
  token 1 小时过期：常驻进程在每轮开始前检查，剩余不足 15 分钟则按 launch signature
  变化的既有逻辑重开进程（`desk.md` §3.1）。
- `permissions.deny`（合并进生成的 settings，不覆盖用户已有规则）：
  `Bash(gh auth:*)`、`Bash(gh pr merge:*)`、`Bash(gh repo delete:*)`、
  `Bash(gh secret:*)`、`Bash(gh release:*)`、`Bash(git push:*)`，以及
  `gh api` 的写方法（`-X POST|PUT|PATCH|DELETE`、`--method`）。deny 规则对
  `bash -c`、管道等变形拦不全，由 guardrails 的 shell 规范化匹配
  （`internal/guardrails/core/command_shell.go`）补一组同名默认策略。
- system prompt 追加一句：push 和 PR 用 `tb_push` / `tb_pr_*`，`gh` 只读。

## 7. 事件回流（第二阶段）

CI 结果和 review 评论唤醒对应会话 / Projects 线程。本机 tb 通常没有公网地址
（同 `cursor.md`），默认轮询本会话开的 PR：`GET /repos/{o}/{r}/pulls/{n}`、
check-runs、review comments，带 `If-None-Match`（304 不扣配额）。有公网地址时在设置里
打开 webhook（manifest 里已预留，`active` 改为 true，用 `webhook_secret` 校验签名）。

## 8. 分阶段

1. **收住现状**（不依赖 App）：§6 中与 token 无关的 env、deny 规则、guardrails 策略、
   system prompt 提示。验收：Desk 会话里 `gh auth token`、`git push` 被拒，transcript
   有记录。
2. **连接 + 写工具**：§3、§4、§5，加 feature flag `github`（`constant.ExtensionGitHub`，
   默认关闭，关闭时 `/api/v1/github` 全部 404，照 Desk 的 gate）。设置页一个
   「连接 GitHub」入口，显示已连接的 App、安装的账号和仓库数。验收：在一个测试仓库
   上，Desk 会话能读 PR、`tb_push` 推出 `tb/<id>`、`tb_pr_create` 开出 bot 作者的 PR。
3. **事件回流**：§7。

## 9. 不做（本期）

- 统一的 tingly-dev App 与托管代签服务。
- 让 agent merge 的开关。
- GitLab / Gitea / GitHub Enterprise Server（Manifest 流程 GHES 也有，留待需求出现）。
- 自动建 worktree、会话分支生命周期管理。

## 10. 待验证

- 组织仓库装 App 的审批门槛对团队用户的实际影响。
- `permissions.deny` 对 `gh api` 写方法的匹配写法，以及与用户已有 settings 的合并顺序。
- 常驻进程换 token 的频率是否会明显打断长会话；必要时改为经 tb 本地代理下发 token
  （`gh` 的 `GH_HOST` / credential helper 路线）。
