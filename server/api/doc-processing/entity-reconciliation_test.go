package docprocessing

import (
	"context"
	"math"
	"testing"
	"time"
)

// ---- pure scoring ----

func TestScoreCandidate(t *testing.T) {
	cases := []struct {
		name string
		sig  map[string]any
		want float64
	}{
		{
			name: "exact name + full overlap + type match -> max",
			sig:  map[string]any{"name_exact": true, "surface_jaccard": 1.0, "type_match": true},
			want: 1.0,
		},
		{
			name: "exact name but type conflict is capped at 0.6",
			sig:  map[string]any{"name_exact": true, "surface_jaccard": 1.0, "type_match": false},
			want: 0.6,
		},
		{
			name: "exact name, no other forms, type match",
			sig:  map[string]any{"name_exact": true, "surface_jaccard": 0.0, "type_match": true},
			want: 0.8,
		},
		{
			name: "no name match, partial overlap, type match -> low",
			sig:  map[string]any{"name_exact": false, "surface_jaccard": 0.5, "type_match": true},
			want: 0.2,
		},
		{
			name: "nothing in common -> zero",
			sig:  map[string]any{"name_exact": false, "surface_jaccard": 0.0, "type_match": false},
			want: 0.0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scoreCandidate(c.sig); !approx(got, c.want) {
				t.Errorf("scoreCandidate = %v, want %v", got, c.want)
			}
		})
	}
}

func TestComputeSignals(t *testing.T) {
	a := map[string]any{
		"entity":            "PostgreSQL",
		"aliases":           []string{"postgres"},
		"entity_type_en":    "database",
		"entity_categories": []string{"datastore", "db"},
	}
	b := map[string]any{
		"entity":            "Postgres",
		"aliases":           []string{"PostgreSQL"},
		"entity_type_en":    "Database", // case-folded match
		"entity_categories": []string{"db"},
	}
	sig := computeSignals(a, b)

	if ok, _ := sig["name_exact"].(bool); !ok {
		t.Errorf("name_exact = %v, want true", sig["name_exact"])
	}
	if j, _ := sig["surface_jaccard"].(float64); !approx(j, 1.0) {
		t.Errorf("surface_jaccard = %v, want 1.0", j)
	}
	if ok, _ := sig["type_match"].(bool); !ok {
		t.Errorf("type_match = %v, want true (case folded)", sig["type_match"])
	}
	if c, _ := sig["category_overlap"].(float64); !approx(c, 0.5) {
		t.Errorf("category_overlap = %v, want 0.5", c)
	}
}

func TestComputeSignalsTypeConflict(t *testing.T) {
	// Same name, different type: scoreCandidate must not auto-merge them.
	a := map[string]any{"entity": "Mercury", "entity_type_en": "planet"}
	b := map[string]any{"entity": "Mercury", "entity_type_en": "element"}
	if got := scoreCandidate(computeSignals(a, b)); got > 0.6 {
		t.Errorf("same-name/different-type score = %v, must be capped <= 0.6", got)
	}
}

func TestSurvivorScoreExtractedBeatsProvisional(t *testing.T) {
	extracted := map[string]any{"entity": "Acme", "entity_status": entityStatusExtracted}
	provisional := map[string]any{
		"entity": "Acme Corporation", "entity_en": "Acme Corp",
		"aliases": []string{"Acme Inc", "ACME"}, "entity_status": entityStatusProvisional,
	}
	if survivorScore(extracted) <= survivorScore(provisional) {
		t.Errorf("extracted (%d) must outrank richer provisional (%d)",
			survivorScore(extracted), survivorScore(provisional))
	}
	if survivorScore(nil) != -1 {
		t.Errorf("nil entity survivorScore = %d, want -1", survivorScore(nil))
	}
}

func TestElectSurvivorPrefersExtractedHead(t *testing.T) {
	store := newFakeReconcileStore()
	store.entities["prov"] = map[string]any{"entity": "Acme", "entity_status": entityStatusProvisional}
	store.entities["ext"] = map[string]any{"entity": "Acme", "entity_status": entityStatusExtracted}

	lo, hi := sortPair("prov", "ext")
	from, into := electSurvivor(context.Background(), store, CandidatePair{LoEntityID: lo, HiEntityID: hi})
	if into != "ext" || from != "prov" {
		t.Errorf("electSurvivor = (from=%s, into=%s), want (prov, ext)", from, into)
	}
}

func TestJaccardAndHelpers(t *testing.T) {
	if got := jaccard(nil, nil); got != 0 {
		t.Errorf("jaccard(nil,nil) = %v, want 0", got)
	}
	if got := jaccard([]string{"a", "b"}, []string{"b", "c"}); !approx(got, 1.0/3.0) {
		t.Errorf("jaccard = %v, want 1/3", got)
	}
	if got := jaccard([]string{"a"}, []string{"a"}); !approx(got, 1.0) {
		t.Errorf("jaccard identical = %v, want 1", got)
	}
	if !hasCommon([]string{"a", "b"}, []string{"c", "b"}) {
		t.Errorf("hasCommon should find shared element")
	}
	if hasCommon([]string{"a"}, []string{"b"}) {
		t.Errorf("hasCommon should be false for disjoint sets")
	}
	if lo, hi := sortPair("b", "a"); lo != "a" || hi != "b" {
		t.Errorf("sortPair(b,a) = (%s,%s), want (a,b)", lo, hi)
	}
}

// ---- orchestration: block -> adjudicate -> apply ----

func TestReconcilerRunRuleTailsAndLLMMiddle(t *testing.T) {
	store := newFakeReconcileStore()
	// auto-merge pair: e2 is the extracted head, e1 a provisional absorbed.
	store.entities["e1"] = map[string]any{"entity": "Acme", "entity_status": entityStatusProvisional}
	store.entities["e2"] = map[string]any{"entity": "Acme Corporation", "entity_status": entityStatusExtracted}
	// middle-band pair handled by the LLM.
	store.entities["e5"] = map[string]any{"entity": "Globex", "entity_status": entityStatusExtracted}
	store.entities["e6"] = map[string]any{"entity": "Globex Inc", "entity_status": entityStatusExtracted}

	store.candidates = []CandidatePair{
		{CandidateID: 101, LoEntityID: "e1", HiEntityID: "e2", Score: 0.9}, // auto_merge
		{CandidateID: 102, LoEntityID: "e3", HiEntityID: "e4", Score: 0.1}, // auto_reject
		{CandidateID: 103, LoEntityID: "e5", HiEntityID: "e6", Score: 0.5}, // -> LLM
	}

	llm := &fakeAdjudicator{
		result: AdjudicationResult{
			Merges: []MergeDecision{{
				CandidateID: 103, FromEntityID: "e6", IntoEntityID: "e5", Method: mergeMethodLLM, Confidence: 0.7,
			}},
		},
	}

	r := &Reconciler{
		store: store,
		llm:   llm,
		cfg:   ReconcileConfig{AutoMergeMin: 0.8, AutoRejectMax: 0.2, MaxLLMGroupsPerRun: 10},
	}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stats.Candidates != 3 {
		t.Errorf("Candidates = %d, want 3", stats.Candidates)
	}
	if stats.AutoMerged != 2 {
		t.Errorf("AutoMerged = %d, want 2 (rule + llm)", stats.AutoMerged)
	}
	if len(store.merges) != 2 {
		t.Fatalf("ApplyMerge called %d times, want 2", len(store.merges))
	}
	// Rule merge survivor election: extracted e2 wins over provisional e1.
	if store.merges[0].FromEntityID != "e1" || store.merges[0].IntoEntityID != "e2" {
		t.Errorf("rule merge = %s->%s, want e1->e2", store.merges[0].FromEntityID, store.merges[0].IntoEntityID)
	}
	if store.decisions[102] != decisionAutoReject {
		t.Errorf("pair 102 decision = %q, want %q", store.decisions[102], decisionAutoReject)
	}
	if store.decisions[101] != decisionApplied || store.decisions[103] != decisionApplied {
		t.Errorf("applied decisions missing: %+v", store.decisions)
	}
	if !store.finished || !store.finishOK {
		t.Errorf("FinishRun(ok=true) not called: finished=%v ok=%v", store.finished, store.finishOK)
	}
	if store.finishWatermark.IsZero() {
		t.Errorf("FinishRun watermarkTo should be set")
	}
}

func TestReconcilerRunQueuesNeedsHuman(t *testing.T) {
	store := newFakeReconcileStore()
	store.entities["a"] = map[string]any{"entity": "X", "entity_status": entityStatusExtracted}
	store.entities["b"] = map[string]any{"entity": "Y", "entity_status": entityStatusExtracted}
	store.candidates = []CandidatePair{
		{CandidateID: 200, LoEntityID: "a", HiEntityID: "b", Score: 0.5},
	}
	llm := &fakeAdjudicator{result: AdjudicationResult{NeedsHuman: []int64{200}}}

	r := &Reconciler{store: store, llm: llm, cfg: ReconcileConfig{AutoMergeMin: 0.8, AutoRejectMax: 0.2, MaxLLMGroupsPerRun: 10}}
	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stats.QueuedHuman != 1 {
		t.Errorf("QueuedHuman = %d, want 1", stats.QueuedHuman)
	}
	if stats.AutoMerged != 0 {
		t.Errorf("AutoMerged = %d, want 0", stats.AutoMerged)
	}
	if store.decisions[200] != decisionNeedsHuman {
		t.Errorf("decision = %q, want %q", store.decisions[200], decisionNeedsHuman)
	}
}

// ---- test doubles ----

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

type fakeReconcileStore struct {
	entities   map[string]map[string]any
	candidates []CandidatePair

	merges          []MergeDecision
	decisions       map[int64]string
	finished        bool
	finishOK        bool
	finishWatermark time.Time
}

func newFakeReconcileStore() *fakeReconcileStore {
	return &fakeReconcileStore{entities: map[string]map[string]any{}, decisions: map[int64]string{}}
}

func (f *fakeReconcileStore) BeginRun(_ context.Context, _ string) (int64, time.Time, error) {
	return 1, time.Time{}, nil
}

func (f *fakeReconcileStore) FinishRun(_ context.Context, _ int64, ok bool, watermarkTo time.Time, _ ReconcileStats, _ error) error {
	f.finished, f.finishOK, f.finishWatermark = true, ok, watermarkTo
	return nil
}

func (f *fakeReconcileStore) BlockCandidates(_ context.Context, _ int64, _ time.Time, _ ReconcileConfig) ([]CandidatePair, error) {
	return f.candidates, nil
}

func (f *fakeReconcileStore) LoadEntity(_ context.Context, id string) (map[string]any, error) {
	return f.entities[id], nil
}

func (f *fakeReconcileStore) RecordDecision(_ context.Context, candidateID int64, decision, _, _ string) error {
	f.decisions[candidateID] = decision
	return nil
}

func (f *fakeReconcileStore) ApplyMerge(_ context.Context, m MergeDecision) (int64, error) {
	f.merges = append(f.merges, m)
	return 0, nil
}

func (f *fakeReconcileStore) MarkClustered(_ context.Context, _ []string, _ string) error { return nil }

type fakeAdjudicator struct {
	result AdjudicationResult
	called bool
}

func (a *fakeAdjudicator) Adjudicate(_ context.Context, _ []map[string]any) (AdjudicationResult, error) {
	a.called = true
	return a.result, nil
}
