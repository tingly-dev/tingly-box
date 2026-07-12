package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// DuoCmd is Tier "Duo": two-process e2e verification (function + memory)
// over every anthropic-source conversion route. tb1 and tb2 each run as a
// full server child process and are observed separately over their
// /api/v1/debug endpoints — the topology, route matrix, and #1255 background
// live on the engine, see internal/protocoltest/duo.go.
type DuoCmd struct {
	Routes      string  `kong:"name='routes',default='all',help='Comma-separated route names for the functional phase, or \"all\" (routes: beta-chat, beta-responses, beta-anthropic, v1-chat, v1-responses, v1-anthropic)'"`
	MemRoutes   string  `kong:"name='mem-routes',default='beta-chat,beta-chat-slow',help='Comma-separated route names for the memory phase, or \"all\" (any route also has a \"-slow\" backpressure variant; default: the Claude Code hot path, fast + backpressure)'"`
	BodyMB      float64 `kong:"name='body-mb',default='2',help='Conversation size per request in MB (mimics agentic full-context turns)'"`
	Batch       int     `kong:"name='batch',default='15',help='Requests per sequential batch (two batches measure the retention slope)'"`
	Workers     int     `kong:"name='workers',default='4',help='Concurrent workers in the burst phase'"`
	PerWorker   int     `kong:"name='per-worker',default='5',help='Requests per worker in the burst phase'"`
	MaxSlopeKB  float64 `kong:"name='max-slope-kb',help='Fail if either instance retains more than this many KB/request post-GC (default: the shared regression threshold, 32)'"`
	StreamKB    int     `kong:"name='stream-kb',default='256',help='Backpressure (-slow) routes: tb1 vmodel response size in KB'"`
	StreamMS    int     `kong:"name='stream-ms',default='500',help='Backpressure (-slow) routes: tb1 vmodel delay parameter in ms (stream wall time is roughly 2x)'"`
	ReadDelayMS int     `kong:"name='read-delay-ms',default='15',help='Backpressure (-slow) routes: client-side pause between SSE reads in ms'"`
	SkipMemory  bool    `kong:"name='skip-memory',help='Run only the functional checks'"`
	SkipFunc    bool    `kong:"name='skip-func',help='Run only the memory phase'"`
	ProfileDir  string  `kong:"name='profile-dir',help='Write per-instance pprof heap profiles (duo-<route>-<tb>-{baseline,final}.pb.gz) to this directory'"`
	JSON        bool    `kong:"name='json',help='Emit results as JSON'"`
	Verbose     bool    `kong:"name='verbose',short='v',help='Relay child instance logs (default: quiet)'"`
}

// duoResult is the JSON output shape.
type duoResult struct {
	TB1URL     string                          `json:"tb1_url"`
	TB2URL     string                          `json:"tb2_url"`
	Functional []protocoltest.DuoCheck         `json:"functional,omitempty"`
	Memory     []*protocoltest.DuoMemoryReport `json:"memory,omitempty"`
	Pass       bool                            `json:"pass"`
}

// resolveRoutes parses a comma-separated route list ("all" = every fast route).
func resolveRoutes(spec string) ([]protocoltest.DuoRoute, error) {
	if strings.TrimSpace(spec) == "all" {
		return protocoltest.AllDuoRoutes(), nil
	}
	var routes []protocoltest.DuoRoute
	for _, name := range strings.Split(spec, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		r, ok := protocoltest.FindDuoRoute(name)
		if !ok {
			return nil, fmt.Errorf("unknown route %q (known: %s, each also as <name>-slow)", name, duoRouteNames())
		}
		routes = append(routes, r)
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("no routes selected")
	}
	return routes, nil
}

func duoRouteNames() string {
	var names []string
	for _, r := range protocoltest.AllDuoRoutes() {
		names = append(names, r.Name)
	}
	return strings.Join(names, ", ")
}

func (cmd *DuoCmd) Run() error {
	if cmd.MaxSlopeKB <= 0 {
		// Same threshold the Go regression test enforces, defined in one place.
		cmd.MaxSlopeKB = protocoltest.DuoDefaultMaxSlopeKB
	}
	funcRoutes, err := resolveRoutes(cmd.Routes)
	if err != nil {
		return err
	}
	memRoutes, err := resolveRoutes(cmd.MemRoutes)
	if err != nil {
		return err
	}

	env, err := bootDuoEnv("duo", cmd.JSON, cmd.Verbose,
		protocoltest.DuoEnvConfig{StreamKB: cmd.StreamKB, StreamMS: cmd.StreamMS})
	if err != nil {
		return err
	}
	defer env.Close()

	bodyBytes := int(cmd.BodyMB * 1024 * 1024)
	result := duoResult{TB1URL: env.TB1.BaseURL, TB2URL: env.TB2.BaseURL, Pass: true}

	progress := func(format string, args ...any) {
		if !cmd.JSON {
			fmt.Printf("  ▸ "+format+"\n", args...)
		}
	}

	if !cmd.SkipFunc {
		if !cmd.JSON {
			fmt.Printf("duo: functional checks (tb2 %s → tb1 %s, body %.1f MB)\n", env.TB2.BaseURL, env.TB1.BaseURL, cmd.BodyMB)
		}
		for _, route := range funcRoutes {
			checks := env.RunFunctionalChecks(route, bodyBytes)
			result.Functional = append(result.Functional, checks...)
			for _, c := range checks {
				if !c.Pass {
					result.Pass = false
				}
			}
			if !cmd.JSON {
				printCheckBlock(route.Name, checks, cmd.Verbose)
			}
		}
	}

	if !cmd.SkipMemory {
		if !cmd.JSON {
			fmt.Println("duo: memory phase (tb1 and tb2 observed separately)")
		}
		for i := range memRoutes {
			route := memRoutes[i]
			var readDelay time.Duration
			if route.Slow {
				readDelay = time.Duration(cmd.ReadDelayMS) * time.Millisecond
			}
			report, err := env.RunMemoryPhase(protocoltest.DuoMemoryConfig{
				Route:      &route,
				BodyBytes:  bodyBytes,
				Batch:      cmd.Batch,
				Workers:    cmd.Workers,
				PerWorker:  cmd.PerWorker,
				ReadDelay:  readDelay,
				ProfileDir: cmd.ProfileDir,
				Progress:   progress,
			})
			if err != nil {
				return fmt.Errorf("memory phase (%s): %w", route.Name, err)
			}
			result.Memory = append(result.Memory, report)

			if report.MaxSlopeKB() > cmd.MaxSlopeKB {
				result.Pass = false
			}
			if !cmd.JSON {
				printMemoryReport(report, cmd.MaxSlopeKB)
			}
		}
	}

	return emitDuoOutcome(cmd.JSON, result, result.Pass, "duo")
}

// printMemoryReport renders one route's memory outcome, tb1 and tb2 side by
// side so a leak reads directly as "which instance".
func printMemoryReport(report *protocoltest.DuoMemoryReport, maxSlopeKB float64) {
	fmt.Printf("  %s:\n", report.Route)
	if report.ReadDelayMS > 0 {
		fmt.Printf("    backpressure: slow reader %d ms between reads\n", report.ReadDelayMS)
	}
	fmt.Printf("    %-26s %14s %14s\n", "", "tb1 (vmodel)", "tb2 (gateway)")
	row := func(label, format string, v1, v2 float64) {
		fmt.Printf("    %-26s %14s %14s\n", label, fmt.Sprintf(format, v1), fmt.Sprintf(format, v2))
	}
	tb1, tb2 := report.TB1, report.TB2
	row("baseline post-GC heap", "%.2f MB", tb1.BaselineHeapMB, tb2.BaselineHeapMB)
	row(fmt.Sprintf("retained after %3d reqs", report.Batch), "%+.2f MB", tb1.AfterBatch1MB, tb2.AfterBatch1MB)
	row(fmt.Sprintf("retained after %3d reqs", 2*report.Batch), "%+.2f MB", tb1.AfterBatch2MB, tb2.AfterBatch2MB)
	verdict := func(slope float64) string {
		if slope > maxSlopeKB {
			return fmt.Sprintf("LEAK (limit %.1f)", maxSlopeKB)
		}
		return "OK"
	}
	fmt.Printf("    %-26s %14s %14s   tb1 %s / tb2 %s\n", "retention slope",
		fmt.Sprintf("%.1f KB/req", tb1.SlopeKBPerRequest),
		fmt.Sprintf("%.1f KB/req", tb2.SlopeKBPerRequest),
		verdict(tb1.SlopeKBPerRequest), verdict(tb2.SlopeKBPerRequest))
	row("allocation churn", "%.2f MB/req", tb1.ChurnMBPerRequest, tb2.ChurnMBPerRequest)
	fmt.Printf("    %-26s %14s %14s\n", fmt.Sprintf("burst peak heap (%d×%d)", report.ConcurrentWorkers, report.ConcurrentTotal/report.ConcurrentWorkers),
		fmt.Sprintf("%.2f MB", tb1.PeakHeapMB), fmt.Sprintf("%.2f MB", tb2.PeakHeapMB))
	row("post-burst delta", "%+.2f MB", tb1.PostBurstDeltaMB, tb2.PostBurstDeltaMB)
	fmt.Printf("    %-26s %14s %14s\n", "goroutines (base→final)",
		fmt.Sprintf("%d→%d", tb1.BaselineGoroutines, tb1.FinalGoroutines),
		fmt.Sprintf("%d→%d", tb2.BaselineGoroutines, tb2.FinalGoroutines))
	if tb2.FinalProfile != "" {
		fmt.Printf("    heap profiles: %s , %s (and tb1 counterparts)\n", tb2.BaselineProfile, tb2.FinalProfile)
		fmt.Printf("    inspect: go tool pprof -top -inuse_space %s\n", tb2.FinalProfile)
	}
}
