package main

import (
	"context"
	"encoding/json"
	"fmt"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

const translatePromptRef = "prompt-translate-medical-product-names-v1.md"

// translator batch-translates Chinese product names to English via the
// shared reviewer LLM client (the same docprocessing.BuildReviewerLLMClient /
// LoadPromptByRef plumbing extract-products.go's translate pass uses).
type translator struct {
	client     docprocessing.LLMJSONExtractor
	modelName  string
	promptText string
	batchSize  int
}

func newTranslator(modelRef string, batchSize int) (*translator, error) {
	promptText, _, _, err := docprocessing.LoadPromptByRef(translatePromptRef)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", translatePromptRef, err)
	}
	client, modelName, err := docprocessing.BuildReviewerLLMClient(modelRef)
	if err != nil {
		return nil, fmt.Errorf("build LLM client for %q: %w", modelRef, err)
	}
	if batchSize <= 0 {
		batchSize = 40
	}
	return &translator{client: client, modelName: modelName, promptText: promptText, batchSize: batchSize}, nil
}

// translateAll translates the given unique Chinese names, returning a
// zh -> en map. A batch that fails to translate is logged and left
// untranslated rather than aborting the whole import.
func (t *translator) translateAll(ctx context.Context, names []string) map[string]string {
	out := make(map[string]string, len(names))
	for start := 0; start < len(names); start += t.batchSize {
		end := start + t.batchSize
		if end > len(names) {
			end = len(names)
		}
		batch := names[start:end]
		translated, err := t.translateBatch(ctx, batch)
		if err != nil {
			fmt.Printf("  warning: translate batch [%d:%d] failed: %v (left untranslated)\n", start, end, err)
			continue
		}
		for i, zh := range batch {
			out[zh] = translated[i]
		}
	}
	return out
}

func (t *translator) translateBatch(ctx context.Context, names []string) ([]string, error) {
	inputJSON, err := json.Marshal(map[string]any{"names": names})
	if err != nil {
		return nil, err
	}
	input := docprocessing.NewLLMJSONInput(ctx,
		"translate-medical-product-names", t.promptText, t.modelName, string(inputJSON),
		"product_names_import", "MID_PNI_TRANSLATE")
	payload, err := t.client.ExtractJSON(ctx, input)
	if err != nil {
		return nil, err
	}
	raw, _ := payload["names_en"].([]any)
	if len(raw) != len(names) {
		return nil, fmt.Errorf("expected %d translations, got %d", len(names), len(raw))
	}
	out := make([]string, len(raw))
	for i, v := range raw {
		out[i], _ = v.(string)
	}
	return out, nil
}
