package kbsearch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type RegistryRow struct {
	ArtifactType    string
	ArtifactID      string
	InputRecordID   int64
	SourceRowID     *int64
	PrimaryLabel    string
	SecondaryLabel  string
	SearchDocument  string
	SnippetBasis    string
	SourceTitle     string
	SourceFilename  string
	CategoryPaths   json.RawMessage
	SourceLineSpans json.RawMessage
	SemanticPayload json.RawMessage
}

func DeleteSearchRegistryRowsForRecord(ctx context.Context, db *sql.DB, artifactType string, recordID int64) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db is nil")
	}
	res, err := db.ExecContext(ctx, `DELETE FROM kb.search_artifacts WHERE artifact_type = $1 AND input_record_id = $2`, strings.TrimSpace(artifactType), recordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func InsertSearchRegistryRows(ctx context.Context, db *sql.DB, rows []RegistryRow) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db is nil")
	}
	if len(rows) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.search_artifacts (
	artifact_type,
	artifact_id,
	input_record_id,
	source_row_id,
	primary_label,
	secondary_label,
	search_document,
	search_vector,
	snippet_basis,
	source_title,
	source_filename,
	category_paths,
	source_line_spans,
	semantic_payload,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, to_tsvector('simple', $7), $8, $9, $10, $11::jsonb, $12::jsonb, $13::jsonb, NOW()
)
ON CONFLICT (artifact_type, artifact_id) DO UPDATE SET
	input_record_id = EXCLUDED.input_record_id,
	source_row_id = EXCLUDED.source_row_id,
	primary_label = EXCLUDED.primary_label,
	secondary_label = EXCLUDED.secondary_label,
	search_document = EXCLUDED.search_document,
	search_vector = EXCLUDED.search_vector,
	snippet_basis = EXCLUDED.snippet_basis,
	source_title = EXCLUDED.source_title,
	source_filename = EXCLUDED.source_filename,
	category_paths = EXCLUDED.category_paths,
	source_line_spans = EXCLUDED.source_line_spans,
	semantic_payload = EXCLUDED.semantic_payload,
	updated_at = NOW()`

	var inserted int64
	for _, row := range rows {
		categoryPaths := normalizeJSONArg(row.CategoryPaths, "[]")
		sourceLineSpans := normalizeJSONArg(row.SourceLineSpans, "[]")
		semanticPayload := normalizeJSONArg(row.SemanticPayload, "{}")
		res, err := db.ExecContext(
			ctx,
			stmt,
			strings.TrimSpace(row.ArtifactType),
			strings.TrimSpace(row.ArtifactID),
			row.InputRecordID,
			row.SourceRowID,
			strings.TrimSpace(row.PrimaryLabel),
			strings.TrimSpace(row.SecondaryLabel),
			strings.TrimSpace(row.SearchDocument),
			strings.TrimSpace(row.SnippetBasis),
			strings.TrimSpace(row.SourceTitle),
			strings.TrimSpace(row.SourceFilename),
			categoryPaths,
			sourceLineSpans,
			semanticPayload,
		)
		if err != nil {
			return inserted, err
		}
		affected, _ := res.RowsAffected()
		inserted += affected
	}
	return inserted, nil
}

func normalizeJSONArg(raw json.RawMessage, fallback string) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return fallback
	}
	return trimmed
}
