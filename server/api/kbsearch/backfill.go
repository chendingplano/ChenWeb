package kbsearch

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// EmbedFunc computes an embedding for text. ok=false means the text could not be
// embedded; that row is counted as failed and left lexical-only (NULL embedding).
type EmbedFunc func(ctx context.Context, text string) (vec []float64, ok bool)

// BackfillResult summarizes a backfill run.
type BackfillResult struct {
	Scanned   int `json:"scanned"`   // rows pulled and attempted
	Embedded  int `json:"embedded"`  // rows successfully embedded + updated
	Skipped   int `json:"skipped"`   // rows with no usable text
	Failed    int `json:"failed"`    // rows whose embedding call failed / wrong dim
	Remaining int `json:"remaining"` // candidates still needing an embedding after this call
}

// BackfillEmbeddings populates kb.search_artifacts.embedding for rows that have a
// non-empty searchable text. By default it processes only rows whose embedding is
// NULL; pass reembedAll=true to recompute every row. artifactType filters to a
// single partition ("" / "all" = every type). limit bounds rows processed in this
// call (<=0 = no bound); call repeatedly until Remaining is 0.
//
// Candidates are read fully before embedding so the SELECT cursor is not held open
// across the per-row embedding calls and UPDATEs. Requires the pgvector migration
// (20260603000001) to have been applied.
func BackfillEmbeddings(ctx context.Context, db *sql.DB, embed EmbedFunc, artifactType string, reembedAll bool, limit int) (BackfillResult, error) {
	var res BackfillResult
	if db == nil {
		return res, fmt.Errorf("db is nil")
	}
	if embed == nil {
		return res, fmt.Errorf("embed func is nil")
	}

	where := []string{"COALESCE(NULLIF(embedding_text, ''), NULLIF(search_document, '')) IS NOT NULL"}
	args := []any{}
	if !reembedAll {
		where = append(where, "embedding IS NULL")
	}
	if at := strings.TrimSpace(artifactType); at != "" && at != "all" {
		where = append(where, fmt.Sprintf("artifact_type = $%d", len(args)+1))
		args = append(args, at)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM kb.search_artifacts WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return res, fmt.Errorf("count backfill candidates: %w", err)
	}

	selectSQL := "SELECT artifact_type, artifact_id, COALESCE(NULLIF(embedding_text, ''), search_document) AS txt " +
		"FROM kb.search_artifacts WHERE " + whereSQL + " ORDER BY artifact_type, artifact_id"
	selArgs := append([]any{}, args...)
	if limit > 0 {
		selectSQL += fmt.Sprintf(" LIMIT $%d", len(selArgs)+1)
		selArgs = append(selArgs, limit)
	}

	type candidate struct {
		artifactType string
		artifactID   string
		text         string
	}
	rows, err := db.QueryContext(ctx, selectSQL, selArgs...)
	if err != nil {
		return res, fmt.Errorf("select backfill candidates: %w", err)
	}
	candidates := make([]candidate, 0, 256)
	for rows.Next() {
		var cnd candidate
		if err := rows.Scan(&cnd.artifactType, &cnd.artifactID, &cnd.text); err != nil {
			rows.Close()
			return res, fmt.Errorf("scan backfill candidate: %w", err)
		}
		candidates = append(candidates, cnd)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return res, err
	}
	rows.Close()

	const updateSQL = `UPDATE kb.search_artifacts
SET embedding = $1::vector, embedding_text = $2, updated_at = NOW()
WHERE artifact_type = $3 AND artifact_id = $4`

	for _, cnd := range candidates {
		if ctx.Err() != nil {
			break
		}
		res.Scanned++
		text := strings.TrimSpace(cnd.text)
		if text == "" {
			res.Skipped++
			continue
		}
		vec, ok := embed(ctx, text)
		if !ok || len(vec) != ConfiguredEmbeddingDim() {
			res.Failed++
			continue
		}
		if _, err := db.ExecContext(ctx, updateSQL, FormatVectorLiteral(vec), text, cnd.artifactType, cnd.artifactID); err != nil {
			return res, fmt.Errorf("update embedding for %s/%s: %w", cnd.artifactType, cnd.artifactID, err)
		}
		res.Embedded++
	}

	res.Remaining = max(total-res.Embedded, 0)
	return res, nil
}
