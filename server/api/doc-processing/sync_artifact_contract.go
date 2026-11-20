package docprocessing

// SyncOperationName normalizes a recorded operation for document sync.
func SyncOperationName(operation string) string { return canonicalOperationName(operation) }

// SyncProcessorAliases returns the status operations owned by a processor.
func SyncProcessorAliases(processor string) []string { return processorStatusAliases(processor) }

// SyncArtifactBaseName returns the artifact filename stem used by processors.
func SyncArtifactBaseName(stagingFilename, parser string) string {
	return buildChunkArtifactBaseName(stagingFilename, parser)
}
