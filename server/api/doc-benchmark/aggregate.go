package docbenchmark

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ScoreUnit is one applicable (case,repetition) sampling unit.
type ScoreUnit struct {
	CaseID            string
	Repetition        int
	Tags              []string
	Applicable        bool
	UpstreamInvalid   bool
	UpstreamChunkHash string
	Scores            []ScoreRow
	Operational       map[string]float64
}

type AggregateRow struct {
	Metric          string   `json:"metric"`
	Component       string   `json:"component,omitempty"`
	Slice           string   `json:"slice,omitempty"`
	Direction       string   `json:"direction"`
	AggregationKind string   `json:"aggregation_kind"`
	Value           *float64 `json:"value"`
	Numerator       int      `json:"numerator"`
	Denominator     int      `json:"denominator"`
	NonNullUnits    int      `json:"non_null_units"`
	ApplicableTotal int      `json:"applicable_total"`
	Sum             float64  `json:"sum,omitempty"`
	Mean            *float64 `json:"mean,omitempty"`
	Median          *float64 `json:"median,omitempty"`
	PopulationSD    *float64 `json:"population_sd,omitempty"`
	CasesWithAny    int      `json:"cases_with_any,omitempty"`
	Count           int      `json:"count,omitempty"`
	TP              int      `json:"tp,omitempty"`
	FP              int      `json:"fp,omitempty"`
	FN              int      `json:"fn,omitempty"`
}

func AggregateScores(units []ScoreUnit, applicableTotal int) ([]AggregateRow, error) {
	return aggregate(units, applicableTotal, "")
}
func AggregateSlices(units []ScoreUnit, applicableTotal int) (map[string][]AggregateRow, error) {
	tags := map[string][]ScoreUnit{}
	for _, u := range units {
		for _, tag := range u.Tags {
			tags[tag] = append(tags[tag], u)
		}
	}
	out := map[string][]AggregateRow{}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		pop := 0
		for _, u := range tags[k] {
			if u.Applicable {
				pop++
			}
		}
		var err error
		out[k], err = aggregate(tags[k], pop, k)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
func aggregate(units []ScoreUnit, applicableTotal int, slice string) ([]AggregateRow, error) {
	type key struct{ m, c string }
	groups := map[key][]ScoreUnit{}
	defs := map[key]ScoreRow{}
	for _, u := range units {
		if !u.Applicable {
			continue
		}
		if len(u.Operational) > 0 {
			for name, value := range u.Operational {
				v := value
				u.Scores = append(u.Scores, ScoreRow{Metric: name, Direction: "lower", AggregationKind: "operational", Value: &v})
			}
		}
		seen := map[key]bool{}
		for _, r := range u.Scores {
			if u.UpstreamInvalid && r.ConditionalAttribution {
				continue
			}
			k := key{r.Metric, r.Component}
			if seen[k] {
				return nil, fmt.Errorf("duplicate score row %s/%s in %s", r.Metric, r.Component, u.CaseID)
			}
			seen[k] = true
			if d, ok := defs[k]; ok && (canonicalAggregationKind(d.AggregationKind) != canonicalAggregationKind(r.AggregationKind) || d.Direction != r.Direction) {
				return nil, fmt.Errorf("conflicting declaration for %s/%s", r.Metric, r.Component)
			}
			groups[k] = append(groups[k], u)
			defs[k] = r
		}
	}
	keys := make([]key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].m != keys[j].m {
			return keys[i].m < keys[j].m
		}
		return keys[i].c < keys[j].c
	})
	out := make([]AggregateRow, 0, len(keys))
	for _, k := range keys {
		r := defs[k]
		us := groups[k]
		kind := canonicalAggregationKind(r.AggregationKind)
		a := AggregateRow{Metric: k.m, Component: k.c, Slice: slice, Direction: r.Direction, AggregationKind: kind, ApplicableTotal: applicableTotal}
		vals := []float64{}
		tp, fp, fn := 0, 0, 0
		num, den := 0, 0
		eligibleUnits := map[string]bool{}
		sum := 0.0
		any := 0
		for _, u := range us {
			for _, x := range u.Scores {
				if u.UpstreamInvalid && x.ConditionalAttribution {
					continue
				}
				if x.Value != nil && (math.IsNaN(*x.Value) || math.IsInf(*x.Value, 0)) {
					return nil, fmt.Errorf("metric %s has non-finite value", x.Metric)
				}
				if x.Metric != k.m || x.Component != k.c {
					continue
				}
				kind := canonicalAggregationKind(r.AggregationKind)
				switch kind {
				case "count_derived_micro":
					tp += x.TP
					fp += x.FP
					fn += x.FN
				case "matched_field_micro":
					num += x.Numerator
					den += x.Denominator
					if x.Denominator > 0 {
						eligibleUnits[fmt.Sprintf("%s/%d", u.CaseID, u.Repetition)] = true
					}
				case "raw_count":
					n := x.Numerator
					if n == 0 && x.Value != nil {
						n = int(*x.Value)
					}
					num += n
					if n != 0 {
						any++
					}
				case "operational":
					if x.Value != nil {
						if math.IsNaN(*x.Value) || math.IsInf(*x.Value, 0) {
							return nil, fmt.Errorf("metric %s has non-finite value", x.Metric)
						}
						vals = append(vals, *x.Value)
						sum += *x.Value
					}
				default:
					if x.Value != nil {
						vals = append(vals, *x.Value)
					}
				}
			}
		}
		a.NonNullUnits = len(vals)
		a.Numerator = num
		a.Denominator = den
		a.CasesWithAny = any
		a.Count = len(vals)
		a.Sum = sum
		switch kind {
		case "count_derived_micro":
			a.NonNullUnits = len(us)
			a.TP, a.FP, a.FN = tp, fp, fn
			a.Numerator = tp
			a.Denominator = fp + tp
			a.Value = microValue(k.m, tp, fp, fn)
			if k.m == "detection_recall" || k.m == "grounding_recall" {
				a.Denominator = tp + fn
			}
			if k.m == "detection_f1" || k.m == "grounding_f1" {
				a.Denominator = tp + fn
			}
		case "matched_field_micro":
			if den > 0 {
				a.NonNullUnits = len(eligibleUnits)
			}
			if den > 0 {
				v := float64(num) / float64(den)
				a.Value = &v
			}
		case "raw_count":
			a.NonNullUnits = len(us)
			v := float64(num)
			a.Value = &v
		case "operational":
			if !setDistribution(&a, vals) {
				return nil, fmt.Errorf("aggregate %s overflow", a.Metric)
			}
		default:
			if !setDistribution(&a, vals) {
				return nil, fmt.Errorf("aggregate %s overflow", a.Metric)
			}
		}
		if kind == "binary_rate_macro" {
			a.Denominator = len(vals)
			for _, v := range vals {
				if v == 1 {
					a.Numerator++
				}
			}
		}
		if a.Value != nil && (math.IsNaN(*a.Value) || math.IsInf(*a.Value, 0)) {
			return nil, fmt.Errorf("aggregate %s overflow", a.Metric)
		}
		if a.Mean != nil && (math.IsNaN(*a.Mean) || math.IsInf(*a.Mean, 0)) {
			return nil, fmt.Errorf("aggregate %s mean overflow", a.Metric)
		}
		if a.PopulationSD != nil && (math.IsNaN(*a.PopulationSD) || math.IsInf(*a.PopulationSD, 0)) {
			return nil, fmt.Errorf("aggregate %s variance overflow", a.Metric)
		}
		out = append(out, a)
	}
	return out, nil
}

func canonicalAggregationKind(kind string) string {
	switch kind {
	case "binary_macro", "rate_macro", "binary_rate":
		return "binary_rate_macro"
	case "raw_failure_count", "rule_count", "raw_counts":
		return "raw_count"
	case "latency", "tokens", "cache_hit", "cache_miss", "cost", "operational_measure":
		return "operational"
	default:
		return kind
	}
}
func microValue(metric string, tp, fp, fn int) *float64 {
	if (metric == "grounding_precision" || metric == "grounding_recall" || metric == "grounding_f1") && tp+fp+fn == 0 {
		return nil
	}
	den := tp + fp
	if metric == "detection_recall" || metric == "grounding_recall" {
		den = tp + fn
	}
	if metric == "detection_f1" || metric == "grounding_f1" {
		if tp+fp == 0 && tp+fn == 0 {
			v := 1.0
			return &v
		}
		if tp+fp == 0 || tp+fn == 0 {
			v := 0.0
			return &v
		}
		p := float64(tp) / float64(tp+fp)
		q := float64(tp) / float64(tp+fn)
		v := 2 * p * q / (p + q)
		return &v
	}
	if den == 0 {
		if tp+fn == 0 {
			v := 1.0
			return &v
		}
		v := 0.0
		return &v
	}
	v := float64(tp) / float64(den)
	return &v
}
func setDistribution(a *AggregateRow, vals []float64) bool {
	if len(vals) == 0 {
		return true
	}
	sort.Float64s(vals)
	mean := 0.0
	for _, v := range vals {
		mean += v
		if math.IsInf(mean, 0) || math.IsNaN(mean) {
			return false
		}
	}
	mean /= float64(len(vals))
	med := vals[len(vals)/2]
	if len(vals)%2 == 0 {
		med = (vals[len(vals)/2-1] + med) / 2
	}
	sd := 0.0
	for _, v := range vals {
		sd += (v - mean) * (v - mean)
	}
	sd = math.Sqrt(sd / float64(len(vals)))
	if math.IsInf(sd, 0) || math.IsNaN(sd) || math.IsInf(mean, 0) || math.IsNaN(mean) {
		return false
	}
	a.Mean = &mean
	a.Median = &med
	a.PopulationSD = &sd
	a.Value = &mean
	a.Sum = mean * float64(len(vals))
	if math.IsInf(a.Sum, 0) || math.IsNaN(a.Sum) {
		return false
	}
	return true
}

type VariantComparison struct {
	Baseline, Candidate                                                                         []ScoreUnit
	DatasetHash, BaselineCaseSetHash, CandidateCaseSetHash, ScorerVersion, NormalizationVersion string
	BaselineDatasetHash, CandidateDatasetHash, BaselineScorerVersion, CandidateScorerVersion    string
	BaselineNormalizationVersion, CandidateNormalizationVersion                                 string
	BaselineUpstreamHash, CandidateUpstreamHash                                                 string
	AllowUpstreamVariation, AllowIncompatible                                                   bool
}
type PairedDelta struct {
	Metric          string   `json:"metric"`
	Component       string   `json:"component,omitempty"`
	Delta           *float64 `json:"delta"`
	PairedUnits     int      `json:"paired_units"`
	ApplicableUnits int      `json:"applicable_units"`
	AggregationKind string   `json:"aggregation_kind,omitempty"`
	Median          *float64 `json:"median,omitempty"`
	PopulationSD    *float64 `json:"population_sd,omitempty"`
}

func CompareVariants(c VariantComparison) ([]PairedDelta, []string, error) {
	warnings := []string{}
	datasetA, datasetB := c.BaselineDatasetHash, c.CandidateDatasetHash
	if datasetA == "" {
		datasetA = c.DatasetHash
	}
	if datasetB == "" {
		datasetB = c.DatasetHash
	}
	scorerA, scorerB := c.BaselineScorerVersion, c.CandidateScorerVersion
	if scorerA == "" {
		scorerA = c.ScorerVersion
	}
	if scorerB == "" {
		scorerB = c.ScorerVersion
	}
	normA, normB := c.BaselineNormalizationVersion, c.CandidateNormalizationVersion
	if normA == "" {
		normA = c.NormalizationVersion
	}
	if normB == "" {
		normB = c.NormalizationVersion
	}
	if datasetA == "" || datasetB == "" || datasetA != datasetB || c.BaselineCaseSetHash == "" || c.CandidateCaseSetHash == "" || c.BaselineCaseSetHash != c.CandidateCaseSetHash || scorerA == "" || scorerB == "" || scorerA != scorerB || normA == "" || normB == "" || normA != normB {
		if !c.AllowIncompatible {
			return nil, nil, fmt.Errorf("incompatible comparison")
		}
		warnings = append(warnings, "INCOMPATIBLE COMPARISON: winner language suppressed")
	}
	if c.BaselineUpstreamHash != c.CandidateUpstreamHash && !c.AllowUpstreamVariation {
		return nil, nil, fmt.Errorf("upstream chunk hashes differ")
	}
	if c.BaselineUpstreamHash != c.CandidateUpstreamHash && c.AllowUpstreamVariation {
		warnings = append(warnings, "end-to-end pipeline delta; isolated component claims suppressed")
	}
	type unitKey struct {
		caseID     string
		repetition int
	}
	b := map[unitKey]ScoreUnit{}
	candidateSeen := map[unitKey]bool{}
	for _, u := range c.Baseline {
		key := unitKey{u.CaseID, u.Repetition}
		if _, ok := b[key]; ok {
			return nil, nil, fmt.Errorf("duplicate baseline unit %s", u.CaseID)
		}
		b[key] = u
	}
	pairs := map[string][]float64{}
	type metricKey struct{ metric, component, kind string }
	pooledA, pooledB := map[metricKey][3]int{}, map[metricKey][3]int{}
	applicableByMetric := map[metricKey]int{}
	for _, u := range c.Candidate {
		keyUnit := unitKey{u.CaseID, u.Repetition}
		if _, seen := candidateSeen[keyUnit]; seen {
			return nil, nil, fmt.Errorf("duplicate candidate unit %s", u.CaseID)
		}
		candidateSeen[keyUnit] = true
		if x, ok := b[keyUnit]; ok {
			if !u.Applicable || !x.Applicable {
				continue
			}
			for _, r := range u.Scores {
				for _, br := range x.Scores {
					if r.Metric != br.Metric || r.Component != br.Component {
						continue
					}
					if canonicalAggregationKind(r.AggregationKind) == "count_derived_micro" && canonicalAggregationKind(br.AggregationKind) == "count_derived_micro" {
						if u.UpstreamInvalid && r.ConditionalAttribution {
							continue
						}
						if x.UpstreamInvalid && br.ConditionalAttribution {
							continue
						}
						key := metricKey{r.Metric, r.Component, canonicalAggregationKind(r.AggregationKind)}
						a := pooledA[key]
						a[0] += br.TP
						a[1] += br.FP
						a[2] += br.FN
						pooledA[key] = a
						q := pooledB[key]
						q[0] += r.TP
						q[1] += r.FP
						q[2] += r.FN
						pooledB[key] = q
						applicableByMetric[key]++
					}
					conditional := r.ConditionalAttribution || br.ConditionalAttribution
					if c.BaselineUpstreamHash != c.CandidateUpstreamHash && c.AllowUpstreamVariation && conditional {
						continue
					}
					if r.Value != nil && br.Value != nil && !(u.UpstreamInvalid && r.ConditionalAttribution) && !(x.UpstreamInvalid && br.ConditionalAttribution) {
						// coverage for macro metrics is tracked independently below
						pairs[r.Metric+"\x00"+r.Component] = append(pairs[r.Metric+"\x00"+r.Component], *r.Value-*br.Value)
					}
				}
			}
		}
	}
	out := []PairedDelta{}
	for m, v := range pairs {
		mean := 0.
		for _, x := range v {
			mean += x
		}
		mean /= float64(len(v))
		med, sd := distribution(v)
		parts := strings.SplitN(m, "\x00", 2)
		metric, component := parts[0], ""
		if len(parts) == 2 {
			component = parts[1]
		}
		out = append(out, PairedDelta{Metric: metric, Component: component, Delta: &mean, Median: med, PopulationSD: sd, AggregationKind: "paired_macro_diagnostic", PairedUnits: len(v), ApplicableUnits: len(v)})
	}
	for key, a := range pooledA {
		q := pooledB[key]
		va := microValue(key.metric, a[0], a[1], a[2])
		vb := microValue(key.metric, q[0], q[1], q[2])
		if va != nil && vb != nil {
			d := *vb - *va
			out = append(out, PairedDelta{Metric: key.metric, Component: key.component, Delta: &d, AggregationKind: key.kind, PairedUnits: applicableByMetric[key], ApplicableUnits: applicableByMetric[key]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return canonicalKey(out[i]) < canonicalKey(out[j]) })
	return out, warnings, nil
}

func distribution(v []float64) (*float64, *float64) {
	if len(v) == 0 {
		return nil, nil
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	m := 0.
	for _, x := range s {
		m += x
	}
	m /= float64(len(s))
	md := s[len(s)/2]
	if len(s)%2 == 0 {
		md = (md + s[len(s)/2-1]) / 2
	}
	sd := 0.
	for _, x := range s {
		sd += (x - m) * (x - m)
	}
	sd = math.Sqrt(sd / float64(len(s)))
	return &md, &sd
}
