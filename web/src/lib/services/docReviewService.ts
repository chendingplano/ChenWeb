const BASE = '/api/v1/doc-review';

export type AspectInfo = {
	name: string;
	group: string; // "P1".."P6"
	label: string;
	priority: string;
	description: string;
	default_model: string;
	is_tool_use: boolean;
};

export type TierInfo = {
	key: string;
	label: string;
	description: string;
	aspect_names: string[];
};

export type ReferenceDoc = {
	record_id: number;
	doc_no: string;
	title: string;
};

export type SubmitInput = {
	input_record_id: number;
	tier: string;
	aspects: string[];
	reference_docs?: ReferenceDoc[];
	notes?: string;
	model_overrides?: Record<string, { model_ref: string }>;
	requester_name: string;
	requester_id: number;
	report_template?: string;
	doc_template?: string;
};

export type SubmitResult = {
	request_id: number;
	status: string;
	review_run_id?: string;
};

export type RequestStatus = {
	id: number;
	input_record_id: number;
	review_run_id?: string;
	tier: string;
	aspects: string[];
	reference_docs?: ReferenceDoc[];
	notes?: string;
	model_overrides?: Record<string, { model_ref: string }>;
	requester_name: string;
	requester_id: number;
	status: string;
	create_time: string;
	start_time?: string;
	end_time?: string;
	error_message?: string;
};

export type FindingItem = {
	id: number;
	pass: string;
	aspect: string;
	severity: string;
	finding_type: string;
	title: string;
	description: string;
	evidence?: string;
	location?: string;
	suggestion?: string;
	confidence: number;
	review_status: string;
};

export async function listAspects(): Promise<AspectInfo[]> {
	const res = await fetch(`${BASE}/aspects`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load aspects');
	return data.aspects;
}

export async function listTiers(): Promise<TierInfo[]> {
	const res = await fetch(`${BASE}/tiers`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load tiers');
	return data.tiers;
}

export async function submitRequest(input: SubmitInput): Promise<SubmitResult> {
	const res = await fetch(`${BASE}/requests`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
	const data = await res.json();
	if (!data.status) {
		throw new Error(data.error_msg || 'Failed to submit review request');
	}
	return {
		request_id: data.request_id,
		status: data.status,
		review_run_id: data.review_run_id
	};
}

export async function getRequest(
	id: number
): Promise<{ request: RequestStatus; findings: FindingItem[] }> {
	const res = await fetch(`${BASE}/requests/${id}`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load request');
	return { request: data.request, findings: data.findings || [] };
}

export async function getReport(id: number): Promise<any> {
	const res = await fetch(`${BASE}/reports/${id}`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load report');
	return data.report;
}

export async function updateFinding(
	id: number,
	review_status: string,
	reviewed_by?: string
): Promise<void> {
	const res = await fetch(`${BASE}/findings/${id}`, {
		method: 'PATCH',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ review_status, reviewed_by })
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to update finding');
}

export async function stopRequest(id: number): Promise<void> {
	const res = await fetch(`${BASE}/requests/${id}/stop`, {
		method: 'POST',
		credentials: 'same-origin'
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to stop request');
}
