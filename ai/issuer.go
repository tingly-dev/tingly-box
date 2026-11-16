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

// Kimi Code serves one gateway behind both protocols: the OpenAI-compatible
// base ends in /coding/v1, the Anthropic-compatible base is /coding (the
// Anthropic SDK appends /v1/messages itself). Per Kimi Code's model
// configuration docs (www.kimi.com/code/docs/en/kimi-code/models.html).
const (
	KimiCodeAnthropicBase = "https://api.kimi.com/coding"
	KimiCodeOpenAIBase    = "https://api.kimi.com/coding/v1"

	KimiCodeIntlAnthropicBase = "https://api.kimi.ai/coding"
	KimiCodeIntlOpenAIBase    = "https://api.kimi.ai/coding/v1"
)

// IsKimiCodeIssuer reports whether issuer is a Kimi Code platform (either).
func IsKimiCodeIssuer(issuer Issuer) bool {
	return issuer == IssuerKimiCode || issuer == IssuerKimiCodeIntl
}

// DualEndpoints returns the (anthropic, openai) bases of an OAuth issuer whose
// credential is served natively on both protocols. Both are empty for every
// issuer bound to a single endpoint. This is the one table behind
// IssuerSupportsDual and the OAuth create paths (web and CLI): an issuer joins
// by getting a row here once its second native endpoint is verified.
func DualEndpoints(issuer Issuer) (anthropicBase, openaiBase string) {
	switch issuer {
	case IssuerKimiCode:
		return KimiCodeAnthropicBase, KimiCodeOpenAIBase
	case IssuerKimiCodeIntl:
		return KimiCodeIntlAnthropicBase, KimiCodeIntlOpenAIBase
	default:
		return ZCodeEndpoints(issuer)
	}
}

// WorkBuddySite is everything that differs between the two WorkBuddy account
// regions: the host that serves the login, the models, the chats and the
// billing meter, and the values the official client identifies itself with.
// Login (ai/oauth/workbuddy.go), inference
// (internal/client/workbuddy_round_tripper.go) and quota
// (ai/quota/fetcher/workbuddy.go) all read it, so a client-version bump is one
// edit here.
type WorkBuddySite struct {
	// Origin is the scheme+host of the whole API (no trailing slash). The
	// billing meter lives here too, under /billing/meter with no /v2 prefix.
	Origin string
	// Domain is the X-Domain header value: Origin's host.
	Domain string
	// Platform is the `platform` the login state is requested for.
	Platform string
	// AgentType is the X-Agent-Type header value; only the international
	// build's client sends one.
	AgentType string
}

// APIBase is the OpenAI-compatible base: chat completions live at
// {APIBase}/chat/completions, i.e. {Origin}/v2/chat/completions.
func (s WorkBuddySite) APIBase() string { return s.Origin + "/v2" }

// WorkBuddy identification. The upstream is not publicly documented; these
// match the WorkBuddy desktop client as observed by integrations run against
// real accounts — see .design/workbuddy-oauth.md#client-identification.
const (
	WorkBuddyProduct = "SaaS"
	// WorkBuddyClientVersion is the desktop client version presented.
	WorkBuddyClientVersion = "5.5.6"
	// WorkBuddyUserAgent is required on chats: any other User-Agent is
	// refused with error 10085.
	WorkBuddyUserAgent = "WorkBuddy/" + WorkBuddyClientVersion
	// WorkBuddyLoginVersion is the `version` the sign-in page is opened with.
	WorkBuddyLoginVersion = "2.0.0"
	// WorkBuddyConfigUserAgent selects the CLI agent's product config on
	// GET /v3/config; the bare WorkBuddyUserAgent gets the IDE's.
	WorkBuddyConfigUserAgent = "CLI/" + WorkBuddyLoginVersion + " " + WorkBuddyUserAgent
)

// ClientHeaders are the headers the desktop client sends with every
// authenticated request (chat, models, billing). WorkBuddy attributes usage by
// them: a chat without them shows under no client in the account's usage
// list. Authorization and X-User-Id are the caller's.
func (s WorkBuddySite) ClientHeaders() map[string]string {
	h := map[string]string{
		"User-Agent":       WorkBuddyUserAgent,
		"X-Domain":         s.Domain,
		"X-Product":        WorkBuddyProduct,
		"X-IDE-Type":       "WorkBuddy",
		"X-IDE-Name":       "WorkBuddy",
		"X-IDE-Version":    WorkBuddyClientVersion,
		"X-Requested-With": "XMLHttpRequest",
		"X-Agent-Intent":   "craft",
		"X-Agent-Purpose":  "conversation",
	}
	if s.AgentType != "" {
		h["X-Agent-Type"] = s.AgentType
	}
	return h
}

// WorkBuddySiteFor returns the site for a WorkBuddy issuer; ok is false for
// any other issuer.
func WorkBuddySiteFor(issuer Issuer) (site WorkBuddySite, ok bool) {
	switch issuer {
	case IssuerWorkBuddy:
		return WorkBuddySite{
			Origin:    "https://www.workbuddy.ai",
			Domain:    "www.workbuddy.ai",
			Platform:  "workbuddy-ai",
			AgentType: "main",
		}, true
	case IssuerWorkBuddyCN:
		return WorkBuddySite{
			Origin:   "https://copilot.tencent.com",
			Domain:   "copilot.tencent.com",
			Platform: "workbuddy",
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
	IssuerAnthropic  Issuer = "anthropic"   // Anthropic OAuth issuer
	IssuerClaudeCode Issuer = "claude_code" // Claude Code OAuth issuer
	IssuerCodex      Issuer = "codex"       // ChatGPT/Codex OAuth issuer
	IssuerGitHub     Issuer = "github"      // GitHub OAuth issuer
	IssuerGoogle     Issuer = "google"      // Google OAuth issuer
	IssuerOpenAI     Issuer = "openai"      // OpenAI OAuth issuer
	IssuerGemini     Issuer = "gemini"      // Gemini CLI OAuth issuer
	IssuerCopilot    Issuer = "copilot"     // GitHub Copilot OAuth issuer
	IssuerCursor     Issuer = "cursor"      // Cursor OAuth issuer
	// IssuerKimiCode / IssuerKimiCodeIntl are the two Kimi Code account
	// platforms (kimi.com mainland, kimi.ai overseas): separate hosts and
	// accounts, so separate issuers, as with ZCode. kimi_code keeps its name
	// for the mainland platform so stored credentials stay valid.
	IssuerKimiCode     Issuer = "kimi_code"      // Kimi Code (mainland, kimi.com)
	IssuerKimiCodeIntl Issuer = "kimi_code_intl" // Kimi Code (overseas, kimi.ai)
	IssuerQwenCode     Issuer = "qwen_code"      // Qwen Code OAuth issuer
	IssuerAntigravity  Issuer = "antigravity"    // Antigravity OAuth issuer
	IssuerIFlow        Issuer = "iflow"          // IFlow OAuth issuer
	IssuerXAI          Issuer = "xai"            // xAI (Grok) OAuth issuer
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
	IssuerQoder       Issuer = "qoder"
	IssuerQoderCN     Issuer = "qoder_cn"
	IssuerMock        Issuer = "mock" // Mock provider for testing
	IssuerUnknown     Issuer = ""
)

func IsQoderIssuer(i Issuer) bool { return i == IssuerQoder || i == IssuerQoderCN }

func QoderInferenceBase(i Issuer) string {
	if i == IssuerQoder {
		return "https://api2.qoder.sh"
	}
	if i == IssuerQoderCN {
		return "https://gateway.qoder.com.cn"
	}
	return ""
}
