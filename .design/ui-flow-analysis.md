# UI 动线分析 — 发现、人工打标反馈与实现结论

> 状态：**第 1 轮已落地**（2026-10-09）。本文是一份**滚动记录**，不是一次性方案：每一轮按同样的四段追加——
> ①动线与发现（分析）→ ②人工打标反馈（产品负责人对每条提议的取舍）→ ③实现结论（做了什么、没做什么、为什么）→ ④验证。
> 这样日后回头看，每个改动都能追溯到"当时看到了什么、谁怎么判的、最后怎么落的"。
>
> 判断标准沿用 `ux-principles.md` 的 P1–P12。与 `ui-redesign.md`（导航骨架）、`agent-page-redesign.md`（Agent 页模板）互补：那两份讲**页面和导航长什么样**，这份讲**用户沿着它们走一遍，哪里断了**。
>
> 方法与局限：本轮分析来自代码走读（路由表、`useActivityItems`、`Layout`、`AgentPage`/`AgentSetupCard`、Credentials、Help、Dashboard），不是用户访谈或埋点；视觉判断只在实现后用 mock 模式截图核对过。后续轮次若有真实使用数据，应优先于这里的推断。

---

## 1. 动线清单

| # | 动线 | 当前路径 |
|---|---|---|
| J1 | 首次接入 | 登录 → `/agent` → 上次访问的 Agent（默认 Claude Code）→ 页内 Quick Start：连 Provider → 选模型 → 安装 → 应用 |
| J2 | 接入第二个工具 | 在 Agent sidebar 的 13 个入口里找 → 重复 Quick Start |
| J3 | 日常观测"正常吗" | rail › Dashboard › Usage（今日） |
| J4 | 排障"请求为什么失败" | Agent 页规则栏的 Troubleshoot / Test all；Dashboard › By Request；System › Logs |
| J5 | 凭据维护（额度、OAuth 过期） | Credentials 页；Dashboard › Quota history |
| J6 | 团队共享 | Team rail › Team Keys |
| J7 | 远程控制 | Remote rail › Bots › Remote Control / IM Notify / Desk |
| J8 | 发现扩展功能 | 用户菜单 › Power-ups；或 System › Experimental |

## 2. 第 1 轮：发现

按影响排序。证据均为 2026-10-09 时的代码位置。

| 编号 | 动线 | 发现 | 证据 |
|---|---|---|---|
| F1 | J1 | Quick Start 里连完 Provider 后**整页刷新**，滚动位置、展开状态、spotlight 全丢（违反 P10 / P12）。根因：Quick Start 自己取一份 provider 列表，与规则区不共享，只好 reload 对齐 | `AgentPage.tsx` 的 `onProviderAdded: () => window.location.reload()` |
| F2 | J1 | 默认落在 Claude Code，sidebar 里没有"先选你在用的那个"的线索 | `lastAgent.ts` |
| F3 | J1 / J2 | Quick Start 进度按 Agent 分别记，第 1 步（连 Provider）是全局状态却在每个 Agent 页都占位 | `AgentSetupCard` 的 `setup-card-*-{agentKey}` |
| F4 | J1 | Help 页注释自称"onboarding 前门"，但没有任何路径送用户过去；内容与 Credentials / Agent 页内引导重复（P8） | `HelpPage.tsx`、`appRoutes.tsx`、`OnboardingGate.tsx` 三处注释互相矛盾 |
| F5 | J4 | "这个请求去了哪、为什么失败"分散在三处，没有一条完整的路；请求旅程（P1）只存在于 Probe 里 | Troubleshoot / Dashboard By Request / System › Logs |
| F6 | J3 | **仪表盘没有 Agent 维度**：后端 `usage/stats|timeseries|records|performance` 全部支持 `scenario` 过滤，`group_by=scenario` 也支持，但筛选栏只有 Provider / Model / Identity | `usageApi.ts` 已有 `scenario` 参数；`DashboardFilterBar` 没有 |
| F7 | J3 / J5 | 系统状态是被动的：rail 上除断线外没有任何角标；额度见底、OAuth 过期要用户自己去页面看 | `ActivityBar.tsx` |
| F8 | J2 | sidebar 不显示哪些 Agent 已接好；状态 chip 只在进入页面后才看得到 | `ClientConfigStatusChip` |
| F9 | J8 | 同一组开关有两个入口（Power-ups、System › Experimental）；"Power-ups / Experimental / Beta"三个词指同一类东西（违反 P3） | `PowerUpsMenu.tsx`、`ExperimentalPage` |
| F10 | J5 | VModel（内置的合成 Provider，用于 onboarding / 演练）与 Credentials 并列为同级页，暗示它是"用户自己的凭据"；侧栏还用缩写 "VModel" 躲截断 | `useActivityItems.tsx` |
| F11 | 全局 | `CredentialPage` 的标题、按钮、空状态、通知、对话框**全是硬编码英文**，中文用户在核心页面看到英文 | `CredentialPage.tsx` |
| F12 | 全局 | rail 标签用 `slice(0, 7) + '…'` 硬截断，不随语言和宽度变化 | `ActivityBar.tsx` |
| F13 | J6 / J2 | Team 既在 `SCENARIOS` 里（`/agent/team`）又是独立 rail 项，`lastAgentPath` 要特判 `id !== 'team'` | `lastAgent.ts` |
| F14 | J7 | "Remote"（rail）→"Remote Control"（行）→"Bots"（行）三个相邻名字，Bot 只是通道 | `useActivityItems.tsx` |

## 3. 第 1 轮：人工打标反馈

产品负责人对分析与提议的逐条取舍（2026-10-09）。**这一栏是人的判断，不是推断，以此为准。**

| 提议 | 反馈 | 结论 |
|---|---|---|
| 落地页是否改（Dashboard 概览 / 其他） | **落地不改** | 保持 `/agent`；不再讨论新增概览页 |
| 每个 Agent 页加"最近请求"（复用请求旅程，F5） | **加请求和用量小板是好的**——"因为时不时要观测" | 采纳，但**先做轻量版**：今日请求 / Token / 错误 + 最近请求，链接到仪表盘；"请求旅程"版留待后续 |
| VModel 挪出 Credentials 同级（F10） | **作为新的 Power-up，默认打开** | 采纳 |
| 仪表盘是否缺 Agent 区分（F6，分析中提出的问号） | **本质上少了 Agent 区分？**——认可这个判断 | 采纳：给仪表盘加 Agent 筛选 |
| 小板的形态（实现后复看） | **小板放上页面后看不到模型信息了，太差；板面被大幅污染，不如弹窗** | **采纳，已改**：常驻卡片 → 头部按钮 + 弹窗。见 §4 的修正提交 |
| 范围 | **暂时只改提及的这一小部分，避免动作太大**；这几项独立，可以拆成多个分支做 | 只做 F1、F4、F6、F10、F11、F12 和小板；其余顺延。实现上每项独立提交，便于以后各自摘成分支 |
| 本文档 | 动线文档单独记录，写明变化分析、人工打标反馈和最终实现结论 | 即本文 |

未被点名的提议（F2、F3、F7、F8、F9、F13、F14）**没有被否决**，只是本轮没有排期。

## 4. 第 1 轮：实现结论

每项一个独立提交（顺序即提交顺序）。

| 提交 | 对应 | 做了什么 | 设计取舍 |
|---|---|---|---|
| `fix(agent): refresh providers in place after Connect AI` | F1 | 去掉 `window.location.reload()`。`AgentSetupCard` 不再自己取 provider，改由 `AgentPage` 传入页面已有的列表；规则栏的 Connect AI 与 Quick Start 的 Connect AI 共用同一份刷新 | 根因是"两份 provider 状态"，所以修的是状态归属，而不是换个刷新方式。连完后第 1 步就地打勾，第 2 步成为当前步，用户停在原处（P10） |
| `chore(ui): let CSS ellipsize rail labels; fix stale onboarding comments` | F12、F4（注释部分） | rail 标签去掉硬截断，交给已有的 CSS ellipsis，并加 `title`；校正 `appRoutes` / `HelpPage` 的过期注释 | **Help 页本身没动**（内容重复的问题 F4 留到后续），只让注释不再误导 |
| `fix(i18n): translate the Credentials page (en/zh/ru)` | F11 | 新增 `credentialPage.*` 命名空间；复用已有的 `layout.credentials`、`templateActions.connectAI`；刷新失败对话框用 `<Trans>` 保留加粗的 Provider 名 | 三套语言同步加（`localeParity` 测试要求）；复数用 i18next 的 `_one/_other`（ru 四档） |
| `feat(power-ups): make VModel a power-up, on by default` | F10 | Power-ups 菜单新增 "Virtual Models" 一行；复用隐藏集合（`'vmodel'` 不在集合里 = 开）。关闭后只隐藏 Credentials 侧栏那一行 | **默认开不需要迁移**：隐藏集合里没有该 id 即为开，老用户无感。关闭后 `/credentials/virtual-models` 仍可直达，rail 仍高亮 Credentials。只剩一个子页时 Credentials 不再弹 sidebar（既有规则） |
| `feat(dashboard): filter usage by agent, with ?scenario= deep link` | F6 | 筛选栏新增 Agent 下拉，贯穿 stats / 时序 / 请求 / 性能 / 热力图。选项 = 当前时间范围内**有用量的**Agent；支持 `?scenario=` 深链接 | 选项单独取（只按时间范围），不从已筛选的 stats 推——否则选中后列表缩成一项（与 Provider 选项同一个坑）。深链指向的 Agent 即使当天没量也保留在选项里，避免下拉显示一个它不提供的值。**精确匹配**：`claude_code` 与 `claude_code:p1` 是两项，各自如实显示 |
| `feat(agent): add a Requests & usage panel to agent pages` | F5（轻量版） | 首版：新增常驻卡片 `AgentActivityCard`，放在规则栏之上 | **此形态被否决（见下一行）**。当时的判断"观测是常驻需求，所以不该随 Quick Start 折叠"本身没错，错在把"常驻需求"等同于"常驻版面" |
| `fix(agent): show Requests & usage in a dialog, not a standing card` | F5（修正） | 卡片改为 `AgentActivityDialog`：Agent 页头部多一个「Requests & usage」按钮（与 Auto Config / Setup Guide 并列），点开才看。内容不变：今日请求 / Token / 错误 + 最近 5 条请求，手动刷新，"Open in Dashboard"链到 `?scenario=`。**打开时才加载和 30 秒刷新**，关闭即停 | Agent 页的主角是模型与路由规则（P9），观测只是"时不时看一眼"（P12：副作用限定在当前表面）。常驻卡片把规则整体下推近一屏，用户看不到自己最关心的模型信息，是**版面代价大于观测收益**。弹窗把代价降为一个按钮，且不打开时零请求。直接引 `chartStyles` 而不是 `dashboard` barrel，避免把图表库带进每个 Agent 页 chunk |

### 明确没做

- 落地页、rail 角标（F7）、sidebar 状态点（F8）、Quick Start 第 1 步全局化（F3）、"不是 Claude Code？"提示（F2）、Power-ups / Experimental 合并（F9）、Remote 命名重整（F14）。
- 弹窗里的"请求旅程"展开（F5 的完整版）——弹窗先回答"有没有流量、健不健康"，"这个请求去了哪"留给下一轮。
- Help 页降级（F4 的内容部分）。

## 5. 第 1 轮：验证

- `pnpm vitest run`：62 个文件、452 个测试通过（含 `localeParity`、`tKeyCoverage`、路由契约）。
- `pnpm typecheck`：71 个错误，**与改动前完全一致**（均为既有遗留，无新增）。
- 修正后复看 mock 截图：Agent 页恢复为头部 + 连接信息 + Model Rules，三条规则整屏可见；点击按钮弹出弹窗，显示 1,842 / 25.9M / 12 错误（0.7%）与 5 条最近请求。此前（卡片形态）的截图：仪表盘 `?scenario=codex` 时 Agent 下拉显示 Codex；Credentials 页正常渲染。
- **已知的 mock 局限**：mock 的 `usage/stats` 不按 `scenario` 过滤，所以仪表盘选了某个 Agent 数字不变；这是 mock 的问题，不是实现的问题，但意味着**按 Agent 过滤的真实效果需要在真实后端上再看一遍**。
- **未验证**：Power-ups 菜单里 VModel 行的悬停展开视觉；中文 / 俄文下小板和仪表盘新增文案的排版。

## 6. 复盘：这一轮学到的

- **"值得常驻"不等于"值得占版面"。** 我把观测判成"常驻需求"，就直接做成了常驻卡片；实际用户要的是"随时能看"，不是"一直摆着"。判断一个新增区块时，应先问它会**挤掉谁**——这里被挤掉的是页面的主角（模型规则）。
- **截图验证要看"主角还在不在"，不只看新东西好不好看。** 首版截图我只核对了小板本身渲染正确，没有对比加入后 Model Rules 被推到了哪里。后续 UI 改动的验证清单应加一条：改动前后同一视口下，原有主角内容是否仍在首屏。

## 7. 追加记录的约定

下一轮请在本文件**末尾追加** `## 8. 第 2 轮：…`，沿用 发现 → 人工打标反馈 → 实现结论 → 验证 四段；已有的 F 编号不重排，新发现从 F15 起。被否决的提议也要留在反馈表里并写明理由——"为什么没做"和"做了什么"一样值得追溯。

---

## 8. 第 2 轮：Agent 页头部 —— 头卡与 Quick Start 合并

### 8.1 发现

| 编号 | 动线 | 发现 | 证据 |
|---|---|---|---|
| F15 | J1 / J2 | 头卡和 Quick Start 是同一件事（"这个工具接上网关了吗"）的两个视角，却是两张各占一块的卡；Auto Config 在两处重复 | 头卡右上按钮 + Quick Start 第 4 步按钮 |
| F16 | J1 | Quick Start 长期占位的根因：**④ 应用配置是自我申报**（只有点了卡片自己的按钮才算），页面头部明明已显示 "Applied"（从磁盘回读），卡片却永远停在 3/4；③ 安装同理 | `AgentSetupCard` 的 `applyDone` 只来自本地存储 |
| F17 | J1 | 全部完成后卡片只折叠成一条横栏，**没有"退场"状态** | `autoCollapsedRef` 只折叠不移除 |
| F18 | J3 | 常驻的头部按钮（Auto Config / Requests & usage / Open Web UI）堆在标题行，随 Agent 数量增多而挤 | 第 1 轮加按钮后更明显 |

### 8.2 人工打标反馈

| 提议 / 实现 | 反馈 | 结论 |
|---|---|---|
| 折叠连接行（原型 `73717af`）：已应用时 Base URL / Key / Plugins 折成一行 | **不是预期的，依然混乱；Quick Start 依然长期占位** | **否决，已 revert**。教训：只缩一块解决不了"两块在讲同一件事" |
| 是否可以优雅合并进头卡？按钮放侧边？ | **有可能合并吗？比如按钮在侧边？我也困惑了** | 先给静态示意图（五种状态）再动手，而不是再做一个原型 |
| 合并方案（示意图：卡内只显示当前一步 + 进度圆点，按钮在右侧竖排，接好后步骤消失） | **方向对，按这个顺序做** | 采纳 |
| 接好后 API Key / 本机·Docker / 插件是否收进下拉 | **倒不用收起来，先完整展开没问题** | 连接行保持完整展开，不做折叠 |
| 实现后复看（右侧竖排按钮的合并版） | **方向似乎对，但好丑……都溢出来，而且怎么面板强制融合了，太诡异了** | **采纳，重做版式**。见 8.3 的修正提交与 8.6 |

### 8.3 实现结论

| 提交 | 对应 | 做了什么 | 设计取舍 |
|---|---|---|---|
| `fix(agent): tick Quick Start's install and apply steps from the real config status` | F16 | 新增 `configApplied`（头部状态芯片同一信号）：配置回读为 applied 时，③ 安装、④ 应用自动完成 | 先于版面改动单独提交：它不改变任何布局，风险最低，且独立有价值（修了"已应用却不打勾"）。只对有回读能力的 Agent（Claude Code / Codex / DSH）生效，其余仍靠请求信号或手点 |
| `feat(agent): merge Quick Start into the header card, actions in a side column` | F15、F17、F18 | 首版：`AgentSetupCard` 新增 `inline` 形态（只显示当前一步 + 四个进度圆点，可点回看，全部完成后整块消失），`AgentPage` 把头卡、步骤、连接行合成一张卡，操作按钮在右侧竖排 | **版式被否决，由下一行取代**；`inline` 步骤组件与"完成后消失"逻辑保留 |
| `fix(agent): lay the merged header out on the title row, setup as a tinted inset` | F15、F17、F18（修正） | 操作按钮回到标题行右侧：主按钮随状态变化（配置未应用时 Auto Config 为主，已应用或有 Web UI 链接时让位），「请求与用量」「重新打开设置步骤」收成图标；Claude Code 的 Unified/Separate 开关仍在标题行；设置步骤变成卡内有底色的嵌入区，完成后整块消失；连接行恢复整行宽度 | 合并不等于抹掉边界：会来了又走的那一块应当有自己的容器（见 8.6）。次要动作用图标，是因为它们是"随时看一眼"而不是"需要你做" |

### 8.4 验证

- `pnpm vitest run`：452 个测试通过；`pnpm typecheck`：71 个，与改动前一致。
- mock 截图（1440×900）：Codex（配置过期、停在第 3 步安装）、DeepSeek（已应用，③④自动完成，停在第 2 步选模型）、OpenAI SDK（无步骤，Base URL / Key 完整展开）、Claude Code（模式开关完整显示）。四种情形下模型规则均在首屏。
- 截图中发现并已修：Claude Code 的模式开关在窄侧栏里被裁切、"Requests & usage" 折成两行 → 侧栏按页面类型调宽（184 / 216px）。

### 8.5 已知不足 / 没做

- **进度圆点占独立一行**，没有并进标题行（示意图里是并排的）：圆点由步骤组件持有，标题由页面持有，并行需要把进度状态提到页面。
- **Auto Config 仍出现两次**（第 4 步按钮 + 侧栏）：第 4 步是"做这件事"，侧栏是"随时再做"，语义不同，但视觉上重复；是否去掉第 4 步里那一个，留给下一轮。
- **连接行完整展开**（按反馈）；折叠不做。
- **未验证**：真实后端上 `configApplied` 的时序（回读有延迟，首屏可能先显示未完成再跳变）；中文 / 俄文下新排版；窄屏（<md）下按钮列掉到卡片下方的样子。
- Claude Code 页的路由图在 1440 宽下右侧被截断，是 `agent-page-redesign.md` §2.4 记录过的既有问题，不是本轮引入。

### 8.6 复盘：为什么首版又丑又"强行融合"

- **竖排侧栏吃掉了宽度，而连接行是为整行设计的。** 右侧按钮栏占约 200px，使 Base URL、插件下拉、安装命令在 1100 宽下全部被截断；1920 宽下内容又被 960 封顶，侧栏悬在卡片中间，竖线悬空。示意图用的是自己写的理想化 HTML，真实组件（`ConfigRow` 等）有固定列宽，**示意图没有暴露这个约束**。
- **"强行融合"是视觉分区缺失。** 步骤块和连接行是两套视觉语言，直接叠在同一个标题下，用户读成"两张卡被粘在一起"。合并不等于抹掉边界：会来了又走的那一块（设置步骤）应当有自己的容器。
- **修正**：按钮回到标题行右侧，次要的（请求与用量、重新打开设置步骤）收成图标，主按钮仍随状态变化；设置步骤变成卡内一块有底色的嵌入区，完成后整块消失；连接行恢复整行宽度，不再被挤。1100 / 1280 / 1920 宽下均无截断（Claude Code 路由图在窄宽下右侧被截断是既有问题，见 8.5）。
- **验证方法的教训**：这次溢出是**用多个窗口宽度的真实截图**才发现的，首版只在 1440 宽下看过。此后 UI 改动的验证至少要覆盖 1100 / 1280 / 1920 三个宽度。示意图只能验证方向，不能验证可行性。

### 8.7 第三次修正：不再合并成一张卡，改为 Tab + 统一的行（**已被 8.8 取代**）

**反馈**（按时间）：

| 版本 | 反馈 | 结论 |
|---|---|---|
| 标题行按钮 + 底色嵌入区的合并卡（8.6 的修正版） | **自然非常杂乱** | 否决。教训：一张卡里同时有标题、状态芯片、按钮、图标、设置区（进度、重置、帮助、步骤、命令）和三行连接信息，控件种类和字号太多；**问题不在版式细节，在"一张卡装太多东西"** |
| 我提出两个方向（撤销合并 / 保留但精简 / 你来指出） | **"快速也变成一行，但合理扩展 row 实现？几个卡用侧边或横向 tab？"** | 采纳：①Quick Start 变成连接列表里的**一行**，需要时展开；②几块内容用**横向 Tab** 分开。选横向而不是侧边：侧边会重蹈侧栏吃宽度的覆辙（8.6） |

**实现**：

| 提交 | 做了什么 | 设计取舍 |
|---|---|---|
| `feat(agent): split the agent page into tabs, setup as one row` | ①页头换成统一的 `PageHeader`（名称 + 状态 + 操作）；②三个 Tab：**连接**（Setup 行 + Base URL / API Key / Plugins 行）、**Model Rules**、**Requests & usage**；③Setup 行与下面的连接行用同一套「168px 标签 \| 内容 \| 操作」网格，标签是"Setup"加一排进度点，内容是当前一步，行尾箭头展开命令和说明（重置、帮助也收进展开区）；④「请求与用量」从弹窗变成 Tab，**只在 Tab 打开时加载和刷新** | 一屏只看一件事，没有任何东西需要挤在同一张卡里。Tab 默认值跟着 Agent 走：已经配了模型的默认进 Model Rules，否则进连接；之后记住用户的选择。Quick Start 第 2 步"选择模型"会先切到 Model Rules 再定位。三个面板都保持挂载（只是隐藏），切换不丢状态，活动面板除外 |
| 同提交内的小修 | Setup 行加 `px: 2` 与连接行对齐；标题栏的 Auto Config 在 Setup 行显示期间变成次要按钮（同一时刻只有一个主按钮）；窄于 `lg` 时操作区掉到标题下方，标题不换行 | 来自 1100 / 1440 宽的真实截图 |

**取舍说明**：
- 安装步骤在行内直接给了「复制」按钮（命令一键到剪贴板），不用先展开才能拿到命令（P11）。
- 「请求与用量」之前做过常驻卡片（挤掉规则）和弹窗（要多点一次），Tab 是第三种形态，目前看是三者里最干净的。
- 完成后 Setup 行整行消失，标题栏出现一个"设置步骤"图标可重新打开。

**已知不足**：
- Model Rules 面板自带的卡片标题和 Tab 名重复（都叫 Model Rules），没动 TemplatePage。
- OpenAI SDK 这类没有状态和按钮的页面，标题和 Tab 之间有一段空白。
- Claude Code 的 Base URL 在 1100 宽下仍会被截断（既有的 ConfigRow 列宽）。
- 新的首次访问默认 Tab 依据"是否已配模型"，没验证过存量用户的实际感受。

### 8.8 第四次修正：Quick Start 是独立的 Tab；Model Rules 不是 Tab

**反馈**：

| 版本 | 反馈 | 结论 |
|---|---|---|
| 8.7（Setup 压成连接 Tab 里的一行；Model Rules 也成了一个 Tab） | **Quick Start 应该是单独的 Tab 吧？** | 采纳。Setup 压成一行是硬塞：Quick Start 是独立的东西，该有自己的位置 |
| 同上 | **Model Rules 还是和以前一样，只是 header 变成了多卡** | 采纳。我把"几个卡用 Tab"误解成了"所有卡都进 Tab"。用户说的"几个卡"是**头部那几张卡**；Model Rules 是页面的主角（P9），不该被藏进 Tab |

**实现（取代 8.7）**：头部 = `PageHeader`（名称 + 状态 + 操作）+ **一行 Tab：Quick Start（带进度，如 2/4，完成后 ✓）、连接、请求与用量**；**Model Rules 恢复为 Tab 条之下的页面主体，始终显示**。

- Tab 条常驻成一行；点开一个 Tab 显示它的面板，**再点一次收起**。"收起"也是一种记住的选择。
- 默认值跟着 Agent：**已经配了模型 → 默认收起**（只有一行 Tab，规则占满首屏）；**还没配模型 → 默认打开 Quick Start**（没有 Quick Start 的 SDK 页则打开连接）。
- Quick Start 面板恢复为完整的四步列表（与最初的卡片一致，去掉了"折叠"和"一次一步 + 圆点"这些补丁），标题处改为显示"下一步该做什么"，因为名称和进度已经在 Tab 上。
- 连接面板回归只有 Base URL / API Key / Plugins 三行；请求与用量面板只在打开时加载和刷新。
- 步骤状态仍来自真实信号（8.3）。

**这条反馈的教训**：我两次把"几个卡"理解错（先是合并成一张，再是全进 Tab）。用户的措辞里"几个卡"是**对象**，我应该先复述"你说的几个卡是 A、B、C，对吗"再动手，而不是挑一种解读直接做。

**已知不足**：
- 已配模型的 Agent 默认收起，Quick Start 的进度只剩 Tab 上一个数字，新手之外的人可能不会注意到它还没完成（比如配置过期）。
- Claude Code 的 Unified/Separate 开关在窄宽下掉到标题下一行，与 Auto Config 同行。
- 没验证：存量用户对默认收起的感受；中文、俄文排版；真实后端上的时序。

### 8.9 第五次修正：Connection 在最前，Quick Start 完成后移到最后，不再收起

**反馈**：**"connection 应该在最前面展示，quick start 结束了就应该到后面去，也不用收起了，反正卡会占用一些"** —— 采纳，并**取代 8.8 里"默认收起"的设计**。

**实现**：
- Tab 顺序：**Connection 固定第一**；Quick Start 在有设置要做时紧跟其后（`Connection · Quick Start 3/4 · Requests & usage`），**全部完成后移到最后**（`Connection · Requests & usage · Quick Start ✓`），仍在，供回看。
- **取消"收起"**：始终有一个 Tab 展开，再点不会收起。默认打开 Connection，之后记住用户的选择。
- 旧版本存下的 `closed` 偏好被忽略，回落到 Connection。

**取舍**：
- 之前"已配模型默认收起，让规则占满首屏"的收益被放弃：用户明确接受卡片占一些空间，换来的是**行为更可预期**（不会有时有面板有时没有，也没有"点哪都没反应"的收起态）。
- Quick Start 的位置随完成状态变化，而不是固定：这样"需要你做的事"离 Connection 近，"已完成的事"退到后面，不占黄金位置。

**已知不足**：
- Tab 位置会在 Quick Start 完成的那一刻变化（从第二个变成最后一个），当前页面上有一次跳动；没有做动画。
- 默认打开 Connection 对新手不如直接打开 Quick Start 直观，但新手在 Connection 旁就能看到 "Quick Start 0/4" 的提示。

### 8.10 头部收尾

来自 8.5 / 8.7 / 8.9 的已知小问题，本次一并收掉：

| 问题 | 处理 |
|---|---|
| Claude Code 的 Unified/Separate 开关挤在标题栏，窄宽下把标题、状态芯片、按钮挤成两行 | 开关移到 **Tab 行最右侧**：它决定的是下面模型规则的布局，理应紧贴规则，而不是挂在标题旁。标题栏只剩名称 + 状态 + 操作，窄宽下也是一行 |
| 标题与 Tab 之间空白偏大（OpenAI SDK 这种没有按钮的页面尤其明显） | 去掉标题栏的底部内边距，三栏间距收紧 |
| 标题栏在 `lg`（1200）以下就把操作区掉到标题下方 | 开关移走后内容变少，断点放宽到 `md`（900） |

**没处理**：
- Claude Code 的 Base URL 在 1100 宽下仍被截断（来自共用的 `ConfigRow` 列宽，改它会影响其它页面，另议）。
- Quick Start 完成时 Tab 位置的一次跳动。
- Model Rules 卡片自带标题与页面重复（卡片仍叫 Model Rules，Tab 条之下又没有同名 Tab，现在已不算重复，可忽略）。

### 8.11 第六次修正：Tab 换成一行弹窗按钮，Connection 回到页面里

**反馈**：**"截断没问题。不过是不是可以用一个 row 把 quick start 和 requests 还有 usage 分开的放一排按钮用于触发弹窗？或者 reset（quick start）"**——采纳，**取代 8.8 / 8.9 的 Tab 方案**。

**实现**：
- 页面从上到下：页头（名称 + 状态 + Auto Config）→ **一行按钮**（Quick Start · Requests · Usage，Claude Code 的 Unified/Separate 在行尾）→ **Connection 卡**（常驻，Base URL / API Key / Plugins）→ Model Rules。没有 Tab、没有面板切换。
- 三个按钮各开各的弹窗：**Quick Start**（四步列表，带进度标签如 `3/4`，完成后 `✓`；**Reset progress 在弹窗里**）、**Requests**（最近 20 条请求）、**Usage**（今日请求 / Token / 错误）。Requests 与 Usage 从原来的"请求与用量"里**拆开**。
- Quick Start 没完成时排在行首，完成后移到行尾（沿用 8.9 的位置规则）。
- 弹窗打开时才加载、每 30 秒刷新；Quick Start 弹窗保持挂载，这样按钮上的进度在打开前就已知。Quick Start 里"Choose Model"会先关弹窗，再定位到下面的规则。

**取舍与结果**：
- 去掉 Tab 之后，Connection 不再与其它面板分时占用同一位置，**不再有"选哪个 Tab"的决定**；一行按钮只有"想看再点"。
- Connection 卡取回整行宽度，上一轮的 Claude Code Base URL 截断也随之消失（1280 宽下完整显示）。
- 代价：Connection 卡常驻，约占 150px；用户明确接受（"反正卡会占用一些"）。

**已知不足**：
- 按钮行不能一眼看出"Quick Start 还差哪一步"，只有 `3/4`。
- 点 Quick Start 里的"Connect"会在 Quick Start 弹窗上再叠一层连接弹窗（两层）。
- 没验证：中文 / 俄文下按钮行的换行、真实后端时序、"完成后移到行尾"（mock 里没有能走完四步的 Agent）。

### 8.12 第七次修正：按钮行并进头卡成为一行；有模型就自动跳过安装 / 应用

**反馈**（两条）：
- **"如果加了模型，quick start 的那些步骤就应该自动跳过"** —— 采纳。
- **"我是说像 baseurl 那样的一行，放在 header 里面"** —— 采纳，纠正 8.11：我把按钮行做成了**页面上单独的一排**，用户要的是**头卡里的一行，与 Base URL 同一种行**。

**实现**：
- **自动跳过**：规则里已经配了模型，就说明这个 Agent 在用了，**安装、应用配置两步自动完成**。行上显示"已跳过：已经配置了模型"，而不是"已安装 / 已应用"——那是对事实的声明，跳过不是。这样配过模型的 Agent 进入 Quick Start 即 4/4，按钮显示 ✓ 并移到行尾。
- **头卡里的一行**：Base URL / API Key / Plugins 之后加一行，标签 **Status**，内容是 Quick Start · Requests · Usage 三个按钮，与上面几行同样的「标签 | 内容 | 操作」列（含标签的 12px 内边距，已对齐）；Claude Code 的 Unified/Separate 开关在这一行的操作列。

**取舍**：
- 单独一排按钮看上去像"页面工具栏"，放进头卡后它就是连接信息的一部分：同一张卡回答"怎么连、状态如何"。
- "Status" 是个宽泛的词（里面有设置进度、请求、用量），暂时够用；如果以后加入更多入口，可能需要更准确的名字。

**已知不足**：
- 自动跳过的是**事实判断**：有模型 ≠ 一定装好了或应用了配置（例如 Claude Code 配置已过期但有模型），此时只靠头部的状态芯片提示。
- 没验证：Quick Start 弹窗里"已跳过"文案的实际显示、中文 / 俄文排版、真实后端时序。

### 8.13 第八次修正：Quick Start 不是弹窗，而是嵌入（完成隐藏，点击重置）

**反馈**：**"不用加进度了。而且 quick start 比较特殊，不是弹窗，而是嵌入（就是原来的位置，ok 就隐藏，点击就重置）"** —— 采纳，**取代 8.11 里"Quick Start 是弹窗"的部分**（Requests / Usage 仍是弹窗）。

**实现**：
- Quick Start **回到原来的位置**：头卡下面、模型规则上面，仍是原来那张卡（保留它自己的进度小标签与 Reset progress、帮助、折叠）。
- **完成就隐藏**（`hidden`）：四步都完成（含"已有模型 → 自动跳过"）时整张卡不显示。进度未知前，已有模型的 Agent 先按"已完成"处理，避免完成的页面先闪一下大卡片。
- **点击就重置**：卡片隐藏时，头卡 Status 行末尾出现 **Quick Start** 按钮；点它会重置进度并把卡片重新显示。按钮上**不再带进度标签**。
- 重置后的卡片会保持显示，直到这一轮再次完成；它显示期间 Status 行里不再有这个按钮（避免两个入口）。

**取舍**：
- Quick Start 是"流程"，不是"一眼"：它需要看到完整的四步和按钮，弹窗里没有页面上下文（比如 Choose Model 要指向下面的规则），所以嵌入更对。Requests / Usage 才是"看一眼"，适合弹窗。
- 按钮只在卡片隐藏时出现：卡片可见时再出一个入口是重复的。

**已知不足**：
- 已有模型的 Agent 点 Quick Start 重置后，因为"已有模型 → 自动跳过"，四步立刻又全部完成（显示 Done），卡片只能靠它自己的折叠箭头收起或刷新页面后消失，没有一个"关闭"按钮。
- 重置只清掉手动记录的进度；"已有模型 / 配置已应用"是实时事实，重置不了。
- 没验证：中文 / 俄文排版、真实后端时序。

### 8.14 Status 行与其它行统一

**反馈**：**"按钮那一行和其他行格格不入"** —— 采纳。

**原因**：这一行是我手写的网格：标签偏灰，内容是一排带边框的实心按钮；而上面 Base URL / API Key / Plugins 都是 `ConfigRow`——加粗标签、蓝色文字的值、小图标。同一张卡里出现了两种视觉语言。这与 8.6 的"强行融合"是同一类错误：**新东西没有套用同一套组件，只是在旁边摆了个长得像的**。

**修正**：这一行改为直接使用 `ConfigRow`（标签样式、列宽、间距由它统一），内容改成三个无边框的蓝色文字按钮（Requests / Usage / Quick Start），Claude Code 的 Unified / Separate 放进 `ConfigRow` 的操作列（与上面 Local | Docker 开关同位置）。

**教训**：往已有列表里加一行，先找这一列表的行组件复用，而不是复刻它的网格；复用才能保证标签、间距、字重一起对齐，以后改行样式也会一起变。

### 8.15 Model mode 单独一行；Status 用分段按钮组

**反馈**：**"claude 的分离也可以放进一行，你新加的一行可以用 button group 更加一致性美观"** —— 采纳。

**实现**：
- **Model mode 行**（仅 Claude Code）：Unified / Separate 不再挂在 Status 行的操作列，而是自己一行，与 Base URL / API Key / Plugins 同级。为此 `useSlotRouting` 不再返回现成的 `modeSwitch`，改为返回 `modeOptions` 和 `requestMode`（切换仍会先弹确认框，已验证）。
- **Status 行**：Requests / Usage / Quick Start 由文字按钮改成**分段按钮组**，直接复用 `ChoiceToggle`（就是 Base URL 后面 Local | Docker 的那个组件），不选中任何项，每项只负责"打开"。`ChoiceToggle` 新增可选的 `optionWidth`（默认不变），因为"Quick Start"比默认宽度长。
- 两行各补一点垂直内边距，行距与上面几行一致。

**取舍**：
- 8.14 复用的是"行"组件，这次复用的是"行里的控件"组件：整张卡里同一类东西（二选一 / 多选一的小开关）都长一个样。
- `ChoiceToggle` 本意是"选一个值"，这里拿来当"一排入口"用，语义略勉强：因为从不选中，不会出现选中态，但读屏会把它读成单选组。如果以后再有类似的"入口组"，应当抽一个 `ActionGroup`，而不是继续借用。

**已知不足**：
- 无障碍：入口组的语义是 toggle group，不是 button group，需要后续修正（见上）。
- 没验证：中文 / 俄文下分段按钮的宽度（100px 是按英文估的）。
