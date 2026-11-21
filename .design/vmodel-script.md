# VModel script — 预定义脚本驱动真实 agent 的端到端测试

状态：**提案（只有设计，未实现）**。索引：[`vmodel.md`](./vmodel.md)。相关：
[`vmodel-definitions.md`](./vmodel-definitions.md)、[`vmodel-sequence.md`](./vmodel-sequence.md)、
[`vmodel-capture.md`](./vmodel-capture.md)、[`cli-harness-refactor.md`](./cli-harness-refactor.md) §5.4、
[`projects/.design/projects-harness.md`](../projects/.design/projects-harness.md)。

标注：**[决定]** 用户已确认；**[建议]** 本文推荐、待用户确认；**[待定]** 需要用户决定（见 §10）。
本文所有条目目前都是 [建议] 或 [待定]。

## 0. 问题

要让 Projects 的每个流程（协调者开线程、线程调 MCP 工具、审批卡、决策卡、
`notify_coordinator`、消息插入、PR 工具、IM 交互……）都能用**真实进程**——真实 Projects
服务、真实 `claude` CLI——跑出精确、可重复、可断言的结果。手段是"回放预定义数据"：
模型这一端由一份写好的脚本扮演，不花钱、不联网、不依赖真模型的随机性。

现有资产各缺一块：

| 资产 | 能做 | 缺 |
| --- | --- | --- |
| `vmodel` `mock` / `recording` | 单个模型 id 的固定回答（文本、多个 tool_calls、分块、延迟、错误） | 无状态：每个请求同一个回答 |
| `vmodel` `sequence` | 按**全局到达顺序**推进的一串结果 | 只有 status/文本，没有工具调用；全局游标在并发下错配（见下） |
| `tb vmodel --capture` / `--fallback` | 逐字节记录客户端请求、任意 model id 都回答 | 只观察，不编排 |
| `projects/internal/harness` | 真实协调者/线程管理/存储/git，Brain/Actor 脚本 | 进程内：`enginetest.Agent` 直接把类型化消息送进引擎，**没有真实 CLI、没有 wire**；CLI 层的 bug（MCP 接线、权限提示、stdin 插入、usage 解析）看不到（项目记忆 real-run-projects-with-scripted-model） |
| `agentboot/process.FakeFactory` / `claude/fixture` | 假 CLI 进程回放 stream-json | 仍然不是真实 CLI |
| `projects/showcase/e2e-demo/mock_anthropic.py`、`.research/inject-probe/probe.py` | 真实 CLI + 手写 Python 脚本模型 | 每个场景重写一遍；识别"谁在请求"靠临时启发式；没有断言、没有覆盖检查、没有 trace |

**为什么全局顺序不行。** 一个真实 Projects 运行里，同一时刻会有多条互不相干的对话在请求模型：
协调者 session、每个线程的 session、线程里的子 agent、CLI 自己的辅助请求（标题生成、分类器、
压缩、`count_tokens`），并行工具调用之后还会交错。`sequence` 的游标是"上游的属性"
（`vmodel-sequence.md`），对限流模拟是对的；对"第 3 个请求应该回 `start_thread`"就是错的——
第 3 个到达的可能是线程 A 的第一个请求，也可能是一个标题生成请求。

`probe.py` 和 `mock_anthropic.py` 已经用实践说明了正确的方向：**先识别请求属于哪条对话，
再在那条对话自己的脚本里按顺序推进**（`probe.py` 用"有没有 Bash 工具 / 首条消息是不是子 agent
的 prompt"分 side/main/sub，每类一个计数器）。本设计把这个做法变成通用、声明式、可断言的机制。

## 1. 核心想法

1. **轨道（track）**：脚本由若干条轨道组成，一条轨道 = 一条对话（一个 CLI session 的主循环、
   一个子 agent、一个线程）。轨道内的步骤严格有序——一条对话本身就是串行的；轨道之间默认无序，
   只在声明的同步点上有先后。
2. **我们写下了每一个 assistant 字节，所以能认出自己的对话。** 脚本发出的每个 `tool_use` id、
   每段文本都进"发出索引"。之后任何请求的历史里都带着它们（CLI 原样回传 assistant 消息和
   `tool_use_id`），查一下最后一条 assistant 消息就知道这是哪条轨道、走到了第几步。
   这比 session 头、system prompt 指纹都可靠，而且天然不受并发影响。
3. **新对话靠"绑定选择器"认领**：一条对话的第一个请求还没有我们的历史，用声明的条件
   （system prompt 片段、工具集合、首条用户消息、请求类别头……）认领一条还没绑定的轨道。
   由我们发出的工具调用派生出来的对话（`start_thread` 开的线程、`Agent` 开的子 agent）
   还能用"发出的参数文本"精确认领（§2.3）。
4. **辅助请求走背景规则**，不消耗步骤；**任何没被认领的请求都是失败**（严格模式），
   并带着"最接近的候选为什么不匹配"写进 trace。
5. **每一步 = 条件 → 回答 → 断言。** 回答复用 vmodel `mock` 块的字段（文本、`tool_calls`、
   分块、延迟、usage、错误注入）；断言是对"这一步匹配到的请求"和"下一次请求带回的
   tool_result"的谓词。

## 2. 请求匹配

### 2.1 可用的信号（真实 Claude Code 2.1.329 capture 里看到的）

来自 `vmodel/examples/capture/claude-code-2.1.329/oauth-direct/raw/conn-0001.in` 和
`.design/claude-code-oauth-compat.md`：

| 信号 | 位置 | 用途 | 可靠性 |
| --- | --- | --- | --- |
| 最后一条 assistant 消息里的 `tool_use` id / 文本 | body `messages` | **续接**：哪条轨道、第几步 | 高：id 是我们铸的 |
| `x-claude-code-request-class`：`main` / `subagent` / `auxiliary` / `compaction` / `workflow` | header | 区分主循环、子 agent、辅助、压缩 | 直连时总发；经代理（自定义 `ANTHROPIC_BASE_URL`）只在 env `CLAUDE_CODE_GATEWAY_HINT_HEADERS` 打开时发（compat 文档，静态分析，**未实测**） |
| `x-claude-code-agent-type` | header | 子 agent 类型 | 同上，仅 `agent:*` 来源 |
| `X-Claude-Code-Session-Id`、`metadata.user_id.session_id` | header / body | 诊断、同一进程的请求分组 | 子 agent 是否共用父 session id **未验证** |
| `x-claude-code-prompt-id` | header | 同一用户回合的请求分组 | 未验证语义 |
| `system` 文本 | body | 绑定：Projects 给线程追加 `You are a thread in the project …`、`Your task: <title>`、`You are on branch <branch> … (<cwd>)`（`projects/internal/project/prompts.go` `threadSystemPrompt`）；协调者有自己的 prompt | 高，但内容随 Projects 改动 |
| `tools` 名字集合 | body | 绑定与断言：协调者有 `mcp__tingly-projects__start_thread`，线程有 `…__notify_coordinator`，辅助请求通常没有工具 | 高 |
| 首条用户消息 | body | 绑定：子 agent 的 prompt、用户的第一句话 | 高 |
| `model` | body | 辅助请求常用 haiku 类 id | 中 |

### 2.2 路由算法（每个 `/v1/messages` 请求）

```
0  count_tokens 等非消息端点 → 脚本级固定回答（不是步骤）
1  class ← request-class 头；没有时启发式：无 tools → auxiliary，否则 main
2  续接：取最后一条 assistant 消息，按 tool_use id（精确）或文本（全等）查发出索引
     命中 (T, k) 且 class 属于 T 接受的类别（默认 main/subagent）
       → 位置 k+1；若 k+1 已被消费且本次请求的"新尾部"（最后一条 assistant 之后的消息）
         与当时相同 → 重试（§2.5）
     对压缩：首条用户消息里含有某条轨道 compaction 步骤发出的摘要文本 → 续接那条轨道
3  新对话：在尚未绑定的轨道里找 bind 选择器匹配的
     恰好 1 条 → 绑定（记下 session id 供诊断）；>1 条 → 歧义失败；0 条 → 4
4  背景规则：按声明顺序第一条匹配的回答，不消耗步骤，可设 min/max 次数
5  未匹配：严格模式回 400 invalid_request_error（消息里写明原因），记为失败
```

要点：

- 续接优先于 class 头：协调者第二个回合（一条 check-in 进来）是"同一对话的新用户消息"，
  历史里有我们第一回合的回答，自然落回协调者轨道；不需要任何 session 关联。
- **分叉的辅助请求**是主要风险：CLI 的某些辅助请求（提示建议、会话记忆提取等）会复制主对话
  历史，按第 2 步会被误认成主轨道的下一步。防线依次是 request-class 头（测试运行时总是设
  `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1`）、轨道的 system 指纹（第一次绑定时记下 system 的哈希，
  之后 system 不同的请求不算这条轨道）、最后才是背景规则。哪些辅助请求会出现、头是否在代理模式下
  真的发出，第一阶段先实测再定（§9 P1 第 1 项），不靠猜。
- 文本全等可能撞车（两条轨道发出同一句话）：加载时校验，撞车的纯文本步骤报警告；运行时再用
  绑定时记下的 session id 消歧。

### 2.3 派生对话：`spawns`

一条轨道发出的工具调用可以声明它会派生出哪条轨道：

- `start_thread`（Projects 开线程，跨进程）：子轨道的第一个请求的 system 里有 `Your task: <title>`；
- `Agent`（CLI 子 agent，同进程）：子轨道的首条用户消息就是我们发出的 `prompt` 参数。

`spawns: thread-a` 做两件事：子轨道在父步骤被送出之前不能绑定（因果守卫，防止错认）；
子轨道的 bind 默认加上"首个请求含有该调用的 `title` / `prompt` / `description` 参数文本"。
作者也可以显式写 bind。并行开的几个子 agent 若 prompt 相同就无法区分——加载时报错，要求写成不同。

### 2.4 轨道间的同步点

- `after: <track>.<step>`：这一步的请求到达后**扣住不回答**，直到引用的步骤已经送出。
- `after: signal:<name>`：扣到测试驱动发出这个信号（例如"审批已点"、"消息已插入"）。
- `hold: {before: first_byte | stop, until: signal:<name>}`：回答流到一半停住。用于确定性地测
  消息插入：实测插入会落在"写入之后的下一个模型请求"（`.research/mid-turn-injection.md`），
  把流停在 `message_stop` 之前、驱动插入消息、再放行，下一个请求一定带着它。
- 每个扣住都有超时（默认 30s，可配），超时即失败并写明在等什么。CLI 的请求超时是 600s
  （`X-Stainless-Timeout`），不会先断。

扣住响应而不是 sleep：顺序由声明保证，不靠时间窗。

### 2.5 重试与错误注入

- 一步回答了错误（`error: {status: 529}` 等，复用 `ErrorInjection`），CLI 的重试请求匹配**下一步**
  （作者写"第二次成功"）。
- 一步回答成功、却又收到同一位置、同一新尾部的请求（CLI 自己重试，或像 capture 文档里那次
  流格式错误导致的 `stream:false` 重发）→ 原样重放那一步的回答，trace 标 `retry`，
  并默认算一条警告（意外重试通常说明我们的回答有问题）。

## 3. 脚本格式

### 3.1 复用 vmodel spec：`kind: script`

[建议] 脚本是 `spec.Definition` 信封里的一个新 kind，不另起文件格式：

- 一个加载器（`spec.Load`）、一个 CLI（`tb vmodel --config`）、严格校验、id 唯一；
- 运行时状态（绑定表、各轨道游标、发出索引）跟 `sequence` 游标一样属于一次 `spec.New` 的
  runtime，每次编译都是新的；
- `tb vmodel --config s.yaml --fallback <script-id>` 让所有 model id 都进这份脚本
  （协调者、线程、haiku 辅助请求用的 id 都不同，fallback 已经解决了这个问题），
  `--capture` 照常套在外面；
- 回答块就是 `mock` 块的字段（`vmodel.SharedMockSpec`：`content`、`tool_calls`、
  `stream_chunks`、`delay`、`usage`、`stop_reason`、`error`），不发明第二套回答词汇。

取舍：

| | `kind: script`（建议） | 独立文档类型 + `--scenario` |
| --- | --- | --- |
| 加载/校验/导出 | 复用 | 另写一套 |
| 与 capture、fallback 组合 | 现成 | 要重新接 |
| 语义纯度 | 一个"模型"其实服务一整个场景，跨多个 model id；`expect` 在信封里原指"响应断言"，script 不用它（请求断言在步骤里） | 干净 |
| matrix | `Runtime.Scenario` 对 script 跳过/拒绝，与 decision 只走原生端点同理 | 不涉及 |

代价可控：信封层只多一个 kind；新增的只有 runtime 级"结束并出报告"（`Runtime.Finish()`，
只有带状态的 kind 实现）和 `count_tokens` 交给 script 回答（目前 handler 对它回 501）。
第一阶段只支持 `anthropic` 协议。

### 3.2 结构

```yaml
id: projects-one-thread
kind: script
protocols: [anthropic]
script:
  strict: true                 # 未匹配请求 = 失败（默认 true）
  hold_timeout: 30s
  count_tokens: 1000           # /v1/messages/count_tokens 的固定回答
  background:                  # 不消耗步骤的辅助请求
    - id: side
      when: {class: [auxiliary]}
      reply: {content: "{}"}
  tracks:
    <track-id>:
      bind: <selector>         # 认领新对话；派生轨道可省略（§2.3）
      accept: [main]           # 接受的 request class，默认 [main, subagent]
      capture: {<var>: <extractor>}   # 绑定时从第一个请求取值
      steps:
        - id: <step-id>        # 可省略，省略时为序号；被 after/spawns/expect_result 引用时必填
          when: <selector>     # 可选守卫：不满足时，optional 步骤被跳过，否则记为错配
          optional: false
          repeat: 1
          after: [<track>.<step> | signal:<name>]
          expect: <selector>   # 对本次请求的断言，失败记录但照常回答
          capture: {<var>: <extractor>}
          reply:               # = mock 块字段
            content: "..."
            tool_calls:
              - name: Bash
                arguments: {command: "git commit -qam '{{title}}'"}
                spawns: <track-id>
                expect_result: {contains: "...", is_error: false}
            stream_chunks: [...]
            delay: 0s
            error: {status: 529}
          hold: {before: stop, until: signal:<name>}
          compact: false       # true：这是对 compaction 请求的回答，发出的文本进摘要索引
```

**选择器**（`bind`、`when`、`expect`、背景规则的 `when` 共用一套谓词，全部是"包含/正则/存在"，
没有整段相等，见 §6）：

| 谓词 | 含义 |
| --- | --- |
| `class: [..]` | request class（§2.2 第 1 步） |
| `model: <regex>` | 请求的 model id |
| `system_contains` / `system_regex` | system 文本 |
| `first_user_contains` / `last_user_contains` / `..._regex` | 首条 / 最后一条用户消息的文本（不含 tool_result） |
| `tools_include` / `tools_exclude` | 工具名集合；名字以 `*` 结尾按前缀，以 `__name` 写可按后缀匹配 MCP 前缀 |
| `has_tool_result: <step>.<call>` | 本次请求带回了某次调用的结果 |
| `header: {name: regex}` | 请求头 |
| `all` / `any` / `not` | 组合 |

**取值器**（`capture`）：`{from: system|first_user|last_user|tool_result:<step>.<call>|header:<name>, regex: "...(group)..."}`
或 `{from: tool_result:<step>.<call>, json: <path>}`。值在回答的字符串里用 `{{var}}`（本轨道）
或 `{{track.var}}`（其他轨道）引用；引用未取到的变量 = 失败，不留空。

**`expect_result`** 挂在工具调用上，在带回它结果的那个请求到达时检查：`contains`、`regex`、
`is_error`、`json: {path: value}`。这是断言"审批通过了 / 被拒了"、"MCP 工具返回了线程 id"、
"`create_pr` 被拒并说明原因"的地方——模型这一端能看到的系统行为，最后都落在 tool_result 里。

**发出的 id 是确定的**：`tool_use` id = `toolu_` + hash(script id, track, step, call 序号, repeat 序号)
的十六进制，`message` id 同理；usage 固定。回答里不含时间、随机数。

### 3.3 Go API

YAML 是主要形式；Go 测试可以直接构造同一组结构（`script.Script{Tracks: …}`），
并在 YAML 不够用时为单个步骤挂一个 Go 回调（`ReplyFunc`），与 `kind: builtin` 的
"有名字的 Go 行为"同一原则：文件里引用名字，二进制里注册。第一阶段不需要回调。

```go
s := script.Load(t, "testdata/one-thread.yaml")
srv := s.Serve(t)                       // 127.0.0.1:0，可选 capture 目录
s.WaitServed(t, "thread-a", "commit")   // 等到这一步已送出
s.Signal("approved")
rep := s.Finish()                       // 未匹配 / 未消费 / 断言失败 / 意外重试
require.True(t, rep.OK(), rep.String())
```

## 4. 断言、兜底与报告

| 情况 | 行为 |
| --- | --- |
| 请求没被任何轨道或背景规则认领 | 严格模式：回 400 `invalid_request_error`，消息写"vmodel script: no track matches; nearest: coordinator (bind: tools_include missing …)"；记一条失败。CLI 对 400 不重试，回合以错误结束，Projects 那边看得到。`strict: false`（写脚本时用）改为交给 `--fallback` 的模型回答并记警告 |
| 轨道已走完还来请求 | 同上，原因写"track exhausted after <step>" |
| `when` 不满足且非 optional | 记错配（期望 vs 实际摘要），回 400，轨道停在原位 |
| `expect` / `expect_result` 失败 | 记失败，**照常回答**，让运行走完、trace 完整；`fail_fast: true` 时改为回 400 |
| 结束时有未消费的非 optional 步骤 | `Finish()` 报失败，列出每条轨道停在哪一步、最后一次请求是什么 |
| 背景规则次数越界（`min`/`max`） | `Finish()` 报失败 |
| 扣住超时 | 回 529 `overloaded_error` 结束这个请求，记失败（写明在等哪个步骤/信号） |
| 意外重试 | 警告；`retries: fail` 时为失败 |

**trace**（`<dir>/trace.jsonl`，每个请求一行，请求结束时写）：

```json
{"seq":7,"t_ms":5120,"track":"thread-a","step":"commit","via":"continuation",
 "class":"main","model":"default-model","session":"24bc…","held_ms":0,
 "checks":[{"on":"expect_result write.0","ok":true}],
 "reply":{"text":"Now commit it.","tool_calls":[{"name":"Bash","id":"toolu_3f…"}]}}
```

`via` 取 `bind` / `continuation` / `spawn` / `retry` / `background` / `unmatched`。
`seq` 与 capture 的 `index.jsonl` 是同一个序号（tap 把序号放进请求 context，script 读出来），
所以任何一行都能找回 `raw/conn-N.in` 里的原始字节。`Finish()` 另写 `report.json` 和
`trace.md`：按轨道排成对话（用户消息 / tool_result / 我们的回答，长文本截断），失败项在顶部。
排序按（轨道, 步骤），不按到达时间——跨轨道的到达顺序本来就不确定，不应该让报告每次都不同。

`tb vmodel --config s.yaml` 下：收到 SIGTERM/中断时写报告，有失败就以非零退出；`--json` 的就绪
输出里列出轨道。外部驱动（非 Go）需要的 `signal` / `wait` 控制接口放第二阶段（§9）。

## 5. 驱动 Projects：真实进程 + 真实 CLI 的旅程

### 5.1 拓扑

```
Go 测试（根模块）
 ├─ script.Serve ── 127.0.0.1:A ◄── claude（协调者 session）   ┐ 都是 Projects 拉起的
 │   (+ capture)                ◄── claude（线程 A / B …）    ┘ 真实 CLI
 ├─ 启动 bin/tingly-project --port B --data-dir D（env -i、隔离 HOME/CLAUDE_CONFIG_DIR）
 │    PUT /settings：base_url = http://127.0.0.1:A（Anthropic 风格）
 └─ 驱动：通过 Projects 的 HTTP API 扮演用户（发消息、点审批、回答决策卡、等事件）
```

- 模型服务器直接作为 CLI 的上游（`ThreadEnv` 把连接的 `base_url` 写成 `ANTHROPIC_BASE_URL`，
  `projects/internal/server/module/settings/settings.go`），中间不经过 tb 网关：网关会改写 body/头，
  它有自己的测试（`cli/harness agent --mock`）。经网关的拓扑放第三阶段。
- CLI 继承 Projects 进程的环境（`agentboot/claude/environment.go` 用 `os.Environ()`），所以在
  Projects 进程上设 `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1`、`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`、
  `DISABLE_TELEMETRY=1`、`DISABLE_AUTOUPDATER=1`，并且 `HOME`、`CLAUDE_CONFIG_DIR` 指到临时目录
  （同一 config dir 的 CLI 会互相看见，项目记忆 cc-native-peer-messaging）。绝不碰真实
  `~/.tingly-box`、`~/.claude`。
- 为什么在根模块：`projects` 不能 import 根模块（`task projects:boundary`），而 script 引擎在
  `vmodel`（根模块）里；Projects 作为**进程**被拉起，不需要 import。仓库用的是本地 git 仓库，
  需要 GitHub 的工具（`create_pr`）沿用 `github_tools_test.go` 的做法：origin 经 `insteadOf`
  指向本地裸仓库，GitHub API 是替身。
- 测试需要本机 `claude`；没有就 `t.Skip`，像 `harness-agent-testing.md` 的真实补充测试一样，
  不进默认 CI（§10 待定 2）。

### 5.2 例子：协调者开线程 → 线程写文件 → 提交要审批 → 用户批准 → 汇报 → 协调者转述

对应 `projects/showcase/e2e-demo` 的流程（单线程版）；`mock_anthropic.py` 里的启发式
在这里都变成声明。协调者的项目上下文是放在第一回合的**用户消息**里的
（`coordinator_session.go` 拼接 `coordinatorSystemPrompt`），所以协调者按工具集合绑定。

```yaml
id: projects-one-thread
kind: script
protocols: [anthropic]
script:
  background:
    - id: side
      when: {class: [auxiliary]}
      reply: {content: "{}"}
  tracks:
    coordinator:
      bind: {tools_include: ["__start_thread"], first_user_contains: "project coordinator"}
      steps:
        - id: plan
          expect: {last_user_contains: "加一个 CHANGELOG", tools_exclude: [SendMessage, ListAgents]}
          reply:
            tool_calls:
              - id: start
                name: mcp__tingly-projects__start_thread
                arguments: {title: Add CHANGELOG, task: "Create CHANGELOG.md with an Unreleased section and commit it."}
                spawns: thread
                expect_result: {is_error: false}
        - id: started
          reply: {content: "开了一个线程：Add CHANGELOG。"}
        - id: report
          expect: {last_user_contains: 'kind="check-in"', all: [{last_user_contains: "CHANGELOG.md"}]}
          reply: {content: "Add CHANGELOG 已提交到自己的分支，等你审阅。"}
    thread:
      bind: {system_contains: "Your task: Add CHANGELOG", tools_include: ["__notify_coordinator"]}
      capture:
        cwd: {from: system, regex: "in your own worktree \\(([^)]+)\\)"}
      steps:
        - id: write
          reply:
            tool_calls:
              - {name: Write, arguments: {file_path: "{{cwd}}/CHANGELOG.md", content: "# Changelog\n\n## Unreleased\n"},
                 expect_result: {is_error: false}}
        - id: commit
          reply:
            content: Now commit it.
            tool_calls:
              - {name: Bash, arguments: {command: "git add CHANGELOG.md && git commit -qm 'Add CHANGELOG'"},
                 expect_result: {is_error: false}}       # 被拒时这里是 is_error: true
        - id: done
          reply: {content: "Added CHANGELOG.md and committed it; git log -1 shows the commit."}
```

```go
func TestProjectsOneThread(t *testing.T) {
	s := script.Load(t, "testdata/projects-one-thread.yaml")
	srv := s.Serve(t, script.Capture(t.TempDir()))
	p := projectse2e.Start(t, projectse2e.Options{ModelURL: srv.URL}) // 隔离 HOME、临时仓库、建项目
	p.Say("加一个 CHANGELOG")
	s.WaitServed(t, "thread", "commit")
	p.Approve(t, p.WaitApproval(t, "Add CHANGELOG"))   // 审批卡出现在线程里，用户点"允许"
	p.WaitThread(t, "Add CHANGELOG", "ready")
	s.WaitServed(t, "coordinator", "report")
	require.True(t, s.Finish().OK())
	th := p.Thread(t, "Add CHANGELOG")
	assert.Equal(t, 1, th.CommitsAhead)                // Projects 自己的状态也断言
	assert.Contains(t, p.AssistantTexts(t), "等你审阅")
}
```

模型一端断言"系统给了模型什么"（工具集合、信封、tool_result）；驱动一端断言"系统留下了什么"
（线程状态、提交、卡片、事件、记忆、PR 记录）。两边合起来才是一条旅程。

### 5.3 各功能怎么表达

| 功能 | 模型一端 | 驱动一端 |
| --- | --- | --- |
| 开线程 / 并行开多个 | `start_thread` + `spawns`；多个线程各一条轨道 | 线程列表、分支、worktree |
| 线程调 MCP 工具（`notify_coordinator`、`create_pr`、`update`…） | 线程轨道发出调用，`expect_result` 断言结果 | 事件、PR 记录、协调者收到的信封 |
| 审批卡 | 发出需要审批的 Bash；`expect_result.is_error` 区分批准/拒绝 | 等卡片、点允许/拒绝 |
| 决策卡 / `propose_threads` | 协调者发出调用；用户回答后下一回合的用户消息里有回答 | 等卡片、选择 |
| 消息插入（运行中发消息） | `hold: {before: stop, until: signal:sent}`；下一步 `expect: last_user_contains: "while you were working"` | 发消息，确认已写入后 `Signal("sent")` |
| 子 agent | `Agent` 调用 + `spawns`，子轨道 `accept: [subagent]` | — |
| 压缩 | `compact: true` 的步骤回答 compaction 请求 | 触发 `/compact`（`coordinator_compact.go`） |
| 模型失败 | `error: {status: 529}` / 中途断流 | 回合失败显示、队列继续 |
| IM | 与上面相同 | 需要假的 IM 平台（第三阶段） |

### 5.4 与 `projects/internal/harness` 的关系：补充，不替代

| 档位 | 脑子 | CLI | 速度 | 跑在哪 | 回答的问题 |
| --- | --- | --- | --- | --- | --- |
| 进程内 `projects/internal/harness` | Brain/Actor（Go） | 无（`enginetest`） | 毫秒 | 每个 PR，CI | 协调者循环、状态机、不变量、规则 |
| 假进程 `process.FakeFactory` | stream-json 回放 | 假 | 毫秒 | CI | agentboot 进程层 |
| **wire（本文）** | 脚本模型 | **真实** | 秒到几十秒 | 手动 / 夜间 / CC 升级时 | CLI 与 Projects 的真实接线：MCP、权限提示、stdin 插入、`--disallowedTools`、`crossSessionInbound`、usage、真实请求形状 |

[建议] 不统一成一种：Brain 是带进程内工具调用的 Go 闭包，脚本是数据，硬统一会让两边都变差。
共享的是**旅程清单**：wire 档挑进程内档已有的旅程（J1 新工作→线程→汇报、审批、通知通道、
GitHub 工具）各做一条真实版本，名字对齐，便于对照"进程内绿、真实红"的情况。
`projects-harness.md` §4 原来设想的"vmodel 序列模型 + 带工具调用的步骤"由本文取代。

## 6. 确定性

| 来源 | 处理 |
| --- | --- |
| 我们发出的 id、usage、文本 | 全部由（脚本, 轨道, 步骤, 序号）哈希得到；不含时间 |
| CLI 的 session id、uuid、日期、system-reminder、CLAUDE.md | 选择器只做包含/正则，从不整段比较；运行目录用干净祖先的临时目录（capture 文档 §3.3 的教训） |
| Projects 的线程 id、分支名、路径 | 用 `capture` 从请求里取，不写死 |
| 跨轨道的到达顺序 | 不需要确定的地方不约束；需要的地方用 `after` / `signal` 扣住响应；报告按（轨道, 步骤）排序 |
| 协调者同时收到两条汇报 | 用 `after` 让线程 B 的最后一步等线程 A 的最后一步，汇报顺序就确定了；不等的话 Projects 可能把两条合进一个回合——这本身是要测的行为，作为单独的旅程写 |
| 计时器（stalled 检测等） | 需要 Projects 提供阈值覆盖（测试用的 env 或设置）；没有之前不写依赖它的旅程 |
| CLI 版本 | 报告和 `meta.json` 记下 `claude --version`；CC 升级会改请求形状，选择器尽量落在 Projects 自己写的文本和工具名上，而不是 CC 的 prompt |
| sleep | 只在模拟慢流时用 `delay`，不用来排序 |

## 7. 与现有资产的关系

| 资产 | 关系 |
| --- | --- |
| `vmodel/spec` | 新 kind `script`；复用加载、校验、导出、runtime 生命周期 |
| `mock` 块 / `vmodel/anthropic` 回答生成 | 步骤的 `reply` 就是 mock 字段，走同一个 Anthropic 回答路径（含 capture 文档 §3.4 修过的流格式） |
| `sequence` | 不改。它是"上游的全局状态"，script 是"每条对话的状态"，语义不同 |
| `--fallback` | 让所有 model id 进脚本；`strict: false` 时也是未匹配请求的去处 |
| `--capture` | 套在外面，trace 与 `index.jsonl` 共用 `seq` |
| `cli/harness agent --mock` | 拓扑不同（经网关）。`agent --mock --config script.yaml` 正好是 `cli-harness-refactor.md` §5.4 那个延后项的一种答案，但那需要用户决定，本文不替它做决定 |
| `projects/internal/harness` | 补充（§5.4） |
| `e2e-demo/mock_anthropic.py`、`run.sh` | 第一阶段的第一条场景就是它的流程；之后可以用脚本替换 `mock_anthropic.py`（`run.sh` 已过时，修它是另一件事） |
| `.research/inject-probe/probe.py` | 以后的协议探针直接写成脚本（`hold` + trace 覆盖了它手写的部分） |

## 8. 否决的做法

- **全局到达顺序（扩展 `sequence` 加工具调用）**：并发下错配，正是问题本身。
- **只按 session 头路由**：代理模式下头默认不发；子 agent 是否共用 session 未知；就算能分组，
  也给不出"走到第几步"。只作诊断和消歧。
- **录真实会话再回放**：每次 CC 升级 prompt 都变，录的东西很快对不上；还要真凭证和外网。
  capture 只用于看清 CLI 发了什么。
- **脚本语言（Starlark/JS）或状态机分支**：我们的回答决定了对话走向，一条对话里唯一的不确定来自
  外部（tool_result 内容、插入的消息、信封），用 `when` + `optional` 就够；真不够时用有名字的 Go 回调。
- **每个场景一个 Python mock**（现状）：不能复用，没有断言、没有覆盖检查、没有 trace。

## 9. 分阶段计划

每个阶段一个（或一串小的）PR，先写用例再写代码。

| 阶段 | 内容 | 不做 |
| --- | --- | --- |
| **P1 最小可用** | 1) 实测：用 `tb vmodel --capture`（`strict: false` 的草稿脚本）跑一遍单线程旅程，记下真实出现的 request class、头（`CLAUDE_CODE_GATEWAY_HINT_HEADERS=1` 在代理模式下是否生效）、辅助请求种类、协调者/线程的绑定文本，写进 `.research/`；据此定下 §2.2 的启发式。2) `vmodel/script`：Go 结构 + `kind: script` 加载与校验；路由（续接索引、bind、背景规则、严格未匹配）；轨道内顺序、`when`/`optional`/`repeat`；`expect`、`expect_result`、`capture` + `{{var}}`；`after`（轨道间）+ `signal`；确定性 id；`count_tokens`；`Finish()` 报告 + `trace.jsonl`；只做 Anthropic Messages（流式 + 非流式）。3) `tb vmodel --config` 能服务它，退出时写报告、失败非零。4) 根模块的 e2e 测试包 + 一个 Projects 驱动小工具（拉起 `tingly-project`、发消息、等审批卡并批准、读线程状态），跑 §5.2 那条旅程；没有 `claude` 就 skip。 | 子 agent、压缩、流中扣住、控制 HTTP 接口、声明式驱动、OpenAI 协议、经网关、IM |
| **P2 覆盖 Projects 功能** | `spawns` 用于 `Agent` 子 agent 和并行工具调用；`compact` 步骤；`hold`（流中扣住）→ 消息插入旅程；错误注入与重试规则；无序步骤组（`unordered: [..]`，组内各步靠 `when` 区分，用于"两条汇报谁先到都行"）；`trace.md`；`tb vmodel` 的控制接口（`POST /_vmodel/script/signal/<name>`、`GET …/wait?step=`），让非 Go 驱动也能用；旅程：两个并行线程、`notify_coordinator` → `propose_threads` 卡、决策卡、`create_pr`（本地裸仓库）、审批拒绝 | 经网关、IM |
| **P3 扩展** | 经 tb 网关的拓扑（tb 内嵌 Projects，provider 指向脚本服务器）；IM（假平台）；OpenAI Responses（codex）；视 Go 驱动的重复程度决定要不要声明式 `drive:`；`cli/harness agent --mock --config`（需用户先决定 §5.4） | — |

P1 的规模估计：`vmodel/script` 是主体（路由 + 校验 + 报告），回答生成全部复用；Projects 驱动
只覆盖一条旅程用到的几个 API。P1 结束时 `mock_anthropic.py` 的流程可以完全用脚本表达，作为验收。

## 10. 需要用户决定的点

1. **格式**：`kind: script` 放进 vmodel spec 信封（建议），还是独立文档类型 + `--scenario`。
2. **运行档位**：wire 档只手动 / 夜间 / CC 升级时跑（建议，像 `harness-agent-testing.md`），
   还是进 CI（要在 CI 里装 `claude` CLI；不需要 API key，但每条旅程几十秒）。
3. **未匹配请求的默认**：严格失败（建议，测试用）+ `strict: false` 写脚本时用，还是默认宽松。
4. **P1 的拓扑**：独立 `tingly-project` 进程、模型服务器直连（建议），还是一开始就走 tb 内嵌 + 网关。
5. **e2e 测试放哪**：根模块新包（例如 `internal/projectse2e`，建议），还是 `cli/harness` 的新子命令。
6. **与进程内 harness 的关系**：补充、只对齐旅程名（建议），还是要求某些旅程两档都必须有。
7. 原话里"精准的去测试Pro"本文理解为 **Projects**；若指别的（例如 Pro 版功能），范围要改。
