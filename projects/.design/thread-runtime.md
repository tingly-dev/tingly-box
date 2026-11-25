# 线程隔离：安全模型、最小契约与统一接口

> 状态：**提案**；Linux 第一版（bubblewrap，网络放通）**已实现**，隔离层是独立的 `sandbox/` 模块、五轴行为模型，见 §11（§10.6 为第一版记录）。调研见 [../.research/agent-sandbox-landscape.md](../.research/agent-sandbox-landscape.md)。
> 相关：[thread-permissions.md](thread-permissions.md)（权限，与隔离正交）、[github-tools.md](github-tools.md)（宿主代做的 GitHub 动作）、[workspace.md](workspace.md)、[thread-lifecycle.md](thread-lifecycle.md)。
> 用户原话：“我们现在都是裸机跑 projects，未来希望引入 microvm 或者 sandbox，但不希望绑定，而是有一个标准层”；“我们要设计的其实是安全模型，和统一接口，和保障这些的最小化模型，然后去对接行业里的各种 microvm 或者 sandbox（不是说生搬硬套 docker k8s 等等）”；“仓库共享目前还不是最大问题……可以不过度设计（除非做不到）”。

## 0 结论

1. **安全模型**：线程的 agent（Claude Code 进程和它起的一切）是**不可信主体**——它读的仓库、网页、issue 评论都可能带提示注入，它装的依赖可能有恶意脚本。Projects 宿主、网关和用户是可信的。我们给出七条保障（§1.3），核心是：**看不到宿主的东西、手里没有宿主的凭据、出不去没批准的网、有外部副作用的事只能经宿主的工具做**。
2. **最小契约**：要求运行时做的事压到四条，各家都做得到——**只挂声明的目录、没有网络、跑一个带 stdio 的进程、能整体杀掉销毁**（§2）。其余一切由 Projects 自己做：沙箱里放一个我们的小代理（`tb-sandboxd`），经那条 stdio 多路复用出“到宿主的唯一通道”；宿主侧的**出口代理**负责域名白名单、凭据注入、到网关和 MCP 的路由、拦截记录。安全策略因此集中在我们手里，在所有运行时上一致、可测。
3. **统一接口**：一个很小的 `Sandbox` 接口（§3）+ 一套**一致性测试**（§5）。测试就是“标准”：适配器通过它，才能被标成相应的隔离等级。
4. **对接**：Firecracker、Cloud Hypervisor、libkrun/microsandbox、Apple Containerization、Kata、gVisor、bubblewrap/Landlock、Seatbelt、E2B、Docker Sandboxes 等都能满足四条契约，差别只在“怎么跑起第一个进程”和隔离强度（§4）。
5. **不过度设计**：同仓库线程共享主仓库 `.git`、远程运行时的文件同步、暂停与快照、进程外插件协议，都记为已知缺口或后续，不影响模型成立。

## 1 安全模型

### 1.1 保护什么

| 资产 | 例子 |
|---|---|
| 宿主用户的机器 | 项目以外的文件、`~/.ssh`、云凭据、浏览器数据、其它项目 |
| Projects/Tingly Box 自己的秘密与数据 | GitHub token（`gh auth token`）、模型 key、网关 token、IM 机器人 token、数据库、其它线程的 MCP 能力 URL |
| 外部世界里以用户名义的动作 | GitHub 写操作、合并、发消息、花钱（模型额度） |
| 其它线程 | 它们的工作目录、进程、能力 |
| 宿主可用性 | CPU、内存、磁盘、进程数 |

### 1.2 信任边界

- **不可信**：线程沙箱里的一切。威胁来源：提示注入（仓库内容、网页、`view` 返回的他人文本）、恶意依赖、模型自己犯错。不假设攻击者能控制模型以外的宿主进程。
- **可信**：Projects 服务、网关、用户、宿主侧的工具实现。协调者不在沙箱里（它没有 Bash/Edit，只调 MCP 工具），算可信侧的编排者。
- 边界上只有一条通道：沙箱 ↔ 宿主的 `tb-sandboxd` 通道。所有进出都经过它，或者被运行时拒绝。

### 1.3 保障

| # | 保障 | 由谁保证 |
|---|---|---|
| **G1 文件受限** | 沙箱只看见声明的挂载（工作区读写；library/memory 按设置；线程自己的 TMPDIR、Claude Code 配置目录），宿主其余文件一概不可见 | 运行时（R1） |
| **G2 无环境凭据** | 沙箱的环境变量与文件里没有宿主秘密；只有线程级、可撤销、范围最小的能力：MCP 能力 URL、给网关用的**占位 token**（真 token 由出口代理注入） | Projects（构造环境；今天的 `GetCleanEnv` 是复制 `os.Environ()`，隔离档位改为白名单） |
| **G3 出口受控** | 默认无网络；能出去的只有经通道到宿主出口代理的连接：网关、MCP、项目白名单域名（CONNECT 隧道）；其余拒绝并记录 | 运行时（R2：没有网络）+ Projects（出口代理） |
| **G4 副作用经工具** | GitHub 写、合并、关闭等只能经宿主上的 Projects 工具发生，从而受项目设置与审批约束 | G2+G3 的推论（沙箱里没有 GitHub 凭据，`api.github.com` 不在默认白名单）+ 工具 |
| **G5 线程互隔** | 一个线程看不到、碰不到别的线程的沙箱、进程、能力 | 运行时（每线程一个沙箱）+ Projects（能力 URL 按线程） |
| **G6 资源有界、可整体终止** | 能限 CPU/内存/磁盘/进程数；杀掉时沙箱里所有进程都结束，不留孤儿 | 运行时（R3、R4） |
| **G7 可见** | 被拦的出口、失败的启动能显示给用户，并能一键放行（加白名单） | Projects（出口代理的记录） |

### 1.4 明确不保障（已知缺口）

- **内核/虚拟化逃逸**：进程级沙箱与宿主共用内核，VM 级不共用；G1–G6 的强度随隔离等级变化（§4 标注），我们只如实标注等级，不承诺挡住逃逸。
- **同仓库线程共享主仓库 `.git`**：worktree 要读写主仓库的 `.git`，挂进沙箱后同仓库的线程能看到、改到彼此的分支与 refs（G5 的缺口）。现在接受；以后如需要，改成每线程独立克隆即可，不影响接口。
- **白名单域名内的数据外泄**：允许了 `registry.npmjs.org`，就可能经它的上传接口外泄。白名单是用户的决定，界面要让人看清放了什么。
- **模型额度滥用**：在额度内的消耗不拦；靠网关的线程级 token 与限额。
- **侧信道、时间信道**：不处理。

## 2 最小契约：运行时只做四件事

行业里各家能力差别很大（有的有原生 exec 和端口映射，有的只能起一个进程；有的能挂目录，有的不能；网络策略五花八门）。要不绑定，就只要求它们的**交集**，把策略留在我们这边：

| # | 运行时必须做到 | 支撑的保障 |
|---|---|---|
| **R1 只挂声明的目录** | 沙箱的文件系统 = 环境（镜像或宿主只读系统目录）+ 我们声明的挂载（宿主路径 → 沙箱路径，读写/只读）；别的宿主路径不可达 | G1 |
| **R2 没有网络** | 沙箱里没有可用的出网接口（只有回环），也连不到宿主的网络服务 | G3、G4 |
| **R3 起一个进程并给出 stdio** | 以给定的 argv/env/cwd 起**一个**进程（我们的 `tb-sandboxd`），把它的 stdin/stdout 交给我们 | 通道的载体 |
| **R4 整体杀掉与销毁** | 一次调用结束沙箱里的所有进程并释放沙箱；可选资源上限 | G6、G5 |

R2 是关键的减法：我们**不要求运行时实现域名白名单、凭据注入、端口映射**——这些各家不一致，有的根本没有。沙箱完全断网，唯一的出口是 R3 那条 stdio，由我们的 `tb-sandboxd` 在里面多路复用：

```
沙箱内                                    宿主
claude ──stdio──┐                         ┌── Projects 引擎（stream-json，原样）
                │                         │
 127.0.0.1:7000 ┤  tb-sandboxd  ══stdio══ ┤  通道 ─┬─ 网关（注入真 token）
 (HTTP 入口)    │  （多路复用）           │       ├─ MCP（按线程能力 URL）
                └─ 信号/退出码            │       └─ 出口代理（白名单 CONNECT，拒绝并记录）
```

- `tb-sandboxd` 是我们发的一个静态小二进制（Go），职责只有三样：起 `claude` 并转发它的 stdio；在沙箱回环上开一个 HTTP 入口，把连接经通道送到宿主；转发信号与退出码。它不做任何安全判断——判断全在宿主侧。
- 沙箱里 `ANTHROPIC_BASE_URL=http://127.0.0.1:7000/gateway`、MCP 地址 `http://127.0.0.1:7000/mcp/<能力>`、`HTTPS_PROXY/HTTP_PROXY=http://127.0.0.1:7000`（npm、pip、go、git 都认）。不认代理的工具连不出去——这是失败即安全（fail closed），被拦的目标进 G7 的记录。
- 通道协议：stdio 上跑一个成熟的多路复用（如 yamux），流类型只有三种：`proc`（claude 的 stdio）、`connect`（一个出站连接，带目标）、`ctl`（信号、退出、心跳）。宿主把 `proc` 流当作今天的 `process.Handle` 交给 agentboot，引擎和 stream-json 解析不变。

**为什么这是“最小”**：四条里任意去掉一条，保障就有一条无法成立（没有 R1 就没有 G1；有网络就绕过出口代理，G2–G4 失效；没有 R3 就没有通道；没有 R4 杀不干净）。而再多要求一条（比如原生端口映射、原生 exec、原生白名单），就会把一部分运行时排除在外。

**宿主运行时（今天的裸机）**不经过这些：它不满足 R1/R2，标为 `isolation: none`，行为与今天完全一样。

## 3 统一接口

> **已被 §11 取代**：实现时按用户要求把策略拆成五个轴，并放进独立的 `sandbox/` 模块。下面是最初的草案，留作对照。

```go
package sandbox

// Runtime is one way to run a thread's agent in isolation. Adapters implement
// R1–R4 and nothing else; policy lives on the host side (Broker).
type Runtime interface {
    Info() Info
    // Start creates the sandbox and starts spec.Entry (tb-sandboxd) in it.
    Start(ctx context.Context, spec Spec) (Box, error)
}

type Info struct {
    Name      string
    Isolation Isolation // none | process | userkernel | vm
    Platforms []string  // linux/amd64, darwin/arm64, …
}

type Spec struct {
    ThreadID  string
    Env       Environment // image ref or host root (read-only system dirs)
    Mounts    []Mount     // HostPath, GuestPath, ReadOnly — nothing else is visible (R1)
    Entry     []string    // argv of tb-sandboxd inside the sandbox
    Limits    *Limits     // optional: CPU, memory, disk, pids
}

type Box interface {
    Stdio() (io.WriteCloser, io.ReadCloser) // R3: the only channel
    Destroy(ctx context.Context) error      // R4: kills everything, releases the sandbox
    Done() <-chan struct{}
}
```

宿主侧（Projects，所有运行时共用）：

```go
// Broker is the host end of a box's channel: it runs the agent process stream
// and decides every outbound connection.
type Broker interface {
    Attach(box Box, t *db.Thread, policy Policy) (process.Handle, error)
}

type Policy struct {
    Gateway   GatewayRoute   // where /gateway goes and which token to inject
    MCP       string         // the thread's capability URL (host side)
    Allow     []string       // domains for CONNECT; deny by default
    OnBlocked func(Blocked)  // G7
}
```

接入点只有一个：agentboot 的 `process.Factory`（今天写死在 `projects/internal/project/agents.go` 的 `claude.NewAgentWithConfig`）。隔离档位的 Factory = `Runtime.Start` + `Broker.Attach`，返回的仍是 `process.Handle`；`host` 档位就是今天的 `OSExecFactory`。

配套要改的只有三处“宿主假设”：

1. **路径**：`Driver.Prepare` 把宿主路径写进 argv/设置文件。挂载时让沙箱路径与宿主路径**相同**（R1 的挂载按同路径挂），就不需要翻译——最省的做法。做不到同路径的运行时（少数）再加映射。
2. **环境变量**：隔离档位改成白名单构造（PATH、HOME、TMPDIR、LANG、上面的三个地址、占位 token），不再复制宿主环境（G2）。
3. **Claude Code 配置目录**：隔离档位给每个线程一个 `CLAUDE_CONFIG_DIR`（宿主线程目录下，同路径挂入），不让沙箱看到宿主的 `~/.claude`（里面有登录凭据和别的会话的转录，违反 G1/G2）。宿主读转录的 `SessionReader` 直接读这个目录。

宿主代做的 git（diff、Stats、推送）照旧在宿主的工作区上跑——工作区就是挂载的宿主目录，不需要进沙箱。

（线程设置文件、MCP 配置文件这类 argv 里出现的宿主文件，以只读同路径挂入；它们不含宿主秘密——MCP 配置里的地址在隔离档位改写为沙箱内入口。）

## 4 对接行业运行时

四条契约在各家怎么满足。“入口”指怎么把 `tb-sandboxd` 跑起来并拿到它的 stdio。标 ⚠ 的是未在本机验证、来自文档或二手资料的判断，适配器落地时由一致性测试确认。

| 运行时 | 隔离 | R1 挂载 | R2 断网 | R3 入口与 stdio | R4 销毁 | 备注 |
|---|---|---|---|---|---|---|
| **bubblewrap**（Linux） | process | `--ro-bind` 系统目录 + `--bind` 声明目录，其它不挂 | `--unshare-net` | 直接 exec，stdio 即进程 stdio | 杀进程组；`--die-with-parent` | 无需 root；可叠加 Landlock/seccomp。srt 在 Linux 上就是这条路 |
| **Seatbelt**（macOS `sandbox-exec`） | process | 生成 profile：只允许声明路径 | profile 拒绝网络 | 直接 exec | 杀进程组 | `sandbox-exec` 被标为已弃用但仍可用 ⚠；srt 在 macOS 上用它 |
| **gVisor**（`runsc`） | userkernel | OCI bundle 的 mounts | `--network=none` | `runsc run` 的 stdio | `runsc kill/delete` | 需要 OCI bundle（可由宿主根只读 + 挂载拼出） |
| **Firecracker** | vm | 只读 rootfs 镜像 + virtio-blk；目录共享需另想办法 ⚠ | 不配 tap 即无网 | `tb-sandboxd` 作为 guest 的 init，stdio 走 vsock（或串口） | 杀 VMM 进程 | 原生不支持 virtiofs ⚠：工作区需块设备或经通道同步，是对接成本最高的一家 |
| **Cloud Hypervisor** | vm | virtiofs（virtiofsd） | 不配网卡 | init + vsock | 杀 VMM | 比 Firecracker 好接：有 virtiofs |
| **libkrun / microsandbox** | vm | virtiofs | 默认会开 TSI（借宿主网络）！须显式关：`krun_disable_implicit_vsock` 后 `krun_add_vsock(ctx, 0)` ⚠ | `krun` 直接运行一个命令，stdio 可接 | 杀进程 | “默认借宿主网络”是 R2 的陷阱，一致性测试必须覆盖 |
| **Apple Containerization**（macOS） | vm（每容器一个） | virtiofs 挂载 | `--network none`（只剩回环） | 进程 stdio 经 vminitd/vsock 转发 | `container rm -f` | 已有项目在 `--network none` + vsock 中继下跑通白名单 CONNECT ⚠ |
| **Kata Containers** | vm | virtiofs | network none | 经 containerd/CRI 的 attach | 删容器 | 要 containerd；适合已有集群的环境 |
| **Docker/Podman**（runc） | process（共享内核） | `-v` 声明目录 | `--network none` | `run -i` 的 stdio | `rm -f` | 隔离弱于 VM；可换 `--runtime=runsc/kata` 提升 |
| **Docker Sandboxes**（`sbx`） | vm | 工作区同步/挂载 ⚠ | 自带默认拒绝的防火墙；需确认能否完全断网 ⚠ | exec 的 stdio ⚠ | 删除沙箱 | 它自己有一套 Kit 策略；我们只用它的“断网 + stdio”，策略仍在我们这边 |
| **E2B**（Firecracker，云或自托管） | vm（远程） | 无宿主挂载：工作区要经通道同步 | `allowInternetAccess:false`（= denyOut 0.0.0.0/0） | envd 的 `Process.Start` 流式 stdio | `kill` | 远程：R1 改为“只有我们同步进去的文件”；第三方兼容后端可能忽略断网标志，必须测 |
| **Kubernetes agent-sandbox** | 取决于 RuntimeClass | PVC/卷 | NetworkPolicy 拒绝全部出站 | `exec`/attach 的 stdio | 删 Sandbox | 远程；同上 |

**两类分别处理：**

- **本地运行时**（bwrap、Seatbelt、gVisor、Cloud Hypervisor、libkrun、Apple、Kata、Docker）：R1 用同路径挂载，工作区就在宿主，宿主代做的 git 不变。镜像只需带 `claude` 与 `tb-sandboxd`；process 档位直接只读挂宿主系统目录，连镜像都不需要。
- **远程运行时**（E2B、k8s、Docker Sandboxes 若不挂载）：没有宿主挂载。R1 变成“沙箱里只有我们放进去的东西”，工作区需要经通道同步（git bundle 进出，或文件同步流）。这是后续工作（§6），接口不变——`Spec.Mounts` 对它们表示“要同步的目录”。

**`claude` 与 `tb-sandboxd` 怎么进沙箱**：process 档位与宿主共享只读系统目录，二者都在；VM/容器档位由镜像提供（我们发一个最小基础镜像：`claude`、`tb-sandboxd`、git、sh），或在同架构时只读挂入宿主上的二进制。Projects 启动后经通道问一次版本，不对就失败并说明。

## 5 一致性测试就是标准

接口很小，真正约束适配器的是一套**对抗性一致性测试**（`sandbox/conformance`）：在沙箱里跑一个探针程序，逐条去破保障，全部被挡住才算通过。每个适配器在 CI（能跑的平台）和本地都跑它；没通过的适配器不能在设置里被选成隔离档位。

| 用例 | 期望 | 对应 |
|---|---|---|
| 读 `~/.ssh`、宿主的 `~/.claude`、Projects 数据目录、另一个线程的目录 | 不存在或拒绝 | G1、G5 |
| 写声明挂载以外的路径；在只读挂载上写 | 失败 | G1 |
| `env` 里出现任何不在白名单里的变量；能找到真实的网关 token | 不出现 | G2 |
| 直连外网 IP、直连宿主 `127.0.0.1:<Projects 端口>`、连宿主局域网地址、DNS 查询 | 全部失败 | G3（R2） |
| 经入口 CONNECT 一个不在白名单的域名 | 被拒，且宿主收到拦截记录 | G3、G7 |
| 经入口访问 `api.github.com`（默认不在白名单） | 被拒 | G4 |
| 用另一个线程的 MCP 能力 URL | 拒绝 | G5 |
| fork 炸弹 / 吃满内存（有 Limits 时） | 被限，宿主不受影响 | G6 |
| 起后台进程后 `Destroy` | 沙箱内无残留进程，宿主无孤儿 | G6 |
| libkrun 类：不显式关 TSI 时能出网 | 测试必须失败（证明测试有效） | R2 |

这让“标准”可以被第三方实现：写一个适配器、跑通测试、标出隔离等级即可，不需要了解 Projects 内部。

## 6 路线

| 阶段 | 内容 | 产出 |
|---|---|---|
| **P0 接缝** | agentboot 的 `process.Factory` 可注入；`sandbox` 包与 `host` 实现；Broker 与 `tb-sandboxd` 的通道（先在 `host` 下以“无隔离但走通道”的模式自测）；环境白名单与线程配置目录的开关 | 现有测试与 harness 不变 |
| **P1 进程级** | `bwrap`（Linux）与 `seatbelt`（macOS）适配器；一致性测试；项目设置“运行环境”；出口白名单与拦截提示 | 第一个真正可用的隔离档位，零额外依赖 |
| **P2 VM 级（本地）** | 先接一家有 virtiofs 的：Linux 选 libkrun 或 Cloud Hypervisor，macOS 选 Apple Containerization；基础镜像 | VM 隔离 |
| **P3 远程** | 工作区经通道同步；E2B 或 k8s 适配器 | 远程运行 |
| 以后 | 暂停/快照映射线程空闲；进程外适配器协议（适配器很小，先进程内）；同仓库线程改独立克隆 | 按需 |

## 7 UX

- 项目设置加“运行环境”：`本机（不隔离）`、`进程沙箱`、`虚拟机`，显示具体值：适配器名、版本、隔离等级、是否通过一致性测试。默认 `本机`，与今天一致。
- 线程卡片显示它在哪跑（与权限模式分开显示：隔离与权限是两个轴）。
- 出口被拦时，线程里出现一条可见事件“拦下了 `pypi.org`”，带“加入白名单”按钮，作用于项目设置，下一次连接生效（出口代理按次判断，不用重启沙箱）。
- 选了隔离档位而运行时不可用：线程启动失败，说清哪一步、怎么修；**不退回本机**。
- 切换只影响之后启动的线程；运行中的不迁移。

## 8 用户的决定（2026-11-25）与未决

已定：

- **网络先全放通，后续再收口。** 在安全与快速上线之间取上线：第一版不做 R2（断网）与出口代理，G3/G7 推迟；沙箱与宿主共用网络，MCP 与网关照旧在 `127.0.0.1`，不需要通道。收口时再加 R2 + `tb-sandboxd` + 出口代理，接口不变。
- **GitHub 不是顾虑**：GitHub 写走 MCP 工具（宿主执行、按项目设置审批），沙箱里不放 gh 凭据即可（G4 仍成立：没有凭据就写不了）；读、`git clone` 公开仓库靠放通的网络。
- **共享内容经挂载进入沙箱**：library、memory、`library/.cache`、工作区都同路径挂入（bwrap/gVisor 用 bind，microVM 用 virtiofs）。
- **先做 Linux**；以后加可选的 **worker 模式**（线程跑在别的机器上），默认仍是本机。

未决：

1. **本机档位**要不要也用线程配置目录和环境白名单（会改变现有行为：不再继承用户的全局 Claude Code 设置）。
2. 收口时的默认白名单（只放网关与 MCP，还是加常见包源）。

## 9 不做

- 不让运行时做策略（白名单、凭据、审批都在宿主）；不依赖任何一家的策略格式。
- 不照搬 Docker/Kubernetes 的对象模型；接口只有 `Start`、stdio、`Destroy`。
- 不在沙箱里放 GitHub 或模型的真实凭据。
- 不迁移运行中的线程。

## 10 Linux 第一版与 worker 模式（推荐）

### 10.1 本机现状（2026-11-25 实测，开发机）

Ubuntu 26.04，x86_64，80 核、247 GB；`/dev/kvm` 存在但当前用户**不在 kvm 组**（不可读写）；已装 `bwrap` 0.11.1、Docker 29.10（runc；注册了 `kata` 但没有 shim，不可用）、QEMU 10.2、containerd；没有 firecracker、cloud-hypervisor、libkrun、gVisor、virtiofsd、podman。`kernel.apparmor_restrict_unprivileged_userns=1`，但 bwrap 以普通用户可用。实测：bwrap 里以假 HOME 运行 `claude --version`（2.1.259）成功；放通网络时沙箱里看不到 `~/.ssh`，`https://api.github.com` 可达。

### 10.2 推荐分三步，每步都能在这台机器上落地

| 步 | 后端 | 隔离 | 这台机器上 | 为什么 |
|---|---|---|---|---|
| **v1** | **bubblewrap** | process（共享内核） | 现在就能跑，不要 sudo、不要守护进程，启动毫秒级 | 最快上线：同路径 `--bind` 挂工作区、library、memory、缓存与线程配置目录；`--tmpfs $HOME` 藏掉 `~/.ssh`、`~/.config/gh`、`~/.tingly-box`；共用网络（决定：先放通），MCP/网关不用改；`--die-with-parent` + `--unshare-pid` 保证杀干净；资源上限用 `systemd-run --user --scope -p MemoryMax=… -p TasksMax=…`（cgroup v2 + systemd 已具备）。Claude Code 自己的沙箱和 srt 在 Linux 上也是这条路 |
| **v2** | **gVisor**（`runsc`） | userkernel | 需要装一个静态二进制；不需要 KVM | 系统调用面比 bwrap 小得多，不需要虚拟化权限；`runsc --rootless --network=host do` 或经 Docker `--runtime=runsc`（⚠ rootless 细节待验证）；挂载同样是 bind |
| **v3** | **libkrun**（或 Cloud Hypervisor + virtiofsd） | vm | 需要把用户加进 kvm 组（`sudo usermod -aG kvm x0079`，需用户决定）并安装 libkrun/libkrunfw | 真正的 microVM：独立内核、virtiofs 挂载、毫秒到百毫秒级启动、可直接拿进程 stdio；libkrun 默认 TSI 借宿主网络，正好符合“先放通”，收口时再关（§4）。Firecracker 因为没有 virtiofs，不作首选 |

三步共用 §3 的接口与 §5 的一致性测试（v1 先跑文件、凭据、互隔、销毁四类用例；网络用例等收口时打开）。v1 的适配器就是“拼 bwrap 参数 + 起进程”，代码量小，接入点是 agentboot 的 `process.Factory`。

**v1 的挂载清单**（全部同路径）：工作区（读写）与它的主仓库 `.git`（读写，已知缺口见 §1.4）、项目其它仓库（只读）、`library/`（读写，含 `outputs/`、`.cache/`）、`memory/`（读写）、线程 TMPDIR（读写）、线程设置与 MCP 配置文件（只读）、线程 `CLAUDE_CONFIG_DIR`（读写）、`claude` 安装目录（只读）、`/usr`、`/etc` 等系统目录（只读）。不挂：`$HOME` 其余部分、Projects/Tingly Box 数据目录（数据库、token）。

### 10.3 worker 模式（以后，默认仍是本机）

worker 是“另一台机器上的运行时”，不是另一套模型：

- worker 机器上跑一个 `tb-worker` 守护进程，它在本地用 v1–v3 的某个后端起沙箱；Projects 的 `worker` 适配器的 `Start` = 连上 `tb-worker`（WebSocket 或 SSH，带 worker 的凭据），请求起沙箱，拿回那条 stdio。对上层仍是同一个 `Box`。
- 这时 MCP 与网关不在 worker 的 `127.0.0.1` 上，所以**worker 模式就需要 §2 的通道**：`tb-sandboxd` 在沙箱里开入口，经那条 stdio 把 MCP、网关的连接带回 Projects 主机（网络可以照样放通，通道只承载这两样）。这是收口之前唯一必须用到通道的场景。
- 工作区不在 Projects 主机上：worker 本地 `git clone`（凭据由主机经通道按需提供，或只用公开仓库），宿主代做的 git（diff、推送）改经通道在 worker 上执行；library/memory 由 worker 同步一份（只读快照 + 输出回传）。这部分等做 worker 时再细化，不影响 v1。
- 选择：项目设置“运行环境”多一个 `worker: <名字>`；worker 的注册、配对可复用 `remote/` 的配对思路（⚠ 待设计）。

### 10.4 bubblewrap 的文件边界（2026-11-25 实测）

bwrap 从一个**空的根**（tmpfs）开始，只有显式 `--bind`/`--ro-bind` 的路径存在；不是“隐藏某些目录”，而是“只放进给定的目录”。实测（普通用户，开发机）：

- 根下只有 `bin dev etc home lib lib64 proc tmp usr`；`~/.ssh`、`~/.tingly-box`、`~/.claude`、仓库目录、`/var/run/docker.sock`、`/root`、`/sys` 全部不存在；授权目录里的文件可读；只读挂载写入失败；`--clearenv` 后环境只剩显式设置的 3 个变量；`--unshare-pid` 后 `/proc` 里只有沙箱自己的 5 个进程。
- 沙箱内指向挂载范围外的符号链接解析到沙箱自己的命名空间里，即“不存在”。

v1 必须带的参数与注意点：

| 项 | 做法 | 不这么做的后果 |
|---|---|---|
| `--unshare-pid` + 新 `--proc /proc` | 必带 | 能读同用户宿主进程的 `/proc/<pid>/environ`（里面有 token） |
| `--clearenv` + 白名单 `--setenv` | 必带 | 宿主环境变量里的秘密进沙箱 |
| `--new-session` | 必带 | TIOCSTI 向宿主终端注入按键 |
| `--die-with-parent` | 必带 | Projects 退出后留下孤儿 |
| `$HOME` 用 `--tmpfs`，需要持久的子目录再单独 bind | 推荐 | 把宿主目录 bind 成 HOME 时，bwrap 会在那个宿主目录里留下挂载点用的**空目录骨架**（实测只有空目录、没有文件） |
| `--dev /dev`（最小 dev），不挂 `/sys` | 推荐 | 设备与内核信息暴露 |
| seccomp（可选，后续） | 拦 `keyctl`、`ptrace` 等 | 同用户的内核 keyring 仍可访问（少见但存在） |

**共用网络带来的缺口（先放通的代价）**：文件边界挡不住经网络能拿到的东西。共用网络命名空间时，沙箱能连宿主的回环和 `0.0.0.0` 上的所有服务（开发机上有 Tingly Box 12580、Projects、redis 6379、ollama 11434 等），也能连宿主网络命名空间里的抽象 unix socket。Tingly Box 的管理 API 要求用户认证（`getUserAuthMiddleware`），其认证材料在被隐藏的 `~/.tingly-box` 下；其它服务是否设了认证不由我们控制。收口时的一步低成本改进：`--unshare-net` + 用户态网络（pasta/slirp4netns，开发机尚未安装）放通外网但不给宿主回环，只把网关与 MCP 两个端口转发进去——这比完整出口代理简单，可作为收口的第一步。

### 10.5 yolo（`bypassPermissions`）下要成立还缺什么（2026-11-25 实测）

worker 可能以 yolo 模式跑，此时沙箱是唯一的防线，要按“沙箱里的代码完全恶意”来看。

**bwrap 自身的文件边界成立**（内核强制，对抗性实测，参数：`--unshare-user --disable-userns --unshare-pid --unshare-ipc --unshare-uts --cap-drop ALL --new-session --die-with-parent --clearenv`）：`/proc/1/root`、`/proc/*/root` 都只指向沙箱自己的根；`..` 走不出挂载（只看到挂载路径上的空目录骨架）；`mount`、`unshare -m`、`unshare -Ur` 都被拒；`CapEff=0`、`NoNewPrivs=1`（setuid 无效）；`/dev` 只有最小集合；只继承 stdio。

**逃逸不在 bwrap，而在“宿主之后会执行或解释沙箱写过的东西”**（混淆代理）。实测与代码核对：

| 途径 | 状况 | 必须的处理 |
|---|---|---|
| **主仓库 `.git` 可写 → 宿主执行仓库配置里的命令** | **实测可逃逸**：沙箱写 `core.fsmonitor`，宿主下一次 `git status`（`Stats`）就在宿主上执行它。同类：`fetchBase` 的 `git fetch`（`core.sshCommand`、`protocol.*.allow`、`remote.*.uploadpack`）、filter 驱动、hooks；用户自己在主仓库里敲 `git status` 也会中招 | 沙箱里把主仓库的 `.git/config`、`.git/hooks`（及 `.git/modules` 若有）**只读覆盖挂载**（实测：写配置与 hook 被拒，正常 `add`/`commit` 不受影响）；宿主侧 `git()` 再加固：`-c core.fsmonitor=false -c core.hooksPath=/dev/null`，`fetch` 改走推送那样的临时裸仓库（`githubRemote` 已是：`--template=`、`GIT_CONFIG_GLOBAL=/dev/null`、`GIT_CONFIG_NOSYSTEM=1`，只经 alternates 读对象，不读工作区配置——推送路径本身是安全的） |
| **共用网络 → 宿主本地服务** | 开发机上 `0.0.0.0`/回环有 redis 6379、ollama、Tingly Box 等；无认证的服务（如 redis 可写文件）就是逃逸面；X11 的抽象 socket 也在网络命名空间里 | yolo 下 **`--unshare-net` 必须开**（实测断网生效），只把网关和 MCP 转发进去；要外网就加用户态网络（pasta，需装 `passt`），它不给宿主回环 |
| 线程设置文件、MCP 配置 | 沙箱若能写，就能改自己的权限规则（非 yolo 时有意义） | 只读挂载 |
| 宿主读沙箱写的文件 | library 用 `os.Root` + `Lstat`（安全）；`send_file` 先 `EvalSymlinks` 再检查根（安全，有极小的 TOCTOU 窗口） | 保持；新代码读工作区文件一律用 `os.Root` |
| `library/.cache` 线程间共享可写 | 一个线程可投毒缓存，另一个线程在**它自己的沙箱里**执行（线程间，不是宿主逃逸） | 记为 G5 缺口；需要时按线程分缓存 |
| 用户在宿主上打开工作区 | IDE 任务、`.envrc`、`.vscode/` 等由人触发执行 | 产品层提示；不属于沙箱 |
| 共享内核 | 进程级沙箱挡不住内核漏洞 | 多租户或不可信代码量大时用 VM 档位（v3） |
| 资源 | 没有 cgroup 时 fork 炸弹影响宿主 | `systemd-run --user --scope -p TasksMax= -p MemoryMax=` |

结论：**bwrap 能做到“只有挂载/授权的路径可访问”，但 yolo 下要真正安全，还必须同时做到三件事：主仓库 `.git` 的配置与 hooks 只读、宿主侧 git 加固、断开宿主网络（只转发网关与 MCP）。** 这三条进 §5 的一致性测试（写 `core.fsmonitor` 后宿主 `git status` 不得执行；连宿主回环端口必须失败）。这也修正了 §1.4：同仓库共享 `.git` 不只是线程间可见的问题，可写的配置是宿主逃逸途径，最小修法是只读覆盖，不需要改成独立克隆。

### 10.6 v1 实现（2026-11-25）

网络按决定保持放通；先做 exec 与 storage 的隔离。

- **agentboot**：`process.Sandbox`（同路径挂载、空 tmpfs HOME、环境变量白名单）挂在 `LaunchSpec` 与 `ExecutionOptions` 上，Runner 交给 factory；`OSExecFactory` 用 bwrap 包住命令：空根 + 宿主 `/usr` `/etc` 等只读、私有 `/proc` `/dev` `/tmp`、user/pid/ipc/uts 命名空间、`--disable-userns`、`--cap-drop ALL`、`--new-session`、`--die-with-parent`。**没有 bwrap 就拒绝启动**（`ErrNoSandbox`），从不退回宿主。沙箱变化进 `launchSignature`，常驻进程带 `--resume` 重开。
- **Projects**：项目设置 `thread_sandbox`（`""` 本机 / `bwrap`，界面“线程运行环境”）。`sandboxFor` 的挂载：线程目录与仓库 git 目录（读写；`config`、`hooks`、子模块与 `config.worktree` 只读覆盖）、项目其它仓库、library、memory、本目录的转录（`$CLAUDE_CONFIG_DIR` 或 `~/.claude` 下 `projects/<名>`；名字超过 200 字符时按 Claude Code 的规则截断并加路径哈希）、设置与 MCP 配置（只读）、用户的 `CLAUDE.md`/skills/agents/commands（只读，不含 `settings.json`）、PATH 上家目录里的工具链（只读）、cwd 祖先目录的 `CLAUDE.md`；最小 `GIT_CONFIG_GLOBAL`（只有 user.name/email）。建不出沙箱时线程失败并写明原因。
- **宿主 git**：`git()` 一律 `-c core.fsmonitor=false`。
- **验证**：bwrap 真实运行的单元测试（秘密文件不可见、只读 config 写不进、环境变量被滤、杀掉后无残留进程、无 bwrap 拒绝启动）；挂载计划测试；`TB_SANDBOX_LIVE=1` 的端到端：真实 `claude` 2.1.259 以 `bypassPermissions` 在沙箱里跑，本地脚本模型让它的 Bash 去探家目录、植入的秘密、token 环境变量、仓库 `.git/config` 并在工作区写文件——全部挡住、写入落到宿主、转录在宿主读的位置。`TB_SANDBOX_LIVE_HOST=1` 是对照（不进沙箱），探针此时看到整个家目录，证明探针有效。端到端发现并修了一个问题：长路径的转录目录名被 Claude Code 截断加哈希，原先挂错目录。

未做：资源上限（`systemd-run --user --scope`）；运行中经审批新加的目录（`AddDirectories`）不会自动挂进沙箱；`library/.cache` 线程间共享；网络（按决定后补）。宿主侧 agentboot 的 `SessionReader` 也没按长路径规则找转录（已告知协调者）。

## 11 隔离层：独立模块、五轴行为模型（2026-11-25，已实现）

用户原话：“我们的实现有可能相对独立么，这样未来实现上，shim 可以在一处考虑，而不必改动太多不同模块……我们自己得定义好 interface 和行为模型，否则就会变成每个都独立适配了”；“隔离这里，我们一般就是网络，存储，执行的隔离，各一更明确清晰一些（以便于组合配置和管理）”。

v1 原本把 bwrap 写在 agentboot、Projects、API enum 和前端四处，加一个运行时要改四处。现在：

**一个模块 `sandbox/`**（独立 Go module，不依赖本仓库其它模块；`sandbox/CLAUDE.md`）：

```
sandbox/
  sandbox.go      行为模型 Policy、接口 Runtime、注册表、Start（统一的校验与通用部分）
  exec.go         ExecCommand：包一层命令的运行时共用的起进程原语
  host.go         host 运行时（不隔离，参考实现）
  conformance/    按轴组织的对抗性一致性测试：conformance.Run(t, rt)
  bwrap/          bubblewrap shim
```

**行为模型：五个轴，各有级别，单独配置、自由组合**

| 轴 | 级别 | 说明 |
|---|---|---|
| storage（文件） | `host` / `confined` | confined：只有声明的挂载（同路径；后挂的盖在先挂的上面，只读文件可以盖住可写目录）、私有 /tmp、空 HOME |
| network（网络） | `open` / `none`（以后 `allowlist`） | none：连宿主本机的服务也不行 |
| exec（进程） | `host` / `confined` | confined：独立命名空间、无特权、不能再建命名空间、看不到别的进程、杀掉即全灭 |
| credentials（凭据） | `inherit` / `explicit` | explicit：只给点名的环境变量；横跨另外三轴的秘密（文件靠 storage、注入的 token 靠 network）在这里集中声明 |
| resources（资源） | `unlimited` / `limited` | CPU、内存、进程数 |

生命周期（起停、暂停、快照）是接口的动作，不是策略；可见性与审计是管理面，不在模型里。

**接口**：`Runtime{Info() (名字、隔离等级、每轴支持的级别), Check(ctx) (本机能否运行、为什么), Start(ctx, Cmd, Policy) Proc}`。调用方只经 `sandbox.Start(ctx, name, cmd, policy)`：逐轴校验运行时能否给出要求的级别（不能就 `ErrUnavailable`，指明哪一轴），检查本机可用，再应用各运行时都一样的部分（credentials 的环境变量过滤、HOME），最后交给运行时。永不降级、永不退回宿主。`Proc` 与 agentboot 的 `process.Handle` 方法集相同。

**各层只依赖接口**：
- agentboot：`process.Sandbox{Runtime 名, Policy}`；`OSExecFactory` 只调 `sandbox.Start`。不含任何 bwrap 代码。
- Projects：按线程构造 Policy（这是 Projects 的业务：线程该看到什么），运行时名存成字符串、用注册表校验；`/projects/runtime` 列出本构建带的运行时（名字、隔离等级、本机可用与原因、线程在其中各轴的级别）。带哪些运行时由一行 import 决定（`projects/internal/project/sandbox.go`）。
- 前端：选项来自接口；不可用的灰掉并给原因；选中后显示各轴级别。轴和级别是固定词汇，新运行时不需要新文案（没有专门名字的就显示它的名字）。

**加一个 shim 只需**：`sandbox/<名>/` 下实现 `Runtime`、`init` 里注册、一行 `conformance.Run(t, …)` 测试通过；在宿主程序里加一行 import。其它模块不动。

**一致性测试**（每轴每级别一组，按运行时声明的支持项跑）：不支持的级别必须被拒绝；storage/confined：挂载外的文件不可见、只读挂载写不进、可写挂载写入落到宿主、HOME 为空、宿主家目录不可见；exec/confined：无 capability、NoNewPrivs、只看到少量进程、不能再建命名空间、kill 后后台进程不留；credentials/explicit：只剩点名的变量；network/none：连不上宿主回环上的监听；network/open：连得上。变异检查：去掉 bwrap 的 `--unshare-net` 或 `--unshare-pid`，相应用例失败（kill 用例确实抓到了残留进程）。

**resources: limited（bwrap，2026-11-25）**：bwrap 自己不限资源，由 `systemd-run --user --scope --quiet --collect -p MemoryMax=<字节> -p MemorySwapMax=0 -p TasksMax=<n> -- bwrap …` 把整个沙箱放进一个临时的 systemd 用户 scope（值为 0 的限制不写）；scope 里的进程被 cgroup 限住，内存超限时整个 scope 被杀（含 shell）。bwrap 的启动环境变量由 `env -i` 在 scope 内按 `Cmd.Env` 原样给出，宿主的 `XDG_RUNTIME_DIR`/`DBUS_SESSION_BUS_ADDRESS` 只给 systemd-run 用，不进沙箱。**失败即关闭**：`Start` 在 limited 时先跑一次探测 scope（要求 `systemd-run` 存在、用户会话可建 scope、cgroup 里有 `memory.max` 与 `pids.max`，即这两个控制器已委派），任一不满足就返回 `ErrUnavailable` 并写明原因，绝不在没有限制时启动；`Runtime.Check` 不变（不要求 systemd）。一致性测试 `resources/limited` 两组用例（64 MiB 下排序 200 MB 的单行必被杀；Tasks=20 下 fork 60 个必有失败）在 limited 起不来时跳过并带原因。CPU 限制尚未做。CI：GitHub 的 ubuntu runner 一般没有 systemd 用户会话（无 `XDG_RUNTIME_DIR`），这两组用例会跳过，不会失败；要在 CI 里真跑需要 `loginctl enable-linger` 并启动 `user@UID.service`，暂不做。

**CI**：`sandbox/**` 触发 PR 检查；`daily.yml` 新增 `test-sandbox`（边界检查、vet、`-race` 测试，装 bubblewrap 并放开 Ubuntu 的 userns 限制，确认一致性测试没有被跳过）。
