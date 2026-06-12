// Package assets provides embedded static assets for the grid-worker daemon.
package assets

import _ "embed"

// DefaultRulesetYAML contains the embedded default secret scanner ruleset.
//
//go:embed default-ruleset.yaml
var DefaultRulesetYAML []byte
