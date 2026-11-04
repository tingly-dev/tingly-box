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

点击（或在行上按 Enter）图片或文本文件，打开一个模态预览（`components/project/LibraryPreview.tsx`）；其他类型仍只有下载。`LazyTree` 因此给文件行加了点击 → `onActivate`（此前只有键盘 Enter 触发）；行内的下载/删除按钮不会触发预览。

1. **不新增后端接口。** 预览走现有的 `GET /library/file`，经 `apiFetch`（宿主的认证照常生效），取回 blob 后展示；该接口的 `Content-Disposition: attachment` 对 `fetch` 没有影响，所以不需要 inline 模式。路径检查因此也是同一套（`LibraryPath`）。
2. **图片**：png / jpg / jpeg / gif / webp / svg，用 `<img src=blob:>` 展示，blob 的类型由扩展名决定（`image/svg+xml` 等）。SVG 作为图片加载，脚本不会执行，也永远不会作为标记内联进页面。关闭后释放 object URL。
3. **文本**：md / txt / json / csv / yaml / 常见源码扩展名，以纯文本（`<pre>`，保留换行）展示，**不渲染 Markdown**——渲染会加载文档里的远程图片、链接，对“看一眼文件内容”来说得不偿失。
4. **大小上限**：图片 25 MB，文本 512 KB（按树里的 `size` 判断，超过时不发请求），改为提示“文件太大”并给出下载按钮。加载中显示转圈，失败显示服务器返回的错误。
5. **键盘与焦点**：MUI Dialog——Esc 关闭、焦点进入对话框并被限制在其中、关闭后回到触发它的行；头部有下载和关闭按钮。

## 没做

- 预览 PDF、视频及其他类型；Markdown 渲染预览；图片缩放/翻页。
- 在某个文件夹里上传（上传仍进资料库根）。
- 删除文件夹（后端 `DELETE` 只删文件，文件夹行不提供删除）。
