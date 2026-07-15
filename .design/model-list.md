# Model List 获取 / 缓存 / 兜底 设计

> 适用对象：改 `internal/server/module/provider/handler.go`（`GetProviderModelsByUUID` / `UpdateProviderModelsByUUID`）、`internal/server/config/config.go`（`FetchAndSaveProviderModels`）、`internal/data/model_list.go`、`internal/data/db/provider_model.go`、`internal/data/provider_template.go` 的贡献者。
> 本文档描述「前端请求某 provider 的模型列表 → gateway 返回」的取数、缓存与兜底最终设计。

---

## 1. 问题域

前端需要展示每个 provider 可用的模型列表。数据可能有四个来源，优先级从高到低：

| 优先级 | 来源 | 说明 | 是否权威 |
|---|---|---|:---:|
| 1 | **DB 缓存** | 上一次成功 fetch 的上游列表，持久化在 SQLite，`api` 来源，TTL 1h | 是（快照） |
| 2 | **VModel 静态列表** | 虚拟 provider 把模型列表存在 provider 记录本身（`VModelDetail.Models`） | 是 |
| 3 | **上游 Provider API** | 实时调 `GET /v1/models`（或等价接口） | 是 |
| 4 | **内嵌模板（兜底）** | 编译期 `//go:embed providers.json` 的快照 | **否，会过期** |

关键张力：**来源 4（模板）是编译期快照，会随上游淘汰模型而过期**。它只能作为"实在拿不到任何真实数据"时的最后兜底，绝不能当作补充数据并入真实列表。

---

## 2. 相关代码地图

| 职责 | 位置 |
|---|---|
| **兜底链唯一真源（resolver）** | `Config.ResolveProviderModels(forceRefresh, uid)` — `internal/server/config/config.go` |
| 上游 API fetch + 持久化（内部） | `Config.fetchAndSaveAPIModels` — `config.go` |
| 缓存预热薄封装（不返回列表） | `Config.FetchAndSaveProviderModels` — `config.go` |
| 服务端点（读路径） | `GetProviderModelsByUUID` — `internal/server/module/provider/handler.go` |
| 刷新端点（写路径） | `UpdateProviderModelsByUUID` — `handler.go` |
| 排序（serving 边界唯一真源） | `SortProviderModels` — `config.go` |
| 缓存 manager（TTL=1h） | `ModelListManager` / `ModelCacheTTL` — `internal/data/model_list.go:14` |
| 存储后端（SQLite/GORM） | `ModelStore` — `internal/data/db/provider_model.go`（PK 仅 `provider_uuid`） |
| 内嵌模板兜底 | `TemplateManager.GetEmbeddedModelsForProvider` — `internal/data/provider_template.go` |
| 响应里的来源标记 | `ModelCacheSource*` — `internal/server/module/provider/types.go` |

---

## 3. 兜底链最终设计（`ResolveProviderModels`）

**整条兜底链收敛在一个函数里**：`Config.ResolveProviderModels(forceRefresh bool, uid string) (ResolvedModels, error)`，返回 `{Models, Source, LastUpdated}`。所有调用方（HTTP 读、HTTP 刷新、CLI、OAuth 完成）都走它，行为完全一致。

```
Step 1  DB 缓存（1h TTL；forceRefresh=true 时跳过）   非空 → 返回，source=db
Step 2  VModel 静态列表（虚拟 provider）              → 返回，source=vmodel（永不过期）
Step 3  上游 API fetch（fetchAndSaveAPIModels，持久化） 成功非空 → 返回，source=api
Step 4  内嵌模板兜底（live，不落库）                  非空 → 返回，source=template
```

- **`forceRefresh` 是读路径与刷新路径的唯一区别**：读传 `false`（缓存优先），手动刷新传 `true`（跳过缓存、强制重查上游）。**只差一个 bool，两条路径再也无法漂移**（Bug #3 类问题从结构上消除）。
- Step 4 三条硬约束：①仅当前面全空才用；②绝不并入非空列表（模板是会过期的编译期快照，并入会复活已淘汰模型）；③live 读取不落库（改进内嵌列表立即生效）。
- 两个 HTTP handler 因此变得很薄——一行 `ResolveProviderModels(...)` + 组装响应。`expiresAt`（仅响应元数据）由 handler 的 `modelListExpiry(source)` 从 source 推导。

`ModelCacheSource` / `ModelListSource` 只有四个取值：`db` / `api` / `vmodel` / `template`。**没有 `merged`。**

> **`FetchAndSaveProviderModels` 仍保留**，但降级为"只预热缓存、不返回列表"的薄封装，供纯触发抓取的调用点使用（TUI 缓存预热、OAuth 重认证）。需要"生效列表"的调用点一律用 `ResolveProviderModels`。
>
> **TUI `availableModels`（`rule_mode.go`）刻意不并入 resolver**：它是渲染期"只读缓存+模板、绝不发网络"的语义，与 resolver"缓存缺失即抓取"不同。

---

## 4. 历史 Bug 与教训

### 4.1 Bug #1：兜底列表无法快速生效

**现象**：改进了内嵌 `providers.json` 并重新编译后，某些 provider 的模型列表要等最多 1 小时才更新。

**根因**：老代码在 Step 3 命中模板后，会用 `SaveModels(..., ModelSourceTemplate)` **把模板结果写进 DB，带 1h TTL**。于是：
- Step 1 的 DB 缓存检查会命中这条模板行（且来源被误标为 `db`），在 TTL 内一直返回旧快照；
- 也挡住了上游 API 的重试——即使上游恢复了，也要等缓存过期。

**修复**：**模板结果不再落库，每次列表为空时实时读取。** 改进内嵌列表后立即生效；上游一旦恢复，下次请求的 Step 2b 就会取到真实数据。此修复保留至今。

### 4.2 Bug #2（曾尝试的危险修复，已回退）：把模板并入非空列表

**设想的现象**：上游被代理/网关拦截，返回一份 HTTP 200 但**不完整**的模型列表，用户预配置的模型无法融入。

**曾经的错误修复**：在读路径和写路径都做 `union(上游列表, 模板列表)`，即使上游列表非空也把模板并进去（引入过 `MergeModelLists` / `MergeTemplateModels` helper 和 `merged` 来源）。

**为什么危险**（回退原因）：
- 模板是编译期快照，会随上游淘汰模型而过期。上游返回一份**健康、完整、权威**的列表时（同样是 200 + 非空），merge 会把模板里**已被上游下线的模型"复活"**。
- "上游被拦截返回不完整" 和 "上游正常返回完整" 两种情况**都是 200 + 非空**，代码无法可靠区分。只要保留"并入非空列表"的行为，正常情况一定会误伤。
- 写路径的 merge 还会把"模板增强过的列表"以 `api` 来源落库，污染缓存。

**结论**：**模板绝不并入非空真实列表；上游非空即视为对增删都权威，原样透传。** 相关 helper、`merged` 来源常量、合并测试全部移除（PR 净删 111 行）。

**安全边界由测试钉死**：`TestGetProviderModelsByUUID_ClaudeCode_NonEmptyCache_NotPollutedByTemplate` —— 缓存里一份缺了 `claude-opus-4-5` 的上游列表，接口原样返回，**不会**把模板里的该模型补回来。

### 4.3 Bug #3：Codex 手动刷新返回空列表

**现象**：Codex（issuer `codex`）provider 点"刷新模型列表"后返回 0 个模型，把原本能正常展示的列表清空。

**根因**：`UpdateProviderModelsByUUID`（刷新端点）只做两件事——`FetchAndSaveProviderModels` + `GetModels`（读 DB）。对 Codex：
- Codex 的 `/models` 端点不支持，`OpenAIClient.ListModels` 直接短路返回 `ErrModelsEndpointNotSupported`（`internal/client/openai.go`，守卫用 `GetIssuer()` 且额外判 `APIBase == CodexAPIBase`）；
- `FetchAndSaveProviderModels` 清掉该错误、命中模板后 `return nil`（成功）**但不落库**；
- 于是 `GetModels` 读 DB 得到空 → 刷新端点返回 `models_count: 0`。

读路径（GET）没这个问题，因为它的 Step 3 会**实时**读模板兜底；但刷新路径当初漏了这一步。

> 排查中一个被证伪的岔路：曾怀疑是"旧 provider 记录只设了 `OAuthDetail.ProviderType`、`Issuer` 为空"导致模板匹配（用裸 `.Issuer`）失败。但 provider store 存/取时会用 `GetIssuer()` 归一化（`provider_store.go` 存 `OAuthProviderType = GetIssuer()`、取时写回 `Issuer`），任何经过存储的记录 `Issuer` 都已补齐，所以这条不成立。**先复现再修**避免了误修。

**修复**：最初把模板兜底抽成 `Handler.templateFallbackModels` 让两个 handler 共用（PR #1364）；随后**全面重构**把整条兜底链收敛进 `Config.ResolveProviderModels`（见 §3），两个 handler 只差一个 `forceRefresh` bool，`templateFallbackModels` 被移除。回归测试 `TestUpdateProviderModelsByUUID_Codex_ReturnsTemplateModels`。

> 重构同时修掉了 **3 个潜伏的同类 bug**：`agent_command.go`、`remote.go`、`oauth/handler.go` 当初都是 `FetchAndSave` + `GetModels` 且没补模板兜底，codex 场景同样会拿到空列表；迁到 `ResolveProviderModels` 后一并修复。

---

## 5. 如果 Bug #2 是真实场景怎么办

若日后确认"上游返回不完整、需要补齐"确为真实需求，正确解法**不是**并入厂商模板快照，而是：

- **只融合用户显式预配置的自定义模型**（用户意图，不存在"被上游淘汰"一说，补回来天经地义）。
- 前提：需要一个**服务端** custom-model 存储。目前 `CustomModel` 概念只存在于前端 localStorage（`frontend/src/hooks/useCustomModels`），后端 `ProviderModelInfo.CustomModel` 字段从未被写入。

在此之前，模板保持"纯兜底、绝不 merge"。

---

## 6. 缓存 TTL 速查

| 缓存 | 常量 | TTL | 位置 | 说明 |
|---|---|---|---|---|
| provider 模型列表（DB） | `ModelCacheTTL` | 1h | `internal/data/model_list.go:14` | 只缓存 `api`/`vmodel` 来源，**不缓存模板** |
| 模板注册表 GitHub 同步 | `DefaultTemplateCacheTTL` | 12h | `internal/data/provider_template.go:26` | 与内嵌兜底**是两条独立路径**；`GetEmbeddedModelsForProvider` 从不读它 |

> 注意：内嵌兜底（`GetEmbeddedModelsForProvider`）走的是编译期 `//go:embed` 的 `tm.embedded`，纯内存查表、不落盘、不发网络请求，与 12h 的 GitHub 模板同步缓存无关。

---

## 7. UX 检查（对照 `.design/ux-principles.md`）

- **显示具体值而非别名**：响应带 `source`（`db`/`api`/`vmodel`/`template`）+ `expiresAt` + `lastUpdated`，前端可如实告诉用户"这份列表从哪来、何时过期"，而不是一个不透明的黑盒。
- **诊断走真实路径**：读路径的兜底链就是真实取数链；`UpdateProviderModelsByUUID` 手动刷新走的也是同一套 `FetchAndSaveProviderModels`，不存在"诊断用假路径"。
- **smart default 而非 toggle**：无需用户在"用缓存/用实时/用兜底"之间选模式，四级兜底自动决策。
