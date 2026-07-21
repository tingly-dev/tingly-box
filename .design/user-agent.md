# User-Agent 处理与优先级

> 适用对象：tingly-box 后端贡献者。
> 本文档是出站 `User-Agent` 行为的**权威说明**。rule flag 侧的注入机制见
> `.design/rule-flags.md`（尤其 §8）；本文聚焦"最终 wire 上发出什么 UA"。

---

## 0. 一句话模型

tingly-box **不是透明转发代理**：它解析入站请求体、经 transform chain、再用
vendor SDK（openai-go / anthropic-sdk-go / go-genai）**重建**出站请求。出站
`User-Agent` 是**合成**的，不是把 client 的头原样透传。

**核心原则：UA 是请求链路的关注点,不是 provider 配置。** 没有任何 provider 级
的 UA 字段(历史上的 `provider.UserAgent` 已彻底移除,理由见 §5)。走哪条 client
实现决定适用哪一套规则,有两套,泾渭分明:

- **A. 通用 pass-through client**（generic OpenAI、generic 非-OAuth Anthropic）——
  UA 完全由**请求侧**决定:rule/scenario `custom_user_agent`,或兜底转发**入站 client UA**。
- **B. 内建特种（vendor）client**（Claude Code OAuth / Kimi / Gemini / Antigravity /
  Codex）——**vendor 特种握手 UA 是唯一且决定性的**。rule/scenario `custom_user_agent`
  与入站 client UA **完全不参与**,也没有任何 provider 配置能覆盖它。

> ⚠️ 别把两套写成一条链。vendor 特种 UA 不是"某条线性链的最弱兜底",而是特种链上
> **决定性的、不可被配置覆盖**的值。

---

## 1. 两套链，分别看

每层都是"set then delegate"，**innermost（最靠近 wire）wins**。

### A. 通用 pass-through client

链（自外向内）：

```
   wrapWithLogging                       ← 只记录，不改 UA
     userAgentTransport                  ← 一处解析固定优先级
       base transport → wire
```

**优先级(在 `userAgentTransport` 一个 RoundTrip 内解析,不靠多层叠放):**
`rule/scenario custom_user_agent` > `入站 client UA` > `SDK 默认 UA`。

- 只有这两条通用链接入 `userAgentTransport`。它读 ctx 里的两个候选值
  (`GetCustomUserAgent` / `GetClientUserAgent`),显式按上面顺序取胜者——**不是**用两个
  叠放的 transport(那样"包裹顺序"与"执行/优先级顺序"相反,极易读错)。
- client 入站 UA 只在 rule/scenario override 为空时才用。
- 两者都空且 client 也没发 UA → 保留 SDK 默认。

### B. 内建特种（vendor）client

链（自外向内）：

```
   vendorRoundTripper                    ← 设 vendor 特种 UA（Codex 例外：不设 UA）
     createSessionBoundTransport         ← 只做 session 绑定,不碰 UA
       SessionBoundTransport → wire
```

**唯一的 UA 来源:vendor 特种 UA(决定性)。**

- `userAgentTransport` **不在这条链上**——没有任何 transport 去读 rule/scenario
  custom_user_agent 或入站 client UA。Gemini 更是
  `req.Header = http.Header{}` 把整个 header 清空后重设,client 的一切头都没了。
- `createSessionBoundTransport` 现在**只做 session 绑定**,不再包 UA override。
- 所以 client 发什么 UA、rule 配什么、任何 provider 配置,**都动不了 vendor 特种 UA**。
- Codex 例外:其 round-tripper 不设 UA,所以发的是 **OpenAI SDK 默认 UA**(详见 §3)。

---

## 2. 通用链结果矩阵（A 类）

以 client 入站发了 `cherry-studio/1.2` 为例（"SDK 默认"指 openai-go / anthropic-sdk-go 自带 UA）：

| rule/scenario UA | client 入站 UA | 实际发出（wire） | 说明 |
|------------------|----------------|------------------|------|
| 空 | 空 | **SDK 默认** | 什么都没配、client 也没发 |
| 空 | `cherry-studio/1.2` | **`cherry-studio/1.2`** | ⭐ 兜底转发:尊重真实 client |
| `Bench/1` | `cherry-studio/1.2` | **`Bench/1`** | rule/scenario override 赢 client |
| `none`（哨兵） | 任意 | **（无 UA 头）** | 显式 strip,见 §4 |

> scenario `custom_user_agent` 与 rule `custom_user_agent` 合并进同一个 `CustomUserAgent`
> 值(rule 非空时赢,否则继承 scenario),由 `userAgentTransport` 当作"rule/scenario
> override"处理。因此**已配置的 scenario 默认 UA 仍会覆盖 client 入站 UA**——"显式配置
> > 尊重 client > SDK 默认"。

## 2b. 特种链结果（B 类）

vendor 特种 client 恒定:

| 场景 | 实际发出（wire） |
|------|------------------|
| 任意 rule/client/配置 | **vendor 特种 UA**(Claude Code OAuth / Kimi / Gemini / Antigravity);Codex → SDK 默认 |

> 特种链没有任何可配置的 UA override 入口——这是**有意的**(见 §5)。

---

## 3. 各内建 client 路径一览

| Client 路径 | 类型 | rule/scenario UA | client 入站 UA | wire 上的 UA | 代码 |
|-------------|------|:---:|:---:|--------------|------|
| 通用 OpenAI（`NewOpenAIClient`,无 vendor 覆盖) | A 通用 | ✅ | ✅ | rule/scenario > client 入站 > openai-go SDK 默认 | `internal/client/openai.go` |
| 通用非-OAuth Anthropic（`NewAnthropicClient` else 分支) | A 通用 | ✅ | ✅ | rule/scenario > client 入站 > anthropic-sdk-go SDK 默认 | `internal/client/anthropic.go` |
| Claude Code OAuth（`claudeRoundTripper`) | B 特种 | ❌ | ❌ | `claude-cli/2.1.86 (external, cli)`(决定性) | `internal/client/claude_round_tripper.go:135` |
| Kimi（`NewKimiClient`) | B 特种 | ❌ | ❌ | `KimiCLI/1.10.6`(决定性) | `internal/client/kimi_round_tripper.go:13` |
| Gemini（`NewGeminiClient`) | B 特种 | ❌ | ❌ | `GeminiCLI/0.1.0 (linux; amd64)`(决定性) | `internal/client/gemini_client.go:145` |
| Antigravity（`NewAntigravityClient`) | B 特种 | ❌ | ❌ | `antigravity/1.11.5 windows/amd64`(决定性) | `internal/client/antigravity_client.go:146` |
| Codex / ChatGPT backend（`codexRoundTripper`) | B 特种 | ❌ | ❌ | **openai-go SDK 默认**(round-tripper 不设 UA) | `internal/client/codex_round_tripper.go` |

> "rule/scenario UA ❌"与"client 入站 UA ❌"表示这些链上**没有对应 transport 去读**,
> 不是"读了但优先级低"——它们对 vendor 链完全不可见。

**Codex 的特殊说明**：`codexRoundTripper` **不在 model 请求上设 User-Agent**。
`codex_cli_rs` 只是 OAuth token 交换的 **body `originator` 参数**（`ai/oauth/hook.go`），
`codex-cli` UA 只用于 quota 拉取（`ai/quota/fetcher/codex.go`）——都不是 model 请求的
wire UA。ChatGPT backend 靠 OAuth token + `ChatGPT-Account-ID` 头 + `originator` body
识别,而非 UA。

> registry 预设（`internal/typ/flag_registry.go::DefaultUserAgents`）里列出的
> `claude-cli/…`、`codex_cli_rs/0.20.0` 等是给 `custom_user_agent` flag 的**快选建议**
> ——它们是"通用链上可选择去冒充的 UA",**不代表对应 vendor 链默认就发这个值**,也不代表
> `custom_user_agent` 对 vendor 链有效(对 vendor 链无效,见 §5)。

---

## 4. `none` 哨兵 — 发送"无 User-Agent"（仅 A 类）

`custom_user_agent = "none"`（`typ.UserAgentNone`）表示**完全去掉** User-Agent 头，
仅对**通用链**有效(vendor 链不读 custom_user_agent)。

实现细节（`custom_ua_transport.go`）：net/http 在头**缺失**时会注入默认
`Go-http-client/<ver>`；只有把头设成**显式空串**才能让请求真正不带 UA。所以哨兵把
`User-Agent` set 为 `""`（present-but-empty），而非删除。

`none` 是 rule/scenario override 值,`userAgentTransport` 优先取它,所以即便 client 发了
UA,`none` 依旧 strip 掉——显式 strip 是通用链里最高优先的意图。

---

## 5. 为什么没有 provider 级 UA / vendor 特种 UA 决定性（不变量）

**历史包袱与移除**:早期有一个 `provider.UserAgent` 字段,作为"调试 override"被包成
`wrapWithUserAgent` 塞进传输链,而且经由 `createSessionBoundTransport`(所有 vendor 链的
公共底座)注入,能**盖过 vendor 特种握手 UA**。这是设计问题:

- provider 是**静态配置**,和"请求在链路上如何呈现身份"是两个正交的轴;把 provider 配置
  塞进请求传输链,就让它有了伸手改写 vendor 握手/指纹 UA 的能力——既是 footgun(Claude
  OAuth 错配即被拒),又让 vendor 特种 UA 名不副实地"可被覆盖"。
- UA 在请求链路上只该有两个合法来源:**调用方是谁**(client 入站 / rule-scenario
  custom_user_agent)和 **vendor 协议要求什么**(vendor pin)。

因此 `provider.UserAgent` 字段**已彻底移除**(struct 字段、`wrapWithUserAgent` /
`userAgentRoundTripper`、DTO、handler、前端表单项全部删除),`createSessionBoundTransport`
回归"只做 session 绑定"的单一职责。结果:

- **vendor 特种 UA 现在是决定性的、不可被任何配置覆盖**(满足"内建特种 client 最高优")。
- 通用链只剩纯请求侧来源(rule/scenario > client 入站 > SDK 默认)。

**边界如何保证**(重要不变量):

- `applyClientUserAgent` / `applyCustomUserAgent`(都由 `resolveRuleFlagsWithScenario`
  统一调用)对**所有**请求都把 UA 写进 `c.Request.Context()`,但这两个 UA 只在传输链里
  **有** `userAgentTransport` 的 client 上被读取。
- `userAgentTransport` **只**接入通用 `NewOpenAIClient`(openai.go)与通用非-OAuth
  Anthropic 分支(anthropic.go else)。
- vendor 链虽内部复用 `NewOpenAIClient`(Kimi / Codex),但用 `extraOptions` 里自带的
  `WithHTTPClient`(含 `kimiRoundTripper` / `codexRoundTripper`,**不含** `userAgentTransport`)
  在 SDK option 末尾覆盖掉通用 httpClient("extra 最后应用");Gemini / Antigravity /
  Claude OAuth 自建 transport 链。这些 vendor RT 都包在 `createSessionBoundTransport`
  **外层**,而后者不碰 UA。
- 所以即便 ctx 里带着 UA,vendor client 也没有任何 transport 去读它——vendor 特种 UA
  岿然不动。

> ⚠️ **给 vendor 链新增 transport 时,切勿引入 `userAgentTransport`;也不要恢复任何
> provider 级 UA override。** 这些都会击穿 vendor 特种 UA 的决定性。若确需为某个 vendor
> 端点冒充别的 UA,应改那条 vendor RT 里硬编码的特种 UA 常量,而不是从请求链路旁路注入。

---

## 6. 代码地图

| 关注点 | 位置 |
|--------|------|
| client UA ctx helper | `internal/typ/id.go`（`WithClientUserAgent` / `GetClientUserAgent` / `ClientUserAgentKey`）|
| rule/scenario UA ctx helper + `none` 哨兵 | `internal/typ/id.go`（`WithCustomUserAgent` / `GetCustomUserAgent` / `UserAgentNone`）|
| UA 解析 transport（A 类,rule/scenario override + client 兜底转发,一处解析优先级）| `internal/client/custom_ua_transport.go`（`userAgentTransport`）|
| session 绑定底座(A + B,不碰 UA) | `internal/client/http.go`（`createSessionBoundTransport` / `SessionBoundTransport`）|
| 通用 OpenAI 链装配（A）| `internal/client/openai.go`（`NewOpenAIClient`）|
| 通用 Anthropic 链装配（A）| `internal/client/anthropic.go`（`NewAnthropicClient` else 分支）|
| vendor 特种 RT（B）| `claude_round_tripper.go` / `kimi_round_tripper.go` / `gemini_client.go` / `antigravity_client.go` / `codex_round_tripper.go` |
| 解析合并 + 挂 ctx（唯一合并点） | `internal/server/rule_flags.go`（`ResolveRuleFlagsWithScenario` → `applyCustomUserAgent` / `applyClientUserAgent`）|
| UA 预设快选(通用链 flag) | `internal/typ/flag_registry.go`（`DefaultUserAgents`）|
| 入站 UA 仅做检测的地方 | `internal/server/user_agent.go`（Cursor 检测）；`internal/server/middleware/multi_mode_memory_log.go`（审计日志）|
| 测试 | `internal/client/custom_ua_transport_test.go`；`internal/server/rule_flags_test.go` |

---

## 7. 相关文档

- `.design/rule-flags.md` §8 — rule flag 视角下的 UA 链层与注入机制。
