# 远程控制

路径：`/remote-control/:platform`（Full Edition）

远程控制功能允许通过主流 IM 平台（即时通讯工具）远程操控 Claude Code，实现随时随地通过聊天发送指令、接收结果。

> **注意**：远程控制功能仅在 **Full Edition** 中可用。

---

![远程控制（Telegram）](../images/remote-control.png)

## 支持的平台

在左侧侧边栏「Remote」分组下可看到所有支持的平台：

| 平台 | 路径 |
|------|------|
| 微信（Weixin） | `/remote-control/weixin` |
| 企业微信（WeCom） | `/remote-control/wecom` |
| Telegram | `/remote-control/telegram` |
| 飞书（Feishu） | `/remote-control/feishu` |
| Lark | `/remote-control/lark` |
| 钉钉（DingTalk） | `/remote-control/dingtalk` |
| QQ | `/remote-control/qq` |
| Discord | `/remote-control/discord` |
| Slack | `/remote-control/slack` |

---

## 页面结构

每个平台页面结构基本一致：

### 平台接入指南（可折叠）

展开后显示该平台的 Bot 配置说明，包括：
- 如何在对应平台创建 Bot
- 需要获取哪些凭证（Token、Secret 等）
- Webhook 地址设置方式（如有）

### Bot 列表

展示当前平台已配置的所有 Bot：
- Bot 名称/别名
- 状态指示器（运行中/已停止/错误）
- Bot 数量汇总（`active N / total N`）

### 操作

每个 Bot 卡片提供以下操作：
- **启用/禁用**开关
- **重启** 按钮
- **删除** 按钮
- **编辑** 配置

---

## 添加 Bot

点击 **Add Bot** 按钮，填写 Bot 配置表单：

| 字段 | 说明 |
|------|------|
| **Name** | Bot 别名（可选，便于识别） |
| **Platform** | 平台选择（当前页面已预选） |
| **Token** | 平台 API Token / Bot Token |
| **Proxy URL** | HTTP/HTTPS 代理（可选，用于访问受限平台） |
| **Chat ID Lock** | 限制 Bot 只响应指定聊天 ID 的消息（可选） |
| **Bash Allowlist** | 允许执行的 Shell 命令白名单（多行，可选） |
| **Model** | 指定该 Bot 使用的 AI 模型 |
| **Working Directory** | 默认工作目录 |

### 微信特殊配置

微信 Bot 使用**扫码授权**而非 Token 方式，配置后系统显示二维码供扫码登录。

---

## Bot 安全设置

### Chat ID Lock

填写聊天 ID（群 ID 或用户 ID）后，Bot 只会响应来自指定对话的消息，防止 Bot 被未授权用户控制。

### Bash Allowlist

每行一条命令模式，限制 Bot 可以执行的 Shell 命令范围。未在白名单中的命令将被拒绝执行。示例：

```
ls
cat *.md
git status
git diff
```

---

## 使用方式

配置完成后，在对应 IM 平台中找到 Bot，发送消息即可：

- 发送代码需求 → Bot 调用 Claude Code 执行
- 查询状态 → Bot 返回当前运行状态
- 发送文件 → Bot 在工作目录处理文件

---

## 相关页面

- [Remote Coder](./13-remote-coder.md)
- [系统设置](./17-system-settings.md)
