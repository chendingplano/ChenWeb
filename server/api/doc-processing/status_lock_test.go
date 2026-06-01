package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLockRecordStatus_SerializesSameRecord(t *testing.T) {
	const goroutines = 50
	var counter int
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := lockRecordStatus(42)
			defer unlock()
			counter++ // racy without the lock; -race must stay clean
		}()
	}
	wg.Wait()
	if counter != goroutines {
		t.Fatalf("counter=%d, want %d", counter, goroutines)
	}
}

func TestLockRecordStatus_DifferentRecordsDoNotDeadlock(t *testing.T) {
	unlockA := lockRecordStatus(1)
	unlockB := lockRecordStatus(2) // different shard target; must not block
	unlockB()
	unlockA()
}

// fakeStatusStore simulates the kb.inputs.status DB cell. Its UpdateInputMetadata
// sleeps to widen the read-modify-write window so a missing lock loses updates.
type fakeStatusStore struct {
	mu  sync.Mutex // guards the simulated DB cell only (not the RMW window)
	raw string
}

func (s *fakeStatusStore) GetInputRecord(_ context.Context, _ int64) (DocMetadataInputRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return DocMetadataInputRecord{StatusRaw: s.raw}, nil
}

func (s *fakeStatusStore) UpdateInputMetadata(_ context.Context, _ int64, upd DocMetadataUpdate) error {
	time.Sleep(time.Millisecond) // widen the RMW window so a missing lock loses updates
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw = upd.StatusRaw
	return nil
}

func TestUpdateInputStatusAtomic_NoLostUpdates(t *testing.T) {
	store := &fakeStatusStore{raw: "[]"}
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = updateInputStatusAtomic(context.Background(), store, 7, func(cur string) (DocMetadataUpdate, error) {
				var arr []map[string]any
				_ = json.Unmarshal([]byte(cur), &arr)
				arr = append(arr, map[string]any{"operation": fmt.Sprintf("op-%d", i)})
				b, _ := json.Marshal(arr)
				return DocMetadataUpdate{StatusRaw: string(b)}, nil
			})
		}()
	}
	wg.Wait()
	var arr []map[string]any
	if err := json.Unmarshal([]byte(store.raw), &arr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(arr) != n {
		t.Fatalf("got %d entries, want %d (lost updates)", len(arr), n)
	}
}
