import { expect, test } from 'bun:test';
import { listKbMetrics } from './kbService';

const originalFetch = globalThis.fetch;

test('benchmark request keeps its source, version and model when loading drops', async () => {
	let requested = '';
	globalThis.fetch = (async (input: string | URL | Request) => {
		requested = String(input);
		return Response.json({ status: true, results: [], total: 0, run_id: '20261010_055329' });
	}) as typeof fetch;
	try {
		const result = await listKbMetrics(753, 'testbed',
			{ skillVersion: '6.0.0', modelName: 'claude-opus-5-5' }, { includeDropped: true });
		const params = new URL(requested, 'http://localhost').searchParams;
		expect(params.get('source')).toBe('testbed');
		expect(params.get('include_dropped')).toBe('true');
		expect(params.get('skill_version')).toBe('6.0.0');
		expect(params.get('model_name')).toBe('claude-opus-5-5');
		expect(result.run_id).toBe('20261010_055329');
	} finally {
		globalThis.fetch = originalFetch;
	}
});

test('production dropped request still uses the production source', async () => {
	let requested = '';
	globalThis.fetch = (async (input: string | URL | Request) => {
		requested = String(input);
		return Response.json({ status: true, results: [], total: 0 });
	}) as typeof fetch;
	try {
		await listKbMetrics(753, 'kb', {}, { includeDropped: true });
		const params = new URL(requested, 'http://localhost').searchParams;
		expect(params.has('source')).toBe(false);
		expect(params.get('include_dropped')).toBe('true');
	} finally {
		globalThis.fetch = originalFetch;
	}
});
