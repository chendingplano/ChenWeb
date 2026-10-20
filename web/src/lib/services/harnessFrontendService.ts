// Client for /api/v1/harness-frontend: ChenWeb as a front end to an agent
// harness such as Claude Code. The harness owns its models, sessions and
// history; these calls only ask it.
const BASE = '/api/v1/harness-frontend';

/** A model the harness offers, as its own model picker lists it. */
export type HarnessModel = {
	value: string;
	resolvedModel?: string;
	displayName: string;
	description: string;
};

/**
 * Agents in the chat page's agent menu that run on a harness gateway, mapped
 * to the gateway's harness name. Agents not listed here keep the configured
 * model list.
 */
export const HARNESS_BY_AGENT: Record<string, string> = {
	'Claude Code': 'claude-code',
	Codex: 'codex',
	'DeepSeek Harness': 'deepseek-harness',
	pi: 'pi'
};

export async function listHarnessModels(
	harness: string,
	fetchFn: typeof fetch = fetch
): Promise<HarnessModel[]> {
	const res = await fetchFn(`${BASE}/${encodeURIComponent(harness)}/models`, {
		credentials: 'same-origin',
		cache: 'no-store'
	});
	if (!res.ok) {
		throw new Error(`Model list unavailable (${res.status})`);
	}
	const body = (await res.json()) as { models?: HarnessModel[] };
	return body.models ?? [];
}

/** A session the harness keeps, as its own session list shows it. */
export type HarnessSession = {
	sessionId: string;
	title: string;
	firstPrompt?: string;
	lastModified: number;
	live: boolean;
};

export type HarnessBlock =
	| { type: 'text'; text: string }
	| { type: 'tool_use'; id: string; name: string; input: unknown }
	| { type: 'tool_result'; toolUseId: string; isError: boolean; text: string }
	| { type: 'image' };

export type HarnessHistoryMessage = {
	uuid: string;
	role: 'user' | 'assistant';
	parentToolUseId: string | null;
	blocks: HarnessBlock[];
};

/** One event of a turn, as the harness streams it. */
export type HarnessEvent =
	| { type: 'session'; sessionId: string; model: string; cwd: string; permissionMode: string }
	| { type: 'text'; text: string }
	| { type: 'tool_use'; id: string; name: string; input: unknown; parentToolUseId: string | null }
	| { type: 'tool_result'; toolUseId: string; isError: boolean; text: string }
	| {
			type: 'permission_request';
			requestId: string;
			toolName: string;
			input: unknown;
			title?: string;
			description?: string;
			reason?: string;
	  }
	| { type: 'status'; status: 'retrying'; attempt: number }
	| {
			type: 'result';
			status: 'completed' | 'interrupted' | 'max_turns' | 'error';
			sessionId: string;
			durationMs: number;
			numTurns: number;
			costUsd: number;
			text?: string;
	  }
	| { type: 'error'; message: string };

export type HarnessTurn = {
	session_id?: string;
	message: string;
	model?: string;
	permission_mode?: 'default' | 'acceptEdits' | 'plan';
};

export type HarnessDecision = {
	behavior: 'allow' | 'deny';
	updated_input?: Record<string, unknown>;
	message?: string;
};

async function json<T>(res: Response): Promise<T> {
	if (!res.ok) {
		let message = `Request failed (${res.status})`;
		try {
			const body = (await res.json()) as { error?: string };
			if (body.error) message = body.error;
		} catch {
			// Keep the status message when the body is not JSON.
		}
		throw new Error(message);
	}
	return (await res.json()) as T;
}

const harnessPath = (harness: string, path: string) => `${BASE}/${encodeURIComponent(harness)}${path}`;

export async function listHarnessSessions(
	harness: string,
	limit = 30,
	fetchFn: typeof fetch = fetch
): Promise<HarnessSession[]> {
	const res = await fetchFn(harnessPath(harness, `/sessions?limit=${limit}`), {
		credentials: 'same-origin',
		cache: 'no-store'
	});
	return (await json<{ sessions?: HarnessSession[] }>(res)).sessions ?? [];
}

export async function getHarnessMessages(
	harness: string,
	sessionId: string,
	fetchFn: typeof fetch = fetch
): Promise<HarnessHistoryMessage[]> {
	const res = await fetchFn(
		harnessPath(harness, `/sessions/${encodeURIComponent(sessionId)}/messages?limit=500`),
		{ credentials: 'same-origin', cache: 'no-store' }
	);
	return (await json<{ messages?: HarnessHistoryMessage[] }>(res)).messages ?? [];
}

export async function decideHarnessPermission(
	harness: string,
	sessionId: string,
	requestId: string,
	decision: HarnessDecision,
	fetchFn: typeof fetch = fetch
): Promise<void> {
	const res = await fetchFn(
		harnessPath(harness, `/sessions/${encodeURIComponent(sessionId)}/permissions/${encodeURIComponent(requestId)}`),
		{
			method: 'POST',
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(decision)
		}
	);
	await json<unknown>(res);
}

export async function interruptHarnessTurn(
	harness: string,
	sessionId: string,
	fetchFn: typeof fetch = fetch
): Promise<void> {
	const res = await fetchFn(harnessPath(harness, `/sessions/${encodeURIComponent(sessionId)}/interrupt`), {
		method: 'POST',
		credentials: 'same-origin'
	});
	await json<unknown>(res);
}

/**
 * Sends one message and calls onEvent for each event the harness streams,
 * until the turn ends. Aborting signal closes the stream, which interrupts
 * the turn.
 */
export async function streamHarnessTurn(
	harness: string,
	turn: HarnessTurn,
	onEvent: (event: HarnessEvent) => void,
	signal?: AbortSignal,
	fetchFn: typeof fetch = fetch
): Promise<void> {
	const res = await fetchFn(harnessPath(harness, '/turns'), {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
		body: JSON.stringify(turn),
		signal
	});
	if (!res.ok || !res.body) {
		await json<unknown>(res);
		throw new Error('The harness returned no stream');
	}
	const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
	let buffer = '';
	while (true) {
		const { done, value } = await reader.read();
		if (done) break;
		buffer += value;
		let boundary: number;
		while ((boundary = buffer.indexOf('\n\n')) >= 0) {
			const frame = buffer.slice(0, boundary);
			buffer = buffer.slice(boundary + 2);
			const data = frame
				.split('\n')
				.filter((line) => line.startsWith('data: '))
				.map((line) => line.slice(6))
				.join('\n');
			if (data) onEvent(JSON.parse(data) as HarnessEvent);
		}
	}
}
