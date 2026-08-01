package assertions

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// ProjectionBuildFunc deterministically recomputes and writes the
// projection for one target from its authoritative source, and
// records/updates the target's kb.projection_state row. Must be idempotent:
// building an already-current projection is a no-op write.
type ProjectionBuildFunc func(ctx context.Context, db *sql.DB, targetID string) error

// ProjectionRepairFunc detects and repairs staleness for one target,
// reporting whether the materialized projection actually needed a fix (so a
// sweep can distinguish "found and repaired corruption" from "already
// current"). For most projection kinds the repair is the same deterministic
// replay as ProjectionBuildFunc (spec §10.8: "Projection repair is
// deterministic from the authoritative store"); the registry keeps it as a
// distinct slot only because DR11 seam 7 names build and repair as separate
// registrations.
type ProjectionRepairFunc func(ctx context.Context, db *sql.DB, targetID string) (repaired bool, err error)

type projectionBuilderEntry struct {
	build  ProjectionBuildFunc
	repair ProjectionRepairFunc
}

// ProjectionBuilderRegistry is DR11 seam 7: adding a new derived-projection
// kind never requires editing project_semantics.go, only registering a
// build/repair pair.
type ProjectionBuilderRegistry struct {
	mu       sync.RWMutex
	builders map[string]projectionBuilderEntry
}

var defaultProjectionRegistry = &ProjectionBuilderRegistry{builders: map[string]projectionBuilderEntry{}}

// RegisterProjectionBuilder adds a build/repair pair to the default registry
// for one projection_kind.
func RegisterProjectionBuilder(kind string, build ProjectionBuildFunc, repair ProjectionRepairFunc) error {
	return defaultProjectionRegistry.Register(kind, build, repair)
}

// Register adds a build/repair pair to this registry instance.
func (r *ProjectionBuilderRegistry) Register(kind string, build ProjectionBuildFunc, repair ProjectionRepairFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if kind == "" {
		return fmt.Errorf("projection kind is required")
	}
	if build == nil {
		return fmt.Errorf("build function is required for projection kind %q", kind)
	}
	if repair == nil {
		repair = func(ctx context.Context, db *sql.DB, targetID string) (bool, error) {
			return true, build(ctx, db, targetID)
		}
	}
	if _, exists := r.builders[kind]; exists {
		return fmt.Errorf("projection builder already registered for kind %q", kind)
	}
	r.builders[kind] = projectionBuilderEntry{build: build, repair: repair}
	return nil
}

// LookupProjectionBuilder returns the default registry's build/repair pair
// for a projection kind.
func LookupProjectionBuilder(kind string) (ProjectionBuildFunc, ProjectionRepairFunc, bool) {
	return defaultProjectionRegistry.Lookup(kind)
}

// Lookup returns the build/repair pair registered for a kind, if any.
func (r *ProjectionBuilderRegistry) Lookup(kind string) (ProjectionBuildFunc, ProjectionRepairFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.builders[kind]
	if !ok {
		return nil, nil, false
	}
	return e.build, e.repair, true
}

// RegisteredProjectionKinds returns every projection_kind with a registered
// builder, in no particular order.
func RegisteredProjectionKinds() []string {
	return defaultProjectionRegistry.Kinds()
}

// Kinds returns every projection_kind registered on this registry instance.
func (r *ProjectionBuilderRegistry) Kinds() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.builders))
	for kind := range r.builders {
		out = append(out, kind)
	}
	return out
}

// ProjectionTargetsFunc returns the projection targets whose authoritative
// source may have changed for one input record -- e.g. the objects touched
// by this record's newly-accepted assertions this run.
type ProjectionTargetsFunc func(ctx context.Context, db *sql.DB, inputRecordID int64) ([]string, error)

type projectionRecordScopeEntry struct {
	targetTable      string
	targetsForRecord ProjectionTargetsFunc
}

var defaultProjectionRecordScopes = struct {
	mu    sync.RWMutex
	scope map[string]projectionRecordScopeEntry
}{scope: map[string]projectionRecordScopeEntry{}}

// RegisterProjectionRecordScope declares, for one projection kind, how to
// find the targets touched by a single input record and the
// kb.projection_state target table they live in. This is the piece
// ProjectSemantics.Run needs to scope a per-record rebuild (and mark a
// failed target stale) generically across every registered kind -- DR11
// seam 7's "adding a projection kind requires no change to
// project_semantics.go" only holds once this is registered alongside the
// build/repair pair. A projection kind repaired only by the periodic sweep
// (RepairStaleProjections) and never triggered per-record does not need
// this registration.
func RegisterProjectionRecordScope(kind, targetTable string, targetsForRecord ProjectionTargetsFunc) error {
	if kind == "" {
		return fmt.Errorf("projection kind is required")
	}
	if targetTable == "" {
		return fmt.Errorf("projection target table is required")
	}
	if targetsForRecord == nil {
		return fmt.Errorf("targetsForRecord is required")
	}
	defaultProjectionRecordScopes.mu.Lock()
	defer defaultProjectionRecordScopes.mu.Unlock()
	if _, exists := defaultProjectionRecordScopes.scope[kind]; exists {
		return fmt.Errorf("record scope already registered for projection kind %q", kind)
	}
	defaultProjectionRecordScopes.scope[kind] = projectionRecordScopeEntry{targetTable: targetTable, targetsForRecord: targetsForRecord}
	return nil
}

// LookupProjectionRecordScope returns the registered per-record target
// resolver and target table for a projection kind, if any.
func LookupProjectionRecordScope(kind string) (targetTable string, targetsForRecord ProjectionTargetsFunc, ok bool) {
	defaultProjectionRecordScopes.mu.RLock()
	defer defaultProjectionRecordScopes.mu.RUnlock()
	e, exists := defaultProjectionRecordScopes.scope[kind]
	if !exists {
		return "", nil, false
	}
	return e.targetTable, e.targetsForRecord, true
}

// ProjectionStateStore persists projection provenance (kb.projection_state).
type ProjectionStateStore struct {
	DB DBX
}

// ProjectionState is one row of kb.projection_state.
type ProjectionState struct {
	ID                    int64
	ProjectionKind        string
	ProjectionTargetTable string
	ProjectionTargetID    string
	AuthoritativeTable    string
	AuthoritativeID       int64
	AuthoritativeRevision int
	ProjectionVersion     int
	Stale                 bool
	StaleReason           string
}

const projectionStateColumns = `
	id, projection_kind, projection_target_table, projection_target_id,
	authoritative_table, authoritative_id, authoritative_revision,
	projection_version, stale, stale_reason`

// Upsert records or updates a target's projection provenance after a
// successful (re)build, clearing any prior staleness.
func (s ProjectionStateStore) Upsert(ctx context.Context, p ProjectionState) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	const stmt = `
INSERT INTO kb.projection_state
	(projection_kind, projection_target_table, projection_target_id,
	 authoritative_table, authoritative_id, authoritative_revision,
	 projection_version, stale, stale_reason, last_built_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE, NULL, NOW())
ON CONFLICT (projection_kind, projection_target_table, projection_target_id)
DO UPDATE SET
	authoritative_table = EXCLUDED.authoritative_table,
	authoritative_id = EXCLUDED.authoritative_id,
	authoritative_revision = EXCLUDED.authoritative_revision,
	projection_version = kb.projection_state.projection_version + 1,
	stale = FALSE,
	stale_reason = NULL,
	last_built_at = NOW(),
	modify_time = NOW()`
	_, err := s.DB.ExecContext(ctx, stmt,
		p.ProjectionKind, p.ProjectionTargetTable, p.ProjectionTargetID,
		p.AuthoritativeTable, p.AuthoritativeID, p.AuthoritativeRevision, maxInt(p.ProjectionVersion, 1))
	return err
}

// Get returns the current provenance row for a target, if any.
func (s ProjectionStateStore) Get(ctx context.Context, kind, targetTable, targetID string) (ProjectionState, error) {
	if s.DB == nil {
		return ProjectionState{}, fmt.Errorf("db is nil")
	}
	const stmt = `SELECT ` + projectionStateColumns + `
FROM kb.projection_state
WHERE projection_kind = $1 AND projection_target_table = $2 AND projection_target_id = $3`
	row := s.DB.QueryRowContext(ctx, stmt, kind, targetTable, targetID)
	var p ProjectionState
	var staleReason sql.NullString
	if err := row.Scan(&p.ID, &p.ProjectionKind, &p.ProjectionTargetTable, &p.ProjectionTargetID,
		&p.AuthoritativeTable, &p.AuthoritativeID, &p.AuthoritativeRevision,
		&p.ProjectionVersion, &p.Stale, &staleReason); err != nil {
		return ProjectionState{}, err
	}
	if staleReason.Valid {
		p.StaleReason = staleReason.String
	}
	return p, nil
}

// MarkStale flags a target's projection as stale without rebuilding it --
// used when the authoritative source is known to have changed but a full
// rebuild has not run yet (e.g. detected by a periodic sweep).
func (s ProjectionStateStore) MarkStale(ctx context.Context, kind, targetTable, targetID, reason string) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	const stmt = `
UPDATE kb.projection_state
SET stale = TRUE, stale_reason = $4, modify_time = NOW()
WHERE projection_kind = $1 AND projection_target_table = $2 AND projection_target_id = $3`
	_, err := s.DB.ExecContext(ctx, stmt, kind, targetTable, targetID, nullableString(reason))
	return err
}

// ListStale returns every stale projection row for a kind (empty kind = any).
func (s ProjectionStateStore) ListStale(ctx context.Context, kind string) ([]ProjectionState, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	const stmt = `SELECT ` + projectionStateColumns + `
FROM kb.projection_state
WHERE stale AND ($1 = '' OR projection_kind = $1)
ORDER BY id`
	rows, err := s.DB.QueryContext(ctx, stmt, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ProjectionState, 0)
	for rows.Next() {
		var p ProjectionState
		var staleReason sql.NullString
		if err := rows.Scan(&p.ID, &p.ProjectionKind, &p.ProjectionTargetTable, &p.ProjectionTargetID,
			&p.AuthoritativeTable, &p.AuthoritativeID, &p.AuthoritativeRevision,
			&p.ProjectionVersion, &p.Stale, &staleReason); err != nil {
			return nil, err
		}
		if staleReason.Valid {
			p.StaleReason = staleReason.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
