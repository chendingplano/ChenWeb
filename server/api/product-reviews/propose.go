package productreviews

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// JSONExtractor is the one LLM capability profile construction needs: a single
// JSON-returning call for the decomposition. The handler layer wires in
// docprocessing.BuildReviewerLLMClient; tests pass a fake.
type JSONExtractor interface {
	ExtractJSON(ctx context.Context, in llmclients.JSONExtractionInput) (map[string]any, error)
}

// Proposer runs the decomposition proposal pass (spec: Decomposition proposal pass).
type Proposer struct {
	Store      Store
	Extractor  JSONExtractor
	ModelName  string
	PromptText string
	Budgets    BudgetsConfig
}

// ProposeInput carries the optional seed excerpts passed to the model.
type ProposeInput struct {
	SeedExcerpts []string
}

// proposedNode is one entry of the model's `nodes` array.
type proposedNode struct {
	Path       []string
	Label      string
	LabelEN    string
	NodeKind   string
	Aliases    []string
	Rationale  string
	Confidence float64
}

// Propose calls the model once, parses the decomposition tree, enforces the
// depth and node budgets breadth-first, and persists the kept nodes with
// origin `llm_proposed` / status `proposed`. On any failure the profile is left
// untouched and CWB_KB_PMR_011 is returned.
func (p Proposer) Propose(ctx context.Context, profileID int64, in ProposeInput) error {
	profile, err := p.Store.GetProfile(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return fmt.Errorf("profile %d not found", profileID)
	}
	nodes, err := p.Store.LoadNodes(ctx, profileID)
	if err != nil {
		return err
	}
	root := rootNode(nodes)
	if root == nil {
		return fmt.Errorf("profile %d has no product root", profileID)
	}

	raw, err := p.Extractor.ExtractJSON(ctx, p.buildInput(ctx, profile, in))
	if err != nil {
		return ErrProposalFailed(err)
	}
	parsed, err := parseProposedNodes(raw)
	if err != nil {
		return ErrProposalFailed(err)
	}

	budgets := p.Budgets.withDefaults()
	kept, discarded := planNodes(parsed, root.Label, budgets)
	if len(kept) == 0 && discarded == 0 {
		return nil // model proposed nothing usable; not an error
	}

	tx, err := p.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// pathKey → inserted node id, seeded with the existing root.
	idByPath := map[string]int64{pathKey([]string{root.Label}): root.ID}
	depthByPath := map[string]int{pathKey([]string{root.Label}): 0}
	for _, pn := range kept {
		parentKey := pathKey(pn.Path[:len(pn.Path)-1])
		parentID, ok := idByPath[parentKey]
		if !ok {
			continue // orphan: parent was not emitted or was dropped
		}
		pid := parentID
		newID, err := insertNode(ctx, tx, ProfileNode{
			ProfileID:    profileID,
			ParentNodeID: &pid,
			NodeKind:     normalizeKind(pn.NodeKind),
			Label:        pn.Label,
			LabelEN:      pn.LabelEN,
			Aliases:      pn.Aliases,
			Depth:        depthByPath[parentKey] + 1,
			Origin:       OriginLLMProposed,
			Status:       StatusProposed,
			Confidence:   pn.Confidence,
			Rationale:    pn.Rationale,
		})
		if err != nil {
			return err
		}
		key := pathKey(pn.Path)
		idByPath[key] = newID
		depthByPath[key] = depthByPath[parentKey] + 1
	}

	if discarded > 0 {
		if err := p.Store.RecordTruncation(ctx, tx, profileID, discarded); err != nil {
			return err
		}
	}
	if err := bumpVersion(ctx, tx, profileID); err != nil {
		return err
	}
	return tx.Commit()
}

func (p Proposer) buildInput(ctx context.Context, profile *Profile, in ProposeInput) llmclients.JSONExtractionInput {
	payload := map[string]any{
		"product_name":        profile.Name,
		"product_description": nilIfEmpty(profile.ProductDescription),
		"seed_excerpts":       orEmpty(in.SeedExcerpts),
	}
	body, _ := json.Marshal(payload)
	const callLoc = "MID-CWB-PMR-PROPOSE"
	return llmclients.JSONExtractionInput{
		PromptName:    llmclients.EnsurePromptName("product-structure", "product_structure", callLoc, p.ModelName),
		PromptText:    p.PromptText,
		ModelName:     p.ModelName,
		InputText:     string(body),
		CallReason:    "product_structure",
		CallLoc:       callLoc,
		DocumentFirst: true,
	}
}

// planNodes de-duplicates, drops nodes past max_depth, orders the survivors
// breadth-first, and caps them at max_nodes. It returns the kept nodes (in the
// order they must be inserted) and the count discarded by the node budget.
func planNodes(in []proposedNode, rootLabel string, b BudgetsConfig) (kept []proposedNode, discarded int) {
	rootKey := normLabel(rootLabel)
	seen := map[string]bool{}
	var eligible []proposedNode
	for _, pn := range in {
		if len(pn.Path) < 2 {
			continue // the product itself or a malformed entry
		}
		if normLabel(pn.Path[0]) != rootKey {
			continue // not rooted at this product
		}
		depth := len(pn.Path) - 1
		if depth > b.MaxDepth {
			continue // past the depth budget — not counted against the node budget
		}
		key := pathKey(pn.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		if strings.TrimSpace(pn.Label) == "" {
			pn.Label = pn.Path[len(pn.Path)-1]
		}
		if strings.TrimSpace(pn.LabelEN) == "" {
			pn.LabelEN = pn.Label
		}
		eligible = append(eligible, pn)
	}

	// Stable breadth-first: shallower paths first, original order within a depth.
	sort.SliceStable(eligible, func(i, j int) bool {
		return len(eligible[i].Path) < len(eligible[j].Path)
	})

	if len(eligible) > b.MaxNodes {
		discarded = len(eligible) - b.MaxNodes
		eligible = eligible[:b.MaxNodes]
	}
	return eligible, discarded
}

func parseProposedNodes(raw map[string]any) ([]proposedNode, error) {
	arr, ok := raw["nodes"].([]any)
	if !ok {
		return nil, fmt.Errorf("response has no `nodes` array")
	}
	out := make([]proposedNode, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		pn := proposedNode{
			Label:      asString(m["label"]),
			LabelEN:    asString(m["label_en"]),
			NodeKind:   asString(m["node_kind"]),
			Rationale:  asString(m["rationale"]),
			Confidence: asFloat(m["confidence"]),
			Aliases:    asStringSlice(m["aliases"]),
			Path:       asStringSlice(m["path"]),
		}
		if len(pn.Path) == 0 {
			continue
		}
		out = append(out, pn)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("`nodes` array held no usable entries")
	}
	return out, nil
}

func normalizeKind(k string) string {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case KindModule:
		return KindModule
	case KindPart:
		return KindPart
	default:
		return KindPart
	}
}

func rootNode(nodes []ProfileNode) *ProfileNode {
	for i := range nodes {
		if nodes[i].NodeKind == KindProduct {
			return &nodes[i]
		}
	}
	return nil
}

func normLabel(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func pathKey(path []string) string {
	parts := make([]string, len(path))
	for i, p := range path {
		parts[i] = normLabel(p)
	}
	return strings.Join(parts, "\x1f")
}

func nilIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
