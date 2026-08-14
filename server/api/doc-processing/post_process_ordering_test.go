package docprocessing

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// orderedIndexer records when its PostProcessIndex ran relative to a shared
// counter, so tests can prove ordering without relying on timing.
type orderedIndexer struct {
	name      string
	dependsOn []string
	delay     time.Duration
	seq       *sequenceCounter
	ranAfter  int // sequence number observed at the moment this processor ran
}

func (p *orderedIndexer) Name() string                              { return p.name }
func (p *orderedIndexer) HandleEvent(context.Context, []byte) error { return nil }
func (p *orderedIndexer) PostProcessDependsOn() []string             { return p.dependsOn }
func (p *orderedIndexer) PostProcessIndex(_ context.Context, _ int64) error {
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	p.ranAfter = p.seq.next()
	return nil
}

// sequenceCounter is a trivial thread-safe monotonically increasing counter.
type sequenceCounter struct {
	mu sync.Mutex
	n  int
}

func (c *sequenceCounter) next() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return c.n
}

func TestRunPostProcessIndexingRunsDependentAfterItsDependency(t *testing.T) {
	seq := &sequenceCounter{}
	// dependency is slow; the dependent must still observe it finished first.
	dependency := &orderedIndexer{name: "extract_entity_relation", delay: 30 * time.Millisecond, seq: seq}
	dependent := &orderedIndexer{name: "extract_product_structure", dependsOn: []string{"extract_entity_relation"}, seq: seq}

	s := &ControlService{}
	s.runPostProcessIndexing(context.Background(), []Processor{dependent, dependency}, 42)

	if dependency.ranAfter == 0 || dependent.ranAfter == 0 {
		t.Fatalf("both processors should have run: dependency=%d dependent=%d", dependency.ranAfter, dependent.ranAfter)
	}
	if dependent.ranAfter < dependency.ranAfter {
		t.Fatalf("dependent ran before its dependency finished: dependency seq=%d, dependent seq=%d", dependency.ranAfter, dependent.ranAfter)
	}
}

func TestRunPostProcessIndexingSkipsAbsentDependencyWithoutHanging(t *testing.T) {
	seq := &sequenceCounter{}
	dependent := &orderedIndexer{name: "extract_product_structure", dependsOn: []string{"extract_entity_relation"}, seq: seq}

	done := make(chan struct{})
	go func() {
		s := &ControlService{}
		s.runPostProcessIndexing(context.Background(), []Processor{dependent}, 42)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runPostProcessIndexing hung waiting on a dependency that was never invoked this run")
	}
	if dependent.ranAfter == 0 {
		t.Fatal("dependent never ran")
	}
}

// failingIndexer is a PostProcessIndexer whose PostProcessIndex always
// returns err.
type failingIndexer struct {
	name string
	err  error
}

func (p *failingIndexer) Name() string                              { return p.name }
func (p *failingIndexer) HandleEvent(context.Context, []byte) error { return nil }
func (p *failingIndexer) PostProcessIndex(context.Context, int64) error {
	return p.err
}

// TestRunPostProcessIndexingPersistsFailedStatusOnError locks in ADR
// 2026081401 DR4: before this fix, a PostProcessIndex error was only logged
// and recorded on an OTel span -- kb.inputs.status never reflected it, for
// any processor. Now it must reach kb.inputs.status as "failed", the same
// way a Phase A/B failure already does.
func TestRunPostProcessIndexingPersistsFailedStatusOnError(t *testing.T) {
	store := &fakeDocProcessingCommandStore{
		records: map[int64]DocMetadataInputRecord{42: {ID: 42}},
	}
	s := &ControlService{InputStore: store}
	indexer := &failingIndexer{name: "associate_semantics", err: errors.New("blocked on ungoverned vocabulary")}

	s.runPostProcessIndexing(context.Background(), []Processor{indexer}, 42)

	rec, err := store.GetInputRecord(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetInputRecord: %v", err)
	}
	if !strings.Contains(rec.StatusRaw, `"proc_status":"failed"`) {
		t.Fatalf("StatusRaw = %q, want a failed proc_status entry for associate_semantics", rec.StatusRaw)
	}
	if !strings.Contains(rec.StatusRaw, `"operation":"associate_semantics"`) {
		t.Fatalf("StatusRaw = %q, want an entry naming operation associate_semantics", rec.StatusRaw)
	}
}

// TestRunPostProcessIndexingLeavesStatusUntouchedOnSuccess locks in that DR4
// only adds a write on the error path -- a successful PostProcessIndex must
// not start writing a status entry it didn't write before this change.
func TestRunPostProcessIndexingLeavesStatusUntouchedOnSuccess(t *testing.T) {
	store := &fakeDocProcessingCommandStore{
		records: map[int64]DocMetadataInputRecord{42: {ID: 42}},
	}
	s := &ControlService{InputStore: store}
	indexer := &orderedIndexer{name: "associate_semantics", seq: &sequenceCounter{}}

	s.runPostProcessIndexing(context.Background(), []Processor{indexer}, 42)

	rec, err := store.GetInputRecord(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetInputRecord: %v", err)
	}
	if rec.StatusRaw != "" {
		t.Fatalf("StatusRaw = %q, want unchanged (empty) on success", rec.StatusRaw)
	}
}
