package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/lib/pq"
)

// inventoryItemsReviewer is the cross-document inventory-item consistency reviewer (P5,
// aspect "inventory_items"; ADR 2026063005). Like the metric reviewer (ADR 2026063002)
// it does not read the document body: it loads the document's extracted inventory items
// and compares each against semantically-related items in OTHER documents, discovered
// through the precomputed kb.artifact_connections edges (Branch A:
// hybrid_search/semantically_related item edges; Branch B: items sharing an
// item_category corpus-wide; Branch C: entity->inventory_item edges attached by shared
// category).
//
// It uses ReviewStrategy StrategyDocument and Input="artifact", so the prompt-cache
// scheduler routes it to runReviewersLegacy, which calls ReviewDocument directly.
type inventoryItemsReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	db         *sql.DB
	maxTasks   int
	maxMatches int // cap on matching items per doc item (INVENTORY_REVIEW_MAX_MATCHES)
	maxItems   int // cap on doc items reviewed; 0 = no cap (INVENTORY_REVIEW_MAX_ITEMS)
}

func (r *inventoryItemsReviewer) Name() string             { return "inventory_items" }
func (r *inventoryItemsReviewer) Group() string            { return "P5" }
func (r *inventoryItemsReviewer) Strategy() ReviewStrategy { return StrategyDocument }

// inventoryItemView is the JSON-serializable subset of an inventory item sent to the LLM.
type inventoryItemView struct {
	ItemID          string          `json:"inventory_item_id,omitempty"`
	ItemName        string          `json:"item_name,omitempty"`
	CanonicalName   string          `json:"canonical_name,omitempty"`
	Manufacturer    string          `json:"manufacturer,omitempty"`
	Brand           string          `json:"brand,omitempty"`
	ModelNumber     string          `json:"model_number,omitempty"`
	PartNumber      string          `json:"part_number,omitempty"`
	Categories      []string        `json:"item_categories,omitempty"`
	Standards       []string        `json:"standards,omitempty"`
	NormalizedSpecs json.RawMessage `json:"normalized_specs,omitempty"`
}

// docInventoryItem is one inventory item extracted from the document under review.
type docInventoryItem struct {
	view  inventoryItemView
	spans []string
}

// matchedInventoryItem is a candidate match from another document, with provenance.
type matchedInventoryItem struct {
	view       inventoryItemView
	recordID   int64
	filename   string
	via        string // "hybrid_search" | "item_category" | "entity"
	confidence float64
}

func (r *inventoryItemsReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	if r.db == nil {
		return nil, fmt.Errorf("(MID_26063060) inventory items reviewer: nil db handle")
	}

	docItems, err := r.loadRecordItems(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26063061) load inventory items for record %d: %w", recordID, err)
	}
	if len(docItems) == 0 {
		r.logger.Info("inventory items review skipped: no items", "record_id", recordID)
		return nil, nil
	}
	if r.maxItems > 0 && len(docItems) > r.maxItems {
		docItems = docItems[:r.maxItems]
	}

	matches, err := r.buildMatches(ctx, recordID, docItems)
	if err != nil {
		return nil, fmt.Errorf("(MID_26063062) build inventory item matches for record %d: %w", recordID, err)
	}

	// Only review items that have at least one cross-document match.
	type reviewUnit struct {
		di      docInventoryItem
		matches []matchedInventoryItem
	}
	var units []reviewUnit
	for i, di := range docItems {
		if ms := matches[i]; len(ms) > 0 {
			units = append(units, reviewUnit{di: di, matches: ms})
		}
	}
	if len(units) == 0 {
		r.logger.Info("inventory items review: no cross-document matches", "record_id", recordID, "items", len(docItems))
		return nil, nil
	}

	r.logger.Info("inventory items review running",
		"record_id", recordID,
		"items", len(docItems),
		"reviewed_items", len(units),
	)

	results, runErr := runReviewerConcurrent(ctx, r.maxTasks, len(units), cfg.OnProgress,
		func(workerCtx context.Context, i int) ([]ReviewFinding, error) {
			if isCtxStopped(workerCtx) {
				return nil, ErrPipelineStopped
			}
			return r.reviewItem(workerCtx, recordID, i, cfg, units[i].di, units[i].matches), nil
		},
	)
	if runErr != nil {
		if isCtxStopped(ctx) {
			return nil, ErrPipelineStopped
		}
		return nil, runErr
	}

	var all []ReviewFinding
	for _, wf := range results {
		all = append(all, wf...)
	}
	return all, nil
}

// reviewItem runs one LLM comparison for a single doc inventory item and its matches.
func (r *inventoryItemsReviewer) reviewItem(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	di docInventoryItem,
	ms []matchedInventoryItem,
) []ReviewFinding {
	start := time.Now()

	payloadObj := map[string]any{
		"inventory_item_under_review": di.view,
		"matching_items":              matchedInventoryItemsPayload(ms),
	}
	inputJSON, err := json.Marshal(payloadObj)
	if err != nil {
		r.logger.Warn("inventory items review: marshal payload failed", "record_id", recordID, "item_index", index, "error", err)
		return nil
	}

	out, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(
		ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, string(inputJSON),
		"review_inventory_items", "MID-CWB-REVIEW-INVENTORY-ITEMS"))
	if err != nil {
		r.logger.Warn("inventory items review item failed; skipping",
			"record_id", recordID, "item_index", index, "error", err)
		return nil
	}

	findings := normalizeFindingsJSON(out)
	loc := strings.Join(di.spans, ",")
	for i := range findings {
		findings[i].Pass = "P5"
		findings[i].Aspect = "inventory_items"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "issue"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "low"
		}
		if findings[i].Location == "" {
			findings[i].Location = loc
		}
	}

	r.logger.Info("inventory items review item done",
		"record_id", recordID,
		"item_index", index,
		"inventory_item_id", di.view.ItemID,
		"matches", len(ms),
		"findings", len(findings),
		"ms_used", time.Since(start).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}

func matchedInventoryItemsPayload(ms []matchedInventoryItem) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{
			"item":             m.view,
			"source_record_id": m.recordID,
			"source_filename":  m.filename,
			"match_via":        m.via,
			"confidence":       m.confidence,
		})
	}
	return out
}

// buildMatches loads the artifact-graph inputs and assembles, per doc-item index, the
// deduped & capped list of matching items across the three branches (ADR DR1). IO is
// isolated here; the pure assembly lives in assembleInventoryMatches.
func (r *inventoryItemsReviewer) buildMatches(
	ctx context.Context,
	recordID int64,
	docItems []docInventoryItem,
) (map[int][]matchedInventoryItem, error) {
	catKeySet := make(map[string]struct{})
	for _, di := range docItems {
		for _, c := range di.view.Categories {
			if c = strings.TrimSpace(c); c != "" {
				catKeySet[c] = struct{}{}
			}
		}
	}

	store := &docprocessing.ConnectionSQLStore{DB: r.db}

	// Branch A: item -> semantically related items (precomputed hybrid_search edges).
	hsEdges, err := store.LoadConnectionsBySource(ctx, recordID, "inventory_item", docprocessing.RelationMethodHybridSearch, "inventory_item")
	if err != nil {
		return nil, err
	}
	// Branch C: entity -> inventory_item edges (any relation method).
	entEdges, err := store.LoadConnectionsBySource(ctx, recordID, "entity", "", "inventory_item")
	if err != nil {
		return nil, err
	}

	// Collect all target item ids needing resolution from kb.inventory_items.
	idSet := make(map[string]struct{})
	for _, e := range hsEdges {
		if e.RelationName == docprocessing.RelationSemanticallyRelated && e.TargetID != "" {
			idSet[e.TargetID] = struct{}{}
		}
	}
	for _, e := range entEdges {
		if e.TargetID != "" {
			idSet[e.TargetID] = struct{}{}
		}
	}
	resolved, err := r.loadItemsByItemID(ctx, idSet)
	if err != nil {
		return nil, err
	}

	var siblings []resolvedInventoryItem
	if len(catKeySet) > 0 {
		cats := make([]string, 0, len(catKeySet))
		for c := range catKeySet {
			cats = append(cats, c)
		}
		if siblings, err = r.loadCategorySiblings(ctx, recordID, cats); err != nil {
			return nil, err
		}
	}

	return assembleInventoryMatches(recordID, docItems, hsEdges, entEdges, resolved, siblings, r.maxMatches), nil
}

// assembleInventoryMatches is the pure (DB-free) match-assembly used by buildMatches. It
// maps each doc-item index to its deduped, cross-document, capped matches.
func assembleInventoryMatches(
	recordID int64,
	docItems []docInventoryItem,
	hsEdges []docprocessing.Connection,
	entEdges []docprocessing.Connection,
	resolved map[string]resolvedInventoryItem,
	siblings []resolvedInventoryItem,
	maxMatches int,
) map[int][]matchedInventoryItem {
	byItemID := make(map[string]int, len(docItems))
	for i, di := range docItems {
		if di.view.ItemID != "" {
			byItemID[di.view.ItemID] = i
		}
	}

	matches := make(map[int][]matchedInventoryItem)
	dedup := make(map[int]map[string]struct{}) // docIdx -> set of matched item ids
	add := func(docIdx int, m matchedInventoryItem) {
		if m.view.ItemID == "" || m.recordID == recordID {
			return // strictly cross-document
		}
		if dedup[docIdx] == nil {
			dedup[docIdx] = make(map[string]struct{})
		}
		if _, seen := dedup[docIdx][m.view.ItemID]; seen {
			return
		}
		dedup[docIdx][m.view.ItemID] = struct{}{}
		matches[docIdx] = append(matches[docIdx], m)
	}

	// Branch A application.
	for _, e := range hsEdges {
		if e.RelationName != docprocessing.RelationSemanticallyRelated {
			continue
		}
		docIdx, ok := byItemID[e.SourceID]
		if !ok {
			continue
		}
		tm, ok := resolved[e.TargetID]
		if !ok {
			continue
		}
		add(docIdx, matchedInventoryItem{view: tm.view, recordID: tm.recordID, filename: tm.filename, via: "hybrid_search", confidence: e.Confidence})
	}

	// Branch B: items sharing a category key (corpus-wide).
	for _, sib := range siblings {
		sibCats := make(map[string]struct{}, len(sib.view.Categories))
		for _, c := range sib.view.Categories {
			sibCats[strings.TrimSpace(c)] = struct{}{}
		}
		for i, di := range docItems {
			shared := false
			for _, c := range di.view.Categories {
				if _, ok := sibCats[strings.TrimSpace(c)]; ok {
					shared = true
					break
				}
			}
			if shared {
				add(i, matchedInventoryItem{view: sib.view, recordID: sib.recordID, filename: sib.filename, via: "item_category"})
			}
		}
	}

	// Branch C: entity-connected items, attached to doc items that share a category.
	for _, e := range entEdges {
		tm, ok := resolved[e.TargetID]
		if !ok {
			continue
		}
		tmCats := make(map[string]struct{}, len(tm.view.Categories))
		for _, c := range tm.view.Categories {
			tmCats[strings.TrimSpace(c)] = struct{}{}
		}
		for i, di := range docItems {
			shared := false
			for _, c := range di.view.Categories {
				if _, ok := tmCats[strings.TrimSpace(c)]; ok {
					shared = true
					break
				}
			}
			if shared {
				add(i, matchedInventoryItem{view: tm.view, recordID: tm.recordID, filename: tm.filename, via: "entity", confidence: e.Confidence})
			}
		}
	}

	// Cap each list to maxMatches, highest-confidence first.
	for idx, list := range matches {
		sort.SliceStable(list, func(a, b int) bool { return list[a].confidence > list[b].confidence })
		if maxMatches > 0 && len(list) > maxMatches {
			list = list[:maxMatches]
		}
		matches[idx] = list
	}
	return matches
}

// inventoryItemColumns is the shared SELECT list for the doc-item load and resolution
// queries. Order must match scanInventoryItemRow.
const inventoryItemColumns = `COALESCE(inventory_item_id, ''), COALESCE(item_name, ''), COALESCE(canonical_name, ''),
       COALESCE(manufacturer, ''), COALESCE(brand, ''), COALESCE(model_number, ''),
       COALESCE(part_number, ''), COALESCE(item_categories, '[]'::jsonb),
       COALESCE(standards, '[]'::jsonb), COALESCE(normalized_specs, '[]'::jsonb)`

// scanInventoryItemRow scans one row of inventoryItemColumns into a view + record id.
// recordID is scanned from a trailing column when present (resolver/sibling queries);
// the per-record loader passes a nil recordID target.
func scanInventoryItemRow(rows *sql.Rows, view *inventoryItemView, recordID *int64, filename *string) error {
	var catsJSON, specsJSON []byte
	var standards []byte
	dst := []any{
		&view.ItemID, &view.ItemName, &view.CanonicalName,
		&view.Manufacturer, &view.Brand, &view.ModelNumber,
		&view.PartNumber, &catsJSON, &standards, &specsJSON,
	}
	if recordID != nil {
		dst = append(dst, recordID)
	}
	if filename != nil {
		dst = append(dst, filename)
	}
	if err := rows.Scan(dst...); err != nil {
		return err
	}
	view.Categories = parseJSONStringArray(catsJSON)
	view.Standards = parseJSONStringArray(standards)
	view.NormalizedSpecs = rawJSONArrayOrNil(specsJSON)
	return nil
}

func (r *inventoryItemsReviewer) loadRecordItems(ctx context.Context, recordID int64) ([]docInventoryItem, error) {
	q := `
SELECT ` + inventoryItemColumns + `, COALESCE(source_line_spans, '[]'::jsonb)
FROM kb.inventory_items
WHERE input_record_id = $1
ORDER BY id`
	rows, err := r.db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []docInventoryItem
	for rows.Next() {
		var (
			di        docInventoryItem
			catsJSON  []byte
			standards []byte
			specsJSON []byte
			spansJSON []byte
		)
		if err := rows.Scan(&di.view.ItemID, &di.view.ItemName, &di.view.CanonicalName,
			&di.view.Manufacturer, &di.view.Brand, &di.view.ModelNumber,
			&di.view.PartNumber, &catsJSON, &standards, &specsJSON, &spansJSON); err != nil {
			return nil, err
		}
		di.view.Categories = parseJSONStringArray(catsJSON)
		di.view.Standards = parseJSONStringArray(standards)
		di.view.NormalizedSpecs = rawJSONArrayOrNil(specsJSON)
		di.spans = parseJSONStringArray(spansJSON)
		out = append(out, di)
	}
	return out, rows.Err()
}

// resolvedInventoryItem is an inventory item loaded for match resolution.
type resolvedInventoryItem struct {
	view     inventoryItemView
	recordID int64
	filename string
}

func (r *inventoryItemsReviewer) loadItemsByItemID(ctx context.Context, idSet map[string]struct{}) (map[string]resolvedInventoryItem, error) {
	out := make(map[string]resolvedInventoryItem)
	if len(idSet) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	q := `
SELECT ` + inventoryItemColumns + `, m.input_record_id, COALESCE(i.staging_filename, '')
FROM kb.inventory_items m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE m.inventory_item_id = ANY($1)`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var ri resolvedInventoryItem
		if err := scanInventoryItemRow(rows, &ri.view, &ri.recordID, &ri.filename); err != nil {
			return nil, err
		}
		out[ri.view.ItemID] = ri
	}
	return out, rows.Err()
}

// inventoryCategorySiblingLimit bounds the corpus-wide category-sibling scan.
const inventoryCategorySiblingLimit = 500

func (r *inventoryItemsReviewer) loadCategorySiblings(ctx context.Context, recordID int64, cats []string) ([]resolvedInventoryItem, error) {
	q := `
SELECT ` + inventoryItemColumns + `, m.input_record_id, COALESCE(i.staging_filename, '')
FROM kb.inventory_items m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE m.input_record_id <> $1
  AND m.item_categories ?| $2
ORDER BY m.id
LIMIT $3`
	rows, err := r.db.QueryContext(ctx, q, recordID, pq.Array(cats), inventoryCategorySiblingLimit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []resolvedInventoryItem
	for rows.Next() {
		var ri resolvedInventoryItem
		if err := scanInventoryItemRow(rows, &ri.view, &ri.recordID, &ri.filename); err != nil {
			return nil, err
		}
		out = append(out, ri)
	}
	return out, rows.Err()
}

// rawJSONArrayOrNil returns raw as a json.RawMessage, or nil when it is empty/blank so
// the field is omitted from the LLM payload (omitempty). Invalid JSON yields nil.
func rawJSONArrayOrNil(raw []byte) json.RawMessage {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "[]" || s == "null" || s == "{}" {
		return nil
	}
	if !json.Valid([]byte(s)) {
		return nil
	}
	return json.RawMessage(append([]byte(nil), s...))
}
