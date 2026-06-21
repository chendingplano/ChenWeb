package docreview

import (
	"testing"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// When no [doc-reviews] config is present, ListTiers falls back to the built-in
// priority-derived tiers.
func TestListTiersFallsBackToDefaults(t *testing.T) {
	old := appconfig.AppConfig.DocReviews
	t.Cleanup(func() { appconfig.AppConfig.DocReviews = old })
	appconfig.AppConfig.DocReviews = nil

	tiers := ListTiers()
	if len(tiers) != 4 {
		t.Fatalf("expected 4 default tiers, got %d", len(tiers))
	}
	if tiers[0].Key != "must_review" || tiers[0].Label != "Must Review" {
		t.Fatalf("unexpected first tier: %+v", tiers[0])
	}
}

// When [doc-reviews] is configured, ListTiers builds tiers from it: normalizing
// hyphenated keys, dropping unknown aspect names, ordering built-ins first then
// extras alphabetically, and deriving labels for custom tiers.
func TestListTiersFromConfig(t *testing.T) {
	old := appconfig.AppConfig.DocReviews
	t.Cleanup(func() { appconfig.AppConfig.DocReviews = old })

	appconfig.AppConfig.DocReviews = map[string][]string{
		"should-review": {"readability"},
		"must-review":   {"logical_flow", "completeness", "not_a_real_aspect"},
		"quick-pass":    {"clarity"},
	}

	tiers := ListTiers()
	if len(tiers) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(tiers))
	}

	// Built-in order first: must_review, should_review; then extras: quick_pass.
	if tiers[0].Key != "must_review" || tiers[1].Key != "should_review" || tiers[2].Key != "quick_pass" {
		t.Fatalf("unexpected order: %s, %s, %s", tiers[0].Key, tiers[1].Key, tiers[2].Key)
	}

	// Unknown aspect name dropped.
	if len(tiers[0].AspectNames) != 2 {
		t.Fatalf("expected 2 valid aspects for must_review, got %v", tiers[0].AspectNames)
	}

	// Built-in label preserved; custom tier label derived as Title Case.
	if tiers[0].Label != "Must Review" {
		t.Fatalf("must_review label=%q", tiers[0].Label)
	}
	if tiers[2].Label != "Quick Pass" {
		t.Fatalf("quick_pass label=%q", tiers[2].Label)
	}
}
