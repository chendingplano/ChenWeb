package docbenchmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type BenchmarkReport struct {
	ID                   string                    `json:"id"`
	GeneratedAt          string                    `json:"generated_at,omitempty"`
	DatasetHash          string                    `json:"dataset_hash,omitempty"`
	ScorerVersion        string                    `json:"scorer_version,omitempty"`
	NormalizationVersion string                    `json:"normalization_version,omitempty"`
	Aggregates           []AggregateRow            `json:"aggregates,omitempty"`
	Slices               map[string][]AggregateRow `json:"slices,omitempty"`
	PairedDeltas         []PairedDelta             `json:"paired_deltas,omitempty"`
	Warnings             []string                  `json:"warnings,omitempty"`
	Provenance           map[string]string         `json:"provenance,omitempty"`
	Completion           map[string]int            `json:"completion,omitempty"`
	Failures             map[string]int            `json:"failures,omitempty"`
	PrimaryVectors       map[string][]AggregateRow `json:"primary_vectors,omitempty"`
	LowestCases          []ReportCase              `json:"lowest_cases,omitempty"`
	Telemetry            map[string]AggregateRow   `json:"telemetry,omitempty"`
	PricingSnapshot      map[string]string         `json:"pricing_snapshot,omitempty"`
	EstimatedCost        *float64                  `json:"estimated_cost,omitempty"`
	Pareto               []ParetoPoint             `json:"pareto,omitempty"`
	Incomplete           bool                      `json:"incomplete,omitempty"`
	Incompatible         bool                      `json:"incompatible,omitempty"`
	NonGating            bool                      `json:"non_gating,omitempty"`
}

type ReportCase struct {
	CaseID          string   `json:"case_id"`
	Score           *float64 `json:"score"`
	ArtifactLinks   []string `json:"artifact_links,omitempty"`
	DiagnosticLinks []string `json:"diagnostic_links,omitempty"`
}
type ParetoPoint struct {
	Variant string   `json:"variant"`
	Quality *float64 `json:"quality"`
	Latency *float64 `json:"latency,omitempty"`
	Cost    *float64 `json:"cost,omitempty"`
}

func (r BenchmarkReport) canonical() BenchmarkReport {
	out := r
	out.Aggregates = append([]AggregateRow(nil), r.Aggregates...)
	sort.Slice(out.Aggregates, func(i, j int) bool { return canonicalKey(out.Aggregates[i]) < canonicalKey(out.Aggregates[j]) })
	out.PairedDeltas = append([]PairedDelta(nil), r.PairedDeltas...)
	sort.Slice(out.PairedDeltas, func(i, j int) bool { return canonicalKey(out.PairedDeltas[i]) < canonicalKey(out.PairedDeltas[j]) })
	if r.Warnings != nil {
		out.Warnings = append([]string(nil), r.Warnings...)
		sort.Strings(out.Warnings)
	}
	if r.Provenance != nil {
		out.Provenance = map[string]string{}
		for k, v := range r.Provenance {
			out.Provenance[k] = v
		}
	}
	if r.LowestCases != nil {
		out.LowestCases = append([]ReportCase(nil), r.LowestCases...)
		sort.Slice(out.LowestCases, func(i, j int) bool { return canonicalKey(out.LowestCases[i]) < canonicalKey(out.LowestCases[j]) })
	}
	if r.Pareto != nil {
		out.Pareto = append([]ParetoPoint(nil), r.Pareto...)
		sort.Slice(out.Pareto, func(i, j int) bool { return canonicalKey(out.Pareto[i]) < canonicalKey(out.Pareto[j]) })
	}
	if r.Slices != nil {
		out.Slices = map[string][]AggregateRow{}
		for k, v := range r.Slices {
			x := append([]AggregateRow(nil), v...)
			sort.Slice(x, func(i, j int) bool { return canonicalKey(x[i]) < canonicalKey(x[j]) })
			out.Slices[k] = x
		}
	}
	if r.PrimaryVectors != nil {
		out.PrimaryVectors = map[string][]AggregateRow{}
		for k, v := range r.PrimaryVectors {
			x := append([]AggregateRow(nil), v...)
			sort.Slice(x, func(i, j int) bool { return canonicalKey(x[i]) < canonicalKey(x[j]) })
			out.PrimaryVectors[k] = x
		}
	}
	if r.PricingSnapshot != nil {
		out.PricingSnapshot = map[string]string{}
		for k, v := range r.PricingSnapshot {
			out.PricingSnapshot[k] = v
		}
	}
	return out
}
func canonicalKey(v any) string { b, _ := json.Marshal(v); return string(b) }
func RenderJSON(r BenchmarkReport) ([]byte, error) {
	b, err := json.MarshalIndent(r.canonical(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
func RenderMarkdown(r BenchmarkReport) string {
	r = r.canonical()
	var b bytes.Buffer
	b.WriteString("# Benchmark report\n\n")
	if r.Incomplete || r.Incompatible || r.NonGating {
		b.WriteString("## Status\n\n")
		if r.Incomplete {
			b.WriteString("- incomplete comparison\n")
		}
		if r.Incompatible {
			b.WriteString("- incompatible comparison; winner language suppressed\n")
		}
		if r.NonGating {
			b.WriteString("- non-gating evaluation; no automatic winner\n")
		}
		b.WriteString("\n")
	}
	if r.ID != "" {
		b.WriteString("Report: " + mdEscape(r.ID) + "\n\n")
	}
	if len(r.Warnings) > 0 {
		b.WriteString("## Warnings\n\n")
		for _, w := range r.Warnings {
			b.WriteString("- " + mdEscape(w) + "\n")
		}
		b.WriteString("\n")
	}
	if len(r.Provenance) > 0 {
		b.WriteString("## Provenance\n\n")
		keys := make([]string, 0, len(r.Provenance))
		for k := range r.Provenance {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("- " + mdEscape(k) + ": " + mdEscape(r.Provenance[k]) + "\n")
		}
		b.WriteString("\n")
	}
	if len(r.Completion) > 0 || len(r.Failures) > 0 {
		ck := make([]string, 0, len(r.Completion))
		for k := range r.Completion {
			ck = append(ck, k)
		}
		sort.Strings(ck)
		fk := make([]string, 0, len(r.Failures))
		for k := range r.Failures {
			fk = append(fk, k)
		}
		sort.Strings(fk)
		b.WriteString("## Completion and failures\n\n")
		for _, k := range ck {
			v := r.Completion[k]
			b.WriteString("- " + mdEscape(k) + ": " + fmt.Sprint(v) + "\n")
		}
		for _, k := range fk {
			v := r.Failures[k]
			b.WriteString("- failure " + mdEscape(k) + ": " + fmt.Sprint(v) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Metrics\n\n| Metric | Value | Aggregation |\n|---|---:|---|\n")
	for _, a := range r.Aggregates {
		v := "null"
		if a.Value != nil {
			v = fmtFloat(*a.Value)
		}
		b.WriteString("| " + mdEscape(a.Metric) + " | " + v + " | " + mdEscape(a.AggregationKind) + " |\n")
	}
	if len(r.Slices) > 0 {
		b.WriteString("\n## Slices\n\n")
		keys := make([]string, 0, len(r.Slices))
		for k := range r.Slices {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("### " + mdEscape(k) + "\n\n")
			for _, a := range r.Slices[k] {
				b.WriteString("- " + mdEscape(a.Metric) + ": " + fmtFloatPtr(a.Value) + " (n=" + fmt.Sprint(a.ApplicableTotal) + ")\n")
			}
		}
	}
	if len(r.PairedDeltas) > 0 {
		b.WriteString("\n## Paired deltas\n\n")
		for _, d := range r.PairedDeltas {
			b.WriteString("- " + mdEscape(d.Metric) + ": " + fmtFloatPtr(d.Delta) + " (" + mdEscape(d.AggregationKind) + ")\n")
		}
	}
	if len(r.LowestCases) > 0 {
		b.WriteString("\n## Lowest-scoring cases\n\n")
		for _, c := range r.LowestCases {
			b.WriteString("- " + mdEscape(c.CaseID) + ": " + fmtFloatPtr(c.Score) + " artifacts=" + mdEscape(strings.Join(c.ArtifactLinks, ", ")) + " diagnostics=" + mdEscape(strings.Join(c.DiagnosticLinks, ", ")) + "\n")
		}
	}
	if len(r.Telemetry) > 0 || r.EstimatedCost != nil || len(r.PricingSnapshot) > 0 {
		b.WriteString("\n## Telemetry and cost\n\n")
		keys := make([]string, 0, len(r.Telemetry))
		for k := range r.Telemetry {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("- " + mdEscape(k) + ": " + fmtFloatPtr(r.Telemetry[k].Value) + "\n")
		}
		if r.EstimatedCost != nil {
			b.WriteString("- estimated cost: " + fmtFloat(*r.EstimatedCost) + "\n")
		}
	}
	if len(r.PrimaryVectors) > 0 {
		b.WriteString("\n## Processor vectors\n\n")
		keys := make([]string, 0, len(r.PrimaryVectors))
		for k := range r.PrimaryVectors {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("### " + mdEscape(k) + "\n\n")
			for _, a := range r.PrimaryVectors[k] {
				b.WriteString("- " + mdEscape(a.Metric) + ": " + fmtFloatPtr(a.Value) + "\n")
			}
		}
	}
	if len(r.PricingSnapshot) > 0 {
		b.WriteString("\n## Pricing\n\n")
		keys := make([]string, 0, len(r.PricingSnapshot))
		for k := range r.PricingSnapshot {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("- " + mdEscape(k) + ": " + mdEscape(r.PricingSnapshot[k]) + "\n")
		}
	}
	if len(r.Pareto) > 0 {
		b.WriteString("\n## Pareto trade-offs\n\n")
		for _, p := range r.Pareto {
			b.WriteString("- " + mdEscape(p.Variant) + ": quality=" + fmtFloatPtr(p.Quality) + " latency=" + fmtFloatPtr(p.Latency) + " cost=" + fmtFloatPtr(p.Cost) + "\n")
		}
	}
	return b.String()
}
func fmtFloatPtr(v *float64) string {
	if v == nil {
		return "null"
	}
	return fmtFloat(*v)
}
func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
func fmtFloat(v float64) string    { return json.Number(formatFloat(v)).String() }
func formatFloat(v float64) string { b, _ := json.Marshal(v); return string(b) }
