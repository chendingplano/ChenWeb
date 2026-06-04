package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"

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
