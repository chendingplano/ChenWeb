package docbenchmark

import (
	"fmt"
	"math"
	"sort"
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
}

func AggregateScores(units []ScoreUnit, applicableTotal int) ([]AggregateRow, error) {
	return aggregate(units, applicableTotal, "")
}
func AggregateSlices(units []ScoreUnit, applicableTotal int) map[string][]AggregateRow {
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
		out[k], _ = aggregate(tags[k], applicableTotal, k)
	}
	return out
}
func aggregate(units []ScoreUnit, applicableTotal int, slice string) ([]AggregateRow, error) {
	type key struct{ m, c string }
	groups := map[key][]ScoreUnit{}
	defs := map[key]ScoreRow{}
	for _, u := range units {
		if !u.Applicable {
			continue
		}
		for _, r := range u.Scores {
			k := key{r.Metric, r.Component}
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
		a := AggregateRow{Metric: k.m, Component: k.c, Slice: slice, Direction: r.Direction, AggregationKind: r.AggregationKind, ApplicableTotal: applicableTotal}
		vals := []float64{}
		tp, fp, fn := 0, 0, 0
		num, den := 0, 0
		sum := 0.0
		any := 0
		for _, u := range us {
			for _, x := range u.Scores {
				if x.Metric != k.m || x.Component != k.c {
					continue
				}
				switch r.AggregationKind {
				case "count_derived_micro":
					tp += x.TP
					fp += x.FP
					fn += x.FN
				case "matched_field_micro":
					num += x.Numerator
					den += x.Denominator
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
		switch r.AggregationKind {
		case "count_derived_micro":
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
				v := float64(num) / float64(den)
				a.Value = &v
			}
		case "raw_count":
			v := float64(num)
			a.Value = &v
		case "operational":
			setDistribution(&a, vals)
		default:
			setDistribution(&a, vals)
		}
		if r.AggregationKind == "binary_rate_macro" || r.AggregationKind == "rate_macro" {
			a.Denominator = len(vals)
			for _, v := range vals {
				if v == 1 {
					a.Numerator++
				}
			}
		}
		out = append(out, a)
	}
	return out, nil
}
func microValue(metric string, tp, fp, fn int) *float64 {
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
func setDistribution(a *AggregateRow, vals []float64) {
	if len(vals) == 0 {
		return
	}
	sort.Float64s(vals)
	mean := 0.0
	for _, v := range vals {
		mean += v
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
	a.Mean = &mean
	a.Median = &med
	a.PopulationSD = &sd
	a.Value = &mean
	a.Sum = mean * float64(len(vals))
}

type VariantComparison struct {
	Baseline, Candidate                                                                         []ScoreUnit
	DatasetHash, BaselineCaseSetHash, CandidateCaseSetHash, ScorerVersion, NormalizationVersion string
	BaselineUpstreamHash, CandidateUpstreamHash                                                 string
	AllowUpstreamVariation, AllowIncompatible                                                   bool
}
type PairedDelta struct {
	Metric                       string   `json:"metric"`
	Delta                        *float64 `json:"delta"`
	PairedUnits, ApplicableUnits int      `json:"paired_units"`
}

func CompareVariants(c VariantComparison) ([]PairedDelta, []string, error) {
	warnings := []string{}
	if c.DatasetHash == "" || c.BaselineCaseSetHash != c.CandidateCaseSetHash || c.ScorerVersion == "" || c.NormalizationVersion == "" {
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
	b := map[string]ScoreUnit{}
	for _, u := range c.Baseline {
		b[fmt.Sprintf("%s/%d", u.CaseID, u.Repetition)] = u
	}
	pairs := map[string][]float64{}
	for _, u := range c.Candidate {
		if x, ok := b[fmt.Sprintf("%s/%d", u.CaseID, u.Repetition)]; ok {
			for _, r := range u.Scores {
				for _, br := range x.Scores {
					if r.Metric == br.Metric && r.Value != nil && br.Value != nil {
						pairs[r.Metric] = append(pairs[r.Metric], *r.Value-*br.Value)
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
		out = append(out, PairedDelta{Metric: m, Delta: &mean, PairedUnits: len(v), ApplicableUnits: len(c.Candidate)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Metric < out[j].Metric })
	return out, warnings, nil
}
