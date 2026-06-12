package scanner

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// Finding represents a single secret detection finding.
type Finding struct {
	RuleID   string
	RuleName string
	Severity string
	FilePath string
	Line     int
	Match    string // Redacted: shows first 4 chars + "***"
}

// Scanner performs secret scanning using a configured Ruleset.
type Scanner struct {
	ruleset     *Ruleset
	concurrency int
	log         zerolog.Logger
}

// New creates a new Scanner.
func New(ruleset *Ruleset, concurrency int, log zerolog.Logger) *Scanner {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Scanner{
		ruleset:     ruleset,
		concurrency: concurrency,
		log:         log.With().Str("component", "scanner").Logger(),
	}
}

// ScanText scans a text string for secrets, attributing findings to filePath.
func (s *Scanner) ScanText(text, filePath string) ([]Finding, error) {
	if s.isExcludedPath(filePath) {
		return nil, nil
	}

	var findings []Finding
	lines := strings.Split(text, "\n")

	for _, rule := range s.ruleset.Rules {
		for lineNum, line := range lines {
			if s.isExcludedPattern(line) {
				continue
			}

			match := rule.Pattern.FindString(line)
			if match == "" {
				continue
			}

			findings = append(findings, Finding{
				RuleID:   rule.ID,
				RuleName: rule.Name,
				Severity: rule.Severity,
				FilePath: filePath,
				Line:     lineNum + 1,
				Match:    redact(match),
			})
		}
	}

	return findings, nil
}

// ScanDirectory scans all files in a directory tree concurrently.
// Files matching exclusion paths from the ruleset are skipped.
func (s *Scanner) ScanDirectory(ctx context.Context, dir string) ([]Finding, error) {
	// Collect all file paths first
	var filePaths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}
		// Skip files larger than 10MB
		info, err := d.Info()
		if err == nil && info.Size() > 10*1024*1024 {
			return nil
		}
		filePaths = append(filePaths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk directory %q: %w", dir, err)
	}

	type result struct {
		findings []Finding
		err      error
	}

	work := make(chan string, len(filePaths))
	results := make(chan result, len(filePaths))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < s.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range work {
				if ctx.Err() != nil {
					return
				}

				// Compute relative path for exclusion matching
				relPath, _ := filepath.Rel(dir, path)
				if s.isExcludedPath(relPath) {
					continue
				}

				data, err := os.ReadFile(path)
				if err != nil {
					s.log.Debug().Err(err).Str("path", path).Msg("cannot read file for scanning")
					continue
				}

				findings, err := s.ScanText(string(data), relPath)
				results <- result{findings: findings, err: err}
			}
		}()
	}

	// Send work
	for _, p := range filePaths {
		work <- p
	}
	close(work)

	// Wait and close results channel
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect findings
	var allFindings []Finding
	for res := range results {
		if res.err != nil {
			s.log.Warn().Err(res.err).Msg("scan error")
			continue
		}
		allFindings = append(allFindings, res.findings...)
	}

	return allFindings, nil
}

// ScanPatch scans a unified diff patch for secrets.
// Only rules with scope "all" or "diff_only" are applied.
func (s *Scanner) ScanPatch(ctx context.Context, patch string) ([]Finding, error) {
	var findings []Finding
	var currentFile string
	lineNum := 0

	scanner := bufio.NewScanner(strings.NewReader(patch))
	for scanner.Scan() {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		line := scanner.Text()

		// Track current file from diff header
		if strings.HasPrefix(line, "+++ b/") {
			currentFile = strings.TrimPrefix(line, "+++ b/")
			lineNum = 0
			continue
		}
		if strings.HasPrefix(line, "@@") {
			// Parse line number from @@ -old,len +new,len @@
			lineNum = parseDiffLineNum(line)
			continue
		}

		// Only scan added lines (starting with '+', not '+++')
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			if !strings.HasPrefix(line, "-") {
				lineNum++
			}
			continue
		}

		lineNum++
		content := line[1:] // strip leading '+'

		if s.isExcludedPath(currentFile) || s.isExcludedPattern(content) {
			continue
		}

		for _, rule := range s.ruleset.Rules {
			if rule.Scope == "workspace_only" {
				continue
			}
			match := rule.Pattern.FindString(content)
			if match == "" {
				continue
			}
			findings = append(findings, Finding{
				RuleID:   rule.ID,
				RuleName: rule.Name,
				Severity: rule.Severity,
				FilePath: currentFile,
				Line:     lineNum,
				Match:    redact(match),
			})
		}
	}

	return findings, nil
}

// isExcludedPath checks if a file path matches any exclusion path glob.
func (s *Scanner) isExcludedPath(path string) bool {
	for _, pattern := range s.ruleset.Exclusions.Paths {
		matched, _ := filepath.Match(pattern, path)
		if matched {
			return true
		}
		// Also try matching just the filename
		matched, _ = filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}
	}
	return false
}

// isExcludedPattern checks if a line contains any exclusion pattern.
func (s *Scanner) isExcludedPattern(line string) bool {
	for _, pattern := range s.ruleset.Exclusions.Patterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	return false
}

// redact returns a redacted version of a sensitive match: first 4 chars + "***".
func redact(s string) string {
	if len(s) <= 4 {
		return "***"
	}
	return s[:4] + "***"
}

// parseDiffLineNum extracts the new-file line number from a @@ hunk header.
// Format: @@ -old_start,old_len +new_start,new_len @@
func parseDiffLineNum(line string) int {
	// Find the '+' field
	plusIdx := strings.Index(line, "+")
	if plusIdx < 0 {
		return 0
	}
	rest := line[plusIdx+1:]
	// Read until comma or space
	end := strings.IndexAny(rest, ", @")
	if end < 0 {
		end = len(rest)
	}
	numStr := rest[:end]
	n := 0
	for _, c := range numStr {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	if n > 0 {
		return n - 1 // will be incremented on first line
	}
	return 0
}
