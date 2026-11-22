package complianceeval

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// GoldConfig is the [compliance-review.*] part of doc-review.local.toml. There is no organization
// profile: activity context is extracted from the evidence, not supplied by the user (research
// 2026111103, activity context).
type GoldConfig struct {
	// Docs is [compliance-review.gold-docs]: record id -> one-line reason. Whether an activity has
	// grounding comes from the labels, not from the list (design D8, revised 2026-11-14).
	Docs map[string]string
	// MaxCandidates caps the candidates pooled per gold activity ([reviewers.compliance].max_candidates,
	// default DefaultMaxCandidates). It is the labelling workload per activity; the reviewer's own search
	// does not read it.
	MaxCandidates int
	// Panel is [compliance-review.gold-panel]: the prompt files of the model panel.
	Panel PanelConfig
}

// GoldPanelTable is the config table that names the model panel's prompt files.
const GoldPanelTable = "[compliance-review.gold-panel]"

// PanelConfig names the model panel's prompt files (in ChenWeb/prompts) and the version of the
// labelling rules that the rules prompt holds, which is stored with each panel run.
type PanelConfig struct {
	RulesPrompt        string `toml:"rules_prompt"`
	RulesVersion       string `toml:"rules_version"`
	ActivitiesPrompt   string `toml:"activities_prompt"`
	JudgePrompt        string `toml:"judge_prompt"`
	AdjActivityPrompt  string `toml:"adjudicate_activity_prompt"`
	AdjJudgementPrompt string `toml:"adjudicate_judgement_prompt"`
}

// Missing lists the unset settings in table order.
func (c PanelConfig) Missing() []string {
	var out []string
	for _, kv := range [][2]string{
		{"rules_prompt", c.RulesPrompt}, {"rules_version", c.RulesVersion}, {"activities_prompt", c.ActivitiesPrompt},
		{"judge_prompt", c.JudgePrompt}, {"adjudicate_activity_prompt", c.AdjActivityPrompt},
		{"adjudicate_judgement_prompt", c.AdjJudgementPrompt},
	} {
		if kv[1] == "" {
			out = append(out, kv[0])
		}
	}
	return out
}

// DefaultMaxCandidates applies when [reviewers.compliance].max_candidates is absent.
const DefaultMaxCandidates = 100

// GoldDoc is one document the gold run labels.
type GoldDoc struct {
	RecordID int64
	Reason   string
}

// GoldDocsTable is the config table that lists the gold documents.
const GoldDocsTable = "[compliance-review.gold-docs]"

type goldFile struct {
	ComplianceReview struct {
		GoldDocs  map[string]string `toml:"gold-docs"`
		GoldPanel PanelConfig       `toml:"gold-panel"`
	} `toml:"compliance-review"`
	Reviewers struct {
		Compliance struct {
			MaxCandidates *int `toml:"max_candidates"`
		} `toml:"compliance"`
	} `toml:"reviewers"`
}

// ParseGoldConfig reads the gold tables from the config file text and validates them.
func ParseGoldConfig(raw []byte) (*GoldConfig, error) {
	var f goldFile
	if err := toml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse compliance-review config: %w", err)
	}
	// The two tables of the first build were merged into gold-docs on 2026-11-14. Either one, even
	// empty, is refused, so no listed document is dropped and no stale header misleads a reader.
	var tables struct {
		ComplianceReview map[string]any `toml:"compliance-review"`
	}
	if err := toml.Unmarshal(raw, &tables); err != nil {
		return nil, fmt.Errorf("parse compliance-review config: %w", err)
	}
	for _, old := range []string{"covered-docs", "uncovered-docs"} {
		if _, ok := tables.ComplianceReview[old]; ok {
			return nil, fmt.Errorf("[compliance-review.covered-docs] and [compliance-review.uncovered-docs] were merged into %s on 2026-11-14; move their entries there and remove the old tables", GoldDocsTable)
		}
	}
	c := &GoldConfig{Docs: f.ComplianceReview.GoldDocs, MaxCandidates: DefaultMaxCandidates, Panel: f.ComplianceReview.GoldPanel}
	if v := f.Reviewers.Compliance.MaxCandidates; v != nil {
		if *v <= 0 {
			return nil, fmt.Errorf("[reviewers.compliance] max_candidates must be a positive number, got %d", *v)
		}
		c.MaxCandidates = *v
	}
	if _, err := c.Documents(); err != nil {
		return nil, err
	}
	return c, nil
}

// Documents returns the listed documents ordered by record id. A key that is not a record id, or two
// keys naming the same record ("416" and "0416"), is an error naming it.
func (c *GoldConfig) Documents() ([]GoldDoc, error) {
	byID := map[int64]GoldDoc{}
	for k, reason := range c.Docs {
		id, err := strconv.ParseInt(strings.TrimSpace(k), 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("%s: %q is not a record id", GoldDocsTable, k)
		}
		if _, dup := byID[id]; dup {
			return nil, fmt.Errorf("%s: record %d is listed twice", GoldDocsTable, id)
		}
		byID[id] = GoldDoc{RecordID: id, Reason: reason}
	}
	out := make([]GoldDoc, 0, len(byID))
	for _, d := range byID {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordID < out[j].RecordID })
	return out, nil
}

// Find returns the listed document with this record id, or false when the list does not hold it.
func (c *GoldConfig) Find(recordID int64) (GoldDoc, bool) {
	docs, _ := c.Documents()
	for _, d := range docs {
		if d.RecordID == recordID {
			return d, true
		}
	}
	return GoldDoc{}, false
}
