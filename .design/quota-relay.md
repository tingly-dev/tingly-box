# Quota 中继：端侧看到中央的 quota

> 适用对象：改 `ai/quota/relay.go`、`ai/quota/fetcher/tinglybox.go`、
> `internal/protocolserver/quota.go`、Team 的 quota 共享开关的贡献者。
> 语义基础见 `quota-semantics.md`，Team 与 Sharing Key 的鉴权见 `team.md`。

## 1. 问题

Tingly Box 既部署在中央，也部署在端侧。端侧把一个 provider 指向中央的
`/tingly/<scenario>`，手里只有模型凭证（global model token 或 Sharing Key），
没有中央背后的 vendor key，所以原先看不到任何 quota。

## 2. 决定：原 quota 模块的延伸，只做展示

- **传输格式就是 `ProviderUsage`**。中央把背后的 provider quota 合成一个
  `ProviderUsage` 返回；端侧的 `tingly_box` fetcher 原样存下，走原有的
  manager / store / 刷新 / 卡片展示，不新增类型，不做反向转换。
- **粒度是上游 provider**。中央遍历该 scenario 下 active rule 的 service
  （默认池与 smart-routing 分支），provider 去重后按 rule 顺序匿名为
  `upstream 1`、`upstream 2`……，窗口标签写成 `upstream N · 5h`。
- **只做展示**。端侧的路由和 statusline 不读这份数据的内部结构，
  quota 核心语义不因 TB 出现任何特例。
- 只考虑"一个中央 + 一个端侧"。多级时每一级把上一级当普通上游配置即可。

## 3. 协议

`GET /tingly/:scenario[/v1]/quota`（模型面，与 `/models` 同一条
`modelAuth → teamScopeMiddleware` 鉴权链）→ `ProviderUsage`

- 只读存储的 quota，不触发上游刷新——端侧不能借此放大对 vendor 的请求。
- 永不下发：provider 名称 / UUID / 类型、`Account`、`Cost`、`Breakdowns`、
  `RawResponse`、`LastError`。读不到 quota 的 provider 直接略过。
- `fetched_at` 取参与合成的最旧一次读取，如实反映数据年龄。

## 4. 权限

| 凭证 | 能否读取 | 下发内容 |
|---|---|---|
| Global model token（实例 owner） | 允许，不受 Team 开关约束 | 窗口原样（含绝对值与余额），仍匿名 |
| Sharing Key，Team 开启「向共享密钥开放 quota」 | 允许，只到自己 Team 的 rule | 只有百分比与重置时间；只有绝对值的窗口（无上限余额）丢弃 |
| Sharing Key，Team 未开启 | 403 | — |

- 开关是 Team 的属性（`teams.quota_visible`），默认关闭，入口在 Team 页的
  「Team 设置」对话框。每次请求从 Team store 的内存镜像读取，改完立即生效；
  Team 停用时视为不共享。
- Sharing Key 访问其他 scenario 的 `/quota` 由原有 Team 路由限制拦截（403）。

## 5. 端侧识别

API base 路径中出现 `/tingly/<scenario>`（允许前面带反向代理前缀、后面带
`/v1`）即为 TB 上游，`inferProviderType` 返回 `tingly_box`。这一步先于
host 规则，因为中央可能在任何 host 上。这不违反"路径不能替 vendor 说话"：
fetcher 只回访 provider 本来就在发模型请求的同一个 host。

失败提示：403 →「上游 Team 未共享 quota」，404 →「上游版本不支持」。

## 6. 暂不做

- 按模型展示、端侧路由 / statusline 按模型读取中央额度。
- 多级链路的环检测与来源透传。
