package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/lib/pq"
)

// ConnectionStore persists artifact-graph edges (kb.artifact_connections).
type ConnectionStore interface {
	// ReplaceConnections idempotently replaces a document's edges for the given
	// relation_method and relation_names: it deletes the existing rows in that
	// scope and inserts conns. Safe to call on every (re)process.
	ReplaceConnections(ctx context.Context, inputRecordID int64, relationMethod string, relationNames []string, conns []Connection) error
}

// ConnectionSQLStore is the PostgreSQL-backed ConnectionStore.
type ConnectionSQLStore struct {
	DB *sql.DB
}

const connectionInsertSQL = `
INSERT INTO kb.artifact_connections
    (source_record_id, target_record_id, source_type, source_id, target_type, target_id,
     relation_name, relation_method, confidence, overlap, provenance,
     semantic_signature, source_desc, target_desc, extra_info)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT (relation_method, source_type, source_id, target_type, target_id, relation_name)
DO UPDATE SET
    source_record_id   = EXCLUDED.source_record_id,
    target_record_id   = EXCLUDED.target_record_id,
    confidence         = EXCLUDED.confidence,
    overlap            = EXCLUDED.overlap,
    provenance         = EXCLUDED.provenance,
    semantic_signature = EXCLUDED.semantic_signature,
    source_desc        = EXCLUDED.source_desc,
    target_desc        = EXCLUDED.target_desc,
    extra_info         = EXCLUDED.extra_info,
    create_time        = NOW()
`

// ReplaceConnections deletes the document's edges in the (relation_method,
// relation_names) scope and inserts the supplied edges, all in one transaction.
func (s *ConnectionSQLStore) ReplaceConnections(ctx context.Context, inputRecordID int64, relationMethod string, relationNames []string, conns []Connection) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("(CWB_CONN_001) nil connection store db handle")
	}
	if len(relationNames) == 0 {
		return fmt.Errorf("(CWB_CONN_002) relationNames must be non-empty for scope-delete")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("(CWB_CONN_003) begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM kb.artifact_connections
		 WHERE source_record_id = $1 AND target_record_id = $1
		   AND relation_method = $2 AND relation_name = ANY($3)`,
		inputRecordID, relationMethod, pq.Array(relationNames),
	); err != nil {
		return fmt.Errorf("(CWB_CONN_004) delete existing connections: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, connectionInsertSQL)
	if err != nil {
		return fmt.Errorf("(CWB_CONN_005) prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, c := range conns {
		overlap, err := encodeJSONB(c.Overlap)
		if err != nil {
			return fmt.Errorf("(CWB_CONN_006) encode overlap: %w", err)
		}
		provenance, err := encodeJSONB(c.Provenance)
		if err != nil {
			return fmt.Errorf("(CWB_CONN_007) encode provenance: %w", err)
		}
		extraInfo, err := encodeJSONB(c.ExtraInfo)
		if err != nil {
			return fmt.Errorf("(CWB_CONN_008) encode extra_info: %w", err)
		}
		var confidence any
		if c.Confidence != 0 {
			confidence = c.Confidence
		}
		var signature any
		if c.SemanticSignature != "" {
			signature = c.SemanticSignature
		}
		sourceRecordID := c.SourceRecordID
		if sourceRecordID <= 0 {
			sourceRecordID = inputRecordID
		}
		targetRecordID := c.TargetRecordID
		if targetRecordID <= 0 {
			targetRecordID = inputRecordID
		}
		sourceDesc := c.SourceDesc
		if sourceDesc == "" {
			sourceDesc = connectionEndpointDesc(c.SourceType, c.SourceID)
		}
		targetDesc := c.TargetDesc
		if targetDesc == "" {
			targetDesc = connectionEndpointDesc(c.TargetType, c.TargetID)
		}
		if _, err := stmt.ExecContext(ctx,
			sourceRecordID, targetRecordID, c.SourceType, c.SourceID, c.TargetType, c.TargetID,
			c.RelationName, relationMethod, confidence, overlap, provenance,
			signature, sourceDesc, targetDesc, extraInfo,
		); err != nil {
			return fmt.Errorf("(CWB_CONN_009) insert connection %s->%s (%s): %w",
				c.SourceID, c.TargetID, c.RelationName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("(CWB_CONN_010) commit: %w", err)
	}
	return nil
}

// ReplaceConnectionsBySource idempotently replaces a document's outgoing edges that
// originate from one source_type, scoped by the source side only. Unlike
// ReplaceConnections (which is intra-document: source_record_id = target_record_id), the
// delete here does NOT constrain target_record_id, so it correctly cleans cross-document
// edges (e.g. a metric semantically linked to artifacts in other documents). Used for the
// hybrid_search / semantically_related metric connections.
func (s *ConnectionSQLStore) ReplaceConnectionsBySource(ctx context.Context, sourceRecordID int64, sourceType, relationMethod string, relationNames []string, conns []Connection) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("(CWB_CONN_011) nil connection store db handle")
	}
	if strings.TrimSpace(sourceType) == "" {
		return fmt.Errorf("(CWB_CONN_012) sourceType must be non-empty for source-scoped delete")
	}
	if len(relationNames) == 0 {
		return fmt.Errorf("(CWB_CONN_013) relationNames must be non-empty for source-scoped delete")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("(CWB_CONN_014) begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM kb.artifact_connections
		 WHERE source_type = $1 AND source_record_id = $2
		   AND relation_method = $3 AND relation_name = ANY($4)`,
		sourceType, sourceRecordID, relationMethod, pq.Array(relationNames),
	); err != nil {
		return fmt.Errorf("(CWB_CONN_015) delete existing source connections: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, connectionInsertSQL)
	if err != nil {
		return fmt.Errorf("(CWB_CONN_016) prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, c := range conns {
		overlap, err := encodeJSONB(c.Overlap)
		if err != nil {
			return fmt.Errorf("(CWB_CONN_017) encode overlap: %w", err)
		}
		provenance, err := encodeJSONB(c.Provenance)
		if err != nil {
			return fmt.Errorf("(CWB_CONN_018) encode provenance: %w", err)
		}
		extraInfo, err := encodeJSONB(c.ExtraInfo)
		if err != nil {
			return fmt.Errorf("(CWB_CONN_019) encode extra_info: %w", err)
		}
		var confidence any
		if c.Confidence != 0 {
			confidence = c.Confidence
		}
		var signature any
		if c.SemanticSignature != "" {
			signature = c.SemanticSignature
		}
		srcRecordID := c.SourceRecordID
		if srcRecordID <= 0 {
			srcRecordID = sourceRecordID
		}
		targetRecordID := c.TargetRecordID
		if targetRecordID <= 0 {
			targetRecordID = sourceRecordID
		}
		sourceDesc := c.SourceDesc
		if sourceDesc == "" {
			sourceDesc = connectionEndpointDesc(c.SourceType, c.SourceID)
		}
		targetDesc := c.TargetDesc
		if targetDesc == "" {
			targetDesc = connectionEndpointDesc(c.TargetType, c.TargetID)
		}
		if _, err := stmt.ExecContext(ctx,
			srcRecordID, targetRecordID, c.SourceType, c.SourceID, c.TargetType, c.TargetID,
			c.RelationName, relationMethod, confidence, overlap, provenance,
			signature, sourceDesc, targetDesc, extraInfo,
		); err != nil {
			return fmt.Errorf("(CWB_CONN_020) insert connection %s->%s (%s): %w",
				c.SourceID, c.TargetID, c.RelationName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("(CWB_CONN_025) commit: %w", err)
	}
	return nil
}

// ReplaceRecordConnectionsByMethod idempotently replaces every intra-document edge a
// processor produced under one relation_method, regardless of relation_name. Unlike
// ReplaceConnections (which scope-deletes by an explicit relation_name set), this deletes
// all (source_record_id = target_record_id = recordID, relation_method) rows and reinserts
// conns. It is the right primitive when the relation_names are open-ended — e.g. the
// canonical relation store (ADR 2026061401), whose subject->object edge name is the
// extracted predicate itself and therefore cannot be enumerated up front.
func (s *ConnectionSQLStore) ReplaceRecordConnectionsByMethod(ctx context.Context, recordID int64, relationMethod string, conns []Connection) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("(CWB_CONN_026) nil connection store db handle")
	}
	if strings.TrimSpace(relationMethod) == "" {
		return fmt.Errorf("(CWB_CONN_027) relationMethod must be non-empty for method-scoped delete")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("(CWB_CONN_028) begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM kb.artifact_connections
		 WHERE source_record_id = $1 AND target_record_id = $1 AND relation_method = $2`,
		recordID, relationMethod,
	); err != nil {
		return fmt.Errorf("(CWB_CONN_029) delete existing record connections: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, connectionInsertSQL)
	if err != nil {
		return fmt.Errorf("(CWB_CONN_030) prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, c := range conns {
		overlap, err := encodeJSONB(c.Overlap)
		if err != nil {
			return fmt.Errorf("(CWB_CONN_031) encode overlap: %w", err)
		}
		provenance, err := encodeJSONB(c.Provenance)
		if err != nil {
			return fmt.Errorf("(CWB_CONN_032) encode provenance: %w", err)
		}
		extraInfo, err := encodeJSONB(c.ExtraInfo)
		if err != nil {
			return fmt.Errorf("(CWB_CONN_033) encode extra_info: %w", err)
		}
		var confidence any
		if c.Confidence != 0 {
			confidence = c.Confidence
		}
		var signature any
		if c.SemanticSignature != "" {
			signature = c.SemanticSignature
		}
		sourceRecordID := c.SourceRecordID
		if sourceRecordID <= 0 {
			sourceRecordID = recordID
		}
		targetRecordID := c.TargetRecordID
		if targetRecordID <= 0 {
			targetRecordID = recordID
		}
		sourceDesc := c.SourceDesc
		if sourceDesc == "" {
			sourceDesc = connectionEndpointDesc(c.SourceType, c.SourceID)
		}
		targetDesc := c.TargetDesc
		if targetDesc == "" {
			targetDesc = connectionEndpointDesc(c.TargetType, c.TargetID)
		}
		if _, err := stmt.ExecContext(ctx,
			sourceRecordID, targetRecordID, c.SourceType, c.SourceID, c.TargetType, c.TargetID,
			c.RelationName, relationMethod, confidence, overlap, provenance,
			signature, sourceDesc, targetDesc, extraInfo,
		); err != nil {
			return fmt.Errorf("(CWB_CONN_034) insert connection %s->%s (%s): %w",
				c.SourceID, c.TargetID, c.RelationName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("(CWB_CONN_035) commit: %w", err)
	}
	return nil
}

// WriteLineOverlapConnectionsForRefs derives and idempotently replaces chunk ->
// artifact line_overlap edges from explicitly supplied refs. Used where the caller
// must filter which artifacts participate (e.g. only Level-0 summaries) and so
// cannot use the whole-type registry loader.
func WriteLineOverlapConnectionsForRefs(ctx context.Context, recordID int64, targetType, relationName string, chunks []Block, refs []ArtifactRef) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("(CWB_CONN_024) project db handle is nil")
	}
	conns := DeriveLineOverlapConnections(recordID, targetType, relationName, chunks, refs)
	store := &ConnectionSQLStore{DB: db}
	return store.ReplaceConnections(ctx, recordID, RelationMethodLineOverlap, []string{relationName}, conns)
}

// WriteLineOverlapConnectionsFromRegistry derives chunk -> artifact line_overlap
// edges using the canonical artifact identity and source line spans already stored
// in kb.search_artifacts (populated by the Reindex*SearchForRecord hook the caller
// runs just before this). Sourcing refs from the registry guarantees target_id
// matches the rest of the system and keeps every processor's wiring uniform.
func WriteLineOverlapConnectionsFromRegistry(ctx context.Context, recordID int64, targetType, relationName string, chunks []Block) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("(CWB_CONN_022) project db handle is nil")
	}
	refs, err := loadArtifactRefsFromRegistry(ctx, db, recordID, targetType)
	if err != nil {
		return fmt.Errorf("(CWB_CONN_023) load %s refs: %w", targetType, err)
	}
	conns := DeriveLineOverlapConnections(recordID, targetType, relationName, chunks, refs)
	store := &ConnectionSQLStore{DB: db}
	return store.ReplaceConnections(ctx, recordID, RelationMethodLineOverlap, []string{relationName}, conns)
}

// replaceChunkRangesForRecord idempotently rewrites kb.chunk_ranges for one document from
// the canonical fixed-size chunk blocks. chunk_id matches ChunkConnectionID (the positional
// id used in connected_artifacts.chunks), and source_line_spans is the chunk's line set
// coalesced into "a:b"/"n" runs so the generated line_range can drive the on-demand
// kb.connected_artifacts() overlap query. Callers pass the canonical fixed-size blocks
// (the .chunks artifact file), not semantic chunks.
func replaceChunkRangesForRecord(ctx context.Context, db *sql.DB, recordID int64, chunks []Block) error {
	if db == nil {
		return fmt.Errorf("(CWB_CONN_025) project db handle is nil")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.chunk_ranges WHERE input_record_id = $1`, recordID); err != nil {
		return err
	}
	const stmt = `INSERT INTO kb.chunk_ranges (input_record_id, chunk_id, source_line_spans)
	              VALUES ($1, $2, $3::jsonb)
	              ON CONFLICT (input_record_id, chunk_id)
	              DO UPDATE SET source_line_spans = EXCLUDED.source_line_spans`
	for _, ch := range chunks {
		spans := blockLinesToSpans(ch.Lines)
		spansJSON, _ := json.Marshal(spans)
		if _, err := tx.ExecContext(ctx, stmt, recordID, ChunkConnectionID(recordID, ch.Index), string(spansJSON)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// blockLinesToSpans collapses a block's (possibly unsorted, duplicated) line numbers into
// sorted "a:b" / "n" run strings, matching the span format parsed by
// kb.line_spans_to_int8multirange and parseMetricLineSpan.
func blockLinesToSpans(lines []BlockLine) []string {
	nums := make([]int, 0, len(lines))
	for _, l := range lines {
		if l.LineNumber > 0 {
			nums = append(nums, l.LineNumber)
		}
	}
	sort.Ints(nums)
	out := []string{}
	for i := 0; i < len(nums); {
		start := nums[i]
		end := start
		j := i + 1
		for j < len(nums) {
			if nums[j] == end {
				j++ // duplicate
				continue
			}
			if nums[j] == end+1 {
				end = nums[j]
				j++
				continue
			}
			break
		}
		if start == end {
			out = append(out, strconv.Itoa(start))
		} else {
			out = append(out, strconv.Itoa(start)+":"+strconv.Itoa(end))
		}
		i = j
	}
	return out
}

// loadArtifactRefsFromRegistry reads the canonical artifact_id and source line
// spans for one artifact type of a document from kb.search_artifacts.
func loadArtifactRefsFromRegistry(ctx context.Context, db *sql.DB, recordID int64, artifactType string) ([]ArtifactRef, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT artifact_id, source_line_spans
		 FROM kb.search_artifacts
		 WHERE artifact_type = $1 AND input_record_id = $2`,
		artifactType, recordID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var refs []ArtifactRef
	for rows.Next() {
		var id string
		var spans []byte
		if err := rows.Scan(&id, &spans); err != nil {
			return nil, err
		}
		refs = append(refs, artifactRefFromRegistry(artifactType, id, spans))
	}
	return refs, rows.Err()
}

// artifactRefFromRegistry parses one registry row into an ArtifactRef. The
// source_line_spans JSON may be string spans ("5", "12:14"), numeric line lists,
// or line objects; normalizeSourceLineSpans collapses all forms to merged spans.
func artifactRefFromRegistry(artifactType, artifactID string, sourceLineSpansJSON []byte) ArtifactRef {
	var raw any
	if len(sourceLineSpansJSON) > 0 {
		_ = json.Unmarshal(sourceLineSpansJSON, &raw)
	}
	return ArtifactRef{Type: artifactType, ID: artifactID, Spans: normalizeSourceLineSpans(raw)}
}

// WriteConnections idempotently replaces a document's edges for one relation_name
// under the given relation_method (used for non-derived edges such as 'llm').
func WriteConnections(ctx context.Context, recordID int64, relationMethod, relationName string, conns []Connection) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("(CWB_CONN_021) project db handle is nil")
	}
	store := &ConnectionSQLStore{DB: db}
	return store.ReplaceConnections(ctx, recordID, relationMethod, []string{relationName}, conns)
}

// LoadConnectionsBySource returns the artifact-graph edges originating from one
// document's artifacts of a given source type. relationMethod and targetType are
// optional filters: pass "" to match any value. Used by readers (e.g. the metric
// document reviewer) that traverse precomputed edges such as the hybrid_search /
// semantically_related links written at index time.
func (s *ConnectionSQLStore) LoadConnectionsBySource(
	ctx context.Context,
	sourceRecordID int64,
	sourceType string,
	relationMethod string,
	targetType string,
) ([]Connection, error) {
	if s == nil || s.DB == nil {
		return nil, fmt.Errorf("(CWB_CONN_031) nil connection store db handle")
	}
	if strings.TrimSpace(sourceType) == "" {
		return nil, fmt.Errorf("(CWB_CONN_032) sourceType must be non-empty")
	}

	conditions := []string{"source_type = $1", "source_record_id = $2"}
	args := []any{sourceType, sourceRecordID}
	next := 3
	if strings.TrimSpace(relationMethod) != "" {
		conditions = append(conditions, fmt.Sprintf("relation_method = $%d", next))
		args = append(args, relationMethod)
		next++
	}
	if strings.TrimSpace(targetType) != "" {
		conditions = append(conditions, fmt.Sprintf("target_type = $%d", next))
		args = append(args, targetType)
		next++
	}

	query := `
SELECT source_record_id, target_record_id, source_type, source_id, target_type, target_id,
       relation_name, relation_method, COALESCE(confidence, 0),
       COALESCE(source_desc, ''), COALESCE(target_desc, '')
FROM kb.artifact_connections
WHERE ` + strings.Join(conditions, " AND ") + `
ORDER BY source_id, confidence DESC NULLS LAST, target_id`

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("(CWB_CONN_033) query connections by source: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Connection
	for rows.Next() {
		var c Connection
		if err := rows.Scan(
			&c.SourceRecordID, &c.TargetRecordID, &c.SourceType, &c.SourceID,
			&c.TargetType, &c.TargetID, &c.RelationName, &c.RelationMethod,
			&c.Confidence, &c.SourceDesc, &c.TargetDesc,
		); err != nil {
			return nil, fmt.Errorf("(CWB_CONN_034) scan connection row: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("(CWB_CONN_035) iterate connection rows: %w", err)
	}
	return out, nil
}

// encodeJSONB marshals a value for a JSONB column, returning nil (SQL NULL) for
// nil, typed-nil pointers, and empty maps so absent data stays NULL.
func encodeJSONB(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 {
			return nil, nil
		}
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Pointer && rv.IsNil() {
			return nil, nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}
