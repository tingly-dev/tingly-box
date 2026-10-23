# Claude Code 场景

路径：`/agent/claude_code`（默认配置）与 `/agent/claude_code/profile/:profileId`（每个 Profile 一个页面）

Claude Code 是 Tingly-Box 的主力场景，将 Claude Code CLI 的请求代理到你配置的 Provider，支持按槽位（slot）配置模型规则、多 Profile 管理和细粒度路由。

这里描述的页面结构（页头、连接卡、Plugins 卡、Quick Start、Model Rules）是所有 Agent 页面共用的——其他 Agent 章节只写各自不同的部分。

---

![Claude Code 场景](../images/claude-code.png)

## 页面结构

从上到下依次为：

### 1. 页头：标题、状态行、按钮

- **标题**：`Claude Code`，旁边有一个**信息**图标，悬停可看该 Agent 的说明。
- **状态行**（标题下方的纯文字）：一个绿点加 `Connected · Last request 4 minutes ago · 1842 requests today · 0.7% error rate`。第一次请求到来之前显示 `No requests yet · finish the setup below`（已连接则显示 `No requests yet`）。一眼就能回答「这个 Agent 真的在用网关吗」。
- **配置状态**：紧挨着状态文字，只有网关能读回其配置文件的 Agent 才有（Claude Code、Codex、DeepSeek Harness、WorkBuddy）：
  - 文件与本页一致时什么都不显示（旧的绿色对勾已取消——没有提示就是已生效）；
  - 警示色的 `Config out of date · 1 difference`——悬停可看文件路径和每个 `字段：已应用值 → 期望值`，然后重新运行 **Auto Config**；
  - 灰色的 `Not applied`：这台机器上的配置文件还没有指向 Tingly-Box。
- **按钮**：一行，靠右排列：**Requests**（该 Agent 最近请求的弹窗，带 *Open in Dashboard* 链接）、**Usage**（打开已按该 Agent 过滤的[用量面板](./11-dashboard.md)）、**Log**（该 Agent 的 AI 请求日志和系统日志）、**Quick Start**（仅当 Quick Start 已完成并自行隐藏后才出现——点击会把引导卡重置后带回来），以及最重要的操作按钮——实心、最靠右：**Auto Config**（网关无法代为配置的 Agent 则是 **Setup Guide**）。

### 2. 连接卡

Agent 的接入地址。Agent 已连接后折叠成一行 `Base URL`（配置过期时会重新展开）。点击这一行或卡片右侧的箭头即可展开/折叠，选择会记在本机。展开后包含：

- **Base URL** `http://localhost:3000/tingly/claude_code`：带复制按钮和 **Local | Docker** 切换（Docker 会把主机替换成 `host.docker.internal`，供容器内的客户端使用）
- **API Key**：Model Token，带显示和复制按钮
- **Slots**：见下文[槽位](#槽位按槽位配置模型规则)

折叠后只有一行（这里是 Codex 页面上的样子）：

![折叠的连接卡](../images/agent-connection-folded.png)

### 3. Plugins 卡

一条汇总场景级插件的长条——每个插件一个分段（名称 + 当前取值，偏离透传默认值时高亮）。点击打开面板：

![Plugins 面板](../images/agent-plugins-panel.png)

| 插件 | 选项 |
|------|------|
| **Thinking** | `By Client`（透传客户端设置）/ `Off` / `Low`（~4K token）/ `Medium`（~10K）/ `High`（~20K）/ `Max`（~32K）。优先以 Provider 原生的 effort 参数发送，必要时回退为 token 预算 |
| **Vision Proxy** | `Off` / 选定一个视觉模型来描述图片，让纯文本模型也能处理带图请求 |
| **Record** | 切换采集点——**Client request**（`In`，客户端发来的原样请求）和 **Upstream request**（`Out`，最终发给 Provider 的请求）。记录写入配置目录下的 `record/` 文件夹 |

这些设置作用于该 Agent 的每条规则；规则自身的 Plugins（见[路由规则与插件](./20-routing-rules.md)）可按规则覆盖。Embedding、Image、Decisions 场景只显示 **Record**——思考和视觉对它们没有意义。

### 4. Quick Start

![全新安装时的 Quick Start](../images/claude-code-quickstart.png)

引导首次使用的 4 步卡片。能用事实判断的步骤会自动打勾，全部完成后自行隐藏。

| 步骤 | 完成条件 |
|------|----------|
| 1. **Connect AI Provider** | 已连接 Provider。**Connect AI** 会*就地*打开选择器；添加完成后卡片直接进入第 2 步，页面不会重新加载 |
| 2. **Select a Model** | 某条规则有了 Provider + 模型。**Choose Model** 会滚动到 Model Rules 并高亮 **Add model**；可 **Skip** |
| 3. **Install Claude Code** | Claude Code 已经通过网关发出过请求、或其配置读回为已应用、或你手动确认。展开可看 npm 安装命令（官方源和镜像源，一键复制）和 **I've installed it** |
| 4. **Auto Config** | 配置读回为已应用。**Auto Config** 一键写入配置*并*安装状态栏；**Config** 先打开完整对话框供你检查；**Skip** 可跳过 |

- 已被事实满足的步骤会显示 `Skipped — a model is already set`，而不是声称「安装过」
- 完成的步骤带 ✓，可再次展开；**Reset progress** 清除手动记录的进度；**?** 重新打开[直连路由引导](./20-routing-rules.md#首次使用引导)
- 四步全部完成后页面不再显示该卡片；页头的 **Quick Start** 可以把它带回来（进度重置）；当所有步骤都已由事实满足时，它显示为 `Quick Start · Done` 并带一个 **Hide** 按钮

### 5. Model Rules

每条规则是一张可折叠的节点图——哪些规则会出现见[槽位](#槽位按槽位配置模型规则)，图本身见[路由规则与插件](./20-routing-rules.md)。页面工具栏：按名称排序、全部折叠/展开、**?** 引导、**Test All**（对所有启用的规则做一次快速流式测试）、**Connect AI**。Claude Code 的规则在这里不能新增、删除或关闭——每条都对应一个槽位——所以本页没有 **New Model** 按钮。

![模型选择弹窗](../images/model-select.png)

点击 Provider 节点，或空规则里的 **Add model**，打开 **[Model Select](./21-model-select.md)** 弹窗。

---

## 槽位：按槽位配置模型规则

Claude Code 按槽位请求模型。连接卡里的 **Slots** 一行把它们显示成标签：`default` · `haiku` · `sonnet` · `opus` · `fable` · `subagent`，各自对应一个环境变量（悬停标签可看）：`ANTHROPIC_MODEL`、`ANTHROPIC_DEFAULT_HAIKU_MODEL`、`ANTHROPIC_DEFAULT_SONNET_MODEL`、`ANTHROPIC_DEFAULT_OPUS_MODEL`、`ANTHROPIC_DEFAULT_FABLE_MODEL`、`CLAUDE_CODE_SUBAGENT_MODEL`。

**不再有 Unified / Separate 模式开关**——状态就是哪些槽位有自己的规则：

- `default` 是**主规则**，名为 **`cc`**，始终开启。没有自己规则的槽位都使用它。
- 点击其他标签，给该槽位**单独一条规则**（标签变为实心，Model Rules 里出现一条以槽位命名的规则——`sonnet`、`opus`……——初始复制主规则的路由，所以在你编辑之前什么都不会变）。再次点击把槽位还给主规则；规则只是被关闭，再次开启即可恢复。
- **Split all** 让每个槽位都有自己的规则；之后它变成 **Merge all**，把它们全部并回主规则。「所有请求用一个模型、只有 subagent 用另一个」就是只拆 `subagent`。
- 页面上显示的规则：主规则加上已拆分槽位的规则。

![所有槽位都已拆分](../images/claude-code-slots.png)

> 此前创建的规则会自动迁移：原「分离模式」的配置保留其路由（旧的 `default` 规则并入 `cc`），仍写着 `tingly/cc`、`tingly/cc-sonnet`…… 的 settings 文件继续可用。

### 1M 上下文

每条规则的标题上有一个 **`1M: Off ⌄`** 标签。选择 **On** 为该规则加上 1M 上下文窗口（上游模型必须支持）；**Off** 只表示这条规则不额外加 1M——客户端自己请求 1M 时仍会生效。如果 Claude Code 的配置已经应用过，切换会**直接重新应用**——不弹对话框——并保留你之前应用的其他设置，同时弹出提示 `1M context on. Claude Code's config is updated; restart Claude Code to take effect.`

### Endpoint

**`Endpoint: Auto ⌄`** 标签选择该规则的上游 API 风格：**Auto**、**Chat**、**Responses** 或 **Anthropic**（*Force Anthropic style*：始终向上游发送 Anthropic Messages，OpenAI 风格的客户端会被转换；没有 Anthropic 端点的 Provider 会回退到 Auto）。它只对 OpenAI 兼容的上游有意义；规则里没有这类上游时，提示会这样说明。

![Endpoint 菜单](../images/rule-endpoint-menu.png)

---

## Auto Config

![Auto Config 模态框](../images/claude-code-config-modal.png)

**Auto Config** 打开 **Claude Code Configuration Guide**，包含两个 Tab。打开时会恢复你上次应用的值；首次使用则从当前规则推导推荐值。

**Auto Config 标签页**——从上到下：

- **Default permission mode**：`settings.json` 中的 `permissions.defaultMode`：Accept edits（推荐）/ Default / Manual approval / Plan mode / Auto rules / Delegate / Don't ask / Bypass permissions
- **Thinking summaries**：**Show thinking summaries** 写入顶层 `showThinkingSummaries`（默认开启；关闭只是隐藏摘要，不影响模型是否思考）
- **Commit attribution**：**Credit Claude**，**默认关闭**：开启后 Claude Code 会在提交里加 `Co-Authored-By: Claude`，在 PR 描述里加「Generated with Claude Code」；关闭时 `includeCoAuthoredBy` 为 `false`，`attribution.commit` / `attribution.pr` 为空
- **Model routing**：上面六个槽位变量，各自按你的规则预填（`cc`、`sonnet`……，开启 1M 时为 `…[1m]`），每个有自己的 **1M** 开关。没有自己规则的槽位跟随 default 槽位
- **Performance & limits**：`API_TIMEOUT_MS`、`CLAUDE_CODE_MAX_OUTPUT_TOKENS`、`MAX_THINKING_TOKENS`、`BASH_DEFAULT_TIMEOUT_MS`、`BASH_MAX_TIMEOUT_MS`、MCP 超时与输出上限、子 Agent 相关项（`CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS`、`CLAUDE_CODE_AUTO_BACKGROUND_TIMEOUT_MS`、`CLAUDE_ASYNC_AGENT_STALL_TIMEOUT_MS`、`CLAUDE_AUTO_BACKGROUND_TASKS`）以及自动压缩（`CLAUDE_CODE_AUTO_COMPACT_WINDOW`、`CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`——默认 **80**，`0` 表示禁用）。留空 = 不写入，Claude Code 用自己的默认值
- **Privacy & behavior**：遥测 / 错误上报 / 非必要流量 / 自动更新开关以及 `USE_BUILTIN_RIPGREP`（开 = `"1"`，关 = 不写入）
- **Network proxy**：`HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY`（Tingly-Box 总会把 `localhost,127.0.0.1,::1` 追加到 `NO_PROXY`）
- **Install Tingly-Box Claude Code status line**（默认勾选；两个 Tab 都可见）：在 `~/.claude/settings.json` 里加入状态栏，在 Claude Code 提示符里显示当前连接状态
- 右上角的重置图标会恢复 Tingly-Box 推荐的默认值；**Preview generated env** 显示将要写入的 `env` 块

底部的 **Auto Config** 按钮写入设置（勾选时包括状态栏）；成功后对话框关闭，并提示写入了哪些文件、是否生成备份。

**Manual 标签页**：同样的设置，以原始脚本形式给出——`~/.claude/settings.json` 和 `~/.claude.json`（JSON / Windows PowerShell / Linux·macOS），外加状态栏脚本，适合高级用户或网关无法直接写文件的机器。

![Manual 标签页](../images/claude-code-config-manual.png)

---

## Profile 管理

Claude Code 支持多个 **Profile**，适合需要不同 Provider 或路由的项目/团队。它们在侧边栏的 Claude Code 之下按**名称**列出，副标题为 `Claude Code · <id>`；列表末尾的 **New profile** 会让你输入名称。每个 Profile 有自己的页面、Base URL（`/tingly/claude_code:<id>`）、规则和设置。

![Claude Code Profile 页面](../images/claude-code-profile.png)

- **标题**：`Claude Code - ds`（Profile 的名称），旁边是**信息**（显示 Base URL）、**重命名**（铅笔）和**删除**（垃圾桶）。状态行以 Profile id 开头：`p1 · No requests yet`
- **按钮**：**Requests**、**Usage**、**Log**，以及实心的 **Setup Guide**——两步对话框（*Connect an AI provider* → *Start Claude Code with this profile*，附可复制的命令）
- **连接卡**：和 Agent 页面一样可折叠。第一行在 **Start**（启动命令 `tingly-box profile p1` 或 `npx -y tingly-box@<version> profile p1`，带 **NPX | Global** 开关）与 **Settings**（生成的 settings 文件路径，有标记表示是否已生成；手动编辑会被覆盖）之间切换。之后是 Base URL、API Key 和 **Slots** 行，与主页面完全一致
- **Plugins 卡**：同上，按 Profile 生效
- **Profile Overrides**：一张紧凑的卡片，初始为 `Fully inherits the main configuration and model routing`。**Add** 打开可搜索的运行参数列表（**Common**：最大输出 token、默认权限模式；**Runtime**、**Limits & tools**、**Behavior & privacy**、**Network**），每项显示继承值。被覆盖的行显示 `Inherited <值>` 和一个 **×** 用来取消覆盖；**Save** 保存，**Restore inheritance** 清除全部覆盖。模型不在这里覆盖——模型来自规则

  ![Profile 覆盖](../images/claude-code-profile-overrides.png)

- **Model Rules**：该 Profile 的主规则 `cc` 以及已拆分槽位的规则

![Profile 引导](../images/claude-code-profile-setup.png)

每个 Profile 生成的 Claude Code 设置保存在可读的 `<profile-id>--<profile-name>` 目录（如 `p1--deepseek`）；重命名或删除 Profile 时目录会同步。

### CLI：用 Profile 启动

#### `tingly-box profile`：查看与启动

```bash
tingly-box profile               # 交互式：列出 Profile，选择后启动
tingly-box profile p1            # 用 Profile p1 启动 Claude Code
tingly-box profile p1 --port 12580  # 连接端口 12580 上的远程 Tingly-Box
tingly-box profile --list        # 列出 Profile（非交互）：ID、名称、槽位
tingly-box profile --show p1     # Profile ID/名称、场景路径、槽位和规则表
```

> `--list` 与 `--show` 互斥。找不到指定的 Profile 时，启动和查看都会回退到交互式选择器。

#### `tingly-box cc`：完整透传 Claude CLI 参数

Tingly-Box 先消费自己的参数，其余原样透传给 Claude CLI——不需要 `--` 分隔符。

```bash
tingly-box cc                           # 启动 Claude Code（默认 Profile）
tingly-box cc --profile p1              # 用 Profile p1 启动（简写：-p p1）
tingly-box cc -p p1 --tingly-port 12580  # Profile p1，远程 Tingly-Box 端口 12580
tingly-box cc -p p1 --dangerously-skip-permissions  # Claude 参数原样透传
tingly-box cc -p p1 /path/to/project    # 在指定目录打开 Claude Code
```

Tingly-Box 自己的参数（`--profile`/`-p`、`--tingly-port`）必须放在所有 Claude 参数之前。

---

## 在浏览器和 IM 里使用 Claude Code

除了 CLI，Claude Code 也可以直接在 Tingly-Box 里运行：**Projects** 页面通过同一个场景启动会话和项目线程（所以你的规则、配额和用量照常生效），IM 聊天也经由 Projects 与 Claude Code 对话。详见 [Projects](./22-projects.md)。

---

## 常见配置流程

1. 在[凭证管理](./08-credentials.md)中至少添加一个 Provider（或在 Quick Start 里点 **Connect AI**）
2. 打开 Claude Code 页面，确认 Base URL 和 API Key
3. （可选）给槽位单独配模型——例如把 `subagent` 或 `haiku` 拆出来用更便宜的模型
4. **Auto Config** → 检查 → **Auto Config**（需要状态栏就保持勾选）
5. 启动 Claude Code；页面顶部的状态行变为 `Connected`

---

## 相关页面

- [场景总览](./02-scenario-overview.md)
- [凭证管理](./08-credentials.md)
- [路由规则与插件](./20-routing-rules.md)
- [Projects](./22-projects.md)
- [其他编码 Agent](./04-scenario-coding-agents.md)
