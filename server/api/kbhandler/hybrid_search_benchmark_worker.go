package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
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
	hybridBenchEffort   = func(ctx context.Context, db *sql.DB, query string, cfg registrySearchConfig, effort string) ([]artifactSearchResult, *searchEffortTrace, error) {
		return searchWithEffort(ctx, db, hybridBenchArtifactType, query, artifactSearchFilters{}, cfg.candidates(), cfg, effort)
	}
	hybridBenchSearch = func(db *sql.DB, query string, vec []float64, cfg registrySearchConfig) ([]artifactSearchResult, error) {
		if registryLexicalBackend() == lexicalBackendParadeDB {
			return queryHybridSearchResultsParadeDB(db, hybridBenchArtifactType, query, vec, artifactSearchFilters{}, 1, cfg.candidates(), cfg)
		}
		return queryHybridSearchResults(db, hybridBenchArtifactType, query, vec, artifactSearchFilters{}, 1, cfg.candidates(), cfg)
	}
)

// runHybridSearchBenchmark searches run.Parallelism questions at a time. Each question is one search
// with its own search run ID, which its embedding and keyword calls carry in the LLM usage logs. The
// run's own settings (query keywords, weights, candidates per list) replace the configured ones.
// Each stored result extends the run's deadline; a run that stops heartbeating is marked failed by
// the next run listing.
func runHybridSearchBenchmark(db *sql.DB, run hybridBenchRun, questions []hybridBenchQuestion) {
	logger := loggerutil.CreateDefaultLogger("20261024-744")
	cfg := loadRegistrySearchConfig()
	cfg.lexicalWeight, cfg.semanticWeight = run.LexicalWeight, run.SemanticWeight
	cfg.candidateLimit = run.CandidateLimit
	keywords := run.QueryKeywords
	workers := max(1, run.Parallelism)

	jobs := make(chan hybridBenchQuestion)
	var stopped atomic.Bool // set when a save fails or the run is no longer running
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for q := range jobs {
				if stopped.Load() {
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), hybridBenchQuestionTimeout)
				qcfg := cfg
				qcfg.call = newSearchCall(ctx, db, run.CreatedBy)
				qcfg.call.keywords = &keywords
				result := benchmarkHybridSearchQuestion(ctx, db, q, qcfg, logger)
				if run.Effort != "" && run.Effort != searchEffortLow {
					benchmarkHybridSearchEffort(ctx, db, q, qcfg, run.Effort, &result, logger)
				}
				alive, err := saveHybridBenchResult(ctx, db, run.ID, result)
				cancel()
				if err != nil {
					if !stopped.Swap(true) {
						failHybridBenchRun(db, run.ID, "failed to save benchmark result", logger)
					}
					logger.Error("save benchmark result failed", "run_id", run.ID, "question_id", q.ID, "error", err)
				} else if !alive {
					stopped.Store(true)
					logger.Warn("benchmark run no longer running; stopping", "run_id", run.ID, "question_id", q.ID)
				}
			}
		}()
	}
	for _, q := range questions {
		if stopped.Load() {
			break
		}
		jobs <- q
	}
	close(jobs)
	wg.Wait()
	if stopped.Load() {
		return
	}
	if _, err := db.Exec(`UPDATE testbed.hybrid_search_benchmark_runs
 SET status = 'done', finished_at = NOW() WHERE id = $1 AND status = 'running'`, run.ID); err != nil {
		logger.Error("finish benchmark run failed", "run_id", run.ID, "error", err)
		return
	}
	logger.Info("hybrid search benchmark completed", "run_id", run.ID, "questions", len(questions), "parallelism", workers)
}

func benchmarkHybridSearchQuestion(ctx context.Context, db *sql.DB, q hybridBenchQuestion, cfg registrySearchConfig, logger ApiTypes.JimoLogger) hybridBenchResult {
	questionID := q.ID
	ctx = withSearchCall(ctx, cfg.call)
	result := hybridBenchResult{
		SearchRunID: cfg.call.runID, QuestionID: &questionID, InputRecordID: q.InputRecordID, HopType: q.HopType, Lang: q.Lang,
		Question: q.Question, TargetArtifactID: q.TargetArtifactID, Keywords: []string{}, HybridTop: []string{},
	}
	start := time.Now()
	vec, ok := hybridBenchEmbed(ctx, q.Question)
	result.EmbeddingMs = msSince(start)
	if !ok {
		result.ErrorMsg = hybridBenchErrEmbedding
		logger.Warn("benchmark question embedding failed", "question_id", q.ID)
		return result
	}
	// Keywords are extracted fresh (not from the cache) so the call is timed and logged; the searches
	// extract them again, and the cache this call fills makes that a lookup. Failures are not cached,
	// so after a failure the searches run without keywords (the raw query, as live search falls back
	// to) instead of calling the model three more times.
	errs := []string{}
	if cfg.call.keywords == nil || *cfg.call.keywords {
		fresh := cfg.call
		fresh.freshKeywords = true
		start = time.Now()
		kws := hybridBenchKeywords(withSearchCall(ctx, fresh), q.Question)
		result.KeywordsMs = msSince(start)
		if kws != nil {
			result.Keywords = kws
		} else {
			errs = append(errs, hybridBenchErrKeywords)
			off := false
			cfg.call.keywords = &off
		}
	}
	for _, mode := range hybridBenchModes {
		modeCfg := cfg
		modeCfg.lexicalWeight, modeCfg.semanticWeight = hybridBenchModeWeights(mode, cfg)
		start = time.Now()
		results, err := hybridBenchSearch(db, q.Question, vec, modeCfg)
		elapsed := msSince(start)
		switch mode {
		case hybridBenchModeLexical:
			result.LexicalMs = elapsed
		case hybridBenchModeSemantic:
			result.SemanticMs = elapsed
		default:
			result.HybridMs = elapsed
		}
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
  lexical_rank, semantic_rank, hybrid_rank, hybrid_top, error_msg, search_run_id,
  embedding_ms, keywords_ms, lexical_ms, semantic_ms, hybrid_ms,
  effort_rank, effort_found, effort_attempts, effort_ms, effort_trace)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,0),$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
		runID, r.QuestionID, r.InputRecordID, r.HopType, r.Lang, r.Question, r.TargetArtifactID, pq.Array(r.Keywords),
		r.LexicalRank, r.SemanticRank, r.HybridRank, top, r.ErrorMsg, r.SearchRunID,
		r.EmbeddingMs, r.KeywordsMs, r.LexicalMs, r.SemanticMs, r.HybridMs,
		r.EffortRank, r.EffortFound, r.EffortAttempts, r.EffortMs, nullJSON(r.EffortTrace)); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// benchmarkHybridSearchEffort runs the effort loop for one question. The loop never sees the
// target; the target's rank in its final list is scored like the other modes.
func benchmarkHybridSearchEffort(ctx context.Context, db *sql.DB, q hybridBenchQuestion, cfg registrySearchConfig, effort string, result *hybridBenchResult, logger ApiTypes.JimoLogger) {
	start := time.Now()
	results, trace, err := hybridBenchEffort(ctx, db, q.Question, cfg, effort)
	result.EffortMs = msSince(start)
	if err != nil {
		logger.Warn("benchmark effort search failed", "question_id", q.ID, "effort", effort, "error", err)
		result.ErrorMsg = strings.TrimPrefix(result.ErrorMsg+"; effort: "+err.Error(), "; ")
		return
	}
	result.EffortRank = hybridBenchTargetRank(results, q.TargetArtifactID)
	attempts, found := 0, false
	if trace != nil {
		attempts, found = len(trace.Attempts), trace.Found
		if trace.Error != "" {
			result.ErrorMsg = strings.TrimPrefix(result.ErrorMsg+"; effort: "+trace.Error, "; ")
		}
		if raw, err := json.Marshal(trace); err == nil {
			result.EffortTrace = raw
		}
	}
	result.EffortAttempts, result.EffortFound = &attempts, &found
}

// nullJSON stores an empty raw message as NULL.
func nullJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

func msSince(start time.Time) *int {
	ms := int(time.Since(start).Milliseconds())
	return &ms
}

func failHybridBenchRun(db *sql.DB, runID int64, msg string, logger ApiTypes.JimoLogger) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `UPDATE testbed.hybrid_search_benchmark_runs
 SET status = 'failed', error_msg = $2, finished_at = NOW() WHERE id = $1 AND status = 'running'`, runID, msg); err != nil {
		logger.Error("save benchmark failure failed", "run_id", runID, "error", err)
	}
}
