# Agent 沙箱与隔离运行时：业界现状（2026-11）

为 [`thread-runtime.md`](../.design/thread-runtime.md) 做的调研：线程今天在宿主上裸跑，将来要能放进 sandbox / microVM，但不绑定某一家。这里记录"标准层"在别处是怎么做的、agent 沙箱领域有什么、哪些可以对齐。

> 来源以 2026-11 的网页搜索结果为准；部分页面（github.com、agent-sandbox.sigs.k8s.io、infoq）本机无法直接抓取，只读到了搜索摘要，标为"摘要"。CRI/CNI/OCI 的部分是长期稳定的公开规范。

## 1 Kubernetes 的分层：为什么能"不绑定"

| 层 | 管什么 | 形状 | 对我们的启示 |
|---|---|---|---|
| **CRI**（Container Runtime Interface） | Pod 沙箱与容器的生命周期、exec/attach、镜像 | kubelet ↔ 运行时的 **gRPC over unix socket**：`RuntimeService`（`RunPodSandbox`/`StopPodSandbox`/`RemovePodSandbox`/`ListPodSandbox`、`CreateContainer`/`StartContainer`/`ExecSync`/`Exec`/`Attach`）+ `ImageService`；`Version`/`Status` 报告运行时能力；`RuntimeClass`/`runtime_handler` 选具体实现 | **编排留在上层，运行时只给最小动词**；进程外插件；"沙箱"与"沙箱里的进程"是两级对象；流式 I/O 用 gRPC，不用 exec 插件 |
| **CNI**（Container Network Interface） | 给沙箱接网络 | **exec 插件**：运行时以 `CNI_COMMAND=ADD/DEL/CHECK` 调可执行文件，stdin 传 JSON 配置，stdout 回 JSON 结果；插件可链式组合；`capabilities`（portMappings、bandwidth…）由运行时按需传入 | 网络与运行时**正交**、可替换；一次性的配置动作适合 exec+JSON；能力声明后再传参 |
| **CSI** | 存储卷的供给与挂载 | gRPC（Controller/Node 两套服务） | 工作区（worktree）的供给可以是单独一层 |
| **OCI runtime/image spec** | 镜像格式、`config.json` 运行时配置（runc 是参考实现） | 文件格式 + CLI 约定 | **参考实现**的价值：今天的"宿主直跑"就是我们的 runc |
| **containerd shim v2** | 把 runc/Kata/gVisor/Firecracker 等接到 containerd | 每个沙箱一个 shim 进程，ttrpc 协议 | Kata、gVisor（runsc）、firecracker-containerd 都靠这一层"换芯不换壳" |

要点：Kubernetes 不自己隔离，隔离强度靠 `RuntimeClass` 选（runc / gVisor / Kata）；上层 API 完全不变。

## 2 Agent 沙箱领域（2025-2026）

**没有统一的运行时 API**。各家 SDK 各不相同；在"编排层"和"打包/权限层"各出现了一个标准化尝试。

### 2.1 Kubernetes Agent Sandbox（kubernetes-sigs/agent-sandbox，SIG Apps）
- `Sandbox` CRD + 控制器：长期运行、有状态、单实例、稳定主机名的工作负载，"像轻量 VM 的单容器"；支持挂起到零与恢复。扩展 CRD：`SandboxTemplate`、`SandboxClaim`、`SandboxWarmPool`（预热池消除冷启动，起一个 Pod 约 1 秒，对 agent 太慢）。
- **不自带隔离**：用 RuntimeClass 选 gVisor 或 Kata。
- 时间线（摘要）：2025-11 KubeCon NA 发布；2026-05 GKE Agent Sandbox GA；2026-06 v0.5.0 API 到 v1beta1；2026 下半年已有 v1.0.x。
- 它在编排层，适合"远程/集群"那一档，不适合单机桌面。
- 来源：[agent-sandbox overview](https://agent-sandbox.sigs.k8s.io/docs/getting_started/overview/)（摘要）、[Cloud Native Now](https://cloudnativenow.com/features/kubernetes-builds-a-sandbox-crd-for-ai-agents/)、[Northflank](https://northflank.com/blog/agent-sandbox-on-kubernetes)、[bex.co](https://bex.co/blog/2026/09/24/kubernetes-agent-sandbox-upstream-sig-apps)

### 2.2 Docker Sandbox Kit Spec v3（docker/sandbox-kit-spec，拟捐 CNCF）
- 把 agent、工具和**它请求的权限**打成普通 OCI 镜像：manifest 注解 `vnd.docker.sandbox.kit.descriptor`，描述放在 `/usr/share/sandbox/kit/<stem>/kit.yaml`，`schemaVersion: "3"`，`capabilities` 下列出各自带版本的请求类型：`network-policy@1/@2`、`credential@1`、`git-identity@1`、`ssh-agent@1`、`volume@1`、`host-mount@1`、`port@1`、`resources@1`、`privileged@1`、`lifecycle@1`、`agent-skills@1` 等。
- **请求 vs 授予**：Kit 只请求，宿主（运行时）决定授予与执行；凭据由代理注入，沙箱里只看到占位符；网络默认拒绝，deny 优先（例：允许 api.github.com 但拒绝 `DELETE /repos/**`）；安装阶段与运行阶段的出口和凭据分开，运行时在 agent 启动前收回安装阶段的授权。
- 目前唯一合规运行时是 Docker Sandboxes（每个 agent 一个 microVM，自带内核和 Docker daemon）。规范仍标实验性。
- 它定义的是**权限与打包的词汇**，正好是我们"策略"那一层可以借的。
- 来源：[Docker blog](https://www.docker.com/blog/docker-sandbox-kit-spec/)、[SPEC-v3](https://github.com/docker/sandbox-kit-spec/blob/main/docs/spec/SPEC-v3.md)、[Go spec 包](https://pkg.go.dev/github.com/docker/sandbox-kit-spec/v3/spec)、[InfoQ](https://www.infoq.com/news/2026/10/docker-sandbox-ai-agent/)、[devops.com](https://devops.com/docker-introduces-open-sandbox-kit-spec-for-ai-agent-permissions)

### 2.3 E2B（Firecracker）与 envd —— 事实上的"沙箱内进程/文件 API"
- 架构：REST API（控制面）→ gRPC → 每节点 orchestrator（起停、暂停、恢复、快照 Firecracker VM）；模板由 Docker 镜像构建；client-proxy 做边缘路由并在有流量时唤醒暂停的沙箱。可自托管（e2b-dev/runtime）。
- **envd**：VM 内守护进程，Connect RPC + REST（端口 49983）。`Process` 服务：`Start`（流式，cmd/args/envs/cwd/pty/stdin）、`Connect`（按 pid/tag 重连）、`List`、`SendInput`/`StreamInput`/`CloseStdin`、`SendSignal`、`Update`（PTY）；`Filesystem` 服务：`Stat`、`ListDir`、`MakeDir`、`Move`、`Remove`、`WatchDir`；文件上传下载走 REST `/files`。
- 已有多个第三方实现同一协议（Rust/Go 客户端、本地替身 gitmoot/sandboxd），k8s agent-sandbox 也有"把 envd 当容器入口"的示例 —— envd 正在变成"沙箱里跑进程和读写文件"的通用协议。
- 来源：[e2b-dev/runtime](https://github.com/e2b-dev/runtime)、[DeepWiki 架构](https://deepwiki.com/e2b-dev/E2B/1.1-system-architecture)、[envd-sandbox 示例](https://agent-sandbox.sigs.k8s.io/docs/use-cases/examples/envd-sandbox/)、[自托管运维](https://bex.co/blog/2026/09/13/e2b-self-hosted-agent-sandbox-beyond-firecracker)

### 2.4 Anthropic sandbox-runtime（srt）与 Claude Code 的 `sandbox` 设置
- `@anthropic-ai/sandbox-runtime`（Apache-2.0）：不用容器的 OS 级沙箱，可包裹任意进程（`srt <cmd>`）。macOS 用 Seatbelt（sandbox-exec），Linux/WSL2 用 bubblewrap + 网络命名空间 + socat，Windows（alpha）用 WFP 出口栅栏。
- 网络：默认拒绝，所有流量经宿主侧 HTTP/SOCKS5 代理按域名白名单放行（Linux 上代理以 unix socket 绑定进沙箱）；白名单热更新；写默认拒绝、读默认放行；官方明说"不是完整隔离边界"，且沙箱内命令继承父进程环境变量（含密钥）。
- Claude Code 的 `sandbox` 设置（settings.json）：`enabled`、`autoAllowBashIfSandboxed`、`allowUnsandboxedCommands`、`excludedCommands`、`network.allowedDomains`、`failIfUnavailable` 等。**只沙箱 Bash 工具**，不是整个 CC 进程。
- 对我们：零基础设施的"进程级"一档；也是网络策略（域名白名单 + 宿主代理）的现成实现。
- 来源：[CC sandboxing 文档](https://code.claude.com/docs/en/sandboxing)、[settings 参考](https://code.claude.com/docs/en/settings-reference)、[sandbox-runtime](https://github.com/anthropics/sandbox-runtime)、[MarkTechPost](https://www.marktechpost.com/2026/10/24/anthropic-sandbox-runtime-ai-agent-sandbox/)

### 2.5 其它运行时（隔离强度从弱到强）
- 进程级：srt / bubblewrap / Seatbelt / Landlock。
- 容器：Docker / Podman（runc）；**gVisor**（runsc，用户态内核）。
- microVM：**Firecracker**（Lambda、Fargate、Fly、Vercel Sandbox、E2B、Cloudflare Sandbox SDK 的底座）、**Kata Containers**（containerd shim，VM 级）、**libkrun**（microsandbox 自托管）、Cloud Hypervisor、Apple Virtualization/Containerization（macOS）。
- 托管服务：E2B、Daytona、Modal、Vercel Sandbox、Cloudflare Sandbox、Blaxel、Tencent CubeSandbox、NVIDIA OpenShell 等，各自 SDK。
- 2026 年的共识（摘要）：跑 AI 生成代码或多租户时，最低一档是 microVM；但 microVM 也有逃逸 CVE（Firecracker 2026 年两起），补丁节奏要看运营方。
- 来源：[DEV 2026 指南](https://dev.to/aiagentengineering/how-to-sandbox-ai-agents-in-2026-firecracker-gvisor-runtimes-isolation-strategies-14pk)、[Upstash 对比](https://upstash.com/blog/best-sandbox-providers-for-ai-agents)、[沙箱清单 gist](https://gist.github.com/wincent/2752d8d97727577050c043e4ff9e386e)、[arXiv 2606.08433 安全对比](https://arxiv.org/pdf/2606.08433)

## 3 结论：能对齐什么

| 我们要的 | 可借的事实标准 | 说明 |
|---|---|---|
| 沙箱与进程的生命周期 | **CRI 的形状**（两级对象、List 按标签找回、能力上报、gRPC 进程外插件） | 不直接实现 CRI（太大、面向容器镜像和 Pod），只借结构 |
| 沙箱里起进程、读写文件 | **envd 的 Process/Filesystem 服务** | 动词集合几乎就是我们要的；远程运行时可直接对接 E2B 协议 |
| 权限与出口策略、凭据 | **Sandbox Kit 的 capability 词汇** + **srt 的域名白名单/宿主代理模型** | 请求-授予、凭据由代理注入、deny 优先 |
| 选择隔离强度 | **RuntimeClass** 思路 | 项目设置里选"运行环境"，上层不变 |
| 集群/远程 | **k8s Sandbox CRD** 作为一个适配器的后端 | 不作为我们的接口本身 |

没有一个现成标准能直接当我们的接口：k8s Sandbox 在编排层、Kit 在打包层、envd 是单一厂商协议、srt 只管进程级。所以做法是**定义我们自己的薄接口，但每一块都对齐上面的现成词汇**，让适配器薄。
