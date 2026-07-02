package docprocessing

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// fakeSemanticProjectionsStore captures SaveSemanticProjections calls.
type fakeSemanticProjectionsStore struct {
	mu         sync.Mutex
	saveCalled int
	lastReq    SaveSemanticProjectionsRequest
}

func (f *fakeSemanticProjectionsStore) SemanticProjectionsExist(_ context.Context, _ int64) (bool, error) {
	return false, nil
}
func (f *fakeSemanticProjectionsStore) DeleteSemanticProjectionsByInputRecordID(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}
func (f *fakeSemanticProjectionsStore) SaveSemanticProjections(_ context.Context, req SaveSemanticProjectionsRequest) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveCalled++
	f.lastReq = req
	return int64(len(req.Projections)), nil
}

// batchSeqExtractor is a sequential fake LLMJSONExtractor for the batch tests.
// Responses are consumed in the order they are registered.
type batchSeqExtractor struct {
	mu      sync.Mutex
	outs    []map[string]any
	errs    []error
	callIdx int
}

func (e *batchSeqExtractor) ExtractJSON(_ context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := e.callIdx
	e.callIdx++
	if i >= len(e.outs) {
		return nil, fmt.Errorf("batchSeqExtractor: unexpected call index %d (only %d responses configured)", i, len(e.outs))
	}
	if e.errs[i] != nil {
		return nil, e.errs[i]
	}
	return e.outs[i], nil
}

// TestSemanticProjectionsImplementsChunkBatch is the compile-time interface check (TDD step 1).
func TestSemanticProjectionsImplementsChunkBatch(t *testing.T) {
	var _ ChunkBatchProcessor = (*SemanticProjectionsProcessor)(nil)
}

// TestSemanticProjectionsBatch_AccumulatesAndSaves verifies the full batch path:
// InitChunkBatch → ProcessChunk×2 → FinalizeChunkBatch saves accumulated projections.
//
// Extractor call order (MaxTasks=1, sequential per-chunk):
//
//	call 0: chunk 0 Pass-1 candidate  → semantic_projection + keywords
//	call 1: chunk 0 Pass-2 enrich     → enriched projection (language=zh, descriptive_name=Name0)
//	call 2: chunk 1 Pass-1 candidate  → semantic_projection + keywords
//	call 3: chunk 1 Pass-2 enrich     → enriched projection (language=zh, descriptive_name=Name1)
func TestSemanticProjectionsBatch_AccumulatesAndSaves(t *testing.T) {
	const recordID = int64(7701)

	ext := &batchSeqExtractor{
		outs: []map[string]any{
			// call 0: chunk 0, Pass 1 candidate
			{
				"semantic_projection": "proj-chunk-0",
				"keywords":            []any{"kw0"},
			},
			// call 1: chunk 0, Pass 2 enrich
			{
				"language":               "zh",
				"descriptive_name":       "Name0",
				"descriptive_name_en":    "Name0-en",
				"keywords":               []any{"kw0"},
				"keywords_en":            []any{"kw0-en"},
				"semantic_projection":    "proj-chunk-0-enriched",
				"semantic_projection_en": "proj-chunk-0-enriched-en",
				"category_paths":         []any{},
				"category_paths_en":      []any{},
			},
			// call 2: chunk 1, Pass 1 candidate
			{
				"semantic_projection": "proj-chunk-1",
				"keywords":            []any{"kw1"},
			},
			// call 3: chunk 1, Pass 2 enrich
			{
				"language":               "zh",
				"descriptive_name":       "Name1",
				"descriptive_name_en":    "Name1-en",
				"keywords":               []any{"kw1"},
				"keywords_en":            []any{"kw1-en"},
				"semantic_projection":    "proj-chunk-1-enriched",
				"semantic_projection_en": "proj-chunk-1-enriched-en",
				"category_paths":         []any{},
				"category_paths_en":      []any{},
			},
		},
		errs: []error{nil, nil, nil, nil},
	}

	store := &fakeSemanticProjectionsStore{}
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		StagingFilename: "test_staging.pdf",
		ParserName:      "opendata",
		StatusRaw:       "[]",
	}}

	p := &SemanticProjectionsProcessor{
		Extractor:           ext,
		Store:               store,
		InputStore:          inputStore,
		Logger:              noopLogger{},
		ProcLogger:          DocProcLogger{},
		Now:                 time.Now,
		CandidateModelName:  "cand-model",
		CandidatePromptRef:  "test-candidate-prompt",
		CandidatePromptText: "candidate task prompt",
		EnrichModelName:     "enrich-model",
		EnrichPromptRef:     "test-enrich-prompt",
		EnrichPromptText:    "enrich task prompt",
		MaxTasks:            1,
		// ArtifactDir / ArtifactWebDir intentionally left empty so that
		// saveSemanticProjectionsToFile / indexSemanticProjections log a
		// warning (missing env) rather than writing real files.
	}

	chunks := []Chunk{
		{SeqNo: 1, Lines: []MarkedLine{{Line: Line{Raw: "line 1", LineNo: 1}}}},
		{SeqNo: 2, Lines: []MarkedLine{{Line: Line{Raw: "line 2", LineNo: 2}}}},
	}

	ctx := context.Background()

	if err := p.InitChunkBatch(ctx, recordID, chunks, "doc-ctx"); err != nil {
		t.Fatalf("InitChunkBatch: %v", err)
	}
	for i := range chunks {
		if err := p.ProcessChunk(ctx, i); err != nil {
			t.Fatalf("ProcessChunk(%d): %v", i, err)
		}
	}
	if err := p.FinalizeChunkBatch(ctx); err != nil {
		t.Fatalf("FinalizeChunkBatch: %v", err)
	}

	// Store must be called exactly once.
	if store.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", store.saveCalled)
	}

	got := store.lastReq.Projections
	if len(got) != 2 {
		t.Fatalf("saved %d projections, want 2; got=%v", len(got), got)
	}

	// Each projection must have semantic_proj_id, create_time, and line_spans stamped.
	for i, proj := range got {
		if _, ok := proj["semantic_proj_id"]; !ok {
			t.Errorf("projections[%d] missing semantic_proj_id", i)
		}
		if _, ok := proj["create_time"]; !ok {
			t.Errorf("projections[%d] missing create_time", i)
		}
		if _, ok := proj["line_spans"]; !ok {
			t.Errorf("projections[%d] missing line_spans", i)
		}
	}

	// Verify distinguishing descriptive_name fields for both chunks.
	name0, _ := got[0]["descriptive_name"].(string)
	if name0 != "Name0" {
		t.Errorf("projections[0].descriptive_name=%q, want %q", name0, "Name0")
	}
	name1, _ := got[1]["descriptive_name"].(string)
	if name1 != "Name1" {
		t.Errorf("projections[1].descriptive_name=%q, want %q", name1, "Name1")
	}

	// PromptName must use EnrichPromptRef (matches legacy HandleEvent).
	if store.lastReq.PromptName != p.EnrichPromptRef {
		t.Errorf("PromptName=%q, want %q", store.lastReq.PromptName, p.EnrichPromptRef)
	}

	// Language must be detected from the enrich pass.
	if store.lastReq.Language != "zh" {
		t.Errorf("Language=%q, want %q", store.lastReq.Language, "zh")
	}
}
