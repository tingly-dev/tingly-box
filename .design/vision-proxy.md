---
updated: 2026-09-02
---

# Vision Proxy

> 适用对象：tingly-box 后端 / 前端贡献者。
> 描述当前 vision proxy 的设计与实现。两个早期分文档
> `vision-proxy-scenario.md` / `vision-proxy-rule.md` 已合并到本文件。

---

## 1. 它做什么

请求里带图、下游模型只认文本时——找一个**有视觉能力的模型**先把图描述
成一段文字，把图块替换成那段文字，再放给下游。下游纯文本模型也能"看懂"
图片。处理器原地改写请求,失败兜底也会把图剥成 marker 文字,绝不让不支
持的 content block 漏到下游。

---

## 2. 两个作用域，同一种效果

可以在两个不同的作用域配置：

| 作用域 | 配置位置 | 谁来用 |
|------|------|------|
| **scenario 级** | `ScenarioConfig.Extensions["vision_proxy_service"]` | "这个场景的所有 rule 都用同一个视觉模型" |
| **rule 级** | `Rule.Flags.VisionProxyService` | "这条 rule 单独用不同视觉模型（或单独关掉）" |

两者**效果完全等价**（同一个处理器、同样的图→文替换），区别只在
**作用域大小**——不是覆盖、不是 fallback、不是叠加。

### 配置矩阵

| rule 设了 | scenario 设了 | 实际用谁 |
|---|---|---|
| ✓ | ✓ | **rule** |
| ✓ | ✗ | rule |
| ✗ | ✓ | scenario |
| ✗ | ✗ | 不启用 |

> **rule 优先于 scenario** —— 更具体的作用域被视为用户意图。两者都配
> 时**不**重复描述,Process 只跑一次,用 rule 的 service。

### 服务形态是 `{provider, model}` 二元

视觉服务必须是一个**完整的 service（provider + model）**,不是只选
provider。这是系统里 service 的统一建模,前端的选择器
（`ModelSelectDialog`）也必须产出两元,不允许退化。

> **为什么 scenario 级不放 ScenarioFlags？** ScenarioFlags 是扁平的
> bool/string/enum,装不下 `{provider, model}` 这种结构;Extensions 就
> 是为这类配置预留的位置。
>
> **为什么没有独立的 on/off 标志？** "是否启用" ≡ "有没有配视觉
> service"。配了就是开,清空就是关。单一事实源、无飘移。前端 UI 也据
> 此把"开关"和"选模型"合并为一个控件,见 §5。

---

## 3. 数据模型

### 3.1 Scenario 级 —— Extensions 存储

```jsonc
// ScenarioConfig.Extensions
{
  "vision_proxy_service": {
    "provider": "<provider-uuid>",
    "model": "claude-3-5-sonnet-latest"
  }
}
```

约定 key:`internal/constant/flag.go` 的 `ExtensionVisionProxyService`
(`"vision_proxy_service"`)。

### 3.2 Rule 级 —— RuleFlags typed 字段

```go
// internal/typ/type.go
type RuleFlags struct {
    // ... 其它 flag ...
    VisionProxyService *VisionProxyService `json:"vision_proxy_service,omitempty" yaml:"vision_proxy_service,omitempty"`
}

type VisionProxyService struct {
    Provider string `json:"provider" yaml:"provider"`
    Model    string `json:"model"    yaml:"model"`
}
```

`*` 指针 + `omitempty`:未配置 → JSON 不出现 → "rule 级未启用",与
scenario 级"Extensions 里没有该 key"的"未启用"语义对称。

### 3.3 Flag registry 的新类型

`internal/typ/flag_registry.go` 加了一个专名类型:

```go
const (
    FlagTypeBool       FlagValueType = "bool"
    FlagTypeString     FlagValueType = "string"
    FlagTypeEnum       FlagValueType = "enum"
    FlagTypeInt        FlagValueType = "int"
    FlagTypeServiceRef FlagValueType = "service_ref"  // {provider, model} 二元
)
```

不引入泛化的 `object` 类型——只为 vision proxy 这一种结构开**专名**。
泛化 object 需要 schema 描述,FlagSpec 会立刻变重;现在系统里也只有这
一个结构对象 flag 的需求。将来若真有别的(如 prompt_template_ref),
照样起新名。

---

## 4. 执行流程

### 4.1 单一入口

不论 rule 级还是 scenario 级,都从同一个 helper 进(`internal/server`
被拆分成 `internal/protocolserver` 之后,helper 挂在
`ProtocolHandler` 上;`internal/server/server.go` 上还留着一份同名
的旧方法,已无调用方,是拆分后的死代码):

```go
// internal/protocolserver/protocol_handler.go
func (ph *ProtocolHandler) applyVisionProxy(c *gin.Context, scenarioType typ.RuleScenario, rule *typ.Rule, typedRequest any) {
    if ph.deps.VisionProxyService == nil {
        return
    }
    sessionID := resolveSessionID(c, typedRequest) // 见 §10.3
    ph.deps.VisionProxyService.Apply(c.Request.Context(), ph.deps.Config, scenarioType, rule, typedRequest, sessionID)
}
```

`ph.deps.VisionProxyService` 是 `*visionproxy.Service`
(`internal/vision/visionproxy/service.go`),在 `server.go` 启动时
构造一次(`visionproxy.NewServiceFromPool(pool, resolver)`),经
`ProtocolHandlerDeps` 注入。`Service.Apply` 内部做两件事:

```go
func (s *Service) Apply(ctx context.Context, cfg *config.Config, scenarioType typ.RuleScenario, rule *typ.Rule, typedRequest any, sessionID typ.SessionID) {
    svc := s.Resolve(cfg, scenarioType, rule) // rule 先,scenario 后
    if svc == nil { return }
    _ = s.Processor.Process(ctx, typedRequest, []*loadbalance.Service{svc}, sessionID)
}
```

优先级集中在 `Service.Resolve` 一个纯函数里,可单测、可读。**只
Process 一次**——既不需要"两个 helper 串联 + lock 互斥",也不存在
"图描述两次"的窗口。

### 4.2 钩子位置

每个入站 handler(`internal/protocolserver/openai_chat.go` /
`openai_responses.go` / `anthropic_message.go` 统管 v1 + beta)在
`determineRuleWithScenario` 之后、`selectService` 之前调用:

```go
rule, err = ph.determineRuleWithScenario(c, scenarioType, requestModel)
// ...
ph.applyVisionProxy(c, scenarioType, rule, reqParams)
provider, selectedService, err = ph.selectService(c, scenarioType, rule, reqParams)
```

放在 `selectService` 之前是为了让下游接到的就是已经"图→文"完成的请求。

### 4.3 处理器细节

`VisionProxyProcessor`(`internal/vision/visionproxy/vision_proxy.go`)
被 `Service.Processor` 持有,`applyVisionProxy` 经 `Service.Apply` 间
接调用——vision proxy **不走** smart routing 的处理器注册表
(`internal/routing/smartrouting/processor.go` 的
`RegisterProcessor`/`LookupProcessor`;该注册表本身还在给其它 op 用,
只是 `proxy_vision` 这一项已经从里面删掉了,见 §7)。

处理器原地改写请求里的 image block:
- 命中描述缓存的 image(不分最新/历史)→ 直接换成缓存里的真实描述,
  不调 vision(§10)
- 未命中缓存 + 最新一条消息里的 image → 调上游 vision 模型描述,成功
  则写入缓存
- 未命中缓存 + 历史消息里的 image → 打 `imageHistoricalText` marker
  (**不调** vision)
- 失败兜底(无可用 service / 上游报错 / 空响应)→ 打
  `imageUnavailableText` marker,**不写入缓存**

支持四种请求形态:`*anthropic.BetaMessageNewParams` /
`*anthropic.MessageNewParams` / `*openai.ChatCompletionNewParams` /
`*responses.ResponseNewParams`(OpenAI Responses API,后加)。
Anthropic 两种形态还会**下钻 `OfToolResult.Content`** 处理工具返回里
的图;OpenAI 形态后来也补上了 tool message 里的图(均见 §6.1)。

---

## 5. UI

两个作用域两个落点,但**外观、交互完全一致**——降低学习成本。

### 5.1 Scenario 级:场景 plugin 行

落点:`frontend/src/components/PluginFeatures.tsx`,由
`ProviderConfigCard` 在各 `Use*Page` 场景页面渲染。

不在通用 `PLUGIN_FEATURES` 的 On/Off 列表里(那会产生"开关 + 独立
模型按钮"的割裂);用专用 `renderVisionProxyButton`:

| 状态 | 按钮形态 |
|------|------|
| 未配 | `Vision Proxy: Off`(灰) |
| 已配 | `Vision Proxy: <model>`(蓝高亮,tooltip 显示完整 `provider / model`) |

点击先弹**下拉**(与 Thinking / Record 等同范式):
- `Off` —— 直接清空 service = 关闭(无需打开大弹窗)
- `On — <model>` —— 进 `ModelSelectDialog` 选/改模型(**选模型即启用**)

持久化:`getScenarioConfig` / `setScenarioConfig` 读写
`Extensions["vision_proxy_service"]`,**不调任何 flag 端点**。

### 5.2 Rule 级:Rule extensions catalog

落点:`frontend/src/components/rule-card/FlagCatalogDialog.tsx`(rule
编辑卡的 Extensions 弹窗)。flag registry 里 `vision_proxy_service` 项
type=`service_ref`,catalog 自动按这个类型分支渲染——一个按钮显示当前
所选 `<provider> / <model>`(未配显示 `Select vision model…`),点击
弹同款 `ModelSelectDialog`。

`RuleCard` 把 `providers` 透传给 `FlagCatalogDialog`,后者只在 picker
打开时使用。

### 5.3 类型层(camel↔snake)

前端 `RuleFlags` 有两份对应类型:

```ts
// 内部 camelCase
interface RuleFlags {
    // ...
    visionProxyService?: { provider: string; model: string };
}
// API snake_case (wire)
interface RuleFlagsApi {
    // ...
    vision_proxy_service?: { provider: string; model: string };
}
```

转换发生在两处:`rule-card/utils.ts` (load) 和
`rule-card/useRuleCardHooks.ts` (save)。两端都已带上
`vision_proxy_service`,加新 flag 时记得同步更新这两处。

---

## 6. 实现中踩过的几个坑(决策来源)

不是改动清单,而是**为什么这么写**的注解,避免后来人不读 commit
history 就推翻这些选择。

### 6.1 `tool_result` 内嵌的 image 必须下钻处理

`processBeta` / `processV1` 早期版本只看顶层 content block 的
`OfImage`。Claude Code 大量场景的图片其实来自工具返回(screenshot /
read-image / 许多 MCP 视觉工具),落在
`OfToolResult.Content[i].OfImage` 这一层。顶层遍历完全看不到这些 image,
于是「钩子触发了、配置取到了、处理器跑了,图却一张没换」——表面看就是
「没生效」。

修复方式:每条消息的 content 交给 walker,先看顶层 `OfImage`,再
**下钻 `OfToolResult.Content`**。两条路径共用 latest-vs-historical
策略。写下这条时 OpenAI 协议的 tool role message 内容还是纯字符串、
不含 image,OpenAI 路径当时确实不需要此处理——但后来 fork 把 tool
message 的 content 从纯文本放宽成了完整的 part union(#1609),tool
message 里也能带图了。不补上这条通道,图片会原样透传给下游,文本
only 的 provider 直接报错拒绝(z.ai code 1210)而不是被描述掉。现在
`collectOpenAI`(`internal/vision/visionproxy/vision_proxy.go`)会同
时看 `OfUser` 和 `OfTool` 两种消息的 content parts。

### 6.2 partial `ScenarioConfig` 写入会清空 `Extensions`

后端 `SetScenarioConfig` 是**整体替换** `c.Scenarios[i] = config`。前端
任何地方如果 POST `{scenario, flags}` 而没带 `extensions`,会把已配的
`vision_proxy_service` 一并抹掉,表现是「配过又没了」。

**约定**:所有 `setScenarioConfig` 调用前必须先 GET-merge:

```ts
const current = (await api.getScenarioConfig(SCENARIO))?.data || {};
const config = { ...current, scenario: SCENARIO, flags: { ...current.flags, ... } };
await api.setScenarioConfig(SCENARIO, config);
```

已修过 `UseClaudeCodePage.confirmModeChange`。其它场景页面的同类
模式切换 / 写配置代码若有 partial 写入,需要同样处理;或在后端 handler
里改成 partial merge(当前未做)。

### 6.3 日志写 `source` 字段会破坏聚合

`internal/obs/multi_logger.go` 的 `WriteEntry` 路由策略:

1. 若 entry 有显式 `source` → 用该 source(**跳过** request_id 自动注入)
2. 否则若 ctx 有 request_id → 路由到 `model_request`,自动注入
   `request_id` 字段

vision proxy 早期版本带了 `source=vision_proxy`,走分支 1,于是 ctx 里
明明有 request_id,日志却拿不到关联键;同时 `MemorySinkConfig` 也没注册
`vision_proxy` 这个 source,前端日志页那一栏根本看不到。

**修复**:不要覆盖 `source`。`logrus.WithContext(ctx)` 把 ctx 传下去,
让框架走分支 2 自动注入 request_id;身份标记改用普通字段
`component=vision_proxy`,不参与 source 路由。

> 一般原则:业务子系统的日志**不应该**自己设置 `source`。除非确实需要
> 路由到独立的 sink(并同时在 `MemorySinkConfig` 里注册),保留缺省路由
> 是稳妥做法。

### 6.4 ctx 的传递

`applyVisionProxy` 从 `c.Request.Context()` 取 ctx,一路原样传下去:
`Service.Apply(ctx, ...)` → `Processor.Process(ctx, ...)` →
`describeAll(ctx, ...)` → `describe(ctx, ...)`,最终到
`logrus.WithContext(ctx)`。这条链路无任何 `context.Background()` 截断,
所以中间件早期注入的 `request_id`(见
`../internal/middleware/memory_log.go`)自然贯穿。

如果未来要拆协程 / 异步执行 describe,**务必显式 propagate ctx**,否则
日志会脱离同请求聚合。

### 6.5 早期"两个 helper + lock"被合并

PR #1082 落地时只有 scenario 级,helper 叫
`applyScenarioVisionProxy`,辅以 `vision_proxy_applied` 锁防止与彼时
仍存在的 smart routing 路径互踩。引入 rule 级时一度想做"两个并列
helper + 共用锁",后来归一为本文 §4 的单入口:优先级在
`resolveVisionService` 里显式表达,Process 只跑一次,**不再需要这把
锁**。两条路径行为本就等价,合并不丢能力。

### 6.6 早期 `ScenarioFlags.VisionProxy bool` 被移除

PR #1082 第一版用 bool flag 表达"启用",后来发现"启用 ≡ 有 service"是
更简的事实源,bool flag 删掉。同样的设计原则套到 rule 级:`*VisionProxyService`
为 nil 即未启用,没有平行 bool。

---

## 7. 历史:smart routing `proxy_vision` op 的清退

在 scenario / rule 路径之前,vision proxy **只能**通过 smart routing
的 `proxy_vision` op 配置:

```
Rule → 启用 Smart Routing → 新建 Smart Rule → 加 proxy_vision op → 再配视觉 services
```

4 层嵌套,而且 op 自带语义错位——`Services` 字段在普通 op 里意味着
"下游候选",在 `proxy_vision` op 里意味着"上游视觉描述器"(同字段反义)。

### 为什么彻底删

`proxy_vision` op 本身不携带条件维度(其匹配条件就是隐式的 `HasImage`),
独立看与 scenario 级开关**完全等价**。它唯一多出的能力是和同 smart
rule 内其他 op AND 组合形成"带条件的 vision proxy",但实际业务里几乎
找不到真用例。scenario + rule 两个作用域覆盖之后,它彻底冗余。

### 已删除的位置

> 包名后来从 `internal/routing/smart_routing` 改成
> `internal/routing/smartrouting`(去掉下划线),下面路径已按现状更新;
> 删除的是 `proxy_vision` 专属的常量/case/switch 分支,**不是整个
> registry**——`RegisterProcessor`/`LookupProcessor` 机制本身还在给其它
> op(如 quota、time range)用。

后端:
- `internal/routing/smartrouting/op.go` —— `PositionProxyVision` / `OpProxyVisionEnabled` 常量,Operations 列表项
- `internal/routing/smartrouting/type.go` —— `IsValid` 里的 `PositionProxyVision` case
- `internal/routing/smartrouting/routing.go` —— `evaluateProxyVisionOp` 及其 switch case
- `internal/server/processor/processor.go` —— `RegisterAll` 不再 `smartrouting.RegisterProcessor(...)`。这个文件本身后来也没了(`internal/server/processor/` 整个目录已删除,vision proxy 的构造/持有迁到了 `internal/vision/visionproxy` + `Server.visionProxyService` / `ProtocolHandlerDeps.VisionProxyService`,见 §4.1、§8)

前端:
- `frontend/src/components/rule-card/SmartRuleCatalogDialog.tsx` —— catalog 注释残留 + `OPERATION_OPTIONS.proxy_vision`
- `frontend/src/components/RoutingGraphTypes.ts` —— `SmartOp.position` 字符串 union 去掉 `proxy_vision`

### 迁移

线上配置可能仍带 `smart_routing: [{ops:[{position:"proxy_vision"}]}]`:

- JSON 反序列化时 `SmartOpPosition` 是字符串别名,能保留字面值;
- 路由 evaluator 缺这个 case → 走 `default` 分支返回 unmatched;
- 结果:**老配置加载成功、该 op 永不匹配、整条 smart rule 不命中**,等
  同于失效。无报错,但功能没了。

> **release note 必须告知**:从 smart routing 的 proxy_vision 迁到 rule
> flag(`Rule.Flags.VisionProxyService`)或 scenario plugin
> (`PluginFeatures` 的 Vision Proxy 控件)。不写自动迁移代码:场景边界
> 太多,自动迁移容易把 services 池错位。

---

## 8. 关键文件索引

| 功能 | 文件 |
|------|------|
| 处理器实现(图描述、改写) | `internal/vision/visionproxy/vision_proxy.go` |
| Service 封装(`Resolve` + `Apply`) | `internal/vision/visionproxy/service.go` |
| **描述缓存**(定容 LRU,§10) | `internal/vision/visionproxy/describe_cache.go` |
| smart routing 处理器接口 / `ProcessorContext`(vision proxy 已不用,给其它 op 用) | `internal/routing/smartrouting/processor.go` |
| **统一入口 helper**(`applyVisionProxy`) | `internal/protocolserver/protocol_handler.go`(`internal/server/server.go` 上还留一份同名死代码,见 §4.1) |
| 构造 + 注入(`NewServiceFromPool`) | `internal/server/server.go`(构造)→ `ProtocolHandlerDeps.VisionProxyService`(注入) |
| `RuleFlags` + `VisionProxyService` | `internal/typ/type.go` |
| Flag registry + `FlagTypeServiceRef` 常量 | `internal/typ/flag_registry.go` |
| `ScenarioFlags` / `ScenarioConfig` | `internal/typ/type.go` |
| 场景配置 Get/Set | `internal/server/config/scenario.go` |
| 场景配置 API | `internal/server/module/scenario/{routes,handler,types}.go` |
| `ExtensionVisionProxyService` 常量(Extensions key) | `internal/constant/flag.go` |
| 入站 handler(钩子点) | `internal/protocolserver/{openai_chat,openai_responses,anthropic_message}.go` |
| Scenario 级 UI | `frontend/src/components/PluginFeatures.tsx` |
| Rule 级 UI | `frontend/src/components/rule-card/FlagCatalogDialog.tsx` |
| `RuleFlags` ↔ wire 转换 | `frontend/src/components/rule-card/{utils.ts,useRuleCardHooks.ts}` |
| 类型定义 | `frontend/src/components/RoutingGraphTypes.ts` |
| 服务选择器对话框(复用) | `frontend/src/components/ModelSelectDialog.tsx` |
| 实现细节补充(处理流程时序图、协议覆盖表) | `internal/vision/visionproxy/README.md` |

---

## 9. 测试

| 层 | 用例 |
|----|------|
| `Service.Resolve` 优先级 | rule + scenario 都配 → rule;只 scenario → scenario;只 rule → rule;都不配 → nil;rule 配但 model 空 → 回退 scenario;nil rule + scenario → scenario |
| `Service.Apply` 行为 | rule 配 + 有图 → 用 rule service 描述;scenario 配 + 有图 → 用 scenario service;都没配 + 有图 → 图保留(no-op);profile 场景(`claude_code:p1`)配的 service 能找到(独立于 base) |
| 单次 Process 不变量 | 两者都配时 Process 也只调一次(用 rule 的 service) |
| `parseScenarioVisionService` | nil/缺键/结构错/缺 provider/缺 model/空串 → nil;provider+model 齐备 → active service |
| 处理器四种请求形态 | Beta / V1 Anthropic、OpenAI ChatCompletion、OpenAI Responses 各覆盖 |
| **tool_result 嵌套 image** | Beta + V1 各一例:tool_result 内的 image 最后一条消息会描述、历史消息只打 marker(不调 vision);OpenAI tool message 内嵌图片同样覆盖 |
| smart routing 残留 | `LookupProcessor(PositionProxyVision, OpProxyVisionEnabled)` 不再可达;catalog 新建 smart rule 时无 `proxy_vision` 选项;老配置带该 op → unmatched,不报错 |
| Flag registry 暴露 | `GET /rule/flags/registry` 返回的 `vision_proxy_service` 项 type=`service_ref` |
| 类型反序列化 | `Rule.Flags.VisionProxyService` 从 JSON 圆环(marshal → unmarshal)保持一致 |
| **描述缓存**(§10) | LRU 淘汰最久未用;`get` 命中前移;`put` 更新已存在 key 不增条目;不同 service / 不同 session 同图片内容不互相命中;base64 与 URL 两种 key 不冲突;同 session 同图第二次命中不再调 vision;不同 session 各自独立调用;历史图片命中缓存后拿到真实描述而非固定 marker;失败描述不写入缓存(下次仍重试);换模型不复用旧模型的描述 |

---

## 10. 图片描述缓存

### 10.1 要解决的问题

§4.3 的"最新消息实时描述、历史消息打固定 marker"这个策略,在两种常见
场景下会浪费成本、甚至反过来伤害下游:

- **重复请求同一张图**(重试 / failover / 多轮工具调用反复截同一张
  图)——每次都重新调用一次视觉模型,白白花 token 和延迟。
- **视觉模型输出非确定**——同一张图两次描述的文本大概率不同,拼进请求
  后会打断下游 provider 的 prompt 前缀缓存命中。

### 10.2 方案:按 `(session, service, image)` 寻址的进程内 LRU

新增 `internal/vision/visionproxy/describe_cache.go`:一个定容量、
LRU 淘汰的内存缓存,key 是:

```go
type visionCacheKey struct {
    session  string // typ.SessionID.String()
    provider string // loadbalance.Service.Provider —— provider UUID,不是名字
    model    string
    content  string // "b64:"+sha256(mediaType+base64) 或 "url:"+remoteURL
}
```

value 只存**拼好的替换文本**,不存图片字节——内存占用只随条目数和描述
文本长度增长,与图片大小无关。默认容量 `defaultDescribeCacheCapacity`
(2000),超出后淘汰最久未用的条目;不加 TTL。

**为什么 key 里有 session,而不是纯按图片内容全局寻址**:讨论中发现
"同一段字节"不等于"同一次提问该复用同一个答案"——视觉模型这次描述得不
好,用户重发同一张图本来还有机会拿到更好的答案,纯全局缓存会把这个坏
描述钉死到被 LRU 淘汰为止;两个不相关会话恰好发了内容相同的图片
(占位图、测试图)也不该互相复用描述。把 `typ.SessionID`(复用
`routing.ResolveSessionID` 已有的 `metadata.user_id` > `X-Tingly-Session-ID`
header > client IP 兜底)纳入 key 后,缓存回答的问题变成"这是**这个会
话**里已经看过的图吗",而不是"这段字节全局出现过吗"——会话有自然的
生命周期,新会话就是全新的图,因此不需要再叠加 TTL。

**为什么 key 里还要有 provider+model**:vision service 在同一 session
内也可能被用户中途切换(rule/scenario 改了配置)。不带这两个维度,切
模型后同一张图会静默复用旧模型的描述,而且这个"没生效"完全无感知。带
上之后,换模型 = 自动、免费地让相关缓存失效,不需要额外监听配置变更。

### 10.3 对 §4.3 处理流程的改动

统一了"最新 / 历史"两条路径的入口决策(`spliceOrCollect`):

1. **任何位置的图片先查缓存**——命中就直接替换成真实描述,不再区分
   最新/历史。这是相对 §4.3 原描述的行为升级:历史图片如果在同一
   session 内命中过缓存(比如上一轮它是最新消息、被真实描述过),这一
   轮会拿到真实描述,而不是固定的 `imageHistoricalText` marker。
2. **未命中 + 历史消息** → 行为不变,仍退回固定 marker,不为了填缓存
   而额外调视觉模型(成本控制不变)。
3. **未命中 + 最新消息** → 照旧调用视觉模型描述,**成功后写入缓存**
   (失败 / fail-strip 结果绝不写入缓存,避免把临时故障永久记成"这张
   图没法描述")。

`VisionProxyProcessor.Process` 因此多了一个 `sessionID typ.SessionID`
入参;调用方(`applyVisionProxy`,§4.2 提到的钩子位置)在调用前用
`resolveSessionID(c, typedRequest)` 提前独立求一次这个值——它是纯函
数,不依赖入站 handler 里"session 注入 context"那一步的先后顺序,所以
不需要挪动任何现有代码。

### 10.4 已知局限

session 在没有 `metadata.user_id` / `X-Tingly-Session-ID` header 时兜
底用 client IP——同一 NAT 后的不同用户会被分到同一个"session 桶"。这
是 `typ.SessionID` / `routing.ResolveSessionID` 本身既有的权衡(LB
affinity 已经在承担同样的代价),缓存层如实继承,不重新设计。
