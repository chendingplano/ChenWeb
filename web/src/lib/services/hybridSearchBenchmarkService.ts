const BASE = '/api/v1/kb/hybrid-search-benchmark/runs';

export type BenchmarkMode = 'lexical' | 'semantic' | 'hybrid';

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
};
// Search settings a run can choose; the server's defaults are the live search configuration.
export type HybridBenchmarkOptions = {
	query_keywords: boolean;
	lexical_weight: number;
	semantic_weight: number;
	candidate_limit: number;
	parallelism: number;
};

async function request<T>(url: string, options: RequestInit = {}): Promise<T> {
	const response = await fetch(url, { credentials: 'same-origin', ...options });
	if (!response.ok) throw new Error(String(response.status));
	return response.json();
}
export function startBenchmarkRun(
	payload: { purpose: string; description: string } & HybridBenchmarkOptions,
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
