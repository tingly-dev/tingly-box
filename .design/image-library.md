# Image Library(素材库:Prompt 素材 + 参考图)

> 适用对象:tingly-box 前端贡献者。
> Image rail 下第三个子页 **Library**(`/image/library`)的定位、数据模型与演进方向。
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

所以 Library 是 **素材**,不是历史。会话 ≠ 素材库(原则 3,一词一义)。

## 2. 关键判断:存的是"片段",不只是整段 Prompt

真正可复用的往往不是一整段 Prompt,而是从里面拆出来的关键词条和描述语句。因此
Prompt 素材统一为一种记录,按**用途**分三种 kind(`utils/imageLibrary.ts`):

| kind | 例子 | 在 Playground 里的动作 |
|---|---|---|
| `prompt` | 整段 Prompt | **替换**输入框 |
| `term` | `rim lighting`、`35mm`、`赛博朋克` | **追加**到 Prompt(`appendPromptPiece`) |
| `phrase` | `a quiet street after rain` | **追加**到 Prompt |

kind 是同一种东西的一个轴,而不是三种东西(原则 4):新建时在表单里切换,事后
也能改。另外两个轴:

- **tags**:自由标签(风格、光线、主体、项目……),小写去重。按标签筛选。
- **sourceId**:片段拆自哪一段 Prompt(溯源)。删除原 Prompt 不删片段。

## 3. 拆解:现在由人来做,接口为 AI 留好

`PromptSplitDialog` 流程:

1. `suggestPromptPieces(text)` 按标点(中英文逗号、分号、顿号、句末标点、换行、
   列表符号)做**机械初切**,并按长度猜 term / phrase。它只是起点,不判断什么重要。
2. 人来审:取消勾选不值得留的、改写措辞让每条能独立使用、切换 term/phrase、
   补一条、统一打标签。已经在库里的片段默认不勾选。
3. 一次事务写入(`saveLibraryPrompts`),每条带 `sourceId`。

**演进位点**:`suggestPromptPieces` 的输入(一段文本)与输出
(`PromptPieceCandidate[]`:text + kind)就是 AI 拆解器要替换的接口。换成 AI 后,
审阅这一步保留——AI 给候选,人确认。

## 4. 组装:现在是手工拼装,未来是 Agent 选取

现在的组装:Playground Prompt 输入框上的书签按钮(`LibraryPromptMenu`)——
"追加到 Prompt"列出词条/语句,"替换 Prompt 为"列出整段 Prompt。

更远的方向:Agent 根据需求,按 kind + tag 选取片段,拼装出符合要求的 Prompt,
把 Prompt 素材当作资产来用。当前数据模型为此准备了:

- 片段是原子的、有类型的(kind)、可检索的(tags)、可溯源的(sourceId)。
- 所有读写只经过 `utils/imageLibrary.ts` 一个模块。

**已知限制**:素材库存在浏览器 IndexedDB 里,只在当前浏览器可见,服务端(以及
将来的 Agent)读不到。走到 Agent 组装那一步时,需要把存储换成后端 API;因为只有
一个存储模块,换后端时只需改这一个文件,UI 不动。页面副标题如实写着"保存在当前
浏览器中"(原则 5)。

## 5. 入口与物件流转(原则 2、11、12)

Playground 不必离开工作面就能存取:

| 方向 | 位置 |
|---|---|
| 存 Prompt | Prompt 输入框书签菜单 → "保存当前 Prompt"(完全相同的文本不重复存) |
| 取 Prompt / 片段 | 同一个菜单,片段追加、整段替换 |
| 存图片 | 灯箱(任意图:结果、原图、参考图、导入图)→ "保存到素材库"(同图不重复存) |
| 取图片 | 参考图行的第四个来源 **Library** → 多选对话框(按点击顺序,受 5 张上限约束) |

Library 页负责浏览、拆解、整理。页上的"在 Playground 中使用 / 追加到 Prompt /
用作参考图"通过 router state 把物件交给 Playground(`libraryHandoff.ts`),图片只
传 id,由 Playground 从库里读(会话历史里不放 base64)。state 消费一次后清掉,
刷新或后退不会重复应用。

## 6. 代码位置

| 路径 | 内容 |
|---|---|
| `frontend/src/utils/imageLibrary.ts` | IndexedDB 存储、kind/tags 模型、拆解与追加的纯函数 |
| `frontend/src/pages/image/ImageLibraryPage.tsx` | 页面:Prompts / 参考图两个 tab(`?tab=references`) |
| `frontend/src/pages/image/components/LibraryPromptsPanel.tsx` | Prompt 素材列表、kind 与标签筛选 |
| `frontend/src/pages/image/components/PromptSplitDialog.tsx` | 拆解对话框 |
| `frontend/src/pages/image/components/LibraryPromptEditorDialog.tsx` | 新建/编辑 |
| `frontend/src/pages/image/components/LibraryReferencesPanel.tsx` | 参考图网格、上传/拖入/粘贴 |
| `frontend/src/pages/image/components/LibraryPromptMenu.tsx` | Playground Prompt 输入框上的菜单 |
| `frontend/src/pages/image/components/LibraryReferencePickerDialog.tsx` | Playground 参考图选择器 |
| `frontend/src/pages/image/components/libraryHandoff.ts` | Library → Playground 的 router state |
