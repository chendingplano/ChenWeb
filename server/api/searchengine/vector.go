package searchengine

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

type candKey struct{ typ, id string }

type vecHit struct {
	candKey
	distance float64
}

func pqStrings(s []string) any { return pq.Array(s) }

// vectorList returns the nearest artifacts to the query vector (registry rows and, for chunks, their
// embedding windows), filtered before ranking. The vector must be the first argument ($1).
//
// The candidates come from a plain ORDER BY distance LIMIT, the only shape pgvector serves from its
// index. Iterative scans keep an HNSW scan going until the LIMIT is met even when filters drop rows.
func vectorList(ctx context.Context, db *sql.DB, vecLiteral string, f Filters, self *ArtifactRef) ([]vecHit, error) {
	a := argList{}
	v := a.add(vecLiteral)
	filter := ""
	if c := filterClauses(&a, f, self, true); len(c) > 0 {
		filter = " AND " + andJoin(c)
	}
	q := fmt.Sprintf(`
SELECT artifact_type, artifact_id, MIN(distance) AS distance FROM (
	(SELECT sa.artifact_type AS artifact_type, sa.artifact_id AS artifact_id, sa.embedding <=> %[1]s::vector AS distance
	 FROM kb.search_artifacts sa WHERE sa.embedding IS NOT NULL%[2]s
	 ORDER BY sa.embedding <=> %[1]s::vector LIMIT %[3]d)
	UNION ALL
	(SELECT sa.artifact_type, sa.artifact_id, cw.embedding <=> %[1]s::vector
	 FROM kb.chunk_search_windows cw
	 JOIN kb.search_artifacts sa ON sa.artifact_type = 'chunk' AND sa.artifact_id = cw.artifact_id
	 WHERE cw.embedding IS NOT NULL%[2]s
	 ORDER BY cw.embedding <=> %[1]s::vector LIMIT %[3]d)
) w GROUP BY artifact_type, artifact_id ORDER BY distance, artifact_id LIMIT %[3]d`, v, filter, candidateLimit)

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SET LOCAL hnsw.iterative_scan = relaxed_order"); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, q, a.vals...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []vecHit
	for rows.Next() {
		var h vecHit
		if err := rows.Scan(&h.typ, &h.id, &h.distance); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// cosines returns the cosine similarity of each candidate to the query vector (best window for a
// chunk), including candidates only the keyword list found. A candidate with no embedding is absent.
func cosines(ctx context.Context, db *sql.DB, vecLiteral string, cands []candKey) (map[candKey]float64, error) {
	types := make([]string, len(cands))
	ids := make([]string, len(cands))
	for i, c := range cands {
		types[i], ids[i] = c.typ, c.id
	}
	const q = `
SELECT t, i, 1 - MIN(d) FROM (
	SELECT sa.artifact_type AS t, sa.artifact_id AS i, sa.embedding <=> $1::vector AS d
	FROM kb.search_artifacts sa
	JOIN unnest($2::text[], $3::text[]) u(t, i) ON sa.artifact_type = u.t AND sa.artifact_id = u.i
	WHERE sa.embedding IS NOT NULL
	UNION ALL
	SELECT 'chunk', cw.artifact_id, cw.embedding <=> $1::vector
	FROM kb.chunk_search_windows cw
	WHERE cw.artifact_id = ANY($3::text[]) AND cw.embedding IS NOT NULL
) c GROUP BY t, i`
	rows, err := db.QueryContext(ctx, q, vecLiteral, pq.Array(types), pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[candKey]float64{}
	wanted := map[candKey]bool{}
	for _, c := range cands {
		wanted[c] = true
	}
	for rows.Next() {
		var k candKey
		var cos float64
		if err := rows.Scan(&k.typ, &k.id, &cos); err != nil {
			return nil, err
		}
		if wanted[k] { // a chunk window row can belong to an artifact of another type with the same id
			out[k] = cos
		}
	}
	return out, rows.Err()
}
