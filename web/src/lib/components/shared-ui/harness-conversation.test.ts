import { describe, expect, test } from 'bun:test';
import { answeredInput, applyEvent, itemsFromHistory, questionsOf, renderMarkdown, toolSummary, withDecision, type HarnessItem } from './harness-conversation';

describe('harness conversation', () => {
	test('streams text into one answer and attaches tool results', () => {
		let items: HarnessItem[] = [];
		items = applyEvent(items, { type: 'text', text: 'Looking ' });
		items = applyEvent(items, { type: 'text', text: 'now.' });
		items = applyEvent(items, { type: 'tool_use', id: 't1', name: 'Read', input: { file_path: 'go.mod' }, parentToolUseId: null });
		items = applyEvent(items, { type: 'tool_use', id: 't2', name: 'Grep', input: {}, parentToolUseId: 'sub' });
		items = applyEvent(items, { type: 'tool_result', toolUseId: 't1', isError: false, text: 'module x' });
		items = applyEvent(items, { type: 'text', text: 'Done.' });
		expect(items.map((i) => i.kind)).toEqual(['assistant', 'tool', 'assistant']);
		expect(items[0]).toMatchObject({ text: 'Looking now.' });
		expect(items[1]).toMatchObject({ name: 'Read', result: { text: 'module x', isError: false } });
	});

	test('permission requests and notices', () => {
		let items = applyEvent([], { type: 'permission_request', requestId: 'r1', toolName: 'Bash', input: { command: 'make' } });
		items = withDecision(items, 'r1', false);
		items = applyEvent(items, { type: 'result', status: 'interrupted', sessionId: 's', durationMs: 1, numTurns: 1, costUsd: 0 });
		items = applyEvent(items, { type: 'result', status: 'completed', sessionId: 's', durationMs: 1, numTurns: 1, costUsd: 0 });
		expect(items).toMatchObject([{ kind: 'permission', requestId: 'r1', state: 'denied' }, { kind: 'notice', code: 'interrupted' }]);
		const withTool = applyEvent(
			applyEvent([], { type: 'tool_use', id: 't9', name: 'Write', input: {}, parentToolUseId: null }),
			{ type: 'permission_request', requestId: 't9', toolName: 'Write', input: {} }
		);
		expect(new Set(withTool.map((item) => item.id)).size).toBe(2);
	});

	test('history skips subagent messages and joins results', () => {
		const items = itemsFromHistory([
			{ uuid: 'u', role: 'user', parentToolUseId: null, blocks: [{ type: 'text', text: 'fix it' }] },
			{ uuid: 'a', role: 'assistant', parentToolUseId: null, blocks: [{ type: 'tool_use', id: 't', name: 'Bash', input: { command: 'make' } }] },
			{ uuid: 's', role: 'assistant', parentToolUseId: 't', blocks: [{ type: 'text', text: 'subagent' }] },
			{ uuid: 'r', role: 'user', parentToolUseId: null, blocks: [{ type: 'tool_result', toolUseId: 't', isError: true, text: 'fail' }] }
		]);
		expect(items).toMatchObject([{ kind: 'user', text: 'fix it' }, { kind: 'tool', result: { isError: true } }]);
	});

	test('tool summaries and question answers', () => {
		expect(toolSummary('Bash', { command: 'go   test ./...' })).toBe('go test ./...');
		const input = { questions: [{ question: 'Which DB?', options: [{ label: 'Postgres' }, { label: 'SQLite' }] }] };
		expect(questionsOf(input)).toHaveLength(1);
		expect(answeredInput(input, { 'Which DB?': ['Postgres', 'SQLite'] })).toEqual({ ...input, answers: { 'Which DB?': 'Postgres, SQLite' } });
	});

	test('markdown escapes raw HTML and unsafe links', () => {
		const html = renderMarkdown('**bold** <script>alert(1)</script> [x](javascript:alert(1)) [ok](https://a.b)');
		expect(html).toContain('<strong>bold</strong>');
		expect(html).not.toContain('<script>');
		expect(html).not.toContain('javascript:');
		expect(html).toContain('href="https://a.b"');
	});
});
