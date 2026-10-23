# MCP 与工具

路径：`/mcp/sources`（**MCP**）、`/tools/servertool`（**Servertool**）、`/mcp/local-mode`

![MCP 工具](../images/mcp.png)

MCP（Model Context Protocol）工具扩展支持为 Claude Code 等场景注册外部工具服务器，同时提供内置的 Web 工具。**Tools** 导航项有两行：**MCP** 和 **Servertool**。

> **注意**：MCP Tools 是一个可选的 Power-up（*MCP Tools*，标为 *Exp.*）：在 [Power-ups 菜单](./02-scenario-overview.md#power-ups齿轮背后的菜单)或[实验性功能](./19-experimental.md)里打开后，**Tools** 导航项才会出现。

---

## MCP（`/mcp/sources`）

两步配置的页面。

### 01 · Config your agents

*Register the gateway with your coding agent. Run once per machine.* **Pick your runtime**——**Claude Code**、**Codex** 或 **OpenCode**——深色代码块会显示对应的注册方式，并带复制按钮：

| Runtime | 要运行的内容 |
|---------|--------------|
| Claude Code | `claude mcp add --transport http tb "<Base URL>/api/v1/mcp/tb" --header "Authorization: Bearer $(cat ~/.tingly-box/config.json | jq -r '.user_token')"` |
| Codex | `codex mcp add tb -- http <endpoint> --header "Authorization: Bearer …"` |
| OpenCode | 写入 `~/.config/opencode/opencode.json` 的 `mcp` 条目（把 `MY_API_KEY` 设为你的令牌；页面说明了如何读取） |

令牌在运行时从你本地的配置（`~/.tingly-box/config.json`）读取——不用复制粘贴密钥——并且端点跟随网关的真实地址，所以不在默认端口上时也是正确的。

### 02 · Config your tools

*Built-in and remote MCP tools available to your agents.* 控件：**Collapse all** 和筛选 **All / Active / Off**。每个工具是一张卡片，带启用开关和一个徽标——**Client**（由客户端侧执行）或 **Server**（由网关执行）。

**内置 Web 工具**（Client）

| 工具 | 说明 |
|------|------|
| **mcp_web_search** | *Browser-side web search via Serper.* 需要 **Serper API Key**（打码，可显示）——填写后点 **Save** |
| **mcp_web_fetch** | *Fetches public web pages via Jina Reader and returns markdown for agents to read.* **Fetch engine** 为 `jina.ai/reader` |

**自定义 MCP 服务器**——每个已注册的服务器是一张卡片，带 **Server** / **Client** 可见性徽标、传输徽标（`STDIO` / `HTTP` / `SSE`）、它的命令或 URL、启用开关以及 **Edit** / **Delete** 图标。列表末尾的 **Add server** 卡片（*Add a remote MCP server via stdio, HTTP or SSE*）打开 **Connect custom MCP server**：

- **Name**：服务器 id
- **Transport**：**STDIO**（*Command to launch*、*Environment variables*、*Environment variable passthrough*、*Working directory*）或 **HTTP** / **SSE**（*HTTP endpoint* / *SSE endpoint*；工作目录只对 STDIO 有效）
- **Proxy**：**Use global proxy configuration**，或填一个代理 URL
- **Tools**：`*` 表示全部，或以空格分隔的工具名列表（如 `mcp_web_search mcp_web_fetch`）

编辑已有服务器时标题是 *Edit — <id>*。**Docs** 和 **Use Weather Example** 快捷入口帮助你上手。

---

## MCP 本地模式（`/mcp/local-mode`）

![MCP 本地模式](../images/mcp-local-mode.png)

配置 Claude Code 将 Tingly-Box 作为 MCP 服务器使用。该页面仍然通过 `/mcp/local-mode` 提供，但不再是侧边栏里的一行——MCP 页面的 01 步骤涵盖了同样的注册。

页面顶部显示当前状态：
- **Active**（绿色）：MCP 服务正在运行，外部客户端可以连接
- 提示信息：`Tingly-Box is running in Client Tool mode. Register MCP sources in the Sources page, then connect your MCP client using the instructions below.`

### Connection Information

展示 **MCP Endpoint URL**（完整地址，含认证信息），是 Claude Code 连接所需的端点。

### Connect Claude Code

**方法一：Claude CLI**

```bash
claude mcp add --transport http tb "<mcp-endpoint-url>" \
  --header "Authorization: Bearer $(cat ~/.tingly-box/config.json | jq -r '.user_token')"
```

命令会自动从 `~/.tingly-box/config.json` 中读取 User Token 作为 Bearer Token，无需手动输入。

**方法二：手动配置文件**

将以下片段添加到 Claude Desktop 配置文件（含认证 Header）：

```json
{
  "mcpServers": {
    "tb": {
      "url": "<mcp-endpoint-url>",
      "headers": { "Authorization": "Bearer <your-token>" }
    }
  }
}
```

页面标注了各操作系统下配置文件的默认路径。

---

## Server Tool（`/tools/servertool`）

路径：`/tools/servertool`

![Server Tool](../images/servertool.png)

**Config your server tools**——*In-process tools injected by the gateway into every AI request.* 页面有同样的 **Collapse all** / **All / Active / Off** 控件，每个服务端工具一张卡片，带启用开关。目前页面列出 **Advisor**（标为 **Server** 和 **Experimental**）：一个*用于疑难决策的子 LLM 咨询工具*——Agent 可以调用的进程内工具，用来咨询第二个模型。用 **Choose Model** 选择第二个模型并 **Save**。

---

## 相关页面

- [实验性功能](./19-experimental.md)
- [防护栏](./15-guardrails.md)
- [Claude Code 场景](./03-scenario-claude-code.md)
