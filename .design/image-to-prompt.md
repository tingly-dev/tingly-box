# Image to Prompt(反推提示词)

> 状态:**v1 已实施**(TEP-2206)。页面、可编辑指令、Playground 交接、灯箱入口已上线;
> 历史在浏览器 IndexedDB(`imageWorkspaceDb.ts` 的 `imagePrompts`),和方案、素材库一样未接后端。
> 关联:`image-layout.md`(Image 入口)、`image-library.md`(描述片段)、`vision-proxy.md`(同样是"把图交给视觉模型",但目的不同)。

## 1. 它回答的问题

"我手上有这张图，什么样的提示词能生成类似的图?"以前要离开 Tingly Box,把图贴进聊天工具，
再把文字抄回 Playground。图、视觉模型、生图模型都已经在 Tingly Box 里，缺的只是这个工作面。

## 2. 名字

- 英文 **Image to Prompt**,中文 **反推提示词**(中文 AIGC 用户搜索的就是这个词)。按钮和灯箱动作用同一组词:
  "Get prompt / 反推提示词"、"Get a prompt from this image / 从这张图反推提示词"。
- 不叫 Describe / 描述:"描述"在 Vision Proxy 里是"把图转成文字给纯文本模型",在素材库里是"描述片段",
  再用一次就一词两义(原则 3)。
- 中文名里有「提示词」,而顶级栏目也叫「提示词」(编码 agent 的提示词)。这里它是动宾短语的一部分，
  且只出现在 Image 下，判断可以分辨;如果有人混淆，再改。

## 3. 位置

Image 侧栏分隔线之后的第一项:Playground 与方案 → 分隔线 → **Image to Prompt** → Library → Image API。
它是工作面，但不是天天用的那个，所以不插在 Playground 和它的方案中间(方案在视觉上属于 Playground)。
随 Image 入口一起显隐，"我不用生图"仍然只有一个开关。

## 4. 页面

与 Playground 同一个工作台形状(左栏 360–420px,右边结果，lg 上占满高度)。

- **图片**:一张。浏览 / 粘贴 / 素材库，或拖到框里、在页面任意处 Ctrl+V。再放一张就是替换——
  问题是"*这张*图的提示词是什么"。发送前缩到长边 1568px 以内、SVG 栅格化为 JPEG(`preparePicture.ts`):
  视觉 API 不收 SVG、限制大小，而且本来就会缩到这个尺寸。
- **视觉模型**:选 provider · model,与 Playground 相同的 `ImageModelPicker`(加了 `scenario` 参数)。
  上次选的记在 localStorage。
- **给模型的指令**:一个直接可编辑的文本框，不藏在设置里——这是这个页面唯一值得调的东西。
  - 默认是"自然语言"模板;改过之后标题旁显示"已修改",出现"恢复默认"。
  - 只在与默认不同时才存进 localStorage,这样以后默认改进了，没动过的人自动拿到新的。
  - "从模板开始…"有三个起点：自然语言 / 标签列表 / 只要风格。它们只是**填进文本框**,不是模式(原则 2、6)。
  - 指令是给模型看的，所以是英文，不进 i18n;模板名称和说明进 i18n。
- **结果**:新的在上。每张卡：缩略图、可编辑的提示词(失焦时写回)、复制、**留作描述片段**、**在 Playground 使用**、删除。
  失败时显示上游的错误和"请确认模型支持图片输入";重试用同样的输入。点缩略图把图和指令放回左栏(原则 10)。

### 模板的来路

结构借鉴了专做这件事的 captioner:JoyCaption 的 "Straightforward" 和 "Stable Diffusion prompt" 两种 caption type、
Florence-2 的 detailed caption。共同点：主体和媒介在前，然后按固定顺序写可见事实(外观与动作、场景、构图与镜头、光线、
配色、风格),不猜看不见的东西，不用"beautiful / masterpiece"这类空词，图里的字原样引用，只输出提示词本身。
模型还是会包代码块、加 "Prompt:"、套引号，`cleanPromptReply` 把这些去掉(有单测)。

## 5. 模型与链路

- 新的内置场景 **`imageprompt`**:只有 `TransportOpenAI`(chat completions)、允许绑定规则和直连路径;
  前端 `SCENARIOS` 不列它，所以 Agent 概览里看不到。页面用 `getOpenAIClient('imageprompt')` 发
  `system = 指令`、`user = [image_url, "Write the prompt for this image."]`,网关照常做协议转换。
- 为什么是场景而不是后端直接调 provider 的接口：直接调会绕过 flags、负载均衡和故障切换(原则 7),
  而且规则存在，脚本也能用同一个名字调。为什么不复用 `openai`:选择器自动建的规则会落进用户的通用场景。
- 没有"哪些模型能看图"的能力元数据，所以不做过滤;选错了，失败卡片说清楚，用户换一个。

## 6. 交接(原则 11)

- **在 Playground 使用**:`navigate('/image/playground', { state: { prompt } })`。Playground 卡片在非方案页
  用它作为初始提示词，用完即从 history state 去掉，刷新不会覆盖之后的输入。
- **留作描述片段**:`saveSnippet`,名字取提示词开头几个字。
- **入口**:Playground 灯箱里任意一张图(结果、导入、参考图)都有"从这张图反推提示词",带着图打开本页。
  图经 `prompt/pictureHandoff.ts` 的一个模块级槽位交过去，不走 router state:那会把几 MB 的 data URL 克隆进会话历史。

运行状态归 store(`prompt/imagePromptStore.ts`),不归页面：请求、AbortController 都在那里，离开页面再回来，
进行中的请求照常完成;删除或清空会中止请求。刷新时仍是 pending 的记录，在读回时由
`createWorkspaceCollection` 的 `revive` 一次性改成"被刷新打断"。

## 7. 这一版刻意不做的

- 方案页里的入口("填进当前提示词标签")、提示词框旁的"从图片…"弹层。
- "试一下":用反推的提示词生成一张，和原图并排对照。
- 一次多张图(提炼共同风格)、负向提示词、按目标生图模型自动选模板。
- 默认视觉模型取自 Vision Proxy 已配置的那个。
- 历史接后端(随 `image-profile.md` §4 一起)。
