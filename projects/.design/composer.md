# Composer（输入框）结构规则

Projects 的每个写作表面共用一套输入框组件（`frontend/packages/projects/src/components/Composer.tsx` 与其草稿层
`ThreadComposer.tsx`）。本文记录表面的清单、结构的分层，以及"什么摆在哪"的规则——差异是按语义刻意保留的，
新表面或改动先对照这里，不要发明第二种摆法。

## 表面

| 表面 | 组件 | 草稿 key |
|---|---|---|
| 文件夹会话面板 / 项目线程面板 | `ThreadPanel` → `ThreadComposer` | `thread.id` |
| 项目会话（协调者对话） | `ConversationPane` → `ThreadComposer` | 主线程 id |
| 文件夹新会话 | `NewSessionView` → `ThreadComposer` | `new-session`（换 folder 不丢） |
| 新聊天首页 | `HomePage` → `ThreadComposer` | `home` |
| 聊天页 | `ChatPage` → `ThreadComposer`（`key={id}`） | `chat.{id}` |

草稿规则：**每个表面都留草稿**（`ThreadComposer` 写 localStorage，`utils/storage`）；发送被接受才清空
（keep-until-accepted：发出失败的请求不吞字）；按会话换 key（换会话/聊天各看各的草稿）。

## 结构分层（什么摆在哪）

一个输入框从上到下只有四个可插槽位，每个槽位有固定的语义：

1. **上方条（`context`）**——"之后启动的工作"按什么默认跑：线程面板的 profile/model/permission（本线程的下一次
   启动）、会话的 project defaults（本项目新线程的默认）。改的是**还没发生的启动**，所以悬在文本之上。
2. **底部行右侧（`model`/`effort` props）**——"这条消息/这轮回复"用谁跑：首页与聊天页的模型、会话的协调者
   model+effort。改的是**眼前这条消息**，跟 Send 同一行。
3. **底部行左侧（`leftSlot`）**——只有新会话在用：folder + launch pickers 是"这份工作**去哪**"的目的地选择，
   新会话没有上方条（还没有线程），所以唯一一次允许内联。
4. **输入框之上、之外的条**——`ComposerStrip`（cwd/branch/auto-answer，仅线程表面）、`HeldMessages`（仅线程
   表面）、`MessageDetailControl`（有转录的表面）、宿主 `statusLine`（仅文件夹会话）。会话表面刻意没有 strip：
   主线程没有 cwd/branch，auto-answer 是 worker-only，协调者的提问由用户在 DecisionCard 上作答。

刻意保留的差异：send-modes 菜单与 Stop 只在 agent 轮次表面（`sendModes` + `onStop`）；普通 LLM 聊天
（`ChatPage`）流式中直接禁发，没有排队语义。attach 只在有上传通道的表面（聊天页没有）。

## 其它规则

- **底部小字**：至多一条，由表面自己拥有（首页的 `home.projectHint`、聊天页的 `chat.disclaimer`），居中放在输入
  框之外。不放快捷键提示——Send 的 Tooltip 已带（`composer.send`），快捷键条（`keysHint`）已删除。
- **minRows**：起始/hero 表面（首页、新会话）3 行，会话中 1 行。
- **ComposerStrip 的折叠状态按线程记忆**（`thread.strip.open.{id}`），不是全浏览器一个开关。

## 历史

- 2026-10：会话 composer 补齐 send-modes 与 Stop（协调者轮次可中断，见 `.design/agentboot-live-control.md`），
  随后草稿接入全部表面、删除死代码 `keysHint`、strip 折叠改 per-thread（PR #2352）。
