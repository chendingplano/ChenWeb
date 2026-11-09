import test from 'node:test';
import assert from 'node:assert/strict';
import {
	approveObjectType, filterObjectTypes, listObjectTypes, restoreObjectTypesState,
	type ObjectType
} from './object-types-client.js';

const proposed: ObjectType = {
	id: 12, name: 'sensor_network', status: 'proposed', raw_value: 'Sensor network',
	source_record_id: 224, source_run_id: 172, source_processor: 'extract_provisions',
	source_model: 'model-a', source_prompt: 'prompt-a', source_spans: ['42-45'],
	created_at: '2026-11-08T20:00:00Z', approved_at: null, approved_by: ''
};
const approved: ObjectType = { ...proposed, id: 1, name: 'equipment', status: 'approved' };

test('filters proposals and searches original values, provenance and record IDs', () => {
	const rows = [proposed, approved];
	assert.deepEqual(filterObjectTypes(rows, 'proposed', ''), [proposed]);
	assert.deepEqual(filterObjectTypes(rows, 'approved', ''), [approved]);
	assert.deepEqual(filterObjectTypes(rows, 'all', 'MODEL-A'), rows);
	assert.deepEqual(filterObjectTypes(rows, 'proposed', ' sensor network '), [proposed]);
	assert.deepEqual(filterObjectTypes(rows, 'all', '224'), rows);
	assert.deepEqual(filterObjectTypes(rows, 'all', 'unknown'), []);
});

test('restores filter, search, selection and scroll on a later mount', () => {
	const saved = { filter: 'all' as const, search: 'sensor', selectedId: 12, scrollTop: 780, listScrollTop: 120 };
	assert.deepEqual(restoreObjectTypesState(JSON.stringify(saved)), saved);
	assert.deepEqual(restoreObjectTypesState('{broken'), {
		filter: 'proposed', search: '', selectedId: null, scrollTop: 0, listScrollTop: 0
	});
	assert.deepEqual(restoreObjectTypesState(JSON.stringify({ filter: 'invalid', search: 42, selectedId: -1, scrollTop: -8 })), {
		filter: 'proposed', search: '', selectedId: null, scrollTop: 0, listScrollTop: 0
	});
});

test('uses authenticated list and approval endpoints, and exposes provider errors', async () => {
	const originalFetch = globalThis.fetch;
	const calls: { path: string; init?: RequestInit }[] = [];
	globalThis.fetch = (async (path: string | URL | Request, init?: RequestInit) => {
		calls.push({ path: String(path), init });
		if (calls.length === 1) return Response.json({ status: true, rows: [proposed] });
		if (calls.length === 2) return Response.json({ status: true, row: { ...proposed, status: 'approved' } });
		return Response.json({ status: false, error_msg: 'Administrator required' }, { status: 403 });
	}) as typeof fetch;
	try {
		assert.deepEqual((await listObjectTypes()).rows, [proposed]);
		assert.equal((await approveObjectType(12)).row.status, 'approved');
		assert.equal(calls[0].path, '/api/v1/kb/object-types');
		assert.equal(calls[0].init?.credentials, 'same-origin');
		assert.equal(calls[1].path, '/api/v1/kb/object-types/12/approve');
		assert.equal(calls[1].init?.method, 'POST');
		await assert.rejects(approveObjectType(12), /Administrator required/);
	} finally {
		globalThis.fetch = originalFetch;
	}
});
