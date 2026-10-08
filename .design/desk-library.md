# Desk 任务库（Library）

> 状态：**设计稿，未实现**。受众：改 Desk 前端（`frontend/src/pages/desk`）的人。
> 前置阅读：`desk.md`（Desk 是什么）、`image-library.md`（同类的"留下来再用"模式）、`ux-principles.md`。

## 1. 为什么

Desk 现在的主线是"选目录 → 写一段话 → 看 agent 干活"，重心在 **chat**。
但多数人反复做的是同一批事：review 当前改动、跑测试并修掉失败、解释这个仓库、写 commit message……
每次都重新敲一遍提示词，是摩擦税。

类比镜像库：不是先开一个终端再敲 `docker run ...`，而是库里挑一个，点一下就跑。

> **Desk 最重要的不是对话框，而是一个常用任务库：选中、一键下发、立刻开工。**
> chat 退到"开工之后的跟进"，不再是入口。

## 2. 一句话模型

```
库里的一条预设  ──▶ Run ──▶  一个新的 Desk session（已经在干活）
(提示词 + 可选默认设置)       （之后就是普通 session，可以接着聊）
```

- 预设是**模板**，session 是**实例**。Run 时把预设**拷一份**成 session 的首条消息，之后互不关联
  （和素材库"按拷贝插入"一致，不做"改一处处处更新"）。
- 库**不**引入新的执行路径：Run = 现有的 `POST /desk/sessions`（目录 + 提示词 + profile + permission mode）。
  后端不需要为"一键开工"改任何东西。

## 3. 命名

- 库叫「任务库」（Library）。条目叫「预设」（Preset）。
- 不叫「任务」：Desk 里已有"后台任务（background tasks）"（`desk.md` §3.9）。
- 不叫「命令」：会和 Claude Code 的 slash command 撞。
- 不叫「技能」：侧栏已有顶级栏目「提示词」（agent 提示词与技能）。

## 4. 一条预设是什么

| 字段 | 必填 | 说明 |
|---|---|---|
| `title` | 是 | 一行，用户认得出来即可：`Review 当前改动` |
| `prompt` | 是 | **字面提示词**，卡片上直接展示（ux §5：给具体值，不是别名） |
| `profile` | 否 | 空 = Default configuration profile（沿用 desk.md 的约定） |
| `permission_mode` | 否 | 空 = 沿用 session 默认 |
| `group` | 否 | 分组标签，如「代码」「Git」「排查」。只用于库内排版 |

**不含目录。** 目录是"这次在哪儿干"，随用随选，不是预设的属性（ux §4：正交维度分轴——
"做什么"和"在哪儿做"是两条轴）。

v1 **不做变量/参数**。需要变化的部分，用户 Run 之前点"编辑后运行"改文本即可。
见 §9 的后续。

## 5. 交互

### 5.1 入口：库是 Desk 的首屏

- 没有选中 session 时，主区**直接显示任务库**（卡片网格），不是空白聊天框（ux §2：直接打开工作面，不先选模式）。
- 库顶部一个目录选择条："在 `~/work/foo` 里运行 ▾"。默认值 = 侧栏当前选中的项目，
  其次是最近使用的目录（沿用 `RecentFolders` + 浏览器项目快捷方式，不新增接口）。
- 侧栏的 "New" 仍然存在，打开原来的任务表单（自由输入），表单提示词框上方是一排最近用过/置顶的预设芯片，
  点一下把提示词填进去。**chat 入口不删，只是不再是唯一入口。**

### 5.2 一键开工

卡片：标题 + 提示词前两行（等宽，省略号）+ 右下 **▶ 运行**。

- 点 ▶：用顶部目录条里的目录 + 预设的 profile/permission mode 直接创建 session，
  并跳转到该 session（已经在 running）。**不弹确认框**。
- 目录尚未选（首次、没有最近目录）：目录条高亮并聚焦，▶ 置灰并提示"先选一个目录"。
  这是唯一会拦住一键运行的情况。
- 次要动作"编辑后运行"（卡片 ⋯ 菜单 / 悬停按钮）：打开原任务表单，预填提示词与设置，用户改完再发。
- 目录不存在等校验失败：由现有创建接口返回，就地显示在目录条下，不跳走。

### 5.3 留下来：怎么进库

只有用户主动才进库（同素材库"没有隐式加入"）：

- 任务表单发送区旁："同时存入任务库"勾选（默认**关**）。
- 任意 session 顶部"存为预设"：取该 session 的首条用户消息作为 `prompt`，标题让用户补一句。
- 库里"新建预设"：标题 + 提示词，两个框。

编辑/删除：卡片 ⋯ 菜单。删除就地二次确认；删除预设不影响已经由它开出的 session。

### 5.4 内置预设

首次打开库就不是空的（ux §6 聪明默认，§8 教育内嵌）。随前端发布，只读，用户可"复制到我的"后再改：

| 分组 | 标题 | 提示词（示意） |
|---|---|---|
| 代码 | Review 当前改动 | Review the uncommitted changes in this repo. List correctness bugs first, then risky spots, then nits. |
| 代码 | 跑测试并修复 | Run the test suite. For each failure, find the root cause and fix it, then rerun until green. Don't skip or disable tests. |
| 代码 | 解释这个仓库 | Give me a map of this repo: entry points, main modules, how to build and run it, and where to start reading. |
| Git | 写 commit message | Look at the staged changes and propose a commit message following this repo's conventions. Don't commit. |
| Git | 总结最近一周 | Summarize what changed in the last 7 days from git log, grouped by area. |
| 排查 | 找 TODO/FIXME | List TODO/FIXME comments grouped by file, flag ones that look like real bugs. |
| 排查 | 依赖有没有问题 | Check dependencies for outdated or known-vulnerable versions and list upgrade candidates. Don't change anything. |

原则：内置预设**默认只读、不改文件**的居多，让第一次点的人不担心；会改文件的（"跑测试并修复"）
走 session 现有的 permission mode 与审批流，不在库里另设安全层。

## 6. 存储

**v1：浏览器本地**，和 Desk 的"项目快捷方式"同一套做法（`desk.md` "Browser project shortcuts"）：

- 用户自己的预设存 localStorage；内置预设在前端代码里，不入存储。
- 优点：零后端改动，立刻能用、能验证"库作为首屏"这个假设是否成立。
- 代价：不跨浏览器、不跨机器。UI 上不特别提示，等真有人问再说。

**后续（不在 v1）**：落到磁盘文件，一条预设一个 `*.md`（frontmatter 放 title/group/profile，正文是提示词），
放在 tingly-box 数据目录下的 `desk/library/`。理由同素材库 §7："是个文件"和"被管理"是同一个状态，
可以放进 git、可以分享。届时加 `GET/PUT/DELETE /api/v1/desk/library/...`，走 swagger + `task codegen`。
前端按 CLAUDE.md 约定，API 层先用 placeholder 函数包住 localStorage，换后端时只换这一层。

## 7. 前端落点

- `frontend/src/pages/desk/library/`：`LibraryView.tsx`（卡片网格 + 目录条）、`PresetCard.tsx`、
  `PresetDialog.tsx`（新建/编辑）、`presetStore.ts`（localStorage + placeholder API）、`builtinPresets.ts`。
- `DeskPage.tsx`：无选中 session 时渲染 `LibraryView`；任务表单上方加预设芯片行。
- 用 MUI；图标走 `@/components/icons`；路径与提示词用共享 `fontMono`。
- 一键运行直接调用任务表单现在用的创建函数，不复制一份。
- i18n：所有文案走现有 i18n，英文名 Library / Preset。

## 8. v1 刻意不做

- 变量/参数、条件、多步骤编排。
- 预设绑定目录、绑定模型 tier。
- 使用次数/最近使用排序（先用手动分组 + 内置顺序）。
- 导入导出、分享、云同步。
- 一次对多个目录批量运行。
- 在库里搜索（预设少于一屏时不需要；超过再加）。

## 9. 后续的自然下一步

1. **单行变量**：提示词里写 `{{branch}}`，点 ▶ 时卡片就地展开一个输入框再运行
   （仍然不弹对话框；没有变量的预设行为不变）。
2. **磁盘文件库**（§6），再到团队仓库共享。
3. **预设 → 定时/触发**：同一条预设挂到 cron 或 IM 的 `@cc` 上，复用库而不是再写一遍提示词。

## 10. Checklist（对照 ux-principles.md）

- [x] 视图按用户问题组织：首屏回答"我现在想做哪件事"，不是"我想怎么跟 agent 聊"。
- [x] 直接进入工作面：库即首屏，▶ 即开工，无模式选择、无确认框。
- [x] 名词唯一：任务库/预设，避开 task/command/skill。
- [x] 正交分轴：做什么（预设）× 在哪儿做（目录条）。
- [x] 展示具体值：卡片显示真实提示词（等宽）。
- [x] 聪明默认：目录默认取当前项目；内置预设开箱即有；"存入库"默认关，不隐式加入。
- [x] 完成 ≠ 锁死：Run 之后 session 照常可聊、可再运行同一预设。
- [x] 下一步的物件：运行后直接落在正在工作的 session 里；"编辑后运行"保留自由度。
- [x] 副作用限于当前表面：删除预设不影响已开出的 session；库操作不碰侧栏项目列表。
