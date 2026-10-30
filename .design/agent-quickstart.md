# Agent 页 Quick Start 与安装入口

状态：已实施（TEP-2336）。

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

## 现行契约

- Install 是头部连接区里的一行（Base URL / API Key 之后），像 Base URL 的 Local | Docker 一样用 官方 | 镜像 开关切换命令，复制按钮在行尾；没有命令的 Agent（VS Code 等）显示各自的安装链接。只展示，无状态，一直显示。
- Quick Start 卡已删除，连同 `setup-card-*` 进度键和重开/重置/隐藏。`connected = 配置已应用 || 发过请求 || 已有模型`，连接区折叠规则不变。
- 头部的 "Quick Start" 按钮打开已有的 Direct Routing Guide（`EntryGuideDialog`），不再有独立引导图；下一步由规则栏的 Connect AI / New Model 与头部状态芯片指路。
- Claude Code 与 Codex 的引导页是"接入"步骤加原有路由步骤：安装（命令，官方/镜像）→ 连 AI → 加模型 → 应用配置 → 启动并确认 → 其余路由步骤。连 AI 与加模型直接复用路由引导的前两步；安装、应用、确认是三个文字面板（`EntryGuideDialog` 的 `agent` 属性，`diagrams.ts` 的 `AGENT_*_STEP`）。其他 Agent 仍是原路由引导；要给它们加，只需在描述里设 `connectGuide`。
- 共用的类型与 `hasModelOnAnyRule` / `scrollToModelsCard` 在 `components/agentSetup.ts`。

## 取舍与风险

- 新用户失去"1 2 3 4"清单；靠主按钮、状态芯片和一键可达的引导图补偿，需在无 Provider / 仅 Provider / 仅模型 / 已应用四种状态下验证真实页面。
- 无配置回读的 Agent（OpenCode、Pi、VS Code、WorkBuddy）的"已连上"只能靠"发过请求"。
- 接入步骤是文字面板，没有为每个 Agent 另画示意图；等首次使用的反馈再决定要不要。

## 范围外

`ImageApiPage` 的 `ImageGenQuickStartDialog` 是另一个入口，不动。
