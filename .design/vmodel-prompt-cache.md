# vmodel — prompt cache simulation (`virtual-prompt-cache`)

> 适用对象：排查"缓存命中率低"的人；改 `internal/protocol/request`（协议转换）、`internal/protocolserver/transform`（clean_header 等 source 半段改写）、`internal/protocol/stream` / `usage`（usage 回写）的人。
> 代码：`vmodel/promptcache/`（模拟器）、`vmodel/virtualserver/prompt_cache.go`（接线）、`internal/protocoltest/cache_hit_rate_test.go`（端到端回归）。

## 动机

"Claude Code stream 模式下 cache 命中率极低"这类报告，原因可能落在四处，而现有手段只能各看一段：

| 可能的原因 | 现有手段 | 盲区 |
|---|---|---|
| 客户端自身每轮改写前缀 | 抓包 | — |
| 网关改写历史（转换形态漂移、动态 header 未清理） | `cache_prefix` 段（逐项 diff 两轮请求） | 只证明"前缀稳定"，不出一个命中率数字 |
| usage 在回写时丢了 cache 字段（stream 转换 / 计费） | 各 converter 单测 | 没有一条从上游 cache_read 到客户端、到 Dashboard 的端到端断言 |
| 上游 provider / 路由本身（换 key、failover、provider 不缓存） | 无 | — |

`virtual-prompt-cache` 把前三段串成一个数字：它是一个**理想的前缀缓存 provider**，命中多少完全由收到的请求决定，并按真实协议形态在 usage 里报回去。经网关打到它，客户端（Claude Code `/cost`、statusline）和 Dashboard 读到的命中率，就是"网关这条路径允许的上限"：

- 它低 → 请求前缀在客户端或网关被改了（回复里直接写出第一个未命中的 block）。
- 它高、真实 provider 低 → 问题在 provider 或路由（多 key / failover / session affinity 没开）。

## 2026-10 排查结论（这个模型的由来）

用本机真实 Claude Code 2.1.293 对抓包服务器跑多轮（含 tool loop 与 `--continue`），再把抓包经网关回放到各 target：

1. **客户端前缀稳定**：去掉 `cache_control` 后，连续请求的 `system` / `tools` / `metadata` / 历史消息逐字节一致。
2. **动态 billing header 在会话内不变**：代理模式下是 `x-anthropic-billing-header: cc_version=2.1.293.<fp>; cc_entrypoint=…;`（没有 `cch` / `cc_prev_req` / `cc_prompt_id`，这些只在直连时出现），`fp` 由首条用户消息算出，会话内不变、跨会话（含 subagent）不同。`clean_header` 按 `x-anthropic-billing-header:` 前缀剥离，对 2.1.293 仍然有效；claude_code 场景下各 target 上游都收不到它。
3. **网关请求侧前缀稳定**：claude_code 场景 × {Anthropic、OpenAI Chat、Responses} × {stream, nonstream} × {有无 thinking} × {generic、DeepSeek、Kimi、GLM、OpenAI 官方 host}，逐轮增长全部保持前缀。
4. **stream 回写不丢 cache**：上游 Anthropic（start/delta 各种分布）、Chat（独立 usage chunk、DeepSeek 式 usage 与 finish_reason 同块）、Responses，stream 与 nonstream 的客户端 usage 与落库 usage 一致。Claude Code 从终态 `message_delta` 取 `input_tokens` / `cache_read_input_tokens`（实测），所以 OpenAI→Anthropic 转换把完整 usage 放在终态 delta 是对的。
5. 发现并修复一处 stream 专有的计数错误：`AnthropicAccumulator` 把 `cache_creation_input_tokens` 在每个携带它的事件上都累加进 input，`message_delta` 重复上报 creation 但不带 `input_tokens` 时 input 被算两遍、命中率被压低（见 `usage-tracking.md` §2.3）。
6. 剩余未覆盖的 usage 形态：只在顶层 `usage.cached_tokens` 报命中的旧版 Moonshot 接口会被读成 0（DeepSeek 与当前 Kimi 都同时报 `prompt_tokens_details.cached_tokens`，不受影响）。

结论：网关本身不是"stream 下命中率低"的原因；若真实环境仍低，优先看路由（规则多 service 且无 session affinity、failover 切换账号）、非 claude_code 场景的同协议直通（billing header 不剥，跨会话共享的 system 前缀失效），以及 provider 自身的缓存行为。这个模型就是用来把这几类原因区分开的。

## 设计

### 模拟的是"理想 provider"，不是某一家

- 粒度是一个 prompt block：一个 tool 定义、一个 system block、一个 message content block（Anthropic）/ 一条 message（Chat）/ 一个 input item（Responses）。不模拟 64/128 token 的 chunk 粒度、最小可缓存长度、Anthropic 的 20-block 回看上限。
- TTL 固定 1 小时，不读请求里的 `ttl`。
- block 的 key 是去掉 `protocol.PromptCacheHintFields`（`cache_control`、`prompt_cache_*`）后的 canonical JSON；Anthropic 的 role 并入每个 block 的 key。移动 breakpoint 不改变 key，这与 `cache_prefix` 段的比较口径一致。
- 前缀用链式哈希（`h_i = H(h_{i-1}, block_i)`）存储，查找是 O(blocks)；条目数有上限（20 万），满了先清过期、再整表清空 —— 诊断用的缓存可以忘，不能无界增长。

目的是让命中率只反映"前缀是否稳定"这一件事；provider 特有的折扣在这个数字之外。

### 两种缓存纪律

| 端点 | 纪律 | 写 | 读 |
|---|---|---|---|
| Anthropic Messages | Explicit | 只写以 breakpoint 结尾的前缀（顶层 `cache_control` = 最后一个 block 是 breakpoint） | 只能读到本请求最后一个 breakpoint 为止 |
| OpenAI Chat / Responses | Automatic | 每个前缀都写 | 任意前缀可读 |

Explicit 的意义：网关如果在发往 Anthropic 形态上游时丢了 `cache_control`，真实 Anthropic 就一点不缓存，模拟器同样报 0 —— 这类回归在 Automatic 下是看不出来的。

usage 按协议形态报回：Anthropic `input_tokens` 不含读写，`cache_read_input_tokens` / `cache_creation_input_tokens` 分列（message_start 与 message_delta 都带）；Chat / Responses 的 `prompt_tokens` / `input_tokens` 是总数，命中在 `*_details.cached_tokens`，不报写入（自动缓存写入不计费）。

### 为什么在 virtualserver 里做，而不是在 model 里

模拟需要上游收到的**原始请求**，而 vmodel 的 model 只拿到解码后的结构体 —— Responses 端点甚至只把最后一条用户文本交给 model。所以 `vmodel.PromptCacheModel` 只是个标记接口（暴露自己的 `*promptcache.Simulator`），handler 在三个端点上读原始 body、跑模拟，然后：

1. 把 model 换成一次性的 `MockModel`，回复文本是模拟报告（`Result.Report()`，含命中率与第一个未命中的 block 标签）；
2. 把模拟 usage 作为 preset 交给 render，替换原本的 token 估算。

这样 model 接口不变，其它 vmodel 的行为也不变（preset 为 nil 时走原逻辑）。缓存按 `端点/模型` 分区；Anthropic 与 OpenAI 注册表各有一个实例、各自一个模拟器。

### 注册为用户可见的默认模型

`virtual-prompt-cache` 进了 `anthropic.RegisterDefaults` / `openai.RegisterDefaults`，即 `/virtual/*` 与内置 virtual provider 可见。它满足注册纪律（README "Positioning & registration discipline"）：协议合规、命名直白、对用户有用 —— 它就是一个诊断工具，而诊断必须走真实路径（`ux-principles.md`）。

## 怎么用

1. 建一条 claude_code 场景的规则（或临时改内置 Claude Code 规则的 service），service 指向内置 virtual provider 的 `virtual-prompt-cache`；要测某个 target 协议，就用对应风格的 virtual provider（Anthropic / OpenAI Chat / OpenAI Responses）。
2. 用 Claude Code 正常对话几轮。每次回复形如：
   `Prompt cache simulation: 48000 of 50000 prompt tokens read from cache (96.0%). Cached prefix: 41 of 43 blocks; first uncached block: messages[12].content[0].`
   第一个未命中的 block 正常情况下是本轮新增的那条；如果落在 `system[…]` / `tools[…]` / 很早的 `messages[…]`，就是那里被改写了。
3. 同时看 Dashboard / `/cost` 的命中率：与回复里的数字不一致，说明 usage 回写路径丢了字段。

## 回归测试

- `vmodel/promptcache`：纪律、TTL、分区、breakpoint 不影响 key、system 变化导致失配。
- `vmodel/virtualserver/prompt_cache_test.go`：三个端点 × stream/nonstream 的增长对话；Anthropic 无 breakpoint 不命中。
- `internal/protocoltest/cache_hit_rate_test.go`（`TestCacheHitRate`）：claude_code 场景经网关到 `virtual-prompt-cache`，三个 target × 两种 stream 模式：
  - 增长：第 N+1 轮客户端读到的 `cache_read` 必须**恰好等于**第 N 轮的整个 prompt；落库 usage 与客户端一致。
  - 跨会话：同样的历史、不同的 billing header 指纹，必须 100% 命中 —— 这条直接守住 `clean_header`（临时关掉剥离时三个 target 全部失败，命中率掉到 13–21%）。

## 已知边界

- 不模拟 provider 的最小长度 / chunk 粒度，所以对很短的 prompt 命中率会高于真实值。
- 只看前缀，不看路由：网关把同一会话分到不同 provider / key 时，真实缓存会失效，而这里每个 virtual provider 共用同一个模拟器。路由层面的问题要看 `session-affinity` 与 failover 日志。
