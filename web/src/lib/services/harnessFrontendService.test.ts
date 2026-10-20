import { describe, expect, test } from 'bun:test';
import { HARNESS_BY_AGENT, listHarnessModels, streamHarnessTurn } from './harnessFrontendService';

describe('harnessFrontendService', () => {
	test('lists the harness models from the harness front-end API', async () => {
		let url = '';
		const models = await listHarnessModels(HARNESS_BY_AGENT['Claude Code'], (async (input: RequestInfo | URL) => {
			url = String(input);
			return Response.json({ models: [{ value: 'sonnet', displayName: 'Sonnet 5.5', description: 'Efficient' }] });
		}) as typeof fetch);
		expect(url).toBe('/api/v1/harness-frontend/claude-code/models');
		expect(models).toEqual([{ value: 'sonnet', displayName: 'Sonnet 5.5', description: 'Efficient' }]);
	});

	test('Codex in the agent menu is served by the codex gateway', async () => {
		let url = '';
		await listHarnessModels(HARNESS_BY_AGENT['Codex'], (async (input: RequestInfo | URL) => {
			url = String(input);
			return Response.json({ models: [] });
		}) as typeof fetch);
		expect(url).toBe('/api/v1/harness-frontend/codex/models');
	});

	test('DeepSeek Harness in the agent menu is served by the deepseek-harness gateway', async () => {
		let url = '';
		await listHarnessModels(HARNESS_BY_AGENT['DeepSeek Harness'], (async (input: RequestInfo | URL) => {
			url = String(input);
			return Response.json({ models: [] });
		}) as typeof fetch);
		expect(url).toBe('/api/v1/harness-frontend/deepseek-harness/models');
	});

	test('reports an unavailable harness', async () => {
		const failing = (async () => new Response('{}', { status: 502 })) as unknown as typeof fetch;
		await expect(listHarnessModels('claude-code', failing)).rejects.toThrow('502');
	});
});

describe('streamHarnessTurn', () => {
	test('parses SSE frames split across chunks', async () => {
		const frames = [
			'event: session\ndata: {"type":"session","sessionId":"s1","model":"m","cwd":"/w","permissionMode":"default"}\n\n',
			'event: text\ndata: {"type":"text","text":"Hel',
			'lo"}\n\nevent: result\ndata: {"type":"result","status":"completed","sessionId":"s1","durationMs":1,"numTurns":1,"costUsd":0}\n\n'
		];
		let sent: unknown;
		const fetchFn = (async (_input: RequestInfo | URL, init?: RequestInit) => {
			sent = JSON.parse(String(init?.body));
			const stream = new ReadableStream<Uint8Array>({
				start(controller) {
					for (const frame of frames) controller.enqueue(new TextEncoder().encode(frame));
					controller.close();
				}
			});
			return new Response(stream, { headers: { 'content-type': 'text/event-stream' } });
		}) as typeof fetch;
		const events: string[] = [];
		await streamHarnessTurn('claude-code', { message: 'hi', model: 'sonnet' }, (e) => events.push(e.type === 'text' ? `text:${e.text}` : e.type), undefined, fetchFn);
		expect(sent).toEqual({ message: 'hi', model: 'sonnet' });
		expect(events).toEqual(['session', 'text:Hello', 'result']);
	});

	test('surfaces a refused turn', async () => {
		const fetchFn = (async () => Response.json({ error: 'a turn is already running in this session' }, { status: 409 })) as unknown as typeof fetch;
		await expect(streamHarnessTurn('claude-code', { message: 'hi' }, () => {}, undefined, fetchFn)).rejects.toThrow('already running');
	});
});
