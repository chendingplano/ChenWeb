package llmusage

import (
	"context"
	"database/sql"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// Outcomes of a call that had a failed try.
const (
	FailedCallRecovered = "recovered" // a later try returned an accepted answer
	FailedCallFailed    = "failed"    // no try returned an accepted answer
)

// FailedCall describes one failed try of a multi-try call: which try it was, the model that made
// it, and how the call ended. PrimaryModel is the model of the first try; FallbackModel is the
// model whose answer was accepted, empty when none was.
type FailedCall struct {
	RecordID, RunID, ReviewRunID int64
	Aspect, Operation, Prompt    string
	Attempt                      int
	Model                        string
	PrimaryModel, FallbackModel  string
	Error                        string
	RawResponse                  string
	Outcome                      string
	ChunkIndex                   *int
}

// RecordFailedCall stores one failed try in kb.failed_llm_calls, independently of the caller's
// outcome. A database error is logged and never returned: the caller's result stands.
func RecordFailedCall(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, call FailedCall) {
	if db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, `
		INSERT INTO kb.failed_llm_calls
			(record_id, run_id, review_run_id, aspect, operation, prompt_name, attempt, model,
			 primary_model, fallback_model, primary_error, raw_response, outcome, chunk_index)
		VALUES (NULLIF($1::bigint, 0), NULLIF($2::bigint, 0), NULLIF($3::bigint, 0), NULLIF($4, ''), $5, $6, $7, $8,
			$9, NULLIF($10, ''), $11, $12, $13, $14)`,
		call.RecordID, call.RunID, call.ReviewRunID, call.Aspect, call.Operation, call.Prompt, call.Attempt, call.Model,
		call.PrimaryModel, call.FallbackModel, call.Error, call.RawResponse, call.Outcome, call.ChunkIndex)
	if err != nil {
		if logger == nil {
			logger = loggerutil.CreateDefaultLogger("MID-20261122-31")
		}
		logger.Error("(MID-20261122-31) failed to record a failed LLM try",
			"record_id", call.RecordID, "review_run_id", call.ReviewRunID, "operation", call.Operation,
			"attempt", call.Attempt, "error", err)
	}
}
