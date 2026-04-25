const BASE = '/api/v1/kb';

export type ParseState = 'all' | 'pending' | 'parsed_success' | 'parsed_failed';

export type KbInputRecord = {
	id: number;
	name?: string;
	parser_name?: string;
	type: string;
	tenant_id?: string;
	ks_store_id?: number;
	title?: string;
	doc_no?: string;
	ks_desc?: string;
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
	doc_metadata?: unknown;
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
	recordId?: string;
	name?: string;
	title?: string;
	docNo?: string;
	parserName?: string;
	operation?: string;
	procStatus?: string;
	modifyStartTime?: string;
	modifyEndTime?: string;
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
	if (params.recordId?.trim()) query.set('record_id', params.recordId.trim());
	if (params.name?.trim()) query.set('name', params.name.trim());
	if (params.title?.trim()) query.set('title', params.title.trim());
	if (params.docNo?.trim()) query.set('doc_no', params.docNo.trim());
	if (params.fileName.trim()) query.set('file_name', params.fileName.trim());
	if (params.parserName?.trim()) query.set('parser_name', params.parserName.trim());
	if (params.operation?.trim()) query.set('operation', params.operation.trim());
	if (params.procStatus?.trim()) query.set('proc_status', params.procStatus.trim());
	if (params.startTime.trim()) query.set('start_time', params.startTime.trim());
	if (params.endTime.trim()) query.set('end_time', params.endTime.trim());
	if (params.modifyStartTime?.trim()) query.set('modify_start_time', params.modifyStartTime.trim());
	if (params.modifyEndTime?.trim()) query.set('modify_end_time', params.modifyEndTime.trim());
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

export type UpdateKbInputPayload = {
	tenant_id?: string | null;
	ks_store_id?: string | number | null;
	title?: string | null;
	doc_no?: string | null;
	ks_desc?: string | null;
	source?: string | null;
	publish_date?: string | null;
	authors?: string[] | string | null;
	owner?: string | number | null;
	public_info?: unknown;
	private_info?: unknown;
	doc_metadata?: unknown;
	notes?: string | null;
	error_msg?: string | null;
};

export async function updateKbInput(id: number, payload: UpdateKbInputPayload): Promise<GetKbInputResponse> {
	const response = await fetch(`${BASE}/inputs/${encodeURIComponent(String(id))}`, {
		method: 'PUT',
		credentials: 'same-origin',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to update kb input (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<GetKbInputResponse>;
}

export type UploadKbInputsPayload = {
	type: string;
	title?: string;
	doc_no?: string;
	authors?: string;
	public_info?: string;
	private_info?: string;
	notes?: string;
	ks_desc?: string;
	parser_name: 'paddleocr' | 'opendata' | 'mineru' | 'docling';
	ks_store_id: number;
	tenant_id: string;
	files: File[];
};

export type UploadKbInputsResponse = {
	status: boolean;
	count: number;
	ids: number[];
};

export async function uploadKbInputs(payload: UploadKbInputsPayload): Promise<UploadKbInputsResponse> {
	const form = new FormData();
	form.set('type', payload.type);
	form.set('parser_name', payload.parser_name);
	form.set('ks_store_id', String(payload.ks_store_id));
	form.set('tenant_id', payload.tenant_id);
	if (payload.title?.trim()) form.set('title', payload.title.trim());
	if (payload.doc_no?.trim()) form.set('doc_no', payload.doc_no.trim());
	if (payload.authors?.trim()) form.set('authors', payload.authors.trim());
	if (payload.public_info?.trim()) form.set('public_info', payload.public_info.trim());
	if (payload.private_info?.trim()) form.set('private_info', payload.private_info.trim());
	if (payload.notes?.trim()) form.set('notes', payload.notes);
	if (payload.ks_desc?.trim()) form.set('ks_desc', payload.ks_desc);
	for (const file of payload.files) {
		form.append('files', file);
	}

	const response = await fetch(`${BASE}/inputs/upload`, {
		method: 'POST',
		credentials: 'same-origin',
		body: form
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to upload kb inputs (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<UploadKbInputsResponse>;
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

// ---------- kb.doc-structure ----------

export type DocStructureLine = {
	line_number: number;
	page_number: number;
	line_type: string;
	corrected_line_type: string;
	font: string;
	font_size: string;
	coords: number[];
	content: string;
};

export type GetDocStructureResponse = {
	status: boolean;
	input_id: number;
	file_name?: string;
	corrected_file?: string;
	lines: DocStructureLine[];
	pages: number;
	total: number;
};

export async function getKbDocStructure(inputRecordId: number): Promise<GetDocStructureResponse> {
	return fetchOrThrow<GetDocStructureResponse>(
		`${BASE}/doc-structure?input_record_id=${encodeURIComponent(String(inputRecordId))}`,
		'Failed to retrieve document structure'
	);
}

// ---------- kb.topic-chunks ----------

export type KbChunkSpan = {
	page_number: number;
	line_number: number;
};

export type KbChunkBBox = {
	page_number: number;
	coords: number[];
};

export type KbTopicChunkRecord = {
	seqno: number;
	topic_type: string;
	topic: string;
	keywords: string[];
	line_tokens: string[];
	source_line_numbers: number[];
	source_line_spans: KbChunkSpan[];
	content_lines: RawLine[];
	bounding_boxes: KbChunkBBox[];
};

export type ListKbTopicChunksResponse = {
	status: boolean;
	input_id: number;
	file_name?: string;
	results: KbTopicChunkRecord[];
	total: number;
};

export async function listKbTopicChunks(inputRecordId: number): Promise<ListKbTopicChunksResponse> {
	return fetchOrThrow<ListKbTopicChunksResponse>(
		`${BASE}/topic-chunks?input_record_id=${encodeURIComponent(String(inputRecordId))}`,
		'Failed to retrieve topic chunks'
	);
}

// ---------- kb.stores ----------

export type KnowledgeStoreStatus = 'active' | 'suspended' | 'inactive' | string;
export type KnowledgeStoreSyncMode = 'auto' | 'manual' | string;

export type KnowledgeStoreRecord = {
	id: number;
	tenant_id?: string;
	ks_type?: string;
	ks_name: string;
	ks_desc?: string;
	ks_sync_mode?: KnowledgeStoreSyncMode;
	ks_sources?: string[];
	status?: KnowledgeStoreStatus;
	notes?: string;
	error_msg?: string;
	public_info?: unknown;
	private_info?: unknown;
	create_time?: string;
	modify_time?: string;
};

export type ListKnowledgeStoresResponse = {
	status: boolean;
	results: KnowledgeStoreRecord[];
	total?: number;
};

export type KnowledgeStoreResponse = {
	status: boolean;
	record: KnowledgeStoreRecord;
};

export type CreateKnowledgeStorePayload = {
	tenant_id?: string | null;
	ks_type?: string | null;
	ks_name: string;
	ks_desc?: string | null;
	ks_sync_mode?: KnowledgeStoreSyncMode | null;
	ks_sources?: string[] | null;
	status?: KnowledgeStoreStatus | null;
	notes?: string | null;
	public_info?: unknown;
	private_info?: unknown;
};

export type UpdateKnowledgeStorePayload = Partial<CreateKnowledgeStorePayload>;

export async function listKnowledgeStores(): Promise<ListKnowledgeStoresResponse> {
	return fetchOrThrow<ListKnowledgeStoresResponse>(`${BASE}/stores`, 'Failed to list knowledge stores');
}

export async function createKnowledgeStore(
	payload: CreateKnowledgeStorePayload
): Promise<KnowledgeStoreResponse> {
	const response = await fetch(`${BASE}/stores`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to create knowledge store (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<KnowledgeStoreResponse>;
}

export async function updateKnowledgeStore(
	id: number,
	payload: UpdateKnowledgeStorePayload
): Promise<KnowledgeStoreResponse> {
	const response = await fetch(`${BASE}/stores/${encodeURIComponent(String(id))}`, {
		method: 'PUT',
		credentials: 'same-origin',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to update knowledge store (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<KnowledgeStoreResponse>;
}

export async function deleteKnowledgeStore(id: number): Promise<{ status: boolean }> {
	const response = await fetch(`${BASE}/stores/${encodeURIComponent(String(id))}`, {
		method: 'DELETE',
		credentials: 'same-origin'
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to delete knowledge store (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<{ status: boolean }>;
}
