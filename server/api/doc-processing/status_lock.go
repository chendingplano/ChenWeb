package docprocessing

import "sync"

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
