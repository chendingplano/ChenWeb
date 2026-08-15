// API client + pure helpers for the "Resolve Metric Range Types" admin page
// (System Admin -> Database Maintenance). See
// openspec/changes/resolve-metric-range-types for the design this implements.

export type MetricRangeTypeError = {
	id: number;
	input_record_id: number;
	metric_id?: string;
	metric_name?: string;
	metric_desc?: string;
	metric_context?: string;
	metric_value?: string;
	value_data_type?: string;
	value_range_type?: string;
	value_range_type_error?: string;
	document_title?: string;
	document_doc_no?: string;
	source_line_spans?: unknown;
	created_at?: string;
};

export type ValueRangeTypeMapEntry = {
	raw_value: string;
	canonical_bucket?: string;
	status: string;
	occurrence_count: number;
	first_seen_record_id?: number;
	last_seen_record_id?: number;
	note?: string;
	create_time?: string;
	create_by?: string;
	modify_time?: string;
	modify_by?: string;
};

export type MetricRangeTypeErrorFilters = {
	input_record_id?: number;
	date_from?: string;
	date_to?: string;
	value_range_type?: string;
};

export type UpsertValueRangeTypeMapEntryPayload = {
	raw_value: string;
	canonical_bucket: string;
	note?: string;
};

/**
 * The only canonical_bucket values ever produced by the governed lookup
 * (assertions.CanonicalMetricValueRangeType / guessCanonicalBucketFromCues).
 * The Map Block's editor offers these as a dropdown but also accepts free
 * text -- kb.metric_value_range_type_map.canonical_bucket has no DB-level
 * enum constraint.
 */
export const CANONICAL_BUCKET_OPTIONS = ['lower_bound', 'upper_bound', 'exact', 'range'] as const;

/** An entry is invalid (needs triage) whenever its status isn't 'approved'. */
export function isInvalidMapEntry(entry: ValueRangeTypeMapEntry): boolean {
	return entry.status !== 'approved';
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, { credentials: 'same-origin', ...init });
	const text = await res.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
	}
	if (!res.ok) {
		const msg =
			parsed && typeof parsed === 'object' && parsed !== null && 'error_msg' in parsed
				? String((parsed as { error_msg: unknown }).error_msg)
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

/** Builds the query string for listMetricRangeTypeErrors' filters (empty string if none set). */
export function buildRangeTypeErrorsQuery(filters: MetricRangeTypeErrorFilters): string {
	const params = new URLSearchParams();
	if (filters.input_record_id != null) params.set('input_record_id', String(filters.input_record_id));
	if (filters.date_from) params.set('date_from', filters.date_from);
	if (filters.date_to) params.set('date_to', filters.date_to);
	if (filters.value_range_type) params.set('value_range_type', filters.value_range_type);
	return params.toString();
}

export function listMetricRangeTypeErrors(
	filters: MetricRangeTypeErrorFilters = {}
): Promise<{ status: boolean; results: MetricRangeTypeError[]; total: number }> {
	const qs = buildRangeTypeErrorsQuery(filters);
	return req(`/api/v1/kb/metrics/range-type-errors${qs ? `?${qs}` : ''}`);
}

export function listValueRangeTypeMapEntries(): Promise<{
	status: boolean;
	results: ValueRangeTypeMapEntry[];
	total: number;
}> {
	return req('/api/v1/kb/metric-value-range-type-map');
}

/**
 * Creates a brand-new kb.metric_value_range_type_map entry, or applies a
 * correction to an existing (invalid) one -- the same call either way (see
 * design D1). Always sets status='approved' server-side and cascades: every
 * kb.metrics row whose value_range_type matches raw_value has its
 * value_range_type_error cleared; corrected_count reports how many.
 */
export function upsertValueRangeTypeMapEntry(
	payload: UpsertValueRangeTypeMapEntryPayload
): Promise<{ status: boolean; entry: ValueRangeTypeMapEntry; corrected_count: number }> {
	return req('/api/v1/kb/metric-value-range-type-map', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
}
