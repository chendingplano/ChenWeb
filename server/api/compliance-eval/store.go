package complianceeval

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Labelling methods of a gold run.
const (
	MethodAgent      = "agent"       // the coding agent labels alone (first iteration)
	MethodModelPanel = "model_panel" // two model annotators and a model adjudicator (deferred)
)

// GoldRun is a complete gold labelling, ready to save. For MethodAgent, Profiles.Annotator1 is the
// agent's model and the other two profiles are empty.
type GoldRun struct {
	Method        string
	SnapshotID    int64
	RulesFile     string
	RulesVersion  string
	RulesSHA256   string
	Profiles      Profiles
	MaxCandidates int
	AuditNotes    string
	Documents     []*GoldDocument
	// BaseRunID and ReviewRunID are set on an augmented run (design D12): the run it extends and the
	// doc-review run whose unjudged passages it adds. Zero means none.
	BaseRunID, ReviewRunID int64
}

// nullIfZero stores 0 as NULL.
func nullIfZero(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: v != 0} }

// nullIfEmpty stores an empty string as NULL.
func nullIfEmpty(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// SaveRun writes the run, its activities and judgements in one transaction and returns the run key
// and id. now is a parameter so tests control the key.
func SaveRun(ctx context.Context, db *sql.DB, r *GoldRun, now time.Time) (key string, id int64, err error) {
	key = now.Format("20060102_150405")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `INSERT INTO testbed.compliance_gold_runs
		(run_key, method, snapshot_id, rules_file, rules_version, rules_sha256, annotator_1, annotator_2, adjudicator, max_candidates, audit_notes,
		 base_run_id, review_run_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		key, r.Method, r.SnapshotID, r.RulesFile, r.RulesVersion, r.RulesSHA256, r.Profiles.Annotator1,
		nullIfEmpty(r.Profiles.Annotator2), nullIfEmpty(r.Profiles.Adjudicator), r.MaxCandidates, r.AuditNotes,
		nullIfZero(r.BaseRunID), nullIfZero(r.ReviewRunID)).Scan(&id); err != nil {
		return "", 0, fmt.Errorf("insert gold run: %w", err)
	}
	for _, d := range r.Documents {
		for _, a := range d.Activities {
			var actID int64
			if err := tx.QueryRowContext(ctx, `INSERT INTO testbed.compliance_gold_activities
				(run_id, input_record_id, covered, activity, object, line_start, line_end, document_level, agreement, rationale, queries, cited_docs)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
				id, d.RecordID, d.Covered, a.Name, a.Object, a.LineStart, a.LineEnd, a.DocumentLevel, a.Agreement, a.Rationale,
				pq.Array(nonNil(a.Queries)), pq.Array(nonNil(a.CitedDocs))).Scan(&actID); err != nil {
				return "", 0, fmt.Errorf("insert activity %q of record %d: %w", a.Name, d.RecordID, err)
			}
			for _, j := range a.Judgements {
				if _, err := tx.ExecContext(ctx, `INSERT INTO testbed.compliance_gold_judgements
					(activity_id, artifact_type, artifact_id, candidate_record, cosine, label_1, reason_1, label_2, reason_2, final_label, adjudicated, rationale, found_by)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
					actID, j.Candidate.ArtifactType, j.Candidate.ArtifactID, j.Candidate.RecordID, j.Candidate.Cosine,
					j.Label1, j.Reason1, nullIfEmpty(j.Label2), j.Reason2, j.Final, j.Adjudicated, j.Rationale, j.Candidate.FoundBy); err != nil {
					return "", 0, fmt.Errorf("insert judgement %s of activity %q: %w", j.Candidate.key(), a.Name, err)
				}
			}
		}
	}
	return key, id, tx.Commit()
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// RunSummary is a saved gold run as read back from the database.
type RunSummary struct {
	Key, Method, RulesFile, RulesVersion, Annotator1 string
	ID, SnapshotID                                   int64
	MaxCandidates                                    int
	Records                                          []int64
	Activities, DocumentLevel                        int
	Labels                                           map[string]int // final label -> judgements
	ActivitiesWithGoverns                            int
	BaseRun                                          string // key of the run an augmented run extends
	ReviewRun                                        int64
	GovernsByChannel                                 map[string]int // found_by channel -> governs judgements
}

// FoundByChannel groups a found_by value: "pool" (q2/lexical...), "cited", "expand", "review", or
// "unknown" for runs saved before found_by existed.
func FoundByChannel(foundBy string) string {
	switch {
	case foundBy == "":
		return "unknown"
	case strings.HasPrefix(foundBy, "q"):
		return "pool"
	}
	if i := strings.Index(foundBy, ":"); i > 0 {
		return foundBy[:i]
	}
	return foundBy
}

// LoadRunSummary reads back a saved gold run by its key.
func LoadRunSummary(ctx context.Context, db *sql.DB, key string) (*RunSummary, error) {
	r := &RunSummary{Key: key, Labels: map[string]int{}, GovernsByChannel: map[string]int{}}
	var review sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT r.id, r.method, r.snapshot_id, r.rules_file, r.rules_version, r.annotator_1, r.max_candidates,
		       COALESCE(b.run_key, ''), r.review_run_id
		FROM testbed.compliance_gold_runs r LEFT JOIN testbed.compliance_gold_runs b ON b.id = r.base_run_id
		WHERE r.run_key = $1`, key).
		Scan(&r.ID, &r.Method, &r.SnapshotID, &r.RulesFile, &r.RulesVersion, &r.Annotator1, &r.MaxCandidates, &r.BaseRun, &review); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("gold run %q does not exist", key)
		}
		return nil, err
	}
	r.ReviewRun = review.Int64
	fb, err := db.QueryContext(ctx, `SELECT j.found_by, count(*) FROM testbed.compliance_gold_judgements j
		JOIN testbed.compliance_gold_activities a ON a.id = j.activity_id
		WHERE a.run_id = $1 AND j.final_label = 'governs' GROUP BY j.found_by`, r.ID)
	if err != nil {
		return nil, err
	}
	for fb.Next() {
		var foundBy string
		var n int
		if err := fb.Scan(&foundBy, &n); err != nil {
			fb.Close()
			return nil, err
		}
		r.GovernsByChannel[FoundByChannel(foundBy)] += n
	}
	fb.Close()
	if err := fb.Err(); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT a.input_record_id, a.document_level,
		       count(j.id) FILTER (WHERE j.final_label = 'governs'),
		       array_remove(array_agg(j.final_label), NULL)
		FROM testbed.compliance_gold_activities a
		LEFT JOIN testbed.compliance_gold_judgements j ON j.activity_id = a.id
		WHERE a.run_id = $1 GROUP BY a.id ORDER BY a.input_record_id, a.line_start`, r.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	for rows.Next() {
		var rec int64
		var doc bool
		var governs int
		var labels []string
		if err := rows.Scan(&rec, &doc, &governs, pq.Array(&labels)); err != nil {
			return nil, err
		}
		if !seen[rec] {
			seen[rec] = true
			r.Records = append(r.Records, rec)
		}
		r.Activities++
		if doc {
			r.DocumentLevel++
		}
		if governs > 0 {
			r.ActivitiesWithGoverns++
		}
		for _, l := range labels {
			r.Labels[l]++
		}
	}
	return r, rows.Err()
}
