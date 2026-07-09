package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// ReviewTool is one document-intrinsic tool a tool-use reviewer may call during
// the managed conversation loop (DR10a). Every tool is record-scoped: its
// Execute queries only artifacts whose input_record_id matches the document
// under review, so the "document-intrinsic" guarantee is enforced structurally
// rather than by prompt convention.
type ReviewTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema for the tool arguments
	Execute     func(ctx context.Context, recordID int64, args map[string]any) (any, error)
}

// toolDB is the database handle the tools query. It defaults to the project DB
// handle but is injectable for tests.
type toolDB struct {
	db *sql.DB
}

const defaultToolSearchLimit = 10

// buildToolRegistry constructs the document-intrinsic core tools (DR10a) bound
// to a single record's database handle, plus the one cross-record tool
// `get_artifact_context` (ADR 2026070201 AR4). The returned map is keyed by
// tool name. The cross-record tool is NOT part of coreToolNames: reviewers get
// it only by listing it explicitly in their TOML `tools`, and it is scoped to
// read-only line access.
func buildToolRegistry(db *sql.DB) map[string]ReviewTool {
	t := toolDB{db: db}
	tools := []ReviewTool{
		t.searchEntitiesTool(),
		t.getEntityTool(),
		t.getEntityRelationsTool(),
		t.searchMetricsTool(),
		t.getMetricTool(),
		t.searchProvisionsTool(),
		t.getProvisionTool(),
		t.getChunkSummaryTool(),
		t.getChunkLinesTool(),
		t.getArtifactContextTool(),
		t.getDocumentMetadataTool(),
	}
	reg := make(map[string]ReviewTool, len(tools))
	for _, tool := range tools {
		reg[tool.Name] = tool
	}
	return reg
}

// defaultToolRegistry builds the registry over the shared project DB handle.
func defaultToolRegistry() map[string]ReviewTool {
	return buildToolRegistry(ApiTypes.ProjectDBHandle)
}

// coreToolNames is the canonical ordered list of the nine core tools.
var coreToolNames = []string{
	"search_entities", "get_entity", "get_entity_relations",
	"search_metrics", "get_metric",
	"search_provisions", "get_provision",
	"get_chunk_summary", "get_chunk_lines",
}

// selectTools returns the subset of the registry a reviewer requested. An empty
// names slice selects all nine core tools (in canonical order). Unknown names
// are skipped.
func selectTools(registry map[string]ReviewTool, names []string) []ReviewTool {
	if len(names) == 0 {
		names = coreToolNames
	}
	out := make([]ReviewTool, 0, len(names))
	for _, n := range names {
		if tool, ok := registry[strings.TrimSpace(n)]; ok {
			out = append(out, tool)
		}
	}
	return out
}

// ── Argument helpers ─────────────────────────────────────────────────────────

func argString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	return strings.TrimSpace(asString(args[key]))
}

func argLimit(args map[string]any) int {
	if args == nil {
		return defaultToolSearchLimit
	}
	switch v := args["limit"].(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	}
	return defaultToolSearchLimit
}

// rowsToMaps scans all rows into a slice of column→value maps. JSONB/[]byte
// columns are returned as raw JSON so the LLM sees structured data.
func rowsToMaps(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = normalizeColValue(vals[i])
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// normalizeColValue turns []byte (text/JSONB) into a string or parsed JSON so
// the result serializes cleanly into the tool-result message.
func normalizeColValue(v any) any {
	b, ok := v.([]byte)
	if !ok {
		return v
	}
	s := string(b)
	trimmed := strings.TrimSpace(s)
	if trimmed != "" && (trimmed[0] == '{' || trimmed[0] == '[') {
		var parsed any
		if err := json.Unmarshal(b, &parsed); err == nil {
			return parsed
		}
	}
	return s
}

// ── Entity tools ─────────────────────────────────────────────────────────────

func (t toolDB) searchEntitiesTool() ReviewTool {
	return ReviewTool{
		Name:        "search_entities",
		Description: "Full-text search entities extracted from the document under review. Returns matching entity rows (entity_id, entity, entity_type, aliases, description, line_spans).",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"query":{"type":"string","description":"search terms"},` +
			`"limit":{"type":"integer","description":"max results (default 10)"}},` +
			`"required":["query"]}`),
		Execute: func(ctx context.Context, recordID int64, args map[string]any) (any, error) {
			q := argString(args, "query")
			if q == "" {
				return nil, fmt.Errorf("search_entities: query is required")
			}
			const sqlText = `
				SELECT entity_id, entity, entity_en, entity_type, aliases, desc_text, line_spans, confidence
				FROM kb.entities
				WHERE input_record_id = $1
				  AND (search_document ILIKE '%' || $2 || '%'
				       OR entity ILIKE '%' || $2 || '%'
				       OR COALESCE(entity_en,'') ILIKE '%' || $2 || '%')
				ORDER BY confidence DESC NULLS LAST
				LIMIT $3`
			rows, err := t.db.QueryContext(ctx, sqlText, recordID, q, argLimit(args))
			if err != nil {
				return nil, fmt.Errorf("search_entities query: %w", err)
			}
			defer rows.Close()
			return rowsToMaps(rows)
		},
	}
}

func (t toolDB) getEntityTool() ReviewTool {
	return ReviewTool{
		Name:        "get_entity",
		Description: "Fetch one entity by its entity_id from the document under review.",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"entity_id":{"type":"string","description":"the entity_id to fetch"}},` +
			`"required":["entity_id"]}`),
		Execute: func(ctx context.Context, recordID int64, args map[string]any) (any, error) {
			id := argString(args, "entity_id")
			if id == "" {
				return nil, fmt.Errorf("get_entity: entity_id is required")
			}
			const sqlText = `
				SELECT entity_id, entity, entity_en, entity_type, aliases, aliases_en,
				       desc_text, desc_text_en, keywords, line_spans, confidence
				FROM kb.entities
				WHERE input_record_id = $1 AND entity_id = $2
				LIMIT 1`
			rows, err := t.db.QueryContext(ctx, sqlText, recordID, id)
			if err != nil {
				return nil, fmt.Errorf("get_entity query: %w", err)
			}
			defer rows.Close()
			out, err := rowsToMaps(rows)
			if err != nil {
				return nil, err
			}
			if len(out) == 0 {
				return map[string]any{"found": false, "entity_id": id}, nil
			}
			return out[0], nil
		},
	}
}

func (t toolDB) getEntityRelationsTool() ReviewTool {
	return ReviewTool{
		Name:        "get_entity_relations",
		Description: "All relations involving an entity (as subject or object) in the document under review. Returns subject/predicate/object triples with line_spans and confidence.",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"entity_id":{"type":"string","description":"the entity_id whose relations to fetch"},` +
			`"limit":{"type":"integer","description":"max results (default 10)"}},` +
			`"required":["entity_id"]}`),
		Execute: func(ctx context.Context, recordID int64, args map[string]any) (any, error) {
			id := argString(args, "entity_id")
			if id == "" {
				return nil, fmt.Errorf("get_entity_relations: entity_id is required")
			}
			// First resolve the entity's surface name so we can match relation
			// endpoints (relations store subject/object by name, not id).
			var name string
			err := t.db.QueryRowContext(ctx,
				`SELECT COALESCE(NULLIF(entity,''), COALESCE(entity_en,'')) FROM kb.entities
				 WHERE input_record_id = $1 AND entity_id = $2 LIMIT 1`,
				recordID, id).Scan(&name)
			if err != nil && err != sql.ErrNoRows {
				return nil, fmt.Errorf("get_entity_relations resolve name: %w", err)
			}
			if strings.TrimSpace(name) == "" {
				return map[string]any{"found": false, "entity_id": id, "relations": []any{}}, nil
			}
			const sqlText = `
				SELECT relation_id, subject, predicate, object, desc_text, line_spans, confidence
				FROM kb.relations
				WHERE input_record_id = $1
				  AND ($2 IN (subject, object) OR subject ILIKE '%' || $2 || '%' OR object ILIKE '%' || $2 || '%')
				ORDER BY confidence DESC NULLS LAST
				LIMIT $3`
			rows, err := t.db.QueryContext(ctx, sqlText, recordID, name, argLimit(args))
			if err != nil {
				return nil, fmt.Errorf("get_entity_relations query: %w", err)
			}
			defer rows.Close()
			rels, err := rowsToMaps(rows)
			if err != nil {
				return nil, err
			}
			return map[string]any{"entity_id": id, "entity": name, "relations": rels}, nil
		},
	}
}

// ── Metric tools ─────────────────────────────────────────────────────────────

func (t toolDB) searchMetricsTool() ReviewTool {
	return ReviewTool{
		Name:        "search_metrics",
		Description: "Full-text search metrics extracted from the document under review. Returns metric rows (metric_id, metric_name, value, unit, threshold, source_line_spans).",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"query":{"type":"string","description":"search terms"},` +
			`"limit":{"type":"integer","description":"max results (default 10)"}},` +
			`"required":["query"]}`),
		Execute: func(ctx context.Context, recordID int64, args map[string]any) (any, error) {
			q := argString(args, "query")
			if q == "" {
				return nil, fmt.Errorf("search_metrics: query is required")
			}
			const sqlText = `
				SELECT metric_id, metric_name, metric_name_en, metric_value, metric_unit,
				       threshold_or_target, metric_desc, source_line_spans, confidence
				FROM kb.metrics
				WHERE input_record_id = $1
				  AND (search_document ILIKE '%' || $2 || '%'
				       OR metric_name ILIKE '%' || $2 || '%'
				       OR COALESCE(metric_name_en,'') ILIKE '%' || $2 || '%')
				ORDER BY confidence DESC NULLS LAST
				LIMIT $3`
			rows, err := t.db.QueryContext(ctx, sqlText, recordID, q, argLimit(args))
			if err != nil {
				return nil, fmt.Errorf("search_metrics query: %w", err)
			}
			defer rows.Close()
			return rowsToMaps(rows)
		},
	}
}

func (t toolDB) getMetricTool() ReviewTool {
	return ReviewTool{
		Name:        "get_metric",
		Description: "Fetch one metric by its metric_id from the document under review.",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"metric_id":{"type":"string","description":"the metric_id to fetch"}},` +
			`"required":["metric_id"]}`),
		Execute: func(ctx context.Context, recordID int64, args map[string]any) (any, error) {
			id := argString(args, "metric_id")
			if id == "" {
				return nil, fmt.Errorf("get_metric: metric_id is required")
			}
			const sqlText = `
				SELECT metric_id, metric_name, metric_name_en, metric_subject, metric_desc,
				       metric_context, metric_value, metric_unit, value_range_type,
				       formula_or_definition, threshold_or_target, measurement_frequency,
				       source_line_spans, confidence
				FROM kb.metrics
				WHERE input_record_id = $1 AND metric_id = $2
				LIMIT 1`
			rows, err := t.db.QueryContext(ctx, sqlText, recordID, id)
			if err != nil {
				return nil, fmt.Errorf("get_metric query: %w", err)
			}
			defer rows.Close()
			out, err := rowsToMaps(rows)
			if err != nil {
				return nil, err
			}
			if len(out) == 0 {
				return map[string]any{"found": false, "metric_id": id}, nil
			}
			return out[0], nil
		},
	}
}

// ── Provision tools ──────────────────────────────────────────────────────────

func (t toolDB) searchProvisionsTool() ReviewTool {
	return ReviewTool{
		Name:        "search_provisions",
		Description: "Full-text search provisions (normative requirements) extracted from the document under review. Returns provision rows (prov_id, prov_name, provision_type, text, source_line_spans).",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"query":{"type":"string","description":"search terms"},` +
			`"limit":{"type":"integer","description":"max results (default 10)"}},` +
			`"required":["query"]}`),
		Execute: func(ctx context.Context, recordID int64, args map[string]any) (any, error) {
			q := argString(args, "query")
			if q == "" {
				return nil, fmt.Errorf("search_provisions: query is required")
			}
			const sqlText = `
				SELECT prov_id,
				       COALESCE(NULLIF(TRIM(prov_name),''),'provision') AS prov_name,
				       COALESCE(TRIM(provision_type),'') AS provision_type,
				       COALESCE(NULLIF(TRIM(prov_desc),''), NULLIF(TRIM(provision),''), NULLIF(TRIM(source_text),''),'') AS provision_text,
				       COALESCE(source_line_spans,'[]'::jsonb) AS source_line_spans
				FROM kb.provisions
				WHERE input_record_id = $1
				  AND (COALESCE(prov_name,'') ILIKE '%' || $2 || '%'
				       OR COALESCE(prov_desc,'') ILIKE '%' || $2 || '%'
				       OR COALESCE(provision,'') ILIKE '%' || $2 || '%'
				       OR COALESCE(source_text,'') ILIKE '%' || $2 || '%')
				LIMIT $3`
			rows, err := t.db.QueryContext(ctx, sqlText, recordID, q, argLimit(args))
			if err != nil {
				return nil, fmt.Errorf("search_provisions query: %w", err)
			}
			defer rows.Close()
			return rowsToMaps(rows)
		},
	}
}

func (t toolDB) getProvisionTool() ReviewTool {
	return ReviewTool{
		Name:        "get_provision",
		Description: "Fetch one provision by its prov_id from the document under review.",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"provision_id":{"type":"string","description":"the prov_id to fetch"}},` +
			`"required":["provision_id"]}`),
		Execute: func(ctx context.Context, recordID int64, args map[string]any) (any, error) {
			id := argString(args, "provision_id")
			if id == "" {
				return nil, fmt.Errorf("get_provision: provision_id is required")
			}
			const sqlText = `
				SELECT prov_id,
				       COALESCE(NULLIF(TRIM(prov_name),''),'provision') AS prov_name,
				       COALESCE(TRIM(provision_type),'') AS provision_type,
				       COALESCE(NULLIF(TRIM(prov_desc),''), NULLIF(TRIM(provision),''), NULLIF(TRIM(source_text),''),'') AS provision_text,
				       COALESCE(source_line_spans,'[]'::jsonb) AS source_line_spans
				FROM kb.provisions
				WHERE input_record_id = $1 AND prov_id = $2
				LIMIT 1`
			rows, err := t.db.QueryContext(ctx, sqlText, recordID, id)
			if err != nil {
				return nil, fmt.Errorf("get_provision query: %w", err)
			}
			defer rows.Close()
			out, err := rowsToMaps(rows)
			if err != nil {
				return nil, err
			}
			if len(out) == 0 {
				return map[string]any{"found": false, "provision_id": id}, nil
			}
			return out[0], nil
		},
	}
}

// ── Chunk tools ──────────────────────────────────────────────────────────────

func (t toolDB) getChunkSummaryTool() ReviewTool {
	return ReviewTool{
		Name:        "get_chunk_summary",
		Description: "Fetch a chunk/section summary by summary_id from the document under review, with its previous and next sibling summaries for context.",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"chunk_id":{"type":"string","description":"the summary_id to fetch (use the summary_id from a line_span or omit to list level-0 summaries)"}},` +
			`"required":[]}`),
		Execute: func(ctx context.Context, recordID int64, args map[string]any) (any, error) {
			id := argString(args, "chunk_id")
			if id == "" {
				// No id: return the level-0 summaries in order (a section map).
				const listSQL = `
					SELECT summary_id, summary_seq_no, summary_text, source_line_spans
					FROM kb.summaries
					WHERE input_record_id = $1 AND summary_level = 0
					ORDER BY summary_seq_no ASC
					LIMIT 50`
				rows, err := t.db.QueryContext(ctx, listSQL, recordID)
				if err != nil {
					return nil, fmt.Errorf("get_chunk_summary list: %w", err)
				}
				defer rows.Close()
				return rowsToMaps(rows)
			}
			var level, seq int
			err := t.db.QueryRowContext(ctx,
				`SELECT summary_level, summary_seq_no FROM kb.summaries
				 WHERE input_record_id = $1 AND summary_id = $2 LIMIT 1`,
				recordID, id).Scan(&level, &seq)
			if err == sql.ErrNoRows {
				return map[string]any{"found": false, "chunk_id": id}, nil
			}
			if err != nil {
				return nil, fmt.Errorf("get_chunk_summary locate: %w", err)
			}
			const sibSQL = `
				SELECT summary_id, summary_seq_no, summary_text, source_line_spans
				FROM kb.summaries
				WHERE input_record_id = $1 AND summary_level = $2
				  AND summary_seq_no BETWEEN $3 - 1 AND $3 + 1
				ORDER BY summary_seq_no ASC`
			rows, err := t.db.QueryContext(ctx, sibSQL, recordID, level, seq)
			if err != nil {
				return nil, fmt.Errorf("get_chunk_summary siblings: %w", err)
			}
			defer rows.Close()
			sibs, err := rowsToMaps(rows)
			if err != nil {
				return nil, err
			}
			return map[string]any{"chunk_id": id, "summaries": sibs}, nil
		},
	}
}

func (t toolDB) getChunkLinesTool() ReviewTool {
	return ReviewTool{
		Name:        "get_chunk_lines",
		Description: "Fetch the raw extracted lines of the document under review for a given line-number range (for deep-dive into a specific passage).",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"start_line":{"type":"integer","description":"first line number (inclusive)"},` +
			`"end_line":{"type":"integer","description":"last line number (inclusive)"}},` +
			`"required":["start_line","end_line"]}`),
		Execute: func(ctx context.Context, recordID int64, args map[string]any) (any, error) {
			start := intArg(args, "start_line")
			end := intArg(args, "end_line")
			if end < start {
				start, end = end, start
			}
			lines, err := loadRecordLines(ctx, recordID)
			if err != nil {
				return nil, err
			}
			var out []map[string]any
			for _, ln := range lines {
				if ln.LineNo >= start && ln.LineNo <= end {
					out = append(out, map[string]any{
						"line_number": ln.LineNo,
						"content":     ln.Content,
					})
				}
			}
			return map[string]any{"start_line": start, "end_line": end, "lines": out}, nil
		},
	}
}

// ── Cross-record artifact context tool (ADR 2026070201 AR4) ─────────────────

// artifactContextRadius is the number of lines returned on each side of an
// artifact's line spans by get_artifact_context.
const artifactContextRadius = 10

// artifactContextMaxLines caps the total lines one call may return.
const artifactContextMaxLines = 120

// getArtifactContextTool returns the source passage around a matched
// artifact's line spans in its OWN document. Unlike the record-scoped core
// tools it is cross-record (the screen-then-verify pattern: structured views
// screen candidates, source context verifies the survivors). It is strictly
// read-only line access; it may also be called on the artifact under review
// itself, e.g. to retrieve context truncated at a window boundary (AR2).
func (t toolDB) getArtifactContextTool() ReviewTool {
	return ReviewTool{
		Name: "get_artifact_context",
		Description: "Fetch the source lines around an artifact's line spans in its source document. " +
			"Cross-record: pass the source_record_id and artifact id (metric_id, prov_id, inventory_item_id, or entity_id) " +
			"of a matched artifact to see the passage it was extracted from.",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"record_id":{"type":"integer","description":"the artifact's source_record_id"},` +
			`"artifact_id":{"type":"string","description":"the artifact id (metric_id, prov_id, inventory_item_id, or entity_id)"}},` +
			`"required":["record_id","artifact_id"]}`),
		Execute: func(ctx context.Context, _ int64, args map[string]any) (any, error) {
			targetRecord := int64(intArg(args, "record_id"))
			artifactID := argString(args, "artifact_id")
			if targetRecord <= 0 {
				return nil, fmt.Errorf("get_artifact_context: record_id is required")
			}
			if artifactID == "" {
				return nil, fmt.Errorf("get_artifact_context: artifact_id is required")
			}
			spans, artifactType, err := t.lookupArtifactSpans(ctx, targetRecord, artifactID)
			if err != nil {
				return nil, err
			}
			if artifactType == "" {
				return map[string]any{"found": false, "record_id": targetRecord, "artifact_id": artifactID}, nil
			}
			lines, err := loadRecordLines(ctx, targetRecord)
			if err != nil {
				return nil, fmt.Errorf("get_artifact_context: %w", err)
			}
			include := make(map[int]bool)
			for _, s := range spans {
				start, end := parseArtifactSpan(s)
				if start == 0 {
					continue
				}
				for n := start - artifactContextRadius; n <= end+artifactContextRadius; n++ {
					include[n] = true
				}
			}
			var out []map[string]any
			for _, ln := range lines {
				if include[ln.LineNo] {
					out = append(out, map[string]any{
						"line_number": ln.LineNo,
						"content":     ln.Content,
					})
					if len(out) >= artifactContextMaxLines {
						break
					}
				}
			}
			return map[string]any{
				"found":         true,
				"record_id":     targetRecord,
				"artifact_id":   artifactID,
				"artifact_type": artifactType,
				"line_spans":    spans,
				"lines":         out,
			}, nil
		},
	}
}

func (t toolDB) getDocumentMetadataTool() ReviewTool {
	return ReviewTool{
		Name: "get_document_metadata",
		Description: "Fetch compact document metadata for a knowledge-base record. " +
			"Use this to verify authority, edition/currency, publication or implementation dates, jurisdiction, title, and document number.",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"record_id":{"type":"integer","description":"the kb.inputs record id"}},` +
			`"required":["record_id"]}`),
		Execute: func(ctx context.Context, _ int64, args map[string]any) (any, error) {
			targetRecord := int64(intArg(args, "record_id"))
			if targetRecord <= 0 {
				return nil, fmt.Errorf("get_document_metadata: record_id is required")
			}
			const q = `
SELECT id, COALESCE(title, ''), COALESCE(doc_no, ''), COALESCE(staging_filename, ''),
       COALESCE(file_name, ''), COALESCE(doc_metadata, '{}'::jsonb)::text
FROM kb.inputs
WHERE id = $1
LIMIT 1`
			var (
				id             int64
				title          string
				docNo          string
				stagingName    string
				fileName       string
				docMetadataRaw string
			)
			err := t.db.QueryRowContext(ctx, q, targetRecord).Scan(&id, &title, &docNo, &stagingName, &fileName, &docMetadataRaw)
			if err == sql.ErrNoRows {
				return map[string]any{"found": false, "record_id": targetRecord}, nil
			}
			if err != nil {
				return nil, fmt.Errorf("get_document_metadata query: %w", err)
			}
			metadata := map[string]any{}
			if strings.TrimSpace(docMetadataRaw) != "" {
				if err := json.Unmarshal([]byte(docMetadataRaw), &metadata); err != nil {
					return nil, fmt.Errorf("get_document_metadata decode doc_metadata: %w", err)
				}
			}
			if docNo == "" {
				docNo = strings.TrimSpace(asString(metadata["doc_no"]))
			}
			if title == "" {
				title = strings.TrimSpace(asString(metadata["title"]))
			}
			filename := strings.TrimSpace(fileName)
			if filename == "" {
				filename = strings.TrimSpace(stagingName)
			}
			out := map[string]any{
				"found":            true,
				"record_id":        id,
				"title":            title,
				"doc_no":           docNo,
				"filename":         filename,
				"staging_filename": stagingName,
				"file_name":        fileName,
				"authority_class":  docAuthorityClass(docNo, title, filename),
				"doc_metadata":     metadata,
			}
			for _, key := range []string{"publish_date", "implementation_date", "language"} {
				if value := strings.TrimSpace(asString(metadata[key])); value != "" {
					out[key] = value
				}
			}
			for _, key := range []string{"authors", "main_drafting_persons", "drafting_persons"} {
				if value, ok := metadata[key]; ok {
					out[key] = value
				}
			}
			return out, nil
		},
	}
}

// lookupArtifactSpans finds an artifact's line spans across the four artifact
// tables. Returns ("", nil) with no error when the id is unknown.
func (t toolDB) lookupArtifactSpans(ctx context.Context, recordID int64, artifactID string) ([]string, string, error) {
	const q = `
SELECT 'metric', COALESCE(source_line_spans, '[]'::jsonb)::text
FROM kb.metrics WHERE input_record_id = $1 AND metric_id = $2
UNION ALL
SELECT 'provision', COALESCE(source_line_spans, '[]'::jsonb)::text
FROM kb.provisions WHERE input_record_id = $1 AND prov_id = $2
UNION ALL
SELECT 'inventory_item', COALESCE(source_line_spans, '[]'::jsonb)::text
FROM kb.inventory_items WHERE input_record_id = $1 AND inventory_item_id = $2
UNION ALL
SELECT 'entity', COALESCE(line_spans, '[]'::jsonb)::text
FROM kb.entities WHERE input_record_id = $1 AND entity_id = $2
LIMIT 1`
	var artifactType, spansJSON string
	err := t.db.QueryRowContext(ctx, q, recordID, artifactID).Scan(&artifactType, &spansJSON)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("get_artifact_context lookup: %w", err)
	}
	return parseJSONStringArray([]byte(spansJSON)), artifactType, nil
}

func intArg(args map[string]any, key string) int {
	if args == nil {
		return 0
	}
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		var n int
		fmt.Sscanf(strings.TrimSpace(v), "%d", &n)
		return n
	}
	return 0
}

// loadRecordLines resolves and parses the document's extracted line-file. It
// mirrors the path the chunk reviewers already use to read line content.
func loadRecordLines(ctx context.Context, recordID int64) ([]Line, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("get_chunk_lines load record %d: %w", recordID, err)
	}
	path, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename, rec.ParserName, rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("get_chunk_lines resolve line file: %w", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("get_chunk_lines read line file: %w", err)
	}
	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("get_chunk_lines parse line file: %w", err)
	}
	return lines, nil
}
