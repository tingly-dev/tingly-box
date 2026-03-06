package mitm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type Action int

const (
	Pass Action = iota
	Block
	Replace
	Stop // Signal to stop processing immediately
)

// CodecMode defines how messages are encoded/decoded
type CodecMode int

const (
	CodecJSON CodecMode = iota // JSON encoding (default)
	CodecText                  // Plain text, line-based
)

type IOContext struct {
	Msg any
}

// 当 Output 触发额外输入时返回这个
type OutputResult struct {
	Action Action
	NewMsg any

	// 额外要写入 child stdin 的消息
	InjectToChild []any
}

type InputHandler func(ctx context.Context, c *IOContext) (Action, any, error)
type OutputHandler func(ctx context.Context, c *IOContext) (*OutputResult, error)

// InputSource defines where input messages come from
type InputSource interface {
	Read(ctx context.Context) (any, error)
}

type Runner struct {
	cmd *exec.Cmd

	stdin  io.WriteCloser
	stdout io.ReadCloser

	// Codec mode for input/output
	Codec CodecMode

	InputSource  InputSource
	InputHandler InputHandler

	OutputHandler OutputHandler

	writeMu sync.Mutex
	wg      sync.WaitGroup
}

func New(
	cmd *exec.Cmd,
	inputHandler InputHandler,
	outputHandler OutputHandler,
) *Runner {
	return &Runner{
		cmd:           cmd,
		InputHandler:  inputHandler,
		OutputHandler: outputHandler,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	var err error

	r.stdin, err = r.cmd.StdinPipe()
	if err != nil {
		return err
	}

	r.stdout, err = r.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	r.cmd.Stderr = os.Stderr

	if err := r.cmd.Start(); err != nil {
		return err
	}

	r.wg.Add(2)

	go r.handleInput(ctx)
	go r.handleOutput(ctx)

	r.wg.Wait()

	return r.cmd.Wait()
}

func (r *Runner) SafeWrite(msg any) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	switch r.Codec {
	case CodecText:
		// Text mode: write as string with newline
		if s, ok := msg.(string); ok {
			r.stdin.Write([]byte(s + "\n"))
		}
	default:
		// JSON mode (default)
		encoder := json.NewEncoder(r.stdin)
		_ = encoder.Encode(msg)
	}
}

func (r *Runner) handleInput(ctx context.Context) {
	defer r.wg.Done()

	src := r.InputSource
	if src == nil {
		src = NewStdinSource()
	}

	for {
		msg, err := src.Read(ctx)
		if err != nil {
			return
		}

		if r.InputHandler == nil {
			r.SafeWrite(msg)
			continue
		}

		ioCtx := &IOContext{Msg: msg}

		action, newMsg, err := r.InputHandler(ctx, ioCtx)
		if err != nil {
			continue
		}

		switch action {
		case Pass:
			r.SafeWrite(msg)
		case Block:
			continue
		case Replace:
			r.SafeWrite(newMsg)
		}
	}
}

func (r *Runner) handleOutput(ctx context.Context) {
	defer r.wg.Done()

	switch r.Codec {
	case CodecText:
		r.handleOutputText(ctx)
	default:
		r.handleOutputJSON(ctx)
	}
}

func (r *Runner) handleOutputJSON(ctx context.Context) {
	decoder := json.NewDecoder(bufio.NewReader(r.stdout))
	encoder := json.NewEncoder(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var msg any
		if err := decoder.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			continue
		}

		if r.OutputHandler == nil {
			_ = encoder.Encode(msg)
			continue
		}

		ioCtx := &IOContext{Msg: msg}

		result, err := r.OutputHandler(ctx, ioCtx)
		if err != nil || result == nil {
			continue
		}

		// 1️⃣ 先处理 output 显示
		switch result.Action {
		case Pass:
			_ = encoder.Encode(msg)
		case Block:
		case Replace:
			_ = encoder.Encode(result.NewMsg)
		case Stop:
			// Stop processing immediately
			return
		}

		// 2️⃣ 再处理注入 child input
		for _, inject := range result.InjectToChild {
			r.SafeWrite(inject)
		}
	}
}

func (r *Runner) handleOutputText(ctx context.Context) {
	reader := bufio.NewReader(r.stdout)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			continue
		}

		// Remove trailing newline for handler
		msg := strings.TrimSuffix(line, "\n")

		if r.OutputHandler == nil {
			fmt.Print(line)
			continue
		}

		ioCtx := &IOContext{Msg: msg}

		result, err := r.OutputHandler(ctx, ioCtx)
		if err != nil || result == nil {
			continue
		}

		// Process output
		switch result.Action {
		case Pass:
			fmt.Println(msg)
		case Block:
		case Replace:
			if s, ok := result.NewMsg.(string); ok {
				fmt.Println(s)
			}
		case Stop:
			// Stop processing immediately
			return
		}

		// Inject to child input
		for _, inject := range result.InjectToChild {
			r.SafeWrite(inject)
		}
	}
}
