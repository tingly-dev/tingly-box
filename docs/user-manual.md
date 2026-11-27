# Tingly Box User Manual

**Tingly Box** is a high-performance LLM proxy providing a unified OpenAI-compatible API for hundreds of model providers.

---

## 1. Installation & Setup

### Method 1: npm / npx (Recommended)
Requires Node.js 18+.

Run one-shot with npx (fetches the release, restarts the server in the
background and opens the web UI):
```bash
npx -y tingly-box@latest
```

Or install globally, then manage the server explicitly:
```bash
npm install -g tingly-box
tb start    # background by default; pass --no-daemon for foreground
```

The binary for your platform is an npm package too (`@tingly-dev/tingly-box-linux-x64`,
`@tingly-dev/tingly-box-darwin-arm64`, …), installed automatically alongside
`tingly-box`. Nothing is fetched from GitHub, so an npm mirror registry
(`--registry=https://registry.npmmirror.com`) is all a restricted network
needs.

`tingly-box` and `tb` are the same CLI; running it with no arguments shows
help. To update a global install: `npm install -g tingly-box@latest`, then
`tb restart` to switch the running server to the new version. `restart` acts
immediately — like `stop`, the command itself is the intent — so be aware it
interrupts any in-flight AI requests.


### Method 2: Docker
Run as a background container:
```bash
mkdir tingly-data
docker run -d \
  --name tingly-box \
  -p 12580:12580 \
  -v "$(pwd)/tingly-data:/app/.tingly-box" \
  ghcr.io/tingly-dev/tingly-box
```

For Docker Compose, building your own image, or troubleshooting, see the [Docker Guide](./docker.md).

---

## 2. Authentication Strategy
Tingly Box uses two distinct JWT-based tokens. You can view yours by running `tingly-box token`.

| Token Type | Prefix | Target | Use Case |
| :--- | :--- | :--- | :--- |
| **User Token** | `tingly-user-` | Management API / UI | Accessing the dashboard and changing config. |
| **Model Token** | `sk-tingly-` | /openai and /anthropic | The API key you put into your Python/Node apps. |

---

## 3. Provider Management

Tingly Box supports two types of provider authentication:

| Type | How to Add | Description |
|:---|:---|:---|
| **API Key** | Web UI or CLI | Uses API tokens for authentication. Simple, works everywhere. |
| **OAuth** | Web UI only | Uses OAuth 2.0 flow for authentication. Works when API keys are unavailable. |

### Adding API Key Providers

#### Using CLI

```bash
# OpenAI example
tingly-box provider add my-openai https://api.openai.com/v1 sk-your-key openai

# Anthropic example
tingly-box provider add my-anthropic https://api.anthropic.com sk-your-key anthropic
```

#### Using Web UI

1. **Open the Web UI** at **http://localhost:12580**
2. **Navigate to the Providers page** and click the **"Add Provider"** button
3. **Fill in the provider details**:
   - Name: A unique identifier for this provider
   - API Base: The provider's API endpoint URL
   - API Style: Choose `openai` or `anthropic` based on the provider
   - Token: Your API key
4. **Click "Add"** to save the provider

### Adding OAuth Providers

OAuth providers can only be added through the Web UI, as they require an interactive authorization flow.

#### Available OAuth Providers

| Provider | Status | Description |
|:---|:---:|:---|
| **Claude Code** | ✅ Available | Anthropic Claude Code with full API access |
| **Gemini CLI** | 🔜 Coming Soon | Google Gemini models |
| **Qwen Code** | 🔜 Coming Soon | Alibaba Qwen models |


#### Adding an OAuth Provider

1. **Open the Web UI** at **http://localhost:12580**

2. **Navigate to the OAuth page** and click the **"Add Auth Provider"** button

3. **Select a provider** from the dialog (e.g., "Claude Code")

4. **Click "Authorize"** — a new browser window will open, redirecting you to the provider's OAuth authorization page (e.g., `https://claude.ai/oauth/authorize`)

5. **Complete the authorization flow** on the provider's site:
   - Log in to your account
   - Grant the requested permissions (e.g., `org:create_api_key`, `user:profile`, `user:inference`, `user:sessions:claude_code`)
   - Confirm authorization

6. **Automatic redirect** — after authorization, the provider redirects back to Tingly Box at `http://localhost:12580/callback`

7. **Provider created** — Tingly Box automatically:
   - Exchanges the authorization code for an access token
   - Creates a new provider with auto-generated name (e.g., `claude_code-a1b2c3`)
   - Stores the OAuth credentials securely
   - Displays a success confirmation

#### Managing OAuth Providers

Once added, OAuth providers appear in your provider list with special indicators:

- **Auth Type**: Displayed as "OAuth" instead of "API Key"
- **Token Management**: Access token, refresh token, and expiry time are stored
- **Auto-Refresh**: When the access token expires, Tingly Box automatically uses the refresh token to get a new one

OAuth providers work exactly like regular API key providers—you can use them in load balancing rules, call them via the unified endpoints, view token usage statistics, and delete them when no longer needed.

#### Security Notes

- The OAuth flow uses **PKCE** (Proof Key for Code Exchange) for enhanced security
- State parameters prevent CSRF attacks during the authorization flow
- Refresh tokens are stored separately and never exposed in API responses

### Supported API Styles

Tingly Box handles the translation between formats. If you add an Anthropic provider, you can still call it using the OpenAI SDK.

| Style | Description |
|:---|:---|
| **openai** | Standard JSON structure used by OpenAI, DeepSeek, Groq, etc. |
| **anthropic** | Specific message structure used by Claude models. |

---

## 4. Load Balancing & Rules
This is the core logic of Tingly Box. It maps a "virtual" model name to one or more "physical" providers.

```mermaid
flowchart TD
    A[Incoming Request<br/>model: 'gpt-3.5-turbo'] --> B{Matching Rule}
    
    subgraph Services ["Available Services for Rule"]
        C1["Service 1: openai-primary<br/>(Weight: 3)"]
        C2["Service 2: openai-backup<br/>(Weight: 1)"]
        C3["Service 3: custom-provider<br/>(Weight: 1)"]
    end

    B --> C1
    B --> C2
    B --> C3

    C1 --> D[Tactic: Weighted Round Robin]
    C2 --> D
    C3 --> D

    D --> E([Selected Provider & Model])
```
### The Architecture
1. **Rule**: Matches an incoming `model` name (e.g., `gpt-4`).
2. **Service**: A list of actual providers attached to that rule.
3. **Tactic**: The logic used to choose between them.

### Tactic Types
| Tactic | Description |
| :--- | :--- |
| **Random** | Selects a provider based on assigned `weight`. |
| **Round Robin** | Cycles through providers every $N$ requests. |
| **Token-Based** | Distributes based on historical token usage (coming soon). |

---

## 5. Integration Examples

### OpenAI SDK (Python)
```python
from openai import OpenAI

client = OpenAI(
    api_key="sk-tingly-model-xxxx", # Your Model Token
    base_url="http://localhost:12580/openai/v1"
)

response = client.chat.completions.create(
    model="gpt-3.5-turbo",
    messages=[{"role": "user", "content": "Hello Tingly!"}]
)
```

### Claude Code Integration
To use Tingly Box with the Claude CLI tool, edit your `~/.claude/settings.json`:
```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-tingly-model-xxxx",
    "ANTHROPIC_BASE_URL": "http://localhost:12580/anthropic/v1",
    "ANTHROPIC_MODEL": "claude-3-5-sonnet-latest"
  }
}
```

## 6. Advanced Configuration

### Error Log Filtering
Configure `~/.tingly-box/global.json` to reduce log noise.
```json
{
  "error_log_filter_expression": "StatusCode >= 400 && Method != 'GET'"
}
```

### Files & Locations
* **Config**: `~/.tingly-box/config.json` (Provider data)
* **Stats**: `~/.tingly-box/state/stats.db` (SQLite DB of token usage)
* **Logs**: `~/.tingly-box/logs/bad_requests.log`

---

## 7. Troubleshooting

* **Server won't start?** Ensure port `12580` isn't taken. Use `tingly-box start --port 9000` to change it.
* **No models showing?** Check if your provider token is valid by testing it with a direct `curl` to the vendor.
* **401 Unauthorized?** You are likely using the **User Token** where the **Model Token** is required, or vice-versa.

## Claude Code Enterprise

### 认证方式

Claude Enterprise 订阅与 Claude Platform API 是不同的认证及计费体系。企业订阅的 OAuth 登录由官方 Claude Code 完成；第三方应用的受支持认证方式为 Claude Platform API key 或组织批准的云推理服务凭证。参见 [Anthropic authentication and credential use](https://code.claude.com/docs/en/legal-and-compliance#authentication-and-credential-use)。

排查企业账号接入时，先记录失败阶段、HTTP 状态和脱敏后的上游错误。浏览器授权、授权码换取 token、token 刷新和模型请求分别对应不同的检查项。企业席位及组织策略由管理员确认。

### 推荐的企业接入路径

- 需要使用企业订阅：在官方 Claude Code 中登录自己的企业账号，由管理员确认席位和 Claude Code 使用权限。
- 需要通过 TB 调用模型：在 TB 中创建 Anthropic API provider，填写组织批准的 Claude Platform API key，使用默认 API endpoint，并配置模型路由。也可根据组织的部署方式使用 TB 已提供的 Bedrock、Vertex AI 等 provider。不要将企业订阅会话 token 作为 API key。
- 在企业允许使用网关的情况下，使用 TB 的 Claude Code 场景配置生成的 endpoint 和 TB model token；通过 `/status` 检查生效的配置。不要覆盖组织下发的 managed settings。

### 定位失败阶段

| 阶段 | 排查内容 |
| --- | --- |
| 官方 CLI 登录 | 企业账号、SSO、席位及组织是否允许 Claude Code |
| OAuth 回调 | 回调是否成功到达；是否收到 OAuth error、state 是否匹配 |
| Token 交换/刷新 | HTTP 状态、错误类型、是否存在 access_token（不要记录值） |
| TB API 请求 | provider 类型、API base、模型以及 HTTP 状态；API key 不应与订阅 token 混用 |
| 企业设置 | `/status` 查看生效的 managed settings；`claude doctor` 检查配置 |

若需进一步确认根因，应提供脱敏后的失败阶段、HTTP 状态、错误类型及 TB/Claude Code 版本。不要提供授权码、access token、refresh token、API key 或完整认证请求。

参考：[Claude Code troubleshooting](https://code.claude.com/docs/en/troubleshooting)、[settings precedence](https://code.claude.com/docs/en/settings#settings-precedence)。
