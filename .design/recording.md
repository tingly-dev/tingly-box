# Recording（录制）：重做规划

> 适用对象：tingly-box 后端 / 前端贡献者。
> 图示：`.design/recording.pencil.md`（章节一一对应）。
> 状态：**规划已确认；R1（接入）待实施**。
> 本文取代旧版 recording 梳理（Phase 0–2，见 git 历史）。旧版沉淀下来、仍然成立的结论
> 在 §6 列出；旧实现（`ProtocolRecorder` + `TransformRecorder`）按 §5 的阶段退场。

---

## 1. 为什么重做

协议层迁到 Protocol Stage 管线之后（`.design/protocol-stage-pipeline.md`），旧录制已基本失效。
问题不在某一处 bug，而在设计本身——**采集点长在协议代码里**：

| # | 问题 | 可验证的表现 |
|---|------|-------------|
| D1 | 录制依附协议路径。`recording.FromGin` / `recorder.*` 散落在 `internal/protocolserver` 约 50 处调用（transform 链两端、passthrough、dispatch、cross、两个 Stage adapter、error_response、MCP helper）。每次协议迁移都要逐条路径重新接线，漏接就静默丢记录 | FP3：Responses 客户端 → Anthropic / Chat provider 时 rule 级录制不出记录（`protocol-stage-pipeline.md` 偏差 8） |
| D2 | 录到的不是真实请求。StagePre / StagePost 是对 SDK 参数对象 `json.Marshal` 的快照：没有 wire header、没有 SDK 序列化时补的字段，也看不到 vendor round-tripper（Claude OAuth、Codex、Gemini envelope、Kimi …）的改写 | 记录里 `headers` 恒为空 map |
| D3 | 一次请求只有一个 "transformed" 槽。Stage 路径每轮 provider 调用（server tool 循环）各跑一次 StagePost，failover 每个 attempt 也会重写——多轮、多 attempt 只剩最后一次 | 带 MCP 工具循环的请求只能看到最后一轮 |
| D4 | 响应侧做不出来。`final_response` 靠 assembler / 合成兜底，质量不达标已暂停；`upstream_response` 没有生产者 | 当前 UI 只放开两个 request 点位 |
| D5 | 生命周期分散。`RecordResponse` / `RecordError` 由各 handler 分支自己调，漏调即丢；还要处理 nil-safe、release 等样板 | 每条新路径都要重复这套样板 |
| D6 | 没有查看面。落盘的 gzip JSONL 没有任何 API / UI 读取；Prompt 页的 `listUserRecordings` 仍是占位 | 用户开了录制也看不到结果 |

D1–D5 的共同根因：**录制试图在协议内部"理解"请求**。而录制真正需要的东西只存在于两条边界上——
客户端进来的 HTTP、发往 provider 的 HTTP。协议内部怎么变，边界上的字节都是事实。

---

## 2. 新设计的三条原则

1. **只在边界录，录 wire 字节。** 入站边界（gin 中间件）与出站边界（最贴近 wire 的
   `http.RoundTripper`）各一个采集点。协议代码**不知道录制存在**——Stage / Bridge / Transform /
   failover 的任何迁移都不需要碰录制。
2. **一次入站请求 = 一条 Trace；每次出站 HTTP 往返 = 一个 Exchange。** 多轮工具循环、failover、
   并发 fan-out 都自然表现为多个 Exchange，不存在"槽被覆盖"。
3. **启用判定一次，生命周期一处。** 是否录、录哪些点位仍走 flag 体系（rule `recording` 覆盖
   scenario `recording_v2`，`typ.EffectiveRecording`，不变）；Trace 的创建与落盘只发生在中间件，
   handler 只做一件事：在 rule 解析完成后 `Enable`。

---

## 3. 数据模型

```
Trace（一次入站请求）
├─ meta        id（= access-log request id）、ts、scenario、rule uuid、session、duration、error、points
├─ inbound
│   ├─ request    method / url / headers / body        ← client_request
│   └─ response   status / headers / body 或 SSE 原文 ← final_response
└─ exchanges[]（按发起顺序）
    ├─ seq、provider（name / uuid / api style）、started_at、ttfb、duration、error
    ├─ request    method / url / headers / body        ← upstream_request
    └─ response   status / headers / body 或 SSE 原文 ← upstream_response
```

- **四个点位与模型一一对应**，现有 `typ.RecordingPoint` 值域、存量配置、前端多选控件全部沿用，
  语义只是从"SDK 快照"变成"wire 字节"。
- **Body 存原文**：合法 JSON 以 `json.RawMessage` 内嵌，否则（SSE、二进制、截断）存字符串并标
  `encoding`。流式响应存 SSE 原文，不做组装——组装是查看端的事。
### 3.1 四个点位 × 流式 / 非流式

录制的对象只有这四种（与 rule `recording` 的选项一致），不录协议转换的中间过程：

| 点位 | 是什么 | 采集位置 | 非流式 | 流式 |
|------|--------|----------|--------|------|
| `client_request` 入站 | 客户端发来的原始请求 | 入站中间件 / handler 交给 `Enable` 的 `bs` | JSON body | 同左——请求侧没有流，只是 body 里 `stream: true` |
| `upstream_request` 出站 | 经过转换、发给 provider 的请求（debug 用） | `wireRecordTransport` | JSON body + 真实 header | 同左（Google 是 URL 上的 `:streamGenerateContent?alt=sse`） |
| `upstream_response` 出站返回 | provider 返回的原始响应 | `wireRecordTransport`，tee response body | 整个 JSON body，读完即收尾 | SSE 原文，随 SDK 读取逐段 tee；EOF 收尾为完整，提前 `Close`（客户端断开、failover 放弃）收尾并标 `incomplete` |
| `final_response` 回到客户端 | 网关写回客户端的响应 | 入站中间件包装 `ResponseWriter` | 整个 JSON body | SSE 原文（含 keep-alive 注释），按 `Write` 顺序 tee；连接中断标 `incomplete` |

流式相关的统一规则：

1. **流不流式看响应，不看请求。** 以响应 `Content-Type: text/event-stream` 判定 `stream: true`。
   请求了流式但在首字节前失败（上游 4xx / 5xx、网关报错）时，响应是普通 JSON 错误，按非流式记。
2. **SSE 存原文，不组装。** 不在热路径上把事件拼回一条 message——旧设计 D4 的质量问题正出在
   组装 / 合成上。原文无损，拆事件、拼消息、展示 delta 都放到查看端（R3）。
3. **时间点分开记。** 每个响应记 `ttfb`（首字节）与 `duration`（收尾），流式下两者差异就是生成耗时。
4. **收尾时机。** Trace 在入站中间件 `c.Next()` 返回后 Emit；此时仍未收尾的上游流（理论上
   handler 返回前都已读完或关闭）按 `incomplete` 落盘，不阻塞 Emit。

- **大小上限只做安全阀**：请求 body 本来就整份在内存里（SDK 已序列化），录制只是多持有一份引用
  / 拷贝，不按小上限截断——长程任务的请求动辄数 MB，截断就等于没录。上限（暂定 32 MiB / body）
  只防异常，超出截断并标 `truncated`。体积问题由 §3.2 的去重解决，而不是靠截断。
- **脱敏在采集时做**：`Authorization`、`x-api-key`、`api-key`、`Cookie`、`Set-Cookie`、
  `x-goog-api-key` 等凭据 header 只保留前后几位。录制文件不应成为凭据泄露面。

### 3.2 长程任务的成本：按元素去重

**问题。** 长程 agent 任务里，每一轮请求都带着完整历史：第 n 轮的请求 ≈ 第 n−1 轮的请求 + 上一轮
回答 + 新的工具结果。每轮整份录下来，一个会话的录制体积是 **O(n²)**——上下文涨到 400 KB、跑
200 轮，单个点位就要录约 40 MB，而其中 95% 以上是重复的。`client_request` 与 `upstream_request`
在同协议路径上几乎逐字相同，开两个点位又再翻一倍。

**做法：请求 body 按"顶层数组的元素"切块，内容寻址，会话内只存一次。**

```
请求 body（wire JSON）
{ "model": "...", "system": [...], "tools": [t1 … t40], "messages": [m1 … m200], "stream": true }
          │ 顶层数组 → 逐元素；其它顶层值 → 超过阈值整体切出；小值留在骨架里
          ▼
骨架（每条 Trace 都存，与轮数无关，<1 KB）           块（分区内首次出现才存）
{ "model": "...", "system": ⟨S1⟩,                      元素块  m200 → 本轮新消息原文
  "tools": ⟨T40⟩, "messages": ⟨M200⟩,                  链节点  M200 = {prev: M199, elem: m200}
  "stream": true }                                     （M1…M199、T1…T40 上一轮已存在，不再写）
```

数组不在骨架里逐个列引用（那样骨架本身随轮数线性增长，整个会话仍是 O(n²)），而是编码成
**前缀链**：第 k 个链节点 = {前一个节点, 第 k 个元素}，骨架只引用最后一个节点。

1. **切分规则与协议无关**：只看 JSON 结构——顶层数组逐元素切，不认 `messages` / `input` /
   `contents` 这些字段名。Anthropic、Chat、Responses、Gemini 的历史都在顶层数组里，自然命中；
   以后新协议无需改录制（与原则 1 一致）。
2. **哈希原始字节**：从 wire 字节切出每个元素的原文（`json.RawMessage`，不反序列化成 map 再编码），
   sha256 取键。还原时沿链回溯、把原文拼回骨架，得到语义等价的 JSON（元素间空白不保留，元素内逐字节一致）。
3. **前缀链让每轮成本只与"变化量"有关**：追加 k 条消息 = 写 k 个新元素块 + k 个链节点；
   第 i 个元素被改（例如 prompt cache 断点后移改了上一条消息）= 从 i 起的节点重写，通常 2–3 个；
   上下文压缩 / 改写历史 = 一次性重写整条链，之后照常增量。
4. **去重范围 = 一个落盘分区（scenario / 日期 / 会话）**，所有点位共享同一个块空间：
   - 同一会话的第 n 轮只新增本轮变化的元素与对应链节点；
   - `client_request` 与 `upstream_request` 里没被转换改动的元素互相去重，多开一个点位的边际
     成本只剩"真正被改过的部分"；
   - `tools` 这种每轮重复、动辄几十 KB 的数组只存一次；反复出现的图片 base64 同理。
   - 分区就是保留期的删除单位：删目录即可，没有跨分区引用，不需要引用计数 / GC。
5. **块和记录写在同一个文件里**：分区文件是 gzip member 追加的 JSONL，一行要么是块
   `{"k":"blob","h":"…","d":<原文>}`，要么是记录 `{"k":"trace",…骨架…}`；块总在第一次引用它的
   记录之前写出。一个分区一个文件，不产生海量小文件（旧 CAS 导出器"一块一个文件、全局 blobs
   目录、无回收"的问题不再出现）。读端顺序扫描一遍即可还原；R3 需要随机访问时再加旁路索引。
6. **热路径不做任何解析**：请求处理中只持有 body 字节；切块、哈希、去重全在异步导出 worker 里做。
   导出队列满时丢弃整条 Trace 并计数（录制永远不反压业务请求）。
7. **响应不切块**：上游返回 / 回到客户端的响应每轮都是新内容（SSE 原文），整体存，只走 gzip。
   它们会在下一轮请求里以 assistant 消息的形态再次出现，但编码不同，不尝试跨形态去重。
8. **去重索引**：导出 worker 为每个打开的分区维护已写块的哈希集合（LRU 管理打开的分区）。
   进程重启后首次写入某个已有分区时，集合为空，已有的块会再写一次——代价是每会话每次重启最多
   重复一份上下文，换来不必在启动时扫描历史文件。

**量级估算**（上下文终值 400 KB、200 轮、开 `client_request` + `upstream_request`）：
不去重约 80 MB（gzip 前）；去重后 ≈ 上下文终值 400 KB + 200 轮 × 两个点位 × （骨架 <1 KB +
被改动的 1–2 条消息）≈ 1–2 MB，再经 gzip。轮数再翻十倍，增长也只是线性的。

---

## 4. 采集与生命周期

```
gin: contextMiddleware → recordingMiddleware ─────────────────────────────┐
                              │ 创建 Trace（未启用），放入 request ctx     │
                              │ 包装 ResponseWriter（未启用时零拷贝透传）   │
                              ▼                                            │
handler 前段：解析 rule / scenario → EffectiveRecording                    │
              → recording.FromContext(ctx).Enable(mode, scenario, rule, body)
                              ▼                                            │
协议管线（Stage / 旧整链 / passthrough / failover …）——对录制无感知        │
                              ▼                                            │
client：logging → advisorLoopback / vendor → ruleFlag round-tripper…                       │
          → wireRecordTransport（只读）→ wire base                         │
             Trace 已启用 ⇒ 追加 Exchange：录 request；tee response body，  │
             读到 EOF / Close 时收尾                                        │
                              ▼                                            │
recordingMiddleware 在 c.Next() 返回后：Trace 已启用 ⇒ 收尾并 Emit ◄────────┘
                              ▼
                     sink（per-scenario，批量异步落盘）
```

### 4.1 出站：`wireRecordTransport`

挂在 **wire base 之上、所有改写型 round-tripper 之内**，看到的就是真正发出去的请求。
wire base 目前有两种形态，挂载点有限且集中在 `internal/client`：

| wire base | 装配点 |
|-----------|--------|
| `TransportPool.GetTransport(...)`（`*http.Transport`） | `openai.go` `NewOpenAIClient`、`anthropic.go` `anthropicTransport`、`opencode_client.go` `openCodeTransport`、`google.go`（非 OAuth） |
| `SessionBoundTransport` | `http.go` `createSessionBoundTransport`（Claude OAuth、Codex、Kimi、Gemini、Antigravity、xAI、Google OAuth 共用） |

- 只读不改写，所以 vendor 链也可以挂（`rule-flags.md` §8 的不变式约束的是改写型 transport）。
- Trace 未启用时只有一次 ctx 取值 + 布尔判断，没有任何拷贝。
- 不走这两种 base 的 client（Bedrock 等走自带 SDK transport 的）在 R1 列为覆盖缺口，逐个确认。
- 已知偏差：Go `http.Transport` 自己补的 header（`Accept-Encoding: gzip`、`Content-Length`）
  在外层看不到；透明解压后的响应体是解压后的明文——对"看请求内容"无影响，记录在案即可。

### 4.2 入站：`recordingMiddleware`

只挂在四个模型入口（Anthropic V1 / Beta、OpenAI Chat / Responses）的路由组上。

- 请求 body：handler 本来就读出了 `bs`，`Enable` 时直接交给 Trace，中间件不额外缓冲。
- 响应：包装 `gin.ResponseWriter`，每次 `Write` 只判断 Trace 是否启用，启用才 tee（带上限）。
  SSE 原样落下，状态码 / header 取写出时的真实值——这正是旧 `final_response` 做不到的。
- 收尾与 Emit 只在这里发生一次，覆盖所有成功 / 失败 / panic 恢复后的分支。

### 4.3 与 failover / 内部调用的关系

- failover 的每个 attempt 是独立的 Exchange，`provider` 字段区分；不再需要 `SetActiveService`。
- advisor / MCP 等网关内部发起的模型调用如果复用同一 request ctx，会自然作为 Exchange 出现；
  loopback 回到本网关的调用会产生自己的 Trace——两者用 `X-Tingly-Advisor-Depth` 关联（R4）。

---

## 5. 分阶段落地

| 阶段 | 内容 | 完成标准 |
|------|------|----------|
| **R1 接入** | `internal/recording` 新增 Trace / Exchange 实体与 ctx 传播（`WithTrace` / `FromContext`）；`wireRecordTransport` 挂到 §4.1 全部装配点；`recordingMiddleware` 挂四个入口；四个 handler 前段 `Enable`；新 schema 落盘，**从第一天起就用 §3.2 的去重格式**（落盘格式是 R3 查看端读取的契约，不先写整份再迁移）。点位先录 `client_request` / `upstream_request` / `upstream_response`。旧 recorder 保持原样并存 | 单测覆盖 transport / middleware / 截断 / 脱敏 / 切块去重与还原（还原结果与原 body 语义等价）；`protocoltest` 新增 recording 用例跑满 source × target × 流式矩阵（FP3 一并消失），断言每个组合都有 Trace、Exchange 数与 provider 调用数一致 |
| **R2 收口** | `final_response` 由中间件 tee 产出并在 UI 放开 `upstream_response` / `final_response`；删除旧 recorder 全部接线（`ProtocolRecorder`、`TransformRecorder`、`AttachRecorderHooks`、`recording.FromGin` 及 handler 里的 `Record*` 调用、`obs.Record` 旧字段）；更新 `protocol-stage-pipeline.md`（"Vendor 之后只有录制"一条随之改为"Vendor 是最后一步"） | `internal/protocolserver` 里 `recording` 引用只剩 handler 前段的一行 `Enable` |
| **R3 查看** | 后端 list / get API（按 scenario、日期、session、rule、provider、错误筛选，分页）+ codegen；前端录制查看页：Trace 列表 → 详情（入站 / 各 Exchange 时间线、请求 / 响应 / SSE 事件分栏、diff 入站与出站）。按 UX 原则"为下一步动作露出产物"：开启录制的 rule / scenario 处直接链到它的录制 | 用户开启录制后无需碰文件系统即可看到结果 |
| **R4 治理** | 保留期与磁盘配额（按天 / 按大小清理）；条件录制（仅错误、采样）；导出（cURL 重放、HAR）；advisor / loopback 关联；脱敏规则可配置 | 长期开启录制不会撑爆磁盘 |

每个阶段独立 vet / test 绿；R2 依赖 R1，R3 可在 R1 之后与 R2 并行（先读 R1 的 schema）。

---

## 6. 从旧设计继承的结论

- 启用走 flag 体系：rule `recording`（multi_enum，Shared / override）覆盖 scenario `recording_v2`；
  写入口严格校验、存归一化形态；旧枚举值（`request` / `request_response` /
  `staged_request_response`）解析层兼容。`internal/typ/recording.go` 不动。
- 落盘根目录固定 `<configDir>/record`，没有 CLI 开关。
- request id 与 access log 共用（`constant.CtxKeyRequestID`），录制与日志可互相定位。
- advisor 防递归 header 由 `client.WithAdvisorLoopback` + `advisorLoopbackTransport` 负责，
  与录制无关（旧版 P4 已修）。
- 点位命名用 `final_response` 而不是 `client_response`：`client_request` 的 "client" 是来源，
  照抄到响应侧会变成目的地，读起来像"客户端产生的响应"。

---

## 7. 决策记录

已定：

- **采集范围**：只录 §3.1 的四个点位（入站、出站、出站返回、回到客户端），不录转换中间过程；
  流式 / 非流式按 §3.1 的规则统一处理。
- **采集位置**：边界 wire 采集取代 transform 链内快照。
- **R1 新旧并存**：R1 新旧两套各写各的目录，R2 补齐 `final_response` 后一次性删除旧接线。
- **R1 范围**：`client_request` + `upstream_request` + `upstream_response`；`final_response` 放 R2。
- **长程任务的体积**：请求 body 按顶层数组元素内容寻址去重、数组编码为前缀链（§3.2），R1 起即生效。

待定（R1 实施时定，倾向写在前面）：

- **落盘**：分区文件 `<configDir>/record/traces/<scenario>/<date>/<session>.jsonl.gz`，
  块与记录同文件（§3.2）。`obs.BatchProcessor` 目前绑定 `*obs.Record`，倾向泛型化
  （`BatchProcessor[T]`）复用批处理，而不是再写一份队列；旧 CAS 导出器不跟进，R2 随旧 recorder 删除。
- **去重范围**：按分区（会话 × 天）而非全局。全局去重能多省跨会话共享的 system / tools，
  但需要引用计数或标记清除才能做保留期；按分区删目录即可。若 R4 观测到跨会话重复占大头，再考虑
  只对 `tools` / `system` 这类块做全局层。
- **body 安全上限**：32 MiB / body，R4 再做成配置项。
