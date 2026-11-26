# Team usage 右侧栏（用量最高的服务商 / 账户）评估

> 评估 + 原型截图，供用户决定；本轮未改生产代码，原型改动已全部回退。
> 截图在 project library：`.../library/outputs/tb-sign/`（下称 `<shots>/`）。
> 截图环境：mock 模式（`USE_MOCK=true vite --mode mock`）、隔离 HOME、`?mockData=populated`、中文、1440×1000 @2x、By Provider 视图（与用户截图一致）。

## 结论（先看这个）

**这个侧栏作为独立一列没有必要。** 它把相邻表格里**同一批行、同一顺序**再列一遍，唯一多出来的信息是「占比 (%)」，而这个数可以由行值 / 合计直接算出来。它还占掉约 25% 宽度，把表格列挤到横向滚动；窄屏（< lg）时它整块堆到表格下方，成了一个全宽的重复块；没有用量时它变成一张很高的空白卡。

推荐 **方案 B：去掉右侧两列排名卡，把「占比」做成两张表的列**。信息零丢失，表格恢复全宽，列不再被截断。若一定要保留「排名」观感，可选 A（保留并统一），但它只是视觉统一，不解决重复和挤压。

## 1. 侧栏在哪、数据从哪来

Team usage 页 = `/dashboard/users`（nav「团队用量 / Team usage」）。

页面是「roster 表 + 选中项明细表」两段，每段右侧各挂一张排名卡：

| 位置 | 文件:行 | 卡片 |
|---|---|---|
| 上段（roster）右栏 | `frontend/src/pages/userUsage/UserUsagePage.tsx:402-440`（表格 `lg:9` @403，右栏 `lg:3 xl:2.4` @424，`RosterTopList` @425-438） | 「用量最高的账户/模型/服务商」，随切换轴变化 |
| 下段（明细）右栏 | `frontend/src/pages/userUsage/RosterDetailSection.tsx:62-184`（表格 `lg: showDetailTop?9:12` @69，右栏 @150，`RosterTopList` @151-158） | 选中项的反向明细排名（如某 Provider 下的账户） |
| 卡片组件 | `frontend/src/components/dashboard/RosterTopList.tsx` | 排名列表：header `minHeight:72` @152、行 `py:0.875` @90、空态 `minHeight:200` @164、`computeShare` @35-54 |

数据完全来自表格的同一份数据：

- 上段 `shareItems` 由 `rows | modelRoster | providerRoster` 映射（`UserUsagePage.tsx:138-158`），就是表格的数据源。
- 下段 `detailShareItems` 由 `activeAxis.detail` 映射（`UserUsagePage.tsx:169-182` + `RosterDetailSection` 的 `items={activeAxis.detail}`），就是明细表的数据源。
- 表格**默认按「总计」降序**（`frontend/src/components/dashboard/RosterAxis.tsx:135-136`，`useState<SortField>('total')` + `'desc'`）。所以排名卡 = 相邻表格的前 6 行，顺序一模一样。
- `computeShare` 只过滤 token>0、降序、取 top 6、尾部聚合成「Others」（`RosterTopList.tsx:35-54`）；唯一新增的量是 `share = tokens/合计`。

## 2. 冗余性分析

- 排名行的 subject + 总计：**100% 重复**相邻表格（同源、同默认顺序）。
- 占比 %：**唯一独有**，但可由 `行总计 / 合计` 得到，且下一行表格里就有分子分母。
- 交互：排名卡「点击选中该行」——表格行本来就可点击选中（`RosterTable.tsx:92-106`），且默认已按总计排序，排名卡没有提供表格没有的能力。
- 别处是否已展示：
  - Dashboard 只有「按模型用量」表（`ServiceStatsTable.tsx`，`DashboardPage.tsx:289`），**没有** provider/account 排名。
  - 统计卡只做总量聚合，不排名。
  - 筛选项列出 provider/model/账户，但不排名。
  - 所以 provider/account 的**总计**只在 Team 的两张表里；**占比%**只在侧栏。=> 把占比放进表格列，就不缺任何信息。

## 3. 全站「右侧栏 / 卡片列 / 排名列表」变体

| 变体 | 文件:行 | 标题/头 | 列表项 | 交互 | 空态 | 响应式 |
|---|---|---|---|---|---|---|
| Team usage 排名卡 `RosterTopList` | `components/dashboard/RosterTopList.tsx:134-181`；挂载见上表 | header `minHeight:72`、`p:2.5`、`h6 fontSizes.base`（@151-162） | 行 `py:0.875 px:1.25`，圆角 hover，序号+名称+占比%，选中 inset 3px 左条（@86-131） | 可点击选中 | 居中文字 + 提示，`minHeight:200`（@163-173） | Grid `lg:3 / xl:2.4`；<lg 整块堆到表下方（见 `team-rail-current-narrow.png`） |
| Dashboard 右卡 `PerformanceSummary` | `components/dashboard/PerformanceSummary.tsx:94-117`；列见 `DashboardPage.tsx:266-287` | header `px:2.25 pt:2 pb:0.75`，标题内联 + loading spinner（@96） | p99..p10 五行，每行 3 个指标 grid（@100-115） | 无（纯展示） | 值显示「—」 | ≥1280px 时固定 `400px` 右列，窄屏堆叠（@271） |
| Projects 信息栏 `RailCard` | `frontend/packages/projects/src/components/project/infoRail.tsx:46-79`；`ThreadInfoRail.tsx` 使用 | **可折叠** DisclosureRow header，标题 `fontSizes.sm`（@60-79） | `Fact` 标签/值行，`py:0.25`（@165-175） | 折叠记忆（localStorage `thread.rail.<id>`）、复制值 | 各自处理 | 面板宽 ≥ `RAIL_MIN_PANEL=880` 才停靠，固定 `RAIL_WIDTH=288`；窄则 sheet / 无（@16-17,46-57） |
| （参考）表格卡 `RosterTable` | `pages/userUsage/RosterTable.tsx:29-37,113-138` | 头 `minHeight:64`、`p:2.5`，搜索框右对齐 | 标准表格行 `py:1.25` | 行选中、排序、分页 | 居中文字 `py:8`（@257-276） | 随 Grid |
| （参考）表格卡 `ServiceStatsTable` | `components/dashboard/ServiceStatsTable.tsx:79-104,218-233` | 头 `p:2.5` + `h6 fontSizes.base`（无 minHeight） | 标准表格行 `py:1.25` | 分页 | 带图标的空态 | 全宽 |
| （参考）明细卡 header | `RosterDetailSection.tsx:72-92` | 头 `minHeight:72` + 计数 Chip | — | — | `RosterAxis.tsx:233-240` 用 `action.hover` 盒子空态 | 随 Grid |

**风格不统一的具体点：**

1. 三张卡 header 高度/内边距各不同：Team 表 `minHeight:64`、明细卡 `72`、Dashboard 指标卡 `px:2.25/pt:2`、Projects `fontSizes.sm` 折叠头。
2. 空态三种写法：表格居中 `py:8`、排名卡 `minHeight:200`、明细表 `action.hover` 盒子——同一页三套。
3. 排名卡没有折叠，Projects 卡可折叠；Dashboard 卡标题与 loading 同行。
4. 断点三套：Team `lg/xl` 网格、Dashboard `1280px` 固定 400、Projects `880px` 面板阈值 + 用户开关。

**可复用件现状：** `@tingly/ui` 有 `Surface`（通用描边卡，无 header）和 `EmptyState`（通用空态）；`StatCard` 是统计卡。**没有**「带标题栏的 RailCard」通用件——三条侧栏各自手写 header 与空态。（Projects 的 `RailCard` 在 `@tingly/projects` 包里，是唯一带折叠头的实现，但跨包，不适合根应用直接复用。）

## 4. 方案（均基于同一 mock 数据、真实截图）

### A) 保留但统一

- 做法：在 `@tingly/ui` 抽一个 `RailCard`（标准 header + body + 空态），Team / Dashboard / Projects 三处改用同一套：统一 header 高度与内边距、行高、排序/占比呈现、空态（用 `EmptyState`）、断点。
- 截图：`<shots>/team-rail-option-unified.png`（原型里把排名卡改成与表格同族的「迷你表」：header 64 + 计数 Chip，行加分隔线、行高对齐表格）。
- 信息：不丢失。布局：仍是 3/12 右列 → 表格依旧被挤（明细表列被截断）；仍是重复。代价：跨 3 处，约 1–2 天（若只统一 Team 两张卡约 2–3 小时），需逐屏回归。可达性：行已是 button 语义，保持。空态：统一到 `EmptyState`。
- 评价：只解决「风格几种」，不解决「重复 + 挤压」。

### B) 简化：占比内联为表格列（推荐）

- 做法：去掉两处右栏 Grid，两张表改全宽；在 roster 表和明细表各加一列「占比」（行总计 / 合计）。
- 截图：`<shots>/team-rail-option-inline.png`（占比 73.4/23.7/2.9 与 60.2/20.0/9.3/6.0/4.4，与侧栏数值完全一致）。
- 信息：零丢失（占比成为普通列，仍是同值）。布局：表格全宽，输出/推理/错误率等列不再被截断；<lg 不再出现全宽重复块。代价：约 5 个文件、2–4 小时（加列 ×2；删右栏 Grid ×2；移除 `RosterTopList` + 测试 + import）。可达性：占比是普通表列（表头+值），比「可点击排名行」对读屏更友好。空态：复用表格空态，占比列显示「—」。
- 评价：**首选**。

### C) 移除

- 做法：去掉两处右栏 Grid，表格改全宽，不加占比列。
- 截图：`<shots>/team-rail-option-removed.png`。
- 信息：只少了「占比%」（看「总计」列 + 默认降序即可看出排名）。布局：同 B 但不加列。代价：约 1 小时（删右栏 + 删 `RosterTopList` 及测试）。
- 评价：最省事；比 B 略少一点信息。

### D) 仅保留第一段排名卡（其他建议）

- 做法：保留上段 roster 排名卡，去掉下段明细右侧排名卡，明细表改全宽。
- 截图：`<shots>/team-rail-option-top-only.png`（明细表列不再截断，上段仍被挤、仍重复）。
- 信息：少了明细占比，保留上段占比。布局：只解掉被挤得最狠的明细表。代价：约 1 小时。
- 评价：半measures，只算过渡。

## 5. 推荐与落地顺序

**推荐 B。** 这个侧栏不必要：同源同序重复、默认排序已等价、唯一独有的占比可内联、且它带来 25% 宽度损失与窄屏重复块。

若改走「统一」而非删除，建议顺序（每一步可独立 PR，先小后大）：

1. `@tingly/ui` 新增 `RailCard`/`CardHeader` + 统一用 `EmptyState`；先把**表格卡 header 统一**（Team 表 64 / 明细 72 / ServiceStats 无 minHeight / Dashboard 2.25 → 一套），这是最扎眼的差异。
2. Team 排名卡改用该件（`team-rail-option-unified.png` 的效果），统一空态与断点。
3. Dashboard `PerformanceSummary` 改用该件（标题/内边距/断点对齐）。
4. Projects `RailCard` 对齐同一套 header 字号与间距。
5. 最后再评估是否把 Team 排名卡也内联掉（即回到 B）。

（我更建议直接做 B，再把 1 的 `EmptyState`/`CardHeader` 统一作为独立的视觉收口；A 的 2–4 步只是「保留重复」的前提下美化。）

## 6. 截图清单（`<shots>/`）

- `team-rail-current.png` — 现状（Provider 视图：右上「用量最高的服务商」+ 右下「用量最高的账户」）。
- `team-rail-current-narrow.png` — 1024px 窄屏：排名卡整块堆到表下方，成为全宽重复块。
- `team-rail-empty-state.png` — 无用量（newcomer）：右侧变成很高的空白卡。
- `team-rail-option-unified.png` — A。
- `team-rail-option-inline.png` — B（占比列）。
- `team-rail-option-removed.png` — C。
- `team-rail-option-top-only.png` — D。
- `dashboard-rail-current.png` — 对照：Dashboard「响应性能」右卡，另一种卡片风格（说明「风格好几种」）。
