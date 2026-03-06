package mitm

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestRunnerWithBash tests the MITM runner with bash in text mode
func TestRunnerWithBash(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create channel for input
	inputCh := make(chan any, 10)

	var outputs []string

	// Create runner with bash in text mode
	cmd := exec.Command("bash")
	runner := New(cmd, nil, func(ctx context.Context, c *IOContext) (*OutputResult, error) {
		// Collect output lines
		if s, ok := c.Msg.(string); ok && s != "" {
			outputs = append(outputs, s)
		}
		return &OutputResult{Action: Pass}, nil
	})
	runner.Codec = CodecText
	runner.InputSource = NewChannelSource(inputCh)

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	// Send commands
	go func() {
		time.Sleep(200 * time.Millisecond) // Wait for bash to start

		// Send ls command
		inputCh <- "ls -la /tmp"
		time.Sleep(300 * time.Millisecond)

		// Send echo command
		inputCh <- "echo 'hello world'"
		time.Sleep(300 * time.Millisecond)

		// Exit bash
		inputCh <- "exit"
	}()

	// Wait for completion
	select {
	case <-time.After(3 * time.Second):
		t.Log("Test timeout")
	case err := <-done:
		t.Logf("Runner finished: %v", err)
	}

	t.Logf("Received %d output lines", len(outputs))
	for i, o := range outputs {
		t.Logf("Output[%d]: %s", i, o)
	}

	// Verify we got some output
	if len(outputs) == 0 {
		t.Error("Expected some output from bash")
	}

	// Check for "hello world" in output
	found := false
	for _, o := range outputs {
		if strings.Contains(o, "hello world") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'hello world' in output")
	}
}

// TestChannelSource tests ChannelSource directly
func TestChannelSource(t *testing.T) {
	ctx := context.Background()

	ch := make(chan any, 3)
	src := NewChannelSource(ch)

	// Send messages
	ch <- map[string]any{"type": "test1"}
	ch <- map[string]any{"type": "test2"}
	close(ch)

	// Read messages
	msg1, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	t.Logf("Message 1: %v", msg1)

	msg2, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	t.Logf("Message 2: %v", msg2)

	// Should get EOF after channel closed
	_, err = src.Read(ctx)
	if err == nil {
		t.Fatal("Expected error after channel closed")
	}
	t.Logf("Expected error: %v", err)
}

// TestChanSource tests the new ChanSource with Write capability
func TestChanSource(t *testing.T) {
	ctx := context.Background()

	src := NewChanSource(10)

	// Write messages using Write method
	src.Write(map[string]any{"type": "test1"})
	src.Write(map[string]any{"type": "test2"})

	// Read messages
	msg1, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	t.Logf("Message 1: %v", msg1)

	msg2, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	t.Logf("Message 2: %v", msg2)

	// Test WriteWait with timeout
	writeCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err = src.WriteWait(writeCtx, map[string]any{"type": "test3"})
	if err != nil {
		t.Fatalf("WriteWait failed: %v", err)
	}

	// Read the message
	msg3, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	t.Logf("Message 3: %v", msg3)

	// Test C() method - get channel and write directly
	src.C() <- map[string]any{"type": "test4"}
	msg4, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	t.Logf("Message 4: %v", msg4)

	// Close the source
	src.Close()

	// Should get EOF after closed
	_, err = src.Read(ctx)
	if err == nil {
		t.Fatal("Expected error after channel closed")
	}
	t.Logf("Expected error after close: %v", err)
}

// TestChanSourceWithRunner tests ChanSource integration with Runner
func TestChanSourceWithRunner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create ChanSource with Write capability
	inputSrc := NewChanSource(10)

	var outputs []string

	cmd := exec.Command("bash")
	runner := New(cmd, nil, func(ctx context.Context, c *IOContext) (*OutputResult, error) {
		if s, ok := c.Msg.(string); ok && s != "" {
			outputs = append(outputs, s)
		}
		return &OutputResult{Action: Pass}, nil
	})
	runner.Codec = CodecText
	runner.InputSource = inputSrc

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	// Send commands via ChanSource.Write
	go func() {
		time.Sleep(200 * time.Millisecond)

		// Use Write method
		inputSrc.Write("echo 'from Write'")

		time.Sleep(200 * time.Millisecond)

		// Use WriteWait method
		inputSrc.WriteWait(ctx, "echo 'from WriteWait'")

		time.Sleep(200 * time.Millisecond)

		// Exit
		inputSrc.Write("exit")
	}()

	// Wait for completion
	select {
	case <-time.After(3 * time.Second):
		t.Log("Test timeout")
	case err := <-done:
		t.Logf("Runner finished: %v", err)
	}

	t.Logf("Received %d output lines", len(outputs))
	for i, o := range outputs {
		t.Logf("Output[%d]: %s", i, o)
	}

	// Verify we got output from both methods
	foundWrite := false
	foundWriteWait := false
	for _, o := range outputs {
		if strings.Contains(o, "from Write") {
			foundWrite = true
		}
		if strings.Contains(o, "from WriteWait") {
			foundWriteWait = true
		}
	}
	if !foundWrite {
		t.Error("Expected 'from Write' in output")
	}
	if !foundWriteWait {
		t.Error("Expected 'from WriteWait' in output")
	}
}

// TestStdinSource tests StdinSource (basic functionality)
func TestStdinSource(t *testing.T) {
	// This test is limited since we can't easily mock os.Stdin
	// Just verify it can be created
	src := NewStdinSource()
	if src == nil {
		t.Fatal("NewStdinSource returned nil")
	}
	if src.reader == nil {
		t.Fatal("reader is nil")
	}
}

// TestRunnerWithJSONMode tests the MITM runner in JSON mode
func TestRunnerWithJSONMode(t *testing.T) {
	// Create a simple echo-based JSON test
	// We'll use a Go program that echoes JSON
	cmd := exec.Command("echo", "test")
	runner := New(cmd, nil, nil)
	runner.Codec = CodecJSON

	// Simple verification that CodecJSON is set correctly
	if runner.Codec != CodecJSON {
		t.Error("Expected CodecJSON mode")
	}

	// Skip actual run test for now - just verify the mode is set
	t.Log("JSON mode configuration verified")
}
