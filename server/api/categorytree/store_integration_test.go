package categorytree

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	_ "github.com/lib/pq"
)

// testDB opens TEST_DATABASE_URL (the chenweb_test database, never miner), skipping when unset.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func initialNode(name string, children ...InitialNode) InitialNode {
	return InitialNode{NodeFields: addOp(name, "").Fields, Children: children}
}

// fakeSource serves fixed artifacts.
type fakeSource struct{ artifacts []Artifact }

func (fakeSource) Type() string { return "test_items" }
func (f *fakeSource) ListRecord(_ context.Context, recordID int64) ([]Artifact, error) {
	var out []Artifact
	for _, a := range f.artifacts {
		if a.RecordID == recordID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeSource) Get(_ context.Context, id string) (*Artifact, error) {
	for _, a := range f.artifacts {
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, ErrNotFound
}

func TestStore_EndToEnd(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	store := &Store{DB: db}
	suffix := fmt.Sprint(time.Now().UnixNano())
	treeName := "test-tree-" + suffix
	artifactType := "test_items_" + suffix

	var recordID int64
	if err := db.QueryRow(`INSERT INTO kb.inputs (type) VALUES ('category-tree-test') RETURNING id`).Scan(&recordID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM kb.category_bindings WHERE artifact_type = $1`, artifactType)
		db.Exec(`DELETE FROM kb.category_classifications WHERE artifact_type = $1`, artifactType)
		db.Exec(`DELETE FROM kb.category_tree_types WHERE artifact_type = $1`, artifactType)
		db.Exec(`DELETE FROM kb.category_tree_revisions WHERE tree_id IN (SELECT id FROM kb.category_trees WHERE canonical_name = $1)`, treeName)
		db.Exec(`DELETE FROM kb.category_tree_changes WHERE tree_id IN (SELECT id FROM kb.category_trees WHERE canonical_name = $1)`, treeName)
		db.Exec(`UPDATE kb.category_nodes SET parent_id = NULL WHERE tree_id IN (SELECT id FROM kb.category_trees WHERE canonical_name = $1)`, treeName)
		db.Exec(`DELETE FROM kb.category_nodes WHERE tree_id IN (SELECT id FROM kb.category_trees WHERE canonical_name = $1)`, treeName)
		db.Exec(`DELETE FROM kb.category_trees WHERE canonical_name = $1`, treeName)
		db.Exec(`DELETE FROM kb.inputs WHERE id = $1`, recordID)
	})

	// Create: root > physical > {temperature, pressure}, root > chemical.
	info, cs, err := store.CreateTree(ctx, CreateTreeInput{
		CanonicalName: treeName, Namespace: "https://example.org/cat/test/",
		PrefLabels:  LangText{"en": "Test", "zh": "测试"},
		Definitions: LangText{"en": "Test tree", "zh": "测试树"},
		Root: initialNode("root",
			initialNode("physical", initialNode("temperature"), initialNode("pressure")),
			initialNode("chemical")),
	}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if cs.ToSeq != 7 || info.CurrentSeq != 7 {
		t.Fatalf("initial seq = %d, want 7 (5 nodes + 2 others)", cs.ToSeq)
	}
	if _, _, err := store.CreateTree(ctx, CreateTreeInput{CanonicalName: treeName, Namespace: "x",
		PrefLabels: LangText{"en": "a", "zh": "b"}, Definitions: LangText{"en": "a", "zh": "b"}, Root: initialNode("root", initialNode("a"))}, "alice"); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate tree: err = %v", err)
	}
	if _, err := store.CreateRevision(ctx, info, "v1.0.0", "initial", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRevision(ctx, info, "v1.0.0", "", "alice"); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate revision: err = %v", err)
	}
	if err := store.SetTreeType(ctx, artifactType, info, "alice"); err != nil {
		t.Fatal(err)
	}

	// Classify two artifacts with a fake decider.
	src := &fakeSource{artifacts: []Artifact{
		{Type: artifactType, ID: "a1", RecordID: recordID, Lang: "en", Proposals: []string{"Physical > Temp"}},
		{Type: artifactType, ID: "a2", RecordID: recordID, Lang: "en"},
	}}
	tree, _ := store.LoadCurrent(ctx, info)
	byArtifact := map[string]map[string]float64{
		"a1": {"physical": 0.9, "chemical": 0.8, "temperature": 0.95},
		"a2": {},
	}
	dec := deciderFunc(func(_ context.Context, req DecisionRequest) (map[string]float64, error) {
		out := map[string]float64{}
		for _, c := range req.Children {
			out[c.ID] = byArtifact[req.Artifact.ID][c.CanonicalName]
		}
		return out, nil
	})
	svc := &Service{Store: store, Sources: map[string]ArtifactSource{artifactType: src},
		Model:  func() (*Model, error) { return &Model{Decider: dec, ModelName: "fake", Profile: "fake"}, nil },
		Logger: loggerutil.CreateDefaultLogger("20261021-417")}
	for _, a := range src.artifacts {
		if st := svc.ClassifyOne(ctx, info, tree, &Model{Decider: dec, ModelName: "fake"}, a); st != StatusPublished {
			t.Fatalf("%s: status %s", a.ID, st)
		}
	}
	assoc, err := store.ArtifactBindings(ctx, artifactType, "a1")
	if err != nil {
		t.Fatal(err)
	}
	if len(assoc.Direct) != 2 || len(assoc.Ancestors) != 2 {
		t.Errorf("a1 direct=%d ancestors=%d, want 2 and 2 (root, physical)", len(assoc.Direct), len(assoc.Ancestors))
	}
	if got := strings.Join(assoc.Direct[1].Path, "/"); got != "root/physical/temperature" {
		t.Errorf("path = %s", got)
	}
	physical := tree.ByName("physical")
	members, err := store.Members(ctx, physical.ID, false, 0)
	if err != nil || len(members) != 1 || members[0].ArtifactID != "a1" {
		t.Errorf("members of physical = %+v, %v", members, err)
	}
	report, _ := store.OthersReport(ctx, info.ID)
	if len(report) != 2 || report[0].Name != "root-others" || report[0].Members != 1 {
		t.Errorf("others report = %+v", report)
	}

	// A definition change on physical stales a1's temperature binding, not chemical.
	res, err := store.ApplyChangeSet(ctx, treeName, []Op{{Op: OpUpdate, Node: "physical",
		Fields: NodeFields{Definitions: LangText{"en": "changed", "zh": "改了"}}}}, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if res.StaleBindings != 1 {
		t.Errorf("stale bindings = %d, want 1", res.StaleBindings)
	}
	stale, _ := store.StaleArtifacts(ctx, artifactType, 10)
	if len(stale) != 1 || stale[0].ArtifactID != "a1" {
		t.Errorf("stale artifacts = %+v", stale)
	}
	// An invalid change set leaves everything as it was.
	if _, err := store.ApplyChangeSet(ctx, treeName, []Op{addOp("ok", "root"), {Op: OpDelete, Node: "root"}}, "bob"); err == nil {
		t.Error("invalid change set accepted")
	}
	if cur, _ := store.GetTree(ctx, treeName); cur.CurrentSeq != res.ToSeq {
		t.Errorf("seq moved after a rejected change set: %d", cur.CurrentSeq)
	}

	// Rename, then view v1.0.0 by replay.
	if _, err := store.ApplyChangeSet(ctx, treeName, []Op{{Op: OpUpdate, Node: "chemical", Fields: NodeFields{CanonicalName: str("chem")}}}, "bob"); err != nil {
		t.Fatal(err)
	}
	info, _ = store.GetTree(ctx, treeName)
	seq, err := store.RevisionSeq(ctx, info.ID, "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.TreeAt(ctx, info, seq)
	if err != nil {
		t.Fatal(err)
	}
	if old.ByName("chemical") == nil || old.ByName("chem") != nil || Validate(old) != nil {
		t.Errorf("revision v1.0.0 does not show the old tree")
	}
	if cur, _ := store.LoadCurrent(ctx, info); cur.ByName("chem") == nil {
		t.Errorf("current tree lacks the rename")
	}

	// Review: an uncertain attempt can be closed or resolved, never with an inner node.
	byArtifact["a2"] = map[string]float64{"physical": 0.5}
	tree, _ = store.LoadCurrent(ctx, info)
	if st := svc.ClassifyOne(ctx, info, tree, &Model{Decider: dec}, src.artifacts[1]); st != StatusInReview {
		t.Fatalf("a2 status = %s", st)
	}
	queue, _ := store.ListClassifications(ctx, ClassificationFilter{ArtifactType: artifactType, ReviewQueue: true})
	if len(queue) != 1 || queue[0].ArtifactID != "a2" {
		t.Fatalf("review queue = %+v", queue)
	}
	if assoc, _ := store.ArtifactBindings(ctx, artifactType, "a2"); len(assoc.Direct) != 1 {
		t.Errorf("an in-review attempt must keep the published binding")
	}
	if err := store.ResolveClassification(ctx, queue[0].ID, []string{physical.ID}, "carol", ""); err == nil {
		t.Error("resolve with an inner node accepted")
	}
	if err := store.ResolveClassification(ctx, queue[0].ID, []string{tree.ByName("pressure").ID}, "carol", "pressure"); err != nil {
		t.Fatal(err)
	}
	if assoc, _ := store.ArtifactBindings(ctx, artifactType, "a2"); len(assoc.Direct) != 1 || assoc.Direct[0].CanonicalName != "pressure" {
		t.Errorf("resolved bindings = %+v", assoc.Direct)
	}
	if q, _ := store.ListClassifications(ctx, ClassificationFilter{ArtifactType: artifactType, ReviewQueue: true}); len(q) != 0 {
		t.Errorf("queue not empty after resolve")
	}

	// Re-extraction: a1 vanished.
	src.artifacts = src.artifacts[1:]
	n, err := store.RemoveVanished(ctx, artifactType, recordID, []string{"a2"})
	if err != nil || n != 2 {
		t.Errorf("removed = %d, %v; want a1's two bindings", n, err)
	}

	// Deleting a node stales its bindings.
	res, err = store.ApplyChangeSet(ctx, treeName, []Op{{Op: OpDelete, Node: "pressure"}}, "bob")
	if err != nil || res.StaleBindings != 1 {
		t.Errorf("delete: stale = %v, %v", res, err)
	}
}
