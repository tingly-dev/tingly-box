# Projects 接入（Tingly Project 嵌入 Tingly Box）

> 读者：要把 Projects 挂进 tingly-box、或改 `@tingly/ui`、`agentboot`、`swagger`
> 的人。记录 Projects 为什么留在自己的仓库、怎么以引用包的方式进来、两边的边界
> 和接入步骤。Projects 自身的产品设计（Conversation、Coordinator、Thread 等）在
> tingly-project 仓库的 `.design/projects.md`，这里不重复。

状态：**部分落地**。`@tingly/ui` 已在 #2038 合并；tingly-project 侧的嵌入边界
（tingly-project #4）和主题统一（tingly-project #5）在评审中；tingly-box 侧的接入
（第 3–5 步）尚未开始。

---

## 1. 结论

- **仓库不合并**。tingly-project 保留自己的仓库、CI 和发版；tingly-box 用 git
  submodule 把它挂在 `libs/tingly-project`，和 `libs/` 下的 SDK 一样。
- **依赖只有一个方向**：tingly-box 根模块 → `tingly-project/projects` →
  `tingly-box/agentboot`、`tingly-box/swagger`。tingly-project 永不 import
  tingly-box 根模块；宿主能力通过 `Host` 接口注入。
- **界面是 tingly-box 的一部分**：路由挂在 `/api/v1/projects` 下，走 tingly-box
  的鉴权；页面挂在 tingly-box 的壳里，用宿主的主题和导航，不是第二个应用。
- **视觉只有一份源**：主题、图标、页面原语在 `@tingly/ui`
  （`frontend/packages/ui`），两边都从它构建。
- **默认关闭**：实验开关 `projects`，做法照搬 Desk。

## 2. 为什么不搬进来

| 方案 | 做法 | 取舍 |
|---|---|---|
| 搬进 tingly-box 做嵌套模块 | `tingly-box/projects/` 自带 go.mod | 一个 PR 改两边；但 tingly-box 仓库变大，Projects 失去独立节奏 |
| **submodule 引用（采用）** | `libs/tingly-project` + `replace` | 两边各自独立；改接口要两个 PR，用 submodule 指针对齐版本 |
| 平铺进 `internal/` | 代码直接并入根模块 | 边界只能靠自觉，很快会反向依赖；不能再独立运行 |

选 submodule：重开一个项目的代价已经付过，Projects 也还在快速迭代，保持独立
更便宜。边界设计与“搬进来”完全一样，将来想搬家，只是把 submodule 换成
`git subtree`。

## 3. 后端边界

tingly-project 对外只暴露两个包：门面 `projects` 和数据类型 `llm`。其余都在
它自己的 `internal/` 下，由 Go 的 `internal` 规则保证 tingly-box 碰不到。

```go
// github.com/tingly-dev/tingly-project/projects
type Host interface {
	// 协调者的模型客户端；model 为空时用宿主默认。
	ClientFor(ctx context.Context, model string) (llm.Client, string, error)
	// 项目设置里协调者和线程可选的模型。
	Models(ctx context.Context) ([]llm.Model, error)
	// 线程里 Claude Code 的环境变量（指向网关）和默认模型。
	ThreadEnv(ctx context.Context) (env []string, defaultModel string)
}

type Options struct {
	DataDir string // 嵌入时为 <tingly-box 数据目录>/projects
	Host    Host
}

func New(opts Options) (*App, error)
func (a *App) Register(api *swagger.RouteGroup) // 挂 /projects/*，不启动任何东西
func (a *App) Start(ctx context.Context)        // 恢复线程、后台任务；可重复调用
func (a *App) Close() error
```

规则：

1. tingly-project 只依赖 `tingly-box/agentboot` 和 `tingly-box/swagger` 两个子模块；
   它的 `task boundary` 检查 `go list -deps` 里不出现 tingly-box 根模块。
2. `Host` 适配器写在 tingly-box 的 `internal/server/module/projects/`，用
   `tbclient` 和网关实现：`ClientFor` 走网关，`ThreadEnv` 复用
   `tbclient.GetClaudeCodeEnv`。
3. tingly-box 只 import `projects` 和 `projects/llm`。

## 4. 路由、鉴权、存储、开关

所有路由都在 `/api/v1/projects` 下（tingly-project #4 已经收拢），嵌入和独立两种
模式路径一致，前端不用区分。

| 路径 | 嵌入时 |
|---|---|
| `/api/v1/projects/*`（含 `/projects/{id}/threads/{tid}/*`、`/projects/models`、`/projects/repos/inspect`） | 挂在 tingly-box 的 V1 组下 |
| `/api/v1/settings*`、`/conversations*`（Chats）、`/health` | 只在独立模式注册；嵌入时由 tingly-box 负责 |

- **鉴权**：嵌入时挂在 `rt.V1` 上，受 tingly-box 的 Bearer token 保护。前端所有
  请求（包括事件流和 Library 下载）都走宿主注入的 `fetch`；事件流已经从
  `EventSource` 换成 `fetch` 读流，所以能带上 `Authorization`。
- **存储**：独立的 SQLite 文件 `<tingly-box 数据目录>/projects/projects.db`，项目
  目录（memory、library、worktree）在同一处。不和 tingly-box 主库合表：各自迁移、
  互不阻塞，删掉这个功能就是删一个目录。
- **开关**：新增 `constant.ExtensionProjects = "projects"`，默认关闭，在
  `/system/experimental` 打开。和 Desk 一样用 `WithGate`：关闭时整个
  `/projects` 返回 404，前端隐藏导航和路由。`App.Start` 在开关第一次打开时才
  运行，关着时不恢复线程，也不拉起 `claude` 进程。
- **OpenAPI**：tingly-box 生成 schema 时调用 `Register`、不调用 `Start`，所以
  tingly-box 的 `openapi.json` 会包含这些路由。tingly-project 也保留自己的一份，
  两边的 codegen 互不依赖。
- **模型路由**：线程沿用 `claude_code` 场景；协调者建议单独新增一个 `projects`
  场景，用量和路由规则可以单独看、单独配。

## 5. 前端

- **共享视觉**：`@tingly/ui` 包含主题（调色板、组件覆盖、字体、
  status/elevation/controls token、模式选项）、Tabler 图标，以及 `PageLayout`、
  `PageHeader`、`Surface`、`EmptyState`、`DialogHeader`、`ConfirmDialog`、
  `CopyIconButton`。tingly-box 的旧路径只做转发。改视觉一律改这里。
- **过渡期复制**：tingly-project 目前在 `frontend/vendor/tingly-ui` 放一份复制，按包名
  `@tingly/ui` 引用（`link:` 依赖），用 `task ui:sync` 定期刷新，`SOURCE_COMMIT`
  记录来源。换成真正的包只需改 `package.json` 的一行。
- **嵌入入口**：tingly-project 的 `embed.ts` 导出 `ProjectsHostProvider`、
  `ProjectsPage`、`ProjectPage`。宿主通过 `ProjectsHost` 注入带鉴权的 `fetch`、
  路由路径（`paths.list`、`paths.project(id)`）和 `onProjectsChanged`。页面不带
  自己的 `ThemeProvider`，直接用宿主的主题。
- **接入方式**：tingly-box 的 `frontend/pnpm-workspace.yaml` 加入
  `../libs/tingly-project/frontend`，在 `appRoutes.tsx` 里用 `lazy()` 挂页面，外面包
  `ExperimentalFeatureGate feature="projects"`。不用 iframe，也不在子路径下另挂
  一个 SPA，否则会出现两套侧栏、两套主题和两套登录状态。
- **共享依赖**：react、react-dom、@mui/material、react-router-dom、i18next、
  react-i18next 作为 peerDependencies，只打包一份。i18n 用独立命名空间
  `projects`（en / zh，ru 交给 tingly-box 的语言对齐测试补齐）。
- **待解决**：tingly-project 内部用 `@/` 别名，和 tingly-box 的 `@/` 冲突。接入时
  要么在 tingly-project 里改成包内相对路径或独立别名，要么由 tingly-box 的 Vite
  按文件来源分别解析。另外页面目前自带外边距，放进 tingly-box 布局后要去掉，
  改用 `PageLayout`。

## 6. 产品与命名

Desk 和 Projects 都驱动 Claude Code，但回答的是不同的问题，所以并存、不合并。

| | Desk | Projects |
|---|---|---|
| 用户在做什么 | 坐下来和一个 Claude Code 一起干活 | 丢一串任务进来就走开，回来只看“等你处理” |
| 结构 | 一个文件夹、一个会话 | 一个对话 + 协调者 + 多个并行线程，每个线程一个 worktree |
| 底层 | `remote/session` + `agentboot` | `agentboot` |

- **导航**：Projects 是一级功能，在左侧 rail 上有自己的图标；有等你处理的线程
  时显示小红点。Desk 留在 Remote 下。
- **命名冲突**：Desk 现在把文件夹叫“项目目录”（`desk.projectDirectory`），用量页
  也有“项目”筛选。Projects 进来后“项目”一词两义（UX 原则：拆开同名冲突）。
  建议把 Desk 的叫法改成“工作文件夹”，用量页的“项目”按实际含义重新命名。
- **固定词汇**：Project、Conversation、Coordinator、Thread、Overview、Library、
  Memory、Instructions、Goal，不翻新。
- **设置**：嵌入时项目设置里没有“模型连接”一栏；协调者和线程的模型选择器列的是
  tingly-box 里可用的模型，名字旁边显示实际路由到的服务商。

## 7. 接入步骤

每一步结束时两个产品都能正常运行。

1. **tingly-project 改边界（行为不变）**：门面 `projects`、公开 `llm`、路由收进
   `/projects`、前端注入 `fetch` 和路由路径（tingly-project #4）。
2. **统一视觉**：tingly-box 抽出 `@tingly/ui`（#2038，已合并）；tingly-project 用
   复制的方式接入，页面改用其原语（tingly-project #5）。
3. **tingly-box 后端接入**：加 submodule `libs/tingly-project`，根 go.mod
   `require` + `replace => ./libs/tingly-project`；新增
   `internal/server/module/projects`（`Host` 适配器 + gate）；在
   `server_modules.go` 挂载；新增开关 `ExtensionProjects`；跑 `task codegen`。
   CI 的 checkout 需要 `submodules: recursive`。
4. **tingly-box 前端接入**：workspace 加入 tingly-project 前端；`lazy()` 路由 +
   `ExperimentalFeatureGate`；rail 入口和小红点；i18n 合并；Desk 改名。
5. **数据与收尾**：首次打开开关时，如果 `~/.tingly-project` 存在而新目录为空，提供
   一键导入（拷贝数据库和项目目录，再对每个仓库跑 `git worktree repair`）。
   tingly-project 的 `@tingly/ui` 复制改成真正的包。

## 8. 风险与待定

| 事项 | 风险 / 问题 | 建议 |
|---|---|---|
| worktree 路径 | git 记录的是绝对路径，直接搬数据目录会断链 | 导入时跑 `git worktree repair`；失败就保留旧目录继续用 |
| agentboot 版本 | tingly-project 锁定的是 pseudo-version，嵌入后由 `replace` 改用本地版本 | 第 3 步的 PR 里一次对齐 |
| 并发 | Desk 和 Projects 各自拉起 `claude` 进程，上限各管各的 | 首批不处理，以后共用 `agentboot/pool` |
| 协调者的路由场景 | 新增 `projects` 场景，还是复用现有场景 | 新增，用量和规则可以单独看 |
| 独立二进制 | 是否继续维护 | 保留：成本低，开发调试和离线演示都靠它 |
| Chats | 嵌入时是否需要 | 不需要，只在独立模式保留 |
