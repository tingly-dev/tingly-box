# 沙箱：安全模型与原语

`sandbox/` 模块的权威设计：安全模型、五轴行为模型、接口原语、各运行时的映射、Projects 的默认值与推进步骤。历史推导（调研、实测、被否决的方案）留在 `projects/.design/thread-runtime.md` 与 `projects/.research/agent-sandbox-landscape.md`。

状态（2026-11-25）：§3–§4 中标 **现行** 的已实现并合入（#2775–#2779）；标 **决定** 的已确认（用户 lgtm），待实施；标 **待验证** 的机制要先做实测再定。

## 1 安全模型

**主体**：agent（Claude Code 及它启动的一切）是不可信的。它读网页、跑仓库里的代码、可能被提示注入，所以按"可能已被劫持"设计。宿主（Tingly Box / Projects）是可信的。

**要保护的**：

| 资产 | 例子 |
|---|---|
| A1 宿主凭据 | `~/.ssh`、`gh` token、云凭据、Tingly Box 数据库与 token、环境变量里的密钥 |
| A2 别处的数据 | 其它项目、其它线程的工作区与转录、家目录其余部分 |
| A3 宿主的服务与能力 | 本机回环上的服务（网关、MCP、数据库）、内网、云元数据 |
| A4 宿主的可用性 | 内存、进程数、CPU；Tingly Box 与其它线程不被拖死 |
| A5 对外副作用 | 推代码、开 PR、发消息、花钱 |

**保证**（每条都由某一轴的某个级别兑现，见 §2）：

| | 保证 | 兑现 |
|---|---|---|
| G1 | 只看得到、写得到被授予的文件 | storage: confined |
| G2 | 拿不到宿主的环境凭据 | credentials: explicit（+ G1 挡住凭据文件） |
| G3 | 只到得了被允许的网络位置 | network: none / allowlist / internet + Expose |
| G4 | 对外副作用只经宿主提供的工具（MCP），由宿主审批 | G3 的 Expose 只暴露宿主点名的端点 |
| G5 | 线程之间互相隔离 | G1 的挂载按线程计算 + exec: confined |
| G6 | 有界、可杀：杀掉即全部结束；资源可封顶、可记账 | exec: confined + resources |
| G7 | 可见：为什么结束、用了多少、被拒绝了什么 | `Proc` 的结束原因、用量（§3.4） |

**不在范围内**：共享内核的 0-day（要防就用 `vm` 隔离等级的运行时）；侧信道；被允许的网络位置上的数据外泄（network 放得越宽，外泄面越大，这是用户的知情选择，界面要说清）；agent 在授予的文件里做的破坏（由 git、审批与审阅兜底）。

**宿主侧的混淆代理**：沙箱写过的东西会被宿主执行或解释（`.git/config` 的 `core.fsmonitor`、hooks）。原则：宿主要执行的文件对沙箱只读；宿主跑 git 一律 `-c core.fsmonitor=false`（现行）。

## 2 行为模型（形式化）

调用方用策略说"隔离什么、到什么程度"，不说"怎么做"。本节是模型的定义；各运行时与它的出入见 §9。

**记号**：𝒫 为绝对路径集，`p ⊑ q` 表示 q 等于 p 或在 p 之下；𝕀 为 IP 地址集，端口 `N = {1..65535}`；`res(n)` 是名字 n 在**连接时**解析出的地址集；⊙ 是沙箱自身的回环。

**策略**是五轴之积 `π = (s, n, x, c, r)`，每轴的值是"级别 + 参数"。每轴的级别按严格程度成链（`≤` 表示更严）：

```
storage      confined(M, h)        ≤ host
network      none(E) ≤ allowlist(A, E) ≤ internet(A, E) ≤ open
exec         confined              ≤ host
credentials  explicit(K)           ≤ inherit
resources    limited(m, t, u)      ≤ unlimited
```

策略之间取积序。**单调性**：π ≤ π′ ⇒ 沙箱在 π 下能做到的事 ⊆ 在 π′ 下能做到的事（以下各轴的保证均满足）。

**运行时** r 声明 `Sup_r : Axis → 2^Level`。r 能运行 π 当且仅当 `∀a. level(π_a) ∈ Sup_r(a)`。支持某级别**不**蕴含支持更严的级别。

### 2.1 storage

参数：挂载 `M ⊆ 𝒫 × {ro, rw} × {req, opt}`，home `h ∈ 𝒫`。`Sys` 为宿主系统目录（只读），`Fresh = {h, /tmp, /run}` 为新建的空目录。

- 有效挂载 `M* = {(p, w) | (p, w, k) ∈ M ∧ (k = req ∨ p 在宿主存在)} ∪ {(p, ro) | p ∈ Sys}`；有 `req` 的路径不存在时启动失败。
- 视图 `V : 𝒫 → {⊥, ro, rw, fresh}`：`V(q)` 取 `M* ∪ {(f, fresh) | f ∈ Fresh}` 中满足 `p ⊑ q` 且 p **最长**的那一项的 w；无则 ⊥。最长前缀决定，与列表顺序无关。
- **保证**（confined）：读到宿主路径 q 的内容 ⇒ `V(q) ∈ {ro, rw}`；写到宿主 q ⇒ `V(q) = rw`；`V(q) = fresh` 时看到的是空的新目录，写入不落回宿主。host 级：V 即宿主自身的访问控制。
- 推论：挂载里的 unix socket 文件是可达的服务端点。挂载一个含宿主服务 socket 的目录，等于暴露该服务（§2.2 的 Expose 之外的通道），调用方须避免。

### 2.2 network

对一台宿主把地址分成不相交的三块：

```
H = 回环 ∪ 宿主所有网卡地址（含其公网地址）
P = (RFC1918 ∪ 169.254/16 ∪ 100.64/10 ∪ fc00::/7 ∪ fe80::/10 ∪ 容器网段) \ H     // v4 映射的 v6 按 v4 归类
U = 𝕀 \ (H ∪ P)
```

参数：Expose 端口 `E ⊆ N`，授予条目 `A`（每条带端口集 `ports(a)`，缺省 `{80, 443}`）。

- Expose：`X(E) = {(127.0.0.1, e) | e ∈ E}`，在沙箱里以 `(⊙, e)` 出现。
- 授予 `G(A) = ⋃ g(a)`：
  - 通配 `*.d`：`{(i, p) | n 是 d 的子域, i ∈ res(n) ∩ U, p ∈ ports(a)}`
  - 精确名 n：`{(i, p) | i ∈ res(n) \ H, p ∈ ports(a)}`
  - IP/CIDR c：`{(i, p) | i ∈ c \ H, p ∈ ports(a)}`
- **可达上界**（沙箱发起的任何连接，实际到达的目的地都在其中）：

```
Reach(none(E))           = {⊙}×N ∪ X(E)
Reach(allowlist(A, E))   = {⊙}×N ∪ X(E) ∪ G(A)
Reach(internet(A, E))    = {⊙}×N ∪ X(E) ∪ U×N ∪ (G(A) ∩ P×N)
Reach(open)              = 宿主可达的一切
```

- 推论：同一 (A, E) 下 Reach 沿链单调；除 open 外，`Reach ∩ H×N ⊆ X(E)`，即到宿主只有 Expose 一条路（G4）。
- **功能下限**：allowlist 对 G(A) 至少提供"经环境里 `HTTP(S)_PROXY` 的 HTTP(S)"；internet 对 U×N 提供 TCP/UDP 与可用的 DNS。
- **环境附加** `NetEnv(n)`：allowlist 时为指向沙箱内代理的 `HTTP_PROXY`、`HTTPS_PROXY`（含小写）与 `NO_PROXY=回环`；其它级别为空。

### 2.3 exec

confined 的保证（对沙箱内每个进程成立）：有效 capability 为空；`no_new_privs = 1`；不能创建命名空间；看不到、也不能向宿主进程发信号；`Kill` 之后有限时间内沙箱内进程集为空。host：无保证。

### 2.4 credentials

调用方给出完整环境 env，模式集 K；`match(K, v) ⇔ ∃k ∈ K. k = v ∨ (k = q* ∧ q 是 v 的前缀)`。沙箱的初始环境：

```
env′ = F(env) ⊕ {HOME = h | h 已设} ⊕ NetEnv(n)        // ⊕ 为按名覆盖
F(env) = env                                  (inherit)
F(env) = {v=x ∈ env | match(K, v)}            (explicit(K))
```

保证：沙箱的初始环境恰为 env′；env′ 不出现在任何宿主进程的 argv 里。

### 2.5 resources

`limited(m, t, u)`：内存 m 字节、任务数 t、CPU u 千分之一核，各取值于 `ℕ ∪ {∞}`；`unlimited = (∞, ∞, ∞)`。

- 保证：任一时刻 `mem ≤ m`、`tasks ≤ t`、CPU 速率 `≤ u/1000` 核。超 m 时沙箱整体结束，原因为 `oom`；超 t 的进程创建失败；CPU 被节流。
- 记账：声明 exec confined 的运行时必须能把结束原因判定为 `oom`（包括宿主 OOM killer 杀进来的情形），与是否 limited 无关。

### 2.6 良构

`wf(π)` 为以下各条的合取，由 `sandbox.Start` 统一检查：
- 每轴级别**显式给出**：模型没有隐式默认，默认值属于调用方（§5）。
- storage：h 与 M 中路径都在 𝒫。
- network：E 无重复；open 时 `E = ∅`；A 语法正确且没有字面量属于 H 的条目。
- resources：limited 时 m、t、u 中至少一项有限。

## 3 原语（接口）

全部在 `sandbox` 包；模块不依赖本仓库其它模块。

### 3.1 Policy 与 Cmd

```go
type Policy struct {
    Storage     Storage     // Level; Mounts []Mount{Path, Writable, Optional}; Home
    Network     Network     // Level; Allow []string; Expose []Endpoint
    Exec        Exec        // Level
    Credentials Credentials // Level; Env []string
    Resources   Resources   // Level; MemoryBytes int64; Tasks int; CPU int（千分之一核）
}
type Endpoint struct{ Port int }  // 宿主回环上的 TCP 端口，沙箱里同端口出现

type Cmd struct {
    Args   []string
    Env    []string  // 完整环境，不继承任何东西
    Dir    string    // 沙箱里看到的路径
    Stderr io.Writer // nil 丢弃
}
```

`Policy` 可 JSON 序列化：它进 agentboot 的 `launchSignature`，策略变化即重开常驻进程。

### 3.2 Runtime 与 Info

```go
type Runtime interface {
    Info() Info                 // 名字、隔离等级、每轴支持的级别；纯数据，不探测
    Check(ctx) error            // 本机能否运行；错误要说清缺什么、怎么补
    Start(ctx, Cmd, Policy) (Proc, error) // 只被 sandbox.Start 调用
}
type Info struct {
    Name      string
    Isolation Isolation         // none | process | userkernel | vm | remote
    Supports  map[Axis][]Level  // 每轴能给的级别；不含默认的含义
}
```

运行时在 `init` 里 `Register`；一个程序带哪些运行时由它 import 哪些包决定。

### 3.3 sandbox.Start 的契约

```
Start(r, cmd, π) = Unavailable(原因)    若 r 未注册 ∨ ¬wf(π) ∨ ∃a. level(π_a) ∉ Sup_r(a) ∨ ¬Check(r)
                 = r.Start(cmd[Env := env′], π)   否则（env′ 见 §2.4）
```

只有这两种结果：从不以别的策略、别的运行时或在宿主上启动。`Unavailable` 包着 `ErrUnavailable`，调用方不得改用别的方式启动。

### 3.4 Proc：结束原因与用量

```go
type Proc interface {
    Stdin() io.WriteCloser   // Wait 返回前有效
    Stdout() io.ReadCloser   // 进程退出后仍可读到排空
    Wait() error             // nil 或 *ExitError
    Kill() error             // 结束沙箱内的一切；幂等
    Done() <-chan struct{}
}

// 决定：所有运行时统一的结束描述（现 microvm 有自己的 ExitError，将并入）
type ExitError struct {
    Code   int     // 退出码；被信号结束时为 128+信号
    Reason End     // exited | signaled | oom | killed（宿主 Kill）| failed（沙箱自身出错）
    Detail string  // 给人看的原因，如沙箱自身的错误输出末尾
}

// 决定：可选接口，运行时有计量单元就实现
type Usage interface {
    Usage() (Stats, error)  // MemoryBytes（当前/峰值）、Tasks、CPU 时间
}
```

状态机：`Running → Ended(reason)` 恰好一次，`reason ∈ {exited(c), signaled(s), oom, killed, failed}`；`Done` 关闭当且仅当已 Ended；`Wait` 返回 nil 当且仅当 `exited(0)`；`Kill` 幂等，在 Running 时使其以 `killed` 结束（除非已因别的原因结束）。`Reason` 是 G7 的核心：线程失败时，用户看到的是"内存超限被终止"，而不是"exit status 137"。

### 3.5 一致性测试 = 标准

`sandbox/conformance` 是运行时必须通过的东西：对每个 `(a, ℓ)`，若 `ℓ ∈ Sup_r(a)`，探针试图越过 §2 的**上界**并必须失败，再验证**功能下限**（依赖外部环境的下限在环境不具备时跳过，上界从不跳过）；若 `ℓ ∉ Sup_r(a)`，`Start` 必须返回 `Unavailable`。一个运行时的测试只有一行 `conformance.Run(t, Runtime{})`。新增原语必须同时新增用例（**决定**）：

| 用例 | 断言 |
|---|---|
| `network/internet` | 上界：宿主回环、宿主网卡地址（含其公网地址）、P 的地址都不可达；⊙ 可达。下限（需要外网，离线时跳过）：一个公网地址可达、DNS 可用 |
| `network/expose` | 点名端口经 `localhost:<端口>` 可达；未点名的宿主回环端口不可达 |
| `network/allowlist`（改） | 被点名授予的 P 地址（宿主之外的一个监听，或测试里用宿主网卡地址以外的网段）可达；名单里写宿主回环被拒；通配符条目解析到 P 被拒 |
| `resources/accounting` | 不限时也能拿到用量；OOM 时 `Reason == oom` |
| `resources/limited cpu` | CPU 配额生效（忙循环的 CPU 时间受限） |

每个用例做变异检查：去掉对应机制，用例必须失败。

## 4 运行时映射

| 轴 / 级别 | host | bwrap（process） | microvm（vm） |
|---|---|---|---|
| storage | host | confined：空根 + 系统目录只读 + `--tmpfs HOME` + 同路径 bind | confined：同一份 `bwrap.StorageArgs` 搭在 `/view`，virtiofsd（自身在 bwrap 里）作为 guest 根 |
| network none | — | `--unshare-net` | guest 无网卡 |
| network allowlist | — | `--unshare-net` + 宿主代理（unix socket）+ 沙箱内桥（本程序） | 宿主代理经 vsock + guest 内桥 |
| network internet | — | pasta（passt 包）给沙箱自己的网络命名空间并经宿主出网：`--no-map-gw`，关掉全部端口转发（`-t/-u/-T/-U none`），DNS 由 `--dns-forward 169.254.1.53 --dns-host <宿主解析器>` 转发，沙箱的 `resolv.conf` 指向它；pasta 起 `/usr/bin/sh`，先用 `nft` 在该命名空间装 `sandbox/netfilter` 的规则（自身回环与 DNS 放行 → 宿主回环与宿主全部地址拒绝 → 按地址授予的 P 放行 → P 拒绝），再 exec bwrap（`--uid/--gid` 映射回宿主用户） | VMM 在 pasta 给的网络命名空间里运行（pasta 参数与 DNS 转发同 bwrap）；guest 网卡是该命名空间里的 tap（`10.0.2.0/24`，网关 `10.0.2.1`），命名空间开转发并 masquerade；`netfilter.Router` 在 forward 链只过滤 guest 发出的包（顺序同上），input 链拒绝 guest 访问命名空间本身。规则在 VM 之外，guest 内核里的 root 也改不了 |
| network open | ✓ | 共用宿主网络 | 不支持（非特权给 VM 接宿主网络要 tap，正是要避开的） |
| Expose | — | 桥（本程序）在沙箱 `127.0.0.1`/`::1` 的同号端口监听 → unix socket → 宿主 `127.0.0.1:端口` | 同上，guest 内桥经 vsock（端口 1100+i） |
| exec confined | — | user/pid/ipc/uts 命名空间、`--disable-userns`、`--cap-drop ALL`、`--new-session`、`--die-with-parent` | 独立内核；guest init 关 user namespace、设 NoNewPrivs、以宿主 uid/gid 运行 |
| credentials | inherit/explicit | inherit/explicit | inherit/explicit（环境经 vsock 控制连接传入） |
| resources limited | — | `systemd-run --user --scope -p MemoryMax -p MemorySwapMax=0 -p TasksMax`，CPU 为 `-p CPUQuota=<u/10>%` | guest 内 cgroup（`memory.max`、`memory.swap.max=0`、`pids.max`、`cpu.max`），命令经 `CLONE_INTO_CGROUP` 进入；VM 规格只够给出限额：内存 = 限额 + 256 MiB（guest 内核与 init），vCPU = ⌈CPU/1000⌉ |
| 记账 | — | 有 systemd 用户会话时一律进 scope（`--unit=tb-sandbox-<随机>.scope`，失败的 scope 不回收，读完 `Result` 再 `reset-failed`）；`Usage` 读 scope 的 cgroup | guest 的 `/proc/vmstat` `oom_kill` 计数；`Usage` 未实现 |

**`internet` 的实测（2026-11-25）**：非特权 user namespace 里直接跑 `nft` 被 AppArmor 拒（`kernel.apparmor_restrict_unprivileged_userns=1`）；在 pasta 建的命名空间里可以（pasta 有自己的 AppArmor 配置，只允许执行 `/usr/bin` 下的命令，所以由 `/usr/bin/sh` 装规则再 exec bwrap）。pasta 默认把宿主正在监听的端口（含宿主回环上的）转发进命名空间，等于把宿主回环暴露给沙箱——一致性测试抓到了（`LEAK-host[127.0.0.1]`），必须显式关掉全部转发。变异检查：不装规则时，宿主其它网卡地址（`192.168.122.1`、容器网桥 `10.200.x`）可达。

microvm 的 `internet`：passt 的 vhost-user 模式与 Cloud Hypervisor v51.1 接不上（passt 报 `Failed to add fd to epoll: Bad file descriptor`，CH 报 `VhostUserGetFeatures ConnectionReset`），所以改为让 VMM 进 pasta 的命名空间、用 tap。forward 链若对回包也做 P 拒绝，回到 `10.0.2.15` 的 SYN-ACK 会被拒（命名空间回 ICMP 不可达，连接超时），所以只过滤从 tap 进来的包。变异检查同上：去掉 forward 规则，宿主各网卡地址可达。

## 5 Projects 的默认值

| 轴 | 默认 | 说明 |
|---|---|---|
| storage | confined | 挂载清单见 `projects/.design/thread-runtime.md` §10.6 |
| network | `internet` + Expose（已实施） | Expose 的端口自动算出：启动环境、设置文件与 MCP 配置里所有指向宿主回环的 URL（`http(s)://127.0.0.1|localhost|[::1]:端口`）的端口，即模型网关与 MCP 服务；不让用户填 |
| exec | confined | |
| credentials | explicit | 白名单 `sandboxEnv` + 启动自己设的变量 |
| resources | unlimited（记账开） | 项目设置可选填每线程配额（内存、进程数、CPU 核数），**不预设**；任一项填了则为 `limited` |

- **运行时选择**：项目设置"线程运行环境"列出运行时；某运行时是否可用，按线程策略在本机实际起一次 `true` 判定（`CheckThreadSandbox`，结果缓存一分钟），所以缺 pasta 之类某一级别的工具时，设置里就显示不可用及原因，而不是等线程失败。现为本机 / bwrap / microvm（microvm 要求运行 Tingly Box 的用户能访问 `/dev/kvm`）。
- **配额（决定）**：写在项目上、按线程生效；可另设项目总额（项目一个 systemd slice，线程是其下的 scope；VM 运行时按线程）。界面上留空即不限，填了做下限校验（内存至少 512 MiB，进程数至少 64，CPU 至少 0.1 核；前后端同一套），宿主运行时下禁用。已做：`Project.ThreadMemoryBytes/ThreadTasks/ThreadCPU`、更新接口校验、复制与变更摘要、codegen、设置界面；当前用量显示与项目总额未做。
- **网络级别**：`internet` 为默认；`allowlist`（附名单编辑）给敏感项目；`open` 不作为普通选项，若保留则明确标为危险（包含宿主回环与内网）。
- **可见性**：线程因 OOM、配额或沙箱失败结束时，写成线程事件并显示原因与用量。 OOM 已做：沙箱以 `EndOOM` 结束的线程，失败原因写明“沙箱内存不足（限额 …）”。

## 6 推进步骤

每步一个 PR，合入前本地验证（一致性测试 + 变异检查 + 必要时真实 CLI）。

| 步 | 内容 | 依赖 |
|---|---|---|
| S1 | **原语与校验（已完成）**：`Endpoint`/`Expose`、`ExitError{Reason}`、`Resources.CPU`、`Usage` 接口、`NetworkInternet` 常量（运行时尚不声明）；§2.6 的结构校验（`WellFormed`）；地址分面（`Classify`）与授予（`ParseGrant`/`Permits`）；代理按 §2.2 的授予规则改；bwrap 与 microvm 实现 Expose；bwrap 的 CPU 配额；结束原因除 `oom` 外统一；一致性测试：allowlist 重写、expose、cpu | 无 |
| S2 | **记账与 OOM（已完成）**：bwrap 有 systemd 用户会话时一律进 scope，按 systemd 记录的 scope 结果（`oom-kill`）判定 `oom`，并实现 `Usage`（读 scope 的 cgroup）；microvm 按 guest 的 `oom_kill` 计数判定 `oom` | S1 |
| S3 | **`internet`（已完成）**：pasta + 命名空间内 nftables，通过 `network/internet`（上界：宿主回环与全部宿主地址不可达；下限：公网按名可达）；microvm 见 S6 | S1 |
| S4 | **Projects 默认切换（已完成）**：线程策略改为 `internet` + 自动 Expose；设置里的可用性按线程策略实测；live 测试：真实 CLI 在 internet 下完成回合，模型经 Expose，公网 200 | S2、S3 |
| S5 | **配额（已完成）**：项目设置的配额字段（后端模型、校验、codegen、前端），按线程生效；用量与 OOM 事件显示 | S2 |
| S6 | **microvm 进 Projects（已完成）**：microvm 的 `internet`（VMM 进 pasta 命名空间 + tap + `netfilter.Router`）；vCPU 与内存改为宿主规格（代价见 §9）；加入运行时列表；live 测试：真实 CLI 在 microvm 的 internet 下完成回合，公网 200，模型经 Expose | S3、S4 |
| S7 | 项目总额（slice）；已知缺口：运行中审批新增的目录挂进沙箱、`library/.cache` 线程间共享、主仓库 git 引用可写 | 按需 |

## 7 决定记录

| 日期 | 决定 | 来源 |
|---|---|---|
| 2026-11-25 | 隔离是独立模块，自己的接口与行为模型；分五轴，各自分级 | 用户 |
| 2026-11-25 | 网络先放通，先做 exec 与 storage | 用户 |
| 2026-11-25 | 资源默认不限，前后端支持自定义配额，不预设 | 用户 |
| 2026-11-25 | 网络默认放外网、沙箱内 localhost 放通、宿主 localhost 不放；补充：P 内网一并拒绝、网关与 MCP 经 Expose 暴露、提供 DNS、按解析后地址判定 | 用户提出，补充项经讨论 lgtm |
| 2026-11-25 | 模型与原语必须正确；机制做不到可以再讨论（不为迁就机制改模型）；模型用形式化定义，实现差异单列（§9），不冗长 | 用户 |
| 2026-11-25 | 接受 passt 依赖；配额按线程生效，可选项目总额；`open` 不作默认、标为危险；confined 运行时永远记账（能判定 OOM）；结束原因进 `Proc` | 讨论后 lgtm |
| 2026-11-25 | microvm 的 `unlimited` 按宿主全量给内存，不设上限；启动开销（约 1.6% 内存）暂时接受，待底层支持（virtio-mem 等）再做按需增长 | 用户 |

## 8 待定与已知缺口

- microvm 的 `unlimited` 每台 VM 启动即占约 1.6% 宿主内存、启动较慢（§9）；待底层支持 virtio-mem 后改为按需增长。
- 被允许的网络面上的外泄无法由沙箱防止；敏感项目用 `allowlist`。
- 存储容量不限；工作区写入不受配额约束。
- `Expose` 只支持 TCP、宿主回环；Unix socket 形式的宿主服务（如 Docker）不在范围内。

## 9 实现与模型的差异

模型（§2、§3）是规范；下面是各实现与它的出入。标 **待修** 的是偏差，必须改到符合模型；其余是模型允许的实现细节或模型之外的可观察差异。

**全体**
- 代理判定在宿主侧：名字不可能被任何条目授予时不做解析，直接拒绝；按解析结果连接时直连那个地址，不再二次解析。
- `Sys` 的具体集合：`/usr`、`/etc`（整个，只读）、合并 usr 留下的 `/bin` `/sbin` `/lib*`（符号链接或目录）、`/opt`、`/etc` 中指向这些目录之外的**单个文件**（如 `resolv.conf`）。`/etc` 中对普通用户可读的配置因此可见。
- `V(q) = ⊥` 的路径不可读写，但**挂载点的各级父目录**作为空目录可见（看得到路径名，看不到内容）。

**bwrap**
- `internet`：`Allow` 只支持按地址/前缀授予 P（包过滤器无法按名字判定），按名字的条目使启动 `Unavailable`；宿主地址集合在启动时取一次，运行中宿主新增的地址不在规则里；pasta 把宿主的地址复制到命名空间的网卡上，连这些地址落在沙箱自己（也被规则拒绝）。需要 passt 与 nftables，缺了 `internet` 为 `Unavailable`。
- `exec confined` 由 user/pid/ipc/uts 命名空间、`--disable-userns`、`--cap-drop ALL`、`--new-session`、`--die-with-parent` 给出；进程以宿主 uid 运行（在 user namespace 里映射为自身）。
- `network none/allowlist` 用 `--unshare-net`：沙箱里只有回环，宿主网络命名空间里的抽象 unix socket 也不可达。`open` 共用宿主网络，包括抽象 socket。
- `allowlist` 的功能下限只覆盖读取代理环境变量的客户端；沙箱内的桥是本程序，作为命令的父进程常驻。
- **偏差**：没有 systemd 用户会话（或 memory/pids 未委派）时，`unlimited` 的沙箱照常运行但不记账，OOM 表现为 `signaled`（137）；`limited` 则拒绝启动。
- `limited` 由 `systemd-run --user --scope` 给出：内存按 cgroup 计（含页缓存），`MemorySwapMax=0`；任务数按 cgroup 的 pids 计（线程也算）。需要 systemd 用户会话与委派的 memory/pids 控制器，否则 `Unavailable`。
- `Fresh` 中的目录是沙箱私有的 tmpfs，计入沙箱的内存。

**microvm**
- `Sup(network) = {none, allowlist, internet}`：none 与 allowlist 下 guest 没有网卡；`internet` 的授予与 bwrap 一样只支持按地址；`open` 不提供。需要 passt 与 nftables。
- 文件经 virtiofsd 提供：宿主上属于 root 的文件在 guest 里显示为 `nobody`（65534）；setuid 不生效。只读由宿主侧（virtiofsd 所在的 bwrap 视图）强制。
- `/tmp`、`/run` 是宿主侧 virtiofsd 进程里的 tmpfs，计入宿主内存而不是 guest 内存。
- guest 里能看到 guest 自己的内核线程（`/proc` 里数十个进程），但看不到宿主进程。
- `unlimited`：vCPU 与内存都按宿主规格给（空闲 vCPU 不占资源；guest 内存按触及分配）。代价：guest 内核启动时为全部内存建页结构（约 1.6%，立即占用宿主内存），和本机开机时一样是全量初始化。实测（宿主 247 GiB、80 核）：每台 VM 常驻约 4.4 GB、启动约 11 s（2 GiB 的 guest 约 0.16 GB、1 s）；guest 里约 700 个内核线程。以后在底层支持时（virtio-mem 热插拔与宿主侧调节器，依赖相应组件的安装）改为小起步、按需增长，不改模型。
- `limited` 由 guest 内的 cgroup 给出，结束原因 `oom` 按 guest 的 `oom_kill` 计数判定（cgroup 内与整机 OOM 都计）。限额在 guest 内核里执行：guest 自己的内核与 init 不计入限额，`/tmp`、`/run` 在宿主侧也不计入。
- 不实现 `Usage`（可选能力）。
- 命令以宿主 uid/gid 运行，无附加组。

**host**
- 支持 storage host、network open、exec host、credentials inherit 与 explicit、resources unlimited：没有隔离，也不记账；只用于"本机"选项与对照测试。
