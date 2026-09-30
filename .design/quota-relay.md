# Quota 中继：端侧看到中央的 quota

端侧 TB 把 provider 指向中央 TB 的 `/tingly/<scenario>`，只持有模型凭证，
原先看不到中央背后的 quota。本功能是 quota 模块的一个特殊 provider 类型，**只做展示**。

## 做法

- **端侧**：API base 路径含 `/tingly/<scenario>` 即识别为 `tingly_box`（先于 host
  规则，因为中央可在任何 host 上）。fetcher 用 provider 已存的 key 请求同一 host 的
  `<prefix>/tingly/<scenario>/quota`，把返回的 `ProviderUsage` 原样存下。
  403 提示「上游不向此 key 共享 quota」，404 提示「上游版本不支持」。
- **中央**：`GET /tingly/:scenario[/v1]/quota`，与 `/models` 同一条鉴权链。取该
  scenario 下 active rule 的 provider，去重后合成一个 `ProviderUsage`：
  - 窗口标签为 `<provider 名> · <窗口>`（无名称时退回 `upstream N`）；
  - 只保留可计量窗口的百分比与重置时间，不下发账号、绝对值、余额、错误；
  - 只读已存的 quota，不触发上游刷新。

## 权限

- 只有 owner 的 model token 可读。
- Sharing Key 一律 403（先实验 owner 场景；Team 级开关另行加入）。

## 暂不做

按模型展示；端侧路由 / statusline 使用中继数据；多级链路的特殊处理（每级各自配置即可）。
