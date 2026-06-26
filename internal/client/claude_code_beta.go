package client

import (
	"strings"

	"github.com/sirupsen/logrus"
)

// classifyUpstreamBetaFlag inspects a token from the upstream
// anthropic-beta header and returns whether to keep it plus a reason
// suitable for logging when it's dropped.
//   - "unknown": not in knownAnthropicBetas — likely garbage or a flag
//     newer than this build. Outer-layer drop.
//   - "not-fingerprint-safe": valid Anthropic flag, but forwarding it
//     would break the claude-cli fingerprint. Inner-layer drop.
//   - "" with keep=true: safe to forward.
func classifyUpstreamBetaFlag(s string) (keep bool, reason string) {
	if _, ok := knownAnthropicBetas[s]; !ok {
		return false, "unknown"
	}
	if _, ok := claudeCodeRequiredBetas[s]; ok {
		return true, ""
	}
	if _, ok := claudeCodeAllowedUpstreamBetas[s]; ok {
		return true, ""
	}
	return false, "not-fingerprint-safe"
}

// mergeBetaFlags builds the outgoing anthropic-beta value in a single pass:
// tokenize → validate → dedupe. The required Claude Code baseline goes
// first (preserving the claude-cli fingerprint), then any upstream-supplied
// flags that are on the narrow allowlist of fingerprint-safe additions.
// Everything else from upstream is dropped with a warn log — see
// claudeCodeAllowedUpstreamBetas. Finally requiredOAuth is appended as a
// fallback if no oauth flag is present in the merged set.
//
// `required` is passed in pre-tokenized so the production caller can hand
// us the package-level slice (claudeCodeRequiredBetasOrdered) and avoid
// re-splitting the constant on every request; tests construct their own.
func mergeBetaFlags(upstream []string, required ...string) string {
	seen := make(map[string]struct{})
	var out []string
	emit := func(p string) {
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	// Required baseline — trusted, emitted as-is.
	for _, p := range required {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		emit(p)
	}
	// Upstream — gated by the fingerprint-safe allowlist.
	for _, v := range upstream {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			keep, reason := classifyUpstreamBetaFlag(p)
			if !keep {
				logrus.WithFields(logrus.Fields{"flag": p, "reason": reason}).
					Warn("dropping upstream anthropic-beta flag")
				continue
			}
			emit(p)
		}
	}
	return strings.Join(out, ",")
}
