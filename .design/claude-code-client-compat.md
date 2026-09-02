# Claude Code 客户端兼容层：逆向分析与实现映射

> 受众：维护 Claude OAuth 链路（`internal/client/claude_client.go` 及其周边）的
> 后端贡献者；下一次 Anthropic 抬高 Claude Code 最低版本时负责"升版"的人。
>
> 本文记录 tingly-box 如何把发往 Claude OAuth provider 的请求"重签"为官方
> Claude Code CLI 的样子：哪些 wire 元素是被模拟的、它们在官方客户端里是怎么
> 生成的、我们逐项对齐到了 2.1.258 的什么行为、哪些地方是有意不对齐的。
> 分析结论全部来自官方 npm 包的逆向 + 真实二进制的抓包，可复现（§2）。

---

## 0. TL;DR

| 项目 | 2.1.86（旧实现） | 2.1.258（当前） | 代码位置 |
|---|---|---|---|
| 版本单一来源 | 散落在 3 处字面量 | `constant.ClaudeCodeVersion` | `internal/constant/claude_code.go` |
| `User-Agent` | `claude-cli/2.1.86 (external, cli)` | `claude-cli/2.1.258 (external, cli)` | `constant.ClaudeCodeUserAgent()` |
| `X-Stainless-Package-Version` | `0.74.0` | `0.112.1` | `claude_round_tripper.go` |
| `X-Stainless-Runtime-Version` | `v24.3.0` | `v26.3.0`（Bun 1.4.1 原生二进制） | 同上 |
| `X-Stainless-OS` / `-Arch` | Go 的 `linux`/`amd64`（❌ 与真实客户端不一致） | SDK 映射名 `Linux`/`x64`、`MacOS`/`arm64` | `stainlessOSName` / `stainlessArchName` |
| `x-stainless-helper-method: stream` | 发送（❌ 真实客户端从不发） | 不发送 | — |
| `anthropic-beta` | 固定字符串（含已废弃的 `token-efficient-tools-2026-03-28`） | 按 model + 请求体 + 入站白名单**逐请求合成**，按官方 push 顺序输出 | `internal/client/claude_betas.go` |
| billing header `cch` | 随机 5 hex（❌ 官方恒为 `00000`） | `cch=00000;` 常量；保留入站的 `cc_workload / cc_is_subagent / cc_prev_req / cc_prompt_id` | `internal/protocol/ops/claude_code_billing_header.go` |
| fingerprint 输入文本 | 首条 user 消息的第一个 text block | 跳过 `<system-reminder>` 块后的第一个 text block（2.1.258 把 reminder 变成了 meta 消息） | `ops.extractFirstUserMessageText` |
| `metadata.user_id` | `{device_id, account_uuid, session_id}` | 同上 + 透传 `parent_session_id`（子 agent） | `ops.MetadataUserID` |
| 子 agent 头 | 无 | 透传 `x-claude-code-agent-id` / `x-claude-code-parent-agent-id` | `typ.ClaudeCodeClientHints` |
| System preamble | `You are Claude Code, Anthropic's official CLI for Claude.` | 不变 | `client.ClaudeCodeSystemHeader` |
| 地理位置隐写（`Today’s` / `2026/09/02`） | 有 normalizer | 两个版本的 bundle 里都**没找到**相关代码；normalizer 保留 | `transform_clean_header.go` |

触发事件：用户在真实使用中收到

```
400 {"type":"error","error":{"type":"invalid_request_error",
 "message":"Claude Code 2.1.86 does not support this model; version 2.1.251 or newer is required. ...",
 "details":{"error_code":"claude_code_version_too_old"}}}
```

即 Anthropic 按 `User-Agent` 里的 claude-cli 版本做门控。只改版本号是不够的：
同一时期 SDK 版本、beta 列表、billing header 字段、metadata 结构都变了，
一个"2.1.258 的 UA + 2.1.86 的其他一切"是明显的指纹异常。

---

## 1. 我们在模拟什么（wire 元素清单）

Claude OAuth 链路（`ClientPool.GetAnthropicClient` → `NewClaudeClient`）的每个
请求由下面几层拼出来。术语对应用户口中的四类内容：

| 用户术语 | 具体内容 | 产生位置 |
|---|---|---|
| **client header** | `User-Agent`、`x-app`、`X-Claude-Code-Session-Id`、`X-Stainless-*`、`anthropic-beta`、`anthropic-version`、`anthropic-dangerous-direct-browser-access`、`accept`、子 agent 头 | `internal/client/claude_client.go`（`applyClaudeCodeHeaders` + `perRequestOptions`）、`claude_betas.go` |
| **system header** | `system[0]` 的 `x-anthropic-billing-header: ...` 文本块；`system[1]` 的身份 preamble | `internal/protocol/ops/request_anthropic_model.go`（`ApplyAnthropic*MetadataTransform`）、`claude_code_billing_header.go` |
| **meta data** | `metadata.user_id` 的 JSON 串 | `internal/protocol/ops/metadata_user_id.go` |
| **clean header** | 转发到**非** Claude OAuth provider 时剥掉 billing header / 隐写标记 / preamble | `internal/protocolserver/transform/transform_clean_header.go`；flag 解析见 `rule_flags.go`（Claude OAuth provider 上自动关闭） |

---

## 2. 分析方法（可复现）

### 2.1 获取官方包

```bash
curl -sS https://registry.npmjs.org/@anthropic-ai/claude-code | jq '.["dist-tags"]'
# {"stable":"2.1.236","latest":"2.1.258","next":"2.1.258"}   (2026-09-02)
curl -sSL https://registry.npmjs.org/@anthropic-ai/claude-code/-/claude-code-2.1.86.tgz  | tar xz
curl -sSL https://registry.npmjs.org/@anthropic-ai/claude-code/-/claude-code-2.1.258.tgz | tar xz
```

**打包形态在 2.1.251 前后变了**：

- `≤ 2.1.2xx`（含 2.1.86）：`package/cli.js` 是 12 MB 的 bundle，可直接用 node 跑，也可直接 grep。
- `≥ 2.1.251`：主包只剩 `install.cjs` / `cli-wrapper.cjs` / `bin/claude.exe`，真正的 CLI 是
  平台原生 Bun 二进制，来自 `optionalDependencies`：`@anthropic-ai/claude-code-{darwin-arm64,darwin-x64,linux-x64,linux-arm64,linux-x64-musl,linux-arm64-musl,win32-x64,win32-arm64}`。
  ```bash
  curl -sSL https://registry.npmjs.org/@anthropic-ai/claude-code-linux-x64/-/claude-code-linux-x64-2.1.258.tgz | tar xz
  ./package/claude --version   # 2.1.258 (Claude Code)
  ```
  这也解释了 `X-Stainless-Runtime-Version` 的变化：不再是用户机器的 node，而是 Bun 伪装的 Node 版本（Bun 1.4.1 → `v26.3.0`）。

### 2.2 从原生二进制里取 JS 源

Bun 单文件可执行把 bundle **明文**嵌在 ELF 里（`/$bunfs/root/*.js`），没有 bytecode 编译。
二进制里 `claude-cli/` 出现两次：第一次（≈97 MB 偏移）是 JSC 字符串表的碎片，
第二次（≈179 MB 偏移）落在完整源码区。向两侧扫"连续可打印字节区"即可切出约 32 MB 的
bundle（含全部 863 个 chunk），之后的分析方法与 `cli.js` 完全一样：

```python
data = open('claude','rb').read()
i = data.find(b'claude-cli/', data.find(b'claude-cli/') + 1)   # 第二次出现
def txt(b): return b in (9,10,13) or 32 <= b < 127 or b >= 128
lo = hi = i; n = 0
while lo > 0 and n <= 2: n = n + 1 if not txt(data[lo-1]) else 0; lo -= 1
n = 0
while hi < len(data) and n <= 2: n = n + 1 if not txt(data[hi]) else 0; hi += 1
open('cli258.js','wb').write(data[lo:hi])
```

### 2.3 静态分析：怎么定位关键函数

bundle 是 minified 的，符号名每版都变，靠字面量锚定：

| 要找的东西 | 锚定字符串 | 2.1.258 中的形态 |
|---|---|---|
| SDK client 创建 / 默认头 | `"x-app":` | `async function jF({apiKey,maxRetries,model,...}){... G={"x-app":St()?"cli-bg":"cli","User-Agent":rI(),[TFe]:Q(), ...}` |
| User-Agent | `claude-cli/` | `` `claude-cli/${VERSION} (external, ${CLAUDE_CODE_ENTRYPOINT??"cli"}${agent-sdk}${client-app}${workload})` `` |
| billing header 渲染 | `x-anthropic-billing-header: cc_version` | `function r4t(fp, agentContext, prevReqId, promptId, opts)` |
| fingerprint | `59cf53e54c78` | `Nct(text, version)`：`[4,7,20].map(i=>text[i]||"0")`，`sha256(salt+chars+version).slice(0,3)` |
| beta 注册表 | `"interleaved-thinking-2025-05-14"` | `Ee(name, header)` 冻结对象；`XZ` 为全量注册表 |
| beta 组成 | `DISABLE_INTERLEAVED_THINKING` | `function Jne(model)`（allModelBetas）；`rEt` 追加 per-query flag |
| metadata.user_id | `account_uuid:` 附近的 `session_id:` | `function XH({agentContext})` |
| SDK 版本 | `X-Stainless-Package-Version` 引用的变量 | `var se="0.112.1"` |
| OS/Arch 映射 | `"MacOS"` | SDK 的 `xs(platform)` / `Ss(arch)` |
| system preamble | `"You are Claude Code` | 三个变体：CLI / Agent SDK 内的 CLI / 纯 Agent SDK |

### 2.4 动态抓包（最可信的证据）

把 `ANTHROPIC_BASE_URL` 指到本地假 API，让**真实二进制**自己发请求，落盘头和 body：

```python
# fake_api.py  <port> <outdir>  —— 对 POST /v1/messages 回一段最小 SSE，其余 404
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json, sys, os
class H(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    def do_POST(self):
        n = int(self.headers.get('Content-Length') or 0); body = self.rfile.read(n)
        json.dump({'path': self.path, 'headers': dict(self.headers), 'body': json.loads(body)},
                  open(os.path.join(sys.argv[2], 'req.json'), 'w'), indent=1)
        evs = [('message_start', {'type':'message_start','message':{'id':'msg_01','type':'message','role':'assistant',
                 'model':'x','content':[],'stop_reason':None,'usage':{'input_tokens':1,'output_tokens':1}}}),
               ('content_block_start', {'type':'content_block_start','index':0,'content_block':{'type':'text','text':''}}),
               ('content_block_delta', {'type':'content_block_delta','index':0,'delta':{'type':'text_delta','text':'hi'}}),
               ('content_block_stop', {'type':'content_block_stop','index':0}),
               ('message_delta', {'type':'message_delta','delta':{'stop_reason':'end_turn'},'usage':{'output_tokens':1}}),
               ('message_stop', {'type':'message_stop'})]
        data = ''.join(f'event: {e}\ndata: {json.dumps(d)}\n\n' for e, d in evs).encode()
        self.send_response(200); self.send_header('Content-Type','text/event-stream')
        self.send_header('Content-Length', str(len(data))); self.end_headers(); self.wfile.write(data)
ThreadingHTTPServer(('127.0.0.1', int(sys.argv[1])), H).serve_forever()
```

```bash
mkdir -p h && echo '{"hasCompletedOnboarding":true}' > h/.claude.json
env -i PATH=$PATH TERM=xterm HOME=$PWD/h \
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 DISABLE_TELEMETRY=1 DISABLE_ERROR_REPORTING=1 \
    ANTHROPIC_BASE_URL=http://127.0.0.1:18091 \
    CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-fake \          # 或 ANTHROPIC_API_KEY=sk-ant-api03-fake
    ./claude -p "say hi" --model claude-sonnet-4-6 < /dev/null
```

抓包时的坑（都踩过）：

- **必须 `env -i`**。宿主环境里若有 `CLAUDE_CODE_ENTRYPOINT=remote`、`CLAUDE_CODE_OAUTH_TOKEN`、
  `CLAUDE_CODE_CONTAINER_ID` 等，二进制会原样带上（UA 变成 `(external, remote)`，还会用宿主的真 token）。
- `-p` 模式的 entrypoint 是 `sdk-cli`，system prompt 也是 Agent SDK 变体；交互式终端才是 `cli`。
  交互式与 `-p` 在 beta 上只差一个 `redact-thinking-2026-02-12`（`-p` 不发）。
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` 会跳过 GrowthBook 拉取，所有 `tengu_*` 灰度 gate 走默认值
  （例如 `structured-outputs` 的 `tengu_tool_pear` 默认 false）。线上用户拿到的 gate 值不可知。
- 假 API 若不给 `request-id` 响应头也没关系，但真实服务会给，2.1.258 会把它作为下一请求的 `cc_prev_req`（仅直连时）。

---

## 3. 逐项对比：2.1.86 → 2.1.258

### 3.1 Client headers（抓包，`-p` 模式，OAuth token）

```
# 2.1.86 (node cli.js)                          # 2.1.258 (native binary)
User-Agent: claude-cli/2.1.86 (external, sdk-cli)   claude-cli/2.1.258 (external, sdk-cli)
x-app: cli                                          cli
X-Claude-Code-Session-Id: <uuid>                    <uuid>
X-Stainless-Lang: js                                js
X-Stainless-Package-Version: 0.74.0                 0.112.1
X-Stainless-OS: Linux                               Linux
X-Stainless-Arch: x64                               x64
X-Stainless-Runtime: node                           node
X-Stainless-Runtime-Version: v20.20.2 (宿主 node)    v26.3.0 (Bun 1.4.1)
X-Stainless-Retry-Count: 0                          0
X-Stainless-Timeout: 600                            600
anthropic-version: 2023-06-01                       2023-06-01
anthropic-dangerous-direct-browser-access: true     true
Accept: application/json                            application/json
Authorization: Bearer sk-ant-oat01-…                （同）
anthropic-beta: …见 3.2                             …见 3.2
POST /v1/messages?beta=true                         同
```

要点：

- **没有 `x-stainless-helper-method`**。CLI 直接 `beta.messages.create({stream:true})`，不走 `.stream()` helper；
  该头只在 SDK 的 tool-runner 路径出现。旧实现固定发 `stream` 是错的，已移除。
- `X-Stainless-OS/Arch` 是 SDK 对 `process.platform/arch` 的映射（`darwin→MacOS`、`win32→Windows`、`linux→Linux`；
  `x64/arm64/x32/arm`）。旧实现直接发 Go 的 `runtime.GOOS/GOARCH`（`darwin`/`amd64`），真实客户端从不会这样。
- `x-app` 在后台会话（`CLAUDE_CODE_SESSION_KIND=bg`）时是 `cli-bg`；我们固定 `cli`。
- 2.1.258 新增可选头：`x-claude-code-agent-id`、`x-claude-code-parent-agent-id`（子 agent 上下文）、
  `x-claude-remote-container-id`、`x-claude-remote-session-id`（CCR）、`x-client-app`（Agent SDK 宿主）、
  `x-anthropic-additional-protection`。我们只透传前两个（真实子 agent 请求经过 tingly 时会带）。
- Header 值经过 `C4t` 校验（拒绝非法 header value）；agent id 用 `SAn` 做百分号编码（`%` 及非可打印 ASCII）。
  `client.sanitizeClaudeHeaderValue` 复刻了它。

### 3.2 `anthropic-beta`

#### 3.2.1 官方逻辑（2.1.258，去混淆后的伪代码）

```js
// allModelBetas(model) —— 每个请求都会带的"基线"
betas = []
if (!model.includes("haiku"))                       betas.push("claude-code-20250219")
if (isClaudeAiOAuth())                              betas.push("oauth-2025-04-20")
if (/\[1m\]/i.test(model) && !DISABLE_1M_CONTEXT)   betas.push("context-1m-2025-08-07")
if (!DISABLE_INTERLEAVED_THINKING && supportsInterleaved(model))
                                                    betas.push("interleaved-thinking-2025-05-14")
if (firstParty && supportsInterleaved(model) && isInteractive && !showThinkingSummaries)
                                                    betas.push("redact-thinking-2026-02-12")
if (supportsInterleaved(model) && !DISABLE_EXPERIMENTAL_BETAS && provider=="firstParty")
                                                    betas.push("thinking-token-count-2026-05-13")
if (!model.includes("claude-3-") && !DISABLE_EXPERIMENTAL_BETAS)
                                                    betas.push("context-management-2025-06-27")
if (growthbook("tengu_tool_pear") && supportsStructured(model))
                                                    betas.push("structured-outputs-2025-12-15")   // 灰度
if (provider=="vertex"||"foundry")                  betas.push("web-search-2025-03-05")
if (firstParty)                                     betas.push("prompt-caching-scope-2026-01-05")
if (supportsMidConversationSystem(model))           betas.push("mid-conversation-system-2026-04-07")
betas.push(...ANTHROPIC_BETAS.split(","))

// per-query (rEt)
if (perTurnEffort(model))                           betas.push("per-turn-control-2026-07-01")
if (growthbook("tengu_mossy_lantern") && midConvToolChange(model))
                                                    betas.push("mid-conversation-tool-changes-2026-07-01")

// 主查询循环里按调用顺序追加
effort 存在              → "effort-2025-11-24"
task_budget 存在         → "task-budgets-2026-03-13"
output_config.format     → "structured-outputs-2025-12-15"
thinking.display=updates → "thinking-display-updates-2026-08-18"
fast mode                → "fast-mode-2026-02-01"        (body: speed:"fast")
auto mode                → "afk-mode-2026-01-31"
cache ttl 1h             → "extended-cache-ttl-2025-04-11"
context hint             → "context-hint-2026-04-09"
evict-on-complete        → "prompt-caching-evict-2026-05-12"
cache diagnosis          → "cache-diagnosis-2026-04-07"
tool search 工具在列     → "advanced-tool-use-2025-11-20"（Bedrock/Vertex 为 "tool-search-tool-2025-10-19"）
```

model 能力判断（`Ve()` 归一化后：小写、去 `[1m]`、去 `-YYYYMMDD` 快照日期）：

| 判断 | 为假的模型 |
|---|---|
| `supportsInterleaved` | `claude-haiku-4-5`、`claude-3-*` |
| `supportsContextManagement` | `claude-3-*` |
| `supportsMidConversationSystem` | `claude-3-*`、opus 4.0/4.1/4.5/4.6/4.7、sonnet 4.0/4.5/4.6、haiku 4.5；其余（sonnet-5 / opus-5 / mythos / fable …）为真 |

count_tokens 只保留 `{claude-code, interleaved-thinking, context-management, oauth}`。

#### 3.2.2 抓包实例

```
2.1.86  OAuth sonnet-4-6 -p : claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24
2.1.258 OAuth sonnet-4-6 -p : claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24,extended-cache-ttl-2025-04-11
2.1.258 API-key sonnet-4-6 -p: claude-code-20250219,interleaved-thinking-2025-05-14,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24
```

（2.1.258 OAuth 多出 `extended-cache-ttl`：订阅用户的 system 块 `cache_control` 带 `ttl:"1h"`。）

#### 3.2.3 旧实现的问题

旧的固定串 `claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,structured-outputs-2025-12-15,fast-mode-2026-02-01,redact-thinking-2026-02-12,token-efficient-tools-2026-03-28`：

- `token-efficient-tools-2026-03-28` 在 2.1.86 里已经是空串常量（`g54=""`），2.1.258 注册表里根本没有这个 flag；
- `fast-mode` / `structured-outputs` 无条件发送，而官方只在开了 fast mode / 带 `format` 时发；
- 缺 `thinking-token-count-2026-05-13`（2.1.258 基线）；
- 对 haiku 也发 `claude-code-20250219`；
- 顺序与官方 push 顺序不一致（`redact-thinking` 应紧跟 `interleaved-thinking`）。

#### 3.2.4 现在的做法（`internal/client/claude_betas.go`）

三层合成，按官方 push 顺序（`claudeCodeBetaEmissionOrder`）输出成**单个** header 值：

1. **基线**：按上表用 outbound model 判断，persona 固定为交互式终端（含 `redact-thinking`）；
   `oauth` 仅在 provider 凭证是 `sk-ant-oat…` 时加；`context-1m` 来自 `context_1m` rule flag
   （`applyContextOneM` 已把入站 header 里的 1M 转成 flag）。
2. **请求体派生**：`output_config.effort` → effort；`output_config.format` / `output_format` → structured-outputs；
   `output_config.task_budget` → task-budgets；`thinking.display=="updates"` → thinking-display-updates；
   `speed=="fast"` → fast-mode；任一 `cache_control.ttl=="1h"`（system / tools / messages）→ extended-cache-ttl；
   tools 含 `tool_search_tool_*` → advanced-tool-use。
3. **入站回放（白名单）**：真实 Claude Code 客户端自己协商出的、无法从 body 反推的 flag
   （per-turn-control、mid-conversation-tool-changes、afk-mode、context-hint、prompt-caching-evict、cache-diagnosis …）
   从入站 `anthropic-beta` 头回放；白名单之外的一律丢弃（`message-batches`、`pdfs`、`managed-agents` 等
   SDK flag 真实 CLI 从不发）。入站 header 由 `protocolserver.applyClaudeCodeClientHints` 挂进 context（`typ.ClaudeCodeClientHints`）。

`ClaudeClient` 的 `MessagesNew*/BetaMessagesNew*` 直接调 SDK 而不再经过 `AnthropicClient` 包装：
包装层的 `withContext1MBeta` 会用 `WithHeaderAdd` 再追加一次 `context-1m`，导致 Go 发出两行同名 header。
`req.Betas` 在 Guard 里清空，理由相同。

### 3.3 Billing header（system header）

#### 3.3.1 格式

```
2.1.86 : x-anthropic-billing-header: cc_version=2.1.86.d9e; cc_entrypoint=sdk-cli; cch=00000;
2.1.258: x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=sdk-cli;
```

2.1.258 渲染函数 `r4t(fp, agentContext, prevReqId, promptId, opts)`，字段顺序固定：

| 字段 | 条件（2.1.258） | 说明 |
|---|---|---|
| `cc_version=<VERSION>.<fp>` | 总是 | fp 见 3.3.2 |
| `cc_entrypoint=<ep>` | 总是 | `CLAUDE_CODE_ENTRYPOINT ?? "unknown"`；交互式 = `cli`，`-p`/SDK = `sdk-cli`，CCR = `remote` |
| `cch=00000` | `provider=="firstParty" && baseURL 是 api.anthropic.com`（或 `_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL`），或 vertex | **常量**，2.1.86 无条件发 |
| `cc_workload=<w>` | 有 AsyncLocalStorage workload（如 `cron`） | 不受 base URL 影响 |
| `cc_is_subagent=true` | agentContext 非主会话 | 不受 base URL 影响 |
| `cc_prev_req=<req_…>` | 直连 && 上一响应的 `request-id` 匹配 `^req_[A-Za-z0-9_-]{1,36}$` | 2.1.86 没有 |
| `cc_prompt_id=<uuid>` | 直连 && 当前 user turn 的 UUID | 2.1.86 没有 |

`CLAUDE_CODE_ATTRIBUTION_HEADER=0` 可整体关闭（非直连时）。

系统块布局（抓包）：`system[0]` = billing header（无 `cache_control`），`system[1]` = 身份 preamble，
`system[2]` = 主 prompt；后两者带 `cache_control:{type:"ephemeral"[,ttl:"1h"]}`（OAuth 订阅为 1h）。

#### 3.3.2 fingerprint

算法两版相同：`sha256("59cf53e54c78" + text[4] + text[7] + text[20] + VERSION).hex[:3]`（越界位用 `"0"`）。
**输入文本变了**：取"第一条非 meta 的 user 消息"的第一个 text block。

- 2.1.86 抓包：`cc_version=2.1.86.d9e`，只有用 `<system-reminder>\nThe following skills…`（首条 user 消息的第一个块）才能复现 → reminder 当时是同一条消息的一部分。
- 2.1.258 抓包：`cc_version=2.1.258.8ee`，只有用 `say hi` 才能复现；同一 wire 消息里排在前面的 4 个 `<system-reminder>` 块都对不上 → reminder 在 2.1.258 内部是独立的 meta 消息，发送时才折叠进同一条 user 消息。

因此 `ops.extractFirstUserMessageText` 现在跳过以 `<system-reminder>` 开头的 text block；一条全是 reminder 的 user 消息整体跳过；
首条 user 消息没有任何 text（纯图片）时返回 `""`（与官方 `FFo` 一致）。

#### 3.3.3 我们的实现（`ops.BuildClaudeCodeBillingHeader`）

- 始终输出 `cc_version=<pinned>.<fp>; cc_entrypoint=cli; cch=00000;`。`cch` 改为常量：旧实现的随机 5 hex 与任何官方版本都对不上。
- 入站若已有 billing header，**原地重建**（保持 `system[0]` 位置），并按官方顺序保留通过校验的
  `cc_workload / cc_is_subagent / cc_prev_req / cc_prompt_id`；校验正则与官方一致，其余键一律丢弃。
- 不合成 `cc_prev_req` / `cc_prompt_id`（见 §5）。

### 3.4 `metadata.user_id`

2.1.258 `XH({agentContext})`：

```js
{ ...CLAUDE_CODE_EXTRA_METADATA,             // 可选，超长时被裁掉
  device_id: <64 hex 设备指纹>,
  account_uuid: <claude.ai 账号 uuid 或 "">,
  session_id: <uuid>,
  ...parent ? {parent_session_id: parent} : {},   // 子 agent
  ...tk ? {tk} : {} }                              // 仅 CLAUDE_CODE_REMOTE
→ metadata.user_id = JSON.stringify(obj)
```

抓包：`{"device_id":"676852d4…","account_uuid":"","session_id":"bf97f200-…"}`（无账号时 `account_uuid` 为空串，键仍在）。
与旧实现的 `MetadataUserID{device_id,account_uuid,session_id}` 一致；新增 `parent_session_id`（`omitempty`）透传。`tk` 不建模。

### 3.5 System prompt preamble

三个身份句不变（`nke` 集合）：

```
You are Claude Code, Anthropic's official CLI for Claude.
You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK.
You are a Claude agent, built on Anthropic's Claude Agent SDK.
```

`client.ClaudeCodeSystemHeader` / `smartrouting.claudeCodeMainPreamble` / `clean_header` 的 preamble 列表都还匹配。
`-p` 模式发的是第三句（这也是抓包里看到 Agent SDK 句子的原因），交互式发第一句。

### 3.6 地理位置隐写

`transform_clean_header.go` 描述的 `Today’s`（U+2019/U+02BC/U+2018）与 `2026/09/02` 标记：
对 2.1.86 `cli.js` 和 2.1.258 全部 32 MB bundle（以及整个 215 MB ELF）搜 `Asia/Shanghai` / `Urumqi` 均为 0 命中；
`TZ=Asia/Shanghai` 抓包中 `Today's date is 2026-09-02.` 为普通 ASCII 撇号与连字符。
结论：该代码在这两个版本里不存在（可能更早被移除或本就来自其他构建）。normalizer 零成本，保留。

### 3.7 其他观察（未改代码，供后续参考）

- `thinking` 现在带 `display`（`-p` 为 `omitted`，交互式可 `summarized`，`updates` 对应 `thinking-display-updates` beta）；Go SDK 已有该字段，透传无损。
- `context_management.edits=[{type:"clear_thinking_20251015",keep:"all"}]` 两版都发；`ClaudeClient.GuardBeta` 在强制关 thinking 时剥掉它的逻辑仍然必要。
- `output_config.effort` 默认值：2.1.86 `medium`，2.1.258 `high`。
- 内置工具集变化很大（2.1.258 多了 `TaskCreate/TaskList/…`、`Workflow`、`ScheduleWakeup`、`SendMessage`、`ListAgents`、`ReportFindings`、`RemoteTrigger` 等）。
  `oauthToolRenameMap` 只负责把第三方客户端的小写名映射回 TitleCase，未受影响。
- CLAUDE_CODE_EXTRA_BODY / `anthropic_beta` body 字段仅 Bedrock 路径使用。

---

## 4. 代码映射

| wire 元素 | 生成/处理位置 | 关键符号 |
|---|---|---|
| 版本 | `internal/constant/claude_code.go` | `ClaudeCodeVersion`、`ClaudeCodeUserAgent()` |
| UA / stainless / x-app / 固定头 | `internal/client/claude_round_tripper.go`、`claude_client.go::applyClaudeCodeHeaders` | `claudeCLIUserAgent`、`stainless*`、`stainlessOSName/ArchName` |
| `anthropic-beta` | `internal/client/claude_betas.go` | `composeClaudeCodeBetas`、`claudeCodeBetaEmissionOrder`、`claudeCodeClientReplayableBetas`、`*ClaudeBetaSignals` |
| 逐请求头（session id / beta / agent id） | `claude_client.go::perRequestOptions`（Guard / GuardBeta 调用） | `sanitizeClaudeHeaderValue` |
| count_tokens beta 子集 | `claude_client.go::countTokensClient` | `filterClaudeCodeCountTokensBetas` |
| 入站 hint 采集 | `internal/protocolserver/rule_flags.go::applyClaudeCodeClientHints` | `typ.ClaudeCodeClientHints` |
| billing header | `internal/protocol/ops/claude_code_billing_header.go`；注入点 `request_anthropic_model.go::ApplyAnthropic{V1,Beta}MetadataTransform`（由 `transform/vendor.go::isClaudeCodeBackend` 门控：host 为 `api.anthropic.com`/`claude.ai`，**或** provider 是 Claude Code OAuth issuer——中继/自建 host 也要重签，否则 `ClaudeClient.Guard` 会因缺 metadata 而 panic） | `BuildClaudeCodeBillingHeader`、`IsBillingHeaderText`、`computeCCVersion`、`extractFirstUserMessageText` |
| metadata | `internal/protocol/ops/metadata_user_id.go` | `MetadataUserID.ParentSessionID` |
| clean header | `internal/protocolserver/transform/transform_clean_header.go` | 复用 `ops.IsBillingHeaderText` |
| UA 预设 | `internal/typ/flag_registry.go::DefaultUserAgents`、`frontend/src/mocks/handlers.ts` | 由 `constant` 派生 |

测试：`internal/client/claude_betas_test.go`（含 httptest wire 级断言）、`claude_round_tripper_test.go`、
`internal/protocol/ops/claude_code_billing_header_test.go`（含抓包 fingerprint `8ee`）、`metadata_user_id_parent_test.go`、
`internal/protocolserver/claude_code_hints_test.go`、`internal/protocol/transform/vendor_claude_relay_test.go`；
**端到端**：`internal/protocoltest/claude_code_identity_test.go` 让一个 Claude Code 形态的请求走完真实网关
（claude_code scenario → Claude OAuth provider → 虚拟上游），逐项断言上游收到的 UA / beta / 子 agent 头 /
billing header / metadata。升版后先跑它。

---

## 5. 决策与取舍

1. **persona 固定为"交互式终端、直连 api.anthropic.com"**。UA `(external, cli)`、`cc_entrypoint=cli`、
   `cch=00000`、基线含 `redact-thinking` 都按这个 persona 取值，不跟随入站客户端的 entrypoint
   （入站可能是 `-p`、Agent SDK、CCR、甚至不是 Claude Code）。理由：一个自洽的 persona 优于把入站的碎片拼起来。
2. **`cch` 用常量 `00000`**，放弃随机值：随机值不匹配任何官方版本，是纯粹的指纹异常；而且 2.1.258 只在直连时发它，
   与 persona 一致。
3. **不合成 `cc_prev_req` / `cc_prompt_id`**，只透传入站已有的。合成需要跨请求状态（按 session 记上一响应的 `request-id`、
   按 user turn 生成稳定 UUID），且语义（服务端是否用于缓存亲和 / 限流）未知，猜错的代价大于缺省。
   想让真实客户端自己带上这两个字段，可在其环境里设 `_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL=1`
   （同时会让客户端认为自己直连：也会发 `cch`、用 1h TTL 等），tingly 会原样保留。**未验证副作用，不默认推荐。**
4. **beta 合成而非透传**：入站 header 不可信（可能来自任意客户端），但真实 Claude Code 协商的请求级 flag 又只有它知道，
   所以是"基线合成 + body 派生 + 白名单回放"。`structured-outputs` 官方受 GrowthBook 灰度，我们改为按 body 是否带 `format` 决定。
5. **入站 UA 不透传**（延续 `.design/user-agent.md` 的"B 类特种链"结论）：pinned UA 是决定性的。
6. **stainless OS/Arch 按 SDK 映射**、**移除 `x-stainless-helper-method`**：两者都是"真实客户端从不这样发"的旧偏差。
7. **fingerprint 跟随 2.1.258 语义**（跳过 reminder）。这意味着对于仍在用 ≤2.1.2xx 老客户端的入站流量，
   我们算出的 fp 与客户端自己算的不同——但 fp 本来就要按我们声称的版本重算，客户端的值从不上行。

---

## 6. 下次升版 checklist

1. `curl registry` 看 `dist-tags.latest`；下载主包 + `claude-code-linux-x64`（§2.1），`./claude --version` 确认。
2. 用 §2.2 脚本切出 bundle；按 §2.3 的锚点逐项核对：
   - `var se="…"` → `stainlessPackageVersion`；跑一次抓包看 `X-Stainless-Runtime-Version` → `stainlessRuntimeVersion`；
   - `Ee(` 注册表：新增/删除的 flag 更新 `claude_betas.go` 常量与 `claudeCodeBetaEmissionOrder`；
   - `function Jne`（allModelBetas）与主循环里的 `.push(` 顺序 → `composeClaudeCodeBetas` 与 emission order；
   - `x-anthropic-billing-header: cc_version` 渲染函数的字段与门控 → `BuildClaudeCodeBillingHeader` 与 `billingHeaderPreservedFields`；
   - `59cf53e54c78` 是否还在、`[4,7,20]` 是否变 → `computeFingerprint`；
   - `account_uuid:`/`session_id:` 的对象字面量 → `MetadataUserID`；
   - `"You are Claude Code` 三句 → preamble 常量；
   - `"x-app":` 处的默认头 → 新增头是否需要透传。
3. 用 §2.4 抓包（API key + OAuth token 各一次，`env -i`），把 `anthropic-beta` 原文和 `cc_version` 写进测试
   （`TestComposeClaudeCodeBetas_*Capture`、`TestComputeFingerprint_MatchesLiveCapture`；注意 `-p` 与交互式差一个 `redact-thinking`）。
4. 改 `constant.ClaudeCodeVersion`，跑 `go test ./internal/client/ ./internal/protocol/ops/ ./internal/protocolserver/... ./internal/protocol/transform/`。
5. 更新本文 §0 表格与 §3 的抓包实例。

---

## 7. 未决 / 后续

- **有状态合成 `cc_prompt_id` / `cc_prev_req`**（§5.3）：若观察到直连与经代理的流量在缓存命中率或限流上有差异，值得做。
  需要：按 session（metadata.session_id）缓存上一 upstream 响应的 `request-id`；按"最后一条 user 消息是否含非 tool_result 文本"判定新 turn 并生成 UUID。
- **`x-app: cli-bg`**：后台会话 persona，目前无信号可判。
- **`thinking.display`**：`Guard` 在 thinking 未指定时强制 `disabled` 的逻辑未动；2.1.258 交互式会显式给 `adaptive` + `display`，通常不会触发。
- 抓包用的假服务与脚本没有入库（§2.4 已内联足以复现）；若需要常态化回归，可以放到 `tests/` 下做成可选的集成测试。
