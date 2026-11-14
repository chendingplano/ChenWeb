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
	Covered   map[string]string
	Uncovered map[string]string
	// MaxCandidates caps the candidates pooled per gold activity ([reviewers.compliance].max_candidates,
	// default DefaultMaxCandidates). It is the labelling workload per activity; the reviewer's own search
	// does not read it.
	MaxCandidates int
}

// DefaultMaxCandidates applies when [reviewers.compliance].max_candidates is absent.
const DefaultMaxCandidates = 100

// GoldDoc is one document the gold run labels.
type GoldDoc struct {
	RecordID int64
	Covered  bool // listed under covered-docs
	Reason   string
}

type goldFile struct {
	ComplianceReview struct {
		CoveredDocs   map[string]string `toml:"covered-docs"`
		UncoveredDocs map[string]string `toml:"uncovered-docs"`
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
	c := &GoldConfig{Covered: f.ComplianceReview.CoveredDocs, Uncovered: f.ComplianceReview.UncoveredDocs, MaxCandidates: DefaultMaxCandidates}
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

// Documents returns the listed documents ordered by record id. A key that is not a record id, or a
// record listed in both tables, is an error naming it.
func (c *GoldConfig) Documents() ([]GoldDoc, error) {
	byID := map[int64]GoldDoc{}
	add := func(table string, m map[string]string, covered bool) error {
		for k, reason := range m {
			id, err := strconv.ParseInt(strings.TrimSpace(k), 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("[compliance-review.%s]: %q is not a record id", table, k)
			}
			if prev, dup := byID[id]; dup && prev.Covered != covered {
				return fmt.Errorf("record %d is listed in both covered-docs and uncovered-docs", id)
			}
			byID[id] = GoldDoc{RecordID: id, Covered: covered, Reason: reason}
		}
		return nil
	}
	if err := add("covered-docs", c.Covered, true); err != nil {
		return nil, err
	}
	if err := add("uncovered-docs", c.Uncovered, false); err != nil {
		return nil, err
	}
	out := make([]GoldDoc, 0, len(byID))
	for _, d := range byID {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordID < out[j].RecordID })
	return out, nil
}

// Find returns the listed document with this record id, or false when neither table lists it.
func (c *GoldConfig) Find(recordID int64) (GoldDoc, bool) {
	docs, _ := c.Documents()
	for _, d := range docs {
		if d.RecordID == recordID {
			return d, true
		}
	}
	return GoldDoc{}, false
}
