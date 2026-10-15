# Projects 接入（宿主侧）

> 读者：要在 tingly-box 里挂上 Projects 的人。Projects 的代码在本仓库 `projects/`
> （原 tingly-project 仓库，2026-10-15 带历史搬入），**嵌入契约**（门面与 `Host`
> 接口、路由/鉴权/存储、前端入口、步骤与风险）写在
> [`projects/.design/projects.md` §9](../projects/.design/projects.md)，
> 这里不重复，只记 tingly-box 这一侧要做什么。

状态：`@tingly/ui` 已合并（#2038）；代码已搬进 `projects/` 和 `frontend/packages/projects`；后端接入（§1、§2）、前端接入（§3）和改名（§4）都已完成。

## 1. 依赖与构建

- `projects/` 是 go.work 里的一个 module（`github.com/tingly-dev/tingly-box/projects`），
  和 `agentboot`、`swagger` 一样用 `replace` 指本地目录，不钉版本。
- 根 go.mod（和 `gui/wails3/go.mod`）`require github.com/tingly-dev/tingly-box/projects` +
  `replace` 指本地目录；CI `verify-compile` 的 cli/gui 不再排除 `projects/**`。
- 只 import `projects` 和 `projects/llm`；其余在它的 `internal/` 下，碰不到。
  反方向（projects import 根模块）由 `task projects:boundary` 和 CI 拦住。
- 前端在 `frontend/packages/projects`（`@tingly/projects`），是 workspace 里的普通包；入口 `src/embed.ts`，
  包内用相对路径（`@/` 留给 Tingly Box 的应用）。独立版 `task projects:frontend` 把它构建进
  `projects/web/dist` 嵌入二进制。

## 2. 后端

- `internal/server/module/projects/`：
  - `Host`：协调者和线程都走网关的 `claude_code` 场景，路由规则、用量和配额照常生效。
    `ClientFor` 用 model token 以 Anthropic Messages 调 `/tingly/claude_code`，不指定模型时用
    该场景的默认档（`tbclient.ClaudeCodeDefaultModel`，即 Claude Code 的 `ANTHROPIC_MODEL`）；
    `Models` 列该场景启用中的请求模型，默认档排第一；`ThreadEnv` 复用 `tbclient.GetClaudeCodeEnv`。
  - `Handler`：给 Projects 单开一个 `/api/v1` 路由组（用户鉴权 + 开关闸门），只罩住它自己的路由。
- `server_modules.go`：`newProjectsApp` 建 App，数据放 `<数据目录>/projects`，和主库分开；
  schema 生成时用内存库、不启动。服务停止时 `Close`，不留下线程的 `claude` 进程。
- 开关：`constant.ExtensionProjects = "projects"`（在 `KnownExtensionBoolFlags` 里），默认关闭。
  关闭时整个 `/api/v1/projects` 返回 404；开着启动时立即 `Start`，运行中打开则在第一个请求时 `Start`。
- 已知代价：开关关着也会在数据目录建一个空的 Projects 数据库（路由要在启动时挂好）。
- 待定：协调者单独的 `projects` 场景（用量和路由规则单独看、单独配），现在先和线程共用 `claude_code`。
- OpenAPI：两边的 schema 按类型名合并，同名类型会互相覆盖（线程的 `RespondRequest` 已改名为
  `ThreadRespondRequest`）。根模块里 imbot 和 team 已有几组同名（`CreateRequest`、`UpdateRequest`、
  `ListResponse`），尚未处理。

## 3. 前端

状态：已完成。页面在 `/projects`、`/projects/:id`，由 `src/pages/projects/ProjectsRoute.tsx`
懒加载（包和它的文案都不进首屏，`dist/index.html` 的 modulepreload 里没有）；鉴权的
`fetch` 在 `src/services/projectsApi.ts`，复用控制面 API 的 base URL 和 `Authorization`；
toast 走 Tingly Box 的 `notify`。mock 数据在 `src/mocks/projectsHandlers.ts`。

- 视觉统一靠 `@tingly/ui`（`frontend/packages/ui`）：Projects 页面用的就是这里的
  主题、图标和页面原语，改视觉一律改这里。
- `appRoutes.tsx` 用 `lazy()` 挂 `ProjectsPage` / `ProjectPage`，外面包
  `ExperimentalFeatureGate feature="projects"`（`ExperimentalFeature` 类型加上
  `'projects'`）；用 `ProjectsHostProvider` 注入带鉴权的 `fetch`、路由路径和
  `onProjectsChanged`。
- 导航：Projects 是一级功能，左侧 rail 有自己的图标（文件夹，和 Projects 页面里一致），
  放在 Image 之后、Remote 之前；有等你处理的线程时图标上有一个点（和 Projects 里“等你处理”
  同色，warning）。点的数据来自 `GET /api/v1/projects` 每个项目的 `threads.waiting`：
  打开时、Projects 页面改动项目后（`onProjectsChanged` 发 `tingly:projects-changed`）、
  标签页回到前台时和每 30 秒各读一次。Desk 留在 Remote 下。开关也在 Power-ups 菜单和
  System → Experimental 里。
- 外边距：`/projects` 路由在宿主布局里没有内边距（项目页是满高的分栏），页面自己排；
  宿主传 `embedded: true`，页面去掉独立版为浮动侧栏按钮留的位置，项目页标题前加
  “项目 /”链接回列表（宿主里没有 Projects 侧栏）。
- i18n：包里的文案在 i18next 命名空间 `projects`（`useTranslation('projects')`），由懒加载
  模块 `addResourceBundle` 加 en / zh；ru 回落到 en（语言对齐测试只管 Tingly Box 自己的
  bundle）。对话记录（`src/transcript/`）仍用默认命名空间里的 `projects.transcript.*`。
  `tKeyCoverage` 另检查包里每个 `t()` key 在 `projects` 的 en / zh 里都有。

## 4. 命名

Desk 原来把文件夹叫“项目目录”（`desk.projectDirectory`），Projects 进来后“项目”
一词两义（UX 原则：拆开同名冲突）。

状态：已完成。Desk 的文件夹改叫“工作文件夹” / Working folder / Рабочая папка
（只改文案，key 不变）。“项目”筛选其实在 User prompts（`/prompt/user`）的录制页，
筛的是录制时 IDE 打开的工作区，改叫“工作区” / Workspace / Рабочая область，
筛选栏的文案也接上了 i18n（原来写死英文）。用量页（Dashboard）没有“项目”筛选。
