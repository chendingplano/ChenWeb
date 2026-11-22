package complianceeval

import (
	"context"
	"database/sql"
	"fmt"

	docreviews "github.com/chendingplano/deepdoc/server/api/doc-reviews"
	"github.com/lib/pq"
)

// LoadGoldActs reads a saved gold run's activities with their governing candidate records.
func LoadGoldActs(ctx context.Context, db *sql.DB, runKey string) ([]GoldAct, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.id, a.input_record_id, a.line_start, a.line_end, a.document_level,
		       COALESCE(array_agg(j.candidate_record) FILTER (WHERE j.final_label = 'governs'), '{}'::bigint[]),
		       count(*) FILTER (WHERE j.final_label = 'ambiguous'),
		       count(j.id),
		       COALESCE(array_agg(j.artifact_type || '/' || j.artifact_id ORDER BY j.id) FILTER (WHERE j.id IS NOT NULL), '{}'::text[]),
		       COALESCE(array_agg(j.final_label ORDER BY j.id) FILTER (WHERE j.id IS NOT NULL), '{}'::text[])
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
		var keys, labels []string
		if err := rows.Scan(&id, &g.Record, &g.LineStart, &g.LineEnd, &g.DocumentLevel, pq.Array(&governs), &g.Ambiguous, &g.Pooled,
			pq.Array(&keys), pq.Array(&labels)); err != nil {
			return nil, err
		}
		g.Governs = map[int64]bool{}
		for _, v := range governs {
			g.Governs[v] = true
		}
		g.Labels = make(map[string]string, len(keys))
		for i, k := range keys {
			g.Labels[k] = labels[i]
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

// GoldRunInfo describes a saved gold run for reporting.
type GoldRunInfo struct {
	Key, Method, Annotator1 string
	SnapshotID              int64
}

// LoadGold reads the activities of one or more gold runs (one run per document for agent-built gold).
// The runs must share one snapshot, whose record set is returned for the scorer's unjudged check, and
// no document may appear in two of them.
func LoadGold(ctx context.Context, db *sql.DB, keys []string) ([]GoldAct, []GoldRunInfo, map[int64]bool, error) {
	var acts []GoldAct
	var infos []GoldRunInfo
	owner := map[int64]string{}
	for _, k := range keys {
		var info GoldRunInfo
		info.Key = k
		if err := db.QueryRowContext(ctx, `SELECT method, annotator_1, snapshot_id FROM testbed.compliance_gold_runs WHERE run_key = $1`, k).
			Scan(&info.Method, &info.Annotator1, &info.SnapshotID); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil, nil, fmt.Errorf("gold run %q does not exist", k)
			}
			return nil, nil, nil, err
		}
		if len(infos) > 0 && info.SnapshotID != infos[0].SnapshotID {
			return nil, nil, nil, fmt.Errorf("gold runs %s and %s use different snapshots (%d, %d); score them separately",
				infos[0].Key, k, infos[0].SnapshotID, info.SnapshotID)
		}
		a, err := LoadGoldActs(ctx, db, k)
		if err != nil {
			return nil, nil, nil, err
		}
		copies := map[int64]map[int64]bool{}
		for i, g := range a {
			if prev, ok := owner[g.Record]; ok && prev != k {
				return nil, nil, nil, fmt.Errorf("record %d is in gold runs %s and %s; score one of them", g.Record, prev, k)
			}
			owner[g.Record] = k
			if copies[g.Record] == nil {
				ids, err := docreviews.SameMD5RecordIDs(ctx, db, g.Record)
				if err != nil {
					return nil, nil, nil, err
				}
				copies[g.Record] = map[int64]bool{g.Record: true}
				for _, id := range ids {
					copies[g.Record][id] = true
				}
			}
			a[i].OwnCopies = copies[g.Record]
		}
		acts = append(acts, a...)
		infos = append(infos, info)
	}
	if len(infos) == 0 {
		return nil, nil, nil, fmt.Errorf("no gold run given")
	}
	members, err := SnapshotMembers(ctx, db, infos[0].SnapshotID)
	if err != nil {
		return nil, nil, nil, err
	}
	in := make(map[int64]bool, len(members))
	for _, m := range members {
		in[m.RecordID] = true
	}
	return acts, infos, in, nil
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
	// A run whose Compliance Review recorded errors is partial; scoring it would read missing
	// activities as reviewer mistakes (openspec compliance-reviewer-redesign).
	var failed int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb.doc_review_findings
		WHERE run_id = $1 AND aspect = 'compliance' AND proc_status = 'failed'`, runID).Scan(&failed); err != nil {
		return nil, err
	}
	if failed > 0 {
		return nil, fmt.Errorf("review run %d is marked failed (its Compliance Review recorded errors); the scorer refuses failed runs", runID)
	}
	// compliance_unchecked reports activities skipped by the limits; it is not an activity.
	rs, err := db.QueryContext(ctx, `SELECT finding_type, title, COALESCE(location, ''), COALESCE(description, '')
		FROM kb.doc_review_findings WHERE run_id = $1 AND aspect = 'compliance' AND finding_type <> 'compliance_unchecked' ORDER BY id`, runID)
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
