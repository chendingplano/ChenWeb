import { ScoreAPIError, type GoldRun, type ScoreRun, type ScoreSummary } from './metric-score-client';

const BASE = '/api/v1/kb/product-scores';

export { goldRunKey, ScoreAPIError, type GoldRun, type ScoreSummary } from './metric-score-client';

// A pass/fail check is boolean; requirement_text, lines, parameters, conditions and related_products
// earn partial credit (0..1); null = the check does not apply to the pair.
export type ProductCheck = boolean | number | null;

export type ProductDetection = {
	gold_products: number;
	predicted_products: number;
	detected: number;
	correct_predicted: number;
	precision: number;
	recall: number;
	f1: number;
	missed_products: { product_key: string; product_name: string }[];
};

export type ProductScore = {
	score: number;
	main: ScoreSummary;
	groups: Record<string, ScoreSummary>;
	groups_by_product_type: Record<string, ScoreSummary>;
	product_detection: ProductDetection;
	field_accuracy: Record<string, number | null>;
	pairs: {
		gold: string;
		pred: string;
		note?: string;
		checks: Record<string, ProductCheck>;
		credit: number;
	}[];
	missed: { gold: string; note?: string; product_missed?: boolean }[];
	false_positives: { pred: string; cause: string; note?: string; gold?: string; of?: string; product_key?: string }[];
	overrides: unknown[];
};

export type ProductRow = Record<string, unknown> & { product_rel_id: string };

export type ProductScoreRun = Omit<ScoreRun, 'score' | 'input'> & {
	score?: ProductScore;
	input?: {
		provenance?: Record<string, unknown>;
		gold_run: GoldRun & Record<string, unknown>;
		extraction: Record<string, unknown>;
		source?: Record<string, unknown>;
		warnings: string[];
		gold: ProductRow[];
		predictions: ProductRow[];
	};
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(BASE + path, { credentials: 'same-origin', ...init });
	const data = await res.json().catch(() => null);
	if (!res.ok || data?.status === false)
		throw new ScoreAPIError(data?.error_code ?? 'request_failed', res.status);
	return data as T;
}

export async function productScoreModels(): Promise<string[]> {
	return (await request<{ models: string[] }>('/models')).models ?? [];
}

export async function productGoldRuns(recordId: number): Promise<GoldRun[]> {
	return (await request<{ runs: GoldRun[] }>(`/gold-runs?record_id=${recordId}`)).runs ?? [];
}

export async function productScoreHistory(
	recordId: number | undefined,
	offset: number
): Promise<{ runs: ProductScoreRun[]; total: number }> {
	const params = new URLSearchParams({ limit: '20', offset: String(offset) });
	if (recordId) params.set('record_id', String(recordId));
	return request('?' + params);
}

export async function productScoreDetail(id: number): Promise<ProductScoreRun> {
	return (await request<{ run: ProductScoreRun }>(`/${id}`)).run;
}

export async function startProductScore(
	recordId: number,
	model: string,
	lang: string,
	gold?: GoldRun
): Promise<ProductScoreRun> {
	return (
		await request<{ run: ProductScoreRun }>('', {
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

export function productScoreArtifact(id: number, kind: string): string {
	return `${BASE}/${id}/artifacts/${kind}`;
}

// Evidence spans "N", "N-M" and table rows "N#rK" / "N#hK": the plain lines and the table rows.
export function evidenceTargets(spans: unknown): { lines: number[]; tables: { line: number; rows: string[] }[] } {
	const lines = new Set<number>();
	const tables = new Map<number, string[]>();
	for (const span of Array.isArray(spans) ? spans : []) {
		const text = String(span).trim();
		const row = text.match(/^(\d+)#([hr]\d+)$/);
		if (row) {
			const line = Number(row[1]);
			tables.set(line, [...(tables.get(line) ?? []), row[2]]);
			continue;
		}
		const range = text.match(/^(\d+)(?:\s*-\s*(\d+))?$/);
		if (!range) continue;
		const start = Number(range[1]);
		const end = Math.min(Number(range[2] ?? range[1]), start + 199);
		for (let line = start; line <= end; line++) if (line > 0) lines.add(line);
	}
	return {
		lines: [...lines].sort((a, b) => a - b),
		tables: [...tables].map(([line, rows]) => ({ line, rows }))
	};
}
