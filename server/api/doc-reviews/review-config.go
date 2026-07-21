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

var defaultReviewOutputLimits = [3]int{100, 200, 300}

// ReviewPackageConfig is a group-level ([packages.P1..P6]) default block.
//
// Names and descriptions are internationalized: `name_en`/`desc_en` are the
// default (fallback) values, `name_zh_cn`/`desc_zh_cn` the Simplified-Chinese
// variants. More languages can be added as additional `name_<locale>` fields.
// Use LocalizedName / LocalizedDesc to resolve the right one for a locale.
type ReviewPackageConfig struct {
	Enabled       bool   `toml:"enabled"`
	NameEN        string `toml:"name_en"`
	NameZhCN      string `toml:"name_zh_cn"`
	DescEN        string `toml:"desc_en"`
	DescZhCN      string `toml:"desc_zh_cn"`
	Strategy      string `toml:"strategy"`
	Model         string `toml:"model"`
	MaxToolTurns  int    `toml:"max_tool_turns"`
	MaxToolTokens int    `toml:"max_tool_tokens"`
}

// LocalizedName returns the package name for locale, falling back to name_en.
func (p ReviewPackageConfig) LocalizedName(locale string) string {
	return pickLocale(locale, p.NameEN, p.NameZhCN)
}

// LocalizedDesc returns the package description for locale, falling back to desc_en.
func (p ReviewPackageConfig) LocalizedDesc(locale string) string {
	return pickLocale(locale, p.DescEN, p.DescZhCN)
}

// ReviewAspectConfig is a per-aspect ([reviewers.<aspect>]) override block (DR3).
// Pointer fields distinguish "unset" (inherit the group default) from a zero
// value that was explicitly configured.
type ReviewAspectConfig struct {
	Enabled       *bool    `toml:"enabled"`
	Checked       *bool    `toml:"checked"`
	Group         string   `toml:"group"`
	NameEN        string   `toml:"name_en"`
	NameZhCN      string   `toml:"name_zh_cn"`
	DescEN        string   `toml:"desc_en"`
	DescZhCN      string   `toml:"desc_zh_cn"`
	Input         string   `toml:"input"` // "per-chunk" or "per-block"
	Model         string   `toml:"model"`
	Prompt        string   `toml:"prompt"`
	MaxToolTurns  *int     `toml:"max_tool_turns"`
	MaxToolTokens *int     `toml:"max_tool_tokens"`
	MaxFindings   []int    `toml:"max_findings"`
	MaxAnalyses   []int    `toml:"max_analyses"`
	Tools         []string `toml:"tools"`
}

// LocalizedName returns the aspect (reviewer) label for locale, falling back to
// name_en. Empty when neither is configured (callers keep the built-in default).
func (a ReviewAspectConfig) LocalizedName(locale string) string {
	return pickLocale(locale, a.NameEN, a.NameZhCN)
}

// LocalizedDesc returns the aspect description for locale, falling back to desc_en.
func (a ReviewAspectConfig) LocalizedDesc(locale string) string {
	return pickLocale(locale, a.DescEN, a.DescZhCN)
}

// normalizeLocale canonicalizes a locale tag to lower-case with a hyphen
// separator (e.g. "zh_CN", "ZH-cn" → "zh-cn"), matching the paraglide locales.
func normalizeLocale(locale string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(locale)), "_", "-")
}

// pickLocale resolves a localized string for the given locale, falling back to
// the English (default) value. New languages are added by extending this switch
// alongside a new `*_<locale>` struct field.
func pickLocale(locale, en, zhCN string) string {
	switch normalizeLocale(locale) {
	case "zh-cn":
		if v := strings.TrimSpace(zhCN); v != "" {
			return v
		}
	}
	return strings.TrimSpace(en)
}

// DocReviewConfig is the parsed doc-review.local.toml.
type DocReviewConfig struct {
	MaxFindings  []int                          `toml:"max_findings"`
	MaxAnalyses  []int                          `toml:"max_analyses"`
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
	cfg, err := parseDocReviewConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("(MID_26061811) parse %s failed: %w", path, err)
	}
	cfg.PackageOrder = packageOrderFromTOML(raw, cfg.Packages)
	return cfg, nil
}

func parseDocReviewConfig(raw []byte) (*DocReviewConfig, error) {
	var cfg DocReviewConfig
	if err := parseTOMLMap(raw, &cfg); err != nil {
		return nil, err
	}
	if err := validateOutputLimitArray("root", "max_findings", cfg.MaxFindings); err != nil {
		return nil, err
	}
	if err := validateOutputLimitArray("root", "max_analyses", cfg.MaxAnalyses); err != nil {
		return nil, err
	}
	for aspect, reviewer := range cfg.Reviewers {
		scope := "reviewer " + aspect
		if err := validateOutputLimitArray(scope, "max_findings", reviewer.MaxFindings); err != nil {
			return nil, err
		}
		if err := validateOutputLimitArray(scope, "max_analyses", reviewer.MaxAnalyses); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

func validateOutputLimitArray(scope, name string, values []int) error {
	if values == nil {
		return nil
	}
	if len(values) != len(defaultReviewOutputLimits) {
		return fmt.Errorf("%s %s must contain exactly %d positive integers", scope, name, len(defaultReviewOutputLimits))
	}
	for _, value := range values {
		if value <= 0 {
			return fmt.Errorf("%s %s must contain exactly %d positive integers", scope, name, len(defaultReviewOutputLimits))
		}
	}
	return nil
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
		label := strings.TrimSpace(pkg.NameEN)
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

// localizedPackageOrder returns the configured package groups in display order
// with each Label resolved for locale (falling back to name_en). It reuses the
// same ordering as configuredPackageOrder; only the labels are localized.
func localizedPackageOrder(locale string) []ReviewPackageInfo {
	order := configuredPackageOrder()
	cfg, err := GetDocReviewConfig()
	if err != nil || cfg == nil {
		return order
	}
	for i := range order {
		if pkg, ok := cfg.Packages[order[i].Key]; ok {
			if label := pkg.LocalizedName(locale); label != "" {
				order[i].Label = label
			}
		}
	}
	return order
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

// ResolveOutputLimits returns the findings and analyses limits for an aspect
// at the requested review depth. Root settings override built-in defaults, and
// each reviewer can independently override either root setting.
func (c *DocReviewConfig) ResolveOutputLimits(aspect string, depth int) (int, int) {
	if depth < 1 || depth > len(defaultReviewOutputLimits) {
		depth = 1
	}
	index := depth - 1
	maxFindings := defaultReviewOutputLimits[index]
	maxAnalyses := defaultReviewOutputLimits[index]
	if c == nil {
		return maxFindings, maxAnalyses
	}
	if len(c.MaxFindings) == len(defaultReviewOutputLimits) {
		maxFindings = c.MaxFindings[index]
	}
	if len(c.MaxAnalyses) == len(defaultReviewOutputLimits) {
		maxAnalyses = c.MaxAnalyses[index]
	}
	if reviewer, ok := c.Reviewers[aspect]; ok {
		if len(reviewer.MaxFindings) == len(defaultReviewOutputLimits) {
			maxFindings = reviewer.MaxFindings[index]
		}
		if len(reviewer.MaxAnalyses) == len(defaultReviewOutputLimits) {
			maxAnalyses = reviewer.MaxAnalyses[index]
		}
	}
	return maxFindings, maxAnalyses
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
