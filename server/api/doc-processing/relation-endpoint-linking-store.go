package docprocessing

import (
	"context"
)

// Load methods backing Phase C endpoint linking (ADR 2026061702 DR4). They
// hydrate map shapes whose keys match what linkRelationEndpoints reads and what
// SaveRelations/SaveEntities write, so the load -> link -> resave round-trip is
// lossless. NOTE: not yet exercised against a live DB.

// LoadEntitiesForRecord returns the record's entities with the identity/grounding
// fields the link step matches on (surface forms + line_spans).
func (s EntityRelationSQLStore) LoadEntitiesForRecord(ctx context.Context, recordID int64) ([]map[string]any, error) {
	if err := s.ensureTables(ctx); err != nil {
		return nil, err
	}
	const q = `
SELECT COALESCE(entity_id, ''), COALESCE(entity, ''), COALESCE(entity_en, ''),
       COALESCE(entity_type, ''), COALESCE(entity_type_en, ''),
       COALESCE(aliases, '[]'::jsonb), COALESCE(aliases_en, '[]'::jsonb),
       COALESCE(line_spans, '[]'::jsonb), COALESCE(categories, '[]'::jsonb),
       COALESCE(entity_status, 'extracted')
FROM kb.entities WHERE input_record_id = $1`
	rows, err := s.DB.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var (
			id, entity, en, etype, etypeEn, status   string
			aliases, aliasesEn, lineSpans, categories []byte
		)
		if err := rows.Scan(&id, &entity, &en, &etype, &etypeEn,
			&aliases, &aliasesEn, &lineSpans, &categories, &status); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"entity_id":         id,
			"entity":            entity,
			"entity_en":         en,
			"entity_type":       etype,
			"entity_type_en":    etypeEn,
			"aliases":           scanJSONStrings(aliases),
			"aliases_en":        scanJSONStrings(aliasesEn),
			"line_spans":        scanJSONStrings(lineSpans),
			"entity_categories": scanJSONStrings(categories),
			"entity_status":     status,
		})
	}
	return out, rows.Err()
}

// LoadRelationsForRecord returns the record's free-form relations, including the
// endpoint line citations (subject_lines/predicate_lines/object_lines) the link
// step matches on. Keys mirror what SaveRelations consumes for a lossless resave.
func (s EntityRelationSQLStore) LoadRelationsForRecord(ctx context.Context, recordID int64) ([]map[string]any, error) {
	if err := s.ensureTables(ctx); err != nil {
		return nil, err
	}
	const q = `
SELECT COALESCE(relation_id, ''), COALESCE(subject, ''), COALESCE(subject_en, ''),
       COALESCE(predicate, ''), COALESCE(predicate_en, ''), COALESCE(object, ''),
       COALESCE(object_en, ''), COALESCE(desc_text, ''), COALESCE(desc_text_en, ''),
       COALESCE(keywords, '[]'::jsonb), COALESCE(keywords_en, '[]'::jsonb),
       COALESCE(line_spans, '[]'::jsonb), COALESCE(categories, '[]'::jsonb),
       COALESCE(subject_lines, '[]'::jsonb), COALESCE(predicate_lines, '[]'::jsonb),
       COALESCE(object_lines, '[]'::jsonb), COALESCE(confidence, 0),
       COALESCE(language, '')
FROM kb.relations WHERE input_record_id = $1`
	rows, err := s.DB.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var (
			relID, subj, subjEn, pred, predEn, obj, objEn string
			desc, descEn                                  string
			keywords, keywordsEn, lineSpans, categories   []byte
			subjLines, predLines, objLines                []byte
			confidence                                    float64
			language                                      string
		)
		if err := rows.Scan(&relID, &subj, &subjEn, &pred, &predEn, &obj, &objEn,
			&desc, &descEn, &keywords, &keywordsEn, &lineSpans, &categories,
			&subjLines, &predLines, &objLines, &confidence, &language); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"language":            language,
			"relation_id":         relID,
			"subject":             subj,
			"subject_en":          subjEn,
			"predicate":           pred,
			"predicate_en":        predEn,
			"object":              obj,
			"object_en":           objEn,
			"desc":                desc,
			"desc_en":             descEn,
			"keywords":            scanJSONStrings(keywords),
			"keywords_en":         scanJSONStrings(keywordsEn),
			"line_spans":          scanJSONStrings(lineSpans),
			"relation_categories": scanJSONStrings(categories),
			"subject_lines":       scanJSONStrings(subjLines),
			"predicate_lines":     scanJSONStrings(predLines),
			"object_lines":        scanJSONStrings(objLines),
			"confidence":          confidence,
		})
	}
	return out, rows.Err()
}

// Compile-time guarantee the SQL store satisfies the Phase C link seam.
var _ RelationLinkStore = EntityRelationSQLStore{}
