# Team and Sharing Key Authorization

> Status: **implemented decision** · Date: 2026-08-19
>
> Scope: Team identity, Team-scoped routing, Sharing Key authorization,
> lifecycle and migration. This document is the source of truth for future
> changes to Team or Sharing Key behavior.

## 1. Decision summary

**Sharing Key 是某一个 Team 的模型访问凭证，不是用户的通用 API Key。**

- 每个 Sharing Key 在任意时刻必须且只能绑定一个 Team。
- Sharing Key 只允许访问 `/tingly/team` 与 `/tingly/team/v1` 模型表面。
- Key 对应的 Team 由服务端从已认证 token 记录中取得，客户端不能选择 Team。
- Key 不能访问其他 Team、其他 `/tingly/*` scenario、`/virtual/*` 或管理 API。
- 管理员可以把 Key 原子地移动到另一个已启用 Team；移动后旧 Team 权限立即失效。
- Team 停用后，其所有 Sharing Key 立即失效；重新启用后恢复。
- 不提供 Global Sharing Key、Legacy Global Key 或逐 Key scope picker。
- 全局 model token 是另一类已有凭证，继续承担跨 scenario 的管理者级模型访问。

用户量有限，因此选择一次性收紧旧语义，不引入永久兼容分支。现存 Sharing Key
自动归入 `default` Team，并从升级后开始遵守相同的 Team 范围限制。

## 2. Why this model

“属于 Team 的 Sharing Key”如果还能访问任意 scenario，会造成权限错觉：用户看到的是
Team 归属，实际拿到的却是全局模型权限。它还会使 Team 停用、Key 移动、用量归属和审计
记录无法从资源关系直接推导。

本设计坚持三个不变量：

1. **名称与权限一致**：Team Key 只能代表一个 Team。
2. **最小权限**：泄漏一个 Sharing Key 的影响面不超过其 Team。
3. **服务端决定范围**：请求路径或参数不能扩大 token 已认证的 Team 身份。

不保留 Legacy Global Key，是因为用户规模尚小，迁移成本低；增加第二套 Sharing Key
语义会永久增加鉴权分支、UI 概念和测试矩阵。

## 3. Vocabulary

| Term | Definition | Must not mean |
|---|---|---|
| **Team** | Sharing Key、Team rules 与 usage 的授权和路由边界 | 用户组织或通用 RBAC role |
| **Sharing Key** | 绑定一个 Team 的模型访问凭证，前缀为 `tb-share-` | 通用 model token、管理 API token |
| **Global model token** | 实例级模型凭证，可访问既有模型表面 | Team Key |
| **Team ID** | 持久 UUID；用于 token、rules、usage 和鉴权上下文 | 用户可编辑标识 |
| **Team slug** | 系统生成的可见编号，如 `t1`、`t2` | 授权主键、用户自定义字段 |
| **Team name** | 用户可编辑的显示名称 | 路由或鉴权标识 |

## 4. Team identity

默认 Team 使用稳定身份：

```text
ID:   00000000-0000-0000-0000-000000000001
slug: default
name: Default
```

新增 Team 的 slug 复用 Profile 编号模式：系统分配最小可用编号 `t1`、`t2`……；
删除 Team 后编号可以复用。用户只能修改 name，不能创建或修改 slug。

授权、routing 和 usage 永远保存不可复用的 UUID，不保存 slug。因而即使删除旧 `t2`
后新 Team 再获得 `t2`，旧规则、Key 或审计记录也不会被错误关联到新 Team。

## 5. Access matrix

| Credential | `/tingly/team[/v1]` | Other `/tingly/*` | `/virtual/*` | Management API |
|---|---:|---:|---:|---:|
| Sharing Key | Allow, bound Team only | Deny | Deny | Deny |
| Global model token | Allow, default Team | Allow | Allow | Not its responsibility |
| User/control-plane auth | Not a model credential | Not a model credential | Not a model credential | Allow by control-plane policy |

Sharing Key 对 `/tingly/team` 的授权不表示访问 bare `team` rules。鉴权后，服务端将请求映射为：

```text
default Team       → rule scenario "team"
non-default Team   → rule scenario "team:<stable-team-uuid>"
```

外部 URL 始终保持 `/tingly/team`；内部 scenario 由认证上下文派生，客户端不能请求
`/tingly/team:<id>` 来选择或冒充另一个 Team。

## 6. Request authorization flow

```text
tb-share-* credential
  → validate token exists and is enabled
  → load token.TeamID
  → require Team exists and is enabled
  → require registered model route is /tingly/:scenario[/v1]
  → require bare scenario == "team"
  → write auth_kind, user_id and team_id to request context
  → derive internal Team rule scenario from authenticated team_id
  → route and record usage with the same team_id
```

鉴权必须 fail closed。新增模型表面不会自动接受 Sharing Key；只有明确符合 Team route
shape 的请求才允许继续。仅验证 token 是否有效而不验证 route，不是合法实现。

## 7. Lifecycle rules

### Create

- 创建 Key 时必须提供一个存在且已启用的 Team。
- 未显式提供 Team 的旧控制面调用归入 `default` Team。

### Move

- 管理员可将 Key 移动到另一个已启用 Team。
- 移动更新原 token 的单一 `team_id`，不产生多 Team membership。
- 当前实现不旋转 raw token；存储和内存 cache 在操作返回前完成更新，因此下一次请求立即使用新 Team。
- 若未来威胁模型要求移动即轮换，应作为单独 breaking decision 记录。

### Disable

- 禁用 Key 只影响该 Key。
- 禁用 Team 会通过运行时校验立即拒绝该 Team 的全部 Key，而不逐条修改 token。

### Delete

- `default` Team 不可删除。
- Team 仍拥有 Sharing Key 时不可删除；必须先移动或删除这些 Key。
- Team 删除后 slug 编号可复用，UUID 不可复用。

## 8. Migration

Team schema 初始化时创建稳定的 `default` Team。所有历史 `api_tokens.team_id` 为空或
NULL 的 Sharing Key 自动回填为 `DefaultTeamID`，raw token 不轮换。

迁移完成后立即应用新权限边界：历史 Sharing Key 不再访问其他 scenario 或 virtual model。
这是刻意接受的安全性 breaking change，不保留 Legacy Global Key。需要跨 scenario 的现有
调用应改用全局 model token，或把所需 routing rules 迁移到对应 Team 后调用
`/tingly/team[/v1]`。

## 9. Product and UX contract

- Team 像 Claude Code Profile 一样直接出现在 Agent layout；额外 Team 显示 `tN - name`。
- `Add Team` 位于 layout，用户只输入 name，slug 由系统生成。
- Team 页面、Sharing Key 列表和 Key 创建弹窗必须持续说明权限边界。
- 提示必须展示真实允许端点 `/tingly/team`、`/tingly/team/v1`，并明确排除其他 Team、
  scenario 与管理 API。
- 不提供“允许其他端点”开关；需要更高权限时应选择另一种凭证，而不是扩大 Team Key。

这对应 `.design/ux-principles.md` 的命名统一、展示具体值、合理默认、内嵌教育和降低视觉噪声。

## 10. Non-goals

- 通用组织、成员或角色 RBAC。
- 一个 Sharing Key 同时属于多个 Team。
- 用户自定义 Team slug。
- 每个 Key 自定义 endpoint allowlist。
- 用 Sharing Key 调用控制面管理 API。
- 在 Team 体系中取代全局 model token。

## 11. Key implementation files

| Layer | File | Responsibility |
|---|---|---|
| Persistence | `internal/db/team_store.go` | Team identity、编号、生命周期 |
| Persistence | `internal/db/api_token_store.go` | Key-Team binding、迁移、启停和移动 |
| Authentication | `internal/middleware/auth.go` | Sharing Key route authorization boundary |
| Routing | `internal/protocolserver/routes_middleware.go` | Authenticated Team → internal rule scenario |
| Usage | `internal/protocolserver/usage_tracking.go` | Persist authenticated `team_id` |
| Control API | `internal/server/module/team/` | Team CRUD |
| Control API | `internal/server/module/sharing/` | Key CRUD、过滤和移动 |
| Frontend | `frontend/src/contexts/TeamContext.tsx` | Shared Team state |
| Frontend | `frontend/src/pages/scenario/UseTeamPage.tsx` | Team workspace |
| Frontend | `frontend/src/pages/scenario/components/SharingKeysDialog.tsx` | Team Key management |
| Frontend | `frontend/src/pages/scenario/components/TeamKeyScopeAlert.tsx` | User-visible security contract |

Primary regression coverage:

- `internal/middleware/auth_sharing_scope_test.go`
- `internal/protocolserver/team_scope_test.go`
- `internal/db/team_store_test.go`
- `internal/server/module/team/handler_test.go`
- `internal/server/module/sharing/team_handler_test.go`
