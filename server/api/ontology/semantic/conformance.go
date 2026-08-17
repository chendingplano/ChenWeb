package semantic

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// ConformanceResult is the outcome of running the shared conformance suite
// against one family adapter.
type ConformanceResult struct {
	ArtifactType string
	SuiteVersion string
	Passed       bool
	Failures     []string
}

// RunConformanceSuite checks an adapter's declaration against the invariants
// every family must satisfy (DR13). It is deliberately declaration-level: it
// verifies that what the adapter PROMISES is expressible and governed, which
// is the precondition for the completeness projection and the writer gate to
// mean anything.
//
// It does not exercise the family's writer -- that is the family's own
// integration tests' job. The distinction matters because DR13 makes suite
// passage a gate on activation, so the suite must be runnable at startup
// against a database with no artifacts in it.
func RunConformanceSuite(a FamilyAdapter) ConformanceResult {
	res := ConformanceResult{ArtifactType: a.ArtifactType(), SuiteVersion: ConformanceSuiteVersion}
	fail := func(format string, args ...any) {
		res.Failures = append(res.Failures, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(a.ArtifactType()) == "" {
		fail("adapter declares an empty artifact type")
	}
	if strings.TrimSpace(a.AdapterName()) == "" || strings.TrimSpace(a.AdapterVersion()) == "" {
		fail("adapter name and version are required for the compliance registry")
	}
	if strings.TrimSpace(a.OccurrenceScope()) == "" {
		fail("adapter declares no occurrence scope: OccurrenceKey would not be deterministic per family (DR13)")
	}
	if len(a.RawIdentityFields()) == 0 {
		fail("adapter declares no raw identity fields: DR2 requires the raw occurrence identity to be explicit")
	}

	stages := a.RequiredStages()
	if len(stages) == 0 {
		fail("adapter declares no required stages: the completeness projection would accept any artifact as complete (DR13)")
	}
	seenStage := map[string]bool{}
	for _, st := range stages {
		if err := ValidateGovernedIdentifier(st.StageTermID); err != nil {
			fail("required stage: %v", err)
			continue
		}
		if seenStage[st.StageTermID] {
			fail("required stage %s is declared twice: outcome cardinality would be ambiguous", st.StageTermID)
		}
		seenStage[st.StageTermID] = true
		if len(st.DecisionScopes) == 0 {
			fail("stage %s declares no decision scopes: finding keys could not be deterministic within it (DR4)", st.StageTermID)
		}
		for _, d := range st.Dimensions {
			if err := ValidateGovernedIdentifier(d); err != nil {
				fail("stage %s dimension: %v", st.StageTermID, err)
			}
		}
		for _, disp := range st.AllowedDispositions {
			if err := ValidateGovernedIdentifier(disp); err != nil {
				fail("stage %s disposition: %v", st.StageTermID, err)
			}
		}
		if len(st.AllowedDispositions) == 0 {
			fail("stage %s declares no disposition contract (DR13)", st.StageTermID)
		}
	}

	// DR6's minimum contract: a family that models instances must be able to
	// express every value axis it uses, and those axes must be governed.
	if len(a.ValueStates()) == 0 {
		fail("adapter declares no value states: DR6's minimum field/state contract cannot be satisfied")
	}
	for _, v := range a.ValueStates() {
		if err := ValidateGovernedIdentifier(v); err != nil {
			fail("value state: %v", err)
		}
	}
	if len(a.ConformanceStates()) == 0 {
		fail("adapter declares no conformance states (DR6/DR9)")
	}
	for _, c := range a.ConformanceStates() {
		if err := ValidateGovernedIdentifier(c); err != nil {
			fail("conformance state: %v", err)
		}
	}
	if len(a.DependencyAxes()) == 0 {
		fail("adapter declares no dependency axes: retry could never target this family (DR10)")
	}

	res.Passed = len(res.Failures) == 0
	return res
}

// VerifyAndRecord runs the suite and records the result in the runtime
// compliance registry, returning the result.
func VerifyAndRecord(ctx context.Context, db *sql.DB, a FamilyAdapter, mode WriterMode) (ConformanceResult, error) {
	res := RunConformanceSuite(a)
	details, err := json.Marshal(map[string]any{"failures": res.Failures})
	if err != nil {
		return res, err
	}
	result := "failed"
	if res.Passed {
		result = "passed"
	}
	// A failing adapter is never recorded in an active writer mode: the
	// database CHECK would reject 'lossless' without a pass anyway, and
	// silently downgrading to 'legacy' here makes the refusal explicit rather
	// than a constraint violation at an unrelated call site.
	recordedMode := mode
	if !res.Passed && (mode == WriterLossless || mode == WriterFallback) {
		recordedMode = WriterLegacy
	}
	err = ComplianceRegistry{DB: db}.Upsert(ctx, ComplianceRecord{
		ArtifactType:            a.ArtifactType(),
		AdapterName:             a.AdapterName(),
		AdapterVersion:          a.AdapterVersion(),
		WriterMode:              recordedMode,
		ConformanceSuiteVersion: ConformanceSuiteVersion,
		LastVerifiedResult:      result,
		Details:                 details,
	})
	return res, err
}

// ActivationError explains why a writer activation was refused.
type ActivationError struct {
	ArtifactType string
	Reason       string
}

func (e *ActivationError) Error() string {
	return fmt.Sprintf("lossless writer activation refused for %q: %s", e.ArtifactType, e.Reason)
}

// AuthorizeWriterActivation implements DR13's normative refusal: "Activation is
// refused when the registered adapter has not passed the current suite."
//
// The current-suite check is the substantive one. A stale pass against an older
// suite version is treated as no pass at all, because the suite version is
// exactly what changes when a new required invariant is added.
func AuthorizeWriterActivation(ctx context.Context, db *sql.DB, artifactType string, gateEnabled bool) error {
	if !gateEnabled {
		return &ActivationError{ArtifactType: artifactType, Reason: "writer gate is disabled"}
	}
	adapter, ok := LookupAdapter(artifactType)
	if !ok {
		return &ActivationError{ArtifactType: artifactType,
			Reason: "no adapter is registered; the family must use the generic unresolved-occurrence fallback (DR13)"}
	}
	rec, err := ComplianceRegistry{DB: db}.Get(ctx, artifactType)
	switch {
	case err == sql.ErrNoRows:
		return &ActivationError{ArtifactType: artifactType, Reason: "no compliance record exists"}
	case err != nil:
		return err
	}
	if rec.LastVerifiedResult != "passed" {
		return &ActivationError{ArtifactType: artifactType,
			Reason: fmt.Sprintf("last conformance result is %q", rec.LastVerifiedResult)}
	}
	if rec.ConformanceSuiteVersion != ConformanceSuiteVersion {
		return &ActivationError{ArtifactType: artifactType,
			Reason: fmt.Sprintf("last pass was against suite %s but the current suite is %s",
				rec.ConformanceSuiteVersion, ConformanceSuiteVersion)}
	}
	if rec.AdapterVersion != adapter.AdapterVersion() {
		return &ActivationError{ArtifactType: artifactType,
			Reason: fmt.Sprintf("registered adapter version %s does not match the running adapter %s",
				rec.AdapterVersion, adapter.AdapterVersion())}
	}
	return nil
}
