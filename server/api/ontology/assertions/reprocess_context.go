package assertions

import "context"

// forceReprocessKey carries the doc-processor "Force Run" intent into the
// Phase D stages (normalize_assertions / associate_semantics) without a
// signature change to NormalizeAllFamilies, AssociateSemantics.Run, or
// DecisionCandidateStore.Propose -- the same context-threaded-flag pattern
// docprocessing.withDocProcessorFlags already uses for the chunk-batch
// lifecycle. phase_d.go translates the event-level force flag into this.
type forceReprocessKey struct{}

// WithForceReprocess marks ctx so the Phase D stages redo their work for a
// record even when up-to-date outputs already exist: Propose creates a fresh
// candidate revision instead of reusing a decided one, and Run re-normalizes
// before adjudicating. Absent (the default for the backlog drain, telemetry
// helper, and tests), the stages keep their idempotent skip-if-current
// behavior.
func WithForceReprocess(ctx context.Context, force bool) context.Context {
	return context.WithValue(ctx, forceReprocessKey{}, force)
}

// forceReprocess reports whether ctx was marked by WithForceReprocess.
func forceReprocess(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(forceReprocessKey{}).(bool)
	return v
}
