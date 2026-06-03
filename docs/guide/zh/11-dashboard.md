# 用量看板

路径：`/dashboard/:timeRange`（默认 `/dashboard/7d`）

用量看板提供 AI 请求的使用统计和可视化分析，帮助了解各 Provider 和模型的调用量、Token 消耗、缓存命中率等指标。

---

![用量看板](../images/dashboard.png)

## 时间范围选择

页面顶部提供时间范围快速切换：

| 选项 | 路径 | 说明 |
|------|------|------|
| Today | `/dashboard/today` | 当日（按小时展示） |
| Yesterday | `/dashboard/yesterday` | 昨日（按小时展示） |
| 3D | `/dashboard/3d` | 近 3 天（按日展示） |
| 7D | `/dashboard/7d` | 近 7 天（按日展示，默认） |
| 30D | `/dashboard/30d` | 近 30 天（按日展示） |
| 90D | `/dashboard/90d` | 近 90 天（按日展示） |

---

## 统计卡片

页面顶部 5 个统计卡片汇总当前时间范围内的关键指标：

| 指标 | 说明 |
|------|------|
| **Total Requests** | 总请求次数 |
| **Total Tokens** | 总 Token 数（细分显示 Input / Output） |
| **Cache Hit Rate** | 缓存命中率（百分比） |
| **Error Rate** | 请求失败率 |
| **Streamed Rate** | 流式响应比例 |

---

## Provider 筛选

顶部下拉菜单按认证类型分组展示所有可用 Provider：

- OAuth（OAuth 授权的 Provider）
- API Key
- Bearer Token
- Basic Auth
- Virtual Model

选择特定 Provider 后，图表和表格仅显示该 Provider 的数据。

---

## 自动刷新

顶部提供 **自动刷新** 开关（Auto-refresh）和手动 **刷新** 按钮，开启后数据每分钟自动更新。

---

## 图表区域

### 时序图（Token 历史）

- **今日/昨日**：按小时粒度展示每小时 Token 使用量（Input / Output 堆叠）
- **3D / 7D / 30D / 90D**：按天粒度展示每日 Token 使用量

### 请求明细视图（By Request）

在 `today` / `yesterday` 时间范围下，图表右侧提供 **By Request** 视图，展示单条请求的详细信息（时间、模型、Token 数、响应时间等）。

---

## 右侧：Top Models

显示当前时间范围内按 Token 消耗量排名的前 6 个模型：
- 模型名称 + 所属 Provider
- Token 消耗量（带进度条）
- 点击可快速筛选到对应 Provider

---

## 底部：服务统计表格

按模型/Provider 细分展示详细统计数据：

| 列 | 说明 |
|----|------|
| Model | 模型名称 + Provider |
| Requests | 请求次数 |
| Input Tokens | 输入 Token 数 |
| Output Tokens | 输出 Token 数 |
| Cache Tokens | 缓存命中 Token 数 |
| Errors | 错误次数 |
| Cache Hit % | 缓存命中率 |
| Streamed % | 流式响应比例 |

---

## Token 热力图

路径：`/overview/:timeRange`（默认 `/overview/90d`）

在活动栏「Usage」分组下，还有一个 **Heatmap（热力图）** 视图，以日历热力图形式展示过去 90 天每日的 Token 使用量，直观呈现使用密度分布。

---

## 相关页面

- [系统设置](./17-system-settings.md)
- [凭证管理](./08-credentials.md)
