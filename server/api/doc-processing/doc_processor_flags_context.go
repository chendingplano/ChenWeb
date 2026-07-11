package docprocessing

import "context"

type docProcessorFlagsKey struct{}

type docProcessorFlags struct {
	force      bool
	forceClear bool
}

// withDocProcessorFlags attaches the event-level force/force_clear flags to
// ctx so per-chunk-batch lifecycle methods (InitChunkBatch/ProcessChunk/
// FinalizeChunkBatch) can read them without a signature change to the
// shared ChunkBatchProcessor interface (ADR 2026071002 DR1).
func withDocProcessorFlags(ctx context.Context, force, forceClear bool) context.Context {
	return context.WithValue(ctx, docProcessorFlagsKey{}, docProcessorFlags{force: force, forceClear: forceClear})
}

// docProcessorFlagsFromContext returns the attached flags, or (true, false)
// if none were attached — matching the event-level defaults (force defaults
// to true, force_clear defaults to false).
func docProcessorFlagsFromContext(ctx context.Context) (force, forceClear bool) {
	if ctx == nil {
		return true, false
	}
	f, ok := ctx.Value(docProcessorFlagsKey{}).(docProcessorFlags)
	if !ok {
		return true, false
	}
	return f.force, f.forceClear
}
