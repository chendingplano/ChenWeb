package docprocessing

import "testing"

// The split processors must each implement ChunkBatchProcessor independently,
// and must NOT share one combined implementation via embedding.
func TestEntityAndRelationImplementBatchSeparately(t *testing.T) {
	var _ ChunkBatchProcessor = (*EntityProcessor)(nil)
	var _ ChunkBatchProcessor = (*RelationProcessor)(nil)

	// The embedded core must no longer expose ProcessChunk as part of the
	// interface (renamed away), so *EntityRelationProcessor alone is not a
	// ChunkBatchProcessor.
	if _, ok := interface{}((*EntityRelationProcessor)(nil)).(ChunkBatchProcessor); ok {
		t.Fatal("combined EntityRelationProcessor must not satisfy ChunkBatchProcessor after split")
	}
}
