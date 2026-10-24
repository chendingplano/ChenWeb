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
export type QuestionRequest = Pick<QuestionRun, 'input_record_id' | 'purpose' | 'description' | 'one_hop_count' | 'two_hop_count' | 'multi_hop_count' | 'total_one_hop_count' | 'total_two_hop_count' | 'total_multi_hop_count'>;

async function request<T>(url: string, options: RequestInit = {}): Promise<T> {
	const response = await fetch(url, { credentials: 'same-origin', ...options });
	if (!response.ok) throw new Error(String(response.status));
	return response.json();
}
export function createQuestionRun(payload: QuestionRequest, signal?: AbortSignal) {
	return request<{ run: QuestionRun }>(BASE, {
		method: 'POST', headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload), signal
	});
}
export function listQuestionRuns(recordID?: number, signal?: AbortSignal) {
	return request<{ runs: QuestionRun[] }>(recordID ? `${BASE}?input_record_id=${recordID}` : BASE, { signal });
}
export function getQuestionRun(runID: number, signal?: AbortSignal) {
	return request<{ run: QuestionRun; questions: GeneratedQuestion[] }>(`${BASE}/${runID}`, { signal });
}
