# `tb restart` 卡顿实验

## 背景

用户反馈 `tb restart` 有时会很慢、卡住。排查后定位到两个独立问题（均已在
`internal/command/server.go`、`internal/command/server_unix.go`、
`internal/mcp/runtime/runtime.go` 修复）：

1. **锁释放顺序错误**：旧代码在服务真正停止（`serverManager.Stop()`）之前就
   调用了 `fileLock.Unlock()`，导致 `restart` 认为"旧进程已停"、立刻尝试绑定
   端口，实际上旧进程可能仍占用着端口，造成竞态。
2. **MCP 数据源串行关闭，且不遵守超时**（真正的大头）：`Runtime.Close()`
   原来逐个 `Disconnect()` 配置的 stdio MCP 数据源，每个数据源在对方子进程
   不响应时，底层 SDK（`modelcontextprotocol/go-sdk` 的
   `CommandTransport`/`pipeRWC.Close`）要经过"等待→SIGTERM→再等待→SIGKILL"
   的完整升级流程，且**不理会传入的 `ctx` 超时**，单个数据源最坏情况要
   10 秒左右。配置了 N 个数据源时，串行关闭会线性叠加到 `N × 10s`，这正是
   "restart 很慢/卡住"最主要的成因。

本实验用真实子进程复现这两种效果的量级差异（重点是第 2 点，因为它是随配置
数量线性放大、最容易在生产环境中被用户实际感知到的部分）。

## 实验设计

- **被测代码**：`internal/mcp/runtime/restart_latency_experiment_test.go`
  中的 `TestRestartLatencyExperiment`（按 `experiment` build tag 隔离，默认
  不参与 `go test ./internal/...`/CI）。
- **两个对照实现**：
  - `sequential`：修复前 `Runtime.Close()` 的行为——在一个共享的 5s
    `context` 下逐个调用 `source.Disconnect(ctx)`。
  - `concurrent`：当前 `Runtime.Close()` 的行为——并发调用所有
    `Disconnect()`，用同一个 5s 预算统一限时返回。
- **两个因子**：
  - 数据源数量：1 个 / 3 个
  - 子进程行为：正常（收到 stdin 关闭后立即退出）/ "卡住"
    （通过 `FAKE_MCP_SLOW=1` 让子进程忽略 SIGTERM、且不响应 stdin
    关闭，只能被 SIGKILL 杀死，用来模拟一个真实场景中挂死的 MCP server）
- **被连接的子进程是真实的 MCP stdio server**：`fakemcp/main.go` 用仓库本身
  依赖的 `github.com/modelcontextprotocol/go-sdk` 的 server 端 API 实现，
  握手协议与生产环境完全一致，只是刻意控制了退出行为这一个变量。

## 如何重新运行

```bash
go test -tags experiment ./internal/mcp/runtime/ -run TestRestartLatencyExperiment -v
```

约耗时 50 秒左右（大部分时间花在"卡住"场景故意等待关闭超时上）。运行结束后
用 `ps aux | grep fakemcp` 确认没有残留进程（测试自带 `pkill` 兜底清理）。

## 一次实际运行结果（本仓库沙箱环境，2026-09-12）

| 场景 | sequential（修复前逻辑） | concurrent（当前代码） |
|---|---|---|
| 1 个数据源 / 正常退出 | 1ms | 1ms |
| 1 个数据源 / 卡住不退出 | **10.0s** | **5.0s** |
| 3 个数据源 / 正常退出 | 3ms | 1ms |
| 3 个数据源 / 卡住不退出 | **30.0s** | **5.0s** |

结论与预期完全吻合：

- 子进程正常退出时，两种实现都是毫秒级，没有区别——说明这个 bug 只在配置了
  "会卡住/响应慢"的 MCP server 时才会显现，这也是为什么它没有在日常开发中
  被立刻发现。
- 子进程卡住时，**修复前的关闭耗时随数据源数量线性增长**（1 个→10s，
  3 个→30s），而**修复后始终封顶在共享的 5s 预算内**，与数据源数量无关。
  这正是用户感知到的"配置了几个 MCP server 之后，restart 越来越慢/直接卡住"
  的根因。

## 文件

- `fakemcp/main.go`：可控的假 MCP stdio server（基于真实 SDK 实现，行为通过
  `FAKE_MCP_SLOW` 环境变量控制）。
- `../../../internal/mcp/runtime/restart_latency_experiment_test.go`：实验
  本体（Go test，`experiment` build tag）。
