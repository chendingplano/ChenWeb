package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeProductsStore struct {
	exists       bool
	existsErr    error
	saveErr      error
	deleteErr    error
	deleteCalled int
	saveCalled   int
	existCalled  int
	lastSave     SaveProductsRequest
}

func (f *fakeProductsStore) ProductsExist(_ context.Context, _ int64) (bool, error) {
	f.existCalled++
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.exists, nil
}

func (f *fakeProductsStore) DeleteProductsByInputRecordID(_ context.Context, _ int64) (int64, error) {
	f.deleteCalled++
	if f.deleteErr != nil {
		return 0, f.deleteErr
	}
	return 0, nil
}

func (f *fakeProductsStore) SaveProducts(_ context.Context, req SaveProductsRequest) (int64, error) {
	f.saveCalled++
	f.lastSave = req
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	return int64(len(req.Products)), nil
}

func TestMergeProductMentionCandidates_DropsOverlapOnlyMentions(t *testing.T) {
	mentions := []productMention{
		{
			MentionText:       "Alarm system",
			CanonicalHint:     "alarm system",
			ProductTypeHint:   "system",
			EvidenceQuote:     "Alarm system",
			EvidenceLines:     []string{"10"},
			HasNormalEvidence: false,
			BlockLines: []BlockLine{
				{Flag: "o", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "Alarm system context"},
			},
		},
		{
			MentionText:       "Infusion pump",
			CanonicalHint:     "infusion pump",
			ProductTypeHint:   "equipment",
			EvidenceQuote:     "Infusion pump",
			EvidenceLines:     []string{"20"},
			HasNormalEvidence: true,
			BlockLines: []BlockLine{
				{Flag: "n", LineNumber: 20, PageNumber: 1, LineType: "paragraph", Content: "The infusion pump shall be inspected monthly."},
			},
		},
	}

	got := mergeProductMentionCandidates(mentions)
	if len(got) != 1 {
		t.Fatalf("candidate count=%d, want 1", len(got))
	}
	if got[0].CanonicalName != "infusion pump" {
		t.Fatalf("canonical_name=%q, want infusion pump", got[0].CanonicalName)
	}
}

func TestMergeProductMentionCandidates_PreservesNonEnglishMentions(t *testing.T) {
	mentions := []productMention{
		{
			MentionText:       "输液泵",
			CanonicalHint:     "输液泵",
			ProductTypeHint:   "equipment",
			EvidenceQuote:     "输液泵",
			EvidenceLines:     []string{"20"},
			HasNormalEvidence: true,
			BlockLines: []BlockLine{
				{Flag: "n", LineNumber: 20, PageNumber: 1, LineType: "paragraph", Content: "输液泵应每月检查一次。"},
			},
		},
	}

	got := mergeProductMentionCandidates(mentions)
	if len(got) != 1 {
		t.Fatalf("candidate count=%d, want 1", len(got))
	}
	if got[0].CanonicalName != "输液泵" {
		t.Fatalf("canonical_name=%q, want 输液泵", got[0].CanonicalName)
	}
}

func TestProductsProcessor_HandleEvent_MultiPassPipeline(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_DIR", tmp)
	t.Setenv("ARTIFACT_WEB_DIR", tmp)

	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              5101,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_5101.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_5101.pdf"),
		StatusRaw:       "[]",
	}}
	productStore := &fakeProductsStore{}
	extractor := &fakeJSONExtractor{
		outs: []map[string]any{
			{
				"mentions": []any{
					map[string]any{
						"mention_text":      "infusion pump",
						"canonical_hint":    "infusion pump",
						"product_type_hint": "equipment",
						"evidence_quote":    "infusion pump",
						"evidence_lines":    []any{"10"},
						"is_explicit":       true,
						"confidence":        0.91,
						"confidence_reason": "explicit mention",
					},
				},
			},
			{
				"mentions": []any{
					map[string]any{
						"mention_text":      "infusion pump",
						"canonical_hint":    "infusion pump",
						"product_type_hint": "equipment",
						"evidence_quote":    "infusion pump",
						"evidence_lines":    []any{"12"},
						"is_explicit":       true,
						"confidence":        0.93,
						"confidence_reason": "explicit mention",
					},
				},
			},
			{
				"products": []any{
					map[string]any{
						"product_name":    "infusion pump",
						"canonical_name":  "infusion pump",
						"product_type":    "equipment",
						"relation_type":   "maintenance_requirement",
						"product_summary": "The input requires monthly inspection of the infusion pump.",
						"evidence_quote":  "The infusion pump shall be inspected monthly.",
						"evidence_lines":  []any{"10", "12"},
						"relation_details": map[string]any{
							"obligation_level":         "mandatory",
							"requirement_text":         "The infusion pump shall be inspected monthly.",
							"conditions":               []any{},
							"exceptions":               []any{},
							"thresholds_or_parameters": []any{},
							"related_products":         []any{},
							"responsible_actor":        "operator",
						},
						"confidence":        0.94,
						"confidence_reason": "explicit requirement",
					},
				},
			},
			{
				"products": []any{
					map[string]any{
						"product_name_en":      nil,
						"canonical_name_en":    nil,
						"product_summary_en":   nil,
						"requirement_text_en":  nil,
						"confidence_reason_en": nil,
					},
				},
			},
			{
				"products": []any{
					map[string]any{
						"category_paths": []any{
							map[string]any{
								"category_path": []any{
									map[string]any{
										"name":       "medical",
										"keywords":   []any{"infusion"},
										"confidence": 0.88,
									},
								},
								"path_keywords":   []any{"infusion pump"},
								"path_confidence": 0.88,
							},
						},
						"category_paths_en": []any{},
					},
				},
			},
		},
	}

	ctx, holder := withBlockBufferHolder(context.Background())
	holder.mu.Lock()
	holder.buffer = &BlockBuffer{
		Blocks: []Block{
			{
				Index: 1,
				Lines: []BlockLine{
					{Flag: "n", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "The infusion pump shall be inspected monthly."},
					{Flag: "o", LineNumber: 11, PageNumber: 1, LineType: "paragraph", Content: "Overlap context."},
				},
			},
			{
				Index: 2,
				Lines: []BlockLine{
					{Flag: "n", LineNumber: 12, PageNumber: 1, LineType: "paragraph", Content: "Each infusion pump must have a maintenance log."},
				},
			},
		},
	}
	holder.mu.Unlock()

	p := NewProductsProcessor(inputStore, productStore, extractor, nil)
	p.MentionPromptText = "extract mentions"
	p.MentionPromptRef = "prompt-extract-product-mentions-v1.md"
	p.MentionPromptErr = nil
	p.MentionModelErr = nil
	p.MentionModelName = "gpt-test"
	p.RelationPromptText = "enrich relations"
	p.RelationPromptRef = "prompt-enrich-product-relations-v1.md"
	p.RelationPromptErr = nil
	p.RelationModelErr = nil
	p.RelationModelName = "gpt-test"
	p.PromptRef = p.RelationPromptRef
	p.ModelName = p.RelationModelName
	p.TranslateEnabled = true
	p.TranslatePromptText = "translate"
	p.TranslatePromptRef = "prompt-translate-products-v1.md"
	p.TranslateModelName = "gpt-test"
	p.TranslatePromptErr = nil
	p.CategorizeEnabled = true
	p.CategorizePromptText = "categorize"
	p.CategorizePromptRef = "prompt-categorize-products-v1.md"
	p.CategorizeModelName = "gpt-test"
	p.CategorizePromptErr = nil

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"5101","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.structuredCalledCount != 5 {
		t.Fatalf("structuredCalledCount=%d, want 5", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if productStore.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", productStore.saveCalled)
	}
	if len(productStore.lastSave.Products) != 1 {
		t.Fatalf("saved products=%d, want 1", len(productStore.lastSave.Products))
	}
	row := productStore.lastSave.Products[0]
	if got := strings.TrimSpace(asString(row["product_rel_id"])); got != "5101_1" {
		t.Fatalf("product_rel_id=%q, want 5101_1", got)
	}
	if got := strings.TrimSpace(asString(row["relation_type"])); got != "maintenance_requirement" {
		t.Fatalf("relation_type=%q", got)
	}
	if got := strings.TrimSpace(asString(row["prompt_name"])); got != "prompt-enrich-product-relations-v1.md" {
		t.Fatalf("prompt_name=%q", got)
	}
	if _, ok := row["category_paths"]; !ok {
		t.Fatalf("category_paths missing")
	}

	artifactPath := filepath.Join(tmp, "5", "5101", "ocr_rslt_5101_opendata.products")
	body, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	var artifactRecords []map[string]any
	if err := json.Unmarshal(body, &artifactRecords); err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	if len(artifactRecords) != 1 {
		t.Fatalf("artifact product count=%d, want 1", len(artifactRecords))
	}
}

func TestExtractProductPayloadWithFallback_EmptyPrimaryResponseSkipsFallback(t *testing.T) {
	extractor := &fakeJSONExtractor{
		errs: []error{
			errors.New("(MID_26050174) failed resolveScopedString, error:(MID_26052923) failed resolveScopedString, error:(MID_26050142) decode llm response: unexpected end of JSON input, json:{[]}"),
		},
	}

	p := NewProductsProcessor(nil, nil, extractor, nil)
	p.FallbackModelName = "gpt-5.4-mini"

	payload, modelName, err := p.extractProductPayloadWithFallback(
		context.Background(),
		"actor",
		"input",
		"prompt",
		"prompt-ref",
		"deepseek-v4-flash",
		structureModelConfig{},
	)
	if err != nil {
		t.Fatalf("extractProductPayloadWithFallback: %v", err)
	}
	if extractor.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if modelName != "deepseek-v4-flash" {
		t.Fatalf("modelName=%q, want deepseek-v4-flash", modelName)
	}
	if got := payload["products"]; got == nil {
		t.Fatalf("products missing from payload: %#v", payload)
	}
	if got := payload["mentions"]; got == nil {
		t.Fatalf("mentions missing from payload: %#v", payload)
	}
}

func TestProductsProcessor_ExtractProductPayloadUsesStructuredContractWhenAvailable(t *testing.T) {
	extractor := &fakeJSONExtractor{out: map[string]any{
		"products": []any{
			map[string]any{
				"product_name": "infusion pump",
			},
		},
	}}

	p := NewProductsProcessor(nil, nil, extractor, nil)
	p.PromptText = "extract products"
	p.PromptRef = "prompt-test"
	p.ModelName = "gpt-test"

	payload, err := p.extractProductPayload(context.Background(), 
		"action", "input text", "extract products", "prompt-test", "gpt-test", structureModelConfig{})
	if err != nil {
		t.Fatalf("extractProductPayload: %v", err)
	}
	if extractor.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if len(extractor.contractNames) != 1 || extractor.contractNames[0] != "chenweb_product_extraction" {
		t.Fatalf("contractNames=%v, want [chenweb_product_extraction]", extractor.contractNames)
	}
	items, ok := payload["products"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("products=%#v", payload["products"])
	}
}

func TestLoadProductPromptFromEnvKeys_PrefersFirstConfiguredKey(t *testing.T) {
	tmp := t.TempDir()
	promptPath := filepath.Join(tmp, "prompt-enrich-product-relations-v1.md")
	want := "enrich product relations"
	if err := os.WriteFile(promptPath, []byte(want), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	t.Setenv("PROMPT_DIR", tmp)
	t.Setenv("ENRICH_PRODUCT_RELATIONS_PROMPT", "prompt-enrich-product-relations-v1.md")
	t.Setenv("EXTRACT_PRODUCTS_PROMPT", "older-prompt.md")

	got, ref, gotPath, err := loadProductPromptFromEnvKeys(
		[]string{"ENRICH_PRODUCT_RELATIONS_PROMPT", "EXTRACT_PRODUCTS_PROMPT", "EXTRACT_PRODUCT_PROMPT"},
		"default.md",
	)
	if err != nil {
		t.Fatalf("loadProductPromptFromEnvKeys: %v", err)
	}
	if got != want {
		t.Fatalf("promptText=%q, want %q", got, want)
	}
	if ref != "prompt-enrich-product-relations-v1.md" {
		t.Fatalf("promptRef=%q", ref)
	}
	if gotPath != promptPath {
		t.Fatalf("promptPath=%q, want %q", gotPath, promptPath)
	}
}

func TestLoadModelConfigFromEnvKeys_PrefersFirstConfiguredKey(t *testing.T) {
	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[gpt-5-4-mini-products-mentions]
host = "cloud"
model_name = "gpt-5.4-mini"
api_key = "sk-test-mentions"
base_url = "https://api.openai.com"
timeout_sec = 90

[gpt-5-4-mini-products-relations]
host = "cloud"
model_name = "gpt-5.4-mini"
api_key = "sk-test-relations"
base_url = "https://api.openai.com"
timeout_sec = 100
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("MODEL_DEF_FILE", modelsPath)
	t.Setenv("EXTRACT_PRODUCT_MENTIONS_MODEL_NAME", "gpt-5-4-mini-products-mentions")
	t.Setenv("EXTRACT_PRODUCT_MODEL_NAME", "gpt-5-4-mini-products-relations")

	ref, gotPath, cfg, err := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_PRODUCT_MENTIONS_MODEL_NAME", "EXTRACT_PRODUCT_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	if err != nil {
		t.Fatalf("loadModelConfigFromEnvKeys: %v", err)
	}
	if ref != "gpt-5-4-mini-products-mentions" {
		t.Fatalf("modelRef=%q", ref)
	}
	if gotPath != modelsPath {
		t.Fatalf("modelPath=%q, want %q", gotPath, modelsPath)
	}
	if cfg.APIKey != "sk-test-mentions" {
		t.Fatalf("apiKey=%q, want sk-test-mentions", cfg.APIKey)
	}
	if cfg.TimeoutSec != 90 {
		t.Fatalf("timeout=%d, want 90", cfg.TimeoutSec)
	}
}
