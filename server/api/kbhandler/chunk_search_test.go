package kbhandler

import (
	"slices"
	"testing"
)

func TestChunkIsASearchPartitionType(t *testing.T) {
	if !slices.Contains(searchArtifactPartitionTypes, "chunk") {
		t.Fatalf("chunk is not a partition type: %v", searchArtifactPartitionTypes)
	}
}
