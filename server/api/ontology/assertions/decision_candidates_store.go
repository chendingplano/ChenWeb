package assertions

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AllowedDecisionCandidateKinds mirrors the kb.semantic_decision_candidates
// CHECK on candidate_kind (spec §10.2).
var AllowedDecisionCandidateKinds = map[string]bool{
	"referent": true, "term_association": true, "assertion": true,
	"occurrence": true, "profile_selection": true,
}

// AllowedDecisionCandidateMethods mirrors the CHECK on method (spec §10.3).
var AllowedDecisionCandidateMethods = map[string]bool{
	"explicit_structured": true, "deterministic_source_span": true,
	"released_mapping": true, "lexical_candidate": true,
	"semantic_candidate": true, "structural_candidate": true, "human": true,
}

// AllowedResolutionOutcomes mirrors the CHECK on resolution_outcome (spec
// §10.4). Outcomes are recorded in the decision payload, not lifecycle
// statuses -- ambiguity is the deferred status with reason
// 'ambiguous_targets', not a fifth outcome.
var AllowedResolutionOutcomes = map[string]bool{
	"matched": true, "new_target_candidate": true, "rejected": true, "deferred": true,
}

// decisionCandidateTransitions is the spec §9.3 operational machine restricted
// to artifact associations: unlike kb.semantic_assertions, a decision
// candidate never reaches 'unsupported' -- that transition is defined only
// for assertions that have already been persisted with evidence.
var decisionCandidateTransitions = map[string]map[string]bool{
	StatusCandidate:  {StatusInReview: true, StatusRejected: true, StatusDeferred: true},
	StatusInReview:   {StatusAccepted: true, StatusRejected: true, StatusDeferred: true},
	StatusAccepted:   {StatusSuperseded: true},
	StatusDeferred:   {}, // only via RetryDeferred
	StatusRejected:   {},
	StatusSuperseded: {},
}

func decisionCandidateTransitionAllowed(from, to string) bool {
	return decisionCandidateTransitions[from][to]
}

var errIllegalDecisionCandidateTransition = errors.New("illegal semantic decision candidate status transition")

// DecisionCandidate is one proposal for an artifact-to-ontology association
// (spec §10.2-§10.7). LogicalIdentityKey + Revision follows the same
// revision pattern as Assertion; PayloadFingerprint detects whether a
// reprocessing run's payload changed (spec §16.3 item 2).
type DecisionCandidate struct {
	ID                    int64           `json:"id"`
	LogicalIdentityKey    string          `json:"logical_identity_key"`
	Revision              int             `json:"revision"`
	PayloadFingerprint    string          `json:"payload_fingerprint"`
	CandidateKind         string          `json:"candidate_kind"`
	ProposedPayload       json.RawMessage `json:"proposed_payload"`
	Method                string          `json:"method"`
	ResolutionOutcome     string          `json:"resolution_outcome,omitempty"`
	ResolutionReason      string          `json:"resolution_reason,omitempty"`
	SourceArtifactType    string          `json:"source_artifact_type,omitempty"`
	SourceArtifactID      string          `json:"source_artifact_id,omitempty"`
	InputRecordID         *int64          `json:"input_record_id,omitempty"`
	SourceLineSpans       json.RawMessage `json:"source_line_spans,omitempty"`
	Confidence            *float64        `json:"confidence,omitempty"`
	Status                string          `json:"status"`
	DecisionReason        string          `json:"decision_reason,omitempty"`
	DependencyFingerprint string          `json:"dependency_fingerprint,omitempty"`
	SupersededBy          *int64          `json:"superseded_by,omitempty"`
	ResultingAssertionID  *int64          `json:"resulting_assertion_id,omitempty"`
	LastSeen              time.Time       `json:"last_seen"`
	CreateTime            time.Time       `json:"create_time"`
	CreateBy              string          `json:"create_by,omitempty"`
	ModifyTime            time.Time       `json:"modify_time"`
	ModifyBy              string          `json:"modify_by,omitempty"`
	Reused                bool            `json:"reused"`
}

// DecisionCandidateStore persists association candidates and enforces the
// spec §9.3 operational machine (minus the assertion-only 'unsupported' arc).
type DecisionCandidateStore struct {
	DB DBX
}

const decisionCandidateColumns = `
	id, logical_identity_key, revision, payload_fingerprint, candidate_kind,
	proposed_payload, method, resolution_outcome, resolution_reason,
	source_artifact_type, source_artifact_id, input_record_id, source_line_spans, confidence,
	status, decision_reason, dependency_fingerprint, superseded_by,
	resulting_assertion_id, last_seen, create_time, create_by, modify_time, modify_by`

const decisionCandidateFrom = "FROM kb.semantic_decision_candidates"

// PayloadFingerprint returns the deterministic identity of a candidate's
// proposed payload. Two identical reprocessings of the same logical
// association produce the same fingerprint, so an unchanged payload can be
// recognized and only bumps last_seen instead of creating review work.
func PayloadFingerprint(payload []byte) (string, error) {
	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		return "", fmt.Errorf("payload fingerprint: %w", err)
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("payload fingerprint: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func scanDecisionCandidate(scan func(dest ...any) error) (DecisionCandidate, error) {
	var (
		c                  DecisionCandidate
		resolutionOutcome  sql.NullString
		resolutionReason   sql.NullString
		sourceArtifactType sql.NullString
		sourceArtifactID   sql.NullString
		lineSpans          json.RawMessage
		confidence         sql.NullFloat64
		decisionReason     sql.NullString
		depFingerprint     sql.NullString
		supersededBy       sql.NullInt64
		resultingAssertion sql.NullInt64
		inputRecordID      sql.NullInt64
		createBy           sql.NullString
		modifyBy           sql.NullString
	)
	if err := scan(
		&c.ID, &c.LogicalIdentityKey, &c.Revision, &c.PayloadFingerprint, &c.CandidateKind,
		&c.ProposedPayload, &c.Method, &resolutionOutcome, &resolutionReason,
		&sourceArtifactType, &sourceArtifactID, &inputRecordID, &lineSpans, &confidence,
		&c.Status, &decisionReason, &depFingerprint, &supersededBy,
		&resultingAssertion, &c.LastSeen, &c.CreateTime, &createBy, &c.ModifyTime, &modifyBy,
	); err != nil {
		return DecisionCandidate{}, err
	}
	if resolutionOutcome.Valid {
		c.ResolutionOutcome = resolutionOutcome.String
	}
	if resolutionReason.Valid {
		c.ResolutionReason = resolutionReason.String
	}
	if sourceArtifactType.Valid {
		c.SourceArtifactType = sourceArtifactType.String
	}
	if sourceArtifactID.Valid {
		c.SourceArtifactID = sourceArtifactID.String
	}
	if inputRecordID.Valid {
		v := inputRecordID.Int64
		c.InputRecordID = &v
	}
	if lineSpans != nil {
		c.SourceLineSpans = lineSpans
	}
	if confidence.Valid {
		v := confidence.Float64
		c.Confidence = &v
	}
	if decisionReason.Valid {
		c.DecisionReason = decisionReason.String
	}
	if depFingerprint.Valid {
		c.DependencyFingerprint = depFingerprint.String
	}
	if supersededBy.Valid {
		v := supersededBy.Int64
		c.SupersededBy = &v
	}
	if resultingAssertion.Valid {
		v := resultingAssertion.Int64
		c.ResultingAssertionID = &v
	}
	if createBy.Valid {
		c.CreateBy = createBy.String
	}
	if modifyBy.Valid {
		c.ModifyBy = modifyBy.String
	}
	return c, nil
}

func validateDecisionCandidate(c DecisionCandidate) error {
	if !AllowedDecisionCandidateKinds[c.CandidateKind] {
		return fmt.Errorf("unsupported candidate_kind %q", c.CandidateKind)
	}
	if !AllowedDecisionCandidateMethods[c.Method] {
		return fmt.Errorf("unsupported method %q", c.Method)
	}
	if len(c.ProposedPayload) == 0 {
		return errors.New("proposed_payload is required")
	}
	if strings.TrimSpace(c.LogicalIdentityKey) == "" {
		return errors.New("logical_identity_key is required")
	}
	if c.ResolutionOutcome != "" && !AllowedResolutionOutcomes[c.ResolutionOutcome] {
		return fmt.Errorf("unsupported resolution_outcome %q", c.ResolutionOutcome)
	}
	return nil
}

// Propose inserts a new candidate for a logical association, or -- if the
// logical association already has a latest revision with an identical
// payload fingerprint -- bumps that revision's last_seen and returns it with
// Reused=true instead of creating duplicate review work (spec §16.3 item 2).
// A changed payload creates a new revision and supersedes the prior one if
// it had reached a completed (accepted/rejected) status.
func (s DecisionCandidateStore) Propose(ctx context.Context, c DecisionCandidate) (DecisionCandidate, error) {
	if s.DB == nil {
		return DecisionCandidate{}, errors.New("db is nil")
	}
	if err := validateDecisionCandidate(c); err != nil {
		return DecisionCandidate{}, err
	}
	fp, err := PayloadFingerprint(c.ProposedPayload)
	if err != nil {
		return DecisionCandidate{}, err
	}

	prior, err := s.GetLatest(ctx, c.LogicalIdentityKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return DecisionCandidate{}, err
	}
	hasPrior := err == nil

	if hasPrior && prior.PayloadFingerprint == fp {
		if _, err := s.DB.ExecContext(ctx, `
UPDATE kb.semantic_decision_candidates SET last_seen = NOW() WHERE id = $1`, prior.ID); err != nil {
			return DecisionCandidate{}, err
		}
		reused, err := s.GetByID(ctx, prior.ID)
		if err != nil {
			return DecisionCandidate{}, err
		}
		reused.Reused = true
		return reused, nil
	}

	revision := 1
	if hasPrior {
		revision = prior.Revision + 1
	}
	if c.Status == "" {
		c.Status = StatusCandidate
	}

	const stmt = `
INSERT INTO kb.semantic_decision_candidates
	(logical_identity_key, revision, payload_fingerprint, candidate_kind,
	 proposed_payload, method, resolution_outcome, resolution_reason,
	 source_artifact_type, source_artifact_id, input_record_id, source_line_spans, confidence,
	 status, create_by, modify_by)
VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, $11, $12::jsonb, $13, $14, $15, $16)
RETURNING ` + decisionCandidateColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(c.LogicalIdentityKey), revision, fp, c.CandidateKind,
		string(c.ProposedPayload), c.Method, nullableString(c.ResolutionOutcome), nullableString(c.ResolutionReason),
		nullableString(c.SourceArtifactType), nullableString(c.SourceArtifactID), nullableInt64(c.InputRecordID), nullableJSON(c.SourceLineSpans),
		nullableFloat(c.Confidence), c.Status, nullableString(c.CreateBy), nullableString(c.ModifyBy),
	)
	next, err := scanDecisionCandidate(row.Scan)
	if err != nil {
		return DecisionCandidate{}, err
	}

	// A new revision always retires whichever revision was current before it
	// -- not only accepted/rejected ones. Leaving a still-open prior revision
	// (candidate/in_review/deferred) un-superseded would let both revisions
	// sit in a non-terminal status simultaneously, so a later stage
	// (associate_semantics, the backlog drain) processes the same logical
	// association twice under two different row ids. This was found live:
	// the backlog drain re-resolved a referent, which changed the candidate's
	// payload fingerprint, which created revision 2 -- but revision 1 (with
	// the stale, still-unresolved payload) was left at 'candidate' and got
	// reprocessed right alongside it, deferring again on stale data.
	if hasPrior && prior.Status != StatusSuperseded {
		if _, err := s.DB.ExecContext(ctx, `
UPDATE kb.semantic_decision_candidates
SET status = $2, superseded_by = $3, modify_time = NOW()
WHERE id = $1`, prior.ID, StatusSuperseded, next.ID); err != nil {
			return DecisionCandidate{}, err
		}
	}
	return next, nil
}

// GetLatest returns the current (highest-revision) row for a logical
// association. Returns sql.ErrNoRows when no candidate exists yet.
func (s DecisionCandidateStore) GetLatest(ctx context.Context, logicalIdentityKey string) (DecisionCandidate, error) {
	if s.DB == nil {
		return DecisionCandidate{}, errors.New("db is nil")
	}
	const stmt = `SELECT ` + decisionCandidateColumns + "\n" + decisionCandidateFrom + `
WHERE logical_identity_key = $1
ORDER BY revision DESC
LIMIT 1`
	row := s.DB.QueryRowContext(ctx, stmt, strings.TrimSpace(logicalIdentityKey))
	return scanDecisionCandidate(row.Scan)
}

// GetByID returns a specific candidate revision by its surrogate id.
func (s DecisionCandidateStore) GetByID(ctx context.Context, id int64) (DecisionCandidate, error) {
	if s.DB == nil {
		return DecisionCandidate{}, errors.New("db is nil")
	}
	const stmt = `SELECT ` + decisionCandidateColumns + "\n" + decisionCandidateFrom + `
WHERE id = $1`
	row := s.DB.QueryRowContext(ctx, stmt, id)
	return scanDecisionCandidate(row.Scan)
}

// ListByStatus returns candidates in a given status, optionally filtered by
// candidate_kind (empty string = any).
func (s DecisionCandidateStore) ListByStatus(ctx context.Context, status, kind string) ([]DecisionCandidate, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	stmt := `SELECT ` + decisionCandidateColumns + "\n" + decisionCandidateFrom + `
WHERE ($1 = '' OR status = $1)
  AND ($2 = '' OR candidate_kind = $2)
ORDER BY id`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(status), strings.TrimSpace(kind))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]DecisionCandidate, 0)
	for rows.Next() {
		c, err := scanDecisionCandidate(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// TransitionStatus moves a candidate between allowed operational states.
func (s DecisionCandidateStore) TransitionStatus(ctx context.Context, id int64, to, reason, by string) (DecisionCandidate, error) {
	if s.DB == nil {
		return DecisionCandidate{}, errors.New("db is nil")
	}
	cur, err := s.GetByID(ctx, id)
	if err != nil {
		return DecisionCandidate{}, err
	}
	if !decisionCandidateTransitionAllowed(cur.Status, to) {
		return DecisionCandidate{}, fmt.Errorf("%w: %s -> %s", errIllegalDecisionCandidateTransition, cur.Status, to)
	}
	const stmt = `
UPDATE kb.semantic_decision_candidates
SET status = $2, decision_reason = $3, modify_time = NOW(), modify_by = $4
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, id, to, nullableString(reason), nullableString(by)); err != nil {
		return DecisionCandidate{}, err
	}
	return s.GetByID(ctx, id)
}

// SetResolution records a candidate's spec §10.4 resolution outcome. This is
// a payload-only update -- it does not itself transition status; ambiguous
// outcomes are expected to be paired with a TransitionStatus(..., StatusDeferred, "ambiguous_targets", ...) call.
func (s DecisionCandidateStore) SetResolution(ctx context.Context, id int64, outcome, reason string) (DecisionCandidate, error) {
	if s.DB == nil {
		return DecisionCandidate{}, errors.New("db is nil")
	}
	if !AllowedResolutionOutcomes[outcome] {
		return DecisionCandidate{}, fmt.Errorf("unsupported resolution_outcome %q", outcome)
	}
	const stmt = `
UPDATE kb.semantic_decision_candidates
SET resolution_outcome = $2, resolution_reason = $3, modify_time = NOW()
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, id, outcome, nullableString(reason)); err != nil {
		return DecisionCandidate{}, err
	}
	return s.GetByID(ctx, id)
}

// SetResultingAssertion links an accepted candidate to the assertion it
// produced (spec §10.7 persist step).
func (s DecisionCandidateStore) SetResultingAssertion(ctx context.Context, id, assertionID int64) (DecisionCandidate, error) {
	if s.DB == nil {
		return DecisionCandidate{}, errors.New("db is nil")
	}
	const stmt = `
UPDATE kb.semantic_decision_candidates
SET resulting_assertion_id = $2, modify_time = NOW()
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, id, assertionID); err != nil {
		return DecisionCandidate{}, err
	}
	return s.GetByID(ctx, id)
}

// DeferCandidate moves a candidate to deferred and records the dependency
// fingerprint it is blocked on (spec §9.3, §16.3 item 12).
func (s DecisionCandidateStore) DeferCandidate(ctx context.Context, id int64, dependencyFingerprint, reason, by string) (DecisionCandidate, error) {
	if s.DB == nil {
		return DecisionCandidate{}, errors.New("db is nil")
	}
	if strings.TrimSpace(dependencyFingerprint) == "" {
		return DecisionCandidate{}, errors.New("dependency fingerprint is required to defer a candidate")
	}
	cur, err := s.GetByID(ctx, id)
	if err != nil {
		return DecisionCandidate{}, err
	}
	if !decisionCandidateTransitionAllowed(cur.Status, StatusDeferred) {
		return DecisionCandidate{}, fmt.Errorf("%w: %s -> %s", errIllegalDecisionCandidateTransition, cur.Status, StatusDeferred)
	}
	const stmt = `
UPDATE kb.semantic_decision_candidates
SET status = 'deferred', dependency_fingerprint = $2, decision_reason = $3,
    modify_time = NOW(), modify_by = $4
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, id, strings.TrimSpace(dependencyFingerprint), nullableString(reason), nullableString(by)); err != nil {
		return DecisionCandidate{}, err
	}
	return s.GetByID(ctx, id)
}

// RetryDeferred re-opens a deferred candidate, but only when the dependency
// fingerprint has changed (spec §16.3 item 12).
func (s DecisionCandidateStore) RetryDeferred(ctx context.Context, id int64, newDependencyFingerprint, by string) (DecisionCandidate, error) {
	if s.DB == nil {
		return DecisionCandidate{}, errors.New("db is nil")
	}
	if strings.TrimSpace(newDependencyFingerprint) == "" {
		return DecisionCandidate{}, errors.New("new dependency fingerprint is required to retry a deferred candidate")
	}
	cur, err := s.GetByID(ctx, id)
	if err != nil {
		return DecisionCandidate{}, err
	}
	if cur.Status != StatusDeferred {
		return DecisionCandidate{}, fmt.Errorf("candidate is %s, not deferred", cur.Status)
	}
	if newDependencyFingerprint == cur.DependencyFingerprint {
		return DecisionCandidate{}, errNoRetryWithoutDependencyChange
	}
	const stmt = `
UPDATE kb.semantic_decision_candidates
SET status = 'candidate', dependency_fingerprint = $2, decision_reason = NULL,
    modify_time = NOW(), modify_by = $3
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, id, strings.TrimSpace(newDependencyFingerprint), nullableString(by)); err != nil {
		return DecisionCandidate{}, err
	}
	return s.GetByID(ctx, id)
}
