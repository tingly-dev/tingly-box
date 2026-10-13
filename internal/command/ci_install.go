package command

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/agent"
)

// CIInstallCmdKong installs an agent's CLI via npm. It is intentionally
// minimal: one agent per invocation, global install, sudo is NOT handled
// (npm's own EACCES message is more informative than anything we'd wrap).
type CIInstallCmdKong struct {
	Agent   string `kong:"flag,name='agent',help='Agent type: cc | oc | cx'"`
	Version string `kong:"flag,name='version',help='Pin a specific version (default: latest)'"`
	Package string `kong:"flag,name='package',help='Override the npm package name'"`
	DryRun  bool   `kong:"flag,name='dry-run',help='Print the npm command without executing it'"`
}

// Run validates flags and shells out to npm. Validation errors exit with 2
// (configuration error); npm's exit code is propagated otherwise.
func (c *CIInstallCmdKong) Run(_ *AppManager) error {
	pkgSpec, err := resolveInstallPackage(c.Agent, c.Package, c.Version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ci install: %v\n", err)
		os.Exit(2)
	}

	if c.DryRun {
		fmt.Printf("npm install -g %s\n", pkgSpec)
		return nil
	}

	if _, err := exec.LookPath("npm"); err != nil {
		fmt.Fprintln(os.Stderr, "ci install: npm not found in PATH; install Node.js / npm first")
		os.Exit(2)
	}

	fmt.Printf("$ npm install -g %s\n", pkgSpec)
	cmd := exec.Command("npm", "install", "-g", pkgSpec)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		// npm already printed a meaningful error to stderr; surface its exit
		// code to the caller so CI gates behave normally.
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("failed to invoke npm: %w", err)
	}
	return nil
}

// resolveInstallPackage produces the "<pkg>[@version]" argument for
// `npm install -g`. It validates the agent, applies a --package override,
// and tacks on a --version pin if given. Pure (no I/O, no os.Exit) so tests
// can drive every branch.
func resolveInstallPackage(agentFlag, override, version string) (string, error) {
	if strings.TrimSpace(agentFlag) == "" {
		return "", fmt.Errorf("missing required flag --agent")
	}

	agentType, err := agent.ParseAgentType(agentFlag)
	if err != nil {
		return "", fmt.Errorf("invalid --agent %q (accepted: cc, oc, cx)", agentFlag)
	}

	pkg := strings.TrimSpace(override)
	if pkg == "" {
		info, ok := agent.GetAgentInfo(agentType)
		if !ok || info.NPMPackage == "" {
			return "", fmt.Errorf("no npm package registered for agent %s; pass --package to override", agentType)
		}
		pkg = info.NPMPackage
	}

	if v := strings.TrimSpace(version); v != "" {
		pkg = pkg + "@" + v
	}
	return pkg, nil
}
