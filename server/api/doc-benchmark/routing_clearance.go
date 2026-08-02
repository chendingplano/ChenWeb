package docbenchmark

import (
	"fmt"
	"sort"
	"strings"
)

type RoutingClearanceEvidence struct {
	DocumentKind     string                      `json:"document_kind"`
	PolicyChecksum   string                      `json:"policy_checksum"`
	BindingChecksums []string                    `json:"binding_checksums,omitempty"`
	GateChecksums    []string                    `json:"gate_checksums,omitempty"`
	Baseline         RoutingClearanceRunEvidence `json:"baseline"`
	Routed           RoutingClearanceRunEvidence `json:"routed"`
	CostYield        map[string]float64          `json:"cost_yield,omitempty"`
	DecisionTraces   []string                    `json:"decision_traces,omitempty"`
}

type RoutingClearanceRunEvidence struct {
	RunID            string                         `json:"run_id"`
	ManifestChecksum string                         `json:"manifest_checksum"`
	Repetitions      int                            `json:"repetitions"`
	Cases            []RoutingClearanceCaseEvidence `json:"cases"`
}

type RoutingClearanceCaseEvidence struct {
	CaseID                  string `json:"case_id"`
	Repetition              int    `json:"repetition"`
	GoldPositiveDenominator int    `json:"gold_positive_denominator"`
	RecallNumerator         int    `json:"recall_numerator"`
	RecallDenominator       int    `json:"recall_denominator"`
	ProcessorFailures       int    `json:"processor_failures,omitempty"`
	InfrastructureFailures  int    `json:"infrastructure_failures,omitempty"`
	ScorerFailures          int    `json:"scorer_failures,omitempty"`
}

type RoutingClearanceDecision struct {
	Approved               bool               `json:"approved"`
	Reason                 string             `json:"reason"`
	DocumentKind           string             `json:"document_kind"`
	CasePairs              int                `json:"case_pairs"`
	GoldPositiveDenom      int                `json:"gold_positive_denominator"`
	BaselineRecall         *float64           `json:"baseline_recall,omitempty"`
	RoutedRecall           *float64           `json:"routed_recall,omitempty"`
	PolicyChecksum         string             `json:"policy_checksum"`
	BindingChecksums       []string           `json:"binding_checksums,omitempty"`
	GateChecksums          []string           `json:"gate_checksums,omitempty"`
	CostYield              map[string]float64 `json:"cost_yield,omitempty"`
	DecisionTraces         []string           `json:"decision_traces,omitempty"`
	ProcessorFailures      int                `json:"processor_failures,omitempty"`
	InfrastructureFailures int                `json:"infrastructure_failures,omitempty"`
	ScorerFailures         int                `json:"scorer_failures,omitempty"`
}

func EvaluateRoutingClearance(e RoutingClearanceEvidence) RoutingClearanceDecision {
	decision := RoutingClearanceDecision{
		DocumentKind:     strings.TrimSpace(e.DocumentKind),
		PolicyChecksum:   strings.TrimSpace(e.PolicyChecksum),
		BindingChecksums: append([]string(nil), e.BindingChecksums...),
		GateChecksums:    append([]string(nil), e.GateChecksums...),
		CostYield:        cloneFloatMap(e.CostYield),
		DecisionTraces:   append([]string(nil), e.DecisionTraces...),
	}
	if strings.TrimSpace(e.Baseline.ManifestChecksum) == "" || strings.TrimSpace(e.Baseline.ManifestChecksum) != strings.TrimSpace(e.Routed.ManifestChecksum) {
		decision.Reason = "manifest_mismatch"
		return decision
	}
	if e.Baseline.Repetitions <= 0 || e.Baseline.Repetitions != e.Routed.Repetitions {
		decision.Reason = "repetition_mismatch"
		return decision
	}

	baselineCases := routingCaseMap(e.Baseline.Cases)
	routedCases := routingCaseMap(e.Routed.Cases)
	if len(baselineCases) != len(routedCases) {
		decision.Reason = "incomplete_paired_cases"
		return decision
	}
	if len(baselineCases) < 3 {
		decision.CasePairs = len(baselineCases)
		decision.Reason = "minimum_three_cases"
		return decision
	}

	var baselineNum, baselineDen, routedNum, routedDen int
	for key, baseline := range baselineCases {
		routed, ok := routedCases[key]
		if !ok {
			decision.Reason = "incomplete_paired_cases"
			return decision
		}
		decision.CasePairs++
		decision.GoldPositiveDenom += baseline.GoldPositiveDenominator
		baselineNum += baseline.RecallNumerator
		baselineDen += baseline.RecallDenominator
		routedNum += routed.RecallNumerator
		routedDen += routed.RecallDenominator
		decision.ProcessorFailures += baseline.ProcessorFailures + routed.ProcessorFailures
		decision.InfrastructureFailures += baseline.InfrastructureFailures + routed.InfrastructureFailures
		decision.ScorerFailures += baseline.ScorerFailures + routed.ScorerFailures
	}
	if decision.GoldPositiveDenom <= 0 || baselineDen <= 0 || routedDen <= 0 {
		decision.Reason = "no_gold_positive_denominator"
		return decision
	}
	if decision.ProcessorFailures > 0 {
		decision.Reason = "processor_failures"
		return decision
	}
	if decision.InfrastructureFailures > 0 {
		decision.Reason = "infrastructure_failures"
		return decision
	}
	if decision.ScorerFailures > 0 {
		decision.Reason = "scorer_failures"
		return decision
	}
	baselineRecall := float64(baselineNum) / float64(baselineDen)
	routedRecall := float64(routedNum) / float64(routedDen)
	decision.BaselineRecall = &baselineRecall
	decision.RoutedRecall = &routedRecall
	if routedRecall < baselineRecall {
		decision.Reason = "routed_recall_below_baseline"
		return decision
	}
	decision.Approved = true
	decision.Reason = "approved"
	return decision
}

func routingCaseMap(cases []RoutingClearanceCaseEvidence) map[string]RoutingClearanceCaseEvidence {
	out := make(map[string]RoutingClearanceCaseEvidence, len(cases))
	for _, c := range cases {
		out[fmt.Sprintf("%s/%d", strings.TrimSpace(c.CaseID), c.Repetition)] = c
	}
	return out
}

func cloneFloatMap(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]float64, len(in))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}
