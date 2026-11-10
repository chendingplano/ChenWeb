package docprocessing

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
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

// WithLLMRunID tags the context with the allocated LLM execution ID for LLM capture metadata.
func WithLLMRunID(ctx context.Context, runID int64) context.Context {
	return withLLMRunID(ctx, runID)
}

// LLMRunIDFromContext returns the allocated LLM execution ID tagged via WithLLMRunID, or 0 if unset.
func LLMRunIDFromContext(ctx context.Context) int64 {
	return llmRunIDFromContext(ctx)
}

// WithLLMUserID carries the initiating user through a document review.
func WithLLMUserID(ctx context.Context, userID string) context.Context {
	return withLLMUserID(ctx, userID)
}

// LLMUserIDFromContext returns the initiating user, if present.
func LLMUserIDFromContext(ctx context.Context) string {
	return llmUserIDFromContext(ctx)
}

// BuildDocContextLine renders the doc_context envelope string for a record.
func BuildDocContextLine(rec DocMetadataInputRecord) string { return buildDocContextLine(rec) }

// RawLinesToJSON serializes lines to the reviewer input JSON shape.
func RawLinesToJSON(lines []Line) string { return rawLinesToJSON(lines) }

// WrapLinesWithDocContext wraps a lines JSON payload in the doc_context envelope.
func WrapLinesWithDocContext(linesJSON, docCtx string) string {
	return wrapLinesWithDocContext(linesJSON, docCtx)
}

// LoadRecordChunks returns the record's extraction chunks, the units the
// chunk-based extractors read, from its saved .chunks file under ARTIFACT_DIR
// (the file the per-chunk batching coordinator reads). When there is no saved
// file it rebuilds them with BuildChunks and the configured CHUNK_SIZE /
// CHUNK_OVERLAP_PERCENT, and reports rebuilt=true.
func LoadRecordChunks(rec DocMetadataInputRecord, lines []Line) (chunks []Chunk, rebuilt bool, err error) {
	if artDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR")); artDir != "" {
		artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
		chunks, err = loadChunksFromArtifactFile(artDir, rec.ID, artifactBase+".chunks", lines)
		if err == nil {
			return chunks, false, nil
		}
		if !isChunkArtifactNotFoundError(err) {
			return nil, false, err
		}
	}
	chunks, err = BuildChunks(lines, ChunkOptions{
		ChunkSize:      envInt("CHUNK_SIZE", DefaultChunkSize, 1),
		OverlapPercent: envInt("CHUNK_OVERLAP_PERCENT", DefaultOverlapPercent, 0),
	})
	return chunks, true, err
}

// LoadPromptByRef resolves a prompt file reference to its text.
func LoadPromptByRef(promptRef string) (promptText, promptRefOut, promptPath string, err error) {
	return loadPromptByRef(promptRef)
}

// ResolveModelsFilePath finds the .models.toml file (env override, MODELS_FILE, or the nearest
// .models.toml above the working directory).
func ResolveModelsFilePath(modelsFileEnv string) (string, error) {
	return resolveModelsFilePath(modelsFileEnv)
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
		Host:         cfg.Host,
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
func BuildReviewerToolClient(modelRef string, logger ApiTypes.JimoLogger) (client llmclients.Client, modelName string, err error) {
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
	}, logger)
	if err != nil {
		return nil, "", err
	}
	return c, cfg.ModelName, nil
}
