package docbenchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"reflect"
	"testing"
)

func TestMatchMetricsContextEdgeLimitAndCancellation(t *testing.T) {
	edges := make([]MetricEdge, MaxMetricEdges+1)
	if _, err := optimalMetricMatchesContext(context.Background(), 1, 1, edges); err == nil {
		t.Fatal("edge limit not enforced")
	} else {
		var limit *MetricResourceLimitError
		if !errors.As(err, &limit) {
			t.Fatalf("error type = %T", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MatchMetricsContext(ctx, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
}

func TestMatchMetricsExactRandomizedAgainstBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for iteration := 0; iteration < 40; iteration++ {
		g, p := 1+rng.Intn(4), 1+rng.Intn(4)
		edges := []MetricEdge{}
		for gi := 0; gi < g; gi++ {
			for pi := 0; pi < p; pi++ {
				if rng.Intn(4) == 0 {
					continue
				}
				w := fmt.Sprintf("%d/10", 6+rng.Intn(5))
				edges = append(edges, exactEdge(fmt.Sprintf("g%d", gi), gi, pi, pi, w))
			}
		}
		got := optimalMetricMatches(g, p, edges)
		want := bruteMetricMatches(g, p, edges)
		if !reflect.DeepEqual(matchPairs(got), matchPairs(want)) {
			t.Fatalf("iteration %d got %#v want %#v edges %#v", iteration, got, want, edges)
		}
	}
}

func TestMatchMetricsLexPrefixChoosesShorterEqualOptimum(t *testing.T) {
	edges := []MetricEdge{exactEdge("a", 0, 0, 0, "1"), exactEdge("a", 0, 1, 1, "1/2"), exactEdge("b", 1, 0, 0, "1/2")}
	got := optimalMetricMatches(2, 2, edges)
	if pairs := matchPairs(got); !reflect.DeepEqual(pairs, [][2]any{{"a", 0}}) {
		t.Fatalf("prefix tie = %#v", pairs)
	}
}

func bruteMetricMatches(g, p int, edges []MetricEdge) []MetricMatch {
	by := map[int][]MetricEdge{}
	for _, e := range edges {
		by[e.GoldIndex] = append(by[e.GoldIndex], e)
	}
	best := new(big.Rat).SetInt64(-1)
	var answer []MetricMatch
	used := make([]bool, p)
	var visit func(int, *big.Rat, []MetricMatch)
	visit = func(gi int, total *big.Rat, current []MetricMatch) {
		if gi == g {
			sortMatches(current)
			if total.Cmp(best) > 0 || (total.Cmp(best) == 0 && pairsLess(current, answer)) {
				best.Set(total)
				answer = append([]MetricMatch(nil), current...)
			}
			return
		}
		visit(gi+1, new(big.Rat).Set(total), append([]MetricMatch(nil), current...))
		for _, e := range by[gi] {
			if used[e.PredictionIndex] {
				continue
			}
			used[e.PredictionIndex] = true
			m := MetricMatch{GoldID: e.GoldID, GoldIndex: e.GoldIndex, PredictionIndex: e.PredictionIndex, PredictionInputIndex: e.PredictionInputIndex, ExactWeight: canonicalRat(edgeRat(e))}
			visit(gi+1, new(big.Rat).Add(total, edgeRat(e)), append(current, m))
			used[e.PredictionIndex] = false
		}
	}
	visit(0, new(big.Rat), nil)
	return answer
}
func matchPairs(m []MetricMatch) [][2]any {
	out := make([][2]any, len(m))
	for i, x := range m {
		out[i] = [2]any{x.GoldID, x.PredictionInputIndex}
	}
	return out
}
func pairsLess(a, b []MetricMatch) bool {
	if b == nil {
		return true
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i].GoldID != b[i].GoldID {
			return a[i].GoldID < b[i].GoldID
		}
		if a[i].PredictionInputIndex != b[i].PredictionInputIndex {
			return a[i].PredictionInputIndex < b[i].PredictionInputIndex
		}
	}
	return len(a) < len(b)
}

func rat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic(s)
	}
	return r
}

func metric(id string, index int, name, subject, value, unit string, lines ...int) MetricRecord {
	return MetricRecord{GoldID: id, PredictionInputIndex: index, Name: ptr(name), Subject: ptr(subject), Value: ptr(value), Unit: ptr(unit), SourceLines: lines}
}

func TestMetricEdgesEligibilityWeightsAndThreshold(t *testing.T) {
	g := metric("g", 0, "request latency", "api", "100", "ms", 1, 2)
	p := metric("", 4, "request latency", "api", "1e2", "msec", 2, 3)
	e := MetricEdgeFor(g, p)
	if !e.Eligible || !e.Acceptable {
		t.Fatalf("edge = %#v", e)
	}
	if e.Components.Source != 116666 || e.Components.Name != 200000 || e.Components.Subject != 150000 || e.Components.Value != 200000 || e.Components.Unit != 100000 || e.Weight != 766666 {
		t.Fatalf("components = %#v weight=%d", e.Components, e.Weight)
	}

	// Two exact nonempty fields are eligible without a span intersection, but this
	// deliberately lands below the acceptance threshold.
	low := MetricEdgeFor(metric("g", 0, "same", "different", "1", "ms"), metric("", 0, "same", "other", "2", "ms"))
	if !low.Eligible || low.Acceptable || low.Weight != 300000 {
		t.Fatalf("low edge = %#v", low)
	}
	one := MetricEdgeFor(metric("g", 0, "same", "x", "1", "ms"), metric("", 0, "same", "y", "2", "s"))
	if one.Eligible {
		t.Fatalf("one exact field made eligible: %#v", one)
	}
	atThreshold := MetricEdgeFor(metric("g", 0, "left", "same", "1", "ms", 8), metric("", 0, "right", "same", "2", "ms", 8))
	if atThreshold.Weight != MetricAcceptanceWeight || !atThreshold.Acceptable {
		t.Fatalf("edge at exact threshold = %#v", atThreshold)
	}
}

func TestMetricEdgeAcceptsMathematicallyExactThreshold(t *testing.T) {
	g := metric("g", 0, "a b", "same", "7", "ms", 1, 2)
	p := metric("", 0, "a b c", "same", "7", "s", 2, 3)
	e := MetricEdgeFor(g, p)
	if !e.Acceptable || e.ExactWeight != "3/5" {
		t.Fatalf("exact threshold edge = %#v", e)
	}
}

func TestMetricEdgeStableFieldsCorePresenceOverridesLegacy(t *testing.T) {
	g := metric("g", 0, "legacy", "subject", "1", "ms", 1)
	p := metric("", 0, "legacy", "subject", "1", "ms", 1)
	if e := MetricEdgeFor(g, p); e.Components.Name != 200000 {
		t.Fatalf("omitted map did not use legacy: %#v", e)
	}
	for _, raw := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`""`)} {
		g.StableFields = map[string]json.RawMessage{"metric_name": raw}
		p.StableFields = map[string]json.RawMessage{"metric_name": raw}
		if e := MetricEdgeFor(g, p); e.Components.Name != 0 {
			t.Fatalf("map core %s did not override legacy: %#v", raw, e)
		}
	}
}

func TestMatchMetricsExactRationalObjectiveAvoidsFlooringTie(t *testing.T) {
	edges := []MetricEdge{
		exactEdge("a", 0, 0, 0, "6000001/10000000"), exactEdge("a", 0, 1, 1, "3/5"),
		exactEdge("b", 1, 0, 0, "3/5"), exactEdge("b", 1, 1, 1, "3/5"),
	}
	got := optimalMetricMatches(2, 2, edges)
	if len(got) != 2 || got[0].PredictionInputIndex != 0 || got[1].PredictionInputIndex != 1 {
		t.Fatalf("exact objective assignment = %#v", got)
	}
}

func exactEdge(id string, gi, pi, input int, weight string) MetricEdge {
	return MetricEdge{GoldID: id, GoldIndex: gi, PredictionIndex: pi, PredictionInputIndex: input, Eligible: true, Acceptable: true, ExactWeight: weight, exactWeight: rat(weight)}
}

func TestMatchMetricsFindsGlobalOptimumAndRectangularForbidden(t *testing.T) {
	edges := []MetricEdge{
		{GoldID: "a", GoldIndex: 0, PredictionInputIndex: 0, PredictionIndex: 0, Eligible: true, Acceptable: true, Weight: 800000},
		{GoldID: "a", GoldIndex: 0, PredictionInputIndex: 1, PredictionIndex: 1, Eligible: true, Acceptable: true, Weight: 700000},
		{GoldID: "b", GoldIndex: 1, PredictionInputIndex: 0, PredictionIndex: 0, Eligible: true, Acceptable: true, Weight: 700000},
	}
	got := optimalMetricMatches(2, 3, edges)
	want := []MetricMatch{{GoldID: "a", GoldIndex: 0, PredictionInputIndex: 1, PredictionIndex: 1, Weight: 700000, ExactWeight: "7/10"}, {GoldID: "b", GoldIndex: 1, PredictionInputIndex: 0, PredictionIndex: 0, Weight: 700000, ExactWeight: "7/10"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("matches = %#v, want %#v", got, want)
	}
}

func TestMatchMetricsEqualOptimumUsesLexicographicallySmallestPairs(t *testing.T) {
	edges := []MetricEdge{
		{GoldID: "a", GoldIndex: 1, PredictionInputIndex: 9, PredictionIndex: 0, Acceptable: true, Weight: 600000},
		{GoldID: "a", GoldIndex: 1, PredictionInputIndex: 3, PredictionIndex: 1, Acceptable: true, Weight: 600000},
		{GoldID: "b", GoldIndex: 0, PredictionInputIndex: 9, PredictionIndex: 0, Acceptable: true, Weight: 600000},
		{GoldID: "b", GoldIndex: 0, PredictionInputIndex: 3, PredictionIndex: 1, Acceptable: true, Weight: 600000},
	}
	got := optimalMetricMatches(2, 2, edges)
	pairs := [][2]any{{got[0].GoldID, got[0].PredictionInputIndex}, {got[1].GoldID, got[1].PredictionInputIndex}}
	want := [][2]any{{"a", 3}, {"b", 9}}
	if !reflect.DeepEqual(pairs, want) {
		t.Fatalf("pairs = %#v, want %#v", pairs, want)
	}
}
