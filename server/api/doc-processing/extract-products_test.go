package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
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

	stagingFilename := filepath.Join(tmp, "ocr_rslt_5101.pdf")
	writeTestArtifacts(t, tmp, 5101, stagingFilename, "opendata",
		strings.Join([]string{
			"10\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tThe infusion pump shall be inspected monthly.",
			"11\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tOverlap context.",
			"12\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tEach infusion pump must have a maintenance log.",
		}, "\n"),
		"overlap: [11]\nlines: [10]\n\noverlap: []\nlines: [12]\n",
	)
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              5101,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_5101.json"),
		StagingFilename: stagingFilename,
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
						"product_name_en":      "infusion pump",
						"canonical_name_en":    nil,
						"product_summary_en":   nil,
						"requirement_text_en":  nil,
						"confidence_reason_en": nil,
					},
				},
			},
		},
	}

	ctx := context.Background()

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
	p.ProductNames = &fakeProductNameStore{lookupResult: &productNameMatch{
		ID: 6279, ProductName: "infusion pump", Exact: true,
		SubCatalog: "01 有源手术器械", CategoryL1: "01 超声手术设备及附件", CategoryL2: "01.1 超声手术设备",
	}}

	if err := p.HandleEvent(ctx, []byte(`{"record_id":"5101","force":true}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if extractor.structuredCalledCount != 4 {
		t.Fatalf("structuredCalledCount=%d, want 4", extractor.structuredCalledCount)
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
	if got := strings.TrimSpace(asString(row["product_rel_id"])); got != "5101_prd_1" {
		t.Fatalf("product_rel_id=%q, want 5101_prd_1", got)
	}
	if got := strings.TrimSpace(asString(row["relation_type"])); got != "maintenance_requirement" {
		t.Fatalf("relation_type=%q", got)
	}
	if got := strings.TrimSpace(asString(row["prompt_name"])); got != "prompt-enrich-product-relations-v1.md" {
		t.Fatalf("prompt_name=%q", got)
	}
	if got, _ := row["product_name_id"].(int64); got != 6279 {
		t.Fatalf("product_name_id=%v, want 6279 (from the resolved kb.product_names match)", row["product_name_id"])
	}
	paths, ok := row["category_paths"].([]CategoryPathEntry)
	if !ok || len(paths) != 1 || len(paths[0].Nodes) != 3 || paths[0].Nodes[0].Name != "01 有源手术器械" {
		t.Fatalf("category_paths not built from the kb.product_names catalog match: %#v", row["category_paths"])
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

// TestExtractProductsFromBlocksWithLLM_MentionsPassUsesCanonicalChunkDocument
// guards against the cache_hit=0 regression (2026-09-12 log investigation):
// the mentions pass's document must be canonicalChunkInputText(chunk, docCtx)
// -- the same bytes every other chunk-based processor sends for this chunk --
// not a bespoke schema+block blob, so it forms the stable prefix DeepSeek's
// prompt cache can actually match. See input_lines.go's canonicalChunkInputText
// doc comment and the metrics enrich-cache fix (2026-08-16) this mirrors.
func TestExtractProductsFromBlocksWithLLM_MentionsPassUsesCanonicalChunkDocument(t *testing.T) {
	block := Block{Index: 7, Lines: []BlockLine{
		{Flag: "n", LineNumber: 10, PageNumber: 1, LineType: "paragraph", Content: "The infusion pump shall be inspected monthly."},
	}}
	chunk := Chunk{SeqNo: 7, Lines: []MarkedLine{
		{Mark: "r", Line: Line{LineNo: 10, PageNo: 1, LineType: "paragraph", Content: "The infusion pump shall be inspected monthly."}},
	}}
	docCtx := "Test Doc | DOC-001"
	extractor := &fakeJSONExtractor{outs: []map[string]any{{"mentions": []any{}}}}
	p := &ProductsProcessor{
		Logger:             loggerutil.CreateDefaultLogger("MID_TEST_PDM1"),
		Extractor:          extractor,
		Now:                time.Now,
		MentionPromptText:  "extract mentions",
		MentionPromptRef:   "prompt-extract-product-mentions-v1.md",
		MentionModelName:   "gpt-test",
		RelationPromptText: "enrich relations",
		RelationPromptRef:  "prompt-enrich-product-relations-v1.md",
		RelationModelName:  "gpt-test",
		MaxTasks:           1,
	}

	if _, err := p.extractProductsFromBlocksWithLLM(context.Background(), []Block{block}, []Chunk{chunk}, docCtx); err != nil {
		t.Fatalf("extractProductsFromBlocksWithLLM: %v", err)
	}
	if len(extractor.inputTexts) != 1 {
		t.Fatalf("inputTexts=%v, want 1 mentions call", extractor.inputTexts)
	}
	want := canonicalChunkInputText(chunk.Lines, docCtx)
	if extractor.inputTexts[0] != want {
		t.Fatalf("mentions pass document = %q, want canonical chunk text %q", extractor.inputTexts[0], want)
	}
}

// TestExtractProductsFromBlocksWithLLM_SingleBlockCandidateRelationsPassReusesMentionsDocument
// asserts the actual cache-hit condition: for a candidate whose evidence all
// comes from one block, pass 2's document must be byte-identical to pass 1's
// document for that same chunk, so pass 2 rides the cache pass 1 already
// warmed instead of paying for a bespoke, never-repeating candidate blob.
func TestExtractProductsFromBlocksWithLLM_SingleBlockCandidateRelationsPassReusesMentionsDocument(t *testing.T) {
	block := Block{Index: 3, Lines: []BlockLine{
		{Flag: "n", LineNumber: 5, PageNumber: 1, LineType: "paragraph", Content: "The infusion pump shall be inspected monthly."},
	}}
	chunk := Chunk{SeqNo: 3, Lines: []MarkedLine{
		{Mark: "r", Line: Line{LineNo: 5, PageNo: 1, LineType: "paragraph", Content: "The infusion pump shall be inspected monthly."}},
	}}
	docCtx := "Test Doc | DOC-002"
	extractor := &fakeJSONExtractor{outs: []map[string]any{
		{"mentions": []any{
			map[string]any{
				"mention_text":      "infusion pump",
				"canonical_hint":    "infusion pump",
				"product_type_hint": "equipment",
				"evidence_quote":    "infusion pump",
				"evidence_lines":    []any{"5"},
				"is_explicit":       true,
				"confidence":        0.9,
				"confidence_reason": "explicit mention",
			},
		}},
		{"products": []any{
			map[string]any{"product_name": "infusion pump", "relation_type": "maintenance_requirement"},
		}},
	}}
	p := &ProductsProcessor{
		Logger:             loggerutil.CreateDefaultLogger("MID_TEST_PDM2"),
		Extractor:          extractor,
		Now:                time.Now,
		MentionPromptText:  "extract mentions",
		MentionPromptRef:   "prompt-extract-product-mentions-v1.md",
		MentionModelName:   "gpt-test",
		RelationPromptText: "enrich relations",
		RelationPromptRef:  "prompt-enrich-product-relations-v1.md",
		RelationModelName:  "gpt-test",
		MaxTasks:           1,
	}

	if _, err := p.extractProductsFromBlocksWithLLM(context.Background(), []Block{block}, []Chunk{chunk}, docCtx); err != nil {
		t.Fatalf("extractProductsFromBlocksWithLLM: %v", err)
	}
	if len(extractor.inputTexts) != 2 {
		t.Fatalf("inputTexts=%v, want 2 calls (mentions + relations)", extractor.inputTexts)
	}
	wantDoc := canonicalChunkInputText(chunk.Lines, docCtx)
	if extractor.inputTexts[0] != wantDoc {
		t.Fatalf("mentions pass document = %q, want canonical chunk text %q", extractor.inputTexts[0], wantDoc)
	}
	if extractor.inputTexts[1] != wantDoc {
		t.Fatalf("relations pass document = %q, want byte-identical to mentions pass document %q (required to hit the warm cache)", extractor.inputTexts[1], wantDoc)
	}
}

func TestExtractProductPayloadWithFallback_EmptyPrimaryResponseSkipsFallback(t *testing.T) {
	extractor := &fakeJSONExtractor{
		errs: []error{
			errors.New("(MID_26050174) failed resolveScopedString, error:(MID_26052923) failed resolveScopedString, error:(MID_26050142) decode llm response: unexpected end of JSON input, json:{[]}"),
		},
	}

	p := NewProductsProcessor(nil, nil, extractor, nil)
	p.FallbackModelName = "qwen-plus"

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
[deepseek-v4-flash-products-mentions]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = "sk-test-mentions"
base_url = "https://api.openai.com"
timeout_sec = 90

[deepseek-v4-flash-products-relations]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = "sk-test-relations"
base_url = "https://api.openai.com"
timeout_sec = 100
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("MODEL_DEF_FILE", modelsPath)
	t.Setenv("EXTRACT_PRODUCT_MENTIONS_MODEL_NAME", "deepseek-v4-flash-products-mentions")
	t.Setenv("EXTRACT_PRODUCT_MODEL_NAME", "deepseek-v4-flash-products-relations")

	ref, gotPath, cfg, err := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_PRODUCT_MENTIONS_MODEL_NAME", "EXTRACT_PRODUCT_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	if err != nil {
		t.Fatalf("loadModelConfigFromEnvKeys: %v", err)
	}
	if ref != "deepseek-v4-flash-products-mentions" {
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

// concurrentTranslateExtractor is a thread-safe LLMStructuredJSONExtractor
// fake for exercising translateProductRows' real concurrent multi-batch path
// -- unlike fakeJSONExtractor (a shared, unsynchronized outs queue, fine for
// the package's other sequential-call tests but not safe to share across
// goroutines), this generates each batch's translation from its own input,
// keyed by product_name, so concurrent batches never contend on shared state.
type concurrentTranslateExtractor struct {
	mu         sync.Mutex
	batchSizes []int
	calls      int32
	failBatch  int // 1-indexed call number to fail with a mismatched-length response; 0 = never
	dropIdx    int // local idx (within failBatch) to drop when failBatch matches
}

func (f *concurrentTranslateExtractor) ExtractJSON(context.Context, llmclients.JSONExtractionInput) (map[string]any, error) {
	return nil, fmt.Errorf("concurrentTranslateExtractor only implements ExtractStructuredJSON")
}

func (f *concurrentTranslateExtractor) ExtractStructuredJSON(_ context.Context, in llmclients.JSONExtractionInput, _ llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error) {
	callNum := int(atomic.AddInt32(&f.calls, 1))
	var req struct {
		Products []map[string]any `json:"products"`
	}
	if err := json.Unmarshal([]byte(in.InputText), &req); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.batchSizes = append(f.batchSizes, len(req.Products))
	f.mu.Unlock()

	out := make([]map[string]any, 0, len(req.Products))
	for i, item := range req.Products {
		if callNum == f.failBatch && i == f.dropIdx {
			continue // drop one row so this batch's returned length mismatches
		}
		out = append(out, map[string]any{
			"idx":             item["idx"],
			"product_name_en": strings.TrimSpace(asString(item["product_name"])) + "_EN",
		})
	}
	return &llmclients.StructuredOutputResult{Parsed: map[string]any{"products": toAnySlice(out)}}, nil
}

func toAnySlice(rows []map[string]any) []any {
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = r
	}
	return out
}

func TestTranslateProductRows_ConcurrentBatching(t *testing.T) {
	products := make([]map[string]any, 25)
	for i := range products {
		products[i] = map[string]any{"product_name": fmt.Sprintf("product-%02d", i)}
	}
	extractor := &concurrentTranslateExtractor{}
	p := &ProductsProcessor{
		Logger:             loggerutil.CreateDefaultLogger("MID_TEST_TPR"),
		Extractor:          extractor,
		Now:                time.Now,
		TranslateBatchSize: 10,
		MaxTasks:           4,
		TranslateModelName: "gpt-test",
	}
	var llmCallCount, fallbackCount int
	out, err := p.translateProductRows(context.Background(), products, "evt1", &llmCallCount, &fallbackCount)
	if err != nil {
		t.Fatalf("translateProductRows: %v", err)
	}
	if llmCallCount != 3 {
		t.Fatalf("llmCallCount=%d, want 3 batches of size <=10 for 25 rows", llmCallCount)
	}
	extractor.mu.Lock()
	sizes := append([]int(nil), extractor.batchSizes...)
	extractor.mu.Unlock()
	total := 0
	for _, s := range sizes {
		total += s
	}
	if total != 25 {
		t.Fatalf("batch sizes %v sum to %d, want 25", sizes, total)
	}
	for i, row := range out {
		want := fmt.Sprintf("product-%02d_EN", i)
		if got := row["product_name_en"]; got != want {
			t.Fatalf("row %d: product_name_en=%v, want %q (batching must not cross-assign rows)", i, got, want)
		}
	}
}

func TestTranslateProductRows_MismatchedBatchLengthAppliesRowsMatchedByIdx(t *testing.T) {
	products := make([]map[string]any, 12)
	for i := range products {
		products[i] = map[string]any{"product_name": fmt.Sprintf("product-%02d", i)}
	}
	// Drop local idx 0 (row 0, the first row of the first batch) so that
	// batch's returned length mismatches; the other 5 rows in that batch
	// must still be applied via idx, not discarded.
	extractor := &concurrentTranslateExtractor{failBatch: 1, dropIdx: 0}
	p := &ProductsProcessor{
		Logger:             loggerutil.CreateDefaultLogger("MID_TEST_TPR"),
		Extractor:          extractor,
		Now:                time.Now,
		TranslateBatchSize: 6,
		MaxTasks:           1, // deterministic call order for this assertion
		TranslateModelName: "gpt-test",
	}
	var llmCallCount, fallbackCount int
	out, err := p.translateProductRows(context.Background(), products, "evt1", &llmCallCount, &fallbackCount)
	if err != nil {
		t.Fatalf("translateProductRows: %v", err)
	}
	if got := out[0]["product_name_en"]; got != nil {
		t.Fatalf("row 0 (the dropped idx) should stay untranslated, got %v", got)
	}
	for i := 1; i < 12; i++ {
		want := fmt.Sprintf("product-%02d_EN", i)
		if got := out[i]["product_name_en"]; got != want {
			t.Fatalf("row %d: product_name_en=%v, want %q", i, got, want)
		}
	}
}

func TestTranslateProductRows_MidBatchDropDoesNotMisassignLaterRows(t *testing.T) {
	products := make([]map[string]any, 6)
	for i := range products {
		products[i] = map[string]any{"product_name": fmt.Sprintf("product-%02d", i)}
	}
	// Drop local idx 2 (the middle of the only batch). Naive positional
	// matching would shift rows 3-5's translations onto rows 2-4; idx-based
	// matching must keep every surviving row attached to its own product.
	extractor := &concurrentTranslateExtractor{failBatch: 1, dropIdx: 2}
	p := &ProductsProcessor{
		Logger:             loggerutil.CreateDefaultLogger("MID_TEST_TPR"),
		Extractor:          extractor,
		Now:                time.Now,
		TranslateBatchSize: 6,
		MaxTasks:           1,
		TranslateModelName: "gpt-test",
	}
	var llmCallCount, fallbackCount int
	out, err := p.translateProductRows(context.Background(), products, "evt1", &llmCallCount, &fallbackCount)
	if err != nil {
		t.Fatalf("translateProductRows: %v", err)
	}
	if got := out[2]["product_name_en"]; got != nil {
		t.Fatalf("row 2 (the dropped idx) should stay untranslated, got %v", got)
	}
	for _, i := range []int{0, 1, 3, 4, 5} {
		want := fmt.Sprintf("product-%02d_EN", i)
		if got := out[i]["product_name_en"]; got != want {
			t.Fatalf("row %d: product_name_en=%v, want %q (must not be shifted onto a neighboring row)", i, got, want)
		}
	}
}
