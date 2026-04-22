//go:build kong

package command

// MCPCmdKong manages MCP builtin server
type MCPCmdKong struct {
	Builtin MCPBuiltinCmdKong `kong:"cmd,help='MCP builtin server'"`
}

// MCPBuiltinCmdKong runs MCP builtin server
type MCPBuiltinCmdKong struct{}

func (m *MCPBuiltinCmdKong) Run(appManager *AppManager) error {
	cmd := MCPBuiltinCommand()
	return cmd.Execute()
}
