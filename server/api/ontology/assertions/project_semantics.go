package assertions

import (
	"context"
	"database/sql"
	"fmt"
)

// ProjectReport summarizes one project_semantics run over one input record
// (spec §10.9 telemetry feeds from this).
type ProjectReport struct {
	TargetsExamined int
	Built           int
	Repaired        int
	Errors          int
}

// ProjectSemantics implements the third DR8 Phase D stage (spec §10.8):
// build derived projections from accepted assertions. This slice's only
// registered projection kind is ProjectionKindObjectPrimaryClass (DR10);
// registering a second kind (e.g. artifact-semantic-link projections for a
// future normalized family) requires no change here, only a new
// RegisterProjectionBuilder call (DR11 seam 7).
type ProjectSemantics struct {
	DB *sql.DB
}

// Run (re)builds every registered projection kind's targets touched by
// assertions accepted for the given input record.
func (p ProjectSemantics) Run(ctx context.Context, inputRecordID int64) (ProjectReport, error) {
	report := ProjectReport{}
	if p.DB == nil {
		return report, fmt.Errorf("db is nil")
	}

	targets, err := p.classificationTargetsForRecord(ctx, inputRecordID)
	if err != nil {
		return report, err
	}

	build, _, ok := LookupProjectionBuilder(ProjectionKindObjectPrimaryClass)
	if !ok {
		return report, fmt.Errorf("no projection builder registered for %q", ProjectionKindObjectPrimaryClass)
	}
	for _, objectID := range targets {
		report.TargetsExamined++
		if err := build(ctx, p.DB, objectID); err != nil {
			report.Errors++
			continue
		}
		report.Built++
	}
	return report, nil
}

// classificationTargetsForRecord returns the distinct subject object ids of
// accepted core:instance_of assertions whose evidence traces back to this
// input record -- the objects whose primary_class_term_id projection may
// need (re)building after this record's Phase D run.
func (p ProjectSemantics) classificationTargetsForRecord(ctx context.Context, inputRecordID int64) ([]string, error) {
	const stmt = `
SELECT DISTINCT a.subject_object_id
FROM kb.semantic_assertions a
JOIN kb.assertion_evidence e ON e.assertion_id = a.id
WHERE e.input_record_id = $1
  AND a.predicate_term_id = 'core:instance_of'
  AND a.subject_object_id IS NOT NULL`
	rows, err := p.DB.QueryContext(ctx, stmt, inputRecordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var objectID string
		if err := rows.Scan(&objectID); err != nil {
			return nil, err
		}
		out = append(out, objectID)
	}
	return out, rows.Err()
}

// RepairStaleProjections sweeps every target of a registered projection kind
// and repairs any whose materialized value no longer matches its
// authoritative source -- catching both genuine staleness (spec §16.3 item
// 14: "a partial projection failure leaves the authoritative association
// committed, marks the projection stale, and repairs idempotently on
// retry") and direct corruption of the materialized column (spec §16.2 item
// 6). This is independent of any single input record, so it is exposed
// separately from Run rather than folded into the per-record Phase D stage.
func RepairStaleProjections(ctx context.Context, db *sql.DB, kind string) (examined, repaired int, err error) {
	if db == nil {
		return 0, 0, fmt.Errorf("db is nil")
	}
	_, repair, ok := LookupProjectionBuilder(kind)
	if !ok {
		return 0, 0, fmt.Errorf("no projection builder registered for %q", kind)
	}

	stateStore := ProjectionStateStore{DB: db}
	states, err := stateStore.ListStale(ctx, "") // includes every kind; filtered below
	if err != nil {
		return 0, 0, err
	}
	targets := make(map[string]struct{})
	for _, s := range states {
		if s.ProjectionKind == kind {
			targets[s.ProjectionTargetID] = struct{}{}
		}
	}
	// Rows that were never marked stale can still be corrupted (a direct
	// hand-edit of the materialized column, which never touches
	// kb.projection_state). Sweep every known target for this kind, not
	// only the ones already flagged.
	const stmt = `SELECT DISTINCT projection_target_id FROM kb.projection_state WHERE projection_kind = $1`
	rows, err := db.QueryContext(ctx, stmt, kind)
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var targetID string
		if err := rows.Scan(&targetID); err != nil {
			rows.Close()
			return 0, 0, err
		}
		targets[targetID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, err
	}
	rows.Close()

	for targetID := range targets {
		examined++
		fixed, err := repair(ctx, db, targetID)
		if err != nil {
			return examined, repaired, fmt.Errorf("repair %s target %s: %w", kind, targetID, err)
		}
		if fixed {
			repaired++
		}
	}
	return examined, repaired, nil
}
