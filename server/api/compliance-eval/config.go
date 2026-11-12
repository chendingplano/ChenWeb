package complianceeval

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// OrgFact is one organization fact with provenance (research 2026111103 §6.1).
type OrgFact struct {
	Value  []string `toml:"value" json:"value"`
	Status string   `toml:"status" json:"status"`
	Source string   `toml:"source" json:"source"`
}

// OrgFactNames are the organization facts the gold run records. An absent fact is "unknown".
var OrgFactNames = []string{
	"operating_territories", "legal_entities", "regulated_roles", "sectors",
	"data_categories", "deployment_regions", "adopted_standards",
}

var orgStatuses = map[string]bool{"asserted": true, "inferred": true, "disputed": true, "unknown": true}

// GoldConfig is the [compliance-review.*] part of doc-review.local.toml.
type GoldConfig struct {
	Covered      map[string]string
	Uncovered    map[string]string
	Organization map[string]OrgFact
}

// GoldDoc is one document the gold run labels.
type GoldDoc struct {
	RecordID int64
	Covered  bool // listed under covered-docs
	Reason   string
}

type goldFile struct {
	ComplianceReview struct {
		CoveredDocs   map[string]string  `toml:"covered-docs"`
		UncoveredDocs map[string]string  `toml:"uncovered-docs"`
		Organization  map[string]OrgFact `toml:"organization"`
	} `toml:"compliance-review"`
}

// ParseGoldConfig reads the gold tables from the config file text and validates them.
func ParseGoldConfig(raw []byte) (*GoldConfig, error) {
	var f goldFile
	if err := toml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse compliance-review config: %w", err)
	}
	c := &GoldConfig{
		Covered:      f.ComplianceReview.CoveredDocs,
		Uncovered:    f.ComplianceReview.UncoveredDocs,
		Organization: f.ComplianceReview.Organization,
	}
	known := map[string]bool{}
	for _, n := range OrgFactNames {
		known[n] = true
	}
	for name, fact := range c.Organization {
		if !known[name] {
			return nil, fmt.Errorf("[compliance-review.organization]: unknown fact %q (known: %s)", name, strings.Join(OrgFactNames, ", "))
		}
		if !orgStatuses[fact.Status] {
			return nil, fmt.Errorf("[compliance-review.organization] %s: status %q must be asserted, inferred, disputed or unknown", name, fact.Status)
		}
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

// OrganizationContext returns every known fact, filling absent ones as unknown.
func (c *GoldConfig) OrganizationContext() map[string]OrgFact {
	out := make(map[string]OrgFact, len(OrgFactNames))
	for _, n := range OrgFactNames {
		if f, ok := c.Organization[n]; ok {
			out[n] = f
		} else {
			out[n] = OrgFact{Value: []string{}, Status: "unknown"}
		}
	}
	return out
}
