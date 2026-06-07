package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

var sceneBlockIndexConfig = artifactIndexConfig{
	SelfType:             searchArtifactSceneBlock,
	CategoryType:         "scene_block",
	InstanceSource:       "generate_scene_blocks",
	Table:                "kb.scene_objects",
	IDColumn:             "object_id",
	CategoryTreeFilename: "scenes.txt",
	LogPrefix:            "scene blocks indexing",
}

// IndexSceneBlocksForRecord runs the Phase C scene-block indexing workflow: populate
// connected_artifacts by line overlap, then refresh hybrid-search semantic links.
func IndexSceneBlocksForRecord(ctx context.Context, recordID int64, inputChunks []Block, logger ApiTypes.JimoLogger) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		if logger != nil {
			logger.Warn("scene blocks indexing skipped: nil project db handle", "record_id", recordID)
		}
		return
	}
	blocks, err := loadIndexedSceneBlocksForRecord(ctx, db, recordID)
	if err != nil {
		if logger != nil {
			logger.Warn("scene blocks indexing: load scene blocks failed", "record_id", recordID, "error", err.Error())
		}
		return
	}
	if len(blocks) == 0 {
		return
	}
	connectedCount := buildArtifactConnectedArtifacts(ctx, db, recordID, inputChunks, blocks, sceneBlockIndexConfig, logger)
	if logger != nil {
		logger.Info("scene blocks indexing result",
			"record_id", recordID,
			"scene_blocks", len(blocks),
			"connected_artifacts", connectedCount,
		)
	}
}

func loadIndexedSceneBlocksForRecord(ctx context.Context, db *sql.DB, recordID int64) ([]indexedArtifact, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT object_id,
		        COALESCE(line_spans, '[]'::jsonb),
		        COALESCE(search_document, '')
		 FROM kb.scene_objects
		 WHERE input_record_id = $1
		 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []indexedArtifact
	for rows.Next() {
		var (
			objectID  string
			spansRaw  []byte
			searchDoc string
		)
		if err := rows.Scan(&objectID, &spansRaw, &searchDoc); err != nil {
			return nil, err
		}
		var spansAny any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		out = append(out, indexedArtifact{
			ID:             strings.TrimSpace(objectID),
			SourceSpans:    normalizeSourceLineSpans(spansAny),
			SearchDocument: strings.TrimSpace(searchDoc),
		})
	}
	return out, rows.Err()
}
