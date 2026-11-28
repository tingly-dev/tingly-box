---
date: 2026-11-25
status: proposal
scope: research + design; no production code
---

# 模型指纹：只通过 API 的主动探针与可比对差异分析

目标：

1. 判断远端/上游提供的模型是否就是目标模型（例如供应商偷偷换模型、换后端、降级到小模型）；
2. 对两个模型做可比对的定量/定性差异分析。

约束：只能发起普通 API 调用；读不到权重、logits。可用信号包括 logprobs（若上游支持）、采样参数、tokenizer / usage 计数、stop、流式 chunk、响应头、TTFT/吞吐、上下文窗口边界、工具调用格式、结构化输出、拒答行为、多语言/罕见字符行为。

本文所有“仓库能做什么”的结论都带文件/行号；不能从代码确认的标 **UNKNOWN**。

---

## 1. 结论（先看这一节）

**最值得先做：Tier 0（响应头 / schema / id / finish_reason / chunk 结构 / TTFT）+ Tier 1 中的 tokenizer 计数探针、工具调用格式探针、确定性输出探针；采集必须直连上游（绕过 TB 的协议转换），并为自己实际采购的模型建立小型基线库。**

理由：

- 这组信号最便宜、最稳定，且能覆盖现实中最常见的“偷换”：换厂商/换模型族、换尺寸档、换 serving 栈、静默 fallback。一个供应商把 Opus 换成 Sonnet、或把官方 API 换成兼容层，tokenizer 计数与工具调用 schema 通常会立刻露馅。
- logprobs 理论上是同族模型/量化版本最强的判别器，但可用性差（Anthropic 没有，很多 OpenAI 兼容层不返回），不应该作为第一层的基础。
- 纯文本输出探针（自报身份、知识、风格）单独用很弱：模型版本更新、provider 的默认 system prompt、采样默认值都会造成漂移。它们只适合作为聚合分数的一部分，不能单独下结论。
- 时序（TTFT/吞吐）在同模型不同硬件、不同 batch、不同负载下可差数倍，只能作为“严重不匹配”的弱证据，不能作为 MATCH 的依据。

**能区分到什么程度（诚实版本）：**

| 要区分的东西 | 仅 API 可达到的程度 | 主要手段 |
|---|---|---|
| 不同厂商 / 不同模型族 | 高 | Tier 1 tokenizer 计数 + 工具 schema + 输出行为 |
| 同族不同尺寸档（Opus vs Sonnet、70B vs 7B） | 中—高 | Tier 1 能力/格式 + 上下文/最大输出边界；Tier 2 logprobs 更稳 |
| 同族相邻版本 / 更新 | 低—中（纯文本）；高（有 logprobs） | Tier 2 logprobs 分布 + 知识/版本探针 |
| 精确 checkpoint / 量化等级 | 低，只能概率判断 | Tier 2 logprobs 分布 + 大基线；文本基本不行 |
| 同一个模型换了 serving 路径 / 聚合商 | 高 | Tier 0 头、schema、id、错误包、chunk 结构 |
| 上游主动代理真实模型（或直接转发） | **做不到**（与真模型不可区分） | 无；这是定义上的边界 |
| 上游针对固定探针做了特判/回放 | 很难，但不是不可能 | 留出探针、随机化、logprobs；只能提高伪造成本 |

**做不到的情况：**

- 证明“权重就是某个精确 checkpoint”。API 只能观测行为；代理真实模型的行为与真实模型相同。
- 稳定区分“同族、同 tokenizer、同格式，但量化/微调不同”的模型——除非有 logprobs 且基线足够密。
- 在 provider 默认 system prompt、采样默认值、安全层不同的情况下做跨 provider 的绝对比较；只能做“同 serving 路径”的基线比较。
- 用 headers / 时序证明模型身份；它们证明的是“谁在服务”，不是“哪套权重”。

因此方案必须把 **serving-path signature** 与 **model-core signature** 分开报告；判定结果也应是 `MATCH / SAME_FAMILY_DIFFERENT_VERSION / DIFFERENT_MODEL / INCONCLUSIVE`，不是一个布尔值。原型在阈值未校准时会给结果加 `PROVISIONAL_` 前缀。

---

## 2. 威胁模型与“目标模型”的定义

| 编号 | 场景 | 能否检测 | 首选层 |
|---|---|---|---|
| T1 | 供应商把声称的模型换成更便宜/更小/别家的模型 | 能，通常很稳 | T1 tokenizer + 工具 schema + 能力边界 |
| T2 | 同一个模型，但换了 serving 栈/聚合商/区域/量化 | 模型核心难，serving 路径能 | T0 头/schema/id + T2 logprobs |
| T3 | 转发真模型，但静默降级：截断上下文、限制输出、加安全层、限速 | 部分能 | T0 实际 usage/finish_reason + T2 上下文/输出边界 |
| T4 | 上游识别探针并特判/回放答案 | 只能提高成本 | 留出探针、随机化、logprobs、真实流量监控 |
| — | 上游直接代理真模型（或就是官方端点） | **不能** | 无（行为等价） |

“目标模型”应拆成两层：

1. **serving-path**：同一 base URL / 网关 / provider 配置 / 协议包装。
2. **model-core**：同一模型族、同一 tokenizer、同一工具/格式约定、相近的知识与能力。

验证一个供应商时，先问“serving path 是否还是签约时那条”，再问“model core 是否还是那个模型”。两条结论不能混为一个“指纹”。

---

## 3. 仓库现状（事实 + 证据）

### 3.1 转发链路不是字节直通

- 网关入口按 `/tingly/{scenario}` 路由（`internal/probe/types.go:588-601` 的 `ScenarioEndpoint`；dispatch 在 `internal/protocolserver/protocol_dispatch.go`）。
- OpenAI Chat → OpenAI Chat 的“passthrough”其实是**解码后重建**：非流式把 SDK 响应 `json.Marshal` 成 map、重写 `model` 后返回（`internal/protocolserver/protocol_passthrough.go:21-98`，尤其 `:44`、`:69`、`:93`）；流式逐 chunk 重建 SSE（`internal/protocol/stream/openai_passthrough.go:44-337`）。
- 非流式重建会丢掉 SDK 未序列化的上游 extras；代码自己承认 `json.Marshal` 会丢 `message.reasoning_content`，需要补回（`internal/protocolserver/protocol_passthrough.go:202-229`）。所以 SDK 路径拿不到“上游原始 JSON 字节”。
- 流式处理器设置的是自己的 SSE 头，不是上游头（`internal/protocol/stream/openai_passthrough.go:51-57`）。定向 grep `resp.Header` 未发现通用路径把上游响应头复制给客户端；命中的是专用 client 对自己合成响应的 header 操作（如 `internal/client/qoder_client.go:232-233`）和错误包装（`internal/client/error_envelope_transport.go:42-43`），不是上游头透传。

**结论：** 要做响应头 / 原始字节 / chunk 级时序指纹，必须用独立 HTTP 客户端直连上游；不能依赖 TB 的 SDK 转发。

### 3.2 请求参数透传：同协议基本保留，跨协议会收窄

- 同类型请求不做协议转换：`internal/protocol/transform/base.go:94-103`（OpenAI Chat 原样保留，只建空 config）。
- 收尾清理只删 `x_thinking` 和空 tools：`internal/protocol/request/thinking_helper.go:48-74`。
- provider 特化在链尾：`internal/protocol/transform/vendor.go:56-62` → `ops.ApplyProviderTransforms`（`internal/protocol/ops/request_openai_extensions.go:20-59`），会按 host 改写/剥离部分字段（例如非 OpenAI host 剥离 prompt-cache 字段，`:24-32`）。
- 跨协议转换明确丢弃 logprobs：`internal/protocol/request/openai_responses_to_anthropic.go:17-23` 写明 `TopLogprobs` 不支持；`internal/protocol/wire/openai_chat.go:103-108` 的 wire DTO 写着 “Logprobs is always null”。
- 请求侧的 `logprobs` / `top_logprobs` 在仓库里**没有任何引用**（定向 grep 只命中响应侧和转换注释）。同协议下 typed 请求很可能会保留它们，但没有测试证明；**UNKNOWN**（且本 worktree 的 `libs/openai-go` 子模块为空，无法核对 SDK 字段）。

**结论：** 用 OpenAI Chat 直连上游时，可以自行发任意参数；经过 TB 时，同协议大多能到上游，但 `logprobs` 没有被显式支持/验证；跨协议（Chat↔Responses↔Anthropic）不要期望 logprobs 存活。

### 3.3 响应侧可观测点

- `model` 会被网关重写为响应模型（`internal/protocolserver/protocol_passthrough.go:69`；流式 `internal/protocol/stream/openai_passthrough.go:203`），所以**经 TB 看到的 `model` 不能作为身份证据**。
- 流式保留上游的 `choice.Logprobs`（`internal/protocol/stream/openai_passthrough.go:199-212`）、`system_fingerprint`（`:219-222`）、`service_tier`（`:224-229`）、`obfuscation`（`:231-243`）。
- 非流式把 SDK 响应整体 marshal（`protocol_passthrough.go:44`），SDK 若带 `system_fingerprint` / `logprobs` 字段则会保留；**是否保留取决于 SDK 版本，UNKNOWN**。
- 跨协议合成的 wire DTO 明确不输出 `system_fingerprint`（`internal/protocol/wire/openai_chat.go:81-82`）。
- `finish_reason` 会被读取；流式如果没有 finish reason 会补一个 `upstream_truncated` 错误（`internal/protocol/stream/openai_passthrough.go:282-295`）。

### 3.4 现有 probe 子系统已经能直连上游，但采不到头/时序

- `internal/probe` 是服务无关的探针实现；`E2EProber` 支持 `provider` / `rule` / `provider_config` 三种 target（`internal/probe/types.go:158-165`、`internal/probe/e2e_probe.go:41-93`）。
- `Direct` 模式跳过 TB loopback 直连上游（`internal/probe/types.go:262-265`；`e2e_probe.go:179-247`）。
- 支持三种协议的**原始请求体**，由 SDK decoder 解析后原样发出，只补 `model` / Anthropic `max_tokens` / 流式 `stream_options.include_usage`（`internal/probe/types.go:297-310`；`internal/probe/helper.go:155-186`、`:283-313`、`:410-457`）。
- 结果里已有：完整 marshaled 响应或全部 chunk（`Result.Content`）、规范化 usage、latency、tool calls、routing echo（`internal/probe/types.go:40-94`；`helper.go:233-279`、`:369-405`、`:504-541`）。
- **缺口**：没有上游响应头、没有 TTFT、没有逐 chunk 时间戳；`Content` 是 SDK 重新 marshal 的结果，不是原始字节（见 3.1）。

### 3.5 tokenizer / token counting

- 本地只有 tiktoken 的 `O200kBase`，用于估算/兜底，不是按模型精确 tokenizer：`internal/protocol/token/codec.go:1-36`、`internal/protocol/token/openai.go:13-29`、`:138-148`、`internal/protocol/token/anthropic.go:51-122`。
- Anthropic 形状的 provider 支持 `/v1/messages/count_tokens`，网关优先转发给上游，只有 Bedrock 等不支持时才回退 tiktoken：`internal/protocolserver/anthropic_count_tokens.go:21-160`（转发 `:152-160`，回退 `:161+`）；`internal/client/bedrock_client.go:29-37`。
- 这意味着：**对 Anthropic 形状端点，`count_tokens` 是一个近似精确的 tokenizer 指纹**；对 OpenAI 形状端点，用响应里的 `usage.prompt_tokens` 做差分。

### 3.6 usage / TTFT / TPS / DB

- 每个请求都会持久化 usage：provider、model、request_model、input/output/cache/reasoning/system tokens、latency、TTFT、streamed、status（`internal/db/usage_record.go:22-63`）。
- TTFT 的唯一定义点是 `protocol.MarkFirstToken`：流式首个带内容的 delta（`internal/protocol/loop.go:63-77`；调用点 `internal/protocol/stream/openai_passthrough.go:247-249`）。
- 聚合到 service stats，含 Avg/P50/P95/P99 TTFT 和 TPS（`internal/protocolserver/usage_tracking.go:23-30`、`:99`、`:234`；`internal/loadbalance/load_balancing.go:91-131`）。
- 这是**被动 Tier 0** 的现成数据源；但它没有原始 body、finish_reason、headers、chunk 结构。

### 3.7 recording 暂时不能作为响应指纹来源

- 请求侧录制可用；响应侧被明确暂停：`internal/recording/recorder.go:304-312` 写着 `upstream_response` 没有 producer，`final_response` 质量不够、暂不 emit。
- 所以现在不能靠既有流量被动拿到上游原始响应/头；需要主动采集，或另开线程恢复 response-side recording。

### 3.8 模型目录 / 模型列表

- `providers.json` + `ModelInfo` 有 context / max_output / openai_endpoints 等静态能力（`internal/catalog/provider_catalog.go:34-42`、`:122`；文档 `.design/model-data.md`），适合作为上下文/输出边界的先验，不是指纹库。
- 运行时 `/v1/models` 的真实原始响应会被保存（`internal/config/model_resolve.go:215-244`，`SaveModelsWithRaw`；字段 `internal/db/provider_model.go:24-43`、`:88-91`）。这是 Tier 0 的一个低成本基线来源（模型 id、created、owned_by、顺序）。

### 3.9 可复用 / 不复用清单

| 组件 | 能否复用于指纹 | 说明 |
|---|---|---|
| `internal/probe` | 部分复用 | 可复用 provider 解析、raw request 构造、结果结构；不能复用其 SDK 路径采集头/时序 |
| `internal/protocol/token` | 仅作本地对照 | 只有 O200kBase，不能代表被测模型的 tokenizer |
| `anthropic_count_tokens` | 复用 | 直连上游的 count_tokens 就是 tokenizer 探针 |
| `vmodel/benchmark` | 只用于测试 mock | 它是虚模型/负载基准，不是真实上游（`vmodel/benchmark/bench.go:20-37`、`client.go:1-21`）；适合给采集器做确定性假目标 |
| `recording` | 暂不可用 | 响应侧暂停（3.7） |
| `internal/catalog` | 只作先验 | 存静态能力，不存行为指纹 |
| Bench 页面 | 未来 UI | `.design/bench.md:581` 已把 “Variant 对比运行（matrix run）” 列为后续方向 |

---

## 4. 探针分类与评估

评分口径：成本按一次完整采集（含 3 次重复）估算；“稳定性”指同模型跨时间/跨批次的复现性；“篡改风险”指上游有意模仿/伪造的难易。

| # | 探针类 | 能区分什么 | 需要的 API 能力 | 成本（token / 时间） | 稳定性 | 被模仿/篡改风险 |
|---|---|---|---|---|---|---|
| P1 | 传输与元数据：响应头、状态码、SSE 事件名、chunk 顺序/大小、`id` 前缀、`created` 粒度、`service_tier`、`obfuscation`、错误包 | serving 栈、聚合商、网关版本、是否真的转发到官方后端；**不区分权重** | 原始 HTTP（不能走 SDK 重建） | 极低 / 秒级 | 同一路径高；跨路径无意义 | 高：头/schema 很容易照抄 |
| P2 | 响应 schema：顶层/usage 键集合、`finish_reason` 词表、`content` 空值形态、tool_calls delta 形态 | API 实现、协议版本、模型族的部分习惯 | 原始 body 或 SDK 完整 struct | 极低 / 秒级 | 高 | 高：可用兼容层模仿 |
| P3 | Tokenizer 计数：固定字符串的 `prompt_tokens` / `count_tokens`；前缀增量 `count(base+X)-count(base)` | tokenizer 身份（强）、隐藏 system prompt/工具开销、包装层是否重分词 | `usage` 或 `/count_tokens`；要能控制输入 | 低 / 秒级 | 很高（精确整数） | 低—中：tokenizer 可复刻，但必须与输出行为一致 |
| P4 | Logprobs 分布：top-k token、概率、top1 margin、熵、与基线 JS 散度 | 同族 checkpoint、量化、蒸馏；最强的“模型核心”信号 | `logprobs=true` + `top_logprobs`（OpenAI Chat） | 低 / 秒级（若可用） | 高（temp=0 下仍有批次噪声） | 低：要伪造必须复现分布 |
| P5 | 确定性输出：算术、排序、代码、固定格式、罕见知识、知识截止 | 模型族/版本、指令微调风格；单条弱，聚合中强 | 普通 chat/messages | 中 / 分钟级 | 中（模型更新、默认 system prompt 会漂移） | 中：可特判固定题 |
| P6 | 工具调用 / 结构化输出：工具名、参数 JSON 键序/空白、并行调用、schema 严格度、`response_format` | 模型族的工具序列化习惯、serving 栈、能力档 | tools / response_format | 中 / 分钟级 | 高—中 | 中：兼容层可模仿 |
| P7 | 停止/截断/采样语义：`stop`、`max_tokens`、`n`、`seed`、`temperature=0`、截断时 `finish_reason` | API 语义、模型是否尊重 stop、输出上限 | 采样参数 | 低—中 / 秒级 | 高 | 高：API 层容易照抄 |
| P8 | 能力边界：上下文窗口、最大输出、多模态、thinking budget、结构化输出支持 | 尺寸档、模型类、provider 限缩 | 长上下文 / 大输出 / 图片 / thinking | 高 / 分钟—小时；长上下文档可能 10万—100万 token | 高（边界是硬事实） | 低：要伪装只能真扩上下文 |
| P9 | 时序：TTFT、首包后 token 间隔、吞吐、chunk 粒度 | 硬件/负载/量化/是否排队；**不区分同硬件同负载下的模型身份** | 流式 + 时间戳 | 低 / 秒—分钟 | 低（网络、batch、区域影响大） | 高：加延迟即可伪造 |
| P10 | 行为/策略：拒答阈值、安全类别、多语言质量、长文召回、指令层级 | 安全层、对齐风格、上下文能力 | 普通调用 + 长上下文 | 中—高 | 低—中（system prompt 依赖强） | 中：策略层可套壳 |

**分类取舍：**

- **把 P1/P2 归为 serving-path，不归为 model-core。** 它们最早、最便宜，但只能回答“是不是同一条服务路径”。
- **P3 是性价比最高的 model-core 信号。** 整数计数没有浮点噪声，而且对“换 tokenizer 的模型族”几乎一票否决。必须用“前缀增量”抵消隐藏 system prompt 与工具定义。
- **P4 是唯一能稳定打同族量化的信号**，但可用性取决于 provider；不能作为基础层。
- **P8 是硬边界**，适合高价值核验；成本高，先用 catalog 的 `context` / `max_output` 先验（`internal/catalog/provider_catalog.go:34-42`）只测边界附近，避免全量二分。
- **P9 只做异常检测**：同区域、同网络、同并发下的稳健基线；超过 5× 中位数才作为弱负面证据。
- **P10 只做辅助**：任何“自报身份”都可能被 system prompt 覆盖，必须与 P3/P6 联合判断。

---

## 5. 分层方案

### Tier 0：每次请求都能采 + 一组微探针

**被动部分（现有流量）：**

- `usage_records` 里的 provider/model/request_model、input/output/cache/reasoning tokens、TTFT、latency、streamed、status（`internal/db/usage_record.go:22-63`）。
- 适合做**漂移告警**：同一 provider+model 的 token 比例、TTFT 分布、error code、缓存命中率发生阶跃变化时报警。不能单独作为身份证据。
- 注意：`model` 是网关重写后的值（`protocol_passthrough.go:69`），`RequestModel` 是客户端请求值；两者都不等于上游实际服务模型。

**主动微探针（一次采集 ~6 个请求）：**

1. `GET /models`（或 Anthropic `/v1/models`）：原始 body、顺序、`created`、`owned_by`、分页；TB 已会保存原始响应（`internal/config/model_resolve.go:244`）。
2. 非流式最小请求：`temperature=0, max_tokens=1`，固定单条 user 消息。记录状态、**全部响应头（allowlist）**、body 键集合、usage 键集合、`id` 模式、`model`、`system_fingerprint`、`service_tier`、`finish_reason`。
3. 同样的请求 + `stream=true`：SSE 事件名、role-only 首 delta、usage chunk 是否最后、`[DONE]`、chunk 数、逐 chunk 时间、TTFT、ITL。
4. 不支持参数探测（每协议 1 次）：`logprobs=true`、`seed`、`top_k`、`n=2`、`stop`。记录是 400、忽略还是生效——错误包是 serving 栈的强特征。
5. Anthropic 形状：固定字符串 `POST /v1/messages/count_tokens`（`internal/protocolserver/anthropic_count_tokens.go:152-160`）。

**成本：** 6—8 个请求，< 1k token，5—20 秒。

**判定方法：**

- 构造两个签名：
  - `serving_path_sig = hash(canonical(headers_allowlist, schema_keys, id_patterns, error_envelope, sse_shape))`；非值字段（request id、created、时间）只保留模式/数量。
  - `schema_sig = hash(sorted(usage_keys), finish_reason_vocab, chunk_order, content_null_shape)`。
- 与基线比：`serving_path_sig` 不同 → `DIFFERENT_SERVING_PATH`（可能是聚合商/区域/网关升级），不直接判假模型。
- 相同 → 进入 Tier 1；基线缺失时只报告“未知路径”。
- 时序只记录，不参与判定。

**这层的价值：** 检测静默 fallback / 换聚合商 / 换网关；为 Tier 1 选对基线（不同 serving path 的同一模型本来就可能不同）。

### Tier 1：几十条固定探针（temp=0，k 次重复）

建议 v0.1 约 40 条，分 8 组：

| 组 | 条数 | 例子 / 目的 | 评分 |
|---|---|---|---|
| tokenizer 计数 | 8—10 对 | CJK、emoji/ZWJ、组合字符、罕见 Unicode、代码标识符、重复 token、JSON 标点；用 `count(base+X)-count(base)` 抵消隐藏前缀 | 精确整数向量；L1 距离；hash |
| 确定性输出 | 6—8 | 算术、排序、字符串反转、字母计数、固定 JSON、代码片段 | 规范化文本相似度 + exact-match 率 |
| 工具调用 | 5—6 | 一个带嵌套 schema 的函数；看 name/arguments 的原始序列化、并行调用、`tool_choice` | 解析后语义相等 + 原始字节 hash（分开存） |
| 结构化输出 | 3—4 | `json_schema` 严格模式、缺字段、额外字段 | 是否 400 / 是否被 grammar 约束 |
| 停止/截断 | 4—5 | `stop`、`max_tokens=1/2`、`finish_reason=length`、stop 是否回显 | 语义精确匹配 |
| 知识/版本 | 4—5 | 罕见可验证事实、知识截止、版本标记 | 多选题准确率 / 聚合 |
| 自报身份/风格 | 2—3 | “你是什么模型”、拒答风格、多语言 | 只作辅助，权重低 |
| 长文召回（轻量） | 2—3 | 4k/8k needle + 唯一 nonce | 召回准确率 |

**成本：** 40 条 × 3 次重复 ≈ 120 个请求；输入+输出约 15k—60k token；1—5 分钟；按当前价格是分级成本（通常几分钱到几毛钱）。

**判定方法：**

- 每条探针产出 `s_p ∈ [0,1]`：
  - 整数计数：`1 - min(1, |observed-baseline| / max(1, baseline))`，或直接 exact。
  - 文本：先 NFC，再 `SequenceMatcher` / token Jaccard；同时存 raw hash 与 normalized hash。
  - 工具：解析 `arguments` 为 JSON 后比较键值集合；原始序列化 hash 单独作为格式特征。
  - 结构化/停止：布尔/枚举精确匹配。
- 按类别取加权中位数 `S_c`，再对类别分取加权中位数得到总体 `S`。
- 关键类别 `C* = {tokenizer, tool, stop}`。任一 `S_c` 低于该类基线 p5 → `DIFFERENT_MODEL`。serving-path 的 headers/schema 单独判定，不混进 model-core 的关键类别。
- `S ≥ τ_match` 且关键类别全过 → `MATCH`；`S ≤ τ_mismatch` → `DIFFERENT_MODEL`；中间 → `INCONCLUSIVE`，按需升级 Tier 2。

### Tier 2：logprobs / 长上下文 / 能力边界

1. **logprobs（首选强指纹）**
   - 请求：`temperature=0`、`max_tokens=8—32`、`logprobs=true`、`top_logprobs=5—10`，固定 prompt。
   - 特征：每个位置的 top-1 token、top-k 集合、概率、top1−top2 margin、熵；与基线做 top-k Jaccard 和 JS 散度。
   - 成本：10—20 条 × 5 次 ≈ 10k token，秒—分钟。
   - 可用性：OpenAI Chat 形状可能支持；Anthropic 不支持；跨协议转换会丢（3.2）。**必须在目标 provider 上先探测是否可用。**
   - 稳定性：temp=0 仍受 batching/数值影响，需容差和多次重复；比文本强一个量级。

2. **tokenizer（精确）**
   - Anthropic 形状：`count_tokens` 直接给精确输入 token 数。
   - OpenAI 形状：用 `usage.prompt_tokens` 的差分。
   - 这一项通常与 Tier 1 tokenizer 组合并，不需要单独长跑；放进 Tier 2 是因为它可能需要逐条校准隐藏前缀。

3. **上下文 / 输出边界**
   - 先用 catalog 的 `context` / `max_output` 作为先验（`internal/catalog/provider_catalog.go:34-42`），只测边界附近（例如 32k±1、128k±1、模型声明上限±1），不要盲二分。
   - 请求“只回复 OK”以压低输出成本；记录 400 / 截断 / 成功。
   - 成本：每次 32k—1M 输入 token；高价值核验才做。

4. **能力开关**
   - 多模态、thinking budget 档位、结构化输出、工具并行。能区分尺寸档/能力档，但对同族同尺寸无意义。

**判定：** 作为 T1 的“升级证据”。logprobs 距离显著低于同模型基线分布、且高于跨模型分布 → 强 `MATCH`/`DIFFERENT_MODEL`；边界不符 → 直接 `CAPABILITY_MISMATCH`；只有文本探针可用时不要给高置信度。

### 成本/收益排序

| 顺序 | 层 | 何时跑 | 主要回答 |
|---|---|---|---|
| 1 | T0 | 每次签约核验 / 定期 | serving path 是否变了 |
| 2 | T1 tokenizer + 工具 + 确定性 | T0 路径相同后 | 模型族/尺寸档是否变了 |
| 3 | T1 知识/风格/长文 | 同上，低权重 | 版本/更新方向 |
| 4 | T2 logprobs | 高价值 / T1 不确定 / 怀疑量化 | 同族 checkpoint 差异 |
| 5 | T2 上下文/输出 | 合同关键能力 | 是否被截断/限缩 |

---

## 6. Fingerprint 记录格式（JSON schema 草案 v0.1）

设计原则：

- **可离线比对**：不依赖采集时的进程/DB；所有判定所需特征都在记录里。
- **可版本演进**：`schema_version`、`probe_set.version`、`feature_extractor_version` 分开。
- **不存秘密**：token/API key/完整 base URL 不进记录；响应头走 allowlist + 模式化；原始 body 只按 hash 引用，是否保留可配置。
- **非值字段模式化**：`id`、`created`、request-id、时间戳只存“模式 + 数量/范围”，不存具体值。
- **channel 必须记**：`direct` / `through_tb` / `through_aggregator`；不同 channel 不可直接比。

```jsonc
{
  "schema_version": "model-fingerprint/v0.1",
  "fingerprint_id": "sha256:...",          // canonical JSON（不含本字段/integrity）的 hash
  "kind": "baseline",                       // baseline | observed | comparison
  "created_at": "2026-11-25T12:00:00Z",
  "tool": {
    "name": "tb-fingerprint",
    "version": "0.1.0",
    "git_sha": "..."
  },

  "target": {
    "requested_model": "claude-opus-4-5",
    "reported_model": "claude-opus-4-5-20251101", // 上游 body 的 model（直连时）
    "provider": {
      "uuid": "...",                          // 可空（外部采集）
      "name": "anthropic",
      "api_style": "anthropic",               // openai | anthropic | google
      "api_base_host": "api.anthropic.com",   // 只存 host，不存 query/path 中的密钥
      "channel": "direct",                    // direct | through_tb | through_aggregator
      "region": "us-east-1"
    },
    "protocol": "anthropic_v1",               // anthropic_v1 | openai_chat | openai_responses
    "gateway": {                              // channel=through_tb 时填
      "version": "...",
      "rule_uuid": null,
      "scenario": null,
      "flags": {}
    }
  },

  "probe_set": {
    "id": "tb-core",
    "version": "1.0.0",
    "sha256": "...",                          // 探针定义文件的 hash
    "tiers": ["t0", "t1"],
    "calibration": {                          // 来自 probes.json；compare 只读这一份
      "status": "uncalibrated",
      "tau_match": 0.90,
      "tau_mismatch": 0.60,
      "critical_category_threshold": 0.99,
      "critical_categories": ["tokenizer", "tool", "stop"]
    }
  },
  "feature_extractor_version": "fx/1",

  "params": {
    "temperature": 0,
    "top_p": 1,
    "seed": null,
    "max_tokens": 64,
    "system_prompt": "none",                  // none | <id>；不要内联可能含隐私的 system 文本
    "stop": []
  },

  "environment": {
    "collected_from": "script|cli|ci",
    "network": "public|vpn|local",
    "rtt_estimate_ms": 42,
    "host": "linux"
  },

  "tiers": {
    "t0": {
      "requests": [
        {
          "request_id": "t0.models",
          "method": "GET",
          "path": "/v1/models",
          "status": 200,
          "elapsed_ms": 210,
          "headers": {
            "allowlist": {
              "content-type": { "value": "application/json" },
              "x-request-id": { "pattern": "^req_[0-9a-f]{24}$" },
              "server": { "value": "cloudflare" }
            }
          },
          "body_sha256": "...",
          "body_summary": {
            "count": 12,
            "id_patterns": ["^claude-[a-z0-9.-]+$"],
            "created_min": 1710000000,
            "created_max": 1760000000,
            "owned_by": ["anthropic"]
          },
          "raw_ref": "blobs/ab/cdef..."      // 可选；敏感时置 null
        }
      ],
      "signature": {
        "serving_path_sig": "sha256:...",
        "schema_sig": "sha256:...",
        "finish_reason_vocab": ["end_turn", "max_tokens", "tool_use"],
        "usage_keys": ["input_tokens", "output_tokens", "cache_read_input_tokens"],
        "id_patterns": { "chat": null, "message": "^msg_[A-Za-z0-9]+$" },
        "stream": {
          "event_names": ["message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"],
          "usage_last": true,
          "done_marker": null,
          "chunk_count": 9
        }
      }
    },

    "t1": {
      "probes": [
        {
          "probe_id": "tok.delta.cjk.v1",
          "group_id": "tokenizer.delta",
          "category": "tokenizer",
          "tier": "t1",
          "input_sha256": "...",
          "input_preview": "base=你好，世界。今天天气很好。 plus=…+👨‍👩‍👧‍👦",
          "request": { "max_tokens": 1, "temperature": 0 },
          "repeats": 3,
          "observations": [
            {
              "seq": 1,
              "status": 200,
              "output_sha256": "...",
              "output_text": "",
              "usage": { "input_tokens": 11, "output_tokens": 1 },
              "finish_reason": "max_tokens",
              "ttft_ms": 420,
              "total_ms": 700,
              "chunk_count": 2,
              "logprobs_digest": null
            }
          ],
          "features": {
            "base_count": 11,
            "plus_count": 18,
            "delta": 7
          }
        }
      ],
      "signature": {
        "category_scores": {
          "tokenizer": 0.99,
          "canonical": 0.91,
          "tools": 0.95,
          "stop": 1.0,
          "knowledge": 0.80
        },
        "aggregate": 0.94
      }
    },

    "t2": {
      "available": true,
      "logprobs": {
        "probes": [
          {
            "probe_id": "lp.canonical.v1",
            "observations": [
              {
                "seq": 1,
                "tokens": ["The", " capital", " of", " France"],
                "top_logprobs": [
                  [["The", -0.01], ["the", -4.8]],
                  [[" capital", -0.02], [" Capital", -3.9]]
                ],
                "digest_sha256": "..."
              }
            ],
            "features": { "top1_prob_mean": 0.98, "margin_mean": 4.1, "entropy_mean": 0.12 }
          }
        ]
      },
      "capability": {
        "context_window_tested": 200000,
        "max_output_tested": 8192,
        "modalities": ["text", "image"],
        "thinking_budgets": [1024, 4096, 16000]
      }
    }
  },

  "quality": {
    "requests": 132,
    "ok": 130,
    "failed": 2,
    "rate_limited": 0,
    "truncated": 1,
    "failure_rate": 0.015,
    "confidence": "high",                     // high | medium | low
    "warnings": ["logprobs unsupported by provider"]
  },

  "integrity": {
    "record_sha256": "...",
    "signature": null                         // 可选：团队私钥签名，用于基线分发
  }
}
```

**版本规则：**

- `schema_version` 用 SemVer；新增可选字段是 minor，删/改语义是 major。读者必须忽略未知字段，遇到 major 不符直接拒绝比较。
- `probe_set.id + version` 必须完全一致才允许直接比较；否则需要显式的 `comparability` 映射（例如 v1.1 只新增了不参与判定的探针）。原型在版本不一致时只比较共享 probe_id，并给出 warning。
- `feature_extractor_version` 变化时，旧记录可以重算特征，但必须记录用的是哪个版本。
- Baseline 记录带 `supersedes` / `superseded_by`；模型更新不覆盖旧条目，新增 `SAME_FAMILY_DIFFERENT_VERSION` 条目。
- 原始 body：默认只存 `body_sha256` + 可选 `raw_ref`（content-addressed blob，按保留期清理）。基线库只提交自造探针的 raw，不提交用户流量。

> 采集器 v0.1 的差异：`t1.signature` 留空（由 `compare.py` 计算）；Tier 2 只存 logprobs 摘要（top1 prob / margin / entropy / tokens / digest），不存完整 `top_logprobs` 数组；`id_patterns` 存模式类（如 `msg_<token>`）而不是正则。上面的 JSON 是目标形态，不是当前原型的逐字节输出。

---

## 7. “远端是否为目标模型”判定算法草案

### 7.1 输入与预处理

1. 校验 `schema_version` / `probe_set.id+version` / `channel` / `protocol` / `system_prompt` 一致；不一致的字段不参与比较，直接 `INCONCLUSIVE` 或降级为“仅 serving path 可比”。
2. 丢弃失败观测：非 2xx（除刻意探测的错误探针）、超时、429、stream 截断；如果 `failure_rate > 20%` → `INCONCLUSIVE`。
3. 归一化：
   - 文本：NFC；去尾部空白；**不要**去 Markdown 围栏/标点，它们本身是行为。同时保留 raw hash。
   - id/created/request-id：只保留模式/范围，不比较具体值。
   - 数值计数：整数精确比较。
   - 工具参数：先解析 JSON，再比较语义；原始字节 hash 单独作为格式特征。
4. 重复：每条探针 k=3—5 次；用中位数/多数投票；报告 IQR。

### 7.2 每类探针的距离

| 类别 | 单探针相似度 `s_p` | 备注 |
|---|---|---|
| tokenizer 计数 | `1 - min(1, |o-b| / max(1,b))`；完全相等=1 | 用增量 `count(base+X)-count(base)`，抵消隐藏前缀 |
| 文本规范化 | `SequenceMatcher.ratio()` 或 token Jaccard | 同时存 exact-match 率 |
| 工具调用 | 语义：键值集合完全相等=1；否则按字段交集 | 原始序列化 hash 只做格式分 |
| schema/停止/截断 | 枚举精确相等=1/0 | 关键类别 |
| logprobs | top-k Jaccard、top1 一致率、JS 散度转相似度 | 多种子/多位置平均 |
| 时序 | 仅异常检测：`1` 若在基线 p5—p95；极端不匹配给低分 | 不参与 MATCH |

### 7.3 聚合与判定

```text
for each probe p: s_p in [0,1], weight w_p, category c(p)
S_c = weighted_median({s_p | c(p)=c})
S   = weighted_median(S_c)                  # 先类别后总体，防止某类探针数量淹没其他类

critical = {tokenizer, tool, stop}            # serving path 单独判，不混进 model-core
if any S_c < tau_c(critical):  return DIFFERENT_MODEL (high confidence)
if S >= tau_match and all critical pass: return MATCH
if S <= tau_mismatch:          return DIFFERENT_MODEL
# 同族不同版本：tokenizer/tools/stop 全过，但 canonical/knowledge 漂移
if critical pass and S in [tau_mismatch, tau_match):
    return SAME_FAMILY_DIFFERENT_VERSION
return INCONCLUSIVE  # 可升级 T2
```

**最近邻：**

- 对基线库所有候选算 `S`，取 top-3；报告 `top1 - top2` 的 margin。
- `margin < δ` 或 top1 分数在不确定带 → `INCONCLUSIVE`，不要硬选。
- 候选库按 `(model, serving_path, protocol, system_prompt)` 分层；先匹配 serving path，再比 model core。

### 7.4 阈值校准（必须用基线，不要拍脑袋）

对每个 `probe_set` × 模型族 × channel：

1. 同一模型、同一路径，至少 3 次独立采集（不同日期/不同 key/不同 region），每次 k=5 重复 → 得到类内相似度分布 `D_intra`。
2. 不同模型（最好同族不同尺寸 + 不同族）→ 类间分布 `D_inter`。
3. `tau_match = quantile(D_intra, 0.05)`；`tau_mismatch = quantile(D_inter, 0.95)`。
4. **如果 `tau_match <= tau_mismatch`，说明当前探针集无法可靠区分**：增加探针/类别，或明确报告“无法区分”，不要降低阈值硬判。
5. 留出 20% 探针不参与阈值拟合，只用于最终核验；每季度轮换，降低被特判的风险。
6. 关键类别单独设阈值（例如 tokenizer 要求精确；工具 schema 要求结构相等），不受总体分数稀释。

### 7.5 噪声与非确定性

- `temperature=0` 不等于完全确定：batching、量化、浮点累加、provider 的 seed 实现都会造成差异；因此所有文本探针都要重复并看分布，而不是单次 hash。
- 流式与非流式、不同 `max_tokens`、不同 system prompt 的结果不可直接比；记录里必须带参数，比较前校验。
- 经 TB 的采集包含网关自己的归一化（model 重写、usage 估算、跨协议丢字段），只能与“同规则/同 flags/同协议”的 TB 基线比；验证供应商应直连。
- 时序只做异常检测：同区域/同网络/同并发下，TTFT 中位数 > 5× 基线且持续，才作为弱负面证据；不要用它做 MATCH。

### 7.6 误报 / 漏报权衡

- **漏报（把替身判成目标）成本更高**（供应商欺诈、合同风险）。因此：
  - 关键类别一票否决；
  - 只给“全部关键类别通过 + 总体分数过阈值 + 最近邻 margin 足够”才 `MATCH`；
  - 不确定时输出 `INCONCLUSIVE` 并列出缺失证据，而不是猜。
- **误报（把真模型判成替身）也有成本**（模型正常更新、provider 换 serving 路径、量化差异）。因此：
  - 先判 serving path，再判 model core；路径变化不直接等于模型变化；
  - 同族版本更新归为 `SAME_FAMILY_DIFFERENT_VERSION`，不归为 `DIFFERENT_MODEL`；
  - 基线更新后重新校准，旧记录标记 `stale`。
- 多假设比较时（多候选模型）做多重比较校正，或要求至少两个独立关键类别同时失败才判 `DIFFERENT_MODEL`。

### 7.7 对抗边界（必须写进报告）

- **代理真模型不可检测**：上游把请求原样转发给真模型，指纹完全一致；这在行为上就是“用了那个模型”，指纹方案无法也不应把它判为假。
- **固定探针可被特判**：如果探针集公开且固定，上游可以识别并回放/路由。缓解：留出探针、随机化模板/顺序、把探针混入真实流量、用 logprobs 这类难以伪造的信号。只能提高成本，不能证明。
- **头/schema/id 容易伪造**：T0 只能证明“声称的 serving 路径”，不能证明权重。
- **时序容易伪造**：加固定延迟即可；不要依赖。
- **tokenizer 可复刻**：开源 tokenizer 谁都能实现；必须与输出行为、工具格式、logprobs 联合判断。
- **logprobs 最难伪造**：要伪造分布基本需要拥有该模型；但 provider 可以拒绝返回 logprobs，或只对探针转发真模型。可用性本身就是限制。
- **结论措辞**：输出“与基线 X 的行为一致/不一致”，不要输出“证明是 X 模型”。

---

## 8. 实现路径建议

### 8.1 为什么先用独立脚本，而不是直接改 probe

- `internal/probe` 已经是现成的直连能力，但它的 SDK 路径拿不到上游响应头和逐 chunk 时间，且 `Content` 是重新 marshal 的（3.1、3.4）。指纹采集需要**原始 HTTP**。
- 本任务明确不接入主流程。先在 `.research/fingerprint-prototype/` 放一个 stdlib-only 的原型，验证探针集和 schema，再决定是否提升为内部包/CLI。
- 仓库根 `CLAUDE.md` 建议脚本放 `script/`，但本仓库的 `.gitignore:3` 忽略整个 `script` 目录；为了让原型随调研一起版本化，本分支放在 `.research/fingerprint-prototype/`。产品命令仍用 Kong（`cli/tingly-box/main.go:25-80`）。

### 8.2 最小可用版本（MVP）范围

本分支已放一个**草案脚本**：`.research/fingerprint-prototype/collect.py` + `compare.py`（见 8.4）。MVP 只做：

- OpenAI Chat 形状 + Anthropic Messages 形状的直连采集；
- Tier 0（models、最小非流式、最小流式、unsupported 参数、count_tokens）；
- Tier 1 的一个子集：tokenizer delta（CJK/emoji/罕见 Unicode/重复 token）、确定性算术/JSON、工具调用、stop/截断；
- 可选 Tier 2：OpenAI Chat logprobs；
- 输出第 6 节的 JSON；`compare.py` 给类别分、总体分、最近邻和四态结论（阈值先用固定占位值，标注待校准）；
- 不读 `~/.tingly-box`、不写 DB、不进主流程、不做 UI。

**明确不做：** 长上下文边界测试、图片、真实基线库、CI 中的真实 API 调用。

### 8.3 提升为产品时的落点

| 阶段 | 位置 | 内容 |
|---|---|---|
| P1（原型） | `.research/fingerprint-prototype/` | 本分支的采集/比对草案；探针定义 JSON；小型基线样例 |
| P2（内部包） | `internal/fingerprint/` | 纯函数：schema、探针定义、特征提取、比较、阈值；`go:embed` 探针与基线；无 HTTP/DB 依赖 |
| P3（CLI） | `internal/command/fingerprint_command.go` + `cli/tingly-box/main.go` 注册 | `tingly-box fingerprint collect --provider <uuid> --model ...` / `compare --baseline ... --observed ...`；`--direct` 显式走上游；读 provider 配置时仍要提示会使用真实凭据 |
| P4（UI） | Bench | `.design/bench.md:581` 的 variant/matrix run 天然是“同配置 A/B + 并排 diff”的入口；指纹可作为其中一个结果视图 |
| P5（被动） | 新表 / OTel | 若要长期监控，新增独立的 `model_observations` 表，**不要**往 `usage_records` 热插入路径塞 headers/raw；或者只在 OTel 上做聚合告警 |

**CLI 设计要点：**

- `collect` 默认直连（指纹要的是上游），`--channel through_tb` 必须显式指定并记录 rule/scenario/flags。
- 凭据从环境变量读（`TB_FP_TOKEN`），不要放在命令行参数里（会进 shell history）。
- 输出目录默认 `./fingerprints/`；`--out -` 打 stdout。
- `compare` 只读 JSON，不联网；支持 `--baseline-dir` 做最近邻。
- 只做同步、串行请求；严格 1 并发（项目规则：限制并发），避免把 provider 打到限流。

**Taskfile：** 根 `CLAUDE.md` 说“没有用户要求不要加 Taskfile”；本任务不加。等用户要固定流程时再加 `fingerprint:collect` / `fingerprint:compare`。

### 8.4 原型脚本（本分支草案）

见：

- `.research/fingerprint-prototype/collect.py`：直连采集 + 写 schema。
- `.research/fingerprint-prototype/compare.py`：读两份 fingerprint，算类别分/总体分/最近邻。
- `.research/fingerprint-prototype/probes.json`：v0.1 探针定义（可读、可 diff、可算 hash）。
- `.research/fingerprint-prototype/README.md`：用法、安全边界、如何接基线。

脚本只依赖 Python 标准库；没有 `requests`/SDK；因此不会把 SDK 的序列化/归一化带进指纹。所有请求串行，默认 3 次重复。

### 8.5 已知模型基线库怎么建

**目录建议：**

```text
.research/fingerprint-prototype/baselines/
  <vendor>/<model_family>/<serving_path>/
    2026-11-25T120000Z.json
    2026-11-27T090000Z.json
    manifest.json      # 采集人/工具版本/探针集版本/密钥来源(不存密钥)
```

**采集要求：**

- 每个 (model, serving_path) 至少 3 次独立采集（不同日期；尽量不同 key/region），每次 k=3—5 重复。
- `serving_path` 用 `api_base_host + api_style + provider 名` 归一化；同一模型在官方 API 与聚合商下是不同基线。
- 覆盖团队实际采购的 3—5 个模型：至少包含目标模型、一个同族小模型（负样本）、一个别家模型（负样本）。
- 官方 API 与常见聚合商各采一份；这样“同模型不同路径”的差异不会被误判成“假模型”。
- 基线记录 `probe_set.version`；探针集变更后旧基线标记 `stale`，不能直接比。
- 基线库只提交自造探针的输入/输出（无用户数据）；不上传 API key；CI 不做真实调用。

**校准流程：**

1. 用 3 次同模型采集算 `D_intra`；
2. 用同族小模型 + 别家模型算 `D_inter`；
3. 按 7.4 选阈值；若不可分，先加探针，不要硬调；
4. 把阈值写进 `probes.json` 的 `calibration` 段，并记录日期/样本数。

### 8.6 测试与验证

- **本分支**：`python3 -m py_compile .research/fingerprint-prototype/*.py`；已用一个本地 Python mock 上游跑通 OpenAI/Anthropic 采集与比较（同记录 `MATCH`、改 tokenizer/工具参数的变体 `DIFFERENT_MODEL`），没有调用真实 API。
- **下一步**：用仓库的 `vmodel` 虚拟服务做确定性假上游（`vmodel/benchmark/bench.go:20-37`），两个不同 vmodel 定义应判 `DIFFERENT_MODEL`，同一模型重复采集应判 `MATCH`；再对一个真实 provider 做人工 smoke test。
- **不要**在 CI 里跑真实 API；不要用真实 `~/.tingly-box` 跑 app/swagger（项目记忆 `isolated-home-for-app-runs`）。

---

## 9. 风险与边界

| 风险 | 说明 | 缓解 |
|---|---|---|
| 供应链“真代理” | 上游转发真模型，任何 API 指纹都一致 | 接受这是定义边界；靠合同/审计/账单，不靠指纹 |
| 模型正常更新 | 同族新版本会让文本/知识探针漂移 | 版本化基线；`SAME_FAMILY_DIFFERENT_VERSION`；关键类别（tokenizer/工具）通常稳定 |
| provider 默认 system prompt | 同一模型不同 provider 的计数/风格不同 | 用 tokenizer 增量抵消前缀；分 serving path 建基线 |
| 探针被特判 | 固定题可被 whitelist | 留出探针、随机化、logprobs、混入真实流量 |
| 成本 | 长上下文/大输出探针很贵 | 先用 catalog 先验，只测边界；高价值才跑 T2 |
| 限流/封号 | 主动探测可能触发风控 | 串行、低频、退避；明确这是 admin 操作 |
| 秘密泄露 | token/base URL/header 可能进记录 | 环境变量、host-only、header allowlist、raw 可选 |
| 基线污染 | 用被换过的模型采基线 | 基线必须来自可信官方端点；多来源交叉；记录采集人/时间 |
| 误判为欺诈 | 量化/区域/版本差异被当替身 | 四态结论 + 置信度 + 缺失证据；不要布尔化 |
| 仓库边界 | 不引入对 providers 的运行时依赖 | 原型独立；产品化后 `internal/fingerprint` 保持纯函数 |

---

## 10. UNKNOWN / 待核实

- `libs/openai-go` 在本 worktree 为空，无法核对 `ChatCompletionNewParams` 是否含 `logprobs` / `top_logprobs` 字段；仓库请求侧也没有任何引用。**UNKNOWN**：经 TB 同协议转发时这些参数是否原样到达上游。
- 非流式 `json.Marshal(*openai.ChatCompletion)` 是否保留 `logprobs` / `system_fingerprint`：取决于 SDK 序列化；**UNKNOWN**。
- 各 provider 对 `logprobs`、`seed`、`top_k`、`n` 的实际接受度/错误包：**UNKNOWN**，必须逐 provider 探测。
- 哪些 OpenAI 兼容 provider 的 `usage.prompt_tokens` 是精确 tokenizer、哪些是估算：**UNKNOWN**；用增量差分可部分识别。
- `count_tokens` 在非 Anthropic 官方端点上的可用性：代码只排除了 Bedrock（`internal/client/bedrock_client.go:29-37`），其余**UNKNOWN**。
- 各 provider 是否会在请求前注入隐藏 system prompt/工具：**UNKNOWN**；tokenizer 增量设计就是为了不受它影响。
- 上游响应头是否通过某些特殊 client（Codex/Kimi/Qoder）被读取：定向 grep 未见通用路径；**UNKNOWN**，但设计要求不依赖它。

---

## 11. 下一步（3—5 个可执行项）

1. **跑通原型**：本分支已用本地 mock 跑通 `collect.py`/`compare.py`；下一步把它接到 `vmodel` 假上游做仓库内自测，确认 schema、SSE 解析、header allowlist、tokenizer 增量都正确。验收：同一定义重复采集 `MATCH`，两个不同 vmodel 定义 `DIFFERENT_MODEL`。
2. **冻结探针集 v0.1**：把 40 条探针写成 `probes.json`，每条带 `id/version/category/weight/ground_truth` 和 `sha256`；为 tokenizer/工具/stop 三类写“预期行为”测试。
3. **采 3—5 个模型的基线**：官方 API + 一个聚合商，各 ≥3 次采集 × 5 重复；产出 `D_intra` / `D_inter`，按 7.4 校准阈值；若不可分，记录需要补哪类探针。
4. **做一次真实供应商核验**：选一个实际采购的 provider，直连采集 → 与官方基线比 → 输出四态结论 + 缺失证据；同时记录耗时/成本/限流情况。
5. **决定 Tier 2 的可用性**：在目标 provider 上逐项探测 `logprobs`、`count_tokens`、上下文边界；把“不可用”写进探针集的 capability 声明，避免以后误判。

> 另：`internal/recording` 的响应侧目前是暂停的（`internal/recording/recorder.go:304-312`）。如果未来要“被动采集真实流量做指纹/漂移告警”，恢复 response-side recording 是一个独立的代码任务，不应塞进本方案。

---

## 12. 一句话总结

**先做“直连上游的 Tier 0 + tokenizer/工具/格式 Tier 1 + 小型基线库”，它能可靠抓住换厂商/换模型族/换 serving 路径/明显降级；同族 checkpoint 级差异只有在有 logprobs 时才能较稳地区分；代理真模型和刻意伪造在 API 层面无法根除，只能提高成本并诚实报告置信度。**





