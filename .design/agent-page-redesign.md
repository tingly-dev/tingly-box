# Agent 页重设计 — 模板、配置状态与实施边界

> 状态：**A–D 已落地，E 部分撤回，Profile 合并暂缓**。当前行为以进度表为准，后文保留原始设计供决策追溯。是 `ui-redesign.md` 的展开：导航层面的改动都比较轻，真正最麻烦的是每一个 Agent 页（`/agent/*`）。
> 证据来自 mock 模式截图（1440 宽，Claude Code 的 Unified / Separate、Auto Config 弹窗、Profile、Codex、Xcode、Claude Desktop）和代码走读（2026-09-30）。原则引用见 `ux-principles.md`（P1–P12）。

---

## 进度（2026-09-30）

| 项 | 状态 | 说明 |
|---|---|---|
| A 不改布局的修复 | ✅ | 没插件的规则不再占 Plugins 列（改为标题行「+ Plugins」）；API Key 单独一行；Local / Docker 与 profile 的 npx / Global 统一为带文字的 `ChoiceToggle`；手动类 Agent 按钮改为「Setup Guide」；ScenarioPage 与 TemplatePage 共用一份规则；Quick Start 进度存服务端（见 ui-redesign P1） |
| B 客户端配置状态 | ✅ Claude Code / Codex / DSH | `GET /api/v1/config/{claude,codex,dsh}/status` + 标题旁状态 chip（`useClientConfigStatus`、`AgentPageDescriptor.clientConfigTool`）；旧 toast / 确认框暂留一个版本 |
| C 验证 | ✅（轻量版）；3 步重排不做 | 安装步骤检测到真实请求即完成，手动确认保留。§3.4 的"4 步改 3 步、第 3 步等待首个请求"曾经实现过，评审后决定不需要（2026-09-30），已 revert，Quick Start 保持 4 步 |
| D 模板 + descriptor | ✅ 13 个 Agent 页 | `AgentPage` + `AgentPageDescriptor`（`pages/scenario/AgentPage.tsx`）：SDK 4 页 → 手动类 5 页 → OpenCode / DSH → Codex → Claude Code，每步一个 commit，渲染出的 DOM 与迁移前一致（Claude Code 标题行改用共用的 `ScenarioCardHeader`，状态 chip 左移 4px）。回退手段是 revert 对应 commit，不做运行时开关。迁移完成后 `ScenarioPage` 并入 `AgentPage`，它的 render-prop 接口随之删除；`ScenarioCardHeader` 和宽度常量移到 `components/ScenarioCardHeader.tsx`，供 Team / Profile / Image API 引用而不必加载 Agent 页。Team、Profile 未迁移（Team 面向团队、有自己的成员 / key 管理；Profile 合并暂缓） |
| E 分区调整 | 部分：插件与模式切换的挪位评审后撤回 | 场景级插件移到 Model Rules 顶部、改名「Rule defaults」，以及 Unified / Separate 移进 Model Rules，都实现过一版，评审后撤回（2026-09-30）：场景插件是 Agent 级设置而非规则默认值（Smart Compact 根本没有规则级对应项），改名造成一物两名（规则卡片里仍叫 Plugins），常用设置被压到 Quick Start 之下。两者留在标题 / 接入卡片；模式切换的更好形态另议。依赖挪位的"规则覆盖默认值"标记一并撤回。Claude Desktop 的弹窗只生成 JSON：不再增删规则，只保留它特有的 labelOverride 标签编辑，并指向 Model Rules（§3.8）。接入区"走完三步后折叠为一行摘要"未做：Quick Start 完成后本身会折叠，接入区只剩 Base URL / Key 两行 |
| Profile 合并 | 暂缓 | 见 §3.6 |

---

## 0. 结论先行

1. **根本问题是两条轴缠在一起**：一条是"客户端配置"，即写到用户机器上的文件，如 `~/.claude/settings.json`；另一条是"网关路由"，即服务端的规则。页面上好几个控件同时动这两条轴，却没有任何地方告诉用户"你的 Claude Code 现在用的，是不是这个页面上显示的"（P4）。
2. **用一个常驻的"客户端配置状态"取代散落的提示。** 状态分为：未应用、已应用、已过期、无法检测。凡是会让客户端配置失效的操作，只负责把状态翻成"已过期"，重新应用永远是同一个位置的一个按钮。后端已经能从磁盘读回 Claude Code / Codex / DSH 实际生效的配置，所以这是走真实链路的检测（P7）。
3. **16 个页面收敛为 1 个模板 + 按能力描述的 descriptor。** 每页都是"接入 / 路由"两个区，页面之间的差异由能力决定，而不是由各页自己的布局决定。
4. **Quick Start 的"完成"改用真实信号**：收到这个 Agent 的第一个请求，替代 "I've installed it" 的自我报告。
5. **按风险从低到高推进**：先做不改布局的修复，再做纯增量的状态指示，然后逐页迁移到模板（Claude Code 最后），最后才调整分区顺序。

---

## 1. 现状：同一种页面，五种长法

16 个 `/agent/*` 页面中，11 个用通用骨架 `ScenarioPage`（`pages/scenario/ScenarioPage.tsx`），另外 5 个自己排版：Claude Code、Codex、Claude Code Profile、Team、Overview。

| 页面 | 标题栏右侧 | Quick Start | 客户端配置入口 | 规则 | 1M |
|---|---|---|---|---|---|
| OpenAI / Anthropic / Embed / Custom SDK | 无 | 无 | 无 | 可增删 | 无 |
| Cursor / Xcode | `Config`（说明弹窗） | 无 | 仅说明 | 可增删 | 无 |
| Claude Desktop | `Config`（生成 `inferenceModels` JSON，**也能在弹窗里增删规则**） | 无 | 手动 | 可增删（两个入口） | 有 |
| Pi / VS Code | `Config`（链接） | 有，第 4 步只有指南 | 手动 | 可增删 | 无 |
| OpenCode | `Auto Config`（先预览再打开弹窗） | 有 | 自动写 `opencode.json` | 可增删 | 无 |
| DSH | `Open Web UI` + `Auto Config`（描边样式） | 有 | 自动 | 可增删 | 无 |
| Codex | `Auto Config` | 有 | 自动写 `config.toml` 等 | 可增删 | 有 |
| Claude Code | `Unified / Separate` + `Auto Config` | 有 | 自动写 `settings.json` | 固定内置规则 | 有 |
| Claude Code Profile | 无（标题旁有重命名、删除） | 无，改为 `Start \| Settings` 行 | 无，改为 Profile Overrides 卡片 | 固定 | — |
| Team | `Enabled` 开关 + `Team Keys` | 无 | 无（分发 key） | 可增删 | 无 |

同一个位置出现了 7 种不同的按钮组合；Quick Start 只有 6 个页面有；"配置"这件事分别叫 `Config`、`Auto Config`、`Apply`、`Settings`、`Profile Overrides`。

---

## 2. 真正的问题（按严重程度）

### 2.1 客户端配置和网关路由缠在一起（P4）

以 Claude Code 为例，会让 `~/.claude/settings.json` 失效的操作有：

| 操作 | 所在位置 | 反馈 |
|---|---|---|
| 切换 Unified / Separate | 页面标题栏 | 先弹确认框，再弹一个会自动消失的 toast："Please reapply the configuration to Claude Code" |
| 切换规则的 1M | 规则标题行 | 直接弹出 Auto Config 弹窗 |
| 改规则的请求模型名（如 `claude-sonnet-5`） | 规则标题行，行内编辑 | **没有任何提示** |
| 改 Auto Config 里的模型槽位 | 弹窗内的 5 个自由文本框（`tingly/cc-haiku`…） | 这些值其实由规则推导而来，等于第二处编辑同一件事的地方 |

结果是用户无法回答**"我的 Claude Code 现在用的，和这个页面显示的一样吗？"** toast 一消失，这个信息就没了。

### 2.2 客户端配置有 4 个入口、同一个词有 4 种用法

- 入口：标题栏按钮、Quick Start 第 4 步（Apply + Config）、1M 开关、模式切换确认框。
- `Auto Config` 这个词同时是：标题栏按钮、弹窗里的 tab、Quick Start 第 4 步的标题、弹窗底部的主按钮。
- `Config`（Cursor / Xcode / Claude Desktop）与 `Auto Config`（Codex / OpenCode）放在同一个位置，行为却完全不同：一个打开说明，一个写文件（P3）。

### 2.3 Quick Start 靠自我报告，而且状态分裂

- 第 3 步"安装"的完成条件是用户点 "I've installed it"。整个流程从来没有验证过工具真的发出了请求（P7）。
- 第 3、4 步的完成状态存在 `localStorage`（`setup-card-step2-done-{k}` 等）。Wails 窗口（`wails://`）和浏览器（`localhost`）是两个 origin，同一个用户在两边看到的进度不同。

### 2.4 路由图在 1440 宽下被横向截断

Claude Code 的 Smart 规则有 4 个条件、3 个 tier，最右侧的服务卡片被裁掉了一截。原因是每条规则右侧都固定留出约 240px 的 Plugins 面板，即使面板是空的（"None enabled. Click to configure."：Claude Desktop 页出现 4 次）。在 Wails 默认 1200 宽的窗口里会更严重。

### 2.5 两层插件，看不见继承关系

- 标题栏的 Plugins 是 scenario 级 flag，规则卡里的 Plugins 是 rule 级 flag。服务端的 `resolveRuleFlagsWithScenario` 规定规则级覆盖 scenario 级（见 `rule-flags.md` §12）。
- 但规则卡只显示规则自己的 flag，不显示继承来的值。`Thinking` 两层都有，用户无法知道某条规则最终生效的是哪个。

### 2.6 Profile 页是另一种布局

Profile 本质上是"Claude Code 的一个变体"，页面结构却不同：`Start | Settings` 行替代了 Quick Start，`Profile Overrides` 卡片替代了 Auto Config 弹窗，而两者编辑的是同一组设置；模式只读；没有应用入口。用户在 default 和 p1 之间切换时，每次都要重新认一遍页面。

### 2.7 小问题（逐个都便宜）

- 紧凑模式下 `Base URL | API Key` 做成 tab，key 被藏在第二个 tab 里；OpenAI / Anthropic 页却是两行都显示。
- Base URL 右侧的笔记本 / 鲸鱼图标是 `EnvironmentModeSwitcher`（`local` / `docker`，决定 host 是 `localhost` 还是 `host.docker.internal`），**只有图标没有文字**（P5）。
- Claude Desktop 可以从弹窗和 `New Rule` 两个地方加规则。
- 规则被请求两次：`TemplatePage` 内部又调用了一次 `useScenarioPageInternal`。

---

## 3. 设计：一个模板，两个区，一个状态

### 3.1 页面结构

```
┌──────────────────────────────────────────────────────────────────────────┐
│ ✳ Claude Code   [default ▾]          ● 最近请求 2 分钟前                     │
│                                      客户端配置  ⚠ 已过期（模型槽位 2 处不同） [重新应用] │
├── 接入 ──────────────────────────────────────────────────────────────────┤
│ Base URL  http://localhost:12580/tingly/claude_code        [复制]          │
│ API Key   tb-••••••••••3f2a                                [复制] [显示]   │
│ 运行位置  (•) 本机 localhost   ( ) Docker host.docker.internal               │
│                                                                          │
│ ✓ 安装    npm install -g @anthropic-ai/claude-code          [复制]   ▸     │
│ ✓ 应用配置  已写入 ~/.claude/settings.json                    [客户端设置 ▸] │
│ ✓ 验证    已收到第一个请求（09-30 14:02）                                   │
├── 路由 ──────────────────────────────────────────────────────────────────┤
│ 模型槽位  (•) 统一：5 个槽位 → 1 条规则   ( ) 分开：5 个槽位 → 5 条规则          │
│ 所有规则的默认值  Thinking: 由客户端 · Smart Compact: 开 · Vision: 关 · Record: 关 │
│                                                                          │
│ claude-sonnet-5   Smart · 4 个条件 → deepseek-v4-pro, glm-5.1 …   插件 4  ⚡ ⚙ ▾ │
│   └ 展开后是现有路由图，占满宽度                                              │
└──────────────────────────────────────────────────────────────────────────┘
```

- **标题栏只放身份和状态**：名字、变体切换（Profile）、连接状态、客户端配置状态。模式切换和各种配置按钮都从标题栏移走。
- **接入区**回答"怎么接上、接上了没有"。三个步骤走完后折叠为一行摘要（"已接入 · 已应用 · 最近请求 2 分钟前"），随时可展开（P10）。
- **路由区**回答"请求会去哪"。Unified / Separate 放在这里，因为它是路由决策：决定有几条规则。
- 活动（最近请求）先不单独做区，而是由标题栏的"最近请求"链接到按该 Agent 过滤的日志或 Dashboard，以后视需要再加。

### 3.2 客户端配置状态（核心机制）

| 状态 | 条件 | 显示 |
|---|---|---|
| 未应用 | 磁盘上的配置文件不存在，或没有指向本网关 | `未应用 [应用配置]` |
| 已应用 | 磁盘上的值与当前规则和模式推导出的值一致 | `✓ 已应用`（安静） |
| 已过期 | 不一致 | `⚠ 已过期（N 处不同）[重新应用]`，点开能看到逐键差异 |
| 无法检测 | 工具只能手动配置（Cursor、Xcode、Claude Desktop、Pi、VS Code），或网关与工具不在同一台机器上（团队部署） | 不显示状态，只提供指南；不假装知道 |

- **可行性**：`GET /api/v1/config/claude`、`/codex`、`/dsh` 已经通过 `agent.ReadMainClaudeCodeSettings()` 等函数直接读取磁盘上的文件（`internal/server/module/configapply/handler.go`）。还需要补一个"按当前规则应该是什么"的对比：可以复用 preview 接口，或新增一个返回差异的接口。OpenCode 已有 preview、apply 和 restore 接口，读回能力待确认。
- **替代掉的东西**：模式切换的确认框和 toast、1M 自动弹窗、改名后的沉默。这些操作都只把状态翻成"已过期"，不打断用户（P12）。重新应用永远在同一个位置（P11）。
- **Auto Config 里的模型槽位**改为由规则推导的只读展示；需要特殊值时，走一个明确的"自定义"逃生口（见 §6）。

### 3.3 一个入口、两个词

- **应用配置**：自动写文件的动作，用于 Claude Code、Codex、OpenCode、DSH。
- **配置指南**：手动步骤，用于 Cursor、Xcode、Claude Desktop、Pi、VS Code，以及所有自动工具的"手动"模式。
- 原 Auto Config 弹窗里的选项（permission mode、thinking summaries、token 上限……）叫**客户端设置**，从接入区打开，弹窗内保留"手动"tab。
- 按钮上显示哪个词，由 descriptor 的能力决定，而不是由页面决定。

### 3.4 验证 = 收到真实请求

> **不做（2026-09-30 评审决定）**：下面的"改为 3 步 + 等待首个请求"实现过一版，评审认为多余，已 revert。保留的只有轻量版：安装步骤检测到真实请求即算完成。

- Quick Start 由 4 步改为 3 步：安装 → 应用配置（或按指南配置）→ 验证。原第 1 步"连接 AI 服务"、第 2 步"选择模型"是路由区的事，空状态由路由区自己引导（现有的 spotlight 保留）。
- 验证步骤显示"等待第一个请求…"，收到该 scenario 的第一条请求后自动完成（数据来自用量记录）。"我已完成"作为手动跳过保留。
- 步骤状态存到服务端，浏览器和 Wails 窗口一致。

### 3.5 插件：默认值 + 覆盖

- 标题栏的 Plugins 行移到路由区顶部，文案改为"所有规则的默认值"。
- 规则展开后，插件区用灰色显示继承来的值，实色显示本规则的覆盖值（"Thinking: High，覆盖默认的'由客户端'"）。
- 规则没有覆盖值时，不再渲染整块虚线面板，只在规则标题行放一个 `插件` 标签（有覆盖时带数量）。路由图因此拿回全部宽度，解决 §2.4 的截断。

### 3.6 Profile = 同一页面的变体

> **暂缓（2026-09-30 决定）**：Profile 页先保持独立布局，不并入 Claude Code 页的变体切换，之后再讨论。本节保留作为讨论材料；Agent 页各阶段的改动不依赖它。

- 标题栏的 `[default ▾]` 切换 default / p1 / p2 / 新建，页面结构完全相同。
- Profile 的接入区：安装步骤不变，"应用配置"换成启动命令 `tingly-box profile p1`（保留 npx / 全局切换）。
- **客户端设置**用同一个组件，Profile 下每一项多一个"继承 / 覆盖"标记。Profile Overrides 卡片并入其中。
- sidebar 中 profile 行以 profile 名字为主标签（见 `ui-redesign.md` §3.2）。

### 3.7 Descriptor：让差异由能力决定

扩展现有的 `ScenarioDescriptor`（目前只有 `id / labelKey / descKey / path / icon / hideable` 这些元数据）：

```ts
interface AgentCapabilities {
  audience: 'tool' | 'sdk' | 'team';
  install?: { commands: {label: string; cmd: string}[]; links?: Link[] };
  apply: { kind: 'auto' | 'manual' | 'none'; settings?: ComponentType; guide?: ComponentType };
  detectDrift?: boolean;              // 有读回接口的才为 true
  routing: { kind: 'fixed-slots' | 'free'; modes?: ('unified' | 'separate')[]; context1M?: boolean };
  variants?: 'profiles';
  extraActions?: Action[];            // 如 DSH 的 Open Web UI
}
```

| Agent | 安装 | 应用 | 检测 drift | 路由 | 变体 |
|---|---|---|---|---|---|
| Claude Code | npm | auto | 是（`settings.json`） | fixed-slots，unified / separate，1M | profiles |
| Codex | npm | auto | 是（`config.toml`） | free，1M | — |
| OpenCode | npm | auto | 待确认（`opencode.json`） | free | — |
| DSH | 链接 | auto | 是 | free | — |
| Claude Desktop | — | manual（生成 JSON） | 否 | free，1M | — |
| Pi / VS Code | 链接 | manual | 否 | free | — |
| Cursor / Xcode | — | manual | 否 | free | — |
| OpenAI / Anthropic / Embed / Custom | — | none（给代码片段） | 否 | free | — |
| Team | — | none（分发 Team Keys） | 否 | free | teams |

有了这张表，5 个自己排版的页面都可以收敛到模板 + descriptor。现有的 `ScenarioPage` 骨架和 slot 机制已经完成了一半。

**已落地的形态**（2026-09-30，与上面的草案有出入的地方）：

- descriptor 与导航用的 `ScenarioDescriptor`（`scenarioRegistry.tsx`）分开放。后者被 nav 静态引用，必须轻；页面 descriptor 会引用各自的配置弹窗，所以每个 descriptor 写在自己的 `Use*Page.tsx` 里，跟着页面一起懒加载（见 `frontend/CLAUDE.md` 的 code-splitting 规则）。
- `setup.kind`：`none` / `guide` / `auto`，对应草案的 `apply.kind`。标题栏按钮文案由它决定（Setup Guide / Auto Config），页面不再自己选。`auto` 的 `apply(t, ctx)` 是一键写配置，`ctx` 带当前规则和槽位模式；加载状态由 `AgentPage` 统一管理，弹窗里的自定义写入走 `slot.runApply` 共用同一个状态。
- 弹窗仍是每个 Agent 自己的组件，通过 `renderDialog(slot)` 接入；关闭时统一清掉待处理的 1M 变更。
- `slotRouting`（草案的 `routing.kind: 'fixed-slots'`）：目前只有 Claude Code。Unified / Separate 的切换、确认框和按模式加载规则都在 `hooks/useSlotRouting.tsx`；这类规则不能增删、不能停用。
- `headerLinks`：DSH 的 Open Web UI；有链接时配置按钮退为 outlined。

### 3.8 顺手修掉的小问题

- Base URL 和 API Key 始终两行都显示，不做 tab。
- 运行位置改成带文字的选择：`本机 localhost` 和 `Docker host.docker.internal`，直接显示会变化的那部分值（P5）。
- Claude Desktop 的规则只从路由区增删，弹窗只负责生成 JSON。
- `TemplatePage` 不再重复请求规则。

---

## 4. 落地顺序

| 步骤 | 内容 | 用户感知 | 可回退性 |
|---|---|---|---|
| **A 不改布局的修复** | 路由图截断；空插件面板改为标签；`Config` / `Auto Config` 命名统一；Base URL 和 Key 两行显示；运行位置加文字；Quick Start 状态存服务端；规则重复请求 | 很小 | 每项独立 |
| **B 客户端配置状态（纯增量）** | 标题栏加状态指示，先做 Claude Code（接口已有），再做 Codex / DSH；旧的 toast、确认框、1M 弹窗保留一个版本，然后移除 | 小，多一个信息 | 独立 |
| **C 验证步骤** | 用"第一个请求"替代自我报告，保留手动跳过 | 小 | 独立 |
| **D 模板 + descriptor** | 逐页迁移，每页一个开关：SDK 页 → 手动工具 → Codex / OpenCode / DSH → Team → Claude Code + Profile（最后） | 中 | 每页独立开关 |
| **E 分区调整** | 模式切换移入路由区；插件默认值行；Profile 变体切换；接入区折叠摘要 | 中 | 随 D 的开关 |

保留不动：路由图本身、Test / Troubleshoot、Connect AI、EntryGuide / TierGuide。

每一步都用 `ui-preview` 在 mock 模式下对 16 个页面各截一张前后对比，宽度用 1440、1200（Wails 默认）和 960。

---

## 5. 与 `ui-redesign.md` 的关系

- `ui-redesign.md` 的 §3.4（接入 / 路由 / 活动三分区）以本文为准：先做"接入 / 路由"两区，活动区暂缓。
- 两份文档共同依赖三件基础设施：UI 状态存服务端（Quick Start 进度、隐藏场景）、route registry、Host Bridge。
- 右下角浮动的 `!`（`FloatingStatusIndicators`）显示的是断线和有新版本，开发模式下强制显示。建议移进 layout：断线状态放在 rail 顶部 logo 旁的状态点，新版本放在 rail 底部用户菜单上的角标，不再浮在内容区上方。

---

## 6. 待决问题

1. **Auto Config 的模型槽位要不要保留可编辑？** 建议默认只读、由规则推导，另设"自定义槽位"开关作为逃生口；需要确认是否有用户依赖手填槽位。
2. **团队部署或远程网关的场景**：drift 检测只在网关和工具在同一台机器时成立。其他情况一律显示"无法检测"，是否可以接受？
3. **Unified / Separate 的叫法**：是否改为直接描述结果（"5 个槽位用 1 条规则 / 5 条规则"）？这会动到现有的 i18n 与用户习惯。
4. **活动区**：是在 Agent 页内做一个简版，还是只链接到过滤后的 Dashboard 和日志？
