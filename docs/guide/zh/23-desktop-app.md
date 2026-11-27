# 桌面应用与托盘面板（Hub）

桌面应用（macOS、Windows、Linux 版 Tingly Box）与浏览器里的界面是**同一个完整产品**：同一个网关、同样的页面，只是放在原生窗口里并带一个系统托盘图标。不再有单独的 *lite* 版本——本指南里的每个页面，包括 Projects、IM、Prompt 管理和 Apply 按钮，这里都有，且各自仍受 [Power-ups](./02-scenario-overview.md#power-ups齿轮背后的菜单)里相应开关的控制。桌面窗口只多了几处原生体验：

- **文件通过原生的"保存"对话框保存**（导出、下载的图片），**外部链接在系统浏览器里打开**，而不是在窗口内
- [提示与帮助](./13-help.md)页会隐藏 *Desktop Shortcut* 卡片——应用本身就是快捷方式

## 快速开始

把 Tingly-Box 当作桌面应用使用，而不是终端工具。

1. 安装并打开适用于 macOS、Windows 或 Linux 的 Tingly Box。窗口里就是完整的 Web UI，网关由应用自己启动——不需要 `tb start`。从[快速开始（首次使用）](./01-getting-started.md#快速开始)的第 3 步继续。
2. 关闭窗口只是把它隐藏：应用会继续留在系统托盘（macOS 是菜单栏）里运行。
3. **左键**点击托盘图标，打开 **Hub 面板**。它显示 **运行中** 或 **连接已断开**，一排跳转图标（**Agent**、**Dashboard**、**Credential**、**Logs**、**System**），以及每个 Provider 的配额。
4. **右键**点击图标，菜单有 **Show Hub**、**Open Tingly Box**、**Dashboard**、**Credentials**、**Logs** 和 **Quit Tingly Box**。

**验证**：Hub 显示「运行中」和各 Provider 的配额圆环。按 `Esc` 或点击别处即可收起面板。macOS 菜单栏和其他原生细节见下文。

---

## 托盘图标与 Hub 面板

Tingly Box 常驻在系统托盘（macOS 的菜单栏）。**左键点击**图标切换 **Hub 面板**——图标下方一个小巧的无边框面板，点击别处或按 Esc 就会关闭（它也会浮在全屏应用之上）。**右键点击**弹出菜单：**Show Hub**、**Open Tingly Box**、**Dashboard**、**Credentials**、**Logs**、**Quit Tingly Box**。

![Hub 面板](../images/hub.png)

Hub（`/hub`）回答那些不用打开整个应用就能解决的问题：*它在运行吗？我的配额快用完了吗？* 自上而下：

- **头部**：应用标志、`Tingly Box`，以及一个状态圆点，显示 **Running** / **Disconnected** 和版本号。有新版本时出现 **Update available** 标签（点击在主窗口里打开 System；开发版从不声称有更新）
- **跳转行**：五个图标，在**主窗口**里打开与对应导航项相同的位置：**Agent**、**Dashboard**、**Credential**、**Logs** 和 **System**（面板本身从不跳转）
- **Provider Quota**：每个有配额读数的 Provider，使用与[凭证管理](./08-credentials.md#quota-列)表格相同的单元格（圆环、`% left`、余额、资源组），带 Provider 的 Logo。列表默认按 **Closest to limit**（最接近上限）排序；排序按钮还提供 **Name** 和 **Type**，你的选择会被记住。点击 Provider 名称打开 Credentials；点击配额单元格刷新它，或用头部的刷新按钮刷新全部
- **Quota history**：底部这一行打开 [Dashboard → Quota history](./11-dashboard.md#quota-historydashboardquota-history)

---

## macOS 菜单栏

在 macOS 上，窗口还有一个真正的菜单栏：

| 菜单 | 项目 |
|------|------|
| **Tingly Box** | About · **Settings…**（⌘,）——打开 System · Hide / Hide Others / Show All · Quit |
| **Edit** | 标准的编辑菜单（这样 ⌘C / ⌘V 在窗口里可用） |
| **Go** | **Agent**（⌘1）· **Dashboard**（⌘2）· **Credentials**（⌘3）· **Logs**（⌘4）· **Show Hub**（⇧⌘H）——与导航栏对应 |
| **View** | Reload（⌘R）· Toggle Full Screen（调试版本还有 Developer Tools） |
| **Window** | 标准的窗口菜单 |
| **Help** | **GitHub** · **Report an Issue** |

Windows 和 Linux 没有全局菜单栏，所以只保留托盘菜单和 Hub 面板。

---

## 相关页面

- [场景总览](./02-scenario-overview.md)
- [系统设置](./17-system-settings.md)
- [凭证管理](./08-credentials.md)
