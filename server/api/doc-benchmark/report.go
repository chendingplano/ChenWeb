package docbenchmark

import (
	"bytes"
	"encoding/json"
	"html"
	"sort"
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
}

func (r BenchmarkReport) canonical() BenchmarkReport {
	out := r
	out.Aggregates = append([]AggregateRow(nil), r.Aggregates...)
	sort.Slice(out.Aggregates, func(i, j int) bool {
		if out.Aggregates[i].Metric != out.Aggregates[j].Metric {
			return out.Aggregates[i].Metric < out.Aggregates[j].Metric
		}
		return out.Aggregates[i].Component < out.Aggregates[j].Component
	})
	out.PairedDeltas = append([]PairedDelta(nil), r.PairedDeltas...)
	sort.Slice(out.PairedDeltas, func(i, j int) bool { return out.PairedDeltas[i].Metric < out.PairedDeltas[j].Metric })
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
	return out
}
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
	if r.ID != "" {
		b.WriteString("Report: `" + html.EscapeString(r.ID) + "`\n\n")
	}
	if len(r.Warnings) > 0 {
		b.WriteString("## Warnings\n\n")
		for _, w := range r.Warnings {
			b.WriteString("- " + html.EscapeString(w) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Metrics\n\n| Metric | Value | Aggregation |\n|---|---:|---|\n")
	for _, a := range r.Aggregates {
		v := "null"
		if a.Value != nil {
			v = fmtFloat(*a.Value)
		}
		b.WriteString("| " + html.EscapeString(a.Metric) + " | " + v + " | " + html.EscapeString(a.AggregationKind) + " |\n")
	}
	return b.String()
}
func fmtFloat(v float64) string    { return json.Number(formatFloat(v)).String() }
func formatFloat(v float64) string { b, _ := json.Marshal(v); return string(b) }
