package docbenchmark

import (
	"context"
	"fmt"
	"strings"
)

// LoadRoutingClearanceEvidence reloads immutable selected benchmark results.
// Document-kind membership is explicit through a document_kind:<value> tag.
func (s SQLStore) LoadRoutingClearanceEvidence(ctx context.Context, baselineRunID, routedRunID, documentKind string) (RoutingClearanceEvidence, error) {
	baseline, err := s.GetRun(ctx, baselineRunID)
	if err != nil {
		return RoutingClearanceEvidence{}, err
	}
	routed, err := s.GetRun(ctx, routedRunID)
	if err != nil {
		return RoutingClearanceEvidence{}, err
	}
	if baseline.Lifecycle != "succeeded" || routed.Lifecycle != "succeeded" {
		return RoutingClearanceEvidence{}, fmt.Errorf("benchmark runs must be terminal succeeded")
	}
	if baseline.ExperimentID != routed.ExperimentID {
		return RoutingClearanceEvidence{}, fmt.Errorf("benchmark runs belong to different experiments")
	}
	var manifest string
	if err := s.DB.QueryRowContext(ctx, "SELECT dataset_hash FROM kb.benchmark_experiments WHERE id=$1", baseline.ExperimentID).Scan(&manifest); err != nil {
		return RoutingClearanceEvidence{}, err
	}
	baselineUnits, baselineFailures, err := s.reportUnits(ctx, baselineRunID)
	if err != nil {
		return RoutingClearanceEvidence{}, err
	}
	routedUnits, routedFailures, err := s.reportUnits(ctx, routedRunID)
	if err != nil {
		return RoutingClearanceEvidence{}, err
	}
	evidence := RoutingClearanceEvidence{DocumentKind: strings.TrimSpace(documentKind)}
	evidence.Baseline = buildRoutingRunEvidence(baselineRunID, manifest, baselineUnits, baselineFailures, documentKind)
	evidence.Routed = buildRoutingRunEvidence(routedRunID, manifest, routedUnits, routedFailures, documentKind)
	return evidence, nil
}

func buildRoutingRunEvidence(runID, manifest string, units []ScoreUnit, failures map[string]int, documentKind string) RoutingClearanceRunEvidence {
	out := RoutingClearanceRunEvidence{RunID: runID, ManifestChecksum: manifest}
	repetitions := map[int]struct{}{}
	for _, unit := range units {
		if !hasDocumentKindTag(unit.Tags, documentKind) {
			continue
		}
		repetitions[unit.Repetition] = struct{}{}
		row := RoutingClearanceCaseEvidence{CaseID: unit.CaseID, Repetition: unit.Repetition}
		found := false
		for _, metricName := range []string{"review_recall", "grounding_recall", "detection_recall", "boundary_recall"} {
			for _, score := range unit.Scores {
				if score.Metric == metricName && score.Denominator > 0 {
					row.GoldPositiveDenominator = score.Denominator
					row.RecallNumerator = score.Numerator
					row.RecallDenominator = score.Denominator
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			row.ScorerFailures = 1
		}
		out.Cases = append(out.Cases, row)
	}
	out.Repetitions = len(repetitions)
	if count := totalFailureCount(failures); count > 0 {
		if len(out.Cases) == 0 {
			out.Cases = append(out.Cases, RoutingClearanceCaseEvidence{CaseID: "__run__"})
		}
		out.Cases[0].InfrastructureFailures += count
	}
	return out
}

func hasDocumentKindTag(tags []string, documentKind string) bool {
	want := strings.ToLower(strings.TrimSpace(documentKind))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "document_kind:"+want || tag == "doc_kind:"+want {
			return true
		}
	}
	return false
}

func totalFailureCount(failures map[string]int) int {
	total := 0
	for _, count := range failures {
		total += count
	}
	return total
}
