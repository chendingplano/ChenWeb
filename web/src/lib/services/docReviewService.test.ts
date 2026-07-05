import test from 'node:test';
import assert from 'node:assert/strict';

import { restartRequest } from './docReviewService';

test('restartRequest posts to the review restart endpoint', async () => {
	const originalFetch = globalThis.fetch;
	let calledUrl = '';
	let calledMethod = '';

	globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
		calledUrl = String(input);
		calledMethod = init?.method ?? 'GET';

		return new Response(JSON.stringify({ status: true }), {
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
