# Managed Agent — 给仓库、定环境、远程推进的托管开发 Agent

> Status: **proposal / 方案 v0** · Date: 2026-09-06
>
> 对标物：Anthropic Managed Agents / Claude Code on the web（选仓库 → 选环境 →
> 隔离容器里跑 agent → 远程交互推进 → 产出分支/PR）。我们不追求同等完成度，
> 目标是把 tingly-box 已有的 gateway、remote control、agentboot、task 四块拼成
> 一条"托管开发任务"的闭环。
>
> 相关：[`bot-arch.md`](bot-arch.md)、[`remote-cc-profile.md`](remote-cc-profile.md)、
> [`afk.md`](afk.md)、[`bot-interaction-api.md`](bot-interaction-api.md)、
> [`team.md`](team.md)、[`security.md`](security.md)、[`ux-principles.md`](ux-principles.md)。

目录：

1. 一句话目标与边界
2. 我们已经有什么（资产盘点）
3. 领域模型：Source / Environment / Workspace / AgentSession / Trigger
4. 架构与数据流
5. 关键决策（每一条都写明"为什么不选更简单的那个"）
6. API 与前端 IA
7. IM（remote control）接入
8. 分阶段落地
9. 待决策事项
10. UX 原则自检

---

## 1. 一句话目标与边界

**目标**：用户在 tb 里指定一个 git 仓库和一个运行环境，写一句任务，tb 在一个
**独立工作区**里启动 Claude Code 跑这个任务；用户可以在 web UI 或 IM（Telegram /
飞书 / …）里随时插话、审批、打断；任务结束后拿到 **diff + 分支 + 一键 PR**。
关掉浏览器任务继续跑，回来还能接着聊。

**明确不做（本期）**：

- 不做云端多租户调度 / 计费 / 弹性扩容。tb 是单机（或小团队）产品，跑在用户自己
  的机器或一台服务器上。
- 不做第二套 agent runtime。执行体就是 Claude Code CLI（走 agentboot），模型流量
  走 tb gateway。AFK（`@tb`）继续做 Smart Guide，不承担代码任务。
- 不自己实现 git 托管、code review、CI。PR 是终点，交给 GitHub/GitLab。

**为什么值得做**：tb 已经把"agent 的模型接入、远程控制、审批交互、profile 配置"
全做完了，缺的只是"agent 在哪跑、跑在什么代码上、结果去哪"这三件事。这是从
"给 agent 供模型"走到"托管 agent 干活"的自然一步。

---

## 2. 我们已经有什么（资产盘点）

| 需要的能力 | 现有资产 | 复用方式 | 缺口 |
|---|---|---|---|
| 启动/驱动 Claude Code、stream-json、权限/ask 路由 | `agentboot/`（`Runner` + `AgentDriver` + `process.Factory` seam） | 直接复用；`process.Factory` 换成容器实现 | 一个 `DockerFactory` |
| 模型访问、profile（`--settings`）、用量归属 | gateway + `agent.MaterializeCCProfileSettings` + model token | 容器内 CC 指向宿主 gateway | 会话级 token / 用量按 session 归属 |
| 远程交互：审批、AskUserQuestion、通知 | `remote/control/bot` 共享 IMPrompter、notify consumer、`/api/v1` interact API | 一个 session 的 prompt 走同一个 prompter | session ↔ chat 的路由绑定 |
| 会话生命周期、transcript | `remote/session`（Session + Transcript）、`agentboot/history` | 扩展字段（workspace、branch、status） | 持久化进 SQLite（见 `remote-storage.md`） |
| 持久任务、排队、取消、cron | `internal/task`（目前**零调用方**） | 作为 session 执行的 supervisor 与 trigger 引擎 | 接上真实 runner |
| 认证 / 多用户 | `UserToken`、Team + Sharing Key | 控制面沿用 UserAuth；容器内只拿 model token | 无 |
| 前端 streaming 渲染 | `remoteagent/stream.go`、`output.go` 的聚合逻辑 | 抽成 web 可用的事件流（SSE） | session 页面 |
| Guard rails | `internal/guardrails` | agent 流量自然经过 | 无 |

结论：**新代码的主体是"工作区 + 环境 + 会话编排 + 一个页面"**，agent 执行本身
不用重写。

---

## 3. 领域模型

五个名词，一词一层，避开 `bot-arch.md` §9 里已经用掉的词（bot / channel /
scenario / profile 都不复用）。

```
Source ──────┐
             ├──▶ Workspace ──▶ AgentSession ──▶ Artifact(diff / branch / PR)
Environment ─┘        ▲
                      │ 创建
Trigger ──────────────┘
```

### 3.1 Source — 代码从哪来

```
Source {
  id, name
  kind:        git
  url:         https://github.com/org/repo.git | git@...
  default_branch
  credential_id    // 引用 secret store，见 §5.3；null = 公开仓库
}
```

### 3.2 Environment — agent 跑在哪、带什么

```
Environment {
  id, name
  runtime:     local | docker            // P3: remote-runner
  image:       string                    // docker: 镜像；local: 忽略
  setup_script: string                   // clone 后、agent 启动前执行（装依赖）
  env:         map[string]string         // 明文变量
  secrets:     []secret_ref              // 注入为 env，但不进日志、不进 transcript
  network:     none | proxy | full       // docker 有效；proxy = 只允许经宿主 tb 出网
  cc_profile:  claude_code[:<id>]        // 沿用 remote-cc-profile.md 的命名，不发明新语法
  resources:   {cpu, memory, disk}       // docker 有效
}
```

**默认值**：安装后自动存在一个 `local` Environment（runtime=local，
cc_profile=claude_code）。检测到 docker 可用时，**提示**而不是自动创建 docker
Environment（ux §6：合理默认优于开关；§12：副作用限定在当前表面）。

### 3.3 Workspace — 一次物化的 checkout

```
Workspace {
  id
  source_id, environment_id
  path:        ~/.tingly-box/workspaces/<id>/repo   // 宿主侧真实路径
  base_ref:    main
  branch:      tb/<slug>-<short-id>                 // agent 工作分支
  container_id (docker)
  state:       provisioning | ready | running | idle | reclaimed
  last_active_at
}
```

Workspace 是 **宿主上的一个目录**（clone 或 worktree），docker 模式下 bind-mount
进容器。它是短命的：session 归档后按 TTL 回收目录/容器，但 **branch 已经 push
出去了**，代码不丢（ux §10：done ≠ locked）。

### 3.4 AgentSession — 一段对话

```
AgentSession {
  id, title
  workspace_id
  status:      queued | running | waiting_input | idle | done | failed | archived
  cc_session_id                 // Claude Code 自己的 session，用于 --resume
  permission_mode               // 同 remote-cc-profile.md §2.1 的运行时覆盖
  owner (user/team)
  created_by:  web | im:<bot>:<chat> | trigger:<id>
  usage:       {input, output, cache_read, cost}   // 由 gateway 按 session token 归集
  events:      append-only（transcript + tool 事件 + 审批记录），复用 remote/session Transcript 形态
}
```

一个 Workspace 可以有多个 session（第一次做完，第二天再来一句"把测试补上"），
新 session 默认 `--resume` 上一个 cc_session_id。

### 3.5 Trigger — 谁来创建 session

```
Trigger {
  id, name, enabled
  kind:        cron | webhook(github pr) | manual
  source_id, environment_id
  prompt_template
  target:      new_session | resume:<session_id>
}
```

直接落在 `internal/task` 的 `Recurrence` / `ScheduledAt` 上，这个包为此预留了
Phase 4 字段，现在给它一个真实的 owner。

---

## 4. 架构与数据流

```
                 web UI (SSE)            IM bot (Telegram/Feishu/…)
                     │                          │  @cc / /task / 审批回复
                     ▼                          ▼
        ┌──────────────────────────────────────────────────────┐
        │  Agent Control Plane   /api/v1/agent/*  (UserAuth)   │
        │  SessionService · WorkspaceService · TriggerService  │
        │  ── 落在 internal/usecase，存 SQLite                  │
        └──────────┬───────────────────┬───────────────────────┘
                   │ 1. provision      │ 3. 事件 / 审批 / steer
                   ▼                   ▼
        ┌──────────────────┐   ┌──────────────────────────────┐
        │ Workspace runtime│   │ agentboot.Runner             │
        │ git clone/push   │   │ process.Factory =            │
        │ docker create    │   │   OSExec (local)             │
        │ setup_script     │   │   DockerExec (docker)        │
        └────────┬─────────┘   └──────────────┬───────────────┘
                 │ mount                      │ stdin/stdout stream-json
                 ▼                            ▼
        ┌─────────────────────────────────────────────────────┐
        │ 容器 / 本地目录：claude CLI  --settings <profile>    │
        │   ANTHROPIC_BASE_URL = http://<host>:<port>/tingly/…│
        │   ANTHROPIC_API_KEY  = 会话级 model token            │
        └───────────────────────────┬─────────────────────────┘
                                    │ 2. 模型流量（唯一出网口，network=proxy 时）
                                    ▼
                          tb gateway → routing → providers
                          usage 记账按 token → session
```

**一次 session 的生命周期**

1. `POST /api/v1/agent/sessions {source, environment, prompt, base_ref?}`
2. WorkspaceService：`git clone --depth=… -b base_ref` 到宿主 workspace 目录；
   建工作分支；docker 模式 `docker create` + bind mount + 注入 env/secrets；跑
   `setup_script`（输出进 events，失败则 session=failed，目录保留可查）。
3. SessionService 签发**会话级 model token**（有效期 = session 生命周期，绑定
   session_id，用量归属），物化 cc_profile 的 settings.json（复用
   `MaterializeCCProfileSettings`），拼 `LaunchSpec`。
4. `agentboot.Runner` 用对应 `process.Factory` 启动 `claude -p --output-format
   stream-json --input-format stream-json --settings …`。事件流：
   - `MessageEvent` → 追加 events，SSE 推给 web / 汇总推给 IM（复用
     `remoteagent/stream.go` 的聚合节奏）；
   - `ApprovalRequestEvent` / `AskRequestEvent` → session=waiting_input；web 出卡片，
     IM 通过共享 IMPrompter 发 prompt；任一侧先答生效（同一 reply namespace）；
   - 用户 steer（追加消息）→ 通过 stdin 的 stream-json user message 投递，正在跑的
     turn 结束后消费（agentboot 已支持 `InitialInput` 通道，扩展为持续输入）。
5. Turn 结束 → session=idle；宿主侧 `git status/diff` 生成 Artifact 摘要；通知
   notify consumer（"完成，改了 7 个文件，[查看 diff] [Push] [开 PR]"）。
6. 用户点 **Push / Create PR**：宿主用 Source credential push 分支、调 GitHub API
   建 PR；PR URL 写回 session，并作为下一步的物件呈现（ux §11）。
7. Archive → 停容器、TTL 后删目录；session events 与 Artifact 永久保留。

---

## 5. 关键决策

### 5.1 执行体 = Claude Code CLI via agentboot，不引 SDK、不用 AFK

- `afk.md` §1 的不变量：AFK 只做 Anthropic-shaped、只服务 Smart Guide。代码任务
  需要的 Edit/Bash/MCP/hook/skills 全套，Claude Code 已经有，重做是浪费。
- `agentboot` 的 `process.Factory` 是**唯一需要新增实现的 seam**：`DockerExecFactory`
  把 `LaunchSpec` 翻译成 `docker exec -i <container> …`，Stdin/Stdout/Wait/Kill 语义
  与 `OSExecFactory` 一致；Runner / Driver / decoder 全部零改动。

### 5.2 隔离分三档，v1 先做前两档

| 档 | runtime | 隔离 | 依赖 | 定位 |
|---|---|---|---|---|
| T0 | `local` | 无（同 @cc 今天） | 无 | 零配置默认；个人机器上跑自己的仓库 |
| T1 | `docker` | 文件系统 + 网络 + 资源 | Docker / OrbStack / Podman | 主推；服务器部署、多任务并行、跑不信任的仓库 |
| T2 | `remote-runner` | 另一台机器 | 一个 tb 以 runner 模式连回来 | P3；控制面与执行面分离 |

T0 不是凑数：它让整条链路（workspace、session、diff、PR）在没有 docker 的机器上
也能跑，且 T0/T1 之间只差一个 Factory 和 provision 步骤，不存在两套代码路径。

### 5.3 凭证永不进沙箱

- **模型**：容器内只有会话级 model token，指向宿主 gateway。provider key 不出宿主。
  token 随 session 归档吊销。
- **git**：clone / fetch / push 全部在**宿主侧**执行，用 Source 的 credential；
  容器内 workspace 是 bind-mount，`git commit` 可以（本地），`git push` 没有远端
  凭证会失败。Push 是控制面动作（按钮 / IM 回复 / `tb push` MCP tool），不是 agent
  的 shell 命令。
  - 代价：agent 自己说"我 push 了"是假的。通过 CC 的 system prompt 追加一句
    "push 由宿主完成，用 `tb_push` 工具"来消解；MCP tool 由 tb 的 mcpserver 暴露。
- **secrets**：Environment.secrets 只以 env 注入进程；events / transcript 里做
  值级 redaction（同 `logging-redesign.md` 的脱敏路径）。

### 5.4 网络策略走宿主代理，不做 iptables

`network=proxy` 时容器 `--network none` + 一个 unix socket / host-only 网络只通
宿主 tb；tb 内置一个最小 HTTP CONNECT 代理，白名单默认只放 gateway 自身与
Source 的 git host（后者其实不需要，因为 git 在宿主做）。`full` 直接 bridge。
不做包级过滤——那是运维产品的事。

### 5.5 持久化直接进 SQLite

`remote-storage.md` 已经决定 remote 子系统告别 JSON 文件；新表
`agent_sources / agent_environments / agent_workspaces / agent_sessions /
agent_session_events / agent_triggers` 一开始就在 SQLite（GORM），
events 表 append-only + 按 session 分页。不复用 `bot_sessions.json`。

### 5.6 `internal/task` 成为 session 的 supervisor

- 每个 AgentSession 的"跑一轮"是一个 Task（`Type=agent_session_turn`，
  `SerializationKey=workspace_id` 保证一个 workspace 同时只有一个 agent 在写）。
- 取消 / 中断 / 重试 / 进程崩溃后的 `interrupted` 状态、cron Trigger，全部是这个包
  已经有的语义。tb 重启后 `queued` 的自动恢复、`running` 的标记为 `interrupted`
  并允许 resume（用 cc_session_id `--resume`）。

### 5.7 一个 session 的交互面是"所有面"，不是选一个

web 和 IM 看到的是同一份 events、同一个 prompter。用户在手机上批准了，网页上的卡片
同步消失。这是 `bot-arch.md` "一个 bot 一个 prompter 一个 reply namespace" 的直接
延伸：session 只是 prompter 的又一个 caller。

---

## 6. API 与前端 IA

### 6.1 API（`/api/v1/agent/*`，UserAuth；先定 model，再 swagger，再 `task codegen`）

```
GET/POST/PUT/DELETE  /agent/sources
GET/POST/PUT/DELETE  /agent/environments
POST                 /agent/environments/:id/probe        // docker 可用？镜像在？setup 能过？（ux §7：走真实链路）

POST   /agent/sessions                 {source_id, environment_id, prompt, base_ref?, title?}
GET    /agent/sessions                 ?status=&source_id=
GET    /agent/sessions/:id
GET    /agent/sessions/:id/events      ?after=<seq>   （SSE：Accept: text/event-stream）
POST   /agent/sessions/:id/messages    {text}          // steer / 追加一轮
POST   /agent/sessions/:id/respond     {request_id, approved|answer}   // 与 bot-interaction-api 的 interact reply 同形
POST   /agent/sessions/:id/interrupt
POST   /agent/sessions/:id/archive

GET    /agent/sessions/:id/diff        // 宿主 git diff base_ref...branch
POST   /agent/sessions/:id/push
POST   /agent/sessions/:id/pull-request   {title?, body?, draft?}

GET/POST/PUT/DELETE  /agent/triggers
POST   /agent/triggers/:id/fire
POST   /agent/webhooks/github          // Trigger(kind=webhook) 的入口，签名校验
```

### 6.2 前端（MUI；图标走 `@/components/icons`）

新 rail **Agents**（与 Remote 平级；不塞进 Remote，因为 Remote 的主语是 bot，
这里的主语是 session——ux §1、§3）：

- **Sessions**（落地页）：列表按 `last_active` 排，每行 = 标题 / 仓库+分支 /
  状态 chip / 用量 / 最后一条消息摘要。顶部一个输入框 + 仓库、环境两个下拉，
  **直接开跑**，没有"新建向导"（ux §2）。
- **Session 详情**：左 transcript（复用 remoteagent 的 tool 渲染语义：折叠的
  tool_use/tool_result、审批卡片内联），底部输入框（steer），右侧
  Diff 面板 + Push / Create PR 按钮 + PR 链接 + 用量。归档的 session 只读但可
  "Resume"（ux §10）。
- **Environments**：卡片列表，每张卡显示 runtime 与具体镜像名、具体 profile 名
  （不是 "default"，ux §5），一个 Probe 按钮。
- **Sources**：仓库列表，credential 只显示来源与末四位。
- **Triggers**：放在 Sources 详情下（trigger 归属于仓库这个用户心智），不是独立页。

---

## 7. IM（remote control）接入

`remote_agent` consumer 增加一种 project 形态：**managed workspace**。

- `/task <source> <prompt>` 或在 action menu 里选仓库 → 创建 session，chat 绑定到
  这个 session（沿用 `Chat.Project` 字段，值为 `session:<id>`）。
- 之后普通消息 = steer；审批 / ask 走已有 prompter（零改动）；完成通知走 notify
  consumer，带 `[Diff] [Push] [PR]` 三个 callback 按钮。
- `@cc` 今天的"本机目录 + 本地 claude"用法原样保留，它就是 T0 的前身；长期看
  @cc 的 executor 可以改为创建一个 `local` Environment 的 session，两条路径合一。
  本期不合，避免动稳定路径。

---

## 8. 分阶段落地

| 阶段 | 交付 | 涉及 | 验收 |
|---|---|---|---|
| **P0 骨架**（~2 周） | Source / Environment(local) / Workspace / Session 模型 + SQLite；宿主 clone；agentboot OSExec 跑 session；events SSE；Sessions 列表 + 详情页（transcript + steer + 审批卡）；diff / push / PR | `internal/usecase/agent`, `internal/db`, `internal/server/module/agent`, `frontend/src/pages/agents` | 在本机对一个公开仓库下一句任务 → 出 PR |
| **P1 隔离**（~2 周） | `DockerExecFactory`；Environment(docker)：镜像、setup_script、env/secrets、network=proxy、资源限制；Probe；会话级 model token + 用量归属 | `agentboot/process/docker.go`, `internal/usecase/agent/runtime`, gateway token | 两个 session 并行跑同一仓库互不干扰；容器内拿不到 provider key |
| **P2 远程推进**（~1.5 周） | IM `/task` + steer + 完成通知；Trigger(cron / GitHub webhook)；重启恢复（task interrupted → resume） | `remote/control/remoteagent`, `internal/task` 接线 | 手机上发任务、批准、收 PR 链接；PR 评论触发续跑 |
| **P3 扩展**（按需） | remote-runner；GitLab；session 内 `tb_push` MCP tool；AFK 做轻量"总结/分诊" session | — | — |

P0 结束就能演示完整故事；P1 是"敢给别人用"的门槛；P2 才是标题里的"远程交互推进"。

---

## 9. 待决策事项

1. **Docker 是否列为推荐依赖**：桌面用户（macOS）需要 Docker Desktop / OrbStack。
   建议：不强依赖，T0 兜底，docker 检测到时在 Environments 页给一个"一键创建
   docker 环境"的引导（ux §8）。
2. **GitHub 凭证形态**：PAT 最简单；GitHub App 更干净（细粒度、webhook 自带）。
   建议 P0 用 PAT，Trigger(webhook) 落地时再评估 App。
3. **agent 能否自己 push**：§5.3 选了"不能"。如果实际使用中体感差，备选是宿主
   起一个 git credential helper 通过 unix socket 给容器签短期 token。
4. **Session 是否归属 Team**：`team.md` 的 Team 目前只管模型访问。建议 owner 先
   记 user，Team 维度留字段不建 UI。
5. **CC 版本管理**：docker 镜像里固定 claude CLI 版本，还是启动时 `npm i -g`？
   建议镜像固定 + Environment 可指定版本 tag，避免线上漂移。
6. **`@cc` 与 managed session 何时合流**：见 §7，本期不合。

---

## 10. UX 原则自检

| 原则 | 本方案的对应 |
|---|---|
| §1 按用户问题组织 IA | "我的任务跑得怎么样了" → Sessions 落地页；"代码在哪跑" → Environments |
| §2 消解模式选择 | 落地页直接输入即运行；local 环境默认存在，不问"选哪种运行时" |
| §3 命名不碰撞 | Source / Environment / Workspace / Session / Trigger 与 bot / channel / scenario / profile 互不复用 |
| §4 正交轴分开 | 代码来源（Source）与运行环境（Environment）是两个对象，不是一个 "project" |
| §5 具体值 | 卡片显示镜像名、profile 名、分支名，不显示 "default" |
| §6 默认优于开关 | network 默认 proxy、分支名自动生成、base_ref 默认 default_branch |
| §7 诊断走真实链路 | Environment Probe 真的起容器、真的跑 setup_script |
| §8 教育内嵌 | docker 未安装时的引导卡；首个 session 的"这是 diff，下一步开 PR" |
| §9 降噪 | transcript 里 tool 事件默认折叠，审批卡片是唯一高亮 |
| §10 done ≠ locked | 归档 session 可 Resume；分支已 push 不随 workspace 回收 |
| §11 交出下一步物件 | 完成即给 diff、PR 链接；IM 通知带按钮 |
| §12 副作用限定表面 | 创建 docker 环境不自动发生；push 永远是显式动作 |
