// Package classcontractsearch maintains kb.ontology_class_contract_search — a
// hybrid-search index over governed class contracts — and answers "which class
// contracts are similar to this one?" for the Metric Ontology Explorer's
// "Metrics of Similar Classes" node (openspec change analysis-node-related-metrics).
//
// It never runs inside a metric-write transaction: Reindex embeds text, which is
// network I/O, so it is always called on a *sql.DB after the write has committed
// (design D8).
package classcontractsearch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
)

// EmbedFunc computes an embedding for text; ok=false means "could not embed,
// proceed lexical-only". Same shape as kbsearch.EmbedFunc — callers pass
// kbhandler.computeQueryEmbedding or docprocessing.embedQueryText.
type EmbedFunc = kbsearch.EmbedFunc

const (
	// rrfK is the Reciprocal Rank Fusion constant, matching kb.search_artifacts
	// hybrid search (server/api/kbhandler/search_registry.go).
	rrfK = 60
	// candidateLimit bounds each side (lexical / semantic) of the fusion. The
	// class corpus is small; this is a defensive ceiling well above it.
	candidateLimit = 200
	// instanceSampleRows caps how many instance-metric rows feed a class's
	// search document (design D7).
	instanceSampleRows = 20
	// docMaxRunes bounds the text handed to the embedder (mirrors the indexer's
	// maxEmbeddingRunes).
	docMaxRunes = 6000
	// lexemeCap bounds how many distinct tokens the similar-class lexical query
	// OR-joins.
	lexemeCap = 40
)

// Store runs the read/write queries against kb.ontology_class_contract_search.
type Store struct {
	DB *sql.DB
}

// Match is one similar class returned by MatchSimilar.
type Match struct {
	ClassTermID string
	Score       float64
}

type docMeta struct {
	moduleID        string
	definitionState string
	currentRevID    sql.NullInt64
	instanceCount   int
}

// Reindex rebuilds the kb.ontology_class_contract_search row for one class term:
// it composes the lexical document, computes the tsvector in SQL, computes the
// embedding best-effort (semantic search enabled AND an embedder available), and
// upserts on class_term_id. Idempotent — repeating it for an unchanged class
// produces the same row (modulo updated_at).
func (s Store) Reindex(ctx context.Context, classTermID string, embed EmbedFunc) error {
	classTermID = strings.TrimSpace(classTermID)
	if classTermID == "" {
		return errors.New("class_term_id is empty")
	}
	if s.DB == nil {
		return errors.New("db is nil")
	}

	doc, meta, err := s.buildDocument(ctx, classTermID)
	if err != nil {
		return err
	}

	var (
		embeddingLiteral any // nil => SQL NULL
		embeddingText    any // nil => SQL NULL
	)
	if kbsearch.SemanticSearchEnabled() && embed != nil && strings.TrimSpace(doc) != "" {
		if vec, ok := embed(ctx, truncateRunes(doc, docMaxRunes)); ok && len(vec) == kbsearch.ConfiguredEmbeddingDim() {
			embeddingLiteral = kbsearch.FormatVectorLiteral(vec)
			embeddingText = truncateRunes(doc, docMaxRunes)
		}
	}

	_, err = s.DB.ExecContext(ctx, `
INSERT INTO kb.ontology_class_contract_search (
    class_term_id, current_contract_revision_id, definition_state, module_id,
    search_document, search_vector, embedding_text, embedding, instance_count, updated_at
) VALUES (
    $1, $2, $3, $4, $5, to_tsvector('simple', $5), $6, $7::vector, $8, NOW()
)
ON CONFLICT (class_term_id) DO UPDATE SET
    current_contract_revision_id = EXCLUDED.current_contract_revision_id,
    definition_state = EXCLUDED.definition_state,
    module_id = EXCLUDED.module_id,
    search_document = EXCLUDED.search_document,
    search_vector = EXCLUDED.search_vector,
    embedding_text = EXCLUDED.embedding_text,
    embedding = EXCLUDED.embedding,
    instance_count = EXCLUDED.instance_count,
    updated_at = NOW()`,
		classTermID, meta.currentRevID, definitionStateOrDefault(meta.definitionState),
		emptyToNil(meta.moduleID), doc, embeddingText, embeddingLiteral, meta.instanceCount)
	if err != nil {
		return fmt.Errorf("upsert class contract search row for %s: %w", classTermID, err)
	}
	return nil
}

// buildDocument composes the lexical document (design D7) and the row metadata.
func (s Store) buildDocument(ctx context.Context, classTermID string) (string, docMeta, error) {
	var meta docMeta

	err := s.DB.QueryRowContext(ctx, `
SELECT COALESCE(module_id, ''), current_contract_revision_id
FROM kb.ontology_term_headers
WHERE term_id = $1`, classTermID).Scan(&meta.moduleID, &meta.currentRevID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", meta, fmt.Errorf("load class header for %s: %w", classTermID, err)
	}

	var contractPayload string
	if meta.currentRevID.Valid {
		err := s.DB.QueryRowContext(ctx, `
SELECT definition_state, COALESCE(contract_payload::text, '{}')
FROM kb.ontology_class_contract_revisions
WHERE id = $1`, meta.currentRevID.Int64).Scan(&meta.definitionState, &contractPayload)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", meta, fmt.Errorf("load current contract revision %d: %w", meta.currentRevID.Int64, err)
		}
	}

	var definition, scope string
	// No row -> both stay empty; that is expected for a synthesized class term.
	_ = s.DB.QueryRowContext(ctx, `
SELECT COALESCE(definition, ''), COALESCE(scope, '')
FROM kb.ontology_terms
WHERE term_id = $1
ORDER BY version DESC
LIMIT 1`, classTermID).Scan(&definition, &scope)

	names, err := s.instanceNames(ctx, classTermID)
	if err != nil {
		return "", meta, err
	}
	meta.instanceCount, err = s.instanceCount(ctx, classTermID)
	if err != nil {
		return "", meta, err
	}

	parts := []string{classTermID, meta.moduleID, definition, scope, meta.definitionState}
	parts = append(parts, contractFacets(contractPayload)...)
	parts = append(parts, names...)
	return strings.Join(dedupeNonEmpty(parts), "\n"), meta, nil
}

func (s Store) instanceNames(ctx context.Context, classTermID string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT DISTINCT COALESCE(m.metric_name, ''), COALESCE(m.metric_name_en, ''), COALESCE(m.metric_subject, '')
FROM kb.semantic_assertions a
JOIN kb.semantic_decision_candidates dc
     ON dc.resulting_assertion_id = a.id AND dc.source_artifact_type = 'metric'
JOIN kb.metrics m ON m.metric_id = dc.source_artifact_id
WHERE a.instance_of_term_id = $1
LIMIT $2`, classTermID, instanceSampleRows)
	if err != nil {
		return nil, fmt.Errorf("load instance names for %s: %w", classTermID, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name, nameEn, subject string
		if err := rows.Scan(&name, &nameEn, &subject); err != nil {
			return nil, err
		}
		out = append(out, name, nameEn, subject)
	}
	return out, rows.Err()
}

func (s Store) instanceCount(ctx context.Context, classTermID string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT m.metric_id)
FROM kb.semantic_assertions a
JOIN kb.semantic_decision_candidates dc
     ON dc.resulting_assertion_id = a.id AND dc.source_artifact_type = 'metric'
JOIN kb.metrics m ON m.metric_id = dc.source_artifact_id
WHERE a.instance_of_term_id = $1`, classTermID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count instances for %s: %w", classTermID, err)
	}
	return n, nil
}

// MatchSimilar returns up to k class terms ranked by similarity to classTermID,
// as Reciprocal Rank Fusion (constant rrfK) of a lexical candidate list
// (ts_rank_cd of search_vector against classTermID's document lexemes) and a
// semantic candidate list (pgvector cosine of embedding). classTermID is
// excluded from its own results. When the stored embedding is absent it embeds
// the class's document on the fly; when no embedding is available at all it
// falls back to the lexical list alone. Returns nil when classTermID has no
// indexed row.
func (s Store) MatchSimilar(ctx context.Context, classTermID string, k int, embed EmbedFunc) ([]Match, error) {
	classTermID = strings.TrimSpace(classTermID)
	if classTermID == "" {
		return nil, errors.New("class_term_id is empty")
	}
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	if k <= 0 {
		k = 20
	}

	var (
		doc       string
		storedVec sql.NullString
	)
	err := s.DB.QueryRowContext(ctx, `
SELECT search_document, embedding::text
FROM kb.ontology_class_contract_search
WHERE class_term_id = $1`, classTermID).Scan(&doc, &storedVec)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load class contract search row for %s: %w", classTermID, err)
	}

	lexQuery := lexemeQuery(doc)
	useLexical := lexQuery != ""

	var vecLiteral string
	if storedVec.Valid && strings.TrimSpace(storedVec.String) != "" {
		vecLiteral = storedVec.String
	} else if kbsearch.SemanticSearchEnabled() && embed != nil && strings.TrimSpace(doc) != "" {
		if vec, ok := embed(ctx, truncateRunes(doc, docMaxRunes)); ok && len(vec) == kbsearch.ConfiguredEmbeddingDim() {
			vecLiteral = kbsearch.FormatVectorLiteral(vec)
		}
	}
	useSemantic := vecLiteral != ""

	if !useLexical && !useSemantic {
		return nil, nil
	}

	sqlText, args := buildMatchQuery(classTermID, lexQuery, vecLiteral, useLexical, useSemantic, k)
	rows, err := s.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("similar-class match query for %s: %w", classTermID, err)
	}
	defer rows.Close()
	var out []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.ClassTermID, &m.Score); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// buildMatchQuery assembles the RRF query for whichever of the two candidate
// lists are available. Placeholders: $1 = self class_term_id, then (when used)
// the lexeme query text and the vector literal, then k.
func buildMatchQuery(selfID, lexQuery, vecLiteral string, useLexical, useSemantic bool, k int) (string, []any) {
	args := []any{selfID}
	next := 2

	lexCTE := ""
	if useLexical {
		args = append(args, lexQuery)
		q := next
		next++
		lexCTE = fmt.Sprintf(`
lexical AS (
    SELECT class_term_id,
           ROW_NUMBER() OVER (ORDER BY ts_rank_cd(search_vector, to_tsquery('simple', $%d)) DESC, class_term_id ASC) AS rnk
    FROM kb.ontology_class_contract_search
    WHERE class_term_id <> $1
      AND search_vector @@ to_tsquery('simple', $%d)
    ORDER BY rnk
    LIMIT %d
)`, q, q, candidateLimit)
	}

	semCTE := ""
	if useSemantic {
		args = append(args, vecLiteral)
		v := next
		next++
		semCTE = fmt.Sprintf(`
semantic AS (
    SELECT class_term_id,
           ROW_NUMBER() OVER (ORDER BY embedding <=> $%d::vector ASC, class_term_id ASC) AS rnk
    FROM kb.ontology_class_contract_search
    WHERE class_term_id <> $1
      AND embedding IS NOT NULL
    ORDER BY rnk
    LIMIT %d
)`, v, candidateLimit)
	}

	args = append(args, k)
	kArg := next

	var body string
	switch {
	case useLexical && useSemantic:
		body = fmt.Sprintf(`
SELECT COALESCE(l.class_term_id, s.class_term_id) AS class_term_id,
       COALESCE(1.0 / (%d + l.rnk), 0.0) + COALESCE(1.0 / (%d + s.rnk), 0.0) AS score
FROM lexical l
FULL OUTER JOIN semantic s ON l.class_term_id = s.class_term_id
ORDER BY score DESC, class_term_id ASC
LIMIT $%d`, rrfK, rrfK, kArg)
		return "WITH" + lexCTE + "," + semCTE + body, args
	case useLexical:
		body = fmt.Sprintf(`
SELECT class_term_id, 1.0 / (%d + rnk) AS score
FROM lexical
ORDER BY score DESC, class_term_id ASC
LIMIT $%d`, rrfK, kArg)
		return "WITH" + lexCTE + body, args
	default:
		body = fmt.Sprintf(`
SELECT class_term_id, 1.0 / (%d + rnk) AS score
FROM semantic
ORDER BY score DESC, class_term_id ASC
LIMIT $%d`, rrfK, kArg)
		return "WITH" + semCTE + body, args
	}
}

// lexemeQuery reduces a search document to an OR-joined to_tsquery('simple', ...)
// argument: the distinct alphanumeric/CJK tokens (>=2 runes), capped. Empty when
// the document has no usable token, in which case the lexical half is skipped.
func lexemeQuery(doc string) string {
	seen := make(map[string]struct{})
	var toks []string
	for _, f := range strings.FieldsFunc(doc, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		f = strings.ToLower(strings.TrimSpace(f))
		if len([]rune(f)) < 2 {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		toks = append(toks, f)
		if len(toks) >= lexemeCap {
			break
		}
	}
	return strings.Join(toks, " | ")
}

// contractFacets pulls the comparable scalar facets out of a contract payload
// (design D7). A malformed or empty payload yields nothing, not an error.
func contractFacets(payload string) []string {
	payload = strings.TrimSpace(payload)
	if payload == "" || payload == "{}" || payload == "null" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return nil
	}
	var out []string
	if v, ok := m["value_type"].(string); ok && strings.TrimSpace(v) != "" {
		out = append(out, v)
	}
	if arr, ok := m["permitted_unit_term_ids"].([]any); ok {
		for _, e := range arr {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func dedupeNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func emptyToNil(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func definitionStateOrDefault(s string) string {
	if strings.TrimSpace(s) == "" {
		return "identity_only"
	}
	return s
}
