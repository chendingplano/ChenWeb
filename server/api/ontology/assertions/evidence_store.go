package assertions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Evidence is one supporting or contradicting record for an assertion (spec
// §8.3). ActorKind='human' marks a qualifying human-authored provenance
// record that the artifact-deletion cascade never removes (spec §10.12).
type Evidence struct {
	ID               int64           `json:"id"`
	AssertionID      int64           `json:"assertion_id"`
	InputRecordID    *int64          `json:"input_record_id,omitempty"`
	ArtifactType     string          `json:"artifact_type"`
	ArtifactID       string          `json:"artifact_id"`
	ArtifactObjectID string          `json:"artifact_object_id,omitempty"`
	EvidenceQuote    string          `json:"evidence_quote,omitempty"`
	SourceLineSpans  json.RawMessage `json:"source_line_spans,omitempty"`
	ExtractionRun    string          `json:"extraction_run,omitempty"`
	Model            string          `json:"model,omitempty"`
	PromptVersion    string          `json:"prompt_version,omitempty"`
	Confidence       *float64        `json:"confidence,omitempty"`
	EvidenceRole     string          `json:"evidence_role"`
	ActorKind        string          `json:"actor_kind"`
	Deleted          bool            `json:"deleted"`
	DeletedReason    string          `json:"deleted_reason,omitempty"`
	DeletedTime      *time.Time      `json:"deleted_time,omitempty"`
	CreateTime       time.Time       `json:"create_time"`
	CreateBy         string          `json:"create_by,omitempty"`
}

// EvidenceStore persists assertion evidence and maintains the accepted <->
// unsupported transition on the owning assertion (spec §10.12, §16.3 item 16):
// deleting an assertion's last active 'supports' evidence row demotes it to
// unsupported; restoring qualifying evidence returns it to accepted.
type EvidenceStore struct {
	DB         DBX
	Assertions AssertionStore
}

const evidenceColumns = `
	id, assertion_id, input_record_id, artifact_type, artifact_id,
	artifact_object_id, evidence_quote, source_line_spans, extraction_run,
	model, prompt_version, confidence, evidence_role, actor_kind, deleted,
	deleted_reason, deleted_time, create_time, create_by`

const evidenceFrom = "FROM kb.assertion_evidence"

func scanEvidence(scan func(dest ...any) error) (Evidence, error) {
	var (
		e                Evidence
		inputRecordID    sql.NullInt64
		artifactObjectID sql.NullString
		evidenceQuote    sql.NullString
		lineSpans        json.RawMessage
		extractionRun    sql.NullString
		model            sql.NullString
		promptVersion    sql.NullString
		confidence       sql.NullFloat64
		deletedReason    sql.NullString
		deletedTime      sql.NullTime
		createBy         sql.NullString
	)
	if err := scan(
		&e.ID, &e.AssertionID, &inputRecordID, &e.ArtifactType, &e.ArtifactID,
		&artifactObjectID, &evidenceQuote, &lineSpans, &extractionRun,
		&model, &promptVersion, &confidence, &e.EvidenceRole, &e.ActorKind, &e.Deleted,
		&deletedReason, &deletedTime, &e.CreateTime, &createBy,
	); err != nil {
		return Evidence{}, err
	}
	if inputRecordID.Valid {
		v := inputRecordID.Int64
		e.InputRecordID = &v
	}
	if artifactObjectID.Valid {
		e.ArtifactObjectID = artifactObjectID.String
	}
	if evidenceQuote.Valid {
		e.EvidenceQuote = evidenceQuote.String
	}
	if lineSpans != nil {
		e.SourceLineSpans = lineSpans
	}
	if extractionRun.Valid {
		e.ExtractionRun = extractionRun.String
	}
	if model.Valid {
		e.Model = model.String
	}
	if promptVersion.Valid {
		e.PromptVersion = promptVersion.String
	}
	if confidence.Valid {
		v := confidence.Float64
		e.Confidence = &v
	}
	if deletedReason.Valid {
		e.DeletedReason = deletedReason.String
	}
	if deletedTime.Valid {
		v := deletedTime.Time
		e.DeletedTime = &v
	}
	if createBy.Valid {
		e.CreateBy = createBy.String
	}
	return e, nil
}

// AddEvidence inserts an evidence row for an assertion.
func (s EvidenceStore) AddEvidence(ctx context.Context, e Evidence) (Evidence, error) {
	if s.DB == nil {
		return Evidence{}, errors.New("db is nil")
	}
	if e.AssertionID == 0 {
		return Evidence{}, errors.New("assertion_id is required")
	}
	if strings.TrimSpace(e.ArtifactType) == "" || strings.TrimSpace(e.ArtifactID) == "" {
		return Evidence{}, errors.New("artifact_type and artifact_id are required")
	}
	if e.EvidenceRole == "" {
		e.EvidenceRole = "supports"
	}
	if e.EvidenceRole != "supports" && e.EvidenceRole != "contradicts" {
		return Evidence{}, errors.New("evidence_role must be supports or contradicts")
	}
	if e.ActorKind == "" {
		e.ActorKind = "processor"
	}

	const stmt = `
INSERT INTO kb.assertion_evidence
	(assertion_id, input_record_id, artifact_type, artifact_id,
	 artifact_object_id, evidence_quote, source_line_spans, extraction_run,
	 model, prompt_version, confidence, evidence_role, actor_kind, create_by)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $13, $14)
RETURNING ` + evidenceColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		e.AssertionID, nullableInt64(e.InputRecordID), e.ArtifactType, e.ArtifactID,
		nullableString(e.ArtifactObjectID), nullableString(e.EvidenceQuote), nullableJSON(e.SourceLineSpans),
		nullableString(e.ExtractionRun), nullableString(e.Model), nullableString(e.PromptVersion),
		nullableFloat(e.Confidence), e.EvidenceRole, e.ActorKind, nullableString(e.CreateBy),
	)
	created, err := scanEvidence(row.Scan)
	if err != nil {
		return Evidence{}, err
	}

	// A new supports-evidence row on an unsupported assertion is a
	// restoration (spec §16.3 item 16): return the assertion to accepted and
	// record the transition.
	if created.EvidenceRole == "supports" {
		if _, err := s.restoreIfUnsupported(ctx, created.AssertionID); err != nil {
			return Evidence{}, err
		}
	}
	return created, nil
}

// ListForAssertion returns evidence rows for an assertion, optionally
// including soft-deleted rows.
func (s EvidenceStore) ListForAssertion(ctx context.Context, assertionID int64, includeDeleted bool) ([]Evidence, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	stmt := `SELECT ` + evidenceColumns + "\n" + evidenceFrom + `
WHERE assertion_id = $1
  AND ($2 OR NOT deleted)
ORDER BY id`
	rows, err := s.DB.QueryContext(ctx, stmt, assertionID, includeDeleted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Evidence, 0)
	for rows.Next() {
		e, err := scanEvidence(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// activeSupportCount returns how many non-deleted 'supports' evidence rows
// remain for an assertion.
func (s EvidenceStore) activeSupportCount(ctx context.Context, assertionID int64) (int, error) {
	const stmt = `
SELECT COUNT(*) ` + evidenceFrom + `
WHERE assertion_id = $1 AND evidence_role = 'supports' AND NOT deleted`
	var n int
	if err := s.DB.QueryRowContext(ctx, stmt, assertionID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// DeleteEvidence soft-deletes an evidence row (application-level deletion,
// e.g. cascading from an input-record deletion per existing retention
// policy -- never a DB-level ON DELETE CASCADE, spec §10.12). If this was the
// assertion's last active supporting evidence, the assertion transitions
// accepted -> unsupported and the deleted evidence id is recorded in the
// transition reason.
func (s EvidenceStore) DeleteEvidence(ctx context.Context, id int64, actor, reason string) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	var assertionID int64
	var role string
	const lookup = `SELECT assertion_id, evidence_role ` + evidenceFrom + ` WHERE id = $1 AND NOT deleted`
	if err := s.DB.QueryRowContext(ctx, lookup, id).Scan(&assertionID, &role); err != nil {
		return err
	}

	const del = `
UPDATE kb.assertion_evidence
SET deleted = TRUE, deleted_reason = $2, deleted_time = NOW()
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, del, id, nullableString(reason)); err != nil {
		return err
	}
	if role != "supports" {
		return nil
	}

	remaining, err := s.activeSupportCount(ctx, assertionID)
	if err != nil {
		return err
	}
	if remaining > 0 {
		return nil
	}
	cur, err := s.Assertions.GetByID(ctx, assertionID)
	if err != nil {
		return err
	}
	if cur.Status != StatusAccepted {
		return nil
	}
	transitionReason := "last qualifying evidence removed (evidence id " + strconv.FormatInt(id, 10) + ")"
	if reason != "" {
		transitionReason += ": " + reason
	}
	_, err = s.Assertions.TransitionStatus(ctx, assertionID, StatusUnsupported, transitionReason, actor)
	return err
}

// restoreIfUnsupported returns an unsupported assertion to accepted once it
// has at least one active supporting evidence row again (spec §16.3 item 16).
func (s EvidenceStore) restoreIfUnsupported(ctx context.Context, assertionID int64) (Assertion, error) {
	cur, err := s.Assertions.GetByID(ctx, assertionID)
	if err != nil {
		return Assertion{}, err
	}
	if cur.Status != StatusUnsupported {
		return cur, nil
	}
	return s.Assertions.TransitionStatus(ctx, assertionID, StatusAccepted, "qualifying evidence restored", "")
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
