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
	'Claude Code': 'claude-code'
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
