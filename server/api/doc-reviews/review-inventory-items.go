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
// and compares each against semantically-related items in OTHER documents. Semantic
// similarity is discovered LIVE at review time via
// docprocessing.FindSimilarArtifactsOnTheFly (Branch A) rather than from precomputed
// hybrid_search/semantically_related edges — on-the-fly search is always fresh and needs
// no inbound/outbound edge bookkeeping. Branch B adds items sharing an item_category
// corpus-wide; Branch C adds object-anchored items retrieved before the LLM/tool loop.
//
// It uses ReviewStrategy StrategyDocument and Input="artifact", so the prompt-cache
// scheduler routes it to runReviewersLegacy, which calls ReviewDocument directly.
//
// Per ADR 2026070201 AR2/AR3 each per-item call carries the canonical scheduler
// window containing the item's span start as a cacheable prefix, and units are
// executed window-grouped (seed → stagger → remainder). With max_tool_turns > 0
// the call runs the DR10b tool loop (AR4).
type inventoryItemsReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	db           *sql.DB
	maxTasks     int
	maxMatches   int // cap on matching items per doc item (INVENTORY_REVIEW_MAX_MATCHES)
	maxItems     int // cap on doc items reviewed; 0 = no cap (INVENTORY_REVIEW_MAX_ITEMS)
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
	SourceLineSpans []string        `json:"source_line_spans,omitempty"`
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
	title      string // source document title (authority classification, AR5 §3)
	docNo      string // source document number from kb.inputs.doc_metadata
	via        string // "hybrid_search" | "item_category" | "object_anchor"
	confidence float64
	context    []map[string]any
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
	r.hydrateMatchedInventoryItemContexts(ctx, matches)

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

	// AR2: load the canonical scheduler windows so each call carries the
	// item's extraction context as a cacheable prefix. A load failure only
	// disables the window layout (units fall back to the payload-only input).
	windows, err := loadArtifactReviewWindows(ctx, recordID)
	if err != nil {
		r.logger.Warn("inventory items review: windows unavailable; reviewing without source context",
			"record_id", recordID, "error", err)
	}

	r.logger.Info("inventory items review running",
		"record_id", recordID,
		"items", len(docItems),
		"reviewed_items", len(units),
		"windows", len(windows),
	)

	// AR3: group units by source window and run seed → stagger → remainder.
	execUnits := make([]artifactReviewUnit, len(units))
	for i := range units {
		i := i
		u := units[i]
		wIdx := windowIndexForSpans(u.di.spans, windows)
		windowJSON := ""
		truncated := false
		if wIdx >= 0 {
			windowJSON = windows[wIdx].inputJSON
			truncated = spansTruncatedByWindow(u.di.spans, windows[wIdx])
		}
		execUnits[i] = artifactReviewUnit{
			windowIdx: wIdx,
			run: func(workerCtx context.Context) []ReviewFinding {
				return r.reviewItem(workerCtx, recordID, i, cfg, u.di, u.matches, windowJSON, truncated)
			},
		}
	}
	return runArtifactUnitsWindowGrouped(ctx, r.maxTasks, execUnits, cfg.OnProgress)
}

// reviewItem runs one LLM comparison for a single doc inventory item and its
// matches. windowJSON is the canonical scheduler window containing the item's
// span start (AR2); empty when no window was resolvable.
func (r *inventoryItemsReviewer) reviewItem(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	di docInventoryItem,
	ms []matchedInventoryItem,
	windowJSON string,
	truncated bool,
) []ReviewFinding {
	start := time.Now()

	payloadObj := map[string]any{
		"inventory_item_under_review": di.view,
		"artifact_line_spans":         di.spans,
		"matching_items":              matchedInventoryItemsPayload(ms),
	}
	if truncated {
		// AR2: the item's spans extend past the included window; tell the
		// model so it does not misread the cut-off as an extraction error.
		payloadObj["context_truncated"] = true
	}
	payloadJSON := marshalArtifactPayload(payloadObj)
	if payloadJSON == "" {
		r.logger.Warn("inventory items review: marshal payload failed", "record_id", recordID, "item_index", index)
		return nil
	}

	var findings []ReviewFinding
	var cacheHitTokens, cacheMissTokens int
	if cfg.MaxToolTurns > 0 && r.toolClient != nil {
		tools := selectTools(r.toolRegistry, cfg.Tools)
		userCtx := artifactReviewToolUserContext(windowJSON, artifactReviewTaskText(cfg.PromptText, payloadJSON))
		callInfo := docReviewCallInfo(ctx, map[string]any{"inv_item_id": di.view.ItemID})
		loopFindings, loopUsage, loopErr := runToolUseReview(
			ctx, r.toolClient, cfg.ModelName, cfg, cfg.PromptText,
			userCtx, tools, recordID, r.logger, "review_inventory_items", callInfo, "MID-20260706-007",
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("inventory items review tool-use loop failed; no findings for item",
				"record_id", recordID, "item_index", index, "error", loopErr)
		}
		findings = loopFindings
	} else {
		// AR2 window-first layout: the window is the document input; the rubric
		// plus the per-item payload form the task tail. Without a window the
		// payload itself is the document input (pre-AR2 layout).
		promptText, inputText := cfg.PromptText, payloadJSON
		if windowJSON != "" {
			promptText, inputText = artifactReviewTaskText(cfg.PromptText, payloadJSON), windowJSON
		}
		out, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(
			ctx, cfg.PromptRef, promptText, cfg.ModelName, inputText,
			"review_inventory_items", "MID-CWB-REVIEW-INVENTORY-ITEMS"))
		if err != nil {
			r.logger.Warn("inventory items review item failed; skipping",
				"record_id", recordID, "item_index", index, "error", err)
			return nil
		}
		findings = normalizeFindingsJSON(out)
		cacheHitTokens = reviewLLMCacheHitTokens(r.client)
		cacheMissTokens = reviewLLMCacheMissTokens(r.client)
	}
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
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}

// matchedInventoryItemsPayload serializes the matched candidates. Raw RRF
// scores are replaced by 1-based rank (AR5 §4), and each match carries its
// source document's authority class (AR5 §3).
func matchedInventoryItemsPayload(ms []matchedInventoryItem) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for i, m := range ms {
		out = append(out, map[string]any{
			"item":                 m.view,
			"source_record_id":     m.recordID,
			"source_filename":      m.filename,
			"source_doc_authority": docAuthorityClass(m.docNo, m.title, m.filename),
			"match_via":            m.via,
			"match_rank":           i + 1,
			"source_context":       m.context,
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

	// Branch A: semantically-similar items discovered LIVE (no materialized edges). A single
	// hybrid search per doc item finds close items regardless of when the other document was
	// indexed, so no inbound/outbound edge bookkeeping is needed.
	hybridMatches := make(map[int][]docprocessing.OnTheFlySemanticMatch, len(docItems))
	for i, di := range docItems {
		if di.view.ItemID == "" {
			continue
		}
		hits, err := docprocessing.FindSimilarArtifactsOnTheFly(ctx, r.db, "inventory_item", di.view.ItemID, "inventory_item", r.maxMatches)
		if err != nil {
			return nil, err
		}
		if len(hits) > 0 {
			hybridMatches[i] = hits
		}
	}

	artifactIDs := make([]string, len(docItems))
	for i, di := range docItems {
		artifactIDs[i] = di.view.ItemID
	}
	objectPeerIDs, err := loadObjectAnchoredPeerIDs(ctx, r.db, recordID, "inventory_item", artifactIDs)
	if err != nil {
		return nil, err
	}

	// Collect every match-side item id needing resolution.
	idSet := make(map[string]struct{})
	for _, hits := range hybridMatches {
		for _, h := range hits {
			if h.ArtifactID != "" {
				idSet[h.ArtifactID] = struct{}{}
			}
		}
	}
	for _, ids := range objectPeerIDs {
		for _, id := range ids {
			if id != "" {
				idSet[id] = struct{}{}
			}
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

	objectMatches := make(map[int][]resolvedInventoryItem, len(objectPeerIDs))
	for docIdx, ids := range objectPeerIDs {
		for _, id := range ids {
			if ri, ok := resolved[id]; ok {
				objectMatches[docIdx] = append(objectMatches[docIdx], ri)
			}
		}
	}

	return assembleInventoryMatches(recordID, docItems, hybridMatches, objectMatches, resolved, siblings, r.maxMatches), nil
}

// assembleInventoryMatches is the pure (DB-free) match-assembly used by buildMatches. It
// maps each doc-item index to its deduped, cross-document, capped matches.
func assembleInventoryMatches(
	recordID int64,
	docItems []docInventoryItem,
	hybridMatches map[int][]docprocessing.OnTheFlySemanticMatch,
	objectMatches map[int][]resolvedInventoryItem,
	resolved map[string]resolvedInventoryItem,
	siblings []resolvedInventoryItem,
	maxMatches int,
) map[int][]matchedInventoryItem {
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

	// Branch A: semantically-similar items from the live hybrid search, keyed by doc item
	// index. The add() filter drops same-document and duplicate hits.
	for docIdx, hits := range hybridMatches {
		for _, h := range hits {
			tm, ok := resolved[h.ArtifactID]
			if !ok {
				continue
			}
			add(docIdx, matchedInventoryItem{view: tm.view, recordID: tm.recordID, filename: tm.filename, title: tm.title, docNo: tm.docNo, via: "hybrid_search", confidence: h.RRFScore})
		}
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
				add(i, matchedInventoryItem{view: sib.view, recordID: sib.recordID, filename: sib.filename, title: sib.title, docNo: sib.docNo, via: "item_category"})
			}
		}
	}

	// Branch C: object-anchored items, keyed directly by the inventory-item-under-review index.
	for docIdx, peers := range objectMatches {
		for _, tm := range peers {
			add(docIdx, matchedInventoryItem{view: tm.view, recordID: tm.recordID, filename: tm.filename, title: tm.title, docNo: tm.docNo, via: "object_anchor"})
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
       COALESCE(standards, '[]'::jsonb), COALESCE(normalized_specs, '[]'::jsonb),
       COALESCE(source_line_spans, '[]'::jsonb)`

// scanInventoryItemRow scans one row of inventoryItemColumns into a resolved
// item (view + record id + source-document fields from the trailing columns).
func scanInventoryItemRow(rows *sql.Rows, ri *resolvedInventoryItem) error {
	var catsJSON, specsJSON []byte
	var standards []byte
	var spansJSON []byte
	dst := []any{
		&ri.view.ItemID, &ri.view.ItemName, &ri.view.CanonicalName,
		&ri.view.Manufacturer, &ri.view.Brand, &ri.view.ModelNumber,
		&ri.view.PartNumber, &catsJSON, &standards, &specsJSON,
		&spansJSON,
		&ri.recordID, &ri.filename, &ri.title, &ri.docNo,
	}
	if err := rows.Scan(dst...); err != nil {
		return err
	}
	ri.view.Categories = parseJSONStringArray(catsJSON)
	ri.view.Standards = parseJSONStringArray(standards)
	ri.view.NormalizedSpecs = rawJSONArrayOrNil(specsJSON)
	ri.view.SourceLineSpans = parseJSONStringArray(spansJSON)
	return nil
}

func (r *inventoryItemsReviewer) loadRecordItems(ctx context.Context, recordID int64) ([]docInventoryItem, error) {
	q := `
SELECT ` + inventoryItemColumns + `
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
		di.view.SourceLineSpans = append([]string(nil), di.spans...)
		out = append(out, di)
	}
	return out, rows.Err()
}

// resolvedInventoryItem is an inventory item loaded for match resolution.
type resolvedInventoryItem struct {
	view     inventoryItemView
	recordID int64
	filename string
	title    string
	docNo    string
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
SELECT ` + inventoryItemColumns + `, m.input_record_id, COALESCE(i.staging_filename, ''),
       COALESCE(i.title, ''), COALESCE(i.doc_metadata->>'doc_no', '')
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
		if err := scanInventoryItemRow(rows, &ri); err != nil {
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
SELECT ` + inventoryItemColumns + `, m.input_record_id, COALESCE(i.staging_filename, ''),
       COALESCE(i.title, ''), COALESCE(i.doc_metadata->>'doc_no', '')
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
		if err := scanInventoryItemRow(rows, &ri); err != nil {
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

func (r *inventoryItemsReviewer) hydrateMatchedInventoryItemContexts(ctx context.Context, matches map[int][]matchedInventoryItem) {
	if len(matches) == 0 {
		return
	}
	linesByRecord := make(map[int64][]Line)
	failedRecords := make(map[int64]bool)
	for idx, list := range matches {
		for i := range list {
			spans := list[i].view.SourceLineSpans
			if len(spans) == 0 || list[i].recordID <= 0 {
				continue
			}
			lines, ok := linesByRecord[list[i].recordID]
			if !ok {
				if failedRecords[list[i].recordID] {
					continue
				}
				var err error
				lines, err = loadRecordLines(ctx, list[i].recordID)
				if err != nil {
					failedRecords[list[i].recordID] = true
					if r.logger != nil {
						r.logger.Warn("inventory items review: matched item context unavailable",
							"source_record_id", list[i].recordID,
							"inventory_item_id", list[i].view.ItemID,
							"error", err,
						)
					}
					continue
				}
				linesByRecord[list[i].recordID] = lines
			}
			list[i].context = artifactSourceContextLines(lines, spans)
		}
		matches[idx] = list
	}
}
