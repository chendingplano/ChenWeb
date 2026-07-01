package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// ---- types ----

// semClusterInvItemGroup is a work unit for one LLM adjudication call: the new
// inventory item E and the existing inventory items that hybrid search + coarse
// filter found as candidates. It serialises to JSON for the adjudication prompt.
type semClusterInvItemGroup struct {
	GroupID string                   `json:"group_id"`
	Members []semClusterInvItemMember `json:"members"`
}

type semClusterInvItemMember struct {
	InventoryItemID  string   `json:"inventory_item_id"`
	ItemName         string   `json:"item_name"`
	CanonicalName    string   `json:"canonical_name"`
	Categories       []string `json:"categories"`
	Manufacturer     string   `json:"manufacturer"`
	Brand            string   `json:"brand"`
	ModelNumber      string   `json:"model_number"`
	PartNumber       string   `json:"part_number"`
	Aliases          []string `json:"aliases"`
	Standards        []string `json:"standards"`
	InputRecordID    int64    `json:"input_record_id"`
	SearchCosine     float64  `json:"search_cosine,omitempty"`
	SearchLexicalScore float64 `json:"search_lexical_score,omitempty"`
}

// ---- config ----

// semClusterInvItemConfig holds the adjudication model and prompt resolved at
// startup for inventory item semantic clustering. Uses separate env vars from
// entity clustering so each can be independently configured.
type semClusterInvItemConfig struct {
	ModelName         string
	ModelCfg          structureModelConfig
	FallbackModelName string
	FallbackModelCfg  structureModelConfig
	PromptText        string
	PromptRef         string
	Enabled           bool
}

func resolveSemClusterInvItemConfig() semClusterInvItemConfig {
	if !semClusterEnabled() {
		return semClusterInvItemConfig{Enabled: false}
	}
	_, _, modelCfg, modelErr := loadModelConfigFromEnvKeys(
		[]string{"SEMCLUSTER_INVITEM_ADJ_MODEL_NAME", "SEMCLUSTER_ADJ_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	if modelErr != nil {
		return semClusterInvItemConfig{Enabled: false, ModelName: "unavailable"}
	}
	promptText, promptRef, _, promptErr := loadProductPromptFromEnvKeys(
		[]string{"SEMCLUSTER_INVITEM_ADJ_PROMPT", "SEMCLUSTER_ADJ_PROMPT"},
		"prompt-inventory-item-adjudicate-v1.md",
	)
	if promptErr != nil {
		return semClusterInvItemConfig{Enabled: false, ModelName: modelCfg.ModelName}
	}
	_, _, fallbackModelCfg, _ := loadOptionalModelConfigFromEnv(
		"SEMCLUSTER_INVITEM_ADJ_FALLBACK",
		"MODEL_DEF_FILE",
	)
	if fallbackModelCfg.ModelName == "" {
		_, _, fallbackModelCfg, _ = loadOptionalModelConfigFromEnv(
			"SEMCLUSTER_ADJ_FALLBACK",
			"MODEL_DEF_FILE",
		)
	}
	return semClusterInvItemConfig{
		ModelName:         modelCfg.ModelName,
		ModelCfg:          modelCfg,
		FallbackModelName: fallbackModelCfg.ModelName,
		FallbackModelCfg:  fallbackModelCfg,
		PromptText:        promptText,
		PromptRef:         strings.TrimSpace(promptRef),
		Enabled:           true,
	}
}

// ---- core entry point ----

// semClusterInventoryItems is the main entry point for semantic clustering of
// newly extracted inventory items. It is called from Phase C post-processing
// after the inventory item search registry has been reindexed. Analogous to
// semClusterEntities.
func semClusterInventoryItems(
	ctx context.Context,
	db *sql.DB,
	recordID int64,
	logger ApiTypes.JimoLogger,
) error {
	start := time.Now()
	cfg := resolveSemClusterInvItemConfig()
	if !cfg.Enabled {
		if logger != nil {
			logger.Info("inventory item semantic clustering skipped: disabled",
				"record_id", recordID)
		}
		return nil
	}

	// 1. Load pending items for this record.
	items, err := loadPendingInventoryItemsForSemCluster(ctx, db, recordID)
	if err != nil {
		return fmt.Errorf("(INVSC_01) load pending inventory items: %w", err)
	}
	if len(items) == 0 {
		if logger != nil {
			logger.Info("inventory item semantic clustering: no pending items",
				"record_id", recordID,
				"ms_used", time.Since(start).Milliseconds(),
			)
		}
		return nil
	}

	if logger != nil {
		logger.Info("inventory item semantic clustering start",
			"record_id", recordID,
			"pending_items", len(items),
			"model", cfg.ModelName,
			"prompt", cfg.PromptRef,
		)
	}

	searchLimit := semClusterGroupSize() * 5

	// 2. For each pending item: hybrid search -> coarse filter -> build group.
	var groups []semClusterInvItemGroup
	skippedNoMatch := 0
	scfg := appconfig.GetArtifactSearchConfig()
	dict := sanitizeTSDictionary(scfg.Dictionary)

	for _, item := range items {
		var vec []float64
		useSem := false
		if kbsearch.SemanticSearchEnabled() && len(item.Embedding) == kbsearch.ConfiguredEmbeddingDim() {
			vec = item.Embedding
			useSem = true
		}

		candidates, qErr := queryArtifactHybridCandidates(
			ctx, db, dict, strings.TrimSpace(item.SearchDocument), vec, useSem,
			searchArtifactInventoryItem, item.ArtifactID, "", searchLimit,
		)
		if qErr != nil {
			if logger != nil {
				logger.Warn("inventory item semantic clustering: hybrid search failed",
					"record_id", recordID, "item_id", item.InventoryItemID, "error", qErr.Error())
			}
			continue
		}

		// 3. Coarse filter.
		var members []semClusterInvItemMember
		members = append(members, inventoryItemRowToSemClusterMember(item))
		minCosine := semClusterMinBlockCosine()
		for _, c := range candidates {
			cosine := 0.0
			if c.cosineSim.Valid {
				cosine = c.cosineSim.Float64
			}
			if cosine < minCosine {
				continue
			}
			cand, loadErr := loadInventoryItemForSemCluster(ctx, db, c.artifactID)
			if loadErr != nil || cand == nil {
				continue
			}
			if cand.InventoryItemID == item.InventoryItemID {
				continue
			}
			if !hasCommonCategory(item.Categories, cand.Categories) {
				continue
			}
			member := inventoryItemRowToSemClusterMember(*cand)
			member.SearchCosine = cosine
			if c.lexScore.Valid {
				member.SearchLexicalScore = c.lexScore.Float64
			}
			members = append(members, member)
		}

		if len(members) <= 1 {
			skippedNoMatch++
			continue
		}
		groups = append(groups, semClusterInvItemGroup{
			GroupID: item.InventoryItemID,
			Members: members,
		})
	}

	if logger != nil {
		logger.Info("inventory item semantic clustering built",
			"record_id", recordID,
			"groups", len(groups),
			"skipped_no_match", skippedNoMatch,
			"ms_used", time.Since(start).Milliseconds(),
		)
	}

	if len(groups) == 0 {
		if logger != nil {
			logger.Info("inventory item semantic clustering finish - no matched")
		}
		return markPendingInvItemsAsClustered(ctx, db, items)
	}

	// 4. Batch groups, call LLM, apply merges.
	adjStart := time.Now()
	mergedIDs, err := batchAdjudicateInvItemsAndApply(ctx, db, groups, cfg, logger)
	if err != nil {
		if logger != nil {
			logger.Warn("inventory item semantic clustering: adjudication failed",
				"record_id", recordID, "error", err.Error())
		}
		return markPendingInvItemsAsClustered(ctx, db, items)
	}

	// 5. Mark items that were NOT merged as cluster heads.
	var headIDs []string
	for _, item := range items {
		if !mergedIDs[item.InventoryItemID] {
			headIDs = append(headIDs, item.InventoryItemID)
		}
	}
	store := &InventoryItemClusterStore{DB: db}
	if markErr := store.MarkClustered(ctx, headIDs, "clustered"); markErr != nil && logger != nil {
		logger.Warn("inventory item semantic clustering: mark clustered failed",
			"record_id", recordID, "head_count", len(headIDs), "error", markErr.Error())
	}

	if logger != nil {
		logger.Info("inventory item semantic clustering finish",
			"record_id", recordID,
			"groups", len(groups),
			"merged", len(mergedIDs),
			"cluster_heads", len(headIDs),
			"skipped_no_match", skippedNoMatch,
			"adj_ms", time.Since(adjStart).Milliseconds(),
			"total_ms", time.Since(start).Milliseconds(),
		)
	}
	return nil
}

// ---- pending-item row and loading ----

type pendingInventoryItemRow struct {
	InventoryItemID string
	ArtifactID      string
	SearchDocument  string
	Embedding       []float64
	ItemName        string
	CanonicalName   string
	Categories      []string
	Manufacturer    string
	Brand           string
	ModelNumber     string
	PartNumber      string
	Aliases         []string
	Standards       []string
	InputRecordID   int64
}

func inventoryItemRowToSemClusterMember(e pendingInventoryItemRow) semClusterInvItemMember {
	return semClusterInvItemMember{
		InventoryItemID: e.InventoryItemID,
		ItemName:        e.ItemName,
		CanonicalName:   e.CanonicalName,
		Categories:      e.Categories,
		Manufacturer:    e.Manufacturer,
		Brand:           e.Brand,
		ModelNumber:     e.ModelNumber,
		PartNumber:      e.PartNumber,
		Aliases:         e.Aliases,
		Standards:       e.Standards,
		InputRecordID:   e.InputRecordID,
	}
}

func loadPendingInventoryItemsForSemCluster(ctx context.Context, db *sql.DB, recordID int64) ([]pendingInventoryItemRow, error) {
	const q = `
	SELECT ii.inventory_item_id, ii.item_name, COALESCE(ii.canonical_name, ''),
	       COALESCE(ii.item_categories, '[]'::jsonb),
	       COALESCE(ii.manufacturer, ''), COALESCE(ii.brand, ''),
	       COALESCE(ii.model_number, ''), COALESCE(ii.part_number, ''),
	       COALESCE(ii.aliases, '[]'::jsonb), COALESCE(ii.standards, '[]'::jsonb),
	       ii.id AS row_id, COALESCE(ii.search_document, ''),
	       sa.embedding_text
	FROM kb.inventory_items ii
	LEFT JOIN kb.search_artifacts sa
	  ON sa.artifact_type = 'inventory_item' AND sa.artifact_id = ii.inventory_item_id
	WHERE ii.input_record_id = $1
	  AND (ii.canonical_item_id IS NULL OR ii.canonical_item_id = '' OR ii.canonical_item_id = ii.inventory_item_id)
	  AND (ii.reconcile_status IS NULL OR ii.reconcile_status = '' OR ii.reconcile_status = 'pending')
	ORDER BY ii.id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []pendingInventoryItemRow
	for rows.Next() {
		var (
			e                  pendingInventoryItemRow
			categoriesRaw      []byte
			aliasesRaw         []byte
			standardsRaw       []byte
			rowID              int64
			embeddingText      sql.NullString
		)
		if err := rows.Scan(
			&e.InventoryItemID, &e.ItemName, &e.CanonicalName,
			&categoriesRaw,
			&e.Manufacturer, &e.Brand,
			&e.ModelNumber, &e.PartNumber,
			&aliasesRaw, &standardsRaw,
			&rowID, &e.SearchDocument,
			&embeddingText,
		); err != nil {
			return nil, err
		}
		e.InputRecordID = recordID
		e.Categories = parseJSONStringArray(categoriesRaw)
		e.Aliases = parseJSONStringArray(aliasesRaw)
		e.Standards = parseJSONStringArray(standardsRaw)
		e.ArtifactID = e.InventoryItemID
		e.Embedding = parseVectorLiteralSafe(embeddingText.String)
		out = append(out, e)
	}
	return out, rows.Err()
}

func loadInventoryItemForSemCluster(ctx context.Context, db *sql.DB, inventoryItemID string) (*pendingInventoryItemRow, error) {
	const q = `
	SELECT ii.inventory_item_id, ii.item_name, COALESCE(ii.canonical_name, ''),
	       COALESCE(ii.item_categories, '[]'::jsonb),
	       COALESCE(ii.manufacturer, ''), COALESCE(ii.brand, ''),
	       COALESCE(ii.model_number, ''), COALESCE(ii.part_number, ''),
	       COALESCE(ii.aliases, '[]'::jsonb), COALESCE(ii.standards, '[]'::jsonb),
	       ii.input_record_id, COALESCE(ii.search_document, '')
	FROM kb.inventory_items ii
	WHERE ii.inventory_item_id = $1
	LIMIT 1`
	row := db.QueryRowContext(ctx, q, inventoryItemID)
	var (
		e                 pendingInventoryItemRow
		categoriesRaw     []byte
		aliasesRaw        []byte
		standardsRaw      []byte
	)
	if err := row.Scan(
		&e.InventoryItemID, &e.ItemName, &e.CanonicalName,
		&categoriesRaw,
		&e.Manufacturer, &e.Brand,
		&e.ModelNumber, &e.PartNumber,
		&aliasesRaw, &standardsRaw,
		&e.InputRecordID, &e.SearchDocument,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	e.Categories = parseJSONStringArray(categoriesRaw)
	e.Aliases = parseJSONStringArray(aliasesRaw)
	e.Standards = parseJSONStringArray(standardsRaw)
	return &e, nil
}

// ---- cluster store ----

// InventoryItemClusterStore implements ApplyMerge and MarkClustered for
// kb.inventory_items reconciliation, analogous to ReconcileSQLStore for entities.
type InventoryItemClusterStore struct {
	DB *sql.DB
}

// ApplyMerge folds fromItemID -> intoItemID atomically: writes a provenance row
// in kb.inventory_item_merges and updates both rows' canonical/reconcile fields.
func (s *InventoryItemClusterStore) ApplyMerge(ctx context.Context, m MergeDecision) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Fold the loser.
	const fold = `
	UPDATE kb.inventory_items
	SET canonical_item_id = $1, reconcile_status = 'merged', reconciled_at = now()
	WHERE inventory_item_id = $2`
	if _, err := tx.ExecContext(ctx, fold, m.IntoEntityID, m.FromEntityID); err != nil {
		return fmt.Errorf("(INVSC_02) fold loser %s: %w", m.FromEntityID, err)
	}

	// Elect the head.
	const head = `
	UPDATE kb.inventory_items
	SET canonical_item_id = $1, reconcile_status = 'clustered', reconciled_at = now()
	WHERE inventory_item_id = $1
	  AND (canonical_item_id IS NULL OR canonical_item_id = '' OR canonical_item_id = $1)`
	if _, err := tx.ExecContext(ctx, head, m.IntoEntityID); err != nil {
		return fmt.Errorf("(INVSC_03) elect head %s: %w", m.IntoEntityID, err)
	}

	evidence, _ := json.Marshal(m.Evidence)
	const ins = `
	INSERT INTO kb.inventory_item_merges (
	    from_item_id, into_item_id, method, confidence, reason, evidence, decided_by
	) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7)`
	if _, err := tx.ExecContext(ctx, ins,
		m.FromEntityID, m.IntoEntityID, m.Method, m.Confidence, m.Reason, evidence, m.DecidedBy,
	); err != nil {
		return fmt.Errorf("(INVSC_04) write merge provenance: %w", err)
	}

	return tx.Commit()
}

// MarkClustered sets reconcile_status='clustered' and reconciled_at for the
// given items that are still canonical heads.
func (s *InventoryItemClusterStore) MarkClustered(ctx context.Context, itemIDs []string, status string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	const q = `
	UPDATE kb.inventory_items
	SET canonical_item_id = inventory_item_id, reconcile_status = $2, reconciled_at = now()
	WHERE inventory_item_id = $1
	  AND (canonical_item_id IS NULL OR canonical_item_id = '' OR canonical_item_id = inventory_item_id)`
	for _, id := range itemIDs {
		if _, err := s.DB.ExecContext(ctx, q, id, status); err != nil {
			return fmt.Errorf("(INVSC_05) mark clustered %s: %w", id, err)
		}
	}
	return nil
}

// ---- adjudication batching and calling ----

// batchAdjudicateInvItemsAndApply batches groups, calls the LLM adjudicator,
// and applies the resulting merge decisions. Returns a set of inventory_item_ids
// that were absorbed.
func batchAdjudicateInvItemsAndApply(
	ctx context.Context,
	db *sql.DB,
	groups []semClusterInvItemGroup,
	cfg semClusterInvItemConfig,
	logger ApiTypes.JimoLogger,
) (map[string]bool, error) {
	merged := make(map[string]bool)
	if len(groups) == 0 {
		return merged, nil
	}

	groupSize := semClusterGroupSize()
	store := &InventoryItemClusterStore{DB: db}

	for start := 0; start < len(groups); start += groupSize {
		end := start + groupSize
		if end > len(groups) {
			end = len(groups)
		}
		batch := groups[start:end]

		// Clip to entity budget so one call doesn't exceed the token window.
		maxEnts := semClusterBatchMaxEntities()
		entCount := 0
		cutAt := len(batch)
		for i, g := range batch {
			entCount += len(g.Members)
			if entCount > maxEnts {
				cutAt = i
				break
			}
		}
		if cutAt > 0 && cutAt < len(batch) {
			batch = batch[:cutAt]
		}

		result, err := callInvItemAdjudicator(ctx, batch, cfg)
		if err != nil {
			return merged, err
		}

		for _, m := range result.Merges {
			if !allInvInSameGroup(batch, m.FromEntityID, m.IntoEntityID) {
				if logger != nil {
					logger.Warn("inventory item semantic clustering: cross-group merge discarded",
						"from", m.FromEntityID, "into", m.IntoEntityID)
				}
				continue
			}
			if callerErr := ctx.Err(); callerErr != nil {
				return merged, callerErr
			}
			m.Method = "semantic_cluster"
			m.DecidedBy = "semantic_cluster"
			if applyErr := store.ApplyMerge(ctx, m); applyErr != nil {
				return merged, fmt.Errorf("(INVSC_06) apply merge from=%s into=%s: %w",
					m.FromEntityID, m.IntoEntityID, applyErr)
			}
			merged[m.FromEntityID] = true
		}
	}

	return merged, nil
}

func allInvInSameGroup(batch []semClusterInvItemGroup, aID, bID string) bool {
	for _, g := range batch {
		hasA, hasB := false, false
		for _, m := range g.Members {
			if m.InventoryItemID == aID {
				hasA = true
			}
			if m.InventoryItemID == bID {
				hasB = true
			}
		}
		if hasA && hasB {
			return true
		}
	}
	return false
}

// callInvItemAdjudicator sends one batch of groups to the LLM for identity
// adjudication of inventory items.
func callInvItemAdjudicator(
	ctx context.Context,
	groups []semClusterInvItemGroup,
	cfg semClusterInvItemConfig,
) (AdjudicationResult, error) {
	if len(groups) == 0 {
		return AdjudicationResult{}, nil
	}

	inputJSON, err := json.Marshal(groups)
	if err != nil {
		return AdjudicationResult{}, fmt.Errorf("(INVSC_07) marshal groups: %w", err)
	}

	payload, err := callInvItemAdjudicatorWithModel(ctx, string(inputJSON), cfg.ModelName, cfg.ModelCfg, cfg)
	if err == nil {
		return parseInvItemAdjudicationResult(payload, groups), nil
	}

	fallbackName := strings.TrimSpace(cfg.FallbackModelName)
	if fallbackName == "" {
		return AdjudicationResult{}, err
	}

	payload, fallbackErr := callInvItemAdjudicatorWithModel(ctx, string(inputJSON), fallbackName, cfg.FallbackModelCfg, cfg)
	if fallbackErr != nil {
		return AdjudicationResult{}, fmt.Errorf("(INVSC_08) primary adjudication failed: %w; fallback failed: %v", err, fallbackErr)
	}
	return parseInvItemAdjudicationResult(payload, groups), nil
}

func callInvItemAdjudicatorWithModel(
	ctx context.Context,
	inputText string,
	modelName string,
	modelCfg structureModelConfig,
	cfg semClusterInvItemConfig,
) (map[string]any, error) {
	extractor := &llmclients.OpenAIJSONClient{
		HTTPClient: &http.Client{Timeout: time.Duration(modelCfg.TimeoutSec) * time.Second},
	}
	applyStructureModelConfigToExtractor(extractor, modelCfg)

	in := llmclients.JSONExtractionInput{
		PromptName: cfg.PromptRef,
		PromptText: cfg.PromptText,
		ModelName:  strings.TrimSpace(modelName),
		InputText:  inputText,
	}

	contract := inventoryItemAdjudicationContract()

	if structuredExtractor, ok := any(extractor).(LLMStructuredJSONExtractor); ok {
		structResult, extractErr := structuredExtractor.ExtractStructuredJSON(ctx, in, contract)
		if extractErr != nil {
			return nil, fmt.Errorf("(INVSC_09) adjudicate with %q: %w", modelName, extractErr)
		}
		if structResult == nil || structResult.Parsed == nil {
			return nil, fmt.Errorf("(INVSC_10) adjudicator %q returned nil payload", modelName)
		}
		return structResult.Parsed, nil
	}

	payload, extractErr := extractor.ExtractJSON(ctx, in)
	if extractErr != nil {
		return nil, fmt.Errorf("(INVSC_11) adjudicate with %q: %w", modelName, extractErr)
	}
	return payload, nil
}

func parseInvItemAdjudicationResult(payload map[string]any, groups []semClusterInvItemGroup) AdjudicationResult {
	groupMemberSets := make(map[string]map[string]bool, len(groups))
	for _, g := range groups {
		set := make(map[string]bool, len(g.Members))
		for _, m := range g.Members {
			set[m.InventoryItemID] = true
		}
		groupMemberSets[g.GroupID] = set
	}

	var out AdjudicationResult
	mergeMin := semClusterMergeMin()

	for _, g := range groups {
		block, ok := payload[g.GroupID]
		if !ok {
			continue
		}
		blockMap, ok := block.(map[string]any)
		if !ok {
			continue
		}

		if groupsRaw, ok := blockMap["groups"].([]any); ok {
			for _, grp := range groupsRaw {
				gm, ok := grp.(map[string]any)
				if !ok {
					continue
				}
				memberIDs := toStringSlice(gm["member_item_ids"])
				if len(memberIDs) < 2 {
					continue
				}
				if !allIDsInSet(memberIDs, groupMemberSets[g.GroupID]) {
					continue
				}
				confidence := toFloat(gm["confidence"])
				if confidence < mergeMin {
					continue
				}
				rationale := asString(gm["rationale"])
				evidence := mapFromAny(gm["evidence"])
				fromIDs, into := electSurvivorFromIDs(memberIDs)
				for _, from := range fromIDs {
					out.Merges = append(out.Merges, MergeDecision{
						FromEntityID: from,
						IntoEntityID: into,
						Method:       "semantic_cluster",
						Confidence:   clamp01(confidence),
						Reason:       rationale,
						Evidence:     evidence,
						DecidedBy:    "semantic_cluster",
					})
				}
			}
		}
	}
	return out
}

func inventoryItemAdjudicationContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_inventory_item_adjudication", map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	})
}

// ---- helpers ----

func markPendingInvItemsAsClustered(ctx context.Context, db *sql.DB, items []pendingInventoryItemRow) error {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.InventoryItemID
	}
	store := &InventoryItemClusterStore{DB: db}
	return store.MarkClustered(ctx, ids, "clustered")
}
