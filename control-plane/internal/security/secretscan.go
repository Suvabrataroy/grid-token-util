package security

import (
	"fmt"
	"regexp"
)

// Severity levels for findings.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// Scope identifies where a pattern should be applied.
type Scope string

const (
	ScopeAll     Scope = "all"
	ScopePayload Scope = "payload"
	ScopeOutput  Scope = "output"
)

// Pattern describes a single secret-detection rule.
type Pattern struct {
	ID       string
	Name     string
	Regex    *regexp.Regexp
	Severity Severity
	Scope    Scope
}

// Finding represents a single match produced by the scanner.
type Finding struct {
	PatternID string   `json:"pattern_id"`
	Name      string   `json:"name"`
	Match     string   `json:"match"` // redacted — never contains the full secret
	Severity  Severity `json:"severity"`
}

// Scanner holds a set of compiled patterns and scans text against them.
type Scanner struct {
	patterns []Pattern
}

// NewScanner returns a Scanner loaded with the supplied patterns.
func NewScanner(patterns []Pattern) *Scanner {
	return &Scanner{patterns: patterns}
}

// NewDefaultScanner returns a Scanner pre-loaded with built-in patterns for
// common high-risk secrets.
func NewDefaultScanner() *Scanner {
	return NewScanner(BuiltinPatterns())
}

// BuiltinPatterns returns the default set of secret-detection rules.
func BuiltinPatterns() []Pattern {
	return []Pattern{
		{
			ID:       "aws_access_key",
			Name:     "AWS Access Key ID",
			Regex:    regexp.MustCompile(`(?i)(AKIA|ASIA|AROA)[A-Z0-9]{16}`),
			Severity: SeverityCritical,
			Scope:    ScopeAll,
		},
		{
			ID:       "aws_secret_key",
			Name:     "AWS Secret Access Key",
			Regex:    regexp.MustCompile(`(?i)aws[_\-\s]?secret[_\-\s]?access[_\-\s]?key[\s]*[=:]["']?\s*([A-Za-z0-9/+]{40})`),
			Severity: SeverityCritical,
			Scope:    ScopeAll,
		},
		{
			ID:       "github_token",
			Name:     "GitHub Personal Access Token",
			Regex:    regexp.MustCompile(`(ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{82})`),
			Severity: SeverityCritical,
			Scope:    ScopeAll,
		},
		{
			ID:       "private_key_pem",
			Name:     "Private Key PEM Header",
			Regex:    regexp.MustCompile(`-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`),
			Severity: SeverityCritical,
			Scope:    ScopeAll,
		},
		{
			ID:       "generic_password",
			Name:     "Generic Password Assignment",
			Regex:    regexp.MustCompile(`(?i)password\s*[=:]\s*["']?[^\s"']{8,}["']?`),
			Severity: SeverityHigh,
			Scope:    ScopePayload,
		},
		{
			ID:       "generic_api_key",
			Name:     "Generic API Key Assignment",
			Regex:    regexp.MustCompile(`(?i)(api[_\-]?key|apikey|api[_\-]?secret)\s*[=:]\s*["']?[A-Za-z0-9\-_]{16,}["']?`),
			Severity: SeverityHigh,
			Scope:    ScopeAll,
		},
		{
			ID:       "stripe_secret_key",
			Name:     "Stripe Secret Key",
			Regex:    regexp.MustCompile(`sk_(live|test)_[A-Za-z0-9]{24,}`),
			Severity: SeverityCritical,
			Scope:    ScopeAll,
		},
	}
}

// ScanText checks text against all registered patterns and returns any
// findings.  The Match field in each Finding is redacted to avoid leaking the
// secret in logs or API responses.
func (sc *Scanner) ScanText(text string) ([]Finding, error) {
	if sc == nil {
		return nil, fmt.Errorf("secretscan: nil scanner")
	}

	var findings []Finding
	for _, p := range sc.patterns {
		matches := p.Regex.FindAllString(text, -1)
		for _, m := range matches {
			findings = append(findings, Finding{
				PatternID: p.ID,
				Name:      p.Name,
				Match:     redact(m),
				Severity:  p.Severity,
			})
		}
	}
	return findings, nil
}

// redact replaces the middle characters of a string with asterisks to avoid
// exposing the full secret value in findings.
func redact(s string) string {
	n := len(s)
	if n <= 6 {
		return "***"
	}
	visible := 3
	return s[:visible] + "***" + s[n-visible:]
}
