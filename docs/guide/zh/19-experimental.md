# 实验性功能

路径：`/system/experimental`

实验性功能页面集中管理尚在早期迭代阶段的特性开关，开启后相关功能将出现在侧边栏导航中。

---

![实验性功能](../images/experimental.png)

## 页面说明

页面标题：**Experimental Features**

页面副标题描述这些功能处于实验阶段，可能在未来版本中有所调整。

---

## 可用实验功能

### Skills（IDE Skills）

**开关标识**：`skill_ide`

开启后，在左侧侧边栏「Prompt」分组下激活 **Skills** 导航项（`/prompt/skill`），允许从 IDE 配置目录同步和管理可复用的 Prompt 片段。

- 仅 **Full Edition** 可见此开关
- 详见 [Prompt 管理](./14-prompt-management.md)

---

### Guardrails（防护栏）

**开关标识**：`guardrails`

开启后，左侧侧边栏显示 **Guardrails** 分组，包含：
- 防护栏总览（`/guardrails`）
- 策略组管理（`/guardrails/groups`）
- 策略规则（`/guardrails/rules`）
- 历史审计（`/guardrails/history`）

开启时页面显示说明信息（Alert），引导用户了解 Guardrails 的配置方式。

详见 [防护栏](./15-guardrails.md)。

---

### MCP Tools

**开关标识**：`mcp`

开启后，左侧侧边栏显示 **Tools** 分组，包含：
- MCP 注册服务器（`/mcp/sources`）
- MCP 本地模式（`/mcp/local-mode`）
- Server Tool（`/tools/servertool`）

开启时页面显示说明信息（Alert），引导用户了解 MCP 的配置方式。

详见 [MCP 与工具](./16-mcp-tools.md)。

---

## 开启方式

1. 访问 **系统设置** → **Experimental**（`/system/experimental`）
2. 找到目标功能的 Chip 开关
3. 点击切换为 **On** 状态
4. 侧边栏将立即刷新，显示新功能入口

---

## 关闭实验功能

将 Chip 开关切换为 **Off**，侧边栏对应功能入口隐藏，但已有配置数据保留。

---

## 相关页面

- [防护栏](./15-guardrails.md)
- [MCP 与工具](./16-mcp-tools.md)
- [Prompt 管理](./14-prompt-management.md)
- [系统设置](./17-system-settings.md)
