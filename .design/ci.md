# CI

仓库里**唯一**的 CI 文档：哪些 workflow 在什么时候跑、为什么这样排、改的时候守什么规矩。
单个子系统的细节留在各自的文档里（见末尾“相关文档”）；写 workflow 本身的语法坑见
[`.github/workflows/AGENTS.md`](../.github/workflows/AGENTS.md)，改 `.github/` 前先读它并用
`actionlint` 检查。

## 一句话

**PR 上只跑前端检查；Go 侧编译、测试和 harness 每天在 `main` 上跑一次；发版时 release 再全量把关。**
`main` 没有 required checks（分支保护 404，两个 ruleset 都是 `disabled`），所以所有 CI 都是信号而不是门；
唯一的硬门是 release 里的 harness。

| 层 | 何时 | 跑什么 | Workflow |
|---|---|---|---|
| PR | 每次 push（只在改到前端时） | 前端 build / lint / `@tingly/projects` 检查 | `verify-frontend.yml` |
| 每日 | 03:23（UTC+8），`main` 与上次绿色 run 同一提交则跳过；可手动触发 | cli / gui 编译、projects / remote 测试、IM 集成测试、catalog check、完整 harness | `daily.yml` |
| 发版 | tag `v*` 或手动 | 前端构建 → 完整 harness（门禁）→ CLI 与 GUI 全平台构建、发布、发布后校验 | `release.yml` → `release-cli.yml` / `release-gui.yml` / `verify-release-*.yml` |
| 手动 | `workflow_dispatch` | npm 发布、npm 发布演练、源码镜像、harness 子集 | `npm.yml`、`harness-npm.yml`、`docker-source.yml` |

## Workflow 清单

| 文件 | 触发 | 状态 |
|---|---|---|
| `verify-frontend.yml` | PR：`frontend/**`（不含 md）、`openapi.json`、`setup-frontend`、自身 | 在用 |
| `daily.yml` | 定时 + 手动 | 在用 |
| `release.yml` | tag `v*` + 手动 | 在用，不要改名 |
| `release-cli.yml`、`release-gui.yml`、`build-frontend.yml`、`harness-template.yml` | `workflow_call`（`release-*` 也可手动） | 在用 |
| `verify-release-cli.yml`、`verify-release-gui.yml` | release 之后 / 手动 | 在用 |
| `npm.yml` | 仅手动 | 在用；**文件名和 `production` 环境绑定了 npm Trusted Publishing，不能改名** |
| `harness-npm.yml`、`docker-source.yml` | 仅手动 | 在用 |
| `verify-ci.yml` | PR：`.github/` 下的 workflow / action / script | 在用（actionlint + shellcheck，0.2 分钟） |
| `harness-docker.yml` | PR / push main：`build/docker/**` | 在用（只在 docker 路径变化时，<5 分钟） |
| `homepage.yml` | PR / push main：`homepage/**` | 在用 |
| `issue-area.yml` | issue 打开 | 在用 |
| `verify-imbot-projects.yml`、`harness-pr.yml`、`harness-matrix.yml` | PR / 合并后 / `ci/**` | **应在 Actions 里停用**：内容已由 `daily.yml` 接手。文件暂留，便于恢复；确认稳定后可删 |

`harness-matrix.yml` 与 `harness-pr.yml` 停用后，harness 的三层漏斗（PR → 合并后 → 发版）收缩为
“每日 + 发版”两层：每日跑的就是 `harness-template.yml`（和发版门禁同一份定义），所以覆盖面没有少，
只是发现问题的时间从“合并时”推迟到“当天夜里”。`catalog check` 原来只在 `harness-pr.yml` 里，
现在放在 `daily.yml` 的 `catalog-check` job。

## 为什么这样排

2026-11-05 → 11-19（14 天，711 个 run，约 14 个 PR/天）的度量：

- PR 触发的 CI 约 **54 job-hours**：Verify Compile 40.2（gui 14.7、cli 10.6、frontend 8.0、
  projects 6.2）、IM 集成测试 9.1、Harness PR 4.3；合并后 Harness Matrix 另有 4.0。
- 平均每个分支 push 约 2 次（最多 13 次），约一半的 run 是被后一次 push 取代的重跑；
  `cancel-in-progress` 有效，但每次 push 的最终 run 仍然全跑。
- 没有 runner 排队（job 1–3 秒内启动），仓库公开所以 Linux runner 免费，单次 run 中位数不到 4 分钟——
  **代价是噪音和注意力，不是延迟或钱。**
- Verify Compile 失败率 9.5%，其中前端 25 次（`Projects web UI` 14、`Lint` 10）；另有 13 个 job 死在
  `Checkout code`，是基础设施抖动。前端是 PR 上唯一经常抓到真问题的检查，所以保留。
- `compile-gui` 命中 73/150 个 PR（49%），和 `compile-cli` 编译几乎同一棵 Go 树，还要装 GTK/WebKit。
- draft PR 只占 2/150，draft 过滤几乎没有收益；文档改动早已被 paths 过滤掉。
- 缓存：`main` 上没有任何 cache（Go 的 workflow 从不在 `main` 跑），PR 的首次 run 都是冷的；
  cache 总量曾达 11.2 GB，超过 10 GB 上限。

所以决定：PR 只留前端；其余 Go 侧检查挪到每日 run，发版的全量构建和 harness 门禁保持不动。
没有选的方案：merge queue（要先启用 required checks，还会在合并后那次 run 之外多跑一次）；
按标签 / ready 才跑重检查（没有 required checks，就没人强制合并前跑过）。合并后的 harness 则改成了每日跑。

## 规则

- **新检查放哪**：先问“只在改到它自己那块时才有信号吗？”是 → 做成按路径触发的 PR 检查，并且要便宜
  （经验线：中位数 <3 分钟）；否 / 偏重 / 偏集成 → 放进 `daily.yml`，让发版去兜底。不要再加“每个 PR 都跑一遍
  Go”的 job。
- **路径过滤**：workflow 自身的文件、它用到的 composite action 都要在 `paths` 里，否则改了它却不验证。
  `paths` 里的否定模式（`!…`）要放在它所否定的模式之后。
- **并发**：PR 级 workflow 用 `concurrency: <name>-${{ github.ref }}` 加 `cancel-in-progress: true`；
  `daily.yml` 用固定组且不取消。
- **新增 harness 的 section** 要同时在 `harness-template.yml` 加一条 leg，没有任何检查强制这个对应关系
  （见 [`cli/harness/README.md`](../cli/harness/README.md) 的 CI 一节）。
- **跳过的上游 job**：`needs` 到可能被跳过的 job，下游要写显式状态判断，见 `AGENTS.md`。
- **定时任务**：minute 不要选 0 或 30；定时 run 只在默认分支上执行，GitHub 会在失败时通知定时任务的
  所有者，目前没有别的告警渠道。
- **缓存**：只用 `setup-go` / `setup-frontend` 自带的缓存，不再手写 `actions/cache` 条目。
  PR 引用上写入的 cache 只有该 PR 自己能读；`daily.yml` 在 `main` 上写入的 cache 才能被所有分支读到。

## 待办 / 未决

- 在 Actions 里停用 `verify-imbot-projects`、`harness-pr`、`harness-matrix`，稳定后删除文件，并同步
  更新 `cli/harness/README.md` 的 Triggers 段。
- 每日 run 失败目前只有 GitHub 的默认通知；需要时加 issue / IM 通知。
- `harness-docker.yml` 的 smoke test 还在 PR 上按路径触发；可并入 `daily.yml`。
- `daily.yml` 只能靠合并后手动触发一次来验证；`actionlint` 由 `verify-ci.yml` 在 PR 上跑。
- 度量的复现方法：

```bash
gh api -X GET repos/tingly-dev/tingly-box/actions/runs -f created=">=2026-11-05" \
  -f per_page=100 --paginate \
  --jq '.workflow_runs[]|[.id,.name,.event,.conclusion,.created_at,.run_started_at,.updated_at,.head_branch,.head_sha[0:8]]|@tsv'
gh api repos/tingly-dev/tingly-box/actions/runs/<id>/jobs      # 单个 run 的 job / step 耗时
gh api repos/tingly-dev/tingly-box/actions/cache/usage         # cache 用量
```

## 相关文档

| 文档 | 内容 |
|---|---|
| [`.github/workflows/AGENTS.md`](../.github/workflows/AGENTS.md) | 改 workflow 的规则和 actionlint 用法（面向 agent） |
| [harness-matrix.md](./harness-matrix.md)、[`cli/harness/README.md`](../cli/harness/README.md) | harness 本身：模式、场景、leg 与 CLI 的对应 |
| [harness-npm.md](./harness-npm.md) | `harness-npm.yml` 演练 npm 发布的做法和限制 |
| [npm.md](./npm.md)、[gui-packaging.md](./gui-packaging.md) | 包怎么构建、怎么发布；GUI 打包与发布后校验 |
| [model-data.md](./model-data.md)、[`internal/catalog/AGENTS.md`](../internal/catalog/AGENTS.md) | `catalog check` 的规则 |

## 附：新增 npm 包的发布流程

`npm.yml` 通过 Trusted Publishing（OIDC）发布，没有 token（原理与背景见
[npm.md](./npm.md) 的 G 节）。本节只记录一件事：**往发布清单里加一个全新的 npm
包时，怎么让 CI 能接手发布**，以及每一步为什么是这个顺序。桌面端打包本身见
[gui-packaging.md](./gui-packaging.md)。

### 为什么新包要先手工占位

Trusted Publisher 只能配置在**已经存在**的包上，而 CI 又只能用 OIDC 发布。所以
一个全新的包名有一个先有鸡还是先有蛋的问题，解法是：

1. 维护者在本机手工发一个**空的占位版本**，只为让包名存在；
2. 在这个包上配置 Trusted Publisher；
3. 之后每个真实版本都由 CI 发布，占位版本不再被碰。

不用 `publish-platform-packages-manual.sh` 去发真实版本：那需要先有 release zip，
而占位的目的就是不依赖 release。该脚本保留给“需要手工补发真实版本”的场景
（`--gui` 对应桌面端的包）。

### 当前的包与 CI 的假设

| 包 | 内容 | 谁发布 |
|---|---|---|
| `tingly-box` | CLI shim | `npm.yml` `publish-cli` |
| `@tingly-dev/tingly-box-<os>-<cpu>` | CLI release zip | `npm.yml` `publish-cli` |
| `tingly-box-gui` | GUI shim | `npm.yml` `publish-gui` |
| `@tingly-dev/tingly-box-gui-<os>-<cpu>` | GUI release zip | `npm.yml` `publish-gui` |

名单的唯一来源是 `build/npx/shared/platform.js`（`PLATFORM_PACKAGES`、
`GUI_PLATFORM_PACKAGES`）。加一个平台只需要改这个文件并按下面的步骤占位，workflow
自己遍历名单。

`publish-gui` 对“还不存在于 npm 的 GUI 平台包”的处理是**跳过并告警**，不发布、
也不写进 shim 的 `optionalDependencies`，该平台回退到 GitHub release 下载。所以漏掉
占位不会让发布失败，只会让那个平台用不上 npm 渠道。已存在但没配 Trusted Publisher
的包则不同：`Verify npm OIDC trusted-publisher exchange` 步骤会在任何发布之前失败。

### 步骤

先要求：账号在 `@tingly-dev` 组织里有发布权限，且开启了 2FA（passkey 或 TOTP）。

#### 1. 占位

```bash
npm login
for cpu in x64 arm64; do
  name="@tingly-dev/tingly-box-gui-linux-$cpu"
  dir="$(mktemp -d)"
  cat > "$dir/package.json" <<JSON
{
  "name": "$name",
  "version": "0.0.0",
  "description": "Placeholder. The real releases are published by CI; not meant to be installed directly.",
  "homepage": "https://github.com/tingly-dev/tingly-box",
  "repository": { "type": "git", "url": "git+https://github.com/tingly-dev/tingly-box.git" },
  "license": "MPL-2.0",
  "author": "Tingly Dev",
  "publishConfig": { "access": "public" },
  "os": ["linux"],
  "cpu": ["$cpu"]
}
JSON
  printf '# %s\n\nPlaceholder; installed by the `tingly-box-gui` package.\n' "$name" > "$dir/README.md"
  (cd "$dir" && npm publish --access public --tag placeholder)
done
```

- 版本 `0.0.0` 低于之后任何真实版本；`--tag placeholder` 避免占用 `latest`。
- 平台包的 `os` / `cpu` 取 Node 的 `process.platform` / `process.arch` 命名，与
  `platform.js` 的键一致（`x64`，不是 `amd64`）。
- 名字必须在 `@tingly-dev` scope 下：未加 scope 的一批相似包名会触发 npm 的垃圾
  包检测（见 npm.md G 节）。

#### 2. 配置 Trusted Publisher

`npm trust` 需要 **npm ≥ 11.15**（`npm install -g npm@^11.15.0`；本仓库其他地方
的 11.5.1 门槛只针对 `npm publish` 的 OIDC 交换），账号开 2FA，且不能用带
“bypass 2FA” 的 granular token。

```bash
for cpu in x64 arm64; do
  npm trust github "@tingly-dev/tingly-box-gui-linux-$cpu" \
    --file npm.yml \
    --repository tingly-dev/tingly-box \
    --environment production \
    --allow-publish \
    --yes
  sleep 2
done
```

- 四个字段都是**精确匹配**：workflow 文件名 `npm.yml`、环境 `production`、仓库
  `tingly-dev/tingly-box`。改文件名或环境名会让发布失败，直到这里同步更新。
- 必须带 `--allow-publish`。只给 `--allow-stage-publish` 时 `npm publish` 会报笼统的
  “package not found”。
- 第一次请求要做一次 2FA；网页上有“接下来 5 分钟免 2FA”的选项，勾上后循环里后面的
  包不再询问。
- 一个包只能有一条配置。已存在时会报错，先 `npm trust list <pkg>` 看 id，再
  `npm trust revoke <pkg> --id <id>`，然后重建。

校验：

```bash
npm trust list "@tingly-dev/tingly-box-gui-linux-x64"
```

#### 3. 收紧发布权限

与其他包保持一致：禁止 token 发布。

```bash
npm access set mfa=publish "@tingly-dev/tingly-box-gui-linux-x64"
npm access set mfa=publish "@tingly-dev/tingly-box-gui-linux-arm64"
```

（`mfa=publish` 要求发布时 2FA 并禁用 token 路径之外的免 2FA 发布；该子命令的语义
以 `npm access --help` 为准，npm 12 的文档里只列出了语法。也可以在 npmjs.com 的包
Settings → Publishing access 里选 “Require two-factor authentication and disallow
tokens”。）

#### 4. 标记占位版本（可选）

```bash
npm deprecate "@tingly-dev/tingly-box-gui-linux-x64@0.0.0" "placeholder"
npm deprecate "@tingly-dev/tingly-box-gui-linux-arm64@0.0.0" "placeholder"
```

### 之后的发布顺序

1. 跑 `release-gui.yml`，让 release 上有 `tingly-box-gui-linux-{amd64,arm64}.zip`；
2. `npm.yml` 以 `publish_gui=true` 触发（`release-gui.yml` 会自动 dispatch），在
   `production` 环境审批后：先并发发布各平台包，再把它们按精确版本写进 shim 的
   `optionalDependencies` 并发布 shim。

### 排错

| 现象 | 原因 |
|---|---|
| OIDC exchange 步骤失败 | 包上没配 Trusted Publisher，或字段与 workflow / 环境不一致 |
| `npm publish` 报 “package not found” | Trusted Publisher 没勾 `npm publish`（只允许了 stage publish） |
| CI 告警 “does not exist on npm yet; skipping it” | 该包没占位；平台回退到 release 下载 |
| `npm trust` 提示 unknown command | npm 版本低于 11.15 |
| 创建配置报已存在 | 一个包只允许一条配置，先 revoke 再建 |
