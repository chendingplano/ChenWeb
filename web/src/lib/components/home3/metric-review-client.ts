// API client + pure helpers for the "Review Metrics" admin page
// (System Admin -> LLM). See openspec/changes/llm-review-metrics for the design.

export type ReviewSeverity = 'high' | 'medium' | 'low';
export type NonMetricCategory = 'not_metric' | 'duplicate' | 'formula_input';

export type InputRecordSummary = {
	id: number;
	title?: string;
	doc_no?: string;
	file_name?: string;
};

export type MetricReviewTally = {
	stored: number;
	kept: number;
	not_metric: number;
	duplicate: number;
	formula_input: number;
	missed: number;
};

export type MissedMetric = {
	lines: string;
	name: string;
	value: string;
	unit: string;
	reason: string;
	severity: ReviewSeverity;
};

export type NonMetricEntry = {
	metric_ids: string[];
	category: NonMetricCategory;
	duplicate_of?: string;
	reason: string;
};

export type AttributeIssue = {
	metric_ids: string[];
	field: string;
	stored: string;
	suggested: string;
	reason: string;
	severity: ReviewSeverity;
};

export type MetricSnapshot = {
	metric_id: string;
	name: string;
	value?: string;
	unit?: string;
	lines?: string;
};

export type MetricReviewReport = {
	summary: string;
	tally: MetricReviewTally;
	missed_metrics: MissedMetric[];
	non_metrics: NonMetricEntry[];
	attribute_issues: AttributeIssue[];
	recommendations: string[];
	metrics: MetricSnapshot[];
};

export type MetricReview = {
	id: number;
	input_record_id: number;
	status: 'running' | 'done' | 'failed';
	report?: MetricReviewReport;
	error_msg?: string;
	model_name?: string;
	prompt_name?: string;
	metrics_count: number;
	created_by?: string;
	created_at: string;
	finished_at?: string;
};

type MetricReviewResponse = { status: boolean; review: MetricReview | null; started?: boolean };

/** A purely numeric query searches by record ID; anything else by title. */
export function buildInputSearchQuery(query: string): string {
	const q = query.trim();
	const params = new URLSearchParams({ page: '1', page_size: '50' });
	if (/^\d+$/.test(q)) params.set('record_id', q);
	else if (q) params.set('title', q);
	return params.toString();
}

const SEVERITY_RANK: Record<string, number> = { high: 0, medium: 1, low: 2 };

/** Highest severity first; stable within a severity. */
export function sortBySeverity<T extends { severity: string }>(items: T[]): T[] {
	return [...items].sort((a, b) => (SEVERITY_RANK[a.severity] ?? 1) - (SEVERITY_RANK[b.severity] ?? 1));
}

export const NON_METRIC_CATEGORY_ORDER: NonMetricCategory[] = ['not_metric', 'duplicate', 'formula_input'];

export const NON_METRIC_CATEGORY_LABEL: Record<NonMetricCategory, string> = {
	not_metric: 'Not a metric',
	duplicate: 'Duplicate',
	formula_input: 'Formula input'
};

/** Groups non-metric entries by category in NON_METRIC_CATEGORY_ORDER, omitting empty groups. */
export function groupNonMetrics(
	entries: NonMetricEntry[]
): { category: NonMetricCategory; entries: NonMetricEntry[] }[] {
	return NON_METRIC_CATEGORY_ORDER.map((category) => ({
		category,
		entries: entries.filter((e) => e.category === category)
	})).filter((g) => g.entries.length > 0);
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, { credentials: 'same-origin', ...init });
	const text = await res.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
	}
	if (!res.ok) {
		const msg =
			parsed && typeof parsed === 'object' && 'error_msg' in parsed
				? String((parsed as { error_msg: unknown }).error_msg)
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

export async function searchInputs(query: string): Promise<InputRecordSummary[]> {
	const res = await req<{ results?: InputRecordSummary[] }>(`/api/v1/kb/inputs?${buildInputSearchQuery(query)}`);
	return res.results ?? [];
}

export async function getMetricReview(recordId: number): Promise<MetricReview | null> {
	const res = await req<MetricReviewResponse>(`/api/v1/kb/metric-reviews/${recordId}`);
	return res.review;
}

export async function startMetricReview(recordId: number, force: boolean): Promise<MetricReviewResponse> {
	return req<MetricReviewResponse>(`/api/v1/kb/metric-reviews/${recordId}`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ force })
	});
}
