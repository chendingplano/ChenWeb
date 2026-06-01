package docprocessing

import (
	"context"
	"sync"
)

// recordStatusLockShards is the number of mutexes striping kb.inputs.status
// writes by record_id. A fixed array keeps memory bounded and leak-free.
const recordStatusLockShards = 256

var recordStatusLocks [recordStatusLockShards]sync.Mutex

// lockRecordStatus acquires the mutex guarding kb.inputs.status writes for the
// given record_id and returns its unlock function. It serializes the
// read-modify-write of the status JSON so concurrent processors do not clobber
// each other's entries.
//
// CONSTRAINT (single instance only): this lock coordinates goroutines within
// one process. If doc-processor is ever scaled to multiple replicas, replace
// this with a DB row lock (SELECT ... FOR UPDATE + jsonb_set) or a shared
// coordinator (Redis / dedicated primary). See
// docs/superpowers/specs/2026-06-01-concurrent-doc-processors-design.md.
func lockRecordStatus(id int64) func() {
	shard := uint64(id) % recordStatusLockShards
	m := &recordStatusLocks[shard]
	m.Lock()
	return m.Unlock
}

// withRecordStatusLock runs fn while holding the per-record status lock. Use it
// to make a GetInputRecord -> append*Status -> Update* sequence atomic when the
// write path is not UpdateInputMetadata (e.g. the chunk-phase
// Store.UpdateInputStatus). fn MUST re-read the current status inside the
// callback so it merges onto the freshest value.
func withRecordStatusLock(id int64, fn func() error) error {
	unlock := lockRecordStatus(id)
	defer unlock()
	return fn()
}

// statusStore is the minimal surface updateInputStatusAtomic needs. It is
// satisfied by DocMetadataStore.
type statusStore interface {
	GetInputRecord(ctx context.Context, id int64) (DocMetadataInputRecord, error)
	UpdateInputMetadata(ctx context.Context, id int64, upd DocMetadataUpdate) error
}

// updateInputStatusAtomic performs the kb.inputs.status read-modify-write under
// the per-record lock. mutate receives the current status JSON and returns the
// DocMetadataUpdate to persist (its StatusRaw must be the new status array;
// other fields may also be set and are written under the same lock).
func updateInputStatusAtomic(
	ctx context.Context,
	store statusStore,
	id int64,
	mutate func(currentStatusRaw string) (DocMetadataUpdate, error),
) error {
	unlock := lockRecordStatus(id)
	defer unlock()
	rec, err := store.GetInputRecord(ctx, id)
	if err != nil {
		return err
	}
	upd, err := mutate(rec.StatusRaw)
	if err != nil {
		return err
	}
	return store.UpdateInputMetadata(ctx, id, upd)
}
