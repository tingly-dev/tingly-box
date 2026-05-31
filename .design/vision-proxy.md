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

约定 key:`internal/server/config/flag.go` 的 `VisionProxyServiceKey`。

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

不论 rule 级还是 scenario 级,都从同一个 helper 进:

```go
// internal/server/vision_proxy.go
func (s *Server) applyVisionProxy(c *gin.Context, scenarioType typ.RuleScenario, rule *typ.Rule, typedRequest any) {
    svc := s.resolveVisionService(scenarioType, rule)  // rule 先,scenario 后
    if svc == nil { return }
    _ = s.visionProxyProcessor.Process(&smartrouting.ProcessorContext{
        Ctx:      c.Request.Context(),
        Request:  typedRequest,
        Services: []*loadbalance.Service{svc},
    })
}
```

优先级集中在 `resolveVisionService` 一个纯函数里,可单测、可读。**只
Process 一次**——既不需要"两个 helper 串联 + lock 互斥",也不存在
"图描述两次"的窗口。

### 4.2 钩子位置

每个入站 handler(`openai_chat.go` / `openai_responses.go` /
`anthropic.go` 统管 v1 + beta)在 `determineRuleWithScenario` 之后、
`SelectService` 之前调用:

```go
rule, err = s.determineRuleWithScenario(c, scenarioType, modelName)
// ...
s.applyVisionProxy(c, scenarioType, rule, typedRequest)
provider, _, err = s.routingSelector.SelectService(c, scenarioType, rule, typedRequest)
```

放在 `SelectService` 之前是为了让下游接到的就是已经"图→文"完成的请求。

### 4.3 处理器细节

`VisionProxyProcessor`(`internal/server/processor/vision_proxy.go`)在
`server.go` 启动时构造一次,被 `Server.visionProxyProcessor` 持有,这
里直接调用——不走 smart routing 注册表(后者已删,§7)。

处理器原地改写请求里的 image block:
- 最新一条消息里的 image → 调上游 vision 模型描述
- 历史消息里的 image → 打 `imageHistoricalText` marker(**不调** vision)
- 失败兜底(无可用 service / 上游报错 / 空响应)→ 打
  `imageUnavailableText` marker

支持三种请求形态:`*anthropic.BetaMessageNewParams` /
`*anthropic.MessageNewParams` / `*openai.ChatCompletionNewParams`。
Anthropic 两种形态还会**下钻 `OfToolResult.Content`** 处理工具返回里
的图(见 §6.1 的踩坑记录)。

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
策略。OpenAI 协议的 tool role message 内容是字符串、不含 image,OpenAI
路径不需要此处理。

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

`pkg/obs/multi_logger.go` 的 `WriteEntry` 路由策略:

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

`applyVisionProxy` 从 `c.Request.Context()` 取 ctx 传给
`ProcessorContext.Ctx`,processor 再传给 `describe(ctx, ...)`,最终到
`logrus.WithContext(ctx)`。这条链路无任何 `context.Background()` 截断,
所以中间件早期注入的 `request_id`(见
`internal/server/middleware/multi_mode_memory_log.go`)自然贯穿。

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

后端:
- `internal/smart_routing/op.go` —— `PositionProxyVision` / `OpProxyVisionEnabled` 常量,Operations 列表项
- `internal/smart_routing/type.go` —— `IsValid` 里的 `PositionProxyVision` case
- `internal/smart_routing/routing.go` —— `evaluateProxyVisionOp` 及其 switch case
- `internal/server/processor/processor.go` —— `RegisterAll` 不再 `smartrouting.RegisterProcessor(...)`(processor 仍然构造并返回给 vision_proxy.go 用)

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
| 处理器实现(图描述、改写) | `internal/server/processor/vision_proxy.go` |
| 处理器接口 / `ProcessorContext` | `internal/smart_routing/processor.go` |
| **统一入口 helper**(`applyVisionProxy` + `resolveVisionService`) | `internal/server/vision_proxy.go` |
| `RuleFlags` + `VisionProxyService` | `internal/typ/type.go` |
| Flag registry + `FlagTypeServiceRef` 常量 | `internal/typ/flag_registry.go` |
| `ScenarioFlags` / `ScenarioConfig` | `internal/typ/type.go` |
| 场景配置 Get/Set | `internal/server/config/config.go` |
| 场景配置 API | `internal/server/module/scenario/{routes,handler,types}.go` |
| `VisionProxyServiceKey` 常量(Extensions key) | `internal/server/config/flag.go` |
| 入站 handler(钩子点) | `internal/server/{openai_chat,openai_responses,anthropic}.go` |
| Scenario 级 UI | `frontend/src/components/PluginFeatures.tsx` |
| Rule 级 UI | `frontend/src/components/rule-card/FlagCatalogDialog.tsx` |
| `RuleFlags` ↔ wire 转换 | `frontend/src/components/rule-card/{utils.ts,useRuleCardHooks.ts}` |
| 类型定义 | `frontend/src/components/RoutingGraphTypes.ts` |
| 服务选择器对话框(复用) | `frontend/src/components/ModelSelectDialog.tsx` |

---

## 9. 测试

| 层 | 用例 |
|----|------|
| `resolveVisionService` 优先级 | rule + scenario 都配 → rule;只 scenario → scenario;只 rule → rule;都不配 → nil;rule 配但 model 空 → 回退 scenario;nil rule + scenario → scenario |
| `applyVisionProxy` 行为 | rule 配 + 有图 → 用 rule service 描述;scenario 配 + 有图 → 用 scenario service;都没配 + 有图 → 图保留(no-op);profile 场景(`claude_code:p1`)配的 service 能找到(独立于 base) |
| 单次 Process 不变量 | 两者都配时 Process 也只调一次(用 rule 的 service) |
| `parseScenarioVisionService` | nil/缺键/结构错/缺 provider/缺 model/空串 → nil;provider+model 齐备 → active service |
| 处理器三种请求形态 | Beta / V1 Anthropic、OpenAI ChatCompletion 各覆盖 |
| **tool_result 嵌套 image** | Beta + V1 各一例:tool_result 内的 image 最后一条消息会描述、历史消息只打 marker(不调 vision) |
| smart routing 残留 | `LookupProcessor(PositionProxyVision, OpProxyVisionEnabled)` 不再可达;catalog 新建 smart rule 时无 `proxy_vision` 选项;老配置带该 op → unmatched,不报错 |
| Flag registry 暴露 | `GET /rule/flags/registry` 返回的 `vision_proxy_service` 项 type=`service_ref` |
| 类型反序列化 | `Rule.Flags.VisionProxyService` 从 JSON 圆环(marshal → unmarshal)保持一致 |
