# 选择项的「自定义…」约定

> 用户原话：「所有的选择选项卡都应该额外添加一个自定义输入的选项，来以便于调整。」
> 落实为：**凡是后端接受任意值的数值 / 时长 / 个数类选择，预设列表末尾必有「自定义…」**；值有硬约束的不加，并说明原因。

## 组件（`@tingly/ui`）

- `SelectWithCustom<V>`：`options`（预设）+ `custom`（`label`、`parse`、`toInput`、`format`、弹层文案）。列表末尾是分隔线 + 「自定义…」；选中后打开 `CustomValueDialog`，**取消不改任何值**；确认走 `custom.parse`，其结果原样交给 `onChange`。值不在预设里（已保存的自定义值）时，列表里多出一项选中的自定义值（`custom.format`），输入框不会显得空白，用户看得到自己设了什么。
- `CustomValueDialog`：小弹层。自动聚焦，Enter 确认、Esc / 取消放弃；校验错误在第一次编辑后才显示；可带单位后缀（min / 行）。弹层而不是行内展开：Select 菜单点选后会收起，弹层在 composer 的窄条、表单里都表现一致，键盘与读屏（Dialog 的焦点管理、`aria-labelledby`）由 MUI 保证。
- 不新增持久化字段：存的是宿主原本的类型。数值类直接存数值（窗口秒数、行数）；已有字符串字段的枚举把参数编进值里（自动回答：`delay:<分钟>`）。`parse` 要**归一化**：键入的值等于某预设时返回预设的值（键入 5 分钟存 `delay`）。
- 样式沿用 DS：`SelectWithCustom` 透传 `Select` 的 props（`variant`、`sx`、`MenuProps`、`renderValue`…），composer 条里的静默样式（`composerPickerSx`）照用；传 `label` 时包成带浮动标签的表单字段。

## 已落地

| 位置 | 值 | 自定义范围 | 回写 |
|---|---|---|---|
| Projects 输入框条「自动回答」（`AutoAnswerSelect`） | 等待时长 | 整数分钟 1–60（须早于常驻进程 65 分钟的空闲回收，否则回答时进程已被收掉） | `PUT /threads/:id/auto-answer`，`auto_answer = "delay:<分钟>"`；后端 `parseAutoAnswer` 校验并按它计时 |
| Limits 编辑器「用量窗口」（`LimitsEditor`） | 窗口 | 整数分钟 1–43200（30 天） | `limits.window_seconds`（秒，原字段） |
| 系统日志 / 请求日志「最近 N 条」（`LogLimitSelect`） | 加载行数 | 1–1000（接口上限） | 组件 state → `limit` 查询参数 |

模型选择（`ModelPicker`、设置里的 `Autocomplete freeSolo`）本来就能输入自定义模型 ID；图片尺寸已有 `__custom__` 项；Limits 的数量类字段本来就是数字输入框。

## 不适合自定义（保持固定选项）

- **推理强度**（`EffortPicker`）：后端 `enum:",low,medium,high,max"` 且 `ValidEffort` 拒绝其他值；`minimal` / `xhigh` 在多数模型上会悄悄折叠成 `low` / `high`（见 `projects/internal/project/effort.go`）。任意输入只会得到静默降级。
- **权限模式**：服务端给出的闭集（`/threads/permission-modes`），自定义值会被拒或更糟——放宽权限。
- 其余闭集：主题、语言、协议（anthropic / openai）、工作区模式（共享 / worktree）、视图开关（详略、列表模式、线程视图）、Tab、状态过滤、Local/Docker、匹配方式（前缀 / 包含 / 精确）、正则 / 子串、Bot 平台、认证方式、Overflow（排队 / 拒绝）、Codex 认证模式、Notify 级别——它们是枚举，不是「调整量」。
- 实体选择（Provider、Profile、模型别名、Team、Notify 目标）：值必须是已存在的实体，选项来自后端列表。
- 时区（`SmartRuleCatalogDialog`）：IANA 闭集，已是 Autocomplete。

## 候选（未做）

仪表盘 / 用量 / 配额的时间范围（`DashboardFilterBar`、`UserUsagePage`、`QuotaHistoryPage`）：API 接受任意 `start_time` / `end_time`，适合「自定义 N 天」，但需要从天数推导聚合粒度（分钟 / 天）与各页的预设联动，单独做更稳。

## 新增选择项时

1. 后端接受任意值吗？是 → 用 `SelectWithCustom`；否 → 固定选项，并在此文档的「不适合」里记一笔。
2. `parse` 里做范围校验和归一化，错误文案说明范围。
3. 不为「自定义」加新字段；能编进现有字段就编，否则存原类型。
4. 三种语言（en / zh / ru）都补文案。
