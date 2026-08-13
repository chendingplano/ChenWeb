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

// DrainDeferredCandidates re-attempts records with any deferred semantic
// candidate. AssociateSemantics performs dependency-aware recovery: changed
// subject links are re-normalized and governed terms are retried only when
// their availability fingerprint changed. Safe to call repeatedly: records
// with unchanged dependencies make no changes.
//
// This re-normalizes every registered family (DR11 seam 5) for each
// affected record rather than flipping the deferred candidate's status
// directly. That is the correct mechanism, not merely a simpler one: a
// resolved referent changes the candidate's proposed_payload
// (subject_object_id gets populated), so re-normalizing naturally produces a
// new payload_fingerprint, which DecisionCandidateStore.Propose already
// turns into a fresh 'candidate' revision that properly supersedes the
// stale deferred one. Directly retrying the deferred row via RetryDeferred
// without re-normalizing was tried first and found live to reprocess the
// OLD, still-unresolved payload -- the dependency-fingerprint retry gate is
// the right tool for a defer reason where the payload is unchanged and
// something external changed (e.g. a governed term being released), not for
// a resolved-referent defer, where the payload itself is what changes.
// Re-normalizing every family rather than hardcoding 'metric' means a
// future family whose normalizer also defers on 'unresolved_referent' is
// covered without editing this drain.
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
WHERE status = 'deferred' AND input_record_id IS NOT NULL
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

	assoc := AssociateSemantics{DB: db}

	for _, recordID := range recordIDs {
		report.RecordsScanned++
		if err := NormalizeAllFamilies(ctx, db, recordID); err != nil {
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
