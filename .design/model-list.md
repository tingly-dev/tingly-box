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
| 服务端点（读路径） | `GetProviderModelsByUUID` — `internal/server/module/provider/handler.go:454` |
| 刷新端点（写路径） | `UpdateProviderModelsByUUID` — `handler.go:398` |
| Fetch + 持久化逻辑 | `Config.FetchAndSaveProviderModels` — `internal/server/config/config.go:2006` |
| 排序（serving 边界唯一真源） | `SortProviderModels` — `config.go:2126` |
| 缓存 manager（TTL=1h） | `ModelListManager` / `ModelCacheTTL` — `internal/data/model_list.go:14` |
| 存储后端（SQLite/GORM） | `ModelStore` — `internal/data/db/provider_model.go`（PK 仅 `provider_uuid`） |
| 内嵌模板兜底 | `TemplateManager.GetEmbeddedModelsForProvider` — `internal/data/provider_template.go:686` |
| 响应里的来源标记 | `ModelCacheSource*` — `internal/server/module/provider/types.go` |

---

## 3. 读路径最终设计（`GetProviderModelsByUUID`）

分四步，逐级兜底。**模板只在前三步全部拿不到东西（列表为空）时才用。**

```
Step 1  DB 缓存（api 来源，1h TTL）           非空 → 直接返回，source=db
Step 2a VModel 静态列表（虚拟 provider）       非空 → 返回，source=vmodel（永不过期）
Step 2b 上游 API 实时 fetch（非虚拟）          成功非空 → 返回，source=api
Step 3  内嵌模板兜底（仅当以上全空）           非空 → 返回，source=template
```

Step 3 的三条硬性约束（`handler.go` 内注释同步）：

1. **仅当 `len(models) == 0` 时才用模板** —— 绝不并入非空列表。
2. **实时读取，不持久化** —— 不写回 DB。
3. 命中模板时 `expiresAt` 设为 1h 后（仅用于响应元数据，不代表落库）。

`ModelCacheSource` 只有四个取值：`db` / `api` / `vmodel` / `template`。**没有 `merged`。**

---

## 4. 两个历史 Bug 与教训

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
