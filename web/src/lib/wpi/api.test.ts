import test from 'node:test';
import assert from 'node:assert/strict';
import { createElementClass, fetchApp, getElementClass, listCodeModules, listElementClasses, runAction, updateElementClass, WpiError } from './api';

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
	const res = await runAction({ kind: 'page', id: 'customer-form', pointer: '/save_action' }, { id: 7 }, { email: 'e@example.com' }, fakeFetch(200, { key_attribute: 'id', key_value: 7 }, seen));
	assert.deepEqual(JSON.parse(String(seen.init?.body)), { ref: { kind: 'page', id: 'customer-form', pointer: '/save_action' }, args: { id: 7 }, input_doc: { email: 'e@example.com' } });
	assert.equal(res.key_value, 7);
});

test('runAction carries field errors', async () => {
	await assert.rejects(runAction({ kind: 'page', id: 'p', pointer: '/save_action' }, {}, {}, fakeFetch(422, { field_errors: { email: ['required'] } })), (err: unknown) => {
		assert.ok(err instanceof WpiError);
		assert.deepEqual(err.fieldErrors, { email: ['required'] });
		return true;
	});
});

test('listElementClasses returns the classes', async () => {
	const seen: { url?: string } = {};
	const classes = await listElementClasses(fakeFetch(200, { classes: [{ descriptor_id: 'slider', version: 1, status: 'draft', description: 'A slider' }] }, seen));
	assert.equal(seen.url, '/api/v1/wpi/classes/element');
	assert.equal(classes[0].status, 'draft');
});

test('createElementClass posts the class row and returns its warnings', async () => {
	const seen: { url?: string; init?: RequestInit } = {};
	const none = await createElementClass({ descriptor_id: 'slider', schema_version: '0.4' }, fakeFetch(201, { status: 'draft' }, seen));
	assert.equal(seen.url, '/api/v1/wpi/classes/element');
	assert.equal(seen.init?.method, 'POST');
	assert.deepEqual(JSON.parse(String(seen.init?.body)), { descriptor_id: 'slider', schema_version: '0.4' });
	assert.deepEqual(none, []);
	const warning = { kind: 'element', descriptor_id: 'fancy-button', path: '/glow', message: 'code module "button" does not support attribute "glow", so it has no effect' };
	assert.deepEqual(await createElementClass({}, fakeFetch(201, { status: 'draft', warnings: [warning] })), [warning]);
});

test('createElementClass carries the status and issues', async () => {
	await assert.rejects(createElementClass({}, fakeFetch(409, { error: 'element class "input" already exists' })), (err: unknown) => {
		assert.ok(err instanceof WpiError);
		assert.equal(err.status, 409);
		assert.equal(err.message, 'element class "input" already exists');
		return true;
	});
	const issues = [{ kind: 'element', descriptor_id: 'slider', path: '/description', message: 'a draft class needs a description' }];
	await assert.rejects(createElementClass({}, fakeFetch(400, { errors: issues })), (err: unknown) => {
		assert.ok(err instanceof WpiError);
		assert.deepEqual(err.issues, issues);
		return true;
	});
});

test('getElementClass returns the whole class row', async () => {
	const seen: { url?: string } = {};
	const row = await getElementClass('slider', fakeFetch(200, { version: 2, status: 'draft', class: { descriptor_id: 'slider' } }, seen));
	assert.equal(seen.url, '/api/v1/wpi/classes/element/slider');
	assert.deepEqual(row, { version: 2, status: 'draft', class: { descriptor_id: 'slider' } });
	await assert.rejects(getElementClass('gone', fakeFetch(404, { error: 'Element class not found' })), (err: unknown) => {
		assert.ok(err instanceof WpiError);
		assert.equal(err.status, 404);
		return true;
	});
});

test('updateElementClass puts the class row and carries the issues', async () => {
	const seen: { url?: string; init?: RequestInit } = {};
	const res = await updateElementClass('slider', { descriptor_id: 'slider' }, fakeFetch(200, { descriptor_id: 'slider', version: 3, status: 'draft' }, seen));
	assert.equal(seen.url, '/api/v1/wpi/classes/element/slider');
	assert.equal(seen.init?.method, 'PUT');
	assert.deepEqual(JSON.parse(String(seen.init?.body)), { descriptor_id: 'slider' });
	assert.equal(res.version, 3);
	assert.deepEqual(res.warnings, []);
	const warning = { kind: 'element', descriptor_id: 'button', path: '/glow', message: 'code module "button" does not support attribute "glow", so it has no effect' };
	const warned = await updateElementClass('button', {}, fakeFetch(200, { descriptor_id: 'button', version: 4, status: 'active', warnings: [warning] }));
	assert.deepEqual(warned.warnings, [warning]);
	const issues = [{ kind: 'element', descriptor_id: 'input', path: '/slider_min', message: 'unknown attribute "slider_min"' }];
	await assert.rejects(updateElementClass('input', {}, fakeFetch(400, { errors: issues })), (err: unknown) => {
		assert.ok(err instanceof WpiError);
		assert.deepEqual(err.issues, issues);
		return true;
	});
});

test('listCodeModules returns the rows of kb.code_modules', async () => {
	const seen: { url?: string; init?: RequestInit } = {};
	const button = { code_module_id: 'button', module_name: 'Button', description: 'A button', module_type: 'element', module_doc: 'button.md', notes: '', status: 'active' };
	assert.deepEqual(await listCodeModules(fakeFetch(200, { code_modules: [button] }, seen)), [button]);
	assert.equal(seen.url, '/api/v1/wpi/code-modules');
	await assert.rejects(listCodeModules(fakeFetch(401, { error: 'Authentication required' })), (err: unknown) => err instanceof WpiError && err.status === 401);
});
