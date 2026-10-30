// Which assistant and model a page's chat offers and selects
// (KnowledgeStore doc-repo/code_modules/interpreter-chat-composer.md).
import { HARNESS_BY_AGENT } from '$lib/services/harnessFrontendService';

/** The assistants a page may offer, in their default order: the harness-backed ones. */
export const CHAT_AGENTS = Object.keys(HARNESS_BY_AGENT);

/** The configured assistants the server has a gateway for; all of them while that is unknown. */
export function offeredAgents(agents: string[], available: string[] | null): string[] {
	return available ? agents.filter((agent) => available.includes(HARNESS_BY_AGENT[agent])) : agents;
}

/** The user's choice if offered, else the configured default if offered, else the first. */
export function pick(offered: string[], chosen: string, fallback?: string): string {
	if (offered.includes(chosen)) return chosen;
	if (fallback && offered.includes(fallback)) return fallback;
	return offered[0] ?? '';
}
