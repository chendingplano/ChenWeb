package categorytree

import (
	"context"
	"fmt"
	"sync"
)

// Classification statuses (kb.category_classifications.status).
const (
	StatusPending      = "pending"
	StatusPublished    = "published"
	StatusFailed       = "failed"
	StatusInReview     = "in_review"
	StatusUnclassified = "unclassified"
)

// Reasons a walk does not publish.
const (
	ReasonRequestFailed   = "request_failed"
	ReasonUncertain       = "uncertain"
	ReasonBudgetExhausted = "budget_exhausted"
)

// Per-child outcomes of a decision.
const (
	OutcomeMatch     = "match"
	OutcomeNoMatch   = "no_match"
	OutcomeUncertain = "uncertain"
)

// Artifact is what is classified: its content for the decision model, its language for the
// labels shown with it, and the LLM's unverified category proposals.
type Artifact struct {
	Type      string         `json:"type"`
	ID        string         `json:"id"`
	RecordID  int64          `json:"record_id"`
	Lang      string         `json:"lang"`
	Content   map[string]any `json:"content"`
	Proposals []string       `json:"proposals"`
}

// DecisionRequest asks which of Children (ordinary children of Parent) Artifact belongs to.
type DecisionRequest struct {
	Artifact Artifact
	Parent   *Node
	Children []*Node
}

// Decider returns P(member) per child node ID. Missing children are treated as uncertain.
type Decider interface {
	Decide(ctx context.Context, req DecisionRequest) (map[string]float64, error)
}

// Options are the walk's limits (devdoc §7). Zero values take the defaults.
type Options struct {
	BatchSize   int     // R: children per request (default 20)
	MaxRequests int     // decision requests per artifact, retries included (default 60)
	Retries     int     // extra attempts per failed request (default 2; negative = none)
	Concurrency int     // requests in flight for one parent (default 4)
	MatchMinP   float64 // p >= MatchMinP is a match (default 0.7)
	NoMatchMaxP float64 // p <= NoMatchMaxP is a non-match (default 0.3)
}

func (o Options) withDefaults() Options {
	if o.BatchSize <= 0 {
		o.BatchSize = 20
	}
	if o.MaxRequests <= 0 {
		o.MaxRequests = 60
	}
	if o.Retries < 0 {
		o.Retries = 0
	} else if o.Retries == 0 {
		o.Retries = 2
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 4
	}
	if o.MatchMinP <= 0 || o.MatchMinP > 1 {
		o.MatchMinP = 0.7
	}
	if o.NoMatchMaxP <= 0 || o.NoMatchMaxP >= o.MatchMinP {
		o.NoMatchMaxP = 0.3
	}
	return o
}

// DecisionRecord is the evidence of one decision request.
type DecisionRecord struct {
	ParentID      string             `json:"parent_id"`
	ChildIDs      []string           `json:"child_ids"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Outcomes      map[string]string  `json:"outcomes,omitempty"`
	Attempts      int                `json:"attempts"`
	Error         string             `json:"error,omitempty"`
	Ignored       []string           `json:"ignored_ids,omitempty"` // answers for IDs not supplied
}

// WalkResult is the outcome of classifying one artifact.
type WalkResult struct {
	Status     string           `json:"status"` // published | failed | in_review
	Reason     string           `json:"reason,omitempty"`
	Leaves     []string         `json:"leaves,omitempty"` // proposed direct memberships
	Unresolved []string         `json:"unresolved,omitempty"`
	Decisions  []DecisionRecord `json:"decisions"`
	Requests   int              `json:"requests"`
	Options    Options          `json:"options"`
}

// Walk verifies an artifact against the tree from the root down (devdoc §5). Only a walk in
// which every visited parent got definite answers returns StatusPublished; a failed request,
// an uncertain answer or an exhausted budget leaves the artifact for retry or review and is
// never turned into a non-match.
func Walk(ctx context.Context, t *Tree, a Artifact, d Decider, opts Options) WalkResult {
	opts = opts.withDefaults()
	res := WalkResult{Options: opts}
	root := t.Root()
	if root == nil {
		return WalkResult{Status: StatusFailed, Reason: "tree has no root", Options: opts}
	}
	budget := &requestBudget{left: opts.MaxRequests}
	var failed, uncertain, exhausted bool
	leaves := map[string]bool{}
	queued := map[string]bool{root.ID: true}
	queue := []string{root.ID}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if t.IsLeaf(id) {
			leaves[id] = true
			continue
		}
		children := t.OrdinaryChildren(id)
		var batches [][]*Node
		for i := 0; i < len(children); i += opts.BatchSize {
			batches = append(batches, children[i:min(i+opts.BatchSize, len(children))])
		}
		records := make([]DecisionRecord, len(batches))
		var wg sync.WaitGroup
		sem := make(chan struct{}, opts.Concurrency)
		for i, batch := range batches {
			wg.Add(1)
			go func(i int, batch []*Node) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				records[i] = decideBatch(ctx, d, a, t.Nodes[id], batch, opts, budget)
			}(i, batch)
		}
		wg.Wait()

		var matched []string
		resolved := true
		for _, r := range records {
			res.Decisions = append(res.Decisions, r)
			switch {
			case r.Error == errBudget:
				exhausted, resolved = true, false
			case r.Error != "":
				failed, resolved = true, false
			}
			for _, cid := range r.ChildIDs {
				switch r.Outcomes[cid] {
				case OutcomeMatch:
					matched = append(matched, cid)
				case OutcomeUncertain:
					if r.Error == "" {
						uncertain, resolved = true, false
					}
				}
			}
		}
		// A match in any batch is followed even when another batch is unresolved: it gathers
		// evidence, and the attempt still does not publish.
		for _, cid := range matched {
			if !queued[cid] {
				queued[cid] = true
				queue = append(queue, cid)
			}
		}
		switch {
		case !resolved:
			res.Unresolved = append(res.Unresolved, id)
		case len(matched) == 0:
			leaves[t.OthersChild(id).ID] = true
		}
	}
	res.Requests = opts.MaxRequests - budget.left
	for _, n := range sortedNodes(t) {
		if leaves[n.ID] {
			res.Leaves = append(res.Leaves, n.ID)
		}
	}
	switch {
	case failed:
		res.Status, res.Reason = StatusFailed, ReasonRequestFailed
	case exhausted:
		res.Status, res.Reason = StatusInReview, ReasonBudgetExhausted
	case uncertain:
		res.Status, res.Reason = StatusInReview, ReasonUncertain
	default:
		res.Status = StatusPublished
	}
	if res.Status != StatusPublished {
		res.Leaves = nil // nothing partial is ever proposed for publication
	}
	return res
}

const errBudget = "request budget exhausted"

type requestBudget struct {
	mu   sync.Mutex
	left int
}

func (b *requestBudget) take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.left <= 0 {
		return false
	}
	b.left--
	return true
}

func decideBatch(ctx context.Context, d Decider, a Artifact, parent *Node, batch []*Node, opts Options, budget *requestBudget) DecisionRecord {
	r := DecisionRecord{ParentID: parent.ID, Outcomes: map[string]string{}}
	supplied := map[string]bool{}
	for _, c := range batch {
		r.ChildIDs = append(r.ChildIDs, c.ID)
		supplied[c.ID] = true
		r.Outcomes[c.ID] = OutcomeUncertain
	}
	var probs map[string]float64
	var err error
	for attempt := 0; attempt <= opts.Retries; attempt++ {
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
		if !budget.take() {
			r.Error = errBudget
			return r
		}
		r.Attempts++
		if probs, err = d.Decide(ctx, DecisionRequest{Artifact: a, Parent: parent, Children: batch}); err == nil {
			break
		}
	}
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.Probabilities = map[string]float64{}
	for id, p := range probs {
		if !supplied[id] {
			r.Ignored = append(r.Ignored, id)
			continue
		}
		if p < 0 || p > 1 {
			continue // invalid: stays uncertain
		}
		r.Probabilities[id] = p
		switch {
		case p >= opts.MatchMinP:
			r.Outcomes[id] = OutcomeMatch
		case p <= opts.NoMatchMaxP:
			r.Outcomes[id] = OutcomeNoMatch
		}
	}
	return r
}

// ValidateLeaves checks that every ID is a current leaf of t (a reviewer's resolution).
func ValidateLeaves(t *Tree, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("at least one leaf is required")
	}
	for _, id := range ids {
		n := t.Nodes[id]
		if n == nil {
			return fmt.Errorf("node %s is not in the current tree", id)
		}
		if !t.IsLeaf(id) {
			return fmt.Errorf("node %s (%s) is not a leaf", id, n.CanonicalName)
		}
	}
	return nil
}
