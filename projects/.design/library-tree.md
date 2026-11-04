# Library 目录树

Overview 的 Library 页把项目资料库显示成可展开的目录树，而不是平铺的文件列表。

## 决定

1. **逐层懒加载。** 只在打开文件夹时请求它那一层。资料库里有很多线程产出时，不再一次遍历整棵树。
2. **新接口 `GET /projects/:id/library/tree?path=<dir>`**，返回 `{dir, path, entries[]}`，`entries` 是该目录的直接子项（`name`、`path`、`kind: dir|file`、`size`、`updated_at`、`origin`）。文件夹在前，之后文件，各按名字排序。旧的平铺 `GET /library` 仍在（上传接口还用它的返回形状），前端不再调用。
3. **访问检查只收紧不放宽**（`Layout.ListLibraryDir`）：
   - 路径各段不能是 `..`，不能以 `.` 开头；`.tmp/<thread>` 等隐藏目录既不列出，也不能直接请求（400）。
   - 先走 `LibraryPath`（拒绝逃出资料库根），再 `EvalSymlinks` 确认真实路径仍在资料库内。
   - 符号链接不列出。
   - 开头的 `/` 被去掉，仍落在资料库内（`/etc` 等于资料库里的 `etc`，不存在则 404）。
   - 错误：越界 / 隐藏 400，不是目录 400，不存在 404。资料库目录尚不存在时根目录为空列表。
4. **隐藏 `.tmp`：沿用原行为。** 平铺列表本来就跳过点开头的目录，这里保持一致，线程的 TMPDIR 是临时文件，不是交付物。其他点文件同样隐藏。
5. **来源 chip 去掉。** 平铺列表里每行一个“上传/线程产出”标签；树里 `outputs/` 目录本身已说明来源，标签只会增加噪音。`origin` 字段仍由接口返回。

## 前端

- `components/common/LazyTree.tsx`：通用的懒加载树。`load(dir)` 返回一层节点，已展开的路径由父组件持有（受控），所以可以记忆。遵循 WAI-ARIA tree：单个 tab stop（roving tabindex）、上下箭头移动、右箭头展开/进入第一个子项、左箭头收起/回到父项、Home/End、Enter/空格切换文件夹或触发 `onActivate`。
- 展开状态按项目存在 localStorage（`tingly-project.library.expanded.<projectId>`），重新进入时先加载根，再沿已记住的路径逐层加载；已不存在的路径在父目录加载后被清掉。
- 刷新（上传、删除后）重新请求所有已加载的目录，期间保留旧行，不闪烁。
- 行内操作（下载、删除）只对文件显示；鼠标悬停或键盘聚焦时替换右侧的大小和时间。不可见时不接收点击；触屏设备没有 hover，操作一直显示。键盘上只有当前行的操作在 tab 顺序里。
- 与“添加目录”选择器（`DirectoryPicker`）的关系：它是按列钻取的对话框，不是树，且只浏览服务器 home，不能直接复用成树。这里沿用它的图标（Folder / ChevronRight）、加载/错误/重试的交互和 i18n 词汇；`DirectoryPicker` 本身未改动。

## 文件预览

点击（或在行上按 Enter）可预览的文件，打开一个模态预览（`components/project/LibraryPreview.tsx`）；其余类型只有下载。`LazyTree` 因此给文件行加了点击 → `onActivate`（此前只有键盘 Enter 触发）；行内的下载/删除按钮不会触发预览。

### 可预览的类型（白名单，按扩展名，不区分大小写）

| 类别 | 扩展名 | 怎么展示 |
|---|---|---|
| 位图 | png、jpg、jpeg、gif、webp | `<img src=blob:>` |
| 纯文本 | md、markdown、txt、log、json、csv、yaml、yml、toml、ini | `<pre>` 里的 React 文本节点 |

**其余一律只能下载**，包括：

- **SVG（svg、svgz）**：它是文档，可含脚本、`<foreignObject>`、外部引用。放进 `<img>` 时浏览器通常会禁用脚本和外部加载，但这依赖宿主（Web、桌面 webview）各自的实现，无法在每个宿主证明安全，所以不预览。
- **HTML / XML / CSS / JS / Shell 等标记或可执行文本**：虽然放进 `<pre>` 也是安全的，仍不列入，避免白名单随“看起来像文本”扩张。
- PDF、压缩包、其他二进制：照旧下载。

### 为什么这样是安全的

1. **不新增后端接口。** 预览走现有的 `GET /library/file`，经 `apiFetch`（宿主的认证照常生效），取回 blob；`Content-Disposition: attachment` 对 `fetch` 没有影响，不需要 inline 模式。路径检查因此是同一套。
2. **类型由我们决定，不信服务器。** 图片的 blob 类型取自白名单里扩展名对应的 MIME（`image/png` 等），不用响应头的 `Content-Type`。
3. **魔数校验（`sniffImage`）。** 在创建 object URL 之前，检查文件头是否真是扩展名声称的格式（PNG 8 字节签名、JPEG `FF D8 FF`、GIF87a/89a、`RIFF…WEBP`）。名为 `.png` 实则是 HTML/SVG 的文件不会交给浏览器，只显示“无法预览，请下载”。
4. **`<img>` 解码失败时安全降级。** `onError` 时去掉图片并显示同样的提示（例如宿主 CSP 不允许 `blob:` 图片时）。关闭后释放 object URL。
5. **文本只作为文本。** 内容以 React 文本节点放入 `<pre>`：不解析 HTML、不渲染 Markdown、没有链接，远程图片和链接也不会被加载。含 NUL 字节的“文本”按二进制处理，不预览。
6. **大小上限**：图片 25 MB，文本 512 KB（按树里的 `size` 判断，超过时不发请求），改为提示“文件太大”并给出下载按钮。加载中显示转圈，失败显示服务器返回的错误。
7. **键盘与焦点**：MUI Dialog——Esc 关闭、焦点进入对话框并被限制在其中、关闭后回到触发它的行；头部有下载和关闭按钮。

## 文件访问：用标准库的 `os.Root`，只在上面加自己的策略

资料库里的文件会被线程（不可信的 agent）写入，线程可以留下符号链接、FIFO、同名替换。所以“请求路径 → 磁盘”这一步不能靠“先检查字符串、再按路径打开”。

### 评估过的标准件（Go 1.26，gin v1.12）

| 候选 | 结论 |
|---|---|
| `os.Root`（`os.OpenRoot`） | **采用。** 在解析路径的同时拒绝 `..` 和指向根外的符号链接（含绝对链接），没有检查与使用之间的间隙（TOCTOU）；Windows 上还拒绝 `NUL` 等保留名。缺口：允许指向根**内**的符号链接，也不挡隐藏目录——这两点是我们的策略，叠在上面。 |
| `os.OpenInRoot` | 等价于 OpenRoot + Open，但每次调用都重开根；我们还要 Lstat/ReadDir/Remove/MkdirAll，直接持有 `*os.Root`。 |
| `filepath.IsLocal` | **采用，仅作词法预检**（拒绝绝对路径、`..`、空、Windows 保留名与盘符）。只看字符串，不看磁盘，所以不能单独用。`Localize` 是把斜杠路径转成 OS 路径，用不上。 |
| `http.Dir` / `http.FS` + `fs.Sub` / `fs.ValidPath` | 不采用。`http.Dir` 会跟随符号链接且文档明说不防逃逸；`fs.ValidPath` 与 IsLocal 等价但更弱（不管 Windows），而且 `http.FileServer` 会把 `/index.html` 重定向。`os.Root.FS()` 也行，但我们不要目录列表、要自己的状态码。 |
| gin `c.File` / `FileAttachment` / `FileFromFS` / `Static` | 不采用。`File`/`FileAttachment` 内部是 `http.ServeFile(path)`：**按路径重新打开**，会把前面做过的检查作废，并对 `index.html` 重定向；`FileFromFS` 取决于传入的 `http.FileSystem`（`gin.Dir` 同样跟随链接）。gin 没有“按已打开句柄发送”的现成方法，所以用 `http.ServeContent`（支持 Range、条件请求）自己写 Content-Disposition。 |

### 实现（`internal/project/library_root.go`）

- `cleanLibraryRel`：去掉首尾 `/`，拒绝 NUL、`filepath.IsLocal` 不通过、空/`.`/`..`/以 `.` 开头的段。（`%2e%2e` 之类由 HTTP 层解码一次后得到 `..`，同样被拒；解码两次才成立的 `%252e` 只是一个字面文件名，找不到即 404。）
- `noLibrarySymlinks`：路径上**每一级**（含文件本身）用 `root.Lstat` 检查，任何符号链接（包括指回资料库内的）都拒绝，防止线程把 `.tmp/…` 别名到可见位置。
- `OpenLibraryFile` / `RemoveLibraryFile` / `WriteLibraryFile` / `ListLibraryDir` / `OpenAttachment` 都经过上面两步 + `os.Root`：
  - 只服务/删除普通文件（FIFO 在打开前就被拒，否则会一直阻塞）；打开后再 `Lstat` 一次，用 `os.SameFile` 确认打开的仍是该路径指向的文件（被换成链接则拒绝）。
  - 下载按**已打开的句柄**发送（`module.ServeDownload`）：类型取扩展名、`X-Content-Type-Options: nosniff`、`attachment`，`index.html` 之类也只是普通文件。
  - 写入：`uploads/` 被换成指向外部的链接时，上传和线程附件不会写出资料库；已存在的目标是链接则拒绝；隐藏名（`.env`）拒绝并返回 400（它本来就不会出现在目录树里）。
- 响应码：路径无效 → 400，不存在/是文件夹/特殊文件 → 404。`DELETE` 因此不能再删空文件夹（前端本来就没有）。

### 残余风险（已知，接受）

检查之间的窗口里，有资料库写权限的一方可以把某一级换成**指向资料库内部**的链接。`os.Root` 保证这仍出不去资料库，最坏是读到同一资料库里的另一个文件（如别的线程的 `.tmp`）；并且需要已经能写这个资料库。`ReadDir` 的条目信息也都用 `root.Lstat` 取，不走路径。Windows 上 `Root` 另外拒绝保留设备名；Linux 上反斜杠只是普通文件名字符。

### 测试

`library_tree_test.go`：符号链接（绝对/相对/指回库内/目录）、`..` 与编码变体、NUL、绝对路径、Windows 风格路径、大小写、竞态（并发把文件和目录在“普通”与“指向库外的链接”之间来回替换，内容永远不含库外数据；朴素的 `os.ReadFile(路径)` 实现会被这个测试抓到）、写入不穿透链接。`server/library_tree_test.go`：HTTP 层的编码路径、响应头与 Range、`index.html`、非 ASCII 文件名、上传不跟随链接、附件下载拒绝链接。

## 没做

- 预览 SVG、PDF、视频及其他类型；Markdown 渲染预览；图片缩放/翻页。
- 在某个文件夹里上传（上传仍进资料库根）。
- 删除文件夹（后端 `DELETE` 只删文件，文件夹行不提供删除）。
