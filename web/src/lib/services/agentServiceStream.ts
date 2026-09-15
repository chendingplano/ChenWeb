export type AgentSource = {
	knowledge_store_id: string;
	document_id: string;
	source_title?: string;
	source_version?: string;
	artifact_type?: string;
	artifact_id?: string;
	line_start?: number;
	line_end?: number;
	page_start?: number;
	page_end?: number;
	page?: number;
};

export type AgentStreamEvent =
	| { type: 'answer_delta'; text: string }
	| { type: 'activity'; status: string; tool?: string; toolCallId?: string; error?: boolean; attempt?: number }
	| { type: 'sources'; sources: AgentSource[] }
	| { type: 'permission_request'; requestId: string; tool: string }
	| { type: 'usage'; inputTokens?: number; outputTokens?: number }
	| { type: 'error'; message: string }
	| { type: 'completion'; status: string };

const allowed = new Set(['answer_delta', 'activity', 'sources', 'permission_request', 'usage', 'error', 'completion']);

export class AgentSSEDecoder {
	private buffer = '';

	push(chunk: string): AgentStreamEvent[] {
		this.buffer = (this.buffer + chunk).replaceAll('\r\n', '\n');
		if (this.buffer.length > 256 * 1024) { this.buffer = ''; return []; }
		const events: AgentStreamEvent[] = [];
		let end: number;
		while ((end = this.buffer.indexOf('\n\n')) >= 0) {
			const frame = this.buffer.slice(0, end);
			this.buffer = this.buffer.slice(end + 2);
			const lines = frame.split('\n');
			const name = lines.find((line) => line.startsWith('event: '))?.slice(7);
			const data = lines.filter((line) => line.startsWith('data: ')).map((line) => line.slice(6)).join('\n');
			if (!name || !allowed.has(name) || !data) continue;
			try {
				const parsed = JSON.parse(data) as AgentStreamEvent;
				if (parsed && typeof parsed === 'object' && parsed.type === name) events.push(parsed);
			} catch { /* malformed frames are not rendered */ }
		}
		return events;
	}
}

export type AgentLiveState = {
	answer: string;
	activity: Array<{ status: string; tool?: string; error?: boolean }>;
	sources: AgentSource[];
	permission: { requestId: string; tool: string } | null;
	status: string;
	error: string;
};

export function emptyAgentLiveState(): AgentLiveState {
	return { answer: '', activity: [], sources: [], permission: null, status: 'running', error: '' };
}

export function applyAgentEvent(state: AgentLiveState, event: AgentStreamEvent): AgentLiveState {
	switch (event.type) {
		case 'answer_delta': return { ...state, answer: state.answer + event.text };
		case 'activity': return { ...state, activity: [...state.activity, { status: event.status, tool: event.tool, error: event.error }].slice(-30) };
		case 'sources': return { ...state, sources: event.sources };
		case 'permission_request': return { ...state, permission: { requestId: event.requestId, tool: event.tool } };
		case 'error': return { ...state, error: event.message };
		case 'completion': return { ...state, status: event.status, permission: null, sources: event.status === 'completed' ? state.sources : [] };
		default: return state;
	}
}
