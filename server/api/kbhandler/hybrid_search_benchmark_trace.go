package kbhandler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

const (
	hybridBenchTraceDefaultLimit = 20
	hybridBenchTraceMaxLimit     = 1000
)

// hybridBenchTraceCandidate is one candidate row as the Analyze dialog shows it.
type hybridBenchTraceCandidate struct {
	Rank          int     `json:"rank"`
	PartitionName string  `json:"partition_name"`
	ArtifactType  string  `json:"artifact_type"`
	ArtifactID    string  `json:"artifact_id"`
	Label         string  `json:"label"` // empty when the artifact no longer exists
	Score         float64 `json:"score"`
	PartitionRank *int    `json:"partition_rank"`
	LexicalRank   *int    `json:"lexical_rank"`
	SemanticRank  *int    `json:"semantic_rank"`
}

type hybridBenchTraceQuery struct {
	ID       int64                                  `json:"id"`
	Seq      int                                    `json:"seq"`
	Source   string                                 `json:"source"`
	Attempt  *int                                   `json:"attempt"`
	Text     string                                 `json:"query_text"`
	Keywords []string                               `json:"keywords"`
	ErrorMsg string                                 `json:"error_msg"`
	Stages   map[string][]hybridBenchTraceCandidate `json:"stages"` // first limit rows + target rows
	Counts   map[string]int                         `json:"counts"` // all rows stored per stage
}

// GetHybridSearchBenchmarkTrace returns one result (as in its run's results), its run's lexical
// backend, and its queries and candidate lists: per stage (per partition for lexical_partition) the
// first limit rows plus the target's rows. Uses QuestionGeneratorAdmin for access control.
func GetHybridSearchBenchmarkTrace(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("20261025-752")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return questionGenerationError(c, http.StatusBadRequest, "invalid result id")
	}
	limit := hybridBenchTraceDefaultLimit
	if raw := c.QueryParam("limit"); raw != "" {
		if limit, err = strconv.Atoi(raw); err != nil || limit < 1 || limit > hybridBenchTraceMaxLimit {
			return questionGenerationError(c, http.StatusBadRequest, "limit must be between 1 and 1000")
		}
	}
	db, ctx := ApiTypes.ProjectDBHandle, c.Request().Context()
	var lexicalBackend string
	result, err := scanHybridBenchResult(db.QueryRowContext(ctx, `SELECT `+hybridBenchResultColumns+`,
 (SELECT u.lexical_backend FROM testbed.hybrid_search_benchmark_runs u WHERE u.id = r.run_id)
 FROM testbed.hybrid_search_benchmark_results r WHERE r.id = $1`, id), &lexicalBackend)
	if errors.Is(err, sql.ErrNoRows) {
		return questionGenerationError(c, http.StatusNotFound, "benchmark result not found")
	}
	if err != nil {
		logger.Error("load benchmark result failed", "result_id", id, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load benchmark trace")
	}
	queries, err := loadHybridBenchTrace(ctx, db, id, result.TargetArtifactID, limit)
	if err != nil {
		logger.Error("load benchmark trace failed", "result_id", id, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load benchmark trace")
	}
	return c.JSON(http.StatusOK, map[string]any{"result": result, "lexical_backend": lexicalBackend, "limit": limit, "queries": queries})
}

func loadHybridBenchTrace(ctx context.Context, db *sql.DB, resultID int64, target string, limit int) ([]hybridBenchTraceQuery, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, seq, source, attempt, query_text, keywords, error_msg
 FROM testbed.hybrid_search_benchmark_queries WHERE result_id = $1 ORDER BY seq`, resultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	queries := []hybridBenchTraceQuery{}
	byID := map[int64]*hybridBenchTraceQuery{}
	for rows.Next() {
		q := hybridBenchTraceQuery{Stages: map[string][]hybridBenchTraceCandidate{}, Counts: map[string]int{}}
		var attempt sql.NullInt64
		if err := rows.Scan(&q.ID, &q.Seq, &q.Source, &attempt, &q.Text, pq.Array(&q.Keywords), &q.ErrorMsg); err != nil {
			return nil, err
		}
		q.Attempt = nullRank(attempt)
		if q.Keywords == nil {
			q.Keywords = []string{}
		}
		queries = append(queries, q)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range queries {
		byID[queries[i].ID] = &queries[i]
	}
	if len(queries) == 0 {
		return queries, nil
	}

	counts, err := db.QueryContext(ctx, `SELECT c.query_id, c.stage, COUNT(*)
 FROM testbed.hybrid_search_benchmark_candidates c
 JOIN testbed.hybrid_search_benchmark_queries q ON q.id = c.query_id
 WHERE q.result_id = $1 GROUP BY 1, 2`, resultID)
	if err != nil {
		return nil, err
	}
	defer counts.Close()
	for counts.Next() {
		var queryID int64
		var stage string
		var n int
		if err := counts.Scan(&queryID, &stage, &n); err != nil {
			return nil, err
		}
		byID[queryID].Counts[stage] = n
	}
	if err := counts.Err(); err != nil {
		return nil, err
	}

	// A re-indexed or deleted artifact keeps its row, without a label.
	candidates, err := db.QueryContext(ctx, `SELECT c.query_id, c.stage, c.stage_rank, c.partition_name, c.artifact_type, c.artifact_id,
 COALESCE(sa.primary_label, ''), c.score, c.partition_rank, c.lexical_rank, c.semantic_rank
 FROM testbed.hybrid_search_benchmark_candidates c
 JOIN testbed.hybrid_search_benchmark_queries q ON q.id = c.query_id
 LEFT JOIN kb.search_artifacts sa ON sa.artifact_type = c.artifact_type AND sa.artifact_id = c.artifact_id
 WHERE q.result_id = $1 AND (c.stage_rank <= $2 OR c.artifact_id = $3)
 ORDER BY c.query_id, c.stage, c.partition_name, c.stage_rank`, resultID, limit, target)
	if err != nil {
		return nil, err
	}
	defer candidates.Close()
	for candidates.Next() {
		var queryID int64
		var stage string
		var c hybridBenchTraceCandidate
		var partitionRank, lexicalRank, semanticRank sql.NullInt64
		if err := candidates.Scan(&queryID, &stage, &c.Rank, &c.PartitionName, &c.ArtifactType, &c.ArtifactID,
			&c.Label, &c.Score, &partitionRank, &lexicalRank, &semanticRank); err != nil {
			return nil, err
		}
		c.PartitionRank, c.LexicalRank, c.SemanticRank = nullRank(partitionRank), nullRank(lexicalRank), nullRank(semanticRank)
		q := byID[queryID]
		q.Stages[stage] = append(q.Stages[stage], c)
	}
	return queries, candidates.Err()
}
