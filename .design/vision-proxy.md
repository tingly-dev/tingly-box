---
updated: 2026-09-10
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

处理器原地改写请求里的 image block,**不再判定"最新一条消息"是哪条**
(§10.3 记录了为什么去掉):
- 命中描述缓存的 image(任何位置)→ 直接换成缓存里的真实描述,不调
  vision(§10)
- 未命中缓存的 image → 按位置**从新到旧**排队,前 N 张调上游 vision 模
  型描述,成功则写入缓存;N 是每请求描述上限(`defaultDescribeLimit`
  = 8,可用 `TINGLY_VISION_DESCRIBE_LIMIT` 覆盖)
- 超出上限的 image → 打 `imageOverLimitText` marker(**不调** vision)。
  这是"本轮暂缓",不是"永远省略":下一轮更新的图已进缓存、不占名额,
  名额就轮到它
- 失败兜底(无可用 service / 上游报错 / 空响应)→ 打
  `imageUnavailableText` marker,**不写入缓存**。marker 文案是**显式的
  错误报告**(说明是 proxy 侧故障、图片被网关移除,并提示模型告知用户),
  不是中性占位符——让下游模型和最终用户都能把问题归因到代理,而不是
  以为图片本身或客户端有问题

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
| **描述缓存**(SQLite 单层 + 内存负缓存,§10) | `internal/vision/visionproxy/describe_cache.go` / `describe_store.go` |
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
| **tool_result 嵌套 image** | Beta + V1 各一例:tool_result 内的 image 无论在哪条消息都按缓存未命中描述;OpenAI tool message 内嵌图片同样覆盖 |
| **每请求描述上限**(§10.3) | `boundNewestFirst` 逆序 + 裁剪(保留项为最新优先、超出项为更旧);上限 1 时只描述最新未命中、更旧的打 marker(Beta / V1 / OpenAI / Responses 各一例,含 Claude Code 尾部 system 消息形态);三张未命中、上限 1 → 三轮收敛到全部缓存,第四轮零调用且文本字节一致;缓存命中不占名额;env 解析(空/非法/显式值/processor 值优先) |
| smart routing 残留 | `LookupProcessor(PositionProxyVision, OpProxyVisionEnabled)` 不再可达;catalog 新建 smart rule 时无 `proxy_vision` 选项;老配置带该 op → unmatched,不报错 |
| Flag registry 暴露 | `GET /rule/flags/registry` 返回的 `vision_proxy_service` 项 type=`service_ref` |
| 类型反序列化 | `Rule.Flags.VisionProxyService` 从 JSON 圆环(marshal → unmarshal)保持一致 |
| **描述缓存**(§10) | `put` 覆盖已存在 key;不同 service / 不同 session 同图片内容不互相命中;base64 与 URL 两种 key 不冲突;同 session 同图第二次命中不再调 vision;不同 session 各自独立调用;历史图片命中缓存后拿到真实描述而非固定 marker;失败描述不写入正缓存,TTL 内不重试不占名额、TTL 后重试并缓存恢复结果;上限 1 且最新图永远失败时,第二轮名额落到更旧的图;换模型不复用旧模型的描述 |

---

## 10. 图片描述缓存

### 10.1 要解决的问题

§4.3 的"最新消息实时描述、历史消息打固定 marker"这个策略,在两种常见
场景下会浪费成本、甚至反过来伤害下游:

- **重复请求同一张图**(重试 / failover / 多轮工具调用反复截同一张
  图)——每次都重新调用一次视觉模型,白白花 token 和延迟。
- **视觉模型输出非确定**——同一张图两次描述的文本大概率不同,拼进请求
  后会打断下游 provider 的 prompt 前缀缓存命中。

### 10.2 方案:按 `(session, service, image)` 寻址的 SQLite 缓存

`internal/vision/visionproxy/describe_cache.go` 的 key 是:

```go
type visionCacheKey struct {
    session  string // sessionScope(typ.SessionID) = "<source>:<value>"
    provider string // loadbalance.Service.Provider —— provider UUID,不是名字
    model    string
    content  string // "b64:"+xxhash64(mediaType+base64)+"-"+len 或 "url:"+xxhash64(url)+"-"+len
}
```

正缓存只有**一层**:`DescribeStore` 接口,生产上是 SQLite
(`describe_store.go`,`vision_descriptions` 表),测试和无库兜底用一个
带上限的进程内 map。value 只存**拼好的替换文本**,不存图片字节——占用
只随条目数和描述文本长度增长,与图片大小无关。

**为什么要落盘**:本节要解决的核心是"历史图片每轮拿到和上一轮完全相同
的文本",前缀缓存才能稳。只在内存里的话,这个稳定性只在进程存活期内成
立——tb 一重启,一个带十几张截图的会话在下一个请求里要把十几张图**全
部重新描述**,成本大,而且下游看到的是十几段措辞全新的文本,前缀整段作
废。落盘之后,重启只是一次行查询的代价,替换文本字节级一致。

**为什么不在库前面再放一层内存 LRU**:早期版本有过。每个请求确实要对
会话里所有图查一次,但 SQLite 在唯一索引上的点查是几十微秒,比对同一
张图的 base64 算哈希还便宜,相对下游模型的秒级延迟不可见。而两级
带来的是两个事实源、提升 / 写穿逻辑和第三个容量数字,审计时已经因此出
过细节问题。单一事实源更值。

库**不按时间过期**:一行只有几百字节,而一段已经付过费的描述,所属
会话回来得越久越值钱,按年龄删掉恰恰扔掉了这一节要的前缀稳定性。唯一的
兜底是总量上限 100000 行,超出时先删最久未用的,正常使用碰不到。这条规
则在启动时跑一次,之后从写路径上最多每小时跑一次。`Get` 命中时会更新
`last_used_at`(它只用来决定淘汰顺序),同一行每小时最多更新一次——否
则一个带几十张历史图的会话每个请求都要发几十条 UPDATE。

**session 分量为什么不用 `typ.SessionID.String()`**:那个 JSON 里带
`ip_backup`。条目一旦跨进程存活,key 就必须只随会话本身变化——用户换个
网络继续同一个会话,不该让已经付过费的描述全部失效。所以取
`<source>:<value>`。

**降级**:库从 `StoreManager` 的共享连接上打开(`server.go` 的
`newVisionDescribeStore`);拿不到连接或迁移失败只打日志,退化为进程内
map,不阻塞启动。库的任何读写错误也只记日志,当作未命中/未写入,绝不
让请求失败。

**为什么 key 里有 session,而不是纯按图片内容全局寻址**:讨论中发现
"同一段字节"不等于"同一次提问该复用同一个答案"——视觉模型这次描述得不
好,用户重发同一张图本来还有机会拿到更好的答案,纯全局缓存会把这个坏
描述钉死;两个不相关会话恰好发了内容相同的图片
(占位图、测试图)也不该互相复用描述。把 `typ.SessionID`(复用
`routing.ResolveSessionID` 已有的 `metadata.user_id` > `X-Tingly-Session-ID`
header > client IP 兜底)纳入 key 后,缓存回答的问题变成"这是**这个会
话**里已经看过的图吗",而不是"这段字节全局出现过吗"——会话有自然的
生命周期,新会话就是全新的图,因此不需要再叠加 TTL。

**为什么 key 里还要有 provider+model**:vision service 在同一 session
内也可能被用户中途切换(rule/scenario 改了配置)。不带这两个维度,切
模型后同一张图会静默复用旧模型的描述,而且这个"没生效"完全无感知。带
上之后,换模型 = 自动、免费地让相关缓存失效,不需要额外监听配置变更。

### 10.3 对 §4.3 处理流程的改动:去掉"最新消息"判定,改为有界、从新到旧

早期规则是"最新一条消息里的图描述、历史消息里的图打 marker"。有了缓存
之后重新审视,这条规则有两个问题:

1. **历史图片一旦未命中就永远是 marker**。它之后每一轮都是历史消息,
   永远不会再成为"最新",永远没机会写入缓存。切换 vision 模型
   (key 里的 provider+model 变了)、上一轮描述失败、会话中途才开启
   proxy、从别的实例迁移过来、触顶淘汰——任何一种都让那些图从此对模
   型不可见。
2. **"最新消息是哪条"本身是位置启发式**。Claude Code 会在 tool_result
   后面再追加一条 system 消息(#1640 修的就是这个),OpenAI 的 tool
   消息、Responses 的多种 item 形态,每种协议都要单独维护"什么算能带
   图的消息"。

理想状态当然是**全量替换**:模型看到的每张图都是真描述。但一个已经带
几十张截图的长会话在开启 proxy 的第一个请求就全描述,成本和延迟都不可
接受。所以取有界版本:

1. **任何位置的图片先查缓存**——命中直接替换,不占名额。
2. **未命中的图按消息顺序收集,然后逆序、折叠**(`boundNewestFirst`):
   最新的排在最前;同一张图在一个请求里出现多次只算一张(`foldDuplicates`,
   描述一次拼到所有位置——否则两处会拿到两段不同文本,而缓存只留一
   份,下一轮另一处必然变);取前 N 张调 vision 描述,**成功后写入缓存**
   (失败 / fail-strip 结果只进短 TTL 的内存负缓存,见 §10.4)。fan-out
   也按这个顺序派发:当前轮的图最先发出,请求 ctx 被截断时最后受影响
   的才是它。
3. **超出 N 的图打 `imageOverLimitText`**,本轮不调 vision。
4. **每次 describe 有独立超时**(`defaultDescribeTimeout` = 60s,
   `TINGLY_VISION_DESCRIBE_TIMEOUT` 可覆盖)。此前只有请求 ctx,vision
   模型卡住会拖住整个请求直到客户端放弃。超时按失败处理,进负缓存。

因为描述过的图从此命中缓存、不占名额,每一轮的名额都自然落到"下一批
最旧的未命中"上,几轮之后整个历史都进缓存——**有界版本在多轮之后收
敛到全量**,而单次请求的成本和延迟始终有上界。这就是"可用、一致、有
界"三者同时成立的方式,剩下的只是 N 取多大:默认 8,配合
`describeConcurrency` = 4 最多两轮上游往返;`TINGLY_VISION_DESCRIBE_LIMIT`
可按 vision 模型快慢调整。

`latestImageAnchor` 及四个 `collect*` 里的 `lastIdx` / `isLast` 随之
删除;`collect*` 只剩"遍历、查缓存、未命中就收集",名额裁剪集中在
`Process` 一处。

**哈希用 xxhash64 + 长度,不用 sha256**:每张历史图每轮都要算一次,
2MB 的 base64 用 sha256 约 8ms,30 张截图的会话每轮白付四分之一秒;
xxhash 快一个数量级。key 已按 session 和 service 分区,碰撞要在几十张
图的空间里发生,64 位加精确长度绰绰有余,且伪造碰撞只影响自己的会话。

`VisionProxyProcessor.Process` 因此多了一个 `sessionID typ.SessionID`
入参;调用方(`applyVisionProxy`,§4.2 提到的钩子位置)在调用前用
`resolveSessionID(c, typedRequest)` 提前独立求一次这个值——它是纯函
数,不依赖入站 handler 里"session 注入 context"那一步的先后顺序,所以
不需要挪动任何现有代码。

### 10.4 已知局限

- session 在没有 `metadata.user_id` / `X-Tingly-Session-ID` header 时兜
  底用 client IP——同一 NAT 后的不同用户会被分到同一个"session 桶"。这
  是 `typ.SessionID` / `routing.ResolveSessionID` 本身既有的权衡(LB
  affinity 已经在承担同样的代价),缓存层如实继承,不重新设计。
- 缓存在每个网关实例自己的 `tingly.db` 里,多实例之间不共享。
- 前缀仍会在这几种情况下断一次:切换 vision service(key 里的
  provider+model 变了,属有意为之)、描述失败(fail-strip 结果不入
  缓存,TTL 过后重新占一个名额描述,文本随之变化)、库触顶后淘汰掉了
  行。
- 描述失败走**纯内存、短 TTL 的负缓存**(`describeFailureTTL` = 10 分
  钟,不落盘):TTL 内该图直接打 `imageUnavailableText`,不重试、不占
  名额。理由是永久失败的图(死链、上游必拒的字节)若每轮重试,会每轮
  占满名额,把它后面所有更旧的图饿死;而暂时性失败(限流、网络)在
  TTL 过后自然重试。TTL 是这两类无法区分的失败之间的折中,不解析上游
  错误码。重启后全部重新尝试一次。
- URL 图片以 URL 文本为身份。每次请求都变化的 URL(带轮换签名 / 过期
  时间的预签名链接)每轮都是一张"新图",会被重新描述;不做 query 剥离,
  因为无法区分哪些参数是签名、哪些是图片本身的一部分。
- 没有可用 vision service 时,所有未命中的图统一打 `imageUnavailableText`,
  不进入名额裁剪:此时"超出名额"和"失败"是同一个状态,分成两种 marker
  只会误导下游模型以为前者下一轮会好。
- 两个并发请求同时带一张新图时都会各描述一次,后写者覆盖缓存。**有
  意不做 singleflight 合并**:并发请求各自拿到一份独立描述,保留了视觉
  模型输出的多样性;缓存只负责让同一会话的后续轮次稳定在最终写入的那
  一份上。

---

## 11. 验证场景

这是 vision proxy 在真实使用中要成立的场景清单,每一行对应
`internal/vision/visionproxy/vision_scenario_test.go` 里的一个测试。测试
通过 `Service.Apply`(含 rule/scenario 解析)驱动,请求用 Claude Code 真
实的消息形状(prompt / system-reminder / assistant tool_use / user
tool_result+image / 尾部 system-reminder),fake vision 客户端每次返回不
同措辞,所以任何一次重描都会以文本变化暴露出来。改动缓存或名额逻辑时
先跑这组;新增场景先加行再加测试。

| # | 场景 | 期望 | 测试 |
|---|------|------|------|
| S1 | 工具循环:每轮多一张截图 | 每张图整个会话只描述一次;历史图片的文本与首次描述时字节一致 | `TestScenario_ToolLoop_HistoryTextIsStable` |
| S2 | 网关重启 | 新进程、同一个 tingly.db,下一轮零次 vision 调用,文本字节一致 | `TestScenario_Restart_NoRedescribe` |
| S3 | 中途切换 vision 模型 | 旧模型的描述一律不复用;按上限每轮从新到旧重描,全部重描后稳定 | `TestScenario_ModelSwitch_ReconvergesBounded` |
| S4 | 会话进行到一半才开启 proxy | 每轮描述上限张最新的,其余打暂缓 marker;ceil(N/上限) 轮后收敛,之后零调用 | `TestScenario_EnabledMidConversation_Converges` |
| S5 | vision 上游故障后恢复 | 故障期 marker 恒定、TTL 内不重试;TTL 后重试一次,恢复后的文本稳定 | `TestScenario_UpstreamOutage_ThenRecovery` |
| S6 | 同一张截图在一个请求里出现两次 | 只描述一次,两处文本相同,下一轮不变 | `TestScenario_SameScreenshotTwice_OneDescribe` |
| S7 | 历史里有一张永远描述失败的图 | 它占一次名额后进负缓存,下一轮名额落到它后面的图 | `TestScenario_PermanentlyBadImage_DoesNotStarveHistory` |

以下场景经分析不需要专门处理,记录结论以免重复讨论:

| 场景 | 结论 |
|------|------|
| Claude Code `/compact` | 历史图片随摘要消失,剩下的都是新图,按常规路径处理 |
| Claude Code `/resume` | `metadata.user_id` 里的 session 不变,继续命中 |
| fork 出新会话 | 新 session,按 S4 收敛 |
| Codex / OpenCode | header 带 session_id,与 Claude Code 等价 |
| 无 session 的 OpenAI 客户端 | IP 兜底,同 NAT 共桶;既有权衡,见 §10.4 |
| imbot 驱动的 agent | 经 agentboot 启动 Claude Code,走 S1 路径 |
| 一条消息里超过上限张图 | 最新的上限张描述,其余暂缓,按 S4 收敛 |
| provider 被删或停用 | 全部统一 fail-strip,marker 恒定,不进名额裁剪 |
| vision 上游卡住 | per-describe 超时(§10.3 第 4 条)兜底,按 S5 处理 |
| 配置热重载 | 新建 Service,负缓存清空,库不变 |
| 多实例 | 各自的 tingly.db,不共享,见 §10.4 |
| 表触顶 10 万行 | 最久未用先删,见 §10.2 |
| 预签名 URL 每轮变化 | 每轮视为新图,见 §10.4 |
