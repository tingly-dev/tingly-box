# Workspace layout: regions, and one way to switch modes

> 状态：**提案，等待决定**（尚未实施；没有代码改动）。范围只到“整块”：页面分几块、卡片放哪、模式怎么切；
> 不含配色、间距、文案等细节。桌面为主，与手机壳（[mobile-ui.md](./mobile-ui.md)，TEP-2558）对齐。
> 衔接：[projects-placement.md](./projects-placement.md)（布局矩阵的来历）、[projects-thread-rail.md](./projects-thread-rail.md)（线程信息栏）、
> [workspace-home.md](./workspace-home.md)（工作区首页）、[visual-system.md](./visual-system.md)（卡片、Tab 的视觉规则）。
> 图片：`pr-assets/workspace-layout/`（`raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/...`，随 PR 一起推送）。
> 设计工具：任务建议用 `npx -y tingly-design` 辅助出图；本次该命令被运行环境拒绝（自动审批策略），所以下面的示意图是手写 HTML 渲染的，不是 tingly-design 的输出。

## 1. 问题

工作区（Workspace）现在有 5 类页面：首页、项目列表、项目页、文件夹页、IM 绑定页。项目页最乱：同一份信息有多个容器，切换用了四种控件，
页面本身还能被改成最多 9 种摆法（其中几格被禁用或被改写）。（截图：1440×900，`pnpm dev:mock`，`?mockData` 默认数据。）

![当前项目页](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/P1-problem-project.png)
![顶栏放置 + 线程在主区](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/P2-problem-band.png)

### 1.1 屏幕与模式清单

| 屏幕 | 区域 | 模式切换 |
|---|---|---|
| 首页 `/workspace` | 标题 + 三个同构段落（项目、工作目录、绑定），每段 3 张卡 | 无；每段有“全部 →” |
| 项目列表 `/projects` | 标题 + 3 列卡片网格 | 下划线 Tab：我的项目 / 已归档；搜索（≥6 个才出现） |
| 项目页 `/projects/:id` | 头部 + 主区（协调者对话，或线程）+ 右侧栏（Overview，或线程）；线程自带 288px 信息栏 | ① 头部“布局”菜单（Overview 3 处 × 线程 3 处 + 1 个开关）② 头部侧栏开关 ③ 下划线 Tab：线程 / 文件库 / 状态 ④ 图标分段：列表 / 竖看板 / 横看板 ⑤ 线程内：详细 / 简洁、信息栏显隐 |
| 项目设置（对话框） | 4 个 Tab：通用 / 记忆 / 环境 / 用量 | 下划线 Tab |
| 文件夹页 `/projects/folder` | 头部 + 主区（新会话或会话）+ 会话列表（右栏第一张卡） | 无布局选项；与项目页是两套结构 |
| IM 绑定 `/projects/im` | 标题 + 一张卡一个机器人 | 无 |

### 1.2 问题

1. **同一份信息多个容器。** 线程列表在对话里（线程卡片）、右栏（Overview）、顶栏（band）、浮窗（Overview 窗口）里各有一份；文件库与状态在 Tab、窗口、浮窗里各有一份。
2. **四种切换控件，互不相通。** 下划线 Tab（线程 / 文件库 / 状态）里再套图标分段（列表 / 看板），页面之上再有“布局”菜单，线程里又有自己的分段和显隐开关。用户记不住“要换成什么样，该去哪个控件”。
3. **布局矩阵藏在无文字图标里。** Overview {侧栏, 窗口, 顶栏} × 线程 {主区, 侧栏, 浮窗}，加“从 Overview 点开时浮窗”；顶栏时线程的两个选项被禁用，菜单底部才说明原因。九格里真正有意义的只有少数几种。
4. **线程一打开，全局就变。** 线程在主区时替换协调者对话（← 返回），在侧栏时替换整个 Overview；想一边看全部线程一边和协调者说话，要先改布局矩阵。
5. **线程信息栏和 Overview 的“状态”Tab 是同一组卡片的两份实现**（状态 / 用量 / 配置 / 议题与 PR），读数相同；用量还在项目设置里有第三份。
6. **窄栏装宽内容。** 看板被塞进 360px 的侧栏，竖看板与横看板看不出区别；文件库的树和预览也挤在这条窄栏里。
7. **项目页与文件夹页是两套结构**：文件夹页已经是“主区 + 一条右栏（会话列表是第一张卡）”，项目页还是“主区 + 可换内容的侧栏”。
8. **首页与列表页重复**：同一组项目卡，一个截断成 3 张，一个全量；首页没有“现在要我做什么”，而这是手机壳首页的第一件事。

## 2. 原则（本方案遵守的）

- **一个区域 = 一件事。** 主区只放“当前在聊的对话”；右栏只放“这个对话的上下文”；整页视图（看板、文件库）是页面级的，不挤进右栏。
- **一种切换模型。** 页面级视图用一条 Tab 带；右栏里的内容用折叠卡片（沿用已有的 `RailCard` / Disclosure，折叠状态按浏览器记忆）；不再有第二个、第三个切换控件。
- **同一份信息一个家。** 线程列表：右栏卡片（紧凑）+ 看板 Tab（全貌），两处，不再有顶栏、窗口等第三个容器；对话里的线程卡片是“这条消息里发生了什么”的引用，保留。
- **保留卡片嵌套。**（用户偏好，见 visual-system §5）本方案不去掉任何外层卡片，只换卡片放的位置。
- **手机与桌面同名词。** 手机壳是 对话 / 线程 / 文件库 三段；桌面 Tab 用同样的三个词。
- **桌面不做“摆放”选项。** 摆放是少数高级用户的偏好，菜单代价由所有人承担。需要并排时用“浮窗”这一个显式动作（线程头部已有）。

## 3. 方案 A（推荐）：页面 Tab + 一条上下文栏

![A1 对话](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/A1-conversation.png)

**区域**：头部（身份 + 3 个动作）→ 一条 Tab 带 `对话 · 看板 · 文件库` → 页面主体。

- **对话 Tab**：主区是对话，右栏是“对话的上下文”。
  - 焦点是协调者：右栏卡片 = 线程（紧凑列表，等你处理在最前；一行一个，行内就是 #2544 的卡片内审批）、协调者（状态）、议题与 PR、用量。
  - 焦点是某个线程：主区顶部是一条面包屑分段 `协调者 | 线程名 ✕`，在两者之间一键切换，不再有“替换对话”的 ←；右栏换成该线程的卡片（状态 / 环境 / 议题与 PR / 用量 / 改动），第一张是折叠的“其他线程”，展开就是上一状态的线程列表。这与文件夹页现有做法一致（“会话”是信息栏的第一张卡）。
  ![A2 线程焦点](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/A2-thread-focus.png)
- **看板 Tab**：整页宽度的看板。列 / 行方向是看板自己的一个小分段（只此一处）；列表视图就是对话 Tab 右栏里的“线程”卡，不再作为第三种视图。点卡片：切到对话 Tab 并聚焦该线程。
  ![A3 看板](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/A3-board-tab.png)
- **文件库 Tab**：整页宽度的树 + 预览（现在挤在 360px 栏里）。
  ![A4 文件库](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/A4-library-tab.png)
- **头部**：名称 · 目标 · IM 标签 · ★ · ⚙。去掉“布局”菜单和侧栏开关；右栏用它自己的折叠把手（对话 Tab 里）。
- **设置对话框**：通用 / 记忆 / 环境。**用量**从设置里移到右栏的“用量”卡（读数，不是配置）。“状态” Tab 取消，并入协调者卡。
- **浮窗**：只保留“把线程浮起为窗口”这一个显式动作（并排看两个线程）。Overview 窗口、顶栏、`overviewFloat` 取消。
- **窄屏（< lg）**：右栏变抽屉（现状如此）；Tab 带不变。手机壳另行实现，Tab 即其分段。

**文件夹页**用同一个壳：头部（名称 + 路径）+ Tab `对话 · 看板`（没有文件库）+ 主区 + 右栏（“会话”卡为第一张）。

**收益**：切换只有一种；信息一处一个家；看板和文件库拿到整页宽度；线程列表不会因为打开线程而消失；项目页与文件夹页同构；手机上直接对应。
**代价**：删除用户已经建好的布局能力（顶栏、窗口、侧栏线程）——有人在用的话是退步；看板与对话之间要切 Tab，不能同屏。

## 4. 方案 B：只修剪，保留现在的两栏

![B1 修剪](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/B1-prune.png)

保持“主区 + 右栏 Overview”的结构，只收敛规则：

- 去掉“布局”矩阵：Overview 固定在右栏；线程固定在主区（`协调者 | 线程` 面包屑，无 ←）；保留“浮起为窗口”一个动作。
- 右栏 Tab 保持 线程 / 文件库 / 状态；视图开关收成 `列表 | 看板`（看板的列 / 行是看板内一个很小的选项）。
- 状态 Tab 与线程信息栏共用同一组卡片组件；用量只在这两处出现，设置里的“用量”去掉。
- 文件夹页不变。

**收益**：改动最小（主要是删代码，没有新页面结构）；不用重做看板与文件库的容器。
**代价**：窄栏装宽内容的问题（#6）没解决；项目页与文件夹页仍是两套；手机的 对话 / 线程 / 文件库 三段与桌面不对应。

## 5. 工作区其他页面（两个方案共用）

![H1 首页](https://raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/workspace-layout/H1-workspace-home.png)

- **首页以“需要你处理”开头**（跨项目与文件夹的待审批 / 待回答，行内就能允许 / 不允许 / 回答，复用 #2544 的线程卡片审批）；其后是项目、工作目录两段（各最多 3 张卡，“全部 →”），绑定收成一行摘要（“管理 ›”）。这与手机壳首页的顺序一致（需要你 → 可以 review / 进行中 → 项目 / 工作目录）。
- **项目列表页**保留（全量 + 搜索 + 已归档），已归档由 Tab 改为列表底部的一个折叠段（Disclosure），页面不再有 Tab。
- **IM 绑定页**不动；首页只留入口。

## 6. 推荐与分步

推荐 **方案 A**：它让“该去哪个控件”只有一个答案，并且让看板和文件库有了合适的宽度；B 是风险更小的退路。无论选哪个，下面的步骤都可以单独合并（每步一个 PR，带截图）：

| 步 | 内容 | 看得出的变化 |
|---|---|---|
| 1 | 项目页：线程在主区用面包屑切换，取消 ←；去掉“布局”矩阵与 `overviewFloat`，保留“浮起为窗口” | 头部少两个图标；线程不再替换整栏 |
| 2 | 线程信息栏与“状态”并成一组卡片；用量只留一处 | 状态 / 用量不再重复 |
| 3（仅 A）| Tab 带 `对话 · 看板 · 文件库`；看板与文件库整页；线程列表成右栏卡片 | 项目页换壳 |
| 4（仅 A）| 文件夹页并入同一个壳 | 文件夹页与项目页同构 |
| 5 | 首页“需要你处理”；项目列表的已归档折叠段 | 首页第一屏有事可做 |

## 7. 需要决定

1. **A 还是 B？**（A 删掉顶栏 / 窗口 / 侧栏线程三种摆法，是否有人依赖？）
2. 浮窗：只留“线程浮起”一个动作可以吗？Overview 窗口是否可以去掉？
3. 看板 Tab 点卡片：切到对话 Tab 聚焦线程（推荐），还是在看板上叠一个侧滑面板？
4. 设置里的“用量”移出到右栏，可以吗？项目级用量详情是否还需要一个“更多 ›”入口？
5. 首页“需要你处理”的行内操作范围：只做允许 / 不允许 / 回答，其余仍进线程？
6. 已有的“布局”偏好（localStorage 里的 `overviewPlacement` / `threadPlacement`）在升级时直接忽略即可吗？

## 8. 复现

```bash
cd frontend && pnpm gen:api && pnpm exec vite --mode mock --port <port> --strictPort
# 截图：python playwright，Chrome 用 ~/.cache/ms-playwright/chromium-1243，TMPDIR 取短路径，
# localStorage 种 layout.activeActivity=projects 与各 tip 的 dismissed；项目页 /projects/p-latency（?thread=t-cache）。
# 布局切换用 localStorage：tingly-project.overviewPlacement / threadPlacement / overview.view。
```
