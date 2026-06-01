package docprocessing

import (
	"sync"
	"testing"
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
