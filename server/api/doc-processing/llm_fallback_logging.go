package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"net"

	"github.com/chendingplano/deepdoc/server/api/llmusage"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// A fallback owns its diagnostics until the caller accepts its result. Each
// invocation has its own scope, even when chunks share the same model client.
type llmFallbackLog struct {
	ctx       context.Context
	finish    func(bool)
	recovered bool
}

func startLLMFallback(ctx context.Context) (context.Context, *llmFallbackLog) {
	ctx, finish := llmclients.DeferCallAlerts(ctx)
	return ctx, &llmFallbackLog{ctx: ctx, finish: finish}
}

func (l *llmFallbackLog) flush() {
	l.finish(l.recovered)
}

func (l *llmFallbackLog) warn(logger ApiTypes.JimoLogger, message string, attrs ...any) {
	llmclients.ReportCallAlert(l.ctx, func() { logger.Warn(message, attrs...) })
}

func (l *llmFallbackLog) record(logger ApiTypes.JimoLogger, operation, prompt, primary, fallback string, primaryErr error) {
	l.recordChunk(ApiTypes.ProjectDBHandle, logger, operation, prompt, primary, fallback, primaryErr, nil)
}

func (l *llmFallbackLog) recordChunk(db *sql.DB, logger ApiTypes.JimoLogger, operation, prompt, primary, fallback string, primaryErr error, chunk *int) {
	l.recovered = true
	llmusage.RecordRecoveredCall(l.ctx, db, logger, llmusage.RecoveredCall{
		RecordID: llmRecordIDFromContext(l.ctx), RunID: llmRunIDFromContext(l.ctx),
		Operation: operation, Prompt: prompt, PrimaryModel: primary, FallbackModel: fallback,
		PrimaryError: primaryErr, ChunkIndex: chunk,
	})
}

func retryLLMTimeout[T any](ctx context.Context, call func() (T, error)) (T, error) {
	result, err := call()
	if ctx.Err() != nil {
		return result, err
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return call()
	}
	return result, err
}
