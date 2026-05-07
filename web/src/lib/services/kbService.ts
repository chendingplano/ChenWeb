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
	ksStoreId?: number | null;
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
	if (params.ksStoreId != null) query.set('ks_store_id', String(params.ksStoreId));
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
		title: record.title?.trim() || record.name?.trim() || record.file_name?.trim() || `Record #${record.id}`,
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
	topicText: string;
	topicType: string;
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
	return fetchOrThrow<ListTopicGraphResponse>(
		`${BASE}/topic-graph`,
		'Failed to list topic graph'
	);
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

export type ListProvisionGraphResponse = ListTopicGraphResponse;
export type GetProvisionCategoryResponse = GetTopicCategoryResponse;
export type GetRecordProvisionsResponse = GetRecordTopicsResponse;

export async function listProvisionGraph(ksStoreId?: number | null): Promise<ListProvisionGraphResponse> {
	const query = new URLSearchParams();
	if (ksStoreId != null) query.set('ks_store_id', String(ksStoreId));
	const suffix = query.toString();
	const result = await fetchOrThrow<ListProvisionGraphResponse>(
		`${BASE}/provision-graph${suffix ? `?${suffix}` : ''}`,
		'Failed to list provision graph'
	);
	const provisionIds = new Set(
		(result.results ?? []).flatMap((node) => node.topicIds ?? [])
	);
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
		if (q.docType && q.docType !== 'all' && record.docType.toLowerCase() !== q.docType) return false;
		if (q.parserName && !record.parserName.toLowerCase().includes(q.parserName)) return false;
		if (q.procStatus && q.procStatus !== 'all' && record.procStatus.toLowerCase() !== q.procStatus) return false;
		return true;
	});

	return Promise.resolve({
		status: true,
		results: structuredClone(results),
		total: results.length
	});
}
