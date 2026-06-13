# 模型选择

模型选择对话框用于为路由规则指定目标 Provider 和模型，是配置转发规则的核心交互入口。

---

![模型选择对话框](../images/model-select.png)

## 打开方式

在任意场景（Claude Code、Codex 等）的 **模型与转发规则** 区域，有以下几种方式可以打开模型选择对话框：

- **点击现有 Provider 节点**：在路由图中点击显示模型名称的卡片（如 `claude-sonnet-4-6`），进入"编辑"模式
- **点击「+ Add」按钮**：在规则行末尾点击添加按钮，进入"新增"模式
- **AgentSetupCard 引导步骤**：在 Claude Code 场景的 Quick Start 第 2 步展开后，点击 **Choose Model** 按钮

---

## 对话框结构

### 左侧：Provider 列表

- 列出所有已配置的 Provider（来自 [凭证管理](./08-credentials.md)）
- 点击 Provider 名称展开/折叠其模型列表
- 每个 Provider 显示支持的所有模型

### 右侧/顶部：搜索与筛选

- **搜索框**：按模型名称或 Provider 名称过滤
- 支持快速定位特定模型

### 选择操作

- 点击模型行即选中，确认后写入路由规则
- 在"新增"模式下，选中模型会自动创建新的转发规则

---

## 相关页面

- [Claude Code 场景](./03-scenario-claude-code.md)
- [路由规则与扩展标记](./20-routing-rules.md)
- [凭证管理](./08-credentials.md)
