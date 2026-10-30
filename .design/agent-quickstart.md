# Agent 页 Quick Start 与安装入口

状态：方案（TEP-2336）。尚未实施；实施后改写为现行契约。

## 问题

Agent 页的 Quick Start 是四步卡：连 Provider、选模型、安装、应用配置。

| 步骤 | 真值来源 | 页面上已有的入口 |
|---|---|---|
| 连 Provider | provider 列表 | 规则栏 Connect AI |
| 选模型 | 规则 | 下方 Model Rules |
| 安装 | 自我申报 | 无（唯一独有内容：命令、镜像、各 Agent 的操作） |
| 应用配置 | Claude Code / Codex / DSH 可回读，其余自我申报 | 头部 Auto Config / Setup Guide 与状态芯片 |

四步里三步与页面重复，却长期占首屏；为它打的补丁很多（进度键、重开/重置/隐藏、`wasAllDone`、连接区折叠依赖 `setupFinished`，见 `ui-flow-analysis.md` §8）。

## 方案对比

1. 只留安装和配置两步：配置就是头部按钮，剩下"两步的壳 + 一条命令"，机制全保留。
2. 改成引导图并保留 Install：有教育价值，但图要三语维护、会过期、没有状态，Install 仍无处安放。
3. Install 进头部：最省，无状态。

## 决定

3 + 2 并用（YZ）：

- Install 是头部连接区里的普通一行，在 Base URL / API Key 之后，只展示命令（含镜像命令和各 Agent 的 `installActions`），不打勾、无状态，一直显示。
- Quick Start 卡整个删除，连同进度键和重开/重置/隐藏。`connected` 改为 `configApplied || 发过请求 || 已有模型`，连接区折叠规则不变。
- 下一步由头部唯一的实心主按钮指路：无 Provider → Connect AI；无模型 → Add model；其余 → Auto Config / Setup Guide；状态芯片说明原因。
- "Quick Start" 此后只表示打开引导图。Claude Code 与 Codex 是主打，各有自己的引导图；其他 Agent 暂只有 Install 行和 Setup Guide。引导图是静态教育（SVG，文案走 i18n，随主题与语言变化），不跟踪进度。

## 取舍与风险

- 新用户失去"1 2 3 4"清单；靠主按钮、状态芯片和一键可达的引导图补偿，需在无 Provider / 仅 Provider / 仅模型 / 已应用四种状态下验证真实页面。
- 无配置回读的 Agent（OpenCode、Pi、VS Code、WorkBuddy）的"已连上"只能靠"发过请求"。
- 引导图随页面改动会过期：只用页面自己的 i18n 文案绘制，不用位图。

## 范围外

`ImageApiPage` 的 `ImageGenQuickStartDialog` 是另一个入口，不动。
