package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// PostProcessIndex runs the semantic projection indexing workflow in Phase C, after all
// doc processors have finished. This ensures connected_artifacts sees later artifacts
// like topics, scenes, provisions, entities, metrics, and inventory items.
func (p *SemanticProjectionsProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	rec, err := p.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("(SPID_26060601) load kb.inputs record %d: %w", recordID, err)
	}

	exists, err := p.Store.SemanticProjectionsExist(ctx, recordID)
	if err != nil {
		return fmt.Errorf("(SPID_26060602) check semantic projections exist for record %d: %w", recordID, err)
	}
	if !exists {
		return nil
	}

	chunks, chunkErr := p.loadRecordChunks(rec, recordID)
	if chunkErr != nil {
		p.Logger.Warn("post-process semantic projections: load chunks failed; indexing without chunk overlap",
			"record_id", recordID, "error", chunkErr)
	}

	if reindexErr := ReindexSemanticProjectionSearchForRecord(ctx, recordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex semantic projection search registry failed", "record_id", recordID, "error", reindexErr)
	}
	IndexSemanticProjectionsForRecord(ctx, recordID, chunks, p.Logger)
	if refreshErr := refreshSemanticProjectionArtifactFile(ctx, ApiTypes.ProjectDBHandle, p.ArtifactDir, recordID, rec); refreshErr != nil {
		p.Logger.Warn("refresh semantic projection artifact failed", "record_id", recordID, "error", refreshErr)
	}
	return nil
}

func IndexSemanticProjectionsForRecord(ctx context.Context, recordID int64, inputChunks []Block, logger ApiTypes.JimoLogger) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		if logger != nil {
			logger.Warn("semantic projections indexing skipped: nil project db handle", "record_id", recordID)
		}
		return
	}
	// Persist the canonical fixed-size chunk line ranges so kb.connected_artifacts() can
	// compute chunk edges on demand (Option A). Skip when chunks failed to load so we never
	// wipe a previously-populated set.
	if len(inputChunks) > 0 {
		if err := replaceChunkRangesForRecord(ctx, db, recordID, inputChunks); err != nil && logger != nil {
			logger.Warn("semantic projections indexing: replace chunk_ranges failed", "record_id", recordID, "error", err.Error())
		}
	}
	artifacts, err := loadIndexedSemanticProjectionsForRecord(ctx, db, recordID)
	if err != nil {
		if logger != nil {
			logger.Warn("semantic projections indexing: load projections failed", "record_id", recordID, "error", err.Error())
		}
		return
	}
	if len(artifacts) == 0 {
		return
	}
	if logger != nil {
		logger.Info("semantic projections indexing result",
			"record_id", recordID,
			"semantic_projections", len(artifacts),
		)
	}
}

func loadIndexedSemanticProjectionsForRecord(ctx context.Context, db *sql.DB, recordID int64) ([]indexedArtifact, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT semantic_proj_id,
		        COALESCE(line_spans, '[]'::jsonb)
		 FROM kb.semantic_projections
		 WHERE input_record_id = $1
		 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []indexedArtifact
	for rows.Next() {
		var (
			projID   string
			spansRaw []byte
		)
		if err := rows.Scan(&projID, &spansRaw); err != nil {
			return nil, err
		}
		var spansAny any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		out = append(out, indexedArtifact{
			ID:          strings.TrimSpace(projID),
			SourceSpans: normalizeSourceLineSpans(spansAny),
		})
	}
	return out, rows.Err()
}

func (p *SemanticProjectionsProcessor) loadRecordChunks(rec DocMetadataInputRecord, recordID int64) ([]Block, error) {
	lineFilePath, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: recordID}, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, fmt.Errorf("resolve line file: %w", err)
	}
	lineBody, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("read line file: %w", err)
	}
	lines, err := ParseInputLines(lineBody)
	if err != nil {
		return nil, fmt.Errorf("parse line file: %w", err)
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, err := loadChunksFromArtifactFile(p.ArtifactDir, recordID, artifactBase+".chunks", lines)
	if err != nil {
		return nil, fmt.Errorf("load chunks: %w", err)
	}
	return chunksToBlocks(chunks), nil
}
