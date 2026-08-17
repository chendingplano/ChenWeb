package semantic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Outcome is one row of kb.semantic_processing_outcomes: what one semantic
// stage determined about one artifact (ADR 2026081801 DR4).
type Outcome struct {
	ID                    int64
	OutcomeKey            string
	InputRecordID         *int64
	ArtifactType          string
	ArtifactID            string
	AssertionID           *int64
	StageTermID           string
	DispositionTermID     string
	ExecutionStatus       ExecutionStatus
	OutcomeCategory       OutcomeCategory
	FindingCount          int
	HighestSeverityTermID string
	RawFragment           string
	DependencyFingerprint string
	InputFingerprint      string
	ProcessorName         string
	ProcessorVersion      string
	ExtractionRun         string
	Model                 string
	PromptVersion         string
	SupersedesOutcomeID   *int64
	Active                bool
	LastSeen              time.Time
	CreateTime            time.Time
	CreateBy              string
}

// Finding is one row of kb.semantic_processing_findings.
type Finding struct {
	ID                    int64
	OutcomeID             int64
	FindingKey            string
	DimensionTermID       string
	FindingTermID         string
	SeverityTermID        string
	RetryStateTermID      string
	ErrorCode             string
	Details               json.RawMessage
	DependencyFingerprint string
	SupersedesFindingID   *int64
	Active                bool
	CreateBy              string
}

// RecordResult reports what Record did, so callers can distinguish idempotent
// replay from real work -- the difference DR11 reports as "new versus reused
// findings" and DR4 requires in order not to re-alert on unchanged replay.
type RecordResult struct {
	Outcome  Outcome
	Findings []Finding
	// Reused is true when an identical envelope already existed and only
	// last_seen advanced.
	Reused bool
	// Superseded is the ID of the outcome this attempt replaced, if any.
	Superseded *int64
}

// ErrNotGoverned is returned when a caller supplies a machine identifier that
// is not an exact governed term.
var ErrNotGoverned = errors.New("value is not a governed semantic identifier")

// OutcomeStore persists outcome envelopes and their typed child findings.
//
// Every write goes through Record, which is atomic by construction: DR4
// requires that superseding an envelope and deactivating its children happen
// in one transaction, and the deferred constraint triggers added by migration
// 20260818000002 reject any commit that violates it.
type OutcomeStore struct {
	DB *sql.DB
}

// Record is the single entry point for writing an attempt's outcome.
//
// Behavior, in DR4's terms:
//
//   - identical replay (same outcome_key, input_fingerprint, and dependency
//     fingerprint) advances last_seen and returns Reused; it does not append a
//     row and does not re-alert;
//   - a changed input or dependency inserts a new row, marks the former active
//     row superseded, and deactivates that row's children -- all in one
//     transaction;
//   - finding_count and highest_severity are derived from the child set that
//     is written, never supplied by the caller.
func (s OutcomeStore) Record(ctx context.Context, out Outcome, findings []Finding) (RecordResult, error) {
	if s.DB == nil {
		return RecordResult{}, fmt.Errorf("db is nil")
	}
	if err := validateOutcome(out, findings); err != nil {
		return RecordResult{}, err
	}

	// The caller never sets these: DR4 makes them transactionally derived from
	// the current child set so a report can trust them without scanning
	// children.
	summary := NewFindingSummary(findings)
	if err := summary.Validate(); err != nil {
		return RecordResult{}, fmt.Errorf("finding summary: %w", err)
	}
	out.FindingCount = summary.Count
	out.HighestSeverityTermID = summary.HighestSeverity

	// The envelope fingerprint aggregates the stage's own dependency with its
	// children's, so a child-level change necessarily supersedes the envelope.
	childFingerprints := make([]string, 0, len(findings))
	for _, f := range findings {
		childFingerprints = append(childFingerprints, f.DependencyFingerprint)
	}
	out.DependencyFingerprint = AggregateFingerprint(out.DependencyFingerprint, childFingerprints)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RecordResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the scope before reading it. Without this, two workers racing on
	// the same occurrence could both read "no active row" and both insert; the
	// partial unique index would reject one, but the loser would have already
	// done its child writes. Locking first makes the loser wait and then take
	// the idempotent-replay path.
	existing, err := lockActiveOutcome(ctx, tx, out.OutcomeKey)
	if err != nil {
		return RecordResult{}, err
	}

	if existing != nil &&
		existing.InputFingerprint == out.InputFingerprint &&
		existing.DependencyFingerprint == out.DependencyFingerprint {
		// Unchanged replay: advance last_seen only (DR4).
		if _, err := tx.ExecContext(ctx,
			`UPDATE kb.semantic_processing_outcomes SET last_seen = NOW() WHERE id = $1`,
			existing.ID); err != nil {
			return RecordResult{}, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE kb.semantic_processing_findings SET last_seen = NOW() WHERE outcome_id = $1 AND active = true`,
			existing.ID); err != nil {
			return RecordResult{}, err
		}
		current, err := loadFindings(ctx, tx, existing.ID)
		if err != nil {
			return RecordResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return RecordResult{}, err
		}
		return RecordResult{Outcome: *existing, Findings: current, Reused: true}, nil
	}

	var supersededID *int64
	if existing != nil {
		// Children first, then the parent: either order commits, because both
		// triggers are DEFERRABLE INITIALLY DEFERRED, but doing children first
		// keeps the intermediate state legal for anything that checks
		// immediately.
		if _, err := tx.ExecContext(ctx,
			`UPDATE kb.semantic_processing_findings SET active = false WHERE outcome_id = $1 AND active = true`,
			existing.ID); err != nil {
			return RecordResult{}, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE kb.semantic_processing_outcomes SET active = false WHERE id = $1`,
			existing.ID); err != nil {
			return RecordResult{}, err
		}
		id := existing.ID
		supersededID = &id
		out.SupersedesOutcomeID = &id
	}

	inserted, err := insertOutcome(ctx, tx, out)
	if err != nil {
		return RecordResult{}, err
	}
	written := make([]Finding, 0, len(findings))
	for _, f := range findings {
		f.OutcomeID = inserted.ID
		w, err := insertFinding(ctx, tx, f)
		if err != nil {
			return RecordResult{}, err
		}
		written = append(written, w)
	}
	if err := tx.Commit(); err != nil {
		return RecordResult{}, err
	}
	return RecordResult{Outcome: inserted, Findings: written, Superseded: supersededID}, nil
}

// validateOutcome enforces the invariants a caller can get wrong, before the
// database does, so the error names the ADR rule rather than a constraint.
func validateOutcome(out Outcome, findings []Finding) error {
	if out.OutcomeKey == "" || out.StageTermID == "" || out.ArtifactType == "" {
		return fmt.Errorf("outcome_key, stage_term_id, and artifact_type are required")
	}
	if out.InputFingerprint == "" || out.DependencyFingerprint == "" {
		return fmt.Errorf("input and dependency fingerprints are required (DR4)")
	}
	if err := ValidateGovernedIdentifier(out.StageTermID); err != nil {
		return fmt.Errorf("stage term: %w", err)
	}
	if out.DispositionTermID != "" {
		if err := ValidateGovernedIdentifier(out.DispositionTermID); err != nil {
			return fmt.Errorf("disposition term: %w", err)
		}
	}
	// DR4: a completed semantic-stage outcome always identifies its artifact.
	// A null artifact is permitted only for a failed invocation-level outcome
	// classified source_or_output_unrecoverable.
	if out.ArtifactID == "" &&
		!(out.ExecutionStatus == ExecutionFailed && out.OutcomeCategory == CategorySourceOrOutputUnrecoverable) {
		return fmt.Errorf("artifact_id is required unless the outcome is a failed source_or_output_unrecoverable invocation (DR4)")
	}
	if out.ExecutionStatus != ExecutionCompleted && out.ExecutionStatus != ExecutionFailed {
		return fmt.Errorf("execution status %q is not binary completed/failed (DR3)", out.ExecutionStatus)
	}
	if want := ExecutionStatusFor(out.OutcomeCategory); want != out.ExecutionStatus {
		return fmt.Errorf("category %q requires execution status %q, got %q (DR3)",
			out.OutcomeCategory, want, out.ExecutionStatus)
	}
	seen := make(map[string]bool, len(findings))
	for _, f := range findings {
		if f.FindingKey == "" {
			return fmt.Errorf("finding_key is required")
		}
		if seen[f.FindingKey] {
			return fmt.Errorf("duplicate finding_key %q in one outcome: findings are keyed within the stage's declared decision scopes (DR4)", f.FindingKey)
		}
		seen[f.FindingKey] = true
		for label, term := range map[string]string{
			"dimension": f.DimensionTermID,
			"finding":   f.FindingTermID,
			"severity":  f.SeverityTermID,
		} {
			if err := ValidateGovernedIdentifier(term); err != nil {
				return fmt.Errorf("%s term: %w", label, err)
			}
		}
		if f.RetryStateTermID != "" {
			if err := ValidateGovernedIdentifier(f.RetryStateTermID); err != nil {
				return fmt.Errorf("retry state term: %w", err)
			}
		}
		if f.DependencyFingerprint == "" {
			return fmt.Errorf("finding %q has no dependency fingerprint: retry could never target it (DR10)", f.FindingKey)
		}
	}
	return nil
}

const outcomeColumns = `id, outcome_key, input_record_id, artifact_type, artifact_id, assertion_id,
	stage_term_id, disposition_term_id, execution_status, outcome_category, finding_count,
	highest_severity_term_id, raw_fragment, dependency_fingerprint, input_fingerprint,
	processor_name, processor_version, extraction_run, model, prompt_version,
	supersedes_outcome_id, active, last_seen, create_time, create_by`

func lockActiveOutcome(ctx context.Context, tx *sql.Tx, outcomeKey string) (*Outcome, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT `+outcomeColumns+` FROM kb.semantic_processing_outcomes
		 WHERE outcome_key = $1 AND active = true FOR UPDATE`, outcomeKey)
	out, err := scanOutcome(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, err
	}
	return &out, nil
}

func insertOutcome(ctx context.Context, tx *sql.Tx, out Outcome) (Outcome, error) {
	row := tx.QueryRowContext(ctx, `
INSERT INTO kb.semantic_processing_outcomes (
  outcome_key, input_record_id, artifact_type, artifact_id, assertion_id,
  stage_term_id, disposition_term_id, execution_status, outcome_category, finding_count,
  highest_severity_term_id, raw_fragment, dependency_fingerprint, input_fingerprint,
  processor_name, processor_version, extraction_run, model, prompt_version,
  supersedes_outcome_id, active, create_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,true,$21)
RETURNING `+outcomeColumns,
		out.OutcomeKey, out.InputRecordID, out.ArtifactType, nullIfEmpty(out.ArtifactID), out.AssertionID,
		out.StageTermID, nullIfEmpty(out.DispositionTermID), string(out.ExecutionStatus), string(out.OutcomeCategory),
		out.FindingCount, nullIfEmpty(out.HighestSeverityTermID), nullIfEmpty(out.RawFragment),
		out.DependencyFingerprint, out.InputFingerprint,
		nullIfEmpty(out.ProcessorName), nullIfEmpty(out.ProcessorVersion), nullIfEmpty(out.ExtractionRun),
		nullIfEmpty(out.Model), nullIfEmpty(out.PromptVersion), out.SupersedesOutcomeID, nullIfEmpty(out.CreateBy))
	return scanOutcome(row)
}

func insertFinding(ctx context.Context, tx *sql.Tx, f Finding) (Finding, error) {
	var details any
	if len(f.Details) > 0 {
		details = []byte(f.Details)
	}
	row := tx.QueryRowContext(ctx, `
INSERT INTO kb.semantic_processing_findings (
  outcome_id, finding_key, dimension_term_id, finding_term_id, severity_term_id,
  retry_state_term_id, error_code, details, dependency_fingerprint, supersedes_finding_id, active, create_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true,$11)
RETURNING id, outcome_id, finding_key, dimension_term_id, finding_term_id, severity_term_id,
          coalesce(retry_state_term_id,''), coalesce(error_code,''), details,
          dependency_fingerprint, supersedes_finding_id, active`,
		f.OutcomeID, f.FindingKey, f.DimensionTermID, f.FindingTermID, f.SeverityTermID,
		nullIfEmpty(f.RetryStateTermID), nullIfEmpty(f.ErrorCode), details,
		f.DependencyFingerprint, f.SupersedesFindingID, nullIfEmpty(f.CreateByOrDefault()))
	return scanFinding(row)
}

// CreateByOrDefault names the writer of a finding; findings inherit their
// outcome's writer when the caller does not set one.
func (f Finding) CreateByOrDefault() string {
	if f.CreateBy != "" {
		return f.CreateBy
	}
	return "semantic_outcome_store"
}

func loadFindings(ctx context.Context, tx *sql.Tx, outcomeID int64) ([]Finding, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, outcome_id, finding_key, dimension_term_id, finding_term_id, severity_term_id,
       coalesce(retry_state_term_id,''), coalesce(error_code,''), details,
       dependency_fingerprint, supersedes_finding_id, active
FROM kb.semantic_processing_findings
WHERE outcome_id = $1 AND active = true ORDER BY id`, outcomeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ActiveOutcome returns the current outcome for a scope, or sql.ErrNoRows.
func (s OutcomeStore) ActiveOutcome(ctx context.Context, outcomeKey string) (Outcome, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT `+outcomeColumns+` FROM kb.semantic_processing_outcomes
		 WHERE outcome_key = $1 AND active = true`, outcomeKey)
	return scanOutcome(row)
}

// ActiveFindings returns the current findings under an outcome.
func (s OutcomeStore) ActiveFindings(ctx context.Context, outcomeID int64) ([]Finding, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, outcome_id, finding_key, dimension_term_id, finding_term_id, severity_term_id,
       coalesce(retry_state_term_id,''), coalesce(error_code,''), details,
       dependency_fingerprint, supersedes_finding_id, active
FROM kb.semantic_processing_findings
WHERE outcome_id = $1 AND active = true ORDER BY id`, outcomeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOutcome(row rowScanner) (Outcome, error) {
	var o Outcome
	var artifactID, disposition, severity, rawFragment sql.NullString
	var procName, procVersion, extractionRun, model, promptVersion, createBy sql.NullString
	var execStatus, category string
	if err := row.Scan(&o.ID, &o.OutcomeKey, &o.InputRecordID, &o.ArtifactType, &artifactID, &o.AssertionID,
		&o.StageTermID, &disposition, &execStatus, &category, &o.FindingCount,
		&severity, &rawFragment, &o.DependencyFingerprint, &o.InputFingerprint,
		&procName, &procVersion, &extractionRun, &model, &promptVersion,
		&o.SupersedesOutcomeID, &o.Active, &o.LastSeen, &o.CreateTime, &createBy); err != nil {
		return Outcome{}, err
	}
	o.ArtifactID = artifactID.String
	o.DispositionTermID = disposition.String
	o.ExecutionStatus = ExecutionStatus(execStatus)
	o.OutcomeCategory = OutcomeCategory(category)
	o.HighestSeverityTermID = severity.String
	o.RawFragment = rawFragment.String
	o.ProcessorName = procName.String
	o.ProcessorVersion = procVersion.String
	o.ExtractionRun = extractionRun.String
	o.Model = model.String
	o.PromptVersion = promptVersion.String
	o.CreateBy = createBy.String
	return o, nil
}

func scanFinding(row rowScanner) (Finding, error) {
	var f Finding
	var details []byte
	if err := row.Scan(&f.ID, &f.OutcomeID, &f.FindingKey, &f.DimensionTermID, &f.FindingTermID,
		&f.SeverityTermID, &f.RetryStateTermID, &f.ErrorCode, &details,
		&f.DependencyFingerprint, &f.SupersedesFindingID, &f.Active); err != nil {
		return Finding{}, err
	}
	f.Details = json.RawMessage(details)
	return f, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
