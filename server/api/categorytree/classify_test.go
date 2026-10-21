package categorytree

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// fakeDecider answers from a canonical-name -> probability table (0 when absent). failParents
// make every request for that parent fail.
type fakeDecider struct {
	t           *Tree
	p           map[string]float64
	failParents map[string]bool
	extra       map[string]float64 // answers for IDs that were not supplied
	mu          sync.Mutex
	calls       int
	batchSizes  []int
}

func (f *fakeDecider) Decide(_ context.Context, req DecisionRequest) (map[string]float64, error) {
	f.mu.Lock()
	f.calls++
	f.batchSizes = append(f.batchSizes, len(req.Children))
	f.mu.Unlock()
	if f.failParents[req.Parent.CanonicalName] {
		return nil, errors.New("boom")
	}
	out := map[string]float64{}
	for _, c := range req.Children {
		out[c.ID] = f.p[c.CanonicalName]
	}
	for k, v := range f.extra {
		out[k] = v
	}
	return out, nil
}

func names(t *Tree, ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = t.Nodes[id].CanonicalName
	}
	return out
}

func TestWalk_MultipleBranches(t *testing.T) {
	tree, _ := physTree(t)
	d := &fakeDecider{t: tree, p: map[string]float64{"physical": 0.9, "chemical": 0.95, "temperature": 0.8}}
	res := Walk(context.Background(), tree, Artifact{}, d, Options{})
	if res.Status != StatusPublished {
		t.Fatalf("status = %s (%s)", res.Status, res.Reason)
	}
	if got := fmt.Sprint(names(tree, res.Leaves)); got != "[chemical temperature]" {
		t.Errorf("leaves = %s", got)
	}
}

func TestWalk_NothingFitsGoesToRootOthers(t *testing.T) {
	tree, _ := physTree(t)
	res := Walk(context.Background(), tree, Artifact{}, &fakeDecider{t: tree}, Options{})
	if res.Status != StatusPublished || fmt.Sprint(names(tree, res.Leaves)) != "[root-others]" {
		t.Errorf("got %s %v", res.Status, names(tree, res.Leaves))
	}
}

func TestWalk_NoChildMatchesGoesToParentOthers(t *testing.T) {
	tree, _ := physTree(t)
	res := Walk(context.Background(), tree, Artifact{}, &fakeDecider{t: tree, p: map[string]float64{"physical": 1}}, Options{})
	if fmt.Sprint(names(tree, res.Leaves)) != "[physical-others]" {
		t.Errorf("leaves = %v", names(tree, res.Leaves))
	}
}

// wideTree: root with 25 leaf children c00..c24.
func wideTree(t *testing.T) *Tree {
	ops := []Op{addOp("root", "")}
	for i := 0; i < 25; i++ {
		ops = append(ops, addOp(fmt.Sprintf("c%02d", i), "root"))
	}
	res, err := Apply(NewTree("w", "ns/"), ops, "x", seqIDs())
	if err != nil {
		t.Fatal(err)
	}
	return res.Tree
}

func TestWalk_MatchInOneBatchOnly(t *testing.T) {
	tree := wideTree(t)
	d := &fakeDecider{t: tree, p: map[string]float64{"c22": 0.9}}
	res := Walk(context.Background(), tree, Artifact{}, d, Options{BatchSize: 20})
	if res.Status != StatusPublished || fmt.Sprint(names(tree, res.Leaves)) != "[c22]" {
		t.Errorf("got %s %v; root-others must not be added", res.Status, names(tree, res.Leaves))
	}
	if d.calls != 2 {
		t.Errorf("calls = %d, want 2 batches", d.calls)
	}
}

func TestWalk_FailedBatchIsNotANonMatch(t *testing.T) {
	tree := wideTree(t)
	calls := 0
	d := deciderFunc(func(_ context.Context, req DecisionRequest) (map[string]float64, error) {
		calls++
		if len(req.Children) == 5 { // the second batch always fails
			return nil, errors.New("timeout")
		}
		return map[string]float64{}, nil
	})
	res := Walk(context.Background(), tree, Artifact{}, d, Options{BatchSize: 20, Concurrency: 1})
	if res.Status != StatusFailed || res.Reason != ReasonRequestFailed || len(res.Leaves) != 0 {
		t.Errorf("got %s/%s leaves %v", res.Status, res.Reason, res.Leaves)
	}
	if calls != 1+3 {
		t.Errorf("calls = %d, want 1 + 3 attempts", calls)
	}
}

type deciderFunc func(context.Context, DecisionRequest) (map[string]float64, error)

func (f deciderFunc) Decide(ctx context.Context, r DecisionRequest) (map[string]float64, error) {
	return f(ctx, r)
}

func TestWalk_UncertainGoesToReview(t *testing.T) {
	tree, _ := physTree(t)
	d := &fakeDecider{t: tree, p: map[string]float64{"physical": 0.5}}
	res := Walk(context.Background(), tree, Artifact{}, d, Options{})
	if res.Status != StatusInReview || res.Reason != ReasonUncertain || len(res.Leaves) != 0 {
		t.Errorf("got %s/%s %v", res.Status, res.Reason, res.Leaves)
	}
}

func TestWalk_BudgetExhausted(t *testing.T) {
	tree, _ := physTree(t)
	d := &fakeDecider{t: tree, p: map[string]float64{"physical": 0.9}}
	res := Walk(context.Background(), tree, Artifact{}, d, Options{MaxRequests: 1})
	if res.Status != StatusInReview || res.Reason != ReasonBudgetExhausted || len(res.Leaves) != 0 {
		t.Errorf("got %s/%s %v", res.Status, res.Reason, res.Leaves)
	}
	if res.Requests != 1 {
		t.Errorf("requests = %d", res.Requests)
	}
}

func TestWalk_IgnoresUnsuppliedIDs(t *testing.T) {
	tree, _ := physTree(t)
	d := &fakeDecider{t: tree, extra: map[string]float64{tree.ByName("temperature").ID: 1}}
	res := Walk(context.Background(), tree, Artifact{}, d, Options{})
	if fmt.Sprint(names(tree, res.Leaves)) != "[root-others]" {
		t.Errorf("leaves = %v", names(tree, res.Leaves))
	}
	if len(res.Decisions[0].Ignored) != 1 {
		t.Errorf("ignored = %v", res.Decisions[0].Ignored)
	}
}

func TestWalk_GroupingDoesNotChangeMemberships(t *testing.T) {
	tree := wideTree(t)
	p := map[string]float64{"c03": 0.9, "c19": 0.8, "c24": 0.99}
	want := ""
	for _, r := range []int{1, 7, 20, 25} {
		res := Walk(context.Background(), tree, Artifact{}, &fakeDecider{t: tree, p: p}, Options{BatchSize: r, MaxRequests: 100})
		got := fmt.Sprint(names(tree, res.Leaves))
		if want == "" {
			want = got
		}
		if got != want || res.Status != StatusPublished {
			t.Errorf("R=%d: %s %s, want %s", r, res.Status, got, want)
		}
	}
}

func TestValidateLeaves(t *testing.T) {
	tree, _ := physTree(t)
	if err := ValidateLeaves(tree, []string{tree.ByName("physical").ID}); err == nil {
		t.Error("inner node accepted")
	}
	if err := ValidateLeaves(tree, []string{tree.ByName("root-others").ID, tree.ByName("pressure").ID}); err != nil {
		t.Error(err)
	}
}
