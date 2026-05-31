# Vision Proxy —— 三项改进

> 接续 `.design/vision-proxy.md`(当前实现)。本文档描述本轮改进的
> 三项具体动作:**多图并发描述**、**描述结果缓存**、**描述前置注入
> 响应**。

---

## 1. 三项改进概览

| 改进 | 价值 | 影响面 |
|------|------|------|
| **多图并发 describe** | N 张图延迟从 Σ(Tᵢ) → max(Tᵢ) | processor 内部,不改对外语义 |
| **描述结果缓存** | 同图重复出现时跳过上游 | processor 内部 + 进程级 LRU |
| **描述前置注入响应** | 客户端实时可见描述,不靠下游模型复述 | 响应流路径,需在 4 协议 × 流/非流上接入 |

三项**互相独立、可分别合入**。建议按顺序:并发 → 缓存 → 注入(注入最重)。

---

## 2. 多图并发 describe

### 2.1 现状

`processBeta` / `processV1` / `processOpenAI` 都是顺序循环:每张图依次
调 `describe(...)`,每次 describe 是阻塞的上游 streaming 调用。N 张图
延迟 = Σ(Tᵢ)。历史消息里的图不调上游(直接打 marker),所以并发只需要
覆盖**当前(latest)消息**里的图。

### 2.2 实现

把"latest 消息中的图"收集成一组 `(blockIndex, src)`,用
`errgroup.Group` 或 `sync.WaitGroup` 并发跑 describe,收集结果后按
index 原地写回 block。

```go
// 仅描绘 latest 消息里的图(历史消息保持单线程 marker 替换不变)
type imgWork struct {
    bi               int
    mediaType, b64, remoteURL string
    text             string  // describe 写入
}

func (p *VisionProxyProcessor) describeBetaLatestConcurrently(
    ctx context.Context, blocks []anthropic.BetaContentBlockParamUnion,
    usable *loadbalance.Service,
) {
    var work []imgWork
    for bi := range blocks {
        if img := blocks[bi].OfImage; img != nil {
            mt, b64, u := extractBetaImageSource(img)
            work = append(work, imgWork{bi: bi, mediaType: mt, b64: b64, remoteURL: u})
        }
        // tool_result 嵌套 image 同样要进 work 列表(对 inner content 做平行处理)
    }
    if len(work) == 0 { return }

    var wg sync.WaitGroup
    wg.Add(len(work))
    for i := range work {
        i := i
        go func() {
            defer wg.Done()
            work[i].text = p.describe(ctx, usable, work[i].mediaType, work[i].b64, work[i].remoteURL)
        }()
    }
    wg.Wait()

    for _, w := range work {
        blocks[w.bi] = anthropic.BetaContentBlockParamUnion{
            OfText: &anthropic.BetaTextBlockParam{Text: w.text},
        }
    }
}
```

### 2.3 约束

- **并发上限**:`describe` 调上游 streaming,N 张图 = N 个并发上游
  连接。给一个上限(默认 8),超过时降为分批。一次请求带几十张图是异常
  场景,不必撑无限并发。
- **失败隔离**:任一张失败不影响其它;每张图的 `text` 字段独立。
- **ctx 取消**:任何一张 describe ctx 取消,其它 goroutine 通过共享
  ctx 自然中止——`describe` 已经接收 ctx,无需额外信号。

### 2.4 OpenAI 路径

同样改造 `processOpenAI` 的 latest 消息分支。tool_result 嵌套那部分
Anthropic 独有,OpenAI 不需要。

### 2.5 测试

`TestVisionProxy_MultipleImages_AllReplacedInOrder` 已用 fake 客户端
按顺序返回三段描述。把 fake 改成**每次 Describe 都 sleep N ms**,断言
3 张图总耗时 < 2.5N(并发证据,而非 ≥ 3N 的串行)。

---

## 3. 描述结果缓存

### 3.1 现状

零缓存。同一张图(同 base64 / 同 URL)在一次请求内 / 跨请求都重新调
上游。Claude Code 多轮对话很容易出现同一截图在 history 里多次出现的
情况(虽然历史消息走 marker 不调上游,但 latest 同图在多次请求里会
重复调)。

### 3.2 缓存键设计

- **base64 数据**:`sha256(mediaType + "|" + b64Data)` 的 hex(64 字符)
- **远程 URL**:URL 字符串直接做 key,前缀 `url:`

> 不直接用 `b64Data` 本身做 key——同图 base64 字符串可达数 MB,做 map
> 键既贵又慢。sha256 hash 算一次就够;比"反复调上游"便宜两个数量级。
>
> mediaType 加入键里防止"两张媒体类型不同但数据偶然相同"的极端碰撞
> (虽然几乎不会发生)。

### 3.3 存储

进程级 LRU,挂在 `VisionProxyProcessor` 上(它本身是单例,持有缓存
天经地义):

```go
type VisionProxyProcessor struct {
    Client   visionClient
    Resolver providerResolver
    cache    *lru.Cache[string, string]  // hashicorp/golang-lru/v2
}
```

容量与 TTL:
- 默认 **512 条**(平均一条描述几百字节,总占用 < 1 MB)。
- **无 TTL**——描述本身就是图片内容的不可变映射,没有"过期"概念。
  容量满靠 LRU 驱逐。配置上不暴露(常量,出问题再调)。

### 3.4 缓存接入点

在 `describe()` 里,**算 cache key → 查 cache → miss 才调上游 → 回写
cache**。失败结果(`imageUnavailableText` 那种 marker)**不写缓存**,
免得一次瞬时失败被永久记忆。

```go
func (p *VisionProxyProcessor) describe(ctx context.Context, usable *loadbalance.Service, mediaType, b64, remoteURL string) string {
    key := visionCacheKey(mediaType, b64, remoteURL)  // 空 key 表示不缓存
    if key != "" {
        if cached, ok := p.cache.Get(key); ok {
            return cached  // hit:无日志噪声,或打一行 DEBUG
        }
    }
    // ... 现有的 describe 上游调用与失败兜底 ...
    if key != "" {
        p.cache.Add(key, success)  // 只缓存成功描述
    }
    return success
}
```

### 3.5 并发安全

`hashicorp/golang-lru/v2` 本身线程安全(内部 sync.Mutex)。与 §2 的多图
并发结合时:多个 goroutine 同时 miss 同一 key 会**重复调上游**(thundering
herd),但同图同请求内大概率走 dedup 路径(每图一次 describe);跨请求
race 几率小到不值得加 singleflight。如果实测发现高频热图重复,再加
singleflight 包裹。**不预先优化。**

### 3.6 测试

- cache hit 测试:同一图调两次 describe,fake 客户端只被调一次
- 失败不缓存:fake 第一次返回错误,第二次返回成功描述 → 第二次必须
  调到上游(不是返回错误的缓存)
- key 区分:不同 mediaType 同 b64 不串台

---

## 4. 描述前置注入响应

### 4.1 关键洞察(你刚说的)

不需要在 SSE 流里注入独立 chunk,**只要在第一个真正带 content 的
响应 chunk 抵达时,把图片描述前置进它的 content delta** 就够了。
不增加新 chunk、不破坏协议结构。

### 4.2 端到端形态

```
客户端请求 (带图)
   │
   ▼
applyVisionProxy → processor 调 vision 上游,描述出 desc₁, desc₂, ...
   │                  │
   │                  └─ 把 descriptions 列表 stash 到 gin.Context
   │
   ▼ (请求里的 image block 被替换成 text,转发给下游)
下游模型开始回流响应
   │
   ▼ 第一个 content-bearing chunk 抵达 stream hook
   │        │
   │        └─ 从 gin.Context 取 descriptions
   │           将 "[Vision: <desc₁>; <desc₂>]\n\n" 前置进 chunk 的 content
   │           标记已注入,后续 chunk 不再触碰
   │
   ▼
客户端看到的第一段输出:[Vision: ...]\n\n<模型真正的回答>
```

### 4.3 三个落点

#### (a) Processor:收集 descriptions

`describe()` / `walkBetaContent` 等改造为**返回**每张图的描述(不止
原地改写 block)。汇总进 `[]string`,通过
`gin.Context.Set(visionDescriptionsKey, descs)` 暂存。

> 与 §2 并发改造合并实现:并发 describe 收集完结果,既写回 block,也
> 同步附加到这条列表。

#### (b) Handler:把列表挂到 gin.Context

`applyVisionProxy` 调完 `Process()` 后,检查 processor 返回值(或从
ProcessorContext 里取——ProcessorContext 需要补一个字段或回调),把
descriptions 列表 `c.Set(visionDescriptionsKey, descs)`。

`visionDescriptionsKey` 常量定义在 `internal/server/vision_proxy.go`。

#### (c) Response stream hook:前置注入

四种协议 × 流式/非流式,共 8 个注入点。但**前置注入逻辑可抽出一个
通用函数**:

```go
// prependVisionDescriptions returns text with "[Vision: ...]\n\n" prefix
// if c has stashed descriptions and they have not yet been injected.
// Returns the original text otherwise. Sets visionDescriptionsInjected=true
// on first successful prepend so subsequent calls are no-ops.
func prependVisionDescriptions(c *gin.Context, text string) string { ... }
```

8 个接入点:

| 协议 | 流式 | 非流式 |
|------|------|------|
| OpenAI Chat | `chunk.Choices[0].Delta.Content` 第一个非空时前置 | `responseMap["choices"][0]["message"]["content"]` 前置 |
| OpenAI Responses | output_text delta 第一个非空时前置 | message output 的 first text item 前置 |
| Anthropic V1 | `content_block_delta` 第一个 `text` delta 前置 | `response.Content[0]` 若是 text 则前置,否则插入一个 text block |
| Anthropic Beta | 同上(beta 结构) | 同上 |

复杂度集中在"找到第一个 text-bearing 位置"这一点。各协议各写一个小
helper 即可,**不引入新框架**。

### 4.4 前置文本格式

```
[Vision: a red apple on a white plate; a screenshot of a terminal]

```

- 用方括号 + `Vision:` 前缀,提示这不是模型输出
- 多张图用 `; ` 分隔
- 后接 `\n\n` 与模型真正回答隔开
- 仅一张图时不带分号:`[Vision: a red apple on a white plate]\n\n`

历史消息里的图(只打 marker 的)**不**进这个列表——它们对当前轮
对话不是新信息,前置出来反而吵。

### 4.5 异常路径

- **下游响应中断 / 客户端断开**(在第一个 content chunk 之前):描述
  没机会注入。这是"实时可见"的固有局限。可在 c.Done 时打一条日志,
  方便排查。
- **下游回应没有任何 text content**(纯 tool_use):前置无处可挂。
  fallback:在 tool_use 之前**额外发一个 text block**(Anthropic 协议
  允许多 content block;OpenAI 协议中 tool_calls 与 content 共存)。
  此路径少见,可作为后续 follow-up。本 PR 第一版**接受这种情况下不
  注入**(描述仍在请求里给下游看了,只是客户端没收到独立的"[Vision:]"
  前缀)。

### 4.6 与 vision proxy "off" 的关系

如果 scenario / rule 都没配 vision_proxy,根本没走过 processor →
gin.Context 里没有 descriptions → hook 走 no-op 路径返回原文。**对
未启用 vision proxy 的请求零开销**。

### 4.7 测试

- 单测 `prependVisionDescriptions`:无 descs 原样返回;有 descs 前置
  并设置 injected;再调一次返回原样(不重复前置)。
- 集成测试:用 stub stream,验证第一个 chunk 被前置、第二个 chunk
  不再注入。
- 协议覆盖:Anthropic Beta / V1 / OpenAI Chat 流式各一个端到端用例。

---

## 5. 后端改动清单

| 文件 | 改动 |
|------|------|
| `internal/server/processor/vision_proxy.go` | `processBeta`/`processV1`/`processOpenAI` 改成并发 describe;`describe` 加 cache 查询/写入 |
| `internal/server/processor/vision_proxy.go` | `VisionProxyProcessor` 加 `cache *lru.Cache[string, string]` 字段;构造函数初始化 |
| `internal/server/processor/processor.go` | `RegisterAll` 构造时传入 cache 容量(常量 `visionCacheCapacity = 512`) |
| `internal/smart_routing/processor.go` | `ProcessorContext` 加 `Descriptions []string` 字段,供 processor 写入、helper 读取 |
| `internal/server/vision_proxy.go` | `applyVisionProxy` Process 完成后,把 `pctx.Descriptions` 经 `c.Set(visionDescriptionsKey, descs)` 暂存到 gin.Context |
| `internal/server/vision_proxy.go` | 新增 `prependVisionDescriptions(c, text)` 通用 helper 与 `visionDescriptionsKey`、`visionDescriptionsInjectedKey` 常量 |
| `internal/protocol/stream/*.go` 等响应处理点 | 在各协议第一个 text-bearing chunk 处调用 `prependVisionDescriptions` |
| `internal/server/openai_chat.go` 等非流响应处理 | 在 `responseMap` / `response.Content` 第一个 text 位置调用 helper |

依赖:
- `github.com/hashicorp/golang-lru/v2`(已在 go.mod 里?需确认,没有则
  `go get` 引入)

---

## 6. 测试矩阵补充

在 `vision-proxy.md` §9 的基础上加:

| 用例 | 期望 |
|----|------|
| 多图并发 | 3 张图各 sleep N ms,总耗时 < 2.5N |
| 缓存 hit | 同图调两次,fake 客户端只被调一次 |
| 失败不缓存 | 第一次错误、第二次成功 → 第二次必须调到上游 |
| key 区分 mediaType | image/png 与 image/jpeg 同 b64 → 不串台 |
| `prependVisionDescriptions` 单测 | 无 descs/有 descs/重复调用 三态正确 |
| 注入到 Anthropic Beta 流第一个 text delta | 客户端第一段输出含 `[Vision: ...]\n\n` |
| 注入到 OpenAI Chat 流第一个 content delta | 同上 |
| Vision proxy 关闭时零开销 | hook 无 descs 时原样返回,无注入 |

---

## 7. 不在本 PR 范围

- 视觉描述计入 usage 统计(单独 follow-up,需要 token tracker 改造)
- 支持多个 vision service 备选(改数据模型,改 UI,单独 follow-up)
- 描述失败重试 + 超时控制(单独 follow-up,需要明确重试策略)
- 流式输出"逐 token" 的 vision 描述前置(当前是阻塞拿全文再前置)
