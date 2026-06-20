package docprocessing

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func (p *SceneBlocksProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	rec, err := p.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("(APID_26060601) load kb.inputs record %d: %w", recordID, err)
	}

	exists, err := p.Store.SceneObjectsExist(ctx, recordID)
	if err != nil {
		return fmt.Errorf("(APID_26060602) check scene blocks exist for record %d: %w", recordID, err)
	}
	if !exists {
		return nil
	}

	chunks, chunkErr := p.loadRecordChunks(rec, recordID)
	if chunkErr != nil {
		p.Logger.Warn("post-process scene blocks: load chunks failed; indexing without chunk overlap",
			"record_id", recordID, "error", chunkErr)
	}

	if reindexErr := ReindexSceneBlockSearchForRecord(ctx, recordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex scene block search registry failed", "record_id", recordID, "error", reindexErr)
	}
	if connErr := WriteLineOverlapConnectionsFromRegistry(ctx, recordID, searchArtifactSceneBlock, RelationHasScene, chunks); connErr != nil {
		p.Logger.Warn("write has-scene connections failed", "record_id", recordID, "error", connErr)
	}
	IndexSceneBlocksForRecord(ctx, recordID, chunks, p.Logger)
	if refreshErr := refreshSceneBlockArtifactFile(ctx, ApiTypes.ProjectDBHandle, p.ArtifactDir, recordID, rec); refreshErr != nil {
		p.Logger.Warn("refresh scene block artifact failed", "record_id", recordID, "error", refreshErr)
	}
	return nil
}

func (p *SceneBlocksProcessor) loadRecordChunks(rec DocMetadataInputRecord, recordID int64) ([]Block, error) {
	lines, err := p.resolveInputLines(context.Background(), LineFileGeneratedEvent{RecordID: recordID}, rec)
	if err != nil {
		return nil, err
	}
	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: p.ChunkSize, OverlapPercent: p.OverlapPercent})
	if err != nil {
		return nil, err
	}
	return chunksToBlocks(chunks), nil
}

func (p *GenerateSummariesProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	rec, err := p.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("(APID_26060603) load kb.inputs record %d: %w", recordID, err)
	}

	chunks, chunkErr := loadFixedSizeChunkBlocks(p.ArtifactDir, rec, recordID)
	if chunkErr != nil {
		p.Logger.Warn("post-process summaries: load chunks failed; indexing without chunk overlap",
			"record_id", recordID, "error", chunkErr)
	}

	if reindexErr := ReindexSummarySearchForRecord(ctx, recordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex summary search registry failed", "record_id", recordID, "error", reindexErr)
	}
	IndexSummariesForRecord(ctx, recordID, chunks, p.Logger)
	return nil
}

func (p *GenerateTopicsProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	rec, err := p.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("(APID_26060604) load kb.inputs record %d: %w", recordID, err)
	}

	chunks, chunkErr := p.loadRecordChunks(rec, recordID)
	if chunkErr != nil {
		p.Logger.Warn("post-process topics: load chunks failed; indexing without chunk overlap",
			"record_id", recordID, "error", chunkErr)
	}

	if reindexErr := ReindexTopicSearchForRecord(ctx, recordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex topic search registry failed", "record_id", recordID, "error", reindexErr)
	}
	IndexTopicsForRecord(ctx, recordID, chunks, p.Logger)
	return nil
}

func (p *GenerateTopicsProcessor) loadRecordChunks(rec DocMetadataInputRecord, recordID int64) ([]Block, error) {
	switch svc := p.Service.(type) {
	case *SemanticChunkingService:
		lineFilePath, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: recordID}, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
		if err != nil {
			return nil, fmt.Errorf("resolve line file: %w", err)
		}
		body, err := os.ReadFile(lineFilePath)
		if err != nil {
			return nil, fmt.Errorf("read line file: %w", err)
		}
		lines, err := ParseInputLinesIncludingTOC(body)
		if err != nil {
			return nil, fmt.Errorf("parse line file: %w", err)
		}
		return semanticBlocksToBlocks(BuildSemanticPageBlocks(lines, svc.FileBlockSize)), nil
	default:
		return loadFixedSizeChunkBlocks(p.ArtifactDir, rec, recordID)
	}
}

func (p *ProvisionsProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	rec, err := p.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("(APID_26060605) load kb.inputs record %d: %w", recordID, err)
	}

	exists, err := p.Store.ProvisionsExist(ctx, recordID, "")
	if err != nil {
		return fmt.Errorf("(APID_26060606) check provisions exist for record %d: %w", recordID, err)
	}
	if !exists {
		return nil
	}

	chunks, chunkErr := p.loadRecordChunks(rec, recordID)
	if chunkErr != nil {
		p.Logger.Warn("post-process provisions: load chunks failed; indexing without chunk overlap",
			"record_id", recordID, "error", chunkErr)
	}

	if reindexErr := ReindexProvisionSearchForRecord(ctx, recordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex provision search registry failed", "record_id", recordID, "error", reindexErr)
	}
	IndexProvisionsForRecord(ctx, recordID, chunks, p.Logger)
	if refreshErr := refreshProvisionArtifactFile(ctx, ApiTypes.ProjectDBHandle, p.ArtifactDir, recordID, rec); refreshErr != nil {
		p.Logger.Warn("refresh provision artifact failed", "record_id", recordID, "error", refreshErr)
	}
	return nil
}

func (p *ProvisionsProcessor) loadRecordChunks(rec DocMetadataInputRecord, recordID int64) ([]Block, error) {
	evt := LineFileGeneratedEvent{RecordID: recordID}
	if p.ExtractProvisionsInput == "blocks" {
		blocks, err := p.resolveBlocks(context.Background(), evt, rec)
		if err != nil {
			return nil, err
		}
		return blocks, nil
	}
	chunks, err := p.resolveChunks(context.Background(), evt, rec)
	if err != nil {
		return nil, err
	}
	return chunksToBlocks(chunks), nil
}

func (p *EntityRelationProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	start := time.Now()
	if p.Logger != nil {
		p.Logger.Info("entity-relation post-process start", "record_id", recordID)
	}
	rec, err := p.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("(APID_26060607) load kb.inputs record %d: %w", recordID, err)
	}

	entitiesExist, err := p.Store.EntitiesExist(ctx, recordID)
	if err != nil {
		return fmt.Errorf("(APID_26060608) check entities exist for record %d: %w", recordID, err)
	}
	relationsExist, err := p.Store.RelationsExist(ctx, recordID)
	if err != nil {
		return fmt.Errorf("(APID_26060609) check relations exist for record %d: %w", recordID, err)
	}
	if !entitiesExist && !relationsExist {
		if p.Logger != nil {
			p.Logger.Info("entity-relation post-process skipped: no artifacts",
				"record_id", recordID,
				"ms_used", time.Since(start).Milliseconds(),
			)
		}
		return nil
	}

	chunkStart := time.Now()
	chunks, chunkErr := p.loadRecordChunks(rec, recordID)
	if chunkErr != nil {
		p.Logger.Warn("post-process entity relation: load chunks failed; indexing without chunk overlap",
			"record_id", recordID, "error", chunkErr)
	} else if p.Logger != nil {
		p.Logger.Info("entity-relation post-process chunks loaded",
			"record_id", recordID,
			"chunks", len(chunks),
			"ms_used", time.Since(chunkStart).Milliseconds(),
		)
	}

	entityReindexStart := time.Now()
	if reindexErr := ReindexEntitySearchForRecord(ctx, recordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex entity search registry failed", "record_id", recordID, "error", reindexErr)
	} else if p.Logger != nil {
		p.Logger.Info("entity-relation post-process entity reindex finished",
			"record_id", recordID,
			"ms_used", time.Since(entityReindexStart).Milliseconds(),
		)
	}
	relationReindexStart := time.Now()
	if reindexErr := ReindexRelationSearchForRecord(ctx, recordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex relation search registry failed", "record_id", recordID, "error", reindexErr)
	} else if p.Logger != nil {
		p.Logger.Info("entity-relation post-process relation reindex finished",
			"record_id", recordID,
			"ms_used", time.Since(relationReindexStart).Milliseconds(),
		)
	}
	if semClusterErr := semClusterEntities(ctx, ApiTypes.ProjectDBHandle, recordID, p.Logger); semClusterErr != nil {
		p.Logger.Warn("semantic clustering failed (non-fatal)", "record_id", recordID, "error", semClusterErr)
	}
	entityIndexStart := time.Now()
	IndexEntitiesForRecord(ctx, recordID, chunks, p.Logger)
	IndexEntityNamesForRecord(ctx, recordID, p.Logger)
	if p.Logger != nil {
		p.Logger.Info("entity-relation post-process entity indexing finished",
			"record_id", recordID,
			"ms_used", time.Since(entityIndexStart).Milliseconds(),
		)
	}
	relationIndexStart := time.Now()
	IndexRelationsForRecord(ctx, recordID, chunks, p.Logger)
	// Canonical relation store (ADR 2026061401): materialize subject->object and
	// predicate-of edges into kb.artifact_connections. Relation categories are written
	// separately as category_name edges.
	IndexRelationGraphForRecord(ctx, recordID, p.Logger)
	if p.Logger != nil {
		p.Logger.Info("entity-relation post-process relation indexing finished",
			"record_id", recordID,
			"ms_used", time.Since(relationIndexStart).Milliseconds(),
		)
		p.Logger.Info("entity-relation post-process finished",
			"record_id", recordID,
			"entities_exist", entitiesExist,
			"relations_exist", relationsExist,
			"total_ms", time.Since(start).Milliseconds(),
		)
	}
	if refreshErr := refreshEntityArtifactFile(ctx, ApiTypes.ProjectDBHandle, p.ArtifactDir, recordID, rec); refreshErr != nil {
		p.Logger.Warn("refresh entity artifact failed", "record_id", recordID, "error", refreshErr)
	}
	if refreshErr := refreshRelationArtifactFile(ctx, ApiTypes.ProjectDBHandle, p.ArtifactDir, recordID, rec); refreshErr != nil {
		p.Logger.Warn("refresh relation artifact failed", "record_id", recordID, "error", refreshErr)
	}
	return nil
}

func (p *EntityRelationProcessor) loadRecordChunks(rec DocMetadataInputRecord, recordID int64) ([]Block, error) {
	return loadFixedSizeChunkBlocks(p.ArtifactDir, rec, recordID)
}

func loadFixedSizeChunkBlocks(artifactDir string, rec DocMetadataInputRecord, recordID int64) ([]Block, error) {
	lineFilePath, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: recordID}, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, fmt.Errorf("resolve line file: %w", err)
	}
	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("read line file: %w", err)
	}
	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("parse line file: %w", err)
	}
	artifactDir = strings.TrimSpace(artifactDir)
	if artifactDir == "" {
		artifactDir = strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, err := loadChunksFromArtifactFile(artifactDir, recordID, artifactBase+".chunks", lines)
	if err != nil {
		return nil, fmt.Errorf("load chunks: %w", err)
	}
	return chunksToBlocks(chunks), nil
}
