import test from 'node:test';
import assert from 'node:assert/strict';
import { savePage, WpiError } from './api';

test('saves the page descriptor to the versioned WPI endpoint', async () => {
	const page = { descriptor_id: 'demo/page', elements: [] };
	const fetchFn = (async (url, init) => {
		assert.equal(url, '/api/v1/wpi/descriptors/page/demo%2Fpage');
		assert.equal(init?.method, 'PUT');
		assert.deepEqual(JSON.parse(String(init?.body)), page);
		return Response.json({ version: 3 });
	}) as typeof fetch;
	assert.equal(await savePage('demo/page', page, fetchFn), 3);
});

test('returns validation errors rather than reporting a save', async () => {
	const fetchFn = (async () => Response.json({ errors: [{ path: 'elements', message: 'Invalid' }] }, { status: 400 })) as typeof fetch;
	await assert.rejects(() => savePage('demo', {}, fetchFn), (error: WpiError) => error.status === 400 && error.issues[0].message === 'Invalid');
});
