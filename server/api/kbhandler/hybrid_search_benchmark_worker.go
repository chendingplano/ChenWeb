package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/lib/pq"
)

// Replaced in tests; production uses the live query embedding, keyword extraction and the
// backend's hybrid query, so the benchmark measures exactly what the search API runs.
var (
	hybridBenchEmbed    = computeQueryEmbedding
	hybridBenchKeywords = searchQueryKeywords
	hybridBenchSearch   = func(db *sql.DB, query string, vec []float64, cfg registrySearchConfig) ([]artifactSearchResult, error) {
		if registryLexicalBackend() == lexicalBackendParadeDB {
			return queryHybridSearchResultsParadeDB(db, hybridBenchArtifactType, query, vec, artifactSearchFilters{}, 1, hybridCandidateLimit, cfg)
		}
		return queryHybridSearchResults(db, hybridBenchArtifactType, query, vec, artifactSearchFilters{}, 1, hybridCandidateLimit, cfg)
	}
)

// runHybridSearchBenchmark processes the questions one at a time (the shared embedding host times
// out under concurrent load). Each stored result extends the run's deadline; a run that stops
// heartbeating is marked failed by the next run listing.
func runHybridSearchBenchmark(db *sql.DB, run hybridBenchRun, questions []hybridBenchQuestion) {
	logger := loggerutil.CreateDefaultLogger("20261024-744")
	cfg := loadRegistrySearchConfig()
	cfg.lexicalWeight, cfg.semanticWeight = run.LexicalWeight, run.SemanticWeight
	for _, q := range questions {
		ctx, cancel := context.WithTimeout(context.Background(), hybridBenchQuestionTimeout)
		result := benchmarkHybridSearchQuestion(ctx, db, q, cfg, logger)
		alive, err := saveHybridBenchResult(ctx, db, run.ID, result)
		cancel()
		if err != nil {
			failHybridBenchRun(db, run.ID, "failed to save benchmark result", logger)
			logger.Error("save benchmark result failed", "run_id", run.ID, "question_id", q.ID, "error", err)
			return
		}
		if !alive {
			logger.Warn("benchmark run no longer running; stopping", "run_id", run.ID, "question_id", q.ID)
			return
		}
	}
	if _, err := db.Exec(`UPDATE testbed.hybrid_search_benchmark_runs
 SET status = 'done', finished_at = NOW() WHERE id = $1 AND status = 'running'`, run.ID); err != nil {
		logger.Error("finish benchmark run failed", "run_id", run.ID, "error", err)
		return
	}
	logger.Info("hybrid search benchmark completed", "run_id", run.ID, "questions", len(questions))
}

func benchmarkHybridSearchQuestion(ctx context.Context, db *sql.DB, q hybridBenchQuestion, cfg registrySearchConfig, logger ApiTypes.JimoLogger) hybridBenchResult {
	questionID := q.ID
	result := hybridBenchResult{
		QuestionID: &questionID, InputRecordID: q.InputRecordID, HopType: q.HopType, Lang: q.Lang,
		Question: q.Question, TargetArtifactID: q.TargetArtifactID, Keywords: []string{}, HybridTop: []string{},
	}
	vec, ok := hybridBenchEmbed(ctx, q.Question)
	if !ok {
		result.ErrorMsg = hybridBenchErrEmbedding
		logger.Warn("benchmark question embedding failed", "question_id", q.ID)
		return result
	}
	// The searches extract keywords again; the per-query cache makes that a lookup.
	if kws := hybridBenchKeywords(ctx, q.Question); kws != nil {
		result.Keywords = kws
	}
	errs := []string{}
	for _, mode := range hybridBenchModes {
		modeCfg := cfg
		modeCfg.lexicalWeight, modeCfg.semanticWeight = hybridBenchModeWeights(mode, cfg)
		results, err := hybridBenchSearch(db, q.Question, vec, modeCfg)
		if err != nil {
			errs = append(errs, mode+": "+err.Error())
			logger.Warn("benchmark search failed", "question_id", q.ID, "mode", mode, "error", err)
			continue
		}
		rank := hybridBenchTargetRank(results, q.TargetArtifactID)
		switch mode {
		case hybridBenchModeLexical:
			result.LexicalRank = rank
		case hybridBenchModeSemantic:
			result.SemanticRank = rank
		default:
			result.HybridRank = rank
			result.HybridTop = hybridBenchTop(results)
		}
	}
	result.ErrorMsg = strings.Join(errs, "; ")
	return result
}

// saveHybridBenchResult stores one question's result and advances the run's progress and deadline.
// alive=false means the run is no longer running (expired), so the worker stops.
func saveHybridBenchResult(ctx context.Context, db *sql.DB, runID int64, r hybridBenchResult) (alive bool, err error) {
	top, err := json.Marshal(r.HybridTop)
	if err != nil {
		return false, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE testbed.hybrid_search_benchmark_runs
 SET done_count = done_count + 1, expires_at = NOW() + $2 * INTERVAL '1 second'
 WHERE id = $1 AND status = 'running'`, runID, int(hybridBenchHeartbeat.Seconds()))
	if err != nil {
		return false, err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO testbed.hybrid_search_benchmark_results
 (run_id, question_id, input_record_id, hop_type, lang, question, target_artifact_id, keywords,
  lexical_rank, semantic_rank, hybrid_rank, hybrid_top, error_msg)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		runID, r.QuestionID, r.InputRecordID, r.HopType, r.Lang, r.Question, r.TargetArtifactID, pq.Array(r.Keywords),
		r.LexicalRank, r.SemanticRank, r.HybridRank, top, r.ErrorMsg); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func failHybridBenchRun(db *sql.DB, runID int64, msg string, logger ApiTypes.JimoLogger) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `UPDATE testbed.hybrid_search_benchmark_runs
 SET status = 'failed', error_msg = $2, finished_at = NOW() WHERE id = $1 AND status = 'running'`, runID, msg); err != nil {
		logger.Error("save benchmark failure failed", "run_id", runID, "error", err)
	}
}
