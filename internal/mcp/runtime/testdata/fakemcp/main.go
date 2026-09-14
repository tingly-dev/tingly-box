//go:build !windows

// Command fakemcp is a minimal, protocol-compliant MCP stdio server (built
// on the same github.com/modelcontextprotocol/go-sdk used by tingly-box
// itself) used only by the Close() regression test in this package to
// control one variable precisely: how the subprocess behaves when
// tingly-box tries to shut it down.
//
// With FAKE_MCP_SLOW=1 it ignores SIGTERM and never reacts to stdin
// closing, simulating a hung/unresponsive MCP subprocess that only SIGKILL
// can end — reproducing the shape of subprocess a real, misbehaving MCP
// server could leave behind.
//
// It registers zero tools; the SDK server already answers "tools/list"
// with an empty list, which is all StdioToolSource's readiness check needs
// to consider the source connected.
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
	// connection otherwise breaks.
	_ = server.Run(context.Background(), &mcp.StdioTransport{})

	if slow {
		// Simulate a subprocess that doesn't go away just because its input
		// stream closed — the parent's only remaining option is SIGKILL.
		select {}
	}
}
