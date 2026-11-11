const BASE = '/api/v1/kb/hybrid-search-benchmark/runs';

export type BenchmarkMode = 'lexical' | 'semantic' | 'hybrid' | 'effort';
export type SearchEffort = 'low' | 'medium' | 'high';

export type HybridBenchmarkRun = {
	id: number;
	purpose: string;
	description: string;
	status: 'running' | 'done' | 'failed';
	error_msg: string;
	question_count: number;
	done_count: number;
	lexical_backend: string;
	query_keywords: boolean;
	embedding_model: string;
	keywords_model: string;
	keywords_prompt: string;
	lexical_weight: number;
	semantic_weight: number;
	candidate_limit: number;
	parallelism: number;
	search_type: string;
	effort: SearchEffort;
	max_efforts: number;
	created_by: string;
	created_at: string;
	finished_at?: string | null;
	hybrid_r10?: number;
};
export type HybridBenchmarkScore = {
	mode: BenchmarkMode;
	group: 'all' | 'hop_type' | 'lang';
	value: string;
	count: number;
	r1: number;
	r5: number;
	r10: number;
	mrr: number;
};
export type HybridBenchmarkResult = {
	id: number;
	search_run_id: number;
	question_id: number | null;
	input_record_id: number;
	hop_type: 'one_hop' | 'two_hop' | 'multi_hop';
	lang: 'en' | 'zh-cn';
	question: string;
	target_artifact_id: string;
	target_artifact_type: string;
	keywords: string[];
	lexical_rank: number | null;
	semantic_rank: number | null;
	hybrid_rank: number | null;
	hybrid_top: string[];
	error_msg: string;
	embedding_ms: number | null;
	keywords_ms: number | null;
	lexical_ms: number | null;
	semantic_ms: number | null;
	hybrid_ms: number | null;
	effort_rank: number | null;
	effort_found: boolean | null;
	effort_attempts: number | null;
	effort_ms: number | null;
	effort_trace: EffortTrace | null;
};
// The effort loop's analysis and attempts (medium/high runs).
export type EffortTrace = {
	effort: SearchEffort;
	max_attempts: number;
	strategy: string;
	need: string;
	found: boolean;
	attempts: {
		queries: string[];
		found: boolean;
		matching_ids: string[];
		reason: string;
		error?: string;
		ms: number;
	}[];
	error?: string;
};
// Search settings a run can choose; the server's defaults are the live search configuration.
export type HybridBenchmarkOptions = {
	query_keywords: boolean;
	lexical_weight: number;
	semantic_weight: number;
	candidate_limit: number;
	parallelism: number;
	effort: SearchEffort;
};

async function request<T>(url: string, options: RequestInit = {}): Promise<T> {
	const response = await fetch(url, { credentials: 'same-origin', ...options });
	if (!response.ok) throw new Error(String(response.status));
	return response.json();
}
export function startBenchmarkRun(
	payload: {
		purpose: string;
		description: string;
		question_ids: number[];
		// "" = the purpose's own type, "all" = every type, or one artifact type.
		search_type: string;
	} & HybridBenchmarkOptions,
	signal?: AbortSignal
) {
	return request<{ run: HybridBenchmarkRun }>(BASE, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload),
		signal
	});
}
export function listBenchmarkRuns(signal?: AbortSignal) {
	return request<{
		runs: HybridBenchmarkRun[];
		purposes: string[];
		search_types: string[];
		defaults: HybridBenchmarkOptions;
	}>(BASE, { signal });
}
export function getBenchmarkRun(id: number, signal?: AbortSignal) {
	return request<{
		run: HybridBenchmarkRun;
		scores: HybridBenchmarkScore[];
		results: HybridBenchmarkResult[];
	}>(`${BASE}/${id}`, { signal });
}

// One result's trace (Analyze tab): the result itself, its run's lexical backend, and every query
// the question searched with the candidate lists of its hybrid search — per stage the first
// `limit` rows plus the target's rows.
export type TraceStage = 'lexical_partition' | 'lexical' | 'semantic' | 'fused';
export type TraceCandidate = {
	rank: number;
	partition_name: string;
	artifact_type: string;
	artifact_id: string;
	label: string;
	score: number;
	partition_rank: number | null;
	lexical_rank: number | null;
	semantic_rank: number | null;
};
export type TraceQuery = {
	id: number;
	seq: number;
	source: 'question' | 'effort';
	attempt: number | null;
	query_text: string;
	keywords: string[];
	error_msg: string;
	stages: Partial<Record<TraceStage, TraceCandidate[]>>;
	counts: Partial<Record<TraceStage, number>>;
};
export function getBenchmarkTrace(resultId: number, signal?: AbortSignal) {
	return request<{
		result: HybridBenchmarkResult;
		lexical_backend: string;
		limit: number;
		queries: TraceQuery[];
	}>(`/api/v1/kb/hybrid-search-benchmark/results/${resultId}/trace`, { signal });
}

// Questions of one purpose matching the "Tests to run" filters (all matches up to `limit`).
export type BenchmarkQuestionFilters = {
	purpose: string;
	created_from: string;
	created_to: string;
	run_id: string;
	hop_type: string;
	lang: string;
	input_record_id: string;
	question: string;
	answer: string;
	target_id: string;
};
export type BenchmarkQuestion = {
	id: number;
	run_id: number;
	input_record_id: number;
	hop_type: 'one_hop' | 'two_hop' | 'multi_hop';
	lang: 'en' | 'zh-cn';
	artifact_type: string;
	artifact_id: string;
	question: string;
	answer: string;
	created_at: string;
};
export function listBenchmarkQuestions(filters: BenchmarkQuestionFilters, signal?: AbortSignal) {
	const params = new URLSearchParams();
	for (const [key, value] of Object.entries(filters)) {
		const v = value.trim();
		if (v) params.set(key, key.startsWith('created_') ? new Date(v).toISOString() : v);
	}
	return request<{ questions: BenchmarkQuestion[]; total: number; limit: number }>(
		`/api/v1/kb/hybrid-search-benchmark/questions?${params}`,
		{ signal }
	);
}
