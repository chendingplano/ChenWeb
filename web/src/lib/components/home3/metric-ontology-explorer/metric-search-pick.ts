// The Search tab deep-links a metric into the explorer by its canonical
// kb.metrics.metric_id — "<record_id>_mtc_<seqno>" (a legacy "<record_id>_<seqno>"
// form also exists). That id is what GET /api/v1/kb/metrics/:metric_id/wiki and the
// source pane's resolver expect. A search hit whose metric_id is blank or oddly
// shaped cannot be opened here, so its result card renders disabled.

const CANONICAL_METRIC_ID = /^\d+(?:_mtc)?_\d+$/;

/**
 * Returns the canonical metric_id to deep-link to, or null when the metric
 * (a kb.metrics row or a search hit) cannot be opened in the explorer.
 */
export function pickableMetricId(
	metric: { metric_id?: string | null } | null | undefined
): string | null {
	const id = (metric?.metric_id ?? '').trim();
	return CANONICAL_METRIC_ID.test(id) ? id : null;
}
