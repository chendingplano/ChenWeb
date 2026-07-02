package docprocessing

import (
	"context"
	"os"
	"testing"
)

// fakeBatchProc is a minimal ChunkBatchProcessor for scheduler tests.
type fakeBatchProc struct{ name string }

func (f *fakeBatchProc) Name() string { return f.name }
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
}
