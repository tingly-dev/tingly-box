# 线程运行时：可插拔的隔离层（Thread Runtime Interface）

> 状态：**提案**，未实现。调研见 [../.research/agent-sandbox-landscape.md](../.research/agent-sandbox-landscape.md)。
> 相关：[workspace.md](workspace.md)（工作区与 worktree）、[thread-permissions.md](thread-permissions.md)（权限，与隔离正交）、[github-tools.md](github-tools.md)（宿主代做的 GitHub 动作）、[thread-lifecycle.md](thread-lifecycle.md)（线程状态机）、[../../.design/host-binding.md](../../.design/host-binding.md)（本地监听地址）。
> 用户原话：“我们现在都是裸机跑 projects，未来希望引入 microvm 或者 sandbox，但不希望绑定，而是有一个标准层，就像 k8s CRI CNI 一样。”

## 0 结论

- 定义一层薄接口 **TRI（Thread Runtime Interface）**：线程的 Claude Code 进程跑在一个“沙箱”里，沙箱由可替换的**运行时**提供。今天的裸机运行就是其中一个实现（`host`），行为逐字节不变，相当于 k8s 里的 runc。
- 照 CRI/CNI/CSI 拆成四块正交的职责：**生命周期与进程**（CRI 形）、**连通性**（宿主服务映射进沙箱 + 出口策略 + 凭据代理，CNI 形）、**工作区**（worktree 怎么给到沙箱，CSI 形）、**环境**（镜像或宿主）。运行时上报**能力**，Projects 按能力决定怎么用，不假设。
- **编排留在 Projects**（线程状态机、队列、审批、报告、GitHub 工具），运行时只给最小动词。**凭据不进沙箱**：GitHub token 留在宿主（工具已经在宿主推送），模型走网关的线程级 token，MCP 走能力 URL。
- 不发明新词汇：生命周期借 CRI 的结构，沙箱内进程/文件借 E2B envd 的动词，策略借 Docker Sandbox Kit 的 capability 词汇和 srt 的域名白名单模型。
- 先内置接口（Go），再开放进程外插件协议（gRPC over unix socket，同 CRI），让 Docker/Podman、Docker Sandboxes、microsandbox、E2B、k8s agent-sandbox 等成为外部适配器。
- 路线：P0 只抽缝不改行为 → P1 进程级（srt）→ P2 容器 → P3 microVM → P4 远程。每一步都可单独交付。

## 1 现状：线程进程从宿主拿了什么

（代码路径以函数名为准；2026-11 的 main。）

**进程。** 唯一的创建点是 `process.Factory.Start(ctx, LaunchSpec)`（`agentboot/process`），`LaunchSpec{Command, Env, WorkDir, InitialInput, Stderr}`，返回 `Handle{Stdin, Stdout, Wait, Kill, Done}`。链路：`Threads.turnLaunch` → `engine.Start` → `Runner.Open` → `claude.Driver.Prepare`（拼参数、`os.Stat` 检查 cwd、读设置文件内联进 argv）→ `OSExecFactory.Start`。I/O 是 stdin/stdout 的 stream-json，审批（`can_use_tool`）也走 stdio，由 `threadPrompter.OnApproval` 回答。进程常驻在 `pool.Pool`，`launchSignature` 变化则带 `--resume` 重开。`projects/internal/project/agents.go` 目前写死 `claude.NewAgentWithConfig`，没有把 Factory 注入进来。

**文件。**

| 路径 | 谁写 |
|---|---|
| worktree：`<repo>/.claude/worktrees/<slug>`（`git worktree add`，`.git` 文件指回主仓库的 `.git/worktrees/…`） | 宿主建，进程改 |
| 线程设置 `<root>/<proj>/threads/<id>.settings.json`、MCP 配置临时文件 | 宿主 |
| memory、library、`library/outputs/<slug>`、`library/.tmp/<thread>`（TMPDIR）、`library/.cache` | 宿主建，进程读写 |
| 项目其它仓库（`additionalDirectories`） | 进程读 |
| `~/.claude/projects/…` 转录、`~/.claude.json` | 进程写，宿主 `SessionReader` 读；**没有设 CLAUDE_CONFIG_DIR，用的是宿主用户的 `~/.claude`** |

**网络。** MCP：`127.0.0.1:<随机端口>/<32 字节能力 token>`（`sessionToolsMCP`）；LLM：`ANTHROPIC_BASE_URL` 指向 Tingly Box 网关 + `ANTHROPIC_AUTH_TOKEN`（`ThreadEnv`）；GitHub：工具在宿主上推送（`githubRemote` 用 `GitHubAuth.Credential` 的 token），`gh api` 也在宿主跑；进程自己的 Bash 能用宿主上的一切凭据（SSH key、gh 登录）和网络。

**宿主代做（直接在 `t.Cwd` 跑 git）。** worktree 增删与清理（`thread_cleanup.go`）、`Stats`、PR diff（`LocalPRDiff`、`fetchBase`）、`MergeDraft`、`originRepo`、推送（`pushBranch` 用 alternates 指向 worktree）、`send_file` 读文件、memory/library 读写。`git()` 直接 `exec`，没有缝。

**已有的缝：** `process.Factory`（最贴近 CRI）、`agentboot.AgentDriver.Prepare`、`ThreadEnv`/`Launcher`/`MCPLauncher`（`launch.go`）、`GitHub`/`GitHubAuth`。**缺的缝：** `Layout` 是具体结构体；`git()` 没有抽象；`SessionToolsConfig` 是函数钩子。

**结论：** 换运行时的难点不在“怎么起进程”（已有 Factory），而在三处**宿主假设**：路径（宿主路径直接进 argv 和设置）、回环地址（MCP、网关在 127.0.0.1）、宿主代做（git 直接在 worktree 里跑）。接口要正面处理这三处。

## 2 业界怎么做“不绑定”（摘要）

详见调研文件。要点：

- **CRI**：kubelet ↔ 运行时的 gRPC；两级对象（PodSandbox / Container）；`ListPodSandbox` 按标签找回；`Status` 报告能力；`RuntimeClass` 选实现（runc / gVisor / Kata）。上层不变，换芯。
- **CNI**：exec 插件 + stdin JSON；与运行时正交；能力声明后才传参（`capabilities`）。
- **agent 沙箱领域没有统一运行时 API**：k8s Agent Sandbox（`Sandbox` CRD，编排层，隔离靠 RuntimeClass）；Docker Sandbox Kit v3（权限请求打进 OCI 镜像，宿主授予，凭据代理注入，拟捐 CNCF，仅 Docker Sandboxes 一家合规）；E2B envd（VM 内 Process/Filesystem 的 Connect RPC，已有第三方兼容实现，正在变成事实协议）；srt / Claude Code `sandbox` 设置（进程级，域名白名单 + 宿主代理，CC 里只包 Bash）。

所以：**自己定义薄接口，每一块对齐现成词汇**，让适配器薄；不直接实现 CRI（面向 Pod 与镜像，太重），也不绑定任何一家的 SDK。

## 3 原则

1. **编排在上，动词在下。** 线程状态机、队列与并发上限、审批卡、报告、GitHub 工具都留在 Projects；运行时只负责“给我一个隔离环境，在里面跑这个进程，把这些东西接进去”。
2. **宿主实现是参考实现。** `host` 运行时就是今天的行为；接口落地时 golden/harness 不变是回归证据。
3. **能力协商，不假设。** 运行时上报能力（隔离等级、工作区方式、出口策略、凭据代理、暂停/快照、宿主服务映射）；Projects 按能力选路径，缺能力时明说，不静默降级。
4. **凭据不进沙箱。** 能在宿主做的有凭据动作（GitHub 推送与 API）留在宿主，经工具完成；必须在沙箱里用的（LLM）给线程级、可撤销的 token；可选的凭据代理按 Kit 的方式注入（沙箱只见占位符）。
5. **隔离与权限正交。** 沙箱决定“出了事能坏多少”，权限模式与审批决定“做之前问不问”。两者分开设置、分开显示（UX 原则 4：正交维度分轴）。将来可以让强隔离档位默认更宽的权限模式，但那是项目设置，不是运行时偷偷改。
6. **失败可见，不回退到裸机。** 选了沙箱而运行时不可用，线程启动失败并说明原因与修法；绝不静默在宿主上跑（安全语义不能被降级）。

## 4 分层

| 层 | 类比 | 管什么 | 谁实现 |
|---|---|---|---|
| **Runtime**（生命周期与进程） | CRI RuntimeService | 沙箱的建、查、停、删、（暂停/恢复）；在沙箱里起进程，stdio 流、信号；沙箱内文件读写 | 运行时适配器 |
| **Connectivity**（连通性） | CNI | 把宿主服务（MCP、网关）映射进沙箱并给出沙箱内地址；出口策略（域名白名单）；可选凭据代理 | 运行时自带，或独立插件（如 srt 代理） |
| **Workspace**（工作区） | CSI | 线程的工作目录怎么给到沙箱：挂载宿主目录 / 拷入 / 沙箱内克隆；宿主代做的 git 在哪跑 | Projects 内的 provider，按运行时能力选 |
| **Environment**（环境） | OCI image / RuntimeClass | 沙箱里有什么：宿主本身，或一个 OCI 镜像（含 `claude`、git、工具链） | 项目设置 + 运行时 |

Projects 这边的对象只有一个：**一个线程一个沙箱**（见 §6 D1），线程的常驻 Claude Code 进程是沙箱里的一个进程。协调者不进沙箱（它只用 MCP 工具，没有 Bash/Edit）。

## 5 接口草案

Go 接口放在新包 `projects/internal/runtime`（或 agentboot，若 Tingly Box 自身的 @cc 会话也要用；见 §8 未决）。字段名示意，落地时再收敛。

```go
// Runtime gives threads an isolated place to run. host is today's behaviour.
type Runtime interface {
    // Info is the runtime's name, version and what it can do.
    Info(ctx context.Context) (Info, error)

    // Sandbox lifecycle: one per thread, found again by labels after a restart.
    Create(ctx context.Context, spec SandboxSpec) (Sandbox, error)
    List(ctx context.Context, labels map[string]string) ([]Sandbox, error)
    Stop(ctx context.Context, id string) error   // processes end, state kept
    Remove(ctx context.Context, id string) error // everything goes
    // Pause / Resume only when Info.Caps.Pause.

    // Exec starts a process in the sandbox: the same Handle agentboot uses today,
    // so process.Factory becomes a thin wrapper (Start = Exec in the thread's sandbox).
    Exec(ctx context.Context, id string, p ProcSpec) (process.Handle, error)

    // Files reach the sandbox's own filesystem when the workspace is not a host mount
    // (Info.Caps.Workspace has no "mount").
    Files(id string) FS // Stat, ReadFile, WriteFile, ReadDir, Remove (envd's verbs)
}

type Info struct {
    Name, Version string
    Isolation     string   // none | process | container | vm | remote
    Caps          Caps
}

type Caps struct {
    Workspace    []string // mount | copy | clone
    HostServices bool     // can map a host address into the sandbox
    Egress       string   // open | allowlist
    CredProxy    bool     // can inject a credential into requests to named hosts
    Pause, Snapshot bool
    Images       bool     // takes an OCI image; false: the host's own environment
}

type SandboxSpec struct {
    Labels    map[string]string // project, thread
    Image     string            // "" = host environment
    Mounts    []Mount           // HostPath, GuestPath, ReadOnly (workspace, library, memory, config dir)
    Services  []Service         // Name, HostAddr → Sandbox.Endpoints[Name] is the in-sandbox URL
    Egress    *Egress           // AllowedDomains, DeniedDomains (Kit network-policy / srt)
    Creds     []Credential      // Host, Header, SecretRef: injected by the proxy, never in env
    Env       []string
    Resources *Resources        // CPU, Memory, Disk
}

type Sandbox struct {
    ID        string
    Labels    map[string]string
    State     string            // running | stopped | paused
    Endpoints map[string]string // service name → URL inside the sandbox
    Paths     map[string]string // host path → guest path, for argv and settings
}

type ProcSpec struct {
    Command []string
    Env     []string
    WorkDir string // guest path
}
```

**两个改造点**（P0 要做的“缝”）：

1. **路径翻译。** `Driver.Prepare` 现在把宿主路径写进 argv（`--settings`、`--mcp-config`、cwd）并 `os.Stat`/`ReadFile` 它们。改为：Prepare 产出的 LaunchSpec 里的路径经 `Sandbox.Paths` 翻成沙箱内路径；需要内联的文件内容仍由宿主读（宿主有它）。`host` 运行时的映射是恒等。
2. **宿主代做的 git 走一个 `Workspace` 接口。** `git()` 的调用点（Stats、diff、MergeDraft、originRepo、推送前的 rev-parse）改成 `ws.Git(ctx, args...)`；挂载模式下仍是宿主 exec（行为不变），沙箱内工作区模式下走 `Runtime.Exec`。推送仍在宿主：`pushBranch` 需要对象库，沙箱内工作区时先由 `Exec` 打包（`git bundle`）取回宿主再推，或在宿主保留一份镜像克隆（§6 D2）。

**进程外插件协议（P2 起）。** 把上面的接口原样定义为 protobuf（`projects/api/runtime/v1`），gRPC over unix socket；Projects 启动时按配置连接 `<data>/runtimes/<name>.sock` 或启动插件可执行文件（插件自己监听 socket）。选 gRPC 而不是 CNI 式 exec 插件：要双向流（stdio）和长连接，CRI 也是这么选的。`host` 与 `srt` 两个内置实现不经过 socket。

## 6 关键决策（带推荐）

**D1 粒度：一个线程一个沙箱。** 线程是工作与权限的单位，有自己的 worktree、设置、TMPDIR、转录；常驻进程（`pool.Pool`）正好映射成“沙箱里的一个长进程”。线程 Stop → `Runtime.Stop`（或 Pause，若支持）；用户关闭线程（resolve）→ `Remove`；Projects 重启后用 `List(labels{thread})` 找回，`--resume` 照旧。备选“一个项目一个沙箱”省资源，但线程之间不再隔离、权限与工作区边界混在一起，不推荐。协调者与 folder 会话（IM）：协调者不进沙箱；folder 会话跟随它的项目设置。

**D2 工作区：本地运行时挂载，远程运行时沙箱内克隆；不挂整个仓库。**
- 今天的 worktree 的 `.git` 是指回主仓库的文件，进程要读写主仓库的 `.git`（对象库、其它线程的分支、所有 refs）。把 worktree 挂进沙箱就必须连主仓库 `.git` 一起挂，**隔离被打穿**（能改别的线程的分支、能读 `.git/config` 里的凭据配置）。
- 推荐：隔离档位（process 以上）给线程一个**独立的克隆**：`git clone --shared`/`--reference <主仓库>`（对象只读借用，refs 自己的），放在宿主的线程目录，整体挂载进沙箱，主仓库的 `objects/` 以只读挂到同一路径（alternates 要能读到）；沙箱里看不到主仓库可写的部分。（新线程的分支从主仓库取；线程推到 origin 后主仓库 `fetch` 即可看到，和今天经 origin 交接一样。）宿主代做的 git（diff、Stats、推送用 alternates）对这个克隆照常工作，改动最小。
- 远程运行时（P4）宿主不能挂载：沙箱内 `git clone`（凭据：只读的、短期的 token，或经凭据代理），宿主代做的 git 走 `Workspace` → `Runtime.Exec`；推送经 `git bundle` 取回宿主再由工具推。
- `host` 档位保持 worktree，不变。

**D3 宿主服务进沙箱：运行时提供映射，地址由运行时回给我们。** MCP 与网关今天都在 127.0.0.1。`SandboxSpec.Services` 声明 `{mcp: 127.0.0.1:p1, gateway: 127.0.0.1:p2}`，运行时负责接通并返回沙箱内 URL：进程级（srt）同一网络命名空间外，用它的 unix socket 代理或直接放行回环；容器用 host-gateway 或 unix socket 挂载；microVM 用 vsock/tap 上的转发；远程用反向隧道。能力 URL 的 token 仍是 MCP 的唯一认证，远程档位必须加 TLS（隧道自带或 mTLS）。`.design/host-binding.md` 的“只听 127.0.0.1”不变：映射是运行时在宿主侧建的转发，不需要把监听改成 0.0.0.0。

**D4 凭据：GitHub 只经工具；模型 token 线程级；其它走凭据代理（可选）。**
- 沙箱里没有 gh 登录、没有 SSH key：GitHub 写只能经 Projects 工具（在宿主执行、按项目设置审批）。这与 `gh` 引导（PR #2773 的 D）天然一致：沙箱里的 `gh` 没凭据，读走 `view`。
- `ANTHROPIC_AUTH_TOKEN` 今天是共享的模型 token；进沙箱前应改为**线程级、可撤销**的网关 token（沙箱被攻破时能单独吊销）。网关侧需要支持签发；这是一个独立前置项。
- 包管理、私有源等需要的凭据：用 Kit 的 `credential@1` 语义，由代理按域名注入，沙箱只见占位符；运行时不支持凭据代理（`Caps.CredProxy=false`）时，项目设置里明确告知“这些凭据会以环境变量进入沙箱”，由用户决定。

**D5 Claude Code 状态：每线程一个配置目录，放在宿主、挂进沙箱。** 今天用宿主用户的 `~/.claude`（转录、`.claude.json`、认证、全局设置），进沙箱后既不该共享（隔离、并发写）也不能丢（宿主 `SessionReader` 要读转录，`--resume` 要它）。推荐 `CLAUDE_CONFIG_DIR=<root>/<proj>/threads/<id>/claude`（宿主路径，挂载进沙箱；远程档位由 `Files` 同步回来）。这一条对 `host` 档位也有益（也是 `cc-native-peer-messaging` 记忆里“按配置目录注册对等会话”的问题的根治），但会改变现有行为（全局设置、登录态不再继承），要单独决定，见 §8。

**D6 镜像与 `claude` 二进制：镜像契约，不注入宿主二进制。** 容器/VM 档位的镜像必须提供：`claude`（PATH 上）、`git`、`sh`、项目需要的工具链。Projects 在建沙箱后 `Exec claude --version` 校验，不满足就失败并说明。注入宿主二进制只在同架构同 libc 时可行，不作为契约。提供一个官方基础镜像（Claude Code + git + 常见工具链），项目可以 `FROM` 它。权限请求将来可以按 Kit 规范写在镜像里（`kit.yaml`），宿主按项目设置授予——这让“沙箱里能连哪些域名”随镜像版本走。

**D7 插件形态：先内置，后开放。** P0–P1 只有进程内 Go 接口与内置 `host`、`srt` 两个实现，先把缝和语义磨稳；P2 起把接口冻结成 `runtime/v1` protobuf，外部适配器通过 unix socket 接入（同 CRI 的 `--container-runtime-endpoint`）。适配器候选：`docker`/`podman`（容器）、Docker Sandboxes（microVM，Kit 合规）、microsandbox（libkrun）、E2B 协议（自托管或云）、k8s agent-sandbox（远程）。版本协商：`Info.Version` + 能力集，Projects 拒绝主版本不符的插件。

**D8 权限模式与审批不动。** 审批仍走 stdio 的 `can_use_tool` → `threadPrompter`，与沙箱无关；`request_permission` 的规则授予照旧写进设置文件。沙箱只是让“万一放行了坏东西”的后果有界。是否在强隔离档位默认 `bypassPermissions`，作为项目设置的一个选项，默认不开。

## 7 路线

| 阶段 | 内容 | 隔离 | 行为变化 | 验证 |
|---|---|---|---|---|
| **P0 抽缝** | 新包 `runtime` 与 `host` 实现；`agents.go` 注入 `process.Factory`（= `host.Exec`）；`Driver.Prepare` 走路径翻译；`git()` 调用点收进 `Workspace`；`Layout` 不动 | none | 无 | 现有测试、golden、harness 全绿即证据 |
| **P1 进程级** | 内置 `srt` 运行时：用 sandbox-runtime 包住整个 `claude` 进程（不只 Bash）；独立克隆工作区（D2）；出口白名单（项目设置，默认含网关、MCP、包源）；可选 per-thread 配置目录（D5） | process | 选了才变 | 真实 CLI 跑 harness 旅程；越权写/出网被拦的用例 |
| **P2 容器** | 冻结 `runtime/v1` 协议；外部适配器 `docker`/`podman`；镜像契约与基础镜像（D6）；服务映射（D3） | container | 同上 | 同一组旅程在容器里跑通 |
| **P3 microVM** | 适配器：Docker Sandboxes / microsandbox / Firecracker（E2B 协议）/ Kata（经 containerd）；暂停与恢复映射线程空闲 | vm | 同上 | 冷启动、恢复时间进 harness 报告 |
| **P4 远程** | 适配器：k8s agent-sandbox、E2B 云；沙箱内克隆（D2）、隧道与 TLS（D3）、`Files` 同步转录（D5） | remote | 同上 | 远端集群上的旅程 |

P1 也可以先做更轻的一步：只在线程设置里打开 Claude Code 自己的 `sandbox`（只包 Bash，配置 `network.allowedDomains`、`failIfUnavailable`）。它不需要运行时接口，可作为 P0 之前的快速收益，但它不隔离 CC 进程本身（Edit/Write 仍按权限规则），不能代替 P1。

## 8 UX

- **项目设置加一行“运行环境”**：`本机（无隔离）` / `进程沙箱` / `容器` / `虚拟机` / 外部运行时名。显示**具体值**（UX 原则 5）：运行时名、版本、隔离等级、镜像。默认 `本机`，与今天一致（原则 6：合理默认值）。
- **线程卡片与详情显示它在哪跑**（一个小标签：`本机` / `vm · docker-sandboxes`），与权限模式分开显示（原则 4）。
- **出口白名单**在同一设置页，带“这次被拦的域名”的一键加入（来自运行时的拦截事件），不让用户去翻日志。
- **运行时不可用**：线程启动失败，卡片说明哪一步失败（找不到 socket、镜像缺 `claude`、虚拟化不可用）和修法；**不回退到本机**（原则 6 的例外：安全设置不做静默默认）。
- 切换运行环境只影响新线程和下一次启动的线程；正在跑的线程不迁移（与权限模式“下一回合生效”的做法一致）。

## 9 未决（需要用户决定）

1. **接口放哪**：`projects/internal/runtime`（只给 Projects），还是 `agentboot`（Tingly Box 的 @cc 远程会话、IM 会话也能用）？推荐 agentboot：进程创建本来就在那里，IM 会话已经是 Projects 线程。
2. **per-thread 配置目录（D5）在 `host` 档位是否也启用**：好处是隔离与并发安全；代价是线程不再继承用户的全局 Claude Code 设置与登录态（走网关时登录态不需要）。
3. **模型 token 线程级化（D4）**：需要网关支持签发与吊销，是否作为 P1 的前置。
4. **首个外部目标**：容器（docker/podman，覆盖面最广）还是直接 microVM（Docker Sandboxes 或 microsandbox，隔离最强）？推荐先容器把协议跑通，再接 microVM。
5. **Kit 规范**：是否在 P2 就让镜像带 `kit.yaml` 表达权限请求，还是先用项目设置表达、等规范稳定（目前仍标实验性）再对齐。

## 10 不做

- 不实现 CRI 本身、不依赖 Kubernetes（远程档位里 k8s 只是一个适配器后端）。
- 不让运行时决定权限或审批。
- 不迁移运行中的线程；不做跨运行时的快照迁移。
- 不在沙箱里放 GitHub 凭据（工具在宿主做）。
