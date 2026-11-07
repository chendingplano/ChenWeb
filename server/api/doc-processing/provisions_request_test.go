package docprocessing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type provisionsRequestRecorder struct {
	fakeJSONExtractor
	inputs []llmclients.JSONExtractionInput
}

func (r *provisionsRequestRecorder) ExtractStructuredJSON(_ context.Context, in llmclients.JSONExtractionInput, _ llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error) {
	r.inputs = append(r.inputs, in)
	return &llmclients.StructuredOutputResult{Parsed: map[string]any{"provisions": []any{}}}, nil
}

func TestProvisionsRequestsShareInstructionPrefix(t *testing.T) {
	r := &provisionsRequestRecorder{}
	p := &ProvisionsProcessor{PromptRef: "test"}
	for i := 0; i < 2; i++ {
		_, err := p.extractProvisionPayloadFromText(context.Background(), "different document text", provisionChunkTask("stable instructions", Chunk{SeqNo: i}), "model", r)
		if err != nil {
			t.Fatal(err)
		}
	}
	a, b := llmclients.PreviewMessages(r.inputs[0]), llmclients.PreviewMessages(r.inputs[1])
	if a[0]["content"] != b[0]["content"] || a[0]["content"] == "You are a document processing engine. Return strict JSON only." {
		t.Fatalf("instructions must be the same system prefix for every chunk: %#v %#v", a, b)
	}
}

func TestProvisionsNoReasoningPolicyAppliesToBothModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.toml")
	model := "host = 'cloud'\nmodel_name = 'test'\napi_key = 'test'\nbase_url = 'https://example.com'\nthinking_type = 'enabled'\n"
	if err := os.WriteFile(path, []byte("[primary]\n"+model+"[fallback]\n"+model), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXTRACT_PROVISIONS_MODELS_FILE", path)
	t.Setenv("EXTRACT_PROVISIONS_MODEL_NAME", "primary")
	t.Setenv("EXTRACT_PROVISIONS_MODEL_FALLBACK", "fallback")
	t.Setenv("MODEL_DEFAULT_REASONING_POLICY", "no-reasoning")
	primary := &llmclients.OpenAIJSONClient{}
	p := NewProvisionsProcessor(nil, nil, primary, nil)
	fallback, ok := p.FallbackExtractor.(*llmclients.OpenAIJSONClient)
	if !ok || primary.ThinkingType != "disabled" || fallback.ThinkingType != "disabled" {
		t.Fatalf("thinking primary=%q fallback=%#v", primary.ThinkingType, p.FallbackExtractor)
	}
}

func TestProvisionsRequestsUseVersionedPromptSchema(t *testing.T) {
	base := "custom instructions with optional response fields"
	if got := provisionChunkTask(base, Chunk{}); got != base {
		t.Fatalf("chunk instructions must come from the prompt file: %q", got)
	}
	block := buildProvisionUserPrompt(Block{Index: 7})
	if !strings.Contains(block, "Block index: 7") || !strings.Contains(block, "Input block lines (JSON array):") {
		t.Fatalf("block input lost its index or lines: %q", block)
	}
	if strings.Contains(block, "schema") || strings.Contains(block, "provision_en") {
		t.Fatalf("block input must not override the versioned prompt schema: %q", block)
	}
}

func TestProvisionsDefaultPromptUsesCompactFormat(t *testing.T) {
	t.Setenv("EXTRACT_PROVISIONS_PROMPT", "")
	t.Setenv("PROMPT_FILE_NAME", "")
	t.Setenv("PROMPT_DIR", t.TempDir())
	text, ref, _, err := loadProvisionsPromptFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if ref != "prompt-extract-provisions-v6.md" || !strings.Contains(text, `"v":2`) {
		t.Fatalf("unexpected default ref=%q text=%q", ref, text)
	}
}
