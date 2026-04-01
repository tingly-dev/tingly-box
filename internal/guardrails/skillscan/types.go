package skillscan

import "time"

// ScannerVersion identifies the built-in scanner rulepack/version.
const ScannerVersion = "v0"

// RiskLevel is the aggregated severity assigned to a scan result.
type RiskLevel string

const (
	// RiskLevelLow means no finding exceeded informational/default severity.
	RiskLevelLow RiskLevel = "low"
	// RiskLevelMedium indicates suspicious behavior that should usually be reviewed.
	RiskLevelMedium RiskLevel = "medium"
	// RiskLevelHigh indicates clearly risky behavior with likely security impact.
	RiskLevelHigh RiskLevel = "high"
	// RiskLevelCritical indicates behavior that should normally block a skill.
	RiskLevelCritical RiskLevel = "critical"
)

// RiskTag is a stable identifier for a specific scanner rule/category.
type RiskTag string

const (
	// Execution risks.
	RiskTagShellExec  RiskTag = "SHELL_EXEC"
	RiskTagRemoteLoad RiskTag = "REMOTE_LOADER"
	RiskTagAutoUpdate RiskTag = "AUTO_UPDATE"

	// Secret access risks.
	RiskTagReadEnvSecrets RiskTag = "READ_ENV_SECRETS"
	RiskTagReadSSHKeys    RiskTag = "READ_SSH_KEYS"
	RiskTagReadKeychain   RiskTag = "READ_KEYCHAIN"

	// Exfiltration risks.
	RiskTagNetExfil RiskTag = "NET_EXFIL_UNRESTRICTED"
	RiskTagWebhook  RiskTag = "WEBHOOK_EXFIL"

	// Prompt / social engineering risks.
	RiskTagPromptInjection    RiskTag = "PROMPT_INJECTION"
	RiskTagSocialEngineering  RiskTag = "SOCIAL_ENGINEERING"
	RiskTagTrojanDistribution RiskTag = "TROJAN_DISTRIBUTION"
	RiskTagSuspiciousPasteURL RiskTag = "SUSPICIOUS_PASTE_URL"
	RiskTagSuspiciousIP       RiskTag = "SUSPICIOUS_IP"

	// Evasion risks.
	RiskTagObfuscation RiskTag = "OBFUSCATION"

	// Web3 risks.
	RiskTagPrivateKeyPattern     RiskTag = "PRIVATE_KEY_PATTERN"
	RiskTagMnemonicPattern       RiskTag = "MNEMONIC_PATTERN"
	RiskTagWalletDraining        RiskTag = "WALLET_DRAINING"
	RiskTagUnlimitedApproval     RiskTag = "UNLIMITED_APPROVAL"
	RiskTagDangerousSelfdestruct RiskTag = "DANGEROUS_SELFDESTRUCT"
	RiskTagHiddenTransfer        RiskTag = "HIDDEN_TRANSFER"
	RiskTagProxyUpgrade          RiskTag = "PROXY_UPGRADE"
	RiskTagFlashLoanRisk         RiskTag = "FLASH_LOAN_RISK"
	RiskTagReentrancyPattern     RiskTag = "REENTRANCY_PATTERN"
	RiskTagSignatureReplay       RiskTag = "SIGNATURE_REPLAY"
)

// TargetKind describes whether a scan ran against a file or a directory.
type TargetKind string

const (
	// TargetKindFile scans one standalone skill file.
	TargetKindFile TargetKind = "file"
	// TargetKindDir scans a directory bundle and hashes all scanned files together.
	TargetKindDir TargetKind = "dir"
)

// RuleTarget controls which view of a file a rule should inspect.
type RuleTarget string

const (
	// RuleTargetContent scans the full file content as-is.
	RuleTargetContent RuleTarget = "content"
	// RuleTargetMarkdownBody scans markdown prose outside fenced code blocks.
	RuleTargetMarkdownBody RuleTarget = "markdown_body"
	// RuleTargetMarkdownCode scans only fenced code blocks in markdown content.
	RuleTargetMarkdownCode RuleTarget = "markdown_code"
)

// Finding is a single rule hit with supporting evidence.
type Finding struct {
	Tag         RiskTag   `json:"tag"`
	Severity    RiskLevel `json:"severity"`
	Description string    `json:"description"`
	File        string    `json:"file"`
	Line        int       `json:"line"`
	Match       string    `json:"match,omitempty"`
	Context     string    `json:"context,omitempty"`
}

// ResultMetadata captures non-policy scan metadata.
type ResultMetadata struct {
	ScannerVersion string    `json:"scanner_version"`
	FilesScanned   int       `json:"files_scanned"`
	ScanDurationMS int64     `json:"scan_duration_ms"`
	ScanTime       time.Time `json:"scan_time"`
}

// Result is the full scanner output for a file or directory.
type Result struct {
	TargetPath   string         `json:"target_path"`
	TargetKind   TargetKind     `json:"target_kind"`
	ArtifactHash string         `json:"artifact_hash"`
	RiskLevel    RiskLevel      `json:"risk_level"`
	RiskTags     []RiskTag      `json:"risk_tags"`
	Findings     []Finding      `json:"findings"`
	Summary      string         `json:"summary"`
	Metadata     ResultMetadata `json:"metadata"`
}

// SkillIdentity binds a scan request to a caller-supplied skill identity.
type SkillIdentity struct {
	ID           string `json:"id,omitempty"`
	Source       string `json:"source,omitempty"`
	VersionRef   string `json:"version_ref,omitempty"`
	ArtifactHash string `json:"artifact_hash,omitempty"`
}

// PayloadType describes the type of input a scan request references.
type PayloadType string

const (
	// PayloadTypeDir scans a local directory bundle.
	PayloadTypeDir PayloadType = "dir"
	// PayloadTypeFile scans a single local file.
	PayloadTypeFile PayloadType = "file"
	// PayloadTypeZip is reserved for future archive scanning support.
	PayloadTypeZip PayloadType = "zip"
	// PayloadTypeRepoURL is reserved for future remote repository scanning support.
	PayloadTypeRepoURL PayloadType = "repo_url"
)

// RequestOptions captures per-request scanner options.
type RequestOptions struct {
	// LanguageHint is reserved for future language-specific tuning.
	LanguageHint []string `json:"language_hint,omitempty"`
	// Deep is reserved for future deeper analysis modes.
	Deep bool `json:"deep,omitempty"`
}

// ScanPayload is a higher-level scan request shape modeled after agentguard's scanner API.
type ScanPayload struct {
	Skill   SkillIdentity  `json:"skill"`
	Payload PayloadRef     `json:"payload"`
	Options RequestOptions `json:"options,omitempty"`
}

// PayloadRef points at the thing the scanner should inspect.
type PayloadRef struct {
	Type PayloadType `json:"type"`
	Ref  string      `json:"ref"`
}

// QuickResult is a compact summary used for cheap preflight scans.
type QuickResult struct {
	ArtifactHash string    `json:"artifact_hash"`
	RiskLevel    RiskLevel `json:"risk_level"`
	RiskTags     []RiskTag `json:"risk_tags"`
	Summary      string    `json:"summary"`
}

// Options configures scanner behavior.
type Options struct {
	// AdditionalRules appends caller-defined rules after the built-in rulepack.
	AdditionalRules []Rule
	// MaxMatchLength truncates stored match text in findings.
	MaxMatchLength int
}
