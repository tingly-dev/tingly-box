# Projects 接入（宿主侧）

> 读者：要在 tingly-box 里挂上 Projects 的人。Projects 的代码在本仓库 `projects/`
> （原 tingly-project 仓库，2026-10-15 带历史搬入），**嵌入契约**（门面与 `Host`
> 接口、路由/鉴权/存储、前端入口、步骤与风险）写在
> [`projects/.design/projects.md` §9](../projects/.design/projects.md)，
> 这里不重复，只记 tingly-box 这一侧要做什么。

状态：`@tingly/ui` 已合并（#2038）；Go 部分已搬进 `projects/`；下面的接入工作尚未开始。

## 1. 依赖与构建

- `projects/` 是 go.work 里的一个 module（`github.com/tingly-dev/tingly-box/projects`），
  和 `agentboot`、`swagger` 一样用 `replace` 指本地目录，不钉版本。
- 接入时根 go.mod 加 `require github.com/tingly-dev/tingly-box/projects` +
  `replace => ./projects`，并去掉 CI `verify-compile` 里 cli/gui 对 `projects/**` 的排除。
- 只 import `projects` 和 `projects/llm`；其余在它的 `internal/` 下，碰不到。
  反方向（projects import 根模块）由 `task projects:boundary` 和 CI 拦住。
- 前端在 `frontend/packages/projects`（`@tingly/projects`），是 workspace 里的普通包；入口 `src/embed.ts`，
  包内用相对路径（`@/` 留给 Tingly Box 的应用）。独立版 `task projects:frontend` 把它构建进
  `projects/web/dist` 嵌入二进制。

## 2. 后端

- 新增 `internal/server/module/projects/`：`Host` 适配器（`ClientFor` 走网关，
  `ThreadEnv` 复用 `tbclient.GetClaudeCodeEnv`，`Models` 列网关可用模型）和 gate。
- `server_modules.go` 里 `projects.New(Options{DataDir: <数据目录>/projects, Host})`，
  `Register(rt.V1)`；schema 生成时只 `Register` 不 `Start`。跑 `task codegen`。
- 开关：新增 `constant.ExtensionProjects = "projects"`（加进
  `KnownExtensionBoolFlags`），默认关闭。照 Desk 用 `WithGate`：关闭时整个
  `/projects` 返回 404；第一次打开时才调用 `App.Start`。
- 模型路由：线程沿用 `claude_code` 场景；协调者建议新增 `projects` 场景，用量和
  路由规则可以单独看、单独配。

## 3. 前端

- 视觉统一靠 `@tingly/ui`（`frontend/packages/ui`）：Projects 页面用的就是这里的
  主题、图标和页面原语，改视觉一律改这里。
- `appRoutes.tsx` 用 `lazy()` 挂 `ProjectsPage` / `ProjectPage`，外面包
  `ExperimentalFeatureGate feature="projects"`（`ExperimentalFeature` 类型加上
  `'projects'`）；用 `ProjectsHostProvider` 注入带鉴权的 `fetch`、路由路径和
  `onProjectsChanged`。
- 导航：Projects 是一级功能，左侧 rail 有自己的图标，有等你处理的线程时显示小红点。
  Desk 留在 Remote 下。
- i18n：合并 `projects` 命名空间（en / zh），ru 由语言对齐测试补齐。

## 4. 命名

Desk 现在把文件夹叫“项目目录”（`desk.projectDirectory`），用量页也有“项目”
筛选。Projects 进来后“项目”一词两义（UX 原则：拆开同名冲突）。建议把 Desk 的
叫法改成“工作文件夹”，用量页的“项目”按实际含义重新命名。
