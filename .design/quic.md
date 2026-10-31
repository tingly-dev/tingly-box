# TEP-2278：部署层终止 QUIC 的端云传输

状态：设计及验证记录；关联 PR #2281。当前 TB 不直接提供 QUIC/UDP 服务端。

## 链路

```text
普通 Agent / SDK
      │ 原有 HTTP
      ▼
本地 TB ── HTTPS / TCP，或 HTTP/3 / QUIC / UDP ──▶ Caddy
  │                                                    │
  │ 可信 HTTPS 发现 + 无副作用探测                       │ HTTP
  │ 不可用时后续请求回到 TCP                             ▼
  └──────────────────────────────────────────────▶ 云端 TB → Provider
```

Caddy 在一个公网端口号的 TCP/UDP 上承载 HTTPS 和 HTTP/3，负责证书及加密终止；云端 TB 继续监听既有 HTTP 端口。仓库提供 `deploy/caddy/Caddyfile`；Caddy 将模型请求及同一路径上的 HEAD 能力探测转发到 TB，保留部署路径前缀。普通客户端到本地 TB 的接口无需改变。部署层终止 QUIC 后，Caddy 到云端 TB 的内部链路是 HTTP；不能把这称为直接 TB-to-TB QUIC。

## 发现、安全与失败语义

- 本地 TB 只接受已验证 HTTPS 的同源同端口 H3 声明，后台对业务请求同一 URL 发带标记的 HEAD 探测云端 TB 的能力，探测结果按 URL 隔离；探测成功后才切换后续请求。明文 HTTP 不自动升级。
- 客户端需要信任部署层证书。自签名证书应通过受控 CA/信任库配置，禁止以关闭证书验证代替信任配置。当前 TB 没有专门的证书导入 UI；实验用进程级 `SSL_CERT_FILE`。
- 不支持 H3 或 UDP 不可达时继续走 TCP；业务请求结果不明或已返回部分流时不能透明重放，避免重复生成和计费。主动取消向上游传播。
- 云端 TB 对带 `Tingly-Transport-Probe: tb-peer-v1` 的 HEAD 请求返回 `Tingly-Transport: tb-peer-v1`，表明请求到达支持此探测的 TB；这个标记不表示 TB 自身监听 HTTP/3。探测走通 HTTP/3 链路还需部署层支持，普通业务请求继续由已有认证与策略保护。
- Caddy 的明文 HTTP 和 HTTPS 共用同一 TCP 端口时，默认 TLS 入口不提供透明明文 API。可用 308 跳转，但客户端必须支持跳转并信任证书。
- 代理到后端使用 HTTP/3 时，Caddy 的 H3-only upstream 不自动回落 HTTP/2；这里推荐的部署是 Caddy → TB 既有 HTTP。
- 原端口明文 HTTP、HTTPS、QUIC 三协议共存曾以 cmux 做独立实验；未接入 `tb start`，不属于本方案的交付。

## 复现验证

使用仓库自带的 `internal/protocoltest` 双进程环境；两个 TB 均运行真实 `server.Start`，云端使用 VModel，不调用付费模型。测试使用 Caddy v2.11.7、临时自签名证书及私有临时配置，不修改系统信任库或已有服务。

```sh
CADDY_BIN=/absolute/path/to/caddy go test -tags=e2e ./internal/protocoltest \
  -run '^TestCaddySelfSignedTwoRealTB$' -v -count=1 -timeout=120s
```

测试覆盖初始 HTTPS/TCP、自动发现 H3、OpenAI 与 Anthropic 上游路径的流式/非流式与用量断言，以及关闭 Caddy H3 后的后续请求回落。部署配置本身的自签名回归可运行 `CADDY_BIN=/absolute/path/to/caddy go test -tags=e2e ./internal/protocoltest -run ^TestCaddyDeploymentConfigHTTP3$ -v`。详细实测结果见下文。网络损耗、延迟与吞吐性能尚待基准测试。

# 自签名 Caddy 与双 TB 验证

验证时间：2026-10-27。代码基线：PR #2281，`21d21b51`。Caddy：v2.11.7。两台 TB 为独立进程，通过生产 `server.Start` 启动；云端 TB 使用 VModel，未调用真实模型或发生模型费用。

```text
普通 Anthropic 客户端 → 本地 TB → Caddy（自签名 HTTPS/HTTP/3）→ 云端 TB（HTTP）→ VModel
```

客户端与 Caddy 之间的 HTTPS 和 HTTP/3 使用同一公网端口号；Caddy 到云端 TB 使用其现有 HTTP 端口。证书仅在测试进程及子进程通过 `SSL_CERT_FILE` 信任，不更改系统证书库。Caddy 在 loopback 临时端口运行，测试结束后停止。

| 检查 | 结果 |
| --- | --- |
| 首次请求 | 本地 TB 日志观察到 HTTP/2 |
| OpenAI Chat 上游 | 观察到该 provider 的 HTTP/3；流式、非流式、用量检查通过 |
| Anthropic 上游 | 观察到该 provider 的 HTTP/3；流式、非流式、用量检查通过 |
| Caddy 关闭 H3 | 首个受影响流式请求返回 502；后续请求恢复成功，日志新增 HTTP/2 |
| 结果 | PASS，约 9.4 秒 |

关闭 H3 后出现的 502 是已发出请求的失败，不应把该请求描述成无损或透明重放。上述记录证实后续请求能够回落；它不证明所有失败都无重复执行。前期使用独立 Caddy 测试夹具还验证过不信任自签名证书时拒绝连接、受控信任后成功、取消传播以及 20 路并发。当前双 TB 测试没有重复这些独立夹具的全部检查。

本次未测公网部署、代理链、不同操作系统或弱网性能；也未证明 TB 直接监听 QUIC。可复现测试代码位于 `internal/protocoltest/caddy_duo_e2e_test.go`，由 `e2e` 构建标签启用。
