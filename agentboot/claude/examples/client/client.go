package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Color codes for output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
)

// Client represents a client that sends queries to Claude
type Client struct {
	debug bool
}

// NewClient creates a new client instance
func NewClient() *Client {
	return &Client{
		debug: false,
	}
}

// SetDebug enables debug output
func (c *Client) SetDebug(debug bool) {
	c.debug = debug
}

// ProcessQuery processes a single query using the stream-json server
// This is a simplified example - in real usage, you might call a server API
func (c *Client) ProcessQuery(ctx context.Context, prompt string) (string, error) {
	// In a real client-server setup, this would make an API call
	// For this example, we demonstrate the pattern

	// Build the stream-json format message
	// The server expects this format internally
	message := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role":    "user",
			"content": prompt,
		},
	}

	if c.debug {
		log.Printf("[Debug] Sending message: %+v", message)
	}

	// In real implementation, send to server and get response
	// For now, return placeholder
	return fmt.Sprintf("Response to: %s", prompt), nil
}

// BatchProcess processes multiple queries from a file
func (c *Client) BatchProcess(ctx context.Context, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fmt.Printf("%s[%d] Processing: %s%s\n", ColorCyan, lineNum, line, ColorReset)

		response, err := c.ProcessQuery(ctx, line)
		if err != nil {
			fmt.Printf("%sError on line %d: %v%s\n", ColorRed, lineNum, err, ColorReset)
			continue
		}

		fmt.Printf("%s%s%s\n\n", ColorWhite, response, ColorReset)
	}

	return scanner.Err()
}

// NetworkClient is a client that connects over network
type NetworkClient struct {
	address string
	conn    net.Conn
	debug   bool
}

// NewNetworkClient creates a new network client
func NewNetworkClient(address string) *NetworkClient {
	return &NetworkClient{
		address: address,
		debug:   false,
	}
}

// SetDebug enables debug output
func (c *NetworkClient) SetDebug(debug bool) {
	c.debug = debug
}

// Connect connects to the server
func (c *NetworkClient) Connect(ctx context.Context) error {
	var err error
	c.conn, err = net.DialTimeout("tcp", c.address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	return nil
}

// Close closes the connection
func (c *NetworkClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Send sends a message to the server
func (c *NetworkClient) Send(ctx context.Context, message string) (string, error) {
	if c.conn == nil {
		return "", fmt.Errorf("not connected to server")
	}

	// Send the message
	_, err := fmt.Fprintln(c.conn, message)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	// Read response
	response := make([]byte, 0, 4096)
	buffer := make([]byte, 1024)

	for {
		// Set read deadline
		err = c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		if err != nil {
			return "", fmt.Errorf("failed to set read deadline: %w", err)
		}

		n, err := c.conn.Read(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// No more data, check if we have any response
				if len(response) > 0 {
					break
				}
				continue
			}
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("failed to read response: %w", err)
		}

		response = append(response, buffer[:n]...)

		// Check if response is complete
		if len(response) > 0 {
			break
		}
	}

	return string(response), nil
}

// Run starts the client's interactive loop
func (c *Client) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("%sClaude Client%s\n", ColorCyan, ColorReset)
	fmt.Printf("%sType your message and press Enter. Type 'quit' or 'exit' to quit.%s\n", ColorYellow, ColorReset)
	fmt.Printf("%sPress Ctrl-C to exit.%s\n\n", ColorYellow, ColorReset)

	// Channel to signal scanner to stop
	stopScan := make(chan struct{})
	defer close(stopScan)

	// Goroutine to handle input
	inputChan := make(chan string)
	errChan := make(chan error, 1)

	go func() {
		for {
			select {
			case <-stopScan:
				return
			default:
				if !scanner.Scan() {
					errChan <- scanner.Err()
					return
				}
				userInput := strings.TrimSpace(scanner.Text())
				inputChan <- userInput
			}
		}
	}()

	for {
		// Show prompt BEFORE waiting for input
		fmt.Printf("%sYou%s> ", ColorGreen, ColorReset)

		select {
		case <-ctx.Done():
			fmt.Printf("\n%sInterrupted. Goodbye!%s\n", ColorYellow, ColorReset)
			return nil

		case err := <-errChan:
			return err

		case userInput := <-inputChan:
			if userInput == "" {
				continue
			}

			// Check for exit commands
			if userInput == "quit" || userInput == "exit" || userInput == "q" {
				fmt.Printf("%sGoodbye!%s\n", ColorYellow, ColorReset)
				return nil
			}

			// Process the input
			response, err := c.ProcessQuery(ctx, userInput)
			if err != nil {
				fmt.Printf("%sError: %v%s\n", ColorRed, err, ColorReset)
				continue
			}

			// Display response
			fmt.Printf("\n%sClaude%s:\n%s%s%s\n\n", ColorPurple, ColorReset, ColorWhite, response, ColorReset)
		}
	}
}

func main() {
	// Parse command line arguments
	client := NewClient()

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--batch", "-b":
			if i+1 < len(args) {
				i++
				ctx := context.Background()
				if err := client.BatchProcess(ctx, args[i]); err != nil {
					log.Fatalf("Batch processing error: %v", err)
				}
				return
			}
		case "--debug", "-d":
			client.SetDebug(true)
		case "--help", "-h":
			fmt.Println("Claude Client Example")
			fmt.Println("\nA simple client demonstrating interaction patterns.")
			fmt.Println("\nUsage: go run client.go [options]")
			fmt.Println("\nOptions:")
			fmt.Println("  --batch, -b <file>        Process multiple queries from a file")
			fmt.Println("  --debug, -d               Enable debug output")
			fmt.Println("  --help, -h                Show this help message")
			fmt.Println("\nFile format for batch mode:")
			fmt.Println("  # Lines starting with # are comments")
			fmt.Println("  Empty lines are ignored")
			fmt.Println("  Each non-empty line is treated as a query")
			fmt.Println("\nFeatures:")
			fmt.Println("  - Simplified input (just type your message)")
			fmt.Println("  - Server handles stream-json format automatically")
			fmt.Println("  - Batch processing support")
			os.Exit(0)
		}
	}

	// Default: Run in interactive mode
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Printf("\n%sInterrupted. Goodbye!%s\n", ColorYellow, ColorReset)
		os.Exit(0)
	}()

	if err := client.Run(ctx); err != nil {
		log.Fatalf("Client error: %v", err)
	}
}
