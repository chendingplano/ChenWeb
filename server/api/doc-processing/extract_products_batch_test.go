package docprocessing

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestProductsProcessorImplementsChunkBatch(t *testing.T) {
	var _ ChunkBatchProcessor = (*ProductsProcessor)(nil)
}

// TestProductsProcessor_BatchProcessChunkAccumulatesAndSaves verifies that
// InitChunkBatch + ProcessChunk (via the per-chunk batching coordinator) +
// FinalizeChunkBatch produces the same saved rows HandleEvent would, and --
// the whole point of registering with the coordinator -- that the relations
// pass (run in FinalizeChunkBatch) sends the byte-identical document
// ProcessChunk already sent for that chunk, so it can ride the cache the
// coordinator's staggered scheduling warms across processors.
func TestProductsProcessor_BatchProcessChunkAccumulatesAndSaves(t *testing.T) {
	const recordID = int64(9902)
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_DIR", tmp)
	t.Setenv("ARTIFACT_WEB_DIR", tmp)

	chunk := Chunk{SeqNo: 0, Lines: []MarkedLine{
		{Mark: "r", Line: Line{LineNo: 10, PageNo: 1, LineType: "paragraph", Content: "The infusion pump shall be inspected monthly."}},
	}}
	docCtx := "Test Doc | DOC-BATCH"

	extractor := &fakeJSONExtractor{outs: []map[string]any{
		{"mentions": []any{
			map[string]any{
				"mention_text":      "infusion pump",
				"canonical_hint":    "infusion pump",
				"product_type_hint": "equipment",
				"evidence_quote":    "infusion pump",
				"evidence_lines":    []any{"10"},
				"is_explicit":       true,
				"confidence":        0.9,
				"confidence_reason": "explicit mention",
			},
		}},
		{"products": []any{
			map[string]any{"product_name": "infusion pump", "relation_type": "maintenance_requirement"},
		}},
	}}
	store := &fakeProductsStore{}
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: recordID, StatusRaw: "[]"}}

	p := &ProductsProcessor{
		Logger:             loggerutil.CreateDefaultLogger("MID_TEST_PBATCH"),
		Now:                time.Now,
		Extractor:          extractor,
		Store:              store,
		InputStore:         inputStore,
		MentionPromptText:  "extract mentions",
		MentionPromptRef:   "prompt-extract-product-mentions-v1.md",
		MentionModelName:   "gpt-test",
		RelationPromptText: "enrich relations",
		RelationPromptRef:  "prompt-enrich-product-relations-v1.md",
		RelationModelName:  "gpt-test",
		MaxTasks:           1,
		ProductNames:       &fakeProductNameStore{},
	}

	ctx := context.Background()
	if err := p.InitChunkBatch(ctx, recordID, []Chunk{chunk}, docCtx); err != nil {
		t.Fatalf("InitChunkBatch: %v", err)
	}
	if err := p.ProcessChunk(ctx, 0); err != nil {
		t.Fatalf("ProcessChunk: %v", err)
	}
	if err := p.FinalizeChunkBatch(ctx); err != nil {
		t.Fatalf("FinalizeChunkBatch: %v", err)
	}

	if store.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", store.saveCalled)
	}
	if len(store.lastSave.Products) != 1 {
		t.Fatalf("saved products=%d, want 1", len(store.lastSave.Products))
	}
	row := store.lastSave.Products[0]
	wantRelID := fmt.Sprintf("%d_prd_1", recordID)
	if got := strings.TrimSpace(asString(row["product_rel_id"])); got != wantRelID {
		t.Fatalf("product_rel_id=%q, want %q", got, wantRelID)
	}
	if got := strings.TrimSpace(asString(row["relation_type"])); got != "maintenance_requirement" {
		t.Fatalf("relation_type=%q", got)
	}

	if len(extractor.inputTexts) != 2 {
		t.Fatalf("inputTexts=%v, want 2 (ProcessChunk mentions call + FinalizeChunkBatch relations call)", extractor.inputTexts)
	}
	wantDoc := canonicalChunkInputText(chunk.Lines, docCtx)
	if extractor.inputTexts[0] != wantDoc {
		t.Fatalf("ProcessChunk document = %q, want canonical chunk text %q", extractor.inputTexts[0], wantDoc)
	}
	if extractor.inputTexts[1] != wantDoc {
		t.Fatalf("FinalizeChunkBatch relations document = %q, want byte-identical to ProcessChunk's %q (required to ride the coordinator's warm cache)", extractor.inputTexts[1], wantDoc)
	}
}

// TestProductsProcessor_InitChunkBatchSkipsWhenProductsExist mirrors
// HandleEvent's force=false existence check: when products already exist for
// the record and force=false, InitChunkBatch must set batchSkip so
// ProcessChunk/FinalizeChunkBatch are no-ops and nothing is re-saved.
func TestProductsProcessor_InitChunkBatchSkipsWhenProductsExist(t *testing.T) {
	const recordID = int64(9903)
	store := &fakeProductsStore{exists: true}
	p := &ProductsProcessor{
		Logger:             loggerutil.CreateDefaultLogger("MID_TEST_PBATCH_SKIP"),
		Now:                time.Now,
		Store:              store,
		InputStore:         &fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: recordID, StatusRaw: "[]"}},
		MentionPromptText:  "extract mentions",
		MentionModelName:   "gpt-test",
		RelationPromptText: "enrich relations",
		RelationModelName:  "gpt-test",
	}

	// force=false via an explicit context (docProcessorFlagsFromContext
	// defaults an unset context to force=true, so this must set it explicitly).
	ctx := withDocProcessorFlags(context.Background(), false, false)
	if err := p.InitChunkBatch(ctx, recordID, []Chunk{{SeqNo: 0}}, ""); err != nil {
		t.Fatalf("InitChunkBatch: %v", err)
	}
	if !p.batchSkip {
		t.Fatalf("batchSkip=false, want true when products already exist and force=false")
	}
	if err := p.ProcessChunk(ctx, 0); err != nil {
		t.Fatalf("ProcessChunk: %v", err)
	}
	if err := p.FinalizeChunkBatch(ctx); err != nil {
		t.Fatalf("FinalizeChunkBatch: %v", err)
	}
	if store.saveCalled != 0 {
		t.Fatalf("saveCalled=%d, want 0 (skip must not save)", store.saveCalled)
	}
}
