package docprocessing

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type artifactObjectConnectionConfig struct {
	ArtifactType     string
	ArtifactTable    string
	ArtifactIDColumn string
	SourceType       string
	LogPrefix        string
}

func indexArtifactObjectConnections(ctx context.Context, db *sql.DB, recordID int64, cfg artifactObjectConnectionConfig, logger ApiTypes.JimoLogger) int64 {
	if db == nil {
		return 0
	}
	artifactType := strings.TrimSpace(cfg.ArtifactType)
	sourceType := strings.TrimSpace(cfg.SourceType)
	artifactTable := strings.TrimSpace(cfg.ArtifactTable)
	artifactIDColumn := strings.TrimSpace(cfg.ArtifactIDColumn)
	if artifactType == "" || sourceType == "" || artifactTable == "" || artifactIDColumn == "" {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+": object connections skipped: incomplete config", "record_id", recordID)
		}
		return 0
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+": begin object connections tx failed", "record_id", recordID, "error", err.Error())
		}
		return 0
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
DELETE FROM kb.artifact_connections
WHERE source_record_id = $1
  AND source_type = $4
  AND target_type = 'object_node'
  AND relation_method = $2
  AND relation_name = $3`,
		recordID, RelationMethodObjectID, RelationBelongTo, sourceType,
	); err != nil {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+": delete object connections failed", "record_id", recordID, "error", err.Error())
		}
		return 0
	}

	insertSQL := fmt.Sprintf(`
INSERT INTO kb.artifact_connections (
	source_record_id, target_record_id, source_type, source_id, target_type, target_id,
	relation_name, relation_method, confidence, source_desc, target_desc, extra_info
)
SELECT
	ao.source_record_id,
	ao.source_record_id,
	$5,
	ao.object_id,
	'object_node',
	onode.object_id,
	$2,
	$3,
	MAX(NULLIF(ao.reconcile_confidence, 0)),
	$5 || ':' || ao.object_id,
	'object_node:' || onode.object_id,
	jsonb_build_object('artifact_type', $4, 'artifact_ids', jsonb_agg(DISTINCT ao.artifact_id ORDER BY ao.artifact_id))
FROM %s a
JOIN kb.artifact_objects ao
  ON ao.source_record_id = a.input_record_id
 AND ao.artifact_type = $4
 AND ao.artifact_id = a.%s
JOIN kb.object_nodes onode
  ON onode.object_id = ao.object_id
WHERE a.input_record_id = $1
  AND COALESCE(ao.object_id, '') <> ''
GROUP BY ao.source_record_id, ao.object_id, onode.object_id
ON CONFLICT (relation_method, source_type, source_id, target_type, target_id, relation_name)
DO UPDATE SET
	source_record_id = EXCLUDED.source_record_id,
	target_record_id = EXCLUDED.target_record_id,
	confidence       = EXCLUDED.confidence,
	source_desc      = EXCLUDED.source_desc,
	target_desc      = EXCLUDED.target_desc,
	extra_info       = EXCLUDED.extra_info,
	create_time      = NOW()`, artifactTable, artifactIDColumn)
	res, err := tx.ExecContext(ctx, insertSQL,
		recordID, RelationBelongTo, RelationMethodObjectID, artifactType, sourceType,
	)
	if err != nil {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+": insert object connections failed", "record_id", recordID, "error", err.Error())
		}
		return 0
	}
	if err := tx.Commit(); err != nil {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+": commit object connections failed", "record_id", recordID, "error", err.Error())
		}
		return 0
	}
	count, err := res.RowsAffected()
	if err != nil {
		return 0
	}
	return count
}
