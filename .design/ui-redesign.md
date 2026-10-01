# UI 重设计 — 重新思考（讨论稿）

> 状态：**讨论稿，未实施**。本文不是实施计划，而是"先想清楚"：现在哪里不对、什么必须保留、新的骨架长什么样、GUI（Wails v3 WebView）对前端的要求、以及在大量存量用户下怎么安全地换过去。
>
> 修订：初稿提议改为单列侧栏，评审认为对存量用户心智变化过大，已改为保留 rail + sidebar 两列、只做减负（§3.2）。GUI 已确定继续用 Wails v3（§4）。
>
> 判断标准沿用 `ux-principles.md` 的 12 条（下文用 **P1–P12** 引用）。现状证据来自 mock 模式截图（1440×900 与 390 宽）和代码走读（2026-09-30）。

---

## 进度（2026-09-30）

| 阶段 | 状态 | 落地内容 |
|---|---|---|
| P0 词汇 | ✅ | Team Keys 统一；Guardrails 的密钥页改名 Secrets；Credential 侧栏行改为 Credentials（见 §3.6） |
| P1 路由契约 + Host Bridge | ✅ | `routes/appRoutes.tsx` + `routes.contract.test.tsx`（托盘路径、所有 redirect）；修复托盘 `/agent/claude-code`；`host/` bridge（浏览器 / Wails 两个实现）；UI 偏好存服务端 `/api/v1/ui-prefs`（隐藏的 Agent、Quick Start 进度）。主题 / 语言仍在本机：它们登录前就要生效 |
| P2 Dashboard 概览 | ❌ 已删除（2026-09-30） | 实现过 `/dashboard/overview`（网关状态、需要处理的事项、各 Agent 最近请求），也试过并入 `/agent`；评审认为整页意义不大：网关状态由 rail 表达，Agent 列表与 sidebar 重复。已删除，旧地址重定向到用量页 |
| P3 导航减负 | ✅ | 底部 4 个按钮合并为偏好菜单；单页 activity 不弹 sidebar；时间范围改为页内筛选；profile / Team 行以名字为主标签；折叠时 rail 浮层；900–1200px 自动折叠；断线指示收进 rail；Dashboard 默认进入用量页。⌘K 未做（增量，后续再议）；GitHub star：两处——Agent 页顶部的可关闭横幅（关闭后 4 天内不再显示，只记在本机浏览器 / 窗口里，不跨端同步；Team 页不显示），以及 sidebar 底部版本号旁常驻的「star」小链接，和版本号同一写法、徽标颜色跟随版本徽标 |
| P4 Agent 页 | ✅（Profile 合并暂缓） | A–D 已落地：descriptor 模板、配置状态；E 中插件与模式切换的挪位、Quick Start 3 步重排，评审后撤回。见 `agent-page-redesign.md` 顶部进度表 |
| `/agent` 目录页、Power-ups | ✅ 已拆除（2026-09-30） | `/agent` 卡片页与 sidebar 重复，删除：`/agent`（rail、落地页、`*`）直接打开上次的 Agent 页（`pages/scenario/lastAgent.ts`，本机记忆，隐藏了则退到第一个可见的）；显示/隐藏改为 sidebar 标题栏的眼睛按钮进入的编辑状态（隐藏的行变灰列出，每行一个眼睛开关）；眼睛第一次出现时弹出指向它的引导气泡，点「知道了」或用过一次眼睛后不再出现（只记在本机）。曾试过铅笔图标（不直观）和悬停眼睛 + 底部「已隐藏 N 个」（不如一个固定入口加一次性教学清楚），均已替换，Agent 的一句话介绍变为 sidebar 行的延迟 tooltip。Power-ups 管的是"rail 上有哪些项"，Team / Image 的开关一并并入。位置几经调整：rail 图标列表（挤掉 Help）、rail 底部单独按钮（看起来奇怪），最终放进左下角用户菜单的二级菜单「Power-ups ›」（悬停展开），与语言、主题这类设定放在一起。挪了位置的入口都配一次性引导气泡（`components/CoachMark` + `hooks/useOneTimeTip`）：眼睛按钮一个、用户按钮一个（"扩展功能的开关移到了这里"）；同一时间只显示一个，用户按钮的排在眼睛之后；点「知道了」或用过一次该入口即永久关闭，只记在本机浏览器 / 窗口里 |

---

## 0. 结论先行

1. **这次重设计的核心是信息架构，不是换皮。** 现在的问题主要是"东西放在哪、叫什么"，不是"长得不好看"。视觉刷新可以放最后，甚至可以不做。
2. **补上"状态概览"，放在 Dashboard 下面，不新增 rail 项。** 用户打开 Tingly Box 第一个问题是"它在正常工作吗"，不是"我要配置哪个场景"。
3. **保留 rail + sidebar 两列布局，只给它"减负"。** 用户已经熟悉这套结构，改成单列的心智成本太高、收益不够。要做的是：rail 底部 4 个杂项图标合并为 1 个菜单；单页目的地不再弹 sidebar；时间范围移出 sidebar；profile 用自己的名字；窄窗口下 sidebar 自动折叠。
4. **先修词汇碰撞，再动布局。** 现在有同一个 i18n key 同时指两个不同页面的情况，这是最便宜、收益最高的一步。
5. **Wails v3 下，前端要"一套代码、多个宿主、按能力降级"。** 保留 Wails asset server 的现有接法（API 已经走同一个 gin engine），把宿主差异收进一个 Host Bridge 模块；UI 偏好移到服务端，解决 `wails://` 与 `localhost` 两个 origin 的偏好分裂。路由要成为 Wails 托盘 / CLI / 前端共用的契约，托盘里现在就有一条写死的错路由。
6. **迁移按层推进、每步可回退：** 词汇 → 路由契约与 bridge（用户无感）→ Dashboard 概览（纯增量）→ 导航减负（逐项）→ 页面内交互 → 视觉。任何一步都不改 endpoint 路径、不改 Auto Config 写出的配置、不让旧 URL 失效。

---

## 1. 现状诊断

### 1.1 导航：两级、过长、把过滤条件当成目的地

- 主 rail 11 项（Dashboard / Agent / Team / Image / Remote / Prompt / Tools / Guardrails / Bench / Credential / System），底部还有 Feedback / 语言 / System / 用户，共约 15 个图标。**1440×900 下 rail 已经放不下**，Credential 下面那一项被裁切。
- 每个 rail 项都会展开一个 200px 的二级 sidebar，**只有一个页面的目的地也要走两级**（Bench 例外）。rail 72px + sidebar 216px ≈ 288px 常驻宽度。
- Dashboard 的 sidebar 里放了 Today / Yesterday / 3d / 7d / 30d / 90d 六个"导航项"。时间范围是**过滤条件**，不是目的地（P4：两个轴被塞进了一个控件）。
- 高频目的地（某个 Agent 的规则）和低频目的地（Develop、Experimental）在视觉上同等权重。

### 1.2 落地页回答的不是用户的问题（P1）

默认进入 `/agent`，这是一个"选择场景 + 隐藏不用的场景 + 开关 Power-ups"的**管理目录**。但一个已经配好的用户每天打开时想知道的是：

- 服务在跑吗？我的工具连上了吗？
- 哪个 provider 额度快用完、OAuth 快过期、最近在报错？
- 刚才那个请求路由到了哪个模型？

这些信息现在分散在 System › Server Status、Credentials 表格里的额度子行、Dashboard 的错误率、右下角浮动的红色 `!`，没有一个页面能回答"现在一切正常吗"。

### 1.3 主角不是主角（P9）

- Agent sidebar 里连续 4 行都叫 **"Claude Code"**，真正的区分信息（`p1 - ds`、`p2 - sonnet`）在灰色副标题里。Team 也一样（"Team / default - Default"、"Team / t1 - Platform"）。
- 规则卡里，没有配置插件的规则也会渲染一整块 "Plugins — None enabled. Click to configure." 的虚线面板；OpenAI SDK 页 3 条规则里这句话出现了 2 次。
- 每条规则都是一张完整横向路由图（Direct/Smart 切换 + T0 + 模型卡 + Add model + 插件面板）。图是很好的**教学和编辑**载体，但作为**浏览**视图太重：Team 页 4 条规则就要滚两屏。

### 1.4 词汇碰撞（P3）

| 现象 | 位置 | 问题 |
|---|---|---|
| **Credential** | 顶层 rail「Credential」 与 Guardrails › 「Credential」 | 两个完全不同的东西，**共用同一个 i18n key `layout.nav.credential`** |
| **Team Keys / Sharing Keys** | 导航叫 Team Keys，Team 页按钮和弹窗叫 Sharing Keys | 同一件东西两个名字 |
| **Credentials / Model Key / Providers / Connect AI** | Credentials 页标题、sidebar「Model Key」、页内「Providers」按钮 | 用户"我有哪些 AI 可用"这一件事有四种叫法 |
| **Agent** | rail「Agent」＝客户端场景；Remote Control 路由图里的「Agent: SmartGuide / Claude Code」＝被遥控的执行体；路由 `/remote-agent` | 一个词三个意思 |
| **Remote / Remote Control / Bots / IM Notify / Desk** | rail 叫 Remote，页叫 Bots，子项叫 Remote Control，路由叫 `/remote-agent` 和 `/bots` | 层级和名字不一致 |
| **Plugins** | Agent 级（Thinking / Smart Compact / Vision Proxy / Record）与规则级（Session affinity / Clean Header…） | 这更像是同一类东西的**两个作用域**，但 UI 呈现为两个互不相关的盒子，看不出继承关系（P4） |
| **可选能力** | Power-ups / Tools / Plugins / Experimental / Beta / Exp. | "这是可选的"有六种说法 |

### 1.5 全局噪声（P9、P12）

- GitHub star 横幅出现在**每一页**顶部，占掉约 60px。
- 右下角浮动的 `!`（`FloatingStatusIndicators`，显示断线与新版本，开发模式下强制显示）浮在内容区上方。它显示的信息是对的，只是位置不对，应该移进 layout（见 `agent-page-redesign.md` §5）。
- 语言、反馈占据 rail 的位置；版本号和 slogan 常驻 sidebar 底部。

### 1.6 多宿主之间已经在漂移

- 托盘菜单（`gui/wails3/systray.go`）写死跳转 `/agent/claude-code`，而真实路由是 `/agent/claude_code`，结果落到 `*` → 被重定向回 `/agent`。**Go 侧写死的路由没有任何测试会发现它坏了。**
- 隐藏场景、sidebar 折叠、当前 activity、各种"已关闭提示"都存在 `localStorage`，按 origin 隔离。浏览器（`http://localhost:port`）和桌面壳（`wails://`）是两个 origin，**同一个用户在两个入口看到的偏好不一样。**
- `utils/protocol.ts` 为 GUI 模式单独分支（返回 `wails://` 作为展示 URL）；`bindings-wails` / `bindings-web` 靠 Vite alias 切换。宿主差异散落在构建配置和工具函数里，而不是一个明确的接口。

### 1.7 窄屏

390 宽下，Claude Code 页标题栏的 `Unified / Separate / Auto Config` 被挤出屏幕，Base URL 被截断成 `http://localhost:3000/…`，而这正是用户要复制的东西（P5、P11）。桌面主场景不必为手机优化，但**WebView 窗口可能只有 1000px 宽**，同类问题会更早出现。

---

## 2. 必须保留的资产

重设计不是推倒重来。以下东西是对的，而且是 Tingly Box 的辨识度所在：

- **路由图**（Direct/Smart、Tier、请求旅程）：保留为规则的**编辑与教学**视图，只是不再是唯一的浏览视图。
- **Connect AI 统一入口**、**Probe / Troubleshoot 走真实链路**、**Quick Start 完成后可重新展开**、**EntryGuide / TierGuide / TeamGuide 内嵌教育**。
- **所有对外契约**：`/tingly/<scenario>` endpoint、API key 格式、Auto Config 写到 `~/.claude/settings.json` 等文件的内容。UI 重设计**不碰**这些，用户已经配好的工具一行都不需要改。
- 旧 URL：`App.tsx` 里已经有一长串 legacy redirect，说明团队已经在为书签和文档链接负责，新设计要延续这一点并加测试。

---

## 3. 新的信息架构

### 3.1 从"用户的问题"出发

| 用户的问题 | 目的地 | 现在的位置 |
|---|---|---|
| 它现在正常吗？需要我处理什么吗？ | **Dashboard › 概览**（新的默认子页） | 分散在 System、Credentials、Dashboard、浮动 `!` |
| 怎么把我的工具（Claude Code / Codex / SDK）接上？ | **Agents › 某个 Agent › 接入** | Agent 页上半部 + Quick Start |
| 某个模型名的请求会被路由到哪里？ | **Agents › 某个 Agent › 路由** | Agent 页 Model Rules |
| 我有哪些 AI 可用、额度还剩多少？ | **Credential**（名字见 §3.6） | Credential › Model Key / VModel |
| 花了多少、谁在用？ | **Dashboard › 用量** | Dashboard + Team usage + Quota history |
| 谁还能用这个实例？ | **团队 Team** | Team rail + Team Keys + Access Control |
| 扩展能力（远程、图像、护栏、MCP、Bench、Prompt） | 保持各自的 rail 项，只显示已启用的 | 同左；启用开关在 `/agent` 页的 Power-ups 区 |
| 设置、日志、实验性功能 | **设置 Settings** | System |

### 3.2 导航骨架：保留两列，只减负

rail + sidebar 两列布局是现有用户最熟悉的东西，也是 VS Code 一类工具的通用心智。改成单列会让每个老用户都要重新找一遍所有东西，这与"谨慎"的前提冲突。§1.1 列出的问题都可以在现有结构里解决：

| 问题 | 在现有结构内的改法 | 用户感知 |
|---|---|---|
| rail 在 1440×900 下放不下 | 底部 Feedback / 语言 / 主题 / 用户 4 个图标合并成 1 个用户菜单，腾出 3 个位置 | 很小：高频图标位置不变 |
| 单页目的地也要走两级 | activity 只有一个子页时不显示 sidebar（Bench 已经是这样），推广为通用规则 | 很小 |
| Dashboard 的 sidebar 放了 6 个时间范围 | 时间范围改为页内分段选择器；sidebar 只留 概览 / 用量 / Team usage / Quota history。`/dashboard/7d` 等 URL 保留，页内选择器直接读 URL | 小：时间范围从左边移到页面顶部 |
| 4 行都叫 "Claude Code" | profile 行的主标签改为 profile 名字（`p2 · sonnet`），缩进挂在 Claude Code 下面，品牌图标保留 | 小 |
| 桌面窗口宽度不够 | 窗口宽度 < 1200px 时 sidebar 自动折叠（`useSidebarCollapsed` 已有），rail 保持不变；悬停 rail 图标时以浮层展开 sidebar | 小：只影响窄窗口 |
| 右下角浮动 `!`、每页的 star 横幅 | 状态收进 rail 顶部 logo 旁的状态点和 rail 图标角标；star 横幅只在 Dashboard 概览出现一次 | 小 |

```
┌────┬──────────────────┬───────────────────────────────────────────┐
│ T● │ Agent         ✎ ⇤│  Claude Code › p2 · sonnet                │
│────│                  │                                           │
│ ▤  │ ✳ Claude Code    │                                           │
│ ⁘◀ │   default        │                                           │
│ ⚇  │   p1 · ds        │                                           │
│ ▣  │   p2 · sonnet ◀  │                                           │
│ ⇄ 1│   + 添加 Profile │                                           │
│ …  │ ──────────────── │                                           │
│ 🔒⚠│ ✳ Claude Desktop │                                           │
│ ⚙  │ ◐ Codex          │                                           │
│    │ ▢ OpenCode       │                                           │
│────│ …                │                                           │
│ 👤 │                  │                                           │
└────┴──────────────────┴───────────────────────────────────────────┘
```

- **rail 的顺序和图标不变**，老用户的肌肉记忆保留；变化只发生在 sidebar 内容和底部杂项。
- **角标**承载"需要处理"的信号（额度低、OAuth 过期、Bot 离线），替代右下角浮动 `!`。
- **⌘K 命令面板是增量，不是替代**：跳转到任意 Agent / 规则 / Credential，直接执行"复制 Base URL"、"Connect AI"。它同时是改名期的安全网——旧名字作为搜索别名。
- 如果将来 rail 仍然超长，再考虑把 Prompt / Tools / Guardrails / Bench 这类实验性项合并成一个"扩展"rail 项。那是一次有代价的改动，要单独评估，不在这一轮。

### 3.3 Dashboard 概览：回答"现在正常吗"

不新增 rail 项，而是给 Dashboard 加一个"概览"子页作为默认页；现有的用量图表挪到"用量"子页，内容不变。落地页（现在是 `/agent`）先只对新安装切到概览，老用户保持原样，另行评估（见 §6）。

```
┌ 状态 ────────────────────────────────────────────────────────────┐
│ ● 网关运行中 · 127.0.0.1:12580 · v1.x · 今日 812 次请求 · 错误 0.5%   │
└──────────────────────────────────────────────────────────────────┘
┌ 需要你处理 (2) ──────────────────────────────────────────────────┐
│ ⚠ Codex OAuth 当前窗口额度 0%，周额度剩 13%        [查看额度] [切换备用] │
│ ⚠ Claude Code OAuth 7 天窗口剩 12%                  [查看额度]          │
└──────────────────────────────────────────────────────────────────┘
┌ 你的 Agents ─────────────────────────────────────────────────────┐
│ ✳ Claude Code   最近请求 2 分钟前  claude-sonnet-5 → deepseek-v4-pro │
│ ◐ Codex         最近请求 1 小时前  gpt-5.6-sol → claude-opus-4-8     │
│ ⊕ OpenAI SDK    尚未收到请求       [复制接入信息]                    │
└──────────────────────────────────────────────────────────────────┘
┌ 最近请求 ───────────────── (跳到 用量 / 日志) ┐
```

- 对新用户，概览就是 onboarding：同一位置显示"1 Connect AI → 2 选一个 Agent → 3 发出第一个请求"的清单，完成后变为上面的状态视图，但清单仍可展开（P10）。
- "需要你处理"每一项都附带下一步动作（P11），而不是只显示一个红点。
- **概览的数据源和托盘菜单用同一个状态 API**（见 §4.5）。

### 3.4 Agent 页：按"接入 / 路由 / 活动"分区

> **展开版见 `agent-page-redesign.md`，以那份为准**：先做"接入 / 路由"两区，加上常驻的客户端配置状态，活动区暂缓。

> 同样出于"心智变化"的考虑：先在**同一个滚动页面内**按这三块排序（接入卡已连接后折叠成一行摘要，底部新增活动区），而不是一上来就拆成三个 tab。等用户习惯了这三块的划分，再评估是否需要 tab。

```
Claude Code › p2 · sonnet                       ● 已连接 · 最近请求 2 分钟前
[ 接入 ]  [ 路由 ]  [ 活动 ]
```

- **接入**：Base URL / API Key（完整字面值，窄窗口下换行而不是截断，P5）、Auto Config、Quick Start、Unified/Separate 模型模式。已连接后默认折叠为一行摘要，随时可展开（P10）。
- **路由**：规则列表（见 §3.5）+ Agent 级插件。
- **活动**：只看这个 Agent 的最近请求、错误、每次请求实际命中的规则和上游——直接回答"刚才那个请求去哪了"。Troubleshoot / Probe 的入口也放在这里。

### 3.5 规则：紧凑列表浏览，展开后才是路由图

```
claude-sonnet-5   Smart · 2 个条件  →  deepseek-v4-pro, glm-5.1   插件 4   ⚡ ⚙ ▸
claude-opus-4-8   Direct            →  claude-opus-4-8 (Anthropic)           ⚡ ⚙ ▸
gpt-5.6-terra     Direct · T0       →  glm-5.1, deepseek-v4-flash            ⚡ ⚙ ▸
```

- 一行一条规则，行内展示**具体值**（模型名、上游、条件摘要），点开即是现在的完整路由图编辑器。
- 没有插件就不渲染插件面板，只在行尾给一个 `+ 插件`（P9）。
- **插件的两个作用域画成一条继承链**：规则展开后显示"继承自 Agent：Smart Compact: On；本规则：Session affinity 1h"，而不是两个看起来无关的 "Plugins" 盒子（P4）。

### 3.6 词汇表（P0 已落地，2026-09-30）

| 概念 | 用词 | 处理 |
|---|---|---|
| 客户端接入点（Claude Code、Codex、SDK…） | **Agent** | 不变。Remote Control 路由图里的 "Agent: Claude Code" 指的也是同一个 agent，不算碰撞 |
| 上游 AI 凭据与账号 | **Credentials** | ✅ sidebar「Model Key」改为「Credentials」，与页面标题一致（key `layout.credentials`）。（上游已移除页内「Providers」按钮，目录改由 Connect AI 进入） |
| 接入 AI 服务的动作 | **Connect AI** | 已统一，保持 |
| 团队成员使用的 key | **Team Key** | ✅ 所有面向用户的文案（导航、页面、Team 页按钮与弹窗、引导、README）统一为 Team Key(s)；后端标识符（sharing）不变 |
| 护栏里要保护的敏感凭据 | **Secrets** | ✅ Guardrails sidebar 不再复用 `layout.nav.credential`，改为独立的 `layout.protectedCredentials`；页面标题同步改为 Secrets（"Protected Credentials" 在 sidebar 里会被截断） |
| 可选能力的成熟度 | `Exp.` / `Beta` | 不变。这是有意设计的两级成熟度（各带 tooltip），不是一词多义 |
| Remote / Bots / Remote Control / IM Notify | — | 不变。rail 以产品支柱命名、Bots 作为入口，是 `bot-arch.md` §10 的既定设计 |
| 请求级行为开关 | **Plugins**（Agent 级默认值 / 规则级覆盖） | 放到 Agent 页阶段处理（`agent-page-redesign.md` §3.5） |

改名对存量用户有成本：每个改名在一个版本内保留"原 X"提示，⌘K 保留旧名别名，i18n en/zh/ru 同步。

---

## 4. 一并考虑：Wails v3 WebView GUI

> **现状（2026-10）**：本节描述的是重构前的两份构建。已落地的实现见 `gui-host-bridge.md`：一份构建、运行时选择 HostBridge、按名调用 Go、取消 lite edition。

GUI 确定继续用 **Wails v3**。现状（`gui/wails3`）：

- **full 模式**：网关在进程内运行；Wails asset server 的 Handler 就是同一个 gin engine，`/api`、`/tingly` 走 gin，其余请求返回内嵌的前端构建产物（`app.go:31-67`）。前端由 `vite.config.wails.ts` 单独构建，靠 Vite alias 在 `bindings-wails` / `bindings-web` 之间切换。
- **slim 模式**：不内嵌界面，只提供托盘，打开系统浏览器。
- 托盘通过 `systray-navigate` 事件让前端 React Router 跳转；关窗口只隐藏，不退出。

所以前端实际上已经跑在三种宿主里：浏览器（`tb open` / slim）、Wails WebView 窗口、团队部署时的远程浏览器。重设计要以此为前提。

### 4.1 保持现有接法，收敛差异

- **API 已经是同一条代码路径**（asset server 直接转给 gin），这点是对的，保留。
- **保留 Wails asset server 提供页面**，不要改成让窗口打开 `http://127.0.0.1:<port>`：Wails 的 runtime、绑定和事件依赖 asset server，绕开它会丢掉托盘跳转等能力。
- 真正要收敛的是剩下的差异：①两份构建 → 让它们只差 bridge 实现这一个模块；②两个 origin（`wails://` 与 `http://localhost`）→ 偏好不能再存 localStorage（§4.7）；③`protocol.ts` 的 GUI 分支 → 收进 bridge。

### 4.2 Host Bridge：按能力降级，而不是按宿主分支

前端只依赖一个接口，页面里不出现 `if (isGUI)`：

```ts
interface HostBridge {
  kind: 'browser' | 'desktop';
  openExternal(url: string): Promise<void>;      // WebView 里 target=_blank 常被吞
  saveFile(name: string, data: Blob): Promise<void>; // 导出 JSONL / Base64 / 日志
  revealPath?(path: string): Promise<void>;      // 打开 ~/.claude 等配置目录
  notify?(title: string, body: string): Promise<void>;
  setTrayStatus?(s: TrayStatus): void;           // 见 4.5
  window?: { minimize(); close(); startDrag(); };
}
```

- 浏览器实现用标准 API 降级（`window.open`、`<a download>`、Notification）。
- 桌面实现基于 Wails v3 的能力：`Browser.OpenURL`、`Dialogs` 的保存对话框、系统托盘、通知，以及用 CSS `--wails-draggable` 标记窗口拖拽区。Wails 相关代码只出现在这个实现里。
- 现有 `bindings-wails` / `bindings-web` 的 alias 可以保留，但只 alias 这一个 bridge 模块；页面代码不直接 import `@/bindings`，`protocol.ts` 的 GUI 分支也收进来。

### 4.3 WebView 兼容清单（设计阶段就要避开的坑）

Wails v3 在三个平台用的是系统 WebView：macOS WKWebView、Windows WebView2、Linux WebKitGTK（版本可能很旧）。浏览器里没问题的东西在这里可能不工作：

- **链接与弹窗**：`target="_blank"`、`window.open`（GitHub release、文档、OAuth 授权页）→ 统一走 `openExternal`。OAuth 授权流程尤其要在壳里真实走一遍。
- **下载**：所有"导出 / 复制为文件"必须走 `saveFile`，WebView 里 `<a download>` 往往无效。
- **剪贴板**：`wails://` 下 `navigator.clipboard` 是否可用要在三端实测；`useCopyFeedback` 已有 `execCommand` 兜底，必要时再走 Wails 的剪贴板 API。
- **原生对话框**：不用 `alert/confirm/prompt`，统一 `ConfirmDialog`（大部分已完成）。
- **CSS**：WebKitGTK 上谨慎使用 `backdrop-filter`、较新的选择器、`dvh` 等；滚动条样式三端差异大。
- **字体**：中文 / 俄文字体回退链要显式声明，WebView 不一定继承浏览器的默认字体设置。

### 4.4 窗口约束直接影响布局

- 设计基线定为 **最小 960×640、默认 1200×800**，而不是 1440×900 的浏览器全屏。在这个宽度下，rail（72px）常驻，sidebar 在 < 1200px 时自动折叠为悬停浮层（§3.2），内容区至少还有约 880px。
- 现在 macOS 窗口用的是 `MacTitleBarDefault`。若改成隐藏标题栏（inset），rail 顶部要为红绿灯按钮让出约 28px 高度，并用 `--wails-draggable` 设置拖拽区；Windows/Linux 用原生标题栏即可。
- 页面标题栏的操作按钮在窄宽度下应收进 `⋯` 菜单，而不是溢出（§1.7）。

### 4.5 托盘 = 概览的缩略版

- 托盘图标颜色 / 角标反映概览的状态：运行中 / 有需要处理的事项 / 服务未运行。
- 菜单项：每个 Agent 的"复制 Base URL"、"打开"；"需要你处理"的条目；打开概览；退出。
- **概览、托盘、`tb status` CLI 共用同一个状态 API**，一个数据源，三个表面。

### 4.6 路由是契约，不是字符串

- 前端维护一份 route registry（路径 + 旧路径别名 + 显示名），用它生成：前端路由表、legacy 重定向、⌘K 索引，并**导出给 Wails 壳（托盘菜单）和 CLI**（`tb open claude_code` → 正确 URL）。托盘菜单最好由前端/服务端下发，而不是在 `systray.go` 里写死。
- 加一个测试：遍历所有 legacy 路径和托盘 / CLI 使用的路径，断言都能解析到真实页面。§1.6 里 `/agent/claude-code` 那个 bug 就是这个测试第一天该抓到的东西。

### 4.7 UI 偏好存到服务端

主题、语言、隐藏的 Agent、已关闭的提示、sidebar 折叠等从 `localStorage` 迁到服务端配置（按用户）。这样浏览器与桌面、多台设备、团队成员各自的视图都一致。`localStorage` 只留纯粹的本机临时状态。

### 4.8 按宿主给下一步动作（P11）

版本更新对话框：浏览器 / CLI 安装用户给 `npm` / docker 命令；桌面用户给"下载并重启"。Server Status 里的"重启服务"在两种宿主下的含义也不同，文案要按宿主说真实会发生什么。

---

## 5. 迁移策略：谨慎地换过去

### 5.1 原则

- **分层，不叠加。** 一个版本只改一层：词汇、导航、页面内交互、视觉，任何两层不在同一个版本里一起变。用户每次只需要重新学一件事。
- **每一步都可回退。** 导航减负逐项上线，每项单独可撤；改动最大的页面内交互（P4）放在 flag 后面，新旧双轨运行一段时间，出问题一键切回。
- **对外契约零变化。** endpoint、key、Auto Config 写出的文件、CLI 命令都不动。
- **旧 URL 永远有效。** 书签、README、GitHub issue 里的链接、别人博客里的截图路径，都靠 route registry + 测试来保证。

### 5.2 阶段

| 阶段 | 内容 | 风险 | 退出条件 |
|---|---|---|---|
| **P0 词汇** | 拆开 Credential 碰撞、统一 Team Keys、统一"可选能力"用词、Remote 系列命名；en/zh/ru 同步；改名处加"原 X"提示 | 低 | i18n key 无一词多义；截图对比通过 |
| **P1 路由契约 + Host Bridge** | route registry、legacy 路径测试、修托盘路由；bridge 收敛 bindings 与 `protocol.ts` 分支；UI 偏好上服务端 | 低（用户不可见） | 托盘 / CLI / 前端共享路由；浏览器与桌面偏好一致 |
| **P2 Dashboard 概览（纯增量）** | Dashboard 新增概览子页；新安装用户以它为落地页，老用户落地页不变，只是 Dashboard 里多一个子页 | 低 | 老用户行为不变；新用户首次请求成功率可观测（本地统计） |
| **P3 导航减负** | §3.2 表格中的各项，逐项上线：底部杂项合并、单页不弹 sidebar、时间范围改为页内过滤、profile 标签、窄窗口自动折叠、角标替代浮动 `!`；⌘K 作为增量 | 低-中 | 每项独立可回退；rail 顺序与图标不变 |
| **P4 页面内交互** | Agent 页三分区、规则紧凑列表、插件作用域继承、Dashboard 时间范围改为页内过滤 | 中 | 逐页上线，每页独立可回退 |
| **P5 视觉（可选）** | 在现有主题系统（Light / Dark / Sunlit / Claude / DeepSeek）上统一 token、密度、字号 | 低-中 | 与 WebView 三引擎的截图对比通过 |
| **P6 Wails v3 壳打磨** | 可与 P3–P5 并行：bridge 的 Wails 实现、托盘菜单改为下发、窗口尺寸与标题栏、三端 WebView 兼容清单（§4.3） | 中 | OAuth、导出、外链、托盘在三平台真实走通 |

### 5.3 验证与沟通

- **截图基线**：扩展现有 `ui-preview` / `docs-screenshots.mjs`，每个阶段在 mock 模式下产出 light/dark × 1440 / 1200（WebView 默认）/ 960（最小）三个宽度的前后对比。
- **文档同步**：README 和 `docs/images/` 的截图在 P3 完成时统一更新；README 里的路径描述走 route registry 的显示名。
- **提前征求意见**：P4（页面内交互，改动最大的一步）之前在 GitHub Discussions 发 RFC（本文 + 截图），开源用户能在变化落地前说话。
- **应用内一次性说明**：挪了位置的东西（如 Dashboard 时间范围），首次遇到时给一次就地提示，可关闭（P10）。

### 5.4 明确不做

- 不改任何 endpoint 路径、key 格式、写到用户机器上的配置文件格式。
- 不在同一个版本里同时改导航和页面内交互。
- 不改 rail + sidebar 两列结构，不重排 rail 顺序。
- 不删掉路由图、Probe、各类 Guide——它们换位置，不消失。
- 不把 GUI 做成与 Web 不同的产品：没有"只有桌面才有"的页面，只有"桌面上多一些宿主能力"。

---

## 6. 需要决策的问题

1. **概览面向谁？** 个人开发者（"我的 Claude Code 正常吗"）还是团队管理员（"团队用量和成员"）？决定概览默认模块的顺序，可按是否启用 Team 自适应。
2. **顶层名词**：继续用 "Agent" 指客户端接入点，还是换成更中性的 "Endpoint / 接入"？前者对现有用户零成本，后者更准确但 Remote Control 里的碰撞仍需另行解决。
3. **老用户的落地页**要不要也从 `/agent` 换到 Dashboard 概览？建议先只对新安装生效，观察后再定。
4. ~~WebView 库选型~~：已定为 Wails v3。
5. **是否要做视觉刷新（P5）**，还是这次只动信息架构与交互？
6. **是否有使用数据**（哪些页面最常用、哪些从未打开）？自托管产品通常没有，若没有，建议 P2 起加一个仅本地、可关闭的页面访问计数，用于后续判断，而不是凭感觉删功能。

---

## 7. 下一步建议

1. 先对 §6 的 1–3 做决定（它们决定词汇和概览）。
2. 用 mock 模式做一个可点击的 `ui_v2` 原型（Dashboard 概览 + 减负后的 sidebar + 一个 Agent 页），在 1200×800 窗口尺寸下评审。
3. 并行启动 P0（词汇）和 P1（路由契约 + Host Bridge）——这两步用户几乎无感，却是后面所有步骤安全落地的前提。
