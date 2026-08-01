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
// build derived projections from accepted assertions. Run iterates every
// registered projection kind (DR11 seam 7); registering a second kind (e.g.
// artifact-semantic-link projections for a future normalized family)
// requires no change here, only RegisterProjectionBuilder plus
// RegisterProjectionRecordScope for the new kind (see
// classification_projection.go's init for the template).
type ProjectSemantics struct {
	DB *sql.DB
}

// Run (re)builds every registered projection kind's targets touched by
// assertions accepted for the given input record. A build failure marks the
// target stale (spec §16.3 item 14) rather than leaving kb.projection_state
// looking current with no record that a rebuild is owed.
func (p ProjectSemantics) Run(ctx context.Context, inputRecordID int64) (ProjectReport, error) {
	report := ProjectReport{}
	if p.DB == nil {
		return report, fmt.Errorf("db is nil")
	}

	stateStore := ProjectionStateStore{DB: p.DB}
	for _, kind := range RegisteredProjectionKinds() {
		targetTable, targetsForRecord, ok := LookupProjectionRecordScope(kind)
		if !ok {
			continue
		}
		build, _, ok := LookupProjectionBuilder(kind)
		if !ok {
			continue
		}
		targets, err := targetsForRecord(ctx, p.DB, inputRecordID)
		if err != nil {
			return report, fmt.Errorf("resolve %q targets for record %d: %w", kind, inputRecordID, err)
		}
		for _, targetID := range targets {
			report.TargetsExamined++
			if err := build(ctx, p.DB, targetID); err != nil {
				report.Errors++
				// Best-effort: the build failure is already counted above
				// regardless of whether marking it stale also succeeds.
				_ = stateStore.MarkStale(ctx, kind, targetTable, targetID, err.Error())
				continue
			}
			report.Built++
		}
	}
	return report, nil
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
