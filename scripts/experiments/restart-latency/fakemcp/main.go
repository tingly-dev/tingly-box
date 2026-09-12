// Command fakemcp is a minimal, fully-protocol-compliant MCP stdio server
// (built on the same github.com/modelcontextprotocol/go-sdk used by
// tingly-box itself) used only by the restart-latency experiment
// (../run.sh) to control one variable precisely: how the subprocess behaves
// when tingly-box tries to shut it down.
//
//   - FAKE_MCP_SLOW unset/"0": behaves like a well-mannered MCP server —
//     exits as soon as its stdin is closed (the "First, closing the input
//     stream" step of the stdio shutdown sequence in the MCP spec).
//   - FAKE_MCP_SLOW=1: ignores SIGTERM and never reacts to stdin closing,
//     simulating a hung/unresponsive MCP subprocess. Only SIGKILL ends it.
//
// It registers zero tools; the SDK server already answers "tools/list"
// with an empty list, which is all the readiness check in
// internal/mcp/runtime/source_stdio.go needs to consider the source
// connected.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	slow := os.Getenv("FAKE_MCP_SLOW") == "1"
	if slow {
		// Dropped at the kernel level — the process cannot "handle" its way
		// out of this even if it wanted to, same as a real wedged process
		// that stopped servicing its signal handlers.
		signal.Ignore(syscall.SIGTERM)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "fakemcp", Version: "0.0.1"}, nil)

	// Run returns once the client closes stdin (normal case) or the
	// connection otherwise breaks. A well-behaved server would let main()
	// return here, which is what the "fast" scenario does.
	_ = server.Run(context.Background(), &mcp.StdioTransport{})

	if slow {
		// Simulate a subprocess that doesn't go away just because its input
		// stream closed — the parent's only remaining option is SIGKILL.
		select {}
	}
}
