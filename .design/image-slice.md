# Image Slice(生图 Playground 的切分下载)

> 适用对象:tingly-box 前端贡献者。
> 描述 Image Playground 的后置切分能力:把一张网格图切成单张 PNG 并打包下载。
> 关联文档:`ux-principles.md`(判断标准)、`imageedit.md`(imagegen/edit 网关链路)。

---

## 1. 背景

ChatGPT 的"贴纸"功能对用户呈现为一个动作:描述一次,拿到一组可直接使用的
贴纸。它底层是**一张几乘几的大图**,再由客户端裁成若干张打包给用户。

我们的 Playground 已经有 generate/edit 双模式,但用户拿到的永远是**整张图**:
想要单个贴纸,必须自己下载、开图像编辑器、手工裁九次。这一步二次处理正是这个
功能要消灭的东西。

---

## 2. 核心决策:只做后置,不碰 prompt

最初的设计里,生成侧有一个"贴纸表"开关:它替用户拼一段网格 scaffold、锁定
方形尺寸、请求透明背景,并在生成前让用户选行列。这一版**被整个砍掉了**,理由:

- **prompt 是用户的东西,产品不该插手。** 一旦我们开始替他改写 prompt,就要
  负责它写得好不好、要不要可编辑、模型认不认——而这些都不是用户找我们要的
  东西。他要的只是"图已经出来了,帮我切开"。
- **生成前拨的旋钮是最差的旋钮。** 行列选择出现在用户还什么都没看到的时候,
  而它真正该被决定的时刻是看着图的时候(原则 2:能直接进入工作面就不要先
  选模式)。何况切分对话框里本来就有同一组行列,前置那份纯属重复。
- **切分对任何图都成立。** 贴纸表、九宫格、精灵图、从别处粘进来的图——只要
  是等分网格就能切。把它绑到"我们自己生成的贴纸表"上反而缩小了适用面
  (原则 4:正交维度分轴)。

结论:**生成侧零改动**(面板、请求参数、run 结构全部保持原样),
能力完全落在拿到图之后。

---

## 3. 切分对话框(`ImageSliceDialog`)

入口在 lightbox 工具栏(`Split into tiles`),对**任何**输出图都可用。
默认 3×3——贴纸表最常见的形状,不对就一次点击改掉。

### 3.1 为什么是等分,而不是自动检测内容边界

自动按连通域/投影检测每个贴纸的 bbox 听起来更聪明,但它把"切得准不准"变成
一个不可解释的黑箱:检测失败时用户既不知道为什么,也没有可调的旋钮。当前版本
**只做等分网格**,并把误差交给两个用户看得懂的参数:

- `Outer margin`:按**短边**比例从四周裁掉的外边距(短边基准保证非方图上的
  裁剪各向同性);
- `Gap between tiles`:从每个格子对称扣掉的间隙(半格/边),保证所有切片同尺寸。

覆盖层实时画出每一刀落在哪里,点击某格可以把它排除。**先给可见、可调、可解释
的等分**;内容边界检测留作后续迭代,届时它的角色也应是"默认值的来源",而不是
取代这两个旋钮。

预览区在图片背后铺一层棋盘格底纹,回答"这张图是透明的还是自带实色背景"——
不透明的图会把它完整盖住,这就是信号。它必须严格贴合图片自身的盒子:挂在更大的
预览容器上会在四周露出一圈,看着像模型自己画了个网格框。棋盘格只在预览里,
切片走 `drawImage(源图 → 全新 canvas)`,CSS 背景到不了导出的像素。

`Output size` 一栏顺带展示切下来的**具体像素**(`Original · 341×341 px`),
因为这是用户唯一看不见的后果:1024 的表切 2×2 是 512px、3×3 是 341px、
4×4 只剩 256px(原则 5:展示具体值)。

### 3.2 tainted canvas

`loadImage()` 一律走 `fetch(src) → blob → objectURL`,而不是把 provider 的
URL 直接塞进 `<img>`:跨域图片画进 canvas 会污染它,`toBlob()` 随即抛错——
而 `toBlob()` 正是本模块存在的理由。gpt-image 返回 `b64_json`(data URL)不受
影响;dall-e-3 返回远程 URL 时若对方不给 CORS,对话框显示明确的不可切片提示,
而不是静默失败。

### 3.3 零依赖 ZIP

`utils/zip.ts` 是一个 store 方法(不压缩)的 ZIP writer。PNG 本身已经压过,
deflate 收益接近 0,不值得为此引入压缩依赖。实现覆盖 APPNOTE 的 stored 子集:
local file header / central directory / EOCD,文件名带 UTF-8 flag(bit 11),
以便中文命名在各解压工具里正常显示。

---

## 4. 顺带补齐:单张下载

在此之前 Playground **完全没有下载入口**(只有 copy prompt / use as reference)。
切片能打包下载而整图不能,是割裂的。lightbox 因此同时补上 `Download`,
文件名按 blob 的实际 MIME 取扩展名,而不是一律 `.png`。

---

## 5. 代码位置

| 文件 | 职责 |
|------|------|
| `frontend/src/utils/zip.ts` | store-only ZIP writer + CRC-32 |
| `frontend/src/utils/download.ts` | 存盘(anchor + 延迟 revoke)、文件名 slug、MIME→扩展名、`fetchBlob` |
| `frontend/src/utils/imageSlice.ts` | 等分网格几何、图片加载、切片渲染 |
| `frontend/src/pages/scenario/components/ImageSliceDialog.tsx` | 切分工作面 |
| `frontend/src/pages/scenario/components/ImageGenPlaygroundCard.tsx` | lightbox 的下载 / 切分入口(仅此,生成侧未改) |
| `frontend/src/mocks/handlers.ts` | mock 侧识别 prompt 里的 `NxM grid`,返回真实网格图 |

`utils/download.ts` 是独立模块而不是 slicing 的一部分:存盘和 slicing 无关,
且这段 anchor 舞蹈在仓库里本已被手写过两遍(`rule-card/utils.ts` 的
`downloadFile`、`GuardrailsPage` 内联),两处都缺少"同步 revoke 会取消下载"
这个修正。本次一并收敛到该模块,旧的两份改为调用它。

`DEFAULT_GRID` / `MARGIN_MAX` / `GUTTER_MAX` 由 `imageSlice.ts` 导出,
滑块上界与 `computeTileRects` 的 clamp 用的是同一组常量——UI 不可能给出一个
几何层会悄悄拒绝的值。

几何与打包逻辑是纯函数,单测在 `zip.test.ts` / `imageSlice.test.ts` /
`download.test.ts`;
canvas 渲染部分不进 jsdom 单测,靠 `.claude/skills/ui-preview` 的真实浏览器
链路验证(生成 → 切分 → 解压出 8 张透明 PNG)。
