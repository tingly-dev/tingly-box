# Harness Relay 拓扑与请求流程 (pencil)

[`harness-relay.md`](./harness-relay.md) 的配套图：harness 怎样隔着一个第三方网关，
用 tb1 的 vmodel 作上游跑 matrix。§2 已实现，其余是提案；图与正文不一致时以正文为准。

图例：`▶` 入口 · `◇` 分支 · `✖` 跳过/失败 · `⎘` 记录

## 1. 三方拓扑

```
  ┌──────────────────────┐   源协议请求       ┌──────────────────────┐   目标协议请求      ┌──────────────────────────────┐
  │ harness              │  model=harness-…   │ 被测网关              │  model=harness-…    │ tb1  tingly-box               │
  │ 客户端 + 断言         ├──────────────────▶│ 任意第三方中转        ├───────────────────▶│ --harness-upstream            │
  │ --gateway profile    │◀──────────────────┤ (协议转换/路由/缓存)   │◀───────────────────┤ /virtual/{anthropic,openai}/v1│
  └─────────┬────────────┘   转换后的响应      └──────────────────────┘   场景 fixture 响应  └──────────────┬───────────────┘
            │                                                                                     ⎘ 记录收到的原始请求
            └──────────────── GET /virtual/captures?id=… ◀──────────────────────────────────────────┘
                              （只读，上游侧断言用）
```

对照现状（duo）：

```
  harness ──▶ tb2（tingly-box 网关）──▶ tb1 /virtual/...        现在
  harness ──▶ 被测网关               ──▶ tb1 /virtual/...        本提案：只换中间一格
```

## 2. tb1 启动：测试模型怎样进 `/virtual`

```
  ▶ tingly-box start [--harness-upstream]
   RegisterDefaults(anthropic, openai)            面向用户的 demo 模型，始终注册
   ◇ --harness-upstream？                          否 → 到此为止，生产实例看不到任何 harness-*
   harnessupstream.New()：scenario.AllScenarios() 改名 harness-<name>，注册进 benchmark 场景服务
   /virtual/{openai,anthropic} 路由组：model auth → Intercept → vmodel handler
   挂载 GET / DELETE /virtual/captures

  一次 POST 经过 Intercept：
   ⎘ 记录（任何模型）
   ◇ 路径是 chat/completions · responses · messages，且模型是 harness-*？
       是 → 场景服务回原始 fixture（流式 / 非流式 / 错误状态），Abort
       否 → c.Next()，普通 vmodel 照常处理
```

| 路由 | 开关关 | 开关开 |
|---|---|---|
| `/virtual/anthropic/v1/messages` | demo 模型 | demo 模型 + `harness-*` |
| `/virtual/openai/v1/chat/completions` | demo 模型 | demo 模型 + `harness-*` |
| `/virtual/openai/v1/responses` | demo 模型 | demo 模型 + `harness-*`（场景自己的 Responses fixture） |
| `/virtual/*/v1/models` | demo 模型 | 不变（不列出 `harness-*`，见正文 §6） |
| `/virtual/captures` | 404 | 返回收到的请求 |

## 3. 一条用例的走向

```
  ▶ harness matrix --gateway relay.yaml --scenario tool_use --source anthropic_v1 --target openai_chat
   ◇ section 可移植？                                         否 → ✖ skipped（写明原因）
   生成 request-id；请求体 model = profile.routes[openai_chat]
   POST {base_url}{paths[anthropic_v1]}   头 X-Harness-Request-Id
        │
        ▼  被测网关：anthropic → openai chat，转发到 tb1
   tb1 /virtual/openai/v1/chat/completions
   ⎘ 按 request-id（无则按模型名）记录原始请求
   ◇ 模型名命中 harness-tool_use？                            否 → 请求失败，报"中转改写了模型名"
   按场景 fixture 回响应（流式/非流式）
        │
        ▼  被测网关：openai chat → anthropic，回给 harness
   客户端侧断言：内容 · 角色 · tool call · usage · finish reason · 流事件
   ◇ 该 section 需要上游侧断言？                               否 → 结束
   GET tb1 /virtual/captures?id=<request-id>
   上游侧断言：请求形状 · cache_control · 前缀稳定性
```

## 4. 关联键回退

```
  ◇ tb1 收到 X-Harness-Request-Id？     是 → 精确匹配
  ◇ 模型名保留为 harness-<scenario>？   是 → 按模型名取最近一条
  否则                                  → 只能单场景串行跑，取最近一条
```
