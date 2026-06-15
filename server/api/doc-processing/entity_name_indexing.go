package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// entity_name_indexing.go catalogues kb.entities.entity_en as a name dictionary and
// connects each dictionary key to the entity instance row it names. It also resolves
// the entity's own kb.entities.categories to kb.artifact_categories and projects the
// (entity, category) membership — entity *names* are never treated as categories
// (ADR 2026061502, Problem 02); only the explicit categories column is.

// entityCategoryType is the kb.artifact_categories.category_type used to resolve an
// entity's categories (the kb.entities.categories array). Entity *names* are catalogued
// separately in the kb.entity_names dictionary and are not category rows.
const entityCategoryType = "entity"

type entityNameGraphRow struct {
	EntityID   string
	Entity     string
	EntityEN   string
	Categories []string
}

func IndexEntityNamesForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) {
	start := time.Now()
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		if logger != nil {
			logger.Warn("entity name indexing skipped: nil project db handle", "record_id", recordID)
		}
		return
	}

	rows, err := loadEntityNameGraphRows(ctx, db, recordID)
	if err != nil {
		if logger != nil {
			logger.Warn("entity name indexing: load entities failed", "record_id", recordID, "error", err.Error())
		}
		return
	}
	if len(rows) == 0 {
		return
	}

	// Resolve the entity's declared categories (kb.entities.categories) in one batch,
	// then reuse the id map for both the category_instance projection and the
	// belong-to-category edges so the projection and the canonical store stay consistent.
	categoryIDs := resolveEntityCategoryIDs(ctx, db, recordID, rows, logger)
	instances := upsertEntityCategoryInstances(ctx, db, recordID, rows, categoryIDs, logger)
	nameMisses := catalogueEntityNames(ctx, db, rows, logger)
	conns := buildEntityNameGraphConnections(recordID, rows, categoryIDs)
	store := &ConnectionSQLStore{DB: db}
	if err := store.ReplaceRecordConnectionsByMethod(ctx, recordID, RelationMethodEntityName, conns); err != nil {
		if logger != nil {
			logger.Warn("entity name indexing: replace connections failed", "record_id", recordID, "error", err.Error())
		}
		return
	}

	if logger != nil {
		logger.Info("entity name indexing finished",
			"record_id", recordID,
			"entities", len(rows),
			"connections", len(conns),
			"category_instances", instances,
			"name_catalogue_misses", nameMisses,
			"ms_used", time.Since(start).Milliseconds(),
		)
	}
}

func entityNameKey(r entityNameGraphRow) string {
	name := strings.TrimSpace(r.EntityEN)
	if name == "" {
		name = strings.TrimSpace(r.Entity)
	}
	return normalizeEntityNameKey(name)
}

func normalizeEntityNameKey(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func buildEntityNameGraphConnections(recordID int64, rows []entityNameGraphRow, categoryIDs map[string]int64) []Connection {
	conns := make([]Connection, 0, len(rows))
	for _, r := range rows {
		entityID := strings.TrimSpace(r.EntityID)
		if entityID == "" {
			continue
		}

		// Edge 1 — name dictionary: entity_name -> entity instance.
		if key := entityNameKey(r); key != "" {
			conns = append(conns, Connection{
				SourceRecordID: recordID,
				TargetRecordID: recordID,
				SourceType:     artifactTypeEntityName,
				SourceID:       key,
				TargetType:     searchArtifactEntity,
				TargetID:       entityID,
				RelationName:   RelationHasInstance,
				RelationMethod: RelationMethodEntityName,
				ExtraInfo: map[string]any{
					"edge_kind": "entity_name_instance",
					"entity":    strings.TrimSpace(r.Entity),
					"entity_en": strings.TrimSpace(r.EntityEN),
				},
			})
		}

		// Edge 2 — category membership: entity -> category, one edge per resolved
		// category. kb.category_instance is the membership projection (D4).
		for _, key := range r.Categories {
			categoryID, ok := categoryIDs[normalizeCategoryKey(key)]
			if !ok {
				continue
			}
			conns = append(conns, Connection{
				SourceRecordID: recordID,
				TargetRecordID: recordID,
				SourceType:     searchArtifactEntity,
				SourceID:       entityID,
				TargetType:     artifactTypeCategory,
				TargetID:       strconv.FormatInt(categoryID, 10),
				RelationName:   RelationBelongToCategory,
				RelationMethod: RelationMethodEntityName,
				ExtraInfo:      map[string]any{"edge_kind": "belong_to_category", "category_key": normalizeCategoryKey(key)},
			})
		}
	}
	return conns
}

func catalogueEntityNames(ctx context.Context, db *sql.DB, rows []entityNameGraphRow, logger ApiTypes.JimoLogger) int {
	seen := map[string]struct{}{}
	misses := 0
	for _, r := range rows {
		nameKey := entityNameKey(r)
		if nameKey == "" {
			continue
		}
		if _, dup := seen[nameKey]; dup {
			continue
		}
		seen[nameKey] = struct{}{}
		if _, err := upsertEntityName(ctx, db, nameKey, firstNonEmptyTrimmed(r.EntityEN, r.Entity)); err != nil {
			misses++
			if logger != nil {
				logger.Warn("entity name indexing: upsert name failed",
					"name_key", nameKey, "error", err.Error())
			}
		}
	}
	return misses
}

// resolveEntityCategoryIDs resolves every entity's declared category keys
// (kb.entities.categories) to kb.artifact_categories ids in one batch, returning
// normalizedKey -> category_id. Entity names are never used as category keys.
func resolveEntityCategoryIDs(ctx context.Context, db *sql.DB, recordID int64, rows []entityNameGraphRow, logger ApiTypes.JimoLogger) map[string]int64 {
	var reqs []categoryRequest
	for _, r := range rows {
		for _, key := range r.Categories {
			reqs = append(reqs, categoryRequest{
				RawKey: key,
				Evidence: map[string]any{
					"artifact_kind": entityCategoryType,
					"entity_id":     strings.TrimSpace(r.EntityID),
					"entity_en":     strings.TrimSpace(r.EntityEN),
				},
			})
		}
	}
	if len(reqs) == 0 {
		return map[string]int64{}
	}
	resolver := newMetricCategoryResolver(db, logger)
	ids, errs := resolver.ResolveBatch(ctx, entityCategoryType, reqs, categoryResolveMaxConcurrency())
	if logger != nil {
		for nk, err := range errs {
			logger.Warn("entity name indexing: resolve entity category failed",
				"record_id", recordID, "category_key", nk, "error", err.Error())
		}
	}
	return ids
}

// upsertEntityCategoryInstances writes one kb.category_instance row per
// (entity, resolved category). Returns the number of rows upserted.
func upsertEntityCategoryInstances(ctx context.Context, db *sql.DB, recordID int64, rows []entityNameGraphRow, categoryIDs map[string]int64, logger ApiTypes.JimoLogger) int {
	extraInfo, _ := json.Marshal(map[string]any{"artifact_type": entityCategoryType, "source": "extract_entity_relation"})
	count := 0
	for _, r := range rows {
		entityID := strings.TrimSpace(r.EntityID)
		if entityID == "" {
			continue
		}
		for _, key := range r.Categories {
			categoryID, ok := categoryIDs[normalizeCategoryKey(key)]
			if !ok {
				continue
			}
			if _, err := db.ExecContext(ctx,
				`INSERT INTO kb.category_instance (category_id, artifact_id, input_record_id, extra_info)
				 VALUES ($1, $2, $3, $4::jsonb)
				 ON CONFLICT (category_id, artifact_id)
				 DO UPDATE SET input_record_id = EXCLUDED.input_record_id, extra_info = EXCLUDED.extra_info`,
				categoryID, entityID, recordID, string(extraInfo)); err != nil {
				if logger != nil {
					logger.Warn("entity name indexing: upsert category_instance failed",
						"record_id", recordID, "entity_id", entityID, "category_id", categoryID, "error", err.Error())
				}
				continue
			}
			count++
		}
	}
	return count
}

func loadEntityNameGraphRows(ctx context.Context, db *sql.DB, recordID int64) ([]entityNameGraphRow, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT entity_id, COALESCE(entity, ''), COALESCE(entity_en, ''),
		        COALESCE(categories, '[]'::jsonb)
		 FROM kb.entities
		 WHERE input_record_id = $1
		 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []entityNameGraphRow
	for rows.Next() {
		var (
			entityID, entity, entityEN string
			catsRaw                    []byte
		)
		if err := rows.Scan(&entityID, &entity, &entityEN, &catsRaw); err != nil {
			return nil, err
		}
		entityID = strings.TrimSpace(entityID)
		if entityID == "" {
			continue
		}
		out = append(out, entityNameGraphRow{
			EntityID:   entityID,
			Entity:     strings.TrimSpace(entity),
			EntityEN:   strings.TrimSpace(entityEN),
			Categories: parseRelationCategories(catsRaw),
		})
	}
	return out, rows.Err()
}

func upsertEntityName(ctx context.Context, db *sql.DB, nameKey, nameRaw string) (int64, error) {
	var nameID int64
	err := db.QueryRowContext(ctx,
		`INSERT INTO kb.entity_names (name_key, name_raw, display_names, seen_count)
		 VALUES ($1, $2, jsonb_build_array($1::text), 1)
		 ON CONFLICT (name_key) DO UPDATE
		   SET seen_count   = kb.entity_names.seen_count + 1,
		       last_seen_at = now(),
		       modify_time  = now()
		 RETURNING name_id`,
		nameKey, nameRaw,
	).Scan(&nameID)
	return nameID, err
}
