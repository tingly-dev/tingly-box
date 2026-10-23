# 其他编码 Agent 场景

本章介绍 Claude Code 和 Codex 之外的编码 Agent 场景：OpenCode、Pi、WorkBuddy、DeepSeek Harness、VS Code、Xcode 和 Claude Desktop（Cursor 有[单独的章节](./04-scenario-cursor.md)）。它们都使用标准的 Agent 页面——状态行、按钮、可折叠的连接卡、Plugins 卡、Model Rules——详见 [Claude Code](./03-scenario-claude-code.md#页面结构)。页头的主操作按钮：网关能代为写客户端配置时是 **Auto Config**，不能时是 **Setup Guide**（手动步骤对话框）。OpenCode、Pi、WorkBuddy、DeepSeek Harness 和 VS Code 有 **Quick Start** 卡（Provider → 模型 → 安装 → 配置）；Xcode、Cursor 和 Claude Desktop 没有。

| Agent | 路径 | 页头操作 | Quick Start |
|-------|------|----------|-------------|
| OpenCode | `/agent/opencode` | Auto Config | 有 |
| Pi | `/agent/pi` | Setup Guide | 有 |
| WorkBuddy | `/agent/workbuddy` | Auto Config | 有 |
| DeepSeek | `/agent/dsh` | Auto Config + **Open Web UI** | 有 |
| VS Code | `/agent/vscode` | Setup Guide | 有 |
| Xcode | `/agent/xcode` | Setup Guide | — |
| Claude Desktop | `/agent/claude_desktop` | Setup Guide | — |

---

## OpenCode

路径：`/agent/opencode`

代理 OpenCode CLI 的请求。**Auto Config** 会展示网关将要写入的配置——JSON、Windows 脚本或 Linux/macOS 脚本——按钮会写入 `~/.config/opencode/opencode.json`。连接卡里有 Base URL 和 API Key 两行。安装：`npm install -g opencode-ai`。

---

## Pi

路径：`/agent/pi`

![Pi 场景](../images/pi-scenario.png)

代理 Pi 编码 Agent。**Setup Guide** 说明如何把 Pi 的 Provider base URL 和 API key 指向本页显示的值（具体配置格式见 Pi 仓库）；Quick Start 的安装步骤带有仓库链接（**View on GitHub**），第 4 步是 **Configure Pi**。

---

## WorkBuddy

路径：`/agent/workbuddy`

![WorkBuddy 场景](../images/workbuddy-scenario.png)

把腾讯 WorkBuddy 作为**自定义 OpenAI 兼容模型**接到 Tingly-Box——不涉及 WorkBuddy 账号登录。（把 WorkBuddy 账号作为上游 Provider 登录是另一回事，见[凭证管理](./08-credentials.md)。）

![WorkBuddy 对话框](../images/workbuddy-config-modal.png)

**Auto Config** 会把**每条规则一个模型**合并写入本机已安装的每个 WorkBuddy 版本的 `models.json`——`~/.workbuddy/models.json`（国内版）和 `~/.workbuddy-ai/models.json`（国际版），两个都装了就都写。文件里的其他模型会保留，写入前先备份，你已有的同名 id 会被跳过并给出警告。对话框会预览将写入的条目（API key 已打码）；如果之后 WorkBuddy 里没出现这个模型，请退出并重新打开 WorkBuddy。

对话框里的 **Manual setup** 是同一件事的手动做法：WorkBuddy → Settings → Model → Add model → Provider 选 **Custom**，然后填写：

- **URL**：以 `/chat/completions` 结尾的完整路径：`<Base URL>/tingly/workbuddy/v1/chat/completions`
- **API Key**：Tingly-Box 令牌
- **Model ID**：本页 Model Rules 中的某个请求模型

勾选 **Tool calling**；只有当规则路由到的模型支持时才勾选 **Image input** 或 **Reasoning**（标记错了会导致调用报错）。对话框里有复制 URL 和 Key 的按钮。

---

## DeepSeek Harness（dsh）

路径：`/agent/dsh`

![DeepSeek Harness 场景](../images/dsh-scenario.png)

代理 [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness)——一个带**自托管 Web UI** 的 Agent harness。

- **Open Web UI**（页头，实心）跳转到本地运行的 Web UI，默认 `http://127.0.0.1:3080`。Tingly-Box **不会**替你启动 dsh——请先用 `npx @deepseek-ai/dsh web` 启动（需要 Node.js）
- Quick Start 第 3 步是 **Install DeepSeek Harness**（同样带 **Open Web UI** 和 **View on GitHub** 链接），第 4 步是 **Configure DSH**
- `dsh` 处于开发者预览阶段，配置面可能变化

**Auto Config** 会在 `$DSH_HOME/settings.yaml`（以及 `$DSH_HOME/.credentials.yaml`）里写入一个 `tingly-box` Provider，并带两个额外设置：

| 设置 | 取值 | 作用 |
|------|------|------|
| **Primary protocol** | `openai-completions`（默认）/ `openai-responses` / `anthropic-messages` | Tingly-Box 与 dsh 适配器之间使用的协议格式。OpenAI Chat 兼容性最好；只有该 Provider 下的模型确实使用其他协议时才改 |
| **Supported input modality** | `text`（默认）/ `text_image` | 模型是否接受图片输入；留空/`text` = 仅文本 |

---

## VS Code

路径：`/agent/vscode`

通过 **Tingly Box 扩展**代理 VS Code Copilot Chat 和其他 AI 扩展，由扩展帮你完成端点和密钥配置。Quick Start 第 3 步提供 **Install in VS Code**（打开扩展）和 **View Marketplace**；第 4 步是 **Follow VS Code Guide**。**Setup Guide** 重复这一说明：先安装扩展，然后按 VS Code 里的引导操作。

---

## Xcode

路径：`/agent/xcode`

把你的模型接入 Xcode 的编码智能功能。**Setup Guide** 列出步骤，附带 URL 和 Key 的复制按钮，以及 Xcode 设置面板的截图：

1. 打开 **Xcode → Settings → Intelligence**
2. **Add a Model Provider** → **Internet-Hosted**
3. 填入 URL `<Base URL>/tingly/xcode`、API Key 和任意 Description（例如 `Tingly Box`）

---

## Claude Desktop

路径：`/agent/claude_desktop`

把 Claude Desktop 的**第三方推理（third-party inference）**路由到你的 Provider。没有需要粘贴的配置文件：**Setup Guide** 带你走一遍 Claude Desktop 自己的设置。

1. **Enable Developer Mode**：下载 Claude Desktop，然后 *Help → Troubleshooting → Enable Developer Mode*
2. **Configure Third-Party Inference**：*Developer → Configure third-party inference*：Connection 选 **Gateway**，Base URL 填 `<Base URL>/tingly/claude_desktop`，再填 API key
3. **Configure Models**（可选）：Claude Desktop 会自己从 `/v1/models` 发现模型。想固定列表时，粘贴生成的 `inferenceModels` JSON；对话框会列出每条 Model Rule，并带一个可填写的 **label** 字段（会成为该模型的显示名 `labelOverride`）。模型本身在 Model Rules 里增删

这里的规则同样有 **`1M`** 标签；切换时会打开 Setup Guide 并显示提示，告诉你重启 Claude Desktop 并重新选择模型（没有需要重新应用的配置文件）。

---

## 场景显隐

在侧边栏的[编辑模式](./02-scenario-overview.md#管理可见性侧边栏编辑模式)（眼睛图标）里隐藏你不用的 Agent。

---

## 相关页面

- [Claude Code 场景](./03-scenario-claude-code.md)
- [Cursor 场景](./04-scenario-cursor.md)
- [Codex 场景](./04-scenario-codex.md)
- [场景总览](./02-scenario-overview.md)
- [凭证管理](./08-credentials.md)
