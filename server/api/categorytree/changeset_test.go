package categorytree

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func seqIDs() func() string {
	n := 0
	return func() string { n++; return fmt.Sprintf("n%02d", n) }
}

func str(s string) *string { return &s }

func addOp(name, parent string) Op {
	return Op{Op: OpAdd, Parent: parent, Fields: NodeFields{
		CanonicalName: str(name),
		PrefLabels:    LangText{"en": "EN " + name, "zh": "中 " + name},
		Definitions:   LangText{"en": "def " + name, "zh": "定义 " + name},
	}}
}

// physTree: root > physical > {temperature, pressure}, root > chemical.
func physTree(t *testing.T) (*Tree, func() string) {
	t.Helper()
	ids := seqIDs()
	res, err := Apply(NewTree("t1", "https://example.org/cat/"), []Op{
		addOp("root", ""),
		addOp("physical", "root"),
		addOp("temperature", "physical"),
		addOp("pressure", "physical"),
		addOp("chemical", "root"),
	}, "alice", ids)
	if err != nil {
		t.Fatalf("build tree: %v", err)
	}
	return res.Tree, ids
}

func TestApply_InitialTreeGetsOthersLeaves(t *testing.T) {
	tree, _ := physTree(t)
	for _, name := range []string{"root-others", "physical-others"} {
		o := tree.ByName(name)
		if o == nil || !o.IsOthers {
			t.Fatalf("missing others leaf %s", name)
		}
		if o.PrefLabels["zh"] != "其他" || o.Definitions["en"] == "" {
			t.Errorf("%s attributes: %+v", name, o)
		}
	}
	if tree.ByName("chemical-others") != nil || tree.ByName("temperature-others") != nil {
		t.Errorf("leaves must not have others children")
	}
	if len(tree.Nodes) != 7 {
		t.Errorf("nodes = %d, want 7", len(tree.Nodes))
	}
}

func TestApply_LeafGetsFirstChild(t *testing.T) {
	tree, ids := physTree(t)
	chem := tree.ByName("chemical")
	res, err := Apply(tree, []Op{addOp("acidity", "chemical")}, "bob", ids)
	if err != nil {
		t.Fatal(err)
	}
	if o := res.Tree.OthersChild(chem.ID); o == nil || o.CanonicalName != "chemical-others" {
		t.Fatalf("chemical-others not added")
	}
	if len(res.Entries) != 2 || res.Entries[1].Author != systemAuthor {
		t.Errorf("entries = %+v", res.Entries)
	}
	if got := res.Stale[0]; len(got) != 1 || got[0] != chem.ID {
		t.Errorf("stale = %v, want chemical (it was a leaf)", res.Stale)
	}
	if tree.ByName("acidity") != nil {
		t.Errorf("Apply modified its input tree")
	}
}

func TestApply_ChildAddedToInnerNodeStalesOthers(t *testing.T) {
	tree, ids := physTree(t)
	res, err := Apply(tree, []Op{addOp("density", "physical")}, "bob", ids)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Stale[0]; len(got) != 1 || got[0] != tree.ByName("physical-others").ID {
		t.Errorf("stale = %v, want physical-others", res.Stale)
	}
}

func TestApply_RenameFollowsOthers(t *testing.T) {
	tree, ids := physTree(t)
	res, err := Apply(tree, []Op{{Op: OpUpdate, Node: "physical", Fields: NodeFields{CanonicalName: str("physics")}}}, "bob", ids)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tree.ByName("physics-others") == nil || res.Tree.ByName("physical-others") != nil {
		t.Errorf("others leaf not renamed")
	}
	if len(res.Stale) != 0 {
		t.Errorf("a rename must not stale bindings: %v", res.Stale)
	}
}

func TestApply_DefinitionChangeStalesSubtree(t *testing.T) {
	tree, ids := physTree(t)
	res, err := Apply(tree, []Op{{Op: OpUpdate, Node: "physical",
		Fields: NodeFields{Definitions: LangText{"en": "new", "zh": "新"}}}}, "bob", ids)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Stale[0]; len(got) != 4 { // physical, its two leaves and its others leaf
		t.Errorf("stale = %v, want physical subtree", res.Stale)
	}
}

func TestApply_LabelChangeStalesNothing(t *testing.T) {
	tree, ids := physTree(t)
	res, err := Apply(tree, []Op{{Op: OpUpdate, Node: "temperature",
		Fields: NodeFields{PrefLabels: LangText{"en": "Temp", "zh": "温度"}}}}, "bob", ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Stale) != 0 {
		t.Errorf("stale = %v", res.Stale)
	}
}

func TestApply_DeleteSubtree(t *testing.T) {
	tree, ids := physTree(t)
	res, err := Apply(tree, []Op{{Op: OpDelete, Node: "physical"}}, "bob", ids)
	if err != nil {
		t.Fatal(err)
	}
	deletes := 0
	for _, e := range res.Entries {
		if e.Op == OpDelete {
			deletes++
		}
	}
	if deletes != 4 {
		t.Errorf("deletes = %d, want 4", deletes)
	}
	if res.Tree.ByName("temperature") != nil || len(res.Tree.Nodes) != 3 {
		t.Errorf("subtree not removed: %d nodes", len(res.Tree.Nodes))
	}
}

func TestApply_DeleteLastChildRemovesOthers(t *testing.T) {
	tree, ids := physTree(t)
	res, err := Apply(tree, []Op{{Op: OpDelete, Node: "temperature"}, {Op: OpDelete, Node: "pressure"}}, "bob", ids)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tree.ByName("physical-others") != nil {
		t.Errorf("physical is a leaf again; its others leaf must go")
	}
	if err := Validate(res.Tree); err != nil {
		t.Error(err)
	}
}

func TestApply_Rejections(t *testing.T) {
	tree, ids := physTree(t)
	cases := map[string][]Op{
		"move under descendant": {{Op: OpMove, Node: "physical", Parent: "temperature"}},
		"move root":             {{Op: OpMove, Node: "root", Parent: "physical"}},
		"delete root":           {{Op: OpDelete, Node: "root"}},
		"edit others":           {{Op: OpUpdate, Node: "root-others", Fields: NodeFields{Source: str("x")}}},
		"child of others":       {addOp("x", "root-others")},
		"duplicate name":        {{Op: OpUpdate, Node: "chemical", Fields: NodeFields{CanonicalName: str("physical")}}},
		"second root":           {addOp("x", "")},
		"atomic":                {addOp("ok", "root"), {Op: OpMove, Node: "root", Parent: "ok"}},
	}
	for name, ops := range cases {
		if _, err := Apply(tree, ops, "bob", ids); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if tree.ByName("ok") != nil {
		t.Errorf("failed change set leaked into the tree")
	}
}

func TestValidate_Problems(t *testing.T) {
	tree, ids := physTree(t)
	cases := map[string]struct {
		ops  []Op
		want string
	}{
		"missing zh label": {[]Op{{Op: OpAdd, Parent: "root", Fields: NodeFields{CanonicalName: str("x"),
			PrefLabels: LangText{"en": "X"}, Definitions: LangText{"en": "x", "zh": "x"}}}}, "missing preferred label in zh"},
		"missing definition": {[]Op{{Op: OpUpdate, Node: "chemical", Fields: NodeFields{Definitions: LangText{"en": "x"}}}},
			"missing definition in zh"},
		"duplicate pref label": {[]Op{{Op: OpUpdate, Node: "chemical",
			Fields: NodeFields{PrefLabels: LangText{"en": "en physical", "zh": "化学"}}}}, "preferred label"},
		"alt equals own pref": {[]Op{{Op: OpUpdate, Node: "chemical",
			Fields: NodeFields{AltLabels: LangList{"en": {"EN chemical"}}}}}, "both its preferred"},
		"alt shared": {[]Op{
			{Op: OpUpdate, Node: "chemical", Fields: NodeFields{AltLabels: LangList{"en": {"chem"}}}},
			{Op: OpUpdate, Node: "pressure", Fields: NodeFields{AltLabels: LangList{"en": {"Chem"}}}}}, "alternative label"},
		"alt is other pref": {[]Op{{Op: OpUpdate, Node: "chemical",
			Fields: NodeFields{AltLabels: LangList{"en": {"EN pressure"}}}}}, "is the preferred label of"},
		"bad slug": {[]Op{{Op: OpUpdate, Node: "chemical", Fields: NodeFields{CanonicalName: str("Chemical X")}}}, "slug"},
		"exact match same tree": {[]Op{{Op: OpUpdate, Node: "chemical",
			Fields: NodeFields{Matches: &[]Match{{Type: "exact", IRI: tree.IRI(tree.ByName("physical").ID)}}}}}, "duplicate"},
	}
	for name, c := range cases {
		_, err := Apply(tree, c.ops, "bob", ids)
		var ve *ValidationError
		if !errors.As(err, &ve) || !strings.Contains(ve.Error(), c.want) {
			t.Errorf("%s: err = %v, want validation problem containing %q", name, err, c.want)
		}
	}
}

func TestReplay_MatchesAppliedTree(t *testing.T) {
	ids := seqIDs()
	var log []Entry
	tree := NewTree("t1", "ns/")
	steps := [][]Op{
		{addOp("root", ""), addOp("a", "root"), addOp("b", "root")},
		{addOp("a1", "a"), {Op: OpUpdate, Node: "b", Fields: NodeFields{CanonicalName: str("bee")}}},
		{{Op: OpMove, Node: "a1", Parent: "bee"}},
		{{Op: OpDelete, Node: "a"}},
	}
	var snapshots []int
	for _, ops := range steps {
		res, err := Apply(tree, ops, "x", ids)
		if err != nil {
			t.Fatal(err)
		}
		tree = res.Tree
		log = append(log, res.Entries...)
		snapshots = append(snapshots, len(log))
	}
	got, err := Replay("t1", "ns/", log)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != len(tree.Nodes) {
		t.Fatalf("replayed %d nodes, want %d", len(got.Nodes), len(tree.Nodes))
	}
	for id, n := range tree.Nodes {
		if r := got.Nodes[id]; r == nil || r.CanonicalName != n.CanonicalName || r.ParentID != n.ParentID {
			t.Errorf("node %s differs: %+v vs %+v", id, r, n)
		}
	}
	for _, at := range snapshots { // every change-set boundary is a valid tree
		r, err := Replay("t1", "ns/", log[:at])
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(r); err != nil {
			t.Errorf("tree at seq %d invalid: %v", at, err)
		}
	}
	early, _ := Replay("t1", "ns/", log[:snapshots[0]])
	if early.ByName("b") == nil || early.ByName("bee") != nil {
		t.Errorf("replay to the first change set must show the old name")
	}
}
