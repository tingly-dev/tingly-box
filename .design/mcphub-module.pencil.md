---
date: 2026-11-15
---

# mcphub 模块抽取 — 架构图

Repo: tingly-box。`mcphub/` 是第三个共享库模块(前有 imbot/agentboot/remote),被 root 与 projects 双向引用,自身零本地依赖。

## 模块依赖图(终态,Phase 3 后)

```
┌────────────────────────────────────────────────────────────────────┐
│                          tingly-box (root)                          │
│                                                                    │
│  cli/tingly-box ── internal/server ── internal/mcp/                │
│        │                                   │                       │
│        │              ┌────────────────────┼───────────────┐       │
│        │              │ runtime (不搬)      │ tools/github  │       │
│        │              │ · sources stdio/   │ · GitHubPreset│       │
│        │              │   http/sse         │ · githubSubset│       │
│        │              │ · profiles/policy  │ · scope 检查   │       │
│        │              │ · builtin_registry │               │       │
│        │              └────────┬───────────┴───────┬───────┘       │
│        │                       │ stdio 子命令       │ Phase 3       │
│        │              "tingly-box mcp-github"      │ (待做)        │
└────────┼───────────────────────┼───────────────────┼───────────────┘
         │                       ▼                   ▼
         │              ┌────────────────────────────────────┐
         │              │           mcphub/  (共享)           │
         │              │                                    │
         │              │  github/   gh CLI 客户端            │
         │              │            · Client{Bin}           │
         │              │            · Repo/Item/Rate/Checks │
         │              │            · Redact/ParseRemote    │
         │              │            · ErrUnavailable        │
         │              │                                    │
         │              │  tool/     通用工具注册核心          │
         │              │            · Definition/Call       │
         │              │            · Register (schema+验证)│
         │              │            · TextResult/StrProp    │
         │              │                                    │
         │              │  loopback/ capability URL 服务      │
         │              │            · Server Add/Revoke     │
         │              │            · 32B hex 密钥路径       │
         │              │            · 临时 mcpServers config │
         │              └────────────────────────────────────┘
         │                       ▲                    ▲
┌────────┼───────────────────────┼────────────────────┼───────────────┐
│        ▼              projects/│                    │               │
│                       ┌────────┴────────────────────┴───────┐       │
│                       │ internal/project (薄适配层)          │       │
│                       │                                    │       │
│                       │  github_alias.go                   │       │
│                       │   · GitHubRepo=gh.Repo 等别名       │       │
│                       │   · GitHub 接口(本地定义)          │       │
│                       │   · dbChecks ⇄ ghChecks 转换        │       │
│                       │                                    │       │
│                       │  tool_server.go / tools.go          │       │
│                       │   · Tool/ToolCall/threadTool        │       │
│                       │   · coordinator turn gating(留)    │       │
│                       │   · register → tool.Register        │       │
│                       │                                    │       │
│                       │  session_tools_mcp.go               │       │
│                       │   · sessionToolsMCP                │       │
│                       │   · 委托 loopback.Server            │       │
│                       └────────────────────────────────────┘       │
└────────────────────────────────────────────────────────────────────┘

边界规则:mcphub 不 import root/projects/remote/imbot/agentboot
         (task mcphub:boundary,CI test-mcphub job)
```

## 一次 GitHub 工具调用的路径(projects)

```
Claude Code 会话(thread)
   │  MCP config 文件(临时,loopback 生成)
   │  {"mcpServers":{"tingly-projects":{"type":"http","url":"http://127.0.0.1:PORT/<32B hex>"}}}
   ▼
loopback.Server ── path→handler ──► mcp.Server("tingly-projects")
                                        │
                        tool.Register: schema 校验
                                        │
                     projects register 适配: ToolCall{ThreadID, Input}
                                        │
                    coordinator? ──是──► turn.run 门控(EditsProject)
                                        │
                        github_tools.go: create_issue/create_pr/
                                         update/comment/view/merge
                                        │
                        Links(projects): record/refresh/link 簿记
                                        │
                        mcphub/github.Client ──► `gh api`(宿主机凭据)
                                                  (失败→ErrUnavailable,
                                                   输出经 Redact)
```

## 迁移阶段状态

| Phase | 内容 | 状态 |
|---|---|---|
| 0 | mcphub 骨架(go.mod/CLAUDE.md/Taskfile/go.work/CI) | ✅ |
| 1 | mcphub/github 客户端迁入,projects 切换 | ✅ |
| 2 | mcphub/tool + mcphub/loopback,projects 委托 | ✅ |
| 3 | root 内置 `mcp-github` stdio 源(默认),保留远程 PAT 预设 | ✅ |

## Phase 3 接线(root 内置 GitHub 源)

```
builtin_registry.RegisterBuiltinTools
   └─► source github-builtin "GitHub (gh)"
        · stdio: <自身绝对路径> mcp-github   · Preset="github"(子集+scope 复用)
        · 默认关闭(同 webtools,用户自行开启);已存在则保留 enabled/visibility/tools/env
        · 远程 PAT 预设已删除(fd61fb7db):无人使用,内置源零配置覆盖同一子集;
          托管端点保存在 mcphub/github.RemoteEndpoint,未来复活归 mcphub

cli/tingly-box main.go ── "mcp-github" (hidden) ──► command.MCPGithubCmdKong
                                                          │ 无 AppManager(stdio 子进程)
                                                          ▼
                                              mcphub/github.Serve(ctx)
                                                  · mcp.Server("tingly-github")
                                                  · 11 个官方命名工具(gh api)
                                                  · GitHubRepos scope 由 root runtime
                                                    的 CheckToolScope 按 owner/repo 强制
```

## 设计要点

- **宿主关注点留在两侧**:线程身份、权限(coordinator turn / grant 规则 / root 的 GitHubRepos scope)都不进 mcphub;mcphub 只认"验证过的 JSON 输入"。
- **凭据策略**:mcphub 不管理 token,靠宿主机 `gh auth login`;一切输出先过 `Redact`。
- **两套栈收敛**:新代码统一 go-sdk v1.6.1(三方锁步);root 旧 mark3labs mcp-go(local/builtin_server)可共存,不混包。
- **UX 不变**:对模型可见的工具名、schema(golden test 锁定)、capability URL 语义全部保持;`Add` 保留"同 key 复用 config"的 memoization。
