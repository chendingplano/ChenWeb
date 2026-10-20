// What the chat page shows for a harness session, built from the events the
// harness streams and from the history it stores. The harness owns the
// conversation; this is only its display.
import { Marked } from 'marked';
import type { HarnessEvent, HarnessHistoryMessage } from '$lib/services/harnessFrontendService';

export type NoticeCode = 'retrying' | 'interrupted' | 'max_turns' | 'error';

export type HarnessItem =
	| { kind: 'user'; id: string; text: string }
	| { kind: 'assistant'; id: string; text: string }
	| { kind: 'tool'; id: string; name: string; input: unknown; result?: { text: string; isError: boolean } }
	| {
			kind: 'permission';
			// The harness's request ID equals the tool call's ID, so the item
			// gets its own key.
			id: string;
			requestId: string;
			toolName: string;
			input: unknown;
			title?: string;
			description?: string;
			reason?: string;
			state: 'pending' | 'allowed' | 'denied';
	  }
	| { kind: 'notice'; id: string; code: NoticeCode; detail?: string };

let counter = 0;
const nextId = (prefix: string) => `${prefix}-${Date.now()}-${++counter}`;

/** Items for a session's stored history. Subagent messages are left out. */
export function itemsFromHistory(messages: HarnessHistoryMessage[]): HarnessItem[] {
	let items: HarnessItem[] = [];
	for (const message of messages) {
		if (message.parentToolUseId !== null) continue;
		for (const block of message.blocks) {
			if (block.type === 'text' && block.text.trim()) {
				items = [...items, { kind: message.role, id: nextId(message.role), text: block.text }];
			} else if (block.type === 'tool_use') {
				items = [...items, { kind: 'tool', id: block.id, name: block.name, input: block.input }];
			} else if (block.type === 'tool_result') {
				items = withToolResult(items, block.toolUseId, block.text, block.isError);
			}
		}
	}
	return items;
}

function withToolResult(items: HarnessItem[], toolUseId: string, text: string, isError: boolean): HarnessItem[] {
	return items.map((item) =>
		item.kind === 'tool' && item.id === toolUseId ? { ...item, result: { text, isError } } : item
	);
}

/** Adds one streamed event to the items. */
export function applyEvent(items: HarnessItem[], event: HarnessEvent): HarnessItem[] {
	switch (event.type) {
		case 'text': {
			const last = items.at(-1);
			if (last?.kind === 'assistant') return [...items.slice(0, -1), { ...last, text: last.text + event.text }];
			return [...items, { kind: 'assistant', id: nextId('assistant'), text: event.text }];
		}
		case 'tool_use':
			// Subagents' own tool calls are not shown; the subagent call itself is.
			if (event.parentToolUseId !== null) return items;
			return [...items, { kind: 'tool', id: event.id, name: event.name, input: event.input }];
		case 'tool_result':
			return withToolResult(items, event.toolUseId, event.text, event.isError);
		case 'permission_request':
			return [
				...items,
				{
					kind: 'permission',
					id: `permission-${event.requestId}`,
					requestId: event.requestId,
					toolName: event.toolName,
					input: event.input,
					title: event.title,
					description: event.description,
					reason: event.reason,
					state: 'pending'
				}
			];
		case 'status':
			return [...items, { kind: 'notice', id: nextId('notice'), code: 'retrying', detail: String(event.attempt) }];
		case 'result':
			if (event.status === 'completed') return items;
			return [...items, { kind: 'notice', id: nextId('notice'), code: event.status }];
		case 'error':
			return [...items, { kind: 'notice', id: nextId('notice'), code: 'error', detail: event.message }];
		default:
			return items;
	}
}

/** Marks a permission request answered. */
export function withDecision(items: HarnessItem[], requestId: string, allowed: boolean): HarnessItem[] {
	return items.map((item) =>
		item.kind === 'permission' && item.requestId === requestId
			? { ...item, state: allowed ? 'allowed' : 'denied' }
			: item
	);
}

/** One line describing what a tool call does, for the collapsed view. */
export function toolSummary(name: string, input: unknown): string {
	const fields = (input ?? {}) as Record<string, unknown>;
	for (const key of ['command', 'file_path', 'path', 'pattern', 'url', 'query', 'description', 'prompt']) {
		if (typeof fields[key] === 'string' && fields[key]) return shorten(String(fields[key]));
	}
	return name.startsWith('mcp__') ? '' : shorten(JSON.stringify(input ?? {}));
}

function shorten(text: string, max = 160): string {
	const line = text.replace(/\s+/g, ' ').trim();
	return line.length > max ? `${line.slice(0, max - 1)}…` : line;
}

export type Question = {
	question: string;
	header?: string;
	multiSelect?: boolean;
	options: { label: string; description?: string }[];
};

/** The questions of an AskUserQuestion call, when well formed. */
export function questionsOf(input: unknown): Question[] {
	const questions = (input as { questions?: unknown } | null)?.questions;
	if (!Array.isArray(questions)) return [];
	return questions.filter(
		(q): q is Question => typeof q?.question === 'string' && Array.isArray(q?.options)
	);
}

/** Claude Code takes answers keyed by question text; multi-select labels are comma-separated. */
export function answeredInput(input: unknown, answers: Record<string, string[]>): Record<string, unknown> {
	return {
		...((input as Record<string, unknown>) ?? {}),
		answers: Object.fromEntries(Object.entries(answers).map(([question, labels]) => [question, labels.join(', ')]))
	};
}

function escapeHtml(text: string): string {
	return text.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!);
}

// Harness text can repeat anything the agent read, so raw HTML is shown as
// text and only http(s)/mailto links are kept.
const safeMarked = new Marked({
	gfm: true,
	breaks: true,
	renderer: {
		html({ text }) {
			return escapeHtml(text);
		},
		link({ href, text }) {
			const safe = /^(https?:|mailto:)/i.test(href) ? href : '';
			return safe
				? `<a href="${escapeHtml(safe)}" target="_blank" rel="noopener noreferrer">${escapeHtml(text)}</a>`
				: escapeHtml(text);
		},
		image({ text }) {
			return escapeHtml(text);
		}
	}
});

export function renderMarkdown(text: string): string {
	return safeMarked.parse(text, { async: false }) as string;
}
