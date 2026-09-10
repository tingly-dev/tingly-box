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

---

## 11. 落地记录

### P0-a 数据模型 + API（2026-09-06）

| 层 | 位置 | 说明 |
|---|---|---|
| 领域 | `internal/managedagent/` | `types.go`（五个名词 + Event）、`store.go`（按实体拆的 store 接口）、`launcher.go`（执行 seam，nil 合法）、`service.go`（不变量：默认 local 环境、docker 建模但拒绝、live workspace 上的删除保护、同 workspace 续接 cc_session_id、分支命名）、`memstore.go`（测试与无 DB 宿主）、`eventlog.go`（每 session 一个 JSONL，同 remote transcript 先例） |
| 持久化 | `internal/db/managed_agent*.go` | 四张索引表 `agent_sources / agent_environments / agent_workspaces / agent_sessions`，由 `StoreManager.ManagedAgent()` 暴露；docker 列（image / network / cpu / memory / disk / container_id）**已经在 schema 里** |
| HTTP | `internal/server/module/managedagent/` | `/api/v1/agent/*`，UserAuth；`GET …/events` 同一 `after` 游标既可 JSON 分页也可 SSE 流 |
| 接线 | `server_routes.go` `UseManagedAgentEndpoints`、`swagger.go` | 运行时无 Launcher → session 持久化为 `queued`；OpenAPI 用 MemStores 注册同一批路由 |
| 磁盘 | `~/.tingly-box/agent/{workspaces,events}` | `constant.GetAgentWorkspacesDir / GetAgentEventsDir` |

与 §6.1 的差异：`diff / push / pull-request / triggers / webhooks / probe` 还没有——它们依赖宿主 git 与执行体，随 P0-b（Launcher：local runtime）和 P2 一起来。

### P0-b 本地执行（2026-09-07）

| 层 | 位置 | 说明 |
|---|---|---|
| git | `internal/managedagent/gitrepo/` | 每个 Source 一个 bare mirror（`agent/sources/`），每个 workspace `clone --reference --dissociate`；`Diff`（已提交 + 工作区 + untracked）、`Push`（`-u origin <branch>`，用宿主 git 凭证） |
| 执行 | `internal/managedagent/agentrun/` | `Launcher`：provision → turn 循环；首轮 `--session-id <预生成 uuid>`，之后 `--resume`；审批/ask → `waiting_input` + pending 表；steer 在运行中排队、空闲时立即开一轮；用量从 `Result.Events` 折算 |
| 路由 | `agentrun.Routing` | `Environment.CCProfile` 非空 → 物化 profile settings（`--settings`）；否则主 scenario env。与 `remote-cc-profile.md` §2 同源（TBClient） |
| API | `GET …/diff`、`POST …/push` | push 是显式动作，运行中的 session 拒绝 push |
| 接线 | `server_routes.go` | 真实 Launcher 已挂上；turn 超时 2h |

真实链路冒烟：`TB_MANAGED_AGENT_E2E=1 go test ./internal/managedagent/agentrun/ -run RealCLI -v`
——真实 `claude` CLI + 真实 Launcher + 临时 gateway + 进程内 vmodel 上游（复用
`protocoltest.AgentTestEnv`，与 `harness agent claude --mock` 同源）。已验证：stdin 投递 prompt、
`--settings` 路由、`--session-id` / `--resume` 续接上下文（第二轮按上下文回答）、用量折算。
CI 不跑它（需要本机 CLI），阶段验收时手动跑。

全栈 e2e：`TB_MANAGED_AGENT_E2E=1 go test ./internal/managedagent/agentrun/ -run FullStack -v`
——真实 `internal/server`（StoreManager、事件日志目录、`UseManagedAgentEndpoints` 里接线的 Launcher、
TBClient 路由）+ HTTP API + 真实 CLI + 虚拟上游。两条 e2e 都断言 `VirtualServer().CallCount() > 0`：
**只看 "Paris" 标记不够**，真模型也会这么答。

e2e 抓到的两个问题（已修）：

1. **子进程 Claude Code 继承父会话身份。** tb 若运行在一个 Claude Code 会话内（Claude Code 终端里
   `tb start`、Claude Code 远程容器），子 `claude` 会继承 `CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST`
   （无视 base URL）与 `CLAUDE_CODE_REMOTE*`（改用父会话 OAuth 身份），tb 的路由被静默忽略。
   修复在 `agentboot/claude/environment.go`：clean env 剔除会话身份变量，用户配置变量
   （`CLAUDE_CONFIG_DIR`、`CLAUDE_CODE_MAX_OUTPUT_TOKENS` 等）保留。@cc 同样受益。
2. **主 scenario 路由改为物化 settings 文件。** 原先只注入 env；现在与 profile 同一机制：
   `~/.tingly-box/claude/default/settings.json` = 用户主 settings 为底 + gateway 路由，`--settings`
   传入。托管会话必须确定性路由，不能依赖宿主 env 恰好一致。env 仍作为兜底。

仍未做：PR 创建（需要 GitHub 凭证模型）、Source 级凭证注入、workspace TTL 回收、重启后 `running` → `interrupted` 的恢复（等 `internal/task` 接线）。

### P0-d IM 挂钩（2026-09-07）

按"挂在通知 hook 上，不做复杂"的原则：

| 层 | 位置 | 说明 |
|---|---|---|
| 事件总线 | `managedagent.EventBus` | `EventStore` 装饰器，每条事件落盘后同步分发给订阅者。Service 与 Launcher 不知道订阅者存在；之后 SSE fan-out 也挂这里 |
| 桥 | `internal/managedagent/imbridge` | 实现 `scenario.Scenario`（名字 `tasks`），订阅总线：首条 user_message → `started` 通知；`approval_request` / `ask_request` → `rt.Ask`（confirm 两个按钮 / 自由文本），回复经 `Service.Respond` 写回；`status idle/failed` → `finished` / `failed` 通知 |
| 路由 | 现有 Notify route | `POST /api/v1/bots/:bot/routes {"source":"tasks", ...}`，`event_filter` 可选 `started / needs_input / finished / failed`。无路由则静默 |
| 接线 | `server_control.go` | bot runtime 建好后注册桥并订阅总线 |

网页与 IM 是同一个 pending 表：任一侧先答生效，后到的回复因 `Respond` 报 not-pending 被丢弃。
IM prompt 预算 2h，超时只是不再占着聊天，session 继续等网页。前端还没有"创建 route"的入口
（bot-arch.md §10 的既有缺口），目前用 API 建。

### P0-e 工作空间管理（2026-09-07）

| 动作 | 位置 | 规则 |
|---|---|---|
| 重启恢复 | `Service.RecoverOnStart`，`Server.Start` 里触发 | `running` / `waiting_input` → `idle` + 状态事件"interrupted by restart"，`cc_session_id` 保留所以下一条消息就是 `--resume`；`queued` → 重新 `Launcher.Start`；Launcher 遇到半成品 checkout 目录先删再 clone |
| TTL 回收 | `Service.ReclaimIdleWorkspaces`，每小时一次 | workspace 的所有 session 都非活跃且最后活动早于 7 天 → `rm -rf` 目录，state=`reclaimed`。session 日志与索引不动；回收后的 workspace 不能再开新 session（从 Source 重新开始） |
| 手动回收 | `POST /agent/workspaces/:id/reclaim`、`GET /agent/workspaces` | 有活跃 session 时 409 |

没有引入 `internal/task`：恢复与回收只是启动时一次 + 一个 ticker，不需要持久任务队列。等到 Trigger（cron / webhook）才接。

### P0 加固（2026-09-07）

自查 + code-review 一轮后修掉的问题，都有测试钉住：

| 问题 | 修法 |
|---|---|
| `Stop` 取消的 context 没人用，归档后进程继续跑 | run 持有 `ctx`，provision 与 turn 都从它派生；归档先写 `archived` 再 Stop，turn 结束时看到 archived 不回写 |
| Interrupt 让 session 变 `failed` | `interrupted` 标记 → `idle: interrupted`；进程尚未启动时的 Interrupt 在 handle 出现时立即取消 |
| turn 结束与 `Send` 之间的窗口会把消息卡在队列里 | 队列检查与 `busy=false` 在同一把锁下完成 |
| 两个并发 `Send` 对非活跃 run 各起一轮 | `register` 后先在 run 锁下 claim busy，再 load |
| provisioning 超时后用已取消的 ctx 写库，workspace 永远 `provisioning` | clone 有 30 分钟上限；落库一律用独立 context |
| Service 与 Launcher 双写 session 整行 | `SendMessage` 只写事件；`Diff` 只在非 running 时刷新计数 |
| IM 把中断 / 重启后的 `idle` 当作"完成"通知 | 只有裸 `idle` 才是 finished |
| 每次轮询整文件 JSON 解码 | cursor 之前的行只读 `{"seq":N` 前缀 |
| 每轮结束跑完整 diff（含 patch）只为一个计数 | `ChangedFiles`（name-only + untracked） |
| BaseRef 为 commit sha 时 `--branch` 失败 | sha 走 clone 后 `checkout --detach` |
| `git clone <url>` 未加 `--`；ssh 可能挂在交互提示上 | `--`；`GIT_SSH_COMMAND=ssh -o BatchMode=yes`（用户未设时） |
| RecoverOnStart 被列表默认 200 条上限截断 | 显式大 Limit |

### P0-g 权限模式（2026-09-07）

模式集合与 Claude Code CLI 一致：`default / acceptEdits / auto / plan / dontAsk / bypassPermissions`，
空值 = 沿用 settings 文件的 `defaultMode`（优先级同 `remote-cc-profile.md` §2.1：session 覆盖 >
settings defaultMode > CLI 默认）。

| 层 | 规则 |
|---|---|
| Environment | `permission_mode` 作为该环境里新 session 的默认 |
| Session | 创建时可覆盖；活跃中 `PUT /agent/sessions/:id/permission-mode` 修改，**从下一轮生效**（正在跑的进程保持启动时的模式，事件里明说） |
| Launcher | 每轮把 `session.permission_mode` 传 `--permission-mode`；`bypassPermissions` 时宿主自动批准审批请求并记录 `approval_response: approved (bypassPermissions)`，不进入 `waiting_input`。这与 @cc 的 `noApprovalModes` 是同一条策略：只有 bypass 承诺无条件放行 |
| `auto` | 交给 CLI 的分类器；分类器不决定的调用仍以审批请求到达宿主，网页 / IM 照常等人答 |
| AskUserQuestion | 任何模式下都不自动回答 |
| UI | composer 的 Permissions 下拉（每项带一句说明，ux §8）；详情页头部的模式 chip 点开即改；环境表单里设默认 |
| 不探测 CLI 版本 | 老 CLI 不支持某模式就让它报错：CLI 的 stderr 尾部（agentboot 新增 `ExecutionOptions.Stderr` 按执行捕获）附在 `session.error` 上，失败的 session 只要 checkout 还在就可以换模式再发一条消息重试（`canRetry`） |
| 守卫 | agentboot 会静默丢弃它不认识的 `--permission-mode`，`modes_test.go` 保证我们提供的每个模式都在它的转发集合里 |

### P0-h 本地目录 Source（2026-09-10）

`SourceKind = local`：填一个宿主上的绝对路径，agent **就地**工作。

| 规则 | 说明 |
|---|---|
| 识别 | 绝对路径 ⇒ `local`（不加类型选择器，ux §2）；想要本地仓库的**副本**用 `file://`（仍走 clone） |
| workspace | 目录本身：`Path = AgentCwd = 路径`，`State = ready`，`Branch = ""`，`BaseRef = HEAD`；同一目录只有一个 workspace，新 session 复用并 `--resume` |
| 不做的事 | 不 clone、不建分支、不 push（`Push` 409）；不是 git 仓库时 `Diff` 返回空而不是报错 |
| 安全 | `ownsPath`：只有 `workspacesDir` 之下的目录才会被 `RemoveAll`；就地 workspace 的回收只退休记录，扫描直接跳过 |
| 运行时 | 只允许 local 环境；docker 环境下拒绝（目录在宿主上，容器里挂载是 P1 的事） |
| 与 @cc 的关系 | 这就是 @cc 今天"本机目录"用法的托管版，§7 里说的合流路径从这里开始 |

### 为 docker 预留了什么（P1 时应当只需要加，不需要改）

1. `Environment.Runtime` 枚举与 docker 字段（image / setup_script / network / resources / secret_refs）已建模、已持久化、已在 API schema 中；`SupportedRuntimes` 是唯一开关——P1 把 `RuntimeDocker` 置 true 并补 `applyEnvironmentInput` 里已经写好的 docker 校验分支。
2. `Workspace.ContainerID` 已有列；`Workspace.Path` 始终是宿主路径，docker 只是把它 bind-mount 进去。
3. `Launcher` 接口不带 runtime 语义：local 与 docker 是同一个 Launcher 实现里两个 `process.Factory`（`agentboot/process`），不是两个 Launcher。
4. `EnvironmentListResponse.supported_runtimes` 让前端在 docker 未就绪时能解释"为什么不能选"，而不是给一个死选项（ux §8）。
5. `Workspace.AgentCwd` 与 `Path` 分离：local 相等，docker 时 `AgentCwd=/workspace`，git 仍在宿主对 `Path` 操作。

---

## 12. Session 留存与一致性；clone 还是 worktree

### 12.1 Claude Code 会话的键是 (config dir, cwd)

Claude Code 把自己的会话写在 `<config dir>/projects/<编码后的 cwd>/<session>.jsonl`。
tb 的原则：

- **tb 的事件日志 + 索引里的 `cc_session_id` 是事实来源**；Claude Code 的 JSONL 只是
  `--resume` 的机制。tb 从不按路径去找 JSONL，只保证 resume 时的 (config dir, cwd)
  与创建时相同。
- 因此 session 挂在 **Workspace** 上而不是 Source 上：一个 workspace 的 `AgentCwd`
  在其生命周期内不变。"路径一直在变"是 workspace 之间在变，不是 workspace 之内。
- local：`AgentCwd = ~/.tingly-box/agent/workspaces/<id>/repo`，config dir 沿用用户的
  `~/.claude`（skills、memory、全局 settings 都在）。
- docker：容器内 `AgentCwd = /workspace` 固定；若 config dir 也是容器自带的，所有
  workspace 的会话会塌缩到同一目录且随容器消失。P1 的做法：每个 workspace 在宿主上有
  一个 `claude/` 状态目录，挂载进容器作为 config dir（`CLAUDE_CONFIG_DIR`）。这样
  (config dir, cwd) 仍然每 workspace 唯一且持久。
- workspace 被回收后 resume 不可能，新 session 从头开始——这是已建模的行为，不是缺陷。

### 12.2 每个 workspace 一个独立 clone，不用 worktree

| 方案 | 网络 | 磁盘 | docker 挂载 | 回收 | 结论 |
|---|---|---|---|---|---|
| 直接 clone | 每次全量 | 不共享 | 直接挂 | `rm -rf` | 太慢 |
| 用户主仓的 worktree | 无 | 共享 | `.git` 文件指向主仓绝对路径，进容器即断 | 需 `worktree prune` | 主仓状态会污染 agent；只适合将来的本地目录 Source |
| tb mirror + worktree / alternates | 增量 | 共享 | 同样是绝对路径问题 | prune | 收益只是省磁盘 |
| **tb mirror + `clone --reference --dissociate`** | 增量 | 不共享 | 只挂 workspace 一个目录 | `rm -rf` | **采用** |

mirror 是优化不是事实来源：mirror 失败退化为直接 clone，workspace 永远是普通目录。
磁盘不共享是有意的代价；将来 local 可以单独做 alternates 优化，不影响 docker。
