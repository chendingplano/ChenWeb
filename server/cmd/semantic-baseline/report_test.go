package main

import (
	"strings"
	"testing"
	"time"
)

// The capacity model is the numeric half of the Phase 0 gate, so its arithmetic
// is worth pinning: a silently wrong projection would authorize Phase 1 on a
// figure nobody checked.
func TestModelCapacityProjectsPerStageEnvelopes(t *testing.T) {
	corpus := CorpusCounts{MetricOccurrences: 7074}
	stages := StageCoverage{
		RequiredStages:    requiredMetricStages,
		MetricOccurrences: 7074,
		OutcomeEnvelopes:  7074 * int64(len(requiredMetricStages)),
	}
	m := modelCapacity(corpus, stages)

	if m.OutcomeEnvelopes != 21222 {
		t.Errorf("outcome envelopes = %d, want 21222 (7074 x 3 required stages)", m.OutcomeEnvelopes)
	}
	// One evidence link, one class decision, and at most one assertion per
	// occurrence before canonical convergence (DR5).
	for name, got := range map[string]int64{
		"evidence links":  m.EvidenceLinks,
		"class decisions": m.ClassDecisions,
		"assertions":      m.AssertionsUpperBound,
	} {
		if got != 7074 {
			t.Errorf("%s = %d, want 7074 (one per occurrence)", name, got)
		}
	}
	if m.FindingsHighEstimate != 21222 {
		t.Errorf("high finding estimate = %d, want one per occurrence per stage", m.FindingsHighEstimate)
	}
	if m.EstimatedBytes <= 0 {
		t.Error("estimated bytes should be positive")
	}
}

func TestModelClassFoundationCapacityProjectsOneProfilePerClassCandidate(t *testing.T) {
	got := modelClassFoundationCapacity(CorpusCounts{MetricOccurrences: 7074, Assertions: 71})

	if got.ProvisionalClassCandidates != 7074 {
		t.Fatalf("provisional class candidates = %d, want 7074", got.ProvisionalClassCandidates)
	}
	if got.ClaimIdentitiesUpperBound != 7074 {
		t.Fatalf("claim identities upper bound = %d, want 7074", got.ClaimIdentitiesUpperBound)
	}
	if got.ObservedProfilesUpperBound != 7074 || got.ObservedProfileObservations != 7074 {
		t.Fatalf("profile projection = %+v, want one profile and observation per candidate", got)
	}
	if got.LegacyAssertions != 71 {
		t.Fatalf("legacy assertions = %d, want 71", got.LegacyAssertions)
	}
	if got.EstimatedBytes <= 0 {
		t.Fatalf("estimated bytes = %d, want positive", got.EstimatedBytes)
	}
}

// The low finding estimate must come from the MEASURED mapping distribution,
// not from a constant: it is what tells a reviewer how many of today's metrics
// are guaranteed to carry a finding after cutover.
func TestSetFindingsLowEstimateCountsOnlyProblemMappings(t *testing.T) {
	b := Baseline{MappingStates: []MappingStateCount{
		{MappingState: "approved", Metrics: 4276},
		{MappingState: "absent", Metrics: 1366},
		{MappingState: "unmapped", Metrics: 803},
		{MappingState: "ambiguous", Metrics: 629},
	}}
	b.SetFindingsLowEstimate()
	if b.Capacity.FindingsLowEstimate != 1432 {
		t.Fatalf("low estimate = %d, want 1432 (803 unmapped + 629 ambiguous)", b.Capacity.FindingsLowEstimate)
	}
}

func TestSetFindingsLowEstimateIncludesProposed(t *testing.T) {
	// A raw value already auto-inserted as 'proposed' is just as unresolved as
	// one with no row at all; both fail the "approved mapping" test.
	b := Baseline{MappingStates: []MappingStateCount{
		{MappingState: "proposed", Metrics: 10},
		{MappingState: "approved", Metrics: 90},
	}}
	b.SetFindingsLowEstimate()
	if b.Capacity.FindingsLowEstimate != 10 {
		t.Fatalf("low estimate = %d, want 10", b.Capacity.FindingsLowEstimate)
	}
}

func TestRenderMarkdownReportsTheLosslessnessGap(t *testing.T) {
	b := Baseline{
		Corpus: CorpusCounts{
			InputRecords: 209, RecordsWithMetrics: 58, MetricOccurrences: 7074,
			Assertions: 253, ActiveEvidenceLinks: 71, MetricSupportLinks: 71,
			UnreachableMetrics: 7020, DuplicateMetricSupport: 17,
		},
		MappingStates: []MappingStateCount{{MappingState: "unmapped", Metrics: 803, DistinctRaw: 211}},
		StageCoverage: StageCoverage{RequiredStages: requiredMetricStages, MetricOccurrences: 7074, OutcomeEnvelopes: 21222},
	}
	out := RenderMarkdown(b, time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC))

	for _, want := range []string{
		"Losslessness gap: 99.24%",
		"| **Metric occurrences with no current supporting link** | **7020** |",
		"| Metric occurrences with duplicate current support | 17 |",
		"7074 × 3 = **21222 envelopes**",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q", want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:        "512 B",
		1024:       "1.0 KiB",
		38_378_712: "36.6 MiB",
		2 << 30:    "2.0 GiB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
