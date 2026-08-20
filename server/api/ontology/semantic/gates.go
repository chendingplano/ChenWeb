package semantic

import (
	"os"
	"strconv"
	"strings"
)

// Named writer gates from ADR 2026081801 Phase 3 and Phase 4. Both the
// metric lossless-writes gate and the fallback-writes gate default ON (task
// 7.4): the fallback path only ever adds a durable unresolved-occurrence
// record for an artifact that today gets no durable record at all, and the
// per-family deny switch below stays available to isolate one family in an
// emergency without touching this default.
const (
	GateMetricLosslessWrites = "LOSSLESS_SEMANTIC_WRITES_METRIC"
	GateFallbackWrites       = "LOSSLESS_SEMANTIC_FALLBACK_WRITES"
	// GateFallbackDenyPrefix builds the per-family emergency deny switch
	// required by Phase 4 item 4, e.g.
	// LOSSLESS_SEMANTIC_FALLBACK_DENY_PROVISION=1.
	GateFallbackDenyPrefix = "LOSSLESS_SEMANTIC_FALLBACK_DENY_"
	// GateProvisionLosslessWrites gates task 7.6's provision instance writer.
	// It defaults OFF, mirroring GateMetricLosslessWrites' own Phase 1-2
	// staging: the adapter and writer are certified and ready, but a live
	// completeness/readiness check (metric's task 6.9 equivalent) has not
	// been run for provisions yet, so activation stays a deliberate later
	// decision, not a side effect of this task.
	GateProvisionLosslessWrites = "LOSSLESS_SEMANTIC_WRITES_PROVISION"
)

// Gates reads the named writer gates. The indirection exists so tests can
// supply values without mutating process environment, and so every gate read
// goes through one place that documents each gate's default.
type Gates struct {
	lookup func(string) (string, bool)
}

// NewGates reads gates from the process environment.
func NewGates() Gates { return Gates{lookup: os.LookupEnv} }

// NewGatesFromMap reads gates from an explicit map (tests).
func NewGatesFromMap(m map[string]string) Gates {
	return Gates{lookup: func(k string) (string, bool) { v, ok := m[k]; return v, ok }}
}

func (g Gates) enabled(name string) bool {
	if g.lookup == nil {
		return false
	}
	raw, ok := g.lookup(name)
	if !ok {
		return name == GateMetricLosslessWrites || name == GateFallbackWrites
	}
	// Anything unparseable is treated as OFF. A typo in a gate value must not
	// switch a writer on.
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	return err == nil && v
}

// MetricLosslessWritesEnabled reports whether LOSSLESS_SEMANTIC_WRITES_METRIC
// is on. It is only one of the activation preconditions: DR13's conformance
// and Phase 2 reader certification are checked by AuthorizeWriterActivation.
func (g Gates) MetricLosslessWritesEnabled() bool { return g.enabled(GateMetricLosslessWrites) }

// FallbackWritesEnabled reports whether the global generic-fallback gate is on.
func (g Gates) FallbackWritesEnabled() bool { return g.enabled(GateFallbackWrites) }

// ProvisionLosslessWritesEnabled reports whether
// LOSSLESS_SEMANTIC_WRITES_PROVISION is on. Like
// MetricLosslessWritesEnabled, this is only one of the activation
// preconditions; AuthorizeWriterActivation checks conformance and compliance
// registry state.
func (g Gates) ProvisionLosslessWritesEnabled() bool { return g.enabled(GateProvisionLosslessWrites) }

// FallbackAllowedFor applies the global gate and the per-family deny switch.
// The deny switch is an emergency isolation control: it can only subtract, so
// no family can be enabled by a per-family variable while the global gate is
// off.
func (g Gates) FallbackAllowedFor(artifactType string) bool {
	if !g.FallbackWritesEnabled() {
		return false
	}
	return !g.enabled(GateFallbackDenyPrefix + strings.ToUpper(artifactType))
}
