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

export type LLMUsageEvent = {
	id: string;
	account_id: string;
	account_name: string;
	profile_id: string;
	provider: string;
	model_name: string;
	prompt_name: string;
	request_started_at: string;
	input_tokens: number;
	output_tokens: number;
	total_tokens: number;
	latency_ms: number;
	error_message: string;
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

export function listLLMUsageEvents(limit = 50): Promise<ListLLMUsageEventsResponse> {
	return req<ListLLMUsageEventsResponse>(`/api/v1/llm/usage-events?limit=${limit}`);
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
