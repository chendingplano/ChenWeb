package classfoundation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MetricSupportDuplicate identifies one metric occurrence with more than one
// active supports-evidence link. A nil InputRecordID is retained in reports so
// legacy malformed evidence remains visible to the cleanup operator.
type MetricSupportDuplicate struct {
	ArtifactType  string
	ArtifactID    string
	InputRecordID *int64
	SupportLinks  int
}

// MetricSupportCleanup reports and retires only duplicate current metric
// support links. It never removes an evidence row: surplus links are
// soft-deleted and paired with their deterministic survivor in audit history.
type MetricSupportCleanup struct {
	DB DBX
}

// ReportCurrentDuplicates lists all current metric support cardinality
// exceptions, including legacy records without an input-record reference.
func (s MetricSupportCleanup) ReportCurrentDuplicates(ctx context.Context) ([]MetricSupportDuplicate, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	const stmt = `
SELECT e.artifact_type, e.artifact_id, e.input_record_id, COUNT(*) AS support_links
FROM kb.assertion_evidence e
WHERE e.artifact_type = 'metric'
  AND e.evidence_role = 'supports'
  AND NOT e.deleted
GROUP BY e.artifact_type, e.artifact_id, e.input_record_id
HAVING COUNT(*) > 1
ORDER BY e.artifact_type, e.artifact_id, e.input_record_id NULLS FIRST`
	rows, err := s.DB.QueryContext(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("report duplicate metric supports: %w", err)
	}
	defer rows.Close()

	duplicates := make([]MetricSupportDuplicate, 0)
	for rows.Next() {
		var duplicate MetricSupportDuplicate
		var inputRecordID sql.NullInt64
		if err := rows.Scan(&duplicate.ArtifactType, &duplicate.ArtifactID, &inputRecordID, &duplicate.SupportLinks); err != nil {
			return nil, fmt.Errorf("scan duplicate metric support: %w", err)
		}
		if inputRecordID.Valid {
			value := inputRecordID.Int64
			duplicate.InputRecordID = &value
		}
		duplicates = append(duplicates, duplicate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate duplicate metric supports: %w", err)
	}
	return duplicates, nil
}

// RetireCurrentDuplicates selects the oldest active support as the survivor
// for each occurrence, audits every surplus link, and soft-deletes the
// surplus. The one-statement operation is atomic and idempotent for already
// retired rows.
func (s MetricSupportCleanup) RetireCurrentDuplicates(ctx context.Context) (int64, error) {
	if s.DB == nil {
		return 0, errors.New("db is nil")
	}
	const stmt = `WITH ranked AS (
    SELECT e.id AS retired_evidence_id,
           e.artifact_type,
           e.artifact_id,
           e.input_record_id,
           FIRST_VALUE(e.id) OVER (
               PARTITION BY e.artifact_type, e.artifact_id, e.input_record_id
               ORDER BY e.create_time, e.id
           ) AS retained_evidence_id,
           ROW_NUMBER() OVER (
               PARTITION BY e.artifact_type, e.artifact_id, e.input_record_id
               ORDER BY e.create_time, e.id
           ) AS support_rank
    FROM kb.assertion_evidence e
    WHERE e.artifact_type = 'metric'
      AND e.evidence_role = 'supports'
      AND NOT e.deleted
), duplicates AS (
    SELECT * FROM ranked WHERE support_rank > 1
), audited AS (
    INSERT INTO kb.metric_support_cleanup_audit (
        artifact_type, artifact_id, input_record_id,
        retained_evidence_id, retired_evidence_id, cleanup_reason
    )
    SELECT artifact_type, artifact_id, input_record_id,
           retained_evidence_id, retired_evidence_id,
           'duplicate_current_metric_support'
    FROM duplicates
    ON CONFLICT (retired_evidence_id) DO NOTHING
)
UPDATE kb.assertion_evidence e
SET deleted = TRUE,
    deleted_reason = 'duplicate_current_metric_support; retained evidence id=' || d.retained_evidence_id,
    deleted_time = NOW()
FROM duplicates d
WHERE e.id = d.retired_evidence_id
  AND NOT e.deleted`
	result, err := s.DB.ExecContext(ctx, stmt)
	if err != nil {
		return 0, fmt.Errorf("retire duplicate metric supports: %w", err)
	}
	retired, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count retired duplicate metric supports: %w", err)
	}
	return retired, nil
}
