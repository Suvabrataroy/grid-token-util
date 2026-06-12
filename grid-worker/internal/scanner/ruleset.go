package scanner

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/grid-computing/grid-worker/internal/assets"
)

// Rule defines a single secret-detection rule.
type Rule struct {
	ID       string         `yaml:"id"`
	Name     string         `yaml:"name"`
	Pattern  *regexp.Regexp `yaml:"-"`
	RawPattern string       `yaml:"pattern"`
	Severity string         `yaml:"severity"` // critical | high | medium | low
	Scope    string         `yaml:"scope"`    // all | diff_only | workspace_only
}

// ruleYAML is the intermediate YAML representation of a rule.
type ruleYAML struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Severity string `yaml:"severity"`
	Scope    string `yaml:"scope"`
}

// Ruleset is the complete secret scanner ruleset.
type Ruleset struct {
	Version    string  `yaml:"version"`
	Rules      []Rule  `yaml:"-"`
	Exclusions struct {
		Paths    []string `yaml:"paths"`
		Patterns []string `yaml:"patterns"`
	} `yaml:"exclusions"`
}

// rulesetYAML is the intermediate YAML representation.
type rulesetYAML struct {
	Version string     `yaml:"version"`
	Rules   []ruleYAML `yaml:"rules"`
	Exclusions struct {
		Paths    []string `yaml:"paths"`
		Patterns []string `yaml:"patterns"`
	} `yaml:"exclusions"`
}

// LoadRuleset loads and parses a ruleset from a YAML file at path.
func LoadRuleset(path string) (*Ruleset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ruleset file %q: %w", path, err)
	}
	return parseRuleset(data)
}

// DefaultRuleset returns the built-in embedded ruleset.
func DefaultRuleset() *Ruleset {
	data := assets.DefaultRulesetYAML
	if len(data) == 0 {
		return hardcodedRuleset()
	}

	rs, err := parseRuleset(data)
	if err != nil {
		return hardcodedRuleset()
	}

	return rs
}

// parseRuleset parses YAML data into a Ruleset, compiling all regex patterns.
func parseRuleset(data []byte) (*Ruleset, error) {
	var raw rulesetYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ruleset YAML: %w", err)
	}

	rs := &Ruleset{
		Version: raw.Version,
	}
	rs.Exclusions.Paths = raw.Exclusions.Paths
	rs.Exclusions.Patterns = raw.Exclusions.Patterns

	for i, r := range raw.Rules {
		compiled, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("rule %d (%s): invalid pattern %q: %w", i, r.ID, r.Pattern, err)
		}

		scope := r.Scope
		if scope == "" {
			scope = "all"
		}

		rs.Rules = append(rs.Rules, Rule{
			ID:         r.ID,
			Name:       r.Name,
			Pattern:    compiled,
			RawPattern: r.Pattern,
			Severity:   r.Severity,
			Scope:      scope,
		})
	}

	return rs, nil
}

// hardcodedRuleset returns the built-in rules when the embedded YAML is unavailable.
func hardcodedRuleset() *Ruleset {
	type ruleSpec struct {
		id, name, pattern, severity, scope string
	}

	specs := []ruleSpec{
		{"SEC-001", "AWS Access Key ID", `(?i)AKIA[0-9A-Z]{16}`, "critical", "all"},
		{"SEC-002", "GitHub Personal Access Token", `ghp_[0-9a-zA-Z]{36}`, "critical", "all"},
		{"SEC-003", "Private Key PEM", `-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----`, "critical", "all"},
		{"SEC-004", "Generic Password Assignment", `(?i)password\s*[:=]\s*\S{8,}`, "high", "all"},
		{"SEC-005", "Generic Secret Assignment", `(?i)secret\s*[:=]\s*\S{8,}`, "high", "all"},
		{"SEC-006", "Bearer Token", `(?i)bearer\s+[a-zA-Z0-9._\-]{20,}`, "medium", "diff_only"},
	}

	rs := &Ruleset{Version: "1"}
	rs.Exclusions.Paths = []string{"**/*.md", "**/testdata/**", "**/*_test.go"}
	rs.Exclusions.Patterns = []string{"EXAMPLE_KEY_PLACEHOLDER", "YOUR_KEY_HERE"}

	for _, s := range specs {
		compiled, err := regexp.Compile(s.pattern)
		if err != nil {
			continue
		}
		rs.Rules = append(rs.Rules, Rule{
			ID:         s.id,
			Name:       s.name,
			Pattern:    compiled,
			RawPattern: s.pattern,
			Severity:   s.severity,
			Scope:      s.scope,
		})
	}

	return rs
}
