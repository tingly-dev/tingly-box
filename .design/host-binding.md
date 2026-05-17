# Host Binding 设计与决策

> 适用对象：tingly-box 后端贡献者，以及任何要新增"本地监听 / 本地 URL"代码的人。
> 本文档记录 `127.0.0.1` vs `localhost` 的策略选择及其理由，作为后续 PR 的对照依据。

---

## 1. 背景

tingly-box 是**本地宿主**的 AI gateway：

- 服务端（HTTP server、OAuth callback server）只绑定回环口，外网不可达。
- 客户端（Claude Code / Codex / OpenCode CLI、浏览器 OAuth 回跳）从同一台机器
  上访问该 gateway。

长期使用 `127.0.0.1` 字面量，commit `4569e8f`（PR #966）把整库 `127.0.0.1`
统一替换为 `localhost`，声称 "much more robust"。实际效果**好坏参半**，触发
本次决策回顾（PR #972）。

---

## 2. 关键事实

### 2.1 `net.Listen("tcp", "<host>:<port>")` 的行为

Go 的 `net.Listen` 接受 host 后会做名字解析，但**只 bind 单个地址**：

| host 参数 | 实际行为 |
|---|---|
| `127.0.0.1` | 一定 bind IPv4 loopback，确定 |
| `::1` | 一定 bind IPv6 loopback |
| `localhost` | 依赖解析返回的**第一个**可 bind 地址。dual-stack Linux 通常先 `::1`；macOS 通常先 `127.0.0.1`；缺 `/etc/hosts` 条目的精简容器直接失败 |
| `""` / `0.0.0.0` | bind 所有 IPv4 接口（**外网可达**，不适合 gateway） |

→ **`localhost` 作 bind 参数是回归**：dual-stack 环境下只 bind `::1` 时，
任何用 `http://127.0.0.1:PORT` 显式访问的 client 会被 connection-refused。

### 2.2 client 侧用 `localhost` 的安全性

| 调用方 | 解析策略 |
|---|---|
| Go `net/http`（默认 `Transport`） | `Dial` 调 `LookupHost` 拿到所有地址，按返回顺序逐个尝试连接 |
| Chrome / Firefox / Safari | RFC 6555 happy eyeballs，并行尝试 v4/v6，谁先成功用谁 |
| 多数 Node / Python HTTP client | 同 Go，多地址 fallback |

→ **client 写 `localhost` 基本无害**：即使 server 只 bind 在 v4，client
解析 localhost 时若同时拿到 v6，连 v6 失败后会自动 fallback 到 v4。

### 2.3 `getLocalIP()` 类工具函数

签名约定是"返回一个 IP 字符串"。下游可能：

- 拼到 URL 里（`http://<ip>:port/...`，含 `localhost` 也能跑，但是侥幸）
- 做 IP 字段比对（`if ip == "127.0.0.1"`，含 `localhost` 直接坏掉）
- 显示在日志 / metrics 里（"localhost" 看起来像 bug）

→ fallback 返回 `"localhost"` 是**契约违规**，必须返回 IP 字面量。

---

## 3. 决策

| 用途 | 选择 | 理由 |
|---|---|---|
| `net.Listen` server bind | **`127.0.0.1`** | 行为确定，dual-stack 安全 |
| 测试代码里探测端口可用性的 bind（如 `getAvailablePort`） | **`127.0.0.1`** | 必须与真正 server bind 用同一 host，否则探测和实际不一致 |
| `getLocalIP()` 等"返回 IP"工具函数的 fallback | **`127.0.0.1`** | 维持函数契约 |
| 写到用户配置 / 文档 / Swagger example / 提示文案里的 URL | **`localhost`** | 与 happy eyeballs 一致，可读性更好；即便 server 切换 IP family 也无需改 |
| 写到内部生成的 client URL（如 `ANTHROPIC_BASE_URL`、`baseURL`） | **`localhost`** | 同上 |
| 写到代码里**比较 host**的判断（如"这是不是本地"） | **同时识别两者**：`host == "127.0.0.1" \|\| host == "localhost"` |

简记：**"Bind 写 IP，URL 写名字。"**

### 3.1 一图流

```
   ┌──────────────────────────────────────────────────────────────┐
   │  SERVER                                                       │
   │                                                               │
   │     net.Listen("tcp", "127.0.0.1:12580")                      │
   │                       │                                       │
   │                       ▼                                       │
   │              ┌──────────────────┐                             │
   │              │ 127.0.0.1:12580  │  one socket, IPv4 loopback  │
   │              └──────────────────┘  确定、可预测                │
   └─────────────────────────┬─────────────────────────────────────┘
                             │
                             │ TCP
                             │
   ┌─────────────────────────┴─────────────────────────────────────┐
   │  CLIENT  (CC / Codex / OpenCode / 浏览器 / Go http.Client)    │
   │                                                               │
   │     URL: http://localhost:12580/tingly/...                    │
   │                       │                                       │
   │                       │ resolve "localhost"                   │
   │                       ▼                                       │
   │              ┌──────────────────┐                             │
   │              │ [::1, 127.0.0.1] │  多候选地址                  │
   │              └──────────────────┘                             │
   │                       │                                       │
   │                       │  happy eyeballs (RFC 6555)            │
   │                       ▼                                       │
   │       try ::1       ──► refused  (server 不在 v6)  ──┐        │
   │                                                       │       │
   │       fallback ──► try 127.0.0.1  ──► SUCCESS  ◄──────┘       │
   │                                                               │
   └───────────────────────────────────────────────────────────────┘
```

对照：把 server bind 也写成 `localhost`（commit 4569e8f 原状态）是反例 ——

```
   ┌──────────────────────────────────────────────────────────────┐
   │  SERVER (WRONG)                                               │
   │                                                               │
   │     net.Listen("tcp", "localhost:12580")                      │
   │                       │                                       │
   │                       │ Go 解析后只 bind 第一个可用地址        │
   │                       ▼                                       │
   │              ┌──────────────────┐                             │
   │              │     ::1:12580    │  IPv4 loopback 未监听        │
   │              └──────────────────┘                             │
   └─────────────────────────┬─────────────────────────────────────┘
                             │
                             │ ✗ 任何写 127.0.0.1 的 client 被 refused
                             │ ✗ 缺 /etc/hosts 的精简容器直接 bind 失败
```

---

## 4. 反模式

下面这些写法应该在 review 阶段被打回：

```go
// ❌ Server bind 用 localhost
net.Listen("tcp", "localhost:8080")

// ❌ getLocalIP / IP fallback 返回域名
func getLocalIP() string {
    ...
    return "localhost"
}

// ❌ 显式裸 0.0.0.0 监听（除非有外网暴露的明确需求）
net.Listen("tcp", "0.0.0.0:8080")

// ❌ host 比较只覆盖一种写法
if host == "127.0.0.1" { /* 漏掉 localhost */ }
```

正确：

```go
// ✅
net.Listen("tcp", "127.0.0.1:8080")

// ✅
func getLocalIP() string {
    ...
    return "127.0.0.1"
}

// ✅
func isLoopback(host string) bool {
    return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// ✅ 内部写出去给 client 的 URL
url := fmt.Sprintf("http://localhost:%d/tingly/codex", port)
```

---

## 5. 历史 config 迁移

存量用户磁盘上已经被旧版 tingly-box 写过形如
`http://127.0.0.1:12580/tingly/...` 的配置：

- `~/.claude/settings.json` → `env.ANTHROPIC_BASE_URL`
- `~/.codex/config.toml` → `[model_providers.tingly-box].base_url`
- `~/.config/opencode/opencode.json` → `provider.tingly-box.options.baseURL`

按本设计，这些字段**应该是 `localhost`**（因为是 client URL，且会被 happy
eyeballs 兜底）。

→ `internal/server/config/migration_localhost.go` 的 `migrate20260517` 一次性
把它们改写。嗅探条件：host == `127.0.0.1` **且**路径含 `/tingly/`（Claude
Code）或处在 `tingly-box` provider 命名空间下（Codex / OpenCode）。用户自己
配置的非 tingly URL（如指向本地 v2ray 的 `socks5://127.0.0.1:7890`）不会
被动到。

---

## 6. 相关 PR / Commit

| ref | 内容 |
|---|---|
| `4569e8f` (PR #966) | 整库 `127.0.0.1 → localhost`，含 server bind（**部分回归**） |
| `28d80fe` (PR #972) | 存量外部 config 文件的 migration |
| `3cc336e` (PR #972) | 把回归的 server bind 改回 `127.0.0.1`，client URL 保持 `localhost` |
