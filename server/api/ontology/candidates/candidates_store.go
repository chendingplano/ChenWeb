package candidates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Candidate is one proposal for governed ontology content (spec §9.3).
// ProposedPayload is the candidate-specific payload (a term, label, mapping,
// or axiom proposal); Fingerprint is its deterministic identity and the
// dedup key. Reused is populated only by CreateCandidate to report that an
// identical proposal already exists.
type Candidate struct {
	ID                    int64             `json:"id"`
	CandidateKind         string            `json:"candidate_kind"`
	ProposedPayload       json.RawMessage   `json:"proposed_payload"`
	ProposedModuleID      string            `json:"proposed_module_id"`
	SourceType            string            `json:"source_type"`
	SourceRef             string            `json:"source_ref"`
	SourceLineSpans       json.RawMessage   `json:"source_line_spans"`
	DiscoveryMethod       string            `json:"discovery_method"`
	Confidence            *float64          `json:"confidence"`
	Fingerprint           string            `json:"fingerprint"`
	CandidateMatches      json.RawMessage   `json:"candidate_matches"`
	Status                string            `json:"status"`
	DecisionReason        string            `json:"decision_reason"`
	DependencyFingerprint string            `json:"dependency_fingerprint"`
	ProposedBy            string            `json:"proposed_by"`
	CreateTime            time.Time         `json:"create_time"`
	CreateBy              string            `json:"create_by"`
	ModifyTime            time.Time         `json:"modify_time"`
	ModifyBy              string            `json:"modify_by"`
	Reused                bool              `json:"reused"`
}

// AllowedCandidateKinds mirrors the schema CHECK on kb.ontology_candidates.
var AllowedCandidateKinds = map[string]bool{
	"term": true, "label": true, "mapping": true, "axiom": true,
	"profile": true, "profile_rule": true, "module_change": true,
}

// CandidateStore persists candidate rows and enforces the spec §9.3 state
// machine.
type CandidateStore struct {
	DB *sql.DB
}

const candidateColumns = `
	id, candidate_kind, proposed_payload, proposed_module_id, source_type,
	source_ref, source_line_spans, discovery_method, confidence, fingerprint,
	candidate_matches, status, decision_reason, dependency_fingerprint,
	proposed_by, create_time, create_by, modify_time, modify_by`

const candidateFrom = "FROM kb.ontology_candidates"

func scanCandidate(scan func(dest ...any) error) (Candidate, error) {
	var (
		c                Candidate
		moduleID         sql.NullString
		sourceRef        sql.NullString
		lineSpans        json.RawMessage
		discoveryMethod  sql.NullString
		confidence       sql.NullFloat64
		matches          json.RawMessage
		decisionReason   sql.NullString
		depFingerprint   sql.NullString
		proposedBy       sql.NullString
		createBy         sql.NullString
		modifyBy         sql.NullString
	)
	if err := scan(
		&c.ID, &c.CandidateKind, &c.ProposedPayload, &moduleID, &c.SourceType,
		&sourceRef, &lineSpans, &discoveryMethod, &confidence, &c.Fingerprint,
		&matches, &c.Status, &decisionReason, &depFingerprint,
		&proposedBy, &c.CreateTime, &createBy, &c.ModifyTime, &modifyBy,
	); err != nil {
		return Candidate{}, err
	}
	if moduleID.Valid {
		c.ProposedModuleID = moduleID.String
	}
	if sourceRef.Valid {
		c.SourceRef = sourceRef.String
	}
	if lineSpans != nil {
		c.SourceLineSpans = lineSpans
	}
	if discoveryMethod.Valid {
		c.DiscoveryMethod = discoveryMethod.String
	}
	if confidence.Valid {
		v := confidence.Float64
		c.Confidence = &v
	}
	if matches != nil {
		c.CandidateMatches = matches
	}
	if decisionReason.Valid {
		c.DecisionReason = decisionReason.String
	}
	if depFingerprint.Valid {
		c.DependencyFingerprint = depFingerprint.String
	}
	if proposedBy.Valid {
		c.ProposedBy = proposedBy.String
	}
	if createBy.Valid {
		c.CreateBy = createBy.String
	}
	if modifyBy.Valid {
		c.ModifyBy = modifyBy.String
	}
	return c, nil
}

func validateCandidate(c Candidate) error {
	if !AllowedCandidateKinds[c.CandidateKind] {
		return fmt.Errorf("unsupported candidate_kind %q", c.CandidateKind)
	}
	if len(c.ProposedPayload) == 0 {
		return errors.New("proposed_payload is required")
	}
	if _, err := canonicalJSON(c.ProposedPayload); err != nil {
		return fmt.Errorf("proposed_payload is not valid JSON: %w", err)
	}
	if c.SourceType == "" {
		c.SourceType = "manual"
	}
	return nil
}

// CreateCandidate inserts a new candidate, computing its fingerprint from the
// normalized payload, source, and intended module. If an identical proposal
// already exists (same fingerprint), the existing row is returned with
// Reused=true instead of creating duplicate review work (spec §16.3 item 1).
func (s CandidateStore) CreateCandidate(ctx context.Context, c Candidate) (Candidate, error) {
	if s.DB == nil {
		return Candidate{}, errors.New("db is nil")
	}
	if err := validateCandidate(c); err != nil {
		return Candidate{}, err
	}
	fp, err := Fingerprint(c.ProposedPayload, c.SourceType, c.SourceRef, c.ProposedModuleID)
	if err != nil {
		return Candidate{}, err
	}
	if c.Status == "" {
		c.Status = StatusDiscovered
	}
	if !ValidateCandidateStatus(c.Status) {
		return Candidate{}, fmt.Errorf("unsupported candidate status %q", c.Status)
	}
	if c.SourceType == "" {
		c.SourceType = "manual"
	}

	payload := c.ProposedPayload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	lineSpans := c.SourceLineSpans
	if len(lineSpans) == 0 {
		lineSpans = []byte("null")
	}
	matches := c.CandidateMatches
	if len(matches) == 0 {
		matches = []byte("null")
	}

	const stmt = `
INSERT INTO kb.ontology_candidates
	(candidate_kind, proposed_payload, proposed_module_id, source_type,
	 source_ref, source_line_spans, discovery_method, confidence, fingerprint,
	 candidate_matches, status, proposed_by, create_by, modify_by)
VALUES ($1, $2::jsonb, $3, $4, $5, $6::jsonb, $7, $8, $9, $10::jsonb,
	$11, $12, $13, $14)
ON CONFLICT (fingerprint) DO NOTHING
RETURNING ` + candidateColumns

	row := s.DB.QueryRowContext(ctx, stmt,
		c.CandidateKind, string(payload), nullableString(c.ProposedModuleID), c.SourceType,
		nullableString(c.SourceRef), string(lineSpans), nullableString(c.DiscoveryMethod),
		nullableFloat(c.Confidence), fp, string(matches), c.Status,
		nullableString(c.ProposedBy), nullableString(c.CreateBy), nullableString(c.ModifyBy),
	)
	created, err := scanCandidate(row.Scan)
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Candidate{}, err
	}

	// Fingerprint already exists: return the existing candidate and report the
	// reuse so callers can record last_seen without duplicating review work.
	existing, err := s.byFingerprint(ctx, fp)
	if err != nil {
		return Candidate{}, err
	}
	existing.Reused = true
	return existing, nil
}

func (s CandidateStore) byFingerprint(ctx context.Context, fingerprint string) (Candidate, error) {
	const stmt = `SELECT ` + candidateColumns + `
` + candidateFrom + `
WHERE fingerprint = $1`
	row := s.DB.QueryRowContext(ctx, stmt, fingerprint)
	return scanCandidate(row.Scan)
}

// GetCandidate returns a candidate by id.
func (s CandidateStore) GetCandidate(ctx context.Context, id int64) (Candidate, error) {
	if s.DB == nil {
		return Candidate{}, errors.New("db is nil")
	}
	const stmt = `SELECT ` + candidateColumns + `
` + candidateFrom + `
WHERE id = $1`
	row := s.DB.QueryRowContext(ctx, stmt, id)
	return scanCandidate(row.Scan)
}

// ListCandidates filters by optional status, candidate_kind, and module.
func (s CandidateStore) ListCandidates(ctx context.Context, status, kind, moduleID string) ([]Candidate, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	stmt := `SELECT ` + candidateColumns + `
` + candidateFrom + `
WHERE ($1 = '' OR status = $1)
  AND ($2 = '' OR candidate_kind = $2)
  AND ($3 = '' OR proposed_module_id = $3)
ORDER BY id`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(status), strings.TrimSpace(kind), strings.TrimSpace(moduleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Candidate, 0)
	for rows.Next() {
		c, err := scanCandidate(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// TransitionStatus moves a candidate between governed states (spec §9.3).
// Rejected and superseded are terminal; a rejected proposal may be
// reconsidered only as a new candidate revision. in_review freezes the
// payload, so UpdatePayload is refused from in_review onward.
func (s CandidateStore) TransitionStatus(ctx context.Context, id int64, to, by string) (Candidate, error) {
	if s.DB == nil {
		return Candidate{}, errors.New("db is nil")
	}
	cur, err := s.GetCandidate(ctx, id)
	if err != nil {
		return Candidate{}, err
	}
	if to == StatusIncludedInRelease {
		// included_in_release is release-owned: only the module release path
		// (which includes the candidate's promoted content) may set it, so
		// approved candidates remain inactive until included in a release
		// (spec §16.3 item 3).
		return Candidate{}, errors.New("included_in_release is set only by the module release path")
	}
	if !transitionAllowed(cur.Status, to) {
		return Candidate{}, fmt.Errorf("candidate status transition %s -> %s is not allowed", cur.Status, to)
	}
	return s.applyTransition(ctx, id, cur.Status, to, "", by)
}

// DeferCandidate moves an editable candidate to deferred and records the
// dependency fingerprint it is blocked on, so that a retry (RetryDeferred) is
// only eligible once that fingerprint changes (spec §9.3, §16.3 item 12).
func (s CandidateStore) DeferCandidate(ctx context.Context, id int64, dependencyFingerprint, reason, by string) (Candidate, error) {
	if s.DB == nil {
		return Candidate{}, errors.New("db is nil")
	}
	if strings.TrimSpace(dependencyFingerprint) == "" {
		return Candidate{}, errors.New("dependency fingerprint is required to defer a candidate")
	}
	cur, err := s.GetCandidate(ctx, id)
	if err != nil {
		return Candidate{}, err
	}
	if !transitionAllowed(cur.Status, StatusDeferred) {
		return Candidate{}, fmt.Errorf("candidate status transition %s -> %s is not allowed", cur.Status, StatusDeferred)
	}
	const stmt = `
UPDATE kb.ontology_candidates
SET status = 'deferred', dependency_fingerprint = $2, decision_reason = $3,
    modify_time = NOW(), modify_by = $4
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, id, strings.TrimSpace(dependencyFingerprint), nullableString(reason), nullableString(by)); err != nil {
		return Candidate{}, err
	}
	return s.GetCandidate(ctx, id)
}

// RetryDeferred re-opens a deferred candidate as draft, but only when the
// dependency fingerprint has actually changed (spec §9.3: deferred candidates
// retry only after dependency changes; spec §16.3 item 12).
func (s CandidateStore) RetryDeferred(ctx context.Context, id int64, newDependencyFingerprint, by string) (Candidate, error) {
	if s.DB == nil {
		return Candidate{}, errors.New("db is nil")
	}
	if strings.TrimSpace(newDependencyFingerprint) == "" {
		return Candidate{}, errors.New("new dependency fingerprint is required to retry a deferred candidate")
	}
	cur, err := s.GetCandidate(ctx, id)
	if err != nil {
		return Candidate{}, err
	}
	if cur.Status != StatusDeferred {
		return Candidate{}, fmt.Errorf("candidate is %s, not deferred", cur.Status)
	}
	if newDependencyFingerprint == cur.DependencyFingerprint {
		return Candidate{}, errNoRetryWithoutDependencyChange
	}
	return s.applyTransition(ctx, id, cur.Status, StatusDraft, newDependencyFingerprint, by)
}

func (s CandidateStore) applyTransition(ctx context.Context, id int64, from, to, newDepFingerprint, by string) (Candidate, error) {
	stmt := `
UPDATE kb.ontology_candidates
SET status = $2, modify_time = NOW(), modify_by = $3`
	args := []any{id, to, nullableString(by)}
	if to == StatusDraft && from == StatusDeferred {
		stmt += `, dependency_fingerprint = $4, decision_reason = NULL`
		args = append(args, newDepFingerprint)
	}
	stmt += ` WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, args...); err != nil {
		return Candidate{}, err
	}
	return s.GetCandidate(ctx, id)
}

// UpdatePayload replaces a candidate's proposed payload. This is only allowed
// while the candidate is still editable (discovered or draft); in_review
// freezes the reviewed payload, and later edits require a new candidate
// revision (spec §9.3).
func (s CandidateStore) UpdatePayload(ctx context.Context, id int64, newPayload []byte, by string) (Candidate, error) {
	if s.DB == nil {
		return Candidate{}, errors.New("db is nil")
	}
	if _, err := canonicalJSON(newPayload); err != nil {
		return Candidate{}, fmt.Errorf("proposed_payload is not valid JSON: %w", err)
	}
	cur, err := s.GetCandidate(ctx, id)
	if err != nil {
		return Candidate{}, err
	}
	if cur.Status != StatusDiscovered && cur.Status != StatusDraft {
		return Candidate{}, fmt.Errorf("candidate payload is frozen in status %s", cur.Status)
	}
	const stmt = `
UPDATE kb.ontology_candidates
SET proposed_payload = $2::jsonb, modify_time = NOW(), modify_by = $3
WHERE id = $1`
	if _, err := s.DB.ExecContext(ctx, stmt, id, string(newPayload), nullableString(by)); err != nil {
		return Candidate{}, err
	}
	return s.GetCandidate(ctx, id)
}
