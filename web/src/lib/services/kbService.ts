import {
	MOCK_CATEGORY_SUMMARIES,
	MOCK_SUMMARY_GRAPH_NODES,
	MOCK_SUMMARY_TREE_RECORDS
} from '$lib/components/home3/summary-mock-data';
import type {
	SummaryCategoryNode,
	SummaryRecordCard,
	SummaryTreeRecord
} from '$lib/components/home3/summary-types';

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
		doc_processor_name?: string;
		'doc-processor-name'?: string;
		time?: string;
		start_time?: string;
		status?: string;
		proc_status?: string;
		'proc-status'?: string;
		error?: string;
		progress?: string;
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
	ksStoreId?: number | null;
	recordId?: string;
	name?: string;
	title?: string;
	docNo?: string;
	parserName?: string;
	operation?: string;
	procStatus?: string;
	excludeDocType?: string;
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
	if (params.ksStoreId != null) query.set('ks_store_id', String(params.ksStoreId));
	if (params.recordId?.trim()) query.set('record_id', params.recordId.trim());
	if (params.name?.trim()) query.set('name', params.name.trim());
	if (params.title?.trim()) query.set('title', params.title.trim());
	if (params.docNo?.trim()) query.set('doc_no', params.docNo.trim());
	if (params.fileName.trim()) query.set('file_name', params.fileName.trim());
	if (params.parserName?.trim()) query.set('parser_name', params.parserName.trim());
	if (params.operation?.trim()) query.set('operation', params.operation.trim());
	if (params.procStatus?.trim()) query.set('proc_status', params.procStatus.trim());
	if (params.excludeDocType?.trim()) query.set('exclude_doc_type', params.excludeDocType.trim());
	if (params.startTime.trim()) query.set('start_time', params.startTime.trim());
	if (params.endTime.trim()) query.set('end_time', params.endTime.trim());
	if (params.modifyStartTime?.trim()) query.set('modify_start_time', params.modifyStartTime.trim());
	if (params.modifyEndTime?.trim()) query.set('modify_end_time', params.modifyEndTime.trim());
	query.set('page', String(params.page));
	query.set('page_size', String(params.pageSize));
	return query.toString();
}

export type DocProcLogRecord = {
	id: number;
	call_reason: string;
	doc_proc_name: string;
	record_id?: number;
	proc_progress?: string;
	entry_type: string;
	create_time: string;
};

export type ListDocProcLogsResponse = {
	status: boolean;
	results: DocProcLogRecord[];
	page: number;
	page_size: number;
	total: number;
};

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

export async function listDocProcLogs(params: {
	docProcName?: string;
	recordId?: number;
	page?: number;
	pageSize?: number;
	orderBy?: string;
	orderDir?: 'asc' | 'desc';
}): Promise<ListDocProcLogsResponse> {
	const query = new URLSearchParams();
	if (params.docProcName?.trim()) query.set('doc_proc_name', params.docProcName.trim());
	if (params.recordId != null) query.set('record_id', String(params.recordId));
	query.set('page', String(params.page ?? 1));
	query.set('page_size', String(params.pageSize ?? 1));
	query.set('order_by', params.orderBy ?? 'create_time');
	query.set('order_dir', params.orderDir ?? 'desc');
	return fetchOrThrow<ListDocProcLogsResponse>(
		`${BASE}/doc-proc-logs?${query.toString()}`,
		'Failed to list doc proc logs'
	);
}

// ---------- kb.metrics / raw-lines / single input ----------

export type SourceLineSpan = { page_number: number; line_number: number } | string | number;

export type KbMetricRecord = {
	id: number;
	input_record_id: number;
	event_id?: string | null;
	input_filename?: string;
	metric_name?: string;
	metric_name_en?: string;
	source_line_spans?: SourceLineSpan[];
	metric_subject?: string;
	metric_subject_en?: string;
	metric_desc?: string;
	metric_desc_en?: string;
	metric_context?: string;
	metric_context_en?: string;
	metric_keywords?: string[];
	metric_keywords_en?: string[];
	model_name?: string;
	location_type?: string;
	metric_unit?: string;
	metric_unit_en?: string;
	metric_value?: string;
	value_data_type?: string;
	value_range_type?: string;
	value_class?: string;
	value_class_en?: string;
	formula_or_definition?: string;
	threshold_or_target?: string;
	measurement_frequency?: string;
	confidence?: number;
	is_explicit_metric?: boolean;
	table_name_or_section?: string;
	reasoning_tags?: string[];
	created_at?: string;
};

export type ListKbMetricsResponse = {
	status: boolean;
	results: KbMetricRecord[];
	total: number;
};

export type GetKbMetricResponse = {
	status: boolean;
	record: KbMetricRecord;
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

export type UpdateKbMetricPayload = {
	metric_name?: string | null;
	metric_name_en?: string | null;
	source_line_spans?: unknown;
	metric_subject?: string | null;
	metric_subject_en?: string | null;
	metric_desc?: string | null;
	metric_desc_en?: string | null;
	metric_context?: string | null;
	metric_context_en?: string | null;
	metric_keywords?: unknown;
	metric_keywords_en?: unknown;
	model_name?: string | null;
	location_type?: string | null;
	metric_unit?: string | null;
	metric_unit_en?: string | null;
	metric_value?: string | null;
	value_data_type?: string | null;
	value_range_type?: string | null;
	value_class?: string | null;
	value_class_en?: string | null;
	formula_or_definition?: string | null;
	threshold_or_target?: string | null;
	measurement_frequency?: string | null;
	confidence?: number | null;
	is_explicit_metric?: boolean | null;
	table_name_or_section?: string | null;
	reasoning_tags?: unknown;
};

export async function updateKbMetric(
	id: number,
	payload: UpdateKbMetricPayload
): Promise<GetKbMetricResponse> {
	const response = await fetch(`${BASE}/metrics/${encodeURIComponent(String(id))}`, {
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
				: `Failed to update kb metric (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<GetKbMetricResponse>;
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

export async function updateKbInput(
	id: number,
	payload: UpdateKbInputPayload
): Promise<GetKbInputResponse> {
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

export async function uploadKbInputs(
	payload: UploadKbInputsPayload
): Promise<UploadKbInputsResponse> {
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

export type UpdateRawLinePayload = {
	input_record_id: number;
	page_number: number;
	line_number: number;
	content: string;
};

export type UpdateRawLineResponse = {
	status: boolean;
	line: RawLine;
};

export async function updateRawLine(payload: UpdateRawLinePayload): Promise<UpdateRawLineResponse> {
	const response = await fetch(`${BASE}/raw-lines`, {
		method: 'PATCH',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to update raw line (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<UpdateRawLineResponse>;
}

export type CreateKbMetricPayload = {
	input_record_id: number;
	metric_name?: string;
	source_line_spans?: SourceLineSpan[];
};

export type CreateKbMetricResponse = {
	status: boolean;
	record: KbMetricRecord;
};

export async function createKbMetric(
	payload: CreateKbMetricPayload
): Promise<CreateKbMetricResponse> {
	const response = await fetch(`${BASE}/metrics`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to create kb metric (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<CreateKbMetricResponse>;
}

export type ExtractKbMetricsPayload = {
	record_id: number;
	lines: string[];
};

export type ExtractedKbMetric = {
	metric_name?: string;
	metric_name_en?: string;
	source_line_spans?: SourceLineSpan[];
	metric_subject?: string;
	metric_subject_en?: string;
	metric_desc?: string;
	metric_desc_en?: string;
	metric_context?: string;
	metric_context_en?: string;
	metric_keywords?: unknown;
	metric_keywords_en?: unknown;
	location_type?: string;
	metric_unit?: string;
	metric_unit_en?: string;
	metric_value?: string;
	value_data_type?: string;
	value_range_type?: string;
	value_class?: string;
	value_class_en?: string;
	formula_or_definition?: string;
	threshold_or_target?: string;
	measurement_frequency?: string;
	confidence?: number;
	is_explicit_metric?: boolean;
	table_name_or_section?: string;
	reasoning_tags?: unknown;
};

export type ExtractKbMetricsResponse = {
	status: boolean;
	metrics?: ExtractedKbMetric[];
	error?: string;
};

export async function extractKbMetrics(
	payload: ExtractKbMetricsPayload
): Promise<ExtractKbMetricsResponse> {
	const response = await fetch(`${BASE}/metrics/extract`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error === 'string'
				? parsed.error
				: `Failed to extract metrics (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<ExtractKbMetricsResponse>;
}

export type SaveExtractedKbMetricsPayload = {
	record_id: number;
	metrics: ExtractedKbMetric[];
};

export type SaveExtractedKbMetricsResponse = {
	status: boolean;
	inserted?: number;
	error?: string;
};

export async function saveExtractedKbMetrics(
	payload: SaveExtractedKbMetricsPayload
): Promise<SaveExtractedKbMetricsResponse> {
	const response = await fetch(`${BASE}/metrics/save`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error === 'string'
				? parsed.error
				: `Failed to save extracted metrics (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<SaveExtractedKbMetricsResponse>;
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

export type UpdateDocStructureLinePayload = {
	input_record_id: number;
	page_number: number;
	line_number: number;
	corrected_line_type?: string;
	content?: string;
	coords?: number[];
};

export async function updateKbDocStructureLine(
	payload: UpdateDocStructureLinePayload
): Promise<GetDocStructureResponse> {
	const response = await fetch(`${BASE}/doc-structure`, {
		method: 'PATCH',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to update doc structure line (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<GetDocStructureResponse>;
}

export type SplitDocStructureLinePayload = {
	input_record_id: number;
	page_number: number;
	line_number: number;
	contents: string[];
	line_type: string;
};

export async function splitKbDocStructureLines(
	payload: SplitDocStructureLinePayload
): Promise<GetDocStructureResponse> {
	const response = await fetch(`${BASE}/doc-structure/split`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to split doc structure line (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<GetDocStructureResponse>;
}

export type DeleteDocStructureLinePayload = {
	input_record_id: number;
	page_number: number;
	line_number: number;
};

export async function deleteKbDocStructureLine(
	payload: DeleteDocStructureLinePayload
): Promise<GetDocStructureResponse> {
	const params = new URLSearchParams({
		input_record_id: String(payload.input_record_id),
		page_number: String(payload.page_number),
		line_number: String(payload.line_number)
	});
	const response = await fetch(`${BASE}/doc-structure?${params}`, {
		method: 'DELETE',
		credentials: 'same-origin'
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to delete doc structure line (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<GetDocStructureResponse>;
}

// ---------- kb.chunks ----------

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

export async function listKbChunks(inputRecordId: number): Promise<ListKbTopicChunksResponse> {
	const result = await fetchOrThrow<ListKbTopicChunksResponse>(
		`${BASE}/chunks?input_record_id=${encodeURIComponent(String(inputRecordId))}`,
		'Failed to retrieve chunks'
	);
	console.log('[kbService] listKbChunks fetched', {
		inputRecordId,
		total: result.total,
		chunks: (result.results ?? []).map((chunk) => ({
			seqno: chunk.seqno,
			line_tokens: chunk.line_tokens,
			source_line_spans: chunk.source_line_spans,
			content_line_count: chunk.content_lines?.length ?? 0
		}))
	});
	return result;
}

// ---------- kb.scene_objects ----------

export type KbSceneActor = { type?: string; name?: string };
export type KbSceneResource = { type?: string; name?: string };
export type KbSceneAction = { sequence?: number; actor?: string; action?: string };
export type KbSceneRelationship = { type?: string; target?: string };
export type KbSceneSourceRef = {
	source_id?: string;
	evidence_type?: string;
	reference?: string;
};
export type KbSceneDiscriminator = {
	intent?: string;
	domain?: string[];
	discriminators?: Array<{
		category?: string;
		value?: string;
		confidence?: number;
		reason?: string;
	}>;
	exploration_plan?: string[];
};

export type KbSceneBlockRecord = {
	id: number;
	object_id: string;
	event_id: string;
	scene_id: string;
	scene_type: string;
	title: string;
	title_en?: string;
	summary: string;
	summary_en?: string;
	line_spans?: SourceLineSpan[];
	actors: KbSceneActor[];
	resources: KbSceneResource[];
	preconditions: string[];
	triggers: string[];
	states: string[];
	states_en?: string[];
	actions: KbSceneAction[];
	constraints: string[];
	decisions: string[];
	outcomes: string[];
	failure_modes: string[];
	root_causes: string[];
	resolutions: string[];
	relationships: KbSceneRelationship[];
	discriminators: KbSceneDiscriminator[];
	keywords: string[];
	keywords_en?: string[];
	confidence: number;
	source_refs: KbSceneSourceRef[];
	model_name: string;
	prompt_name: string;
	create_time: string;
	modify_time: string;
};

export type ListKbSceneBlocksResponse = {
	status: boolean;
	input_id: number;
	file_name?: string;
	results: KbSceneBlockRecord[];
	total: number;
};

export async function listKbSceneBlocks(
	inputRecordId: number
): Promise<ListKbSceneBlocksResponse> {
	const result = await fetchOrThrow<ListKbSceneBlocksResponse>(
		`${BASE}/scene-blocks?input_record_id=${encodeURIComponent(String(inputRecordId))}`,
		'Failed to retrieve scene blocks'
	);
	console.log('[kbService] listKbSceneBlocks fetched', {
		inputRecordId,
		total: result.total,
		scene_blocks: (result.results ?? []).map((block) => ({
			id: block.id,
			scene_id: block.scene_id,
			scene_type: block.scene_type,
			title: block.title,
			confidence: block.confidence
		}))
	});
	return result;
}

// ---------- kb.products ----------

export type KbProductRecord = {
	id: number;
	product_rel_id: string;
	product_name: string;
	product_name_en?: string;
	canonical_name: string;
	canonical_name_en?: string;
	product_type?: string;
	relation_type?: string;
	relation_summary?: string;
	relation_summary_en?: string;
	evidence_quote?: string;
	evidence_lines?: SourceLineSpan[];
	obligation_level?: string;
	requirement_text?: string;
	requirement_text_en?: string;
	conditions: string[];
	exceptions: string[];
	parameters: string[];
	related_products: string[];
	responsible_actor?: string;
	confidence: number;
	confidence_reason?: string;
	model_name: string;
	prompt_name: string;
	create_time: string;
	modify_time: string;
};

export type ListKbProductsResponse = {
	status: boolean;
	input_id: number;
	file_name?: string;
	results: KbProductRecord[];
	total: number;
};

export async function listKbProducts(inputRecordId: number): Promise<ListKbProductsResponse> {
	const result = await fetchOrThrow<ListKbProductsResponse>(
		`${BASE}/products?input_record_id=${encodeURIComponent(String(inputRecordId))}`,
		'Failed to retrieve products'
	);
	console.log('[kbService] listKbProducts fetched', {
		inputRecordId,
		total: result.total,
		products: (result.results ?? []).map((p) => ({
			id: p.id,
			product_rel_id: p.product_rel_id,
			product_name: p.product_name,
			product_type: p.product_type,
			confidence: p.confidence
		}))
	});
	return result;
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
	return fetchOrThrow<ListKnowledgeStoresResponse>(
		`${BASE}/stores`,
		'Failed to list knowledge stores'
	);
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

export async function deleteKbInput(id: number): Promise<{ status: boolean }> {
	const response = await fetch(`${BASE}/inputs/${encodeURIComponent(String(id))}`, {
		method: 'DELETE',
		credentials: 'same-origin'
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to delete kb input (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<{ status: boolean }>;
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

// ---------- kb.document-summaries (phase 1 mock adapters) ----------

export type ListSummaryGraphResponse = {
	status: boolean;
	results: SummaryCategoryNode[];
};

export type GetSummaryCategoryResponse = {
	status: boolean;
	categoryPath: string;
	summaries: SummaryRecordCard[];
};

export type SearchSummaryTreeParams = {
	recordId?: string;
	title?: string;
	docNo?: string;
	fileName?: string;
	docType?: string;
	parserName?: string;
	operation?: string;
	procStatus?: string;
	createStart?: string;
	createEnd?: string;
	modifyStart?: string;
	modifyEnd?: string;
	ksStoreId?: number | null;
};

export type SearchSummaryTreeResponse = {
	status: boolean;
	results: SummaryTreeRecord[];
	total: number;
};

export type GetRecordSummariesResponse = {
	status: boolean;
	recordId: number;
	summaries: SummaryRecordCard[];
};

export async function listSummaryGraph(): Promise<ListSummaryGraphResponse> {
	return fetchOrThrow<ListSummaryGraphResponse>(
		`${BASE}/summary-graph`,
		'Failed to list summary graph'
	);
}

export async function listSummaryGraphMock(): Promise<ListSummaryGraphResponse> {
	return Promise.resolve({
		status: true,
		results: structuredClone(MOCK_SUMMARY_GRAPH_NODES)
	});
}

export async function getSummaryCategory(
	categoryPath: string
): Promise<GetSummaryCategoryResponse> {
	return fetchOrThrow<GetSummaryCategoryResponse>(
		`${BASE}/summary-category?category_path=${encodeURIComponent(categoryPath)}`,
		'Failed to load summary category'
	);
}

export async function getSummaryCategoryMock(
	categoryPath: string
): Promise<GetSummaryCategoryResponse> {
	return Promise.resolve({
		status: true,
		categoryPath,
		summaries: structuredClone(MOCK_CATEGORY_SUMMARIES[categoryPath] ?? [])
	});
}

function summaryTreeStatusText(
	record: KbInputRecord,
	operation: string
): { operation: string; procStatus: string } {
	const items = record.status ?? [];
	const desiredOperation = operation.trim().toLowerCase();
	const matched =
		desiredOperation !== ''
			? [...items]
					.reverse()
					.find((item) => (item?.operation ?? '').trim().toLowerCase() === desiredOperation)
			: [...items].reverse().find((item) => item != null);

	if (!matched) {
		return { operation: '—', procStatus: 'pending' };
	}

	return {
		operation: matched.operation?.trim() || '—',
		procStatus:
			matched.proc_status?.trim() ||
			matched['proc-status']?.trim() ||
			matched.status?.trim() ||
			'pending'
	};
}

function mapKbInputToSummaryTreeRecord(
	record: KbInputRecord,
	operation: string
): SummaryTreeRecord {
	const statusSummary = summaryTreeStatusText(record, operation);
	return {
		id: record.id,
		title:
			record.title?.trim() ||
			record.name?.trim() ||
			record.file_name?.trim() ||
			`Record #${record.id}`,
		fileName: record.file_name?.trim() || record.name?.trim() || '—',
		docType: record.type?.trim() || '—',
		docNo: record.doc_no?.trim() || '—',
		parserName: record.parser_name?.trim() || '—',
		procStatus: statusSummary.procStatus,
		createTime: record.create_time ? new Date(record.create_time).toLocaleString() : '—',
		modifyTime: record.modify_time ? new Date(record.modify_time).toLocaleString() : '—',
		summaries: []
	};
}

export async function searchSummaryTree(
	params: SearchSummaryTreeParams
): Promise<SearchSummaryTreeResponse> {
	const response = await listKbInputs({
		docType: params.docType?.trim() || 'all',
		parseState: 'all',
		fileName: params.fileName?.trim() || '',
		startTime: params.createStart?.trim() || '',
		endTime: params.createEnd?.trim() || '',
		page: 1,
		pageSize: 100,
		ksStoreId: params.ksStoreId ?? null,
		recordId: params.recordId?.trim() || '',
		title: params.title?.trim() || '',
		docNo: params.docNo?.trim() || '',
		parserName: params.parserName?.trim() || '',
		operation: params.operation?.trim() || '',
		procStatus: params.procStatus?.trim() || '',
		modifyStartTime: params.modifyStart?.trim() || '',
		modifyEndTime: params.modifyEnd?.trim() || ''
	});

	return {
		status: response.status,
		results: response.results.map((record) =>
			mapKbInputToSummaryTreeRecord(record, params.operation?.trim() || '')
		),
		total: response.total
	};
}

export async function getRecordSummaries(recordId: number): Promise<GetRecordSummariesResponse> {
	return fetchOrThrow<GetRecordSummariesResponse>(
		`${BASE}/record-summaries?record_id=${encodeURIComponent(String(recordId))}`,
		'Failed to load record summaries'
	);
}

// ---------- kb.topic-graph / topic-category / record-topics ----------

export type TopicCategoryNodeApi = {
	id: string;
	categoryPath: string;
	label: string;
	metadata: {
		desc: string;
		confidence: number;
		keywords: string[];
		create_time: string;
	};
	childIds: string[];
	topicIds: string[];
	hasTopicsFile: boolean;
	expanded: boolean;
};

export type TopicCardApi = {
	id: string;
	pdfFileName: string;
	topicKeywords: string[];
	topicKeywordsEn?: string[];
	topicText: string;
	topicDescEn?: string;
	topicType: string;
	categoryPaths?: string[];
	categoryPathsEn?: string[];
	sourceLineSpecs?: string[];
	inputId: number;
	page: number;
	coords: number[];
	targets: Array<{ page: number; coords: number[] }>;
};

export type ListTopicGraphResponse = {
	status: boolean;
	results: TopicCategoryNodeApi[];
};

export type GetTopicCategoryResponse = {
	status: boolean;
	categoryPath: string;
	topics: TopicCardApi[];
};

export type GetRecordTopicsResponse = {
	status: boolean;
	recordId: number;
	topics: TopicCardApi[];
};

export type FilterGraphNodesRequest = {
	mode: 'summary' | 'topic';
	candidatePaths: string[];
	semanticText: string;
	threshold: number;
};

export type FilterGraphNodesResponse = {
	status: boolean;
	matches: Array<{ categoryPath: string; score: number }>;
};

export async function listTopicGraph(): Promise<ListTopicGraphResponse> {
	return fetchOrThrow<ListTopicGraphResponse>(`${BASE}/topic-graph`, 'Failed to list topic graph');
}

export async function getTopicCategory(categoryPath: string): Promise<GetTopicCategoryResponse> {
	return fetchOrThrow<GetTopicCategoryResponse>(
		`${BASE}/topic-category?category_path=${encodeURIComponent(categoryPath)}`,
		'Failed to load topic category'
	);
}

export async function getRecordTopics(recordId: number): Promise<GetRecordTopicsResponse> {
	return fetchOrThrow<GetRecordTopicsResponse>(
		`${BASE}/record-topics?record_id=${encodeURIComponent(String(recordId))}`,
		'Failed to load record topics'
	);
}

export type UpdateRecordTopicPayload = {
	record_id: number;
	topic_id: string;
	topic_type: string;
	topic_text: string;
	topic_desc_en: string;
	topic_keywords: string[];
	topic_keywords_en: string[];
	category_paths: string[];
	category_paths_en: string[];
};

export type KbFrontendConfig = {
	topic_types: string[];
	mandatory_processors: string[];
	required_processors: string[];
	max_doc_process_pipelines: number;
};

export async function getKbFrontendConfig(): Promise<KbFrontendConfig> {
	const response = await fetchOrThrow<{ status: boolean; config: KbFrontendConfig }>(
		`${BASE}/config`,
		'Failed to load kb frontend config'
	);
	return response.config;
}

export async function updateRecordTopic(payload: UpdateRecordTopicPayload): Promise<{ status: boolean }> {
	const response = await fetch(`${BASE}/record-topic`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const text = await response.text().catch(() => '');
		throw new Error(text || `Failed to update record topic (${response.status})`);
	}
	return response.json() as Promise<{ status: boolean }>;
}

export type ListProvisionGraphResponse = ListTopicGraphResponse;
export type GetProvisionCategoryResponse = GetTopicCategoryResponse;
export type GetRecordProvisionsResponse = GetRecordTopicsResponse;

export async function listProvisionGraph(
	ksStoreId?: number | null
): Promise<ListProvisionGraphResponse> {
	const query = new URLSearchParams();
	if (ksStoreId != null) query.set('ks_store_id', String(ksStoreId));
	const suffix = query.toString();
	const result = await fetchOrThrow<ListProvisionGraphResponse>(
		`${BASE}/provision-graph${suffix ? `?${suffix}` : ''}`,
		'Failed to list provision graph'
	);
	const provisionIds = new Set((result.results ?? []).flatMap((node) => node.topicIds ?? []));
	/*
	console.log('[Compliance Provisions] provision graph received', {
		ksStoreId,
		nodes: result.results?.length ?? 0,
		uniqueProvisions: provisionIds.size,
		provisionRefs: (result.results ?? []).reduce(
			(count, node) => count + (node.topicIds?.length ?? 0),
			0
		)
	});
	*/
	return result;
}

export async function getProvisionCategory(
	categoryPath: string,
	ksStoreId?: number | null
): Promise<GetProvisionCategoryResponse> {
	const query = new URLSearchParams();
	query.set('category_path', categoryPath);
	if (ksStoreId != null) query.set('ks_store_id', String(ksStoreId));
	const result = await fetchOrThrow<GetProvisionCategoryResponse>(
		`${BASE}/provision-category?${query.toString()}`,
		'Failed to load provision category'
	);
	/*
	console.log('[Compliance Provisions] provision category received', {
		categoryPath,
		ksStoreId,
		provisions: result.topics?.length ?? 0,
		provisionIds: (result.topics ?? []).map((topic) => topic.id)
	});
	*/
	return result;
}

export async function getRecordProvisions(recordId: number): Promise<GetRecordProvisionsResponse> {
	const result = await fetchOrThrow<GetRecordProvisionsResponse>(
		`${BASE}/record-provisions?record_id=${encodeURIComponent(String(recordId))}`,
		'Failed to load record provisions'
	);
	/*
	console.log('[Compliance Provisions] record provisions received', {
		recordId,
		provisions: result.topics?.length ?? 0,
		provisionIds: (result.topics ?? []).map((topic) => topic.id)
	});
	*/
	return result;
}

// ---------- kb.artifact-category (multi-type category lookups) ----------

export type ArtifactCategoryItem = {
	id: string;
	label: string;
	sublabel?: string;
	inputId: number;
	page: number;
	// Extended metric fields (populated only for metrics)
	category_paths?: string[];
	category_paths_en?: string[];
	value?: string;
	desc?: string;
	desc_en?: string;
	confidence?: number;
	context?: string;
	context_en?: string;
	keywords?: string[];
	keywords_en?: string[];
	is_explicit_metric?: boolean;
	location_type?: string;
	measurement_frequency?: string;
	reasoning_tags?: unknown[];
	source_line_spans?: unknown[];
	subject?: string;
	subject_en?: string;
	source?: string;
	threshold?: string;
	unit?: string;
	unit_en?: string;
	value_class?: string;
	value_class_en?: string;
	value_data_type?: string;
	value_range_type?: string;
};

export type GetArtifactCategoryResponse = {
	status: boolean;
	categoryPath: string;
	type: string;
	items: ArtifactCategoryItem[];
};

export type ArtifactCategoryCount = {
	type: string;
	count: number;
};

export type GetArtifactCategoryCountsResponse = {
	status: boolean;
	categoryPath: string;
	counts: ArtifactCategoryCount[];
};

export async function getMetricCategory(
	categoryPath: string,
	ksStoreId?: number | null
): Promise<GetArtifactCategoryResponse> {
	const query = new URLSearchParams();
	query.set('category_path', categoryPath);
	if (ksStoreId != null) query.set('ks_store_id', String(ksStoreId));
	return fetchOrThrow<GetArtifactCategoryResponse>(
		`${BASE}/metric-category?${query.toString()}`,
		'Failed to load metric category'
	);
}

export async function getSceneCategory(
	categoryPath: string,
	ksStoreId?: number | null
): Promise<GetArtifactCategoryResponse> {
	const query = new URLSearchParams();
	query.set('category_path', categoryPath);
	if (ksStoreId != null) query.set('ks_store_id', String(ksStoreId));
	return fetchOrThrow<GetArtifactCategoryResponse>(
		`${BASE}/scene-category?${query.toString()}`,
		'Failed to load scene category'
	);
}

export async function getProductCategory(
	categoryPath: string,
	ksStoreId?: number | null
): Promise<GetArtifactCategoryResponse> {
	const query = new URLSearchParams();
	query.set('category_path', categoryPath);
	if (ksStoreId != null) query.set('ks_store_id', String(ksStoreId));
	return fetchOrThrow<GetArtifactCategoryResponse>(
		`${BASE}/product-category?${query.toString()}`,
		'Failed to load product category'
	);
}

export async function getArtifactCategoryCounts(
	categoryPath: string
): Promise<GetArtifactCategoryCountsResponse> {
	return fetchOrThrow<GetArtifactCategoryCountsResponse>(
		`${BASE}/artifact-category-counts?category_path=${encodeURIComponent(categoryPath)}`,
		'Failed to load artifact category counts'
	);
}

export async function filterGraphNodes(
	params: FilterGraphNodesRequest
): Promise<FilterGraphNodesResponse> {
	const response = await fetch(`${BASE}/graph-node-filter`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(params)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to filter graph nodes (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<FilterGraphNodesResponse>;
}

// ---------- kb.provisions ----------

export type KbProvisionRecord = {
	id: number;
	input_record_id: number;
	prov_id: number;
	input_filename?: string;
	prov_name?: string;
	prov_name_en?: string;
	provision_type?: string;
	source_text?: string;
	source_line_spans?: Array<string | SourceLineSpan>;
	provision?: string;
	provision_en?: string;
	provision_subject?: string;
	provision_subject_en?: string;
	prov_desc?: string;
	prov_desc_en?: string;
	prov_context?: string;
	prov_context_en?: string;
	provision_keywords?: string[];
	provision_keywords_en?: string[];
	category_paths?: unknown;
	category_paths_en?: unknown;
	location_type?: string;
	confidence?: number;
	is_explicit?: boolean;
	need_verify?: boolean;
	model_name?: string;
	prompt_name?: string;
	created_at?: string;
};

export type ListKbProvisionsResponse = {
	status: boolean;
	results: KbProvisionRecord[];
	total: number;
};

export async function listKbProvisions(inputRecordId: number): Promise<ListKbProvisionsResponse> {
	return fetchOrThrow<ListKbProvisionsResponse>(
		`${BASE}/provisions?input_record_id=${encodeURIComponent(String(inputRecordId))}`,
		'Failed to list kb provisions'
	);
}

export type CreateKbProvisionPayload = {
	input_record_id: number;
	provision_name?: string;
	source_line_spans?: SourceLineSpan[];
};

export type CreateKbProvisionResponse = {
	status: boolean;
	prov_id: number;
	provision_name: string;
	span_count: number;
	message?: string;
};

export async function createKbProvision(
	payload: CreateKbProvisionPayload
): Promise<CreateKbProvisionResponse> {
	const response = await fetch(`${BASE}/provisions`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to create kb provision (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<CreateKbProvisionResponse>;
}

export type ExtractedKbProvision = {
	prov_name?: string;
	prov_name_en?: string;
	provision_type?: string;
	source_text?: string;
	source_line_spans?: SourceLineSpan[];
	provision?: string;
	provision_en?: string;
	provision_subject?: string;
	provision_subject_en?: string;
	prov_desc?: string;
	prov_desc_en?: string;
	prov_context?: string;
	prov_context_en?: string;
	provision_keywords?: unknown;
	provision_keywords_en?: unknown;
	category_paths?: unknown;
	category_paths_en?: unknown;
	location_type?: string;
	confidence?: number;
	is_explicit?: boolean;
	need_verify?: boolean;
};

export type ExtractKbProvisionsPayload = {
	record_id: number;
	source_line_spans: { page_number: number; line_number: number }[];
};

export type ExtractKbProvisionsResponse = {
	status: boolean;
	provisions?: ExtractedKbProvision[];
	language?: string;
	error?: string;
};

export async function extractKbProvisions(
	payload: ExtractKbProvisionsPayload
): Promise<ExtractKbProvisionsResponse> {
	const response = await fetch(`${BASE}/provisions/extract`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error === 'string'
				? parsed.error
				: `Failed to extract provisions (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<ExtractKbProvisionsResponse>;
}

export type SaveExtractedKbProvisionsPayload = {
	record_id: number;
	provisions: ExtractedKbProvision[];
};

export type SaveExtractedKbProvisionsResponse = {
	status: boolean;
	inserted?: number;
	error?: string;
};

export async function saveExtractedKbProvisions(
	payload: SaveExtractedKbProvisionsPayload
): Promise<SaveExtractedKbProvisionsResponse> {
	const response = await fetch(`${BASE}/provisions/save`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error === 'string'
				? parsed.error
				: `Failed to save extracted provisions (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<SaveExtractedKbProvisionsResponse>;
}

export async function searchSummaryTreeMock(
	params: SearchSummaryTreeParams
): Promise<SearchSummaryTreeResponse> {
	const q = {
		recordId: params.recordId?.trim().toLowerCase() ?? '',
		title: params.title?.trim().toLowerCase() ?? '',
		docNo: params.docNo?.trim().toLowerCase() ?? '',
		fileName: params.fileName?.trim().toLowerCase() ?? '',
		docType: params.docType?.trim().toLowerCase() ?? '',
		parserName: params.parserName?.trim().toLowerCase() ?? '',
		procStatus: params.procStatus?.trim().toLowerCase() ?? ''
	};

	const results = MOCK_SUMMARY_TREE_RECORDS.filter((record) => {
		if (q.recordId && !String(record.id).includes(q.recordId)) return false;
		if (q.title && !record.title.toLowerCase().includes(q.title)) return false;
		if (q.docNo && !record.docNo.toLowerCase().includes(q.docNo)) return false;
		if (q.fileName && !record.fileName.toLowerCase().includes(q.fileName)) return false;
		if (q.docType && q.docType !== 'all' && record.docType.toLowerCase() !== q.docType)
			return false;
		if (q.parserName && !record.parserName.toLowerCase().includes(q.parserName)) return false;
		if (q.procStatus && q.procStatus !== 'all' && record.procStatus.toLowerCase() !== q.procStatus)
			return false;
		return true;
	});

	return Promise.resolve({
		status: true,
		results: structuredClone(results),
		total: results.length
	});
}

export async function stopKbInput(id: number): Promise<void> {
	const response = await fetch(`${BASE}/inputs/${encodeURIComponent(String(id))}/stop`, {
		method: 'POST',
		credentials: 'same-origin'
	});
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to stop pipeline (${response.status})`;
		throw new Error(msg);
	}
}

// ---------- Inventory Items (extracted item instances) ----------

export type InventoryItemSpec = {
	name?: string;
	value?: string | number;
	unit?: string;
};

export type KbInventoryItemRecord = {
	id: number;
	inventory_item_id: string;
	input_record_id?: number;
	item_name?: string;
	canonical_name?: string;
	item_category?: string;
	manufacturer?: string;
	brand?: string;
	model_number?: string;
	part_number?: string;
	normalized_specs?: InventoryItemSpec[];
	raw_specs?: InventoryItemSpec[];
	standards?: string[];
	aliases?: string[];
	evidence_quote?: string;
	source_line_spans?: SourceLineSpan[];
	validation_flags?: string[];
	missing_required_attrs?: string[];
	dedupe_key?: string;
	schema_version?: string;
	dictionary_version?: string;
	confidence?: number;
	confidence_reason?: string;
	model_name?: string;
	prompt_name?: string;
	create_time?: string;
	modify_time?: string;
};

export type ListKbInventoryItemsResponse = {
	status: boolean;
	input_id?: number;
	file_name?: string;
	results: KbInventoryItemRecord[];
	total: number;
};

/** List the persisted inventory item rows for a single `kb.inputs` record. */
export async function listKbInventoryItems(
	inputRecordId: number
): Promise<ListKbInventoryItemsResponse> {
	return fetchOrThrow<ListKbInventoryItemsResponse>(
		`${BASE}/inventory-items?input_record_id=${encodeURIComponent(String(inputRecordId))}`,
		'Failed to list kb inventory items'
	);
}

// ---------- Semantic Projections (extracted, read-only) ----------

/** One node in a category path: a category name with its keywords and score. */
export type SemanticCategoryNode = {
	name?: string;
	keywords?: string[];
	confidence?: number;
};

/** A single ranked category path attached to a semantic projection. */
export type SemanticCategoryPath = {
	category_path?: SemanticCategoryNode[];
	path_keywords?: string[];
	path_confidence?: number;
};

export type KbSemanticProjectionRecord = {
	id: number;
	semantic_proj_id: string;
	input_record_id?: number;
	event_id?: string;
	language?: string;
	descriptive_name?: string;
	descriptive_name_en?: string;
	keywords?: string[];
	keywords_en?: string[];
	semantic_projection?: string;
	semantic_projection_en?: string;
	category_paths?: SemanticCategoryPath[];
	category_paths_en?: SemanticCategoryPath[];
	model_name?: string;
	prompt_name?: string;
	create_time?: string;
};

export type ListKbSemanticProjectionsResponse = {
	status: boolean;
	input_id?: number;
	file_name?: string;
	results: KbSemanticProjectionRecord[];
	total: number;
};

/** List the persisted semantic-projection rows for a single `kb.inputs` record. */
export async function listKbSemanticProjections(
	inputRecordId: number
): Promise<ListKbSemanticProjectionsResponse> {
	return fetchOrThrow<ListKbSemanticProjectionsResponse>(
		`${BASE}/semantic-projections?input_record_id=${encodeURIComponent(String(inputRecordId))}`,
		'Failed to list kb semantic projections'
	);
}

// ---------- Inventory Categories (Category Review) ----------

export type InventoryCategoryStatus =
	| 'pending_review'
	| 'approved'
	| 'rejected'
	| 'merged';

export type InventorySpecSchema = {
	canonical_unit: string;
	aliases: string[];
};

export type InventoryPlausibleRange = {
	min?: number | null;
	max?: number | null;
	unit?: string;
};

export type InventoryCategoryRecord = {
	category_key: string;
	status: InventoryCategoryStatus;
	canonical_of?: string;
	display_names: string[];
	required_attrs: string[];
	specs: Record<string, InventorySpecSchema>;
	plausible_ranges: Record<string, InventoryPlausibleRange>;
	seen_count: number;
};

export type ListInventoryCategoriesResponse = {
	status: boolean;
	results: InventoryCategoryRecord[];
	total: number;
};

export type UpdateInventoryCategoryPayload = {
	status?: InventoryCategoryStatus;
	canonical_of?: string;
	required_attrs?: string[];
	specs?: Record<string, InventorySpecSchema>;
	plausible_ranges?: Record<string, InventoryPlausibleRange>;
};

/**
 * List inventory categories from the ontology registry. When `status` is
 * `pending_review`, results are ordered by `seen_count` descending
 * (highest-impact first), matching the Category Review workflow.
 */
export async function listInventoryCategories(
	status: InventoryCategoryStatus | 'all' = 'pending_review',
	limit = 100
): Promise<ListInventoryCategoriesResponse> {
	const params = new URLSearchParams();
	if (status && status !== 'all') params.set('status', status);
	params.set('limit', String(limit));
	return fetchOrThrow<ListInventoryCategoriesResponse>(
		`${BASE}/inventory-categories?${params.toString()}`,
		'Failed to list inventory categories'
	);
}

/**
 * Update (approve / reject / merge / edit schema of) an inventory category.
 * All payload fields are optional — only supplied fields are updated.
 */
export async function updateInventoryCategory(
	key: string,
	payload: UpdateInventoryCategoryPayload
): Promise<{ status: boolean; result: InventoryCategoryRecord }> {
	const response = await fetch(
		`${BASE}/inventory-categories/${encodeURIComponent(key)}`,
		{
			method: 'PATCH',
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(payload)
		}
	);
	if (!response.ok) {
		const parsed = await response.json().catch(() => null);
		const msg =
			parsed && typeof parsed.error_msg === 'string'
				? parsed.error_msg
				: `Failed to update category (${response.status})`;
		throw new Error(msg);
	}
	return response.json();
}
