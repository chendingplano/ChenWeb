package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// Replaced in tests.
var launchHybridSearchBenchmark = func(db *sql.DB, run hybridBenchRun, questions []hybridBenchQuestion) {
	go runHybridSearchBenchmark(db, run, questions)
}

const hybridBenchRunColumns = `id, purpose, description, status, error_msg, question_count, done_count,
 lexical_backend, query_keywords, embedding_model, lexical_weight, semantic_weight, candidate_limit,
 created_by, created_at, finished_at`

func scanHybridBenchRun(row interface{ Scan(...any) error }, extra ...any) (hybridBenchRun, error) {
	var run hybridBenchRun
	dest := []any{&run.ID, &run.Purpose, &run.Description, &run.Status, &run.ErrorMsg, &run.QuestionCount, &run.DoneCount,
		&run.LexicalBackend, &run.QueryKeywords, &run.EmbeddingModel, &run.LexicalWeight, &run.SemanticWeight, &run.CandidateLimit,
		&run.CreatedBy, &run.CreatedAt, &run.FinishedAt}
	err := row.Scan(append(dest, extra...)...)
	return run, err
}

// StartHybridSearchBenchmark creates a run over every question of the requested purpose and
// processes it in the background. Uses QuestionGeneratorAdmin for access control.
func StartHybridSearchBenchmark(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("20261024-741")
	var req hybridBenchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 16*1024))
	if err := decoder.Decode(&req); err != nil {
		return questionGenerationError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := req.normalize(); err != nil {
		return questionGenerationError(c, http.StatusBadRequest, err.Error())
	}
	db, ctx := ApiTypes.ProjectDBHandle, c.Request().Context()
	questions, err := loadHybridBenchQuestions(ctx, db, req.Purpose)
	if err != nil {
		logger.Error("load benchmark questions failed", "purpose", req.Purpose, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load questions")
	}
	if len(questions) == 0 {
		return questionGenerationError(c, http.StatusBadRequest, "no questions for this purpose")
	}
	cfg := loadRegistrySearchConfig()
	userID, _ := c.Get("question-generator-user-id").(string)
	run, err := scanHybridBenchRun(db.QueryRowContext(ctx, `INSERT INTO testbed.hybrid_search_benchmark_runs
  (purpose, description, question_count, lexical_backend, query_keywords, embedding_model,
   lexical_weight, semantic_weight, candidate_limit, created_by, expires_at)
  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW()+$11*INTERVAL '1 second') RETURNING `+hybridBenchRunColumns,
		req.Purpose, req.Description, len(questions), string(registryLexicalBackend()), searchKeywordsEnabled(logger),
		strings.TrimSpace(os.Getenv("EMBEDDING_MODEL_NAME")), cfg.lexicalWeight, cfg.semanticWeight, hybridCandidateLimit,
		userID, int(hybridBenchHeartbeat.Seconds())))
	if err != nil {
		logger.Error("benchmark run insert failed", "purpose", req.Purpose, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to create benchmark run")
	}
	logger.Info("hybrid search benchmark started", "run_id", run.ID, "purpose", run.Purpose,
		"questions", run.QuestionCount, "lexical_backend", run.LexicalBackend, "user_id", userID)
	launchHybridSearchBenchmark(db, run, questions)
	return c.JSON(http.StatusAccepted, map[string]any{"run": run})
}

func ListHybridSearchBenchmarkRuns(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("20261024-742")
	db, ctx := ApiTypes.ProjectDBHandle, c.Request().Context()
	if _, err := db.ExecContext(ctx, `UPDATE testbed.hybrid_search_benchmark_runs
 SET status = 'failed', error_msg = 'benchmark interrupted or timed out', finished_at = NOW()
 WHERE status = 'running' AND expires_at <= NOW()`); err != nil {
		logger.Error("expire benchmark runs failed", "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load benchmark runs")
	}
	rows, err := db.QueryContext(ctx, `SELECT `+hybridBenchRunColumns+`,
 (SELECT AVG(COALESCE(res.hybrid_rank <= 10, FALSE)::int)::float8
  FROM testbed.hybrid_search_benchmark_results res WHERE res.run_id = r.id)
 FROM testbed.hybrid_search_benchmark_runs r ORDER BY id DESC LIMIT $1`, hybridBenchRunListLimit)
	if err != nil {
		logger.Error("list benchmark runs failed", "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load benchmark runs")
	}
	defer rows.Close()
	runs := []hybridBenchRun{}
	for rows.Next() {
		var r10 sql.NullFloat64
		run, err := scanHybridBenchRun(rows, &r10)
		if err != nil {
			logger.Error("scan benchmark run failed", "error", err)
			return questionGenerationError(c, http.StatusInternalServerError, "failed to load benchmark runs")
		}
		if r10.Valid {
			run.HybridR10 = &r10.Float64
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		logger.Error("list benchmark runs failed", "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load benchmark runs")
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": runs, "purposes": hybridBenchPurposes})
}

func GetHybridSearchBenchmarkRun(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("20261024-743")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return questionGenerationError(c, http.StatusBadRequest, "invalid run id")
	}
	db, ctx := ApiTypes.ProjectDBHandle, c.Request().Context()
	run, err := scanHybridBenchRun(db.QueryRowContext(ctx, `SELECT `+hybridBenchRunColumns+`
 FROM testbed.hybrid_search_benchmark_runs WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return questionGenerationError(c, http.StatusNotFound, "benchmark run not found")
	}
	if err != nil {
		logger.Error("load benchmark run failed", "run_id", id, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load benchmark run")
	}
	results, err := loadHybridBenchResults(ctx, db, id)
	if err != nil {
		logger.Error("load benchmark results failed", "run_id", id, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load benchmark run")
	}
	return c.JSON(http.StatusOK, map[string]any{"run": run, "scores": scoreHybridBench(results), "results": results})
}

func loadHybridBenchQuestions(ctx context.Context, db *sql.DB, purpose string) ([]hybridBenchQuestion, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, input_record_id, hop_type, lang, question, metric_id
 FROM testbed.generated_questions WHERE purpose = $1 AND metric_id <> '' ORDER BY id`, purpose)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var questions []hybridBenchQuestion
	for rows.Next() {
		var q hybridBenchQuestion
		if err := rows.Scan(&q.ID, &q.InputRecordID, &q.HopType, &q.Lang, &q.Question, &q.TargetArtifactID); err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	return questions, rows.Err()
}

func loadHybridBenchResults(ctx context.Context, db *sql.DB, runID int64) ([]hybridBenchResult, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, question_id, input_record_id, hop_type, lang, question,
 target_artifact_id, keywords, lexical_rank, semantic_rank, hybrid_rank, hybrid_top, error_msg
 FROM testbed.hybrid_search_benchmark_results WHERE run_id = $1 ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []hybridBenchResult{}
	for rows.Next() {
		var r hybridBenchResult
		var questionID, lexical, semantic, hybrid sql.NullInt64
		var top []byte
		if err := rows.Scan(&r.ID, &questionID, &r.InputRecordID, &r.HopType, &r.Lang, &r.Question,
			&r.TargetArtifactID, pq.Array(&r.Keywords), &lexical, &semantic, &hybrid, &top, &r.ErrorMsg); err != nil {
			return nil, err
		}
		if questionID.Valid {
			r.QuestionID = &questionID.Int64
		}
		r.LexicalRank, r.SemanticRank, r.HybridRank = nullRank(lexical), nullRank(semantic), nullRank(hybrid)
		if err := json.Unmarshal(top, &r.HybridTop); err != nil {
			return nil, err
		}
		if r.Keywords == nil {
			r.Keywords = []string{}
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func nullRank(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	rank := int(v.Int64)
	return &rank
}
