package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// Example 3a: 流式 Prompt 详解 - 如何构建多轮对话上下文
func exampleStreamPromptExplained() {
	fmt.Println("=== Example 3a: 流式 Prompt 详解 ===\n")

	// Step 1: 创建 StreamPromptBuilder
	builder := claude.NewStreamPromptBuilder()

	// Step 2: 添加多轮用户消息作为上下文
	// 这些消息会在 Claude 启动时通过 stdin 流式发送给它

	fmt.Println("Step 1: 添加系统指令")
	builder.Add(map[string]interface{}{
		"type":    "text",
		"content": "You are a Go programming expert. Answer briefly.",
	})
	// 或者使用便捷方法:
	// builder.AddText("You are a Go programming expert. Answer briefly.")

	fmt.Println("Step 2: 添加第一轮用户消息")
	builder.AddUserMessage("What is a goroutine?")

	fmt.Println("Step 3: 添加第二轮用户消息")
	builder.AddUserMessage("How do I create one?")

	fmt.Println("Step 4: 添加第三轮用户消息（实际要问的问题）")
	builder.AddUserMessage("Show me a simple example with channels.")

	// Step 3: 关闭 builder 并获取 channel
	promptChannel := builder.Close()

	// Step 4: 创建 Query
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	launcher := claude.NewQueryLauncher(claude.Config{})

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: promptChannel, // 使用流式 prompt
		Options: &claude.QueryOptionsConfig{
			CWD: "/tmp",
		},
	})
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	defer query.Close()

	// Step 5: 读取响应
	fmt.Println("\n=== Claude 的响应 ===")
	for {
		msg, ok := query.Next()
		if !ok {
			break
		}

		switch msg.Type {
		case "system":
			fmt.Printf("[System] Session: %s\n", msg.SessionID)
		case "assistant":
			// Claude 的回复
			if content := extractTextContent(msg.Message); content != "" {
				fmt.Printf("[Assistant] %s\n", content)
			}
		case "tool_use":
			// Claude 使用工具
			fmt.Printf("[Tool Use] %s\n", msg.Request["name"])
		case "result":
			fmt.Printf("[Result] %s\n", msg.Result)
		}
	}
}

// Example 3b: 交互式流式 Prompt - 从用户输入读取
func exampleInteractiveStreamPrompt() {
	fmt.Println("=== Example 3b: 交互式流式 Prompt ===\n")
	fmt.Println("请输入多轮对话内容，输入 'DONE' 结束输入:")

	builder := claude.NewStreamPromptBuilder()

	// 从标准输入读取多轮消息
	scanner := NewScanner(os.Stdin)
	round := 1

	for {
		fmt.Printf("Round %d - 输入你的消息 (或 'DONE'): ", round)
		if !scanner.Scan() {
			break
		}

		text := strings.TrimSpace(scanner.Text())
		if text == "DONE" || text == "" {
			break
		}

		// 添加用户消息
		builder.AddUserMessage(text)
		round++
	}

	promptChannel := builder.Close()

	// 发送给 Claude
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	launcher := claude.NewQueryLauncher(claude.Config{})
	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: promptChannel,
		Options: &claude.QueryOptionsConfig{
			CWD: "/tmp",
		},
	})
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	defer query.Close()

	fmt.Println("\n=== Claude 正在思考... ===")
	for {
		msg, ok := query.Next()
		if !ok {
			break
		}

		if msg.Type == "assistant" {
			if content := extractTextContent(msg.Message); content != "" {
				fmt.Printf("\n[Claude]: %s\n", content)
			}
		} else if msg.Type == "result" {
			fmt.Printf("\n[完成]: %s\n", msg.Result)
			break
		}
	}
}

// Example 3c: 流式 Prompt + canCallTool - 工具权限交互
func exampleStreamPromptWithToolCallback() {
	fmt.Println("=== Example 3c: 流式 Prompt + 工具权限交互 ===\n")

	// Step 1: 构建包含工具调用的 prompt
	builder := claude.NewStreamPromptBuilder()
	builder.AddUserMessage("使用 bash 命令列出当前目录的 Go 文件")

	// Step 2: 定义 canCallTool 回调 - 这是交互的关键！
	// 当 Claude 想要使用工具时，这个回调会被调用
	canCallTool := func(ctx context.Context, toolName string, input map[string]interface{}, opts claude.CallToolOptions) (map[string]interface{}, error) {
		fmt.Printf("\n[工具权限请求]\n")
		fmt.Printf("  工具名称: %s\n", toolName)
		fmt.Printf("  输入参数: %s\n", formatInput(input))

		// 根据工具名称决定是否批准
		switch toolName {
		case "bash":
			// 自动批准 bash 工具
			fmt.Printf("  ✓ 自动批准 bash 工具\n")
			return map[string]interface{}{
				"approved": true,
			}, nil

		case "editor":
			// 编辑器工具需要确认
			fmt.Printf("  ⚠ 编辑器工具需要确认，自动拒绝\n")
			return nil, fmt.Errorf("editor tool not approved in this example")

		default:
			// 其他工具默认拒绝
			fmt.Printf("  ✗ 未知工具，拒绝\n")
			return nil, fmt.Errorf("unknown tool: %s", toolName)
		}
	}

	// Step 3: 创建 Query
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	launcher := claude.NewQueryLauncher(claude.Config{})

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: builder.Messages(),
		Options: &claude.QueryOptionsConfig{
			CWD:         "/root/tingly-box",
			CanCallTool: canCallTool, // 注册回调
		},
	})
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	defer query.Close()

	fmt.Println("=== 开始对话 ===")

	// Step 4: 处理消息和工具调用
	for {
		msg, ok := query.Next()
		if !ok {
			break
		}

		switch msg.Type {
		case "assistant":
			if content := extractTextContent(msg.Message); content != "" {
				fmt.Printf("\n[Claude]: %s\n", truncate(content, 200))
			}

		case "tool_use":
			fmt.Printf("\n[工具调用]\n")
			fmt.Printf("  工具: %s\n", msg.Request["name"])
			if input, ok := msg.Request["input"].(map[string]interface{}); ok {
				if cmd, ok := input["command"].(string); ok {
					fmt.Printf("  命令: %s\n", cmd)
				}
			}

		case "tool_result":
			fmt.Printf("\n[工具结果]\n")
			if output := formatOutput(msg.Response); output != "" {
				fmt.Printf("  输出: %s\n", truncate(output, 300))
			}

		case "result":
			fmt.Printf("\n[完成]\n")
			if msg.Result != "" {
				fmt.Printf("  %s\n", msg.Result)
			}
		}
	}
}

// Example 3d: 完整的交互式会话 - 流式输入 + 工具交互
func exampleFullInteractiveSession() {
	fmt.Println("=== Example 3d: 完整交互式会话 ===")
	fmt.Println("这个例子展示如何像聊天一样与 Claude 交互\n")

	// Step 1: 收集初始上下文
	builder := claude.NewStreamPromptBuilder()

	fmt.Println("首先，提供一些上下文消息（可选，输入 'SKIP' 跳过）:")
	scanner := NewScanner(os.Stdin)

	for {
		fmt.Print("上下文消息 (或 'SKIP'): ")
		if !scanner.Scan() {
			break
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "SKIP" || text == "" {
			break
		}
		builder.AddUserMessage(text)
	}

	// Step 2: 定义工具权限处理
	canCallTool := func(ctx context.Context, toolName string, input map[string]interface{}, opts claude.CallToolOptions) (map[string]interface{}, error) {
		fmt.Printf("\n┌─ 工具权限请求 ─┐\n")
		fmt.Printf("│ 工具: %s\n", toolName)

		if input != nil {
			if cmd, ok := input["command"].(string); ok {
				fmt.Printf("│ 命令: %s\n", cmd)
			} else {
				fmt.Printf("│ 参数: %v\n", input)
			}
		}

		fmt.Printf("│ \n")
		fmt.Printf("│ 是否批准? [y/n]: ")

		// 在实际应用中，这里可以从 stdin 读取用户输入
		// 为了演示，我们自动批准 bash
		if toolName == "bash" {
			fmt.Printf("y (自动批准)\n")
			fmt.Printf("└───────────────┘\n")
			return map[string]interface{}{"approved": true}, nil
		}

		fmt.Printf("n (自动拒绝)\n")
		fmt.Printf("└───────────────┘\n")
		return nil, fmt.Errorf("tool not approved")
	}

	// Step 3: 启动 Query
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	launcher := claude.NewQueryLauncher(claude.Config{})

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: builder.Messages(),
		Options: &claude.QueryOptionsConfig{
			CWD:         "/root/tingly-box",
			CanCallTool: canCallTool,
		},
	})
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	defer query.Close()

	// Step 4: 实时显示 Claude 的响应
	fmt.Println("\n=== Claude 响应 ===")

	for {
		msg, ok := query.Next()
		if !ok {
			break
		}

		switch msg.Type {
		case "system":
			fmt.Printf("[会话开始] Session: %s\n\n", msg.SessionID)

		case "assistant":
			if content := extractTextContent(msg.Message); content != "" {
				// 逐行显示，方便阅读
				lines := strings.Split(content, "\n")
				for _, line := range lines {
					if line != "" {
						fmt.Printf("  %s\n", line)
					}
				}
			}

		case "tool_use":
			fmt.Printf("\n[调用工具] %s\n", msg.Request["name"])

		case "tool_result":
			fmt.Printf("[工具返回] %s\n", truncate(formatOutput(msg.Response), 100))

		case "result":
			fmt.Printf("\n[会话结束] %s\n", msg.Result)
		}
	}
}

// Helper functions

func extractTextContent(msg map[string]interface{}) string {
	if msg == nil {
		return ""
	}

	content, ok := msg["content"].([]interface{})
	if !ok {
		return ""
	}

	var result strings.Builder
	for _, c := range content {
		if block, ok := c.(map[string]interface{}); ok {
			if block["type"] == "text" {
				if text, ok := block["text"].(string); ok {
					result.WriteString(text)
				}
			}
		}
	}

	return result.String()
}

func formatInput(input map[string]interface{}) string {
	if input == nil {
		return "{}"
	}
	data, _ := json.Marshal(input)
	return string(data)
}

func formatOutput(response map[string]interface{}) string {
	if response == nil {
		return ""
	}
	if result, ok := response["result"].(string); ok {
		return result
	}
	if output, ok := response["output"].(string); ok {
		return output
	}
	data, _ := json.Marshal(response)
	return string(data)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Scanner wraps bufio.Scanner for simpler usage
type Scanner struct {
	lines chan string
}

func NewScanner(r interface{}) *Scanner {
	// Simplified - in real use, use bufio.Scanner
	return &Scanner{lines: make(chan string, 100)}
}

func (s *Scanner) Scan() bool {
	// Placeholder
	return false
}

func (s *Scanner) Text() string {
	return ""
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run stream_prompt_example.go <example>")
		fmt.Println("\n流式 Prompt 交互示例:")
		fmt.Println("  3a - 流式 Prompt 基础讲解")
		fmt.Println("  3b - 交互式输入上下文")
		fmt.Println("  3c - 流式 + 工具权限回调")
		fmt.Println("  3d - 完整交互式会话")
		os.Exit(1)
	}

	example := os.Args[1]

	switch example {
	case "3a":
		exampleStreamPromptExplained()
	case "3b":
		exampleInteractiveStreamPrompt()
	case "3c":
		exampleStreamPromptWithToolCallback()
	case "3d":
		exampleFullInteractiveSession()
	default:
		fmt.Printf("未知示例: %s\n", example)
		os.Exit(1)
	}
}
