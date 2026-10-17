# Harness Relay — 用 harness 测任意中转网关（提案）

> 状态：**第 1 阶段（tb1 侧）已实现**（`vmodel/harnessupstream`，`tingly-box start --harness-upstream`）；
> 第 2、3 阶段（harness 侧）仍是提案。
> 流程图见 [`harness-relay.pencil.md`](./harness-relay.pencil.md)，本文是契约与取舍的权威来源。
>
> 相关：[`harness-matrix.md`](./harness-matrix.md)（被移植的套件）、
> [`harness-duo.md`](./harness-duo.md)（复用的双进程拓扑）、
> [`vmodel.md`](./vmodel.md)（注册纪律）。

## 1. 要解决的问题

harness matrix 目前只能测 tingly-box 自己。被测网关是进程内起的
（`internal/protocoltest/testenv.go` `newGatewayCore`），路由靠直接调
`appConfig.AddProvider` / `AddRequestConfig` 生成（`testenv.go` `setupRouteCore`），
请求路径写死成 `/tingly/...`（`testenv.go` `buildRequest`）。

断言层本来就不依赖 tingly-box：客户端侧只看 HTTP 响应（`RoundTripResult`），
上游侧只看上游收到的请求（`VirtualServer.LastRequest`）。所以只要换掉"被测网关"
和"上游"这两端，同一套断言就能测任意 LLM 中转。

## 2. 拓扑

```
harness（客户端：发请求 + 断言）→ 被测网关（任意第三方）→ tb1（tingly-box，vmodel 作上游）
```

就是 duo 的拓扑（tb2 网关 → tb1 `/virtual/...`），只是把 tb2 换成外部网关。
tb1 是真实可部署的 tingly-box 进程，所以既能由 harness 拉起，也能由第三方单独起一个，
把自己的中转对接上去。

## 3. tb1 侧：测试模型直接挂在 `/virtual`

| 决定 | 内容 | 理由 |
|---|---|---|
| 不新开路由前缀 | 场景挂在现有的 `/virtual/anthropic/v1/messages`、`/virtual/openai/v1/{chat/completions,responses}` 上 | vmodel 本来就是做虚拟上游的，harness 只是它的一种用途；中转对接方式与普通虚拟模型完全一样 |
| 启动开关 | `tingly-box start --harness-upstream` 打开时才挂载（`server.WithHarnessUpstream`） | `vmodel/README.md` 的注册纪律：`RegisterDefaults` 里的东西对生产终端用户可见，测试夹具必须 opt-in。先例是 duo 的 `registerDuoStreamModels` |
| 模型名前缀 | 场景 `text` → 模型 `harness-text` | 场景名太通用，避免和现有模型撞名 |
| 原样回放 fixture，不做适配 | `/virtual` 两个路由组前加一层中间件：模型是 `harness-*` 就交给 `benchmark.NewScenarioServer()` 回场景的原始线上 fixture 并中止；其他模型照常进 vmodel handler | 场景 fixture 是逐协议手写的原始字节（含 Responses、thinking、错误、多轮 `StreamFor`），翻译成 vmodel 的类型化响应会丢细节。原样回放与进程内 matrix 用的是同一份字节 |
| 请求记录 | `GET /virtual/captures?request_id=&model=&limit=`（新到旧，默认 20 条）、`DELETE` 清空；只记录 `/virtual` 的 POST，**包括非 harness 模型**，最多 512 条；`Authorization` / `X-Api-Key` / `Api-Key` 替换成 `[redacted]` | content_shapes、cache_controls、cache_prefix、thinking_limits 断言的是"上游收到了什么"，tb1 独立进程后不能再同进程读 |
| 鉴权 | 沿用 `/virtual` 现有的 model token 中间件（`server_routes.go` `UseVirtualModelEndpoints`） | 中转那边按普通上游填 key |

### 关联键

一条请求经过中转后，harness 要在 tb1 找到它。优先顺序：

1. 请求头 `X-Harness-Request-Id`（中转透传时最精确）。
2. 模型名（中转丢头但保留模型名时）。
3. 都没有：同一时刻只跑一个场景，取最近一条。

## 4. 各套件能否移植

| 套件 (`--mode`) | 外部网关可用 | 说明 |
|---|---|---|
| single（A→B 单跳） | ✅ | 纯黑盒 |
| content_shapes | ✅ | 依赖 `/virtual/captures` |
| cache_controls / cache_prefix | ✅ | 依赖 `/virtual/captures`；中转最容易出问题、也最费钱的地方 |
| replay / agent | ✅ | 只需要 base URL + key |
| idempotent / transitive（多跳） | ⚠️ | 中转要能把上游指回自己或另一个实例 |
| thinking_limits | ⚠️ | 部分用例依赖 tingly 的 rule flag |
| flags / flag_paths / servertool / vendor | ❌ | 测的是 tingly 自有功能 |
| routing / lb / duo memory | ❌ | 依赖 tingly 的规则 API、`/api/v1/requests`、`/api/v1/debug` |

不可移植的 section 在外部模式下报 skipped 并写明原因（`cli/harness/matrix.go`
`matrixSections` 加一个 `portable` 标记）。

## 5. harness 侧：被测网关 profile

```yaml
name: some-relay
base_url: http://127.0.0.1:3000
api_key: sk-xxx
paths:              # 每种源协议的入口路径；默认用官方路径
  anthropic_v1: /v1/messages
  openai_chat: /v1/chat/completions
  openai_responses: /v1/responses
routes:             # 目标协议 → 发给中转的模型名（中转里配成转到 tb1 对应端点）
  anthropic_v1: harness-anthropic
  openai_chat: harness-openai-chat
```

`harness matrix --gateway profile.yaml`。`TestEnv` 改为持有一个 `Gateway` 接口
（`BaseURL` / `APIKey` / `PathFor` / `RouteModel` / `SupportsTarget`），现有的进程内
实现收进去，行为不变。

## 6. 已知取舍与待决

- **`/models` 不列出 `harness-*`。** 中间件只拦 POST；模型清单由 virtualserver 生成。需要
  "拉取模型列表"才能配置的中转，第一版要手填模型名。
- **记录非 harness 模型的请求是刻意的。** 中转改写了模型名时，请求会落到普通 vmodel 上，
  这时 capture 正是暴露问题的地方。
- **错误场景** 直接沿用 fixture 的状态码和中途断流（`StreamHTTPError`、`error-midstream-close`），无需额外表达。
- **模型名被改写。** 很多中转会把模型名重写成固定上游模型，此时只能靠请求头或单场景运行。
- **"正确"的定义。** 部分期望值带有 tingly 的取舍（usage 拆分、finish reason 映射）。
  外部模式下是否降级为 warning，待定。
- **轻量模式保留。** 上游在 harness 进程内跑（现状）仍是本地和 CI 的默认；外部模式按需使用。
- Google 目标不覆盖（vmodel 端点本来就跳过它）。

## 7. 分阶段

1. ✅ tb1 侧：开关 + `harness-*` 场景回放 + `/virtual/captures`（`internal/server/harness_upstream_test.go`）。验收：现有 duo 拓扑下，
   tb2 指向 `harness-*` 模型跑 single，结果与进程内一致。
2. harness 侧：`Gateway` 接口重构（零行为变化）+ `--gateway` profile + 可移植 section。
   验收：把 tingly-box 自己当外部网关跑一遍。
3. replay / agent 接 profile；多跳在 profile 声明支持时开启；常见中转的配置清单。
