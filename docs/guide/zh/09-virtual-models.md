# 虚拟模型

路径：`/credentials/virtual-models`

虚拟模型（Virtual Models）是 Tingly-Box 内置的合成模型 Provider，无需真实 API Key 即可使用，适用于演示、开发调试和干运行（dry-run）场景。

## 快速开始

不用真实 API Key，也能完整试一遍 Tingly-Box。

1. 点导航栏 **Credential** → **VModel**（`/credentials/virtual-models`）。
2. 在 **Enabled** 列确认你要用的虚拟 Provider（例如 `tingly`）是开启的。
3. 打开任意 Agent 页面，比如 **Agent** → **OpenAI SDK**，在 **模型规则** 里点 **新建模型**，选虚拟 Provider 和它的某个模型。
4. 用该页的 Base URL 和 API Key 发一个请求（见 [SDK 快速开始](./05-scenario-sdk-proxy.md#快速开始)），或点规则页头的闪电图标做一次快速测试。

**验证**：能收到响应，但内容是模拟的——没有调用任何真实模型，也不消耗配额。请求依然会出现在[仪表盘](./11-dashboard.md#快速开始)里。如果侧边栏里找不到 **VModel**，请到 **Power-ups** 里把 *Virtual Models* 重新打开。

---

![虚拟模型](../images/virtual-models.png)

## 页面概览

从 **Credential → VModel** 进入（侧边栏的这一行是缩写；页面本身的标题是 **Virtual Models**）。VModel 是一个 **Power-up**——默认开启；在 [Power-ups 菜单](./02-scenario-overview.md#power-ups齿轮背后的菜单)里关闭 *Virtual Models* 后侧边栏的这一行会消失（页面仍可通过 URL 访问，这些 Provider 也继续可用）。

副标题：*Built-in synthetic model providers for onboarding, demos, and dry-runs. They respond locally without contacting any upstream.*

### 虚拟模型表格（`Virtual Models (N)`）

| 列 | 说明 |
|----|------|
| Name | 虚拟 Provider 名称（`tingly`、`tingly-anthropic`、`tingly-gemini`、`tingly-openai` 等），带 **Builtin** 徽标 |
| API Style | OpenAI / Anthropic 协议徽标 |
| Models | 它提供的合成模型名（标签） |
| Enabled | 启用/禁用开关 |

表格下方有一行说明：*Builtin providers are seeded on every startup; they cannot be deleted, only enabled or disabled here.*

---

## 使用场景

| 场景 | 说明 |
|------|------|
| **初始化演示** | 在未配置真实 Provider 时，通过虚拟模型走通完整 UI 流程 |
| **开发调试** | 测试转发规则配置是否正确，无需消耗真实 API 配额 |
| **功能演示** | 向团队演示 Tingly-Box 功能，无需暴露真实 API Key |

---

## 与真实 Provider 的区别

- 虚拟模型 Provider **内置**（每次启动都会重新写入），无法通过 UI 新增或删除
- 返回的响应为**模拟内容**，不调用真实 AI 模型
- 可通过开关**启用/禁用**单个虚拟 Provider
- 适用于 Tingly-Box 的**所有场景**（Claude Code、OpenAI、Anthropic 等）

---

## 启用方式

1. 打开 **Credential → VModel**（`/credentials/virtual-models`）
2. 找到目标虚拟 Provider
3. 将对应开关切换为**启用**

启用后，虚拟 Provider 将出现在各场景的模型路由选项中，可像真实 Provider 一样配置转发规则。

---

## 相关页面

- [凭证管理](./08-credentials.md)
- [场景总览 · Power-ups](./02-scenario-overview.md#power-ups齿轮背后的菜单)
