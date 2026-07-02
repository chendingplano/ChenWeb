package docprocessing

import (
	"context"
	"os"
	"testing"
)

// fakeBatchProc is a minimal ChunkBatchProcessor for scheduler tests.
type fakeBatchProc struct{ name string }

func (f *fakeBatchProc) Name() string                                                 { return f.name }
func (f *fakeBatchProc) HandleEvent(context.Context, []byte) error                    { return nil }
func (f *fakeBatchProc) InitChunkBatch(context.Context, int64, []Chunk, string) error { return nil }
func (f *fakeBatchProc) ProcessChunk(context.Context, int) error                      { return nil }
func (f *fakeBatchProc) FinalizeChunkBatch(context.Context) error                     { return nil }

func TestMaxDocProcessorTasks(t *testing.T) {
	os.Unsetenv("MAX_DOC_PROCESSOR_TASKS")
	if got := maxDocProcessorTasks(10); got != 10 {
		t.Fatalf("default: want 10, got %d", got)
	}
	os.Setenv("MAX_DOC_PROCESSOR_TASKS", "3")
	defer os.Unsetenv("MAX_DOC_PROCESSOR_TASKS")
	if got := maxDocProcessorTasks(10); got != 3 {
		t.Fatalf("env: want 3, got %d", got)
	}
}

func TestOrderBatchProcessorsSeedFirst(t *testing.T) {
	metrics := &fakeBatchProc{name: "extract_metrics"} // multi-pass
	inv := &fakeBatchProc{name: "extract_inventory_items"}
	ordered := orderBatchProcessorsSeedFirst([]ChunkBatchProcessor{metrics, inv})
	if ordered[0].Name() != "extract_inventory_items" {
		t.Fatalf("seed must be 1-pass, got %s", ordered[0].Name())
	}

	// All multi-pass — must return original order unchanged.
	m1 := &fakeBatchProc{name: "extract_metrics"}
	m2 := &fakeBatchProc{name: "extract_semantic_projections"}
	allMulti := orderBatchProcessorsSeedFirst([]ChunkBatchProcessor{m1, m2})
	if allMulti[0].Name() != "extract_metrics" || allMulti[1].Name() != "extract_semantic_projections" {
		t.Fatalf("all-multi: want original order, got %s,%s", allMulti[0].Name(), allMulti[1].Name())
	}

	// Already seed-first — 1-pass processor stays first, order preserved.
	inv2 := &fakeBatchProc{name: "extract_inventory_items"}
	prov := &fakeBatchProc{name: "extract_provisions"}
	already := orderBatchProcessorsSeedFirst([]ChunkBatchProcessor{inv2, prov})
	if already[0].Name() != "extract_inventory_items" || already[1].Name() != "extract_provisions" {
		t.Fatalf("already-seed-first: want unchanged order, got %s,%s", already[0].Name(), already[1].Name())
	}
}
