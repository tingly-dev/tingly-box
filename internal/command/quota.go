package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/quota/fetcher"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// QuotaCommand represents the quota management command
func QuotaCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quota [command]",
		Short: "Manage provider quota/usage information",
		Long: `Manage and view provider quota/usage information.

Displays current token usage, limits, and costs for configured AI providers.
Supports both listing all providers and viewing specific provider details.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// No subcommand provided, default to list
			return runQuotaList(appManager)
		},
	}

	// Add subcommands
	cmd.AddCommand(quotaListCommand(appManager))
	cmd.AddCommand(quotaGetCommand(appManager))
	cmd.AddCommand(quotaRefreshCommand(appManager))
	cmd.AddCommand(quotaSummaryCommand(appManager))

	return cmd
}

// quotaListCommand creates the list subcommand
func quotaListCommand(appManager *AppManager) *cobra.Command {
	var refresh bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all provider quotas",
		Long:  "Display quota/usage information for all configured providers.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if refresh {
				return runQuotaRefresh(appManager)
			}
			return runQuotaList(appManager)
		},
	}

	cmd.Flags().BoolVarP(&refresh, "refresh", "r", false, "Refresh quota data before listing")

	return cmd
}

// quotaGetCommand creates the get subcommand
func quotaGetCommand(appManager *AppManager) *cobra.Command {
	var refresh bool

	cmd := &cobra.Command{
		Use:   "get [provider]",
		Short: "Get provider quota details",
		Long: `Display detailed quota/usage information for a specific provider.

If no provider name is given, enters interactive mode to select one.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runQuotaGetInteractive(appManager)
			}
			return runQuotaGet(appManager, args[0], refresh)
		},
	}

	cmd.Flags().BoolVarP(&refresh, "refresh", "r", false, "Refresh quota data before displaying")

	return cmd
}

// quotaRefreshCommand creates the refresh subcommand
func quotaRefreshCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh [provider]",
		Short: "Refresh provider quota data",
		Long: `Fetch fresh quota/usage data from provider APIs.

If no provider name is given, refreshes all providers.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runQuotaRefresh(appManager)
			}
			return runQuotaRefreshProvider(appManager, args[0])
		},
	}

	return cmd
}

// quotaSummaryCommand creates the summary subcommand
func quotaSummaryCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Show quota summary",
		Long:  "Display a summary of quota usage across all providers.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuotaSummary(appManager)
		},
	}

	return cmd
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

	// Register fetchers
	qm.RegisterFetcher(fetcher.NewAnthropicFetcher(logrus.StandardLogger()))
	qm.RegisterFetcher(fetcher.NewOpenAIFetcher(logrus.StandardLogger()))
	qm.RegisterFetcher(fetcher.NewGeminiFetcher(logrus.StandardLogger()))
	qm.RegisterFetcher(fetcher.NewCursorFetcher(logrus.StandardLogger()))
	qm.RegisterFetcher(fetcher.NewCopilotFetcher(logrus.StandardLogger()))
	qm.RegisterFetcher(fetcher.NewVertexAIFetcher(logrus.StandardLogger()))
	qm.RegisterFetcher(fetcher.NewZaiFetcher(logrus.StandardLogger()))
	qm.RegisterFetcher(fetcher.NewKimiK2Fetcher(logrus.StandardLogger()))
	qm.RegisterFetcher(fetcher.NewOpenRouterFetcher(logrus.StandardLogger()))
	qm.RegisterFetcher(fetcher.NewMiniMaxFetcher(logrus.StandardLogger()))
	qm.RegisterFetcher(fetcher.NewCodexFetcher(logrus.StandardLogger()))

	return qm, nil
}

// runQuotaList lists all provider quotas
func runQuotaList(appManager *AppManager) error {
	ctx := context.Background()

	qm, err := createQuotaManager(appManager)
	if err != nil {
		return err
	}

	usages, err := qm.ListQuota(ctx)
	if err != nil {
		return fmt.Errorf("failed to get quota data: %w", err)
	}

	if len(usages) == 0 {
		fmt.Println("📊 No quota data available.")
		fmt.Println("\nTip: Use 'quota refresh' to fetch quota data from providers.")
		return nil
	}

	fmt.Println("\n📊 Provider Quota Overview")
	fmt.Println(strings.Repeat("=", 80))

	for _, usage := range usages {
		printQuotaOverview(usage)
		fmt.Println(strings.Repeat("-", 80))
	}

	return nil
}

// runQuotaGet displays detailed quota for a specific provider
func runQuotaGet(appManager *AppManager, providerName string, refresh bool) error {
	ctx := context.Background()

	// Find provider by name
	providers := appManager.ListProviders()
	var provider *typ.Provider
	for _, p := range providers {
		if p.Name == providerName {
			provider = p
			break
		}
	}

	if provider == nil {
		return fmt.Errorf("provider not found: %s", providerName)
	}

	qm, err := createQuotaManager(appManager)
	if err != nil {
		return err
	}

	// Refresh if requested
	if refresh {
		fmt.Printf("🔄 Refreshing quota data for %s...\n", providerName)
		_, err := qm.RefreshProvider(ctx, provider.UUID)
		if err != nil {
			fmt.Printf("⚠️  Refresh failed: %v\n", err)
		} else {
			fmt.Println("✅ Refresh complete\n")
		}
	}

	// Get quota data
	usage, err := qm.GetQuota(ctx, provider.UUID)
	if err != nil {
		return fmt.Errorf("failed to get quota: %w", err)
	}

	printQuotaDetails(usage)
	return nil
}

// runQuotaGetInteractive runs interactive mode for get
func runQuotaGetInteractive(appManager *AppManager) error {
	providers := appManager.ListProviders()

	if len(providers) == 0 {
		fmt.Println("❌ No providers configured.")
		return nil
	}

	fmt.Println("\n📊 View Provider Quota")
	fmt.Println("\nSelect a provider:")

	for i, provider := range providers {
		status := "✅"
		if !provider.Enabled {
			status = "❌"
		}
		fmt.Printf("%d. %s %s\n", i+1, status, provider.Name)
	}

	fmt.Print("\nEnter provider number or name: ")
	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(input)

	var name string
	var num int
	if _, err := fmt.Sscanf(input, "%d", &num); err == nil && num > 0 && num <= len(providers) {
		name = providers[num-1].Name
	} else {
		name = input
	}

	return runQuotaGet(appManager, name, false)
}

// runQuotaRefresh refreshes all provider quotas
func runQuotaRefresh(appManager *AppManager) error {
	ctx := context.Background()

	qm, err := createQuotaManager(appManager)
	if err != nil {
		return err
	}

	fmt.Println("🔄 Refreshing quota data for all providers...")

	usages, err := qm.Refresh(ctx)
	if err != nil {
		return fmt.Errorf("failed to refresh quota: %w", err)
	}

	fmt.Printf("✅ Refreshed %d provider(s)\n\n", len(usages))

	for _, usage := range usages {
		printQuotaOverview(usage)
		fmt.Println()
	}

	return nil
}

// runQuotaRefreshProvider refreshes a specific provider
func runQuotaRefreshProvider(appManager *AppManager, providerName string) error {
	ctx := context.Background()

	// Find provider by name
	providers := appManager.ListProviders()
	var provider *typ.Provider
	for _, p := range providers {
		if p.Name == providerName {
			provider = p
			break
		}
	}

	if provider == nil {
		return fmt.Errorf("provider not found: %s", providerName)
	}

	qm, err := createQuotaManager(appManager)
	if err != nil {
		return err
	}

	fmt.Printf("🔄 Refreshing quota data for %s...\n", providerName)

	usage, err := qm.RefreshProvider(ctx, provider.UUID)
	if err != nil {
		return fmt.Errorf("failed to refresh quota: %w", err)
	}

	fmt.Println("✅ Refresh complete\n")
	printQuotaDetails(usage)

	return nil
}

// runQuotaSummary displays quota summary
func runQuotaSummary(appManager *AppManager) error {
	ctx := context.Background()

	qm, err := createQuotaManager(appManager)
	if err != nil {
		return err
	}

	summary, err := qm.Summary(ctx)
	if err != nil {
		return fmt.Errorf("failed to get summary: %w", err)
	}

	fmt.Println("\n📊 Quota Summary")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Total Providers:   %d\n", summary.TotalProviders)
	fmt.Printf("✅ OK:              %d\n", summary.OKProviders)
	fmt.Printf("❌ Error:           %d\n", summary.ErrorProviders)
	fmt.Printf("⚠️  Warning (>80%%):  %d\n", summary.WarningProviders)
	fmt.Println(strings.Repeat("=", 50))

	if len(summary.ByType) > 0 {
		fmt.Println("\nBy Provider Type:")
		for pt, count := range summary.ByType {
			fmt.Printf("  %s: %d\n", pt, count)
		}
	}

	return nil
}

// printQuotaOverview prints a one-line overview of provider quota
func printQuotaOverview(usage *quota.ProviderUsage) {
	status := "✅"
	if usage.LastError != "" {
		status = "❌"
	}

	// Get primary window info
	primaryInfo := ""
	if usage.Primary != nil {
		primaryInfo = fmt.Sprintf(" | %s: %.1f%%", usage.Primary.Label, usage.Primary.UsedPercent)
	}

	fmt.Printf("%s %-20s%s\n", status, usage.ProviderName, primaryInfo)
}

// printQuotaDetails prints detailed quota information
func printQuotaDetails(usage *quota.ProviderUsage) {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Printf("📊 %s Quota Details\n", strings.ToUpper(usage.ProviderName))
	fmt.Println(strings.Repeat("=", 70))

	// Status
	status := "✅ OK"
	if usage.LastError != "" {
		status = fmt.Sprintf("❌ Error: %s", usage.LastError)
	}
	fmt.Printf("Status:     %s\n", status)
	fmt.Printf("Provider:   %s (%s)\n", usage.ProviderName, usage.ProviderType)
	fmt.Printf("Fetched:    %s\n", usage.FetchedAt.Format("2006-01-02 15:04:05"))

	// Account info
	if usage.Account != nil {
		fmt.Println("\n👤 Account:")
		if usage.Account.Name != "" {
			fmt.Printf("  Name: %s\n", usage.Account.Name)
		}
		if usage.Account.Email != "" {
			fmt.Printf("  Email: %s\n", usage.Account.Email)
		}
		if usage.Account.Tier != "" {
			fmt.Printf("  Tier: %s\n", usage.Account.Tier)
		}
	}

	// Primary window
	if usage.Primary != nil {
		fmt.Println("\n📊 Primary Quota:")
		printUsageWindow(usage.Primary, 1)
	}

	// Secondary window
	if usage.Secondary != nil {
		fmt.Println("\n📊 Secondary Quota:")
		printUsageWindow(usage.Secondary, 2)
	}

	// Tertiary window
	if usage.Tertiary != nil {
		fmt.Println("\n📊 Tertiary Quota:")
		printUsageWindow(usage.Tertiary, 3)
	}

	// Cost
	if usage.Cost != nil {
		fmt.Println("\n💰 Cost:")
		fmt.Printf("  Used:    $%.2f %s\n", usage.Cost.Used, usage.Cost.CurrencyCode)
		if usage.Cost.Limit > 0 {
			fmt.Printf("  Limit:   $%.2f %s\n", usage.Cost.Limit, usage.Cost.CurrencyCode)
			percent := (usage.Cost.Used / usage.Cost.Limit) * 100
			fmt.Printf("  Percent: %.1f%%\n", percent)
		}
		if usage.Cost.ResetsAt != nil {
			fmt.Printf("  Resets:  %s\n", usage.Cost.ResetsAt.Format("2006-01-02"))
		}
	}

	fmt.Println(strings.Repeat("=", 70))
}

// printUsageWindow prints a usage window
func printUsageWindow(w *quota.UsageWindow, indent int) {
	prefix := strings.Repeat("  ", indent)
	fmt.Printf("%sLabel:     %s\n", prefix, w.Label)
	fmt.Printf("%sType:      %s\n", prefix, w.Type)
	fmt.Printf("%sUsed:      %s", prefix, formatUsageValue(w.Used, w.Unit))
	fmt.Printf(" / %s\n", formatUsageValue(w.Limit, w.Unit))
	fmt.Printf("%sPercent:   %.1f%%\n", prefix, w.UsedPercent)

	if w.ResetsAt != nil {
		if time.Until(*w.ResetsAt) > 0 {
			fmt.Printf("%sResets in: %s\n", prefix, formatDuration(time.Until(*w.ResetsAt)))
		} else {
			fmt.Printf("%sResets at: %s\n", prefix, w.ResetsAt.Format("2006-01-02 15:04"))
		}
	}
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
