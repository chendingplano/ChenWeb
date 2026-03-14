const BASE = '/api/v1/chatter';

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		method,
		headers: body ? { 'Content-Type': 'application/json' } : {},
		body: body ? JSON.stringify(body) : undefined,
		credentials: 'same-origin'
	});
	if (!res.ok) {
		const err = await res
			.json()
			.catch(() => ({ error: { code: 'UNKNOWN', message: res.statusText } }));
		throw err;
	}
	return res.json();
}

export type ChatterSettings = {
	agents: string[];
	models: string[];
	attachments: string[];
	skills: string[];
	resultOptions: string[];
	slashCommands: string[];
};

export type ChatterPrompt = {
	id: string;
	title: string;
	content: string;
	updatedAt: string;
};

export type ChatterSession = {
	id: string;
	title: string;
	createdAt?: string;
	updatedAt?: string;
};

export type ChatterDialogItem = {
	id: string;
	role: 'user' | 'assistant' | 'system';
	content: string;
	createdAt: string;
};

export const chatterService = {
	getSettings: (): Promise<{ settings: ChatterSettings }> => request('GET', '/settings'),
	updateSettings: (settings: ChatterSettings): Promise<{ status: string }> =>
		request('PUT', '/settings', { settings }),
	getPrompts: (): Promise<{ prompts: ChatterPrompt[] }> => request('GET', '/prompts'),
	getSlashCommands: (): Promise<{ commands: string[] }> => request('GET', '/slash-commands'),
	listSessions: (): Promise<{ sessions: ChatterSession[] }> => request('GET', '/sessions'),
	createSession: (): Promise<{ session: ChatterSession }> => request('POST', '/sessions'),
	getDialogs: (sessionId: string): Promise<{ dialogs: ChatterDialogItem[] }> =>
		request('GET', `/sessions/${sessionId}/dialogs`),
	sendMessage: (
		sessionId: string,
		payload: { input: string; model: string; agent: string; resultType: string }
	): Promise<{ dialog: ChatterDialogItem }> => request('POST', `/sessions/${sessionId}/messages`, payload)
};
