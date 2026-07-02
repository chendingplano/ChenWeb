package docprocessing

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// nonBatchProc implements Processor but NOT ChunkBatchProcessor.
type nonBatchProc struct {
	name string
	ran  int32
}

func (n *nonBatchProc) Name() string { return n.name }
func (n *nonBatchProc) HandleEvent(context.Context, []byte) error {
	atomic.AddInt32(&n.ran, 1)
	return nil
}

func TestPartitionBatchProcessors(t *testing.T) {
	batch := &fakeBatchProc{name: "extract_inventory_items"}
	legacy := &nonBatchProc{name: "generate_topics"}
	got := partitionBatchProcessors([]Processor{batch, legacy})
	if len(got.batch) != 1 || got.batch[0].Name() != "extract_inventory_items" {
		t.Fatalf("batch partition wrong: %+v", got.batch)
	}
	if len(got.unsupported) != 1 || got.unsupported[0].Name() != "generate_topics" {
		t.Fatalf("unsupported partition wrong: %+v", got.unsupported)
	}
}

// recordingProc records the wall-clock start of each ProcessChunk call and how
// many run concurrently, so tests can assert seed concurrency and staggering.
type recordingProc struct {
	name       string
	delay      time.Duration
	mu         sync.Mutex
	starts     []time.Time
	concurrent int32
	maxConc    int32
}

func (r *recordingProc) Name() string { return r.name }
func (r *recordingProc) InitChunkBatch(context.Context, int64, []Chunk, string) error { return nil }
func (r *recordingProc) FinalizeChunkBatch(context.Context) error                     { return nil }
func (r *recordingProc) ProcessChunk(ctx context.Context, idx int) error {
	n := atomic.AddInt32(&r.concurrent, 1)
	for {
		old := atomic.LoadInt32(&r.maxConc)
		if n <= old || atomic.CompareAndSwapInt32(&r.maxConc, old, n) {
			break
		}
	}
	r.mu.Lock()
	r.starts = append(r.starts, time.Now())
	r.mu.Unlock()
	time.Sleep(r.delay)
	atomic.AddInt32(&r.concurrent, -1)
	return nil
}

func TestSeedPhaseRunsChunksConcurrently(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")
	seed := &recordingProc{name: "extract_inventory_items", delay: 50 * time.Millisecond}
	remainder := &recordingProc{name: "extract_provisions", delay: 10 * time.Millisecond}

	s := &ControlService{}
	chunks := make([]Chunk, 4)
	// Exercise only the scheduling core via the extracted helper (Step 3 extracts it).
	s.scheduleChunkBatch(context.Background(),
		[]ChunkBatchProcessor{seed, remainder}, chunks, 1)

	if got := atomic.LoadInt32(&seed.maxConc); got < 2 {
		t.Fatalf("seed chunks should overlap; max concurrency was %d, want >= 2", got)
	}
	if len(seed.starts) != 4 || len(remainder.starts) != 4 {
		t.Fatalf("each processor should run all 4 chunks; seed=%d remainder=%d",
			len(seed.starts), len(remainder.starts))
	}
}
