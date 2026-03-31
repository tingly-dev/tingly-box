package skillscan

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Scanner scans local skill files/directories using built-in and custom rules.
type Scanner struct {
	rules          []Rule
	maxMatchLength int
}

// New creates a new local skill scanner.
func New(opts Options) *Scanner {
	rules := append(DefaultRules(), opts.AdditionalRules...)
	maxMatchLength := opts.MaxMatchLength
	if maxMatchLength <= 0 {
		maxMatchLength = 120
	}
	return &Scanner{
		rules:          rules,
		maxMatchLength: maxMatchLength,
	}
}

// CalculateArtifactHash exposes the deterministic content hash without forcing
// the caller to run a full scan first.
func (s *Scanner) CalculateArtifactHash(path string) (string, error) {
	files, _, err := walkPath(path)
	if err != nil {
		return "", err
	}
	return artifactHash(files), nil
}

// Scan accepts a higher-level payload shape and routes it to the local scanner.
func (s *Scanner) Scan(payload ScanPayload) (Result, error) {
	switch payload.Payload.Type {
	case PayloadTypeDir, PayloadTypeFile:
		return s.ScanPath(payload.Payload.Ref)
	case PayloadTypeZip, PayloadTypeRepoURL:
		return Result{}, fmt.Errorf("unsupported payload type: %s", payload.Payload.Type)
	default:
		return Result{}, fmt.Errorf("unknown payload type: %s", payload.Payload.Type)
	}
}

// ScanPath scans a directory or file path and returns an independent result.
func (s *Scanner) ScanPath(path string) (Result, error) {
	start := time.Now()
	files, kind, err := walkPath(path)
	if err != nil {
		return Result{}, err
	}

	findings := make([]Finding, 0)
	riskTags := make(map[RiskTag]struct{})
	for _, file := range files {
		fileFindings := s.scanFile(file)
		findings = append(findings, fileFindings...)
		for _, finding := range fileFindings {
			riskTags[finding.Tag] = struct{}{}
		}
	}

	resultTags := make([]RiskTag, 0, len(riskTags))
	for tag := range riskTags {
		resultTags = append(resultTags, tag)
	}
	slices.Sort(resultTags)

	result := Result{
		TargetPath:   path,
		TargetKind:   kind,
		ArtifactHash: artifactHash(files),
		RiskLevel:    aggregateRiskLevel(findings),
		RiskTags:     resultTags,
		Findings:     findings,
		Summary:      summarize(resultTags, findings),
		Metadata: ResultMetadata{
			ScannerVersion: ScannerVersion,
			FilesScanned:   len(files),
			ScanDurationMS: time.Since(start).Milliseconds(),
			ScanTime:       time.Now(),
		},
	}
	return result, nil
}

// QuickScan runs a local scan and returns a compact summary result.
func (s *Scanner) QuickScan(path string) (QuickResult, error) {
	hash, err := s.CalculateArtifactHash(path)
	if err != nil {
		return QuickResult{}, err
	}
	result, err := s.ScanPath(path)
	if err != nil {
		return QuickResult{}, err
	}
	return QuickResult{
		ArtifactHash: hash,
		RiskLevel:    result.RiskLevel,
		RiskTags:     result.RiskTags,
		Summary:      result.Summary,
	}, nil
}

func (s *Scanner) scanFile(file fileContent) []Finding {
	findings := make([]Finding, 0)
	contentViews := map[RuleTarget]string{
		RuleTargetContent: file.Content,
	}
	if file.Extension == ".md" {
		// Markdown skills mix operator instructions and embedded code. Splitting the
		// file lets prompt rules inspect prose while execution rules inspect code.
		contentViews[RuleTargetMarkdownBody] = extractMarkdownBody(file.Content)
		contentViews[RuleTargetMarkdownCode] = extractMarkdownCode(file.Content)
	} else {
		contentViews[RuleTargetMarkdownCode] = file.Content
		contentViews[RuleTargetMarkdownBody] = ""
	}

	for _, rule := range s.rules {
		if !ruleAppliesToFile(rule, file.Extension) {
			continue
		}

		view := contentViews[rule.Target]
		if view == "" {
			continue
		}

		findings = append(findings, s.scanContent(rule, view, file.RelativePath, "")...)
	}

	// Encoded payloads are re-scanned against the whole rulepack because hidden
	// prompts or scripts can be embedded in otherwise benign-looking files.
	decodedPayloads := extractAndDecodeBase64(file.Content)
	for _, decoded := range decodedPayloads {
		for _, rule := range s.rules {
			findings = append(findings, s.scanContent(rule, decoded, file.RelativePath, "decoded_from:base64")...)
		}
	}

	return dedupeFindings(findings)
}

// scanContent applies one rule to one logical content view and reports
// line-oriented findings for downstream UIs and policy engines.
func (s *Scanner) scanContent(rule Rule, content, relativePath, context string) []Finding {
	lines := strings.Split(content, "\n")
	findings := make([]Finding, 0)
	for i, line := range lines {
		for _, pattern := range rule.Patterns {
			matches := pattern.FindStringSubmatch(line)
			if len(matches) == 0 {
				continue
			}
			if rule.Validator != nil && !rule.Validator(content, matches) {
				continue
			}

			matchText := strings.TrimSpace(matches[0])
			if len(matchText) > s.maxMatchLength {
				matchText = matchText[:s.maxMatchLength] + "..."
			}
			findings = append(findings, Finding{
				Tag:         rule.ID,
				Severity:    rule.Severity,
				Description: rule.Description,
				File:        relativePath,
				Line:        i + 1,
				Match:       matchText,
				Context:     context,
			})
		}
	}
	return findings
}

// ruleAppliesToFile checks extension gating before any content work is done.
func ruleAppliesToFile(rule Rule, ext string) bool {
	for _, fileType := range rule.FileTypes {
		if fileType == "*" || strings.EqualFold(fileType, ext) {
			return true
		}
	}
	return false
}

// dedupeFindings collapses duplicate hits that can arise from overlapping regexes
// or repeated rescans of identical decoded payloads.
func dedupeFindings(findings []Finding) []Finding {
	if len(findings) <= 1 {
		return findings
	}

	seen := make(map[string]struct{}, len(findings))
	out := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		key := fmt.Sprintf("%s|%s|%d|%s|%s", finding.Tag, finding.File, finding.Line, finding.Match, finding.Context)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, finding)
	}
	return out
}

// aggregateRiskLevel returns the highest severity across all findings.
func aggregateRiskLevel(findings []Finding) RiskLevel {
	level := RiskLevelLow
	for _, finding := range findings {
		if severityWeight(finding.Severity) > severityWeight(level) {
			level = finding.Severity
		}
	}
	return level
}

func severityWeight(level RiskLevel) int {
	switch level {
	case RiskLevelCritical:
		return 4
	case RiskLevelHigh:
		return 3
	case RiskLevelMedium:
		return 2
	default:
		return 1
	}
}

// summarize generates a short operator-facing summary suitable for UI badges/cards.
func summarize(tags []RiskTag, findings []Finding) string {
	if len(findings) == 0 {
		return "No local skill security issues detected"
	}

	parts := make([]string, 0, 4)
	if slices.Contains(tags, RiskTagShellExec) || slices.Contains(tags, RiskTagRemoteLoad) {
		parts = append(parts, "code execution capabilities")
	}
	if slices.Contains(tags, RiskTagPrivateKeyPattern) || slices.Contains(tags, RiskTagMnemonicPattern) {
		parts = append(parts, "hardcoded secrets")
	}
	if slices.Contains(tags, RiskTagWebhook) || slices.Contains(tags, RiskTagNetExfil) {
		parts = append(parts, "data exfiltration risks")
	}
	if slices.Contains(tags, RiskTagPromptInjection) {
		parts = append(parts, "prompt injection instructions")
	}
	if slices.Contains(tags, RiskTagWalletDraining) || slices.Contains(tags, RiskTagUnlimitedApproval) {
		parts = append(parts, "dangerous Web3 patterns")
	}
	if slices.Contains(tags, RiskTagTrojanDistribution) || slices.Contains(tags, RiskTagSocialEngineering) {
		parts = append(parts, "operator manipulation guidance")
	}

	if len(parts) == 0 {
		return fmt.Sprintf("Found %d potential security finding(s)", len(findings))
	}
	return fmt.Sprintf("Found %d potential security finding(s): %s", len(findings), strings.Join(parts, ", "))
}

// extractMarkdownCode keeps line numbers aligned by blanking prose lines instead
// of removing them, so finding line numbers still point to the original file.
func extractMarkdownCode(content string) string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	inBlock := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inBlock = !inBlock
			result = append(result, "")
			continue
		}
		if inBlock {
			result = append(result, line)
		} else {
			result = append(result, "")
		}
	}
	return strings.Join(result, "\n")
}

// extractMarkdownBody is the inverse view of extractMarkdownCode and is used for
// prose-oriented prompt/social-engineering rules.
func extractMarkdownBody(content string) string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	inBlock := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inBlock = !inBlock
			result = append(result, "")
			continue
		}
		if inBlock {
			result = append(result, "")
		} else {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

var base64Pattern = regexp.MustCompile(`(?:['"` + "`" + `]|base64[,\s]+)([A-Za-z0-9+/]{20,}={0,2})(?:['"` + "`" + `]|\s|$)`)

// extractAndDecodeBase64 pulls likely text payloads out of source content so the
// scanner can inspect simple encoded strings without a full decoder pipeline.
func extractAndDecodeBase64(content string) []string {
	matches := base64Pattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	decoded := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(match[1])
		if err != nil {
			continue
		}
		text := string(data)
		if !looksTextual(text) || len(strings.TrimSpace(text)) <= 5 {
			continue
		}
		decoded = append(decoded, text)
	}
	return decoded
}

// looksTextual filters out binary-ish decoded blobs and keeps rescans focused on
// strings that could plausibly contain hidden prompts or scripts.
func looksTextual(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
