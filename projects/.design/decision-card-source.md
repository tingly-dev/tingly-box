# 决策卡的来源：每个回答都说明它答的是哪张卡

## 问题

协调者为 #2728 和 #2682 的冲突各发了一张决策卡，用户依次选了“我自己处理”“放弃 PR”。协调者收到的是两条普通用户消息，内容只有选项文字（`Choose` 有意不引用协调者写的问题，避免把它的措辞当成用户的话）。两张卡的选项完全相同，协调者无法判断哪条对应哪张卡、哪个线程，只能再发一张卡确认。按顺序配对也不可靠：用户可以先答后一张，IM 和网页也可以交错作答。

复现测试：`decision_source_test.go` 的 `TestTwoAnswersSayWhichCardAndThreadEachIsFor`（修复前，两条回答的信封都只有 `source/msg/at/trust`）。

## 设计

1. **卡片声明它关于什么**。`request_decision` 新增两个可选参数：`thread_id`（线程 id 前缀）和 `number`（PR 或 issue 编号）。两者都不填时卡片照常可用。系统负责解析和补全（`Coordinator.resolveSource`）：
   - `thread_id` 必须是本项目的工作线程（不能是协调者自己的主线程），否则报错 `no thread "…" in this project`。
   - 只给了线程：如果它**恰好有一个** open 的 PR，补上该编号。
   - 只给了编号：如果**恰好一个**线程的 links 里有这个编号，补上该线程；有多个或没有就不补，不猜。
   - PR 还是 issue（`link_kind`）从线程的 links 读取；不知道时显示为 `#N`。
   - 线程在项目里的编号 `thread_seq` 和标题 `thread_title` 在发卡时取一份快照。payload 里的这些字段都由系统写入，模型传进来的会被覆盖。
2. **卡片上显示来源**。给用户看的地方都用项目内的线程编号称呼线程（`#12 Fix login`，与 IM 里 `threadTag`、`@12`、`/session 12` 一致），不用 id。网页卡片在“需要你来决定”下面显示一行“关于 #12 <线程标题> · PR #2728”，可点击打开线程（编号和标题取线程的当前值，快照作为后备）。IM 文字卡在问题下面加一行 `About #12 Fix login · PR #2728`；一个 chat 里有多张未答卡片、回复又不够明确时，列表的每一行也带上来源：`a3f9 · <问题> (#12 Fix login · PR #2728)`（四位代码规则沿用 #2692，未改）。
3. **回答带上系统写的来源头**。`Choose`（网页点击、IM 按钮、IM 文字/代码回复都走这里）仍然把**只有选项文字**的用户消息交给协调者，同时在这条消息的 payload 里记录 `answer: {card, question, thread_id, thread_title, number, link_kind}`。协调者读到的信封是：

   ```
   <event source="web" msg="ab12cd34" at="…" kind="answer" card="ef56ab78" question="Resolve the conflicts of PR #2728?" thread="Fix login" id="1a2b3c4d" pr="2728" trust="principal">
   I'll handle it
   </event>
   ```

   来源都在信封属性里，是系统写的、在用户的话前面；正文仍是用户选的选项，trust 仍是 `principal`，所以它仍然是用户本人的批准，`msg` 仍可被 `cite`。问题文字放在属性里，不会被当成用户说的话（属性值经过 `attrValue` 清洗，不能伪造标签或换行）。
4. **转给线程时也带上问题**。协调者用 `message_thread` 带 `cite` 引用这条回答时，系统在引用的用户原话下面追加一行 `(the user's pick on the decision card "…", about #12 Fix login · PR #2728)`。单独一句“我自己处理”对线程没有意义，附上问题线程才知道答的是什么。这一行同样由系统写入（协调者写不出 `quoted by the system` 标记）。
5. **协调者提示词**：问题涉及某个线程或它的 PR 时传 `thread_id`（以及 `number`）；收到 `kind="answer"` 的用户事件，按信封里的 `card` 和 `id` 处理、转给对应线程并 cite 这条消息；连续几条回答各自对应各自的卡，不按顺序配对。`pr_conflicts` 的指令改为“用事件里的线程作为 thread_id、PR 作为 number”。协调者自己历史里的卡片记录也带上卡片 id 和来源：`(I asked the user, card ef56ab78 about #12 Fix login · PR #2728: …)`，与回答里的 `card` 对得上。

协调者这一侧仍用 8 位 id 前缀（信封的 `id` 属性、`request_decision` 的 `thread_id`）：它的线程表、`list_threads`、`get_thread`、`message_thread` 等所有工具都按 id 前缀寻址，信封里给 id 才能直接拿来调用工具。线程编号只出现在给用户看的文字里，以及协调者历史中的卡片记录。

## 没有做的

- 用户在网页聊天框里**用文字**回答卡片（不点按钮）仍是一条普通消息，不带来源头，卡片也不会关闭（与之前一致，见 `projects.md` 的决策卡一节）。
- 网页上用户自己的回答气泡仍只显示选项文字，没有显示“回答：<问题>”。

## 测试

- `decision_source_test.go`：两张卡两条回答各自带对的 card/thread/PR（网页 + IM 来源）；只给 PR 编号找到线程；没有来源的卡也带 card 和 question；别的项目的线程、主线程被拒绝，未知编号不猜；cite 一条回答时线程收到问题；IM 纯文字 chat 里两张同问题的卡按来源列出、用代码作答后回答带对的来源。
- `links_conflicts_test.go` 的 `TestAConflictAnswerReachesItsOwnThread`：真实 MCP 路径的完整一轮：冲突 → 协调者带 `thread_id` 发卡 → 用户选择 → 协调者从信封读到 `kind="answer"` 和 `id` → `message_thread` + `cite` → 线程收到用户原话和问题。
- `tool_calls.golden` / `tool_list.golden` 已更新；`ConversationPane.test.tsx` 覆盖网页卡片的来源行。
