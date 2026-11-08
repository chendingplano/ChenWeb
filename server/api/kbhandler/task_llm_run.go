package kbhandler

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chendingplano/deepdoc/server/api/llmrun"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// taskLLMExecution resolves an execution independently of the task table's ID.
// table is an internal constant, never a request parameter.
func taskLLMExecution(ctx context.Context, db *sql.DB, table string, id int64) (int64, error) {
	executionID, err := llmrun.Resolve(ctx, db, fmt.Sprintf("%s/%d", table, id))
	if err != nil {
		return 0, err
	}
	if _, err := db.ExecContext(ctx, `UPDATE `+table+` SET llm_run_id = $2 WHERE id = $1`, id, executionID); err != nil {
		loggerutil.CreateDefaultLogger("20261108-697").Warn("persist task LLM execution ID failed", "table", table, "id", id, "error", err)
	}
	return executionID, nil
}
