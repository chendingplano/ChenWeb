package docprocessing

import (
	"context"
	"net/http"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// This file exports a thin, stable surface of doc-processing internals so the
// docreviews package — which owns the reviewer framework and the individual
// reviewers after they were relocated out of doc-processing — can reuse the
// shared line-file, LLM, config, and concurrency helpers without exporting each
// helper at its (widely-used) definition site.
//
// These are intentionally thin pass-throughs. The reviewer code binds them to
// local names (see docreviews/review_framework_aliases.go) so the relocated
// files read the same as before the move.

// ErrReviewStopped re-exports the pipeline stop sentinel for reviewers.
var ErrReviewStopped = ErrPipelineStopped

// IsCtxStopped reports whether the context carries a pipeline-stop signal.
func IsCtxStopped(ctx context.Context) bool { return isCtxStopped(ctx) }

// WithLLMRecordID tags the context with the record under review for LLM capture.
func WithLLMRecordID(ctx context.Context, recordID int64) context.Context {
	return withLLMRecordID(ctx, recordID)
}

// BuildDocContextLine renders the doc_context envelope string for a record.
func BuildDocContextLine(rec DocMetadataInputRecord) string { return buildDocContextLine(rec) }

// RawLinesToJSON serializes lines to the reviewer input JSON shape.
func RawLinesToJSON(lines []Line) string { return rawLinesToJSON(lines) }

// WrapLinesWithDocContext wraps a lines JSON payload in the doc_context envelope.
func WrapLinesWithDocContext(linesJSON, docCtx string) string {
	return wrapLinesWithDocContext(linesJSON, docCtx)
}

// LoadPromptByRef resolves a prompt file reference to its text.
func LoadPromptByRef(promptRef string) (promptText, promptRefOut, promptPath string, err error) {
	return loadPromptByRef(promptRef)
}

// ParseTOMLMap parses TOML bytes into out (used by reviewer config loading).
func ParseTOMLMap(data []byte, out any) error { return parseTOMLMap(data, out) }

// AsString coerces an arbitrary JSON value to a trimmed string.
func AsString(v any) string { return asString(v) }

// EnvInt reads an int env var with a fallback and minimum.
func EnvInt(key string, fallback, min int) int { return envInt(key, fallback, min) }

// NewLLMJSONInput builds a JSON extraction input for a reviewer LLM call.
func NewLLMJSONInput(
	ctx context.Context,
	promptName, promptText, modelName, inputText, callReason, callLoc string,
) llmclients.JSONExtractionInput {
	return newLLMJSONInput(ctx, promptName, promptText, modelName, inputText, callReason, callLoc)
}

// RunReviewConcurrent runs fn over n items with at most maxTasks workers,
// collecting the per-item results (mirrors the internal runConcurrent helper).
func RunReviewConcurrent[T any](
	ctx context.Context,
	maxTasks, n int,
	fn func(ctx context.Context, i int) (T, error),
) ([]T, error) {
	return runConcurrent(ctx, maxTasks, n, fn)
}

// BuildReviewerLLMClient resolves a model reference and constructs a configured
// JSON-extraction client for a reviewer. It encapsulates the (unexported)
// model-config type so callers in other packages never need to see it.
func BuildReviewerLLMClient(modelRef string) (client LLMJSONExtractor, modelName string, err error) {
	_, _, cfg, err := loadModelConfigByRef(modelRef, "MODEL_DEF_FILE")
	if err != nil {
		return nil, "", err
	}
	timeoutSec := cfg.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 100
	}
	c, err := llmclients.NewOpenAIJSONClientFromConfig(llmclients.OpenAIJSONClientConfig{
		ModelName:    cfg.ModelName,
		APIKey:       cfg.APIKey,
		BaseURL:      cfg.BaseURL,
		ProfileName:  cfg.ProfileName,
		ThinkingType: cfg.ThinkingType,
		TimeoutSec:   timeoutSec,
	}, nil)
	if err != nil {
		return nil, "", err
	}
	c.HTTPClient = &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	return c, cfg.ModelName, nil
}

// BuildReviewerToolClient resolves a model reference and constructs a
// tool-capable chat client (shared llm.Client) for tool-use reviewers (DR10b).
// Unlike BuildReviewerLLMClient — which returns the JSON-only ExtractJSON
// interface with no function-calling support — this returns the full
// Complete(ctx, Request{Tools,...}) path. DeepSeek and other OpenAI-compatible
// models support function calling through this client.
func BuildReviewerToolClient(modelRef string) (client llmclients.Client, modelName string, err error) {
	_, _, cfg, err := loadModelConfigByRef(modelRef, "MODEL_DEF_FILE")
	if err != nil {
		return nil, "", err
	}
	timeoutSec := cfg.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 100
	}
	c, err := llmclients.NewClient(llmclients.ProviderConfig{
		ID:          llmclients.ProviderOpenAICompatible,
		BaseURL:     cfg.BaseURL,
		APIKey:      cfg.APIKey,
		ProfileName: modelRef,
		HTTPClient:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	})
	if err != nil {
		return nil, "", err
	}
	return c, cfg.ModelName, nil
}
