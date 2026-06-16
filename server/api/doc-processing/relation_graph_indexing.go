package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// relation_graph_indexing.go implements the canonical relation store write-path
// (ADR 2026061401, D6). For every row in kb.relations it materializes the relation
// as a set of triples in kb.artifact_connections:
//
//  1. subject->object : (entity:subject_entity_id) --[predicate_en]--> (entity:object_entity_id)
//  2. predicate-of     : (relation_predicate:predicate_key) --[has-predicate]--> (relation:relation_id)
//
// Category membership is written separately as category_name edges in
// kb.artifact_connections. Predicates are catalogued in the kb.relation_predicates
// dictionary (D2).
//
// relationCategoryType is the kb.artifact_categories.category_type used to resolve a
// relation's categories. Predicates get their own dictionary table and are not
// category rows.
const relationCategoryType = "relation"

// relationGraphRow is the persisted view of one relation used by the graph step.
type relationGraphRow struct {
	RelationID      string
	SubjectEntityID string
	ObjectEntityID  string
	Predicate       string
	PredicateEN     string
	Categories      []string
}

// IndexRelationGraphForRecord writes the canonical relation-store edges for a record
// (ADR 2026061401 D6). Best-effort: a failure is logged and does not abort the caller.
func IndexRelationGraphForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) {
	start := time.Now()
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		if logger != nil {
			logger.Warn("relation graph indexing skipped: nil project db handle", "record_id", recordID)
		}
		return
	}

	rows, err := loadRelationGraphRows(ctx, db, recordID)
	if err != nil {
		if logger != nil {
			logger.Warn("relation graph indexing: load relations failed", "record_id", recordID, "error", err.Error())
		}
		return
	}
	if len(rows) == 0 {
		return
	}

	// Resolve every relation category in one batch (creating missing categories via the
	// LLM, coalesced process-wide).
	categoryIDs := resolveRelationCategoryIDs(ctx, db, recordID, rows, logger)
	categories := loadResolvedCategoriesForKeys(ctx, db, relationCategoryType, categoryIDs, logger, "relation graph indexing")

	// Catalogue each distinct predicate in the kb.relation_predicates dictionary (D2).
	// The predicate-of edge only needs the predicate_key, so a catalogue failure is
	// logged but does not drop the edge.
	predicateMisses := catalogueRelationPredicates(ctx, db, rows, logger)

	conns := buildRelationGraphConnections(recordID, rows)
	store := &ConnectionSQLStore{DB: db}
	if err := store.ReplaceRecordConnectionsByMethod(ctx, recordID, RelationMethodEntityRelation, conns); err != nil {
		if logger != nil {
			logger.Warn("relation graph indexing: replace connections failed", "record_id", recordID, "error", err.Error())
		}
		return
	}
	categoryConns := buildRelationCategoryConnections(recordID, rows, categories)
	if err := store.ReplaceConnectionsBySource(ctx, recordID, searchArtifactRelation, RelationMethodCategoryName, []string{RelationBelongTo}, categoryConns); err != nil {
		if logger != nil {
			logger.Warn("relation graph indexing: replace category connections failed", "record_id", recordID, "error", err.Error())
		}
		return
	}

	if logger != nil {
		logger.Info("relation graph indexing finished",
			"record_id", recordID,
			"relations", len(rows),
			"connections", len(conns),
			"category_connections", len(categoryConns),
			"predicate_catalogue_misses", predicateMisses,
			"ms_used", time.Since(start).Milliseconds(),
		)
	}
}

// relationPredicateKey returns the normalized English predicate key for a relation,
// falling back to the original-language predicate when predicate_en is empty.
func relationPredicateKey(r relationGraphRow) string {
	predicateEN := strings.TrimSpace(r.PredicateEN)
	if predicateEN == "" {
		predicateEN = strings.TrimSpace(r.Predicate)
	}
	return normalizePredicate(predicateEN)
}

// buildRelationGraphConnections turns relations into the canonical triples of ADR
// 2026061401 (D3). Pure: no DB, no side effects.
func buildRelationGraphConnections(recordID int64, rows []relationGraphRow) []Connection {
	conns := make([]Connection, 0, len(rows)*2)
	for _, r := range rows {
		predicateKey := relationPredicateKey(r)

		// Edge 1 — the relation statement itself: subject entity -> object entity.
		if r.SubjectEntityID != "" && r.ObjectEntityID != "" && predicateKey != "" {
			conns = append(conns, Connection{
				SourceRecordID: recordID,
				TargetRecordID: recordID,
				SourceType:     searchArtifactEntity,
				SourceID:       r.SubjectEntityID,
				TargetType:     searchArtifactEntity,
				TargetID:       r.ObjectEntityID,
				RelationName:   predicateKey,
				RelationMethod: RelationMethodEntityRelation,
				ExtraInfo: map[string]any{
					"edge_kind":   "subject_object",
					"relation_id": r.RelationID,
					"predicate":   strings.TrimSpace(r.Predicate),
				},
			})
		}

		// Edge 2 — predicate typing: relation_predicate -> relation.
		if predicateKey != "" {
			conns = append(conns, Connection{
				SourceRecordID: recordID,
				TargetRecordID: recordID,
				SourceType:     artifactTypeRelationPredicate,
				SourceID:       predicateKey,
				TargetType:     searchArtifactRelation,
				TargetID:       r.RelationID,
				RelationName:   RelationHasPredicate,
				RelationMethod: RelationMethodEntityRelation,
				ExtraInfo:      map[string]any{"edge_kind": "predicate_of"},
			})
		}
	}
	return conns
}

func buildRelationCategoryConnections(recordID int64, rows []relationGraphRow, categories map[string]resolvedCategory) []Connection {
	artifacts := make([]indexedArtifact, 0, len(rows))
	for _, r := range rows {
		relationID := strings.TrimSpace(r.RelationID)
		if relationID == "" {
			continue
		}
		artifacts = append(artifacts, indexedArtifact{
			ID:         relationID,
			Categories: r.Categories,
		})
	}
	return buildArtifactCategoryConnections(recordID, artifacts, relationIndexConfig, categories)
}

// catalogueRelationPredicates upserts each distinct predicate_key into the
// kb.relation_predicates dictionary. Returns the number of predicates that failed to
// catalogue (logged, non-fatal).
func catalogueRelationPredicates(ctx context.Context, db *sql.DB, rows []relationGraphRow, logger ApiTypes.JimoLogger) int {
	seen := map[string]struct{}{}
	misses := 0
	for _, r := range rows {
		predicateKey := relationPredicateKey(r)
		if predicateKey == "" {
			continue
		}
		if _, dup := seen[predicateKey]; dup {
			continue
		}
		seen[predicateKey] = struct{}{}
		if _, err := upsertRelationPredicate(ctx, db, predicateKey, strings.TrimSpace(r.Predicate)); err != nil {
			misses++
			if logger != nil {
				logger.Warn("relation graph indexing: upsert predicate failed",
					"predicate_key", predicateKey, "error", err.Error())
			}
		}
	}
	return misses
}

// loadRelationGraphRows reads the relation fields the graph step needs from kb.relations.
func loadRelationGraphRows(ctx context.Context, db *sql.DB, recordID int64) ([]relationGraphRow, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT relation_id,
		        COALESCE(subject_entity_id, ''),
		        COALESCE(object_entity_id, ''),
		        COALESCE(predicate, ''),
		        COALESCE(predicate_en, ''),
		        COALESCE(categories, '[]'::jsonb)
		 FROM kb.relations
		 WHERE input_record_id = $1
		 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []relationGraphRow
	for rows.Next() {
		var (
			relationID string
			subjectID  string
			objectID   string
			predicate  string
			predEN     string
			catsRaw    []byte
		)
		if err := rows.Scan(&relationID, &subjectID, &objectID, &predicate, &predEN, &catsRaw); err != nil {
			return nil, err
		}
		relationID = strings.TrimSpace(relationID)
		if relationID == "" {
			continue
		}
		out = append(out, relationGraphRow{
			RelationID:      relationID,
			SubjectEntityID: strings.TrimSpace(subjectID),
			ObjectEntityID:  strings.TrimSpace(objectID),
			Predicate:       strings.TrimSpace(predicate),
			PredicateEN:     strings.TrimSpace(predEN),
			Categories:      parseRelationCategories(catsRaw),
		})
	}
	return out, rows.Err()
}

// parseRelationCategories decodes the kb.relations.categories JSONB array into a
// deduplicated string slice. Accepts a JSON string array; anything else yields nil.
func parseRelationCategories(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	return uniqueStrings(arr)
}

// resolveRelationCategoryIDs resolves every relation's category keys to
// kb.artifact_categories ids in one batch, returning normalizedKey -> category_id.
func resolveRelationCategoryIDs(ctx context.Context, db *sql.DB, recordID int64, rows []relationGraphRow, logger ApiTypes.JimoLogger) map[string]int64 {
	var reqs []categoryRequest
	for _, r := range rows {
		for _, key := range r.Categories {
			reqs = append(reqs, categoryRequest{
				RawKey:   key,
				Evidence: map[string]any{"artifact_kind": relationCategoryType, "predicate": r.PredicateEN},
			})
		}
	}
	if len(reqs) == 0 {
		return map[string]int64{}
	}
	resolver := newMetricCategoryResolver(db, logger)
	ids, errs := resolver.ResolveBatch(ctx, relationCategoryType, reqs, categoryResolveMaxConcurrency())
	if logger != nil {
		for nk, err := range errs {
			logger.Warn("relation graph indexing: resolve category failed",
				"record_id", recordID, "category_key", nk, "error", err.Error())
		}
	}
	return ids
}

// upsertRelationPredicate idempotently catalogues a predicate in the
// kb.relation_predicates dictionary and returns its predicate_id. The key is the
// normalized English predicate; predicate_raw records the first-seen original form.
func upsertRelationPredicate(ctx context.Context, db *sql.DB, predicateKey, predicateRaw string) (int64, error) {
	var predicateID int64
	err := db.QueryRowContext(ctx,
		`INSERT INTO kb.relation_predicates (predicate_key, predicate_raw, display_names, seen_count)
		 VALUES ($1, $2, jsonb_build_array($1::text), 1)
		 ON CONFLICT (predicate_key) DO UPDATE
		   SET seen_count   = kb.relation_predicates.seen_count + 1,
		       last_seen_at = now(),
		       modify_time  = now()
		 RETURNING predicate_id`,
		predicateKey, predicateRaw,
	).Scan(&predicateID)
	return predicateID, err
}
