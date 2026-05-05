package main

import "fmt"

// ProviderCmd groups the real-provider e2e subcommands (Phase 3 — not yet
// implemented). The leaves return a not-implemented error until Phase 3 work
// lands.
type ProviderCmd struct {
	Test ProviderTestCmd `kong:"cmd,help='Test against a real provider API'"`
	List ProviderListCmd `kong:"cmd,help='List available test providers'"`
}

// Help returns the long usage for `harness provider --help`.
func (*ProviderCmd) Help() string {
	return `Test against real provider APIs for live compatibility.

This subcommand is planned for Phase 3 implementation.

Features:
  - Test against real provider APIs
  - Accept provider credentials via flags/config
  - Validate live API compatibility

See specification in .sdlc/docs/cli-harness-spec-20260413.spec.md`
}

// ProviderTestCmd runs protocol transformations against real provider APIs.
type ProviderTestCmd struct {
	Provider  string   `kong:"arg,optional,help='Provider name or UUID'"`
	Scenarios []string `kong:"name='scenario',sep=',',help='Test scenarios (can repeat or comma-separate)'"`
}

func (*ProviderTestCmd) Run() error {
	return fmt.Errorf("provider test not yet implemented - planned for Phase 3")
}

// ProviderListCmd lists provider configurations available for testing.
type ProviderListCmd struct{}

func (*ProviderListCmd) Run() error {
	return fmt.Errorf("provider list not yet implemented - planned for Phase 3")
}
