package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai/quota/fetcher"

	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ============== Kong Command Structures ==============

// QuotaCmdKong is the streamlined quota command.
// Default behavior: interactive mode to select provider.
type QuotaCmdKong struct {
	NoRefresh    bool   `kong:"flag,name='no-refresh',help='Skip refresh and show cached data only'"`
	AllProviders bool   `kong:"flag,name='all',short='a',help='Show all providers instead of interactive mode'"`
	Provider     string `kong:"arg,optional,help='Provider name or UUID (interactive mode if omitted)'"`
}

func (q *QuotaCmdKong) Run(appManager *AppManager) error {
	// If provider specified, show only that provider
	if q.Provider != "" {
		return runQuotaShowProvider(appManager, q.Provider, !q.NoRefresh)
	}
	// No provider specified - check if user wants all providers or interactive
	if q.AllProviders {
		return runQuotaShowAll(appManager, !q.NoRefresh)
	}
	// Default: interactive mode
	return runQuotaInteractive(appManager, !q.NoRefresh)
}

// ============== Business Logic Functions ==============

// runQuotaShowAll shows all providers with optional refresh
func runQuotaShowAll(appManager *AppManager, refresh bool) error {
	ctx := context.Background()

	qm, err := createQuotaManager(appManager)
	if err != nil {
		return err
	}

	// Refresh if requested (default behavior)
	if refresh {
		fmt.Println("🔄 Refreshing quota data...")
		_, err := qm.Refresh(ctx)
		if err != nil {
			fmt.Printf("⚠️  Refresh failed: %v\n", err)
		} else {
			fmt.Println("✅ Refresh complete")
		}
	}

	usages, err := qm.ListQuota(ctx)
	if err != nil {
		return fmt.Errorf("failed to get quota data: %w", err)
	}

	providers := appManager.ListProviders()
	if len(providers) == 0 {
		fmt.Println("No providers configured.")
		return nil
	}

	return displayQuotaForProviders(providers, usages)
}

// runQuotaShowProvider shows a specific provider with optional refresh
func runQuotaShowProvider(appManager *AppManager, providerName string, refresh bool) error {
	ctx := context.Background()

	// Find provider by name or UUID
	provider, err := findProvider(appManager, providerName)
	if err != nil {
		return err
	}

	qm, err := createQuotaManager(appManager)
	if err != nil {
		return err
	}

	// Refresh if requested (default behavior)
	if refresh {
		fmt.Printf("🔄 Refreshing quota data for %s...\n", providerName)
		_, err := qm.RefreshProvider(ctx, provider.UUID)
		if err != nil {
			fmt.Printf("⚠️  Refresh failed: %v\n", err)
		} else {
			fmt.Println("✅ Refresh complete")
		}
	}

	usage, err := qm.GetQuota(ctx, provider.UUID)
	if err != nil {
		return fmt.Errorf("failed to get quota: %w", err)
	}

	printQuotaDetails(usage)
	return nil
}

// runQuotaInteractive runs interactive mode for provider selection
func runQuotaInteractive(appManager *AppManager, refresh bool) error {
	providers := appManager.ListProviders()

	if len(providers) == 0 {
		fmt.Println("❌ No providers configured.")
		return nil
	}

	if len(providers) == 1 {
		// Only one provider, auto-select it
		return runQuotaShowProvider(appManager, providers[0].Name, refresh)
	}

	fmt.Println("\n📊 View Provider Quota")
	fmt.Println("\nSelect a provider:")

	for i, provider := range providers {
		status := "✅"
		if !provider.Enabled {
			status = "❌"
		}
		fmt.Printf("%d. %s %s (%s)\n", i+1, status, provider.Name, provider.UUID[:8])
	}

	fmt.Print("\nEnter provider number, name, or UUID: ")
	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(input)

	var name string
	var num int
	if _, err := fmt.Sscanf(input, "%d", &num); err == nil && num > 0 && num <= len(providers) {
		name = providers[num-1].Name
	} else {
		// Try as name or UUID directly
		name = input
	}

	return runQuotaShowProvider(appManager, name, refresh)
}

// findProvider finds a provider by name or UUID (exact match).
func findProvider(appManager *AppManager, nameOrUUID string) (*typ.Provider, error) {
	providers := appManager.ListProviders()
	for _, p := range providers {
		if p.Name == nameOrUUID || p.UUID == nameOrUUID {
			return p, nil
		}
	}
	return nil, fmt.Errorf("provider not found: %s", nameOrUUID)
}

// createQuotaManager creates a quota manager for CLI use
func createQuotaManager(appManager *AppManager) (*quota.Manager, error) {
	// Create quota store
	config := appManager.AppConfig()
	store, err := quota.NewGormStore(config.ConfigDir(), logrus.StandardLogger())
	if err != nil {
		return nil, fmt.Errorf("failed to create quota store: %w", err)
	}

	// Create quota manager with default config
	qConfig := quota.DefaultConfig()
	qm := quota.NewManager(qConfig, store, appManager, logrus.StandardLogger())

	// Register all built-in fetchers
	fetcher.RegisterAll(qm, logrus.StandardLogger())

	return qm, nil
}

// displayQuotaForProviders displays quota info for given providers
func displayQuotaForProviders(providers []*typ.Provider, usages []*quota.ProviderUsage) error {
	// Build a lookup from store data
	usageMap := make(map[string]*quota.ProviderUsage, len(usages))
	for _, u := range usages {
		usageMap[u.ProviderUUID] = u
	}

	for i, p := range providers {
		if u, ok := usageMap[p.UUID]; ok {
			printQuotaDetails(u)
		} else {
			// No data yet — show provider with "not fetched" status
			printQuotaDetails(&quota.ProviderUsage{
				ProviderUUID: p.UUID,
				ProviderName: p.Name,
				ProviderType: quota.ProviderType(p.APIStyle),
				LastError:    "no data — run 'quota' to fetch",
			})
		}

		// Empty line between providers (except after last one)
		if i < len(providers)-1 {
			fmt.Println()
		}
	}

	return nil
}

// printQuotaDetails prints detailed quota information (minimal style)
func printQuotaDetails(usage *quota.ProviderUsage) {
	// Provider header
	providerName := usage.ProviderName
	if len(providerName) > 40 {
		// Truncate long provider names
		providerName = providerName[:37] + "..."
	}

	fmt.Println("─────────────────────────────────────────────────────────────")
	fmt.Printf("📊 %s (%s)\n", providerName, usage.ProviderType)

	// Account tier on separate line
	if usage.Account != nil && usage.Account.Tier != "" {
		fmt.Printf("Tier: %s\n", usage.Account.Tier)
	} else if usage.Account != nil && usage.Account.Name != "" {
		fmt.Printf("Account: %s\n", usage.Account.Name)
	}

	// Fetched time on separate line
	fmt.Printf("Fetched: %s\n", usage.FetchedAt.Format("2006-01-02 15:04:05"))

	// Status
	if usage.LastError != "" {
		fmt.Printf("Status: Error: %s\n", usage.LastError)
	}

	fmt.Println()

	// Display quota windows with progress bars
	displayWindowsWithProgress(usage)

	// Footer status
	if usage.LastError == "" {
		fmt.Printf("\n✅ Status: OK\n")
	}

	fmt.Println("─────────────────────────────────────────────────────────────")
}

// displayWindowsWithProgress displays quota windows with visual progress bars
func displayWindowsWithProgress(usage *quota.ProviderUsage) {
	usage.NormalizeWindows()
	windows := usage.Windows

	if len(windows) == 0 {
		fmt.Println("No quota data available")
		return
	}

	for _, window := range windows {
		printWindowWithProgress(window)
	}
}

// printWindowWithProgress prints a single usage window with progress bar
func printWindowWithProgress(window *quota.UsageWindow) {
	// Status icon based on usage percentage
	statusIcon := getStatusIcon(window.UsedPercent)

	// Format usage values
	var usageStr string
	if window.Used == 0 && window.Limit == 0 && window.Unit == quota.UsageUnitPercent {
		// Percentage-only quota
		usageStr = fmt.Sprintf("%.1f%%", window.UsedPercent)
	} else {
		usedStr := formatUsageValue(window.Used, window.Unit)
		limitStr := formatUsageValue(window.Limit, window.Unit)
		usageStr = fmt.Sprintf("%s / %s (%.1f%%)", usedStr, limitStr, window.UsedPercent)
	}

	// Progress bar
	progressBar := renderProgressBar(window.UsedPercent, 20)

	// Reset time
	resetInfo := ""
	if window.ResetsAt != nil {
		resetInfo = fmt.Sprintf(" — %s", formatResetTime(*window.ResetsAt))
	}

	// Print line: [icon] Label: usage [progress_bar] reset_info
	fmt.Printf("%s %s: %s [%s]%s\n", statusIcon, window.Label, usageStr, progressBar, resetInfo)
}

// renderProgressBar creates a visual progress bar
func renderProgressBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}

	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// getStatusIcon returns an icon based on usage percentage
func getStatusIcon(percent float64) string {
	switch {
	case percent >= 90:
		return "🔴" // Critical
	case percent >= 70:
		return "🟠" // Warning
	case percent >= 40:
		return "🟡" // Moderate
	default:
		return "🟢" // Good
	}
}

// formatResetTime formats reset time in compact form
func formatResetTime(resetsAt time.Time) string {
	duration := time.Until(resetsAt)

	// Already expired
	if duration <= 0 {
		return fmt.Sprintf("expired %s", resetsAt.Format("Jan 2 15:04"))
	}

	// Less than 1 hour
	if duration < time.Hour {
		return fmt.Sprintf("resets in %dm", int(duration.Minutes()))
	}

	// Less than 24 hours
	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		mins := int(duration.Minutes()) % 60
		if mins > 0 {
			return fmt.Sprintf("resets in %dh %dm", hours, mins)
		}
		return fmt.Sprintf("resets in %dh", hours)
	}

	// Less than 7 days
	if duration < 7*24*time.Hour {
		days := int(duration.Hours() / 24)
		hours := int(duration.Hours()) % 24
		if hours > 0 {
			return fmt.Sprintf("resets in %dd %dh", days, hours)
		}
		return fmt.Sprintf("resets in %dd", days)
	}

	// More than a week - show date
	return fmt.Sprintf("resets %s", resetsAt.Format("Jan 2 15:04"))
}

// printUsageWindow prints a usage window
func printUsageWindow(w *quota.UsageWindow, indent int) {
	prefix := strings.Repeat("  ", indent)
	fmt.Printf("%sLabel:     %s\n", prefix, w.Label)
	fmt.Printf("%sType:      %s\n", prefix, w.Type)

	// For percentage-only quotas, show only percentage
	if w.Used == 0 && w.Limit == 0 && w.Unit == quota.UsageUnitPercent {
		fmt.Printf("%sUtilization: %.1f%%\n", prefix, w.UsedPercent)
	} else {
		fmt.Printf("%sUsed:      %s", prefix, formatUsageValue(w.Used, w.Unit))
		fmt.Printf(" / %s\n", formatUsageValue(w.Limit, w.Unit))
		fmt.Printf("%sPercent:   %.1f%%\n", prefix, w.UsedPercent)
	}

	if w.ResetsAt != nil {
		if time.Until(*w.ResetsAt) > 0 {
			fmt.Printf("%sResets in: %s\n", prefix, formatDuration(time.Until(*w.ResetsAt)))
		} else {
			fmt.Printf("%sResets at: %s\n", prefix, w.ResetsAt.Format("2006-01-02 15:04"))
		}
	}
}

// printUsageWindowInline prints a usage window on a single line (for breakdowns)
func printUsageWindowInline(w *quota.UsageWindow, indent int) {
	prefix := strings.Repeat("  ", indent)

	// For percentage-only quotas (Used=0, Limit=0), show only percentage
	if w.Used == 0 && w.Limit == 0 && w.Unit == quota.UsageUnitPercent {
		fmt.Printf("%s%s: %.1f%%", prefix, w.Label, w.UsedPercent)
	} else {
		fmt.Printf("%s%s: %s / %s (%.1f%%)", prefix, w.Label, formatUsageValue(w.Used, w.Unit), formatUsageValue(w.Limit, w.Unit), w.UsedPercent)
	}

	if w.ResetsAt != nil {
		if time.Until(*w.ResetsAt) > 0 {
			fmt.Printf(" — resets in %s", formatDuration(time.Until(*w.ResetsAt)))
		}
	}
	fmt.Println()
}

// formatUsageValue formats a usage value with appropriate unit
func formatUsageValue(value float64, unit quota.UsageUnit) string {
	switch unit {
	case quota.UsageUnitTokens:
		if value >= 1000000 {
			return fmt.Sprintf("%.1fM", value/1000000)
		} else if value >= 1000 {
			return fmt.Sprintf("%.1fK", value/1000)
		}
		return fmt.Sprintf("%.0f", value)
	case quota.UsageUnitRequests:
		return fmt.Sprintf("%.0f", value)
	case quota.UsageUnitCurrency:
		return fmt.Sprintf("$%.2f", value)
	case quota.UsageUnitPercent:
		return fmt.Sprintf("%.1f%%", value)
	case quota.UsageUnitCredits:
		if value >= 1000000 {
			return fmt.Sprintf("%.1fM", value/1000000)
		} else if value >= 1000 {
			return fmt.Sprintf("%.1fK", value/1000)
		}
		return fmt.Sprintf("%.0f", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", hours, mins)
	} else {
		days := int(d.Hours() / 24)
		hours := int(d.Hours()) % 24
		return fmt.Sprintf("%dd %dh", days, hours)
	}
}
