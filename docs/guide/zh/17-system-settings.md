# 系统设置

路径：`/system`、`/system/logs`

系统设置页面提供全局偏好配置、服务器状态查看、代理设置、语言/主题切换和日志查看功能。

---

![系统设置](../images/system.png)

## 系统设置主页（`/system`）

General 标签页由四张卡片组成，每张各自回答一个问题：

### 服务器状态卡（Server Status）

只回答「网关是否健康」这一个问题：

| 字段 | 说明 |
|------|------|
| Server | 运行中（Running）/ 已停止 / 不可用，附带 Connected/Disconnected 指示 |
| Uptime | 服务器已运行时长 |
| Proxy | 当前是否生效代理 |

**操作**（右上角图标）：**Force Logout**（强制退出当前 Web 会话，清除 Token 并返回登录页）与 **Refresh Status**（手动刷新状态）。

---

### Quick Proxy 卡

为所有对外 API 请求配置统一的 HTTP/HTTPS 代理——一个可复用的预设，Provider 和 OAuth 一键即可采用（若某 Provider 单独设置了代理，则以该设置为准）：

1. 在文本框中输入代理地址（如 `http://127.0.0.1:7890`）
2. 点击 **Save** 保存
3. 保存成功后显示绿色对号图标

> 如需为某个 Provider 单独配置代理，请在 [凭证管理](./08-credentials.md) 的 Provider 编辑表单中设置。

---

### Appearance & Language 卡

用户偏好设置，与服务器状态卡分离，使后者只回答「网关是否健康」，不与个人偏好混在一起：

- **Language**：`English` / `中文`
- **Theme**：`Light` / `Dark` / `Sunlit` / `Claude` / `System`（跟随系统设置）

---

### About 卡

- **当前版本**：显示版本号
  - 有可用更新时显示更新提示
  - 开发版本显示 `dev` 标记
- **License**：MPL-2.0 + Commercial
- **GitHub**：项目仓库链接

---

## 日志页面（`/system/logs`）

路径：`/system/logs`

实时查看 Tingly-Box 服务器的运行日志。

### 功能

![日志页面](../images/logs.png)

**Debug Mode 开关**（右上角）：
- 开启：日志级别切换为 `debug`，输出更详细的调试信息
- 关闭：日志级别为 `info`（默认）

**LogExplorer 区域：**
- 实时流式显示服务器日志
- 支持滚动查看历史日志
- 日志条目包含时间戳、级别、来源模块、消息内容

---

## 相关页面

- [访问控制](./18-access-control.md)
- [实验性功能](./19-experimental.md)
- [凭证管理](./08-credentials.md)
