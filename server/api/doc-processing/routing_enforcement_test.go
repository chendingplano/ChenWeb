package docprocessing

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

// stubClearanceChecker returns a canned result for exact-match subjects and
// records every lookup it receives, so tests can assert partial-clearance
// behavior (only the exact expected subject is ever cleared).
type stubClearanceChecker struct {
	cleared map[string]bool // key: subject_kind/subject_id/subject_checksum/document_kind/net_plan_delta_checksum
	errFor  map[string]error
	seen    []RoutingClearanceSubject
}

func subjectKey(s RoutingClearanceSubject) string {
	return fmt.Sprintf("%s/%d/%s/%s/%s", s.SubjectKind, s.SubjectID, s.SubjectChecksum, s.DocumentKind, s.NetPlanDeltaChecksum)
}

func (c *stubClearanceChecker) ResolveEffective(_ context.Context, subject RoutingClearanceSubject) (EffectiveRoutingClearance, error) {
	c.seen = append(c.seen, subject)
	key := subjectKey(subject)
	if c.errFor != nil {
		if err, ok := c.errFor[key]; ok {
			return EffectiveRoutingClearance{}, err
		}
	}
	if c.cleared != nil && c.cleared[key] {
		return EffectiveRoutingClearance{ClearanceID: 1}, nil
	}
	return EffectiveRoutingClearance{}, ErrNoEffectiveRoutingClearance
}

func gateShadowFor(decisions ...ProcessorGateResolution) ProcessorGateShadowPlan {
	return ProcessorGateShadowPlan{Decisions: decisions}
}

func baseEnforcementRequest() RoutingEnforcementRequest {
	return RoutingEnforcementRequest{
		Mode:                DocPipelineModeEnforced,
		PolicyID:            7,
		PolicyVersion:       3,
		DocumentKind:        "standard",
		RequestedProcessors: []string{"static_analyzer", "chunking", "extract_metrics", "extract_provisions"},
		BindingSource:       "store_default",
		SelectedSpec:        ProductionPipelineSpec{Name: "legacy_default"},
		BaselineSpec:        ProductionPipelineSpec{Name: "legacy_default"},
		GateShadow:          gateShadowFor(),
	}
}

func TestFinalizeRoutingPlan_ExplicitBypassNeverExcludesAnything(t *testing.T) {
	req := baseEnforcementRequest()
	req.Explicit = true
	req.BindingSource = "conditional_binding"
	req.BindingID = 42
	req.SelectedSpec = ProductionPipelineSpec{Name: "narrow", Processors: []string{"extract_metrics"}}
	req.GateShadow = gateShadowFor(ProcessorGateResolution{Processor: "extract_metrics", Effect: GateEffectSkip, Source: "policy_gate", WinningRuleID: 5})

	got, err := FinalizeRoutingPlan(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	wantEffective := []string{"static_analyzer", "chunking", "extract_metrics", "extract_provisions"}
	if !sameSet(got.EffectiveProcessors, wantEffective) {
		t.Fatalf("effective=%v want=%v (explicit bypass must not exclude anything)", got.EffectiveProcessors, wantEffective)
	}
	if len(got.ExcludedByPipeline) != 0 || len(got.ExcludedByGate) != 0 {
		t.Fatalf("excluded pipeline=%v gate=%v, want none", got.ExcludedByPipeline, got.ExcludedByGate)
	}
	if len(got.Alarms) != 0 {
		t.Fatalf("alarms=%v, want none for explicit bypass", got.Alarms)
	}
}

func TestFinalizeRoutingPlan_ExplicitAndRunOverrideGateDecisionsBypassClearance(t *testing.T) {
	req := baseEnforcementRequest()
	req.GateShadow = gateShadowFor(ProcessorGateResolution{Processor: "extract_provisions", Effect: GateEffectSkip, Source: "run_override"})

	got, err := FinalizeRoutingPlan(context.Background(), req, nil) // nil clearance store
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if !contains(got.ExcludedByGate, "extract_provisions") {
		t.Fatalf("excluded by gate=%v, want extract_provisions excluded (run override needs no clearance)", got.ExcludedByGate)
	}
	if contains(got.EffectiveProcessors, "extract_provisions") {
		t.Fatalf("effective=%v, extract_provisions must not run", got.EffectiveProcessors)
	}
	if len(got.Alarms) != 0 {
		t.Fatalf("alarms=%v, want none for a run override", got.Alarms)
	}
}

func TestFinalizeRoutingPlan_ConditionalBindingSelectsPipelineWhenCleared(t *testing.T) {
	req := baseEnforcementRequest()
	req.BindingSource = "conditional_binding"
	req.BindingID = 42
	req.BindingName = "standards-metrics"
	req.BindingPredicateChecksum = "sha256:binding"
	req.SelectedSpec = ProductionPipelineSpec{Name: "narrow", Processors: []string{"extract_metrics"}}
	req.BaselineSpec = ProductionPipelineSpec{Name: "legacy_default"} // empty Processors -- allows everything

	baselineEffective, _ := applyPolicyFilter(req.RequestedProcessors, DocPipelineModeEnforced, req.BaselineSpec)
	selectedEffective, _ := applyPolicyFilter(req.RequestedProcessors, DocPipelineModeEnforced, req.SelectedSpec)
	_, deltaChecksum, err := BuildNetPlanDelta(baselineEffective, selectedEffective)
	if err != nil {
		t.Fatal(err)
	}
	subjectChecksum := ConditionalBindingSubjectChecksum(req.PolicyVersion, PipelineBinding{PipelineName: "narrow", PredicateChecksum: "sha256:binding"}, req.SelectedSpec, req.BaselineSpec)
	stub := &stubClearanceChecker{cleared: map[string]bool{
		subjectKey(RoutingClearanceSubject{PolicyID: 7, PolicyVersion: 3, SubjectKind: "conditional_binding", SubjectID: 42, SubjectChecksum: subjectChecksum, DocumentKind: "standard", NetPlanDeltaChecksum: deltaChecksum}): true,
	}}

	got, err := FinalizeRoutingPlan(context.Background(), req, stub)
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if got.Pipeline.Name != "narrow" || got.UsedFallbackPipeline {
		t.Fatalf("pipeline=%+v fallback=%v, want the cleared conditional binding's pipeline enforced", got.Pipeline, got.UsedFallbackPipeline)
	}
	if !contains(got.ExcludedByPipeline, "extract_provisions") {
		t.Fatalf("excluded by pipeline=%v, want extract_provisions excluded by the narrow pipeline's allowlist", got.ExcludedByPipeline)
	}
	for _, alarm := range got.Alarms {
		if alarm.Kind == RoutingAlarmKindFallbackWarning {
			t.Fatalf("alarms=%v, want no fallback warning when cleared", got.Alarms)
		}
	}
}

func TestFinalizeRoutingPlan_ConditionalBindingFallsBackToBaselineWhenUncleared(t *testing.T) {
	req := baseEnforcementRequest()
	req.BindingSource = "conditional_binding"
	req.BindingID = 42
	req.BindingName = "standards-metrics"
	req.BindingPredicateChecksum = "sha256:binding"
	req.SelectedSpec = ProductionPipelineSpec{Name: "narrow", Processors: []string{"extract_metrics"}}
	req.BaselineSpec = ProductionPipelineSpec{Name: "legacy_default"}

	stub := &stubClearanceChecker{} // no clearances at all -> ErrNoEffectiveRoutingClearance

	got, err := FinalizeRoutingPlan(context.Background(), req, stub)
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if got.Pipeline.Name != "legacy_default" || !got.UsedFallbackPipeline {
		t.Fatalf("pipeline=%+v fallback=%v, want fallback to baseline pipeline", got.Pipeline, got.UsedFallbackPipeline)
	}
	if contains(got.ExcludedByPipeline, "extract_provisions") {
		t.Fatalf("excluded by pipeline=%v, want nothing excluded once falling back to the unrestricted baseline", got.ExcludedByPipeline)
	}
	if !contains(got.ShadowPipelineExclusions, "extract_provisions") {
		t.Fatalf("shadow pipeline exclusions=%v, want extract_provisions recorded shadow-only", got.ShadowPipelineExclusions)
	}
	if !contains(got.EffectiveProcessors, "extract_provisions") {
		t.Fatalf("effective=%v, extract_provisions must still run (uncleared suppression stays shadow)", got.EffectiveProcessors)
	}
	found := false
	for _, alarm := range got.Alarms {
		if alarm.Kind == RoutingAlarmKindFallbackWarning {
			found = true
		}
	}
	if !found {
		t.Fatalf("alarms=%v, want a fallback_warning alarm", got.Alarms)
	}
}

func TestFinalizeRoutingPlan_IncomparablePipelineIsSuppressiveAndGatedByClearance(t *testing.T) {
	req := baseEnforcementRequest()
	req.RequestedProcessors = []string{"static_analyzer", "chunking", "extract_metrics", "extract_provisions", "extract_products"}
	req.BindingSource = "conditional_binding"
	req.BindingID = 9
	req.BindingPredicateChecksum = "sha256:binding"
	// Incomparable: removes extract_metrics/extract_provisions, adds extract_products.
	req.BaselineSpec = ProductionPipelineSpec{Name: "legacy_default", Processors: []string{"static_analyzer", "chunking", "extract_metrics", "extract_provisions"}}
	req.SelectedSpec = ProductionPipelineSpec{Name: "products", Processors: []string{"static_analyzer", "chunking", "extract_products"}}

	stub := &stubClearanceChecker{}
	got, err := FinalizeRoutingPlan(context.Background(), req, stub)
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if !got.UsedFallbackPipeline {
		t.Fatalf("got=%+v, want the incomparable (removes+adds) pipeline treated as suppressive and gated by clearance", got)
	}
	if len(stub.seen) != 1 || stub.seen[0].SubjectKind != "conditional_binding" {
		t.Fatalf("clearance lookups=%v, want exactly one conditional_binding lookup", stub.seen)
	}
}

func TestFinalizeRoutingPlan_ProcessorGateExcludesWhenCleared(t *testing.T) {
	req := baseEnforcementRequest()
	req.GateShadow = gateShadowFor(ProcessorGateResolution{
		Processor: "extract_provisions", Effect: GateEffectSkip, Source: "policy_gate",
		WinningRuleID: 5, WinningChecksum: "sha256:gate",
	})
	gate := PipelineGate{TargetProcessor: "extract_provisions", Effect: GateEffectSkip, PredicateChecksum: "sha256:gate"}
	subjectChecksum := ProcessorGateSubjectChecksum(req.PolicyVersion, gate)
	_, gateDelta, err := BuildNetPlanDelta(req.RequestedProcessors, removeProcessorName(req.RequestedProcessors, "extract_provisions"))
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubClearanceChecker{cleared: map[string]bool{
		subjectKey(RoutingClearanceSubject{PolicyID: 7, PolicyVersion: 3, SubjectKind: "processor_rule", SubjectID: 5, SubjectChecksum: subjectChecksum, DocumentKind: "standard", NetPlanDeltaChecksum: gateDelta}): true,
	}}

	got, err := FinalizeRoutingPlan(context.Background(), req, stub)
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if !contains(got.ExcludedByGate, "extract_provisions") {
		t.Fatalf("excluded by gate=%v, want extract_provisions excluded once cleared", got.ExcludedByGate)
	}
	if contains(got.EffectiveProcessors, "extract_provisions") {
		t.Fatalf("effective=%v, extract_provisions must not run", got.EffectiveProcessors)
	}
	if len(got.Alarms) != 0 {
		t.Fatalf("alarms=%v, want none once cleared", got.Alarms)
	}
}

func TestFinalizeRoutingPlan_ProcessorGateStaysShadowWhenUncleared(t *testing.T) {
	req := baseEnforcementRequest()
	req.GateShadow = gateShadowFor(ProcessorGateResolution{
		Processor: "extract_provisions", Effect: GateEffectSkip, Source: "policy_gate",
		WinningRuleID: 5, WinningChecksum: "sha256:gate",
	})
	stub := &stubClearanceChecker{}

	got, err := FinalizeRoutingPlan(context.Background(), req, stub)
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if !contains(got.EffectiveProcessors, "extract_provisions") {
		t.Fatalf("effective=%v, want extract_provisions to keep running (uncleared suppression stays shadow)", got.EffectiveProcessors)
	}
	if !contains(got.ShadowGateExclusions, "extract_provisions") {
		t.Fatalf("shadow gate exclusions=%v, want extract_provisions recorded shadow-only", got.ShadowGateExclusions)
	}
	found := false
	for _, alarm := range got.Alarms {
		if alarm.Kind == RoutingAlarmKindFallbackWarning {
			found = true
		}
	}
	if !found {
		t.Fatalf("alarms=%v, want a fallback_warning alarm", got.Alarms)
	}
}

func TestFinalizeRoutingPlan_PartialClearanceOnlyCoversMatchingSubject(t *testing.T) {
	req := baseEnforcementRequest()
	req.RequestedProcessors = []string{"static_analyzer", "chunking", "extract_metrics", "extract_provisions"}
	req.GateShadow = gateShadowFor(
		ProcessorGateResolution{Processor: "extract_metrics", Effect: GateEffectSkip, Source: "policy_gate", WinningRuleID: 5, WinningChecksum: "sha256:gate-a"},
		ProcessorGateResolution{Processor: "extract_provisions", Effect: GateEffectSkip, Source: "policy_gate", WinningRuleID: 6, WinningChecksum: "sha256:gate-b"},
	)
	metricsGate := PipelineGate{TargetProcessor: "extract_metrics", Effect: GateEffectSkip, PredicateChecksum: "sha256:gate-a"}
	metricsChecksum := ProcessorGateSubjectChecksum(req.PolicyVersion, metricsGate)
	_, metricsDelta, err := BuildNetPlanDelta(req.RequestedProcessors, removeProcessorName(req.RequestedProcessors, "extract_metrics"))
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubClearanceChecker{cleared: map[string]bool{
		subjectKey(RoutingClearanceSubject{PolicyID: 7, PolicyVersion: 3, SubjectKind: "processor_rule", SubjectID: 5, SubjectChecksum: metricsChecksum, DocumentKind: "standard", NetPlanDeltaChecksum: metricsDelta}): true,
	}}

	got, err := FinalizeRoutingPlan(context.Background(), req, stub)
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if !contains(got.ExcludedByGate, "extract_metrics") {
		t.Fatalf("excluded by gate=%v, want extract_metrics excluded (it has a matching clearance)", got.ExcludedByGate)
	}
	if !contains(got.EffectiveProcessors, "extract_provisions") {
		t.Fatalf("effective=%v, want extract_provisions still running (no clearance covers it)", got.EffectiveProcessors)
	}
	if !contains(got.ShadowGateExclusions, "extract_provisions") {
		t.Fatalf("shadow gate exclusions=%v, want extract_provisions recorded shadow-only", got.ShadowGateExclusions)
	}
}

func TestFinalizeRoutingPlan_PolicyIntegrityFailureFailsClosedAndAlarms(t *testing.T) {
	req := baseEnforcementRequest()
	req.GateShadow = gateShadowFor(ProcessorGateResolution{Processor: "extract_provisions", Effect: GateEffectSkip, Source: "policy_gate", WinningRuleID: 5, WinningChecksum: "sha256:gate"})

	got, err := FinalizeRoutingPlan(context.Background(), req, multipleAlwaysClearanceChecker{})
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if !contains(got.EffectiveProcessors, "extract_provisions") {
		t.Fatalf("effective=%v, want extract_provisions to keep running when clearance lookup fails closed", got.EffectiveProcessors)
	}
	found := false
	for _, alarm := range got.Alarms {
		if alarm.Kind == RoutingAlarmKindPolicyIntegrity {
			found = true
		}
	}
	if !found {
		t.Fatalf("alarms=%v, want a policy_integrity_failure alarm", got.Alarms)
	}
}

type multipleAlwaysClearanceChecker struct{}

func (multipleAlwaysClearanceChecker) ResolveEffective(context.Context, RoutingClearanceSubject) (EffectiveRoutingClearance, error) {
	return EffectiveRoutingClearance{}, ErrMultipleEffectiveRoutingClearances
}

type erroringClearanceChecker struct{}

func (erroringClearanceChecker) ResolveEffective(context.Context, RoutingClearanceSubject) (EffectiveRoutingClearance, error) {
	return EffectiveRoutingClearance{}, errors.New("connection refused")
}

func TestFinalizeRoutingPlan_OperatorFailureFailsClosedAndAlarms(t *testing.T) {
	req := baseEnforcementRequest()
	req.GateShadow = gateShadowFor(ProcessorGateResolution{Processor: "extract_provisions", Effect: GateEffectSkip, Source: "policy_gate", WinningRuleID: 5, WinningChecksum: "sha256:gate"})

	got, err := FinalizeRoutingPlan(context.Background(), req, erroringClearanceChecker{})
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if !contains(got.EffectiveProcessors, "extract_provisions") {
		t.Fatalf("effective=%v, want extract_provisions to keep running when clearance lookup errors", got.EffectiveProcessors)
	}
	found := false
	for _, alarm := range got.Alarms {
		if alarm.Kind == RoutingAlarmKindOperatorFailure {
			found = true
		}
	}
	if !found {
		t.Fatalf("alarms=%v, want an operator_failure alarm", got.Alarms)
	}
}

func TestFinalizeRoutingPlan_IndeterminateGateStaysShadowAndAlarms(t *testing.T) {
	req := baseEnforcementRequest()
	req.GateShadow = gateShadowFor(ProcessorGateResolution{Processor: "extract_provisions", Effect: GateEffectSkip, Source: "indeterminate_fallback"})

	got, err := FinalizeRoutingPlan(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if !contains(got.EffectiveProcessors, "extract_provisions") {
		t.Fatalf("effective=%v, want extract_provisions to keep running (indeterminate is never enforced)", got.EffectiveProcessors)
	}
	found := false
	for _, alarm := range got.Alarms {
		if alarm.Kind == RoutingAlarmKindGateConflict {
			found = true
		}
	}
	if !found {
		t.Fatalf("alarms=%v, want a gate_conflict alarm", got.Alarms)
	}
}

func TestFinalizeRoutingPlan_MandatoryProcessorsAlwaysRestored(t *testing.T) {
	req := baseEnforcementRequest()
	// Defense-in-depth: even a fabricated skip decision targeting a
	// mandatory processor must never remove it from the effective set.
	req.GateShadow = gateShadowFor(ProcessorGateResolution{Processor: "chunking", Effect: GateEffectSkip, Source: "policy_gate", WinningRuleID: 99, WinningChecksum: "sha256:bad"})

	// Even a clearance store that clears everything must not remove a
	// mandatory processor: restoration is unconditional.
	got, err := FinalizeRoutingPlan(context.Background(), req, alwaysClearedChecker{})
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if !contains(got.EffectiveProcessors, "chunking") {
		t.Fatalf("effective=%v, want chunking always present", got.EffectiveProcessors)
	}
}

type alwaysClearedChecker struct{}

func (alwaysClearedChecker) ResolveEffective(context.Context, RoutingClearanceSubject) (EffectiveRoutingClearance, error) {
	return EffectiveRoutingClearance{ClearanceID: 1}, nil
}

func TestFinalizeRoutingPlan_PlanOnlyModeNeverExcludesAnything(t *testing.T) {
	req := baseEnforcementRequest()
	req.Mode = DocPipelineModePlanOnly
	req.BindingSource = "conditional_binding"
	req.BindingID = 42
	req.BindingPredicateChecksum = "sha256:binding"
	req.SelectedSpec = ProductionPipelineSpec{Name: "narrow", Processors: []string{"extract_metrics"}}
	req.GateShadow = gateShadowFor(ProcessorGateResolution{Processor: "extract_metrics", Effect: GateEffectSkip, Source: "policy_gate", WinningRuleID: 5, WinningChecksum: "sha256:gate"})

	got, err := FinalizeRoutingPlan(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("FinalizeRoutingPlan: %v", err)
	}
	if !sameSet(got.EffectiveProcessors, req.RequestedProcessors) {
		t.Fatalf("effective=%v want=%v (plan-only mode never excludes)", got.EffectiveProcessors, req.RequestedProcessors)
	}
	if len(got.ExcludedByPipeline) != 0 || len(got.ExcludedByGate) != 0 {
		t.Fatalf("excluded pipeline=%v gate=%v, want none in plan-only mode", got.ExcludedByPipeline, got.ExcludedByGate)
	}
}

func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func sameSet(got, want []string) bool {
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	return reflect.DeepEqual(g, w)
}

// TestRestoreMandatoryProcessorsDerivedFromRegistry proves the mandatory
// processor safety net is derived from isMandatoryProcessor over the
// production registry, not a hardcoded pair: static_analyzer/chunking are
// restored (their names make isMandatoryProcessor true regardless of class),
// a mandatory-class processor (facet_tier1) is restored too, a name absent
// from the registry is not, and classify_document -- Class "routed", not
// mandatory, specifically so an authored gate can skip it -- is not restored
// either (P5 review 2026080302 finding P5-29).
func TestRestoreMandatoryProcessorsDerivedFromRegistry(t *testing.T) {
	got := restoreMandatoryProcessors(
		[]string{"extract_metrics"},
		[]string{"extract_metrics", "static_analyzer", "chunking", "facet_tier1", "classify_document", "not_a_processor"},
	)
	set := stringSet(got)
	for _, want := range []string{"extract_metrics", "static_analyzer", "chunking", "facet_tier1"} {
		if !set[want] {
			t.Fatalf("result %v missing mandatory processor %q", got, want)
		}
	}
	if set["not_a_processor"] {
		t.Fatalf("result %v must not restore a name absent from the registry", got)
	}
	if set["classify_document"] {
		t.Fatalf("result %v must not restore classify_document -- it is routed, not mandatory", got)
	}
}
