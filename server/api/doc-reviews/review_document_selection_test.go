package docreviews

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestBuildReviewers_FiltersToRequestedAspects(t *testing.T) {
	p := &ReviewProcessor{
		RequestedAspects:    []string{"grammar_spelling"},
		GrammarClient:       &fakeJSONExtractor{},
		GrammarModelName:    "test-model",
		GrammarPromptText:   "prompt",
		ToneVoiceClient:     &fakeJSONExtractor{},
		ToneVoiceModelName:  "test-model",
		ToneVoicePromptText: "prompt",
		MaxConcurrent:       1,
	}

	runners := p.buildReviewers(DocMetadataInputRecord{})
	if len(runners) != 1 {
		t.Fatalf("buildReviewers returned %d runners, want 1", len(runners))
	}
	if got := runners[0].reviewer.Name(); got != "grammar_spelling" {
		t.Fatalf("buildReviewers returned reviewer %q, want grammar_spelling", got)
	}
}

func TestBuildReviewers_WithoutRequestedAspectsIncludesConfiguredReviewers(t *testing.T) {
	p := &ReviewProcessor{
		GrammarClient:       &fakeJSONExtractor{},
		GrammarModelName:    "test-model",
		GrammarPromptText:   "prompt",
		ToneVoiceClient:     &fakeJSONExtractor{},
		ToneVoiceModelName:  "test-model",
		ToneVoicePromptText: "prompt",
		MaxConcurrent:       1,
	}

	runners := p.buildReviewers(DocMetadataInputRecord{})
	if len(runners) != 2 {
		t.Fatalf("buildReviewers returned %d runners, want 2", len(runners))
	}
}

func TestBuildReviewers_PreservesArtifactRetrievalMatchLimitsAboveTen(t *testing.T) {
	t.Setenv("METRIC_REVIEW_MAX_MATCHES", "25")
	t.Setenv("PROVISION_REVIEW_MAX_MATCHES", "25")
	t.Setenv("ENTITY_REVIEW_MAX_MATCHES", "25")
	t.Setenv("INVENTORY_REVIEW_MAX_MATCHES", "25")

	p := &ReviewProcessor{
		MetricsClient:            &fakeJSONExtractor{},
		MetricsModelName:         "model",
		MetricsPromptText:        "prompt",
		ProvisionsClient:         &fakeJSONExtractor{},
		ProvisionsModelName:      "model",
		ProvisionsPromptText:     "prompt",
		EntitiesClient:           &fakeJSONExtractor{},
		EntitiesModelName:        "model",
		EntitiesPromptText:       "prompt",
		InventoryItemsClient:     &fakeJSONExtractor{},
		InventoryItemsModelName:  "model",
		InventoryItemsPromptText: "prompt",
	}

	runners := p.buildReviewers(DocMetadataInputRecord{})
	seen := 0
	for _, runner := range runners {
		var got int
		switch reviewer := runner.reviewer.(type) {
		case *metricsReviewer:
			got = reviewer.maxMatches
		case *provisionsReviewer:
			got = reviewer.maxMatches
		case *entitiesReviewer:
			got = reviewer.maxMatches
		case *inventoryItemsReviewer:
			got = reviewer.maxMatches
		default:
			continue
		}
		seen++
		if got != 25 {
			t.Errorf("%s maxMatches = %d, want 25", runner.reviewer.Name(), got)
		}
	}
	if seen != 4 {
		t.Fatalf("artifact reviewers found = %d, want 4", seen)
	}
}

func TestBuildReviewers_PropagatesDepthSpecificOutputLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc-review.local.toml")
	if err := os.WriteFile(path, []byte(`
max_findings = [10,20,30]
max_analyses = [11,21,31]
[reviewers.grammar_spelling]
enabled = true
checked = true
group = "P1"
model = "test-model"
prompt = "grammar-spelling.txt"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOC_REVIEW_CONFIG_FILE", path)

	oldOnce, oldCfg, oldErr := docReviewCfgOnce, docReviewCfg, docReviewCfgErr
	docReviewCfgOnce = sync.Once{}
	docReviewCfg = nil
	docReviewCfgErr = nil
	t.Cleanup(func() {
		docReviewCfgOnce = oldOnce
		docReviewCfg = oldCfg
		docReviewCfgErr = oldErr
	})

	p := &ReviewProcessor{
		ReviewDepth:       2,
		GrammarClient:     &fakeJSONExtractor{},
		GrammarModelName:  "test-model",
		GrammarPromptText: "prompt",
	}

	runners := p.buildReviewers(DocMetadataInputRecord{})
	if len(runners) != 1 {
		t.Fatalf("buildReviewers returned %d runners, want 1", len(runners))
	}

	cfg := runners[0].cfg
	if cfg.MaxFindings != 20 || cfg.MaxAnalyses != 21 || cfg.ReviewDepth != 2 {
		t.Fatalf("reviewer cfg limits/depth = %d/%d depth=%d, want 20/21 depth=2", cfg.MaxFindings, cfg.MaxAnalyses, cfg.ReviewDepth)
	}
}
