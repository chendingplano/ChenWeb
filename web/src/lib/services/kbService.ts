const BASE = '/api/v1/kb';

export type ParseState = 'all' | 'pending' | 'parsed_success' | 'parsed_failed';

export type KbInputRecord = {
	id: number;
	name?: string;
	type: string;
	title?: string;
	doc_no?: string;
	source?: string;
	file_name?: string;
	backup_filename?: string;
	result_filename?: string;
	publish_date?: string;
	authors?: string;
	owner?: number;
	status: Array<{
		operation?: string;
		time?: string;
		start_time?: string;
		status?: string;
		proc_status?: string;
		'proc-status'?: string;
		error?: string;
	}>;
	create_time: string;
	modify_time: string;
	public_info?: unknown;
	private_info?: unknown;
	notes?: string;
	error_msg?: string;
};

export type ListKbInputsParams = {
	docType: string;
	parseState: ParseState;
	fileName: string;
	startTime: string;
	endTime: string;
	page: number;
	pageSize: number;
};

export type ListKbInputsResponse = {
	status: boolean;
	results: KbInputRecord[];
	page: number;
	page_size: number;
	total: number;
};

function buildQuery(params: ListKbInputsParams): string {
	const query = new URLSearchParams();
	query.set('doc_type', params.docType);
	query.set('parse_state', params.parseState);
	if (params.fileName.trim()) query.set('file_name', params.fileName.trim());
	if (params.startTime.trim()) query.set('start_time', params.startTime.trim());
	if (params.endTime.trim()) query.set('end_time', params.endTime.trim());
	query.set('page', String(params.page));
	query.set('page_size', String(params.pageSize));
	return query.toString();
}

export async function listKbInputs(params: ListKbInputsParams): Promise<ListKbInputsResponse> {
	const response = await fetch(`${BASE}/inputs?${buildQuery(params)}`, {
		method: 'GET',
		credentials: 'same-origin'
	});
	if (!response.ok) {
		const payload = await response.json().catch(() => null);
		const msg =
			payload && typeof payload.error_msg === 'string'
				? payload.error_msg
				: `Failed to list kb inputs (${response.status})`;
		throw new Error(msg);
	}
	return response.json();
}

// ---------- kb.metrics / raw-lines / single input ----------

export type SourceLineSpan = { page_number: number; line_number: number };

export type KbMetricRecord = {
	id: number;
	input_record_id: number;
	extract_id: string;
	input_filename: string;
	metric_name?: string;
	source_line_spans?: SourceLineSpan[];
	metric_subject?: string;
	metric_desc?: string;
	metric_context?: string;
	metric_keywords?: string[];
	location_type?: string;
	metric_unit?: string;
	formula_or_definition?: string;
	threshold_or_target?: string;
	measurement_frequency?: string;
	confidence?: number;
	is_explicit_metric?: boolean;
	reasoning_tags?: string[];
	created_at?: string;
};

export type ListKbMetricsResponse = {
	status: boolean;
	results: KbMetricRecord[];
	total: number;
};

async function fetchOrThrow<T>(url: string, fallback: string): Promise<T> {
	const response = await fetch(url, { method: 'GET', credentials: 'same-origin' });
	if (!response.ok) {
		const payload = await response.json().catch(() => null);
		const msg =
			payload && typeof payload.error_msg === 'string'
				? payload.error_msg
				: `${fallback} (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<T>;
}

export async function listKbMetrics(inputRecordId: number): Promise<ListKbMetricsResponse> {
	return fetchOrThrow<ListKbMetricsResponse>(
		`${BASE}/metrics?input_record_id=${encodeURIComponent(String(inputRecordId))}`,
		'Failed to list kb metrics'
	);
}

export type GetKbInputResponse = {
	status: boolean;
	record: KbInputRecord;
};

export async function getKbInput(id: number): Promise<GetKbInputResponse> {
	return fetchOrThrow<GetKbInputResponse>(
		`${BASE}/inputs/${encodeURIComponent(String(id))}`,
		'Failed to retrieve kb input'
	);
}

export type RawLine = {
	line_number: number;
	page_number: number;
	line_type: string;
	content: string;
	coords: number[];
};

export type GetRawLinesResponse = {
	status: boolean;
	input_id: number;
	file_name?: string;
	lines: RawLine[];
	pages: number;
};

export async function getRawLines(inputRecordId: number): Promise<GetRawLinesResponse> {
	return fetchOrThrow<GetRawLinesResponse>(
		`${BASE}/raw-lines?input_record_id=${encodeURIComponent(String(inputRecordId))}`,
		'Failed to retrieve raw lines'
	);
}
