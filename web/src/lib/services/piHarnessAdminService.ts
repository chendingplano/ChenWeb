const BASE = '/api/v1/system-admin/pi-harness';

export type PiProfileLimits = {
	max_tool_calls: number;
	max_elapsed: number;
	max_output_tokens: number;
	max_evidence_bytes: number;
	max_history_tokens: number;
	max_history_tool_result_bytes: number;
};

export type PiProfile = {
	slug: string;
	version: string;
	friendly_name: string;
	description: string;
	provider: string;
	model: string;
	provider_disclosure: string;
	thinking_level: string;
	prompt_file: string;
	allowed_tools: string[];
	allowed_knowledge_stores: string[];
	allowed_document_groups: string[];
	permission_default: 'ask' | 'auto';
	limits: PiProfileLimits;
	enabled: boolean;
	pilot_users: string[];
	harness: string;
};

export type PiHarnessAdminState = {
	gateway_url: string;
	initialized: boolean;
	profiles: PiProfile[];
	prompt_choices: Record<string, string[]>;
	/** Registered harness adapters a service can run on. */
	harnesses: string[];
};

export type PiProfileCandidate = Omit<PiProfile, 'version' | 'provider_disclosure' | 'allowed_tools'> & {
	expected_active_version: string;
};

export type PiGatewayCheck = { status: 'healthy' | 'unavailable'; elapsed_ms: number; checked_at: string };

async function request<T>(path: string, init: RequestInit = {}, fetchFn: typeof fetch = fetch): Promise<T> {
	const response = await fetchFn(`${BASE}${path}`, {
		credentials: 'same-origin',
		cache: 'no-store',
		...init
	});
	if (!response.ok) {
		let message = `Request failed (${response.status})`;
		try {
			const body = (await response.json()) as { error?: string };
			if (body.error) message = body.error;
		} catch {
			// Keep the status-based message when the response is not JSON.
		}
		const error = new Error(message);
		Object.assign(error, { status: response.status });
		throw error;
	}
	return response.json() as Promise<T>;
}

export const getPiHarnessAdminState = (fetchFn: typeof fetch = fetch) => request<PiHarnessAdminState>('', {}, fetchFn);
export const updatePiProfile = (slug: string, candidate: PiProfileCandidate, fetchFn: typeof fetch = fetch) =>
	request<{ profile: PiProfile }>(`/profiles/${encodeURIComponent(slug)}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(candidate)
	}, fetchFn);
export const initializePiHarnessProfiles = (fetchFn: typeof fetch = fetch) =>
	request<{ initialized: boolean }>('/initialize', { method: 'POST' }, fetchFn);
export const checkPiGateway = (fetchFn: typeof fetch = fetch) => request<PiGatewayCheck>('/gateway-check', { method: 'POST' }, fetchFn);
