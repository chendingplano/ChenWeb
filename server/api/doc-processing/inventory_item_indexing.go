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

// inventoryItemIndexConfig binds the shared artifact-indexing engine
// (artifact_indexing.go) to the inventory-item family (spec 3.3). Inventory indexing is
// almost identical to metric indexing (spec 3.1); both run the same Phase C engine and
// differ only in this config and how their source rows are loaded.
var inventoryItemIndexConfig = artifactIndexConfig{
	SelfType:                   searchArtifactInventoryItem,
	CategoryType:               "inventory_item",
	InstanceSource:             "extract_inventory_items",
	Table:                      "kb.inventory_items",
	IDColumn:                   "inventory_item_id",
	CategoryTreeFilename:       "inventory_items.txt",
	LogPrefix:                  "inventory items indexing",
	WarnOnMissingCategoryPaths: true,
}

// IndexInventoryItemsForRecord runs the post-save inventory-item indexing workflow
// (spec 3.3.2-3.3.5): connected_artifacts JSON, category_name edges, category-path
// inventory_items.txt entries, and hybrid_search semantic links. Step 3.3.1
// (kb.search_artifacts) is handled separately by ReindexInventoryItemSearchForRecord,
// which the caller runs first. Each step is best-effort and logged; a failure in one step
// does not abort the others.
func IndexInventoryItemsForRecord(
	ctx context.Context,
	recordID int64,
	model_name string,
	prompt_ref string,
	inputChunks []Block,
	logger ApiTypes.JimoLogger) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		if logger != nil {
			logger.Warn("inventory items indexing skipped: nil project db handle", "record_id", recordID)
		}
		return
	}
	items, err := loadIndexedInventoryItemsForRecord(ctx, db, recordID)
	if err != nil {
		if logger != nil {
			logger.Warn("inventory items indexing: load items failed", "record_id", recordID, "error", err.Error())
		}
		return
	}
	if len(items) == 0 {
		return
	}
	if logger != nil {
		logger.Info("inventory items indexing start", "record_id", recordID, "inventory_items", len(items))
	}

	runArtifactHydrationForSemanticLinking(ctx, db, recordID, items, inventoryItemIndexConfig, logger)
	resolver := newMetricCategoryResolver(db, logger) // category resolver is category-type-agnostic
	categoryConnections := upsertArtifactCategoryConnections(ctx, db, recordID,
		model_name, prompt_ref, items, inventoryItemIndexConfig, resolver, logger)
	categoryPathItems := indexArtifactsByCategoryPaths(ctx, db, recordID, items, inventoryItemIndexConfig, logger)
	semanticLinks := connectArtifactsBySearch(ctx, db, recordID, items, inventoryItemIndexConfig, logger)

	if logger != nil {
		logger.Info("inventory items indexing result",
			"record_id", recordID,
			"category_connections", categoryConnections,
			"category_path_items", categoryPathItems,
			"semantic_links", semanticLinks,
		)
	}
}

// loadIndexedInventoryItemsForRecord loads the stored inventory items into the generic
// indexedArtifact view used by the shared indexing engine. item_categories is a JSONB
// array (unlike kb.metrics.metric_categories, which is a TEXT JSON string) so it is
// parsed with parseJSONStringArray.
func loadIndexedInventoryItemsForRecord(ctx context.Context, db *sql.DB, recordID int64) ([]indexedArtifact, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT inventory_item_id,
		        COALESCE(source_line_spans, '[]'::jsonb),
		        COALESCE(item_categories, '[]'::jsonb),
		        COALESCE(search_document, '')
		 FROM kb.inventory_items
		 WHERE input_record_id = $1
		 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []indexedArtifact
	for rows.Next() {
		var (
			itemID    string
			spansRaw  []byte
			catsRaw   []byte
			searchDoc string
		)
		if err := rows.Scan(&itemID, &spansRaw, &catsRaw, &searchDoc); err != nil {
			return nil, err
		}
		var spansAny any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		out = append(out, indexedArtifact{
			ID:             strings.TrimSpace(itemID),
			SourceSpans:    normalizeSourceLineSpans(spansAny),
			Categories:     parseJSONStringArray(catsRaw),
			SearchDocument: strings.TrimSpace(searchDoc),
		})
	}
	return out, rows.Err()
}

// PostProcessIndex runs the inventory-item indexing workflow in Phase C, after all doc
// processors have finished. It is invoked by the pipeline controller for every record
// whose pipeline included extract_inventory_items. Running here (rather than inside
// HandleEvent) guarantees that the artifacts inventory indexing depends on — semantic
// projections, topics, scenes, provisions, entities, metrics, and their
// kb.search_artifacts rows — are already present, regardless of Phase B ordering. It is
// idempotent and safe to run for both freshly-extracted and pre-existing items.
func (p *InventoryItemsProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	rec, err := p.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("(IID_26060501) load kb.inputs record %d: %w", recordID, err)
	}

	exists, err := p.Store.InventoryItemsExist(ctx, recordID)
	if err != nil {
		return fmt.Errorf("(IID_26060502) check inventory items exist for record %d: %w", recordID, err)
	}
	if !exists {
		return nil
	}

	// Chunks back connected_artifacts.chunks and the has-inventory-items line-overlap
	// edges. If they cannot be loaded, indexing still proceeds for the remaining outputs.
	chunks, chunkErr := p.loadRecordChunks(rec, recordID)
	if chunkErr != nil {
		p.Logger.Warn("post-process inventory items: load chunks failed; indexing without chunk overlap",
			"record_id", recordID, "error", chunkErr)
	}

	if reindexErr := ReindexInventoryItemSearchForRecord(ctx, recordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex inventory item search registry failed", "record_id", recordID, "error", reindexErr)
	}
	if connErr := WriteLineOverlapConnectionsFromRegistry(ctx, recordID, searchArtifactInventoryItem, RelationHasInventoryItems, chunks); connErr != nil {
		p.Logger.Warn("write has-inventory-items connections failed", "record_id", recordID, "error", connErr)
	}
	IndexInventoryItemsForRecord(ctx, recordID, p.ModelName, p.PromptRef, chunks, p.Logger)
	if refreshErr := refreshInventoryItemsArtifactFile(ctx, ApiTypes.ProjectDBHandle, p.ArtifactDir, recordID, rec); refreshErr != nil {
		p.Logger.Warn("refresh inventory items artifact failed", "record_id", recordID, "error", refreshErr)
	}
	return nil
}

// loadRecordChunks resolves and loads the record's persisted chunk blocks from the
// .chunks artifact, mirroring HandleEvent's loader but without status side effects so it
// is safe to call from the Phase C post-process step.
func (p *InventoryItemsProcessor) loadRecordChunks(rec DocMetadataInputRecord, recordID int64) ([]Block, error) {
	lineFilePath, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: recordID}, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, fmt.Errorf("resolve line file: %w", err)
	}
	lineBody, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("read line file: %w", err)
	}
	lines, err := ParseInputLinesIncludingTOC(lineBody)
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
