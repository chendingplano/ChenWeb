export type LLMDailyReport = {
	account_id: string;
	account_name: string;
	workspace_day: string;
	timezone_name: string;
	opening_balance: number;
	closing_balance: number;
	spend_amount: number;
	currency_code: string;
	input_tokens: number;
	output_tokens: number;
	total_tokens: number;
	request_count: number;
	reconciliation_status: string;
};

export type LLMModelActivityReport = {
	provider: string;
	model_name: string;
	currency_code: string;
	workspace_day: string;
	spend_amount: number;
	input_tokens: number;
	output_tokens: number;
	total_tokens: number;
	request_count: number;
};

export type LLMUsageEvent = {
	id: string;
	account_id: string;
	account_name: string;
	profile_id: string;
	record_id: number | null;
	provider: string;
	model_name: string;
	prompt_name: string;
	call_reason: string;
	call_loc: string;
	request_started_at: string;
	input_tokens: number;
	output_tokens: number;
	total_tokens: number;
	latency_ms: number;
	error_message: string;
};

// LLMUsageEventDetail mirrors the backend UsageEventAdmin shape, returned by
// the usage-events-admin and usage-events/by-ids endpoints (more fields than
// the plain LLMUsageEvent returned by the lightweight usage-events list).
export type LLMUsageEventDetail = {
	id: string;
	account_id: string | null;
	account_name: string | null;
	profile_id: string | null;
	record_id: number | null;
	provider: string;
	model_name: string;
	prompt_name: string;
	call_reason: string;
	call_loc: string;
	request_started_at: string;
	input_tokens: number;
	output_tokens: number;
	total_tokens: number;
	prompt_cache_hit_tokens: number;
	prompt_cache_miss_tokens: number;
	latency_ms: number;
	error_message: string;
	input_body_ref: string;
	output_body_ref: string;
	metadata_json: unknown;
};

export type LLMCurrentBalance = {
	account_id: string;
	account_name: string;
	provider: string;
	workspace_day: string;
	captured_at: string;
	balance_amount: number;
	currency_code: string;
};

export type LLMTodaySummary = {
	workspace_day: string;
	timezone_name: string;
	spend_amount: number;
	currency_code: string;
	request_count: number;
	total_tokens: number;
	error_count: number;
};

export type ListLLMDailyReportsResponse = {
	reports: LLMDailyReport[];
};

export type ListLLMModelActivityReportsResponse = {
	reports: LLMModelActivityReport[];
};

export type ListLLMUsageEventsResponse = {
	usage_events: LLMUsageEvent[];
};

export type ListLLMCurrentBalancesResponse = {
	balances: LLMCurrentBalance[];
};

export type GetLLMTodaySummaryResponse = {
	summary: LLMTodaySummary;
};

export type RunLLMReconciliationNowResponse = {
	ok: boolean;
	message?: string;
	usage_days_processed?: number;
	usage_rows_affected?: number;
	accounts_considered?: number;
	snapshots_created?: number;
	reports_reconciled?: number;
};

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
			parsed && typeof parsed === 'object' && parsed !== null && 'message' in parsed
				? String((parsed as { message: unknown }).message)
				: parsed && typeof parsed === 'object' && parsed !== null && 'error' in parsed
					? String((parsed as { error: unknown }).error)
					: parsed && typeof parsed === 'object' && parsed !== null && 'error_msg' in parsed
						? String((parsed as { error_msg: unknown }).error_msg)
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

export function listLLMDailyReports(limit = 30): Promise<ListLLMDailyReportsResponse> {
	return req<ListLLMDailyReportsResponse>(`/api/v1/llm/reports/daily?limit=${limit}`);
}

export function listLLMModelActivityReports(limit = 30): Promise<ListLLMModelActivityReportsResponse> {
	return req<ListLLMModelActivityReportsResponse>(`/api/v1/llm/reports/models?limit=${limit}`);
}

export function listLLMUsageEvents(limit = 50): Promise<ListLLMUsageEventsResponse> {
	return req<ListLLMUsageEventsResponse>(`/api/v1/llm/usage-events?limit=${limit}`);
}

// Fetches llm_usage_event rows for a set of ids, e.g. the ids recorded in
// kb.doc_review_logs.detail.llm_usage_event_ids. Returns [] for an empty list.
export function getLLMUsageEventsByIds(ids: string[]): Promise<LLMUsageEventDetail[]> {
	if (ids.length === 0) return Promise.resolve([]);
	return req<{ usage_events: LLMUsageEventDetail[] }>(
		`/api/v1/llm/usage-events/by-ids?ids=${encodeURIComponent(ids.join(','))}`
	).then((res) => res.usage_events || []);
}

export function listLLMCurrentBalances(limit = 20): Promise<ListLLMCurrentBalancesResponse> {
	return req<ListLLMCurrentBalancesResponse>(`/api/v1/llm/balances/current?limit=${limit}`);
}

export function getLLMTodaySummary(): Promise<GetLLMTodaySummaryResponse> {
	return req<GetLLMTodaySummaryResponse>('/api/v1/llm/summary/today');
}

export function runLLMReconciliationNow(): Promise<RunLLMReconciliationNowResponse> {
	return req<RunLLMReconciliationNowResponse>('/api/v1/llm/reconciliation/run', { method: 'POST' });
}
