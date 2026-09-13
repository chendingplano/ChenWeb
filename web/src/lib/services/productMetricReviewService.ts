// Typed client for the Product Metric Reviewer backend
// (openspec change product-metric-reviewer, Phase 4 endpoints under
// /api/v1/kb/product-profiles and /api/v1/kb/product-reviews).
//
// Every call returns the decoded JSON body and throws Error(error_msg) on a
// non-2xx response or a `{ status: false }` envelope — the same contract the
// rest of the ChenWeb services use.

const BASE = '/api/v1/kb';

// ── shared shapes ───────────────────────────────────────────────────────────

export type NodeKind = 'product' | 'module' | 'part' | 'aspect';
export type NodeOrigin = 'llm_proposed' | 'graph_expanded' | 'user_added';
export type NodeStatus = 'proposed' | 'accepted' | 'rejected';
export type Grounding = 'object_node' | 'keyword_concept' | 'ontology_term' | 'ungrounded';
export type ProfileStatus = 'draft' | 'ready';
export type Tier = 'direct' | 'part' | 'aspect' | 'document_scope';
export type RunStatus = 'pending' | 'running' | 'completed' | 'failed';

export type SourceRef = { kind: string; id: number; detail: string };

export type ProfileNode = {
	id: number;
	profile_id: number;
	parent_node_id: number | null;
	node_kind: NodeKind;
	label: string;
	label_en: string;
	aliases: string[];
	depth: number;
	origin: NodeOrigin;
	status: NodeStatus;
	confidence: number;
	rationale: string;
	object_id: string;
	concept_id: string;
	term_id: string;
	grounding: Grounding;
	reconcile_status: string;
	aspect_key: string;
	relation_types: string[];
	match_mode: string;
	source_refs: SourceRef[];
};

/** A node grounded to an object whose reconcile status needs a reviewer's eye. */
export function needsReconcileReview(n: ProfileNode): boolean {
	return n.reconcile_status === 'ambiguous' || n.reconcile_status === 'pending_review';
}

export type Profile = {
	id: number;
	tenant_id: string;
	name: string;
	product_description: string;
	keywords: string[];
	notes: string;
	version: number;
	status: ProfileStatus;
	truncated: boolean;
	truncated_count: number;
	/** Kept `kb.product_drawings` row associated with this profile, if any
	 *  (spec: product-review-results-layout). */
	drawing_id?: number | null;
	created_at: string;
	updated_at: string;
};

export type AspectEntry = {
	aspect_key: string;
	name: string;
	description: string;
	relation_types: string[];
	match_mode: string;
};

export type ReviewRequest = {
	id: number;
	tenant_id: string;
	profile_id: number;
	profile_version: number;
	artifact_types: string[];
	filters: Record<string, unknown>;
	notes: string;
	requester: string;
	created_at: string;
	updated_at: string;
};

export type NodeCoverage = {
	node_id: number;
	label: string;
	kind: NodeKind;
	grounding: Grounding;
	artifact_count: number;
	document_count: number;
};

export type GapEntry = { node_id: number; label: string; kind: NodeKind; grounding: Grounding };

export type RunReport = {
	profile_id: number;
	profile_version: number;
	artifact_types: string[];
	coverage: NodeCoverage[];
	gaps: GapEntry[];
	attributed_count: number;
	document_scope_count: number;
	scoped_document_count: number;
	result_count: number;
	truncated_count: number;
	per_document_truncated?: Record<string, number>;
	generated_at: string;
};

export type ReviewRun = {
	id: number;
	request_id: number;
	run_number: number;
	status: RunStatus;
	started_at: string | null;
	finished_at: string | null;
	result_count: number;
	attributed_count: number;
	document_scope_count: number;
	scoped_document_count: number;
	truncated_count: number;
	report_json: RunReport | Record<string, never>;
	report_md: string;
	error_message: string;
	created_at: string;
	updated_at: string;
};

export type ResultRow = {
	artifact_type: string;
	artifact_id: string;
	source_row_id: number;
	input_record_id: number;
	node_id: number | null;
	tier: Tier;
	score: number;
	paths: string[];
	inclusion_reason: string;
	source_line_spans: unknown;
	primary_label: string;
};

export type ObjectName = {
	object_id: string;
	name: string;
	name_en: string;
};

export type MetricDetail = {
	artifact_id: string;
	metric_name: string;
	subject: string;
	value: string;
	threshold: string;
	unit: string;
	frequency: string;
	class: string;
	data_type: string;
	range_type: string;
};

export type ScopedDocument = {
	input_record_id: number;
	fused_score: number;
	matching_node_ids: number[];
	matching_paths: string[];
	match_reasons: string[];
	doc_kind: string;
};

export type ArtifactRef = { artifact_type: string; artifact_id: string; tier: Tier };
export type TierChange = { artifact_type: string; artifact_id: string; from: Tier; to: Tier };

export type RunDiff = {
	added: ArtifactRef[];
	removed: ArtifactRef[];
	retiered: TierChange[];
	profile_version_changed: boolean;
	previous_profile_version: number;
	current_profile_version: number;
	previous_run_number: number;
};

// ── transport ───────────────────────────────────────────────────────────────

async function call<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(`${BASE}${path}`, { credentials: 'same-origin', ...init });
	const data = await res.json().catch(() => null);
	if (!res.ok || !data || data.status === false) {
		const msg =
			(data && (data.error_msg as string)) ||
			(data && (data.error_code as string)) ||
			`request to ${path} failed (${res.status})`;
		throw new Error(msg);
	}
	return data as T;
}

function jsonBody(body: unknown): RequestInit {
	return {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	};
}

// ── profiles ────────────────────────────────────────────────────────────────

export function createProfile(input: {
	name: string;
	product_description?: string;
	keywords?: string[];
	notes?: string;
	tenant_id?: string;
}): Promise<{ status: true; profile: Profile }> {
	return call('/product-profiles', jsonBody(input));
}

export function getProfile(
	id: number
): Promise<{ status: true; profile: Profile; nodes: ProfileNode[] }> {
	return call(`/product-profiles/${id}`);
}

/** A profile plus its latest review request/run, for the intake page's
 *  past-reviews list (spec: product-review-history-list). Both are absent
 *  when the profile has never had a review started. */
export type ProfileSummary = Profile & {
	latest_request_id?: number;
	latest_run_id?: number;
	latest_run_status?: string;
	latest_run_finished_at?: string;
};

export function listProfiles(limit?: number): Promise<{ status: true; profiles: ProfileSummary[] }> {
	const q = limit ? `?limit=${limit}` : '';
	return call(`/product-profiles${q}`);
}

export function buildProfile(
	id: number,
	seedExcerpts: string[] = []
): Promise<{ status: true; nodes: ProfileNode[] }> {
	return call(`/product-profiles/${id}/build`, jsonBody({ seed_excerpts: seedExcerpts }));
}

export function setProfileReady(
	id: number,
	ready = true
): Promise<{ status: true; profile_status: ProfileStatus }> {
	return call(`/product-profiles/${id}/ready`, jsonBody({ ready }));
}

/** Associates a kept product-drawings row with a profile (spec:
 *  product-review-results-layout); pass 0 to clear the association. */
export function setProfileDrawing(id: number, drawingId: number): Promise<{ status: true }> {
	return call(`/product-profiles/${id}/drawing`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ drawing_id: drawingId })
	});
}

export function addNode(
	profileId: number,
	node: {
		parent_node_id?: number | null;
		node_kind: NodeKind;
		label: string;
		label_en?: string;
		aliases?: string[];
	}
): Promise<{ status: true; node_id: number }> {
	return call(`/product-profiles/${profileId}/nodes`, jsonBody(node));
}

export function updateNode(
	profileId: number,
	nodeId: number,
	patch: {
		label?: string;
		label_en?: string;
		aliases?: string[];
		node_kind?: NodeKind;
		status?: NodeStatus;
		parent_node_id?: number;
	}
): Promise<{ status: true }> {
	return call(`/product-profiles/${profileId}/nodes/${nodeId}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(patch)
	});
}

export function deleteNode(profileId: number, nodeId: number): Promise<{ status: true }> {
	return call(`/product-profiles/${profileId}/nodes/${nodeId}`, { method: 'DELETE' });
}

export function acceptNode(profileId: number, nodeId: number) {
	return updateNode(profileId, nodeId, { status: 'accepted' });
}
export function rejectNode(profileId: number, nodeId: number) {
	return updateNode(profileId, nodeId, { status: 'rejected' });
}

// ── self-service intake ─────────────────────────────────────────────────────
// POST /product-reviews/intake (openspec change product-review-intake): one
// call that either surfaces an existing profile for the same product name
// (duplicate: true — offer "view results" or "re-run") or creates, builds,
// and runs a fresh review in one step (duplicate: false — a run is returned).

export type IntakeResponse = {
	status: true;
	duplicate: boolean;
	profile: Profile;
	run?: ReviewRun;
	latest_request_id?: number;
	latest_run?: ReviewRun;
};

export function startProductReviewIntake(input: {
	name: string;
	product_description?: string;
	keywords?: string[];
	notes?: string;
	tenant_id?: string;
	resume_profile_id?: number;
	model?: 'Qwen' | 'OpenAI';
}): Promise<IntakeResponse> {
	return call('/product-reviews/intake', jsonBody(input));
}

// ── aspect vocabulary ───────────────────────────────────────────────────────

export function listAspects(locale?: string): Promise<{ status: true; aspects: AspectEntry[] }> {
	const q = locale ? `?lang=${encodeURIComponent(locale)}` : '';
	return call(`/product-reviews/aspects${q}`);
}

// ── reviews & runs ──────────────────────────────────────────────────────────

export function createReview(input: {
	profile_id: number;
	artifact_types?: string[];
	filters?: Record<string, unknown>;
	notes?: string;
}): Promise<{ status: true; run: ReviewRun }> {
	return call('/product-reviews', jsonBody(input));
}

export function listReviews(
	profileId?: number
): Promise<{ status: true; requests: ReviewRequest[] }> {
	const q = profileId ? `?profile_id=${profileId}` : '';
	return call(`/product-reviews${q}`);
}

export function getReview(
	requestId: number
): Promise<{ status: true; request: ReviewRequest; runs: ReviewRun[] }> {
	return call(`/product-reviews/${requestId}`);
}

export function rerunReview(requestId: number): Promise<{ status: true; run: ReviewRun }> {
	return call(`/product-reviews/${requestId}/rerun`, { method: 'POST' });
}

export function getRun(runId: number): Promise<{ status: true; run: ReviewRun }> {
	return call(`/product-reviews/runs/${runId}`);
}

export type ResultQuery = {
	node_id?: number;
	tier?: Tier;
	exclude_tier?: Tier[];
	path?: string;
	input_record_id?: number;
	artifact_type?: string;
};

function resultQueryString(q: ResultQuery = {}): string {
	const p = new URLSearchParams();
	if (q.node_id != null) p.set('node_id', String(q.node_id));
	if (q.tier) p.set('tier', q.tier);
	if (q.exclude_tier?.length) p.set('exclude_tier', q.exclude_tier.join(','));
	if (q.path) p.set('path', q.path);
	if (q.input_record_id != null) p.set('input_record_id', String(q.input_record_id));
	if (q.artifact_type) p.set('artifact_type', q.artifact_type);
	const s = p.toString();
	return s ? `?${s}` : '';
}

export function getRunResults(
	runId: number,
	query?: ResultQuery
): Promise<{ status: true; results: ResultRow[]; count: number }> {
	return call(`/product-reviews/runs/${runId}/results${resultQueryString(query)}`);
}

export function getRunDocuments(
	runId: number
): Promise<{ status: true; documents: ScopedDocument[]; count: number }> {
	return call(`/product-reviews/runs/${runId}/documents`);
}

export function getRunDiff(runId: number): Promise<{ status: true; diff: RunDiff }> {
	return call(`/product-reviews/runs/${runId}/diff`);
}

export function getMetricDetail(
	artifactId: string
): Promise<{ status: true; metric: MetricDetail }> {
	return call(`/product-reviews/artifacts/${encodeURIComponent(artifactId)}/metric`);
}

export function getObjectNames(
	objectIds: string[]
): Promise<{ status: true; objects: ObjectName[] }> {
	return call(`/product-reviews/objects?ids=${objectIds.map(encodeURIComponent).join(',')}`);
}

/** URL for the CSV export — the sandbox blocks script-driven downloads, so hand
 *  this to an <a href> the viewer clicks. */
export function runExportUrl(runId: number, query?: ResultQuery): string {
	return `${BASE}/product-reviews/runs/${runId}/export${resultQueryString(query)}`;
}
