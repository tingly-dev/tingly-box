# Vision Proxy —— rule 级扩展 + smart routing 路径清退

> 适用对象：tingly-box 后端 / 前端贡献者。
> 已有文档：[`vision-proxy-scenario.md`](vision-proxy-scenario.md) 描述
> **scenario 级**的实现；本文档接续它，描述 rule 级路径的加入与
> smart routing `proxy_vision` op 的彻底移除。

---

## 1. 出发点

### 1.1 rule 级是什么

**rule 级和 scenario 级在行为上等价、在优先级上 rule 更具体：**

| 维度 | scenario 级（已存在） | rule 级（本文档新增） |
|------|---------|---------|
| 数据形态 | `{provider, model}` | `{provider, model}` |
| 配置位置 | `ScenarioConfig.Extensions["vision_proxy_service"]` | `Rule.Flags.VisionProxyService` |
| 作用域 | 该场景下所有请求 | 仅匹配到该 rule 的请求 |
| 行为 | 描述图、替换图、走下游 | **同上** |
| **两者都配时** | — | **rule 胜出（更具体）；scenario 兜底** |

| 配置情况 | 用什么 |
|------|------|
| rule 配了 | rule 的 service |
| rule 没配、scenario 配了 | scenario 的 service |
| 都没配 | 不启用 |
| 都配了 | **rule 的**（rule 更具体） |

### 1.2 为什么需要它

同一场景下不同 rule 想用**不同**的视觉模型（或部分 rule 干脆不想开），
scenario 级一刀切不够细。rule flag 是已有的"per-rule 局部开关"机制
（cursor_compat / thinking_effort / block_tools 都在这），vision proxy
进来就在这一层，与同类配置语义对齐。

### 1.3 smart routing `proxy_vision` op 的彻底清退

上一个 PR 把它在前端 catalog 隐藏、保留了后端机制做兜底。现在 scenario
级 + rule 级覆盖了所有合理的作用域选择，smart routing 那条路再也没有
独占的能力，**这次彻底删掉**：删 processor 注册、删 evaluator、删
position/op 常量；前端把残留入口（OPERATION_OPTIONS 那个 label 表项、
类型 union 里的字符串）一并清掉。已存的 smart routing 规则里若还带
`proxy_vision` op，加载时 evaluator 缺失 → op 永远不匹配 → 静默 no-op
（无报错；详见 §6 的迁移说明）。

---

## 2. 数据模型

### 2.1 `VisionProxyService` 结构

`internal/typ/type.go`（与 `RuleFlags` 同文件即可，scenario 路径目前用
`map[string]interface{}` 解析，rule 路径直接 typed struct，更稳）：

```go
type VisionProxyService struct {
    Provider string `json:"provider" yaml:"provider"`
    Model    string `json:"model"    yaml:"model"`
}
```

### 2.2 `RuleFlags` 加字段

```go
type RuleFlags struct {
    // ... 既有字段 ...

    // VisionProxyService enables the rule-scoped vision proxy when set.
    // When a request matched by this rule carries an image, the configured
    // service describes it and the image block is replaced with text before
    // the request reaches the downstream model. Semantically identical to
    // ScenarioConfig.Extensions["vision_proxy_service"] — only narrower in
    // scope (per-rule instead of per-scenario).
    VisionProxyService *VisionProxyService `json:"vision_proxy_service,omitempty" yaml:"vision_proxy_service,omitempty"`
}
```

`*` 指针 + `omitempty`：未配置 → JSON 不出现 → "rule 级未启用"，与 scenario
级"Extensions 里没有该 key"的"未启用"语义对称。

---

## 3. Flag registry 的新类型

### 3.1 现状回顾

`internal/typ/flag_registry.go` 现支持 4 种类型：`bool` / `string` /
`enum` / `int`。每条 flag 在 `RuleFlagRegistry()` 里声明 `FlagSpec`，
前端 `FlagCatalogDialog` 按 `spec.type` 分支渲染。

### 3.2 新增 `FlagTypeServiceRef`

不引入泛化的 "object" 类型——只为 vision proxy 这一种结构开一个**专名**
类型，叫 `service_ref`：

```go
const (
    FlagTypeBool       FlagValueType = "bool"
    FlagTypeString     FlagValueType = "string"
    FlagTypeEnum       FlagValueType = "enum"
    FlagTypeInt        FlagValueType = "int"
    FlagTypeServiceRef FlagValueType = "service_ref"   // ← 新增：{provider, model}
)
```

为什么用专名而非通用 `object`：
- 泛化 object 需要 schema 描述（字段名、字段类型……），FlagSpec 立刻
  变重；
- 这是 codebase 里唯一的"结构对象 flag"需求；
- 专名让前端能直接路由到 `ModelSelectDialog`，不用通用表单引擎；
- 将来若有别的结构需要（不太可能），照样起新名（如
  `prompt_template_ref`）。

注册项：

```go
{
    Key:         "vision_proxy_service",
    Label:       "Vision Proxy",
    Description: "Describe images via a vision-capable model so text-only downstreams can read them. Applies only to requests matched by this rule.",
    Type:        FlagTypeServiceRef,
    Category:    FlagCategoryRequest,   // 或新加 FlagCategoryVision，待定
}
```

---

## 4. 执行接入

### 4.1 不走 transform 链

transform 链只做本地 mutation，不能调上游 API（vision describer 是
真实的上游调用）。沿用既有路径模式：在 `SelectService` 之前，由一个
helper 直接调 `VisionProxyProcessor.Process()`。

### 4.2 单一入口：rule 优先、scenario 兜底，集中在一处

不再用"两个并列 helper + 锁互斥"。改为**一个 helper**，内部先查 rule、
再查 scenario，决定用哪个 service，**只 Process 一次**。优先级显式表
达在一个函数里，可单测、可读、不需要 `vision_proxy_applied` 这类锁。

PR #1082 引入的 `applyScenarioVisionProxy` 在本 PR 被重构为统一的
`applyVisionProxy`（场景级和 rule 级共用），原函数和它那把 lock 一起
退场。两条路径行为本就等价，合并不丢能力。

`internal/server/vision_proxy.go`（新文件，替代
`vision_proxy_scenario.go`；旧文件删除）：

```go
// resolveVisionService picks the effective vision service for this
// request. Rule level wins over scenario level when both are set —
// the more specific scope is taken to be the user's intent.
// Returns nil when neither scope configures one (proxy disabled).
func (s *Server) resolveVisionService(scenarioType typ.RuleScenario, rule *typ.Rule) *loadbalance.Service {
    // Rule level
    if rule != nil && rule.Flags.VisionProxyService != nil {
        if svc := buildService(rule.Flags.VisionProxyService.Provider, rule.Flags.VisionProxyService.Model); svc != nil {
            return svc
        }
    }
    // Scenario level
    if cfg := s.config.GetScenarioConfig(scenarioType); cfg != nil {
        if svc := parseScenarioVisionService(cfg.Extensions); svc != nil {
            return svc
        }
    }
    return nil
}

func (s *Server) applyVisionProxy(c *gin.Context, scenarioType typ.RuleScenario, rule *typ.Rule, typedRequest any) {
    if s.visionProxyProcessor == nil || typedRequest == nil {
        return
    }
    svc := s.resolveVisionService(scenarioType, rule)
    if svc == nil {
        return
    }
    _ = s.visionProxyProcessor.Process(&smartrouting.ProcessorContext{
        Ctx:      c.Request.Context(),
        Request:  typedRequest,
        Services: []*loadbalance.Service{svc},
    })
}
```

`buildService` 是个本地小工具（provider/model 非空才返回 service，
统一 Active/Weight 默认值），`parseScenarioVisionService` 即原
`parseVisionProxyService`（解析 Extensions 里那个 nested map）搬过来。

### 4.3 调用点

每个入站 handler（`openai_chat.go` / `openai_responses.go` /
`anthropic.go`）当前的这两行：

```go
s.applyScenarioVisionProxy(c, scenarioType, typedRequest)
provider, _, err := s.routingSelector.SelectService(c, scenarioType, rule, typedRequest)
```

替换成：

```go
s.applyVisionProxy(c, scenarioType, rule, typedRequest)
provider, _, err := s.routingSelector.SelectService(c, scenarioType, rule, typedRequest)
```

注意调用点位置不变（仍在 `SelectService` 之前，仍在
`determineRuleWithScenario` 之后——helper 现在需要 rule 参数）。

### 4.4 关于 `vision_proxy_applied` 锁

PR #1082 加这把锁是为了防 scenario 路径与 smart routing 路径互踩。
现在：
- smart routing 路径被删（§1.3 + §5.2）；
- 场景级 + rule 级合并为单次 Process。

**锁不再必要，本 PR 一起删除。** 减少状态。

### 4.5 与 §1.3 移除 smart routing 的关系

scenario 级 + rule 级覆盖之后，smart routing 那条路（按 `HasImage` 隐式
匹配，靠 smart rule 的 services 池跑 vision）在表达力上**真的没有任何
新增价值**了——它能做的，rule 级（按 rule 匹配 + flag 显式配 service）
更直接、语义更清晰。所以这次直接删干净，不再保留兜底。

---

## 5. 后端改动清单

### 5.1 加 rule 级路径 + 合并两路执行

| 文件 | 改动 |
|------|------|
| `internal/typ/type.go` | 加 `type VisionProxyService struct` + `RuleFlags.VisionProxyService *VisionProxyService` |
| `internal/typ/flag_registry.go` | 加 `FlagTypeServiceRef` 常量；`RuleFlagRegistry()` 列表追加 `vision_proxy_service` 项 |
| `internal/server/vision_proxy.go`（新增，**替代**旧 `vision_proxy_scenario.go`） | `applyVisionProxy(c, scenarioType, rule, typedRequest)` 统一入口 + `resolveVisionService(scenarioType, rule)`（rule 优先、scenario 兜底）+ 内部 `parseScenarioVisionService(extensions)` |
| `internal/server/vision_proxy_scenario.go` | **删除**：函数和 `visionProxyAppliedKey` 锁迁入 `vision_proxy.go` 并被合并/裁掉 |
| `internal/server/vision_proxy_scenario_test.go` / `_wiring_test.go` | 重命名/重写为 `vision_proxy_test.go`：parse 单测保留（针对 scenario 解析）+ rule helper 单测 + 优先级单测（rule 与 scenario 都配时 rule 胜出）+ 既有 wiring 用例平移 |
| `internal/server/openai_chat.go` / `openai_responses.go` / `anthropic.go` | `s.applyScenarioVisionProxy(c, scenarioType, typedRequest)` → `s.applyVisionProxy(c, scenarioType, rule, typedRequest)`（多传一个 rule 参数；调用位置不变） |

### 5.2 清退 smart routing `proxy_vision`

| 文件 | 改动 |
|------|------|
| `internal/server/processor/processor.go` | `RegisterAll` 不再调 `smartrouting.RegisterProcessor(PositionProxyVision, ...)`；继续返回 `*VisionProxyProcessor`（scenario / rule 路径仍要用） |
| `internal/smart_routing/op.go` | 删 `PositionProxyVision` / `OpProxyVisionEnabled` 常量；删 Operations 列表里的对应项 |
| `internal/smart_routing/type.go` | `SmartOpPosition.IsValid()` 的 case 列表去掉 `PositionProxyVision` |
| `internal/smart_routing/routing.go` | 删 `evaluateProxyVisionOp` + 调用它的 switch case |
| 测试 | `stage_smart_routing_processor_test.go`、`processor_test.go` 等显式注册/卸载 `PositionProxyVision` 的用例删掉或改成"该 op 已不存在"的负向用例 |

### 5.3 数据迁移

线上配置可能仍带 `smart_routing: [{ops:[{position:"proxy_vision"}]}]`：

- JSON 反序列化时 `SmartOpPosition` 是字符串别名，能保留 `"proxy_vision"` 字面值；
- 路由 evaluator 的 switch 缺这个 case → `default` 分支返回 unmatched
  （需确认 default 分支不返回 error；调研报告说默认行为是 unmatched 静默
  返回，本 PR 实现时再实测一次）；
- 结果：老配置加载成功、proxy_vision op 永不匹配、整条 smart rule 不
  命中，**该 rule 等同于失效**。

> **风险**：用户已在 smart routing 里配了 `proxy_vision` 但**没在 rule
> flag 或 scenario plugin 里重配**的话，升级后会突然"没效果"。本 PR
> 的 release note 必须明确告知 + 建议改用 rule flag 或 scenario plugin。
> 不打算写代码做自动迁移（场景边界太多，自动迁移容易把 services 池
> 错位）。

---

## 6. 前端改动清单

### 6.1 渲染新 flag 类型

`frontend/src/components/rule-card/FlagCatalogDialog.tsx`：

| 元素 | 实现 |
|------|------|
| `FlagValueType` union | 加 `'service_ref'` |
| `isFlagActive(spec, flags)` | `service_ref` 分支：`obj.provider && obj.model` 才算 active |
| getter `flagToServiceRef(flags, key)` | 返回 `flags.visionProxyService \|\| { provider:'', model:'' }` |
| setter | 写回 `flags.visionProxyService` |
| 渲染分支 | `service_ref` → 按钮 `Vision Proxy: <model> ▾`（未配显示 Off）；点击弹下拉 `Off / On — <model>`；`On` 项打开 `ModelSelectDialog` 让用户选 provider+model（与场景级控件完全一致的范式） |

UI 心智：rule 级控件和 scenario 级控件**外观、交互完全一致**，因为效果
等价；差别只在控件挂在哪里（场景页 Plugin 行 vs rule 编辑卡的
Extensions 区）。这是有意为之，降低学习成本。

### 6.2 清退 smart routing 残留

`frontend/src/components/rule-card/SmartRuleCatalogDialog.tsx`：
- 删 `POSITION_OPTIONS` 里 `proxy_vision` 那段注释代码（已经注释，直接删除整段）；
- 删 `OPERATION_OPTIONS.proxy_vision` 项（之前保留是给老配置渲染 label，本次彻底放弃，老配置那个 op 显示 `(unknown)` 即可，迁移文档里说明）。

`frontend/src/components/RoutingGraphTypes.ts`：
- `SmartOp.position` 字符串 union 去掉 `'proxy_vision'`。

### 6.3 类型定义

`frontend/src/types`（或现有 rule 类型文件）的 `RuleFlags` 加：

```ts
visionProxyService?: { provider: string; model: string };
```

API SDK 是 codegen 的，按 CLAUDE.md 约定先放 placeholder，提示用户后续
用 swagger 重新生成。

---

## 7. 测试

### 7.1 必加

| 层 | 用例 |
|----|------|
| `resolveVisionService` 优先级 | rule 配 + scenario 配 → 返回 rule 的；rule 不配 + scenario 配 → 返回 scenario 的；都不配 → nil；rule 配但 provider 或 model 空 → 回退到 scenario |
| `applyVisionProxy` 集成 | rule 配 + 有图 → 用 rule service 描述；scenario 配 + 有图 → 用 scenario service；都没配 + 有图 → 图保留（no-op）；有 rule 配但无图 → no-op |
| 单次 Process | 同一次请求中 `Process` 最多被调一次（即使两者都配，也只描述一次 —— 由 rule 的 service） |
| 类型反序列化 | `Rule.Flags.VisionProxyService` 从 JSON 圆环（marshal → unmarshal）保持一致 |
| Flag registry 暴露 | `GET /rule/flags/registry` 返回的 `vision_proxy_service` 项 type=`service_ref` |
| 既有 scenario 路径回归 | PR #1082 的 wiring 测试场景（profile scenario `claude_code:p1` 配了 service → 图被替换）在合并后的 `applyVisionProxy` 上仍通过 |

### 7.2 smart routing 清退验证

| 用例 | 期望 |
|----|------|
| 老配置加载 | smart_routing 数组里带 `position:"proxy_vision"` → 加载无 error；该 op evaluator 缺失 → unmatched；整条 smart rule 不命中、不报错 |
| 处理器注册表 | `LookupProcessor(PositionProxyVision, OpProxyVisionEnabled)` 不再返回 ok |
| 前端 catalog | 新建 smart rule 时 position 列表不含 proxy_vision |

---

## 8. 关键文件索引

| 功能 | 文件 |
|------|------|
| 处理器实现（复用，不动） | `internal/server/processor/vision_proxy.go` |
| 统一 helper（新增，替代旧 `vision_proxy_scenario.go`） | `internal/server/vision_proxy.go` |
| `RuleFlags` + `VisionProxyService` | `internal/typ/type.go` |
| Flag registry + 新类型常量 | `internal/typ/flag_registry.go` |
| Handler 钩子点 | `internal/server/{openai_chat,openai_responses,anthropic}.go` |
| Flag 渲染 | `frontend/src/components/rule-card/FlagCatalogDialog.tsx` |
| Smart routing 残留清理 | `internal/smart_routing/{op,type,routing}.go`、`frontend/src/components/rule-card/SmartRuleCatalogDialog.tsx`、`frontend/src/components/RoutingGraphTypes.ts` |

---

## 9. 与 scenario 文档的关系

`vision-proxy-scenario.md` 描述场景级路径的设计动机、数据模型、UI、
踩过的坑。**用户视角**它仍然准确（"在场景页配一个视觉模型即启用"
没变）。**实现视角**它有一处过时：那条路径的执行入口 `applyScenarioVisionProxy`
和 `visionProxyAppliedKey` 锁，被本 PR 合并/删除（见 §4 / §5.1）。

具体说：
- 想了解**场景级的产品定位与用户操作** → `vision-proxy-scenario.md` 仍然权威；
- 想了解**当前的代码组织（执行路径合并、rule 级、smart routing 清退）** → 看本文档；
- 两者**效果等价、rule 优先 scenario 兜底**（§1.1）是本 PR 的核心语义。

合并这两篇成一份 `vision-proxy.md` 是合理的下一步，但留给后续——本 PR
不动 scenario 文档的内容，避免把"功能演进"和"文档重排"混在一起评审。
