package productreviews

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
)

// configFileName is searched for by walking up from the working directory.
// Override with the PRODUCT_REVIEW_CONFIG_FILE env var. Same shape and locale
// resolution as doc-review.local.toml.
const configFileName = "product-review.local.toml"

// Built-in budget defaults, used when the config omits a value or has no file.
const (
	defaultMaxDepth       = 3
	defaultMaxNodes       = 200
	defaultMaxDocuments   = 400
	defaultMaxResults     = 2000
	defaultPerDocumentCap = 60
)

// ModelsConfig names the two model refs the app may call (see design D8).
type ModelsConfig struct {
	Structure     string `toml:"structure"`
	AspectMapping string `toml:"aspect_mapping"`
}

// BudgetsConfig bounds profile construction and per-run retrieval.
type BudgetsConfig struct {
	MaxDepth       int `toml:"max_depth"`
	MaxNodes       int `toml:"max_nodes"`
	MaxDocuments   int `toml:"max_documents"`
	MaxResults     int `toml:"max_results"`
	PerDocumentCap int `toml:"per_document_cap"`
}

func (b BudgetsConfig) withDefaults() BudgetsConfig {
	if b.MaxDepth <= 0 {
		b.MaxDepth = defaultMaxDepth
	}
	if b.MaxNodes <= 0 {
		b.MaxNodes = defaultMaxNodes
	}
	if b.MaxDocuments <= 0 {
		b.MaxDocuments = defaultMaxDocuments
	}
	if b.MaxResults <= 0 {
		b.MaxResults = defaultMaxResults
	}
	if b.PerDocumentCap <= 0 {
		b.PerDocumentCap = defaultPerDocumentCap
	}
	return b
}

// GroundingConfig holds the thresholds a node must clear to ground to a source.
type GroundingConfig struct {
	ObjectEmbeddingMin float64 `toml:"object_embedding_min"`
	ConceptLabelMin    float64 `toml:"concept_label_min"`
	OntologyLabelMin   float64 `toml:"ontology_label_min"`
}

// ScoringConfig weights document-scope matches (used by retrieval, Phase 3).
type ScoringConfig struct {
	StandardsBoost      float64            `toml:"standards_boost"`
	RelationTypeWeights map[string]float64 `toml:"relation_type_weights"`
}

// AspectConfig is one [aspects.<key>] block. An aspect either declares the
// relation_types it maps to (matching is an indexed join) or sets
// match_mode = "lexical".
type AspectConfig struct {
	NameEN        string   `toml:"name_en"`
	NameZhCN      string   `toml:"name_zh_cn"`
	DescEN        string   `toml:"desc_en"`
	DescZhCN      string   `toml:"desc_zh_cn"`
	RelationTypes []string `toml:"relation_types"`
	MatchMode     string   `toml:"match_mode"`
}

// LocalizedName returns the aspect name for locale, falling back to name_en.
func (a AspectConfig) LocalizedName(locale string) string {
	return pickLocale(locale, a.NameEN, a.NameZhCN)
}

// LocalizedDesc returns the aspect description for locale, falling back to desc_en.
func (a AspectConfig) LocalizedDesc(locale string) string {
	return pickLocale(locale, a.DescEN, a.DescZhCN)
}

// ResolvedMatchMode is "join" when the aspect declares relation types, else
// "lexical". An explicit match_mode = "lexical" always wins.
func (a AspectConfig) ResolvedMatchMode() string {
	if strings.EqualFold(strings.TrimSpace(a.MatchMode), MatchModeLexical) {
		return MatchModeLexical
	}
	if len(a.RelationTypes) > 0 {
		return MatchModeJoin
	}
	return MatchModeLexical
}

// Config is the parsed product-review.local.toml.
type Config struct {
	Models      ModelsConfig            `toml:"models"`
	Budgets     BudgetsConfig           `toml:"budgets"`
	Grounding   GroundingConfig         `toml:"grounding"`
	Scoring     ScoringConfig           `toml:"scoring"`
	Aspects     map[string]AspectConfig `toml:"aspects"`
	aspectOrder []string                `toml:"-"`
}

// AspectEntry is one resolved aspect for the vocabulary endpoint.
type AspectEntry struct {
	AspectKey     string   `json:"aspect_key"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	RelationTypes []string `json:"relation_types"`
	MatchMode     string   `json:"match_mode"`
}

// AspectVocabulary returns the configured aspects in file order, with Name and
// Description resolved for locale (empty locale → English).
func (c *Config) AspectVocabulary(locale string) []AspectEntry {
	if c == nil {
		return nil
	}
	out := make([]AspectEntry, 0, len(c.aspectOrder))
	for _, key := range c.aspectOrder {
		a, ok := c.Aspects[key]
		if !ok {
			continue
		}
		out = append(out, AspectEntry{
			AspectKey:     key,
			Name:          a.LocalizedName(locale),
			Description:   a.LocalizedDesc(locale),
			RelationTypes: append([]string(nil), a.RelationTypes...),
			MatchMode:     a.ResolvedMatchMode(),
		})
	}
	return out
}

// normalizeLocale canonicalizes a locale tag to lower-case with a hyphen
// separator (e.g. "zh_CN" → "zh-cn"), matching the paraglide locales.
func normalizeLocale(locale string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(locale)), "_", "-")
}

// pickLocale resolves a localized string, falling back to the English value.
// New languages are added by extending this switch and the *_<locale> fields.
func pickLocale(locale, en, zhCN string) string {
	switch normalizeLocale(locale) {
	case "zh-cn":
		if v := strings.TrimSpace(zhCN); v != "" {
			return v
		}
	}
	return strings.TrimSpace(en)
}

var (
	cfgOnce sync.Once
	cfg     *Config
	cfgErr  error
)

// GetConfig loads and caches product-review.local.toml. It returns (nil, nil)
// when no config file is present so callers can fall back to built-in defaults.
func GetConfig() (*Config, error) {
	cfgOnce.Do(func() { cfg, cfgErr = loadConfig() })
	return cfg, cfgErr
}

func loadConfig() (*Config, error) {
	path, ok := resolveConfigPath()
	if !ok {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("(CWB-PMR-CFG-01) read %s: %w", path, err)
	}
	return ParseConfig(raw)
}

// ParseConfig parses config bytes and fills in budget defaults. Exported for tests.
func ParseConfig(raw []byte) (*Config, error) {
	var c Config
	if err := toml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("(CWB-PMR-CFG-02) parse: %w", err)
	}
	c.Budgets = c.Budgets.withDefaults()
	c.aspectOrder = aspectOrderFromTOML(raw, c.Aspects)
	return &c, nil
}

// aspectOrderFromTOML recovers the declaration order of [aspects.<key>] blocks;
// go-toml unmarshals a table into an unordered map.
func aspectOrderFromTOML(raw []byte, aspects map[string]AspectConfig) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "[aspects.") || !strings.HasSuffix(trimmed, "]") {
			continue
		}
		key := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(trimmed, "[aspects."), "]"), `"' `)
		if key == "" || seen[key] {
			continue
		}
		if _, ok := aspects[key]; !ok {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func resolveConfigPath() (string, bool) {
	if override := strings.TrimSpace(os.Getenv("PRODUCT_REVIEW_CONFIG_FILE")); override != "" {
		if _, err := os.Stat(override); err == nil {
			return override, true
		}
		return "", false
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for cur := wd; ; {
		candidate := filepath.Join(cur, configFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}
