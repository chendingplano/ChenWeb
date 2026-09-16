package productreviews

import (
	"context"
	"database/sql"
	"strings"

	"github.com/lib/pq"
)

// Hybrid-search tuning, mirroring api/kbhandler/search_registry.go: rrfK is the
// standard Reciprocal Rank Fusion constant; hybridCandidateLimit bounds each
// ranked list before fusion.
const (
	rrfK                 = 60
	hybridCandidateLimit = 200
)

type rrfHit struct {
	ArtifactType  string
	ArtifactID    string
	InputRecordID int64
	SourceRowID   int64
	Score         float64
	// VectorSim is the raw cosine similarity (1 - embedding distance) to the
	// query vector, 0 when there was no vector half or no semantic match.
	// Unlike Score (a rank-fused value only meaningful within one node's own
	// candidate list), this is comparable across different nodes' searches —
	// needed to tell which of several matched nodes an artifact is actually
	// closest to (bug 2026091601).
	VectorSim float64
}

// rrfSearch fuses a lexical ranking and — when vec is non-nil — a pgvector
// cosine ranking over the given kb.search_artifacts partitions, via RRF. It
// never calls an LLM: the vector is a pre-computed node embedding.
// minSimilarity floors the vector half (1 - embedding distance); <= 0 falls
// back to defaultHybridSimilarityMin. The lexical half is unaffected — it
// already requires a real text match (bug 2026091301: the vector half had no
// floor at all, so it always returned its nearest 200 neighbors by raw
// distance, however unrelated, once a node's label had any embedding).
func rrfSearch(ctx context.Context, db *sql.DB, partitions []string, queryText string, vec []float64, limit int, minSimilarity float64) ([]rrfHit, error) {
	queryText = strings.TrimSpace(queryText)
	if queryText == "" || len(partitions) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = hybridCandidateLimit
	}
	if minSimilarity <= 0 {
		minSimilarity = defaultHybridSimilarityMin
	}

	const lexCTE = `
lex AS (
	SELECT sa.artifact_type, sa.artifact_id, sa.input_record_id,
	       COALESCE(sa.source_row_id, 0) AS source_row_id,
	       ROW_NUMBER() OVER (
	         ORDER BY ts_rank_cd(
	           COALESCE(sa.search_vector, to_tsvector('simple', COALESCE(sa.search_document, ''))),
	           plainto_tsquery('simple', $1)) DESC, sa.artifact_id) AS rnk
	FROM kb.search_artifacts sa
	WHERE sa.artifact_type = ANY($2)
	  AND (COALESCE(sa.search_vector, to_tsvector('simple', COALESCE(sa.search_document, '')))
	         @@ plainto_tsquery('simple', $1)
	       OR lower(COALESCE(sa.primary_label, ''))   LIKE '%' || lower($1) || '%'
	       OR lower(COALESCE(sa.secondary_label, '')) LIKE '%' || lower($1) || '%'
	       OR lower(COALESCE(sa.search_document, '')) LIKE '%' || lower($1) || '%')
	ORDER BY rnk
	LIMIT $3
)`

	var query string
	var args []any
	if len(vec) == 0 {
		query = `WITH ` + lexCTE + `
SELECT lex.artifact_type, lex.artifact_id, lex.input_record_id, lex.source_row_id,
       1.0 / ($4 + lex.rnk) AS score, 0.0 AS vecsim
FROM lex
ORDER BY score DESC, lex.artifact_id ASC`
		args = []any{queryText, pq.Array(partitions), limit, rrfK}
	} else {
		query = `WITH ` + lexCTE + `,
sem AS (
	SELECT sa.artifact_type, sa.artifact_id, sa.input_record_id,
	       COALESCE(sa.source_row_id, 0) AS source_row_id,
	       ROW_NUMBER() OVER (ORDER BY sa.embedding <=> $4::vector, sa.artifact_id) AS rnk,
	       1 - (sa.embedding <=> $4::vector) AS vecsim
	FROM kb.search_artifacts sa
	WHERE sa.artifact_type = ANY($2) AND sa.embedding IS NOT NULL
	  AND (1 - (sa.embedding <=> $4::vector)) >= $6
	ORDER BY rnk
	LIMIT $3
)
SELECT COALESCE(lex.artifact_type, sem.artifact_type),
       COALESCE(lex.artifact_id, sem.artifact_id),
       COALESCE(lex.input_record_id, sem.input_record_id),
       COALESCE(NULLIF(lex.source_row_id, 0), sem.source_row_id, 0),
       COALESCE(1.0 / ($5 + lex.rnk), 0.0) + COALESCE(1.0 / ($5 + sem.rnk), 0.0) AS score,
       COALESCE(sem.vecsim, 0.0) AS vecsim
FROM lex
FULL OUTER JOIN sem
  ON lex.artifact_type = sem.artifact_type AND lex.artifact_id = sem.artifact_id
ORDER BY score DESC, 2 ASC`
		args = []any{queryText, pq.Array(partitions), limit, formatVector(vec), rrfK, minSimilarity}
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []rrfHit
	for rows.Next() {
		var h rrfHit
		if err := rows.Scan(&h.ArtifactType, &h.ArtifactID, &h.InputRecordID, &h.SourceRowID, &h.Score, &h.VectorSim); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
