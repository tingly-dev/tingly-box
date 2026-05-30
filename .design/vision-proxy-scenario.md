# Vision Proxy —— 场景级开关（Scenario Plugin）

> 适用对象：tingly-box 后端 / 前端贡献者。
> 本文档描述把 **vision-proxy（图像代理）** 从"智能路由专属"扩展为
> **每个场景可独立开启的 plugin 特性**的设计与实操。
> 智能路由内的 `proxy_vision` op 行为**保持不变**，本设计是叠加而非替换。

---

## 1. 背景与动机

vision-proxy 的作用：当请求里带图片、但下游是纯文本模型时，先用一个
**有视觉能力的模型**把图片描述成文本，再把图片块替换为该文本，让纯文本
模型也能"看懂"图片。

### 现状的问题

目前 vision-proxy **只能通过智能路由（smart routing）配置**：它是一个
`proxy_vision|enabled` 的 `SmartOp`，挂在某条 `SmartRouting` 规则上，用该
规则的 `Services` 池作为视觉上游。

要用它，用户必须穿过 4 层嵌套概念：

```
Rule → 启用 Smart Routing → 新建 Smart Rule → 加 proxy_vision op → 再配视觉 services
```

对"我只想让图片能被处理"的普通用户，这个认知和操作成本过高，而且
"图像代理"藏在"智能路由的一个操作"里，非常不直观。

### 设计目标

把 vision-proxy 提升为**场景级（per-scenario）的 plugin 开关**，落在每个
场景已有的配置体系里（`PluginFeatures` 那一块，和 `smart_compact` /
`thinking_effort` / `clean_header` 并列）：

- **粒度 = 场景**：`claude_code` 配一套视觉服务，`codex` 配另一套，各管各。
  这正是用户期望的形态——不是全局统一，也不是 per-rule。
- **可发现**：打开任意场景页面就能看到这个开关。
- **一次配置**：一个开关 + 一个视觉 **service（provider + model 二元）** 选择器。
- **零行为改动**：复用现有 `VisionProxyProcessor.Process()`，逻辑不动。

---

## 2. 路径职责与 smart routing op 的去留

| 粒度 | 入口 | 视觉服务来源 | 状态 |
|------|------|------|------|
| **场景级**（本设计） | 场景 plugin 的 `vision_proxy` 开关 | `ScenarioConfig.Extensions["vision_proxy_service"]` | 唯一对用户透出的入口 |
| smart routing `proxy_vision` op | smart rule 的 `Services` 池 | 同上 | **deprecated**：后端保留，前端不再透出 |

> **全局方案被否决**：视觉服务应分场景而非全局，因为不同场景（编码助手
> vs 通用对话）对视觉模型的成本/质量取舍不同。
>
> **smart routing 路径为何 deprecated**：proxy_vision op 本身不携带条件
> 维度（其"匹配条件"就是隐式的 `HasImage`），独自存在时与场景级开关在
> 表达力上**完全等价**。它唯一多出的能力是和同一 smart rule 内其他 op
> AND 组合形成"带条件的 vision proxy"，但实际业务里几乎找不到真实
> 用例。同时，smart rule 的 `Services` 字段在普通 op 里意味着"下游候
> 选"，在 proxy_vision op 里意味着"上游视觉描述器"——同字段反义,正是
> 我们要消除的认知负担来源。
>
> **处置**：
> - **本 PR**：前端 `SmartRuleCatalogDialog` 的 `POSITION_OPTIONS` 中
>   `proxy_vision` 已注释，新建 smart rule 时不再可选；`OPERATION_OPTIONS`
>   保留 label 表项以便已存配置渲染。后端 processor 注册不动，已存配置
>   照常工作。
> - **后续**：观察一段时间无人受影响后，可移除后端 op 注册、清理类型
>   union、彻底删除处理器对应分支。

---

## 3. 数据模型

视觉代理需要的只有**一个视觉 service（provider + model）**。

> **没有独立的 on/off 开关**。"是否启用"完全等价于"有没有配视觉
> service"——配了就是开，清空就是关。这样前端把"开关"和"选模型"合并成
> **一个控件**（标签即所选模型，详见 §6），避免"先开 On、再到另一个按钮
> 选模型"的割裂；后端也只有单一事实源，杜绝 flag 与 service 漂移。早期
> 设计里的 `ScenarioFlags.VisionProxy bool` 已移除，`applyScenarioVisionProxy`
> 直接以 service 是否存在为准。

### 3.1 视觉服务 → `ScenarioConfig.Extensions`

视觉目标是一个**完整的 service = provider + model（二元）**，不是只选
provider。它是结构化对象，不适合塞进扁平的 bool/string flag，因此存到
`ScenarioConfig.Extensions`（`map[string]interface{}`，已有的扩展位）：

```jsonc
// ScenarioConfig.Extensions
{
  "vision_proxy_service": {
    "provider": "<provider-uuid>",
    "model": "claude-3-5-sonnet-latest"
  }
}
```

约定 key：`vision_proxy_service`。其结构与 `loadbalance.Service` 的
`{provider, model}` 子集一致——这是系统里 service 的统一建模，前端选择器
必须产出 provider + model 两个元素（复用 `ModelSelectDialog`），不允许
退化成只选 provider。

> **为什么不放 ScenarioFlags？** ScenarioFlags 是扁平的 bool/string/enum，
> 装不下 `{provider, model}` 这种结构体；Extensions 正是为这类配置预留的。

---

## 4. 执行流程

### 4.1 钩子点：在服务选择之前

每个入站 handler（`openai_chat.go` / `openai_responses.go` /
`anthropic.go` 统管 v1 + beta）在 `determineRuleWithScenario` 解析出
rule、**调用 `SelectService` 之前**，插入一个共享 helper：

```go
// internal/server/vision_proxy_scenario.go（新增）
func (s *Server) applyScenarioVisionProxy(
    c *gin.Context,
    scenarioType typ.RuleScenario,
    typedRequest any, // *anthropic.BetaMessageNewParams / *anthropic.MessageNewParams / *openai.ChatCompletionNewParams
) {
    cfg := s.config.GetScenarioConfig(scenarioType)
    if cfg == nil {
        return
    }
    svc := parseVisionProxyService(cfg.Extensions) // 读 Extensions["vision_proxy_service"]，service 存在即启用
    if svc == nil {
        return
    }
    // 复用现有处理器，零行为改动
    s.visionProxyProcessor.Process(&smartrouting.ProcessorContext{
        Ctx:      c.Request.Context(),
        Request:  typedRequest,
        Services: []*loadbalance.Service{svc},
    })
}
```

调用位置示例（`openai_chat.go`，`SelectService` 之前）：

```go
rule, err = s.determineRuleWithScenario(c, scenarioType, req.Model)
// ...
s.applyScenarioVisionProxy(c, scenarioType, &req.ChatCompletionNewParams) // ← 新增
provider, selectedService, err = s.routingSelector.SelectService(c, scenarioType, rule, &req.ChatCompletionNewParams)
```

### 4.2 复用既有处理器

`VisionProxyProcessor`（`internal/server/processor/vision_proxy.go`）的
`Process(pctx *ProcessorContext)` 完全复用：

- `pctx.Request`：typed 请求结构（原地改写，图片块 → 文本块）。
- `pctx.Services`：候选视觉上游池——这里就放场景配置的那**一个** service。
- 失败兜底（无可用 service / 描述失败 / 空响应）已在处理器内实现：图片仍
  会被剥离成 marker 文本，下游纯文本模型不会因不支持的 content block 报错。

处理器实例（`s.visionProxyProcessor`）在 `server.go` 启动时已构造（当前由
`processor.RegisterAll` 注册进 smart routing 的全局注册表）。本设计需要在
`Server` 上**额外持有一个引用**以便直接调用，注册表注册照旧保留给 smart
routing 路径。

### 4.3 去重：天然成立

两条路径**不会对同一请求重复描述**，且无需显式标记：

- 场景级 helper 在 `SelectService` **之前**运行，把图片块替换成文本块。
- smart routing 的 `proxy_vision` op 靠 `RequestContext.HasImage` 匹配
  （`internal/smart_routing/context.go` 的 `hasImageInBetaContent` 等）。
- 一旦图片已被替换为文本，重新提取的 `HasImage == false`，smart op 自然
  不再触发。

> 实操中 `RequestContext` 在 `SmartRoutingStage` 内重新提取，所以场景级先
> 跑完即可。为稳妥，实现时仍在 `gin.Context` 上打一个
> `vision_proxy_applied` 标记并在处理器入口 short-circuit，作为双保险。

---

## 5. 后端改动清单

| 文件 | 改动 |
|------|------|
| `internal/server/config/flag.go` | 定义 `VisionProxyServiceKey = "vision_proxy_service"` 常量（无 bool flag） |
| `internal/server/vision_proxy_scenario.go`（新增） | `applyScenarioVisionProxy` helper（以 service 是否存在为启用判据）+ `parseVisionProxyService(Extensions)` |
| `internal/server/server.go` | 在 `Server` 上持有 `visionProxyProcessor` 引用（`RegisterAll` 返回并赋值） |
| `internal/server/openai_chat.go` / `openai_responses.go` / `anthropic.go` | `SelectService` 前调用 `applyScenarioVisionProxy`（anthropic.go 一处统管 v1 + beta） |
| `internal/server/module/scenario/*` | 复用现有 `/scenario/:scenario`（config 整体存取 `Extensions`）端点；**无需新端点、无需 flag 端点**。如需 swagger 类型化，补 `VisionProxyService{Provider, Model}` 模型定义 |

### Swagger

视觉 service 经由已有的 `SetScenarioConfig`（写整个 `ScenarioConfig`，含
`Extensions`）读写，沿用既有 swagger 定义。若决定为视觉 service 提供
独立的类型化端点，按 CLAUDE.md 约定**先在后端定义 model 并补 swagger**：

```go
// 可选：类型化端点的请求/响应 model
type VisionProxyService struct {
    Provider string `json:"provider"`
    Model    string `json:"model"`
}
```

---

## 6. 前端改动清单

落点：`frontend/src/components/PluginFeatures.tsx`（场景 plugin 特性区，
经 `ProviderConfigCard` 在各 `Use*Page` 场景页面渲染）。

**核心：开关与选模型合并为单个控件**，不在 `PLUGIN_FEATURES` 通用 On/Off
列表里（那会产生"开关 + 独立模型按钮"的割裂）。

| 元素 | 实现 |
|------|------|
| 单一 Vision Proxy 控件 | `renderVisionProxyButton`：一个按钮，标签即所选模型（未配则 `Vision Proxy: Off`，灰色；配了则 `Vision Proxy: <model>`，蓝色高亮，tooltip 显示完整 `provider / model`）。点击先弹**下拉**（与 Thinking/Record 等同范式）：`Off`（直接清空=关闭）/ `On — <model>`（进 `ModelSelectDialog` 选/改模型）。**选模型即启用**，`Off` 一键关闭、无需打开大弹窗 |
| 持久化 | 选中/清除经 `getScenarioConfig` / `setScenarioConfig` 写 `Extensions["vision_proxy_service"]`；启用与否由该 service 是否存在驱动，**不调任何 flag 端点** |
| 类型定义 | `frontend/src/types` 里 `ScenarioConfig.extensions` 增加 `vision_proxy_service?: { provider: string; model: string }` 的形状提示 |

### API SDK（codegen）

按 CLAUDE.md：client api sdk 由 codegen 生成。若引入新的类型化端点，
前端先用 **placeholder 函数**占位，并提示用户后续用 swagger 重新生成。
若复用现有 `getScenarioConfig`/`setScenarioConfig`，则无新增 SDK。

### 图标

按 CLAUDE.md：UI 图标从 `@/components/icons` 取（Tabler→MUI），不直接从
`@mui/icons-material` 导入；缺失时用 `tablerMui(IconFoo)` 工厂。

---

## 7. 关键文件索引

| 功能 | 文件 |
|------|------|
| 处理器实现（复用） | `internal/server/processor/vision_proxy.go` |
| 处理器接口 / `ProcessorContext` | `internal/smart_routing/processor.go` |
| `ScenarioFlags` / `ScenarioConfig` | `internal/typ/type.go` |
| 场景配置 Get/Set | `internal/server/config/config.go` |
| 场景配置 API | `internal/server/module/scenario/{routes,handler,types}.go` |
| `loadbalance.Service` | `internal/loadbalance/load_balancing.go` |
| 入站 handler（钩子点） | `internal/server/{openai_chat,openai_responses,anthropic}.go` |
| 场景 plugin UI | `frontend/src/components/PluginFeatures.tsx` |
| 场景页面容器 | `frontend/src/components/ProviderConfigCard.tsx`、`frontend/src/pages/scenario/Use*Page.tsx` |

---

## 8. 测试

| 层 | 用例 |
|----|------|
| `parseVisionProxyService` 单测 | nil/缺键/结构错/缺 provider/缺 model/空串 → nil（即关闭）；provider+model 齐备 → 返回 active service（启用） |
| `applyScenarioVisionProxy` | 无 service → no-op；有 service + 有图 → 图片块被替换为文本 |
| 去重 | 场景级处理后 `HasImage == false`，smart routing `proxy_vision` 不再触发；`vision_proxy_applied` 标记 short-circuit |
| 三种请求形态 | Beta / V1 Anthropic、OpenAI ChatCompletion 各覆盖（复用 `vision_proxy_test.go` 的夹具） |
| 场景隔离 | `claude_code` 配 service A、`codex` 配 service B，各自请求用各自的 service |
| 配置往返 | `SetScenarioConfig` 存 `vision_proxy_service` → `GetScenarioConfig` 读回结构一致 |
</content>
</invoke>
