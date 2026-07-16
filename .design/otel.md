# OTel 可观测性设计 — pkg/otel

> 遥测只有一个出口：可选的 OTLP 端点，metrics 和 traces 共用。
> 每请求的持久化数据（usage 记录、请求录制）在源头写入，永远不从聚合指标反推。
> 没有出口就不装管道——宁可全局 no-op，也不要"记录了再丢弃"的中间态。

关联代码：`pkg/otel/`（setup.go / config.go / tracer.go / attributes.go / tracker/ / exporter/）。
接线点：`internal/server/server.go`（`NewSetup` + `server.otelSetup` / `server.tokenTracker`）。
包级 API 文档见 `pkg/otel/README.md`；本文记录**为什么长成这样**。

---

## 1. 三条数据通路的职责边界（最重要的决定）

tingly-box 有三类"请求发生了什么"的数据，各有唯一的权威来源：

| 通路 | 回答的问题 | 权威来源 | 去向 |
|---|---|---|---|
| **聚合指标**（OTel metrics） | 总量多少 / 多快 / 错误率 | `tracker.RecordUsage`（usage_tracking 调用） | 仅 OTLP 端点 |
| **Trace spans** | 这个请求内部发生了什么 | `Tracer` 打点（接入中） | 仅 OTLP 端点 |
| **每请求持久化** | 精确账单 / 回放 / 审计 | `usage_tracking.go` 直写 UsageStore；recording 管道直写 `obs.Sink` | SQLite / JSONL 文件 |

**历史教训（为什么要立这条边界）**：旧版 pkg/otel 有两个"exporter"违反了它——

- `SinkExporter` 从聚合的 Sum 数据点反向合成 `obs.Record` 灌进录制管道。信息是有损的（token 值被丢弃、request id 没有、时间戳是导出时现打的），而且 cumulative temporality 下每个已知序列**每 10 秒重发一遍、永不停止**，7 个 instrument 共享同一属性集 → 一个逻辑请求变成每周期 7 条重复骨架记录。
- `SQLiteExporter` 是文档承认的占位符：每个导出周期遍历全部数据点、提取 6 个属性，喂给两个空函数体。配置面上还有一个默认开启的 `SQLite.Enabled` 开关——违反 UX 原则"开关必须真实"。

两者已删除（commit `9dd2e9a`）。**规则：想要每请求数据，去源头记录；指标管道只承载聚合。**

## 2. 没有出口就不装管道

- OTLP 未配置（默认）时：MeterProvider **不挂 reader**（instrument 调用近零成本的 no-op）、**不安装 TracerProvider**（span 走全局 no-op，`IsRecording()==false`）。业务代码可以无条件打点。
- 反面教材一：旧 tracer 配了 `AlwaysSample` 却没有任何 span processor——span 被完整构建然后原地丢弃。这是最差的中间态：付了记录成本、得不到任何数据、还让读代码的人以为 tracing 是通的。
- 反面教材二：旧 meter 在没有 exporter 时兜底到 stdout——删掉 SQLite/Sink 后这个兜底会变成每 10 秒往服务器控制台打印指标。一并删除。
- **规则：provider 只在有地方送数据时构造。** 三态只允许两个：真通，或真 no-op。

## 3. 命名：直接采用 OTel GenAI 语义约定（gen_ai.*）

**时机决定**：做这个决定时 `llm.*` 指标没有任何消费方（SQLite exporter 是空的、sink exporter 已删、OTLP 默认关闭、前端 dashboard 读的是 UsageStore 而非 OTel），迁移成本恰好为零，于是整体切换、不留旧名、不做双写（commit `99c9d93`）。若未来规范漂移，键名集中在 `pkg/otel/attributes.go` 和 `tracker/token_tracker.go` 顶部两处。

**指标形态**（这是规范的设计，不是我们的省略）：

| Instrument | 类型 | 单位 | 说明 |
|---|---|---|---|
| `gen_ai.client.token.usage` | histogram | `{token}` | 按 `gen_ai.token.type` 属性切分 |
| `gen_ai.client.operation.duration` | histogram | `s`（秒，不是毫秒） | count 即请求数；失败挂 `error.type` 属性 |

**刻意没有**独立的 request count / error count counter——duration 直方图的 count 就是请求数，`error.type` 分类失败。两个直方图都配了规范建议的显式分桶。

**token.type 的开放枚举扩展**：规范只定义 `input`/`output`；网关额外发 `cache_read`（缓存读 token）和 `system`（系统操作 token）。语义约定的枚举是开放的，这是合法用法；若规范将来收编 cache 类型，跟进改名即可。

**命名空间纪律**：网关自有维度不占用标准前缀，放 `tingly.*`：`tingly.scenario` / `tingly.provider.uuid` / `tingly.rule.uuid` / `tingly.streaming` / `tingly.user.tier`。观测平台（Datadog / New Relic 等）的 GenAI 面板自动识别 `gen_ai.*`，我们的维度作为自定义标签共存。

**Span 约定**：命名 `"{operation} {request model}"`（如 `chat claude-sonnet-4-6`），kind CLIENT，属性 `gen_ai.operation.name` / `gen_ai.provider.name` / `gen_ai.request.model` + `tingly.*`；token 用量是 span 属性 `gen_ai.usage.input_tokens` / `output_tokens`（`Tracer.SetTokenUsage`），不是 event。

## 4. 基数纪律（#1255 的血泪，不可回退）

cumulative 指标的每个不同属性组合都是一条**进程生命周期内永不释放**的时间序列。两条铁律：

1. **近唯一值永远不做指标属性**：latency、request id、原始错误文本。latency 是直方图的**值**；`error.type` 截断 64 字节。#1255 曾因 latency 上属性导致每请求永久泄漏 ~0.8MB。
   （span 不受此限：span 导出后即释放，每请求值就该放 span 上。）
2. **属性字符串必须与请求缓冲区解绑**：model / request model / error code 可能是 gjson 解析出的整个请求体的子串切片，保留属性会 pin 住整个 multi-MB 缓冲区。`RecordUsage` 对这些值 `strings.Clone`。

守护测试：`tracker/token_tracker_test.go` 的 `TestRecordUsage_NoHighCardinalityAttributes`。

## 5. Trace 管道的具体接线

- **OTLP trace exporter**（`exporter/otlp_trace.go`）与 metrics 共用同一端点配置（gRPC 或 http/protobuf），batch span processor 批量发送。
- **采样**：parent-based——上游传来的已采样 `traceparent` 永远尊重；新 trace 按 `OTLPConfig.TraceSampleRatio` 采样，(0,1) 之外的值（含零值）= 全采（网关 QPS 下的合理默认）。
- **传播**：启用 tracing 时安装 W3C `TraceContext` + `Baggage` propagator——trace id 能双向穿过网关（下游 agent → tingly-box → 上游 provider）。这是 LLM 网关做 tracing 的核心价值：把网关这一跳挂进调用方的完整 trace。
- **Tracer helper 陷阱**：`EndSpan(span, err)` 已经记录 exception 事件并置 error status，**不要**再对同一个错误调 `RecordError`，会重复两条（e2e 测出并有断言防回归）。
- **线格式验证**：`pkg/otel/trace_e2e_test.go` 起进程内 OTLP collector，用官方 proto 反序列化断言 payload——resource → scope → spans 层次、traceId/parentSpanId 链接、gen_ai.* 属性、error status。任何改动破坏标准兼容性会在这里挂掉。

## 6. 社区背景与跟踪点（2026-07 记录）

- 规范权威来源：<https://github.com/open-telemetry/semantic-conventions-genai>（2026 年从主 semconv 仓库拆出）；属性注册表：<https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/>
- **状态：Development（实验性）**，无稳定化时间表，属性名可能漂移（先例：`gen_ai.system` → `gen_ai.provider.name`）。核心概念已收敛，社区判断"现在构建是合理的赌注"。过渡机制：`OTEL_SEMCONV_STABILITY_OPT_IN=gen_ai_latest_experimental`。
- 平行约定：OpenLLMetry（Traceloop，`llm.*` 命名，2026-03 被 ServiceNow 收购）、OpenInference（Arize，`openinference.*`，定义了 14 种 span kind，正在与 gen_ai 收敛）。我们旧的 `llm.*` 键即源自 OpenLLMetry。
- 厂商原生支持 gen_ai：Datadog（v1.37+）、New Relic、Dynatrace、Honeycomb。Claude Code / Copilot / Codex 已在发这套遥测。
- **跟踪点**：内容捕获属性（`gen_ai.input.messages` 等，默认必须 opt-in，含敏感数据）、agent/MCP 约定、cache token type 是否被规范收编。

## 7. 未来工作（接入打点时）

1. 在请求管道（protocol handler / client 层）用 `server.otelSetup.Tracer()` 打 span：入口处 `StartRequestSpan`，上游调用一个子 span，出口 `SetTokenUsage` + `EndSpan`。
2. 从入站 HTTP 头 Extract 上游 trace context（propagator 已装好），出站注入 `traceparent`。
3. 非 chat 操作（embeddings 等）把 `UsageOptions.Operation` / span operation 传对。
4. 内容捕获（prompt/completion 上 span）：等规范稳定 + 产品决策，必须默认关闭。
5. OTLP 配置目前只有代码内 `DefaultConfig()`（默认关）——暴露到用户配置文件/UI 时记得走 swagger codegen 流程。

## 8. 决策记录索引

| Commit | 决定 |
|---|---|
| `c291d76` | tracker 属性集单次构建；MultiExporter 去锁、错误不吞 |
| `9dd2e9a` | 单一出口：删 Sink/SQLite exporter、删假 tracer、去 stdout 兜底、`NewMeterSetup(ctx, cfg)` 不再依赖 internal 存储 |
| `8d82cbd` | 真实 trace 管道：OTLP trace exporter + batcher + parent-based 采样 + W3C propagator；`MeterSetup`→`Setup` |
| `9b229c0` | OTLP 线格式 e2e（进程内 collector 反序列化断言） |
| `99c9d93` | 全面采用 gen_ai.* 语义约定；8 instrument → 2 直方图；`tingly.*` 自有命名空间 |
