# 沙箱：macOS 运行时（macvm、seatbelt）

让 Mac 上的线程也能隔离运行。模型、原语、一致性标准仍以 `.design/sandbox.md` 为准，本文写 macOS 上的两个运行时：`sandbox/macvm`（虚拟机，主）与 `sandbox/seatbelt`（Seatbelt，补充，§10），以及评估后没有采用的 Apple `container`（§11）。各自选什么底座、五轴怎么映射、guest 镜像怎么来、要什么签名，以及与模型的出入。

状态（2026-11-26）：设计 + 首版实现，**都没有在真 Mac 上跑过**。本机（Linux）做了：`GOOS=darwin` 的 vet 与编译；macvm 的 vz 调用对 vz 库源码做类型检查；共用的 guest 部分（`vmguest`）在 KVM 虚拟机上真跑；usernet 用两端 socket 对真跑；其余用假的 hypervisor 与纯函数测试。标 **待验证** 的要在 Mac 上实测后再定。

## 1 为什么是虚拟机

macOS 没有 Linux 那样的命名空间与 cgroup。`sandbox-exec`（Seatbelt）只能按路径/操作拒绝，给不出独立的网络、进程视图与资源上限，且已被 Apple 标为弃用（用户已否决）。所以 Mac 上的隔离等级只能是 `vm`：一台 Linux 虚拟机，guest init 沿用 microvm 的（同一套 vsock 协议），宿主侧换掉 VMM。

## 2 底座的选择

| | Virtualization.framework 直接调用（cgo，Code-Hex/vz） | vfkit（外部进程） | libkrun / krunkit |
|---|---|---|---|
| 虚拟化 | Virtualization.framework | Virtualization.framework | Hypervisor.framework + libkrun 自己的设备模型 |
| virtio-fs 只读 | **宿主强制**（`VZSharedDirectory(readOnly:)`），多目录一个设备（`VZMultipleDirectoryShare`） | **不支持**：vfkit 写死 `readOnly=false`（v0.6.4 `pkg/vf/virtio.go`），只读只能靠 guest 内核 | libkrun 进程内的 virtiofs 实现，只读由它强制；guest 攻破设备模型即在宿主进程内 |
| vsock | 进程内 `VZVirtioSocketDevice`，直接得到 `net.Conn` | 每个端口一个宿主 unix socket | TSI / unix socket |
| 网络 | `VZFileHandleNetworkDeviceAttachment`（数据报 socket 收发以太帧）接我们自己的用户态网络栈 | 同（`unixSocketPath`） | TSI（guest 的 socket 由 libkrun 代为在宿主发起，**不过滤**，宿主回环可达）或 passt/gvproxy |
| 用户要装的 | 无 | `brew install vfkit` | `brew tap slp/krun` 等 |
| 签名 | Tingly Box 二进制要带 `com.apple.security.virtualization`（ad-hoc 签名即可） | vfkit 自己带 | `com.apple.security.hypervisor` |
| 构建 | cgo，只能在 macOS 上编译（发布已经在 macOS runner 上 `CGO_ENABLED=1`） | 纯 Go，可交叉编译 | cgo + 外部 C 库 |

**选择：直接调用 Virtualization.framework（经 `github.com/Code-Hex/vz/v3`）。**

理由按安全模型排序：
1. G1 的只读必须由宿主强制（sandbox.md §1："宿主要执行的文件对沙箱只读"，`.git/config`、hooks）。`vm` 等级本来就是为了防 guest 内核被攻破；vfkit 的只读只在 guest 内核里，guest 内核一旦被攻破，`.git/config` 可写，宿主 git 即执行沙箱植入的命令。这一条排除 vfkit。libkrun 的 virtiofs 在宿主进程里，攻击面更大，TSI 又绕过过滤，排除。
2. UX：用户不用另装任何东西；Apple 的框架随系统提供。
3. vsock 与网络都在进程内，不需要 unix socket 路径（macOS 的 `$TMPDIR` 很长，socket 路径 104 字节的上限常被撞到）。

代价，及如何收住：
- **cgo**：VZ 的调用只放在一个 `darwin && cgo` 文件里（`vz_darwin.go`），经一个小接口 `hypervisor` 与其余代码隔开；其余全是纯 Go，可在 Linux 上测（假 hypervisor）。`darwin && !cgo` 与非 darwin 编译出桩，`Check` 说明原因。发布的 darwin 构建已是 `CGO_ENABLED=1`，不变。
- **签名**：见 §6。
- **本机验证**：Linux 上无法编译 cgo 的 VZ 绑定（要 macOS SDK）。`vz_darwin.go` 用一个只做类型检查的脚本（`go/types` + `FakeImportC`，按 darwin/arm64 选文件）对 vz 的源码做类型检查，确认调用与 vz 的 API 对得上；真正的编译与运行要在 Mac 上做（**待验证**）。

## 3 结构

```
宿主（Tingly Box 进程）                                   guest（Linux VM）
  macvm.Runtime ──hypervisor 接口── VZ（vz_darwin.go）       内核（镜像）
    ├─ virtio-blk  只读  ← 镜像 rootfs（erofs）               init = tb-guest（vmguest 包，与 microvm 同一份）
    ├─ virtio-fs   "tb"  ← 每个挂载一个共享目录，只读/可写由宿主强制
    ├─ vsock      控制/stdin/stdout/stderr/代理/Expose（与 microvm 同端口）
    ├─ virtio-net （仅 internet）← 数据报 socket ← usernet（gVisor 网络栈 + 过滤）
    └─ console    → 日志文件（出错时取末尾）
```

- **guest 协议与 init 共用**：microvm 的 `spec`/`result`、vsock 端口与 guest init 抽到 `sandbox/vmguest`，microvm 行为不变；init 多认识几项可选字段（§4.1 的共享挂载、Fresh 目录、根 overlay、passwd 条目），microvm 不发就不做。镜像里的 init 是 `sandbox/vmguest/cmd/tb-guest` 交叉编译的 Linux 程序（microvm 的 init 仍是宿主二进制本身）。
- **宿主侧网络端点**：allowlist 的代理与 Expose 的转发仍是 `netproxy`；`netproxy.ListenOn` 接受任意 `net.Listener`，macvm 给的是 vsock 监听。

## 4 五轴的映射

| 轴 / 级别 | macvm |
|---|---|
| storage confined | guest 根是镜像的只读 rootfs（virtio-blk 只读 + erofs），其上 overlay 一层 tmpfs（guest 内存）供 init 建挂载点；`Sys` = 镜像的 `/usr`、`/etc` 等，**不是宿主的**；`Fresh`（Home、`/tmp`、`/run`）是 guest 内的 tmpfs；每个挂载是 virtio-fs 设备 `tb` 下的一个共享目录（`VZMultipleDirectoryShare`，名字为序号），init 按路径长度从短到长 bind 到同一路径，最长前缀决定视图，与 microvm 同；共享根随后卸载 |
| network none | guest 无网卡 |
| network allowlist | guest 无网卡；代理经 vsock（`netproxy`），guest 内桥与 microvm 同 |
| network internet | guest 一张 virtio-net，接宿主进程内的 `usernet`：gVisor 的 TCP/IP 栈终结 guest 的 TCP/UDP，按 §2.2 判定目的地址后由宿主 `net.Dial` 发起（公网 U 放行；H 一律拒绝；P 只放按地址授予的）；DNS 在 `netfilter.DNSAddr`（169.254.1.53）由 usernet 转给宿主解析器。过滤在 VM 之外，guest 内核里的 root 也改不了 |
| network open | 不支持（同 microvm） |
| Expose | guest 内桥经 vsock（端口 1100+i），与 microvm 同 |
| exec confined | 独立内核；guest init 关 user namespace、设 NoNewPrivs、以宿主 uid/gid 运行（同 microvm） |
| credentials | inherit/explicit，环境经 vsock 控制连接传入 |
| resources limited | guest 内 cgroup，同 microvm；VM 规格 = 限额 + 256 MiB，vCPU = ⌈CPU/1000⌉ |
| resources unlimited | vCPU 与内存按宿主规格（`hw.memsize`），并受 VZ 的 `maximumAllowed*` 约束；同 microvm 的决定（§7 2026-11-25） |
| 记账 | guest 的 `oom_kill` 计数判定 `oom`（同 microvm）；`Usage` 不实现 |

### 4.1 挂载的细节

- **目录挂载**：一个共享目录，`readOnly` 照挂载的 `Writable` 取反，宿主强制。
- **文件挂载**：VZ 只能共享目录。不共享父目录（那会把同目录的其它文件交给 guest 内核，如 `~/.claude/.credentials.json`、家目录本身）。改为每台 VM 一个宿主侧暂存目录，文件以**硬链接**放进去再共享（同一个 inode：原地写入两边都看得到）；跨卷等硬链接失败时，只读文件改为**复制**（启动时的快照），可写文件则启动失败（`Unavailable`）。偏差：宿主侧以"写临时文件再改名"方式更新的文件，guest 里看到的是启动时的版本。
- **挂载点的父目录**：init 在 overlay 上 `mkdir -p`，与 bwrap 一样作为空目录可见。
- **uid**：命令以宿主 uid/gid（通常 501/20）运行；镜像的 `/etc/passwd` 没有这个用户，init 在 overlay 上补一条（Node 的 `os.userInfo()` 等要它）。

### 4.2 命令与用户态

guest 是 Linux，宿主的 Mach-O 可执行文件在里面跑不了，所以 macvm 的系统目录来自镜像。这是模型里 `Sys` 的具体集合的差异（§2.1 只说"宿主系统目录（只读）"）。为让调用方知道，`Info` 加一个纯数据字段：

```go
// System is where the sandbox's system directories come from: empty for the
// host's own; otherwise the guest OS of the runtime's image, whose
// directories replace the host's and in which host executables do not run.
System string `json:"system,omitempty"`
```

- macvm：`System = "linux"`。其余运行时为空。
- **命令解析**：`Cmd.Args[0]` 是宿主上的绝对路径（agentboot 解析出的 `claude`）时，guest init 先照原样执行；失败为 `ENOENT`/`ENOEXEC`（宿主的 Mach-O）时，按基名在镜像的 `PATH` 里找。只有 `System != ""` 的 spec 才这么做。
- **Projects**：`System != ""` 时，不挂宿主的工具链目录与 CLI 目录（Mach-O，放进 `PATH` 只会先被找到再执行失败），`PATH` 设为镜像的标准路径。其余挂载（工作区、仓库、库、记忆、转录、设置）不变。

## 5 guest 镜像

镜像 = 内核 + rootfs + 清单：

```
<镜像目录>/
  manifest.json   {"version": 1, "arch": "arm64", "files": {"kernel": "<sha256>", "rootfs.erofs": "<sha256>"}}
  kernel          arm64：未压缩的 Image（VZLinuxBootLoader 在 Apple 芯片上要求）；amd64：bzImage
  rootfs.erofs    Debian（bookworm-slim）：bash、coreutils、git、ca-certificates、curl、ripgrep、iproute2、procps，
                  Claude Code 的 Linux 版本，/sbin/tb-guest（init）
```

- **内核要求**：virtio-blk、virtio-fs、virtio-vsock、virtio-net、virtio-console、erofs、overlayfs、tmpfs、cgroup2（memory、pids、cpu）、devtmpfs，全部编进内核（不用模块与 initrd）。
- **在哪找**：`TB_SANDBOX_MACVM_IMAGE` 指向的目录；否则 `<用户缓存目录>/tingly-box/sandbox/macvm/<version>-<arch>/`（macOS 为 `~/Library/Caches/...`）。`Check` 校验清单与各文件的 SHA-256；缺失或不符时说明缺什么、怎么补。
- **怎么来**：`sandbox/macvm/image/build.sh` 在 Linux（或 Docker）上构建：`debootstrap`/容器导出 rootfs → 装 Claude Code 与 `tb-guest` → `mkfs.erofs`；内核用上游源码加 `kernel.config` 片段编译。**分发（决定前的建议）**：CI 按架构构建并作为 GitHub Release 资产发布，版本与摘要写进代码；首次选用时由设置页的按钮下载到缓存目录并校验。首版只做"本地找 + 校验"与构建脚本，下载与发布流程是后续步骤（§8）。
- **为什么是 erofs + 只读块设备**：只读、压缩、无需挂载工具；guest 写不回镜像，多台 VM 共用同一份文件。
- **版本漂移**：镜像里的 Claude Code 版本固定于镜像，与宿主的不同；转录与设置格式向后兼容。镜像升级即新版本目录。

## 6 签名与 entitlement

- 调用 Virtualization.framework 的进程必须带 `com.apple.security.virtualization`。它不是受限 entitlement，**ad-hoc 签名即可**：`codesign -f -s - --entitlements sandbox/macvm/vz.entitlements <二进制>`。
- 发布：CLI（`release-cli.yml` 的 darwin 任务）与 GUI（`TinglyBox.app`）在打包前用同一份 entitlements 重签。`go run`/本地构建的二进制没有它。
- `Check` 在 darwin 上用 `codesign -d --entitlements :- <本二进制>` 读出 entitlements，没有这一项时返回"本程序没有 com.apple.security.virtualization entitlement：用 `codesign …` 重签"，设置页照常显示为不可用及原因，不等线程失败。
- 不需要 `com.apple.vm.networking`（只有桥接网络要）；不需要 hardened runtime 例外。

## 7 不支持的平台

macvm 在所有平台都注册（设置页总能列出它），`Check` 给出原因：

| 构建 | `Check` |
|---|---|
| 非 darwin | `macvm runs only on macOS (Apple Virtualization.framework); on Linux use microvm` |
| darwin、无 cgo | `this build has no cgo, which macvm needs for Virtualization.framework: build with CGO_ENABLED=1` |
| darwin、cgo | macOS ≥ 12（virtio-fs）、entitlement、镜像，逐项检查 |

## 8 待验证与后续

- **待验证（要真 Mac）**：vz 调用本身；VZ 的 virtio-fs 对宿主 uid/gid 与权限的呈现；宿主进程退出时 VM 随之结束（VZ 的 VM 在属于本进程的 XPC 服务里）；`unlimited` 按宿主全量内存时的启动开销；镜像内核在 VZ 上启动。
- **后续**：镜像的 CI 构建与发布、设置页的下载按钮；发布流程的 codesign 步骤；amd64（Intel Mac）镜像；IPv6（usernet 首版只给 IPv4）；ICMP（ping 不通）。

## 9 macvm 与模型的出入（并入 sandbox.md §9）

- `Sys` 来自镜像（`Info.System = "linux"`）；宿主可执行文件在沙箱里不能运行；`Args[0]` 的绝对宿主路径执行失败时按基名在镜像 `PATH` 中查找。
- `internet` 的授予与 bwrap、microvm 一样只支持按地址；只有 IPv4；UDP 按流转发、空闲超时后回收；ICMP 不提供。
- 文件挂载经暂存目录的硬链接（同一 inode），跨卷时只读文件为启动时的快照，可写文件 `Unavailable`。
- `Fresh` 与根 overlay 在 guest 内存里，计入 guest 内存（不同于 microvm 的宿主侧 tmpfs）。
- 不实现 `Usage`。

## 10 seatbelt（`sandbox-exec`）

Apple 已弃用 `sandbox-exec`，但它仍随系统提供，是 macOS 上不用虚拟机的唯一隔离，用户要求"即使弃用也支持"。能给什么、不能给什么如下；给不了的级别 `Start` 返回 `Unavailable`，**不降级**（模型不为迁就机制而改）。

| 轴 | seatbelt |
|---|---|
| storage confined | `(deny default)` + 系统目录只读 + 每个挂载一条规则；SBPL 是"最后匹配者生效"，规则按路径深度升序写，所以更深的路径压住较浅的（只读文件压在可写目录里：先 `allow`，后 `deny file-write*`）；挂载路径先解析符号链接（内核按真实路径匹配）。`Fresh`：没有独立的挂载视图，HOME、TMPDIR 指向沙箱自己的临时目录，真实 home 不可读 |
| network | **none**：全部拒绝，只放行 Expose 的 `localhost:端口`（宿主服务本来就在宿主回环上，直接连，不要桥）；**allowlist**：加上宿主回环上的代理端口（`netproxy` 的代理听 `127.0.0.1:0`），环境里 `HTTP(S)_PROXY` 指向它；**open**：`(allow network*)`。**internet 给不了**：SBPL 的 `remote ip` 只能写 `*` 或 `localhost` 加端口，无法表达"公网、不含宿主地址与内网" |
| exec confined | 只能向自己发信号；`process-info` 默认拒绝（看不到别的进程）；子进程继承 profile |
| credentials | inherit/explicit |
| resources | 只有 unlimited；limited 给不了（没有 cgroup） |

与模型的出入：
- 没有沙箱自己的回环 ⊙：回环是宿主的，沙箱里监听的端口别的进程也能连；Expose 之外的宿主回环端口不可连（profile 只放行点名端口）。
- `Home`、`/tmp` 不是同路径的空目录：用环境变量指向临时目录，硬编码 `/tmp` 的程序会被拒。
- `Kill` 只结束命令进程，不保证结束它 `setsid` 出去的后代（无 pid 命名空间）。
- 没有 OOM 判定、没有 `Usage`（`oom` 表现为 `signaled`）。
- 挂载点的各级父目录因 `file-read-metadata` 全开而"可见"，包括其它路径的存在性（名字，不含内容）。
- **默认策略下不可用**：Projects 的线程策略是 `internet`，seatbelt 因此在设置里显示为不可用并说明"cannot give network=internet"；只有 `none`/`allowlist`/`open` 的策略才能用它。是否给项目加"网络级别"设置是后续的产品决定。
- profile 的基线（mach-lookup 的服务、系统目录列表）按公开的做法写成，**没有在 Mac 上验证**，很可能要调。

## 11 Apple `container`（评估，不采用为默认）

[apple/container](https://github.com/apple/container) 是 Apple 的开源命令行（Swift，基于 Containerization 框架）：每个容器一台轻量 Linux 虚拟机，用 OCI 镜像，Apple 芯片、macOS 26 起（macOS 15 勉强可用，网络功能受限）。

**像 macvm 的地方**：底层同样是 Virtualization.framework，每线程一台 VM；`-v 主机目录:容器目录` 用 virtiofs 挂载；`--cpus`、`--memory` 限额；`--publish-socket` 把容器里的 unix socket 发到宿主；镜像用 OCI，不用自己做 erofs。

**不采用为默认的原因**（按安全模型）：
1. **internet 级别给不了**。它的网络是 vmnet：容器能连到宿主网关与宿主所有监听在 `0.0.0.0` 的服务和局域网，没有出口过滤（[#1320](https://github.com/apple/container/issues/1320)）；`--internal` 网络按社区反馈也并不拦截出站 TCP（[#2062](https://github.com/apple/container/issues/2062)），自定义网络要 macOS 26。要补就得自己在容器前面放过滤，等于 macvm 里的 usernet，只是换了层壳。
2. **挂载的只读与文件级挂载**：命令行里的挂载选项（`ro` 等）我没有能验证，CLI 与行为随版本变化快；只读由宿主强制与否取决于其实现，不如直接调框架确定。
3. **依赖**：要用户另外安装 `container` 并启动它的服务（`container system start`）、macOS 26 + Apple 芯片；我们的程序要通过它的 CLI（`container run -i`）接 stdin/stdout，没有稳定的 API 契约；OOM、结束原因要从退出码推。
4. **签名的好处没了但换来了依赖**：不需要给我们的二进制签 entitlement 是它的优点，但这一点用一个"装好就能用"的前提换来，对不了解的用户更难解释。

**可以以后加的位置**：作为第三个 VM 运行时（`applecontainer`），只给 `none`/`allowlist`（`--network none` 若可用，加 `--publish-socket` 经 `netproxy` 的 unix socket；我没有验证 `--network none` 是否存在）。它对只做开发机试用、不想签名的用户有价值。当前不做，等有人要。

## 12 推进

| 步 | 内容 |
|---|---|
| M1 | 设计（本文） |
| M2 | `vmguest`：microvm 的 guest init 与协议共用，init 能自建视图（overlay、fresh、virtio-fs 绑定、passwd、resolv.conf、按基名找命令） |
| M3 | `usernet`：gVisor 用户态网络栈，按 §2.2 过滤 |
| M4 | `seatbelt` |
| M5 | `macvm` + Projects 接入（`Info.System`、系统目录来自镜像时不挂宿主工具链、设置页的名字与不可用原因） |
| 后 | 镜像的 CI 构建与发布、设置页下载按钮、发布流程的 codesign 步骤、amd64 镜像、IPv6/ICMP、项目的网络级别设置、`applecontainer` |
