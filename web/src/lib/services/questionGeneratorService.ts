const BASE = '/api/v1/kb/question-generator/runs';

export type QuestionRun = {
	id: number;
	input_record_id: number;
	purpose: string;
	description: string;
	one_hop_count: number;
	two_hop_count: number;
	multi_hop_count: number;
	total_one_hop_count: number;
	total_two_hop_count: number;
	total_multi_hop_count: number;
	generated_one_hop_count: number;
	generated_two_hop_count: number;
	generated_multi_hop_count: number;
	documents_processed: number;
	lang: 'mixed' | 'en' | 'zh-cn';
	status: 'running' | 'done' | 'failed';
	error_msg: string;
	model_name: string;
	prompt_name: string;
	source_sha256?: string;
	created_at: string;
	finished_at?: string;
};
export type GeneratedQuestion = {
	purpose: string;
	created_at: string;
	prompt_name: string;
	model_name: string;
	id: number;
	run_id: number;
	input_record_id: number;
	lang: 'en' | 'zh-cn';
	metric_id: string;
	metric_snapshot: Record<string, unknown>;
	set_no: number;
	source_sha256: string;
	hop_type: 'one_hop' | 'two_hop' | 'multi_hop';
	question: string;
	answer: string;
	reasoning_steps: string[];
	step_line_ids?: number[][];
	evidence: { line_id: number; text: string }[];
};
export type QuestionRequest = {
	input_record_id: number;
	purpose: 'search_metrics';
	description: string;
	metrics_per_document?: number;
	total_metrics?: number;
	num_questions_to_generate?: number;
	confirmed_available_questions?: number;
};

export class QuestionCapacityError extends Error {
	constructor(
		public availableMetrics: number,
		public availableQuestions: number,
		public confirmationRequired: boolean
	) {
		super('question generation capacity');
	}
}

async function request<T>(url: string, options: RequestInit = {}): Promise<T> {
	const response = await fetch(url, { credentials: 'same-origin', ...options });
	if (response.status === 409) {
		const capacity = await response.json();
		throw new QuestionCapacityError(
			capacity.available_metrics,
			capacity.available_questions,
			capacity.confirmation_required
		);
	}
	if (!response.ok) throw new Error(String(response.status));
	return response.json();
}
export function createQuestionRun(payload: QuestionRequest, signal?: AbortSignal) {
	return request<{ run: QuestionRun }>(BASE, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload),
		signal
	});
}
export function listQuestionRuns(recordID?: number, signal?: AbortSignal) {
	return request<{ runs: QuestionRun[] }>(recordID ? `${BASE}?input_record_id=${recordID}` : BASE, {
		signal
	});
}
export function listQuestionJobs(page: number, pageSize: number, signal?: AbortSignal) {
	const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
	return request<{ runs: QuestionRun[]; total: number; page: number; page_size: number }>(
		`${BASE}?${params}`,
		{ signal }
	);
}
export function getQuestionRun(runID: number, signal?: AbortSignal) {
	return request<{ run: QuestionRun; questions: GeneratedQuestion[] }>(`${BASE}/${runID}`, {
		signal
	});
}

export type QuestionFilters = {
	purpose: string;
	run_id: string;
	input_record_id: string;
	lang: string;
	created_from: string;
	created_to: string;
};
export type QuestionFilterOptions = {
	purposes: string[];
	run_ids: number[];
	record_ids: number[];
	languages: string[];
};
export function listQuestions(
	filters: QuestionFilters,
	page: number,
	pageSize: number,
	signal?: AbortSignal
) {
	const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
	for (const [key, value] of Object.entries(filters)) {
		if (value) params.set(key, key.startsWith('created_') ? new Date(value).toISOString() : value);
	}
	return request<{
		questions: GeneratedQuestion[];
		total: number;
		page: number;
		page_size: number;
	}>(`/api/v1/kb/question-generator/questions?${params}`, { signal });
}
export function getQuestionFilters(signal?: AbortSignal) {
	return request<QuestionFilterOptions>('/api/v1/kb/question-generator/question-filters', {
		signal
	});
}

export function archiveQuestions(ids: number[], signal?: AbortSignal) {
	return request<{ archived_count: number }>('/api/v1/kb/question-generator/questions/archive', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ ids }),
		signal
	});
}
