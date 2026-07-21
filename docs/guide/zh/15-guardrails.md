# 防护栏（Guardrails）

路径：`/guardrails`、`/guardrails/groups`、`/guardrails/rules`、`/guardrails/history`

防护栏（Guardrails）对 AI Agent 的工具调用和工具结果进行基于规则的安全检查，防止危险操作、保护隐私数据、控制资源访问。

> **注意**：Guardrails 功能需要在 [实验性功能](./19-experimental.md) 中开启 Guardrails 开关后，侧边栏才会显示。

---

![防护栏总览](../images/guardrails.png)

## 防护栏总览（`/guardrails`）

### 统计看板

**Policy Breakdown（左卡）** — 按策略类型汇总：

| 类型 | 说明 |
|------|------|
| Resource Access | 文件读写、网络访问控制策略数量（启用/总计） |
| Command Execution | Shell 命令执行控制策略数量 |
| Privacy (Content) | 内容隐私过滤策略数量 |

**Event Summary（右卡）** — 防护事件统计：

| 指标 | 颜色 |
|------|------|
| Total Events | — |
| Allow | 绿色 |
| Review | 黄色 |
| Blocked | 红色 |
| Masked | 紫色 |

### 策略导入/导出

**Import Policies** 按钮 → 打开导入对话框：
- 选择文件（YAML/JSON 格式的策略片段）
- 或直接粘贴策略内容（支持多种格式）
- 确认 **Import**

**Export Imports** 按钮 → 打开导出对话框：
- 复选框列表，选择要导出的策略片段文件
- 支持 **Select All** / **Clear** 批量操作
- 点击 **Export** 下载所选策略为 YAML 文件

---

## 策略组（`/guardrails/groups`）

![策略组](../images/guardrails-groups.png)

策略组将多条策略归类管理，支持整体启用/禁用。

> 页面说明文字：`Groups organize policies and control whether those policy sets participate in evaluation. Built-in is a policy label, not a group type.`

### 组列表

每个策略组显示：
- 组名称
- 严重级别（Low / Medium / High）
- 启用/禁用状态
- 包含的策略数量
- 操作：编辑、删除

> `Default` 组为内置组，不可删除（显示锁定图标）。

### 创建/编辑组

点击 **New Group** 或编辑图标：
- **Name**：组名称
- **Severity**：Low / Medium / High
- **Enabled**：启用开关

### 策略分配（Assign Policies）

页面下半部分展示**可分配的策略列表**，每条策略带有：
- 策略名称和类型标签（如 `Privacy`）
- 描述（如 `No patterns configured`）
- 独立的开关控制

勾选后该策略加入当前组，取消勾选则从组中移除。一条策略可以同时属于多个组。

---

## 策略规则（`/guardrails/rules`）

![防护栏规则](../images/guardrails-rules.png)

路径：`/guardrails/rules`

详见 [防护栏规则管理](#策略规则详解) 节。

### 三个标签页

| 标签 | 说明 |
|------|------|
| **Resource Access** | 文件读/写/删除和网络访问规则 |
| **Command Execution** | Shell 命令执行模式匹配规则 |
| **Privacy** | 内容正则/关键词过滤规则 |

### 批量操作

每个标签页顶部提供：
- **Enable All**：启用当前标签下所有策略
- **Disable All**：禁用当前标签下所有策略

### 策略列表

每条策略显示：
- 策略 ID（自动生成）
- 策略名称
- 状态（Enabled / Disabled / No active group）
- 所属组
- 操作：编辑（内置 + 自定义）、删除（仅自定义）

### 创建策略

点击 **Add Policy** 或编辑图标打开策略编辑器：

**基础字段：**
- ID（自动生成，可自定义）
- 名称
- 所属策略组

**Resource Access 特有字段：**
- Actions：read / write / delete / network（多选）
- Resources：资源路径列表（glob 模式）
- Tools：适用的工具名称列表

**Command Execution 特有字段：**
- Terms/Patterns：命令关键词或正则列表
- Actions：execute / install（多选）

**Privacy 特有字段：**
- Patterns：关键词或正则表达式列表
- Pattern Mode：substring（子串匹配）/ regex（正则）
- Case Sensitive：大小写敏感开关

**通用字段：**
- Scenario Scope：适用场景（Anthropic / Claude Code / OpenAI 等）
- Verdict：block（拦截）/ allow（放行）/ mask（脱敏）/ review（人工审核）
- Reason：拦截原因说明（会在拦截时返回给 Agent）

### 策略注册表

底部 **Registry** 区域允许从远程策略仓库下载和安装预置策略集，快速建立基础防护规则。

---

## 历史审计（`/guardrails/history`）

![防护栏历史](../images/guardrails-history.png)

路径：`/guardrails/history`

查看所有防护栏触发记录。

### 筛选

- **Verdict**：All / Allow / Review / Block / Mask
- **Time**：All / 1h / 24h / 7d

### 事件列表

可展开的表格行，展示摘要后展开可查看：
- Provider 和模型
- 请求方向（请求/响应）
- 触发的策略列表
- 拦截/审核消息

### 操作

- **Refresh**：手动刷新事件列表
- **Clear History**：清空全部历史（需确认）

---

## 相关页面

- [实验性功能](./19-experimental.md)
- [MCP 与工具](./16-mcp-tools.md)
