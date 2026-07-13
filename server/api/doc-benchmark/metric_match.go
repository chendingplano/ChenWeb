package docbenchmark

import (
	"encoding/json"
	"math/big"
	"sort"
)

const MetricWeightScale = 1_000_000
const MetricAcceptanceWeight = 600_000

type MetricRecord struct {
	GoldID                                                                           string
	PredictionInputIndex                                                             int
	Name, NameEn, Subject, SubjectEn, Value, Unit, UnitEn                            *string
	Desc, DescEn, Context, ContextEn                                                 *string
	LocationType, ValueDataType, ValueRangeType, ValueClass, ValueClassEn            *string
	FormulaOrDefinition, ThresholdOrTarget, MeasurementFrequency, TableNameOrSection *string
	IsExplicitMetric                                                                 *bool
	SourceLines                                                                      []int
	// StableFields is presence-aware canonical JSON. Map membership means the gold
	// fixture explicitly specifies the field; a JSON null is therefore distinct
	// from omission, while its normalized value is absent.
	StableFields map[string]json.RawMessage
}

type MetricEdgeComponents struct{ Source, Name, Subject, Value, Unit int }
type MetricEdgeExactComponents struct{ Source, Name, Subject, Value, Unit string }

type MetricEdge struct {
	GoldID                                           string
	GoldIndex, PredictionIndex, PredictionInputIndex int
	Eligible, Acceptable                             bool
	Weight                                           int
	ExactWeight                                      string
	Components                                       MetricEdgeComponents
	ExactComponents                                  MetricEdgeExactComponents
	exactWeight                                      *big.Rat
}

type MetricMatch struct {
	GoldID                                           string
	GoldIndex, PredictionIndex, PredictionInputIndex int
	Weight                                           int
	ExactWeight                                      string
	Components                                       MetricEdgeComponents
	ExactComponents                                  MetricEdgeExactComponents
}

func MetricEdgeFor(gold, prediction MetricRecord) MetricEdge {
	g, p := normalizeRecord(gold), normalizeRecord(prediction)
	sourceCommon := intIntersectionCount(g.SourceLines, p.SourceLines)
	sourceUnion := len(g.SourceLines) + len(p.SourceLines) - sourceCommon
	c := MetricEdgeComponents{
		Source:  scaledRatio(350000, sourceCommon, sourceUnion),
		Name:    scaledRatio(200000, tokenIntersection(g.Name, p.Name), tokenUnion(g.Name, p.Name)),
		Subject: scaledRatio(150000, tokenIntersection(g.Subject, p.Subject), tokenUnion(g.Subject, p.Subject)),
	}
	exactComponents := []*big.Rat{weightedJaccard(35, 100, sourceCommon, sourceUnion), weightedJaccard(20, 100, tokenIntersection(g.Name, p.Name), tokenUnion(g.Name, p.Name)), weightedJaccard(15, 100, tokenIntersection(g.Subject, p.Subject), tokenUnion(g.Subject, p.Subject)), new(big.Rat), new(big.Rat)}
	if ValuesAgree(g.Value, p.Value) {
		c.Value = 200000
		exactComponents[3].SetFrac64(20, 100)
	}
	if UnitsAgree(g.Unit, p.Unit) {
		c.Unit = 100000
		exactComponents[4].SetFrac64(10, 100)
	}
	exact := 0
	if exactPresentAgreement(g.Name, p.Name) {
		exact++
	}
	if exactPresentAgreement(g.Subject, p.Subject) {
		exact++
	}
	if ValuesAgree(g.Value, p.Value) {
		exact++
	}
	if UnitsAgree(g.Unit, p.Unit) {
		exact++
	}
	exactWeight := new(big.Rat)
	for _, component := range exactComponents {
		exactWeight.Add(exactWeight, component)
	}
	weight := ratMillionthsFloor(exactWeight)
	exactC := MetricEdgeExactComponents{canonicalRat(exactComponents[0]), canonicalRat(exactComponents[1]), canonicalRat(exactComponents[2]), canonicalRat(exactComponents[3]), canonicalRat(exactComponents[4])}
	eligible := sourceCommon > 0 || exact >= 2
	return MetricEdge{GoldID: gold.GoldID, PredictionInputIndex: prediction.PredictionInputIndex,
		Eligible: eligible, Acceptable: eligible && exactWeight.Cmp(metricThresholdRat()) >= 0, Weight: weight, ExactWeight: canonicalRat(exactWeight), exactWeight: exactWeight, Components: c, ExactComponents: exactC}
}

func BuildMetricEdges(gold, predictions []MetricRecord) []MetricEdge {
	out := make([]MetricEdge, 0, len(gold)*len(predictions))
	for gi, g := range gold {
		for pi, p := range predictions {
			e := MetricEdgeFor(g, p)
			e.GoldIndex, e.PredictionIndex = gi, pi
			out = append(out, e)
		}
	}
	return out
}

func MatchMetrics(gold, predictions []MetricRecord) []MetricMatch {
	return optimalMetricMatches(len(gold), len(predictions), BuildMetricEdges(gold, predictions))
}

func optimalMetricMatches(goldCount, predictionCount int, edges []MetricEdge) []MetricMatch {
	candidates := make([]MetricEdge, 0, len(edges))
	for _, e := range edges {
		if e.Acceptable {
			candidates = append(candidates, e)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].GoldID != candidates[j].GoldID {
			return candidates[i].GoldID < candidates[j].GoldID
		}
		if candidates[i].PredictionInputIndex != candidates[j].PredictionInputIndex {
			return candidates[i].PredictionInputIndex < candidates[j].PredictionInputIndex
		}
		if candidates[i].GoldIndex != candidates[j].GoldIndex {
			return candidates[i].GoldIndex < candidates[j].GoldIndex
		}
		return candidates[i].PredictionIndex < candidates[j].PredictionIndex
	})
	optimum := constrainedMaximumWeight(goldCount, predictionCount, candidates, nil, nil)
	if optimum == nil || optimum.Sign() <= 0 {
		return nil
	}
	fixed := make([]MetricEdge, 0, minInt(goldCount, predictionCount))
	forbidden := make(map[[2]int]bool)
	last := -1
	fixedWeight := new(big.Rat)
	for fixedWeight.Cmp(optimum) < 0 {
		selected := -1
		for i := last + 1; i < len(candidates); i++ {
			e := candidates[i]
			if conflictsFixed(e, fixed) {
				continue
			}
			trialForbidden := cloneForbidden(forbidden)
			for j := last + 1; j < i; j++ {
				trialForbidden[edgeKey(candidates[j])] = true
			}
			trialFixed := append(append([]MetricEdge(nil), fixed...), e)
			trial := constrainedMaximumWeight(goldCount, predictionCount, candidates, trialFixed, trialForbidden)
			if trial != nil && trial.Cmp(optimum) == 0 {
				selected = i
				forbidden = trialForbidden
				break
			}
		}
		if selected < 0 {
			break
		}
		e := candidates[selected]
		fixed = append(fixed, e)
		fixedWeight.Add(fixedWeight, edgeRat(e))
		last = selected
	}
	out := make([]MetricMatch, len(fixed))
	for i, e := range fixed {
		out[i] = MetricMatch{GoldID: e.GoldID, GoldIndex: e.GoldIndex, PredictionIndex: e.PredictionIndex, PredictionInputIndex: e.PredictionInputIndex, Weight: e.Weight, ExactWeight: canonicalRat(edgeRat(e)), Components: e.Components, ExactComponents: e.ExactComponents}
	}
	sortMatches(out)
	return out
}

func constrainedMaximumWeight(goldCount, predictionCount int, edges, fixed []MetricEdge, forbidden map[[2]int]bool) *big.Rat {
	usedGold, usedPred := make([]bool, goldCount), make([]bool, predictionCount)
	total := new(big.Rat)
	for _, e := range fixed {
		if e.GoldIndex < 0 || e.GoldIndex >= goldCount || e.PredictionIndex < 0 || e.PredictionIndex >= predictionCount || usedGold[e.GoldIndex] || usedPred[e.PredictionIndex] || forbidden[edgeKey(e)] {
			return nil
		}
		usedGold[e.GoldIndex] = true
		usedPred[e.PredictionIndex] = true
		total.Add(total, edgeRat(e))
	}
	golds, preds := []int{}, []int{}
	for i := 0; i < goldCount; i++ {
		if !usedGold[i] {
			golds = append(golds, i)
		}
	}
	for i := 0; i < predictionCount; i++ {
		if !usedPred[i] {
			preds = append(preds, i)
		}
	}
	n := len(golds) + len(preds)
	if n == 0 {
		return new(big.Rat).Set(total)
	}
	absent := new(big.Rat).Neg(new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(100), nil)))
	w := make([][]*big.Rat, n)
	for i := range w {
		w[i] = make([]*big.Rat, n)
		for j := range w[i] {
			w[i][j] = new(big.Rat)
		}
	}
	for r := 0; r < len(golds); r++ {
		for c := 0; c < len(preds); c++ {
			w[r][c] = new(big.Rat).Set(absent)
		}
	}
	goldPos, predPos := map[int]int{}, map[int]int{}
	for i, v := range golds {
		goldPos[v] = i
	}
	for i, v := range preds {
		predPos[v] = i
	}
	for _, e := range edges {
		r, rok := goldPos[e.GoldIndex]
		c, cok := predPos[e.PredictionIndex]
		if rok && cok && !forbidden[edgeKey(e)] && edgeRat(e).Cmp(w[r][c]) > 0 {
			w[r][c] = new(big.Rat).Set(edgeRat(e))
		}
	}
	return new(big.Rat).Add(total, hungarianMaximum(w))
}

func hungarianMaximum(weight [][]*big.Rat) *big.Rat {
	n := len(weight)
	u, v := make([]*big.Rat, n+1), make([]*big.Rat, n+1)
	for i := 0; i <= n; i++ {
		u[i] = new(big.Rat)
		v[i] = new(big.Rat)
	}
	p, way := make([]int, n+1), make([]int, n+1)
	inf := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(200), nil))
	for i := 1; i <= n; i++ {
		p[0] = i
		j0 := 0
		minv := make([]*big.Rat, n+1)
		for j := range minv {
			minv[j] = new(big.Rat).Set(inf)
		}
		used := make([]bool, n+1)
		for {
			used[j0] = true
			i0 := p[j0]
			delta := new(big.Rat).Set(inf)
			j1 := 0
			for j := 1; j <= n; j++ {
				if used[j] {
					continue
				}
				cur := new(big.Rat).Neg(weight[i0-1][j-1])
				cur.Sub(cur, u[i0])
				cur.Sub(cur, v[j])
				if cur.Cmp(minv[j]) < 0 {
					minv[j] = new(big.Rat).Set(cur)
					way[j] = j0
				}
				if minv[j].Cmp(delta) < 0 {
					delta = new(big.Rat).Set(minv[j])
					j1 = j
				}
			}
			for j := 0; j <= n; j++ {
				if used[j] {
					u[p[j]].Add(u[p[j]], delta)
					v[j].Sub(v[j], delta)
				} else {
					minv[j].Sub(minv[j], delta)
				}
			}
			j0 = j1
			if p[j0] == 0 {
				break
			}
		}
		for {
			j1 := way[j0]
			p[j0] = p[j1]
			j0 = j1
			if j0 == 0 {
				break
			}
		}
	}
	total := new(big.Rat)
	for j := 1; j <= n; j++ {
		if p[j] > 0 && weight[p[j]-1][j-1].Sign() > 0 {
			total.Add(total, weight[p[j]-1][j-1])
		}
	}
	return total
}

func weightedJaccard(wn, wd, n, d int) *big.Rat {
	if n == 0 || d == 0 {
		return new(big.Rat)
	}
	return new(big.Rat).Mul(new(big.Rat).SetFrac64(int64(wn), int64(wd)), new(big.Rat).SetFrac64(int64(n), int64(d)))
}
func metricThresholdRat() *big.Rat { return new(big.Rat).SetFrac64(3, 5) }
func canonicalRat(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	return r.RatString()
}
func edgeRat(e MetricEdge) *big.Rat {
	if e.exactWeight != nil {
		return e.exactWeight
	}
	if e.ExactWeight != "" {
		if r, ok := new(big.Rat).SetString(e.ExactWeight); ok {
			return r
		}
	}
	return new(big.Rat).SetFrac64(int64(e.Weight), MetricWeightScale)
}
func ratMillionthsFloor(r *big.Rat) int {
	n := new(big.Int).Mul(r.Num(), big.NewInt(MetricWeightScale))
	n.Quo(n, r.Denom())
	return int(n.Int64())
}

func edgeKey(e MetricEdge) [2]int { return [2]int{e.GoldIndex, e.PredictionIndex} }
func conflictsFixed(e MetricEdge, fixed []MetricEdge) bool {
	for _, f := range fixed {
		if e.GoldIndex == f.GoldIndex || e.PredictionIndex == f.PredictionIndex {
			return true
		}
	}
	return false
}
func cloneForbidden(in map[[2]int]bool) map[[2]int]bool {
	out := make(map[[2]int]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sortMatches(m []MetricMatch) {
	sort.Slice(m, func(i, j int) bool {
		if m[i].GoldID != m[j].GoldID {
			return m[i].GoldID < m[j].GoldID
		}
		return m[i].PredictionInputIndex < m[j].PredictionInputIndex
	})
}

func normalizeRecord(r MetricRecord) NormalizedMetric {
	return NormalizeMetric(MetricFields{Name: r.Name, Subject: r.Subject, Value: r.Value, Unit: r.Unit, SourceLines: r.SourceLines})
}
func scaledRatio(scale, numerator, denominator int) int {
	if numerator == 0 || denominator == 0 {
		return 0
	}
	return scale * numerator / denominator
}
func intIntersectionCount(a, b []int) int {
	i, j, n := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			n++
			i++
			j++
		}
	}
	return n
}
func tokenIntersection(a, b NormalizedField) int {
	if len(a.Tokens) == 0 || len(b.Tokens) == 0 {
		return 0
	}
	return sortedIntersectionCount(a.Tokens, b.Tokens)
}
func tokenUnion(a, b NormalizedField) int {
	common := tokenIntersection(a, b)
	if common == 0 {
		return 0
	}
	return len(a.Tokens) + len(b.Tokens) - common
}
