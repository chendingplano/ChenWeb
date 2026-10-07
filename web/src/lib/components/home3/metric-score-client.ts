const BASE = '/api/v1/kb/metric-scores';
export type GoldRun = {
	skill_version: string;
	model_name: string;
	benchmark_run_id: string;
	rows: number;
};
export function goldRunKey(run: GoldRun): string {
	return JSON.stringify([run.skill_version, run.model_name, run.benchmark_run_id]);
}
export type ScoreSummary = {
	score: number;
	soft_precision: number;
	soft_recall: number;
	precision: number;
	recall: number;
	gold_rows: number;
	predicted_rows: number;
	matched: number;
};
export type MetricScore = {
	score: number;
	main: ScoreSummary;
	without_test_parameters: ScoreSummary | null;
	field_accuracy: Record<string, number | null>;
	pairs: {
		gold: string;
		pred: string;
		note?: string;
		checks: Record<string, boolean | null>;
		credit: number;
	}[];
	missed: { gold: string; note?: string }[];
	false_positives: { pred: string; cause: string; note?: string }[];
	overrides: unknown[];
};
export type ScorerInput = {
	provenance?: Record<string, unknown>;
	gold_run: GoldRun & Record<string, unknown>;
	extraction: Record<string, unknown>;
	source?: Record<string, unknown>;
	warnings: string[];
	gold: Record<string, unknown>[];
	predictions: Record<string, unknown>[];
};
export type ScoreRun = {
	id: number;
	input_record_id: number;
	title: string;
	lang: string;
	status: 'running' | 'done' | 'failed';
	model_name: string;
	prompt_name: string;
	created_by: string;
	created_at: string;
	finished_at?: string;
	error_msg?: string;
	error_code?: string;
	input?: ScorerInput;
	matches?: unknown;
	score?: MetricScore;
	report?: string;
};
export class ScoreAPIError extends Error {
	constructor(
		public code: string,
		public status: number
	) {
		super(code);
	}
}
async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(BASE + path, { credentials: 'same-origin', ...init });
	const data = await res.json().catch(() => null);
	if (!res.ok || data?.status === false)
		throw new ScoreAPIError(data?.error_code ?? 'request_failed', res.status);
	return data as T;
}
export async function scoreModels(): Promise<string[]> {
	return (await request<{ models: string[] }>('/models')).models ?? [];
}
export async function goldRuns(recordId: number): Promise<GoldRun[]> {
	return (await request<{ runs: GoldRun[] }>(`/gold-runs?record_id=${recordId}`)).runs ?? [];
}
export async function scoreHistory(
	recordId: number | undefined,
	offset: number
): Promise<{ runs: ScoreRun[]; total: number }> {
	const params = new URLSearchParams({ limit: '20', offset: String(offset) });
	if (recordId) params.set('record_id', String(recordId));
	return request('?' + params);
}
export async function scoreDetail(id: number): Promise<ScoreRun> {
	return (await request<{ run: ScoreRun }>(`/${id}`)).run;
}
export async function startScore(
	recordId: number,
	model: string,
	lang: string,
	gold?: GoldRun
): Promise<ScoreRun> {
	return (
		await request<{ run: ScoreRun }>('', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				record_id: recordId,
				model,
				lang,
				...(gold
					? {
							gold_version: gold.skill_version,
							gold_model: gold.model_name,
							gold_run_id: gold.benchmark_run_id
						}
					: {})
			})
		})
	).run;
}
export function scoreArtifact(id: number, kind: string): string {
	return `${BASE}/${id}/artifacts/${kind}`;
}
