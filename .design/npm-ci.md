# npm 包与 CI 发布：新增包的流程

`npm.yml` 通过 Trusted Publishing（OIDC）发布，没有 token（原理与背景见
[npm.md](./npm.md) 的 G 节）。本文只记录一件事：**往发布清单里加一个全新的 npm
包时，怎么让 CI 能接手发布**，以及每一步为什么是这个顺序。桌面端打包本身见
[gui-packaging.md](./gui-packaging.md)。

## 为什么新包要先手工占位

Trusted Publisher 只能配置在**已经存在**的包上，而 CI 又只能用 OIDC 发布。所以
一个全新的包名有一个先有鸡还是先有蛋的问题，解法是：

1. 维护者在本机手工发一个**空的占位版本**，只为让包名存在；
2. 在这个包上配置 Trusted Publisher；
3. 之后每个真实版本都由 CI 发布，占位版本不再被碰。

不用 `publish-platform-packages-manual.sh` 去发真实版本：那需要先有 release zip，
而占位的目的就是不依赖 release。该脚本保留给“需要手工补发真实版本”的场景
（`--gui` 对应桌面端的包）。

## 当前的包与 CI 的假设

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

## 步骤

先要求：账号在 `@tingly-dev` 组织里有发布权限，且开启了 2FA（passkey 或 TOTP）。

### 1. 占位

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

### 2. 配置 Trusted Publisher

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

### 3. 收紧发布权限

与其他包保持一致：禁止 token 发布。

```bash
npm access set mfa=publish "@tingly-dev/tingly-box-gui-linux-x64"
npm access set mfa=publish "@tingly-dev/tingly-box-gui-linux-arm64"
```

（`mfa=publish` 要求发布时 2FA 并禁用 token 路径之外的免 2FA 发布；该子命令的语义
以 `npm access --help` 为准，npm 12 的文档里只列出了语法。也可以在 npmjs.com 的包
Settings → Publishing access 里选 “Require two-factor authentication and disallow
tokens”。）

### 4. 标记占位版本（可选）

```bash
npm deprecate "@tingly-dev/tingly-box-gui-linux-x64@0.0.0" "placeholder"
npm deprecate "@tingly-dev/tingly-box-gui-linux-arm64@0.0.0" "placeholder"
```

## 之后的发布顺序

1. 跑 `release-gui.yml`，让 release 上有 `tingly-box-gui-linux-{amd64,arm64}.zip`；
2. `npm.yml` 以 `publish_gui=true` 触发（`release-gui.yml` 会自动 dispatch），在
   `production` 环境审批后：先并发发布各平台包，再把它们按精确版本写进 shim 的
   `optionalDependencies` 并发布 shim。

## 排错

| 现象 | 原因 |
|---|---|
| OIDC exchange 步骤失败 | 包上没配 Trusted Publisher，或字段与 workflow / 环境不一致 |
| `npm publish` 报 “package not found” | Trusted Publisher 没勾 `npm publish`（只允许了 stage publish） |
| CI 告警 “does not exist on npm yet; skipping it” | 该包没占位；平台回退到 release 下载 |
| `npm trust` 提示 unknown command | npm 版本低于 11.15 |
| 创建配置报已存在 | 一个包只允许一条配置，先 revoke 再建 |
