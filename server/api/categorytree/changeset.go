package categorytree

import (
	"fmt"
	"strings"
)

// Operations of a change set (devdoc §4.1).
const (
	OpAdd    = "add"
	OpDelete = "delete"
	OpMove   = "move"
	OpUpdate = "update"
)

// systemAuthor is recorded on the changes the engine adds itself ("others" leaves).
const systemAuthor = "system"

// NodeFields are the attributes an add sets or an update changes. In an update a nil field is
// left unchanged and a non-nil one replaces the whole value.
type NodeFields struct {
	CanonicalName *string  `json:"canonical_name,omitempty"`
	PrefLabels    LangText `json:"pref_labels,omitempty"`
	AltLabels     LangList `json:"alt_labels,omitempty"`
	HiddenLabels  LangList `json:"hidden_labels,omitempty"`
	Definitions   LangText `json:"definitions,omitempty"`
	ScopeNotes    LangText `json:"scope_notes,omitempty"`
	Source        *string  `json:"source,omitempty"`
	Matches       *[]Match `json:"matches,omitempty"`
}

// Op is one operation. Node and Parent take a node ID or a canonical name, so a change set
// can refer to nodes it adds itself.
type Op struct {
	Op     string     `json:"op"`
	Node   string     `json:"node,omitempty"`   // delete, move, update
	Parent string     `json:"parent,omitempty"` // add, move; empty on add = the root
	Fields NodeFields `json:"fields"`           // add, update
	Reason string     `json:"reason,omitempty"`
}

// Entry is one log entry: one operation on one node.
type Entry struct {
	Op     string `json:"op"`
	NodeID string `json:"node_id"`
	Before *Node  `json:"before,omitempty"`
	After  *Node  `json:"after,omitempty"`
	Author string `json:"author"`
	Reason string `json:"reason,omitempty"`
}

// ApplyResult is the outcome of applying a change set to a tree.
type ApplyResult struct {
	Tree    *Tree
	Entries []Entry
	// Stale maps an entry index to the nodes whose bindings that entry makes stale (§4.3).
	Stale map[int][]string
}

// Apply applies ops to a copy of before, adds the "others" changes they require, validates
// the result and works out which bindings go stale. before is not modified. newID mints
// node IDs.
func Apply(before *Tree, ops []Op, author string, newID func() string) (*ApplyResult, error) {
	if len(ops) == 0 {
		return nil, fmt.Errorf("change set has no operations")
	}
	t := before.Clone()
	var entries []Entry
	for i, op := range ops {
		es, err := applyOp(t, op, author, newID)
		if err != nil {
			return nil, fmt.Errorf("operation %d (%s): %w", i+1, op.Op, err)
		}
		entries = append(entries, es...)
	}
	entries = append(entries, reconcileOthers(t, newID)...)
	if err := Validate(t); err != nil {
		return nil, err
	}
	return &ApplyResult{Tree: t, Entries: entries, Stale: staleTargets(before, t, entries)}, nil
}

func applyOp(t *Tree, op Op, author string, newID func() string) ([]Entry, error) {
	switch op.Op {
	case OpAdd:
		var parentID string
		if strings.TrimSpace(op.Parent) != "" {
			p := t.Resolve(op.Parent)
			if p == nil {
				return nil, fmt.Errorf("parent %q not found", op.Parent)
			}
			if p.IsOthers {
				return nil, fmt.Errorf("parent %s is an \"others\" leaf", p.CanonicalName)
			}
			parentID = p.ID
		} else if len(t.Nodes) > 0 {
			return nil, fmt.Errorf("parent is required: the tree already has a root")
		}
		if op.Fields.CanonicalName == nil {
			return nil, fmt.Errorf("canonical_name is required")
		}
		n := &Node{ID: newID(), TreeID: t.ID, ParentID: parentID}
		setFields(n, op.Fields)
		if t.ByName(n.CanonicalName) != nil {
			return nil, fmt.Errorf("canonical name %s already exists", n.CanonicalName)
		}
		t.put(n)
		return []Entry{{Op: OpAdd, NodeID: n.ID, After: n.clone(), Author: author, Reason: op.Reason}}, nil

	case OpDelete:
		n, err := ordinaryTarget(t, op.Node)
		if err != nil {
			return nil, err
		}
		if n.ParentID == "" {
			return nil, fmt.Errorf("the root cannot be deleted")
		}
		sub := t.Subtree(n.ID)
		entries := make([]Entry, 0, len(sub))
		for i := len(sub) - 1; i >= 0; i-- { // descendants first
			d := t.Nodes[sub[i]]
			entries = append(entries, Entry{Op: OpDelete, NodeID: d.ID, Before: d.clone(), Author: author, Reason: op.Reason})
		}
		for _, id := range sub {
			t.remove(id)
		}
		return entries, nil

	case OpMove:
		n, err := ordinaryTarget(t, op.Node)
		if err != nil {
			return nil, err
		}
		if n.ParentID == "" {
			return nil, fmt.Errorf("the root cannot be moved")
		}
		p := t.Resolve(op.Parent)
		if p == nil {
			return nil, fmt.Errorf("parent %q not found", op.Parent)
		}
		if p.IsOthers {
			return nil, fmt.Errorf("parent %s is an \"others\" leaf", p.CanonicalName)
		}
		for _, id := range t.Subtree(n.ID) {
			if id == p.ID {
				return nil, fmt.Errorf("%s cannot be moved under itself or its descendant %s", n.CanonicalName, p.CanonicalName)
			}
		}
		if p.ID == n.ParentID {
			return nil, nil
		}
		before := n.clone()
		n.ParentID = p.ID
		t.put(n)
		return []Entry{{Op: OpMove, NodeID: n.ID, Before: before, After: n.clone(), Author: author, Reason: op.Reason}}, nil

	case OpUpdate:
		n, err := ordinaryTarget(t, op.Node)
		if err != nil {
			return nil, err
		}
		before := n.clone()
		setFields(n, op.Fields)
		if other := t.ByName(n.CanonicalName); other != nil && other.ID != n.ID {
			return nil, fmt.Errorf("canonical name %s already exists", n.CanonicalName)
		}
		t.put(n)
		return []Entry{{Op: OpUpdate, NodeID: n.ID, Before: before, After: n.clone(), Author: author, Reason: op.Reason}}, nil
	}
	return nil, fmt.Errorf("unknown operation %q", op.Op)
}

// ordinaryTarget resolves the node an operation acts on; "others" leaves are engine-managed.
func ordinaryTarget(t *Tree, ref string) (*Node, error) {
	n := t.Resolve(ref)
	if n == nil {
		return nil, fmt.Errorf("node %q not found", ref)
	}
	if n.IsOthers {
		return nil, fmt.Errorf("%s is an \"others\" leaf and is maintained automatically", n.CanonicalName)
	}
	return n, nil
}

func setFields(n *Node, f NodeFields) {
	if f.CanonicalName != nil {
		n.CanonicalName = strings.TrimSpace(*f.CanonicalName)
	}
	if f.PrefLabels != nil {
		n.PrefLabels = cloneText(f.PrefLabels)
	}
	if f.AltLabels != nil {
		n.AltLabels = cloneList(f.AltLabels)
	}
	if f.HiddenLabels != nil {
		n.HiddenLabels = cloneList(f.HiddenLabels)
	}
	if f.Definitions != nil {
		n.Definitions = cloneText(f.Definitions)
	}
	if f.ScopeNotes != nil {
		n.ScopeNotes = cloneText(f.ScopeNotes)
	}
	if f.Source != nil {
		n.Source = strings.TrimSpace(*f.Source)
	}
	if f.Matches != nil {
		n.Matches = append([]Match(nil), (*f.Matches)...)
	}
}

// OthersName is the canonical name of the "others" leaf under a parent.
func OthersName(parent string) string { return parent + "-others" }

// othersAttributes builds the attributes of the "others" leaf of p (devdoc §3.1).
func othersAttributes(p *Node) (name string, labels, defs LangText) {
	en, zh := p.Label("en"), p.Label("zh")
	return OthersName(p.CanonicalName),
		LangText{"en": "Others", "zh": "其他"},
		LangText{
			"en": fmt.Sprintf("Belongs under %s and matches none of %s's other children.", en, en),
			"zh": fmt.Sprintf("属于“%s”，但不属于“%s”的任何其他子类。", zh, zh),
		}
}

// reconcileOthers gives every inner node exactly one up-to-date "others" leaf and removes
// the "others" leaf of a node that has no ordinary children left.
func reconcileOthers(t *Tree, newID func() string) []Entry {
	var entries []Entry
	for _, n := range sortedNodes(t) {
		if n.IsOthers {
			continue
		}
		hasOrdinary := len(t.OrdinaryChildren(n.ID)) > 0
		others := t.OthersChild(n.ID)
		name, labels, defs := othersAttributes(n)
		switch {
		case hasOrdinary && others == nil:
			o := &Node{ID: newID(), TreeID: t.ID, ParentID: n.ID, CanonicalName: name, IsOthers: true,
				PrefLabels: labels, Definitions: defs}
			t.put(o)
			entries = append(entries, Entry{Op: OpAdd, NodeID: o.ID, After: o.clone(), Author: systemAuthor,
				Reason: fmt.Sprintf("%s became an inner node", n.CanonicalName)})
		case !hasOrdinary && others != nil:
			entries = append(entries, Entry{Op: OpDelete, NodeID: others.ID, Before: others.clone(), Author: systemAuthor,
				Reason: fmt.Sprintf("%s has no ordinary children left", n.CanonicalName)})
			t.remove(others.ID)
		case others != nil && (others.CanonicalName != name || !sameText(others.Definitions, defs) || !sameText(others.PrefLabels, labels)):
			before := others.clone()
			others.CanonicalName, others.PrefLabels, others.Definitions = name, labels, defs
			t.put(others)
			entries = append(entries, Entry{Op: OpUpdate, NodeID: others.ID, Before: before, After: others.clone(), Author: systemAuthor,
				Reason: fmt.Sprintf("follows changes to %s", n.CanonicalName)})
		}
	}
	return entries
}

func sortedNodes(t *Tree) []*Node {
	var out []*Node
	if root := t.Root(); root != nil {
		for _, id := range t.Subtree(root.ID) {
			out = append(out, t.Nodes[id])
		}
		return out
	}
	for _, n := range t.Nodes { // invalid tree; validation will report it
		out = append(out, n)
	}
	return out
}

func sameText(a, b LangText) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// staleTargets applies the devdoc §4.3 table. Each node is attributed to the first entry
// that makes its bindings stale.
func staleTargets(before, after *Tree, entries []Entry) map[int][]string {
	out := map[int][]string{}
	seen := map[string]bool{}
	mark := func(i int, ids ...string) {
		for _, id := range ids {
			if id != "" && !seen[id] {
				seen[id] = true
				out[i] = append(out[i], id)
			}
		}
	}
	for i, e := range entries {
		switch e.Op {
		case OpDelete:
			mark(i, e.NodeID)
		case OpUpdate:
			if e.After.IsOthers {
				continue // follows its parent's labels; its meaning is unchanged
			}
			if !sameText(e.Before.Definitions, e.After.Definitions) || !sameText(e.Before.ScopeNotes, e.After.ScopeNotes) {
				mark(i, after.Subtree(e.NodeID)...)
			}
		case OpAdd, OpMove:
			if e.After.IsOthers {
				continue
			}
			p := before.Nodes[e.After.ParentID]
			if p == nil {
				continue // the parent is new in this change set: it has no bindings
			}
			if before.IsLeaf(p.ID) {
				mark(i, p.ID) // the leaf becomes an inner node
			} else if o := before.OthersChild(p.ID); o != nil {
				mark(i, o.ID) // members of "others" may now match the new child
			}
		}
	}
	return out
}

// Replay rebuilds a tree from log entries in order, as when viewing a revision.
func Replay(id, namespace string, entries []Entry) (*Tree, error) {
	t := NewTree(id, namespace)
	for i, e := range entries {
		switch e.Op {
		case OpAdd, OpMove, OpUpdate:
			if e.After == nil {
				return nil, fmt.Errorf("entry %d (%s %s) has no after value", i+1, e.Op, e.NodeID)
			}
			t.put(e.After.clone())
		case OpDelete:
			t.remove(e.NodeID)
		default:
			return nil, fmt.Errorf("entry %d has unknown operation %q", i+1, e.Op)
		}
	}
	return t, nil
}
