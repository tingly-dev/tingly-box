package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// TokenKind identifies which tingly-box token a sub-command operates on.
// "auth" is the UserToken (control panel + control API). "model" is the
// ModelToken (used by AI clients hitting the box's OpenAI/Anthropic
// endpoints). Both are managed independently from upstream provider creds.
type TokenKind string

const (
	tokenKindAuth  TokenKind = "auth"
	tokenKindModel TokenKind = "model"
)

// ============== Kong Command Structures ==============

// TokenCmdKong manages tingly-box's own auth and model tokens.
// Upstream provider credentials are not handled here — those live under
// the `provider` and `oauth` commands.
type TokenCmdKong struct {
	List    TokenListCmdKong    `kong:"cmd,name='list',default='1',hidden,help='List tingly-box tokens (default)'"`
	View    TokenViewCmdKong    `kong:"cmd,help='View a tingly-box token (auth or model)'"`
	Refresh TokenRefreshCmdKong `kong:"cmd,help='Refresh (rotate) a tingly-box token (auth or model)'"`
}

// TokenListCmdKong prints both tokens with masked previews.
type TokenListCmdKong struct{}

func (t *TokenListCmdKong) Run(appManager *AppManager) error {
	return runBoxTokenList(appManager)
}

// TokenViewCmdKong shows a single tingly-box token. When kind is omitted,
// the user is prompted interactively.
type TokenViewCmdKong struct {
	Kind string `kong:"arg,optional,help='Which token to view: auth or model'"`
	Reveal bool   `kong:"flag,name='reveal',short='r',help='Print the full token instead of a masked preview'"`
}

func (t *TokenViewCmdKong) Run(appManager *AppManager) error {
	kind, err := resolveTokenKind(t.Kind)
	if err != nil {
		return err
	}
	return runBoxTokenView(appManager, kind, t.Reveal)
}

// TokenRefreshCmdKong rotates a tingly-box token (auth or model) and
// persists the new value. Existing clients using the old token will need
// to be updated.
type TokenRefreshCmdKong struct {
	Kind string `kong:"arg,optional,help='Which token to refresh: auth or model'"`
	Reveal bool   `kong:"flag,name='reveal',short='r',help='Print the full rotated token after success'"`
	Yes    bool   `kong:"flag,name='yes',short='y',help='Skip the rotation confirmation prompt'"`
}

func (t *TokenRefreshCmdKong) Run(appManager *AppManager) error {
	kind, err := resolveTokenKind(t.Kind)
	if err != nil {
		return err
	}
	return runBoxTokenRefresh(appManager, kind, t.Reveal, t.Yes)
}

// ============== Business Logic Functions ==============

// resolveTokenKind validates an argument value and prompts the user when
// the argument was omitted on the command line.
func resolveTokenKind(arg string) (TokenKind, error) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "auth", "user":
		return tokenKindAuth, nil
	case "model":
		return tokenKindModel, nil
	case "":
		// fall through to interactive
	default:
		return "", fmt.Errorf("unknown token kind %q (expected 'auth' or 'model')", arg)
	}

	fmt.Println("Which tingly-box token?")
	fmt.Println("  [1] auth   — control panel & control API (UserToken)")
	fmt.Println("  [2] model  — model API for AI clients (ModelToken)")
	fmt.Print("Enter choice: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "1", "auth", "user":
		return tokenKindAuth, nil
	case "2", "model":
		return tokenKindModel, nil
	}
	return "", fmt.Errorf("invalid selection")
}

// runBoxTokenList prints both tingly-box tokens (masked) along with the
// endpoint each one is used for.
func runBoxTokenList(appManager *AppManager) error {
	cfg, err := globalConfigOrErr(appManager)
	if err != nil {
		return err
	}
	port := cfg.ServerPort
	if port == 0 {
		port = 12580
	}

	fmt.Println()
	fmt.Println("Tingly Box Tokens")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  auth   %s\n", maskTokenPreview(cfg.GetUserToken()))
	fmt.Printf("         used at  http://localhost:%d/login/<token>\n", port)
	fmt.Printf("  model  %s\n", maskTokenPreview(cfg.GetModelToken()))
	fmt.Printf("         used at  http://localhost:%d (OpenAI/Anthropic API)\n", port)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("Tip: 'tingly-box token view <auth|model> --reveal' to copy.")
	return nil
}

// runBoxTokenView prints a single tingly-box token, masked unless --reveal.
func runBoxTokenView(appManager *AppManager, kind TokenKind, reveal bool) error {
	cfg, err := globalConfigOrErr(appManager)
	if err != nil {
		return err
	}
	value := readBoxToken(cfg, kind)
	if value == "" {
		return fmt.Errorf("%s token is not set; run 'tingly-box token refresh %s' to generate one", kind, kind)
	}
	port := cfg.ServerPort
	if port == 0 {
		port = 12580
	}

	fmt.Println()
	fmt.Println("Tingly Box Token")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Kind:     %s\n", kind)
	fmt.Printf("Purpose:  %s\n", boxTokenPurpose(kind))
	fmt.Printf("Endpoint: %s\n", boxTokenEndpoint(kind, port, value, reveal))
	if reveal {
		fmt.Printf("Token:    %s\n", value)
	} else {
		fmt.Printf("Token:    %s   (use --reveal to copy full token)\n", maskTokenPreview(value))
	}
	fmt.Println(strings.Repeat("=", 60))
	if reveal {
		fmt.Println("Note: token printed in full above — handle with care.")
	}
	return nil
}

// runBoxTokenRefresh rotates the chosen token, persists the change, and
// prints the new value. Prompts for confirmation unless --yes is set
// because rotation invalidates any clients still using the old token.
func runBoxTokenRefresh(appManager *AppManager, kind TokenKind, reveal, yes bool) error {
	cfg, err := globalConfigOrErr(appManager)
	if err != nil {
		return err
	}

	if !yes {
		fmt.Printf("Rotate the %s token now? Existing clients using the current token will stop working until updated. [y/N]: ", kind)
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(input)) {
		case "y", "yes":
		default:
			fmt.Println("Cancelled.")
			return nil
		}
	}

	newToken, err := generateBoxToken(kind)
	if err != nil {
		return fmt.Errorf("failed to generate %s token: %w", kind, err)
	}
	if err := writeBoxToken(cfg, kind, newToken); err != nil {
		return fmt.Errorf("failed to persist %s token: %w", kind, err)
	}

	port := cfg.ServerPort
	if port == 0 {
		port = 12580
	}
	fmt.Println()
	fmt.Printf("Rotated %s token successfully.\n", kind)
	fmt.Printf("Endpoint: %s\n", boxTokenEndpoint(kind, port, newToken, reveal))
	if reveal {
		fmt.Printf("Token:    %s\n", newToken)
	} else {
		fmt.Printf("Token:    %s   (use --reveal to copy full token)\n", maskTokenPreview(newToken))
	}
	fmt.Println("Restart any clients (web UI, AI tools) with the new value.")
	return nil
}

// ============== Helpers ==============

func globalConfigOrErr(appManager *AppManager) (*serverconfig.Config, error) {
	if appManager == nil || appManager.AppConfig() == nil {
		return nil, fmt.Errorf("application config not initialised")
	}
	cfg := appManager.AppConfig().GetGlobalConfig()
	if cfg == nil {
		return nil, fmt.Errorf("global config not available")
	}
	return cfg, nil
}

func readBoxToken(cfg *serverconfig.Config, kind TokenKind) string {
	switch kind {
	case tokenKindAuth:
		return cfg.GetUserToken()
	case tokenKindModel:
		return cfg.GetModelToken()
	}
	return ""
}

func writeBoxToken(cfg *serverconfig.Config, kind TokenKind, value string) error {
	switch kind {
	case tokenKindAuth:
		return cfg.SetUserToken(value)
	case tokenKindModel:
		return cfg.SetModelToken(value)
	}
	return fmt.Errorf("unknown token kind: %s", kind)
}

func generateBoxToken(kind TokenKind) (string, error) {
	switch kind {
	case tokenKindAuth:
		return serverconfig.GenerateUserToken()
	case tokenKindModel:
		return serverconfig.GenerateModelToken()
	}
	return "", fmt.Errorf("unknown token kind: %s", kind)
}

func boxTokenPurpose(kind TokenKind) string {
	switch kind {
	case tokenKindAuth:
		return "control panel & control API authentication"
	case tokenKindModel:
		return "model API authentication for AI clients"
	}
	return ""
}

// boxTokenEndpoint formats the endpoint hint for the given token. For the
// auth token the URL embeds the value when the user opted into --reveal;
// otherwise a placeholder is shown so logs don't leak the secret.
func boxTokenEndpoint(kind TokenKind, port int, value string, reveal bool) string {
	switch kind {
	case tokenKindAuth:
		if reveal && value != "" {
			return fmt.Sprintf("http://localhost:%d/login/%s", port, value)
		}
		return fmt.Sprintf("http://localhost:%d/login/<token>", port)
	case tokenKindModel:
		return fmt.Sprintf("http://localhost:%d", port)
	}
	return ""
}

// maskTokenPreview returns a redacted preview of a token suitable for
// display, e.g. "tb-mod…wxyz". Tokens shorter than 12 chars are fully
// masked.
func maskTokenPreview(t string) string {
	if t == "" {
		return "<empty>"
	}
	if len(t) <= 12 {
		return strings.Repeat("•", len(t))
	}
	return t[:6] + "…" + t[len(t)-4:]
}
