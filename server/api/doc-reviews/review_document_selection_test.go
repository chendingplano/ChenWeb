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
