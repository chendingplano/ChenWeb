package docprocessing

import (
	"context"
	"fmt"
	"strings"
)

// Phase C endpoint linking (ADR 2026061702 DR4). After both extraction passes
// finish, ControlService.runPostProcessIndexing calls this once per record. It
// loads the independently-extracted entities and the free-form relations, runs
// the pure, line-anchored linkRelationEndpoints, then persists the linked +
// deduplicated relations and any minted provisional entities — replacing the
// record's relations idempotently so a Phase C re-run is safe.

// RelationLinkStore is the persistence seam for Phase C linking. The two Load
// methods are new; Delete/SaveRelations/SaveEntities already exist on
// EntityRelationSQLStore (the replace-relations pattern keeps it idempotent).
type RelationLinkStore interface {
	LoadEntitiesForRecord(ctx context.Context, recordID int64) ([]map[string]any, error)
	LoadRelationsForRecord(ctx context.Context, recordID int64) ([]map[string]any, error)
	DeleteRelationsByInputRecordID(ctx context.Context, recordID int64) (int64, error)
	SaveRelations(ctx context.Context, req SaveRelationsRequest) (int64, error)
	SaveEntities(ctx context.Context, req SaveEntitiesRequest) (int64, error)
}

// LinkStats summarises one Phase C link pass (logged into the doc-proc summary).
type LinkStats struct {
	Relations   int // relations loaded
	Linked      int // relations after link + dedup
	Provisional int // provisional entities minted
}

// linkRecordEndpoints performs the Phase C link for one record. Idempotent:
// re-running re-derives the same linked set from the stored surface forms and
// replaces the record's relations.
func linkRecordEndpoints(
	ctx context.Context,
	store RelationLinkStore,
	recordID int64,
	eventID, language, modelName, promptName, createTime string,
) (LinkStats, error) {
	var stats LinkStats

	entities, err := store.LoadEntitiesForRecord(ctx, recordID)
	if err != nil {
		return stats, fmt.Errorf("(MID_26061730) load entities for record %d: %w", recordID, err)
	}
	relations, err := store.LoadRelationsForRecord(ctx, recordID)
	if err != nil {
		return stats, fmt.Errorf("(MID_26061731) load relations for record %d: %w", recordID, err)
	}
	stats.Relations = len(relations)
	if len(relations) == 0 {
		return stats, nil
	}

	// Preserve the stored language when the caller didn't supply one (Phase C
	// re-saves the relations, and SaveRelations writes the language column).
	if strings.TrimSpace(language) == "" {
		for _, r := range relations {
			if lang := strings.TrimSpace(asString(r["language"])); lang != "" {
				language = lang
				break
			}
		}
	}

	origEntityCount := len(entities)
	linked, outEntities := linkRelationEndpoints(relations, entities, recordID, origEntityCount+1, createTime)
	provisionals := outEntities[origEntityCount:]
	stats.Linked = len(linked)
	stats.Provisional = len(provisionals)

	// Replace the record's relations with the linked + deduped set.
	if _, err := store.DeleteRelationsByInputRecordID(ctx, recordID); err != nil {
		return stats, fmt.Errorf("(MID_26061732) clear relations for record %d: %w", recordID, err)
	}
	if _, err := store.SaveRelations(ctx, SaveRelationsRequest{
		InputRecordID: recordID,
		EventID:       eventID,
		Language:      language,
		ModelName:     modelName,
		PromptName:    promptName,
		Relations:     linked,
	}); err != nil {
		return stats, fmt.Errorf("(MID_26061733) save linked relations for record %d: %w", recordID, err)
	}

	// Persist newly minted provisional entities so the edges resolve to real ids.
	if len(provisionals) > 0 {
		if _, err := store.SaveEntities(ctx, SaveEntitiesRequest{
			InputRecordID: recordID,
			EventID:       eventID,
			Language:      language,
			ModelName:     modelName,
			PromptName:    promptName,
			Entities:      provisionals,
		}); err != nil {
			return stats, fmt.Errorf("(MID_26061734) save provisional entities for record %d: %w", recordID, err)
		}
	}

	return stats, nil
}
