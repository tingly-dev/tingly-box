# Runbook — tingly-box 阶段验收基线

> 这是「一个开发阶段结束后，发 PR 前」的**真实补充测试**操作手册。
> 入口脚本：`scripts/harness-baseline.sh`。harness 全量命令参考见
> [`cli/harness/README.md`](../cli/harness/README.md)。

## 这套测试是什么 / 不是什么

- **是**：用**真实 agent CLI**（claude / codex / opencode）打**真实 tingly-box gateway**，
  跑一遍数据面（协议转换 + 路由 + dispatch + 真实上游）。
- **不是 CI**：它依赖开发者本机安装的 agent CLI，以及（真实模式下）provider 凭证。
  CI 侧的 hermetic 覆盖由 `.github/workflows/harness-matrix.yml` 负责；本基线是它的
  **真实补充**——CI 能保证协议函数正确，本基线能保证「真实 agent 接进来真的能用」。

两条路径，都在 harness 的 Tier C `agent` 命令里：

| 路径 | 命令 | 上游 | 依赖 | 何时用 |
|---|---|---|---|---|
| **mock 冒烟** | `--mock` | 进程内 vmodel 虚拟上游 | 仅本地 CLI | 每次都先跑，零配置 |
| **真实扫描** | `--real providers.yaml` | 真实 provider | 本地 CLI + 凭证 | 接入真实 provider 后 |

---

## 1. 零配置冒烟（先跑这个）

```bash
./scripts/harness-baseline.sh
```

它会：build harness → 预检三个 CLI 在 PATH → 跑 `agent batch --mock`（claude/codex/opencode
各一次，vmodel 虚拟上游）→ 打印汇总表 + 写产物。

**预期耗时**：分钟级（opencode 因 bun 启动较慢，单条 ~15–20s）。

**绿了意味着**：数据面在 vmodel 虚拟上游下，三个真实 agent CLI 都能完整跑通
（协议转换 + 规则匹配 + dispatch + 响应回写），且输出含真实回答内容（不只 exit 0）。

## 2. 接入真实 provider 扫描

先**生成配置模板**（从内嵌 provider 模板，OAuth-only 自动跳过）：

```bash
go build -o harness ./cli/harness
./harness init-config --output providers.yaml
```

编辑 `providers.yaml`，**填 apikey、配置 models**：

```yaml
providers:
  - name: "anthropic"
    baseurl: "https://api.anthropic.com"
    apikey: "sk-ant-..."          # 填这里
    api_style: "anthropic"
    models:
      - "claude-3-5-sonnet-20241022"   # 想测的模型
  - name: "openai"
    baseurl: "https://api.openai.com"
    apikey: "sk-..."
    api_style: "openai"
    models:
      - "gpt-4o"
```

> `apikey` 空 / 占位符 / 未展开的 `${VAR}` 的条目会被自动 skip 并打印原因，不会失败。

**用环境变量共享 key / 注入值**：`apikey` 和 `baseurl` 都支持 `${VAR}` 和 `$VAR` 引用
（在加载时从进程环境解析），所以多个 provider 可以共用一个 key，或把敏感值挪出文件：

```yaml
providers:
  - name: "anthropic"
    baseurl: "${TB_ANTHROPIC_BASE}/v1/"   # 引用 + 字面后缀也行
    apikey: "${ANTHROPIC_API_KEY}"        # 多个 provider 可引用同一个 env
    api_style: "anthropic"
    models: ["claude-3-5-sonnet-20241022"]
  - name: "anthropic-gateway"
    baseurl: "${TB_ANTHROPIC_BASE}/v1/"
    apikey: "${ANTHROPIC_API_KEY}"        # 共享同一个 env
    api_style: "anthropic"
    models: ["claude-3-5-haiku-20241022"]
```

> 引用的 env 未设置时，值保持字面 `${VAR}` 不变 → apikey 会被当缺失自动 skip（不会把
> 字面 token 发给上游）。baseurl 没设则只是地址错、上游连不上，不会误用凭证。

然后跑（mock 冒烟 + 真实扫描）：

```bash
./scripts/harness-baseline.sh --real providers.yaml
```

每个 `provider×model` 会起一个隔离 gateway、repoint built-in rule 到该 provider、
跑真实 agent CLI。真实扫描的失败**不阻塞** mock 的结论，但会反映在退出码与 summary 里。

## 3. 读结果

```
.tmp/harness/<run-id>/
  harness-summary.csv      # 每条结果一行（即时落盘，Ctrl-C 可续）
  harness-output/<id>.md   # 每条完整 prompt + output
.tmp/harness/latest -> <最新 run-id>
```

- **stdout 表格**：一眼看 PASS/FAIL/TIMEOUT + 耗时 + API style。
- **CSV**：`status` 列（PASS/FAIL/TIMEOUT）、`exit_code`、`error`、`output_id`（关联明细 md）。
- **明细 md**：某个条目失败时，看它的 output 文件定位根因。

## 4. 重跑红的

```bash
# 只重跑上次 summary 里 FAIL/TIMEOUT 的条目（按 (agent,entry) 最新行判定红绿）
./scripts/harness-baseline.sh --real providers.yaml --only-failing

# 续跑：跳过所有已记录的 (agent,entry)，只补跑没跑过的
./scripts/harness-baseline.sh --real providers.yaml --resume

# 透传任意 harness 参数（如放宽超时）
./scripts/harness-baseline.sh --real providers.yaml --extra "--timeout 5m"
```

`--only-failing` 与 `--resume` 互斥。CSV 是 **append-only 事实记录**：重跑只追加新行，
读时看最新一行；不会删旧行。

---

## 5. 真实 provider 的「通过」是怎么判定的

> 这是本基线相对原 harness 加的关键一环，理解它有助于判断结果。

真实 provider 的成功判定 = **exit 0 且 内容断言通过**：

1. **exit 0** —— CLI 自身没崩。
2. **内容断言**（`assertRealAgentContent`）—— 输出去 ANSI/空白后非空，且不含错误特征
   （`traceback` / `unauthorized` / `invalid_api_key` / `status: 4` / `status: 5` /
   `error: ` / `rate limit` / `insufficient_quota` …，见 `cli/harness/agent_assert.go`
   的 `realAgentErrorMarkers`，大小写不敏感子串匹配）。

为什么要加第 2 条：真实上游可能返回 5xx 但 agent CLI 优雅退出 0，旧逻辑会记成 PASS
（**假绿**）。内容断言抓住这种情况。设计上**宁可误报也不漏报**。

## 6. 内容断言误报怎么办

真实模型偶尔会在合法回答里出现 marker（比如模型自述 "error: ..." 的示例文本）。

处理流程：
1. 打开该条目的 `harness-output/<id>.md`，确认输出**确实是**模型合法回答。
2. 若是误报：临时用 `--extra "--prompt '...'"` 换个 prompt 复测确认；或人工放过该条。
3. 若该 marker 在你的 provider 上长期误报，从 `realAgentErrorMarkers`（单一常量数组）
   里移除它，并在本 runbook 记一笔。
4. **不要**为了让某条变绿而禁用整个内容断言——那是回到假绿。

---

## 7. 常见失败排查

| 现象 | 原因 / 处理 |
|---|---|
| 脚本 exit 2「缺失 agent CLI」 | 安装对应 CLI 并加入 PATH，重跑 |
| `apikey` 相关条目被 skip | `providers.yaml` 里该 provider 没填 key（自动跳过，正常）|
| 上游 401/403，terminal fail | key 错 / baseurl 错；检查 `providers.yaml` 的 apikey、baseurl |
| TIMEOUT | 默认每条 2m；用 `--extra "--timeout 5m"` 放宽，或换更快模型 |
| `codex` mock 报 404 `.../chat/completions` | 历史：agent 路径的虚拟 provider 漏了 `/v1` 前缀（gateway 把 responses 转成 chat/completions 后拼到 `virtualURL` 而非 `virtualURL/v1`，VirtualServer 404）。**已修复**（`SetupAgent` 现对 OpenAI 风格加 `/v1`，与 matrix 路径对齐）。若再出现，看 `internal/protocoltest/agent_env.go` 的 `SetupAgent`。 |
| tool_use 相关跳过/失败 | 见 [`cli/harness/PLANNING.md`](../cli/harness/PLANNING.md) §1 的已知缺陷登记表 |

## 8. 退出码 & CI 边界

| 退出码 | 含义 |
|---|---|
| `0` | 全绿（mock + 可选 real 都过）|
| `1` | 有 FAIL/TIMEOUT |
| `2` | 环境错误（CLI 缺失 / build 失败 / config 文件不存在）|

**本基线不进 CI**（依赖本地凭证 + 本地 CLI）。CI 侧 hermetic 覆盖：
`.github/workflows/harness-matrix.yml`（matrix / replay virtual+vmodel / lb / duo / routing）。
真实 provider 扫描按 [`PLANNING.md`](../cli/harness/PLANNING.md) §4 的约定，
留作 manual / nightly。

---

## 附：一键速查

```bash
# 日常：阶段验收，零配置
./scripts/harness-baseline.sh

# 接入真实 provider
./harness init-config --output providers.yaml   # 填 key、配 models
./scripts/harness-baseline.sh --real providers.yaml

# 重跑红的
./scripts/harness-baseline.sh --real providers.yaml --only-failing

# 看最新结果
cat .tmp/harness/latest/harness-summary.csv
```
