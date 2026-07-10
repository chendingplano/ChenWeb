package docreviews

import "testing"

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
