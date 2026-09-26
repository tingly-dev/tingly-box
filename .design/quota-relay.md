# Quota 中继：端侧看到中央的 quota

> 适用对象：改 `ai/quota/gateway.go`、`ai/quota/fetcher/tinglybox.go`、
> `internal/protocolserver/quota.go`、Team 的 quota 共享开关的贡献者。
> 语义基础见 `quota-semantics.md`，Team 与 Sharing Key 的鉴权见 `team.md`。

## 1. 问题

Tingly Box 既部署在中央，也部署在端侧。端侧把一个 provider 指向中央的
`/tingly/<scenario>`，手里只有模型凭证（global model token 或 Sharing Key），
没有中央背后的 vendor key，因此原先看不到任何 quota：provider 卡片为空，
端侧的 `service_quota` 路由对它一律放行。

直接透传中央的 provider quota 会暴露中央的账号、provider 和余额。

## 2. 决定

**下发"凭证能用的模型还剩多少"，不下发 provider。**

```
credential → 可达的 rules（= /models 的列表）→ 背后的 services → provider quota
                     ↑ 中央侧逐模型投影、脱敏后下发
```

- 只解决"一个中央 + 一个端侧"。多级时每一级把上一级当普通上游配置即可；
  投影天然只减不增（端侧再转发的只是它收到的投影）。
- 暴露范围复用既有鉴权：`/tingly/:scenario/quota` 与 `/models` 走同一条
  `modelAuth → teamScopeMiddleware` 链，Sharing Key 只能看到自己 Team 的模型，
  客户端无法选择 scenario 之外的范围。

## 3. 协议

`GET /tingly/:scenario[/v1]/quota`（模型面，模型凭证鉴权）

```json
{"models": [
  {"model": "sonnet",
   "windows": [{"key": "5h", "type": "session", "kind": "limit", "used_percent": 30, "...": "..."}],
   "recovers_at": "…", "fetched_at": "…"},
  {"model": "private", "unreadable": true}
]}
```

- 只读存储的 quota，不触发上游刷新——端侧不能借此放大对 vendor 的请求。
- **每个模型由剩余最多的 service 代表**：请求会被路由到还有额度的地方，
  所以这是"还剩多少"的诚实答案；取平均会掩盖耗尽（quota-semantics §2.2）。
  规则包含默认池与所有 smart-routing 分支中的 active service。
- 永不下发：provider 名称 / UUID / 类型、`Account`、`Cost`、`RawResponse`、
  `LastError` 原文（可能含上游 URL 或 key 片段，只下发 `unreadable`）。

## 4. 权限

| 凭证 | 能否读取 | 下发内容 |
|---|---|---|
| Global model token（实例 owner） | 允许 | 窗口原样（含绝对值与余额） |
| Sharing Key，Team 开启「向共享密钥开放 quota」 | 允许 | 只有百分比与重置时间：可计量窗口换算为 `x/100 percent`，无上限窗口保留 `unlimited` 标记，只有绝对值的窗口（无上限余额）丢弃 |
| Sharing Key，Team 未开启 | 403 | — |

- 开关是 Team 的属性（`teams.quota_visible`），默认关闭：quota 反映的是运营者
  账号的使用强度，是否共享由 Team 管理员决定。入口在 Team 页的「Team 设置」对话框，
  开关旁直接说明共享什么、不共享什么。
- 每次请求都从 Team store 的内存镜像读取，修改后下一次读取立即生效；
  Team 停用时视为不共享。

## 5. 端侧

- **识别**：API base 的路径中出现 `/tingly/<scenario>`（允许前面带反向代理前缀、
  后面带 `/v1`）即为 TB 上游，`inferProviderType` 返回 `tingly_box`。
  这一步先于 host 规则，因为中央可能在任何 host 上（本地、内网、公网）。
  这不违反"路径不能替 vendor 说话"：TB fetcher 只回访 provider 本来就在
  发模型请求的同一个 host，凭证不会被带去别处。
- **读取**：`tingly_box` fetcher 用 provider 已存的 key 调
  `<prefix>/tingly/<scenario>/quota`；403 显示为"上游 Team 未共享 quota"，
  404 显示为"上游版本不支持"，让用户知道该找谁。
- **映射**（`quota.GatewayUsage`）：
  - 每个模型 → 一个 `Group: "model"` 的 breakdown，保存完整窗口；
  - 每个模型的最紧窗口 → 提升到 `Windows`，标签为 `<model> · <window>`，
    provider 卡片因此一模型一条进度条。
- **按模型读取**（`ProviderUsage.ForModel`）：一个 TB provider 下的多个模型
  各自耗尽，账号级窗口不能互相 gate。`service_quota` 路由和 statusline 都用
  `usage.ForModel(service.Model)`，只看该模型自己的 breakdown；未被上游报告的
  模型视为未知（不计为 0%）。对其他 provider 类型 `ForModel` 原样返回。

## 6. 范围外

- 多级链路的环检测、来源时间透传：按"每级自己配"处理，不做。
- 在模型响应上附带额度响应头：拉取已覆盖 quota 页、statusline 与路由，
  响应头只能覆盖发过请求的模型，暂不做。
