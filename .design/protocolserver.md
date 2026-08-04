# ProtocolServer — 模型服务面从 internal/server 独立

Status: steps 1–5 done + 后续进一步提升（middleware / routing 到 top-level，affinity 并入根）
Date: 2026-08-04

## 动机

`internal/server`（~22.7k 行 + 子包）杂糅了三块职责：

1. **模型服务面**（最重）：`/tingly/:scenario` 协议网关 — protocol dispatch、openai/anthropic 端点、
   MCP in-gateway、failover、load balance 引擎、guardrails 运行时求值、recording、usage tracking。
2. **管理面**：`/api/v1` WebUI Management API — server_control、webui_api、guardrails_handler(2310 行)、
   status/log/token/model_request handler、oauth、`module/*`。
3. **共享底座**：Server 构造/生命周期/路由/中间件、`config`。

一个 gin engine、一个端口；`getModelAuthMiddleware` 管 `/tingly`，`getUserAuthMiddleware` 管
`/api/v1` — 物理上一台服务器，逻辑上两个信任域，但代码上完全平铺在一个包里。

## 目标形态

`internal/protocolserver` 是一个**自包含的库**（不是独立进程）：拥有自己的状态与路由注册函数，
由 `internal/server` 构造并挂载到它的 gin engine 上。

```
internal/protocolserver/
├── handler.go            # ProtocolHandler + Deps（原 protocol_handler.go，包入口）
├── dispatch / cross / passthrough / transform / clone / roundtrip / 1m
├── openai_* / anthropic_* / mcp_*
├── failover_dispatch / load_balance（选路引擎）
├── guardrails_runtime* / recording_transform / usage_tracking / tracking_context
├── routes.go             # RegisterRoutes(...) ← 原 UseAIEndpoints
└── 子包整体迁入: forwarding/ recording/ transform/ servertool/ advisortool/
```

**后续进一步提升**（steps 1–5 之后，2026-08-04）：

- **middleware**：`internal/server/middleware` → top-level `internal/middleware`（#1494）。原本
  protocolserver 与其它非 server 包要用中间件只能反向依赖 server；提升后三方（server /
  protocolserver / handler）作为 peer 引用，依赖方向理顺。顺带订正了 `doc.go`（ring-buffer 500→50、
  补 routing 字段、修正 `UserAuthMiddleware` / `RateLimit` 描述、补 Gzip）。
- **routing**：`internal/protocolserver/routing` → top-level `internal/routing`（5f1ef829）。选路引擎
  pipeline（selector / 各 stage）独立成包，server 与 protocolserver 共用。
- **affinity**：`internal/protocolserver/affinity/affinity.go` → `internal/protocolserver/affinity.go`
  并入根（`package protocolserver`，5fe2ab7a）。它本就只服务 serving 侧、且依赖 protocolserver
  内部状态，没必要单独成包，故直接并回。

### 依赖方向（拆完后必须单向）

```
server → protocolserver
server → module/*
server, protocolserver → middleware / routing   （共享底座，top-level peer）
module/* → protocolserver   （仅接口 / helper，如 forwarding、rule flags）
protocolserver ✗ server / module/*   （禁止反向引用）
```

外部调用方（cli/harness、gui/wails3、internal/command、protocoltest、servertest、vmodel 测试）
全部只走 `*server.Server` 门面（`NewServer` + `With*` options + lifecycle 方法），不碰内部
handler 类型 — **拆分对外部 API 零破坏**。

## 所有权划分

| 状态 | 归属 | 理由 |
|---|---|---|
| `routingSelector`、affinity store、recording sinks、`servertoolPipeline`、`tokenTracker`、usage tracking | protocolserver 拥有 | serving-only，管理面不碰 |
| `config`、`clientPool`、`templateManager`、`mcpRuntime`、`healthMonitor` | server 拥有，构造时注入 | 双方共用，server 是装配根 |
| `LoadBalancer` | protocolserver 拥有，暴露 getter | 选路引擎在 serving 侧；管理面 `LoadBalancerAPI` 已用窄接口 `LoadBalancerEngine`，消费方向正好反转。选路 pipeline 本身已下沉为 top-level `internal/routing`，server 与 protocolserver 共用 |
| `guardrailsRuntime` | protocolserver 拥有 | 请求时求值在 serving 侧；`guardrails_handler.go` 已只通过 4 个 adapter 方法访问（`GuardrailsRuntime` 接口），实现者从 `*Server` 换成 protocolserver 侧即可 |

## 四个耦合点及处理

1. **热重载回调**。`ProtocolHandlerDeps` 里 `GetServertoolPipeline`、`CurrentGuardrailsRuntime`、
   `GetOrCreateScenarioSink` 等是回调，因为对应字段在 config reload 时被 `*Server` 原地替换。
   状态所有权移入 protocolserver 后回调可消掉：protocolserver 暴露 `Reload(cfg)`（或自行订阅
   config watcher），server 在 `watcher.AddCallback` 中转发。这是最能减复杂度的一步。
2. **共享 helper**：`rule_flags.go`、`scenario.go`、`model_list_helper.go`、`user_agent.go`、
   `server_flags.go` 双方都用。语义重心在 serving，随迁并导出；若引出循环依赖再下沉独立小包。
3. **`module/mcp/forwarder.go`** 是唯一引用 `forwarding/` 的管理面代码 — forwarding 迁走后它
   改 import `protocolserver/forwarding`，方向正确。
4. **历史 import cycle**（`token_handler.go` 注释：root server 已 import webui）。拆分后按上述
   单向依赖固定，可后续加 depguard 守住。

## 迁移路径（每步独立可编译、独立提交）

重构原则：**先 mv 再改**，不重写。用 `tingly-go refactor move`（自动更新引用）做 package/file
级迁移，逻辑变更单独成步。

1. **迁纯 serving 子包**：`forwarding/ recording/ transform/ servertool/
   advisortool/` → `internal/protocolserver/...`。纯 import 路径变更，零逻辑改动。
   （`routing/` 后来进一步提到 top-level `internal/routing`；`affinity/` 并回 protocolserver 根。）
2. **迁根文件**：protocol_* / openai_* / anthropic_* / mcp_* / failover_dispatch /
   load_balance（引擎与 simulator）/ guardrails_runtime* / recording_transform /
   usage_tracking / tracking_context / server_affinity + 共享 helper。
   `ProtocolHandlerDeps` 即 `protocolserver.Deps`。未导出符号跨包引用处按需导出（mv 后最小修补）。
3. **反转状态所有权**：LoadBalancer / guardrailsRuntime / affinity / recording sinks /
   servertoolPipeline 的构造移入 protocolserver；server 改经 getter/接口访问；消掉热重载回调，
   代之以 `Reload`。
4. **路由收口**：`UseAIEndpoints` → `protocolserver.RegisterRoutes(engine, modelAuthMW, ...)`，
   server 只负责装配和挂载。

测试文件随对应源文件走；`protocoltest` / `servertest` 走 `*Server` 门面，不受影响。

## 遗留 / 后续

- `load_balance_handler.go`（`LoadBalancerAPI`，管理面 HTTP 包装）与 `guardrails_handler.go`
  留在 server 侧（管理面），只消费 protocolserver 暴露的接口。
- **`internal/mcpserver`**（Step 5，2026-08-04）：MCP 转换引擎（adapters/forwarder/
  loop processor/stream interceptor/tool executor/continuation store，~3.6k 行）已从
  `server/module/mcp` 独立；管理面（handler/routes，/api/v1/mcp CRUD）留在 module/mcp。
  `protocolserver → server/module/*` 反向依赖清零。注意 mcpserver 引用
  `protocolserver/forwarding`（context provider 接口）——方向可接受，若要彻底解耦可将
  ForwardContext 下沉。
- LB 模拟器（`load_balance_simulator.go`）与 serving 侧测试中若干仍构造 `&Server{}`
  的用例留在 server 包（它们需要 unexported failover 入口经 aiHandler 走通）。
- 拆完后可为依赖方向加 lint 规则（depguard / go vet 自定义）。middleware / routing 已提升为
  top-level peer，方向已天然单向，depguard 主要用来守住 `protocolserver ✗ server/module/*` 这条线。
- `swagger.go` 的 `GenerateOpenAPI` 只走管理面路由，不涉及 `/tingly`，不受影响。
- 热重载：`servertoolPipeline` 仍经 `GetServertoolPipeline` 回调（config reload 原地重建
  pipeline 的逻辑仍在 server 的 `registerAdviserFromConfig`）；如需彻底消掉，可把 adviser
  注册也移入 protocolserver 并暴露 `Reload(cfg)`。
