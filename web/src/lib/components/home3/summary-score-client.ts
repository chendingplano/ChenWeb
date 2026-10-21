import { ScoreAPIError } from './metric-score-client';

const BASE = '/api/v1/kb/summary-scores';

export type SummaryGoldRun = {
	skill_version: string;
	model_name: string;
	benchmark_run_id: string;
	rows: number;
};
export type SummaryScoreRun = {
	id: number;
	input_record_id: number;
	title: string;
	algorithm: string;
	lang: string;
	status: 'running' | 'done' | 'failed';
	model_name: string;
	prompt_name: string;
	created_by: string;
	created_at: string;
	finished_at?: string;
	error_code?: string;
	input?: Record<string, any>;
	matches?: Record<string, any>;
	score?: Record<string, any>;
	report?: string;
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(BASE + path, { credentials: 'same-origin', ...init });
	const data = await response.json().catch(() => null);
	if (!response.ok || data?.status === false)
		throw new ScoreAPIError(data?.error_code ?? 'request_failed', response.status);
	return data as T;
}

export async function summaryScoreModels(): Promise<string[]> {
	return (await request<{ models: string[] }>('/models')).models ?? [];
}
export async function summaryScoreAlgorithms(recordId: number): Promise<string[]> {
	return (await request<{ algorithms: string[] }>(`/algorithms?record_id=${recordId}`)).algorithms ?? [];
}
export async function summaryGoldRuns(recordId: number): Promise<SummaryGoldRun[]> {
	return (await request<{ runs: SummaryGoldRun[] }>(`/gold-runs?record_id=${recordId}`)).runs ?? [];
}
export async function summaryScoreHistory(recordId?: number): Promise<{ runs: SummaryScoreRun[]; total: number }> {
	const params = new URLSearchParams({ limit: '50', offset: '0' });
	if (recordId) params.set('record_id', String(recordId));
	return request('?' + params);
}
export async function summaryScoreDetail(id: number): Promise<SummaryScoreRun> {
	return (await request<{ run: SummaryScoreRun }>(`/${id}`)).run;
}
export async function startSummaryScore(input: {
	record_id: number;
	algorithm: string;
	model: string;
	lang: string;
	gold?: SummaryGoldRun;
}): Promise<SummaryScoreRun> {
	return (await request<{ run: SummaryScoreRun }>('', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			record_id: input.record_id,
			algorithm: input.algorithm,
			model: input.model,
			lang: input.lang,
			...(input.gold ? { gold_version: input.gold.skill_version, gold_model: input.gold.model_name, gold_run_id: input.gold.benchmark_run_id } : {})
		})
	})).run;
}

export function summaryScoreArtifact(id: number, kind: string): string {
	return `${BASE}/${id}/artifacts/${kind}`;
}
