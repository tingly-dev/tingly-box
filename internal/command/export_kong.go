//go:build !legacy

package command

// ExportCmdKong exports routing rules
type ExportCmdKong struct {
	RequestModel string `kong:"flag,name='request-model',required,help='Request model name'"`
	Scenario     string `kong:"flag,name='scenario',required,help='Rule scenario'"`
	Format       string `kong:"flag,name='format',default='jsonl',help='Export format (jsonl, base64)'"`
	Output       string `kong:"flag,name='output',short='o',help='Output file path'"`
}

func (e *ExportCmdKong) Run(appManager *AppManager) error {
	return runExport(appManager, e.RequestModel, e.Scenario, e.Format, e.Output)
}
