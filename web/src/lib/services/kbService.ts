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
		status?: string;
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
