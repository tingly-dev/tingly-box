//go:build kong

package command

import (
	"github.com/spf13/cobra"
)

// ExportCmdKong exports routing rules
type ExportCmdKong struct {
	RequestModel string `kong:"flag,name='request-model',required,help='Request model name'"`
	Scenario     string `kong:"flag,name='scenario',required,help='Rule scenario'"`
	Format       string `kong:"flag,name='format',default='jsonl',help='Export format (jsonl, base64)'"`
	Output       string `kong:"flag,name='output',short='o',help='Output file path'"`
}

func (e *ExportCmdKong) Run(appManager *AppManager) error {
	// Create a mock cobra command for the existing runExport function
	mockCmd := &cobra.Command{}
	if err := mockCmd.Flags().Set("request-model", e.RequestModel); err != nil {
		return err
	}
	if err := mockCmd.Flags().Set("scenario", e.Scenario); err != nil {
		return err
	}
	if err := mockCmd.Flags().Set("format", e.Format); err != nil {
		return err
	}
	if e.Output != "" {
		if err := mockCmd.Flags().Set("output", e.Output); err != nil {
			return err
		}
	}
	return runExport(appManager, mockCmd)
}
