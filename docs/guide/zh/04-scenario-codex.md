# Codex 场景

路径：`/agent/codex`

Codex 场景将 OpenAI Codex CLI 的请求代理到你配置的 Provider。页面是标准的 Agent 结构——带状态行和按钮的页头、连接卡、Plugins 卡、Quick Start、Model Rules——详见 [Claude Code](./03-scenario-claude-code.md#页面结构)；本章只写 Codex 特有的部分。

---

![Codex 场景](../images/codex.png)

## 与 Claude Code 的不同之处

- **Auto Config** 写入 `~/.codex/config.toml` 和 `~/.codex/auth.json`（以及可选的模型目录）——状态行会读回它们，所以这里同样会出现 `Config out of date · N differences`
- **Model Rules 是自由形式的**：没有槽位。**New Model** 可以新增规则（见下文），每条规则都可以删除或关闭
- Quick Start 第 3 步安装 CLI：`npm install -g @openai/codex`（官方源和镜像源命令）
- 每条规则的标题上有 **`1M: Off ⌄`**、**`Endpoint: Auto ⌄`**，以及仅在本页出现的 **`Compact: Auto ⌄`**。切换 **1M** 会打开带「待应用变更」提示的 Auto Config 对话框（Codex 没有直接重新应用）；在里面重新应用并重启 Codex

## New Model：一次添加多个模型

**New Model**（Model Rules 右上角）以「创建规则」模式打开模型选择器：*Select a model for your new rule*。打开模型工具栏上的 **Select multiple** 开关，标题变成 *Create a rule for each model*：跨 Provider 随意点击多个模型——被选中的会打勾——然后用底部栏，`3 models selected` → **Add 3 models**（**Clear** 清空已选）。每个被选的模型会成为一条独立的规则，以模型名命名（重名时加一个短后缀），并以该模型作为服务。

![New Model 多选](../images/new-model-multi.png)

## 按模型设置自动压缩

规则标题上的 **Compact** 标签设置 Codex 何时压缩*该模型*的上下文：

![Compact 弹层](../images/codex-compact.png)

- 拖动滑块（50–100 %）选择触发点；弹层显示该模型的 **Window**、**Compacts at** 位置，以及压缩时 **Free when compacting** 的剩余 token
- 不动它就跟随 **Auto Config** 的值（`Following Auto Config (75%)`）；**Use Auto Config value** 可重置
- 它只写进 Codex 配置（模型目录），不影响请求——所以**要在重新应用 Auto Config 后才生效**
- 当留给长工具输出的空间太少，或可用窗口限制了该值时，会给出提示

## Auto Config 对话框

![Configure Codex](../images/codex-config-modal.png)

**Quick Config** 标签页（右上角的重置图标恢复默认值）：

- **Authentication**：*Tingly Box gateway*（网关密钥写入 `~/.codex/auth.json`）、*Tingly Box gateway + keep official ChatGPT login*（网关密钥写进 `config.toml`，`auth.json` 里的 ChatGPT 登录保持不变，Codex App 仍能识别你的账号）或 *Direct to OpenAI*（用官方 ChatGPT 登录直连，不走网关）。后两种可以选择要写入的已保存 **ChatGPT account**（某个 Codex OAuth Provider），或保留现有的 `auth.json`
- **Files · Write model catalog**：写入 `~/.codex/tingly-model-catalog.json`，让 Tingly-Box 的模型出现在 Codex 的 `/model` 选择器里（`model_catalog_json`）；关闭则 Codex 使用内置列表
- **Model & reasoning**：留空表示「用 Codex 默认值」：**Reasoning effort**（`none` … `xhigh`、`max`、`ultra`；Tingly-Box 默认 `medium`）、**Reasoning summary**（`auto` / `concise` / `detailed` / `none`）、**Verbosity**（`low` / `medium` / `high`）、**Force reasoning summaries**（`supports_reasoning_summaries`，写入每个目录条目——代理的模型需要它才能返回摘要；仅在开启目录时生效）、**Usable window (%)**（`effective_context_window_percent`，50–100，默认 82）和 **Auto-compact at (%)**（默认 75；不会高于可用窗口；是所有模型的默认值，可用 Compact 标签按模型覆盖）

**Manual** 标签页：由服务端渲染的真实 `config.toml`、`auth.json` 和模型目录，供手动复制粘贴使用。

## 配置流程

1. 在[凭证管理](./08-credentials.md)中添加 Provider（或在 Quick Start 里点 **Connect AI**）
2. 安装 Codex（Quick Start 第 3 步）
3. **Auto Config** → 选择认证方式 → **Auto Config**
4. 可选：用 **New Model** 添加更多模型，并按模型调整 **Compact**
5. 使用 Codex；状态行显示 `Connected`

---

## 相关页面

- [Claude Code 场景](./03-scenario-claude-code.md)
- [其他编码 Agent](./04-scenario-coding-agents.md)
- [凭证管理](./08-credentials.md)
