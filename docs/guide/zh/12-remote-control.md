# Remote

路径：`/bots/*`、`/remote-agent`、`/notify`

**Remote** 分组用于通过主流 IM 平台远程控制 Claude Code（及其他 Agent）。按关注点拆分为三个页面：**Bots**（接入消息账号）、**Remote Control**（将收到的聊天指令路由到某个 Agent）、**IM Notify**（将结果主动推送回聊天）。

---

## Bots（`/bots/overview`、`/bots/:platform`）

![Bots 总览](../images/bots-overview.png)

资源层：接入并维护 Remote Control 与 IM Notify 共用的消息账号。页面副标题：「Connect and maintain the messaging accounts used by Remote Control and IM Notify.」

支持平台：Telegram、Feishu、Lark、DingTalk、Weixin（微信）、WeCom（企业微信）、QQ、Discord、Slack——通过平台标签行筛选（**All** 加每个平台一个标签，均显示 `active N / total N` 计数）。

### 连接列表

| 列 | 说明 |
|----|------|
| Status | 启用/禁用开关 + On/Off 徽章 |
| Name | Bot 别名 |
| Bot UUID | 唯一 Bot 标识（含复制按钮） |
| Platform | 平台标识（如 `telegram`） |
| Capabilities | **Remote** / **Notify** 徽章——该 Bot 参与另外两个页面中的哪些功能 |
| Actions | **Access**（会话 ID / 群组授权）、恢复、编辑、删除 |

### 接入 Bot

点击右上角 **Connect a bot**：

![连接 Bot 对话框](../images/bots-connect-dialog.png)

1. 从下拉菜单选择 **Platform**
2. 填写该平台所需凭证（如 Telegram 的 **Bot Token**，从 `@BotFather` 获取）
3. 可选：**Alias**（友好名称）、**Proxy URL**（Bot API 请求使用的 HTTP/HTTPS 代理）
4. 点击 **Connect bot**

每个平台标签下还提供可展开的 **Setup Guide**，包含该平台专属的接入步骤、凭证说明和示例。

> 微信（Weixin）Bot 使用**扫码登录**而非 Token——发起连接后对话框会显示二维码供扫描。

---

## Remote Control（`/remote-agent`）

![Remote Control](../images/remote-control.png)

一个页面、**每个 Bot 一张卡片**——既不是先选平台再筛选的页面，也不是单张合并的路由图。页面副标题：「Choose who can control each bot and where chat commands route.」平台过去是进入页面前先选的一个标签页，现在只是卡片上的一个图标（大多数部署只有一两个 Bot，旧的平台瓦片大多只是在展示空平台）。

### 单 Bot 卡片

每张卡片承载一个 Bot 的 Remote Control 用途：

- **头部**：平台图标 + Bot 名称、一行实时状态文字，以及 **Remote Control 开关**（关闭只会卸载这一项用途——Bot 资源本身，以及它的其他能力如 IM Notify，不受影响），再加上 **Edit**/**Restart**/**Delete** 操作
- **状态行**用一句话回答「现在能用吗」，并用颜色区分：`Remote Control off`（灰色）· `Checking access…` · `Couldn't check who can control` / `Nobody can control yet` / `@tb has no model yet`（警示色）· `N can control`（成功色）
- **关闭**状态的卡片是安静的而非划线提示——没有纸张背景、身份信息变灰，路由图折叠为一行摘要而不是完整图示；它不是坏了，只是没在运行，下面的每一项设置依然一键可达
- **已启用但尚不可用**的 Bot（还没人获得授权，或 `@tb` 还没选模型）会在卡片内直接显示下一步的具体提示——例如可发给 Bot 配对的配对码，或提醒「有 N 个直接会话联系过该 Bot 但都还不能控制它」
- 头部下方，**展开**的卡片显示路由图：谁能发指令进来 → 这个 Bot → `@tb` / `@cc` 两个分支。图中每个节点都可单独点击编辑；入口节点会打开 **Access** 工作面，内容是允许发起指令的直接会话 ID 和/或群组——读取的正是卡片状态行所概括的同一份授权数据，确保「谁能控制这个 Bot」只有一个权威来源

### Claude Code Profile / 模型

Bot 路由图中的 `@cc` 分支可以指向某个具体的 Claude Code Profile（如果已配置任何 Profile）；`@tb`（SmartGuide）分支需要一个 Provider + 模型组合，从该节点打开的模型选择器中设置。

> 旧的分平台路径 `/remote-agent/:platform` 书签，以及拆分前的 `/remote-control/*`，都会自动重定向到这里。

---

## IM Notify（`/notify`）

![IM Notify](../images/im-notify.png)

与 Remote Control 页面结构相同：**每个 Bot 一组**，而不是按平台筛选的列表。页面副标题：「Authorize a target, send through the production path, and see whether delivery worked.」让场景和自动化流程能够向 Bot 已观察到的聊天/群组主动推送消息，与 Remote Control 的入站路由互不干扰。

### 单 Bot 分组

每个 Bot 分组自带一个 **Notify 开关**（与 Remote Control 相同，只卸载这一项用途），并以一个始终展开的图形列出该 Bot 能触达的每个聊天/群组，每个 **Chat 节点**将具体的平台 ID（如 `telegram:123456789`，标注为 **Direct** 或 **Group**）与 `/notify` 所需的稳定内部目标 UUID 配对，操作就内联在节点上（无需额外点击跳转）：

- **Notify** —— 发送测试/手动消息
- **Confirm**，显示为 **Allow Notify & Test**，用于尚未授权的目标
- **Custom** —— 发送自定义内容
- 复制（目标 UUID）、撤销、删除

新目标必须先经过显式授权（**Allow Notify & Test**）才能接收自动化通知——避免 Bot 偶然观察到的任何聊天悄然变成通知目标。此页面回答的是「现在能发给谁」，而不是旧版那种只读的「哪些场景路由指向这个 Bot」。

右上角 **API guide** 按钮提供供脚本/自动化调用的通知 API 文档。

---

## Bot 安全设置

### 授权访问（Access）

在 Bots 页面点击某个 Bot 行的 **Access**，限制哪些会话 ID 或群组可以向其发送指令——与 Remote Control 路由图中 **Access** 节点编辑的是同一份授权配置。

### Bash 白名单

在每条 Bot→Agent 路由上配置：每行一个命令模式，限制 Bot 可触发的 Shell 命令。不在白名单中的命令将被拒绝。示例：

```
ls
cat *.md
git status
git diff
```

---

## 使用方式

Bot 接入（Bots）并路由到某个 Agent（Remote Control）后，即可在 IM 平台上向其发送消息：

- 发送代码请求 → Bot 调用路由到的 Agent（如 Claude Code）执行
- 查询状态 → Bot 返回当前运行状态
- 发送文件 → Bot 在工作目录中处理该文件

长任务完成、告警等出站更新通过 IM Notify 推送给已授权的目标。

---

## 相关页面

- [场景总览](./02-scenario-overview.md)
- [系统设置](./17-system-settings.md)
