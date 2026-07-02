package docprocessing

import "testing"

func TestMetricsProcessorImplementsChunkBatch(t *testing.T) {
	var _ ChunkBatchProcessor = (*MetricsProcessor)(nil)
}
