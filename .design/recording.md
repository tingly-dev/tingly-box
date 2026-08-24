# Recording 梳理:意图、现状与整合方向

> 适用对象:tingly-box 后端 / 前端贡献者。
> 状态:**梳理文档 + 分阶段落地记录**。Phase 0 只梳理,Phase 1 起逐步
> 改行为,当前进度见 §6(Phase 1–3 已完成,收窄为仅 request 录制)。
> obs 包内部的 pipeline 化重构规划见
> `internal/obs/PLANNING.md`(Phase 2),本文与其互补:PLANNING 管
> "record 怎么被采集与导出",本文管 "record 由谁启用、在哪些层出现、
> 与 rule flag 体系怎么融合"。

---

## 1. 意图(统一口径)

历史上 recording 的意图一直比较混乱(全局 CLI 开关、scenario 开关、
client 层独立 RoundTripper 三者并存),导致了当前零碎的局面。统一后的
意图只有三句话:

1. **Record 实体长期存在并沿请求链传播,只是不一定启用。**
   每个请求都可以有一个 record 实体(recorder),从 handler 入口创建,
   经 transform chain 传播到 client transport;"是否启用/录多深"是它
   身上的状态,不是"是否存在"的条件。这与 obs PLANNING Phase 2 的
   "hot path 总是构建完整 record,裁剪放在 exporter 出口" 是同一思想。

2. **启用来源走 flag 体系:scenario 可以启用,rule flag 也可以启用。**
   这正是 rule flag 系统的既有设计(shared flag + 继承,参照
   `thinking_effort` / `skip_usage`,见 `.design/rule-flags.md` §12)。
   recording 满足 rule flag 的全部定义特征——局部、可选、可叠加、
   per-rule 语义成立("只录打到某条 rule 的流量"是最典型的 debug 诉求)。

3. **Client 层不做判定,只负责用最终的 transport 录制出站请求。**
   到了 client 层,"是否需要录 request"已经在上游(flag 解析)决定;
   而且只要 record 启用,出站 wire 请求总是要录的——所以 client 层
   反而简单:在**最贴近 wire 的 transport** 上无条件录制(recorder
   在 ctx 里就录),不再有自己的 mode 判定逻辑。

---

## 2. 现状全景(as-is)

### 2.1 三个互相独立的开关来源

| 来源 | 粒度 | 位置 | 状态 |
|------|------|------|------|
| ~~CLI `--record-mode` / `--record-dir`~~ | 全局 | — | **已移除(Phase 1)**:启用与否是 flag 关注点,不是 CLI 参数。落盘目录固定为 `<configDir>/record`(`StartServerOptions.RecordDir` 内部解析,无用户 flag) |
| `ScenarioFlags.RecordingV2` | scenario | `internal/typ/type.go`(json `recording_v2`) | 生效,scenario-only,**不在 `RuleFlagRegistry()`** |
| rule 级 | 单条 rule | — | **不存在** |

`Server.GetScenarioRecordMode(scenario)`(`server_options.go`)现在只读
scenario 的 `recording_v2`,不再有全局 fallback。

### 2.2 两套录制机制

**A. Chain 级(v2,现役)** — `recording.ProtocolRecorder` +
`TransformRecorder`(StagePre / StagePost):

```
handler 入口 (EnsureProtocolRecorder, 读 GetScenarioRecordMode)
  │  recorder 存 gin ctx;sink 按 scenario 懒建 (scenarioRecordSinks)
  ▼
BuildTransformChain: StagePre 录原始请求 → … → Vendor → StagePost 录最终(SDK 形态)请求
  ▼
流式 hooks / RecordResponse / RecordError → sink.Emit
```

- 只接了 **Anthropic 入站** handler(`anthropic_message.go` V1 + Beta)。
- **OpenAI Chat / Responses handler 传 `nil` recorder**
  (`openai_chat.go`、`openai_responses.go` 里 `TransformOpenAIChat/
  Responses(..., nil, ...)`)——OpenAI 入站流量即使开了 `recording_v2`
  也不录。
- mode 语义:`request` = 只录 transformed request;`request_response`
  再加最终响应;`staged_request_response` 再加原始(客户端)请求。

**B. Client 级(legacy,已死)** — `client.RecordRoundTripper`
(`internal/client/record_roundtripper.go`),见 §3。

### 2.3 Client transport 链的层次(通用链)

```
[RecordRoundTripper]   ← 仅 SetRecordSink 后挂载;最外层
  loggingRoundTripper  ← wrapWithLogging(仅日志)
    ruleFlagTransport  ← wrapWithRuleFlags(header 改写:UA / extra_headers)
      base (wire)      ← transport pool / vendor round-tripper
```

vendor 链(Claude OAuth / Codex / Kimi / Gemini / Antigravity)自建
transport,不挂 `ruleFlagTransport`(不变式,见 rule-flags.md §8),
也同样可能被 `SetRecordSink` 包上 `RecordRoundTripper`(advisor 路径)。

### 2.4 Sink 生命周期

- `scenarioRecordSinks map[RuleScenario]*obs.Sink`:root `*Server` 持有,
  按 scenario 懒建(`GetOrCreateScenarioSink`),chain 级录制用它。
- `ClientPool.recordSink`:生产路径 `server.clientPool =
  client.NewClientPool()` **从不设置 sink**(`WithRecordSink` 只有测试
  在用;`server.recordSink` 字段声明后无人读写)。
- 唯一在运行时给 client 塞 sink 的是 advisor 路径:
  `servertool/hook.go::applyHooks` 把 scenario sink 放进 ctx →
  `mcp/runtime/advisor_call.go` 对 advisor wrapper client 调
  `SetRecordSink(sink)`。

---

## 3. 问题清单(逐条,可验证)

**P1 — `recording_v2` 游离在 flag registry 之外。**
scenario-only、无 FlagSpec、前端 `RecordingV2Control.tsx` 硬编码
(rule-flags.md §13 已把"scenario flag registry 化"列为未做项)。

**P2 — rule 级 recording 不存在。** 意图 §1.2 要求的"rule flag 可以
启用"没有对应实现。

**P3 — `RecordRoundTripper` 的录制路径是死代码。✅ 已清除(Phase 1)。**
原论证链:
1. `obs.NewSink` 只接受三种 v2 mode(`request` / `request_response` /
   `staged_request_response`),其余返回 nil(`sink.go`);
2. `RecordRoundTripper.RoundTrip` 开头对这三种 mode **直接透传**
   (early return,意图是"v2 由 chain 级负责,client 层不重复录");
3. 挂载只发生在 sink 非空时(`SetRecordSink` → `applyRecordMode`);
4. 生产 pool 无 sink(§2.4),唯一挂载点是 advisor wrapper——挂上即
   early-return。
   ⇒ 该文件 ~450 行的录制 / SSE 重组逻辑全部不可达。
Phase 1 已删除:`record_roundtripper.go` 整个文件、各 client 的
`recordSink` 字段 / `SetRecordSink` / `applyRecordMode`(含接口方法与
vmodel no-op 实现)、pool 的 sink 字段与 builder、advisor_call.go 中
两处对 `SetRecordSink` 的死调用。

**P4 — advisor 防递归 header 缺失(真 bug)。✅ 已修复(Phase 1.5)。**
原状:`X-Tingly-Advisor-Depth: 1` 的唯一设置点在 `RecordRoundTripper.
RoundTrip` 的 early-return **之后**——即从未真正发出;服务端却靠这个
header 跳过 MCP tool 注入 / 标记 loopback(`protocol_transform.go`、
`transform_mcp_tool_injection.go`)。
修复方式:advisor 调用侧(`mcp/runtime/advisor_call.go`)在 SDK 调用前
`client.WithAdvisorLoopback(ctx)` 标记 ctx;通用 pass-through 链挂载只读
的 `advisorLoopbackTransport`(`internal/client/advisor_loopback.go`,
挂载点:`NewOpenAIClient` 与 `anthropicTransport`)按标记盖这一个 header。
vendor 链不挂——它们固定指向真实 vendor 端点,不可能 loopback。
同批清理了从未生效的 advisor sink 注入:`WithAdvisorRecordSink` /
`GetAdvisorRecordSink`(tool/context.go)、`HookDeps.GetScenarioSink`
及其注入点(servertool/hook.go)与实现(mcp_tool_error.go)全部删除;
advisor 调用的录制将来随统一录制路径(Phase 3)回归。

**P5 — 录制覆盖不对称。✅ 已修复(Phase 2):** OpenAI Chat / Responses
handler 接上 recorder(prologue 按 effective mode 创建、存 gin ctx,
下游经 `recording.FromGin` 自取,不走签名);OpenAI→OpenAI 纯透传路径(`nonstreamOpenAIChat` /
`streamOpenAIChat`)此前从不触碰 recorder(emit 永不发生),已补上
成功/失败两侧的 emit(流式透传无 chunk tap,final 由 writer 状态合成,
请求侧点位不受影响)。

**P6 — 挂载位置录不到真实出站请求。✅ 已修复(Phase 3,收窄为仅
request)。** `RecordRoundTripper` 挂在**最外层**(§2.3),看到的是
`ruleFlagTransport` / vendor round-tripper 改写 header **之前**的请求;
chain 级 StagePost 录的是 SDK 参数形态,`Headers` 硬编码空 map、`URL`
用的是入站 gin 请求路径(不是真实 upstream URL)——`URL` 字段从建立时
就没被正确录过。修复方式见 §3.7:`upstream_request` 的采集点从链路层
迁到 client 层最内层 wire transport,不再是 SDK 参数快照,而是真实
method/URL/body(header 本次刻意不采集,见 §3.7)。

**P7 — `ScenarioContextKey` 定义在死文件里。✅ 已解决(Phase 1):**
迁至 `internal/client/context.go`,引用方
(`routes_middleware.go`、`servertool/hook.go`)不变。

---

## 3.5 采集点位模型(Phase 2 定稿)

录制配置从"三档模式枚举"改为**沿链路采集点的多选集合**(逗号分隔存储,
先例 `block_tools`),值域 `typ.RecordingPoint`:

| 点位 | 中文 | 采集内容 | 现状 |
|------|------|----------|------|
| `client_request` | 入站请求 | client 发来的原始请求(transform 前) | ✅ handler 入口 + 链路层 `TransformRecorder`(仅此一个点位留在链路层) |
| `upstream_request` | 出站请求 | 发往 provider 的最终请求,真实 method/URL/body(header 不采集,见 §3.7) | ✅ **Phase 3**:client 层最内层 wire transport(`internal/client/wire_recorder.go`),不再是链路层 SDK 参数快照 |
| `upstream_response` | 服务返回 | provider 的原始响应(wire 级) | ❌ 值域内、**UI 不放开**(无采集实现——本次只做 request,不做 response,见 §3.7) |
| `final_response` | 最终返回 | 返回给 client 的响应 | ⏸ **暂停**:采集质量不达标(流式靠组装/合成兜底),emit 与 UI 选项均已注释(recorder.go / flag_registry.go / RecordingV2Control),响应路径重做(Phase 4 EventTap)后恢复。值域与内部采集(SetAssembledResponse)保留;存量选了该点位的配置只落 request 点位,行为有测试钉死 |

> **当前支持面 = 两个 request 点位。** 响应侧(服务返回 + 最终返回)整体
> 暂停,待 Phase 3(wire)/ Phase 4(EventTap)分别恢复;暂停以注释形式
> 保留代码位置,恢复时取消注释即可。

> **命名:`final_response` 而非 `client_response`。** `client_request` 里
> "client" 是来源(客户端发出的请求);若照抄成 `client_response`,"client"
> 就变成了目的地(发给客户端的响应)——同一前缀在请求/响应两侧含义相反,
> 读起来像是"客户端产生的响应"。`upstream_response` 不受影响,因为
> "upstream 的响应"本来就是来源性描述。改用 `final_response` 对齐已经在
> 用的 `obs.Record.FinalResponse` / json key `final_response`,也对齐这里
> 一直沿用的"最终返回"人话说法。

- **旧值兼容**:`request` → 出站;`request_response` → 出站+最终;
  `staged_request_response` → 入站+出站+最终。`typ.ParseRecordingMode`
  统一归一化(去重、按管线序排序、未知 token 丢弃),存量配置零迁移;
  写入口(rule/scenario)用 `IsValidRecordingMode` 严格校验拒绝未知 token,
  并存归一化形态,配置随触碰逐步收敛到点集形式。
- **继承**:`RuleFlags.Recording`(registry key `recording`,新类型
  `multi_enum`,Shared/override)覆盖 scenario 级 `recording_v2` 默认,
  与 `thinking_effort` 同模式;解析点 `typ.EffectiveRecording(rule, scenario)`
  (handler prologue)与 `resolveRuleFlagsWithScenario`(ctx 传播)。
- **过滤位置**:recorder 构造时归一化 mode,`emit()` 按 `Has(point)` 挑
  字段;`client_request` 挂链路层 StagePre,`upstream_request` 挂 client
  层最内层 wire transport(Phase 3 迁移,见 §3.7;两者都靠 `recorder.Wants`
  nil-safe 判定是否采集)。obs 层不再校验三档枚举,sink 只认"非空即
  启用"——mode 语义完全归 typ。
- **sink 归属**:仍按 scenario 建目录/缓存(`GetOrCreateScenarioSink`
  改为接收请求的 effective mode,rule 开、scenario 关也能建 sink);
  录多深由 recorder 按请求过滤,sink 自身 mode 仅剩创建信息。
- 行为护栏:`protocoltest` 的 flag 套件新增 `recording` 用例(rule 级
  flag 单独启用 → gzip JSONL 落盘 → 断言 slim record 恰好带所选点位;
  测试侧经既有导出面 `GetOrCreateScenarioSink` + `obs.Sink.ForceFlush`
  冲刷,不为测试新增生产 API)。

### 3.6 Recorder 生命周期收敛(request 录制做扎实)

定位修正:recorder 是**高层的、请求级的观察者组件**,不是协议代码临时
起意创建/发射的东西——协议代码只向它**汇报**,发射一次、发生在高层收口。

原状的问题(与定位相反):
- 4 个 handler prologue 各自复制同一段创建逻辑(启用判定 + body
  marshal + Ensure);
- `RecordError` / `RecordResponse` 散落在协议派发代码 40+ 处,且
  `RecordError` **当场 emit**:failover 可重试的尝试失败、MCP 循环的
  continuation 哨兵(`ErrMCPStreamContinue`)都会提前发射——第一条
  emit 后 `release()` 清空字段,后续 emit 产出空壳垃圾记录。

收敛后的生命周期(`recording.ProtocolRecorder` 文档为准):
0. **统一从 ctx 取,不走签名**(前置重构,独立 PR):recorder 不作为
   显式参数在协议派发链里层层透传(原 40+ 处签名已全部去参)。gin 域
   内的函数一律 `recording.FromGin(c)` 按需自取(创建点已存 gin ctx,
   nil-safe)——与 rule flags 走 ctx 而非参数列表同构。仅三类例外保留
   显式参数:`RunGeneric*NonStream`(只有 `context.Context`、无 gin ctx
   的叶子 helper,由 gin 域调用方取好传入);recording 包自己的构造 API
   (`NewStreamRecorder` / `AttachRecorderHooks` / `NewTransformRecorder`
   / `newServerOpsAdapter`);`handlePreStreamFailure` 的窄接口参数
   (两种 recorder 实现共用)。
1. **单一创建点**:`ProtocolHandler.BeginRuleRecording`(rule 解析后的
   prologue,唯一知道 rule 级 flag 的最早时刻),4 个 handler 各一行;
   禁用时返回 nil,全部下游方法 nil-safe。
2. **协议代码只汇报**:`RecordError` 改为**只记不发**(暂存 lastErr,
   40+ 调用点零改动、语义整体修正);Stage 录制、流式 hooks、
   `SetAssembledResponse` 本就是汇报。
3. **发射恰好一次**(emit latch):
   - `RecordResponse`(终态成功)现场 emit,幂等;
   - `FinalizeIfPending` 在 `DispatchWithPriorityFailover` 顶部 defer
     ——所有网关请求的唯一高层收口(注意单服务 early-return 也被覆盖),
     兜底发射失败/中止请求的记录并带上最后暂存的错误。
   失败尝试 + failover 成功 = 一条干净的成功记录(绑定胜出 provider);
   全部失败 = 一条带错误的记录;请求侧捕获永不丢失。

生命周期护栏:`recording/lifecycle_external_test.go`(错误只记不发/
失败后成功不带残留错误/重复成功只发一条/中止请求 finalize 兜底)。

尚未做(记入 Phase 4):`RecordResponse` 调用点(20+)仍在协议派发代码
里——彻底"阶段化"要等 obs PLANNING 的 `RecordCtx`/EventTap,把成功侧
汇报也并入管线观察者;本次先把错误侧与发射权收上来。

### 3.7 `upstream_request` 迁到 client 层最内层 wire transport(Phase 3,收窄为仅 request)

修正 §1.3 一直声明、但 Phase 2 尚未兑现的设计:出站请求录制的正确位置
是 client 层,拿到**真正发出去的请求**,而不是链路层的 SDK 参数快照
(P6)。范围明确收窄——**只做 request,不做 response**;`upstream_response`
点位仍不放开。

**机制**(`internal/client/wire_recorder.go`):

- `WireRecorder` 接口(`WantsUpstreamRequest() bool` +
  `RecordWireRequest(method, url string, body []byte)`)
  定义在 `internal/client` 里,不反向依赖 `protocolserver/recording`——
  `*recording.ProtocolRecorder` 通过新增的同名方法结构性满足这个接口
  (与 `typ.GetRuleFlags` 跨界同一手法,鸭子类型免掉包依赖)。
- `wireRecorderTransport` 只读、不改写请求,通过 `WithWireRecorder(ctx, rec)`
  从 ctx 取 recorder;`BeginRuleRecording` 在创建 recorder 后立即把它挂进
  `c.Request` 的 ctx。
- **挂载位置 = 最内层**,紧贴真正碰 wire 的 transport,在 `ruleFlagTransport`
  / vendor round-tripper **之内**:
  - `createSessionBoundTransport`(`http.go`)内部包一次,一次性覆盖它的
    全部调用方——Anthropic/Google 的 OAuth 分支、Codex、Kimi、Gemini、
    Antigravity;
  - 另外 3 处直接调用 `GetGlobalTransportPool().GetTransport(...)` 的
    call site(`openai.go`、`anthropic.go` 的 `anthropicTransport`、
    `google.go` 的非 OAuth 分支)各自显式包一层。
  - 只读的性质使它**不受**"vendor 链不挂 `ruleFlagTransport`"这条不变式
    约束(那条不变式挡的是改写);由于它在 RoundTrip 调用链的最内层
    (outer 层先执行自己的 mutation 再调用 inner.RoundTrip),挂在这个
    位置时,extra_headers、custom UA、vendor 握手 header 等所有外层的
    改写都已经体现在 `req` 上——捕获到的就是即将发出的真实字节。
- **不采集 header**:请求头常年带凭证(`Authorization`/`x-api-key`,以及
  `extra_headers` 里任意自定义的密钥型字段),而记录会落到磁盘文件上。
  与其维护一份名称片段/denylist 式的脱敏策略(参考
  `logging_roundtripper.go::redactProxy` 的先例),不如干脆不采集 header——
  `RecordWireRequest` 签名里没有 headers 参数,structurally 就不会录到。
  只采 method/URL/body,安全性由构造保证而非依赖脱敏规则;如未来确有
  需要,再评估加回来。
- **advisor loopback 隔离**:advisor 自己的出站 LLM 调用与主请求共享同一
  条派生 ctx(`context.WithoutCancel`,值不受影响),若不处理会把主请求的
  `upstream_request` 覆盖成 advisor 内部调用的数据。`advisor_call.go` 在
  `WithAdvisorLoopback` 旁新增 `client.WithoutWireRecorder(ctx)`
  显式遮蔽继承值(存字面 `nil` 会在 `ctx.Value` 处丢失,故用一个
  no-op 实现遮蔽,而不是尝试存 nil)。
- **链路层收敛**:`buildTransformChain` 不再注册 `upstream_request` 的
  StagePost 步骤;`recording_transform.go` 的 `TransformRecorder` 简化为
  只做 `client_request`(StagePre),不再有 stage 概念。

**已知缺口,本次刻意不动**:排查发现 **Claude Code OAuth
(`NewClaudeClient`/`ClaudeClient`)从不经过 `createSessionBoundTransport`
或连接池**——它直接用 Anthropic SDK 的默认 `http.Client` 构造
(`anthropic.NewClient(options...)`,从未见过 `WithHTTPClient`),连
`wrapWithLogging` 都没有。这是与本次改动无关的、更早就存在的架构缺口
(Claude Code OAuth 是最敏感的握手路径,给它接 transport 是单独一块更大
的风险,本次不顺手做)。意味着 Claude Code 场景下 `upstream_request`
暂时仍录不到——留给未来单独评估。

**测试**:`internal/client/wire_recorder_test.go`(捕获/放行/不采集
header 的钉子测试/body 复原/loopback 遮蔽/nil 安全);`protocoltest` 的
`recording` flag 用例升级为端到端断言(真实 upstream URL、
`transformed_request.headers` 恒为空)——跑在真实 `net/http.Transport`
打到 httptest mock provider 的全链路上,不是单元级 mock。

## 4. 目标架构(to-be)

```
                     启用判定(一次,handler 入口)
   scenario flag (recording_v2, 场景默认) ──┐
   rule flag (recording, override 继承) ────┤→ EffectiveRecording / BeginRuleRecording
                                              │
                                              ▼
              record 实体(per-request recorder)创建/禁用
                     │ 挂 gin ctx + request ctx(WithWireRecorder),全链传播
                     ▼
   链路层: StagePre 录 client_request(原始入站请求)          ← 现状
                     ▼
   client 层: 最内层 wire transport 录 upstream_request      ← ✅ Phase 3
     (真实 method/URL/body,header 不采集,ctx 里有 recorder 才录)
                     ▼
              sink(per-scenario)裁剪按 recorder.Wants 逐点位   ← 现状
```

要点(✅ = 已落地;其余为 response 恢复时的后续):

1. ✅ **Flag 建模**:`RuleFlags.Recording`(`multi_enum`,采集点位多选,
   见 §3.5),`Shared: true`,`InheritanceMode: "override"`(rule 显式
   设置 > scenario `recording_v2` 默认)。`FlagCategoryObservability`。

2. ✅ **Record 实体传播**:recorder 由 `BeginRuleRecording` 创建后,除
   gin ctx 外同时挂进 `c.Request.Context()`(`client.WithWireRecorder`),
   client transport 用 ctx 取用——与 `typ.GetRuleFlags` 同一手法。
   "存在但未启用"时为 nil,下游全部方法 nil-safe,零成本。

3. ✅ **Client 层收敛**(§3.7):`wireRecorderTransport` 挂最内层 wire
   transport,只读不写,vendor 链也覆盖(Claude Code OAuth 除外,见
   §3.7 已知缺口)。`RecordRoundTripper` 已整体删除(Phase 1)。

4. ✅ **覆盖补齐**:OpenAI Chat / Responses handler 接上 recorder(P5,
   Phase 2)。

5. **Mode 语义收敛(response 部分待续)**:request 侧(client_request +
   upstream_request)已完整落地;response 侧(`upstream_response` /
   `final_response`)仍暂停,待 Phase 4 EventTap 把响应侧汇报也并入
   管线观察者后再开放,裁剪逻辑(`recorder.Wants`)已经是统一模型,
   届时只需补 producer,不用再改选点/继承那一层。

---

## 5. 开放问题(已拍板项标注)

- ~~CLI `--record-mode` 的去留~~ **已拍板(Phase 1)**:废弃,启用完全
  归 flag 体系;落盘目录固定 `<configDir>/record`。
- ~~Sink 归属~~ **已拍板(Phase 2)**:rule 级启用仍写 scenario sink,
  落盘按 scenario 组织;录多深由 recorder 按请求过滤。
- ~~`recording_v2` 字段名~~ **已拍板(Phase 2)**:scenario 级保持
  `recording_v2` json key(兼容存量),rule 级用 `recording`;两级的值
  统一为点位集合(旧枚举解析层兼容)。
- ~~`upstream_request` 挂载位置~~ **已拍板并落地(Phase 3)**:client 层
  最内层 wire transport,只读,详见 §3.7。
- ~~Header 脱敏策略~~ **已拍板(不采集)**:干脆不采集 header,避免维护
  脱敏策略,见 §3.7。
- **响应流录制在 client 层还是 chain 层**(仍开放,待 response 恢复时
  拍板):chain 级已有流式 hooks,client 层再录 wire 响应会重复;倾向
  client 层只录 wire **请求**(已完成),`upstream_response`
  新增时机与 `final_response` 恢复时机是否绑定,留到 Phase 4 一并定。
- **OpenAI 纯透传流式的 `final_response` 质量**(仍开放):该路径无
  chunk tap,final 由 writer 状态合成(仅 status/headers);补 tap 归入
  Phase 4(EventTap)。
- **Claude Code OAuth 的 `upstream_request` 覆盖**(新,仍开放):
  `NewClaudeClient` 从不经过 `createSessionBoundTransport`/连接池,连
  `wrapWithLogging` 都没有,是与本次改动无关的更早缺口(§3.7)。要覆盖
  这条最常用的路径,需要单独评估给它接一层 transport 的风险(OAuth
  握手路径,最敏感)。

---

## 6. 分阶段落地(防止一次改动过大)

| 阶段 | 内容 | 涉及 | 风险 |
|------|------|------|------|
| **Phase 0 ✅** | 本梳理文档 | `.design/recording.md` | 无 |
| **Phase 1 清障 ✅(收窄范围)** | 已做:删 `RecordRoundTripper` 死代码与全部 `SetRecordSink` 机制;`ScenarioContextKey` 迁出(P7);删 `ClientPool.recordSink` / `server.recordSink` / `server.recordMode` / `WithRecordMode` / `WithRecording`;去除 CLI `--record-mode` / `--record-dir`(目录固定默认)。**刻意未动**:advisor/MCP 侧接线(`WithAdvisorRecordSink`、`HookDeps.GetScenarioSink`)与 P4 header 修复——单独小步处理 | `internal/client`、`internal/server`、`internal/command`、`gui/wails3`、`vmodel` | 低(删死代码,行为不变) |
| **Phase 1.5 advisor 小步 ✅** | 修 P4(advisor ctx 标记 + 通用链只读 header transport,附单测);清 `WithAdvisorRecordSink` / `GetScenarioSink` 死数据注入 | `internal/client`、`mcp/runtime`、`servertool` | 低 |
| **Phase 1.8 前置重构 ✅(两个独立 PR)** | (a) recorder ctx 统一:协议签名去参,`recording.FromGin` 唯一取用入口(§3.6 第 0 条,#1649);(b) `recording` 包提升为 `internal/recording`(高层组件,不寄居协议包下,纯 mv);(c) flag 基建先行:`multi_enum` 类型 + registry 约束 + 前端 registry 驱动复选控件(通用能力,不含 recording 语义)——先调 flag 体系本身,recording 的语义变更单独渐进 | `protocolserver`、`internal/recording`、`typ`、frontend | 低(纯重构/纯新增能力) |
| **Phase 2 flag 融入 ✅(含点位模型重构)** | 采集点位多选模型(§3.5);`RuleFlags.Recording` 进 registry(multi_enum,Shared/override);继承 + 四个 handler 接线 + OpenAI 透传路径补 emit(修 P5);写入口校验/归一化;`RecordingV2Control` 多选化 + codegen;flag 行为套件补 `recording` 用例 | `typ`、`obs`、`server`、`protocolserver`、`protocoltest`、frontend | 中 |
| **Phase 3 wire 录制 ✅(收窄为仅 request)** | `wireRecorderTransport` 挂 client 层最内层 wire transport(含 vendor 链,Claude Code OAuth 除外——见 §3.7 已知缺口);recorder 经 request ctx 传播(`WithWireRecorder`);链路层 StagePost 移除,`upstream_request` 全部由 wire 层产出(修 P6);advisor loopback 显式遮蔽(`WithoutWireRecorder`);header 不采集(免脱敏策略) | `internal/client`、`protocolserver` | 中 |
| **Phase 4 obs 汇合** | 与 `internal/obs/PLANNING.md` Phase 2 合流(RecordCtx / EventTap / ModeFilterExporter);response 侧两个点位(`upstream_response` 新增、`final_response` 恢复)在此阶段一并做;scenario 前端控件 registry 化;Claude Code OAuth 的 transport 覆盖单独评估 | `obs`、`transform`、`internal/client`、frontend | 按其自身计划 |

Phase 1 与 Phase 2 互不依赖,可并行;Phase 3 依赖 Phase 2(recorder 的
启用判定先统一)。每阶段独立 vet / test 绿。

---

## 7. 与现有文档的关系

- `.design/rule-flags.md`:`recording_v2` 已同步标注为升级到 shared flag
  `recording`(共享表 + scenario-only 表),Phase 3 的挂载位置变更(链路层
  StagePost → client 层 wire transport)也已同步进该文档的链路骨架描述。
- `internal/obs/PLANNING.md`:record 采集/导出侧的权威规划;本文的
  Phase 4 即与其合流点。两文档口径一致:record 实体总是构建,裁剪在
  出口。
- `.design/user-agent.md` / rule-flags.md §8:vendor 链不变式只约束
  **改写型** transport;只读的 recordTransport 挂 vendor 链不在禁区,
  但新增时仍须逐链核对(Gemini 清空 header 的链要确认挂载点在清空
  之后)。
