package docprocessing

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// entity_name_indexing.go catalogues kb.entities.entity_en as a dictionary and
// connects each dictionary key to the entity instance row it names.

const entityNameCategoryType = "entity"

type entityNameGraphRow struct {
	EntityID string
	Entity   string
	EntityEN string
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

	categoryMisses := resolveEntityNameCategories(ctx, db, recordID, rows, logger)
	nameMisses := catalogueEntityNames(ctx, db, rows, logger)
	conns := buildEntityNameGraphConnections(recordID, rows)
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
			"category_resolution_misses", categoryMisses,
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

func buildEntityNameGraphConnections(recordID int64, rows []entityNameGraphRow) []Connection {
	conns := make([]Connection, 0, len(rows))
	for _, r := range rows {
		key := entityNameKey(r)
		if key == "" || strings.TrimSpace(r.EntityID) == "" {
			continue
		}
		conns = append(conns, Connection{
			SourceRecordID: recordID,
			TargetRecordID: recordID,
			SourceType:     artifactTypeEntityName,
			SourceID:       key,
			TargetType:     searchArtifactEntity,
			TargetID:       strings.TrimSpace(r.EntityID),
			RelationName:   RelationHasInstance,
			RelationMethod: RelationMethodEntityName,
			ExtraInfo: map[string]any{
				"edge_kind": "entity_name_instance",
				"entity":    strings.TrimSpace(r.Entity),
				"entity_en": strings.TrimSpace(r.EntityEN),
			},
		})
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

func resolveEntityNameCategories(ctx context.Context, db *sql.DB, recordID int64, rows []entityNameGraphRow, logger ApiTypes.JimoLogger) int {
	reqs := entityNameCategoryRequests(rows)
	if len(reqs) == 0 {
		return 0
	}

	resolver := newMetricCategoryResolver(db, logger)
	_, errs := resolver.ResolveBatch(ctx, entityNameCategoryType, reqs, categoryResolveMaxConcurrency())
	if logger != nil {
		for nk, err := range errs {
			logger.Warn("entity name indexing: resolve entity category failed",
				"record_id", recordID, "category_key", nk, "error", err.Error())
		}
	}
	return len(errs)
}

func entityNameCategoryRequests(rows []entityNameGraphRow) []categoryRequest {
	var reqs []categoryRequest
	for _, r := range rows {
		rawKey := firstNonEmptyTrimmed(r.EntityEN, r.Entity)
		if rawKey == "" {
			continue
		}
		reqs = append(reqs, categoryRequest{
			RawKey: rawKey,
			Evidence: map[string]any{
				"artifact_kind": entityNameCategoryType,
				"entity_id":     strings.TrimSpace(r.EntityID),
				"entity":        strings.TrimSpace(r.Entity),
				"entity_en":     strings.TrimSpace(r.EntityEN),
			},
		})
	}
	return reqs
}

func loadEntityNameGraphRows(ctx context.Context, db *sql.DB, recordID int64) ([]entityNameGraphRow, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT entity_id, COALESCE(entity, ''), COALESCE(entity_en, '')
		 FROM kb.entities
		 WHERE input_record_id = $1
		 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []entityNameGraphRow
	for rows.Next() {
		var entityID, entity, entityEN string
		if err := rows.Scan(&entityID, &entity, &entityEN); err != nil {
			return nil, err
		}
		entityID = strings.TrimSpace(entityID)
		if entityID == "" {
			continue
		}
		out = append(out, entityNameGraphRow{
			EntityID: entityID,
			Entity:   strings.TrimSpace(entity),
			EntityEN: strings.TrimSpace(entityEN),
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
