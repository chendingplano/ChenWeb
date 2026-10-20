import { describe, expect, test } from 'bun:test';
import { createLatestCodeMonitorLoader, getCodeMonitor } from './code-monitor-client';

describe('getCodeMonitor', () => {
	test('requests the selected range and returns typed package rows', async () => {
		const originalFetch = globalThis.fetch;
		let requestUrl = '';
		let credentials: RequestCredentials | undefined;
		globalThis.fetch = (async (input, init) => {
			requestUrl = String(input);
			credentials = init?.credentials;
			return Response.json({
				packages: [
					{
						package_path: 'shared/go',
						rows: [
							{
								collection_date: '2026-10-20',
								total_lines: 120,
								frontend_files: 3,
								backend_files: 5
							}
						]
					}
				]
			});
		}) as typeof fetch;

		try {
			const response = await getCodeMonitor('last_7_days');
			expect(requestUrl).toBe('/api/v1/code-monitor?range=last_7_days');
			expect(credentials).toBe('same-origin');
			expect(response.packages[0].rows[0].total_lines).toBe(120);
		} finally {
			globalThis.fetch = originalFetch;
		}
	});

	test('throws when the endpoint returns an error', async () => {
		const originalFetch = globalThis.fetch;
		globalThis.fetch = (async () => new Response('unavailable', { status: 503 })) as typeof fetch;
		try {
			await expect(getCodeMonitor('today')).rejects.toThrow('503');
		} finally {
			globalThis.fetch = originalFetch;
		}
	});
});

describe('createLatestCodeMonitorLoader', () => {
	test('marks older range responses stale when a newer request finishes first', async () => {
		let resolveOld!: (value: { packages: [] }) => void;
		const loader = createLatestCodeMonitorLoader((range) =>
			range === 'today'
				? new Promise((resolve) => { resolveOld = resolve; })
				: Promise.resolve({ packages: [] })
		);

		const oldRequest = loader.load('today');
		const newRequest = await loader.load('yesterday');
		resolveOld({ packages: [] });
		const oldResult = await oldRequest;

		expect(newRequest.status).toBe('success');
		expect(oldResult.status).toBe('stale');
	});

	test('marks an older range error stale after a newer request has started', async () => {
		let rejectOld!: (error: Error) => void;
		const loader = createLatestCodeMonitorLoader((range) =>
			range === 'today'
				? new Promise((_, reject) => { rejectOld = reject; })
				: Promise.resolve({ packages: [] })
		);

		const oldRequest = loader.load('today');
		await loader.load('yesterday');
		rejectOld(new Error('old request failed'));
		expect((await oldRequest).status).toBe('stale');
	});
});
