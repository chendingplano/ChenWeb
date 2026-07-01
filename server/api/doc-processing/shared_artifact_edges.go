package docprocessing

import (
	"context"
	"database/sql"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/lib/pq"
)

// sharedArtifactOverlap is one (anchor artifact, self artifact) line-overlap pair returned
// by the kb.search_artifacts self-join, used to build #shared_artifact edges.
type sharedArtifactOverlap struct {
	anchorType string
	anchorID   string
	selfID     string
}

// buildSharedArtifactEdges turns overlap pairs into #shared_artifact /
// line-overlapped-artifact Connections. The overlapping anchor is the SOURCE and the self
// artifact is the TARGET; both endpoints are intra-document (source_record_id =
// target_record_id = recordID). Pure (no IO) for testability.
func buildSharedArtifactEdges(recordID int64, selfType, extraSource string, overlaps []sharedArtifactOverlap) []Connection {
	out := make([]Connection, 0, len(overlaps))
	for _, o := range overlaps {
		out = append(out, Connection{
			SourceRecordID: recordID,
			TargetRecordID: recordID,
			SourceType:     o.anchorType,
			SourceID:       o.anchorID,
			TargetType:     selfType,
			TargetID:       o.selfID,
			RelationName:   RelationSharedArtifact,
			RelationMethod: RelationMethodLineOverlapArtifact,
			Confidence:     1.0,
			ExtraInfo:      map[string]any{"source": extraSource, "anchor_type": o.anchorType},
		})
	}
	return out
}

// loadSharedArtifactOverlaps finds, for every selfType artifact in the record, the artifacts
// of anchorTypes in the same document whose line spans overlap it. It self-joins
// kb.search_artifacts on the GiST-indexed line_range && operator, so anchor and self are
// both read from the registry using their canonical artifact_ids (the ids the artifact graph
// keys on). This is race-safe in Phase C: every family registers itself in Phase B, so all
// anchor rows already exist by the time family indexing runs.
func loadSharedArtifactOverlaps(ctx context.Context, db *sql.DB, recordID int64, selfType string, anchorTypes []string) ([]sharedArtifactOverlap, error) {
	const q = `
SELECT t.artifact_type AS anchor_type, t.artifact_id AS anchor_id, s.artifact_id AS self_id
FROM kb.search_artifacts s
JOIN kb.search_artifacts t
  ON t.input_record_id = s.input_record_id
 AND t.artifact_type = ANY($3)
 AND t.line_range IS NOT NULL
 AND t.line_range && s.line_range
WHERE s.input_record_id = $1 AND s.artifact_type = $2 AND s.line_range IS NOT NULL
ORDER BY s.artifact_id, t.artifact_type, t.artifact_id`
	rows, err := db.QueryContext(ctx, q, recordID, selfType, pq.Array(anchorTypes))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []sharedArtifactOverlap
	for rows.Next() {
		var o sharedArtifactOverlap
		if err := rows.Scan(&o.anchorType, &o.anchorID, &o.selfID); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// writeSharedArtifactEdges materializes intra-document #shared_artifact edges from every
// artifact of anchorTypes that shares lines with a selfType artifact (anchor = source, self =
// target). It is idempotent per (selfType, record): ReplaceSharedArtifactEdges scope-deletes
// this family's edges by target_type before inserting, so parallel Phase-C family runs do not
// clobber each other. Returns the number of edges written. Best-effort: failures are logged,
// not fatal.
func writeSharedArtifactEdges(ctx context.Context, db *sql.DB, recordID int64, selfType string, anchorTypes []string, extraSource string, logger ApiTypes.JimoLogger) int {
	if db == nil {
		return 0
	}
	overlaps, err := loadSharedArtifactOverlaps(ctx, db, recordID, selfType, anchorTypes)
	if err != nil {
		if logger != nil {
			logger.Warn(selfType+" indexing: load shared-artifact overlaps failed",
				"record_id", recordID, "error", err.Error())
		}
		return 0
	}
	conns := buildSharedArtifactEdges(recordID, selfType, extraSource, overlaps)
	store := &ConnectionSQLStore{DB: db}
	if err := store.ReplaceSharedArtifactEdges(ctx, recordID, selfType, conns); err != nil {
		if logger != nil {
			logger.Warn(selfType+" indexing: replace shared-artifact edges failed",
				"record_id", recordID, "error", err.Error())
		}
		return 0
	}
	return len(conns)
}
