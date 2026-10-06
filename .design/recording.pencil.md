# Recording 重做 (pencil)

`.design/recording.md` 的图示，二级标题与正文章节对应。契约与取舍以正文为准，这里只画流程、结构与数据形态。

图例：`▸` 采集点 · `║` 请求主路径 · `┆` 只读旁路（录制不改请求）· `@x` 内容寻址块 · `⟨X⟩` 前缀链节点 ·
`✗` 旧设计的问题 · `⟳` 每轮 provider 调用都发生

---

## 0. 价值定位

```
                 客户端本地记录                         只有网关看得到
            ┌──────────────────────┐        ┌──────────────────────────────────────┐
  client ──►│ 对话消息（客户端视角）  │──► GW ──►│ 转换后的请求 · 实际 provider / 模型       │──► provider
            │ ≈ client_request      │        │ failover 每次尝试 · provider 原始返回     │
            │ ≈ final_response 效果  │        │ = upstream_request / upstream_response   │
            └──────────────────────┘        └──────────────────────────────────────┘
                 价值：对比基准                     价值：定位转换 / 路由问题（核心）

  查看端 = 两组 diff：  入站 ↔ 出站请求        出站返回 ↔ 回到客户端
```

---

## 1. 为什么重做

旧设计把采集点埋在协议管线内部，新设计只在两条边界上采集。

```
  旧：采集点长在协议代码里（约 50 处调用）            新：只在两条边界上录 wire 字节

  client                                              client
    ║                                                   ║
    ▼                                                   ▼
  handler ── EnsureProtocolRecorder                   ┌─ recordingMiddleware ─────────┐ ▸ client_request
    ║                                                 │  （入站边界）                   │ ▸ final_response
    ▼                                                 └───────────────┬───────────────┘
  ② source 半段 ── ▸ StagePre（SDK 快照）✗ D2                         ║
    ║                                                                 ▼
  ③ 转换（Stage / BaseTransform）                      handler → Plan → ② → ③ → ④ → Vendor
    ║                                                   （协议管线：完全不知道录制存在）
    ▼                                                                 ║
  ④ target 半段 ⟳ ── ▸ StagePost（覆盖上一轮）✗ D3                     ▼
    ║                                                  client：vendor / ruleFlag / logging rt
    ▼                                                                 ║
  passthrough / dispatch / cross / stage adapter                     ▼
    ┆  各自调 RecordResponse / RecordError  ✗ D5      ┌─ wireRecordTransport ──────────┐ ▸ upstream_request
    ┆  漏接一条路径就静默丢记录（FP3）    ✗ D1        │  （出站边界，只读）⟳            │ ▸ upstream_response
    ▼                                                 └───────────────┬───────────────┘
  provider                                                            ║
                                                                      ▼
  响应：assembler 组装 / 合成兜底     ✗ D4                          provider
  落盘后无 API / UI 可看               ✗ D6
                                                      协议迁移（Stage / Bridge / Transform / failover）
                                                      对录制零影响：边界上的字节就是事实
```

## 3. 数据模型

一次入站请求 = 一条 Trace；每次出站 HTTP 往返 = 一个 Exchange。多轮、failover、fan-out 都是"多一个 Exchange"。

```
  Trace  rid = access-log request id
  ├─ meta      ts · scenario · rule · session · duration · error · points
  ├─ inbound
  │    ├─ request ......... ▸ client_request
  │    └─ response ........ ▸ final_response
  └─ exchanges[]
       ├─ #1  provider A   round 1          request ▸ upstream_request   response ▸ upstream_response
       ├─ #2  provider A   round 2 (tool)   request ▸                    response ▸
       ├─ #3  provider A   ✗ 5xx            request ▸                    response ▸（错误 JSON）
       └─ #4  provider B   failover         request ▸                    response ▸

  旧模型：一条 Record 只有 original / transformed 两个槽，#1–#4 只剩最后写入的那个
```

### 3.1 四个点位 × 流式 / 非流式

```
                         非流式                                 流式
                 ┌──────────────────────────────┬──────────────────────────────────────┐
  client_request │ JSON body（handler 已读出的 bs）│ 同左：请求侧没有流，只是 "stream": true │
                 ├──────────────────────────────┼──────────────────────────────────────┤
upstream_request │ JSON body + 真实 header         │ 同左（Google 在 URL 上 :streamGenerate…）│
                 ├──────────────────────────────┼──────────────────────────────────────┤
upstream_response│ 整个 JSON，读完即收尾            │ SSE 原文，随 SDK 读取逐段 tee           │
                 │                              │   EOF ............ complete             │
                 │                              │   提前 Close ...... incomplete           │
                 ├──────────────────────────────┼──────────────────────────────────────┤
  final_response │ 整个 JSON（Writer tee）         │ SSE 原文 + keep-alive，按 Write 顺序 tee │
                 │                              │   连接中断 ........ incomplete           │
                 └──────────────────────────────┴──────────────────────────────────────┘
```

流不流式看响应，不看请求：

```
  请求 "stream": true
        │
        ▼
  响应 Content-Type ?
        ├── text/event-stream ─────────► stream: true    SSE 原文；记 ttfb 与 duration
        └── application/json ──────────► stream: false   首字节前失败（上游 4xx/5xx、网关报错）
                                                          就是普通 JSON 错误，按非流式记

  时间轴（流式）
  t0 发出 ──────── ttfb（首字节）───────────────────────── duration（EOF / Close）
                   │◄──────── 生成耗时 = duration − ttfb ────────►│
```

### 3.2 长程任务的成本：按元素去重

**问题：每轮重发整段历史，整份录制随轮数平方增长。**

```
  轮次   请求 body（■ = 已出现过的内容   □ = 本轮新增）           整份录制累计
   1     [tools][sys] □                                          ▏
   2     [tools][sys] ■ □                                        ▏▏
   3     [tools][sys] ■ ■ □                                      ▏▏▏
   …
  200    [tools][sys] ■ ■ ■ ■ ■ ■ ■ ■ … ■ ■ □                    ▏▏▏▏▏▏▏▏▏▏▏▏ … ≈ 40 MB / 点位
                      └──────── 95%+ 是重复 ────────┘
  再开 upstream_request：同协议路径上几乎逐字相同  ──►  × 2 ≈ 80 MB
```

**切块：只看 JSON 结构，不认字段名。**

```
  wire body
  { "model": "claude-…",  "stream": true,  "system": [s1],  "tools": [t1 … t40],  "messages": [m1 … m200] }
     └──── 小的顶层值 ────┘                  └──────────── 顶层数组 → 逐元素切块 ──────────────┘
           留在骨架里                         （messages / input / contents 都自动命中）

  每个元素：从 wire 字节切出原文（json.RawMessage）→ sha256 → @hash
```

**前缀链：骨架大小与轮数无关。**

```
  只做元素去重的骨架（✗ 引用列表本身随轮数线性增长，会话总量仍是 O(n²)）
    "messages": [@m1, @m2, @m3, … , @m200]

  前缀链：每个节点 = {prev, elem}，骨架只引用最后一个节点
    "messages": ⟨M200⟩

    ⟨M1⟩ ◄── ⟨M2⟩ ◄── ⟨M3⟩ ◄── … ◄── ⟨M199⟩ ◄── ⟨M200⟩      ← 骨架只存这一个引用
     │        │        │                 │          │
    @m1      @m2      @m3              @m199      @m200
```

**每轮只写变化量**：

```
  ① 追加（最常见）：第 201 轮追加 1 条 assistant + 1 条 tool_result
     ⟨M1⟩ ◄─ … ◄─ ⟨M200⟩ ◄─ ⟨M201⟩ ◄─ ⟨M202⟩          新写：2 个元素块 + 2 个节点
                             ═══════════════

  ② prompt cache 断点后移：cache_control 从 m200 挪到 m202，m200 原文变了
     ⟨M1⟩ ◄─ … ◄─ ⟨M199⟩ ◄─ ⟨M200'⟩ ◄─ ⟨M201'⟩ ◄─ ⟨M202'⟩   新写：@m200' + 3 个节点
                              ═════════════════════════

  ③ 上下文压缩 / 改写历史：从第一个变化处起整条重写一次，之后回到 ①
     ⟨C1⟩ ◄─ ⟨C2⟩ ◄─ … ◄─ ⟨C30⟩                       新写：30 个元素 + 30 个节点（一次性）
```

**去重范围 = 一个分区，所有点位共享一个块空间。**

```
  分区：<configDir>/record/traces/<scenario>/<date>/<session>.jsonl.gz

          client_request                     upstream_request（经过转换）
   tools  ⟨T40⟩ ─────────────┐        ┌──────── ⟨T40⟩        tools 未改 → 同一条链
   msgs   ⟨M200⟩ ──┐         │        │   ┌──── ⟨N200⟩       只有被转换改过的元素
                   ▼         ▼        ▼   ▼                   才产生新块（@n…）
               ┌──────────────────────────────┐
               │  分区块空间  @m1…@m200 @t1…@t40 │  ◄── 两个点位共用；多开一个点位只多付
               │             @n7 @n200 …       │      "真正被改过"的那部分
               └──────────────────────────────┘

  保留期 = 删分区目录。没有跨分区引用 → 不需要引用计数 / GC
```

**一个分区一个文件：块与记录交错追加。**

```
  <session>.jsonl.gz   （gzip member 逐批追加）
  ┌────────────────────────────────────────────────────────────────────────────┐
  │ {"k":"blob","h":"t1…","d":{…tool 定义…}}                                     │
  │ …                                                                          │
  │ {"k":"blob","h":"M1…","d":{"p":null,"e":"m1…"}}        ← 链节点也是块          │
  │ {"k":"trace","rid":"r1","inbound":{…"messages":"⟨M1⟩"…},"exchanges":[…]}     │  第 1 轮
  │ {"k":"blob","h":"m2…","d":{…}}                                               │
  │ {"k":"blob","h":"M2…","d":{"p":"M1…","e":"m2…"}}                             │
  │ {"k":"trace","rid":"r2",…"messages":"⟨M2⟩"…}                                 │  第 2 轮
  │ …                                                                          │
  └────────────────────────────────────────────────────────────────────────────┘
  规则：块总在第一次引用它的记录之前写出 → 读端顺序扫一遍即可还原
  对比旧 CAS 导出器：一块一个文件 · 全局 blobs 目录 · 无回收   ✗

  <session>.index.jsonl   （旁路索引，每条 Trace 一行元数据）
  ┌────────────────────────────────────────────────────────────────────────────┐
  │ {"rid":"r1","ts":…,"rule":…,"provider":…,"status":200,"ms":…,"off":0}       │
  │ {"rid":"r2",…,"off":18234}                                                  │
  └────────────────────────────────────────────────────────────────────────────┘
  列表只读索引；打开详情才按 off 读数据文件并还原（元数据 / body 分离，见正文 §8）
```

**热路径与导出 worker 的分工。**

```
  请求处理（热路径）                                    导出 worker（异步，单 goroutine）
  ─────────────────────────────                        ───────────────────────────────────────
  只持有 body 字节，不解析                              按分区取已写哈希集合（LRU 管打开的分区）
        │                                              切块 → sha256 → 查集合
        │ Trace 收尾                                     ├─ 已有 → 只写引用
        ▼                                              └─ 新块 → 先写块行，再入集合
  Emit ──► 有界队列 ───────────────────────────────►    写骨架记录行
              │                                         批量 gzip member 追加
              └─ 队列满：丢弃整条 Trace + 计数
                 （录制永远不反压业务请求）               重启后集合为空：已有分区的块
                                                        至多再写一份（换来启动不扫描）
```

**量级（上下文终值 400 KB、200 轮、开两个请求点位，gzip 前）。**

```
  整份录制   ████████████████████████████████████████  ≈ 80 MB    O(n²)
  元素去重   ██▌                                         ≈ 4–5 MB   骨架引用列表仍 O(n²)
  + 前缀链   █                                           ≈ 1–2 MB   O(n)：终值上下文 + 每轮变化量
```

## 4. 采集与生命周期

```
  gin  contextMiddleware
         │
         ▼
  ┌─ recordingMiddleware ─────────────────────────────────────────────────────────────┐
  │  t := NewTrace()（未启用）→ request ctx                                            │
  │  包装 ResponseWriter：每次 Write 只判断 t.Enabled()，启用才 tee                     │
  │                                                                                   │
  │     c.Next() ─────────────────────────────────────────────────────────────┐       │
  │                                                                           │       │
  │     handler 前段：解析 rule / scenario                                     │       │
  │        mode := EffectiveRecording(rule, scenario)                          │       │
  │        recording.FromContext(ctx).Enable(mode, scenario, rule, bs) ▸ 入站请求│       │
  │                     ║                                                     │       │
  │     协议管线（对录制无感知）                                                │       │
  │                     ║                                                     │       │
  │     client 链：logging → advisorLoopback / vendor rt → ruleFlag            │       │
  │                     ║                                                     │       │
  │              wireRecordTransport ⟳ ───── t 已启用？                        │       │
  │                     ║                     └─ 追加 Exchange：▸ 出站请求      │       │
  │                     ║                        tee response body ▸ 出站返回   │       │
  │                     ║                        EOF / Close 时收尾             │       │
  │                  wire base                                                │       │
  │                                                                           │       │
  │     ◄─────────────────────────────────────────────────────────────────────┘       │
  │  t 已启用？ → 收尾（未完成的流标 incomplete）→ Emit                               │
  │  （成功 / 失败 / panic 恢复后都只走这一处）                                         │
  └───────────────────────────────────────────────────────────────────────────────────┘
```

### 4.1 出站：`wireRecordTransport` 挂在哪

```
  通用链（OpenAI / Anthropic / OpenCode / Google 非 OAuth）
     SDK ─► logging ─► advisorLoopback ─► ruleFlag ─► [wireRecordTransport] ─► pool *http.Transport

  vendor 链（Claude OAuth / Codex / Kimi / Gemini / Antigravity / xAI）
     SDK ─► logging ─► vendor rt（改 header / envelope）─► [wireRecordTransport] ─► SessionBoundTransport

                                              ▲
                                              └─ 所有改写之后、wire 之前：录到的就是真正发出去的请求
  未覆盖：Bedrock 等自带 SDK transport 的 client —— R1 逐个核对
```

## 5. 分阶段落地

```
  R1 接入 ─────────────► R2 收口 ─────────────► R3 查看 ─────────────► R4 治理
  Trace / Exchange        final_response          list / get API          保留期 · 磁盘配额
  ctx 传播                （Writer tee）          录制查看页               仅错误 / 采样
  wireRecordTransport     删除旧 recorder 接线     rule / scenario 处      cURL / HAR 导出
  recordingMiddleware     UI 放开两个响应点位      直接链到它的录制         脱敏规则可配置
  去重落盘格式（§3.2）                                                      advisor 关联
  ▸ client_request
  ▸ upstream_request          ▸ final_response
  ▸ upstream_response
  新旧并存 ═══════════════════╗
                              ╚═► 只剩新录制

  R3 读的是 R1 定下的落盘格式，可以在 R1 之后与 R2 并行
```
