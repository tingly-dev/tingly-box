# UX 梳理 — 前端"杂乱"的审计、讨论与决定

> 状态：**讨论中**。"已确定"一节里的三项（Star 入口、Dashboard 头部、Agent 页 v4）已定方案并有对应 PR；其余是待定或已否决的记录，不是实现计划。
> 来源：对 mock 模式下 6 个页面的截图审计（1440×900，实验功能全开，2026-10-11）和随后的讨论。草图（线框、方案对比）保存在设计画布里，本文只记录结论与理由。
> 约定：现在处于草图阶段，**得到明确同意之前不改代码**。相关文档：[ux-principles.md](./ux-principles.md)（判断标准）、[agent-page-redesign.md](./agent-page-redesign.md)（Agent 页的既有重设计）、[ui-redesign.md](./ui-redesign.md)。

---

## 0. 结论先行

1. "杂乱"来自四处，而不是只来自侧栏：**页面内部、侧栏 / 一级导航、概念命名、整体视觉**。只改侧栏解决不了。
2. **Agent 与模型是级联关系，不是配对。** 各 Agent 本来就独立配置，分开配不是问题；需要的是模型与服务能独立管理，这一点 Credentials / VModel 已基本做到。
3. **已确定一项改动**：Star 入口合并为一个，见 §1。
4. 其余 40 个审计问题等待逐项确认（§3）；Profile 放哪里、侧栏分组等方案尚未定（§5）。

---

## 1. 已确定（未开工）

**Star 入口合并为一个。** 去掉页面最顶的"Star on GitHub"横条，也去掉侧栏左下角（页脚"Your Intelligence, Orchestrated."旁）的星标；在一级图标栏的 Help 下方放一个常驻的 Star 按钮，一个按钮搞定。

涉及位置（备查，改动前以代码为准）：

- `frontend/src/layout/GitHubStarBanner.tsx`：顶部横条
- `frontend/src/layout/Sidebar.tsx`：页脚里的 Star 链接
- `frontend/src/layout/ActivityBar.tsx`：一级图标栏

依据：[ux-principles.md](./ux-principles.md) §9（降低视觉噪声）。同一个动作有两处入口，其中一处还占整行，且与当前任务无关。

**Dashboard 头部与 Tab：采用方案 2**（解决已确认的 D1–D4）。

- 4 个筛选（Agent / Provider / Model / Identity）保留，和时间段、Auto、刷新一起放进一条白色工具栏：筛选在左，范围与刷新在右，不折行。
- 时间段改成下拉（Today / Yesterday / 3 days / 7 days / 30 days / 90 days），不再像 Tab。
- 视图 Tab（Summary / By Request / Activity）收进内容面板顶部，左对齐下划线样式；整页只剩"侧栏 + 面板 Tab"两层。
- 5 张指标卡去掉装饰图标；只有 Cache Hit Rate 和 Error Rate 保留状态点，颜色沿用现有健康阈值。
- **不做"自定义范围"**（现状没有，需要后端和路由支持，未确认）。
- D5（Response Performance 行距）、D6（二级侧栏三项）未确认，不动。
- 实现方式：独立分支（`claude/dashboard-toolbar`，从 main 切出，不依赖本文档所在分支）。**合并顺序：本文档（ux-review）先合并，Dashboard 实现随后。**

**Agent 页 v4**（已定方案并已实现，见 PR [#2031](https://github.com/tingly-dev/tingly-box/pull/2031)）。判断依据：先问用户来这一页想知道什么——接上了吗、请求怎么走、要改哪里——而不是把后端字段平铺。

- 路由图原样保留（节点、T0/T1/T2 向下排列、每个 tier 行末常驻的"+ Add model"都不动；Quick Start 的聚光灯依赖这个节点）。
- 规则标题行统一（Smart 与 Direct 相同）：规则名、`1M`、`Endpoint`（都常驻，偏离默认值高亮）、有插件才显示的 `Plugins N`、只有图标的 Test、"⋯"菜单。常驻右侧的 Plugins 面板改为浮层。
- 状态行：已接入 · 最近请求 · 今日次数 · 错误率，加 Requests / Usage / Log 三个按钮；Log 替代原来的 Troubleshoot（后者本来就只是日志查看器）；配置过期时在这一行给"重新应用"。
- 接入卡：展开态保持原版；折叠时保留展开态第一行（Base URL 标签 + URL），折叠箭头在右上角，与规则卡一致。页面级 Plugins 单独成卡。
- 工具栏：Test All、Connect AI、New Rule、"⋯"。
- 实现时确认的事实：Endpoint 是规则级 flag（`openai_endpoint_override`），所有规则都接受，Anthropic / Google 上游会忽略它，所以常驻显示，没有 OpenAI 兼容上游时用提示说明；观测摘要用现有的 usage 记录与统计接口，没有新增后端接口；Image API、Profile、Team 页没有状态行，它们的"⋯"菜单仍保留 Troubleshoot。
- 还没处理：页面级 Plugins 卡目前所有 Agent 页都显示（没有统一的"这页有没有页面级 Plugins"信号）；"⋯"菜单里的 Edit flag 未改名。
- 不做：把接入区做成多种新摆法（5 种备选均被否决，原版只需要修折叠态）、共享路由层、Smart 条件规则分 tier。

**Star 入口合并**（PR [#2032](https://github.com/tingly-dev/tingly-box/pull/2032)）已实现：顶部横条与侧栏页脚星标移除，图标栏 Help 下方加一个常驻 Star。

Dashboard 头部方案 2 已实现（PR [#2030](https://github.com/tingly-dev/tingly-box/pull/2030)）。以上三个实现 PR 互相独立，**都应在本文档之后合并**。

---

## 2. 事实更正（避免后续讨论建立在错误数字上）

- 一级入口在界面上叫 **"Agent"**，不是 "Home"；代码里它的 key 是 `scenario`，文案取自 `layout.nav.home`。
- 默认可见的 Agent 侧栏约 **12 行**，不是 17 行：`custom`、`pi`、`cursor` 默认隐藏（`pages/scenario/scenarioRegistry.tsx` 的 `DEFAULT_HIDDEN`）。
- 侧栏已有"眼睛"图标的编辑模式，可手动隐藏 / 显示 Agent；因此"自动判断常用并折叠"与现有机制重叠，且会让条目莫名消失，已撤回。
- 一级图标栏在实验功能全开时是 11 项 + Help；默认用户看到约 7 项（Prompt / Tools / Guardrails / Bench / Desk 受实验开关控制）。

---

## 3. 审计：6 个页面的 40 个问题

类别：页面内部 / 导航 / 命名 / 视觉。编号对应画布里的标注图（A1–F6）。**这些是观察，不是都要改；是否算问题、先改哪些，由负责人逐项确认。**

### 跨页面共性

1. 几乎每页都有 4–7 个同权重的按钮 / 控件并排。
2. 同一件事在不同地方叫不同名字：Config、Auto Config、Reapply、Settings、Profile Overrides（与 [agent-page-redesign.md](./agent-page-redesign.md) §2.2 一致）。
3. 二级侧栏经常只有 2–3 项（Credential 2 项、Dashboard 3 项），一整列只放几行。
4. 图标栏标签与页面标题不一致："Agent"，"Credential" 对 "Credentials"。

### A · Claude Code 页（默认配置）

| # | 类别 | 观察 |
|---|---|---|
| A1 | 视觉 | 页面最顶常驻"Star on GitHub"横条（→ 已确定，见 §1） |
| A2 | 命名 | "Config out of date (1 difference)"、"Reapply"、"Auto Config" 三种叫法，状态在左、动作在最右，隔得远 |
| A3 | 命名 | "Status"一行其实是三个跳转（Requests / Usage / Quick Start） |
| A4 | 页面内部 | 同一张卡里混着要复制的值、开关和跳转，没有重点 |
| A5 | 页面内部 | Local \| Docker 只作用于 Base URL，却飘在最右，与 API Key 行图标错位 |
| A6 | 页面内部 | "Model Rules"右侧 3 个同权重描边按钮 + 2 个无文字图标 |
| A7 | 页面内部 | 规则标题行一行塞 8 个元素，⚡ 和 ⚙ 无文字 |
| A8 | 页面内部 | 展开 Plugins 后右侧固定占约 240px，路由图被截断（与 [agent-page-redesign.md](./agent-page-redesign.md) §2.4 一致） |
| A9 | 导航 | Profile 与 Agent 同级、同图标，像并列的 Agent；Profile 行两行高，其余单行 |
| A10 | 导航 | 侧栏底部"Anthropic SDK"被页脚切掉一半，没有"还有更多"提示 |
| A11 | 命名 | 图标栏"Credential"与页标题"Credentials"不一致 |

### B · Claude Code 的 Profile 页

| # | 类别 | 观察 |
|---|---|---|
| B1 | 命名 | 侧栏叫"ds"，页标题叫"Claude Code"，Profile 名只在小字里 |
| B2 | 页面内部 | "Start \| Settings"带 NPX \| Global，下一行 Base URL 又带 Local \| Docker：两组二选一上下叠着 |
| B3 | 页面内部 | 默认页与 Profile 页结构不同：同一个 Agent 两种页面 |
| B4 | 页面内部 | Profile Overrides 大部分是说明文字，操作只有一个"Add" |
| B5 | 页面内部 | Model Rules 标题行同 A6 |
| B6 | 导航 | 4 个 Claude Code 行都叫"Claude Code · pN"，选中的名字与页标题对不上 |

### C · Credentials 页

| # | 类别 | 观察 |
|---|---|---|
| C1 | 页面内部 | 表格 8 列 |
| C2 | 页面内部 | Name 被截断，而 Provider 列重复同一信息 |
| C3 | 页面内部 | Quota 一列里混着 3 种格式 |
| C4 | 页面内部 | 每行 Actions 是"编辑 \| Models \| ⋮"三层并排 |
| C5 | 页面内部 | OAuth 与 API Keys 两张表列不同，页面一屏装不下 |
| C6 | 命名 | 一级叫"Credential"，二级第一项又叫"Credentials"，同名重复；二级这一项其实是已连接的 AI Provider（OAuth + API Key，按钮也是"Connect AI"），不该叫 credentials。（原先误记为"侧栏只有两项"，已更正为命名问题） |

### D · Dashboard（Usage）

| # | 类别 | 观察 |
|---|---|---|
| D1 | 页面内部 | **✅ 已确认。** 顶部 4 个筛选下拉 + 时间段 + Auto 开关 + 刷新，共 7 个控件 |
| D2 | 视觉 | **✅ 已确认。** 时间段 Tab 折行，"90 days"掉到第二行 |
| D3 | 视觉 | **✅ 已确认。** 5 张指标卡右上角各有一个不承载信息的图标 |
| D4 | 页面内部 | **✅ 已确认。** 页内另有居中的"Summary / By Request / Activity"，与侧栏、时间段 Tab 三层并存 |
| D5 | 视觉 | Response Performance 行距很大，信息密度低 |
| D6 | 导航 | 二级侧栏三项 |

### E · System 页

| # | 类别 | 观察 |
|---|---|---|
| E1 | 视觉 | Theme 6 个选项，下面还有一行 8 色"Palette preview"，像可点击但只是预览 |
| E2 | 页面内部 | General 一页堆 4 张卡，没有锚点导航 |
| E3 | 导航 | Access Control 在 System 下，与它保护的 Credential 分开 |
| E4 | 导航 | Experimental / Develop / Logs 等低频项与 General 同级 |

### F · Remote · Bots 页

| # | 类别 | 观察 |
|---|---|---|
| F1 | 页面内部 | 7 张大平台卡片横排当过滤器，WeCom 被截断 |
| F2 | 页面内部 | 每行 Actions：1 个文字链接 + 3 个无标签图标，删除红色常驻 |
| F3 | 视觉 | Capabilities 的蓝色实心标签像按钮；关闭后变白底描边，同样像按钮 |
| F4 | 视觉 | Status 列同时有开关和 On/Off 徽标，信息重复 |
| F5 | 页面内部 | Platform 徽标与上面的平台卡片过滤重复 |
| F6 | 导航 | 二级侧栏用分隔线分 3 组，总共只有 4 项 |

### 处理状态（截至目前）

| 状态 | 审计项 | 说明 |
|---|---|---|
| 已解决 | A1 | Star 横条与侧栏星标合并成图标栏里的一个 Star（[#2032](https://github.com/tingly-dev/tingly-box/pull/2032)） |
| 已解决 | A3、A7、A8 | 去掉"Status"行（Requests/Usage 进状态行，另加 Log）；规则标题行统一；常驻的 Plugins 面板改成浮层，路由图不再被挤截断（[#2031](https://github.com/tingly-dev/tingly-box/pull/2031)） |
| 部分解决 | A2、A4、A6 | 状态与"重新应用"同在状态行，但 Auto Config 仍在标题栏；Plugins 单独成卡，其余仍在接入卡里；规则区工具栏收成 Test All / Connect AI / New Rule / "⋯"，仍有 3 个按钮 |
| 已解决 | D1–D4 | Dashboard 单条工具栏、时间段下拉、指标卡去装饰图标、视图 Tab 收进面板（[#2030](https://github.com/tingly-dev/tingly-box/pull/2030)） |
| 未处理 | A5、A9–A11、B1–B6、C1–C6、D5、D6、E1–E4、F1–F6 | Profile 页与侧栏、Credentials 表格与命名（C6）、System、Bots 都还没有方案或实现 |

---

## 4. 已否决 / 撤回

| 方案 | 结论与理由 |
|---|---|
| 共享路由层（入口 → 映射 → 出口，路由方案被多个 Agent 引用） | **否决。** 同时用很多 Agent 的人很少，各自分开配更直观；共享层要改规则存储（见 [rule-storage.md](./rule-storage.md)），为少数场景多一层概念，不划算 |
| 自动判断"常用"并折叠侧栏 | **撤回。** 现有编辑模式已能手动隐藏；自动判断会让条目莫名消失 |
| Profile 缩进跟随 Agent（A）、单独分一类（B） | 评审后均不满意 |
| 12 种侧栏草稿合集 | 没有满意的；说明问题不只在侧栏 |

---

## 5. 待定

| 项 | 现状 |
|---|---|
| 审计的 40 个问题 | 逐项确认中。**已确认是问题并已定方案：** D1–D4（见 §1，方案 2）（Dashboard 顶部 7 个控件、时间段 Tab 折行、指标卡无意义图标、页内三层 Tab）；C6 重新归类为命名问题。其余等待标记"最想先改 / 不算问题" |
| Profile 放哪里 | C（页面顶部"配置：xxx ▾"切换器，侧栏不放 Profile）已画草图、尚未表态。成立的前提是默认页与 Profile 页能合并（B3），需先读代码评估改动量 |
| 侧栏加分区标题、折叠 SDK | 收益小于预想（默认可点条目 12 → 9），暂挂起 |
| Credentials 服务"被谁引用"只读反查；Dashboard 按 Agent / 按服务切换 | 提过未细聊；需要后端配合（按规则反查引用；用量按服务聚合） |
| Credential 二级入口改名 | 已定方向：二级不叫 credentials。候选"AI Provider"，具体名称待定；同时要对齐一级标签、页标题（"Credentials"）和 i18n（[ux-principles.md](./ux-principles.md) §3：一词一义） |
| 页面归位（Logs → Dashboard，Access Control → Credentials）、"实验室"入口 | 暂缓 |

---

## 6. 方法

- 每次先在草图上讨论，得到明确同意后才改代码；不在草图阶段提交任何改动。
- 审计用 `.claude/skills/ui-preview`（Playwright + Chromium，mock 模式）截图；mock 的首次提示气泡（coach mark）会遮挡页面，截图前需点掉。
- 新的改动按 [ux-principles.md](./ux-principles.md) 的 12 条 checklist 过一遍。
