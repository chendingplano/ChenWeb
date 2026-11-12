package complianceeval

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

// LoadGoldActs reads a saved gold run's activities with their governing candidate records.
func LoadGoldActs(ctx context.Context, db *sql.DB, runKey string) ([]GoldAct, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.id, a.input_record_id, a.line_start, a.line_end, a.document_level,
		       COALESCE(array_agg(j.candidate_record) FILTER (WHERE j.final_label = 'governs'), '{}'::bigint[]),
		       count(*) FILTER (WHERE j.final_label = 'ambiguous'),
		       count(j.id)
		FROM testbed.compliance_gold_activities a
		JOIN testbed.compliance_gold_runs r ON r.id = a.run_id
		LEFT JOIN testbed.compliance_gold_judgements j ON j.activity_id = a.id
		WHERE r.run_key = $1
		GROUP BY a.id ORDER BY a.input_record_id, a.line_start`, runKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GoldAct
	for rows.Next() {
		var id int64
		var g GoldAct
		var governs []int64
		if err := rows.Scan(&id, &g.Record, &g.LineStart, &g.LineEnd, &g.DocumentLevel, pq.Array(&governs), &g.Ambiguous, &g.Pooled); err != nil {
			return nil, err
		}
		g.Governs = map[int64]bool{}
		for _, v := range governs {
			g.Governs[v] = true
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("gold run %q has no activities (does it exist?)", runKey)
	}
	return out, nil
}

// LoadReviewRows reads the Compliance Review findings of one doc-review run.
func LoadReviewRows(ctx context.Context, db *sql.DB, runID int64) ([]ReviewRow, error) {
	var record int64
	if err := db.QueryRowContext(ctx, `SELECT input_record_id FROM kb.doc_review_runs WHERE id = $1`, runID).Scan(&record); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("review run %d does not exist", runID)
		}
		return nil, err
	}
	rs, err := db.QueryContext(ctx, `SELECT finding_type, title, COALESCE(location, ''), COALESCE(description, '')
		FROM kb.doc_review_findings WHERE run_id = $1 AND aspect = 'compliance' ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []ReviewRow
	for rs.Next() {
		var ft, title, loc, desc string
		if err := rs.Scan(&ft, &title, &loc, &desc); err != nil {
			return nil, err
		}
		row, err := ParseReviewRow(record, ft, title, loc, desc)
		if err != nil {
			return nil, fmt.Errorf("review run %d: %w", runID, err)
		}
		out = append(out, row)
	}
	if err := rs.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("review run %d has no compliance findings", runID)
	}
	return out, nil
}
