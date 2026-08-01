package assertions

import (
	"context"
	"database/sql"
	"fmt"
)

// ProjectionKindObjectPrimaryClass is the DR10 derived-convenience-column
// projection: kb.object_nodes.primary_class_term_id, maintained from
// accepted core:instance_of assertions. This closes the P2 chunk E
// deferral ("primary_class_term_id is a DERIVED projection... not yet
// populated: classification-as-assertion needs the assertion store").
const ProjectionKindObjectPrimaryClass = "object_primary_class_term_id"

// init registers the classification projection with the default registry
// (DR11 seam 7): adding this projection kind required only this file.
func init() {
	if err := RegisterProjectionBuilder(ProjectionKindObjectPrimaryClass, buildPrimaryClassProjection, repairPrimaryClassProjection); err != nil {
		panic(err)
	}
}

// primaryClassificationFor returns the deterministic "primary" classification
// for an object node: the earliest-accepted core:instance_of assertion whose
// object is a governed term (spec §16.3 item 6-adjacent determinism -- object
// nodes may carry several simultaneous accepted classifications, per research
// §5.2 and CQ-I03; primary_class_term_id is a read-optimized convenience
// pointer to one of them, never the system of record for "which classes does
// this object have").
func primaryClassificationFor(ctx context.Context, db *sql.DB, objectID string) (termID string, assertionID int64, revision int, found bool, err error) {
	const stmt = `
SELECT object_ref_id, id, revision
FROM kb.semantic_assertions
WHERE subject_object_id = $1
  AND predicate_term_id = 'core:instance_of'
  AND object_ref_kind = 'ontology_term'
  AND status = 'accepted'
ORDER BY id ASC
LIMIT 1`
	row := db.QueryRowContext(ctx, stmt, objectID)
	if err := row.Scan(&termID, &assertionID, &revision); err != nil {
		if err == sql.ErrNoRows {
			return "", 0, 0, false, nil
		}
		return "", 0, 0, false, err
	}
	return termID, assertionID, revision, true, nil
}

// currentPrimaryClassTermID reads the currently materialized value, without
// consulting the authoritative source -- used to detect whether a stored
// value matches what buildPrimaryClassProjection would produce.
func currentPrimaryClassTermID(ctx context.Context, db *sql.DB, objectID string) (string, error) {
	var v sql.NullString
	err := db.QueryRowContext(ctx, `SELECT primary_class_term_id FROM kb.object_nodes WHERE object_id = $1`, objectID).Scan(&v)
	if err != nil {
		return "", err
	}
	return v.String, nil
}

// buildPrimaryClassProjection deterministically recomputes
// kb.object_nodes.primary_class_term_id for one object from its
// authoritative accepted classification assertion (spec §10.8). Idempotent:
// recomputing an already-current projection writes the same value again.
func buildPrimaryClassProjection(ctx context.Context, db *sql.DB, objectID string) error {
	termID, assertionID, revision, found, err := primaryClassificationFor(ctx, db, objectID)
	if err != nil {
		return fmt.Errorf("compute primary classification for %s: %w", objectID, err)
	}

	stateStore := ProjectionStateStore{DB: db}
	if !found {
		if _, err := db.ExecContext(ctx, `UPDATE kb.object_nodes SET primary_class_term_id = NULL WHERE object_id = $1`, objectID); err != nil {
			return fmt.Errorf("clear primary_class_term_id for %s: %w", objectID, err)
		}
		if _, err := db.ExecContext(ctx, `
DELETE FROM kb.projection_state
WHERE projection_kind = $1 AND projection_target_table = 'kb.object_nodes' AND projection_target_id = $2`,
			ProjectionKindObjectPrimaryClass, objectID); err != nil {
			return fmt.Errorf("clear projection_state for %s: %w", objectID, err)
		}
		return nil
	}

	if _, err := db.ExecContext(ctx, `UPDATE kb.object_nodes SET primary_class_term_id = $2 WHERE object_id = $1`, objectID, termID); err != nil {
		return fmt.Errorf("set primary_class_term_id for %s: %w", objectID, err)
	}
	if err := stateStore.Upsert(ctx, ProjectionState{
		ProjectionKind:        ProjectionKindObjectPrimaryClass,
		ProjectionTargetTable: "kb.object_nodes",
		ProjectionTargetID:    objectID,
		AuthoritativeTable:    "kb.semantic_assertions",
		AuthoritativeID:       assertionID,
		AuthoritativeRevision: revision,
	}); err != nil {
		return fmt.Errorf("record projection_state for %s: %w", objectID, err)
	}
	return nil
}

// repairPrimaryClassProjection compares the materialized value against what
// the authoritative source currently implies -- catching both genuine
// staleness (the accepted assertion changed) and direct corruption of the
// materialized column (someone edited kb.object_nodes.primary_class_term_id
// without going through this mechanism, spec §16.2 item 6) -- and repairs
// via the same deterministic replay as a fresh build (spec §10.8) if and
// only if a mismatch is found.
func repairPrimaryClassProjection(ctx context.Context, db *sql.DB, objectID string) (bool, error) {
	expectedTermID, _, _, found, err := primaryClassificationFor(ctx, db, objectID)
	if err != nil {
		return false, err
	}
	currentValue, err := currentPrimaryClassTermID(ctx, db, objectID)
	if err != nil {
		return false, err
	}
	if found && currentValue == expectedTermID {
		return false, nil
	}
	if !found && currentValue == "" {
		return false, nil
	}
	return true, buildPrimaryClassProjection(ctx, db, objectID)
}
