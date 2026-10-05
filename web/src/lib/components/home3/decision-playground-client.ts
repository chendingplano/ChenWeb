export type QuestionType = 'noul' | 'choice' | 'score';

export type PlaygroundModel = {
	key: string;
	model_name: string;
	model_type: string;
	provider: string;
};

export type DecisionPolicy = {
	id: number;
	name: string;
	description: string;
	current_version: number;
	latest_version: number;
};

export type DecisionPolicyVersion = {
	policy_id: number;
	policy_name: string;
	version: number;
	content: string;
	note: string;
};

export type PlaygroundOptions = {
	models: PlaygroundModel[];
	policies: DecisionPolicy[];
	policy_error?: string;
};

export type PlaygroundQuestion = {
	id: string;
	type: QuestionType;
	instructions: string;
	// choice: { option: description }; score: ordered levels; noul: omitted.
	criteria?: Record<string, string> | string[];
};

export type JevAnswer = {
	type: QuestionType;
	noul?: number;
	choice?: string;
	score?: number;
	legend?: Record<string, string>;
	probabilities?: Record<string, number>;
	confidence?: number;
};

export type PlaygroundRunRequest = {
	model_key: string;
	policy_id: number;
	policy_version: number;
	policy: string;
	text: string;
	questions: PlaygroundQuestion[];
};

export type PlaygroundRunResult = {
	model_key: string;
	provider: string;
	answers: Record<string, JevAnswer>;
	raw: unknown;
	elapsed_ms: number;
	usage?: { input_tokens: number; output_tokens: number };
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
		const body = (parsed ?? {}) as { message?: unknown; error?: unknown };
		const msg = body.message ? String(body.message) : `HTTP ${res.status}`;
		throw new Error(body.error ? `${msg}: ${String(body.error)}` : msg);
	}
	return parsed as T;
}

export function getPlaygroundOptions(): Promise<PlaygroundOptions> {
	return req<PlaygroundOptions>('/api/v1/llm/decision-playground/options');
}

export function getPolicyCurrentVersion(id: number): Promise<DecisionPolicyVersion> {
	return req<DecisionPolicyVersion>(`/api/v1/llm/decision-playground/policies/${id}`);
}

export function runDecision(body: PlaygroundRunRequest): Promise<PlaygroundRunResult> {
	return req<PlaygroundRunResult>('/api/v1/llm/decision-playground/run', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
}

// parseCriteria turns the criteria text area into question criteria: one
// "option: description" per line for choice, one level per line for score.
export function parseCriteria(type: QuestionType, text: string): Record<string, string> | string[] | undefined {
	const lines = text
		.split('\n')
		.map((l) => l.trim())
		.filter(Boolean);
	if (type === 'score') return lines;
	if (type === 'choice') {
		const out: Record<string, string> = {};
		for (const line of lines) {
			const i = line.indexOf(':');
			const key = (i < 0 ? line : line.slice(0, i)).trim();
			if (key) out[key] = i < 0 ? '' : line.slice(i + 1).trim();
		}
		return out;
	}
	return undefined;
}
