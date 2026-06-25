package docreviews

import (
	"context"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// Local bindings to doc-processing's exported shim surface (review_exports.go).
// The reviewer framework and individual reviewers were relocated here from
// doc-processing; these aliases let the relocated files keep referring to the
// shared helpers by their original (unqualified) names.

// ── Type aliases ─────────────────────────────────────────────────────────────
type (
	Line                   = docprocessing.Line
	LLMJSONExtractor       = docprocessing.LLMJSONExtractor
	DocMetadataStore       = docprocessing.DocMetadataStore
	EntityRelationStore    = docprocessing.EntityRelationStore
	DocMetadataInputRecord = docprocessing.DocMetadataInputRecord
	DocMetadataSQLStore    = docprocessing.DocMetadataSQLStore
	SQLStore               = docprocessing.SQLStore
	LineFileGeneratedEvent = docprocessing.LineFileGeneratedEvent
)

// ── Function / value aliases ─────────────────────────────────────────────────
var (
	isCtxStopped            = docprocessing.IsCtxStopped
	withLLMRecordID         = docprocessing.WithLLMRecordID
	buildDocContextLine     = docprocessing.BuildDocContextLine
	rawLinesToJSON          = docprocessing.RawLinesToJSON
	wrapLinesWithDocContext = docprocessing.WrapLinesWithDocContext
	loadPromptByRef         = docprocessing.LoadPromptByRef
	parseTOMLMap            = docprocessing.ParseTOMLMap
	asString                = docprocessing.AsString
	envInt                  = docprocessing.EnvInt
	newLLMJSONInput         = docprocessing.NewLLMJSONInput

	ResolveInputFilePath        = docprocessing.ResolveInputFilePath
	ParseInputLinesIncludingTOC = docprocessing.ParseInputLinesIncludingTOC

	// ErrPipelineStopped re-exports the doc-processing stop sentinel.
	ErrPipelineStopped = docprocessing.ErrReviewStopped
)

// runConcurrent runs fn over n items with at most maxTasks workers (generic, so
// it cannot be a plain value alias — it forwards to the doc-processing shim).
func runConcurrent[T any](
	ctx context.Context,
	maxTasks, n int,
	fn func(ctx context.Context, i int) (T, error),
) ([]T, error) {
	return docprocessing.RunReviewConcurrent(ctx, maxTasks, n, fn)
}

func newDocReviewLLMJSONInput(
	ctx context.Context,
	promptName string,
	promptText string,
	modelName string,
	inputText string,
	callReason string,
	callLoc string,
) llmclients.JSONExtractionInput {
	in := newLLMJSONInput(ctx, promptName, promptText, modelName, inputText, callReason, callLoc)
	in.DocumentFirst = true
	return in
}
