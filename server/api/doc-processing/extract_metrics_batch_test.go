package docprocessing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestMetricsProcessorImplementsChunkBatch(t *testing.T) {
	var _ ChunkBatchProcessor = (*MetricsProcessor)(nil)
}

// TestMetricsProcessor_BatchProcessChunkAccumulatesAndSaves verifies that
// InitChunkBatch + ProcessChunk (×N chunks) + FinalizeChunkBatch accumulates
// candidate mentions across chunks and saves the enriched metric rows with
// correct metric_id assignments.
func TestMetricsProcessor_BatchProcessChunkAccumulatesAndSaves(t *testing.T) {
	const recordID = int64(9901)

	// Two chunks, each yielding one candidate in Pass 1; Pass 2 enriches both.
	// groupCandidatesByChunk splits by ChunkIndex, so 2 chunks → 2 Pass-2 batches.
	ext := &metricsSeqErrExtractor{
		outs: []map[string]any{
			// Pass 1 – chunk 0
			{
				"language": "en",
				"candidates": []any{
					map[string]any{
						"metric_name_hint":  "Throughput",
						"subject_hint":      "system",
						"evidence_quote":    "metric text 10",
						"source_line_spans": []any{float64(10)},
						"unit_hint":         "rps",
						"value_hint":        "1000",
						"confidence":        0.9,
					},
				},
			},
			// Pass 1 – chunk 1
			{
				"language": "en",
				"candidates": []any{
					map[string]any{
						"metric_name_hint":  "Latency",
						"subject_hint":      "p99",
						"evidence_quote":    "metric text 20",
						"source_line_spans": []any{float64(20)},
						"unit_hint":         "ms",
						"value_hint":        "200",
						"confidence":        0.85,
					},
				},
			},
			// Pass 2 – batch 1 (chunk 0 candidate)
			{
				"language": "en",
				"metrics": []any{
					map[string]any{
						"metric_name":       "Throughput",
						"source_line_spans": []any{float64(10)},
					},
				},
				"uncertain_metrics": []any{},
			},
			// Pass 2 – batch 2 (chunk 1 candidate)
			{
				"language": "en",
				"metrics": []any{
					map[string]any{
						"metric_name":       "Latency",
						"source_line_spans": []any{float64(20)},
					},
				},
				"uncertain_metrics": []any{},
			},
		},
		errs: []error{nil, nil, nil, nil},
	}

	store := &fakeMetricsStore{}
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:        recordID,
		StatusRaw: "[]",
	}}

	p := &MetricsProcessor{
		Logger:                loggerutil.CreateDefaultLogger("TEST_BATCH"),
		ProcLogger:            DocProcLogger{DB: nil},
		Now:                   time.Now,
		Extractor:             ext,
		Store:                 store,
		InputStore:            inputStore,
		MentionPromptText:     "candidates prompt",
		MentionPromptRef:      "test-candidates-prompt",
		MentionModelName:      "test-mention-model",
		RelationPromptText:    "enrich prompt",
		RelationPromptRef:     "test-enrich-prompt",
		RelationModelName:     "test-relation-model",
		MetricEnrichGroupSize: 10, // keep both candidates in one Pass-2 batch
		MaxTasks:              1,
		ChunkDir:              t.TempDir(),
	}

	// Build two minimal chunks.
	chunks := []Chunk{
		metricsBlocksToChunks([]Block{makeMetricsBlock(0, 10)})[0],
		metricsBlocksToChunks([]Block{makeMetricsBlock(1, 20)})[0],
	}

	ctx := context.Background()
	if err := p.InitChunkBatch(ctx, recordID, chunks, ""); err != nil {
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

	// Store must have been called exactly once.
	if store.saveCalled != 1 {
		t.Fatalf("saveCalled=%d, want 1", store.saveCalled)
	}

	// Both enriched metrics must be present.
	got := store.lastSave.Metrics
	if len(got) != 2 {
		t.Fatalf("saved %d metrics, want 2; metrics=%v", len(got), got)
	}

	// metric_id must follow the "{recordID}_mtc_{1-based}" pattern.
	for i, m := range got {
		want := fmt.Sprintf("%d_mtc_%d", recordID, i+1)
		if id, _ := m["metric_id"].(string); id != want {
			t.Errorf("metrics[%d].metric_id=%q, want %q", i, id, want)
		}
	}

	// PromptName must use RelationPromptRef (Fix 3).
	if store.lastSave.PromptName != p.RelationPromptRef {
		t.Errorf("PromptName=%q, want %q", store.lastSave.PromptName, p.RelationPromptRef)
	}
}
