package assertions

import (
	"context"
	"database/sql"
	"fmt"
)

// DrainReport summarizes one backlog-drain pass over deferred decision
// candidates.
type DrainReport struct {
	RecordsScanned     int
	RecordsReprocessed int
	Accepted           int
	Deferred           int
	Rejected           int
}

// DrainDeferredCandidates re-attempts input records with decision candidates
// deferred on 'unresolved_referent' -- the ADR 2026070701 DR5 bulk-backfill
// pattern, reused rather than a new backlog mechanism. Safe to call
// repeatedly: records with nothing newly resolved make no changes.
//
// This re-runs the metric normalizer for each affected record rather than
// flipping the deferred candidate's status directly. That is the correct
// mechanism, not merely a simpler one: a resolved referent changes the
// candidate's proposed_payload (subject_object_id gets populated), so
// re-normalizing naturally produces a new payload_fingerprint, which
// DecisionCandidateStore.Propose already turns into a fresh 'candidate'
// revision that properly supersedes the stale deferred one. Directly
// retrying the deferred row via RetryDeferred without re-normalizing was
// tried first and found live to reprocess the OLD, still-unresolved payload
// -- the dependency-fingerprint retry gate is the right tool for a defer
// reason where the payload is unchanged and something external changed
// (e.g. a governed term being released), not for a resolved-referent defer,
// where the payload itself is what changes.
//
// This drain targets specifically the 'unresolved_referent' reason -- the
// dominant real-world blocker found in this workspace's gold corpus (chunk D
// of the P3 implementation log: kb.artifact_objects has zero rows for
// artifact_type='metric'). A governed-term-availability drain is a natural,
// separable follow-up if a released module is ever rolled back after
// candidates were deferred on it; it is not built here because no such case
// has been observed.
func DrainDeferredCandidates(ctx context.Context, db *sql.DB, limit int) (DrainReport, error) {
	report := DrainReport{}
	if db == nil {
		return report, fmt.Errorf("db is nil")
	}
	if limit <= 0 {
		limit = 200
	}

	const stmt = `
SELECT DISTINCT input_record_id
FROM kb.semantic_decision_candidates
WHERE status = 'deferred' AND dependency_fingerprint = 'unresolved_referent'
  AND input_record_id IS NOT NULL
ORDER BY input_record_id
LIMIT $1`
	rows, err := db.QueryContext(ctx, stmt, limit)
	if err != nil {
		return report, err
	}
	var recordIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return report, err
		}
		recordIDs = append(recordIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return report, err
	}
	rows.Close()

	metricNormalizer, ok := LookupNormalizer("metric")
	if !ok {
		return report, fmt.Errorf("no normalizer registered for family %q", "metric")
	}
	assoc := AssociateSemantics{DB: db}

	for _, recordID := range recordIDs {
		report.RecordsScanned++
		if _, err := metricNormalizer.Normalize(ctx, db, recordID); err != nil {
			return report, fmt.Errorf("re-normalize record %d: %w", recordID, err)
		}
		assocReport, err := assoc.Run(ctx, recordID)
		if err != nil {
			return report, fmt.Errorf("reprocess record %d after drain: %w", recordID, err)
		}
		if assocReport.Accepted > 0 {
			report.RecordsReprocessed++
		}
		report.Accepted += assocReport.Accepted
		report.Deferred += assocReport.Deferred
		report.Rejected += assocReport.Rejected
	}

	return report, nil
}
