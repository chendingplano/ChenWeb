import { describe, expect, test } from 'bun:test';
import { HARNESS_BY_AGENT, listHarnessModels } from './harnessFrontendService';

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

	test('reports an unavailable harness', async () => {
		const failing = (async () => new Response('{}', { status: 502 })) as unknown as typeof fetch;
		await expect(listHarnessModels('claude-code', failing)).rejects.toThrow('502');
	});
});
