import test from 'node:test';
import assert from 'node:assert/strict';
import { fetchApp, runAction, WpiError } from './api';

function fakeFetch(status: number, body: unknown, seen: { url?: string; init?: RequestInit } = {}): typeof fetch {
	return (async (url: string, init?: RequestInit) => {
		seen.url = url;
		seen.init = init;
		return new Response(JSON.stringify(body), { status });
	}) as unknown as typeof fetch;
}

test('fetchApp returns the bundle', async () => {
	const seen: { url?: string } = {};
	const bundle = await fetchApp('customer-admin', fakeFetch(200, { app: { descriptor_id: 'customer-admin' }, warnings: [] }, seen));
	assert.equal(seen.url, '/api/v1/wpi/apps/customer-admin');
	assert.equal(bundle.app.descriptor_id, 'customer-admin');
});

test('fetchApp surfaces validation errors', async () => {
	const issues = [{ kind: 'page', descriptor_id: 'p', path: '/x', message: 'bad' }];
	await assert.rejects(fetchApp('a', fakeFetch(422, { errors: issues })), (err: unknown) => {
		assert.ok(err instanceof WpiError);
		assert.equal(err.status, 422);
		assert.deepEqual(err.issues, issues);
		return true;
	});
});

test('runAction sends the reference, args and input doc', async () => {
	const seen: { init?: RequestInit } = {};
	const res = await runAction({ kind: 'action', id: 'save-customer' }, { id: 7 }, { email: 'e@example.com' }, fakeFetch(200, { key_attribute: 'id', key_value: 7 }, seen));
	assert.deepEqual(JSON.parse(String(seen.init?.body)), { ref: { kind: 'action', id: 'save-customer' }, args: { id: 7 }, input_doc: { email: 'e@example.com' } });
	assert.equal(res.key_value, 7);
});

test('runAction carries field errors', async () => {
	await assert.rejects(runAction({ kind: 'action', id: 's' }, {}, {}, fakeFetch(422, { field_errors: { email: ['required'] } })), (err: unknown) => {
		assert.ok(err instanceof WpiError);
		assert.deepEqual(err.fieldErrors, { email: ['required'] });
		return true;
	});
});
