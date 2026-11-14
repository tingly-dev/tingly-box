# Workspace layout: regions, and one way to switch modes

> 状态：**提案，等待决定**（尚未实施；没有代码改动）。范围只到“整块”：页面分几块、卡片放哪、模式怎么切；
> 不含配色、间距、文案等细节。桌面为主，与手机壳（[mobile-ui.md](./mobile-ui.md)，TEP-2558）对齐。
> 衔接：[projects-placement.md](./projects-placement.md)（布局矩阵的来历）、[projects-thread-rail.md](./projects-thread-rail.md)（线程信息栏）、
> [workspace-home.md](./workspace-home.md)（工作区首页）、[visual-system.md](./visual-system.md)（卡片、Tab 的视觉规则）。
> 图片：`pr-assets/workspace-layout/`（`raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/...`，随 PR 一起推送）。
> 设计工具：任务建议用 `npx -y tingly-design` 辅助出图；本次该命令被运行环境拒绝（自动审批策略），所以示意图是手写 HTML 渲染的，不是 tingly-design 的输出。
>
> **修订（2026-11-14）**：用户指出问题的根源不是“摆放方式太多”——左右分屏、原地替换、线程放右边或顶上，这些灵活性是看板的优点——
> 而是**同一份信息的卡片在不同位置里排列和交互不一致**：有时有侧栏有时没有，有时有状态有时没有，状态有时在 Tab 里。
> 所以推荐方案改为 **方案 C：保留全部摆放，统一内容与交互**（第 3 节）；最初的“收窄成 Tab”（方案 A）与“修剪”（方案 B）降为备选（第 4 节）。
> **首页部分已批准，另一线程实施**（第 5 节）；项目页 / 看板部分仍在讨论，**不实施、不开 PR**。

## 1. 问题

工作区（Workspace）有 5 类页面：首页、项目列表、项目页、文件夹页、IM 绑定页。项目页最复杂。（截图：1440×900，`pnpm dev:mock`，默认数据。）

![当前项目页](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/P1-problem-project.png)
![顶栏放置 + 线程在主区](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/P2-problem-band.png)

### 1.1 屏幕与模式清单

| 屏幕 | 区域 | 模式切换 |
|---|---|---|
| 首页 `/workspace` | 标题 + 三个同构段落（项目、工作目录、绑定），每段 3 张卡 | 无；每段有“全部 →” |
| 项目列表 `/projects` | 标题 + 3 列卡片网格 | 下划线 Tab：我的项目 / 已归档；搜索（≥6 个才出现） |
| 项目页 `/projects/:id` | 头部 + 主区（协调者对话，或线程）+ 右侧栏（Overview，或线程）；线程自带 288px 信息栏 | ① 头部“布局”菜单（Overview 3 处 × 线程 3 处 + 1 个开关）② 头部侧栏开关 ③ 下划线 Tab：线程 / 文件库 / 状态 ④ 图标分段：列表 / 竖看板 / 横看板 ⑤ 线程内：详细 / 简洁、信息栏显隐 |
| 项目设置（对话框） | 4 个 Tab：通用 / 记忆 / 环境 / 用量 | 下划线 Tab |
| 文件夹页 `/projects/folder` | 头部 + 主区（新会话或会话）+ 会话列表（有会话时并入信息栏的第一张卡） | 无布局选项；与项目页是两套结构 |
| IM 绑定 `/projects/im` | 标题 + 一张卡一个机器人 | 无 |

### 1.2 真正的问题：同一信息，在不同位置里长得不一样

摆放有很多种本身没问题；问题是下面这些“随位置变样”的地方（均已在代码里核实）：

| 信息 | 它在哪些位置怎么变 |
|---|---|
| **线程的状态 / 环境 / 用量 / 改动** | 只有线程面板宽度 ≥ 880px 且用户没关时，才是线程右边的一列卡片（`RAIL_MIN_PANEL = 880`，`useInfoRail`）。1440 屏上 Overview 在侧栏时，主区约 700px，**线程在主区也看不到这一列**；线程在侧栏（360px）或浮窗（520px）时永远没有，只剩头部的几个小标签和弹层。同一份“状态”，有没有取决于窗口宽度和摆放。显隐按钮也只在宽的时候才出现。 |
| **协调者的状态** | 只在 Overview 的“状态”Tab 里（侧栏 / 窗口）；Overview 在顶栏时变成一个按钮、点开又是一个窗口；线程占了侧栏时 Overview 整个被换走，要点 ← 才看得到。 |
| **文件库** | Overview 的 Tab（侧栏 / 窗口）；顶栏时是一个按钮 → 另开一个窗口。 |
| **线程列表** | 项目页：Overview 的 Tab；顶栏：分列的 band；文件夹页：没有会话时是侧栏，有会话时变成信息栏的第一张卡。 |
| **头部的侧栏按钮** | 含义随位置变：侧栏时显隐侧栏，窗口时开关窗口，顶栏时折叠 band；文件夹页有会话时按钮直接消失。 |
| **“浮起”按钮** | 线程头部有，但 Overview 在顶栏时没有。 |
| **“布局”矩阵** | 3 × 3 里有格子被禁用（顶栏时线程不能侧栏 / 浮窗），“从 Overview 点开时浮窗”是第 4 个开关；说明藏在菜单底部。图标没有文字。 |
| **用量** | 线程信息栏、Overview 的“状态”Tab、项目设置的“用量”Tab 三份。 |
| **项目页 vs 文件夹页** | 同一个“主区 + 右栏”的想法，两套实现，规则不同。 |

另外两点是体验层面的：看板被放进 360px 的窄栏（竖 / 横两种几乎看不出区别）；首页与列表页重复，而首页没有“现在要我做什么”。

## 2. 原则

- **摆放可以多，规则只能一套。** 保留 Overview {停靠, 顶栏, 窗口} × 线程 {主区, 侧栏, 浮窗}；统一的是“放进哪个容器，内容和控件都一样”。
- **一份信息，一个组件，多个容器。** 容器只决定位置和尺寸，不决定里面有什么、怎么操作。
- **宽度只决定形态，不决定有没有。** 放得下就常驻，放不下就弹出同一份内容；不会因为窗口窄了就“没有状态”。
- **同一个按钮永远是同一个意思。**
- **保留卡片嵌套。**（用户偏好，见 visual-system §5）本方案不去掉任何外层卡片，只换卡片放的位置。
- **手机与桌面同名词。** 手机壳是 对话 / 线程 / 文件库；桌面面板的 Tab 用同样的词。

## 3. 方案 C（推荐）：保留全部摆放，统一内容与交互

### 3.1 一致性契约

| # | 规则 | 解决 |
|---|---|---|
| R1 | **一个“面板”**：Tab `线程 · 文件库 · 状态` + 一个视图开关（列表 / 看板）。停靠、顶栏、窗口三种容器里内容和控件一字不差。顶栏时面板横向展开，点“文件库 / 状态”就在顶栏里换内容，不再另开窗口。 | 文件库 / 状态 / 线程列表随位置变样 |
| R2 | **面板永远在，线程打开不再替换它。** 线程放在“侧栏”位时，排在主区与面板之间（主区 \| 线程 \| 面板）；放不下时面板折成一条窄条（Tab 图标 + 需要你的红点）。 | 打开线程后 Overview 消失、要点 ← |
| R3 | **状态跟随焦点。** 面板的“状态”Tab 显示焦点的那一组卡片：焦点是协调者 → 协调者的卡；焦点是线程 → 该线程的卡。焦点在主区顶部的面包屑 `协调者 \| 线程名 ✕` 里切换。同一套卡片组件（现在线程信息栏和“状态”Tab 是两份实现）。 | 状态有时有、有时没有、有时在 Tab 里 |
| R4 | **线程头部一个“详情”按钮。** 宽度够 → 常驻为线程右边的一列；不够 → 在按钮下面弹出同一组卡片（浮窗里也一样）。它与面板的“状态”Tab 是同一组卡片的两个入口，不是两份内容。 | 线程详情依赖宽度才出现 |
| R5 | **摆放 3 × 3 全部合法，没有被禁用的格子。** 因为面板永远在（R2），Overview 的摆放与线程的摆放完全正交；“从 Overview 点开时浮窗”并入“线程在浮窗”。“布局”按钮带文字（`摆放`），两组各三选一。 | 布局矩阵里的禁用 / 例外 |
| R6 | **头部“面板”按钮含义恒定**：显示 / 隐藏面板，不论它在哪个容器。**线程头部“浮起”按钮恒在**（顶栏时也在）。 | 同一个按钮随位置变意思 |
| R7 | **文件夹页用同一个面板**：Tab `会话 · 状态`（没有文件库），三种容器同样可选；不再有“有会话时列表变成信息栏第一张卡”的特例。 | 项目页与文件夹页两套规则 |
| R8 | **用量只在一处**：面板的“状态”Tab（协调者 / 线程各自的用量卡）；项目设置里不再有“用量”Tab，项目级汇总放进协调者的状态卡，带“详情 ›”。 | 用量三份 |

### 3.2 示意

**一个面板，三个容器**（R1）：

![C1 一个面板三个容器](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/C1-one-panel-three-containers.png)

**同一份详情，三种宽度**（R4）：

![C2 同一份详情三种宽度](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/C2-one-detail-three-widths.png)

**状态跟随焦点**（R3；线程在主区、面板停靠在右并停在“状态”Tab）：

![C3 状态跟随焦点](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/C3-status-follows-focus.png)

### 3.3 收益与代价

**收益**：用户已经习惯的所有摆法都保留；任何位置下“状态 / 文件库 / 线程列表”的位置和长相可预期；实现上是**合并**（一个 Panel 组件、一组状态卡片、一个线程详情入口），代码更少。
**代价**：“侧栏线程 + 停靠面板”是三列，窄屏要靠面板折叠条；`overviewFloat` 偏好被并入线程浮窗；设置里的“用量”Tab 去掉；R2 改变了“线程占用侧栏”的老行为，需要确认。

## 4. 备选：A（收窄成 Tab）与 B（只修剪）

在用户澄清“灵活性是优点”之前提出，保留作为参照；只有当以后决定**减少摆法**时才相关。

- **A**：页面级 Tab `对话 · 看板 · 文件库` + 一条上下文栏，去掉停靠 / 顶栏 / 窗口 / 侧栏线程的摆放，只留“浮起线程”。看板和文件库整页宽度；与手机壳一一对应；代价是删掉已有的布局能力。
  ![A1](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/A1-conversation.png)
  ![A2](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/A2-thread-focus.png)
  ![A3](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/A3-board-tab.png)
  ![A4](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/A4-library-tab.png)
- **B**：只删“布局矩阵”，Overview 固定右栏、线程固定主区，保留一个“浮起”。改动最小，但窄栏装宽内容、两页两套结构、手机不对应的问题都还在。
  ![B1](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/B1-prune.png)

C 与 A 并不互斥：C 的面板在“停靠 + 看板”下仍然偏窄——可以在 C 之上再给看板加一个“整页展开”的动作（见第 7 节问题 3），那是 A 的看板 Tab 的轻量版。

## 5. 工作区首页（已批准，另一线程实施）

![H1 首页](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/H1-workspace-home.png)

- **首页以“需要你处理”开头**（跨项目与文件夹的待审批 / 待回答，行内就能允许 / 不允许 / 回答，复用 #2544 的线程卡片审批）；其后是项目、工作目录两段（各最多 3 张卡，“全部 →”），绑定收成一行摘要（“管理 ›”）。与手机壳首页的顺序一致（需要你 → 可以 review / 进行中 → 项目 / 工作目录）。
- **项目列表页**保留（全量 + 搜索 + 已归档），已归档由 Tab 改为列表底部的折叠段（Disclosure）。
- **IM 绑定页**不动；首页只留入口。
- 用户同时要求把**通知开关移到最上面并重新设计展示与交互，前端通知默认开启**——这属于同一线程的工作，不在本文范围。

## 6. 分步（方案 C；每步一个可单独合并的 PR，带截图）

| 步 | 内容 | 看得出的变化 |
|---|---|---|
| 1 | 抽出统一的 `Panel`（Tab + 视图开关），三种容器都用它；顶栏里的文件库 / 状态就地换内容，不再另开窗口 | 顶栏模式不再有两个多余窗口；三种容器长得一样 |
| 2 | 状态卡片合成一组、跟随焦点；线程头部“详情”按钮（常驻一列或弹出）；用量只留一处 | 窄宽度下线程也有详情；状态不再两份 |
| 3 | 线程打开不再替换面板；摆放 3 × 3 全开放，“摆放”按钮带文字；`overviewFloat` 并入线程浮窗；面板折叠条 | 去掉 ←；布局菜单没有灰格 |
| 4 | “面板”与“浮起”按钮含义统一 | 头部按钮行为一致 |
| 5 | 文件夹页用同一个 Panel（`会话 · 状态`） | 项目页与文件夹页同构 |
| — | 首页（需要你处理等）：另一线程 | — |

## 7. 需要决定

1. **方案 C 作为主线，可以吗？**（A / B 只在以后想减少摆法时再用。）
2. **R2**：线程放在“侧栏”位时，排在面板的**左边**（主区 | 线程 | 面板，窄时面板折成条），而不是替换面板——可以接受吗？还是更想保持“线程占用侧栏”、只把面板的入口改成恒在的折叠条？
3. 看板在停靠面板里偏窄：是否加一个“整页展开”动作（点一下看板铺满主区，再点还原），不改变摆放？
4. **R4/R3**：线程详情与面板“状态”Tab 是同一组卡片的两个入口——还是只保留一个入口更干净（比如只保留面板的“状态”Tab，线程头部“详情”按钮只是跳到它）？
5. **R8**：项目设置里去掉“用量”Tab，项目级汇总放进协调者状态卡，可以吗？
6. 已存的摆放偏好（`overviewPlacement` / `threadPlacement` / `overviewFloat`）：沿用，`overviewFloat=1` 的用户迁移为线程浮窗，可以吗？

## 8. 复现

```bash
cd frontend && pnpm gen:api && pnpm exec vite --mode mock --port <port> --strictPort
# 截图：python playwright，Chrome 用 ~/.cache/ms-playwright/chromium-1243，TMPDIR 取短路径，
# localStorage 种 layout.activeActivity=projects 与各 tip 的 dismissed；项目页 /projects/p-latency（?thread=t-cache）。
# 摆放用 localStorage：tingly-project.overviewPlacement / threadPlacement / overviewFloat / overview.view。
```
