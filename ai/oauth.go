package ai

// CodexAPIBase is the API base URL for ChatGPT/Codex OAuth provider
const CodexAPIBase = "https://chatgpt.com/backend-api"

// OAuth issuer constants - represent the OAuth identity providers
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
	IssuerKimi        Issuer = "kimi_code"   // Kimi OAuth issuer
	IssuerQwen        Issuer = "qwen_code"   // Qwen OAuth issuer
	IssuerAntigravity Issuer = "antigravity" // Antigravity OAuth issuer
	IssuerIFlow       Issuer = "iflow"       // IFlow OAuth issuer
	IssuerMock        Issuer = "mock"        // Mock provider for testing
)
