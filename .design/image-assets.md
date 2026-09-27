# Image Assets(素材:Prompt 片段 + 参考图)

> 适用对象:tingly-box 前后端贡献者。
> Image rail 下的 **Assets / 素材** 子页(`/image/assets`)与其后端
> `internal/imageasset`(`/api/v1/image-assets`)的定位、数据模型、独立性与演进方向。
> 关联文档:`image-layout.md`(Image 入口)、`ux-principles.md`(判断标准)。

---

## 1. 为什么要有这一页

Playground 的会话(`utils/playgroundSession.ts`)记录的是"发生过的一切",会被清空,
也不做挑选。用户真正想留下来反复用的东西是另一类:

| 用户此刻在问 | 留存的物件 |
|---|---|
| "上次那个效果好的 Prompt 呢?" | 完整 Prompt |
| "那几个让它好用的词/句子,能不能拿来拼新的?" | **词条**(term)与**描述语句**(phrase) |
| "角色设定图 / 风格板每次都要重新找" | 参考图 |

所以这里存的是**素材**,不是历史。会话 ≠ 素材(原则 3,一词一义)。

## 2. 命名:Image Assets / 素材

一个词贯穿各层(原则 3):API `/api/v1/image-assets`、Go 包 `internal/imageasset`、
数据目录 `<配置目录>/image-assets/`、页面 `/image/assets`、侧栏 **Assets**(中文"素材")。

- 不用 library / lib:那是"装东西的地方",不是东西本身;`lib` 在本仓库里还指
  `libs/` 下的 SDK 子模块。
- 选 assets:内容本身就是可复用的资产,也贴合将来"Agent 选取资产、拼装 Prompt"。
- 带 `image-` 前缀:与 Web UI 已有的 `/assets` 静态资源路由区分开。

## 3. 关键判断:存的是"片段",不只是整段 Prompt

真正可复用的往往不是一整段 Prompt,而是从里面拆出来的关键词条和描述语句。因此
Prompt 素材统一为一种记录 `PromptPiece`,按**用途**分三种 kind:

| kind | 例子 | 在 Playground 里的动作 |
|---|---|---|
| `prompt` | 整段 Prompt | **替换**输入框 |
| `term` | `rim lighting`、`35mm`、`赛博朋克` | **追加**到 Prompt(`appendPiece`) |
| `phrase` | `a quiet street after rain` | **追加**到 Prompt |

kind 是同一种东西的一个轴,而不是三种东西(原则 4):新建时在表单里切换,事后
也能改。另外两个轴:

- **tags**:自由标签(风格、光线、主体、项目……),小写去重。按标签筛选。
- **source_id**:片段拆自哪一段 Prompt(溯源)。不是外键:删除原 Prompt 不删片段。

## 4. 拆解:现在由人来做,接口为 AI 留好

`SplitDialog` 流程:

1. `suggestPieces(text)` 按标点(中英文逗号、分号、顿号、句末标点、换行、
   列表符号)做**机械初切**,并按长度猜 term / phrase(短句末尾的句号不影响判断,
   存为词条时去掉)。它只是起点,不判断什么重要。
2. 人来审:取消勾选不值得留的、改写措辞让每条能独立使用、切换 term/phrase、
   补一条、统一打标签。已经存过的片段默认不勾选。
3. 一次事务写入(`POST /image-assets/pieces`),每条带 `source_id`。

**演进位点**:`suggestPieces` 的输入(一段文本)与输出
(`PieceCandidate[]`:text + kind)就是 AI 拆解器要替换的接口。换成 AI 后,
审阅这一步保留——AI 给候选,人确认。

## 5. 组装:现在是手工拼装,未来是 Agent 选取

现在的组装:Playground Prompt 输入框上的书签按钮(`PromptMenu`)——
"追加到 Prompt"列出词条/语句,"替换 Prompt 为"列出整段 Prompt。

更远的方向:Agent 根据需求,按 kind + tag 选取片段,拼装出符合要求的 Prompt。
为此:片段是原子的、有类型的、可检索的、可溯源的;并且存在**后端**,Agent 与其他
客户端都能通过同一个 API 读到(见 §6)。

## 6. 后端:`internal/imageasset`

### 独立性

目标是将来可以拆成独立服务,所以:

- **自己的存储**:`<配置目录>/image-assets/assets.db`(SQLite,WAL)与
  `<配置目录>/image-assets/references/<id>.<ext>`(图片文件)。不用 tingly.db,
  不用 StoreManager。
- **依赖最少**:只依赖 gin、gorm、logrus 与仓库内的 `swagger` 路由包;不依赖
  `internal/` 下任何包。
- **鉴权归宿主**:包本身不做鉴权,路由注册在宿主已经挂好用户鉴权中间件的
  `apiV1` 组上(`server_webui_api.go`)。
- **懒打开**:`NewStore` 不碰磁盘,第一次请求才建目录、开库、迁移表;所以生成
  OpenAPI 时注册路由不会产生文件。`Server.Stop` 里关闭。

### API(`/api/v1/image-assets`)

| 方法与路径 | 作用 |
|---|---|
| `GET /pieces` | 列出片段,最近编辑的在前 |
| `POST /pieces` | 批量新建/更新(带 `id` 即更新,保留 `created_at`),一个事务,全成或全不成 |
| `DELETE /pieces/:id` | 删除片段 |
| `GET /references` | 列出参考图元数据(不含图片字节),最新在前 |
| `POST /references` | 以 data URL 上传(每次最多 10 张、每张 25 MB);按 SHA-256 去重,已存在的返回 `existing: true` |
| `PUT /references/:id` | 重命名 |
| `DELETE /references/:id` | 删除记录与文件 |
| `GET /references/:id/content` | 图片字节;同一 id 的字节永不变,响应可长期缓存 |

- 图片类型由字节判定(PNG/JPEG/WebP/GIF),不信任文件名或声明的类型;尺寸由服务端
  读出(WebP 从文件头解析,不引新依赖)。
- 先写文件再写记录,记录永远不会指向不存在的文件;插入失败会删掉刚写的文件。
- 模型名带前缀或具体化(`PromptPiece`、`ReferenceImage`、`ImageAsset*Request/Response`),
  因为 OpenAPI 的 schema 按 Go 类型名全局登记。
- 鉴权只认 `Authorization` 头,`<img src>` 带不上,所以前端用带鉴权的请求取字节、
  转成 data URL,并按 id 缓存(`store.ts`)。

### 已知取舍

- 列表页会取回每张图的完整字节(按 id 缓存,每次页面加载只取一次)。图多了以后,
  加一个缩略图端点即可,API 形状不用变。
- 之前分支上的 IndexedDB 版本没有发布过,不做迁移。

## 7. 入口与物件流转(原则 2、11、12)

Playground 不必离开工作面就能存取:

| 方向 | 位置 |
|---|---|
| 存 Prompt | Prompt 输入框书签菜单 → "保存当前 Prompt"(完全相同的整段 Prompt 不重复存) |
| 取 Prompt / 片段 | 同一个菜单,片段追加、整段替换 |
| 存图片 | 灯箱(任意图:结果、原图、参考图、导入图)→ "存为素材"(同图不重复存,由服务端判定) |
| 取图片 | 参考图行的第四个来源 **Assets / 素材** → 多选对话框(按点击顺序,受 5 张上限约束) |

Assets 页负责浏览、拆解、整理。页上的"在 Playground 中使用 / 追加到 Prompt /
用作参考图"通过 router state 把物件交给 Playground(`handoff.ts`),图片只传 id,
由 Playground 从 store 读(会话历史里不放 base64)。state 消费一次后清掉,
刷新或后退不会重复应用。

## 8. 代码位置

后端:`internal/imageasset/`

| 文件 | 内容 |
|---|---|
| `asset.go` | 领域类型、校验与限额 |
| `image.go` | 图片类型嗅探、尺寸(含 WebP 头解析)、data URL 解码 |
| `store.go` | SQLite + 文件存储 |
| `handler.go` / `routes.go` | HTTP 层与路由注册(带 swagger 描述) |

前端:`frontend/src/pages/image/assets/`,和 Playground 的组件分开。Playground 只通过
`usePlaygroundAssets`、`PromptMenu`、`ImagePickerDialog` 三个入口接入;Assets 反过来
只用到 Playground 的几个通用小件(`ThumbImage`、`imageFiles`)。

| 文件 | 内容 |
|---|---|
| `model.ts` | 类型(`PromptPiece`、`AssetImage`)与纯函数:拆解、追加、标签、搜索 |
| `store.ts` | 对后端 API 的映射与缓存:`listPieces` / `savePieces` / `deletePiece`、`listImages` / `addImages` / `renameImage` / `deleteImage`、`subscribeAssets` |
| `useImageAssets.ts` | 读取并订阅变更的 hook |
| `handoff.ts` | Assets 页 → Playground 的 router state |
| `usePlaygroundAssets.ts` | Playground 侧:存 Prompt、存图、接收 handoff、选择器开关 |
| `PromptMenu.tsx` / `ImagePickerDialog.tsx` | Playground 里的书签菜单与参考图选择器 |
| `PiecesPanel.tsx` / `ImagesPanel.tsx` | Assets 页的两个 tab |
| `PieceEditorDialog.tsx` / `SplitDialog.tsx` | 新建编辑、拆解 |
| `fields.tsx` / `AssetsChrome.tsx` | 共用的类型切换、标签输入、搜索框、空状态 |

另外:`services/imageAssetsApi.ts`(生成的客户端上的薄封装)、
`mocks/imageAssetsHandlers.ts`(mock 模式的内存实现)、
页面入口 `pages/image/ImageAssetsPage.tsx`(`?tab=references` 打开参考图 tab)。
