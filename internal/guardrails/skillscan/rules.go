package skillscan

import (
	"regexp"
	"strconv"
	"strings"
)

// Rule defines a single detection rule.
type Rule struct {
	// ID is the stable machine-readable tag emitted in findings/results.
	ID RiskTag
	// Description is a short human-readable summary of the rule's intent.
	Description string
	// Severity contributes to the aggregated risk level when the rule matches.
	Severity RiskLevel
	// FileTypes restricts the rule to matching file extensions; "*" matches all files.
	FileTypes []string
	// Target selects which logical view of the file should be scanned.
	Target RuleTarget
	// Patterns are regexes evaluated line-by-line against the selected view.
	Patterns []*regexp.Regexp
	// Validator can reject false positives using full-content context.
	Validator func(content string, match []string) bool
}

// DefaultRules returns the built-in local skill scanning rules.
func DefaultRules() []Rule {
	return []Rule{
		{
			ID:          RiskTagShellExec,
			Description: "Detects command execution capabilities",
			Severity:    RiskLevelHigh,
			FileTypes:   []string{".js", ".ts", ".jsx", ".tsx", ".mjs", ".cjs", ".py", ".sh", ".bash", ".md"},
			Target:      targetForMarkdownCode(),
			Patterns: compilePatterns(
				`require\s*\(\s*['"`+"`"+`]child_process['"`+"`"+`]\s*\)`,
				`from\s+['"`+"`"+`]child_process['"`+"`"+`]`,
				`\bexec\s*\(`,
				`\bexecSync\s*\(`,
				`\bspawn\s*\(`,
				`\bspawnSync\s*\(`,
				`\bsubprocess\.`,
				`\bos\.system\s*\(`,
				`\bos\.popen\s*\(`,
				`curl.*\|\s*(bash|sh)`,
				`wget.*\|\s*(bash|sh)`,
			),
		},
		{
			ID:          RiskTagRemoteLoad,
			Description: "Detects dynamic code loading from remote sources",
			Severity:    RiskLevelCritical,
			FileTypes:   []string{".js", ".ts", ".jsx", ".tsx", ".mjs", ".cjs", ".py", ".md"},
			Target:      targetForMarkdownCode(),
			Patterns: compilePatterns(
				`import\s*\(\s*[^'"`+"`"+`\s]`,
				`require\s*\(\s*[^'"`+"`"+`\s]`,
				`fetch\s*\([^)]*\)\.then\([^)]*\)\s*\.then\([^)]*eval`,
				`exec\s*\(\s*requests\.get`,
				`eval\s*\(\s*requests\.get`,
				`__import__\s*\(`,
				`importlib\.import_module\s*\(`,
			),
		},
		{
			ID:          RiskTagAutoUpdate,
			Description: "Detects auto-update or remote self-modifying behavior",
			Severity:    RiskLevelCritical,
			FileTypes:   []string{".js", ".ts", ".py", ".sh", ".bash", ".md"},
			Target:      targetForMarkdownCode(),
			Patterns: compilePatterns(
				`cron|schedule|interval.*exec|setInterval.*exec`,
				`auto.?update|self.?update`,
				`download.*execute`,
			),
		},
		{
			ID:          RiskTagReadEnvSecrets,
			Description: "Detects environment secret access",
			Severity:    RiskLevelMedium,
			FileTypes:   []string{".js", ".ts", ".jsx", ".tsx", ".mjs", ".cjs", ".py"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`process\.env\s*\[`,
				`process\.env\.`,
				`os\.environ`,
				`os\.getenv\s*\(`,
				`dotenv\.load_dotenv`,
			),
		},
		{
			ID:          RiskTagReadSSHKeys,
			Description: "Detects access to SSH key material",
			Severity:    RiskLevelCritical,
			FileTypes:   []string{"*"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`~\/\.ssh`,
				`\.ssh\/id_rsa`,
				`\.ssh\/id_ed25519`,
				`\.ssh\/known_hosts`,
				`authorized_keys`,
			),
		},
		{
			ID:          RiskTagReadKeychain,
			Description: "Detects access to keychain/browser credential stores",
			Severity:    RiskLevelCritical,
			FileTypes:   []string{"*"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`keychain`,
				`security\s+find-`,
				`Chrome.*Login\s+Data`,
				`Firefox.*logins\.json`,
				`credential.*manager`,
			),
		},
		{
			ID:          RiskTagNetExfil,
			Description: "Detects generic outbound upload/exfiltration primitives",
			Severity:    RiskLevelHigh,
			FileTypes:   []string{".js", ".ts", ".jsx", ".tsx", ".mjs", ".cjs", ".py", ".md"},
			Target:      targetForMarkdownCode(),
			Patterns: compilePatterns(
				`fetch\s*\([^)]+,\s*\{[^}]*method\s*:\s*['"`+"`"+`]POST['"`+"`"+`]`,
				`axios\.post\s*\(`,
				`requests\.post\s*\(`,
				`new\s+FormData\s*\(`,
				`multipart\/form-data`,
			),
		},
		{
			ID:          RiskTagWebhook,
			Description: "Detects webhook-based exfiltration endpoints",
			Severity:    RiskLevelCritical,
			FileTypes:   []string{"*"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`discord(?:app)?\.com\/api\/webhooks`,
				`api\.telegram\.org\/bot`,
				`hooks\.slack\.com`,
				`webhook\s*[:=]\s*['"`+"`"+`]https?:`,
				`webhook\.site`,
				`pipedream`,
			),
		},
		{
			ID:          RiskTagPromptInjection,
			Description: "Detects prompt injection or instruction override content",
			Severity:    RiskLevelCritical,
			FileTypes:   []string{".md"},
			Target:      RuleTargetMarkdownBody,
			Patterns: compilePatterns(
				`ignore\s+(previous|all|above|prior)\s+(instructions?|rules?|guidelines?)`,
				`disregard\s+(previous|all|above|prior)\s+(instructions?|rules?|guidelines?)`,
				`you\s+are\s+(now|a)\s+(?:DAN|jailbroken|unrestricted)`,
				`(?:no|without|skip)\s+(?:need\s+(?:for\s+)?)?confirm(?:ation)?`,
				`bypass\s+(?:security|safety|restrictions?|confirm)`,
				`自动执行`,
				`无需确认`,
				`忽略(?:之前|所有|上面)(?:的)?(?:指令|规则|说明)`,
			),
		},
		{
			ID:          RiskTagSocialEngineering,
			Description: "Detects manipulative or coercive operator instructions",
			Severity:    RiskLevelMedium,
			FileTypes:   []string{".md"},
			Target:      RuleTargetMarkdownBody,
			Patterns: compilePatterns(
				`CRITICAL\s+REQUIREMENT`,
				`WILL\s+NOT\s+WORK\s+WITHOUT`,
				`MANDATORY.*(?:install|download|run|execute)`,
				`you\s+MUST\s+(?:install|download|run|execute|paste)`,
				`IMPORTANT:\s*(?:you\s+)?must`,
				`必须(?:安装|下载|执行|运行)`,
			),
		},
		{
			ID:          RiskTagTrojanDistribution,
			Description: "Detects trojanized download instructions",
			Severity:    RiskLevelCritical,
			FileTypes:   []string{".md"},
			Target:      RuleTargetMarkdownBody,
			Patterns: compilePatterns(
				`releases\/download\/.*\.(zip|tar|exe|dmg|appimage)`,
				`password\s*[:=]\s*['"`+"`"+`]?\w+['"`+"`"+`]?`,
				`chmod\s+\+x\s`,
				`\.\/\w+.*(?:run|execute|start|launch)`,
			),
			Validator: func(content string, _ []string) bool {
				signals := 0
				if regexp.MustCompile(`https?:\/\/.*(?:releases\/download|\.zip|\.tar|\.exe|\.dmg)`).MatchString(content) {
					signals++
				}
				if regexp.MustCompile(`password\s*[:=]`).MatchString(content) {
					signals++
				}
				if regexp.MustCompile(`(?:chmod\s+\+x|\.\/\w+|run\s+the|execute)`).MatchString(content) {
					signals++
				}
				return signals >= 2
			},
		},
		{
			ID:          RiskTagSuspiciousPasteURL,
			Description: "Detects URLs to paste sites and code-sharing platforms",
			Severity:    RiskLevelHigh,
			FileTypes:   []string{"*"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`glot\.io\/snippets\/`,
				`pastebin\.com\/`,
				`hastebin\.com\/`,
				`paste\.ee\/`,
				`dpaste\.org\/`,
				`rentry\.co\/`,
				`ghostbin\.com\/`,
				`pastie\.io\/`,
			),
		},
		{
			ID:          RiskTagSuspiciousIP,
			Description: "Detects hardcoded public IP addresses",
			Severity:    RiskLevelMedium,
			FileTypes:   []string{"*"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`,
			),
			Validator: func(_ string, match []string) bool {
				if len(match) < 2 {
					return false
				}
				parts := strings.Split(match[1], ".")
				if len(parts) != 4 {
					return false
				}
				values := make([]int, 0, 4)
				for _, part := range parts {
					n, err := strconv.Atoi(part)
					if err != nil || n < 0 || n > 255 {
						return false
					}
					values = append(values, n)
				}
				if values[0] == 127 || values[0] == 0 || values[0] == 10 {
					return false
				}
				if values[0] == 172 && values[1] >= 16 && values[1] <= 31 {
					return false
				}
				if values[0] == 192 && values[1] == 168 {
					return false
				}
				if values[0] == 169 && values[1] == 254 {
					return false
				}
				if values[1] == 0 && values[2] == 0 && values[3] == 0 {
					return false
				}
				return true
			},
		},
		{
			ID:          RiskTagObfuscation,
			Description: "Detects common obfuscation or encoded payload patterns",
			Severity:    RiskLevelHigh,
			FileTypes:   []string{".js", ".ts", ".mjs", ".py", ".md"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`eval\s*\(`,
				`new\s+Function\s*\(`,
				`setTimeout\s*\(\s*['"`+"`"+`]`,
				`setInterval\s*\(\s*['"`+"`"+`]`,
				`atob\s*\([^)]+\).*eval`,
				`Buffer\.from\s*\([^,]+,\s*['"`+"`"+`]base64['"`+"`"+`]\s*\).*eval`,
				`exec\s*\(`,
				`compile\s*\([^)]+,\s*['"`+"`"+`]<[^>]+>['"`+"`"+`],\s*['"`+"`"+`]exec['"`+"`"+`]\s*\)`,
				`\\x[0-9a-fA-F]{2}(?:\\x[0-9a-fA-F]{2}){10,}`,
				`\\u[0-9a-fA-F]{4}(?:\\u[0-9a-fA-F]{4}){10,}`,
				`String\.fromCharCode\s*\(\s*\d+(?:\s*,\s*\d+){10,}\s*\)`,
				`eval\s*\(\s*function\s*\(\s*p\s*,\s*a\s*,\s*c\s*,\s*k\s*,\s*e\s*,\s*[dr]\s*\)`,
			),
		},
		{
			ID:          RiskTagPrivateKeyPattern,
			Description: "Detects hardcoded private keys",
			Severity:    RiskLevelCritical,
			FileTypes:   []string{"*"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`['"`+"`"+`]0x[a-fA-F0-9]{64}['"`+"`"+`]`,
				`private[_\s]?key\s*[:=]\s*['"`+"`"+`]0x[a-fA-F0-9]{64}`,
				`PRIVATE_KEY\s*[:=]\s*['"`+"`"+`][a-fA-F0-9]{64}`,
			),
		},
		{
			ID:          RiskTagMnemonicPattern,
			Description: "Detects hardcoded mnemonic phrases",
			Severity:    RiskLevelCritical,
			FileTypes:   []string{"*"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`['"`+"`"+`]\s*\b(abandon|ability|able|about|above|absent|absorb|abstract|absurd|abuse)\b(\s+\w+){11,23}\s*['"`+"`"+`]`,
				`seed[_\s]?phrase\s*[:=]\s*['"`+"`"+`]`,
				`mnemonic\s*[:=]\s*['"`+"`"+`]`,
				`recovery[_\s]?phrase\s*[:=]\s*['"`+"`"+`]`,
			),
		},
		{
			ID:          RiskTagWalletDraining,
			Description: "Detects wallet draining patterns",
			Severity:    RiskLevelCritical,
			FileTypes:   []string{".js", ".ts", ".sol"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`approve\s*\([^,]+,\s*(type\s*\(\s*uint256\s*\)\s*\.max|0xffffffff|MaxUint256|MAX_UINT)`,
				`transferFrom.*approve|approve.*transferFrom`,
				`permit\s*\(.*deadline`,
			),
		},
		{
			ID:          RiskTagUnlimitedApproval,
			Description: "Detects unlimited token approvals",
			Severity:    RiskLevelHigh,
			FileTypes:   []string{".js", ".ts", ".sol"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`\.approve\s*\([^,]+,\s*ethers\.constants\.MaxUint256`,
				`\.approve\s*\([^,]+,\s*2\s*\*\*\s*256\s*-\s*1`,
				`\.approve\s*\([^,]+,\s*type\(uint256\)\.max`,
				`setApprovalForAll\s*\([^,]+,\s*true\)`,
			),
		},
		{
			ID:          RiskTagDangerousSelfdestruct,
			Description: "Detects selfdestruct in contracts",
			Severity:    RiskLevelHigh,
			FileTypes:   []string{".sol"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`selfdestruct\s*\(`,
				`suicide\s*\(`,
			),
		},
		{
			ID:          RiskTagHiddenTransfer,
			Description: "Detects non-standard transfer implementations",
			Severity:    RiskLevelMedium,
			FileTypes:   []string{".sol"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`function\s+(\w+)[^{]*\{[^}]*\.transfer\s*\(`,
				`\.call\{value:\s*[^}]+\}\s*\(['"`+"`"+`]['"`+"`"+`]\)`,
			),
			Validator: func(_ string, match []string) bool {
				if len(match) > 1 {
					name := strings.ToLower(match[1])
					return name != "transfer" && name != "_transfer"
				}
				return true
			},
		},
		{
			ID:          RiskTagProxyUpgrade,
			Description: "Detects proxy upgrade patterns",
			Severity:    RiskLevelMedium,
			FileTypes:   []string{".sol", ".js", ".ts"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`upgradeTo\s*\(`,
				`upgradeToAndCall\s*\(`,
				`_setImplementation\s*\(`,
				`IMPLEMENTATION_SLOT`,
			),
		},
		{
			ID:          RiskTagFlashLoanRisk,
			Description: "Detects flash loan usage",
			Severity:    RiskLevelMedium,
			FileTypes:   []string{".sol", ".js", ".ts"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`flashLoan\s*\(`,
				`flash\s*Loan`,
				`IFlashLoan`,
				`executeOperation\s*\(`,
				`AAVE.*flash`,
			),
		},
		{
			ID:          RiskTagReentrancyPattern,
			Description: "Detects potential reentrancy vulnerabilities",
			Severity:    RiskLevelHigh,
			FileTypes:   []string{".sol"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`\.call\{[^}]*\}\s*\([^)]*\)[^;]*;[^}]*\w+\s*[+\-*/]?=`,
				`\.transfer\s*\([^)]*\)[^;]*;[^}]*\w+\s*[+\-*/]?=`,
			),
		},
		{
			ID:          RiskTagSignatureReplay,
			Description: "Detects missing replay protection in signatures",
			Severity:    RiskLevelHigh,
			FileTypes:   []string{".sol"},
			Target:      RuleTargetContent,
			Patterns: compilePatterns(
				`ecrecover\s*\([^)]+\)`,
			),
			Validator: func(content string, _ []string) bool {
				fnMatch := regexp.MustCompile(`function\s+\w+[^{]*\{([^}]+ecrecover[^}]+)\}`).FindStringSubmatch(content)
				if len(fnMatch) > 1 {
					return !strings.Contains(strings.ToLower(fnMatch[1]), "nonce")
				}
				return true
			},
		},
	}
}

func compilePatterns(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		// Built-in rules default to case-insensitive matching so prose/code variants
		// do not need separate regexes.
		out = append(out, regexp.MustCompile("(?i)"+pattern))
	}
	return out
}

func targetForMarkdownCode() RuleTarget {
	return RuleTargetMarkdownCode
}
