import test from 'node:test';
import assert from 'node:assert/strict';
import { savePage, WpiError } from './api';

test('saves the page descriptor to the versioned WPI endpoint and returns the stored page', async () => {
	const page = { descriptor_id: 'demo/page', panels: {} };
	const stored = { ...page, page_layouter: { layouter: 'single-pane', class: { descriptor_id: 'single-pane' } } };
	const fetchFn = (async (url, init) => {
		assert.equal(url, '/api/v1/wpi/descriptors/page/demo%2Fpage');
		assert.equal(init?.method, 'PUT');
		assert.deepEqual(JSON.parse(String(init?.body)), page);
		return Response.json({ version: 3, page: stored });
	}) as typeof fetch;
	assert.deepEqual(await savePage('demo/page', page, fetchFn), { version: 3, page: stored });
});

test('returns validation errors rather than reporting a save', async () => {
	const fetchFn = (async () => Response.json({ errors: [{ path: 'panels', message: 'Invalid' }] }, { status: 400 })) as typeof fetch;
	await assert.rejects(() => savePage('demo', {}, fetchFn), (error: WpiError) => error.status === 400 && error.issues[0].message === 'Invalid');
});

test('requests create-only page storage when creating a page', async () => {
 const fetchFn = (async (_url, init) => {
  assert.equal(new Headers(init?.headers).get('If-None-Match'), '*');
  return Response.json({ version: 1, page: {} });
 }) as typeof fetch;
 await savePage('example', {}, fetchFn, true);
});

test('includes the temporary recovery identity when creating a page', async () => {
 const fetchFn = (async (_url, init) => {
  assert.equal(new Headers(init?.headers).get('X-WPI-Recovery-ID'), 'unsaved-123');
  return Response.json({ version: 1, page: {} });
 }) as typeof fetch;
 await savePage('example', {}, fetchFn, true, 'unsaved-123');
});
