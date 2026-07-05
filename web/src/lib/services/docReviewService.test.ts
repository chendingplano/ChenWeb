import test from 'node:test';
import assert from 'node:assert/strict';

import { getRequest, listReviewRuns, restartRequest } from './docReviewService';

test('restartRequest posts to the review restart endpoint', async () => {
	const originalFetch = globalThis.fetch;
	let calledUrl = '';
	let calledMethod = '';

	globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
		calledUrl = String(input);
		calledMethod = init?.method ?? 'GET';

		return new Response(JSON.stringify({ status: true, request_id: 38, run_id: 91 }), {
			status: 200,
			headers: { 'Content-Type': 'application/json' }
		});
	}) as typeof fetch;

	try {
		await restartRequest(38);
	} finally {
		globalThis.fetch = originalFetch;
	}

	assert.equal(calledUrl, '/api/v1/doc-review/requests/38/restart');
	assert.equal(calledMethod, 'POST');
});

test('restartRequest surfaces API failures', async () => {
	const originalFetch = globalThis.fetch;

	globalThis.fetch = (async () =>
		new Response(JSON.stringify({ status: false, error_msg: 'cannot rerun review' }), {
			status: 409,
			headers: { 'Content-Type': 'application/json' }
		})) as typeof fetch;

	try {
		await assert.rejects(() => restartRequest(91), /cannot rerun review/);
	} finally {
		globalThis.fetch = originalFetch;
	}
});

test('listReviewRuns reads from the review runs endpoint', async () => {
	const originalFetch = globalThis.fetch;
	let calledUrl = '';

	globalThis.fetch = (async (input: RequestInfo | URL) => {
		calledUrl = String(input);
		return new Response(JSON.stringify({ status: true, runs: [] }), {
			status: 200,
			headers: { 'Content-Type': 'application/json' }
		});
	}) as typeof fetch;

	try {
		await listReviewRuns({ runId: '42', status: 'completed' });
	} finally {
		globalThis.fetch = originalFetch;
	}

	assert.equal(calledUrl, '/api/v1/doc-review/runs?run_id=42&status=completed');
});

test('getRequest includes run_id when loading a specific review run', async () => {
	const originalFetch = globalThis.fetch;
	let calledUrl = '';

	globalThis.fetch = (async (input: RequestInfo | URL) => {
		calledUrl = String(input);
		return new Response(
			JSON.stringify({
				status: true,
				request: { id: 38, input_record_id: 12, tier: 'custom', aspects: [], requester_name: 'Chen', requester_id: 0, status: 'completed', create_time: '2026-07-05T00:00:00Z' },
				findings: [],
				aspect_statuses: [],
				packages: []
			}),
			{
				status: 200,
				headers: { 'Content-Type': 'application/json' }
			}
		);
	}) as typeof fetch;

	try {
		await getRequest(38, { runId: 91 });
	} finally {
		globalThis.fetch = originalFetch;
	}

	assert.equal(calledUrl, '/api/v1/doc-review/requests/38?run_id=91');
});
