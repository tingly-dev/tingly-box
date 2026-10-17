package ai

// CodexAPIBase is the API base URL for ChatGPT/Codex OAuth provider
const CodexAPIBase = "https://chatgpt.com/backend-api"

// ZCode (GLM Coding Plan) endpoints, per account platform. Each platform serves
// the same plan behind two protocols, and the credential a ZCode login resolves
// is accepted on both — which is what lets one ZCode provider answer Anthropic
// and OpenAI clients without protocol translation (see Provider.ResolveEndpoint).
const (
	ZCodeZaiAnthropicBase = "https://api.z.ai/api/anthropic"
	ZCodeZaiOpenAIBase    = "https://api.z.ai/api/coding/paas/v4"

	ZCodeBigModelAnthropicBase = "https://open.bigmodel.cn/api/anthropic"
	ZCodeBigModelOpenAIBase    = "https://open.bigmodel.cn/api/coding/paas/v4"
)

// ZCodeEndpoints returns the (anthropic, openai) coding-plan bases for a ZCode
// issuer. Both are empty for any other issuer.
func ZCodeEndpoints(issuer Issuer) (anthropicBase, openaiBase string) {
	switch issuer {
	case IssuerZCode:
		return ZCodeZaiAnthropicBase, ZCodeZaiOpenAIBase
	case IssuerZCodeCN:
		return ZCodeBigModelAnthropicBase, ZCodeBigModelOpenAIBase
	default:
		return "", ""
	}
}

// WorkBuddySite is everything that differs between the two WorkBuddy account
// regions: the host that serves both the login and the models, and the values
// the official client identifies itself with. Login (ai/oauth/workbuddy.go)
// and inference (internal/client/workbuddy_round_tripper.go) both read it, so a
// client-version bump is one edit here.
type WorkBuddySite struct {
	// Origin is the scheme+host of the login and model API (no trailing slash).
	Origin string
	// Domain is the X-Domain header value: Origin's host.
	Domain string
	// Platform is the `platform` the login state is requested for.
	Platform string
	// ProductCode is the X-Product-Code header value.
	ProductCode string
}

// APIBase is the OpenAI-compatible base: chat completions live at
// {APIBase}/chat/completions, i.e. {Origin}/v2/chat/completions.
func (s WorkBuddySite) APIBase() string { return s.Origin + "/v2" }

// WorkBuddy identification sent on every login and inference request. The
// upstream is not publicly documented; these match the official client as
// observed by third-party integrations — see .design/workbuddy-oauth.md.
const (
	WorkBuddyProduct   = "SaaS"
	WorkBuddyUserAgent = "CLI/2.108.1 CodeBuddy/2.108.1"
)

// WorkBuddySiteFor returns the site for a WorkBuddy issuer; ok is false for
// any other issuer.
func WorkBuddySiteFor(issuer Issuer) (site WorkBuddySite, ok bool) {
	switch issuer {
	case IssuerWorkBuddy:
		return WorkBuddySite{
			Origin:      "https://www.workbuddy.ai",
			Domain:      "www.workbuddy.ai",
			Platform:    "workbuddy-ai",
			ProductCode: "workbuddy",
		}, true
	case IssuerWorkBuddyCN:
		return WorkBuddySite{
			Origin:      "https://copilot.tencent.com",
			Domain:      "copilot.tencent.com",
			Platform:    "ide",
			ProductCode: "codebuddy",
		}, true
	default:
		return WorkBuddySite{}, false
	}
}

// IsWorkBuddyIssuer reports whether issuer signs in through the WorkBuddy flow.
func IsWorkBuddyIssuer(issuer Issuer) bool {
	_, ok := WorkBuddySiteFor(issuer)
	return ok
}

// Issuer - represent the OAuth identity providers
type Issuer string

const (
	IssuerAnthropic   Issuer = "anthropic"   // Anthropic OAuth issuer
	IssuerClaudeCode  Issuer = "claude_code" // Claude Code OAuth issuer
	IssuerCodex       Issuer = "codex"       // ChatGPT/Codex OAuth issuer
	IssuerGitHub      Issuer = "github"      // GitHub OAuth issuer
	IssuerGoogle      Issuer = "google"      // Google OAuth issuer
	IssuerOpenAI      Issuer = "openai"      // OpenAI OAuth issuer
	IssuerGemini      Issuer = "gemini"      // Gemini CLI OAuth issuer
	IssuerCopilot     Issuer = "copilot"     // GitHub Copilot OAuth issuer
	IssuerCursor      Issuer = "cursor"      // Cursor OAuth issuer
	IssuerKimiCode    Issuer = "kimi_code"   // Kimi Code OAuth issuer
	IssuerQwenCode    Issuer = "qwen_code"   // Qwen Code OAuth issuer
	IssuerAntigravity Issuer = "antigravity" // Antigravity OAuth issuer
	IssuerIFlow       Issuer = "iflow"       // IFlow OAuth issuer
	IssuerXAI         Issuer = "xai"         // xAI (Grok) OAuth issuer
	// IssuerZCode / IssuerZCodeCN are the two ZCode (GLM Coding Plan) account
	// platforms. They are separate issuers rather than one issuer with a
	// platform switch: the two accounts live on different hosts, hold different
	// plans, and must never be swapped by a re-authentication.
	IssuerZCode   Issuer = "zcode"    // ZCode / Z.ai GLM Coding Plan (global)
	IssuerZCodeCN Issuer = "zcode_cn" // ZCode / BigModel GLM Coding Plan (China)
	// IssuerWorkBuddy / IssuerWorkBuddyCN are the two Tencent WorkBuddy account
	// regions. CodeBuddy is another entry point onto the same CN accounts, not a
	// third channel, so it has no issuer of its own.
	IssuerWorkBuddy   Issuer = "workbuddy"    // WorkBuddy (international, workbuddy.ai)
	IssuerWorkBuddyCN Issuer = "workbuddy_cn" // WorkBuddy / CodeBuddy (China, copilot.tencent.com)
	IssuerMock        Issuer = "mock"         // Mock provider for testing
	IssuerUnknown     Issuer = ""
)
