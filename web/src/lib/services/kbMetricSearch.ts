const BASE = '/api/v1/kb/metrics/search';

export type KbMetricSearchParams = {
	q: string;
	page?: number;
	pageSize?: number;
	inputRecordId?: number;
	isExplicitMetric?: boolean;
	valueClass?: string;
	valueDataType?: string;
	metricUnit?: string;
};

export type KbMetricSearchFilters = {
	input_record_id?: number;
	is_explicit_metric?: boolean;
	value_class?: string;
	value_data_type?: string;
	metric_unit?: string;
};

export type KbMetricSearchResult = {
	id: number;
	metric_id?: string;
	input_record_id: number;
	input_filename?: string;
	metric_name?: string;
	metric_name_en?: string;
	metric_subject?: string;
	metric_subject_en?: string;
	metric_value?: string;
	metric_unit?: string;
	metric_unit_en?: string;
	value_class?: string;
	value_class_en?: string;
	value_data_type?: string;
	is_explicit_metric?: boolean;
	table_name_or_section?: string;
	metric_keywords?: string[];
	metric_keywords_en?: string[];
	source_line_spans?: Array<string | number | { page_number: number; line_number: number }>;
	score: number;
	snippet: string;
	primary_label: string;
};

export type SearchKbMetricsResponse = {
	status: boolean;
	query: string;
	page: number;
	page_size: number;
	total: number;
	applied_filters: KbMetricSearchFilters;
	results: KbMetricSearchResult[];
};

function buildQuery(params: KbMetricSearchParams): string {
	const query = new URLSearchParams();
	query.set('q', params.q);
	if (params.page != null) query.set('page', String(params.page));
	if (params.pageSize != null) query.set('page_size', String(params.pageSize));
	if (params.inputRecordId != null) query.set('input_record_id', String(params.inputRecordId));
	if (params.isExplicitMetric != null) {
		query.set('is_explicit_metric', params.isExplicitMetric ? 'true' : 'false');
	}
	if (params.valueClass?.trim()) query.set('value_class', params.valueClass.trim());
	if (params.valueDataType?.trim()) query.set('value_data_type', params.valueDataType.trim());
	if (params.metricUnit?.trim()) query.set('metric_unit', params.metricUnit.trim());
	return query.toString();
}

export async function searchKbMetrics(params: KbMetricSearchParams): Promise<SearchKbMetricsResponse> {
	const response = await fetch(`${BASE}?${buildQuery(params)}`, {
		method: 'GET',
		credentials: 'same-origin'
	});
	if (!response.ok) {
		const payload = await response.json().catch(() => null);
		const msg =
			payload && typeof payload.error_msg === 'string'
				? payload.error_msg
				: `Failed to search kb metrics (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<SearchKbMetricsResponse>;
}
