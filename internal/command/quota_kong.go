package command

// QuotaCmdKong is the Kong version of quota command with streamlined behavior.
// The default behavior (no subcommand) is to list all quotas.
type QuotaCmdKong struct {
	// List quotas (default behavior) - this is the default command
	List    QuotaListCmdKong    `kong:"cmd,name='list',default='1',hidden,help='List all provider quotas (default)'"`
	Get     QuotaGetCmdKong     `kong:"cmd,help='Get provider quota details'"`
	Refresh QuotaRefreshCmdKong `kong:"cmd,help='Refresh quota data'"`
	Summary QuotaSummaryCmdKong `kong:"cmd,help='Show quota summary'"`
}

// QuotaListCmdKong lists all provider quotas with optional refresh
type QuotaListCmdKong struct {
	Refresh bool `kong:"flag,name='refresh',short='r',help='Refresh before listing'"`
}

func (q *QuotaListCmdKong) Run(appManager *AppManager) error {
	if q.Refresh {
		return runQuotaRefresh(appManager)
	}
	return runQuotaList(appManager)
}

// QuotaRefreshCmdKong refreshes quota data
// Supports optional provider argument to refresh specific provider
type QuotaRefreshCmdKong struct {
	Provider string `kong:"arg,optional,help='Provider name or UUID to refresh (refreshes all if omitted)'"`
}

func (q *QuotaRefreshCmdKong) Run(appManager *AppManager) error {
	if q.Provider == "" {
		return runQuotaRefresh(appManager)
	}
	return runQuotaRefreshProvider(appManager, q.Provider)
}

// QuotaSummaryCmdKong shows quota summary
type QuotaSummaryCmdKong struct{}

func (q *QuotaSummaryCmdKong) Run(appManager *AppManager) error {
	return runQuotaSummary(appManager)
}

// QuotaGetCmdKong shows details for a specific provider
// This was merged into the list command, but we keep a separate command for explicit "get" usage
type QuotaGetCmdKong struct {
	Provider string `kong:"arg,optional,help='Provider name or UUID'"`
	Refresh  bool   `kong:"flag,name='refresh',short='r',help='Refresh before displaying'"`
}

func (q *QuotaGetCmdKong) Run(appManager *AppManager) error {
	if q.Provider == "" {
		return runQuotaGetInteractive(appManager)
	}
	return runQuotaGet(appManager, q.Provider, q.Refresh)
}
