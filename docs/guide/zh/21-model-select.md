# Model Select（模型选择）

Model Select 对话框为转发规则指定目标 Provider 和模型——是配置路由规则的主要交互入口。同一个选择器也用于 Image Playground 的 *Choose another model…*、Bench 的目标，以及 [Custom](./06-scenario-special.md) 和 [Team](./07-team.md) 的规则编辑。

---

![模型选择弹窗](../images/model-select.png)

## 如何打开

在任意 Agent 的 **Model Rules** 区域：

- **点击路由图中的模型卡片**（如 `claude-sonnet-5`）——以*编辑*模式打开，标题为 **Choose Model**，当前的 Provider 和模型已经勾选
- **点击 ＋ Add model**（层末尾或智能规则末尾的虚线卡片）——给该规则再加一个模型
- **New Model**（工具栏）——*创建规则*模式：*Select a model for your new rule*；点一个模型就为它建一条规则。打开 **Select multiple** 后标题变成 *Create a rule for each model*——见 [Codex · New Model](./04-scenario-codex.md#new-model一次添加多个模型)
- **Quick Start → Choose Model**——滚动到 Model Rules 并高亮 **＋ Add model**

---

## 对话框结构

### 左侧：Provider 栏

- **Search credentials…** 按名称过滤 Provider
- Provider 按凭证类型分组——**OAUTH** 和 **API KEY**——每组带数量
- 每一行显示 Provider 的 **Logo**、名称和**协议徽标**（`O` OpenAI 风格，`A` Anthropic 风格；融合 Provider 两个都有），当前选中项所在的 Provider 旁有 ✓

### 右侧：模型

顶部工具栏：

- **Refresh models**：从 Provider 拉取最新的模型列表
- **Edit Provider**：原地编辑该 Provider（名称、URL、Key、代理），无需离开选择器
- **Search models…**：按模型名过滤。按回车会把搜索词**按 Provider、在这台设备上**记住（浏览器本地存储）；搜索框为空时，被记住的词会显示为 **Recent searches** 标签，一键复用（× 忘记）
- **Select multiple**：仅创建规则模式下有

下面是模型数量（`4 models`）和卡片。**第一张卡片永远是 `＋ Custom Model — Enter any model ID`**，用于列表里没有的模型（你添加的自定义 ID 会按 Provider 记住，可以在它们的卡片上编辑或删除）。之后依次是（有的话）**New Models**（刚出现在 Provider 列表里的，带 `NEW` 标记，可忽略）、**Recent** 和 **Starred Models** 分区，然后是全部模型。模型加载时，只有拉取较慢才会出现转圈，所以快速的拉取不会闪一下。

底部是该 Provider 的 **Provider Quota**（与[凭证管理](./08-credentials.md#quota-列)相同的圆环/`% left` 单元格），带 **Details**（上游原始响应）和刷新按钮。

### 选择

点击模型卡片即选中。*编辑*模式下写入路由规则并关闭；*添加*模式下为该规则新增一个服务；*创建规则*模式下创建规则（或者在 **Select multiple** 下切换选中——每个选中的带勾——然后用底部栏的 `N models selected` → **Add N models** 一次全部创建，**Clear** 清空）。

### 卡片四角

| 位置 | 内容 |
|------|------|
| 左上 | `NEW` 徽标，或自定义模型的标记 |
| 右上 | 当前选中模型上的对勾 |
| 左下 | 上次测试的通过/失败圆点（如有）——点击可重新打开该结果 |
| 右下 | 仅悬停显示的操作：**Copy**（精确的模型 id）和 **Test**（闪电）；自定义模型卡片还有 **Edit** 和 **Delete** |

**Test** 会打开对准该单个模型的[探测面板](./20-routing-rules.md#探测面板probe-panel)——选择请求形态、**Run Test**、查看 Journey、Token 用量和响应——无需先把模型选入规则。结果会保留为状态圆点，所以连续测试多个模型就留下一张快速记分卡。

---

## 相关页面

- [Claude Code 场景](./03-scenario-claude-code.md)
- [路由规则与插件](./20-routing-rules.md)
- [凭证管理](./08-credentials.md)
