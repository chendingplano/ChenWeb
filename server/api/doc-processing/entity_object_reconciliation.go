package docprocessing

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// Entity object-link lifecycle values for kb.entities.object_link_status.
// This is a third status axis, independent of entity_status (provenance)
// and reconcile_status (entity<->entity dedup lifecycle, ADR 2026061701 R2)
// — see ADR 2026070101 Phase 4 §New State on kb.entities.
const (
	entityObjectLinkPending   = "pending"
	entityObjectLinkExcluded  = "excluded"
	entityObjectLinkLinked    = "linked"
	entityObjectLinkDeferred  = "deferred"
	entityObjectLinkExhausted = "exhausted"
)

// entityTypeToObjectType is the static, deliberately conservative allow-list
// filtering which entity_type values are even attempted as object
// candidates. Absent from this map means "never attempt matching" — see ADR
// 2026070101 Phase 3 §Entity Type -> Object Type Compatibility for the
// reasoning behind each inclusion/exclusion.
var entityTypeToObjectType = map[string]string{
	"software_system": "system",
	"system":          "system",
	"platform":        "system",
	"equipment":       "equipment",
	"device":          "equipment",
	"machine":         "equipment",
	"product":         "equipment",
	"material":        "material",
	"substance":       "material",
	"organization":    "organization",
	"company":         "organization",
	"agency":          "organization",
	"place":           "place",
	"location":        "place",
	"facility":        "place",
}

// entityObjectTypeCandidate looks up the object_type an entity would carry
// if linked, preferring the English entity_type and falling back to the
// original-language one. The second return value is false when the entity's
// type is not in the allow-list, meaning it must never reach FindCandidates.
func entityObjectTypeCandidate(entityTypeEn, entityType string) (string, bool) {
	key := normalizeObjectToken(firstNonEmptyTrimmed(entityTypeEn, entityType))
	if key == "" {
		return "", false
	}
	objType, ok := entityTypeToObjectType[key]
	return objType, ok
}

// matchEntityToExistingObject looks for a single confident existing
// kb.object_nodes match for obj, using the same acceptance thresholds
// ObjectReconciler.ReconcileOne uses for its non-creating branches (a single
// exact match, or a clear single best match >= 0.95, no tie). Unlike
// ReconcileOne, it never creates a node: Phase 3's match-only restraint (ADR
// 2026070101 Phase 3 §Scope) is enforced here in code, not left to a caller's
// discipline.
func matchEntityToExistingObject(ctx context.Context, store ObjectNodeStore, opts ObjectReconcileOptions, obj ArtifactObject) (ObjectNodeCandidate, bool, error) {
	candidates, err := store.FindCandidates(ctx, obj, opts)
	if err != nil {
		return ObjectNodeCandidate{}, false, err
	}
	if len(candidates) == 0 {
		return ObjectNodeCandidate{}, false, nil
	}
	if len(candidates) == 1 && candidates[0].Score >= 1 {
		return candidates[0], true, nil
	}
	if len(candidates) > 1 && candidates[0].Score == candidates[1].Score {
		return ObjectNodeCandidate{}, false, nil
	}
	if candidates[0].Score >= 0.95 {
		return candidates[0], true, nil
	}
	return ObjectNodeCandidate{}, false, nil
}

// entityToArtifactObjectCandidate normalizes an entity row into the shared
// ArtifactObject shape metrics/provisions/inventory items already use, so
// FindCandidates and persistence need no entity-specific code path.
func entityToArtifactObjectCandidate(e pendingEntityRow, objType string) ArtifactObject {
	obj := ArtifactObject{
		SourceRecordID:  e.InputRecordID,
		InputRecordID:   e.InputRecordID,
		ArtifactType:    searchArtifactEntity,
		ArtifactID:      e.EntityID,
		ObjectName:      e.Entity,
		ObjectNameEn:    e.EntityEN,
		ObjectType:      objType,
		ObjectRole:      "represented_entity",
		Aliases:         uniqueStrings(e.Aliases),
		Description:     e.Desc,
		Confidence:      1,
		ReconcileStatus: ObjectReconcilePending,
		ExtInfo:         map[string]any{"source": searchArtifactEntity},
	}
	obj.NormalizedNames = buildObjectNormalizedNames(obj, e.AliasesEN)
	return obj
}

// EntityObjectStore is the DB seam ReconcileEntityObjectsForRecord needs
// beyond ObjectNodeStore: loading a record's entities and updating
// kb.entities.object_link_status for terminal outcomes.
type EntityObjectStore interface {
	LoadEntitiesForRecord(ctx context.Context, recordID int64) ([]pendingEntityRow, error)
	SetEntityObjectLinkStatus(ctx context.Context, entityID, status string) error
}

// ArtifactObjectPersister is the persistence seam ArtifactObjectSQLStore
// already implements; declared as an interface here so orchestration is
// testable without a real database.
type ArtifactObjectPersister interface {
	ReplaceObjectsForRecord(ctx context.Context, recordID int64, artifactType string, objects []ArtifactObject) error
}

// ReconcileEntityObjectsForRecord implements the Phase 3 Amendment (ADR
// 2026070101 Phase 4 §Phase 3 Amendment): for each of a record's entities,
// attempt a match-only link to an existing kb.object_nodes row. A confident
// match is persisted as a real kb.artifact_objects row (never a bespoke
// edge), so the standard artifact-object indexer produces the same
// belong_to edge metrics/provisions/inventory items already use. Entities
// excluded by the type allow-list get object_link_status='excluded'
// immediately, with no FindCandidates call. Entities with no confident
// match are left at the 'pending' default so Phase 4's LLM classifier
// (entity_object_resolve.go) can pick them up — never silently unexplained
// (ADR 2026070101 Phase 3 §What "Unlinked" Means).
//
// ReplaceObjectsForRecord is always called, even with zero matched objects,
// so a reprocess that now finds fewer/no matches still clears this record's
// stale entity-sourced kb.artifact_objects rows (idempotency).
func ReconcileEntityObjectsForRecord(ctx context.Context, entityStore EntityObjectStore, objectStore ArtifactObjectPersister, reconciler ObjectReconciler, recordID int64, logger ApiTypes.JimoLogger) error {
	entities, err := entityStore.LoadEntitiesForRecord(ctx, recordID)
	if err != nil {
		return err
	}

	var matched []ArtifactObject
	for _, e := range entities {
		objType, eligible := entityObjectTypeCandidate(e.EntityTypeEN, e.EntityType)
		if !eligible {
			if logger != nil {
				logger.Info("entity object-link skipped: type not eligible", "entity_id", e.EntityID, "entity_type", e.EntityType)
			}
			if err := entityStore.SetEntityObjectLinkStatus(ctx, e.EntityID, entityObjectLinkExcluded); err != nil && logger != nil {
				logger.Warn("entity object-link: set excluded status failed", "entity_id", e.EntityID, "error", err.Error())
			}
			continue
		}

		obj := entityToArtifactObjectCandidate(e, objType)
		candidate, ok, err := matchEntityToExistingObject(ctx, reconciler.Store, reconciler.Options, obj)
		if err != nil {
			if logger != nil {
				logger.Warn("entity object-link skipped: candidate search failed", "entity_id", e.EntityID, "error", err.Error())
			}
			continue
		}
		if !ok {
			if logger != nil {
				logger.Info("entity object-link skipped: no confident candidate", "entity_id", e.EntityID)
			}
			continue
		}

		obj.ObjectID = candidate.Node.ObjectID
		obj.ReconcileStatus = ObjectReconcileMatched
		obj.ReconcileConfidence = candidate.Score
		obj.ExtInfo["reconcile_method"] = candidate.Method
		matched = append(matched, obj)
	}

	if err := objectStore.ReplaceObjectsForRecord(ctx, recordID, searchArtifactEntity, matched); err != nil {
		return err
	}

	for _, obj := range matched {
		if err := entityStore.SetEntityObjectLinkStatus(ctx, obj.ArtifactID, entityObjectLinkLinked); err != nil && logger != nil {
			logger.Warn("entity object-link: set linked status failed", "entity_id", obj.ArtifactID, "error", err.Error())
		}
	}
	return nil
}

// EntityObjectSQLStore implements EntityObjectStore against kb.entities.
type EntityObjectSQLStore struct {
	DB *sql.DB
}

// LoadEntitiesForRecord loads every canonical (non-merged-away) entity for a
// record — canonical here means the entity<->entity dedup pass (ADR
// 2026061701) has not folded it into another head. It intentionally does not
// filter on that dedup pass's own reconcile_status: object-linking runs
// after semClusterEntities in Phase C, so most of a record's entities will
// already have progressed past that axis's 'pending' value by the time this
// runs.
func (s EntityObjectSQLStore) LoadEntitiesForRecord(ctx context.Context, recordID int64) ([]pendingEntityRow, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	const q = `
SELECT e.entity_id, e.entity, COALESCE(e.entity_en, ''),
       COALESCE(e.entity_type, ''), COALESCE(e.entity_type_en, ''),
       COALESCE(e.aliases, '[]'::jsonb), COALESCE(e.aliases_en, '[]'::jsonb),
       COALESCE(e.desc_text, ''), COALESCE(e.desc_text_en, '')
FROM kb.entities e
WHERE e.input_record_id = $1
  AND (e.canonical_entity_id IS NULL OR e.canonical_entity_id = '' OR e.canonical_entity_id = e.entity_id)
ORDER BY e.id`
	rows, err := s.DB.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []pendingEntityRow
	for rows.Next() {
		var (
			e                        pendingEntityRow
			aliasesRaw, aliasesEnRaw []byte
		)
		if err := rows.Scan(
			&e.EntityID, &e.Entity, &e.EntityEN,
			&e.EntityType, &e.EntityTypeEN,
			&aliasesRaw, &aliasesEnRaw,
			&e.Desc, &e.DescEN,
		); err != nil {
			return nil, err
		}
		e.InputRecordID = recordID
		e.Aliases = parseJSONStringArray(aliasesRaw)
		e.AliasesEN = parseJSONStringArray(aliasesEnRaw)
		out = append(out, e)
	}
	return out, rows.Err()
}

// SetEntityObjectLinkStatus updates kb.entities.object_link_status for one
// entity, identified by entity_id.
func (s EntityObjectSQLStore) SetEntityObjectLinkStatus(ctx context.Context, entityID, status string) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	_, err := s.DB.ExecContext(ctx, `
UPDATE kb.entities
SET object_link_status = $1
WHERE entity_id = $2`, status, entityID)
	return err
}
