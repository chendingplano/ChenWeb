package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// A single statement moves complete rows atomically. If the archive insert
// fails, PostgreSQL rolls back the removal from the active table too.
func archiveGeneratedQuestions(ctx context.Context, db *sql.DB, ids []int64, userID string) (int64, error) {
	result, err := db.ExecContext(ctx, `WITH moved AS (
 DELETE FROM testbed.generated_questions WHERE id = ANY($1)
 RETURNING `+generatedQuestionColumns+`
)
INSERT INTO testbed.generated_questions_archive (`+generatedQuestionColumns+`, archived_by)
SELECT `+generatedQuestionColumns+`, $2 FROM moved`, pq.Array(ids), userID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func ArchiveGeneratedQuestions(c echo.Context) error {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 64*1024))
	if err := decoder.Decode(&req); err != nil {
		return questionGenerationError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return questionGenerationError(c, http.StatusBadRequest, "invalid request body")
	}
	if len(req.IDs) == 0 || len(req.IDs) > 1000 {
		return questionGenerationError(c, http.StatusBadRequest, "provide between 1 and 1000 question IDs")
	}
	for _, id := range req.IDs {
		if id < 1 {
			return questionGenerationError(c, http.StatusBadRequest, "invalid question ID")
		}
	}
	logger := loggerutil.CreateDefaultLogger("20261025-748")
	userID, _ := c.Get("question-generator-user-id").(string)
	count, err := archiveGeneratedQuestions(c.Request().Context(), ApiTypes.ProjectDBHandle, req.IDs, userID)
	if err != nil {
		logger.Error("archive generated questions failed", "question_ids", req.IDs, "user_id", userID, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to archive generated questions")
	}
	logger.Info("generated questions archived", "question_ids", req.IDs, "archived_count", count, "user_id", userID)
	return c.JSON(http.StatusOK, map[string]any{"archived_count": count})
}
