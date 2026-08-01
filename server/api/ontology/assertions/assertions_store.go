package assertions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AllowedRefKinds mirrors the kb.semantic_assertions CHECK on
// subject_ref_kind/object_ref_kind (ADR DR9).
var AllowedRefKinds = map[string]bool{
	"object_node": true, "ontology_term": true, "assertion": true,
	"artifact": true, "literal": true,
}

// Assertion is one revision of a qualified semantic assertion (spec §8.3).
// LogicalIdentityKey groups revisions of the same claim (spec §10.7); the
// latest revision for a given key is the current state.
type Assertion struct {
	ID                    int64           `json:"id"`
	LogicalIdentityKey    string          `json:"logical_identity_key"`
	Revision              int             `json:"revision"`
	SubjectRefKind        string          `json:"subject_ref_kind"`
	SubjectRefID          string          `json:"subject_ref_id"`
	SubjectObjectID       string          `json:"subject_object_id,omitempty"`
	PredicateTermID       string          `json:"predicate_term_id"`
	ObjectRefKind         string          `json:"object_ref_kind,omitempty"`
	ObjectRefID           string          `json:"object_ref_id,omitempty"`
	ObjectObjectID        string          `json:"object_object_id,omitempty"`
	ObjectLiteral         json.RawMessage `json:"object_literal,omitempty"`
	AssertionKindTermID   string          `json:"assertion_kind_term_id,omitempty"`
	Polarity              string          `json:"polarity"`
	Modality              string          `json:"modality,omitempty"`
	Qualifiers            json.RawMessage `json:"qualifiers,omitempty"`
	Confidence            *float64        `json:"confidence,omitempty"`
	ValueForm              string          `json:"value_form,omitempty"`
	NumericValue            *float64        `json:"numeric_value,omitempty"`
	LowerValue               *float64        `json:"lower_value,omitempty"`
	UpperValue                *float64        `json:"upper_value,omitempty"`
	LowerInclusive             *bool           `json:"lower_inclusive,omitempty"`
	UpperInclusive              *bool           `json:"upper_inclusive,omitempty"`
	Comparator                   string          `json:"comparator,omitempty"`
	UnitTermID                     string          `json:"unit_term_id,omitempty"`
	QuantityKindTermID              string          `json:"quantity_kind_term_id,omitempty"`
	RawText                          string          `json:"raw_text,omitempty"`
	Status                string          `json:"status"`
	DecisionReason        string          `json:"decision_reason,omitempty"`
	DependencyFingerprint string          `json:"dependency_fingerprint,omitempty"`
	SupersededBy          *int64          `json:"superseded_by,omitempty"`
	ValidTimeStart        *time.Time      `json:"valid_time_start,omitempty"`
	ValidTimeEnd          *time.Time      `json:"valid_time_end,omitempty"`
	TransactionTime       time.Time       `json:"transaction_time"`
	CreateTime            time.Time       `json:"create_time"`
	CreateBy              string          `json:"create_by,omitempty"`
	ModifyTime            time.Time       `json:"modify_time"`
	ModifyBy              string          `json:"modify_by,omitempty"`
}

// DBX is the subset of database/sql the store needs; both *sql.DB and *sql.Tx
// satisfy it (the P2 terms-store pattern), so a caller can run the store
// inside its own transaction.
type DBX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// AssertionStore persists assertion revisions and enforces the spec §9.3
// operational state machine.
type AssertionStore struct {
	DB DBX
}

const assertionColumns = `
	id, logical_identity_key, revision, subject_ref_kind, subject_ref_id,
	subject_object_id, predicate_term_id, object_ref_kind, object_ref_id,
	object_object_id, object_literal, assertion_kind_term_id, polarity,
	modality, qualifiers, confidence, value_form, numeric_value, lower_value,
	upper_value, lower_inclusive, upper_inclusive, comparator, unit_term_id,
	quantity_kind_term_id, raw_text, status, decision_reason,
	dependency_fingerprint, superseded_by, valid_time_start, valid_time_end,
	transaction_time, create_time, create_by, modify_time, modify_by`

const assertionFrom = "FROM kb.semantic_assertions"

func scanAssertion(scan func(dest ...any) error) (Assertion, error) {
	var (
		a               Assertion
		subjectObjectID sql.NullString
		objectRefKind   sql.NullString
		objectRefID     sql.NullString
		objectObjectID  sql.NullString
		objectLiteral   json.RawMessage
		assertionKind   sql.NullString
		modality        sql.NullString
		qualifiers      json.RawMessage
		confidence      sql.NullFloat64
		valueForm       sql.NullString
		numericValue    sql.NullFloat64
		lowerValue      sql.NullFloat64
		upperValue      sql.NullFloat64
		lowerInclusive  sql.NullBool
		upperInclusive  sql.NullBool
		comparator      sql.NullString
		unitTermID      sql.NullString
		quantityKindID  sql.NullString
		rawText         sql.NullString
		decisionReason  sql.NullString
		depFingerprint  sql.NullString
		supersededBy    sql.NullInt64
		validTimeStart  sql.NullTime
		validTimeEnd    sql.NullTime
		createBy        sql.NullString
		modifyBy        sql.NullString
	)
	if err := scan(
		&a.ID, &a.LogicalIdentityKey, &a.Revision, &a.SubjectRefKind, &a.SubjectRefID,
		&subjectObjectID, &a.PredicateTermID, &objectRefKind, &objectRefID,
		&objectObjectID, &objectLiteral, &assertionKind, &a.Polarity,
		&modality, &qualifiers, &confidence, &valueForm, &numericValue, &lowerValue,
		&upperValue, &lowerInclusive, &upperInclusive, &comparator, &unitTermID,
		&quantityKindID, &rawText, &a.Status, &decisionReason,
		&depFingerprint, &supersededBy, &validTimeStart, &validTimeEnd,
		&a.TransactionTime, &a.CreateTime, &createBy, &a.ModifyTime, &modifyBy,
	); err != nil {
		return Assertion{}, err
	}
	if subjectObjectID.Valid {
		a.SubjectObjectID = subjectObjectID.String
	}
	if objectRefKind.Valid {
		a.ObjectRefKind = objectRefKind.String
	}
	if objectRefID.Valid {
		a.ObjectRefID = objectRefID.String
	}
	if objectObjectID.Valid {
		a.ObjectObjectID = objectObjectID.String
	}
	if objectLiteral != nil {
		a.ObjectLiteral = objectLiteral
	}
	if assertionKind.Valid {
		a.AssertionKindTermID = assertionKind.String
	}
	if modality.Valid {
		a.Modality = modality.String
	}
	if qualifiers != nil {
		a.Qualifiers = qualifiers
	}
	if confidence.Valid {
		v := confidence.Float64
		a.Confidence = &v
	}
	if valueForm.Valid {
		a.ValueForm = valueForm.String
	}
	if numericValue.Valid {
		v := numericValue.Float64
		a.NumericValue = &v
	}
	if lowerValue.Valid {
		v := lowerValue.Float64
		a.LowerValue = &v
	}
	if upperValue.Valid {
		v := upperValue.Float64
		a.UpperValue = &v
	}
	if lowerInclusive.Valid {
		v := lowerInclusive.Bool
		a.LowerInclusive = &v
	}
	if upperInclusive.Valid {
		v := upperInclusive.Bool
		a.UpperInclusive = &v
	}
	if comparator.Valid {
		a.Comparator = comparator.String
	}
	if unitTermID.Valid {
		a.UnitTermID = unitTermID.String
	}
	if quantityKindID.Valid {
		a.QuantityKindTermID = quantityKindID.String
	}
	if rawText.Valid {
		a.RawText = rawText.String
	}
	if decisionReason.Valid {
		a.DecisionReason = decisionReason.String
	}
	if depFingerprint.Valid {
		a.DependencyFingerprint = depFingerprint.String
	}
	if supersededBy.Valid {
		v := supersededBy.Int64
		a.SupersededBy = &v
	}
	if validTimeStart.Valid {
		v := validTimeStart.Time
		a.ValidTimeStart = &v
	}
	if validTimeEnd.Valid {
		v := validTimeEnd.Time
		a.ValidTimeEnd = &v
	}
	if createBy.Valid {
		a.CreateBy = createBy.String
	}
	if modifyBy.Valid {
		a.ModifyBy = modifyBy.String
	}
	return a, nil
}

func validateAssertion(a Assertion) error {
	if !AllowedRefKinds[a.SubjectRefKind] {
		return fmt.Errorf("unsupported subject_ref_kind %q", a.SubjectRefKind)
	}
	if strings.TrimSpace(a.SubjectRefID) == "" {
		return errors.New("subject_ref_id is required")
	}
	if strings.TrimSpace(a.PredicateTermID) == "" {
		return errors.New("predicate_term_id is required")
	}
	if a.ObjectRefID == "" && len(a.ObjectLiteral) == 0 {
		return errors.New("one of object_ref_id or object_literal is required")
	}
	if a.ObjectRefID != "" && !AllowedRefKinds[a.ObjectRefKind] {
		return fmt.Errorf("unsupported object_ref_kind %q", a.ObjectRefKind)
	}
	if strings.TrimSpace(a.LogicalIdentityKey) == "" {
		return errors.New("logical_identity_key is required")
	}
	if a.Polarity == "" {
		a.Polarity = "positive"
	}
	if a.Polarity != "positive" && a.Polarity != "negative" {
		return fmt.Errorf("unsupported polarity %q", a.Polarity)
	}
	return nil
}

// CreateAssertion inserts revision 1 for a new logical assertion.
func (s AssertionStore) CreateAssertion(ctx context.Context, a Assertion) (Assertion, error) {
	if s.DB == nil {
		return Assertion{}, errors.New("db is nil")
	}
	if err := validateAssertion(a); err != nil {
		return Assertion{}, err
	}
	if a.Polarity == "" {
		a.Polarity = "positive"
	}
	if a.Status == "" {
		a.Status = StatusCandidate
	}
	if !ValidAssertionStatus(a.Status) {
		return Assertion{}, fmt.Errorf("unsupported assertion status %q", a.Status)
	}

	const stmt = `
INSERT INTO kb.semantic_assertions
	(logical_identity_key, revision, subject_ref_kind, subject_ref_id,
	 subject_object_id, predicate_term_id, object_ref_kind, object_ref_id,
	 object_object_id, object_literal, assertion_kind_term_id, polarity,
	 modality, qualifiers, confidence, value_form, numeric_value, lower_value,
	 upper_value, lower_inclusive, upper_inclusive, comparator, unit_term_id,
	 quantity_kind_term_id, raw_text, status, valid_time_start, valid_time_end,
	 create_by, modify_by)
VALUES ($1, 1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13::jsonb,
	$14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29)
RETURNING ` + assertionColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(a.LogicalIdentityKey), a.SubjectRefKind, strings.TrimSpace(a.SubjectRefID),
		nullableString(a.SubjectObjectID), strings.TrimSpace(a.PredicateTermID),
		nullableString(a.ObjectRefKind), nullableString(a.ObjectRefID),
		nullableString(a.ObjectObjectID), nullableJSON(a.ObjectLiteral),
		nullableString(a.AssertionKindTermID), a.Polarity, nullableString(a.Modality),
		nullableJSON(a.Qualifiers), nullableFloat(a.Confidence), nullableString(a.ValueForm),
		nullableFloat(a.NumericValue), nullableFloat(a.LowerValue), nullableFloat(a.UpperValue),
		nullableBool(a.LowerInclusive), nullableBool(a.UpperInclusive), nullableString(a.Comparator),
		nullableString(a.UnitTermID), nullableString(a.QuantityKindTermID), nullableString(a.RawText),
		a.Status, nullableTime(a.ValidTimeStart), nullableTime(a.ValidTimeEnd),
		nullableString(a.CreateBy), nullableString(a.ModifyBy),
	)
	return scanAssertion(row.Scan)
}

// CreateRevision inserts the next revision for an existing logical assertion
// (spec §10.7: a decision-relevant change creates a payload revision and
// supersedes any completed prior revision, never mutating it in place). If
// the current latest revision is accepted, it is marked superseded and
// points at the new revision.
func (s AssertionStore) CreateRevision(ctx context.Context, a Assertion) (Assertion, error) {
	if s.DB == nil {
		return Assertion{}, errors.New("db is nil")
	}
	if err := validateAssertion(a); err != nil {
		return Assertion{}, err
	}
	prior, err := s.GetLatest(ctx, a.LogicalIdentityKey)
	if err != nil {
		return Assertion{}, fmt.Errorf("no prior revision for %q: %w", a.LogicalIdentityKey, err)
	}
	if a.Polarity == "" {
		a.Polarity = "positive"
	}
	if a.Status == "" {
		a.Status = StatusCandidate
	}
	if !ValidAssertionStatus(a.Status) {
		return Assertion{}, fmt.Errorf("unsupported assertion status %q", a.Status)
	}

	const stmt = `
INSERT INTO kb.semantic_assertions
	(logical_identity_key, revision, subject_ref_kind, subject_ref_id,
	 subject_object_id, predicate_term_id, object_ref_kind, object_ref_id,
	 object_object_id, object_literal, assertion_kind_term_id, polarity,
	 modality, qualifiers, confidence, value_form, numeric_value, lower_value,
	 upper_value, lower_inclusive, upper_inclusive, comparator, unit_term_id,
	 quantity_kind_term_id, raw_text, status, valid_time_start, valid_time_end,
	 create_by, modify_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $12, $13, $14::jsonb,
	$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30)
RETURNING ` + assertionColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(a.LogicalIdentityKey), prior.Revision+1, a.SubjectRefKind, strings.TrimSpace(a.SubjectRefID),
		nullableString(a.SubjectObjectID), strings.TrimSpace(a.PredicateTermID),
		nullableString(a.ObjectRefKind), nullableString(a.ObjectRefID),
		nullableString(a.ObjectObjectID), nullableJSON(a.ObjectLiteral),
		nullableString(a.AssertionKindTermID), a.Polarity, nullableString(a.Modality),
		nullableJSON(a.Qualifiers), nullableFloat(a.Confidence), nullableString(a.ValueForm),
		nullableFloat(a.NumericValue), nullableFloat(a.LowerValue), nullableFloat(a.UpperValue),
		nullableBool(a.LowerInclusive), nullableBool(a.UpperInclusive), nullableString(a.Comparator),
		nullableString(a.UnitTermID), nullableString(a.QuantityKindTermID), nullableString(a.RawText),
		a.Status, nullableTime(a.ValidTimeStart), nullableTime(a.ValidTimeEnd),
		nullableString(a.CreateBy), nullableString(a.ModifyBy),
	)
	next, err := scanAssertion(row.Scan)
	if err != nil {
		return Assertion{}, err
	}

	if prior.Status == StatusAccepted {
		const supersede = `
UPDATE kb.semantic_assertions
SET status = $2, superseded_by = $3, modify_time = NOW()
WHERE id = $1`
		if _, err := s.DB.ExecContext(ctx, supersede, prior.ID, StatusSuperseded, next.ID); err != nil {
			return Assertion{}, err
		}
	}
	return next, nil
}

// GetLatest returns the current (highest-revision) row for a logical
// assertion.
func (s AssertionStore) GetLatest(ctx context.Context, logicalIdentityKey string) (Assertion, error) {
	if s.DB == nil {
		return Assertion{}, errors.New("db is nil")
	}
	const stmt = `SELECT ` + assertionColumns + "\n" + assertionFrom + `
WHERE logical_identity_key = $1
ORDER BY revision DESC
LIMIT 1`
	row := s.DB.QueryRowContext(ctx, stmt, strings.TrimSpace(logicalIdentityKey))
	return scanAssertion(row.Scan)
}

// GetByID returns a specific assertion revision by its surrogate id.
func (s AssertionStore) GetByID(ctx context.Context, id int64) (Assertion, error) {
	if s.DB == nil {
		return Assertion{}, errors.New("db is nil")
	}
	const stmt = `SELECT ` + assertionColumns + "\n" + assertionFrom + `
WHERE id = $1`
	row := s.DB.QueryRowContext(ctx, stmt, id)
	return scanAssertion(row.Scan)
}

// ListBySubjectObject returns the latest revision of every assertion whose
// subject is the given object node -- the dominant query shape DR9's fast
// path exists for.
func (s AssertionStore) ListBySubjectObject(ctx context.Context, subjectObjectID, status string) ([]Assertion, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	stmt := `SELECT ` + assertionColumns + "\n" + assertionFrom + `
WHERE subject_object_id = $1
  AND ($2 = '' OR status = $2)
  AND revision = (
	SELECT MAX(revision) FROM kb.semantic_assertions a2
	WHERE a2.logical_identity_key = kb.semantic_assertions.logical_identity_key
  )
ORDER BY id`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(subjectObjectID), strings.TrimSpace(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Assertion, 0)
	for rows.Next() {
		a, err := scanAssertion(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TransitionStatus moves an assertion between allowed operational states
// (spec §9.3).
func (s AssertionStore) TransitionStatus(ctx context.Context, id int64, to, reason, by string) (Assertion, error) {
	if s.DB == nil {
		return Assertion{}, errors.New("db is nil")
	}
	cur, err := s.GetByID(ctx, id)
	if err != nil {
		return Assertion{}, err
	}
	if !transitionAllowed(cur.Status, to) {
		return Assertion{}, fmt.Errorf("%w: %s -> %s", errIllegalAssertionTransition, cur.Status, to)
	}
	const stmt = `
UPDATE kb.semantic_assertions
SET status = $2, decision_reason = $3, modify_time = NOW(), modify_by = $4
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, id, to, nullableString(reason), nullableString(by)); err != nil {
		return Assertion{}, err
	}
	return s.GetByID(ctx, id)
}

// DeferAssertion moves a candidate/in-review assertion to deferred and
// records the dependency fingerprint it is blocked on (spec §9.3, mirroring
// the P2 candidate defer/retry gate for the operational machine).
func (s AssertionStore) DeferAssertion(ctx context.Context, id int64, dependencyFingerprint, reason, by string) (Assertion, error) {
	if s.DB == nil {
		return Assertion{}, errors.New("db is nil")
	}
	if strings.TrimSpace(dependencyFingerprint) == "" {
		return Assertion{}, errors.New("dependency fingerprint is required to defer an assertion")
	}
	cur, err := s.GetByID(ctx, id)
	if err != nil {
		return Assertion{}, err
	}
	if !transitionAllowed(cur.Status, StatusDeferred) {
		return Assertion{}, fmt.Errorf("%w: %s -> %s", errIllegalAssertionTransition, cur.Status, StatusDeferred)
	}
	const stmt = `
UPDATE kb.semantic_assertions
SET status = 'deferred', dependency_fingerprint = $2, decision_reason = $3,
    modify_time = NOW(), modify_by = $4
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, id, strings.TrimSpace(dependencyFingerprint), nullableString(reason), nullableString(by)); err != nil {
		return Assertion{}, err
	}
	return s.GetByID(ctx, id)
}

// errNoRetryWithoutDependencyChange mirrors the P2 candidate gate: a deferred
// assertion is only eligible to retry once its dependency fingerprint has
// actually changed (spec §16.3 item 12).
var errNoRetryWithoutDependencyChange = errors.New("dependency fingerprint unchanged; deferred assertion is not eligible for retry")

// RetryDeferred re-opens a deferred assertion as a candidate, but only when
// the dependency fingerprint has changed.
func (s AssertionStore) RetryDeferred(ctx context.Context, id int64, newDependencyFingerprint, by string) (Assertion, error) {
	if s.DB == nil {
		return Assertion{}, errors.New("db is nil")
	}
	if strings.TrimSpace(newDependencyFingerprint) == "" {
		return Assertion{}, errors.New("new dependency fingerprint is required to retry a deferred assertion")
	}
	cur, err := s.GetByID(ctx, id)
	if err != nil {
		return Assertion{}, err
	}
	if cur.Status != StatusDeferred {
		return Assertion{}, fmt.Errorf("assertion is %s, not deferred", cur.Status)
	}
	if newDependencyFingerprint == cur.DependencyFingerprint {
		return Assertion{}, errNoRetryWithoutDependencyChange
	}
	const stmt = `
UPDATE kb.semantic_assertions
SET status = 'candidate', dependency_fingerprint = $2, decision_reason = NULL,
    modify_time = NOW(), modify_by = $3
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, id, strings.TrimSpace(newDependencyFingerprint), nullableString(by)); err != nil {
		return Assertion{}, err
	}
	return s.GetByID(ctx, id)
}
