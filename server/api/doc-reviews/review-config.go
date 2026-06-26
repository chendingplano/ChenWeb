package docreviews

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// docReviewConfigFileName is the default file name searched for, walking up from
// the working directory. Override with the DOC_REVIEW_CONFIG_FILE env var.
const docReviewConfigFileName = "doc-review.local.toml"

// ReviewPackageConfig is a group-level ([packages.P1..P6]) default block.
type ReviewPackageConfig struct {
	Enabled       bool   `toml:"enabled"`
	Name          string `toml:"name"`
	Desc          string `toml:"desc"`
	Strategy      string `toml:"strategy"`
	Model         string `toml:"model"`
	MaxToolTurns  int    `toml:"max_tool_turns"`
	MaxToolTokens int    `toml:"max_tool_tokens"`
}

// ReviewAspectConfig is a per-aspect ([reviewers.<aspect>]) override block (DR3).
// Pointer fields distinguish "unset" (inherit the group default) from a zero
// value that was explicitly configured.
type ReviewAspectConfig struct {
	Enabled       *bool    `toml:"enabled"`
	Checked       *bool    `toml:"checked"`
	Group         string   `toml:"group"`
	Model         string   `toml:"model"`
	Prompt        string   `toml:"prompt"`
	MaxToolTurns  *int     `toml:"max_tool_turns"`
	MaxToolTokens *int     `toml:"max_tool_tokens"`
	Tools         []string `toml:"tools"`
}

// DocReviewConfig is the parsed doc-review.local.toml.
type DocReviewConfig struct {
	Packages     map[string]ReviewPackageConfig `toml:"packages"`
	Reviewers    map[string]ReviewAspectConfig  `toml:"reviewers"`
	PackageOrder []ReviewPackageInfo            `toml:"-"`
}

// ResolvedReviewerConfig is the effective configuration for one aspect after
// merging its group (package) defaults with its per-aspect overrides.
type ResolvedReviewerConfig struct {
	Aspect        string
	Group         string
	Enabled       bool
	ModelRef      string
	PromptRef     string
	MaxToolTurns  int
	MaxToolTokens int
	Tools         []string
	Found         bool // true if the aspect had a [reviewers.<aspect>] block
}

var (
	docReviewCfgOnce sync.Once
	docReviewCfg     *DocReviewConfig
	docReviewCfgErr  error
)

// GetDocReviewConfig loads and caches doc-review.local.toml. It returns
// (nil, nil) when no config file is present so callers can fall back to their
// existing (env-based) configuration.
func GetDocReviewConfig() (*DocReviewConfig, error) {
	docReviewCfgOnce.Do(func() {
		docReviewCfg, docReviewCfgErr = loadDocReviewConfig()
	})
	return docReviewCfg, docReviewCfgErr
}

// loadDocReviewConfig finds and parses the doc-review config file.
func loadDocReviewConfig() (*DocReviewConfig, error) {
	path, ok := resolveDocReviewConfigPath()
	if !ok {
		return nil, nil // no file — callers fall back to env defaults
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("(MID_26061810) read %s failed: %w", path, err)
	}
	var cfg DocReviewConfig
	if err := parseTOMLMap(raw, &cfg); err != nil {
		return nil, fmt.Errorf("(MID_26061811) parse %s failed: %w", path, err)
	}
	cfg.PackageOrder = packageOrderFromTOML(raw, cfg.Packages)
	return &cfg, nil
}

func packageOrderFromTOML(raw []byte, packages map[string]ReviewPackageConfig) []ReviewPackageInfo {
	seen := map[string]bool{}
	var out []ReviewPackageInfo
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "[packages.") || !strings.HasSuffix(trimmed, "]") {
			continue
		}
		key := strings.TrimSuffix(strings.TrimPrefix(trimmed, "[packages."), "]")
		key = strings.Trim(key, `"' `)
		if key == "" || seen[key] {
			continue
		}
		pkg, ok := packages[key]
		if !ok {
			continue
		}
		seen[key] = true
		label := strings.TrimSpace(pkg.Name)
		if label == "" {
			label = key
		}
		out = append(out, ReviewPackageInfo{Key: key, Label: label})
	}
	return out
}

func configuredPackageOrder() []ReviewPackageInfo {
	cfg, err := GetDocReviewConfig()
	if err != nil || cfg == nil || len(cfg.PackageOrder) == 0 {
		return defaultReviewPackageOrder()
	}
	return append([]ReviewPackageInfo(nil), cfg.PackageOrder...)
}

func defaultReviewPackageOrder() []ReviewPackageInfo {
	return []ReviewPackageInfo{
		{Key: "P1", Label: "Language & Style"},
		{Key: "P2", Label: "Structure & Organization"},
		{Key: "P3", Label: "Content Quality"},
		{Key: "P4", Label: "Consistency"},
		{Key: "P5", Label: "Technical & Compliance"},
		{Key: "P6", Label: "Meta & Process"},
	}
}

// resolveDocReviewConfigPath honors DOC_REVIEW_CONFIG_FILE, otherwise walks up
// the directory tree from the working directory looking for the default file.
func resolveDocReviewConfigPath() (string, bool) {
	if override := strings.TrimSpace(os.Getenv("DOC_REVIEW_CONFIG_FILE")); override != "" {
		if _, err := os.Stat(override); err == nil {
			return override, true
		}
		return "", false
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	cur := wd
	for {
		candidate := filepath.Join(cur, docReviewConfigFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", false
}

// ResolveReviewer returns the effective config for one aspect, merging the
// [reviewers.<aspect>] block with its group's [packages.<group>] defaults.
// group is the fallback group key (e.g. "P1") used when the aspect block omits
// `group`. Returns Found=false when neither the aspect nor its package exist.
func (c *DocReviewConfig) ResolveReviewer(aspect, group string) ResolvedReviewerConfig {
	out := ResolvedReviewerConfig{Aspect: aspect, Group: group, Enabled: true}
	if c == nil {
		out.Found = false
		return out
	}

	rev, hasRev := c.Reviewers[aspect]
	if hasRev && strings.TrimSpace(rev.Group) != "" {
		out.Group = strings.TrimSpace(rev.Group)
	}

	// Start from the group (package) defaults.
	if pkg, ok := c.Packages[out.Group]; ok {
		out.Enabled = pkg.Enabled
		out.ModelRef = pkg.Model
		out.MaxToolTurns = pkg.MaxToolTurns
		out.MaxToolTokens = pkg.MaxToolTokens
		out.Found = true
	}

	// Apply per-aspect overrides on top.
	if hasRev {
		out.Found = true
		if rev.Enabled != nil {
			out.Enabled = *rev.Enabled
		}
		if strings.TrimSpace(rev.Model) != "" {
			out.ModelRef = strings.TrimSpace(rev.Model)
		}
		if strings.TrimSpace(rev.Prompt) != "" {
			out.PromptRef = strings.TrimSpace(rev.Prompt)
		}
		if rev.MaxToolTurns != nil {
			out.MaxToolTurns = *rev.MaxToolTurns
		}
		if rev.MaxToolTokens != nil {
			out.MaxToolTokens = *rev.MaxToolTokens
		}
		if len(rev.Tools) > 0 {
			out.Tools = append([]string(nil), rev.Tools...)
		}
	}
	return out
}
