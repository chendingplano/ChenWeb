package docprocessing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// chdirToRepoRootForPromptLoading makes loadPromptByRef's relative
// "prompts/<ref>" candidate resolve during `go test`, which otherwise runs
// with the package directory as its working directory. Mirrors
// generate_scene_blocks_prompt_test.go's TestLoadSceneBlocksPromptFromEnv...
// pattern.
func chdirToRepoRootForPromptLoading(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir repo root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
}

func TestParseEntityObjectClassificationReadsExpectedShape(t *testing.T) {
	payload := map[string]any{
		"decision":           "associate",
		"confidence":         0.9,
		"rationale":          "recurring measured equipment",
		"selected_object_id": "obj_pump",
	}
	got, err := parseEntityObjectClassification(payload, "test-model")
	if err != nil {
		t.Fatalf("parseEntityObjectClassification: %v", err)
	}
	if got.Decision != "associate" || got.Confidence != 0.9 || got.SelectedObjectID != "obj_pump" || got.ModelName != "test-model" {
		t.Fatalf("got %+v, unexpected shape", got)
	}
}

func TestParseEntityObjectClassificationAcceptsQualitativeConfidence(t *testing.T) {
	got, err := parseEntityObjectClassification(map[string]any{
		"decision":   "exclude",
		"confidence": "high",
	}, "test-model")
	if err != nil {
		t.Fatalf("parseEntityObjectClassification: %v", err)
	}
	if got.Confidence != 0.9 {
		t.Fatalf("got.Confidence = %v, want 0.9 (qualitative 'high' per the shared tolerant parser)", got.Confidence)
	}
}

func TestParseEntityObjectClassificationRejectsMissingDecision(t *testing.T) {
	_, err := parseEntityObjectClassification(map[string]any{"confidence": 0.9}, "test-model")
	if err == nil {
		t.Fatalf("want error for missing decision, got nil")
	}
}

func TestParseEntityObjectClassificationRejectsEmptyPayload(t *testing.T) {
	_, err := parseEntityObjectClassification(nil, "test-model")
	if err == nil {
		t.Fatalf("want error for nil payload, got nil")
	}
}

func TestEntityObjectClassifierJSONResolverCallsLLMAndParses(t *testing.T) {
	chdirToRepoRootForPromptLoading(t)
	client := &stubLLMJSONExtractor{results: []stubLLMJSONResult{
		{payload: map[string]any{
			"decision":   "exclude",
			"confidence": 0.95,
			"rationale":  "generic organization mention, no reuse value",
		}},
	}}
	resolver := entityObjectClassifierJSONResolver{client: client, modelName: "test-model"}

	got, err := resolver.ClassifyEntityForObjectLink(context.Background(),
		pendingEntityRow{EntityID: "1_ent_1", Entity: "Some Org", EntityType: "organization"},
		nil,
	)
	if err != nil {
		t.Fatalf("ClassifyEntityForObjectLink: %v", err)
	}
	if got.Decision != "exclude" || got.Confidence != 0.95 {
		t.Fatalf("got %+v, unexpected shape", got)
	}
	if client.calls != 1 {
		t.Fatalf("client.calls = %d, want 1", client.calls)
	}
}

func TestEntityObjectClassifierJSONResolverPropagatesClientError(t *testing.T) {
	client := &stubLLMJSONExtractor{results: []stubLLMJSONResult{{err: errClassifierBoom}}}
	resolver := entityObjectClassifierJSONResolver{client: client, modelName: "test-model"}

	_, err := resolver.ClassifyEntityForObjectLink(context.Background(), pendingEntityRow{EntityID: "1_ent_1"}, nil)
	if err == nil {
		t.Fatalf("want error propagated from the LLM client, got nil")
	}
}
