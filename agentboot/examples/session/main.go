package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tingly-dev/tingly-box/agentboot/session/claude"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: session <project-path> [limit]")
		os.Exit(1)
	}

	projectPath := os.Args[1]
	limit := 10 // Default limit

	// Parse optional limit argument
	if len(os.Args) >= 3 {
		var parsedLimit int
		_, err := fmt.Sscanf(os.Args[2], "%d", &parsedLimit)
		if err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Create Claude session store
	store, err := claude.NewStore("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session store: %v\n", err)
		os.Exit(1)
	}

	// Get recent sessions
	ctx := context.Background()
	sessions, err := store.GetRecentSessions(ctx, projectPath, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
		os.Exit(1)
	}

	// Output as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(sessions); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding output: %v\n", err)
		os.Exit(1)
	}
}

