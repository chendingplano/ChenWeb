export type LLMModelEntry = {
	key: string;
	host: string;
	model_name: string;
	base_url: string;
	timeout_sec: number;
	thinking_type: string;
	max_inflight: number;
	max_requests_per_minute: number;
	max_tokens_per_minute: number;
	token_reserve_per_call: number;
};

export type GetModelsTOMLResponse = {
	ok: boolean;
	path: string;
	models: LLMModelEntry[];
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, {
		credentials: 'same-origin',
		...init
	});
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
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

export function getModelsTOML(): Promise<GetModelsTOMLResponse> {
	return req<GetModelsTOMLResponse>('/api/v1/llm/models-toml');
}

export function upsertModelTOML(key: string, entry: Omit<LLMModelEntry, 'key'>): Promise<{ ok: boolean; key: string }> {
	return req<{ ok: boolean; key: string }>(`/api/v1/llm/models-toml/${encodeURIComponent(key)}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(entry)
	});
}

export function deleteModelTOML(key: string): Promise<{ ok: boolean; key: string }> {
	return req<{ ok: boolean; key: string }>(`/api/v1/llm/models-toml/${encodeURIComponent(key)}`, {
		method: 'DELETE'
	});
}

export function addModel(input: {
	profile_name: string;
	model_name: string;
	thinking_type: string;
	timeout_sec: number;
	max_inflight: number;
	max_requests_per_minute: number;
	max_tokens_per_minute: number;
	token_reserve_per_call: number;
	host: string;
	account_name: string;
	provider: string;
	base_url: string;
	api_key: string;
}): Promise<unknown> {
	return req<unknown>('/api/v1/llm/models', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}
