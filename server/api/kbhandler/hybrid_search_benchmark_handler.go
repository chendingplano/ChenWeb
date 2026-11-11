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
 lexical_backend, search_type, query_keywords, embedding_model, keywords_model, keywords_prompt,
 lexical_weight, semantic_weight, candidate_limit, parallelism, effort, max_efforts, created_by, created_at, finished_at`

func scanHybridBenchRun(row interface{ Scan(...any) error }, extra ...any) (hybridBenchRun, error) {
	var run hybridBenchRun
	dest := []any{&run.ID, &run.Purpose, &run.Description, &run.Status, &run.ErrorMsg, &run.QuestionCount, &run.DoneCount,
		&run.LexicalBackend, &run.SearchType, &run.QueryKeywords, &run.EmbeddingModel, &run.KeywordsModel, &run.KeywordsPrompt,
		&run.LexicalWeight, &run.SemanticWeight, &run.CandidateLimit, &run.Parallelism, &run.Effort, &run.MaxEfforts,
		&run.CreatedBy, &run.CreatedAt, &run.FinishedAt}
	err := row.Scan(append(dest, extra...)...)
	return run, err
}

// StartHybridSearchBenchmark creates a run over every question of the requested purpose and
// processes it in the background. Uses QuestionGeneratorAdmin for access control.
func StartHybridSearchBenchmark(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("20261024-741")
	var req hybridBenchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 128*1024))
	if err := decoder.Decode(&req); err != nil {
		return questionGenerationError(c, http.StatusBadRequest, "invalid request body")
	}
	opts, err := req.normalize(hybridBenchDefaults(loadRegistrySearchConfig(), searchKeywordsEnabled(logger)))
	if err != nil {
		return questionGenerationError(c, http.StatusBadRequest, err.Error())
	}
	db, ctx := ApiTypes.ProjectDBHandle, c.Request().Context()
	searchType := req.SearchType
	if searchType == "" {
		searchType = hybridBenchPurposeSearchType(req.Purpose)
	}
	questions, err := loadHybridBenchQuestions(ctx, db, req.Purpose, searchType, req.QuestionIDs)
	if errors.Is(err, errHybridBenchSelection) || errors.Is(err, errHybridBenchSearchType) {
		return questionGenerationError(c, http.StatusBadRequest, err.Error())
	}
	if err != nil {
		logger.Error("load benchmark questions failed", "purpose", req.Purpose, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load questions")
	}
	if len(questions) == 0 {
		return questionGenerationError(c, http.StatusBadRequest, "no questions for this purpose")
	}
	keywordsModel, keywordsPrompt := "", ""
	if opts.QueryKeywords {
		keywordsModel = strings.TrimSpace(os.Getenv(envSearchKeywordsModel))
		keywordsPrompt = envOrDefault(envSearchKeywordsPrompt, defaultSearchKeywordsPrompt)
	}
	userID, _ := c.Get("question-generator-user-id").(string)
	run, err := scanHybridBenchRun(db.QueryRowContext(ctx, `INSERT INTO testbed.hybrid_search_benchmark_runs
  (purpose, description, question_count, lexical_backend, search_type, query_keywords, embedding_model, keywords_model, keywords_prompt,
   lexical_weight, semantic_weight, candidate_limit, parallelism, effort, max_efforts, created_by, expires_at)
  VALUES ($1,$2,$3,$4,$17,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,NOW()+$16*INTERVAL '1 second') RETURNING `+hybridBenchRunColumns,
		req.Purpose, req.Description, len(questions), string(registryLexicalBackend()), opts.QueryKeywords,
		strings.TrimSpace(os.Getenv("EMBEDDING_MODEL_NAME")), keywordsModel, keywordsPrompt,
		opts.LexicalWeight, opts.SemanticWeight, opts.CandidateLimit, opts.Parallelism, opts.Effort, searchEffortMaxAttempts(opts.Effort),
		userID, int(hybridBenchHeartbeat.Seconds()), searchType))
	if err != nil {
		logger.Error("benchmark run insert failed", "purpose", req.Purpose, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to create benchmark run")
	}
	logger.Info("hybrid search benchmark started", "run_id", run.ID, "purpose", run.Purpose,
		"questions", run.QuestionCount, "lexical_backend", run.LexicalBackend, "search_type", run.SearchType, "query_keywords", run.QueryKeywords,
		"lexical_weight", run.LexicalWeight, "semantic_weight", run.SemanticWeight, "candidate_limit", run.CandidateLimit, "parallelism", run.Parallelism, "effort", run.Effort, "max_efforts", run.MaxEfforts, "selected_questions", len(req.QuestionIDs), "user_id", userID)
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
	defaults := hybridBenchDefaults(loadRegistrySearchConfig(), searchKeywordsEnabled(logger))
	return c.JSON(http.StatusOK, map[string]any{"runs": runs, "purposes": hybridBenchPurposes, "search_types": searchArtifactPartitionTypes, "defaults": defaults})
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

const hybridBenchResultColumns = `id, COALESCE(search_run_id, 0), question_id, input_record_id, hop_type, lang, question,
 target_artifact_id, target_artifact_type, keywords, lexical_rank, semantic_rank, hybrid_rank, hybrid_top, error_msg,
 embedding_ms, keywords_ms, lexical_ms, semantic_ms, hybrid_ms,
 effort_rank, effort_found, effort_attempts, effort_ms, effort_trace`

func loadHybridBenchResults(ctx context.Context, db *sql.DB, runID int64) ([]hybridBenchResult, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+hybridBenchResultColumns+`
 FROM testbed.hybrid_search_benchmark_results WHERE run_id = $1 ORDER BY question_id NULLS LAST, id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []hybridBenchResult{}
	for rows.Next() {
		r, err := scanHybridBenchResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// scanHybridBenchResult scans hybridBenchResultColumns, then any extra columns into extra.
func scanHybridBenchResult(row interface{ Scan(...any) error }, extra ...any) (hybridBenchResult, error) {
	var r hybridBenchResult
	var questionID, lexical, semantic, hybrid, embeddingMs, keywordsMs, lexicalMs, semanticMs, hybridMs sql.NullInt64
	var effortRank, effortAttempts, effortMs sql.NullInt64
	var effortFound sql.NullBool
	var effortTrace []byte
	var top []byte
	dest := []any{&r.ID, &r.SearchRunID, &questionID, &r.InputRecordID, &r.HopType, &r.Lang, &r.Question,
		&r.TargetArtifactID, &r.TargetArtifactType, pq.Array(&r.Keywords), &lexical, &semantic, &hybrid, &top, &r.ErrorMsg,
		&embeddingMs, &keywordsMs, &lexicalMs, &semanticMs, &hybridMs,
		&effortRank, &effortFound, &effortAttempts, &effortMs, &effortTrace}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return r, err
	}
	if questionID.Valid {
		r.QuestionID = &questionID.Int64
	}
	r.LexicalRank, r.SemanticRank, r.HybridRank = nullRank(lexical), nullRank(semantic), nullRank(hybrid)
	r.EmbeddingMs, r.KeywordsMs = nullRank(embeddingMs), nullRank(keywordsMs)
	r.LexicalMs, r.SemanticMs, r.HybridMs = nullRank(lexicalMs), nullRank(semanticMs), nullRank(hybridMs)
	r.EffortRank, r.EffortAttempts, r.EffortMs = nullRank(effortRank), nullRank(effortAttempts), nullRank(effortMs)
	if effortFound.Valid {
		r.EffortFound = &effortFound.Bool
	}
	r.EffortTrace = json.RawMessage(effortTrace)
	if err := json.Unmarshal(top, &r.HybridTop); err != nil {
		return r, err
	}
	if r.Keywords == nil {
		r.Keywords = []string{}
	}
	return r, nil
}

// nullRank converts a nullable integer column (a rank or a duration) to *int.
func nullRank(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	rank := int(v.Int64)
	return &rank
}
