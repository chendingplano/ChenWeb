package docprocessing

import (
	"context"
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
