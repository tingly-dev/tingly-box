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

## 2 行为模型：五轴

调用方用 `Policy` 说"隔离什么、到什么程度"，从不说"怎么做"。五个轴互相独立，各有级别；运行时声明每轴能给哪些级别，给不了就拒绝启动（`ErrUnavailable`），**从不降级、从不退回宿主**。

### 2.1 storage（文件）

| 级别 | 定义 | 状态 |
|---|---|---|
| `host` | 宿主的整个文件系统 | 现行（host 运行时） |
| `confined` | 只有：宿主系统目录只读（`/usr`、`/etc`、合并 usr 留下的 `/bin` `/lib` 等、`/opt`、`/etc` 指出去的单个文件）；私有的空 `/tmp`、`/run`；空的 `Home`；`Mounts` | 现行 |

`Home` 在 `confined` 下必填（绝对路径）。`Mount{Path, Writable, Optional}`：在沙箱里出现在**同一路径**；默认只读；`Optional` 的路径不存在就跳过，否则启动失败。**嵌套规则**：更深的挂载盖在更浅的上面，与列表顺序无关（只读文件可以盖住可写目录的一部分）。只读由宿主侧强制（bind mount 的只读标志、VM 下由 virtiofsd 在宿主侧返回 EROFS），沙箱内无法解除。

### 2.2 network（网络）

先定义四个可达面（按**实际连接的地址**归类，不按域名）：

| 面 | 内容 |
|---|---|
| ① 自身 | 沙箱自己的回环（它自己起的服务） |
| ② 宿主自身 | 一切会落到宿主本机的地址：宿主回环 `127.0.0.0/8`、`::1`，**以及宿主所有网卡上的地址（即使是公网地址）**——连到它们就连到了宿主上监听 `0.0.0.0` 的服务 |
| ③ 内网 | 不属于 ② 的私有与特殊地址：RFC1918、链路本地（含 `169.254.169.254` 云元数据）、CGNAT `100.64/10`、IPv6 ULA `fc00::/7`、链路本地 `fe80::/10`、容器网桥网段；IPv4 映射的 IPv6 地址按其 IPv4 归类 |
| ④ 公网 | 其余地址 |

级别。每一级给的是**可达范围的上界**（安全保证：上界之外一律到不了）与**功能下限**（至少能用什么）：

| 级别 | 上界（除此之外都不可达） | 功能下限 | 状态 |
|---|---|---|---|
| `none` | ① + Expose | — | 现行 |
| `allowlist` | ① + Expose + `Allow` 授予的目的地（见下） | 设置了 `HTTP(S)_PROXY` 的 HTTP(S) 访问；不保证其它协议 | 现行（授予规则待改，见下） |
| `internet` | ① + Expose + ④；可另加 `Allow` 授予的 ③ | 任意 TCP/UDP 到 ④，可用的 DNS | **决定**，机制待验证 |
| `open` | 宿主的网络，全部可达 | 同宿主 | 现行；界面不作默认，标为危险 |

**Expose `[]Endpoint`（决定）**：把宿主回环上点名的 TCP 端口暴露进来，沙箱里在 `localhost:<同一端口>` 出现，URL 两边一致。**它是到达 ② 的唯一途径**，用来接宿主提供的能力（模型网关、Projects MCP），即 G4 在网络上的落点。`none`、`allowlist`、`internet` 都可带；`open` 下没有意义（不可与 `open` 同用，校验拒绝）。代价：沙箱里这些端口号被占用，沙箱自己的服务不能再监听同号端口。

**Allow 的授予规则（决定，修正现行实现）**：
- 语法：主机名或 IP/CIDR；`*.example.com` 表示其子域名；可带 `:端口`，不带端口时只放 80 与 443。
- **永不授予 ②**：解析结果落在 ② 的一律拒绝，名单里写 `127.0.0.1` 也不行（要到宿主上的服务，用 Expose）。校验时拒绝字面量属于 ② 的条目。
- **③ 只能被点名授予**：精确主机名（如内部镜像 `registry.corp.lan`）或 IP/CIDR 可以授予解析到 ③ 的地址；**通配符条目只授予 ④**（防止 `*.example.com` 下被人控制的子域名解析到内网）。
- 在 `internet` 下，`Allow` 只用来额外授予 ③；④ 本来就通。

补充约定：
- **判定在连接时、按解析后的地址**，防 DNS rebinding；`allowlist` 的代理在宿主侧解析后判定。
- **DNS**：`internet` 下运行时必须提供解析（宿主 `resolv.conf` 常指向 `127.0.0.53`，属于 ②，不能直接用）；`none` 与 `allowlist` 下沙箱里没有 DNS（`allowlist` 由代理解析），因此也没有 DNS 外泄通道。
- **线程之间**：每个沙箱的 ① 只属于它自己；除 `open` 外，一个沙箱到不了另一个沙箱的服务。

### 2.3 exec（进程）

| 级别 | 定义 | 状态 |
|---|---|---|
| `host` | 以宿主用户直接运行 | 现行 |
| `confined` | 无 capability；NoNewPrivs（setuid 无效）；不能再建命名空间；看不到、发不了信号给宿主进程；杀掉即沙箱内全部结束 | 现行 |

### 2.4 credentials（凭据）

| 级别 | 定义 | 状态 |
|---|---|---|
| `inherit` | 调用方给的完整环境 | 现行 |
| `explicit` | 只保留 `Env` 点名的变量（精确名，或以 `*` 结尾的前缀），外加 `HOME=Storage.Home` | 现行 |

环境变量是这一轴唯一直接管的东西；凭据文件靠 storage 挡，注入的 token 靠 network 与 Expose 限定去处。环境永远不出现在命令行上（`argv` 对宿主上所有进程可见）。

### 2.5 resources（资源）

| 级别 | 定义 | 状态 |
|---|---|---|
| `unlimited` | 不设人为上限 | 现行 |
| `limited` | 按 `MemoryBytes`、`Tasks`、**`CPU`（决定：千分之一核）** 封顶；零值表示该项不限，但**至少一项非零**（全零的 `limited` 校验拒绝，避免与 `unlimited` 两种写法表达同一件事） | 内存/进程数现行（bwrap），CPU 决定 |

- **记账（决定）**：凡声明 `exec: confined` 的运行时，必须把沙箱放进自己的计量单元（cgroup scope 或一台 VM），无论是否限额：要能整组杀掉、能判定结束原因是否为 OOM（宿主内存紧张时宿主的 OOM killer 也会杀进来，同样要判定为 `oom`）。报告用量是可选能力（`Usage`）。`host` 运行时不提供记账。
- **内存超限**：整个沙箱结束（含 agent 自身），结束原因必须是 `oom`，不是笼统的失败。
- **VM 的 `unlimited`**：VM 必须有大小。定义为"不设比宿主更小的人为上限"：vCPU 为宿主核数，内存为宿主内存，按需分配（现行实现是 2 vCPU / 2 GiB，**待改**）。
- 存储容量不在本轴：`/tmp`、`HOME` 的 tmpfs 计入内存；写工作区不受限（已知缺口）。

### 2.6 组合与校验

- 各轴独立配置、自由组合，有效与否只看运行时是否声明支持。
- **没有隐式默认**：每一轴都必须显式给出级别，空级别校验拒绝；默认值是调用方（如 Projects）的事，不是模型的事。
- 与级别无关的结构校验在 `sandbox.Start` 统一做：路径必须是绝对路径；`Expose` 端口在 1–65535 且不重复、不与 `open` 同用；`Allow` 语法正确且无 ② 字面量；`limited` 至少一项非零。
- 生命周期（起停、暂停、快照）是接口的动作，不是策略。

## 3 原语（接口）

全部在 `sandbox` 包；模块不依赖本仓库其它模块。

### 3.1 Policy 与 Cmd

```go
type Policy struct {
    Storage     Storage     // Level; Mounts []Mount{Path, Writable, Optional}; Home
    Network     Network     // Level; Allow []string; Expose []Endpoint（决定）
    Exec        Exec        // Level
    Credentials Credentials // Level; Env []string
    Resources   Resources   // Level; MemoryBytes int64; Tasks int; CPU int（千分之一核，决定）
}
type Endpoint struct{ Port int }  // 决定：宿主回环上的 TCP 端口，沙箱里同端口出现

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

`Start(ctx, name, cmd, policy)` 依次：找运行时（无则 `ErrUnavailable`）→ 结构校验（§2.6）→ 逐轴校验 `Supports`（缺哪轴写明）→ `Check` → 应用所有运行时都一样的部分（credentials 过滤、`HOME`）→ 交给运行时。任何一步失败都返回包着 `ErrUnavailable` 的错误，**调用方不得改用别的方式启动**。

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

`Reason` 是 G7 的核心：线程失败时，用户要能看到"内存超限被终止"而不是"exit status 137"。

### 3.5 一致性测试 = 标准

`sandbox/conformance` 是运行时必须通过的东西：对它声明的每个级别，沙箱里的探针试图打破保证并必须失败；不支持的级别必须被拒绝。一个运行时的测试只有一行 `conformance.Run(t, Runtime{})`。新增原语必须同时新增用例（**决定**）：

| 用例 | 断言 |
|---|---|
| `network/internet` | 上界：宿主回环、宿主网卡地址（含其公网地址）、③ 的地址都不可达；① 可达。下限（需要外网，离线时跳过）：一个公网地址可达、DNS 可用 |
| `network/expose` | 点名端口经 `localhost:<端口>` 可达；未点名的宿主回环端口不可达 |
| `network/allowlist`（改） | 被点名授予的 ③ 地址（宿主之外的一个监听，或测试里用宿主网卡地址以外的网段）可达；名单里写宿主回环被拒；通配符条目解析到 ③ 被拒 |
| `resources/accounting` | 不限时也能拿到用量；OOM 时 `Reason == oom` |
| `resources/limited cpu` | CPU 配额生效（忙循环的 CPU 时间受限） |

每个用例做变异检查：去掉对应机制，用例必须失败。

## 4 运行时映射

| 轴 / 级别 | host | bwrap（process） | microvm（vm） |
|---|---|---|---|
| storage | host | confined：空根 + 系统目录只读 + `--tmpfs HOME` + 同路径 bind | confined：同一份 `bwrap.StorageArgs` 搭在 `/view`，virtiofsd（自身在 bwrap 里）作为 guest 根 |
| network none | — | `--unshare-net` | guest 无网卡 |
| network allowlist | — | `--unshare-net` + 宿主代理（unix socket）+ 沙箱内桥（本程序） | 宿主代理经 vsock + guest 内桥 |
| network internet | — | **待验证**：pasta（passt 包）给用户态网络，`--no-map-gw` 关 ②；③ 的过滤见下 | **待验证**：passt vhost-user 网卡 + guest 内 nftables（guest init 以 root 设置，agent 无权改） |
| network open | ✓ | 共用宿主网络 | 不支持（非特权给 VM 接宿主网络要 tap，正是要避开的） |
| Expose | — | **决定**：复用桥，沙箱 `localhost:端口` → unix socket → 宿主 `127.0.0.1:端口` | **决定**：同上，经 vsock |
| exec confined | — | user/pid/ipc/uts 命名空间、`--disable-userns`、`--cap-drop ALL`、`--new-session`、`--die-with-parent` | 独立内核；guest init 关 user namespace、设 NoNewPrivs、以宿主 uid/gid 运行 |
| credentials | inherit/explicit | inherit/explicit | inherit/explicit（环境经 vsock 控制连接传入） |
| resources limited | — | `systemd-run --user --scope -p MemoryMax -p MemorySwapMax=0 -p TasksMax`，CPU 加 `-p CPUQuota`（决定） | VM 内存与 vCPU 数，Tasks 用 guest 内 cgroup（决定） |
| 记账（决定） | — | 不限时也进 scope，读 `memory.events` 的 `oom_kill` 判定 OOM | VM 本身；guest 报告用量 |

**`internet` 的 ③ 过滤（待验证，推进第一步就做）**：
- bwrap：pasta 本身不按目标地址过滤。候选一，在沙箱网络命名空间里装 nftables 规则（沙箱内无 capability，改不了）；本机实测在非特权 user namespace 里 `nft` 被拒（`kernel.apparmor_restrict_unprivileged_userns=1`），要看 pasta 自带的 AppArmor 配置下能否做到。候选二，在沙箱模块里用用户态 TCP/IP 栈自己转发（可完全掌控目的地址，但要引入依赖）。两者都不行时，`internet` 在 bwrap 上暂不提供（`ErrUnavailable`），而不是退回 `open`。
- microvm：guest 内核（Kata 的 6.18）内置 nf_tables，guest init 以 root 装规则后再以普通用户运行命令；passt 需要安装。
- 依赖：**接受**新增 `passt`（apt 安装；开发机尚未安装，需要用户执行 `sudo apt install passt`）。

## 5 Projects 的默认值（决定）

| 轴 | 默认 | 说明 |
|---|---|---|
| storage | confined | 挂载清单见 `projects/.design/thread-runtime.md` §10.6 |
| network | `internet` + `Expose{网关端口, Projects MCP 端口}` | 端口从线程的启动配置（`ANTHROPIC_BASE_URL`、MCP 配置）里取，自动计算，不让用户填 |
| exec | confined | |
| credentials | explicit | 白名单 `sandboxEnv` + 启动自己设的变量 |
| resources | unlimited（记账开） | 项目设置可选填配额，**不预设** |

- **运行时选择**：项目设置"线程运行环境"列出本机能用的运行时（现为本机 / bwrap；microvm 在 network `internet` 可用后加入）。
- **配额（决定）**：写在项目上、按线程生效；可另设项目总额（项目一个 systemd slice，线程是其下的 scope；VM 运行时按线程）。界面上留空即不限，填了做下限校验（内存过小 Claude Code 起不来），并显示当前用量作参考。前后端都支持，走 codegen。
- **网络级别**：`internet` 为默认；`allowlist`（附名单编辑）给敏感项目；`open` 不作为普通选项，若保留则明确标为危险（包含宿主回环与内网）。
- **可见性**：线程因 OOM、配额或沙箱失败结束时，写成线程事件并显示原因与用量。

## 6 推进步骤

每步一个 PR，合入前本地验证（一致性测试 + 变异检查 + 必要时真实 CLI）。

| 步 | 内容 | 依赖 |
|---|---|---|
| S1 | **原语与校验**：`Endpoint`/`Expose`、`ExitError{Reason}`、`Resources.CPU`、`Usage` 接口、`NetworkInternet` 常量（运行时先不声明）；§2.6 的结构校验；代理按 §2.2 的授予规则改（永不授予 ②、通配符不授予 ③）并改一致性测试；新用例的骨架 | 无 |
| S2 | **Expose + 结束原因 + 记账**：bwrap 与 microvm 实现 Expose（复用桥）；bwrap 不限时也进 scope、OOM 判定；microvm 统一到 `ExitError` | S1 |
| S3 | **`internet` 机制验证**：pasta（bwrap）与 passt + nftables（microvm）实测 ③ 的过滤，定方案后实现并通过 `network/internet` | S1；需安装 passt |
| S4 | **Projects 默认切换**：线程策略改为 `internet` + 自动 Expose；live 测试覆盖（模型经网关、MCP 工具可用、宿主回环与内网不可达） | S2、S3 |
| S5 | **配额**：项目设置的配额字段（后端模型、校验、codegen、前端），按线程生效；用量与 OOM 事件显示 | S2 |
| S6 | **microvm 进 Projects**：VM `unlimited` 改为宿主规格；加入运行时列表 | S3、S4 |
| S7 | 项目总额（slice）；已知缺口：运行中审批新增的目录挂进沙箱、`library/.cache` 线程间共享、主仓库 git 引用可写 | 按需 |

## 7 决定记录

| 日期 | 决定 | 来源 |
|---|---|---|
| 2026-11-25 | 隔离是独立模块，自己的接口与行为模型；分五轴，各自分级 | 用户 |
| 2026-11-25 | 网络先放通，先做 exec 与 storage | 用户 |
| 2026-11-25 | 资源默认不限，前后端支持自定义配额，不预设 | 用户 |
| 2026-11-25 | 网络默认放外网、沙箱内 localhost 放通、宿主 localhost 不放；补充：③ 内网一并拒绝、网关与 MCP 经 Expose 暴露、提供 DNS、按解析后地址判定 | 用户提出，补充项经讨论 lgtm |
| 2026-11-25 | 模型与原语必须正确；机制做不到可以再讨论（不为迁就机制改模型） | 用户 |
| 2026-11-25 | 接受 passt 依赖；配额按线程生效，可选项目总额；`open` 不作默认、标为危险；资源永远记账；结束原因进 `Proc` | 讨论后 lgtm |

## 8 待定与已知缺口

- **现行实现与模型的偏差（S1/S2 修正）**：`allowlist` 的代理目前按名单放行任何地址，包括宿主回环（一致性测试正是用宿主回环上的监听当"被放行"的目标）；按 §2.2 应永不授予 ②、通配符不授予 ③。`limited` 目前接受全零。Projects 线程现为 `open`。

- `internet` 在 bwrap 上的 ③ 过滤机制（§4），S3 实测后定。
- 被允许的网络面上的外泄无法由沙箱防止；敏感项目用 `allowlist`。
- 存储容量不限；工作区写入不受配额约束。
- `Expose` 只支持 TCP、宿主回环；Unix socket 形式的宿主服务（如 Docker）不在范围内。
