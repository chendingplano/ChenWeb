package llmusage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// RecoveredCall describes a failed primary whose model fallback was accepted.
type RecoveredCall struct {
	RecordID, RunID             int64
	Operation, Prompt           string
	PrimaryModel, FallbackModel string
	PrimaryError                error
	ChunkIndex                  *int
}

// RecordRecoveredCall stores diagnostics independently of the caller's outcome.
// A database error raises an operational alert without undoing the recovery.
func RecordRecoveredCall(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, call RecoveredCall) {
	if db == nil {
		return
	}
	raw := ""
	var structured *llmclients.StructuredOutputError
	if errors.As(call.PrimaryError, &structured) {
		raw = structured.Raw
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, `
		INSERT INTO kb.failed_llm_calls
			(record_id, run_id, operation, prompt_name, primary_model, fallback_model, primary_error, raw_response, chunk_index)
		VALUES (NULLIF($1::bigint, 0), NULLIF($2::bigint, 0), $3, $4, $5, $6, $7, $8, $9)`,
		call.RecordID, call.RunID, call.Operation, call.Prompt, call.PrimaryModel, call.FallbackModel, call.PrimaryError.Error(), raw, call.ChunkIndex)
	if err != nil {
		if logger == nil {
			logger = loggerutil.CreateDefaultLogger("MID-20261119-71")
		}
		logger.Error("(MID-20261119-71) failed to record recovered LLM call",
			"record_id", call.RecordID, "run_id", call.RunID, "operation", call.Operation, "error", err)
	}
}
