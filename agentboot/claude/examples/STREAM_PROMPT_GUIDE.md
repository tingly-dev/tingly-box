# 流式 Prompt 交互模式详解

## 核心概念

流式 Prompt 模式通过 **stdin** 向 Claude CLI 发送多轮消息，通过 **stdout** 接收响应。

```
┌─────────────────────────────────────────────────────────────────┐
│                         你的程序                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. 构建 Prompt                          2. 接收响应             │
│  ┌─────────────────┐                    ┌──────────────────┐   │
│  │StreamPromptBuilder│                   │   query.Next()   │   │
│  ├─────────────────┤                    ├──────────────────┤   │
│  │ AddUserMessage()│                    │ Messages channel │   │
│  │ AddText()       │                    │ Errors channel   │   │
│  │ Add()           │                    │ Done channel     │   │
│  └────────┬────────┘                    └────────┬─────────┘   │
│           │                                     │               │
│           ▼                                     ▼               │
└───────────┼─────────────────────────────────────┼───────────────┘
            │                                     │
            │ stdin (line-delimited JSON)        │ stdout
            │                                     │
            ▼                                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Claude CLI 进程                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  --input-format stream-json                                     │
│                                                                 │
│  读取 stdin ───────► 处理 ───────► 写入 stdout                  │
│     ▲                           │                               │
│     │                           ▼                               │
│     │                    ┌─────────────┐                       │
│     │                    │ canCallTool │ ◄─── 需要工具权限时   │
│     │                    │   回调      │      调用你的回调       │
│     │                    └─────────────┘                       │
│     │                           │                               │
│     │                           ▼                               │
│     │                    写入 stdout (control_response)         │
│     │                                                           │
│     └─────────── 读取 stdin (control_response)                  │
└─────────────────────────────────────────────────────────────────┘
```

## 交互流程

### 方式一：预构建多轮上下文

```go
builder := claude.NewStreamPromptBuilder()

// 添加上下文消息
builder.AddUserMessage("我是 Go 初学者")
builder.AddUserMessage("什么是 goroutine？")
builder.AddUserMessage("给我一个例子")

// 关闭 builder，开始查询
query := launcher.Query(ctx, QueryConfig{
    Prompt: builder.Messages(),  // StreamPrompt
    Options: &QueryOptionsConfig{...},
})

// 读取响应
for msg, ok := query.Next(); ok; msg, ok = query.Next() {
    // 处理消息
}
```

**流程图：**
```
你的程序                    Claude CLI
   │                           │
   ├─消息1────────────────────►│
   ├─消息2────────────────────►│
   ├─消息3────────────────────►│
   │                    [处理中]│
   │◄───────── assistant ───────┤
   │◄───────── tool_use ────────┤  [需要工具]
   │
   [canCallTool 被调用]
   │
   ├─control_response─────────►│  [批准工具]
   │◄───────── tool_result ─────┤
   │◄───────── result ──────────┤
```

### 方式二：实时流式输入

```go
// 创建一个 channel 作为消息源
messageChan := make(chan map[string]interface{})

// 启动 goroutine 持续发送消息
go func() {
    for {
        msg := <-getUserInput()  // 从某处获取输入
        messageChan <- msg
    }
    close(messageChan)
}()

// 使用 channel 作为 prompt
query := launcher.Query(ctx, QueryConfig{
    Prompt: messageChan,
    Options: &QueryOptionsConfig{
        CanCallTool: myToolCallback,
    },
})
```

## canCallTool 交互详解

当 Claude 需要使用工具时，会通过 **stdout** 发送 `control_request`：

```
{"type":"control_request","request_id":"xxx","request":{"subtype":"can_use_tool","tool_name":"bash","input":{...}}}
```

你的程序需要：
1. 解析这个请求
2. 调用你的 `canCallTool` 回调
3. 通过 **stdin** 返回 `control_response`：

```
{"type":"control_response","response":{"subtype":"success","request_id":"xxx","response":{...}}}
```

## 消息类型

| 类型 | 方向 | 说明 |
|------|------|------|
| `system` | Claude→你 | 会话初始化，包含 session_id |
| `assistant` | Claude→你 | Claude 的文本回复 |
| `tool_use` | Claude→你 | Claude 要使用工具 |
| `tool_result` | Claude→你 | 工具执行结果 |
| `result` | Claude→你 | 会话结束，最终结果 |
| `control_request` | Claude→你 | 工具权限请求 |
| `control_response` | 你→Claude | 工具权限响应 |
| `control_cancel_request` | Claude→你 | 取消请求 |

## 完整示例

```go
package main

import "github.com/tingly-dev/tingly-box/agentboot/claude"

func main() {
    // 1. 构建流式 prompt
    builder := claude.NewStreamPromptBuilder()
    builder.AddUserMessage("列出当前目录的文件")

    // 2. 定义工具权限回调
    canCallTool := func(ctx context.Context, toolName string, input map[string]interface{}, opts claude.CallToolOptions) (map[string]interface{}, error) {
        if toolName == "bash" {
            println("批准使用 bash 工具")
            return map[string]interface{}{"approved": true}, nil
        }
        return nil, fmt.Errorf("工具未批准")
    }

    // 3. 创建 Query
    launcher := claude.NewQueryLauncher(claude.Config{})
    query, _ := launcher.Query(ctx, claude.QueryConfig{
        Prompt: builder.Messages(),
        Options: &claude.QueryOptionsConfig{
            CanCallTool: canCallTool,
        },
    })
    defer query.Close()

    // 4. 读取响应
    for {
        msg, ok := query.Next()
        if !ok { break }

        switch msg.Type {
        case "assistant":
            fmt.Printf("Claude: %s\n", msg.Message)
        case "tool_use":
            fmt.Printf("使用工具: %s\n", msg.Request["name"])
        case "result":
            fmt.Printf("完成: %s\n", msg.Result)
        }
    }
}
```

## 运行示例

```bash
cd /root/tingly-box/agentboot/claude/examples

# 3a - 流式 Prompt 基础讲解
go run stream_prompt_example.go 3a

# 3c - 流式 + 工具权限回调 (推荐)
go run stream_prompt_example.go 3c

# 3d - 完整交互式会话
go run stream_prompt_example.go 3d
```
