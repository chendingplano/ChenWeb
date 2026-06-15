package kbsearch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lib/pq"
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
	Keywords        []string
	// EmbeddingText is the exact text that was embedded (stored for re-embedding /
	// debugging). Embedding is the pgvector value; nil when no embedding was
	// computed, in which case a NULL vector is stored and the row is lexical-only.
	EmbeddingText string
	Embedding     []float64
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

// insertStmtLexical is the original lexical-only upsert (13 positional args). Used
// when semantic search is disabled or before the pgvector migration is applied, so
// it never references the embedding_text / embedding columns.
const insertStmtLexical = `
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
	keywords,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, to_tsvector('simple', $7), $8, $9, $10, $11::jsonb, $12::jsonb, $13::jsonb, $14, NOW()
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
	keywords = EXCLUDED.keywords,
	updated_at = NOW()`

// insertStmtSemantic adds embedding_text ($14) and embedding ($15::vector). Used
// only when semantic search is enabled (env SEARCH_SEMANTIC_ENABLED=true), which
// requires the pgvector migration (20260603000001) to have been applied.
const insertStmtSemantic = `
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
	keywords,
	embedding_text,
	embedding,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, to_tsvector('simple', $7), $8, $9, $10, $11::jsonb, $12::jsonb, $13::jsonb, $14, $15, $16::vector, NOW()
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
	keywords = EXCLUDED.keywords,
	embedding_text = EXCLUDED.embedding_text,
	embedding = EXCLUDED.embedding,
	updated_at = NOW()`

func InsertSearchRegistryRows(ctx context.Context, db *sql.DB, rows []RegistryRow) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db is nil")
	}
	if len(rows) == 0 {
		return 0, nil
	}

	semantic := SemanticSearchEnabled()
	stmt := insertStmtLexical
	if semantic {
		stmt = insertStmtSemantic
	}

	var inserted int64
	for _, row := range rows {
		categoryPaths := normalizeJSONArg(row.CategoryPaths, "[]")
		sourceLineSpans := normalizeJSONArg(row.SourceLineSpans, "[]")
		semanticPayload := normalizeJSONArg(row.SemanticPayload, "{}")
		kw := row.Keywords
		if kw == nil {
			kw = []string{}
		}
		args := []any{
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
			pq.Array(kw), // $14 keywords
		}
		if semantic {
			args = append(args, embeddingTextArg(row), formatEmbeddingArg(row.Embedding)) // $15, $16
		}
		res, err := db.ExecContext(ctx, stmt, args...)
		if err != nil {
			return inserted, err
		}
		affected, _ := res.RowsAffected()
		inserted += affected
	}
	return inserted, nil
}

// embeddingTextArg returns the text to store in embedding_text, or NULL when both
// the embedding text and the embedding are absent (keeps lexical-only rows clean).
func embeddingTextArg(row RegistryRow) any {
	text := strings.TrimSpace(row.EmbeddingText)
	if text == "" {
		return nil
	}
	return text
}

// formatEmbeddingArg renders a []float64 as the pgvector text literal for binding
// to a $N::vector placeholder, or nil (SQL NULL) when no vector exists.
func formatEmbeddingArg(vec []float64) any {
	if len(vec) == 0 {
		return nil
	}
	return FormatVectorLiteral(vec)
}

func normalizeJSONArg(raw json.RawMessage, fallback string) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return fallback
	}
	return trimmed
}
