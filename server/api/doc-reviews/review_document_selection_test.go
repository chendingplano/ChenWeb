package docreviews

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func resetDocReviewConfigCacheForTest(t *testing.T) {
	t.Helper()

	oldCfg, oldErr := docReviewCfg, docReviewCfgErr
	docReviewCfgOnce = sync.Once{}
	docReviewCfg = nil
	docReviewCfgErr = nil
	t.Cleanup(func() {
		docReviewCfgOnce = sync.Once{}
		docReviewCfg = oldCfg
		docReviewCfgErr = oldErr
		if oldCfg != nil || oldErr != nil {
			docReviewCfgOnce.Do(func() {})
		}
	})
}

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

	resetDocReviewConfigCacheForTest(t)

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

func TestBuildReviewers_NormalizesReviewDepthForResolvedConfig(t *testing.T) {
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
	resetDocReviewConfigCacheForTest(t)

	p := &ReviewProcessor{
		ReviewDepth:       0,
		GrammarClient:     &fakeJSONExtractor{},
		GrammarModelName:  "test-model",
		GrammarPromptText: "prompt",
	}

	runners := p.buildReviewers(DocMetadataInputRecord{})
	if len(runners) != 1 {
		t.Fatalf("buildReviewers returned %d runners, want 1", len(runners))
	}

	cfg := runners[0].cfg
	if cfg.ReviewDepth != 1 {
		t.Fatalf("cfg.ReviewDepth = %d, want 1", cfg.ReviewDepth)
	}
	if cfg.MaxFindings != 10 || cfg.MaxAnalyses != 11 {
		t.Fatalf("cfg limits = %d/%d, want 10/11", cfg.MaxFindings, cfg.MaxAnalyses)
	}
}
