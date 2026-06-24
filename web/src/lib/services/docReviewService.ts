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
	report_id?: number;
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
	report_id?: number;
};

export type AspectStatus = {
	aspect: string;
	pass: string;
	status: string; // pending | running | success | failed
	finding_count: number;
	error_message?: string;
	start_time?: string;
	end_time?: string;
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
		review_run_id: data.review_run_id,
		report_id: data.report_id,
	};
}

export async function getRequest(
	id: number
): Promise<{ request: RequestStatus; findings: FindingItem[]; aspect_statuses: AspectStatus[] }> {
	const res = await fetch(`${BASE}/requests/${id}`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load request');
	return { request: data.request, findings: data.findings || [], aspect_statuses: data.aspect_statuses || [] };
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

// ── DR16: finding actions (auto-fix / edit / accept / delete) ─────────────────

export type LineEdit = { line_no: number; content: string };

export type AutoFixResult = {
	fixable: boolean;
	message?: string;
	original?: LineEdit[];
	corrected?: LineEdit[];
};

// Runs the LLM auto-fix for a finding. A resolved result with fixable=false means
// the caller should surface `message` to the user (e.g. unfixable / not configured).
export async function autoFixFinding(id: number): Promise<AutoFixResult> {
	const res = await fetch(`${BASE}/findings/${id}/auto-fix`, {
		method: 'POST',
		credentials: 'same-origin'
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Auto-fix failed');
	return data.result as AutoFixResult;
}

// Returns the current content of a finding's offending line(s) for the Edit Tool.
export async function getFindingLines(id: number): Promise<LineEdit[]> {
	const res = await fetch(`${BASE}/findings/${id}/lines`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load lines');
	return (data.lines || []) as LineEdit[];
}

// Saves user-edited line content back to the line-file. Returns the count changed.
export async function editFindingLines(id: number, lines: LineEdit[]): Promise<number> {
	const res = await fetch(`${BASE}/findings/${id}/edit`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ lines })
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to save edits');
	return data.changed ?? 0;
}

// Rebuilds the report JSON/markdown + PDF for a report id from current findings.
export async function regenerateReport(reportId: number): Promise<void> {
	const res = await fetch(`${BASE}/reports/${reportId}/regenerate`, {
		method: 'POST',
		credentials: 'same-origin'
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to regenerate report');
}

// Builds the Document Review Correction Report (Typst + PDF) from the recorded
// correction activities for a report. Returns the generated PDF file name (empty
// when PDF generation is disabled on the server).
export async function generateCorrectionReport(reportId: number): Promise<string> {
	const res = await fetch(`${BASE}/reports/${reportId}/correction-report`, {
		method: 'POST',
		credentials: 'same-origin'
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to generate correction report');
	return data.pdf_file || '';
}

export async function stopRequest(id: number): Promise<void> {
	const res = await fetch(`${BASE}/requests/${id}/stop`, {
		method: 'POST',
		credentials: 'same-origin'
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to stop request');
}

// ── DR15: live job monitor ───────────────────────────────────────────────────

export type ActiveJob = {
	request_id: number;
	input_record_id: number;
	review_run_id: string;
	tier: string;
	status: string;
	requester_name: string;
	doc_title?: string;
	create_time: string;
	start_time?: string;
	aspects: AspectStatus[];
};

// Returns every review job that still has at least one unfinished aspect.
export async function listActiveJobs(): Promise<ActiveJob[]> {
	const res = await fetch(`${BASE}/active`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load active jobs');
	return data.jobs || [];
}

// ── Request list + search ─────────────────────────────────────────────────────

export type RequestListItem = {
	request_id: number;
	input_record_id: number;
	doc_title: string;
	tier: string;
	status: string;
	requester_name: string;
	aspect_count: number;
	total_findings: number;
	report_id?: number;
	create_time: string;
	start_time?: string;
	end_time?: string;
};

export type RequestListFilter = {
	requestId?: string;
	title?: string;
	requester?: string;
	tier?: string;
	status?: string;
	createStart?: string;
	createEnd?: string;
	limit?: number;
};

// Lists all document-review requests matching the filter (newest first).
export async function listRequests(filter: RequestListFilter = {}): Promise<RequestListItem[]> {
	const params = new URLSearchParams();
	if (filter.requestId) params.set('request_id', filter.requestId.trim());
	if (filter.title) params.set('title', filter.title.trim());
	if (filter.requester) params.set('requester', filter.requester.trim());
	if (filter.tier && filter.tier !== 'all') params.set('tier', filter.tier);
	if (filter.status && filter.status !== 'all') params.set('status', filter.status);
	if (filter.createStart) params.set('create_start', filter.createStart);
	if (filter.createEnd) params.set('create_end', filter.createEnd);
	if (filter.limit) params.set('limit', String(filter.limit));
	const qs = params.toString();
	const res = await fetch(`${BASE}/requests${qs ? `?${qs}` : ''}`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load requests');
	return data.requests || [];
}
