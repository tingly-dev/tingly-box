# OTel 可观测性设计 — pkg/otel

> 遥测只有一个出口：可选的 OTLP 端点，metrics 和 traces 共用。
> 每请求的持久化数据（usage 记录、请求录制）在源头写入，永远不从聚合指标反推。
> 没有出口就不装管道——宁可全局 no-op，也不要"记录了再丢弃"的中间态。

关联代码：`internal/otel/`（setup.go / config.go / tracer.go / attributes.go / tracker/ / exporter/）。
接线点：`internal/server/server.go`（`NewSetup` + `server.otelSetup` / `server.tokenTracker`）。
包级 API 文档见 `internal/otel/README.md`；本文记录**为什么长成这样**。

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

- OTLP 未配置（默认）时：**什么 provider 都不建**——`Tracker()` 返回 nil（调用方本就有 nil 守卫，每请求的属性构建/clone/排序全部跳过），`Tracer()` 包一个显式 noop provider（不是全局委托 provider，避免后续 SetTracerProvider 使"保证 no-op"失效）。业务代码可以无条件打点。`Setup` 所有方法 nil-receiver 安全。
- 反面教材一：旧 tracer 配了 `AlwaysSample` 却没有任何 span processor——span 被完整构建然后原地丢弃。这是最差的中间态：付了记录成本、得不到任何数据、还让读代码的人以为 tracing 是通的。
- 反面教材二：旧 meter 在没有 exporter 时兜底到 stdout——删掉 SQLite/Sink 后这个兜底会变成每 10 秒往服务器控制台打印指标。一并删除。
- **规则：provider 只在有地方送数据时构造。** 三态只允许两个：真通，或真 no-op。
- **全局注册顺序**：`otel.SetMeterProvider` / `SetTracerProvider` / `SetTextMapPropagator` 在**全部构造成功之后**才调用——失败的 NewSetup 不得让进程全局指向已 Shutdown 的 provider。

## 3. 命名：直接采用 OTel GenAI 语义约定（gen_ai.*）

**时机决定**：做这个决定时 `llm.*` 指标没有任何消费方（SQLite exporter 是空的、sink exporter 已删、OTLP 默认关闭、前端 dashboard 读的是 UsageStore 而非 OTel），迁移成本恰好为零，于是整体切换、不留旧名、不做双写（commit `99c9d93`）。若未来规范漂移，键名集中在 `internal/otel/attributes.go` 和 `tracker/token_tracker.go` 顶部两处。

**实现来源**：instrument 名/单位/描述用官方 `semconv/v1.37.0/genaiconv` 构造器，标准属性键用 `semconv` 常量别名——semconv 版本升级即自动跟进规范改名；只有 `tingly.*` 键是我们自己的（单一定义在 tracker 包，attributes.go 为 span 侧别名）。

**指标形态**（这是规范的设计，不是我们的省略）：

| Instrument | 类型 | 单位 | 说明 |
|---|---|---|---|
| `gen_ai.client.token.usage` | histogram | `{token}` | 按 `gen_ai.token.type` 属性切分 |
| `gen_ai.client.operation.duration` | histogram | `s`（秒，不是毫秒） | count 即请求数；失败挂 `error.type` 属性 |

**刻意没有**独立的 request count / error count counter——duration 直方图的 count 就是请求数，`error.type` 分类失败。两个直方图都配了规范建议的显式分桶。

**canceled ≠ error**：`error.type` 只在 `Status=="error"` 时附加。客户端取消（Ctrl-C 中断流式响应）在 LLM UI 里是日常操作，若带上 error.type，标准错误率查询（"带 error.type 的点数 / 总数"，Datadog/New Relic 的默认口径）会在正常流量上报警。取消请求的精确记录在 UsageStore 里。`error.type` 值截断 64 字节且保证 UTF-8 合法（OTLP 要求合法 UTF-8，脏值会让严格的 collector 拒收整批导出）。

**token.type 的开放枚举扩展**：规范只定义 `input`/`output`；网关额外发 `cache_read`（缓存读 token）和 `system`（系统操作 token）。语义约定的枚举是开放的，这是合法用法；若规范将来收编 cache 类型，跟进改名即可。

**命名空间纪律**：网关自有维度不占用标准前缀，放 `tingly.*`：`tingly.scenario` / `tingly.provider.uuid` / `tingly.rule.uuid` / `tingly.streaming` / `tingly.user.tier`。观测平台（Datadog / New Relic 等）的 GenAI 面板自动识别 `gen_ai.*`，我们的维度作为自定义标签共存。

**Span 约定**：命名 `"{operation} {request model}"`（如 `chat claude-sonnet-4-6`），kind CLIENT，属性 `gen_ai.operation.name` / `gen_ai.provider.name` / `gen_ai.request.model` + `tingly.*`；token 用量是 span 属性 `gen_ai.usage.input_tokens` / `output_tokens`（`Tracer.SetTokenUsage`），不是 event。

## 4. 基数纪律（#1255 的血泪，不可回退）

cumulative 指标的每个不同属性组合都是一条**进程生命周期内永不释放**的时间序列。两条铁律：

1. **近唯一值永远不做指标属性**：latency、request id、原始错误文本。latency 是直方图的**值**；`error.type` 截断 64 字节。#1255 曾因 latency 上属性导致每请求永久泄漏 ~0.8MB。
   （span 不受此限：span 导出后即释放，每请求值就该放 span 上。）
2. **属性字符串必须与请求缓冲区解绑**：model / request model / error code 可能是 gjson 解析出的整个请求体的子串切片，保留属性会 pin 住整个 multi-MB 缓冲区。`RecordUsage` 对这些值 `strings.Clone`。
3. **span 属性同样要解绑（trace 侧变体）**：span 结束后会在 batch 导出队列里滞留（collector 慢时最多囤 2048 个），别名请求体的属性会造成瞬时数 GB 的 pin。`StartRequestSpan` 对 model `strings.Clone`；调用方经 `SetSpanAttributes` 传入请求来源字符串时须遵守同样纪律。

守护测试（指针级断言，比对 `unsafe.StringData` 确认属性值不与源缓冲区共享存储）：
- `tracker/token_tracker_test.go`：`TestRecordUsage_NoHighCardinalityAttributes`（latency 不上属性 + error.type 截断）、`TestRecordUsage_DetachesRequestBufferStrings`（clone 解绑）
- `internal/otel/oom_regression_test.go`：`TestStartRequestSpan_DetachesModelString`（span 侧解绑）
- `internal/obs/memorylog_test.go`：`TestFireDetachesValues`（内存日志 detach，另一条 OOM 战线，见 logging-redesign）

## 5. Trace 管道的具体接线

- **OTLP trace exporter**（`exporter/otlp_trace.go`）与 metrics 共用同一端点配置（gRPC 或 http/protobuf），batch span processor 批量发送。
- **采样**：parent-based——上游传来的已采样 `traceparent` 永远尊重；新 trace 按 `OTLPConfig.TraceSampleRatio` 采样，(0,1) 之外的值（含零值）= 全采（网关 QPS 下的合理默认）。
- **传播**：启用 tracing 时安装 W3C `TraceContext` + `Baggage` propagator——trace id 能双向穿过网关（下游 agent → tingly-box → 上游 provider）。这是 LLM 网关做 tracing 的核心价值：把网关这一跳挂进调用方的完整 trace。
- **operation 轴两侧对齐**：`StartRequestSpan(ctx, operation, provider, model, scenario)` 的 operation 参数与 `UsageOptions.Operation` 同轴同默认（"chat"）——metrics 和 span 对同一请求永远报同一个 operation。
- **Tracer helper 陷阱**：`EndSpan(span, err)` 已经记录 exception 事件并置 error status，**不要**再对同一个错误调 `RecordError`，会重复两条（e2e 测出并有断言防回归）。
- **线格式验证**：`internal/otel/trace_e2e_test.go` 起进程内 OTLP collector，用官方 proto 反序列化断言 payload——resource → scope → spans 层次、traceId/parentSpanId 链接、gen_ai.* 属性、error status。任何改动破坏标准兼容性会在这里挂掉。

## 6. 社区背景与跟踪点（2026-07 记录）

- 规范权威来源：<https://github.com/open-telemetry/semantic-conventions-genai>（2026 年从主 semconv 仓库拆出）；属性注册表：<https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/>
- **状态：Development（实验性）**，无稳定化时间表，属性名可能漂移（先例：`gen_ai.system` → `gen_ai.provider.name`）。核心概念已收敛，社区判断"现在构建是合理的赌注"。过渡机制：`OTEL_SEMCONV_STABILITY_OPT_IN=gen_ai_latest_experimental`。
- 平行约定：OpenLLMetry（Traceloop，`llm.*` 命名，2026-03 被 ServiceNow 收购）、OpenInference（Arize，`openinference.*`，定义了 14 种 span kind，正在与 gen_ai 收敛）。我们旧的 `llm.*` 键即源自 OpenLLMetry。
- 厂商原生支持 gen_ai：Datadog（v1.37+）、New Relic、Dynatrace、Honeycomb。Claude Code / Copilot / Codex 已在发这套遥测。
- **跟踪点**：内容捕获属性（`gen_ai.input.messages` 等，默认必须 opt-in，含敏感数据）、agent/MCP 约定、cache token type 是否被规范收编。

## 7. 请求链路打点接线（2026-08 实施）

### 7.1 为什么 span 生命周期归中间件，不归 trackUsage

`trackUsage*` 有 30+ 个调用点（dispatch / cross / passthrough / mcp loop / error_response），
且 failover 每次失败尝试也会经 `failAttemptSetup` 调一次——它**不是每请求恰好一次**的终点，
不能承载 EndSpan。root span 的唯一正确 owner 是 gin middleware（一进一出各恰好一次）。

同理，gin context 的 tracking 键值包**不改造成 span**：OTel span 是只写导出型，UsageStore
计费 / health monitor / LB stats 需要在请求末端读回这些值；把 span 当内存态数据载体正是
§1 删掉的 SinkExporter 反模式。context 键值包的定位是**进程内参数载体**，span 是它在
观测通路上的**投影之一**（与 UsageRecord、metrics、memory log 平行）。曾考虑把 ~20 个
ctx key 合并为单一 struct（"B 方案"）——被否决：这些 key 分属身份（auth 写入）、能力句柄
（recorder/guardrails state）、追踪元数据三个领域，写入方横跨 middleware/routing/
protocolserver/protocol 四个包，`constant` 包的松散 key 正是为了避免跨域 god-struct 与
import 环。追踪元数据簇已被 `SetTrackingContext`/`GetTrackingContext` 单点收口，够用。

### 7.2 Span 拓扑

```
{operation} {request model}   (root, kind CLIENT, 每 HTTP 请求一个, middleware 拥有)
 ├─ gen_ai.* 请求属性 + tingly.* 维度 + token usage 属性（trackUsage 回写）
 ├─ routing            (child) 选路 pipeline；attrs: tingly.lb.service_id / tingly.lb.tactic
 └─ failover.attempt   (child, 仅多服务 failover 循环产生, 每次尝试一个)
      attrs: tingly.failover.attempt / tingly.lb.service_id / provider uuid
      outcome: committed → OK；retryable/terminal 失败 → error status + http status attr
```

**`upstream` span 推迟到下一批**（连同 `internal/client` 的出站 traceparent 注入）。
原因有两条，第二条是决定性的：

1. 统一行文法之后它不再是骨架的必需品——日志行是一等公民、同一种行形态，
   少了 upstream span 只是把那一行从 STAGE 降级成 LOG（失去实测时长），叙事完整。
   单服务请求的上游耗时可由行头总耗时减去 routing 得到；多服务时 attempt span
   本就在测每次尝试的耗时。
2. **以当时的形态发布是残缺的**：`NewClaudeClient` 从不设置 `WithHTTPClient`，
   走 SDK 默认 transport，所以 Claude Code OAuth——最主要的 provider——根本不经过
   那一层。发一个悄悄漏掉主路径的上游计时，比不发更糟：它让人以为时长数据是完整的。
   下一批要连同"Claude Code 路径不走连接池"这个既有问题一起修（那条路径同时也
   忽略 provider proxy_url、反而继承环境代理）。

同批发现、但属既有缺陷、不在本 PR 范围：`AnthropicClient.applyRecordMode` 无守卫地写
`c.httpClient.Transport`，而 ClaudeClient 的 `httpClient` 为 nil ⇒ **对 Claude Code
provider 开启录制会 panic**（已用临时测试确证）。

**为什么 routing 要打点**：只有 root + attempt 时，单服务请求（最常见）整条 trace
只有一个 span。routing 回答"选了谁、用什么 tactic"，是骨架的最小可用集。transform
不单独建 span（亚毫秒级），它的日志行本就是视图里的一等行。

**打点集中在一处,不散落到 handler**:routing span 一度是 6 个 handler 里各一对手工括号
（`endRouting := ph.startRoutingSpan(c)` … `endRouting(err)`）——新增端点就得有人记得补,
是典型的"细碎"型可维护性债。现在 handler 调用 `ph.selectService` / `selectServiceForEmbeddings`
/ `selectServiceForImageGeneration` 这三个带打点的包装方法,span 只在 `tracing.go`
出现一次。业务文件里剩余的 span 接触点只有两处不可再收的:failover 循环内的 attempt span
（本就一处）、`usage_tracking.go` 的 token 用量镜像（与其余记账步骤并列）。

**span 代码归 protocolserver,不进 `internal/middleware`**:后者装的是 server 与
protocolserver **共用**的那些（auth / access log / CORS / gzip / IO 超时），而网关自己的
中间件（legacyScenarioAlias / profileAlias / context / tracing）历来跟着它们注册的路由走。
搬过去还要让 middleware 反向引用 `GetTrackingContext`，而 protocolserver 已经正向依赖
middleware——会成环。文件名从 `tracing_middleware.go` 改为 `tracing.go`，因为它装的
不只是中间件。


**attempt / routing span 不换入 `c.Request` 的 context**：ambient span 必须始终是 root，token usage 才会落在 root 上（GenAI 约定要求）。代价是 upstream span 与 attempt span 是兄弟而非父子——视图侧按**时间包含关系**做嵌套展示，比改动语义更便宜且更稳。

- **root span**：`tracingMiddleware`（`contextMiddleware` 之后）创建。入口 Extract 入站
  `traceparent`（propagator 未启用时是全局 no-op，零成本）；`c.Next()` 之后从 tracking
  context 读回 provider/model/rule/LB 决策，`span.SetName("{operation} {request model}")`
  （operation 从路径推导：/embeddings → embeddings，其余 chat，与 UsageOptions.Operation
  同轴同默认）并补全属性，按 HTTP 状态置 error（**canceled ≠ error 规则同 §3**：
  `c.Request.Context().Err() == Canceled` 时不置 error status）。
- **失败尝试不再丢失**：`UpdateTrackingForFailover` 覆盖 ctx 只留最终赢家（UsageRecord
  的既定语义，不变）；尝试历史的结构化归宿是 per-attempt child span。不在内存里平行
  维护 attempts 列表，也不塞进 UsageRecord——那违反 §1 三通路边界。
- **attempt span 不换入 c.Request context**：token usage 回写（`Tracer.SetTokenUsage`）
  走 ambient ctx，必须始终落在 root span 上；attempt span 是纯结果记录，做 root 的
  子节点即可。
- **出站注入**：`propagatingTransport`（internal/client）在两个 transport 汇聚点包一层
  （`createSessionBoundTransport` + 直连 `GetGlobalTransportPool().GetTransport` 的
  openai/anthropic/google 三处），Inject 前按 RoundTripper 契约 clone request + header。
  未启用 tracing 时 propagator 为 no-op、ctx 无 span——注入零头。

### 7.3 trace_id 关联（三通路互跳）

root span sampled 且 valid 时，middleware 把 trace id 写入
`constant.CtxKeyTraceID`；两个消费方：

1. **UsageRecord.TraceID**（新列 `trace_id`，AutoMigrate 增列）——从精确账单跳到 trace。
2. **memory access log 的 `trace_id` 字段**（middleware/memory_log.go）——从请求日志跳到 trace。

未启用 tracing 时 key 不设置、列为空串，零行为变化。

### 7.4 内存 trace 查看器（默认渠道，2026-08 实施）

不配 OTLP 的用户（绝大多数）也必须能看到 trace——否则打点的价值对他们不存在，
且 UsageRecord / access log 上的 `trace_id` 指向一个不存在的地方。

**规则修正**：§2"没有出口就不装管道"中的"出口"从"仅 OTLP"扩展为
"OTLP **或内存查看器**"。内存查看器是真实出口：数据从源头正向记录、有界、
有真实消费方（logs 页 UI）。它**不是**被删的 SinkExporter 反模式——那个错在
从聚合指标反向合成记录且无人消费；方向和消费方都不同。

- `../internal/otel` 的 ring-buffer SpanProcessor **常驻注册**：trace provider 在无 OTLP 时
  也构建（仅挂内存 processor）；OTLP 配置后二者并存（sdktrace 多 processor）。
  metrics 侧不变（无 OTLP 时 `Tracker()` 仍为 nil）。W3C propagator 随之常装。
- **硬性有界**（#1255 战线）：按 trace 数 + 估算字节双重封顶，超限逐最旧 trace 整体
  淘汰。默认值写死（smart defaults over toggles），暂不暴露配置。
- **不碰请求/响应内容**：span 属性维持现状；内容回放归 recording 管道。
- **重启即清空**：与 memory log 同语义。要持久 trace 走 OTLP 接真后端，两路不混。
- **前端入口只在 logs 页**：不做 usage 页入口、不做独立 traces 页（首版）。

### 7.4.1 展开态是一个"请求旅程"，不是两个视图

首版把 span 瀑布和原有的事件 timeline 并排放在展开态里——同一个故事讲两遍，
读者要在脑子里做 join，是 ux-principles §1（信息架构围绕用户问题组织）的反例。
现在合并成单一叙事（`RequestJourney.tsx`）：

- **一种行形态贯穿全部记录**：trace 阶段与日志行用同一个行文法渲染——
  `[类型徽章] 名称 · 明细 → 结果 · 时长`。徽章（STAGE / LOG）表达"这条记录是什么"，
  从属关系交给时间顺序表达（阶段的日志行本就落在它的时间窗内，自然排在它下面），
  因此**不需要缩进层级**。这条是对着一个 agent 轨迹查看器的设计定的：那里
  TOOL / SUBTOOL / ASSISTANT 也是同一种行、靠徽章区分，`参数 → 结果` 单行截断。
  之前页面在一个展开态里混了四种形态（表格行、阶段行、注解子行、属性 dump），
  读者每换一种就要重新学一次怎么读；
- **明细查看方式也只有一种**：点任意行，在同一个面板里展开它的完整载荷，
  统一等宽 `k=v`。唯一保留结构的是 smart routing 的逐条规则评估——它确实有结构，
  但渲染进同一个容器、用同一套排版，而不是自成一种卡片；
- **trace span 是主干**：一个阶段一行，按时间排序；
- **是列表，不是图表**：时长条只有在需要看**重叠/并发**时才值那块空间，而这些阶段
  严格串行——右对齐的等宽时长列比条形更好比较。首版画了条形图，代价是 600px 横向
  空间、12ms 的 routing 缩成一个看不见的点、两条近似等长的条互相干扰。整个展开态是
  **一个 CSS grid**（图标 / 阶段 / 事实 / 状态 / 时长），注解也落在同一套列上，
  避免每层缩进各自起一条参差的左边缘；
- **attempt 与其 upstream 合并为一行**：一次 failover 尝试和它内部的上游调用是
  同一件事说两遍（同一个状态码、几乎相同的时长，只是一个用 service id 命名、
  一个用 host 命名）。视图按时间包含把 upstream 折进 attempt，两边的事实都保留
  （`service_id · host`）——这是"两套叙事"在更小尺度上的复发；
- **日志事件是注解**：按时间包含关系挂到所属阶段行下；阶段本身没有事实可陈述时
  （派生阶段、或关闭 tracing 时的全部阶段），首条注解上提到事实列，
  否则会渲染成"标签 + 两个空格子 + 下一行吊着的文字"这种稀疏阶梯；
- **smart routing 注解陈述评估规模而非结果**："rule matched" 没有说出任何
  阶段行未说的事；改为 "2 routing rules evaluated · #1 matched"，同时充当
  "点开有规则明细"的可见提示。规则明细本身收进展开态（每条规则四行，
  而多数时候读这个视图不是为了看路由）；
- **access log envelope 整条丢弃**：status / latency / path 已在表格行头，重复一次是纯噪音
  （唯一例外：它是仅有的事件时保留，覆盖"请求在任何阶段之前就失败"）；
- **字段人性化**：logrus 的 ns 时长转 ms，摘要已展示的字段不再 dump；
- **ID 降级**：request id / trace id 移到展开态底部的小字——报 bug 时才需要，读旅程时不需要。

**无 trace 时同一形状降级**：阶段改由事件的 `stage` 字段派生（无时长条），
所以关掉 tracing 或 trace 被淘汰时，logs 页的结构不变、不退化成裸日志流。
不做 "Trace / Timeline" tab 切换——那是 §2 反对的 mode picker，等于把 join 成本还给用户。

### 7.5 仍然悬置

1. 内容捕获（prompt/completion 上 span）：等规范稳定 + 产品决策，必须默认关闭。
2. OTLP 配置目前只有代码内 `DefaultConfig()`（默认关）——暴露到用户配置文件/UI 时记得走 swagger codegen 流程。
3. MCP loop 每次迭代的独立 gen_ai span（当前 token usage 回写 root，last-write-wins；
   精确值在 UsageStore）。

## 8. 决策记录索引

| Commit | 决定 |
|---|---|
| `c291d76` | tracker 属性集单次构建；MultiExporter 去锁、错误不吞 |
| `9dd2e9a` | 单一出口：删 Sink/SQLite exporter、删假 tracer、去 stdout 兜底、`NewMeterSetup(ctx, cfg)` 不再依赖 internal 存储 |
| `8d82cbd` | 真实 trace 管道：OTLP trace exporter + batcher + parent-based 采样 + W3C propagator；`MeterSetup`→`Setup` |
| `9b229c0` | OTLP 线格式 e2e（进程内 collector 反序列化断言） |
| `99c9d93` | 全面采用 gen_ai.* 语义约定；8 instrument → 2 直方图；`tingly.*` 自有命名空间 |
| `99c9d93`+1 | 评审修复：canceled 不算 error、UTF-8 安全截断、全局注册后置、noop tracer、Tracker() 无出口时为 nil、semconv/genaiconv 官方常量替换手打、OTLP 选项构建泛型化、service.version 走真实版本 |
