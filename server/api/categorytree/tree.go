// Package categorytree implements category trees and the filing of artifacts under them
// (devdoc 2026102003-devdoc-categories, openspec change category-trees).
//
// A tree is prepared in advance, one per artifact type, and changed only by logged change
// sets (changeset.go). Classification (classify.go) never changes the tree: a decision
// model walks down it and the artifact is bound to the existing leaves that fit, or to an
// "others" leaf. Only direct leaf memberships are stored; ancestors follow from parent links.
package categorytree

import (
	"sort"
	"strings"
)

// Languages every node must carry a preferred label and a definition in.
var requiredLangs = []string{"en", "zh"}

// LangText is one text per BCP 47 language tag.
type LangText map[string]string

// LangList is zero or more texts per language tag.
type LangList map[string][]string

// Match links a node to a concept in another tree or an external vocabulary
// (skos:exactMatch or skos:closeMatch).
type Match struct {
	Type string `json:"type"` // exact | close
	IRI  string `json:"iri"`
}

// Node is one category. ParentID is empty only for the root.
type Node struct {
	ID            string   `json:"id"`
	TreeID        string   `json:"tree_id"`
	ParentID      string   `json:"parent_id,omitempty"`
	CanonicalName string   `json:"canonical_name"`
	IsOthers      bool     `json:"is_others"`
	PrefLabels    LangText `json:"pref_labels"`
	AltLabels     LangList `json:"alt_labels,omitempty"`
	HiddenLabels  LangList `json:"hidden_labels,omitempty"`
	Definitions   LangText `json:"definitions"`
	ScopeNotes    LangText `json:"scope_notes,omitempty"`
	Source        string   `json:"source,omitempty"`
	Matches       []Match  `json:"matches,omitempty"`
}

func (n *Node) clone() *Node {
	c := *n
	c.PrefLabels = cloneText(n.PrefLabels)
	c.Definitions = cloneText(n.Definitions)
	c.ScopeNotes = cloneText(n.ScopeNotes)
	c.AltLabels = cloneList(n.AltLabels)
	c.HiddenLabels = cloneList(n.HiddenLabels)
	c.Matches = append([]Match(nil), n.Matches...)
	return &c
}

// Label returns the preferred label in lang, falling back to English.
func (n *Node) Label(lang string) string {
	if s := strings.TrimSpace(n.PrefLabels[lang]); s != "" {
		return s
	}
	return n.PrefLabels["en"]
}

func cloneText(m LangText) LangText {
	if m == nil {
		return nil
	}
	out := make(LangText, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneList(m LangList) LangList {
	if m == nil {
		return nil
	}
	out := make(LangList, len(m))
	for k, v := range m {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// Tree is the set of non-retired nodes of one tree.
type Tree struct {
	ID        string
	Namespace string
	Nodes     map[string]*Node

	children map[string][]string // parent ID -> child IDs; nil when stale
}

// NewTree returns an empty tree.
func NewTree(id, namespace string) *Tree {
	return &Tree{ID: id, Namespace: namespace, Nodes: map[string]*Node{}}
}

// Clone returns a deep copy, so a change set can be tried without touching the original.
func (t *Tree) Clone() *Tree {
	c := NewTree(t.ID, t.Namespace)
	for id, n := range t.Nodes {
		c.Nodes[id] = n.clone()
	}
	return c
}

// put inserts or replaces a node.
func (t *Tree) put(n *Node) {
	t.Nodes[n.ID] = n
	t.children = nil
}

func (t *Tree) remove(id string) {
	delete(t.Nodes, id)
	t.children = nil
}

func (t *Tree) index() {
	if t.children != nil {
		return
	}
	t.children = map[string][]string{}
	for id, n := range t.Nodes {
		if n.ParentID != "" {
			t.children[n.ParentID] = append(t.children[n.ParentID], id)
		}
	}
	for p, ids := range t.children {
		sort.Slice(ids, func(i, j int) bool {
			return t.Nodes[ids[i]].CanonicalName < t.Nodes[ids[j]].CanonicalName
		})
		t.children[p] = ids
	}
}

// Roots returns every node without a parent; a valid tree has exactly one.
func (t *Tree) Roots() []*Node {
	var out []*Node
	for _, n := range t.Nodes {
		if n.ParentID == "" {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CanonicalName < out[j].CanonicalName })
	return out
}

// Root returns the root, or nil when the tree does not have exactly one.
func (t *Tree) Root() *Node {
	if roots := t.Roots(); len(roots) == 1 {
		return roots[0]
	}
	return nil
}

// Children returns all children of id, "others" included, ordered by canonical name.
func (t *Tree) Children(id string) []*Node {
	t.index()
	ids := t.children[id]
	out := make([]*Node, 0, len(ids))
	for _, c := range ids {
		out = append(out, t.Nodes[c])
	}
	return out
}

// OrdinaryChildren returns the children of id other than its "others" leaf.
func (t *Tree) OrdinaryChildren(id string) []*Node {
	var out []*Node
	for _, c := range t.Children(id) {
		if !c.IsOthers {
			out = append(out, c)
		}
	}
	return out
}

// OthersChild returns the "others" leaf under id, or nil.
func (t *Tree) OthersChild(id string) *Node {
	for _, c := range t.Children(id) {
		if c.IsOthers {
			return c
		}
	}
	return nil
}

// IsLeaf reports whether id has no children.
func (t *Tree) IsLeaf(id string) bool {
	t.index()
	return len(t.children[id]) == 0
}

// Subtree returns id and all its descendants.
func (t *Tree) Subtree(id string) []string {
	out := []string{id}
	for i := 0; i < len(out); i++ {
		for _, c := range t.Children(out[i]) {
			out = append(out, c.ID)
		}
	}
	return out
}

// Path returns the nodes from the root down to id. It stops early on a broken parent link.
func (t *Tree) Path(id string) []*Node {
	var rev []*Node
	seen := map[string]bool{}
	for cur := t.Nodes[id]; cur != nil && !seen[cur.ID]; cur = t.Nodes[cur.ParentID] {
		seen[cur.ID] = true
		rev = append(rev, cur)
		if cur.ParentID == "" {
			break
		}
	}
	out := make([]*Node, len(rev))
	for i, n := range rev {
		out[len(rev)-1-i] = n
	}
	return out
}

// ByName returns the node with the canonical name, or nil.
func (t *Tree) ByName(name string) *Node {
	for _, n := range t.Nodes {
		if n.CanonicalName == name {
			return n
		}
	}
	return nil
}

// Resolve finds a node by ID or, failing that, by canonical name.
func (t *Tree) Resolve(ref string) *Node {
	ref = strings.TrimSpace(ref)
	if n := t.Nodes[ref]; n != nil {
		return n
	}
	return t.ByName(ref)
}

// IRI returns the node's concept IRI: the tree namespace followed by the opaque ID.
func (t *Tree) IRI(id string) string { return t.Namespace + id }
