// Read wrapper for the Metric Ontology Explorer.
//
// One call — GET /api/v1/kb/metrics/:metric_id/graph — returns the related rows
// for every chain node of the selected metric, keyed by the model.ts chain-node
// id. The record tabs read their slice of that payload; `PROJECT` shapes a raw
// row into the node's display columns (openspec metric-scoped-explorer D2).

export type Cell = string | number | boolean | null;

export type MetricGraphMetric = {
	metric_id: string;
	metric_name: string;
	metric_name_en: string;
	input_record_id: number;
};

export type MetricGraphRow = Record<string, unknown>;

export type MetricGraph = {
	metric: MetricGraphMetric;
	nodes: Record<string, { rows: MetricGraphRow[] }>;
};

async function getJson<T>(url: string, fallback: string): Promise<T> {
	const res = await fetch(url, { method: 'GET', credentials: 'same-origin' });
	if (!res.ok) {
		const payload = await res.json().catch(() => null);
		const msg =
			payload && typeof payload.error_msg === 'string'
				? payload.error_msg
				: `${fallback} (${res.status})`;
		throw new Error(msg);
	}
	return res.json() as Promise<T>;
}

export async function getMetricGraph(metricId: string): Promise<MetricGraph> {
	return getJson<MetricGraph>(
		`/api/v1/kb/metrics/${encodeURIComponent(metricId)}/graph`,
		'failed to load metric graph'
	);
}

// --- Analysis satellite: related-metrics cohorts (openspec analysis-node-related-metrics) ---

export type RelatedMetricsScope = 'same_class' | 'similar_class';

export type RelatedMetricRow = {
	metric_id: string;
	metric_name: string;
	metric_name_en: string;
	input_record_id: number;
	class_term_id: string;
	class_label: string;
	metric_value?: string;
	metric_unit?: string;
	source_filename?: string;
	score?: number; // similar_class only
	matched_class_term_id?: string; // similar_class only
};

export type RelatedMetrics = {
	status: boolean;
	metric_id: string;
	scope: RelatedMetricsScope;
	class_term_id: string | null; // null => selected metric has no resolved governed class
	results: RelatedMetricRow[];
};

/**
 * GET /api/v1/kb/metrics/:metric_id/related-metrics — the Analysis satellite's
 * two chain nodes. Lazily fetched on tab open; the caller caches per
 * (metricId, scope). `limit` defaults server-side (20 similar / 200 same).
 */
export async function getRelatedMetrics(
	metricId: string,
	scope: RelatedMetricsScope,
	limit?: number
): Promise<RelatedMetrics> {
	const q = new URLSearchParams({ scope });
	if (limit != null) q.set('limit', String(limit));
	return getJson<RelatedMetrics>(
		`/api/v1/kb/metrics/${encodeURIComponent(metricId)}/related-metrics?${q.toString()}`,
		'failed to load related metrics'
	);
}

const dash = (v: unknown): Cell =>
	v === null || v === undefined || v === '' ? '—' : (v as Cell);
const score4 = (v: unknown): Cell =>
	typeof v === 'number' && Number.isFinite(v) ? Number(v.toFixed(4)) : dash(v);
const clip = (v: unknown, n = 90): Cell => {
	const s = v == null ? '' : String(v);
	return s.length > n ? s.slice(0, n - 1) + '…' : dash(s);
};
const s = (r: MetricGraphRow, k: string): unknown => r[k];
const firstOf = (r: MetricGraphRow, ...keys: string[]): unknown => {
	for (const k of keys) {
		const v = r[k];
		if (v !== null && v !== undefined && v !== '') return v;
	}
	return null;
};

/** Shapes one graph row into the chain node's `columns` order (model.ts). */
export const PROJECT: Record<string, (r: MetricGraphRow) => Cell[]> = {
	object__mention: (r) => [
		dash(s(r, 'id')),
		dash(firstOf(r, 'object_name_en', 'object_name')),
		dash(s(r, 'object_id')),
		dash(s(r, 'reconcile_status'))
	],
	object__node: (r) => [
		dash(s(r, 'id')),
		dash(firstOf(r, 'canonical_name_en', 'canonical_name')),
		dash(s(r, 'object_type')),
		dash(s(r, 'object_id')),
		dash(s(r, 'reconcile_status'))
	],
	keyword__concept: (r) => [
		dash(s(r, 'concept_id')),
		dash(s(r, 'pref_label')),
		dash(s(r, 'status')),
		dash(s(r, 'scope')),
		clip(s(r, 'gloss'))
	],
	mdef__term: (r) => [
		dash(s(r, 'term_id')),
		dash(s(r, 'term_kind')),
		dash(s(r, 'module_id')),
		dash(s(r, 'status')),
		clip(s(r, 'definition'))
	],
	mdef__contract: (r) => [
		dash(s(r, 'id')),
		dash(s(r, 'term_id')),
		dash(s(r, 'revision')),
		dash(s(r, 'definition_state')),
		dash(s(r, 'effective_from'))
	],
	proc__extract: (r) => [
		dash(s(r, 'model_name')),
		s(r, 'is_explicit_metric') ? 'yes' : 'no',
		dash(s(r, 'confidence')),
		dash(s(r, 'location_type')),
		dash(s(r, 'created_at'))
	],
	proc__normalize: (r) => [
		dash(s(r, 'stage')),
		dash(s(r, 'disposition')),
		dash(s(r, 'execution_status')),
		dash(s(r, 'outcome_category')),
		dash(s(r, 'finding_count'))
	],
	proc__associate: (r) => [
		dash(s(r, 'stage')),
		dash(s(r, 'disposition')),
		dash(s(r, 'execution_status')),
		dash(s(r, 'outcome_category')),
		dash(s(r, 'finding_count'))
	],
	proc__project: (r) => [
		dash(s(r, 'assertion_id')),
		dash(s(r, 'status')),
		dash(s(r, 'value_state_term_id')),
		dash(s(r, 'conformance_state_term_id')),
		dash(s(r, 'normalized_against_contract_revision_id'))
	],
	ev__ae: (r) => [
		dash(s(r, 'id')),
		dash(s(r, 'assertion_id')),
		dash(s(r, 'input_record_id')),
		dash(s(r, 'artifact_object_id')),
		clip(s(r, 'evidence_quote'))
	],
	ev__dc: (r) => [
		dash(s(r, 'id')),
		dash(s(r, 'candidate_kind')),
		clip(s(r, 'logical_identity_key')),
		dash(s(r, 'status')),
		dash(s(r, 'resulting_assertion_id'))
	],
	ev__sa: (r) => [
		dash(s(r, 'id')),
		dash(s(r, 'subject')),
		dash(s(r, 'predicate_term_id')),
		dash(s(r, 'object')),
		dash(s(r, 'assertion_kind_term_id')),
		dash(s(r, 'confidence'))
	],
	// Analysis satellite — rows are RelatedMetricRow from getRelatedMetrics, not
	// the metric_graph payload (openspec analysis-node-related-metrics).
	analysis__same_class: (r) => [
		dash(s(r, 'metric_id')),
		dash(firstOf(r, 'metric_name_en', 'metric_name')),
		dash(s(r, 'metric_value')),
		dash(s(r, 'metric_unit')),
		dash(firstOf(r, 'source_filename', 'input_record_id'))
	],
	analysis__similar_class: (r) => [
		dash(s(r, 'metric_id')),
		dash(firstOf(r, 'metric_name_en', 'metric_name')),
		dash(firstOf(r, 'class_label', 'class_term_id')),
		dash(s(r, 'matched_class_term_id')),
		score4(s(r, 'score'))
	]
};
