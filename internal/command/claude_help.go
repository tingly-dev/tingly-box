package command

import (
	"fmt"
	"strings"
)

// PrintCCPassthroughHelp prints shared help about passing arguments to Claude
// Code via the '--' separator. Used by both the cc and profile commands.
//
// cmdName is "cc" or "profile" — drives command-specific examples and hints.
func PrintCCPassthroughHelp(cmdName string) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("PASSING ARGUMENTS TO CLAUDE")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()
	fmt.Println("To pass arguments to Claude Code, use '--' to separate them from")
	fmt.Println("tingly-box options. This is especially important when passing flags")
	fmt.Println("that might conflict (e.g., -p for Claude's print mode).")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  tingly-box %s -- --model opus              # Use opus model\n", cmdName)
	fmt.Printf("  tingly-box %s -- -p                        # Claude's print mode\n", cmdName)
	fmt.Printf("  tingly-box %s -- -p hi                     # Print mode with prompt\n", cmdName)
	if cmdName == "cc" {
		fmt.Println()
		fmt.Println("  tingly-box cc --profile ds -- --model opus # Profile + model")
	}
	fmt.Printf("  tingly-box %s -- --help                    # Show Claude CLI help\n", cmdName)
	fmt.Println()
	if cmdName == "cc" {
		fmt.Println("ALTERNATIVE: profile command")
		fmt.Println("  tingly-box profile <name> [--] [CLAUDE_ARGS...]")
		fmt.Println("  Example: tingly-box profile ds -- -p hi")
		fmt.Println()
		fmt.Println("PROFILES: tingly-box profile list | tingly-box profile show <name>")
	}
	fmt.Println()
}
