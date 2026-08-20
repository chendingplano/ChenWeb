package semantic

import "testing"

// Task 7.4/7.7: all three writer gates default on. The fallback path only
// ever adds a durable unresolved-occurrence record for an artifact that
// otherwise gets no durable record at all, and task 7.6 already certified
// the provision writer's conformance and completeness against real corpus
// data -- the same "safe to leave on" reasoning task 6.9 established for the
// metric gate applies to provision once certified.
func TestWriterGatesDefaultOn(t *testing.T) {
	g := NewGatesFromMap(nil)
	if !g.MetricLosslessWritesEnabled() {
		t.Error("LOSSLESS_SEMANTIC_WRITES_METRIC must default on")
	}
	if !g.FallbackWritesEnabled() {
		t.Error("LOSSLESS_SEMANTIC_FALLBACK_WRITES must default on")
	}
	if !g.ProvisionLosslessWritesEnabled() {
		t.Error("LOSSLESS_SEMANTIC_WRITES_PROVISION must default on")
	}
	if !g.FallbackAllowedFor("metric") {
		t.Error("fallback should be allowed by default when no deny switch is set")
	}
}

func TestWriterGatesExplicitlyEnabled(t *testing.T) {
	g := NewGatesFromMap(map[string]string{
		GateMetricLosslessWrites: "true",
		GateFallbackWrites:       "1",
	})
	if !g.MetricLosslessWritesEnabled() {
		t.Error("metric gate should be enabled")
	}
	if !g.FallbackAllowedFor("provision") {
		t.Error("fallback should be allowed when the global gate is on and no deny switch is set")
	}
}

func TestWriterGatesExplicitlyDisabled(t *testing.T) {
	g := NewGatesFromMap(map[string]string{
		GateMetricLosslessWrites: "false",
		GateFallbackWrites:       "false",
	})
	if g.MetricLosslessWritesEnabled() {
		t.Error("metric gate must be off when explicitly set to false")
	}
	if g.FallbackWritesEnabled() {
		t.Error("fallback gate must be off when explicitly set to false")
	}
	if g.FallbackAllowedFor("provision") {
		t.Error("fallback must not be allowed for any family when the global gate is explicitly off")
	}
}

// A typo in a gate value must not switch a writer on.
func TestUnparseableGateValueIsOff(t *testing.T) {
	g := NewGatesFromMap(map[string]string{GateMetricLosslessWrites: "yes-please"})
	if g.MetricLosslessWritesEnabled() {
		t.Error("an unparseable gate value must be treated as off")
	}
}

// Phase 4 item 4: the per-family deny switch is emergency isolation. It can
// only subtract, so no family can be enabled by a per-family variable while the
// global gate is off.
func TestPerFamilyDenySwitchOnlySubtracts(t *testing.T) {
	denied := NewGatesFromMap(map[string]string{
		GateFallbackWrites:                   "true",
		GateFallbackDenyPrefix + "PROVISION": "true",
	})
	if denied.FallbackAllowedFor("provision") {
		t.Error("denied family must be isolated even when the global gate is on")
	}
	if !denied.FallbackAllowedFor("metric") {
		t.Error("denying one family must not disable the others")
	}

	onlyFamilyVar := NewGatesFromMap(map[string]string{
		GateFallbackWrites:                   "false",
		GateFallbackDenyPrefix + "PROVISION": "false",
	})
	if onlyFamilyVar.FallbackAllowedFor("provision") {
		t.Error("a per-family variable must never enable a family while the global gate is explicitly off")
	}
}
