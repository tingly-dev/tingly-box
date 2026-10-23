# 访问控制

路径：`/access-control`

访问控制页面（**System → Access Control**）管理 Tingly-Box 的核心认证令牌：**User Token (Control Panel)**——Web UI 登录凭证——和 **Model Token (API Proxy)**——各 Agent 场景的 API Key。页首的卡片概括了两者的区别（模型令牌供 API 客户端访问 `/openai/*`、`/anthropic/*`、`/tingly/*`；可以分享给 API 用户，但绝不要分享用户令牌）。

---

![访问控制](../images/access-control.png)

## 用户令牌（User Token）

用于 Web UI 的登录认证。

### 查看当前令牌

- 默认脱敏显示（`••••••••`）
- 点击眼睛图标切换明文显示
- 点击复制图标将令牌复制到剪贴板

### 安全提示

页面顶部的 **Token Security** 摘要说明两个令牌的区别，并提醒切勿将用户令牌分享给 API 用户。

令牌下方的 `Token is secure (randomly generated)` 表示它是随机生成的。若仍在使用**默认令牌**（初始安装时设置），页面会改为显示安全警告和 **Reset Now** 按钮。

### 重置令牌

点击 **Reset User Token**，弹出确认对话框，说明重置后果：
- 生成新的随机令牌
- 当前会话会自动更新
- 其他浏览器/设备需要重新登录
- 旧令牌立即失效

点击 **Reset** 确认后，成功对话框会显示新令牌（含复制按钮），保存好之后点击 **I've Saved My Token**。如果令牌在别处被改过，**Pull latest token from server** 可以刷新本页显示的内容。

---

## 模型令牌（Model Token）

模型令牌是各 Agent 场景代理接口的通用 API Key。

### 特点

- 所有场景（Claude Code、OpenAI 代理等）共享同一模型令牌
- 可与其他开发人员或环境共享，用于访问代理接口
- **不等同于**用户令牌，不能用于登录 Web UI

### 查看与复制

操作方式与用户令牌相同：眼睛图标切换显示，复制图标写入剪贴板。

### 重置模型令牌

点击 **Reset Model Token** 弹出确认对话框（*旧令牌立即失效——所有 API 客户端都需要更新*），确认后生成新令牌；更新完客户端后点击 **I've Updated My Clients**。

> **注意**：重置模型令牌后，所有配置了旧令牌的工具（Claude Code CLI、OpenAI SDK 客户端等）都需要更新令牌配置。

---

## 与 Team Keys 的关系

原先 Credential 下独立的「API Tokens」/「Sharing」页面（旧路径 `/tingly-box-token`）已**停用**——该地址现会直接重定向到 [Team Keys](./07-team.md#team-keys-总览)（`/agent/team/keys`）。具名、可多个的外部客户端访问令牌现在是 Team 的概念：每个 Team 的 **Team Keys** 相当于原来那份全局 API Tokens 列表，只是作用范围收窄到该 Team 自己的路由规则，而不是触达所有场景。

| | 用户令牌 | 模型令牌 | Team Key |
|-|---------|---------|-----------|
| 页面 | `/access-control` | `/access-control` | `/agent/team/keys`，或某个 Team 自己的工作区页面 |
| 数量 | 1 个 | 1 个 | 每个 Team 可多个 |
| 用途 | Web UI 登录 | Agent 场景 API Key（全实例、不区分范围） | 外部客户端访问，仅限该 Team 的 `/tingly/team[/v1]` 端点 |
| 命名 | 无 | 无 | 可自定义名称 |

---

## 相关页面

- [Team](./07-team.md)
- [系统设置](./17-system-settings.md)
