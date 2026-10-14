import { ScoreAPIError, type GoldRun, type ScoreRun } from './metric-score-client';

const BASE = '/api/v1/kb/reference-scores';

export { goldRunKey, ScoreAPIError, type GoldRun } from './metric-score-client';

export type ReferenceScoreSummary = {
	score: number;
	soft_precision: number;
	soft_recall: number;
	precision: number;
	recall: number;
	gold_rows: number;
	predicted_rows: number;
	matched: number;
};

// A pass/fail check is boolean; `relationships` earns its set overlap (0..1); null = not applicable.
export type ReferenceCheck = boolean | number | null;

export type ReferenceScore = {
	score: number;
	main: ReferenceScoreSummary;
	groups: Record<string, ReferenceScoreSummary>;
	field_accuracy: Record<string, number | null>;
	pairs: {
		gold: string;
		pred: string;
		note?: string;
		checks: Record<string, ReferenceCheck>;
		credit: number;
	}[];
	missed: { gold: string; note?: string }[];
	false_positives: { pred: string; cause: string; note?: string }[];
	overrides: unknown[];
};

export type ReferenceRow = Record<string, unknown> & { ref_id: string };

export type ReferenceScoreRun = Omit<ScoreRun, 'score' | 'input'> & {
	score?: ReferenceScore;
	input?: {
		provenance?: Record<string, unknown>;
		gold_run: GoldRun & Record<string, unknown>;
		extraction: Record<string, unknown>;
		own_identifier?: { gold?: string; attached?: string[]; result?: string };
		source?: Record<string, unknown>;
		warnings: string[];
		gold: ReferenceRow[];
		predictions: ReferenceRow[];
	};
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(BASE + path, { credentials: 'same-origin', ...init });
	const data = await res.json().catch(() => null);
	if (!res.ok || data?.status === false)
		throw new ScoreAPIError(data?.error_code ?? 'request_failed', res.status);
	return data as T;
}

export async function referenceScoreModels(): Promise<string[]> {
	return (await request<{ models: string[] }>('/models')).models ?? [];
}

export async function referenceGoldRuns(recordId: number): Promise<GoldRun[]> {
	return (await request<{ runs: GoldRun[] }>(`/gold-runs?record_id=${recordId}`)).runs ?? [];
}

export async function referenceScoreHistory(
	recordId: number | undefined,
	offset: number
): Promise<{ runs: ReferenceScoreRun[]; total: number }> {
	const params = new URLSearchParams({ limit: '20', offset: String(offset) });
	if (recordId) params.set('record_id', String(recordId));
	return request('?' + params);
}

export async function referenceScoreDetail(id: number): Promise<ReferenceScoreRun> {
	return (await request<{ run: ReferenceScoreRun }>(`/${id}`)).run;
}

export async function startReferenceScore(
	recordId: number,
	model: string,
	lang: string,
	gold?: GoldRun
): Promise<ReferenceScoreRun> {
	return (
		await request<{ run: ReferenceScoreRun }>('', {
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

export function referenceScoreArtifact(id: number, kind: string): string {
	return `${BASE}/${id}/artifacts/${kind}`;
}
