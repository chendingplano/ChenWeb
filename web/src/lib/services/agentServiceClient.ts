import { AgentSSEDecoder, type AgentSource, type AgentStreamEvent } from './agentServiceStream';

const BASE = '/api/v1/agent-services';

export type AgentProfile = {
	slug: string;
	friendly_name: string;
	description: string;
	provider: string;
	model: string;
	provider_disclosure: string;
	permission_default: 'ask' | 'auto';
	allowed_knowledge_stores: string[];
};
export type AgentConversation = {
	id: string;
	service_slug: string;
	profile_slug: string;
	title: string;
	model_name: string;
	updated_at: string;
};
export type AgentMessage = { id: string; role: 'user' | 'assistant'; content: string; status: string; attempt_id?: string; sequence_no: number };
export type AgentResumeState = {
	conversation: AgentConversation;
	messages: AgentMessage[];
	sources_by_message?: Record<string, Array<AgentSource & { document_title?: string; page_start?: number; page_end?: number }>>;
	hidden_message_ids?: string[];
	omission_notice?: string;
};

async function jsonRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
	const response = await fetch(`${BASE}${path}`, { credentials: 'same-origin', cache: 'no-store', ...init });
	if (!response.ok) {
		let message = `Request failed (${response.status})`;
		try { const body = await response.json() as { error?: string }; if (body.error) message = body.error; } catch { /* no structured error */ }
		throw new Error(message);
	}
	return response.json() as Promise<T>;
}

export const listAgentProfiles = () => jsonRequest<AgentProfile[]>('');
export const listAgentConversations = () => jsonRequest<AgentConversation[]>('/conversations');
export const getAgentConversation = (id: string) => jsonRequest<AgentResumeState>(`/conversations/${encodeURIComponent(id)}`);
export const createAgentConversation = (slug: string, title: string) => jsonRequest<AgentConversation>(`/${encodeURIComponent(slug)}/conversations`, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ title }) });
export const deleteAgentConversation = (id: string) => jsonRequest<unknown>(`/conversations/${encodeURIComponent(id)}`, { method: 'DELETE' });
export const stopAgentRun = (conversationId: string, runId: string) => jsonRequest<unknown>(`/conversations/${encodeURIComponent(conversationId)}/runs/${encodeURIComponent(runId)}/cancel`, { method: 'POST' });
export const decideAgentPermission = (conversationId: string, runId: string, requestId: string, allowed: boolean) => jsonRequest<unknown>(`/conversations/${encodeURIComponent(conversationId)}/runs/${encodeURIComponent(runId)}/permissions/${encodeURIComponent(requestId)}`, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ allowed }) });
export const rateAgentMessage = (conversationId: string, messageId: string, rating: 'helpful' | 'unhelpful') => jsonRequest<unknown>(`/conversations/${encodeURIComponent(conversationId)}/messages/${encodeURIComponent(messageId)}/feedback`, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ rating }) });

export async function streamAgentRun(
	conversationId: string,
	message: string,
	permissionMode: 'ask' | 'auto',
	onEvent: (event: AgentStreamEvent) => void,
	onRunId: (runId: string) => void,
	signal?: AbortSignal
): Promise<void> {
	const response = await fetch(`${BASE}/conversations/${encodeURIComponent(conversationId)}/runs`, {
		method: 'POST', credentials: 'same-origin', cache: 'no-store', signal,
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify({ message, permission_mode: permissionMode, idempotency_key: crypto.randomUUID() })
	});
	if (!response.ok) {
		let message = `Agent unavailable (${response.status})`;
		try { const body = await response.json() as { error?: string }; if (body.error) message = body.error; } catch { /* no structured error */ }
		throw new Error(message);
	}
	const runId = response.headers.get('X-Agent-Run-Id');
	if (runId) onRunId(runId);
	if (!response.body) throw new Error('The agent stream did not start.');
	const reader = response.body.getReader();
	const decoder = new TextDecoder();
	const frames = new AgentSSEDecoder();
	try {
		while (true) {
			const { done, value } = await reader.read();
			if (done) break;
			for (const event of frames.push(decoder.decode(value, { stream: true }))) onEvent(event);
		}
		for (const event of frames.push(decoder.decode())) onEvent(event);
	} finally { reader.releaseLock(); }
}
