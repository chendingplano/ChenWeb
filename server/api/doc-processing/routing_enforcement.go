package docprocessing

import (
	"context"
	"errors"
	"fmt"
)

// RoutingClearanceChecker resolves whether a specific suppressive routing
// decision has an effective D2 benchmark clearance. RoutingClearanceStore
// satisfies this in production; tests inject a stub.
type RoutingClearanceChecker interface {
	ResolveEffective(ctx context.Context, subject RoutingClearanceSubject) (EffectiveRoutingClearance, error)
}

// RoutingEnforcementRequest bundles the pure decisions E1 (processor-gate
// shadow) and C1 (pipeline-binding selection) already computed, plus the
// policy identity D2 needs to look up clearance. FinalizeRoutingPlan is the
// single boundary that turns those decisions -- plus clearance lookups --
// into the real effective processor set, without disturbing the shadow
// explanation already persisted in P5RoutingSnapshot.
type RoutingEnforcementRequest struct {
	Mode string // DocPipelineModePlanOnly | DocPipelineModeEnforced
	// Explicit is true for the evt.Operations bypass path: nothing is ever
	// suppressed, matching "explicit-request and named-pipeline precedence
	// remains unchanged" (spec 2026080102 section 5.1).
	Explicit bool
	// PipelineName/PipelineVersion identify the resolved kb.pipelines
	// version D2 clearance lookups key on (ADR 2026081001 DR3) -- replaces
	// the retired system-wide PolicyID/PolicyVersion.
	PipelineName    string
	PipelineVersion int
	// DocumentKind is the clearance coverage slice key.
	DocumentKind string

	// RequestedProcessors is the pre-gate baseline -- the same input
	// BuildProductionProcessorPlanFromFacts' applyPolicyFilter received.
	RequestedProcessors []string

	// Binding* mirror ProductionPipelineBindingResolution's fields for the
	// pure pipeline-selection decision already made.
	BindingSource            string
	BindingID                int64
	BindingName              string
	BindingPredicateChecksum string

	SelectedSpec ProductionPipelineSpec // the pipeline BindingSource actually selected
	BaselineSpec ProductionPipelineSpec // DefaultProductionPipelineName -- the store-default/baseline fallback pipeline

	GateShadow ProcessorGateShadowPlan // BuildProcessorGateShadowPlan's pure decision
}

type RoutingEnforcementResult struct {
	Pipeline             ProductionPipelineSpec
	UsedFallbackPipeline bool

	EffectiveProcessors []string
	ExcludedByPipeline  []string // enforced pipeline-allowlist exclusions
	ExcludedByGate      []string // enforced gate skip/defer exclusions

	ShadowPipelineExclusions []string // suppressive binding decision that stayed shadow-only (uncleared)
	ShadowGateExclusions     []string // suppressive gate decision that stayed shadow-only (uncleared)

	Alarms []RoutingAlarm
}

// ExcludedProcessorNames is everything FinalizeRoutingPlan actually removed
// from the effective set -- what control.go's applyPlanEnforcement drops
// from the runtime processor list.
func (r RoutingEnforcementResult) ExcludedProcessorNames() []string {
	return append(append([]string(nil), r.ExcludedByPipeline...), r.ExcludedByGate...)
}

// FinalizeRoutingPlan applies DR7's enforcement order -- explicit bypass,
// explicit/run override (already resolved inside GateShadow by
// ResolveProcessorGate), conditional binding evaluation, conditional-binding
// clearance or store-default fallback, pipeline allowlist, processor gate
// evaluation, gate clearance, mandatory restoration, final effective set --
// to produce the real effective processor set. It never mutates plan-only
// mode's execution and never enforces a suppressive decision without an
// exact D2 clearance match; an uncleared suppressive decision stays
// shadow-only and raises a fallback-warning alarm instead.
//
// Gate decisions are evaluated per-processor independent of which other
// processors are present in the baseline (ResolveProcessorGate only
// consults the one processor's spec/gates/facts), so reusing the
// precomputed GateShadow after a binding-level allowlist fallback remains
// valid: a decision for a processor no longer present after the pipeline
// allowlist step is simply not applied.
func FinalizeRoutingPlan(ctx context.Context, req RoutingEnforcementRequest, clearances RoutingClearanceChecker) (RoutingEnforcementResult, error) {
	result := RoutingEnforcementResult{Pipeline: req.SelectedSpec}
	enforced := req.Mode == DocPipelineModeEnforced && !req.Explicit

	// --- conditional binding evaluation + conditional-binding clearance or
	// store-default fallback ---
	pipeline := req.SelectedSpec
	if req.BindingSource == "conditional_binding" {
		// Suppressiveness is "baseline_effective_processors -
		// selected_effective_processors" (spec 2026080102 section 9) --
		// each pipeline's allowlist applied to the actual request, not the
		// pipeline spec's raw Processors field (which is empty for an
		// unrestricted pipeline, not "runs nothing").
		baselineEffective, _ := applyPolicyFilter(req.RequestedProcessors, DocPipelineModeEnforced, req.BaselineSpec)
		selectedEffective, _ := applyPolicyFilter(req.RequestedProcessors, DocPipelineModeEnforced, req.SelectedSpec)
		delta, deltaChecksum, err := BuildNetPlanDelta(baselineEffective, selectedEffective)
		if err != nil {
			return RoutingEnforcementResult{}, fmt.Errorf("compute binding net plan delta: %w", err)
		}
		if delta.Suppressive {
			if !enforced {
				result.ShadowPipelineExclusions = delta.RemovedProcessors
			} else {
				subject := RoutingClearanceSubject{
					PipelineName: req.PipelineName, PipelineVersion: req.PipelineVersion,
					SubjectKind: "conditional_binding", SubjectID: req.BindingID,
					SubjectChecksum: ConditionalBindingSubjectChecksum(req.PipelineVersion, PipelineBinding{
						PipelineName: req.SelectedSpec.Name, PredicateChecksum: req.BindingPredicateChecksum,
					}, req.SelectedSpec, req.BaselineSpec),
					DocumentKind: req.DocumentKind, NetPlanDeltaChecksum: deltaChecksum,
				}
				cleared, alarm := checkRoutingClearance(ctx, clearances, subject)
				if alarm != nil {
					result.Alarms = append(result.Alarms, *alarm)
				}
				if !cleared {
					pipeline = req.BaselineSpec
					result.UsedFallbackPipeline = true
					result.ShadowPipelineExclusions = delta.RemovedProcessors
					result.Alarms = append(result.Alarms, RoutingAlarm{
						Kind: RoutingAlarmKindFallbackWarning, Severity: RoutingAlarmSeverityWarning,
						Message: fmt.Sprintf("conditional binding %q selected pipeline %q removes %v vs baseline %q without an effective clearance; enforced execution falls back to %q",
							req.BindingName, req.SelectedSpec.Name, delta.RemovedProcessors, req.BaselineSpec.Name, req.BaselineSpec.Name),
					})
				}
			}
		}
	}
	result.Pipeline = pipeline

	// --- pipeline allowlist ---
	filterMode := req.Mode
	if !enforced {
		filterMode = DocPipelineModePlanOnly
	}
	effective, excludedByPipeline := applyPolicyFilter(req.RequestedProcessors, filterMode, pipeline)
	if enforced {
		result.ExcludedByPipeline = excludedByPipeline
	}

	// --- processor gate evaluation + gate clearance ---
	kept := make([]string, 0, len(effective))
	for _, processor := range effective {
		decision, ok := gateDecisionFor(req.GateShadow, processor)
		if !ok || decision.Effect != GateEffectSkip {
			kept = append(kept, processor)
			continue
		}
		switch decision.Source {
		case "run_override", "explicit_request", "mandatory":
			// Explicit/run overrides retain precedence and never require a
			// P5 clearance (spec section 9).
			if enforced {
				result.ExcludedByGate = append(result.ExcludedByGate, processor)
				continue
			}
			kept = append(kept, processor)
		case "policy_gate":
			if !enforced {
				result.ShadowGateExclusions = append(result.ShadowGateExclusions, processor)
				kept = append(kept, processor)
				continue
			}
			gateBaseline := effective
			gateSelected := removeProcessorName(effective, processor)
			_, gateDeltaChecksum, err := BuildNetPlanDelta(gateBaseline, gateSelected)
			if err != nil {
				return RoutingEnforcementResult{}, fmt.Errorf("compute gate net plan delta for %s: %w", processor, err)
			}
			gate := PipelineGate{TargetProcessor: decision.Processor, Effect: decision.Effect, PredicateChecksum: decision.WinningChecksum}
			subject := RoutingClearanceSubject{
				PipelineName: req.PipelineName, PipelineVersion: req.PipelineVersion,
				SubjectKind: "processor_rule", SubjectID: decision.WinningRuleID,
				SubjectChecksum: ProcessorGateSubjectChecksum(req.PipelineVersion, gate),
				DocumentKind:    req.DocumentKind, NetPlanDeltaChecksum: gateDeltaChecksum,
			}
			cleared, alarm := checkRoutingClearance(ctx, clearances, subject)
			if alarm != nil {
				result.Alarms = append(result.Alarms, *alarm)
			}
			if cleared {
				result.ExcludedByGate = append(result.ExcludedByGate, processor)
				continue
			}
			result.ShadowGateExclusions = append(result.ShadowGateExclusions, processor)
			result.Alarms = append(result.Alarms, RoutingAlarm{
				Kind: RoutingAlarmKindFallbackWarning, Severity: RoutingAlarmSeverityWarning,
				Message: fmt.Sprintf("processor gate rule %d (%s %s) has no effective clearance; %s stays shadow-only and continues to run",
					decision.WinningRuleID, decision.Effect, processor, processor),
			})
			kept = append(kept, processor)
		case "indeterminate_fallback":
			// A genuine policy ambiguity resolved via DOC_PIPELINE_ON_CONFLICT
			// fallback: never enforced, always surfaced.
			result.ShadowGateExclusions = append(result.ShadowGateExclusions, processor)
			result.Alarms = append(result.Alarms, RoutingAlarm{
				Kind: RoutingAlarmKindGateConflict, Severity: RoutingAlarmSeverityWarning,
				Message: fmt.Sprintf("processor gate for %s is indeterminate; fallback effect %s stays shadow-only", processor, decision.Effect),
			})
			kept = append(kept, processor)
		default:
			kept = append(kept, processor)
		}
	}

	// --- mandatory restoration ---
	result.EffectiveProcessors = restoreMandatoryProcessors(kept, req.RequestedProcessors)
	return result, nil
}

func gateDecisionFor(shadow ProcessorGateShadowPlan, processor string) (ProcessorGateResolution, bool) {
	norm := normalizeRuntimeName(processor)
	for _, decision := range shadow.Decisions {
		if decision.Processor == norm {
			return decision, true
		}
	}
	return ProcessorGateResolution{}, false
}

func checkRoutingClearance(ctx context.Context, clearances RoutingClearanceChecker, subject RoutingClearanceSubject) (bool, *RoutingAlarm) {
	if clearances == nil {
		return false, nil
	}
	if _, err := clearances.ResolveEffective(ctx, subject); err != nil {
		switch {
		case errors.Is(err, ErrNoEffectiveRoutingClearance):
			return false, nil
		case errors.Is(err, ErrMultipleEffectiveRoutingClearances):
			return false, &RoutingAlarm{
				Kind: RoutingAlarmKindPolicyIntegrity, Severity: RoutingAlarmSeverityError,
				Message: fmt.Sprintf("multiple effective routing clearances for %s/%d (pipeline %s v%d); failing closed to shadow",
					subject.SubjectKind, subject.SubjectID, subject.PipelineName, subject.PipelineVersion),
			}
		default:
			return false, &RoutingAlarm{
				Kind: RoutingAlarmKindOperatorFailure, Severity: RoutingAlarmSeverityError,
				Message: fmt.Sprintf("routing clearance lookup failed for %s/%d: %s; failing closed to shadow",
					subject.SubjectKind, subject.SubjectID, err.Error()),
			}
		}
	}
	return true, nil
}

func removeProcessorName(list []string, target string) []string {
	norm := normalizeRuntimeName(target)
	out := make([]string, 0, len(list))
	for _, name := range list {
		if normalizeRuntimeName(name) == norm {
			continue
		}
		out = append(out, name)
	}
	return out
}

// restoreMandatoryProcessors is a defense-in-depth safety net: mandatory
// processors should never be excluded by construction (isMandatoryProcessor
// forces Effect=require in ResolveProcessorGate, and applyPolicyFilter exempts
// them from the pipeline allowlist), but this guarantees it explicitly rather
// than relying on those invariants alone. The mandatory set is derived from
// isMandatoryProcessor over the production registry rather than a hardcoded
// pair, so a newly declared mandatory-class processor can never silently fall
// through the safety net (P5 review 2026080302 finding P5-29).
func restoreMandatoryProcessors(kept, requested []string) []string {
	keptSet := stringSet(kept)
	out := append([]string(nil), kept...)
	for _, name := range requested {
		norm := normalizeRuntimeName(name)
		if keptSet[norm] {
			continue
		}
		if spec := findProductionProcessorSpec(norm); spec != nil && isMandatoryProcessor(*spec) {
			out = append(out, name)
			keptSet[norm] = true
		}
	}
	return out
}

// findProductionProcessorSpec returns the registry spec for a normalized
// processor name, or nil when the name is not a declared production processor.
func findProductionProcessorSpec(name string) *ProcessorSpec {
	for i := range productionProcessorSpecs {
		if productionProcessorSpecs[i].Name == normalizeRuntimeName(name) {
			return &productionProcessorSpecs[i]
		}
	}
	return nil
}
