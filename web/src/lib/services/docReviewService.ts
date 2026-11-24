const BASE = '/api/v1/doc-review';

export type AspectInfo = {
	name: string;
	group: string; // "P1".."P6"
	label: string;
	priority: string;
	description: string;
	default_model: string;
	is_tool_use: boolean;
	checked: boolean; // initial checked state from doc-review.local.toml
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
	review_depth: 1 | 2 | 3;
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
	run_id: number;
	status: string;
	review_run_id?: string;
	report_id?: number;
};

export type RequestStatus = {
	id: number;
	input_record_id: number;
	latest_run_id?: number;
	review_run_id?: string;
	tier: string;
	aspects: string[];
	review_depth: number;
	reference_docs?: ReferenceDoc[];
	notes?: string;
	model_overrides?: Record<string, { model_ref: string }>;
	requester_name: string;
	requester_id: number;
	status: string;
	run_status?: string;
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
	progress: number;
	finding_count: number;
	error_message?: string;
	start_time?: string;
	end_time?: string;
};

export type ReviewPackageInfo = {
	key: string;
	label: string;
};

export type ReviewerPackageInfo = {
	key: string;
	label: string;
	aspect_names: string[];
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
	artifact_id?: string;
	run_id?: number;
	metadata?: unknown;
	reference_doc?: unknown;
	// Summary fields of a compliance finding (openspec compliance-review-storage). Compliance
	// findings arrive without description, evidence, suggestion or metadata; their details
	// load on demand with getFindingDetails.
	line_start?: number;
	line_end?: number;
	proc_status?: string;
	storage_version?: number;
	counts?: ComplianceCounts;
};

export type ComplianceCounts = {
	activities: number;
	search_results: number;
	selected: number;
	relevant: number;
	judged: number;
	verdicts: Record<string, number>;
};

// listAspects loads the review aspects and package (group) list. Labels and
// descriptions are resolved server-side for `locale` (falling back to name_en
// when a translation is missing); pass the current UI locale to get localized
// text. `packages` drives the per-group headers on the selection page.
export async function listAspects(
	locale?: string
): Promise<{ aspects: AspectInfo[]; packages: ReviewPackageInfo[]; reviewer_packages: ReviewerPackageInfo[] }> {
	const query = locale ? `?lang=${encodeURIComponent(locale)}` : '';
	const res = await fetch(`${BASE}/aspects${query}`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load aspects');
	return {
		aspects: data.aspects || [],
		packages: data.packages || [],
		reviewer_packages: data.reviewer_packages || [{ key: 'all', label: 'All', aspect_names: [] }],
	};
}

export async function listTiers(): Promise<TierInfo[]> {
	const res = await fetch(`${BASE}/tiers`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load tiers');
	return data.tiers;
}

export async function listLanguages(): Promise<string[]> {
	const res = await fetch(`${BASE}/languages`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load languages');
	return data.languages || ['en'];
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
		run_id: data.run_id,
		status: data.status,
		review_run_id: data.review_run_id,
		report_id: data.report_id,
	};
}

export type RequestFindingsQuery = {
	language?: string;
	pass?: string;
	aspect?: string;
	runId?: number;
};

export async function getRequest(
	id: number,
	query: RequestFindingsQuery = {}
): Promise<{ request: RequestStatus; findings: FindingItem[]; aspect_statuses: AspectStatus[]; packages: ReviewPackageInfo[] }> {
	const params = new URLSearchParams();
	if (query.language && query.language !== 'en') params.set('language', query.language);
	if (query.pass) params.set('pass', query.pass);
	if (query.aspect) params.set('aspect', query.aspect);
	if (query.runId) params.set('run_id', String(query.runId));
	const suffix = params.toString() ? `?${params.toString()}` : '';
	const res = await fetch(`${BASE}/requests/${id}${suffix}`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load request');
	return { request: data.request, findings: data.findings || [], aspect_statuses: data.aspect_statuses || [], packages: data.packages || [] };
}

// ── Finding Details panel: doc-review-logs lookup ──────────────────────────

export type DocReviewLogRow = {
	id: number;
	input_record_id: number | null;
	run_id: number | null;
	pass: string | null;
	aspect: string;
	unit_type: string;
	unit_key: string;
	unit_location: unknown;
	matched_units: unknown;
	findings: unknown;
	outcome: string;
	detail: unknown;
	create_time: string;
};

// Finds the kb.doc_review_logs row for a specific artifact within a specific
// run. Log rows are keyed "<artifact_id>#<suffix>" (e.g. "244_mtc_1#31082"),
// not the bare artifact id, so this matches on that exact artifact_id
// followed by a '#' boundary (or a bare exact match, for unit_keys with no
// suffix). GET /api/v1/kb/doc-review-logs matches unit_key with ILIKE '%...%'
// (substring), so this filters the response client-side rather than relying
// on the server-side match alone (which would also return e.g. "..._mtc_10#...").
export async function getDocReviewLogsFor(unitKey: string, runId: number): Promise<DocReviewLogRow | null> {
	const params = new URLSearchParams({ unit_key: unitKey, run_id: String(runId), page_size: '50' });
	const res = await fetch(`/api/v1/kb/doc-review-logs?${params}`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load doc review logs');
	const rows = (data.results || []) as DocReviewLogRow[];
	return rows.find((row) => row.unit_key === unitKey || row.unit_key.startsWith(`${unitKey}#`)) ?? null;
}

// getReport loads a report; view 'summary' leaves out its finding arrays, which the report
// view loads through getRequest and listRunFindings instead.
export async function getReport(id: number, view?: 'summary'): Promise<any> {
	const res = await fetch(`${BASE}/reports/${id}${view ? `?view=${view}` : ''}`, { credentials: 'same-origin' });
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

export type TranslateFindingResult = {
	finding: FindingItem;
	translated: boolean;
	needs_confirmation: boolean;
};

export async function translateFinding(
	id: number,
	language: string,
	confirm = false
): Promise<TranslateFindingResult> {
	const res = await fetch(`${BASE}/findings/${id}/translate`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ language, confirm })
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to translate finding');
	return {
		finding: data.finding,
		translated: !!data.translated,
		needs_confirmation: !!data.needs_confirmation
	};
}

// ── DR16: finding actions (auto-fix / edit / accept / delete) ─────────────────

export type LineEdit = { line_no: number; content: string };

export type AutoFixResult = {
	fixable: boolean;
	message?: string;
	model_name?: string;
	elapsed_ms?: number;
	original?: LineEdit[];
	corrected?: LineEdit[];
};

// Runs the LLM and returns a preview of the proposed fix. Nothing is written
// to disk until applyAutoFix is called with the confirmed corrected lines.
export async function autoFixFinding(id: number): Promise<AutoFixResult> {
	const res = await fetch(`${BASE}/findings/${id}/auto-fix`, {
		method: 'POST',
		credentials: 'same-origin'
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Auto-fix failed');
	return data.result as AutoFixResult;
}

// Commits the user-confirmed corrected lines, writing them to disk and marking
// the finding as fixed. Returns the number of lines actually changed.
export async function applyAutoFix(
	id: number,
	corrected: LineEdit[],
	modelName: string
): Promise<number> {
	const res = await fetch(`${BASE}/findings/${id}/auto-fix/apply`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ corrected, model_name: modelName })
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Apply auto-fix failed');
	return data.changed as number;
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

// Re-runs an existing review request from scratch and returns the new run id.
export async function restartRequest(id: number): Promise<{ requestId: number; runId: number }> {
	const res = await fetch(`${BASE}/requests/${id}/restart`, {
		method: 'POST',
		credentials: 'same-origin'
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to restart request');
	return {
		requestId: data.request_id,
		runId: data.run_id
	};
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

export type ReviewRunListItem = {
	run_id: number;
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

export type ReviewRunListFilter = {
	runId?: string;
	title?: string;
	requester?: string;
	tier?: string;
	status?: string;
	createStart?: string;
	createEnd?: string;
	limit?: number;
};

// Lists all document-review runs matching the filter (newest first).
export async function listReviewRuns(filter: ReviewRunListFilter = {}): Promise<ReviewRunListItem[]> {
	const params = new URLSearchParams();
	if (filter.runId) params.set('run_id', filter.runId.trim());
	if (filter.title) params.set('title', filter.title.trim());
	if (filter.requester) params.set('requester', filter.requester.trim());
	if (filter.tier && filter.tier !== 'all') params.set('tier', filter.tier);
	if (filter.status && filter.status !== 'all') params.set('status', filter.status);
	if (filter.createStart) params.set('create_start', filter.createStart);
	if (filter.createEnd) params.set('create_end', filter.createEnd);
	if (filter.limit) params.set('limit', String(filter.limit));
	const qs = params.toString();
	const res = await fetch(`${BASE}/runs${qs ? `?${qs}` : ''}`, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to load review runs');
	return data.runs || [];
}

// ── Compliance review storage (openspec compliance-review-storage) ─────────────

export type Page<T> = { items: T[]; total?: number; next_cursor?: string };

async function getJSON(url: string, fallback: string): Promise<any> {
	const res = await fetch(url, { credentials: 'same-origin' });
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || fallback);
	return data;
}

function query(params: Record<string, string | number | undefined>): string {
	const q = new URLSearchParams();
	for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '') q.set(k, String(v));
	const s = q.toString();
	return s ? `?${s}` : '';
}

// listRunFindings returns one page of a run's finding summaries for an aspect.
export async function listRunFindings(
	runId: number,
	opts: { aspect: string; cursor?: string; limit?: number; excludeStatus?: string[] }
): Promise<Page<FindingItem>> {
	const data = await getJSON(
		`${BASE}/runs/${runId}/findings${query({ aspect: opts.aspect, cursor: opts.cursor, limit: opts.limit, exclude_status: opts.excludeStatus?.join(',') })}`,
		'Failed to load findings'
	);
	return { items: data.items || [], total: data.total ?? 0, next_cursor: data.next_cursor || undefined };
}

export type ComplianceActivitySummary = {
	activity_id: number;
	local_label: string;
	display_order: number;
	kind: string;
	label: string;
	line_start?: number;
	line_end?: number;
	source_artifact_type?: string;
	source_artifact_id?: string;
	document_level?: boolean;
};

export type ComplianceCandidateSummary = {
	id: number;
	activity_id: number;
	activity_label: string;
	local_label: string;
	candidate_type: string;
	candidate_id: string;
	candidate_record_id?: number;
	document_title?: string;
	relevance_answer?: string;
	relevance_score?: number;
	kept: boolean;
	status: string; // rejected | relevance_failed | pending | judgement_failed | judged
	verdict?: string;
	binding?: string;
	summary?: string;
	note?: string;
	display_order: number;
};

export type ComplianceCandidateDetail = ComplianceCandidateSummary & {
	finding_id: number;
	cosine?: number;
	error_message?: string;
	judgement_order?: number;
	evidence: Record<string, unknown>;
};

export type FindingDetails = {
	finding: FindingItem;
	content?: {
		description: string;
		evidence: string;
		suggestion: string;
		error_message?: string;
		canonical_language?: string;
		stage_models?: Record<string, string>;
		extra?: Record<string, unknown>;
		language?: string;
	};
	run_context?: { assessment_date?: string; schema_version: number; prompts?: Record<string, string>; models?: Record<string, string>; review_flags?: string[] } | null;
	activities?: Page<ComplianceActivitySummary>;
	candidates?: Page<ComplianceCandidateSummary>;
	legacy?: { description: string; evidence: string; suggestion: string; error_msg?: string };
};

export type CandidateFilter = 'judged' | 'all';

export async function getFindingDetails(id: number, opts: { language?: string; filter?: CandidateFilter } = {}): Promise<FindingDetails> {
	const data = await getJSON(`${BASE}/findings/${id}/details${query({ language: opts.language, filter: opts.filter })}`, 'Failed to load finding details');
	return data.details as FindingDetails;
}

export async function listFindingActivities(id: number, cursor?: string): Promise<Page<ComplianceActivitySummary>> {
	const data = await getJSON(`${BASE}/findings/${id}/activities${query({ cursor })}`, 'Failed to load activities');
	return { items: data.items || [], next_cursor: data.next_cursor || undefined };
}

export async function listFindingCandidates(id: number, filter: CandidateFilter, cursor?: string): Promise<Page<ComplianceCandidateSummary>> {
	const data = await getJSON(`${BASE}/findings/${id}/candidates${query({ filter, cursor })}`, 'Failed to load candidates');
	return { items: data.items || [], total: data.total ?? 0, next_cursor: data.next_cursor || undefined };
}

export async function getComplianceCandidate(id: number): Promise<ComplianceCandidateDetail> {
	const data = await getJSON(`${BASE}/compliance/candidates/${id}`, 'Failed to load candidate');
	return data.candidate as ComplianceCandidateDetail;
}

export type ComplianceSearchSummary = {
	id: number;
	activity_id: number;
	local_label: string;
	activity_label: string;
	ordinal: number;
	purpose: string;
	query?: string;
	reference?: string;
	cited_document?: string;
	keywords: string[];
	filters: Record<string, unknown>;
	result_limit: number;
	potentially_truncated: boolean;
	result_count: number;
};

export type ComplianceSearchResult = {
	rank: number;
	candidate_type: string;
	candidate_id: string;
	candidate_record_id?: number;
	document_title?: string;
	score?: number;
	diagnostics: Record<string, unknown>;
};

export async function listFindingSearches(id: number): Promise<ComplianceSearchSummary[]> {
	const data = await getJSON(`${BASE}/findings/${id}/searches`, 'Failed to load search history');
	return data.items || [];
}

export async function listSearchResults(searchId: number, cursor?: string): Promise<Page<ComplianceSearchResult>> {
	const data = await getJSON(`${BASE}/compliance/searches/${searchId}/results${query({ cursor })}`, 'Failed to load search results');
	return { items: data.items || [], next_cursor: data.next_cursor || undefined };
}

// exportFinding loads a finding's complete audit record, assembled on the server.
export async function exportFinding(id: number): Promise<unknown> {
	const res = await fetch(findingExportUrl(id), { credentials: 'same-origin' });
	if (!res.ok) {
		const data = await res.json().catch(() => ({}));
		throw new Error(data.error_msg || 'Failed to export finding');
	}
	return res.json();
}

export function findingExportUrl(id: number): string {
	return `${BASE}/findings/${id}/export`;
}
