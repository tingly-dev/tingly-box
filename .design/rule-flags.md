# Rule Flags 设计与实操

> 适用对象：tingly-box 后端 / 前端贡献者。
> 历史回溯：2026-05 引入 registry，把原本仅 2 个布尔 flag 的隐藏入口
> 升级为 catalog 化的"路由规则扩展"系统。

---

## 1. 为什么需要 rule 级 flag

我们已有的三层语义：

| 维度 | 粒度 | 例子 | 现状 |
|------|------|------|------|
| Provider | provider 实例 | `user_agent`、`api_base`、`proxy_url`、`timeout` | 全局静态生效 |
| Scenario flags | scenario | `disable_stream_usage` 等 | 跨 rule 共享 |
| **Rule flags** | **单条 rule** | **`cursor_compat`、`skip_usage`、`use_max_completion_tokens` …** | **本文档讨论** |

很多场景介于"provider 通用"和"协议层通用"之间——它们只对**某一类客户端 / 某一类模型 / 某一种调试目的**有意义。把这些行为塞到 Provider 配置里会污染默认值；做成 Scenario flag 又过于粗粒度。Rule flag 是这一类"局部、可选、可叠加的开关"的归宿。

设计原则：

1. **可发现**：UI 必须能列出所有可选 flag 及其语义，而不是让用户照着源码猜 key 名。
2. **可叠加**：同一条 rule 可同时启用多个 flag，且语义互不依赖。
3. **可扩展**：新增 flag 只在**一处**注册元数据（`flag_registry.go`），后端行为代码 + 前端 UI 不应硬编码 flag 列表。
4. **不污染默认**：未启用时必须是 zero-cost no-op，绝不影响其它 rule 的正常路径。

---

## 2. 整体架构图

```
              ┌─────────────────────────────────────────────────────────┐
              │                       Frontend                           │
              │  ┌────────────────┐    GET /rule/flags/registry          │
              │  │ FlagCatalog    │ ◄────────────────────────────┐       │
              │  │ Dialog         │                              │       │
              │  └────────┬───────┘                              │       │
              │           │ Switch / TextField per FlagSpec       │       │
              │           ▼                                       │       │
              │  ┌────────────────┐  POST /rule/:uuid (flags={})  │       │
              │  │ RuleExtensions │ ─────────────────────────┐    │       │
              │  │ Card (right    │                          │    │       │
              │  │  of GraphRow)  │                          │    │       │
              │  └────────────────┘                          │    │       │
              └──────────────────────────────────────────────┼────┼───────┘
                                                             │    │
                                                             ▼    │
              ┌─────────────────────────────────────────────────┐ │
              │                    Backend                       │ │
              │                                                  │ │
              │  internal/typ/flag_registry.go ──────────────────┤ │
              │      RuleFlagRegistry() []FlagSpec  ◄────────────┘ │
              │                                                    │
              │  internal/typ/type.go                              │
              │      Rule.Flags = RuleFlags{ … typed fields … }    │
              │                                                    │
              │  Persisted as JSON column on the rule row.         │
              │                                                    │
              └─────────────────────┬──────────────────────────────┘
                                    │ rule resolved at request time
                                    ▼
              ┌─────────────────────────────────────────────────────┐
              │   internal/server/openai.go : OpenAIChatCompletion   │
              │   ── ruleFlags := resolveRuleFlags(rule)             │
              │   ├─ applyMaxCompletionTokensRewrite(&req)           │ ← request mutation
              │   ├─ WithCustomUserAgent(ctx, ruleFlags.UA)          │ ← context-passed
              │   ├─ reqCtx.Extra["skip_usage"] = …                  │ ← downstream hint
              │   └─ (existing) applyCursorCompatFlag                │
              └─────────────────────┬───────────────────────────────┘
                                    │
                ┌───────────────────┼───────────────────┐
                ▼                   ▼                   ▼
          request mutation     transport chain      response mutation
        (max_tokens rewrite,  (UA injection via   (skip_usage strips
         cursor norm. etc.)    customUA RT)        `usage` field in
                                                   stream + nonstream)
                                    │
                                    ▼
                              upstream provider
```

---

## 3. Flag Registry：唯一可信源

```go
// internal/typ/flag_registry.go
type FlagSpec struct {
    Key         string        // 与 RuleFlags 上的 json tag 完全对应
    Label       string        // UI 展示用人话
    Description string        // 鼠标 hover 详细说明
    Type        FlagValueType // bool | string
    Category    FlagCategory  // compatibility | request | response
    Placeholder string        // string 类型的输入框 hint
}

func RuleFlagRegistry() []FlagSpec { … }
```

**约束** (`flag_registry_test.go` 强制)：

- 每个 `FlagSpec.Key` **必须**对应 `RuleFlags` struct 上的某个 json tag——这条测试可以挡住"加了 spec 忘了加字段"或反过来。
- `Label` / `Description` 非空。
- `Type` 必须在已知枚举内。

---

## 4. 当前已注册 flag 一览

| Key | Type | 类别 | 作用 | 注入点 |
|-----|------|------|------|--------|
| `cursor_compat` | bool | compatibility | Cursor IDE 内容归一化 + 工具门控 + stream usage 抑制 | `cursor_compat.go` |
| `cursor_compat_auto` | bool | compatibility | 通过请求头识别 Cursor，自动启用 cursor_compat | `cursor_compat.go::resolveCursorCompat` |
| `skip_usage` | bool | response | 剥离响应中的 `usage`（流式 + 非流式 + Anthropic 转 OpenAI 路径） | `shouldStripUsage(reqCtx.Extra)` |
| `use_max_completion_tokens` | bool | request | 把 `max_tokens` 字段名重写为 `max_completion_tokens`（OpenAI o1/o3/gpt-5 系列必需） | `applyMaxCompletionTokensRewrite` |
| `custom_user_agent` | string | request | 覆盖出站 User-Agent header | `customUserAgentTransport` + `WithCustomUserAgent(ctx, …)` |

---

## 5. 三种 flag 的注入手法

```
┌────────────────────────┐
│       Request body     │   ← Type 1: 直接改 request struct
│   (e.g. max_completion │     调用位置：openai.go 在 transform 链之前
│    _tokens 重写)        │     函数形态：applyXxx(&req)
└────────────────────────┘

┌────────────────────────┐
│   Per-request context  │   ← Type 2: 通过 context 把"hint"带到深层组件
│  (e.g. custom UA)       │     调用位置：openai.go 把 c.Request 的 ctx 替换
└────────────────────────┘     消费位置：transport / round-tripper

┌────────────────────────┐
│  Response post-process │   ← Type 3: 在响应返回前剥/改字段
│  (e.g. skip_usage)      │     调用位置：protocol_dispatch.go 的派发分支
└────────────────────────┘     形态：shouldStripUsage(extra) 统一判定
```

任何新 flag 都应归入这三类之一；如果有第四类，先在 PR 描述里说明。

---

## 6. UA 链层 — 实操中最容易踩坑的部分

User-Agent 优先级：

```
   ┌────────────────────────────────────┐
   │  outer adapter (e.g. claudeRT)     │ ← vendor 硬编码 UA，先 set
   │     │  set "claude-cli/2.1.86"     │
   │     ▼ delegates to                 │
   │  wrapWithUserAgent(provider)       │ ← provider.UserAgent（调试 override）
   │     │  if non-empty: set provider  │   非空时覆盖 vendor 硬编码
   │     ▼                              │
   │  customUserAgentTransport          │ ← rule-level custom_user_agent
   │     │  if ctx has UA: set rule UA  │   非空时覆盖一切（innermost wins）
   │     ▼                              │
   │  base http.Transport (sends wire)  │
   └────────────────────────────────────┘
```

> **innermost wins**：每一层都是"set then delegate"——内层是最后写 header
> 的人，所以最贴近 wire 的那层决定最终 UA。

最终结果：

| 场景 | rule UA | provider UA | vendor 硬编码 | 实际发出的 UA |
|------|---------|-------------|--------------|--------------|
| 默认 | 空 | 空 | claude-cli/… | claude-cli/… |
| Provider 配了 | 空 | "MyOrg/1.0" | claude-cli/… | MyOrg/1.0 |
| Rule 配了 | "Bench/1" | "MyOrg/1.0" | claude-cli/… | Bench/1 |
| Rule 配了 | "Bench/1" | 空 | — | Bench/1 |

⚠️ **重要**：`provider.UserAgent` 一旦设置就会覆盖 vendor 硬编码——这是**有意为之**的调试通道（详见 `ai/provider.go` 上该字段的 doc comment）。Claude Code OAuth 等端点对 UA 有真实校验，错配会让 OAuth 直接被拒；操作员设置该字段时要清楚自己在干嘛。

并非所有 client 都接入这条链。当前只有：

| Client | Provider UA | Rule UA |
|--------|-------------|---------|
| 通用 OpenAI (`client/openai.go`) | ✅ | ✅ |
| 通用 Anthropic 非-OAuth (`client/anthropic.go` else 分支) | ✅ | ✅ |
| Claude Code OAuth (`claudeRoundTripper`) | ✅ | ❌ |
| Codex / Gemini / Google | ✅ | ❌ |

vendor-specialized 路径不接 rule UA，是因为它们 UA 跟整个 OAuth/握手协议绑定，rule 级覆盖反而会破坏 vendor 校验。这条边界写进了 `flag_registry.go` 中 `custom_user_agent` 的 description 里。

---

## 7. 前端 UI 设计

### 卡片位置

```
┌─────────────────────────────────────────────────────────────────────┐
│ Rule Card                                            [⚙ settings]    │
│                                                                      │
│  ┌──────────────────────────────── horizontal scroll ──→  ┐ ┌─────┐  │
│  │ ModelNode → ArrowNode → ProviderNode → ProviderNode … │ │ Ext │  │
│  │                                                        │ │Card │  │
│  └────────────────────────────────────────────────────────┘ └─────┘  │
│           ↑                                                    ↑     │
│   随 provider 增多可横向滚动                          常驻右侧不滚动 │
└─────────────────────────────────────────────────────────────────────┘
```

关键 CSS 决策：

- 外层 `display: flex`，graph 在 `flexGrow:1, minWidth:0` 的 box 里启用 `overflowX:auto`；`minWidth:0` 是让 flex item 能收缩到内容宽以下的关键，否则会把 Extensions 推出视口。
- Extensions Card `flexShrink:0`，所以滚动条出现时它**不缩**。
- Card 高度 = `PROVIDER_NODE_STYLES.height`（72px），视觉与 provider 行对齐；内容溢出时 card 内 `overflowY:auto`。

### Catalog 弹窗

按 `FlagSpec.Category` 分组（compatibility / request / response）。每个 flag：

- `type: bool` → 一个 `Switch`。
- `type: string` → `Switch`（占位，未来可改成独立 enable 控制）+ `TextField`。当前空字符串视为未启用。
- 已启用的 flag 在边框 / 背景上有高亮。

---

## 8. 新增一个 flag — 操作手册

按以下顺序走，**勿乱序**，否则前端会因 parser/reducer 缺分支而 cast 失败：

```
1. internal/typ/type.go
   ├─ 给 RuleFlags struct 加一个字段（snake_case json tag）
   └─ 注意 yaml tag 与 json 保持一致

2. internal/typ/flag_registry.go
   └─ 在 RuleFlagRegistry() 返回值里追加一个 FlagSpec，
      key 必须与上一步的 json tag 一字不差

3. internal/server/  (行为落地)
   ├─ 选定 §5 中的注入类型 (1/2/3)
   ├─ 在对应 helper（applyXxx / WithXxx / shouldXxx）里加分支
   └─ 在 openai.go / protocol_dispatch.go 等"枢纽"调用 helper

4. internal/server/rule_flags_test.go  或  与新逻辑同包的 _test.go
   └─ 覆盖：nil-safe / no-op when flag absent / 启用时的实际效果

5. frontend/src/components/RoutingGraphTypes.ts
   ├─ RuleFlags 接口加 camelCase 字段
   └─ RuleFlagsApi 接口加 snake_case 字段（与后端 json tag 对齐）

6. frontend/src/components/rule-card/utils.ts
   ├─ ruleToConfigRecord：snake_case → camelCase 映射
   ├─ formatRuleFlags / parseRuleFlags：扩展 string-key 兼容路径
   └─ countActiveFlags：新字段计入

7. frontend/src/components/rule-card/RuleExtensionsCard.tsx
   └─ flagBoolValue / flagStringValue switch 增加 case

8. frontend/src/components/rule-card/FlagCatalogDialog.tsx
   └─ flagToBool / flagToString / setBool / setString switch 增加 case

9. frontend/src/components/rule-card/useRuleCardHooks.ts
   └─ autoSave 的 flags 序列化：camelCase → snake_case payload

10. (可选) 重新跑 frontend `npm run gen:api`（CLAUDE.md 约定）
```

测试 `TestRuleFlagRegistry_KeysMatchStructFields` 会在第 1、2 步任一步漏改时立即红，是这条链路最稳的安全网。

---

## 9. 设计取舍

| 选项 | 已采纳 | 备择 | 取舍理由 |
|------|--------|------|----------|
| `RuleFlags` 用 typed struct 还是 `map[string]any` | typed struct | map | 编译期检查；JSON 兼容性靠 `omitempty` 与零值 |
| Registry 由后端 owner | ✅ | 前端硬编码 + 后端硬编码两边对 | 单一可信源，新加 flag 只动一处元数据 |
| 卡片放路由图内 vs. 路由图外（固定右侧） | 固定右侧 | 卡片随路由图滚动 | flag 是 rule 级而非 provider 级，常驻可见更符合心智模型 |
| string flag 的"启用"语义 | 空 = 未启用 | 独立 enable Switch + 文本 | 一个字段一个状态，UI 更简单；权衡是无法区分"空字符串"和"未配置" |
| UA 链 vendor pin 是否不可覆盖 | 否，可被 provider/rule 覆盖 | vendor pin 强制 | 把 `provider.UserAgent` 当调试 override 更灵活；用 doc comment 规约用法 |

---

## 10. 当前未做 / 后续可做

- **UI**：string flag 加独立 enable Switch（让"空"与"未启用"可区分）。
- **UI**：Catalog 加搜索框 / category collapse（flag 数量超过 ~8 个时会拥挤）。
- **后端**：`internal/client/openai.go` 通用 OpenAI client 用 `http.DefaultTransport` 而非 `createSessionBoundTransport`，导致它**没有**走 transport pool / session 隔离——这是个独立问题，不在 rule flag 范畴，但补齐时记得保留 §6 的两层 UA 包装顺序。
- **后端**：长期看，部分 ScenarioFlags 可以下沉成 rule flag（例如 `disable_stream_usage` 已与 `skip_usage` 高度重叠），等业务确认后做一次合并。
- **测试**：当前测试覆盖 helper 级，缺一个 "rule 配了 skip_usage → 真实 HTTP 响应里没有 usage" 的端到端 case；待 mock provider fixture 成熟后补。
