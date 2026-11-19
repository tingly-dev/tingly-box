package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/server"
)

func inactiveTag(active bool) string {
	if active {
		return ""
	}
	return ",inactive"
}

// ---- output ----

type lbRunOutput struct {
	Rule          string            `json:"rule"`
	Tactic        string            `json:"tactic"`
	Affinity      int               `json:"affinity_secs"`
	Services      []string          `json:"services"`
	Steps         []lbStepOutput    `json:"steps"`
	FinalBreakers map[string]string `json:"final_breakers"`
	FinalHealth   map[string]string `json:"final_health"`
}

type lbStepOutput struct {
	Trace   *server.LBTrace `json:"trace,omitempty"`
	Advance string          `json:"advance,omitempty"`
}

func (o lbRunOutput) renderTable(w *os.File) {
	tactic := o.Tactic
	if tactic == "" {
		tactic = "tier"
	}
	fmt.Fprintf(w, "LB scenario %q  tactic=%s  affinity=%ds\n", o.Rule, tactic, o.Affinity)
	fmt.Fprintf(w, "services: %s\n\n", strings.Join(o.Services, "  "))
	fmt.Fprintf(w, "%-4s %-10s %-34s %-7s %s\n", "#", "session", "attempts", "status", "pin")

	n := 0
	for _, st := range o.Steps {
		if st.Advance != "" {
			fmt.Fprintf(w, "     -- advance %s --\n", st.Advance)
			continue
		}
		n++
		tr := st.Trace
		sess := tr.Session
		if sess == "" {
			sess = "-"
		}
		fmt.Fprintf(w, "%-4d %-10s %-34s %-7d %s\n",
			n, sess, strings.Join(tr.Attempts, " → "), tr.FinalStatus, dashIfEmpty(tr.PinAfter))
	}

	fmt.Fprintf(w, "\nfinal breakers: %s\n", formatBreakers(o.FinalBreakers))
	fmt.Fprintf(w, "final health:   %s\n", formatBreakers(o.FinalHealth))
}

// renderGraph prints a pencil-graph view: per request a hop line (failover path
// annotated with ✓/✗ + status) and a state line (each svc's breaker/health +
// the affinity pin) — mirroring .design/tier-routing.pencil.md.
func (o lbRunOutput) renderGraph(w *os.File, orderedIDs []string) {
	tactic := o.Tactic
	if tactic == "" {
		tactic = "tier"
	}
	fmt.Fprintf(w, "LB scenario %q  tactic=%s  affinity=%ds\n", o.Rule, tactic, o.Affinity)
	fmt.Fprintf(w, "services: %s\n", strings.Join(o.Services, "  "))
	fmt.Fprintf(w, "legend:   ✓=2xx committed   ✗=non-2xx (buffered/terminal)   →=in-request failover hop\n\n")

	n := 0
	for _, st := range o.Steps {
		if st.Advance != "" {
			fmt.Fprintf(w, "        ⋯ advance %s (breaker + health clocks move) ⋯\n\n", st.Advance)
			continue
		}
		n++
		tr := st.Trace

		hops := make([]string, len(tr.Attempts))
		for i, sid := range tr.Attempts {
			status := 0
			if i < len(tr.Statuses) {
				status = tr.Statuses[i]
			}
			mark := "✗"
			if status >= 200 && status < 300 {
				mark = "✓"
			}
			hops[i] = fmt.Sprintf("%s %s%d", sid, mark, status)
		}
		sess := tr.Session
		if sess == "" {
			sess = "-"
		}
		fmt.Fprintf(w, "#%-2d %-8s %s   →  client=%d\n", n, sess, strings.Join(hops, "  →  "), tr.FinalStatus)

		states := make([]string, 0, len(orderedIDs))
		for _, id := range orderedIDs {
			states = append(states, fmt.Sprintf("%s=%s/%s", id, tr.BreakerAfter[id], tr.HealthAfter[id]))
		}
		line := strings.Join(states, "   ")
		if tr.PinAfter != "" {
			line += "   pin=" + tr.PinAfter
		}
		fmt.Fprintf(w, "       state: %s\n\n", line)
	}
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatBreakers(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for id, st := range m {
		parts = append(parts, fmt.Sprintf("%s=%s", id, st))
	}
	return strings.Join(parts, "  ")
}
