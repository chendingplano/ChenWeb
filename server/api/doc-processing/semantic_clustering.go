package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// ---- types ----

// semClusterGroup is a work unit for one LLM adjudication call: the new entity E
// and the existing entities that hybrid search + coarse filter found as candidates.
// It serialises to JSON for the adjudication prompt.
type semClusterGroup struct {
	GroupID string             `json:"group_id"`
	Members []semClusterMember `json:"members"`
}

type semClusterMember struct {
	EntityID           string   `json:"entity_id"`
	Entity             string   `json:"entity"`
	EntityEN           string   `json:"entity_en"`
	Aliases            []string `json:"aliases"`
	AliasesEN          []string `json:"aliases_en"`
	EntityType         string   `json:"entity_type"`
	EntityTypeEN       string   `json:"entity_type_en"`
	Desc               string   `json:"desc"`
	DescEN             string   `json:"desc_en"`
	Keywords           []string `json:"keywords"`
	KeywordsEN         []string `json:"keywords_en"`
	Categories         []string `json:"categories"`
	EntityStatus       string   `json:"entity_status"`
	InputRecordID      int64    `json:"input_record_id"`
	CanonicalEntityID  string   `json:"canonical_entity_id,omitempty"`
	ReconcileStatus    string   `json:"reconcile_status"`
	SearchCosine       float64  `json:"search_cosine,omitempty"`
	SearchLexicalScore float64  `json:"search_lexical_score,omitempty"`
}

// ---- env var accessors ----

const (
	defaultSemClusterMinBlockCosine = 0.85
	defaultSemClusterMergeMin       = 0.90
	defaultSemClusterHumanMin       = 0.60
	defaultSemClusterGroupSize      = 3
	defaultSemClusterBatchMaxEnts   = 40
)

func semClusterEnabled() bool {
	v := strings.TrimSpace(os.Getenv("SEMCLUSTER_ENABLED"))
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	return err == nil && b
}

func semClusterMinBlockCosine() float64 {
	if v := strings.TrimSpace(os.Getenv("SEMCLUSTER_MIN_BLOCK_COSINE")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultSemClusterMinBlockCosine
}

func semClusterMergeMin() float64 {
	if v := strings.TrimSpace(os.Getenv("SEMCLUSTER_MERGE_MIN")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultSemClusterMergeMin
}

func semClusterHumanMin() float64 {
	if v := strings.TrimSpace(os.Getenv("SEMCLUSTER_HUMAN_MIN")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultSemClusterHumanMin
}

func semClusterGroupSize() int {
	if v := strings.TrimSpace(os.Getenv("SEMCLUSTER_GROUP_SIZE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultSemClusterGroupSize
}

func semClusterBatchMaxEntities() int {
	if v := strings.TrimSpace(os.Getenv("SEMCLUSTER_BATCH_MAX_ENTITIES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultSemClusterBatchMaxEnts
}

// ---- config ----

// semClusterConfig holds the adjudication model and prompt resolved at startup.
type semClusterConfig struct {
	ModelName  string
	ModelCfg   structureModelConfig
	PromptText string
	PromptRef  string
	Enabled    bool
}

// ResolveSemClusterConfig loads the adjudication model and prompt from env vars.
// Returns a config where Enabled=false when the model or prompt is unavailable.
func ResolveSemClusterConfig() semClusterConfig {
	if !semClusterEnabled() {
		return semClusterConfig{Enabled: false}
	}
	_, _, modelCfg, modelErr := loadModelConfigFromEnvKeys(
		[]string{"SEMCLUSTER_ADJ_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	if modelErr != nil {
		return semClusterConfig{Enabled: false}
	}
	promptText, promptRef, _, promptErr := loadProductPromptFromEnvKeys(
		[]string{"SEMCLUSTER_ADJ_PROMPT"},
		"prompt-entity-adjudicate-v1.md",
	)
	if promptErr != nil {
		return semClusterConfig{Enabled: false}
	}
	return semClusterConfig{
		ModelName:  modelCfg.ModelName,
		ModelCfg:   modelCfg,
		PromptText: promptText,
		PromptRef:  strings.TrimSpace(promptRef),
		Enabled:    true,
	}
}

// ---- core entry point ----

// semClusterEntities is the main entry point for semantic clustering of newly
// extracted entities in one document. It is called from Phase C post-processing
// after the entity search registry has been reindexed.
func semClusterEntities(
	ctx context.Context,
	db *sql.DB,
	recordID int64,
	logger ApiTypes.JimoLogger,
) error {
	start := time.Now()
	cfg := ResolveSemClusterConfig()
	if !cfg.Enabled {
		if logger != nil {
			logger.Info("semantic clustering skipped: disabled", "record_id", recordID)
		}
		return nil
	}

	// 1. Load newly extracted (pending) entities for this record.
	entities, err := loadPendingEntitiesForSemCluster(ctx, db, recordID)
	if err != nil {
		return fmt.Errorf("(SEMCL_01) load pending entities: %w", err)
	}
	if len(entities) == 0 {
		if logger != nil {
			logger.Info("semantic clustering: no pending entities",
				"record_id", recordID,
				"ms_used", time.Since(start).Milliseconds(),
			)
		}
		return nil
	}

	if logger != nil {
		logger.Info("semantic clustering start",
			"record_id", recordID,
			"pending_entities", len(entities),
		)
	}

	blockStart := time.Now()
	searchLimit := semClusterGroupSize() * 5

	// 2. For each pending entity: hybrid search → coarse filter → build group.
	var groups []semClusterGroup
	skippedNoMatch := 0
	scfg := appconfig.GetArtifactSearchConfig()
	dict := sanitizeTSDictionary(scfg.Dictionary)

	for _, e := range entities {
		var vec []float64
		useSem := false
		if kbsearch.SemanticSearchEnabled() && len(e.Embedding) == kbsearch.ConfiguredEmbeddingDim() {
			vec = e.Embedding
			useSem = true
		}

		candidates, qErr := queryArtifactHybridCandidates(
			ctx, db, dict, strings.TrimSpace(e.SearchDocument), vec, useSem,
			searchArtifactEntity, e.ArtifactID, searchLimit,
		)
		if qErr != nil {
			if logger != nil {
				logger.Warn("semantic clustering: hybrid search failed",
					"record_id", recordID, "entity_id", e.EntityID, "error", qErr.Error())
			}
			continue
		}

		// 3. Coarse filter.
		var members []semClusterMember
		members = append(members, entityRowToSemClusterMember(e))
		minCosine := semClusterMinBlockCosine()
		for _, c := range candidates {
			cosine := 0.0
			if c.cosineSim.Valid {
				cosine = c.cosineSim.Float64
			}
			if cosine < minCosine {
				continue
			}
			cand, loadErr := loadEntityForSemCluster(ctx, db, c.artifactID)
			if loadErr != nil || cand == nil {
				continue
			}
			if cand.EntityID == e.EntityID {
				continue // self – should not happen, but guard.
			}
			if !sameEntityType(e.EntityType, e.EntityTypeEN, cand.EntityType, cand.EntityTypeEN) {
				continue
			}
			if !hasCommonCategory(e.Categories, cand.Categories) {
				continue
			}
			member := entityRowToSemClusterMember(*cand)
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
		groups = append(groups, semClusterGroup{
			GroupID: e.EntityID,
			Members: members,
		})
	}

	if logger != nil {
		logger.Info("semantic clustering candidates built",
			"record_id", recordID,
			"groups", len(groups),
			"skipped_no_match", skippedNoMatch,
			"ms_used", time.Since(blockStart).Milliseconds(),
		)
	}

	if len(groups) == 0 {
		return markPendingAsClustered(ctx, db, entities)
	}

	// 4. Batch groups, call LLM, apply merges.
	adjStart := time.Now()
	mergedIDs, err := batchAdjudicateAndApply(ctx, db, groups, cfg, logger)
	if err != nil {
		if logger != nil {
			logger.Warn("semantic clustering: adjudication failed",
				"record_id", recordID, "error", err.Error())
		}
		// Safe fallback: mark all as cluster heads (batch reconciler can re-merge later).
		return markPendingAsClustered(ctx, db, entities)
	}

	// 5. Mark entities that were NOT merged as cluster heads.
	var headIDs []string
	for _, e := range entities {
		if !mergedIDs[e.EntityID] {
			headIDs = append(headIDs, e.EntityID)
		}
	}
	store := &ReconcileSQLStore{DB: db}
	if markErr := store.MarkClustered(ctx, headIDs, "clustered"); markErr != nil && logger != nil {
		logger.Warn("semantic clustering: mark clustered failed",
			"record_id", recordID, "head_count", len(headIDs), "error", markErr.Error())
	}

	if logger != nil {
		logger.Info("semantic clustering finished",
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

// ---- pending-entity row and loading ----

type pendingEntityRow struct {
	EntityID       string
	ArtifactID     string
	SearchDocument string
	Embedding      []float64
	Entity         string
	EntityEN       string
	EntityType     string
	EntityTypeEN   string
	Aliases        []string
	AliasesEN      []string
	Desc           string
	DescEN         string
	Keywords       []string
	KeywordsEN     []string
	Categories     []string
	EntityStatus   string
	InputRecordID  int64
}

func entityRowToSemClusterMember(e pendingEntityRow) semClusterMember {
	return semClusterMember{
		EntityID:      e.EntityID,
		Entity:        e.Entity,
		EntityEN:      e.EntityEN,
		Aliases:       e.Aliases,
		AliasesEN:     e.AliasesEN,
		EntityType:    e.EntityType,
		EntityTypeEN:  e.EntityTypeEN,
		Desc:          e.Desc,
		DescEN:        e.DescEN,
		Keywords:      e.Keywords,
		KeywordsEN:    e.KeywordsEN,
		Categories:    e.Categories,
		EntityStatus:  e.EntityStatus,
		InputRecordID: e.InputRecordID,
	}
}

func loadPendingEntitiesForSemCluster(ctx context.Context, db *sql.DB, recordID int64) ([]pendingEntityRow, error) {
	const q = `
SELECT e.entity_id, e.entity, COALESCE(e.entity_en, ''),
       COALESCE(e.entity_type, ''), COALESCE(e.entity_type_en, ''),
       COALESCE(e.aliases, '[]'::jsonb), COALESCE(e.aliases_en, '[]'::jsonb),
       COALESCE(e.desc, ''), COALESCE(e.desc_en, ''),
       COALESCE(e.keywords, '[]'::jsonb), COALESCE(e.keywords_en, '[]'::jsonb),
       COALESCE(e.categories, '[]'::jsonb),
       e.entity_status, e.id AS row_id, COALESCE(e.search_document, ''),
       sa.embedding_text
FROM kb.entities e
LEFT JOIN kb.search_artifacts sa
  ON sa.artifact_type = 'entity' AND sa.artifact_id = (
    e.input_record_id::text || '_ent_' || split_part(e.entity_id, '_ent_', 2)
  )
WHERE e.input_record_id = $1
  AND (e.canonical_entity_id IS NULL OR e.canonical_entity_id = '' OR e.canonical_entity_id = e.entity_id)
  AND (e.reconcile_status IS NULL OR e.reconcile_status = '' OR e.reconcile_status = 'pending')
ORDER BY e.id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []pendingEntityRow
	for rows.Next() {
		var (
			e                                          pendingEntityRow
			aliasesRaw, aliasesEnRaw                   []byte
			keywordsRaw, keywordsEnRaw                  []byte
			categoriesRaw                              []byte
			rowID                                      int64
			embeddingText                              sql.NullString
		)
		if err := rows.Scan(
			&e.EntityID, &e.Entity, &e.EntityEN,
			&e.EntityType, &e.EntityTypeEN,
			&aliasesRaw, &aliasesEnRaw,
			&e.Desc, &e.DescEN,
			&keywordsRaw, &keywordsEnRaw,
			&categoriesRaw,
			&e.EntityStatus, &rowID, &e.SearchDocument,
			&embeddingText,
		); err != nil {
			return nil, err
		}
		e.InputRecordID = recordID
		e.Aliases = parseJSONStringArray(aliasesRaw)
		e.AliasesEN = parseJSONStringArray(aliasesEnRaw)
		e.Keywords = parseJSONStringArray(keywordsRaw)
		e.KeywordsEN = parseJSONStringArray(keywordsEnRaw)
		e.Categories = parseJSONStringArray(categoriesRaw)
		e.ArtifactID = kbsearch.BuildArtifactID(recordID, searchArtifactEntity, lastDelimitedToken(e.EntityID))
		e.Embedding = parseVectorLiteralSafe(embeddingText.String)
		out = append(out, e)
	}
	return out, rows.Err()
}

func loadEntityForSemCluster(ctx context.Context, db *sql.DB, entityID string) (*pendingEntityRow, error) {
	const q = `
SELECT e.entity_id, e.entity, COALESCE(e.entity_en, ''),
       COALESCE(e.entity_type, ''), COALESCE(e.entity_type_en, ''),
       COALESCE(e.aliases, '[]'::jsonb), COALESCE(e.aliases_en, '[]'::jsonb),
       COALESCE(e.desc, ''), COALESCE(e.desc_en, ''),
       COALESCE(e.keywords, '[]'::jsonb), COALESCE(e.keywords_en, '[]'::jsonb),
       COALESCE(e.categories, '[]'::jsonb),
       e.entity_status, e.input_record_id, COALESCE(e.search_document, '')
FROM kb.entities e
WHERE e.entity_id = $1
LIMIT 1`
	row := db.QueryRowContext(ctx, q, entityID)
	var (
		e                                          pendingEntityRow
		aliasesRaw, aliasesEnRaw                   []byte
		keywordsRaw, keywordsEnRaw                  []byte
		categoriesRaw                              []byte
	)
	if err := row.Scan(
		&e.EntityID, &e.Entity, &e.EntityEN,
		&e.EntityType, &e.EntityTypeEN,
		&aliasesRaw, &aliasesEnRaw,
		&e.Desc, &e.DescEN,
		&keywordsRaw, &keywordsEnRaw,
		&categoriesRaw,
		&e.EntityStatus, &e.InputRecordID, &e.SearchDocument,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	e.Aliases = parseJSONStringArray(aliasesRaw)
	e.AliasesEN = parseJSONStringArray(aliasesEnRaw)
	e.Keywords = parseJSONStringArray(keywordsRaw)
	e.KeywordsEN = parseJSONStringArray(keywordsEnRaw)
	e.Categories = parseJSONStringArray(categoriesRaw)
	return &e, nil
}

// ---- coarse filter helpers ----

// normTypeKey folds a type string for comparison: trim, replace underscores
// with spaces, then normalizeSurfaceForm (lowercase + collapse whitespace).
func normTypeKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return normalizeSurfaceForm(strings.ReplaceAll(s, "_", " "))
}

func sameEntityType(typeA, typeAEN, typeB, typeBEN string) bool {
	a, b := strings.TrimSpace(typeA), strings.TrimSpace(typeB)
	aEN, bEN := strings.TrimSpace(typeAEN), strings.TrimSpace(typeBEN)

	match := func(x, y string) bool {
		return x != "" && y != "" && normTypeKey(x) == normTypeKey(y)
	}
	if match(a, b) || match(aEN, bEN) || match(a, bEN) || match(aEN, b) {
		return true
	}
	// Allow if either side has no type info (provisional entities often lack type).
	if (a == "" && aEN == "") || (b == "" && bEN == "") {
		return true
	}
	return false
}

func hasCommonCategory(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return true // don't filter if either side lacks categories
	}
	set := make(map[string]bool, len(a))
	for _, x := range a {
		set[normalizeSurfaceForm(x)] = true
	}
	for _, y := range b {
		if set[normalizeSurfaceForm(y)] {
			return true
		}
	}
	return false
}

// ---- adjudication batching and calling ----

// batchAdjudicateAndApply batches groups, calls the LLM adjudicator, and applies
// the resulting merge decisions. Returns a set of entity_ids that were absorbed.
func batchAdjudicateAndApply(
	ctx context.Context,
	db *sql.DB,
	groups []semClusterGroup,
	cfg semClusterConfig,
	logger ApiTypes.JimoLogger,
) (map[string]bool, error) {
	merged := make(map[string]bool)
	if len(groups) == 0 {
		return merged, nil
	}

	groupSize := semClusterGroupSize()
	store := &ReconcileSQLStore{DB: db}

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

		result, err := callAdjudicator(ctx, batch, cfg)
		if err != nil {
			return merged, err
		}

		// Apply merges, guarding against cross-group merges.
		for _, m := range result.Merges {
			if !allInSameGroup(batch, m.FromEntityID, m.IntoEntityID) {
				if logger != nil {
					logger.Warn("semantic clustering: cross-group merge discarded",
						"from", m.FromEntityID, "into", m.IntoEntityID)
				}
				continue
			}
			if callerErr := ctx.Err(); callerErr != nil {
				return merged, callerErr
			}
			m.Method = "semantic_cluster"
			m.DecidedBy = "semantic_cluster"
			if _, applyErr := store.ApplyMerge(ctx, m); applyErr != nil {
				return merged, fmt.Errorf("(SEMCL_02) apply merge from=%s into=%s: %w",
					m.FromEntityID, m.IntoEntityID, applyErr)
			}
			merged[m.FromEntityID] = true
		}
	}

	return merged, nil
}

func allInSameGroup(batch []semClusterGroup, aID, bID string) bool {
	for _, g := range batch {
		hasA, hasB := false, false
		for _, m := range g.Members {
			if m.EntityID == aID {
				hasA = true
			}
			if m.EntityID == bID {
				hasB = true
			}
		}
		if hasA && hasB {
			return true
		}
	}
	return false
}

// callAdjudicator sends one batch of groups to the LLM for identity adjudication.
// It uses a dedicated extractor instance to avoid mutating shared state during
// concurrent Phase C runs.
func callAdjudicator(
	ctx context.Context,
	groups []semClusterGroup,
	cfg semClusterConfig,
) (AdjudicationResult, error) {
	if len(groups) == 0 {
		return AdjudicationResult{}, nil
	}

	inputJSON, err := json.Marshal(groups)
	if err != nil {
		return AdjudicationResult{}, fmt.Errorf("(SEMCL_03) marshal groups: %w", err)
	}

	extractor := &llmclients.OpenAIJSONClient{
		HTTPClient: &http.Client{Timeout: time.Duration(cfg.ModelCfg.TimeoutSec) * time.Second},
	}
	applyStructureModelConfigToExtractor(extractor, cfg.ModelCfg)

	in := llmclients.JSONExtractionInput{
		PromptName: cfg.PromptRef,
		PromptText: cfg.PromptText,
		ModelName:  strings.TrimSpace(cfg.ModelName),
		InputText:  string(inputJSON),
	}

	contract := entityAdjudicationContract()

	var payload map[string]any
	if structuredExtractor, ok := any(extractor).(LLMStructuredJSONExtractor); ok {
		structResult, extractErr := structuredExtractor.ExtractStructuredJSON(ctx, in, contract)
		if extractErr != nil {
			return AdjudicationResult{}, fmt.Errorf("(SEMCL_04) adjudicate: %w", extractErr)
		}
		if structResult == nil || structResult.Parsed == nil {
			return AdjudicationResult{}, errors.New("(SEMCL_05) adjudicator returned nil payload")
		}
		payload = structResult.Parsed
	} else {
		payload, err = extractor.ExtractJSON(ctx, in)
		if err != nil {
			return AdjudicationResult{}, fmt.Errorf("(SEMCL_06) adjudicate: %w", err)
		}
	}

	return parseAdjudicationResult(payload, groups), nil
}

// parseAdjudicationResult unpacks the LLM's per-group output into AdjudicationResult.
// It enforces the cross-group guard: a merge whose members span two groups is discarded.
func parseAdjudicationResult(payload map[string]any, groups []semClusterGroup) AdjudicationResult {
	groupMemberSets := make(map[string]map[string]bool, len(groups))
	for _, g := range groups {
		set := make(map[string]bool, len(g.Members))
		for _, m := range g.Members {
			set[m.EntityID] = true
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

		// groups[*].member_entity_ids → MergeDecision (one per absorbed entity).
		if groupsRaw, ok := blockMap["groups"].([]any); ok {
			for _, grp := range groupsRaw {
				gm, ok := grp.(map[string]any)
				if !ok {
					continue
				}
				memberIDs := toStringSlice(gm["member_entity_ids"])
				if len(memberIDs) < 2 {
					continue
				}
				// Check all members belong to this group (cross-group guard).
				if !allIDsInSet(memberIDs, groupMemberSets[g.GroupID]) {
					continue
				}
				confidence := toFloat(gm["confidence"])
				if confidence < mergeMin {
					continue // below auto-merge threshold → skip (batch reconciler handles)
				}
				rationale := asString(gm["rationale"])
				evidence := mapFromAny(gm["evidence"])
				// Survivor election is deterministic (lexicographic entity_id).
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

func allIDsInSet(ids []string, set map[string]bool) bool {
	for _, id := range ids {
		if !set[id] {
			return false
		}
	}
	return true
}

// electSurvivorFromIDs picks a cluster head deterministically from the given
// entity IDs. Lexicographic ordering is used as the deterministic tie-break;
// when the entities are loaded and richer comparison is needed, use
// electSurvivor in entity-reconciliation.go instead.
func electSurvivorFromIDs(entityIDs []string) (fromIDs []string, into string) {
	if len(entityIDs) < 2 {
		return nil, ""
	}
	into = entityIDs[0]
	for _, id := range entityIDs[1:] {
		if id < into {
			into = id
		}
	}
	fromIDs = make([]string, 0, len(entityIDs)-1)
	for _, id := range entityIDs {
		if id != into {
			fromIDs = append(fromIDs, id)
		}
	}
	return fromIDs, into
}

func entityAdjudicationContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_entity_adjudication", map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	})
}

// ---- helpers ----

func markPendingAsClustered(ctx context.Context, db *sql.DB, entities []pendingEntityRow) error {
	ids := make([]string, len(entities))
	for i, e := range entities {
		ids[i] = e.EntityID
	}
	store := &ReconcileSQLStore{DB: db}
	return store.MarkClustered(ctx, ids, "clustered")
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// parseVectorLiteralSafe parses a vector literal string like "[0.1,0.2,...]"
// and returns the float64 slice, or nil on any failure.
func parseVectorLiteralSafe(raw string) []float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	vec, err := parseVectorLiteral(raw)
	if err != nil {
		return nil
	}
	return vec
}

// mapFromAny converts an any value to map[string]any if possible.
func mapFromAny(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}
