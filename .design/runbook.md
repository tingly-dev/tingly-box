# Runbook — tingly-box 阶段验收基线

> 一个开发阶段结束后、发 PR 前的**真实补充测试**操作手册。
> 入口：`scripts/harness-baseline.sh`；全量命令参考 [`cli/harness/README.md`](../cli/harness/README.md)。

## 这套测试是什么 / 不是什么

- **是**：用**真实 agent CLI**（claude / codex / opencode）打**真实 tingly-box gateway**，跑一遍
  数据面（协议转换 + 路由 + dispatch + 真实上游）。
- **不是 CI**：依赖开发者本机的 agent CLI，以及（真实模式下）provider 凭证。CI 侧的 hermetic
  覆盖在 `.github/workflows/harness-matrix.yml`；本基线是它的**真实补充**——CI 保证协议函数对，
  本基线保证「真实 agent 接进来真的能用」。

两条路径（都在 harness 的 Tier C `agent` 命令里）：

| 路径 | 命令 | 上游 | 依赖 | 何时用 |
|---|---|---|---|---|
| **mock 冒烟** | `--mock` | 进程内 vmodel 虚拟上游 | 仅本地 CLI | 每次先跑，零配置 |
| **真实扫描** | `--config providers.yaml` | 真实 provider | 本地 CLI + 凭证 | 接入真实 provider 后 |

---

## 1. 零配置冒烟（每次先跑）

```bash
./scripts/harness-baseline.sh
```

build harness → 预检三个 CLI 在 PATH → 跑 `agent batch --mock`（claude/codex/opencode 各一次，vmodel
虚拟上游）→ 打印汇总表 + 写产物。分钟级（opencode 因 bun 启动较慢，单条 ~15–20s）。

绿了 = 数据面在 vmodel 虚拟上游下三个真实 agent CLI 都能跑通（协议转换 + 规则匹配 + dispatch +
响应回写），且输出含真实回答内容（不只 exit 0）。

## 2. 配置 `providers.yaml`（真实测试用）

真实扫描需要一个 `providers.yaml` 描述要测哪些 provider×model。先**生成模板**（从内嵌 provider
模板，OAuth-only 自动跳过）：

```bash
go build -o harness ./cli/harness
./harness init-config --output providers.yaml
```

然后编辑。一个完整例子（字段含义见下表）：

```yaml
env:                                       # 可选：共享变量表（见 §2.2）
  ANTHROPIC_API_KEY: "sk-ant-..."
  PROXY_BASE: "https://gateway.internal"

prompt: "用一句话解释递归"                  # 可选：顶层 prompt，锁定所有条目（见 §2.3）

providers:
  - name: "anthropic"
    baseurl: "${PROXY_BASE}/v1/anthropic/" # 支持 ${VAR} 引用
    apikey: "${ANTHROPIC_API_KEY}"         # 多 provider 共用一个 key
    api_style: "anthropic"                 # 必填：anthropic | openai | google
    models:                                # 要测的模型列表
      - "claude-3-5-sonnet-20241022"
    prompt: "测中文：解释递归"              # 可选：provider 级 prompt（顶层未设时生效）
  - name: "openai"
    baseurl: "https://api.openai.com"
    apikey: "sk-..."
    api_style: "openai"
    models: ["gpt-4o"]
    # 不填 prompt → 用 agent 默认
```

### 2.1 字段速查

| 字段 | 层级 | 必填 | 说明 |
|---|---|---|---|
| `name` | provider | 是 | provider 标识 |
| `baseurl` | provider | 是 | 上游地址，支持 `${VAR}` |
| `apikey` | provider | 是 | 凭证；空 / 占位符 / 未展开 `${VAR}` → 该条目自动 skip |
| `api_style` | provider | 是 | `anthropic` / `openai` / `google` |
| `models` | provider | 是 | 要测的模型列表；每个 `provider×model` 展开成一条独立测试 |
| `prompt` | provider | 否 | 该 provider 的测试 prompt（见 §2.3） |
| `enable` | provider | 否 | `false` 跳过该 provider；不填或 `true` 启用（默认启用）|
| `api_type` | provider | 否 | 显式指定 `openai_chat` / `openai_responses` / `anthropic_v1` / `anthropic_beta` / `google`，不填按 `api_style` 推断 |
| `env` | 顶层 | 否 | 共享变量表（见 §2.2） |
| `prompt` | 顶层 | 否 | 锁定所有条目的 prompt（见 §2.3） |

### 2.2 凭证与变量：`${VAR}` 引用（`apikey` / `baseurl` / `prompt` 都支持）

`apikey`、`baseurl`、`prompt` 里的 `${VAR}` / `$VAR` 会在加载时展开。解析优先级：

> **yaml `env:` 表 → 进程环境变量 → 未设置（留字面 `${VAR}`）**

**方式 A（推荐）：yaml 内联 `env:` 段** —— 自包含、多个 provider 共用、值本身也可引用：

```yaml
env:
  ANTHROPIC_API_KEY: "sk-ant-..."             # 直接写值
  OPENAI_API_KEY: "${MY_SECRET_OPENAI}"        # 值再从进程环境解析
  PROXY_BASE: "https://gateway.internal"
providers:
  - name: "anthropic"
    baseurl: "${PROXY_BASE}/v1/"               # 引用 + 字面后缀也行
    apikey: "${ANTHROPIC_API_KEY}"
    ...
```

- `env:` 表优先于进程环境（同名以表为准）；`env:` 值可互相引用；自引用（`FOO: "${FOO}"`）留字面不循环。
- 安全：`env:` 里**明文 key 不要提交 git**；只放 `${...}` 引用 + 真实 key 放进程环境 / gitignore
  的独立文件，则 `providers.yaml` 可安全提交。

**方式 B：进程环境变量** —— 不想在 yaml 留任何值时全从 shell 注入：

```bash
export ANTHROPIC_API_KEY="sk-ant-..."          # 或写进 ~/.zshrc 持久化
# 或单次前缀（不污染当前 shell）：
ANTHROPIC_API_KEY="sk-ant-..." ./scripts/harness-baseline.sh --real providers.yaml
```

变量名规则 `[A-Za-z_][A-Za-z0-9_]*`，叫什么由你定（只要 export 名和 yaml 里对应）。`${VAR}` 和
`$VAR` 等价。跑前 `echo $VAR` 确认有值。

**变量未设置时**：值保持字面 `${VAR}` → apikey 被当缺失、条目自动 skip 并提示（不会把字面 token
发给上游）；baseurl 没设只是地址错、上游连不上，不会误用凭证。

### 2.3 测试 prompt

可在 yaml 配置自定义测试 prompt（也支持 `${VAR}`）。两层，优先级：

> **CLI `--prompt` / 位置参数 > 顶层 `prompt` > provider 级 `prompt` > agent 默认**

- **顶层 `prompt`**（`providers` 同级）—— **锁定**：一旦设置，所有 provider×model 都用它，
  provider 级 `prompt` 被忽略（只有 CLI `--prompt` 能覆盖）。适合「用同一个 prompt 跑全部」。
- **provider 级 `prompt`**（每个 provider 内）—— 仅当**没有**顶层 `prompt` 时生效，按 provider 区分。
- 都不填 → 用 agent 默认（claude `"What is the capital of France?"`、codex `"What is 2+2?"`、
  opencode `"Hello, world!"`）。

---

## 3. 跑测试

**推荐：基线脚本**（自动 build + mock 冒烟 + 真实扫描）：

```bash
./scripts/harness-baseline.sh --real providers.yaml
```

先跑 mock 冒烟（零配置），再对每个 `provider×model` 起隔离 gateway、绑定真实 provider、跑真实
agent CLI。真实扫描的失败**不阻塞** mock 结论，但反映在退出码与 summary 里。

**直接用 harness**（跳过 mock / 精细控制）：

```bash
./harness agent batch  --config providers.yaml                  # 全部 agent × 全部 provider×model
./harness agent claude --config providers.yaml                   # 单 agent
./harness agent claude --config providers.yaml "你的 prompt"     # + CLI prompt（覆盖 yaml）
./harness agent claude --config providers.yaml --filter anthropic  # 指定 agent 只跑指定 provider（按 name，含其全部 model）
./harness agent batch  --config providers.yaml --filter anthropic,openai  # 全部 agent 只跑指定 provider
./harness agent batch  --config providers.yaml --timeout 5m      # 放宽每条超时（默认 2m）
```

## 4. 读结果

```
.tmp/harness/<run-id>/{harness-summary.csv, harness-output/<id>.md}
.tmp/harness/latest -> <最新 run-id>
```

- **stdout 表格**：一眼看 PASS/FAIL/TIMEOUT + 耗时 + API style。
- **CSV**（`--summary <path>` 改路径）：`status` / `exit_code` / `error` / `output_id`（关联明细）。
- **明细 md**（`--output-dir <dir>` 改目录）：某条失败时看它的完整 prompt + output 定位根因。

## 5. 重跑红的 / 续跑

```bash
./scripts/harness-baseline.sh --real providers.yaml --only-failing    # 只重跑 FAIL/TIMEOUT 的条目
./scripts/harness-baseline.sh --real providers.yaml --resume          # 跳过已记录的，补跑没跑过的
./harness agent batch --config providers.yaml --only-failing          # 直接 harness 形式
./harness agent batch --config providers.yaml --resume ""
```

`--only-failing` 与 `--resume` 互斥。CSV 是 **append-only**：重跑只追加新行，看结果取最新一行。

---

## 6. 真实 provider 的「通过」是怎么判定的

真实 provider 成功 = **exit 0 且 内容断言通过**：

1. **exit 0** —— CLI 没崩。
2. **内容断言**（`assertRealAgentContent`）—— 输出去 ANSI/空白后非空，且不含错误特征
   （`traceback` / `unauthorized` / `invalid_api_key` / `status: 4` / `status: 5` / `error: ` /
   `rate limit` / `insufficient_quota` …，见 `cli/harness/agent_assert.go` 的
   `realAgentErrorMarkers`，大小写不敏感子串匹配）。

为什么要加第 2 条：真实上游可能返回 5xx 但 agent CLI 优雅退出 0，旧逻辑记成 PASS（**假绿**）。
内容断言抓住这种情况，设计上**宁可误报也不漏报**。

### 内容断言误报怎么办

真实模型偶尔会在合法回答里出现 marker（如模型自述 "error: ..." 的示例）。处理：

1. 看该条 `harness-output/<id>.md`，确认输出**确实是**合法回答。
2. 误报：换 prompt（CLI `--prompt` 或改 yaml prompt）复测确认，或人工放过该条。
3. 该 marker 在你的 provider 上长期误报 → 从 `realAgentErrorMarkers`（单一常量数组）移除，并在此记一笔。
4. **不要**为让某条变绿而禁用整个内容断言——那是回到假绿。

---

## 7. 常见失败排查

| 现象 | 原因 / 处理 |
|---|---|
| 脚本 exit 2「缺失 agent CLI」 | 安装对应 CLI 并加入 PATH，重跑 |
| 条目被 skip「missing apikey」 | 该 provider 没填 key，或 `${VAR}` 引用的 env 未设置（自动跳过，正常）|
| 上游 401/403，terminal fail | key 错 / baseurl 错；检查 `providers.yaml` |
| TIMEOUT | 默认每条 2m；`--timeout 5m` 放宽，或换更快模型 |
| `codex` mock 报 404 `.../chat/completions` | **已修复**（`SetupAgent` 现对 OpenAI 风格加 `/v1`，与 matrix 路径对齐）。若复现看 `internal/protocoltest/agent_env.go` |
| tool_use 相关跳过/失败 | 见 [`cli/harness/PLANNING.md`](../cli/harness/PLANNING.md) §1 已知缺陷登记表 |

## 8. 退出码 & CI 边界

| 退出码 | 含义 |
|---|---|
| `0` | 全绿（mock + 可选 real 都过）|
| `1` | 有 FAIL/TIMEOUT |
| `2` | 环境错误（CLI 缺失 / build 失败 / config 不存在）|

本基线**不进 CI**（依赖本地凭证 + CLI）。CI hermetic 覆盖见
`.github/workflows/harness-matrix.yml`（matrix / replay virtual+vmodel / lb / duo / routing）；
真实扫描按 [`PLANNING.md`](../cli/harness/PLANNING.md) §4 留作 manual / nightly。

---

## 附：一键速查

```bash
# 零配置冒烟
./scripts/harness-baseline.sh

# 真实测试最短路径
./harness init-config --output providers.yaml      # 生成模板
$EDITOR providers.yaml                              # 填 apikey、留想测的 models（可用 ${VAR} / env: / prompt）
./scripts/harness-baseline.sh --real providers.yaml # 跑（mock + real）

# 精细控制（直接 harness，跳过 mock）
./harness agent batch --config providers.yaml --filter anthropic,openai --timeout 5m
./harness agent claude --config providers.yaml "自定义 prompt"

# 重跑红的 / 续跑
./scripts/harness-baseline.sh --real providers.yaml --only-failing

# 看结果
cat .tmp/harness/latest/harness-summary.csv
```

> harness 的 `providers.yaml`（测试用 provider 列表）和 tingly-box 服务自身的运行配置（如
> `tb.yaml`）是两个不同的东西——前者只驱动 harness 真实扫描，不影响服务运行。
