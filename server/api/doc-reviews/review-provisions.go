package docreviews

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/lib/pq"
)

// provisionsReviewer is the cross-document provision consistency reviewer (P5, aspect
// "provisions"; ADR 2026063003). Like the metric reviewer it does not read the document
// body: it loads the document's extracted provisions and compares each against
// semantically-related provisions in OTHER documents. Semantic similarity is discovered
// LIVE at review time via docprocessing.FindSimilarArtifactsOnTheFly (Branch A) rather
// than from precomputed hybrid_search/semantically_related edges — on-the-fly search is
// always fresh and needs no inbound/outbound edge bookkeeping. Branch B adds
// object-anchored provisions retrieved before the LLM/tool loop.
//
// ReviewStrategy StrategyDocument + Input="artifact" routes it to runReviewersLegacy,
// which calls ReviewDocument directly.
//
// Per ADR 2026070201 AR2/AR3 each per-provision call carries the canonical
// scheduler window containing the provision's span start as a cacheable
// prefix, and units are executed window-grouped (seed → stagger → remainder).
// With max_tool_turns > 0 the call runs the DR10b tool loop (AR4).
type provisionsReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	db           *sql.DB
	maxTasks     int
	maxMatches   int // cap on matching provisions per doc provision (PROVISION_REVIEW_MAX_MATCHES)
	maxProvision int // cap on doc provisions reviewed; 0 = no cap (PROVISION_REVIEW_MAX_PROVISIONS)
}

func (r *provisionsReviewer) Name() string             { return "provisions" }
func (r *provisionsReviewer) Group() string            { return "P5" }
func (r *provisionsReviewer) Strategy() ReviewStrategy { return StrategyDocument }

// provisionView is the JSON-serializable subset of a provision sent to the LLM.
type provisionView struct {
	ProvID          string   `json:"prov_id,omitempty"`
	ProvName        string   `json:"prov_name,omitempty"`
	Type            string   `json:"provision_type,omitempty"`
	Provision       string   `json:"provision,omitempty"`
	Subject         string   `json:"provision_subject,omitempty"`
	Categories      []string `json:"category_paths,omitempty"`
	SourceLineSpans []string `json:"source_line_spans,omitempty"`
}

// docProvision is one provision extracted from the document under review.
type docProvision struct {
	view  provisionView
	spans []string
}

// matchedProvision is a candidate match from another document, with provenance.
type matchedProvision struct {
	view       provisionView
	recordID   int64
	filename   string
	title      string // source document title (authority classification, AR5 §3)
	docNo      string // source document number from kb.inputs.doc_metadata
	via        string // "hybrid_search" | "object_anchor"
	confidence float64
	context    []map[string]any
}

func (r *provisionsReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	if r.db == nil {
		return nil, fmt.Errorf("(MID_26063020) provisions reviewer: nil db handle")
	}

	docProvs, err := r.loadRecordProvisions(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26063021) load provisions for record %d: %w", recordID, err)
	}
	if len(docProvs) == 0 {
		r.logger.Info("provisions review skipped: no provisions", "record_id", recordID)
		return nil, nil
	}
	if r.maxProvision > 0 && len(docProvs) > r.maxProvision {
		docProvs = docProvs[:r.maxProvision]
	}

	matches, err := r.buildMatches(ctx, recordID, docProvs)
	if err != nil {
		return nil, fmt.Errorf("(MID_26063022) build provision matches for record %d: %w", recordID, err)
	}
	r.hydrateMatchedProvisionContexts(ctx, matches)

	units := buildProvisionReviewUnits(docProvs, matches)
	if len(units) == 0 {
		r.logger.Info("provisions review skipped: no review units", "record_id", recordID, "provisions", len(docProvs))
		return nil, nil
	}

	// AR2: load the canonical scheduler windows so each call carries the
	// provision's extraction context as a cacheable prefix. A load failure only
	// disables the window layout (units fall back to the payload-only input).
	windows, err := loadArtifactReviewWindows(ctx, recordID)
	if err != nil {
		r.logger.Warn("provisions review: windows unavailable; reviewing without source context",
			"record_id", recordID, "error", err)
	}

	r.logger.Info("provisions review running",
		"record_id", recordID,
		"provisions", len(docProvs),
		"reviewed_provisions", len(units),
		"windows", len(windows),
	)

	// AR3: group units by source window and run seed → stagger → remainder.
	execUnits := make([]artifactReviewUnit, len(units))
	for i := range units {
		i := i
		u := units[i]
		wIdx := windowIndexForSpans(u.dp.spans, windows)
		windowJSON := ""
		truncated := false
		if wIdx >= 0 {
			windowJSON = windows[wIdx].inputJSON
			truncated = spansTruncatedByWindow(u.dp.spans, windows[wIdx])
		}
		execUnits[i] = artifactReviewUnit{
			windowIdx: wIdx,
			run: func(workerCtx context.Context) []ReviewFinding {
				return r.reviewProvision(workerCtx, recordID, i, cfg, u.dp, u.matches, windowJSON, truncated)
			},
		}
	}
	return runArtifactUnitsWindowGrouped(ctx, r.maxTasks, execUnits, cfg.OnProgress)
}

type provisionReviewUnit struct {
	dp      docProvision
	matches []matchedProvision
}

func buildProvisionReviewUnits(docProvs []docProvision, matches map[int][]matchedProvision) []provisionReviewUnit {
	units := make([]provisionReviewUnit, 0, len(docProvs))
	for i, dp := range docProvs {
		units = append(units, provisionReviewUnit{dp: dp, matches: matches[i]})
	}
	return units
}

// reviewProvision runs one LLM comparison for a single doc provision and its
// matches. windowJSON is the canonical scheduler window containing the
// provision's span start (AR2); empty when no window was resolvable.
func (r *provisionsReviewer) reviewProvision(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	dp docProvision,
	ms []matchedProvision,
	windowJSON string,
	truncated bool,
) []ReviewFinding {
	start := time.Now()

	payloadObj := map[string]any{
		"provision_under_review": dp.view,
		"artifact_line_spans":    dp.spans,
		"matching_provisions":    matchedProvisionsPayload(ms),
	}
	if truncated {
		// AR2: the provision's spans extend past the included window; tell the
		// model so it does not misread the cut-off as an extraction error.
		payloadObj["context_truncated"] = true
	}
	payloadJSON := marshalArtifactPayload(payloadObj)
	if payloadJSON == "" {
		r.logger.Warn("provisions review: marshal payload failed", "record_id", recordID, "provision_index", index)
		return nil
	}

	var findings []ReviewFinding
	var cacheHitTokens, cacheMissTokens int
	if cfg.MaxToolTurns > 0 && r.toolClient != nil {
		tools := selectTools(r.toolRegistry, cfg.Tools)
		userCtx := artifactReviewToolUserContext(windowJSON, artifactReviewTaskText(cfg.PromptText, payloadJSON))
		loopFindings, loopUsage, loopErr := runToolUseReview(
			ctx, r.toolClient, cfg.ModelName, cfg, cfg.PromptText,
			userCtx, tools, recordID, r.logger,
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("provisions review tool-use loop failed; no findings for provision",
				"record_id", recordID, "provision_index", index, "error", loopErr)
		}
		findings = loopFindings
	} else {
		// AR2 window-first layout: the window is the document input; the rubric
		// plus the per-provision payload form the task tail. Without a window
		// the payload itself is the document input (pre-AR2 layout).
		promptText, inputText := cfg.PromptText, payloadJSON
		if windowJSON != "" {
			promptText, inputText = artifactReviewTaskText(cfg.PromptText, payloadJSON), windowJSON
		}
		out, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInputWithMetadata(
			ctx, cfg.PromptRef, promptText, cfg.ModelName, inputText,
			"review-provision", "MID-20260706-0001", provisionReviewCallMetadata(ctx, dp.view.ProvID)))
		if err != nil {
			r.logger.Warn("provisions review provision failed; skipping",
				"record_id", recordID, "provision_index", index, "error", err)
			return nil
		}
		findings = normalizeFindingsJSON(out)
		cacheHitTokens = reviewLLMCacheHitTokens(r.client)
		cacheMissTokens = reviewLLMCacheMissTokens(r.client)
	}
	loc := strings.Join(dp.spans, ",")
	for i := range findings {
		findings[i].Pass = "P5"
		findings[i].Aspect = "provisions"
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

	r.logger.Info("provisions review provision done",
		"record_id", recordID,
		"provision_index", index,
		"prov_id", dp.view.ProvID,
		"matches", len(ms),
		"findings", len(findings),
		"ms_used", time.Since(start).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}

// provisionReviewCallMetadata builds the llm_usage_event.metadata_json payload
// for one provision-review LLM call (ADR 2026070501).
func provisionReviewCallMetadata(ctx context.Context, provID string) map[string]any {
	meta := map[string]any{"provision_id": provID}
	if runID := llmRunIDFromContext(ctx); runID > 0 {
		meta["run_id"] = runID
	}
	return meta
}

// matchedProvisionsPayload serializes the matched candidates. Raw RRF scores
// are replaced by 1-based rank (AR5 §4), and each match carries its source
// document's authority class (AR5 §3).
func matchedProvisionsPayload(ms []matchedProvision) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for i, m := range ms {
		out = append(out, map[string]any{
			"provision":            m.view,
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

// buildMatches loads the artifact-graph inputs and assembles, per doc-provision index,
// the deduped & capped list of matching provisions across the two branches (ADR DR1).
func (r *provisionsReviewer) buildMatches(
	ctx context.Context,
	recordID int64,
	docProvs []docProvision,
) (map[int][]matchedProvision, error) {
	// Branch A: semantically-similar provisions discovered LIVE (no materialized edges). A
	// single hybrid search per doc provision finds close provisions regardless of when the
	// other document was indexed, so no inbound/outbound edge bookkeeping is needed.
	hybridMatches := make(map[int][]docprocessing.OnTheFlySemanticMatch, len(docProvs))
	for i, dp := range docProvs {
		if dp.view.ProvID == "" {
			continue
		}
		hits, err := docprocessing.FindSimilarArtifactsOnTheFly(ctx, r.db, "provision", dp.view.ProvID, "provision", r.maxMatches)
		if err != nil {
			return nil, err
		}
		if len(hits) > 0 {
			hybridMatches[i] = hits
		}
	}

	artifactIDs := make([]string, len(docProvs))
	for i, dp := range docProvs {
		artifactIDs[i] = dp.view.ProvID
	}
	objectPeerIDs, err := loadObjectAnchoredPeerIDs(ctx, r.db, recordID, "provision", artifactIDs)
	if err != nil {
		return nil, err
	}

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
	resolved, err := r.loadProvisionsByProvID(ctx, idSet)
	if err != nil {
		return nil, err
	}

	objectMatches := make(map[int][]resolvedProvision, len(objectPeerIDs))
	for docIdx, ids := range objectPeerIDs {
		for _, id := range ids {
			if rp, ok := resolved[id]; ok {
				objectMatches[docIdx] = append(objectMatches[docIdx], rp)
			}
		}
	}

	return assembleProvisionMatches(recordID, docProvs, hybridMatches, objectMatches, resolved, r.maxMatches), nil
}

// assembleProvisionMatches is the pure (DB-free) match-assembly used by buildMatches.
func assembleProvisionMatches(
	recordID int64,
	docProvs []docProvision,
	hybridMatches map[int][]docprocessing.OnTheFlySemanticMatch,
	objectMatches map[int][]resolvedProvision,
	resolved map[string]resolvedProvision,
	maxMatches int,
) map[int][]matchedProvision {
	matches := make(map[int][]matchedProvision)
	dedup := make(map[int]map[string]struct{})
	add := func(docIdx int, m matchedProvision) {
		if m.view.ProvID == "" || m.recordID == recordID {
			return // strictly cross-document
		}
		if dedup[docIdx] == nil {
			dedup[docIdx] = make(map[string]struct{})
		}
		if _, seen := dedup[docIdx][m.view.ProvID]; seen {
			return
		}
		dedup[docIdx][m.view.ProvID] = struct{}{}
		matches[docIdx] = append(matches[docIdx], m)
	}

	// Branch A: semantically-similar provisions from the live hybrid search, keyed by doc
	// provision index. The add() filter drops same-document and duplicate hits.
	for docIdx, hits := range hybridMatches {
		for _, h := range hits {
			tp, ok := resolved[h.ArtifactID]
			if !ok {
				continue
			}
			add(docIdx, matchedProvision{view: tp.view, recordID: tp.recordID, filename: tp.filename, title: tp.title, docNo: tp.docNo, via: "hybrid_search", confidence: h.RRFScore})
		}
	}

	// Branch B: object-anchored provisions, keyed directly by the provision-under-review index.
	for docIdx, peers := range objectMatches {
		for _, tp := range peers {
			add(docIdx, matchedProvision{view: tp.view, recordID: tp.recordID, filename: tp.filename, title: tp.title, docNo: tp.docNo, via: "object_anchor"})
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

func (r *provisionsReviewer) loadRecordProvisions(ctx context.Context, recordID int64) ([]docProvision, error) {
	const q = `
SELECT COALESCE(prov_id, ''), COALESCE(prov_name, ''), COALESCE(provision_type, ''),
       COALESCE(provision, ''), COALESCE(provision_subject, ''),
       COALESCE(category_paths, '[]'::jsonb), COALESCE(source_line_spans, '[]'::jsonb)
FROM kb.provisions
WHERE input_record_id = $1
ORDER BY id`
	rows, err := r.db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []docProvision
	for rows.Next() {
		var (
			dp        docProvision
			catsJSON  []byte
			spansJSON []byte
		)
		if err := rows.Scan(&dp.view.ProvID, &dp.view.ProvName, &dp.view.Type,
			&dp.view.Provision, &dp.view.Subject, &catsJSON, &spansJSON); err != nil {
			return nil, err
		}
		dp.view.Categories = parseJSONStringArray(catsJSON)
		dp.spans = parseJSONStringArray(spansJSON)
		dp.view.SourceLineSpans = append([]string(nil), dp.spans...)
		out = append(out, dp)
	}
	return out, rows.Err()
}

// resolvedProvision is a provision loaded for match resolution.
type resolvedProvision struct {
	view     provisionView
	recordID int64
	filename string
	title    string
	docNo    string
}

func (r *provisionsReviewer) loadProvisionsByProvID(ctx context.Context, idSet map[string]struct{}) (map[string]resolvedProvision, error) {
	out := make(map[string]resolvedProvision)
	if len(idSet) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	const q = `
SELECT COALESCE(p.prov_id, ''), p.input_record_id, COALESCE(p.prov_name, ''),
       COALESCE(p.provision_type, ''), COALESCE(p.provision, ''), COALESCE(p.provision_subject, ''),
       COALESCE(p.category_paths, '[]'::jsonb), COALESCE(p.source_line_spans, '[]'::jsonb),
       COALESCE(i.staging_filename, ''),
       COALESCE(i.title, ''), COALESCE(i.doc_metadata->>'doc_no', '')
FROM kb.provisions p
LEFT JOIN kb.inputs i ON i.id = p.input_record_id
WHERE p.prov_id = ANY($1)`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			rp        resolvedProvision
			catsJSON  []byte
			spansJSON []byte
		)
		if err := rows.Scan(&rp.view.ProvID, &rp.recordID, &rp.view.ProvName, &rp.view.Type,
			&rp.view.Provision, &rp.view.Subject, &catsJSON, &spansJSON, &rp.filename,
			&rp.title, &rp.docNo); err != nil {
			return nil, err
		}
		rp.view.Categories = parseJSONStringArray(catsJSON)
		rp.view.SourceLineSpans = parseJSONStringArray(spansJSON)
		out[rp.view.ProvID] = rp
	}
	return out, rows.Err()
}

func (r *provisionsReviewer) hydrateMatchedProvisionContexts(ctx context.Context, matches map[int][]matchedProvision) {
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
						r.logger.Warn("provisions review: matched provision context unavailable",
							"source_record_id", list[i].recordID,
							"prov_id", list[i].view.ProvID,
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
