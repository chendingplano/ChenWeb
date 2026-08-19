package semantic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Materialization states for kb.unresolved_semantic_occurrences.
const (
	MaterializationPending       = "pending"
	MaterializationClaimed       = "claimed"
	MaterializationMaterializing = "materializing"
	MaterializationMaterialized  = "materialized"
	MaterializationAbandoned     = "abandoned"
)

// UnresolvedOccurrence is DR13's generic option-3 fallback row: an identified
// artifact that a family cannot yet turn into a semantic assertion, preserved
// durably and discoverably instead of being skipped.
type UnresolvedOccurrence struct {
	ID                     int64
	OccurrenceKey          string
	InputRecordID          *int64
	ArtifactType           string
	ArtifactID             string
	SourceRevision         string
	RawPayload             json.RawMessage
	Provenance             json.RawMessage
	MaterializationState   string
	ResultingAssertionID   *int64
	CurrentOutcomeID       *int64
	SupersedesOccurrenceID *int64
	InputFingerprint       string
	DependencyFingerprint  string
	Active                 bool
	LeaseToken             string
	LeaseExpiresAt         *time.Time
	SagaIdempotencyKey     string
	SagaCompletedAt        *time.Time
	LastSeen               time.Time
	CreateBy               string
}

// OccurrenceStore persists and materializes unresolved semantic occurrences.
type OccurrenceStore struct {
	DB *sql.DB
}

// Upsert records an unresolved occurrence following DR13's idempotency rules:
// an identical replay advances last_seen; changed input or dependencies create
// a superseding row and deactivate the former one transactionally.
//
// It refuses an occurrence with no artifact_id. DR13 is explicit that the
// fallback applies only to an identified artifact -- when nothing can be
// identified, DR3 classifies the attempt source_or_output_unrecoverable and no
// occurrence row is fabricated. Fabricating one would create a durable promise
// to materialize something that cannot be named.
func (s OccurrenceStore) Upsert(ctx context.Context, occ UnresolvedOccurrence) (UnresolvedOccurrence, bool, error) {
	if s.DB == nil {
		return UnresolvedOccurrence{}, false, fmt.Errorf("db is nil")
	}
	if occ.ArtifactID == "" {
		return UnresolvedOccurrence{}, false, fmt.Errorf(
			"unresolved occurrence requires an identified artifact: an unidentifiable invocation is source_or_output_unrecoverable and creates no occurrence (DR13)")
	}
	if occ.OccurrenceKey == "" || occ.ArtifactType == "" {
		return UnresolvedOccurrence{}, false, fmt.Errorf("occurrence_key and artifact_type are required")
	}
	if occ.InputFingerprint == "" || occ.DependencyFingerprint == "" {
		return UnresolvedOccurrence{}, false, fmt.Errorf("input and dependency fingerprints are required (DR13)")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return UnresolvedOccurrence{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := lockActiveOccurrence(ctx, tx, occ.OccurrenceKey)
	if err != nil {
		return UnresolvedOccurrence{}, false, err
	}
	if existing != nil &&
		existing.InputFingerprint == occ.InputFingerprint &&
		existing.DependencyFingerprint == occ.DependencyFingerprint {
		if _, err := tx.ExecContext(ctx,
			`UPDATE kb.unresolved_semantic_occurrences SET last_seen = NOW() WHERE id = $1`,
			existing.ID); err != nil {
			return UnresolvedOccurrence{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return UnresolvedOccurrence{}, false, err
		}
		return *existing, true, nil
	}
	if existing != nil {
		if _, err := tx.ExecContext(ctx,
			`UPDATE kb.unresolved_semantic_occurrences SET active = false WHERE id = $1`,
			existing.ID); err != nil {
			return UnresolvedOccurrence{}, false, err
		}
		id := existing.ID
		occ.SupersedesOccurrenceID = &id
	}
	inserted, err := insertOccurrence(ctx, tx, occ)
	if err != nil {
		return UnresolvedOccurrence{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return UnresolvedOccurrence{}, false, err
	}
	return inserted, false, nil
}

// Claim takes an expiring lease on one claimable occurrence for a family, using
// FOR UPDATE SKIP LOCKED so concurrent workers never contend on the same row
// (DR13). A row whose lease has expired is claimable again, which is what makes
// a crashed worker recoverable without leaving duplicate active state.
func (s OccurrenceStore) Claim(ctx context.Context, artifactType, leaseToken string, leaseFor time.Duration) (UnresolvedOccurrence, error) {
	if leaseToken == "" {
		return UnresolvedOccurrence{}, fmt.Errorf("lease token is required")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return UnresolvedOccurrence{}, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
SELECT `+occurrenceColumns+`
FROM kb.unresolved_semantic_occurrences
WHERE active = true
  AND artifact_type = $1
  AND materialization_state IN ('pending', 'claimed', 'materializing')
  AND (lease_expires_at IS NULL OR lease_expires_at < NOW())
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT 1`, artifactType)
	occ, err := scanOccurrence(row)
	if err != nil {
		return UnresolvedOccurrence{}, err
	}
	expires := time.Now().Add(leaseFor)
	if _, err := tx.ExecContext(ctx, `
UPDATE kb.unresolved_semantic_occurrences
SET materialization_state = 'claimed', lease_token = $2, lease_expires_at = $3
WHERE id = $1`, occ.ID, leaseToken, expires); err != nil {
		return UnresolvedOccurrence{}, err
	}
	if err := tx.Commit(); err != nil {
		return UnresolvedOccurrence{}, err
	}
	occ.MaterializationState = MaterializationClaimed
	occ.LeaseToken = leaseToken
	occ.LeaseExpiresAt = &expires
	return occ, nil
}

// MaterializeFunc performs the family-specific writes of a materialization
// inside the store's transaction: create or reuse the assertion, evidence,
// class-resolution decision, outcome envelope, and findings. It returns the
// resulting assertion and outcome IDs.
type MaterializeFunc func(ctx context.Context, tx *sql.Tx, occ UnresolvedOccurrence) (assertionID int64, outcomeID int64, err error)

// Materialize runs fn and supersedes the occurrence in ONE transaction (DR13).
//
// The atomicity is the requirement, not an optimization: DR13 states "a crash
// cannot leave a materialized assertion paired with an active unresolved
// occurrence". Because fn's writes and the supersession commit together, that
// pairing is unreachable rather than merely unlikely, and the recoverable-saga
// path with its idempotency key is only needed by families whose
// materialization genuinely cannot fit in one transaction.
//
// The lease token is re-checked inside the transaction: a worker whose lease
// expired while it was working must not commit over the worker that took over.
func (s OccurrenceStore) Materialize(ctx context.Context, occ UnresolvedOccurrence, leaseToken string, fn MaterializeFunc) (UnresolvedOccurrence, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return UnresolvedOccurrence{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentToken sql.NullString
	var active bool
	if err := tx.QueryRowContext(ctx,
		`SELECT lease_token, active FROM kb.unresolved_semantic_occurrences WHERE id = $1 FOR UPDATE`,
		occ.ID).Scan(&currentToken, &active); err != nil {
		return UnresolvedOccurrence{}, err
	}
	if !active {
		return UnresolvedOccurrence{}, fmt.Errorf("occurrence %d is no longer active", occ.ID)
	}
	if currentToken.String != leaseToken {
		return UnresolvedOccurrence{}, fmt.Errorf("lease for occurrence %d was taken over by another worker", occ.ID)
	}

	assertionID, outcomeID, err := fn(ctx, tx, occ)
	if err != nil {
		return UnresolvedOccurrence{}, fmt.Errorf("materialize occurrence %d: %w", occ.ID, err)
	}

	row := tx.QueryRowContext(ctx, `
UPDATE kb.unresolved_semantic_occurrences
SET materialization_state = 'materialized',
    resulting_assertion_id = $2,
    current_outcome_id = $3,
    active = false,
    saga_completed_at = NOW(),
    lease_token = NULL,
    lease_expires_at = NULL
WHERE id = $1
RETURNING `+occurrenceColumns, occ.ID, assertionID, outcomeID)
	updated, err := scanOccurrence(row)
	if err != nil {
		return UnresolvedOccurrence{}, err
	}
	if err := tx.Commit(); err != nil {
		return UnresolvedOccurrence{}, err
	}
	return updated, nil
}

// ActiveOccurrence returns the current occurrence for a key, or sql.ErrNoRows.
func (s OccurrenceStore) ActiveOccurrence(ctx context.Context, occurrenceKey string) (UnresolvedOccurrence, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT `+occurrenceColumns+` FROM kb.unresolved_semantic_occurrences
		 WHERE occurrence_key = $1 AND active = true`, occurrenceKey)
	return scanOccurrence(row)
}

// ActiveOccurrencesForInputRecord returns every currently-active unresolved
// occurrence for one input record -- task 7.1's generic-discovery reader.
// DR13 requires the current row to be "queryable through the same generic
// semantic-discovery API as assertions"; input_record_id is the scope
// AssertionListFilter already uses for that same per-document discovery, so
// this mirrors it rather than introducing a second convention.
func (s OccurrenceStore) ActiveOccurrencesForInputRecord(ctx context.Context, inputRecordID int64) ([]UnresolvedOccurrence, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT `+occurrenceColumns+`
FROM kb.unresolved_semantic_occurrences
WHERE input_record_id = $1 AND active = true
ORDER BY id`, inputRecordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnresolvedOccurrence
	for rows.Next() {
		occ, err := scanOccurrence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, occ)
	}
	return out, rows.Err()
}

const occurrenceColumns = `id, occurrence_key, input_record_id, artifact_type, artifact_id,
	source_revision, raw_payload, provenance, materialization_state, resulting_assertion_id,
	current_outcome_id, supersedes_occurrence_id, input_fingerprint, dependency_fingerprint,
	active, lease_token, lease_expires_at, saga_idempotency_key, saga_completed_at, last_seen, create_by`

func lockActiveOccurrence(ctx context.Context, tx *sql.Tx, key string) (*UnresolvedOccurrence, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT `+occurrenceColumns+` FROM kb.unresolved_semantic_occurrences
		 WHERE occurrence_key = $1 AND active = true FOR UPDATE`, key)
	occ, err := scanOccurrence(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, err
	}
	return &occ, nil
}

func insertOccurrence(ctx context.Context, tx *sql.Tx, occ UnresolvedOccurrence) (UnresolvedOccurrence, error) {
	state := occ.MaterializationState
	if state == "" {
		state = MaterializationPending
	}
	var rawPayload, provenance any
	if len(occ.RawPayload) > 0 {
		rawPayload = []byte(occ.RawPayload)
	}
	if len(occ.Provenance) > 0 {
		provenance = []byte(occ.Provenance)
	}
	row := tx.QueryRowContext(ctx, `
INSERT INTO kb.unresolved_semantic_occurrences (
  occurrence_key, input_record_id, artifact_type, artifact_id, source_revision,
  raw_payload, provenance, materialization_state, supersedes_occurrence_id,
  input_fingerprint, dependency_fingerprint, active, saga_idempotency_key, create_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,true,$12,$13)
RETURNING `+occurrenceColumns,
		occ.OccurrenceKey, occ.InputRecordID, occ.ArtifactType, occ.ArtifactID,
		nullIfEmpty(occ.SourceRevision), rawPayload, provenance, state, occ.SupersedesOccurrenceID,
		occ.InputFingerprint, occ.DependencyFingerprint,
		nullIfEmpty(occ.SagaIdempotencyKey), nullIfEmpty(occ.CreateBy))
	return scanOccurrence(row)
}

func scanOccurrence(row rowScanner) (UnresolvedOccurrence, error) {
	var o UnresolvedOccurrence
	var sourceRevision, leaseToken, sagaKey, createBy sql.NullString
	var rawPayload, provenance []byte
	if err := row.Scan(&o.ID, &o.OccurrenceKey, &o.InputRecordID, &o.ArtifactType, &o.ArtifactID,
		&sourceRevision, &rawPayload, &provenance, &o.MaterializationState, &o.ResultingAssertionID,
		&o.CurrentOutcomeID, &o.SupersedesOccurrenceID, &o.InputFingerprint, &o.DependencyFingerprint,
		&o.Active, &leaseToken, &o.LeaseExpiresAt, &sagaKey, &o.SagaCompletedAt,
		&o.LastSeen, &createBy); err != nil {
		return UnresolvedOccurrence{}, err
	}
	o.SourceRevision = sourceRevision.String
	o.RawPayload = json.RawMessage(rawPayload)
	o.Provenance = json.RawMessage(provenance)
	o.LeaseToken = leaseToken.String
	o.SagaIdempotencyKey = sagaKey.String
	o.CreateBy = createBy.String
	return o, nil
}
