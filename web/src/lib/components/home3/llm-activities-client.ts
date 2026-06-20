export type LLMDailyReport = {
	account_id: string;
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

export type ListLLMDailyReportsResponse = {
	reports: LLMDailyReport[];
};

export type ListLLMUsageEventsResponse = {
	usage_events: LLMUsageEvent[];
};

async function req<T>(path: string): Promise<T> {
	const res = await fetch(path, { credentials: 'same-origin' });
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
			parsed && typeof parsed === 'object' && parsed !== null && 'error_msg' in parsed
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
