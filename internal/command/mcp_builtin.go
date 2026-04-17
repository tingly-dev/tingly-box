package command

import (
	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/mcp/builtin_server"
)

// MCPBuiltinCommand creates the cobra command for the builtin MCP server.
func MCPBuiltinCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "mcp-builtin",
		Short:  "Start the builtin MCP server (internal use)",
		Hidden: true, // Hide from help as it's for internal use
		RunE: func(cmd *cobra.Command, args []string) error {
			// Start the builtin MCP server
			return builtinserver.Serve()
		},
	}

	return cmd
}