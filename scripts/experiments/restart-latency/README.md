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
   的完整升级流程，且**不理会传入的 `ctx` 超时**。配置了 N 个数据源时，串行
   关闭会线性叠加成 N 倍，这正是"restart 很慢/卡住"最主要的成因。修复除了把
   关闭改成并发执行、统一限时外，还顺手把 `CommandTransport.TerminateDuration`
   从 5s 调到 2s（`session.go`），让 SDK 自己的优雅关闭升级流程本身就能在共享
   预算内跑完，尽量不必依赖下面第 3 点的强制兜底。
3. **并发关闭后的兜底**：`Runtime.Close()` 到达共享超时后不再等待、直接返回，
   而调用它的进程通常紧随其后就退出——如果什么都不做，还没来得及被 SDK 自己
   的 SIGKILL 收尾的子进程就会变成孤儿进程。修复加了一个 lock-free 的
   `ForceKill()`：超时那一刻直接对还没关闭的子进程发 SIGKILL，不再依赖那个可能
   随进程一起消失的后台 goroutine。

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
    `Disconnect()`，用同一个 5s 预算统一限时返回，超时未完成的直接
    `ForceKill()`。
  两者都跑在当前的 `TerminateDuration=2s` 配置下（这是一个全局共享的 SDK
  参数，不因关闭策略而不同），所以下表里 sequential 的数字也比"5s 预算 ×
  N"最初的估计低——它反映的是"如果仍然串行关闭，现在的 SDK 超时配置下要多
  久"，用来单独隔离"并发 vs 串行"这一个变量。
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

约耗时 25 秒左右（大部分时间花在"卡住"场景故意等待关闭超时上）。运行结束后
用 `ps aux | grep fakemcp` 确认没有残留进程（测试自带 `pkill` 兜底清理；修复后
`ForceKill()` 本身也应保证子进程立即被杀，不依赖这个兜底）。

## 一次实际运行结果（本仓库沙箱环境，2026-09-12，`TerminateDuration=2s`）

| 场景 | sequential（关闭仍是串行） | concurrent（当前代码：并发 + ForceKill） |
|---|---|---|
| 1 个数据源 / 正常退出 | 1ms | 1ms |
| 1 个数据源 / 卡住不退出 | **4.0s** | **4.0s** |
| 3 个数据源 / 正常退出 | 2ms | 1ms |
| 3 个数据源 / 卡住不退出 | **12.0s** | **4.0s** |

结论与预期吻合：

- 子进程正常退出时，两种实现都是毫秒级，没有区别——说明这个 bug 只在配置了
  "会卡住/响应慢"的 MCP server 时才会显现，这也是为什么它没有在日常开发中
  被立刻发现。
- 子进程卡住时，**串行关闭的耗时随数据源数量线性增长**（1 个→4s，
  3 个→12s），而**并发关闭始终封顶在单个数据源自己的关闭时间左右**
  （这里是 SDK 自身 SIGTERM/SIGKILL 升级流程在 `TerminateDuration=2s` 下的
  ~4s，不是我们外层 5s 预算触发的强制超时——1 个数据源的场景里，SDK 自己
  就在外层超时之前完成了关闭，`ForceKill()` 根本没被触发；3 个数据源并发关闭
  互不阻塞，也仍然是 ~4s），与数据源数量无关。这正是用户感知到的"配置了几个
  MCP server 之后，restart 越来越慢/直接卡住"的根因——严重程度（是 `N×4s`
  还是 `N×10s`）取决于 `TerminateDuration`，但"随 N 线性叠加 vs 恒定"这个
  本质差异，就是并发化修复要解决的问题。

## 文件

- `fakemcp/main.go`：可控的假 MCP stdio server（基于真实 SDK 实现，行为通过
  `FAKE_MCP_SLOW` 环境变量控制）。
- `../../../internal/mcp/runtime/restart_latency_experiment_test.go`：实验
  本体（Go test，`experiment` build tag）。
