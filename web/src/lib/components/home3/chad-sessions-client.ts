export type ChadSessionSummary = {
	id: string;
	title?: string;
	updated: number;
	turns?: number;
	cwd?: string;
	messageCount: number;
};

export type ChadSessionMessage = {
	role?: string;
	name?: string;
	content?: unknown;
	toolCallCommand?: string;
	toolCallParameters?: unknown;
};

export type ChadSessionDetail = {
	id: string;
	title?: string;
	updated: number;
	turns?: number;
	cwd?: string;
	meta?: unknown;
	messages: ChadSessionMessage[];
};

async function request<T>(path: string): Promise<T> {
	const response = await fetch(path, { credentials: 'same-origin' });
	const text = await response.text();
	let body: unknown = null;
	if (text) {
		try {
			body = JSON.parse(text);
		} catch {
			body = null;
		}
	}
	if (!response.ok) {
		const message =
			body && typeof body === 'object' && 'error_msg' in body
				? String((body as { error_msg: unknown }).error_msg)
				: `HTTP ${response.status}`;
		throw new Error(message);
	}
	return body as T;
}

export function listChadSessions(): Promise<{ sessions: ChadSessionSummary[] }> {
	return request<{ sessions: ChadSessionSummary[] }>('/api/v1/chad/sessions');
}

export function getChadSession(id: string): Promise<ChadSessionDetail> {
	return request<ChadSessionDetail>(`/api/v1/chad/sessions/${encodeURIComponent(id)}`);
}
