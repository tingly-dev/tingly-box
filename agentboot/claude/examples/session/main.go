package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	ccsession "github.com/tingly-dev/tingly-box/agentboot/claude/session"
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
	store, err := ccsession.NewStore("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session store: %v\n", err)
		os.Exit(1)
	}

	// Use default filter to exclude meta-only and empty sessions
	filter := ccsession.DefaultSessionFilter()

	// Get recent sessions with filter applied
	ctx := context.Background()
	sessions, err := store.GetRecentSessionsFiltered(ctx, projectPath, limit, filter)
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
