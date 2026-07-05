export type ModelProfile = {
	id: string;
	account_id: string;
	account_name: string;
	profile_name: string;
	model_name: string;
	thinking_type: string;
	timeout_sec: number;
	max_inflight: number;
	max_requests_per_minute: number;
	max_tokens_per_minute: number;
	token_reserve_per_call: number;
	is_active: boolean;
	created_at: string;
	updated_at: string;
};

export type ListModelProfilesResponse = {
	profiles: ModelProfile[];
};

export type CreateModelProfileInput = {
	account_id: string;
	profile_name: string;
	model_name: string;
	thinking_type: string;
	timeout_sec: number;
	max_inflight: number;
	max_requests_per_minute: number;
	max_tokens_per_minute: number;
	token_reserve_per_call: number;
	is_active: boolean;
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

export function listModelProfiles(): Promise<ListModelProfilesResponse> {
	return req<ListModelProfilesResponse>('/api/v1/llm/profiles');
}

export function createModelProfile(input: CreateModelProfileInput): Promise<ModelProfile> {
	return req<ModelProfile>('/api/v1/llm/profiles', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}

export function updateModelProfile(id: string, input: CreateModelProfileInput): Promise<ModelProfile> {
	return req<ModelProfile>(`/api/v1/llm/profiles/${id}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}
