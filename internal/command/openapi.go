package command

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/server"
)

// SwaggerCommand creates the swagger command
func SwaggerCommand(appManager *AppManager) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "swagger",
		Short: "Generate OpenAPI v3 schema (swagger)",
		Long: `Generate OpenAPI v3 JSON schema without starting the server.

The schema includes all API routes registered by the server and can be used
for documentation, client code generation, and API testing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			output, _ := cmd.Flags().GetString("output")
			toStdout, _ := cmd.Flags().GetBool("stdout")

			// Get config from appManager
			cfg := appManager.GetGlobalConfig()
			if cfg == nil {
				return fmt.Errorf("config not available")
			}

			// Generate OpenAPI schema
			json, err := server.GenerateOpenAPI(cfg)
			if err != nil {
				return fmt.Errorf("failed to generate OpenAPI schema: %w", err)
			}

			// Write output
			if toStdout {
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
		},
	}

	cmd.Flags().StringP("output", "o", "", "Output file path (default: openapi.json)")
	cmd.Flags().Bool("stdout", false, "Write to stdout instead of file")

	return cmd
}
