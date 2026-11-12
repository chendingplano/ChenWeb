package complianceeval

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// GoldRun is a complete gold labelling, ready to save.
type GoldRun struct {
	SnapshotID   int64
	RulesFile    string
	RulesSHA256  string
	Profiles     Profiles
	PoolLimit    int
	Organization map[string]OrgFact
	Documents    []*GoldDocument
}

// SaveRun writes the run, its activities and judgements in one transaction and returns the run key
// and id. now is a parameter so tests control the key.
func SaveRun(ctx context.Context, db *sql.DB, r *GoldRun, now time.Time) (key string, id int64, err error) {
	org, err := json.Marshal(r.Organization)
	if err != nil {
		return "", 0, err
	}
	key = now.Format("20060102_150405")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `INSERT INTO testbed.compliance_gold_runs
		(run_key, snapshot_id, rules_file, rules_sha256, annotator_1, annotator_2, adjudicator, pool_limit, organization)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		key, r.SnapshotID, r.RulesFile, r.RulesSHA256, r.Profiles.Annotator1, r.Profiles.Annotator2,
		r.Profiles.Adjudicator, r.PoolLimit, string(org)).Scan(&id); err != nil {
		return "", 0, fmt.Errorf("insert gold run: %w", err)
	}
	for _, d := range r.Documents {
		for _, a := range d.Activities {
			var actID int64
			if err := tx.QueryRowContext(ctx, `INSERT INTO testbed.compliance_gold_activities
				(run_id, input_record_id, covered, activity, object, line_start, line_end, document_level, agreement, rationale)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
				id, d.RecordID, d.Covered, a.Name, a.Object, a.LineStart, a.LineEnd, a.DocumentLevel, a.Agreement, a.Rationale).Scan(&actID); err != nil {
				return "", 0, fmt.Errorf("insert activity %q of record %d: %w", a.Name, d.RecordID, err)
			}
			for _, j := range a.Judgements {
				if _, err := tx.ExecContext(ctx, `INSERT INTO testbed.compliance_gold_judgements
					(activity_id, artifact_type, artifact_id, candidate_record, cosine, label_1, reason_1, label_2, reason_2, final_label, adjudicated, rationale)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
					actID, j.Candidate.ArtifactType, j.Candidate.ArtifactID, j.Candidate.RecordID, j.Candidate.Cosine,
					j.Label1, j.Reason1, j.Label2, j.Reason2, j.Final, j.Adjudicated, j.Rationale); err != nil {
					return "", 0, fmt.Errorf("insert judgement %s of activity %q: %w", j.Candidate.key(), a.Name, err)
				}
			}
		}
	}
	return key, id, tx.Commit()
}
