package kbhandler

import (
	"strings"
	"testing"
)

func TestChunkSearchPartitionAndSemanticWindows(t *testing.T) {
	tables := paradeDBPartitions("all", artifactSearchFilters{ArtifactTypes: []string{"chunk"}})
	if len(tables) != 1 || tables[0] != "kb.search_artifacts_chunk" {
		t.Fatalf("chunk partition: %v", tables)
	}
	sql := hybridSemanticAndFusedCTEs(" AND sa.input_record_id = $3", registrySearchConfig{candidateLimit: 12})
	if !strings.Contains(sql, "kb.chunk_search_windows") || !strings.Contains(sql, "MIN(distance)") || strings.Count(sql, "sa.input_record_id = $3") != 2 {
		t.Fatalf("missing window deduplication or filters: %s", sql)
	}
}
