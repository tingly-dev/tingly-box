package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ============== Kong Command Structures ==============

// CCmdKong launches Claude Code with passthrough mode
// Kong's passthrough mode requires at least one positional arg
type CCmdKong struct {
	Args []string `kong:"arg,optional,passthrough"`
}

func (c *CCmdKong) Run(appManager *AppManager) error {
	profile, port, claudeArgs, err := parseCCFlags(c.Args)
	if err != nil {
		return err
	}
	return runCC(appManager, profile, port, claudeArgs)
}

// ============== Business Logic Functions ==============

// parseCCFlags consumes tingly-box-specific flags from the beginning of args
// and returns the remaining args verbatim for claude.
//
// Recognized flags: -p/--profile, --tingly-port. Scanning stops at the first
// token that is not a recognized tingly-box flag, so everything from that
// point on is passed to claude unchanged — no "--" separator required.
func parseCCFlags(args []string) (profile string, port int, claudeArgs []string, err error) {
	i := 0
	for i < len(args) {
		switch {
		case args[i] == "--profile" || args[i] == "-p":
			if i+1 >= len(args) {
				return "", 0, nil, fmt.Errorf("flag %s requires a value", args[i])
			}
			profile = args[i+1]
			i += 2

		case args[i] == "--tingly-port":
			if i+1 >= len(args) {
				return "", 0, nil, fmt.Errorf("flag %s requires a value", args[i])
			}
			p, parseErr := strconv.Atoi(args[i+1])
			if parseErr != nil || p <= 0 || p > 65535 {
				return "", 0, nil, fmt.Errorf("flag --tingly-port requires a valid port number, got %q", args[i+1])
			}
			port = p
			i += 2

		default:
			// First unrecognized token — everything from here is claude's
			return profile, port, args[i:], nil
		}
	}
	return profile, port, nil, nil
}

// runCC orchestrates: ensure server → resolve profile → write settings → exec claude.
// If portOverride > 0, it takes precedence over the server's configured port.
func runCC(appManager *AppManager, profile string, portOverride int, claudeArgs []string) error {
	globalConfig := appManager.GetGlobalConfig()
	scenario := typ.ScenarioClaudeCode

	// Resolve profile if specified
	var profileID string
	var profileMeta *typ.ProfileMeta
	if profile != "" {
		resolved, err := globalConfig.ResolveProfileNameOrID(scenario, profile)
		if err != nil {
			// Profile not found — show interactive list so user can pick one
			profiles := globalConfig.GetProfiles(scenario)
			selected, selErr := selectProfileInteractive(profiles, profile)
			if selErr != nil {
				return fmt.Errorf("profile error: %w", err)
			}
			resolved = selected
		}
		profileID = resolved

		// Get profile metadata
		profiles := globalConfig.GetProfiles(scenario)
		for i := range profiles {
			if profiles[i].ID == profileID {
				profileMeta = &profiles[i]
				break
			}
		}
	}

	// Build the scenario path (with or without profile)
	scenarioPath := string(scenario)
	if profileID != "" {
		scenarioPath = string(typ.ProfiledScenarioName(scenario, profileID))
	}

	// Build base URL and token
	port := portOverride
	if port == 0 {
		port = appManager.GetServerPort()
	}
	if port == 0 {
		port = 12580
	}
	baseURL := fmt.Sprintf("http://localhost:%d", port)
	apiKey := globalConfig.GetModelToken()

	// Unified mode determination:
	// 1. If profile is used, use profile's unified setting
	// 2. Otherwise, use scenario flag (defaults to false/separate mode)
	var envUnified bool
	if profileMeta != nil {
		// Profile mode: use profile's unified setting
		envUnified = profileMeta.Unified
	} else {
		// Default mode: use scenario flag
		if sc := globalConfig.GetScenarioConfig(scenario); sc != nil {
			envUnified = sc.GetDefaultFlags().Unified
		}
	}
	env := generateCCEnv(globalConfig, baseURL, apiKey, scenarioPath, envUnified, profileID != "")

	// Build settings file. Profile mode uses the profileID; default mode uses
	// "default" as a stable, predictable name so the file is reused across runs.
	settingsID := profileID
	if settingsID == "" {
		settingsID = "default"
	}
	settingsPath, err := config.BuildCCProfileSettings(settingsID, scenarioPath, env)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}

	// Discover claude binary
	variant, err := claude.FindClaudeCLI(context.Background())
	if err != nil {
		return fmt.Errorf("claude CLI not found: %w", err)
	}

	// Build claude args: --settings <file> + passthrough
	execArgs := []string{"--settings", settingsPath}
	execArgs = append(execArgs, claudeArgs...)

	// Exec replaces current process
	binPath := variant.Path
	//nolint:gosec // intentional exec of user-installed CLI
	execCmd := exec.Command(binPath, execArgs...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Env = os.Environ()

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("failed to run claude CLI: %w", err)
	}
	return nil
}

// selectProfileInteractive shows a numbered list of profiles and prompts the
// user to select one. notFoundName is the profile name/ID the user originally
// requested (used in the error message when profiles is empty).
// Returns the selected profile ID, or an error if no selection can be made.
func selectProfileInteractive(profiles []typ.ProfileMeta, notFoundName string) (string, error) {
	if len(profiles) == 0 {
		if notFoundName != "" {
			return "", fmt.Errorf("profile '%s' not found and no profiles are configured", notFoundName)
		}
		return "", fmt.Errorf("no profiles configured")
	}

	if notFoundName != "" {
		fmt.Fprintf(os.Stderr, "Profile '%s' not found. Available profiles:\n", notFoundName)
	} else {
		fmt.Fprintln(os.Stderr, "Available profiles:")
	}
	for i, p := range profiles {
		mode := "separate"
		if p.Unified {
			mode = "unified"
		}
		fmt.Fprintf(os.Stderr, "  [%d] %s (%s, %s)\n", i+1, p.Name, p.ID, mode)
	}
	fmt.Fprintf(os.Stderr, "  [0] Continue without profile\n")
	fmt.Fprintf(os.Stderr, "Select profile [1-%d, 0 to skip]: ", len(profiles))

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("no input")
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" || line == "0" {
		return "", nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(profiles) {
		return "", fmt.Errorf("invalid selection '%s'", line)
	}
	return profiles[n-1].ID, nil
}


// generateCCEnv builds the env vars map for Claude Code settings.
//
// The per-tier model names are resolved from the rules the request will
// actually hit, looked up by their canonical UUIDs: profile rules under
// "builtin:<scenario>:<tier>" (e.g. "builtin:claude_code:p1:haiku"), main
// scenario rules under the legacy built-in constants. Reading the rule's
// request_model (instead of assuming the seeded name) keeps the env aligned
// when a user renames a rule's model; the seeded name is the fallback when
// the rule is missing or inactive.
func generateCCEnv(cfg *config.Config, baseURL, apiKey, scenarioPath string, unified bool, isProfile bool) map[string]string {
	env := map[string]string{
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"API_TIMEOUT_MS":                           "3000000",
		"ANTHROPIC_BASE_URL":                       baseURL + "/tingly/" + scenarioPath,
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
	}

	ruleModel := func(fallback string, uuids ...string) string {
		if cfg != nil {
			for _, uuid := range uuids {
				if r := cfg.GetRuleByUUID(uuid); r != nil && r.Active {
					if m := strings.TrimSpace(r.RequestModel); m != "" {
						// Mirror the frontend quick-config: a rule with the 1M
						// context flag advertises itself to Claude Code via the
						// [1m] model-name suffix (the client strips it back off
						// and sends the context-1m beta header instead).
						if r.Flags.Context1M && !strings.HasSuffix(m, config.Context1MSuffix) {
							m += config.Context1MSuffix
						}
						return m
					}
				}
			}
		}
		return fallback
	}
	// tierModel resolves one tier slot: profile rules by canonical profiled
	// UUID with the short tier name as fallback, main scenario rules by the
	// modern built-in UUID (legacy UUID as a compatibility fallback for
	// configs not yet renamed by migration) with the canonical tingly/* name
	// as the final fallback.
	tierModel := func(tier, legacyUUID, legacyFallback string) string {
		if isProfile {
			return ruleModel(tier, config.BuiltinRuleUUID(typ.RuleScenario(scenarioPath), tier))
		}
		return ruleModel(legacyFallback, config.BuiltinRuleUUID(typ.ScenarioClaudeCode, tier), legacyUUID)
	}

	if unified {
		model := tierModel("cc", config.RuleUUIDBuiltinCC, "tingly/cc")
		env["ANTHROPIC_MODEL"] = model
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = model
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = model
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = model
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = model
	} else {
		env["ANTHROPIC_MODEL"] = tierModel("default", config.RuleUUIDBuiltinCCDefault, "tingly/cc-default")
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = tierModel("haiku", config.RuleUUIDBuiltinCCHaiku, "tingly/cc-haiku")
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = tierModel("opus", config.RuleUUIDBuiltinCCOpus, "tingly/cc-opus")
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = tierModel("sonnet", config.RuleUUIDBuiltinCCSonnet, "tingly/cc-sonnet")
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = tierModel("subagent", config.RuleUUIDBuiltinCCSubagent, "tingly/cc-subagent")
	}

	return env
}
