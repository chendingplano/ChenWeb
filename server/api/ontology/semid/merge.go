package semid

import (
	"errors"
	"fmt"
)

// MergeGraph tracks pairwise canonical-identity merge decisions with
// tombstones (DR15, ADR kernel tests 19-21). A merge sets merged_into on the
// losing node; resolving follows the chain. There is NO transitive closure:
// pairwise decisions never fabricate a new decision row (test 20) -- A->B and
// B->C resolve A to C, but no A->C decision is created.
type MergeGraph struct {
	mergedInto map[string]string
	neverMerge map[string]map[string]bool
}

// NewMergeGraph returns an empty merge graph.
func NewMergeGraph() *MergeGraph {
	return &MergeGraph{
		mergedInto: map[string]string{},
		neverMerge: map[string]map[string]bool{},
	}
}

// SetNeverMerge marks the unordered pair (a, b) as never-mergeable. Any
// automatic merge between them is refused (test 21).
func (g *MergeGraph) SetNeverMerge(a, b string) {
	if a == "" || b == "" || a == b {
		return
	}
	if g.neverMerge[a] == nil {
		g.neverMerge[a] = map[string]bool{}
	}
	if g.neverMerge[b] == nil {
		g.neverMerge[b] = map[string]bool{}
	}
	g.neverMerge[a][b] = true
	g.neverMerge[b][a] = true
}

// IsNeverMerge reports whether the unordered pair is never-mergeable.
func (g *MergeGraph) IsNeverMerge(a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}
	return g.neverMerge[a][b]
}

// Merge sets from.merged_into = to. It refuses self-merges, merges that
// involve a never_merge pair, and merging a node that is already merged (it
// must be unmerged first). The losing node is kept as a tombstone.
func (g *MergeGraph) Merge(from, to string) error {
	if from == "" || to == "" {
		return errors.New("merge requires two node ids")
	}
	if from == to {
		return errors.New("cannot merge a node into itself")
	}
	if g.IsNeverMerge(from, to) {
		return fmt.Errorf("merge %s -> %s blocked by never_merge", from, to)
	}
	if g.mergedInto[from] != "" {
		return fmt.Errorf("node %s is already merged into %s; unmerge first", from, g.mergedInto[from])
	}
	g.mergedInto[from] = to
	return nil
}

// MergedInto returns the direct tombstone target of a node, if any.
func (g *MergeGraph) MergedInto(id string) (string, bool) {
	to, ok := g.mergedInto[id]
	return to, ok
}

// Resolve follows the merged_into chain to the surviving canonical node.
func (g *MergeGraph) Resolve(id string) string {
	cur := id
	seen := map[string]bool{}
	for {
		if seen[cur] {
			return cur // cycle guard; should not happen
		}
		seen[cur] = true
		to, ok := g.mergedInto[cur]
		if !ok {
			return cur
		}
		cur = to
	}
}

// Unmerge clears a node's tombstone, restoring it as an independent node
// (test 19). It is the split operation.
func (g *MergeGraph) Unmerge(id string) {
	delete(g.mergedInto, id)
}
