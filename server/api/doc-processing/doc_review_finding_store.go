package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// DocReviewFindingFilter specifies optional server-side filters for findings listing.
type DocReviewFindingFilter struct {
	InputRecordID *int64
	RunID         *int64
	Pass          string
	Aspect        string
	Severity      string
	ReviewStatus  string
	FindingType   string
	ArtifactID    string
	Title         string
	Page          int
	PageSize      int
}

// DocReviewFindingRow is one row returned from kb.doc_review_findings.
type DocReviewFindingRow struct {
	ID            int64
	InputRecordID int64
	RunID         int64
	Pass          string
	Aspect        string
	Severity      string
	FindingType   string
	Title         string
	Description   string
	Evidence      string
	Location      string
	Suggestion    string
	Confidence    float64
	ReviewStatus  string
	ArtifactID    string
	Metadata      json.RawMessage
	ReferenceDoc  json.RawMessage
}

// DocReviewFindingSQLStore provides read-only access to persisted review findings.
type DocReviewFindingSQLStore struct{ DB *sql.DB }

func (s DocReviewFindingSQLStore) ListDocReviewFindings(ctx context.Context, f DocReviewFindingFilter) ([]DocReviewFindingRow, int64, error) {
	if s.DB == nil {
		return nil, 0, fmt.Errorf("db is nil")
	}
	page, pageSize := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}

	where, args, index := make([]string, 0, 9), make([]any, 0, 9), 1
	add := func(column, operator string, value any) {
		where = append(where, "f."+column+" "+operator+" $"+strconv.Itoa(index))
		args = append(args, value)
		index++
	}
	if f.InputRecordID != nil {
		add("input_record_id", "=", *f.InputRecordID)
	}
	if f.RunID != nil {
		add("run_id", "=", *f.RunID)
	}
	if f.Pass != "" {
		add("pass", "=", f.Pass)
	}
	if f.Aspect != "" {
		add("aspect", "=", f.Aspect)
	}
	if f.Severity != "" {
		add("severity", "=", f.Severity)
	}
	if f.ReviewStatus != "" {
		add("review_status", "=", f.ReviewStatus)
	}
	if f.FindingType != "" {
		add("finding_type", "=", f.FindingType)
	}
	if f.ArtifactID != "" {
		// A compliance finding names only its first activity's artifact; its other activities
		// carry their own (openspec compliance-review-storage).
		p := "$" + strconv.Itoa(index)
		where = append(where, "(f.artifact_id ILIKE "+p+" ESCAPE '\\' OR EXISTS (SELECT 1 FROM kb.doc_review_compliance_activities a"+
			" WHERE a.finding_id = f.id AND a.source_artifact_id ILIKE "+p+" ESCAPE '\\'))")
		args = append(args, "%"+escapeDocReviewLogLike(f.ArtifactID)+"%")
		index++
	}
	if f.Title != "" {
		where = append(where, "f.title ILIKE $"+strconv.Itoa(index)+" ESCAPE '\\'")
		args = append(args, "%"+escapeDocReviewLogLike(f.Title)+"%")
		index++
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM kb.doc_review_findings f "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// A compliance finding returns marker metadata, and a normalized one its content from its
	// details row (openspec compliance-review-storage).
	stmt := `
SELECT f.id, f.input_record_id, f.run_id, f.pass, f.aspect, f.severity, f.finding_type, f.title,
       COALESCE(d.description, f.description, ''), COALESCE(d.evidence, f.evidence, ''), COALESCE(f.location,''),
       COALESCE(d.suggestion, f.suggestion, ''), COALESCE(f.confidence,0), COALESCE(f.review_status,'pending'),
       COALESCE(f.artifact_id,''),
       CASE WHEN f.aspect = 'compliance' THEN jsonb_strip_nulls(jsonb_build_object('schema_version', f.metadata->'schema_version',
            'result_kind', f.metadata->'result_kind', 'storage_version', f.storage_version))::text
            ELSE COALESCE(f.metadata, '{}'::jsonb)::text END,
       COALESCE(f.reference_doc, '{}'::jsonb)::text
FROM kb.doc_review_findings f
LEFT JOIN kb.doc_review_finding_details d ON d.finding_id = f.id
` + whereClause + `
ORDER BY f.run_id DESC, f.id ASC
LIMIT $` + strconv.Itoa(index) + ` OFFSET $` + strconv.Itoa(index+1)
	rows, err := s.DB.QueryContext(ctx, stmt, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]DocReviewFindingRow, 0)
	for rows.Next() {
		var row DocReviewFindingRow
		var metadata, referenceDoc sql.NullString
		if err := rows.Scan(
			&row.ID, &row.InputRecordID, &row.RunID, &row.Pass, &row.Aspect, &row.Severity,
			&row.FindingType, &row.Title, &row.Description, &row.Evidence, &row.Location,
			&row.Suggestion, &row.Confidence, &row.ReviewStatus, &row.ArtifactID,
			&metadata, &referenceDoc,
		); err != nil {
			return nil, 0, err
		}
		row.Metadata = rawDocReviewLogJSON(metadata)
		row.ReferenceDoc = rawDocReviewLogJSON(referenceDoc)
		result = append(result, row)
	}
	return result, total, rows.Err()
}
