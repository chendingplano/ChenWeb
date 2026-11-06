import { ScoreAPIError, type GoldRun, type ScoreRun, type ScoreSummary } from './metric-score-client';

const BASE = '/api/v1/kb/provision-scores';

export { goldRunKey, ScoreAPIError, type GoldRun, type ScoreSummary } from './metric-score-client';

// A pass/fail check is boolean; `text` and `lines` earn partial credit (0..1); null = left out.
export type ProvisionCheck = boolean | number | null;

export type ProvisionScore = {
	score: number;
	main: ScoreSummary;
	groups: Record<string, ScoreSummary>;
	recall_by_kind: Record<string, { gold_rows: number; matched: number; soft_recall: number }>;
	field_accuracy: Record<string, number | null>;
	pairs: {
		gold: string;
		pred: string;
		note?: string;
		checks: Record<string, ProvisionCheck>;
		credit: number;
	}[];
	missed: { gold: string; note?: string }[];
	false_positives: { pred: string; cause: string; note?: string; gold?: string; of?: string }[];
	overrides: unknown[];
};

export type ProvisionRow = Record<string, unknown> & { prov_id: string };

export type ProvisionScoreRun = Omit<ScoreRun, 'score' | 'input'> & {
	score?: ProvisionScore;
	input?: {
		provenance?: Record<string, unknown>;
		gold_run: GoldRun & Record<string, unknown>;
		extraction: Record<string, unknown>;
		source?: Record<string, unknown>;
		warnings: string[];
		gold: ProvisionRow[];
		predictions: ProvisionRow[];
	};
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(BASE + path, { credentials: 'same-origin', ...init });
	const data = await res.json().catch(() => null);
	if (!res.ok || data?.status === false)
		throw new ScoreAPIError(data?.error_code ?? 'request_failed', res.status);
	return data as T;
}

export async function provisionScoreModels(): Promise<string[]> {
	return (await request<{ models: string[] }>('/models')).models ?? [];
}

export async function provisionGoldRuns(recordId: number): Promise<GoldRun[]> {
	return (await request<{ runs: GoldRun[] }>(`/gold-runs?record_id=${recordId}`)).runs ?? [];
}

export async function provisionScoreHistory(
	recordId: number | undefined,
	offset: number
): Promise<{ runs: ProvisionScoreRun[]; total: number }> {
	const params = new URLSearchParams({ limit: '20', offset: String(offset) });
	if (recordId) params.set('record_id', String(recordId));
	return request('?' + params);
}

export async function provisionScoreDetail(id: number): Promise<ProvisionScoreRun> {
	return (await request<{ run: ProvisionScoreRun }>(`/${id}`)).run;
}

export async function startProvisionScore(
	recordId: number,
	model: string,
	lang: string,
	gold?: GoldRun
): Promise<ProvisionScoreRun> {
	return (
		await request<{ run: ProvisionScoreRun }>('', {
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

export function provisionScoreArtifact(id: number, kind: string): string {
	return `${BASE}/${id}/artifacts/${kind}`;
}
