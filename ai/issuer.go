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
	// IssuerZCode / IssuerZCodeCN are the two ZCode (GLM Coding Plan) account
	// platforms. They are separate issuers rather than one issuer with a
	// platform switch: the two accounts live on different hosts, hold different
	// plans, and must never be swapped by a re-authentication.
	IssuerZCode   Issuer = "zcode"    // ZCode / Z.ai GLM Coding Plan (global)
	IssuerZCodeCN Issuer = "zcode_cn" // ZCode / BigModel GLM Coding Plan (China)
	IssuerMock    Issuer = "mock"     // Mock provider for testing
	IssuerUnknown Issuer = ""
)
