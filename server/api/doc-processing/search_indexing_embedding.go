package docprocessing

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// maxEmbeddingRunes bounds the text sent to the embedding API. text-embedding-3-small
// accepts up to ~8191 tokens; for CJK text tokens roughly track runes, so a
// conservative rune budget avoids request failures on very large artifacts.
const maxEmbeddingRunes = 6000
const defaultEmbeddingMaxGoroutines = 20
const defaultEmbeddingBatchSize = 8
const embeddingMaxAttempts = 3

type batchEmbedder interface {
	EmbedBatch(ctx context.Context, in llmclients.EmbedBatchInput) ([][]float64, error)
}

type preparedEmbedding struct {
	rowIndex int
	text     string
	runes    int
}

// newSearchEmbedder builds an Embedder from EMBEDDING_MODEL_NAME (+ .models.toml),
// mirroring the chunking service's resolution. Returns ok=false when no embedding
// model is configured, in which case the caller proceeds lexical-only.
func newSearchEmbedder() (embedder Embedder, modelName string, timeoutSec int, ok bool) {
	if strings.TrimSpace(os.Getenv("EMBEDDING_MODEL_NAME")) == "" {
		return nil, "", 0, false
	}
	_, _, cfg, err := loadModelConfigFromEnv("EMBEDDING_MODEL_NAME", "")
	if err != nil || strings.TrimSpace(cfg.ModelName) == "" {
		return nil, "", 0, false
	}
	timeoutSec = cfg.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	client := newOpenAIJSONClientForStructureModel(
		cfg,
		kbsearch.EmbeddingDimensionsForModel(cfg.ModelName, cfg.BaseURL),
	)
	return client, cfg.ModelName, timeoutSec, true
}

// embedRegistryRows fills EmbeddingText + Embedding on each row in place, best-effort.
// Embedding is never fatal to reindexing: a missing model, per-row API failures, and
// dimension mismatches are logged and leave that row lexical-only (NULL embedding).
func embedRegistryRows(
	ctx context.Context,
	rows []kbsearch.RegistryRow,
	logger ApiTypes.JimoLogger,
	callReason string,
	callLoc string,
) {
	if callReason == "" || callLoc == "" {
		logger.Error("missing callReason/callLoc")
	}

	logger.Info("check-call-reason", "callerReason", callReason, "callLoc", callLoc)
	if len(rows) == 0 {
		return
	}
	embedder, modelName, timeoutSec, ok := newSearchEmbedder()
	if !ok {
		if logger != nil {
			logger.Warn("semantic search enabled but no embedding model configured; indexing lexical-only",
				"env", "EMBEDDING_MODEL_NAME")
		}
		return
	}
	maxWorkers := embeddingMaxGoroutines()
	if maxWorkers > len(rows) {
		maxWorkers = len(rows)
	}
	if maxWorkers <= 1 {
		maxWorkers = 1
	}
	batchSize := embeddingBatchSize()
	if maxItems := kbsearch.EmbeddingMaxBatchItemsForModel(modelName, ""); maxItems > 0 && batchSize > maxItems {
		batchSize = maxItems
	}
	if batchSize <= 1 {
		batchSize = 1
	}

	work := make(chan int, len(rows))
	var wg sync.WaitGroup

	taskStartTime := time.Now()
	if logger != nil {
		logger.Info("embedding rows start",
			"embedding_model", modelName,
			"artifact_type", rows[0].ArtifactType,
			"embedding_timeout_sec", timeoutSec,
			"total_rows", len(rows),
			"concurrent_workers", maxWorkers,
			"batch_size", batchSize)
	}

	for range maxWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				indices, ok := dequeueEmbeddingBatch(work, batchSize)
				if !ok {
					return
				}
				logger.Info("check-call-reason", "callerReason", callReason, "callLoc", callLoc)
				embedRowBatch(ctx, embedder, modelName, timeoutSec, rows, indices, logger, callReason, callLoc)
			}
		}()
	}

	for i := range rows {
		work <- i
	}
	close(work)
	wg.Wait()
	if logger != nil {
		logger.Info("embedding rows end  ",
			"total_rows", len(rows),
			"concurrent_workers", maxWorkers,
			"total_time_ms", time.Since(taskStartTime).Milliseconds())
	}
}

// embedQueryText embeds a single query string with the configured search embedder,
// returning (vec, true) only when semantic search is enabled, a model is configured, and
// the vector matches the configured dimension. Read-time callers (the metrics reviewer's
// on-the-fly hybrid search) use it because, unlike indexing, they have no pre-stored
// embedding to reuse; a false result means "proceed lexical-only".
func embedQueryText(ctx context.Context, text string) ([]float64, bool) {
	if !kbsearch.SemanticSearchEnabled() {
		return nil, false
	}
	embedder, modelName, timeoutSec, ok := newSearchEmbedder()
	if !ok {
		return nil, false
	}
	text = truncateRunes(strings.TrimSpace(text), maxEmbeddingRunes)
	if text == "" {
		return nil, false
	}
	if timeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
	}
	vec, err := embedder.Embed(ctx, llmclients.EmbedInput{ModelName: modelName, InputText: text})
	if err != nil || len(vec) != kbsearch.ConfiguredEmbeddingDim() {
		return nil, false
	}
	return vec, true
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func embeddingMaxGoroutines() int {
	raw := strings.TrimSpace(os.Getenv("EMBEDDING_MAX_GOROUTINES"))
	if raw == "" {
		return defaultEmbeddingMaxGoroutines
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultEmbeddingMaxGoroutines
	}
	return n
}

func embeddingBatchSize() int {
	raw := strings.TrimSpace(os.Getenv("EMBEDDING_BATCH_SIZE"))
	if raw == "" {
		return defaultEmbeddingBatchSize
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultEmbeddingBatchSize
	}
	return n
}

func dequeueEmbeddingBatch(work <-chan int, batchSize int) ([]int, bool) {
	first, ok := <-work
	if !ok {
		return nil, false
	}
	indices := make([]int, 0, batchSize)
	indices = append(indices, first)
	for len(indices) < batchSize {
		select {
		case idx, ok := <-work:
			if !ok {
				return indices, true
			}
			indices = append(indices, idx)
		default:
			return indices, true
		}
	}
	return indices, true
}

func embedRowBatch(
	ctx context.Context,
	embedder Embedder,
	modelName string,
	timeoutSec int,
	rows []kbsearch.RegistryRow,
	indices []int,
	logger ApiTypes.JimoLogger,
	callReason string,
	callLoc string,
) {
	prepared := make([]preparedEmbedding, 0, len(indices))
	for _, i := range indices {
		text := strings.TrimSpace(rows[i].EmbeddingText)
		if text == "" {
			text = strings.TrimSpace(rows[i].SearchDocument)
		}
		if text == "" {
			continue
		}
		text = truncateRunes(text, maxEmbeddingRunes)
		prepared = append(prepared, preparedEmbedding{
			rowIndex: i,
			text:     text,
			runes:    len([]rune(text)),
		})
	}
	if len(prepared) == 0 {
		return
	}

	maxBatchRunes := kbsearch.EmbeddingMaxBatchRunesForModel(modelName, "")
	maxBatchItems := kbsearch.EmbeddingMaxBatchItemsForModel(modelName, "")
	for start := 0; start < len(prepared); {
		end := start
		totalRunes := 0
		for end < len(prepared) {
			if maxBatchItems > 0 && end-start >= maxBatchItems {
				break
			}
			nextRunes := totalRunes + prepared[end].runes
			if maxBatchRunes > 0 && end > start && nextRunes > maxBatchRunes {
				break
			}
			totalRunes = nextRunes
			end++
		}
		logger.Info("check-call-reason", "callerReason", callReason, "callLoc", callLoc)
		embedPreparedRegistryRows(ctx, embedder, modelName, timeoutSec, rows, prepared[start:end], logger, callReason, callLoc)
		start = end
	}
}

func embedPreparedRegistryRows(
	ctx context.Context,
	embedder Embedder,
	modelName string,
	timeoutSec int,
	rows []kbsearch.RegistryRow,
	prepared []preparedEmbedding,
	logger ApiTypes.JimoLogger,
	callReason string,
	callLoc string,
) {
	texts := make([]string, 0, len(prepared))
	rowIndices := make([]int, 0, len(prepared))
	for _, item := range prepared {
		texts = append(texts, item.text)
		rowIndices = append(rowIndices, item.rowIndex)
	}

	logger.Info("check-call-reason", "callerReason", callReason, "callLoc", callLoc)
	vecs, err := embedBatchWithRetry(ctx, embedder, modelName, texts, timeoutSec, rows, rowIndices, logger, callReason, callLoc)
	if err != nil {
		for _, i := range rowIndices {
			if logger != nil {
				logger.Warn("embedding failed; row indexed lexical-only",
					"artifact_type", rows[i].ArtifactType,
					"artifact_id", rows[i].ArtifactID,
					"embedding_timeout_sec", timeoutSec,
					"error", err.Error())
			}
		}
		return
	}
	if len(vecs) != len(rowIndices) {
		if logger != nil {
			for _, i := range rowIndices {
				logger.Warn("embedding batch size mismatch; row indexed lexical-only",
					"artifact_type", rows[i].ArtifactType,
					"artifact_id", rows[i].ArtifactID,
					"got_embeddings", len(vecs),
					"want_embeddings", len(rowIndices))
			}
		}
		return
	}

	for pos, i := range rowIndices {
		vec := vecs[pos]
		if len(vec) != kbsearch.ConfiguredEmbeddingDim() {
			if logger != nil {
				logger.Warn("embedding dimension mismatch; row indexed lexical-only",
					"artifact_type", rows[i].ArtifactType,
					"artifact_id", rows[i].ArtifactID,
					"got_dim", len(vec),
					"want_dim", kbsearch.ConfiguredEmbeddingDim())
			}
			continue
		}
		rows[i].EmbeddingText = texts[pos]
		rows[i].Embedding = vec
	}
}

func embedWithRetry(
	ctx context.Context,
	embedder Embedder,
	modelName string,
	text string,
	timeoutSec int,
	row kbsearch.RegistryRow,
	logger ApiTypes.JimoLogger,
) ([]float64, error) {
	var lastErr error
	for attempt := 1; attempt <= embeddingMaxAttempts; attempt++ {
		vec, err := embedder.Embed(ctx, llmclients.EmbedInput{ModelName: modelName, InputText: text})
		if err == nil {
			return vec, nil
		}
		lastErr = err
		if attempt == embeddingMaxAttempts || !isRetryableEmbeddingError(err) {
			return nil, lastErr
		}
		if logger != nil {
			logger.Info("embedding transient failure; retrying",
				"artifact_type", row.ArtifactType,
				"artifact_id", row.ArtifactID,
				"attempt", attempt,
				"max_attempts", embeddingMaxAttempts,
				"embedding_timeout_sec", timeoutSec,
				"error", err.Error())
		}
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}
	return nil, lastErr
}

func embedBatchWithRetry(
	ctx context.Context,
	embedder Embedder,
	modelName string,
	texts []string,
	timeoutSec int,
	rows []kbsearch.RegistryRow,
	rowIndices []int,
	logger ApiTypes.JimoLogger,
	callReason string,
	callLoc string,
) ([][]float64, error) {
	logger.Info("check-call-reason", "callerReason", callReason, "callLoc", callLoc)
	if len(texts) == 1 {
		logger.Info("check-call-reason", "callerReason", callReason, "callLoc", callLoc)
		vec, err := embedWithRetry(ctx, embedder, modelName, texts[0], timeoutSec, rows[rowIndices[0]], logger)
		if err != nil {
			return nil, err
		}
		return [][]float64{vec}, nil
	}
	batcher, ok := embedder.(batchEmbedder)
	if !ok {
		out := make([][]float64, 0, len(texts))
		for pos, text := range texts {
			logger.Info("check-call-reason", "callerReason", callReason, "callLoc", callLoc)
			vec, err := embedWithRetry(ctx, embedder, modelName, text, timeoutSec, rows[rowIndices[pos]], logger)
			if err != nil {
				return nil, err
			}
			out = append(out, vec)
		}
		return out, nil
	}

	logger.Info("check-call-reason", "callerReason", callReason, "callLoc", callLoc)
	var lastErr error
	for attempt := 1; attempt <= embeddingMaxAttempts; attempt++ {
		vecs, err := batcher.EmbedBatch(ctx, llmclients.EmbedBatchInput{
			ModelName:  modelName,
			InputTexts: texts,
			CallReason: callReason,
			CallLoc:    callLoc,
		})
		if err == nil {
			return vecs, nil
		}
		lastErr = err
		if attempt == embeddingMaxAttempts || !isRetryableEmbeddingError(err) {
			return nil, lastErr
		}
		if logger != nil {
			logger.Info("embedding transient batch failure; retrying",
				"artifact_type", rows[rowIndices[0]].ArtifactType,
				"artifact_id", rows[rowIndices[0]].ArtifactID,
				"attempt", attempt,
				"max_attempts", embeddingMaxAttempts,
				"embedding_timeout_sec", timeoutSec,
				"batch_size", len(texts),
				"error", err.Error())
		}
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}
	return nil, lastErr
}

func isRetryableEmbeddingError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "Client.Timeout exceeded") {
		return true
	}
	return strings.Contains(msg, "status 429") ||
		strings.Contains(msg, "status 520") ||
		strings.Contains(msg, "status 521") ||
		strings.Contains(msg, "status 522") ||
		strings.Contains(msg, "status 523") ||
		strings.Contains(msg, "status 524") ||
		strings.Contains(msg, "status 500") ||
		strings.Contains(msg, "status 502") ||
		strings.Contains(msg, "status 503") ||
		strings.Contains(msg, "status 504")
}
