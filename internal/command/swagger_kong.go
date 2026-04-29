//go:build !legacy

package command

import (
	"fmt"
	"os"

	"github.com/tingly-dev/tingly-box/internal/server"
)

// SwaggerCmdKong generates OpenAPI schema
type SwaggerCmdKong struct {
	Output string `kong:"flag,name='output',short='o',help='Output file path'"`
	Stdout bool   `kong:"flag,name='stdout',help='Write to stdout'"`
}

func (s *SwaggerCmdKong) Run(appManager *AppManager) error {
	return runSwagger(appManager, s.Output, s.Stdout)
}

// runSwagger extracts swagger logic from SwaggerCommand
func runSwagger(appManager *AppManager, output string, stdout bool) error {
	cfg := appManager.GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("config not available")
	}

	json, err := server.GenerateOpenAPI(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate OpenAPI schema: %w", err)
	}

	if stdout {
		fmt.Println(json)
	} else {
		if output == "" {
			output = "openapi.json"
		}
		if err := os.WriteFile(output, []byte(json), 0644); err != nil {
			return fmt.Errorf("failed to write to file %s: %w", output, err)
		}
		fmt.Fprintf(os.Stderr, "OpenAPI schema written to: %s\n", output)
	}

	return nil
}
