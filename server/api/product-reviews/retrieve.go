package productreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Result tiers, highest precedence first. The tier is set by *what* a result
// matched, independent of *which path* found it (spec: Relevance tiering).
const (
	TierDirect        = "direct"
	TierPart          = "part"
	TierAspect        = "aspect"
	TierDocumentScope = "document_scope"
)

func tierRank(t string) int {
	switch t {
	case TierDirect:
		return 0
	case TierPart:
		return 1
	case TierAspect:
		return 2
	default:
		return 3
	}
}

func tierBonus(t string) float64 {
	// Keeps attributed results ranked above document_scope in the final sort so
	// a large scoped tier never drowns the answer (spec D4 trade-off).
	return float64(3 - tierRank(t))
}

// Result is one retrieved artifact with its provenance.
type Result struct {
	ArtifactType    string          `json:"artifact_type"`
	ArtifactID      string          `json:"artifact_id"`
	SourceRowID     int64           `json:"source_row_id"`
	InputRecordID   int64           `json:"input_record_id"`
	NodeID          *int64          `json:"node_id"`
	Tier            string          `json:"tier"`
	Score           float64         `json:"score"`
	Paths           []string        `json:"paths"`
	InclusionReason string          `json:"inclusion_reason"`
	LineSpans       json.RawMessage `json:"source_line_spans"`
	PrimaryLabel    string          `json:"primary_label"`
}

// pathHit is one (artifact, path) contribution before union.
type pathHit struct {
	Artifact   Artifact
	Path       string // "document_first" | "direct_concept" | "direct_hybrid"
	AnchorNode int64  // node whose hybrid search produced this hit (0 otherwise)
	Score      float64
}

// RetrieveInput is the assembled state a run's retrieval works from.
type RetrieveInput struct {
	Nodes         []ScopeNode
	ArtifactTypes []string
	ScopedDocs    []ScopedDoc
}

// RetrieveResult is the run's result set plus the counts the report needs.
type RetrieveResult struct {
	Results              []Result
	TruncatedCount       int
	AttributedCount      int
	DocumentScopeCount   int
	PerDocumentTruncated map[int64]int
}

// Retriever executes Path D and Path E and assembles the result set. No LLM.
type Retriever struct {
	DB     *sql.DB
	Config *Config
}

// Retrieve gathers Path D (document-first) and Path E (direct) hits for every
// requested artifact type and assembles, tiers, scores, and caps them.
func (r Retriever) Retrieve(ctx context.Context, in RetrieveInput) (*RetrieveResult, error) {
	scopedRecs := make([]int64, 0, len(in.ScopedDocs))
	for _, d := range in.ScopedDocs {
		scopedRecs = append(scopedRecs, d.InputRecordID)
	}
	conceptIDs := dedupeStrings(conceptIDsOf(in.Nodes))

	var hits []pathHit
	for _, at := range in.ArtifactTypes {
		adapter, err := AdapterFor(at)
		if err != nil {
			return nil, err
		}

		// Path D — every artifact of every scoped document.
		docFirst, err := adapter.ListByDocuments(ctx, r.DB, scopedRecs)
		if err != nil {
			return nil, err
		}
		for _, a := range docFirst {
			hits = append(hits, pathHit{Artifact: a, Path: "document_first", Score: 0.01})
		}

		// Path E — concept equality, no document restriction.
		byConcept, err := adapter.MatchBySubjectConcept(ctx, r.DB, conceptIDs)
		if err != nil {
			return nil, err
		}
		for _, a := range byConcept {
			hits = append(hits, pathHit{Artifact: a, Path: "direct_concept", Score: 1.0})
		}

		// Path E — hybrid over this type's partition, anchored per node.
		for _, n := range in.Nodes {
			if n.isAspect() && len(n.RelationTypes) > 0 {
				continue
			}
			rrf, err := rrfSearch(ctx, r.DB, []string{adapter.Partition()}, n.labelText(), n.Embedding, hybridCandidateLimit)
			if err != nil {
				return nil, err
			}
			if len(rrf) == 0 {
				continue
			}
			ids := make([]string, 0, len(rrf))
			scoreByID := make(map[string]float64, len(rrf))
			for _, h := range rrf {
				ids = append(ids, h.ArtifactID)
				scoreByID[h.ArtifactID] = h.Score
			}
			projected, err := adapter.Project(ctx, r.DB, ids)
			if err != nil {
				return nil, err
			}
			for _, a := range projected {
				hits = append(hits, pathHit{Artifact: a, Path: "direct_hybrid", AnchorNode: n.ID, Score: scoreByID[a.ArtifactID]})
			}
		}
	}

	res := assembleResults(in.Nodes, in.ScopedDocs, hits, r.Config.Budgets.withDefaults())
	return &res, nil
}

// assembleResults is the pure core: union on (type, id), node attribution,
// tiering, scoring, inclusion reasons, and the per-document / per-run caps.
// Deterministic given identical inputs (spec: Retrieval determinism).
func assembleResults(nodes []ScopeNode, scoped []ScopedDoc, hits []pathHit, budgets BudgetsConfig) RetrieveResult {
	rootID := RootNodeID(nodes)
	kindByNode := map[int64]string{}
	conceptToNodes := map[string][]int64{}
	keyToNodes := map[string][]int64{}
	for _, n := range nodes {
		kindByNode[n.ID] = n.Kind
		if n.ConceptID != "" {
			conceptToNodes[n.ConceptID] = append(conceptToNodes[n.ConceptID], n.ID)
		}
		for _, k := range n.NameKeys {
			keyToNodes[k] = append(keyToNodes[k], n.ID)
		}
	}
	scopedReason := map[int64]string{}
	scopedSet := map[int64]bool{}
	for _, d := range scoped {
		scopedSet[d.InputRecordID] = true
		if len(d.Reasons) > 0 {
			scopedReason[d.InputRecordID] = strings.Join(dedupeStrings(d.Reasons), "; ")
		}
	}

	type agg struct {
		art      Artifact
		paths    map[string]bool
		rawScore float64
		nodeHits map[int64]bool
	}
	byKey := map[string]*agg{}
	order := []string{}
	for _, h := range hits {
		key := h.Artifact.ArtifactType + "\x1f" + h.Artifact.ArtifactID
		a := byKey[key]
		if a == nil {
			a = &agg{art: h.Artifact, paths: map[string]bool{}, nodeHits: map[int64]bool{}}
			byKey[key] = a
			order = append(order, key)
		}
		a.rawScore += h.Score
		switch h.Path {
		case "document_first":
			a.paths["document_first"] = true
		case "direct_concept", "direct_hybrid":
			a.paths["direct"] = true
		}
		if h.Path == "direct_concept" {
			for _, nid := range conceptToNodes[h.Artifact.SubjectConcept] {
				a.nodeHits[nid] = true
			}
		}
		if h.Path == "direct_hybrid" && h.AnchorNode != 0 {
			a.nodeHits[h.AnchorNode] = true
		}
		for _, nid := range keyToNodes[normalizeName(h.Artifact.SubjectText)] {
			a.nodeHits[nid] = true
		}
	}

	results := make([]Result, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		tier, nodeID := tierFor(a.nodeHits, kindByNode, rootID)
		if tier == "" {
			if !scopedSet[a.art.InputRecordID] {
				continue // no node match and not in scope — nothing places it
			}
			tier = TierDocumentScope
		}
		r := Result{
			ArtifactType:  a.art.ArtifactType,
			ArtifactID:    a.art.ArtifactID,
			SourceRowID:   a.art.SourceRowID,
			InputRecordID: a.art.InputRecordID,
			Tier:          tier,
			Score:         a.rawScore + tierBonus(tier),
			Paths:         sortedKeys(a.paths),
			LineSpans:     a.art.LineSpans,
			PrimaryLabel:  a.art.PrimaryLabel,
		}
		if tier != TierDocumentScope && nodeID != 0 {
			nid := nodeID
			r.NodeID = &nid
			r.InclusionReason = fmt.Sprintf("%s-tier match: %q subject aligns with scope node #%d",
				tier, firstNonEmpty(a.art.SubjectText, a.art.PrimaryLabel), nodeID)
		} else {
			reason := scopedReason[a.art.InputRecordID]
			if reason == "" {
				reason = "document is in the profile's scope set"
			}
			r.InclusionReason = fmt.Sprintf("no scope node matched; document %d in scope (%s)",
				a.art.InputRecordID, reason)
		}
		results = append(results, r)
	}

	perDocTrunc, dropped := capPerDocument(results, budgets.PerDocumentCap)
	results = dropped
	runTrunc := 0
	results, runTrunc = capPerRun(results, budgets.MaxResults)

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].ArtifactType != results[j].ArtifactType {
			return results[i].ArtifactType < results[j].ArtifactType
		}
		return results[i].ArtifactID < results[j].ArtifactID
	})

	out := RetrieveResult{Results: results, PerDocumentTruncated: perDocTrunc}
	for _, r := range results {
		if r.Tier == TierDocumentScope {
			out.DocumentScopeCount++
		} else {
			out.AttributedCount++
		}
	}
	for _, n := range perDocTrunc {
		out.TruncatedCount += n
	}
	out.TruncatedCount += runTrunc
	return out
}

// tierFor picks the highest-precedence tier among the nodes an artifact matched.
func tierFor(nodeHits map[int64]bool, kindByNode map[int64]string, rootID int64) (string, int64) {
	best, bestNode := "", int64(0)
	consider := func(tier string, node int64) {
		if best == "" || tierRank(tier) < tierRank(best) {
			best, bestNode = tier, node
		}
	}
	for nid := range nodeHits {
		switch {
		case nid == rootID:
			consider(TierDirect, nid)
		case kindByNode[nid] == KindModule || kindByNode[nid] == KindPart:
			consider(TierPart, nid)
		case kindByNode[nid] == KindAspect:
			consider(TierAspect, nid)
		}
	}
	return best, bestNode
}

// capPerDocument keeps the highest-scoring PerDocumentCap results per document,
// dropping document_scope before attributed tiers.
func capPerDocument(results []Result, cap int) (map[int64]int, []Result) {
	trunc := map[int64]int{}
	if cap <= 0 {
		return trunc, results
	}
	byDoc := map[int64][]Result{}
	docOrder := []int64{}
	for _, r := range results {
		if _, ok := byDoc[r.InputRecordID]; !ok {
			docOrder = append(docOrder, r.InputRecordID)
		}
		byDoc[r.InputRecordID] = append(byDoc[r.InputRecordID], r)
	}
	out := make([]Result, 0, len(results))
	for _, rec := range docOrder {
		group := byDoc[rec]
		if len(group) <= cap {
			out = append(out, group...)
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			ri, rj := tierRank(group[i].Tier), tierRank(group[j].Tier)
			if (ri == 3) != (rj == 3) {
				return ri < rj // document_scope (rank 3) sorts last
			}
			if group[i].Score != group[j].Score {
				return group[i].Score > group[j].Score
			}
			return group[i].ArtifactID < group[j].ArtifactID
		})
		out = append(out, group[:cap]...)
		trunc[rec] = len(group) - cap
	}
	return trunc, out
}

// capPerRun enforces the run-level cap, removing document_scope results (lowest
// score first) before touching attributed tiers.
func capPerRun(results []Result, cap int) ([]Result, int) {
	if cap <= 0 || len(results) <= cap {
		return results, 0
	}
	excess := len(results) - cap
	var scoped, attributed []Result
	for _, r := range results {
		if r.Tier == TierDocumentScope {
			scoped = append(scoped, r)
		} else {
			attributed = append(attributed, r)
		}
	}
	ascByScore := func(s []Result) {
		sort.SliceStable(s, func(i, j int) bool {
			if s[i].Score != s[j].Score {
				return s[i].Score < s[j].Score
			}
			return s[i].ArtifactID > s[j].ArtifactID
		})
	}
	ascByScore(scoped)
	if excess >= len(scoped) {
		excess -= len(scoped)
		scoped = nil
	} else {
		scoped = scoped[excess:]
		excess = 0
	}
	if excess > 0 {
		ascByScore(attributed)
		if excess >= len(attributed) {
			attributed = nil
		} else {
			attributed = attributed[excess:]
		}
	}
	removed := len(results) - (len(scoped) + len(attributed))
	return append(attributed, scoped...), removed
}

func conceptIDsOf(nodes []ScopeNode) []string {
	out := []string{}
	for _, n := range nodes {
		if n.ConceptID != "" {
			out = append(out, n.ConceptID)
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
