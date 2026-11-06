package docprocessing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestAmbiguousObjectResolverUsesStablePromptPrefix(t *testing.T) {
	promptPath := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(promptPath, []byte("Resolve the object."), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RESOLVE_AMBIGUOUS_OBJECT_PROMPT", promptPath)
	stop := errors.New("capture request")
	client := &stubLLMJSONExtractor{results: []stubLLMJSONResult{{err: stop}, {err: stop}}}
	resolver := ambiguousObjectLLMJSONResolver{client: client, modelName: "test"}
	for _, id := range []string{"first", "second"} {
		if _, err := resolver.ResolveAmbiguousObject(context.Background(), ArtifactObject{ArtifactID: id}, nil); !errors.Is(err, stop) {
			t.Fatalf("ResolveAmbiguousObject: %v", err)
		}
	}
	first, second := llmclients.PreviewMessages(client.inputs[0]), llmclients.PreviewMessages(client.inputs[1])
	if first[0]["content"] != "Resolve the object." || first[0]["content"] != second[0]["content"] {
		t.Fatalf("expected stable prompt as first message, got %v and %v", first, second)
	}
	if first[1]["content"] == second[1]["content"] {
		t.Fatal("different artifacts must produce different input messages")
	}
}

func TestAmbiguousObjectResolverDisablesThinking(t *testing.T) {
	models := filepath.Join(t.TempDir(), "models.toml")
	config := "[test]\nhost = 'cloud'\nmodel_name = 'test'\napi_key = 'test'\nbase_url = 'https://example.com'\nthinking_type = 'enabled'\n"
	if err := os.WriteFile(models, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODEL_DEF_FILE", models)
	t.Setenv("RESOLVE_AMBIGUOUS_OBJECT_MODEL_NAME", "test")
	resolver, _, err := NewAmbiguousObjectLLMResolverFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	client := resolver.(ambiguousObjectLLMJSONResolver).client.(*llmclients.OpenAIJSONClient)
	if client.ThinkingType != "disabled" {
		t.Fatalf("thinking type = %q, want disabled", client.ThinkingType)
	}
}
