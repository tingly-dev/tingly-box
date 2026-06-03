# 系统设置

路径：`/system`、`/system/logs`

系统设置页面提供全局偏好配置、服务器状态查看、代理设置、语言/主题切换和日志查看功能。

---

![系统设置](../images/system.png)

## 系统设置主页（`/system`）

### 服务器状态卡（Server Status）

实时显示 Tingly-Box 服务器的运行状态：

| 字段 | 说明 |
|------|------|
| Status | 运行中（Running）/ 已停止 / 不可用 |
| Uptime | 服务器已运行时长 |

**代理设置（Proxy）：**
- **Respect env proxy**：开关，开启后使用系统环境变量中的代理配置（`HTTP_PROXY`、`HTTPS_PROXY`）
- 关闭后使用 Direct 模式（不使用代理）

**其他操作：**
- **Refresh Status**：手动刷新服务器状态
- **Force Logout**：强制退出当前 Web 会话（清除 Token，返回登录页）

**语言切换：**
- **EN**：切换为英文界面
- **ZH**：切换为中文界面

**主题切换：**
- **Light**：浅色模式
- **Dark**：深色模式
- **Auto**：跟随系统设置

---

### 全局代理 URL 卡（Global Proxy URL）

为所有对外 API 请求配置统一的 HTTP/HTTPS 代理：

1. 在文本框中输入代理地址（如 `http://proxy.example.com:8080`）
2. 点击 **Save** 保存
3. 保存成功后显示绿色对号图标

> 此处配置的全局代理对所有 Provider 的请求生效，如需为某个 Provider 单独配置代理，请在 [凭证管理](./08-credentials.md) 的 Provider 编辑表单中设置。

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
