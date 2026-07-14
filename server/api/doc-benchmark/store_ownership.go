package docbenchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

var _ OwnershipStore = SQLStore{}

// PersistInputOwnership atomically binds a newly seeded production input to
// its execution workspace and immutable attempt snapshot. Repeating the exact
// binding is safe; an attempt or workspace cannot be rebound to another input.
func (s SQLStore) PersistInputOwnership(ctx context.Context, attemptID string, inputID int64) error {
	if err := checkDB(s); err != nil {
		return err
	}
	if attemptID == "" || inputID <= 0 {
		return errors.New("benchmark: invalid input ownership")
	}
	tx, err := s.DB.BeginTx(txctx(ctx), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var workspaceInput sql.NullInt64
	if err = tx.QueryRowContext(txctx(ctx), `SELECT input_record_id FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1 FOR UPDATE`, attemptID).Scan(&workspaceInput); err != nil {
		return err
	}
	if workspaceInput.Valid && workspaceInput.Int64 != inputID {
		return &ConflictError{Resource: "workspace input ownership", Key: attemptID}
	}
	if !workspaceInput.Valid {
		if _, err = tx.ExecContext(txctx(ctx), `UPDATE kb.benchmark_workspaces SET input_record_id=$2 WHERE execution_attempt_id=$1 AND input_record_id IS NULL`, attemptID, inputID); err != nil {
			return err
		}
	}
	var snapshot sql.NullInt64
	if err = tx.QueryRowContext(txctx(ctx), `SELECT input_record_id_snapshot FROM kb.benchmark_case_attempts WHERE id=$1 AND kind='execution' FOR UPDATE`, attemptID).Scan(&snapshot); err != nil {
		return err
	}
	if snapshot.Valid && snapshot.Int64 != inputID {
		return &ConflictError{Resource: "attempt input snapshot", Key: attemptID}
	}
	if !snapshot.Valid {
		if _, err = tx.ExecContext(txctx(ctx), `UPDATE kb.benchmark_case_attempts SET input_record_id_snapshot=$2 WHERE id=$1 AND kind='execution' AND input_record_id_snapshot IS NULL`, attemptID, inputID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s SQLStore) LoadOwnership(attemptID string) (Ownership, error) {
	if err := checkDB(s); err != nil {
		return Ownership{}, err
	}
	var o Ownership
	var marker, workRoot, evidencePath, evidenceRoot, verifiedHash sql.NullString
	var verifiedSize sql.NullInt64
	err := s.DB.QueryRow(`SELECT execution_attempt_id,canonical_dir,work_root,evidence_path,evidence_root,nonce,verified,verified_hash,verified_size,verified_marker_hash,cleanup_state FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1`, attemptID).
		Scan(&o.AttemptID, &o.Workspace, &workRoot, &evidencePath, &evidenceRoot, &o.Nonce, &o.Verified, &verifiedHash, &verifiedSize, &marker, &o.CleanupState)
	if err != nil {
		return Ownership{}, err
	}
	o.WorkRoot, o.EvidencePath, o.EvidenceRoot, o.VerifiedHash = workRoot.String, evidencePath.String, evidenceRoot.String, verifiedHash.String
	if verifiedSize.Valid {
		o.VerifiedSize = verifiedSize.Int64
	}
	if marker.Valid {
		o.VerifiedMarker = marker.String
	}
	return o, nil
}

func (s SQLStore) SaveOwnership(o Ownership) error {
	if err := checkDB(s); err != nil {
		return err
	}
	if o.AttemptID == "" || o.Workspace == "" || o.Nonce == "" {
		return errors.New("benchmark: invalid ownership")
	}
	_, err := s.DB.Exec(`INSERT INTO kb.benchmark_workspaces(execution_attempt_id,canonical_dir,work_root,evidence_path,evidence_root,nonce,cleanup_state,verified,verified_hash,verified_size,verified_marker_hash) VALUES ($1,$2,$3,$4,$5,$6,COALESCE(NULLIF($7,''),'pending'),$8,NULLIF($9,''),$10,NULLIF($11,'')) ON CONFLICT (execution_attempt_id) DO UPDATE SET canonical_dir=EXCLUDED.canonical_dir,work_root=EXCLUDED.work_root,evidence_path=EXCLUDED.evidence_path,evidence_root=EXCLUDED.evidence_root,nonce=EXCLUDED.nonce,cleanup_state=EXCLUDED.cleanup_state`, o.AttemptID, o.Workspace, o.WorkRoot, o.EvidencePath, o.EvidenceRoot, o.Nonce, o.CleanupState, o.Verified, o.VerifiedHash, o.VerifiedSize, o.VerifiedMarker)
	return err
}

func (s SQLStore) LockOwnership(attemptID string) (Ownership, error) {
	if err := checkDB(s); err != nil {
		return Ownership{}, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return Ownership{}, err
	}
	defer tx.Rollback()
	var o Ownership
	var marker, workRoot, evidencePath, evidenceRoot, verifiedHash sql.NullString
	var verifiedSize sql.NullInt64
	err = tx.QueryRow(`SELECT execution_attempt_id,canonical_dir,work_root,evidence_path,evidence_root,nonce,verified,verified_hash,verified_size,verified_marker_hash,cleanup_state FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1 FOR UPDATE`, attemptID).Scan(&o.AttemptID, &o.Workspace, &workRoot, &evidencePath, &evidenceRoot, &o.Nonce, &o.Verified, &verifiedHash, &verifiedSize, &marker, &o.CleanupState)
	if err != nil {
		return Ownership{}, err
	}
	o.WorkRoot, o.EvidencePath, o.EvidenceRoot, o.VerifiedHash = workRoot.String, evidencePath.String, evidenceRoot.String, verifiedHash.String
	if verifiedSize.Valid {
		o.VerifiedSize = verifiedSize.Int64
	}
	if marker.Valid {
		o.VerifiedMarker = marker.String
	}
	if err = tx.Commit(); err != nil {
		return Ownership{}, err
	}
	return o, nil
}

func (s SQLStore) MarkVerified(attemptID, hash string, size int64, marker AllocationMarker) error {
	return s.MarkVerifiedCAS(nil, attemptID, marker.Nonce, markerDigest(marker), hash, size, marker)
}

func (s SQLStore) MarkCleanupState(attemptID, state string, cause error) error {
	if err := checkDB(s); err != nil {
		return err
	}
	var msg any
	if cause != nil {
		msg = cause.Error()
	}
	_, err := s.DB.Exec(`UPDATE kb.benchmark_workspaces SET cleanup_state=$2,cleanup_error=$3,cleaned_at=CASE WHEN $2='cleaned' THEN now() ELSE cleaned_at END WHERE execution_attempt_id=$1`, attemptID, state, msg)
	return err
}

func markerJSON(m AllocationMarker) any { b, _ := json.Marshal(m); return b }
