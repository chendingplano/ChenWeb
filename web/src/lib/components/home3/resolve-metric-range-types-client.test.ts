import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildRangeTypeErrorsQuery,
	isInvalidMapEntry,
	listMetricRangeTypeErrors,
	listValueRangeTypeMapEntries,
	upsertValueRangeTypeMapEntry,
	type ValueRangeTypeMapEntry
} from './resolve-metric-range-types-client.js';

type FetchCall = { input: string | URL | Request; init?: RequestInit };

function installFetchMock(handler: (call: FetchCall) => Promise<Response>) {
	const originalFetch = globalThis.fetch;
	const calls: FetchCall[] = [];
	globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
		const call = { input, init };
		calls.push(call);
		return handler(call);
	}) as typeof fetch;
	return {
		calls,
		restore() {
			globalThis.fetch = originalFetch;
		}
	};
}

test('buildRangeTypeErrorsQuery is empty when no filters are set', () => {
	assert.equal(buildRangeTypeErrorsQuery({}), '');
});

test('buildRangeTypeErrorsQuery includes only the filters that are set', () => {
	const qs = buildRangeTypeErrorsQuery({ input_record_id: 244, value_range_type: 'threshold_min' });
	const params = new URLSearchParams(qs);
	assert.equal(params.get('input_record_id'), '244');
	assert.equal(params.get('value_range_type'), 'threshold_min');
	assert.equal(params.has('date_from'), false);
	assert.equal(params.has('date_to'), false);
});

test('buildRangeTypeErrorsQuery includes date_from and date_to when set', () => {
	const qs = buildRangeTypeErrorsQuery({ date_from: '2026-08-01', date_to: '2026-08-14' });
	const params = new URLSearchParams(qs);
	assert.equal(params.get('date_from'), '2026-08-01');
	assert.equal(params.get('date_to'), '2026-08-14');
});

test('listMetricRangeTypeErrors calls the range-type-errors endpoint with no query string when unfiltered', async () => {
	const mock = installFetchMock(async () => Response.json({ status: true, results: [], total: 0 }));
	try {
		await listMetricRangeTypeErrors();
		assert.equal(mock.calls.length, 1);
		assert.equal(mock.calls[0].input, '/api/v1/kb/metrics/range-type-errors');
	} finally {
		mock.restore();
	}
});

test('listMetricRangeTypeErrors appends filters to the query string', async () => {
	const mock = installFetchMock(async () => Response.json({ status: true, results: [], total: 0 }));
	try {
		await listMetricRangeTypeErrors({ input_record_id: 244 });
		assert.equal(mock.calls.length, 1);
		assert.equal(mock.calls[0].input, '/api/v1/kb/metrics/range-type-errors?input_record_id=244');
	} finally {
		mock.restore();
	}
});

test('listValueRangeTypeMapEntries loads the map entries list', async () => {
	const entries: ValueRangeTypeMapEntry[] = [
		{ raw_value: 'threshold_min', status: 'proposed', occurrence_count: 3 },
		{ raw_value: 'min', canonical_bucket: 'lower_bound', status: 'approved', occurrence_count: 17 }
	];
	const mock = installFetchMock(async () => Response.json({ status: true, results: entries, total: 2 }));
	try {
		const res = await listValueRangeTypeMapEntries();
		assert.equal(mock.calls[0].input, '/api/v1/kb/metric-value-range-type-map');
		assert.equal(res.total, 2);
		assert.deepEqual(res.results, entries);
	} finally {
		mock.restore();
	}
});

test('upsertValueRangeTypeMapEntry POSTs raw_value/canonical_bucket and returns the corrected count', async () => {
	const mock = installFetchMock(async () =>
		Response.json({
			status: true,
			entry: { raw_value: 'threshold_min', canonical_bucket: 'lower_bound', status: 'approved', occurrence_count: 3 },
			corrected_count: 3
		})
	);
	try {
		const res = await upsertValueRangeTypeMapEntry({
			raw_value: 'threshold_min',
			canonical_bucket: 'lower_bound'
		});
		assert.equal(mock.calls.length, 1);
		assert.equal(mock.calls[0].input, '/api/v1/kb/metric-value-range-type-map');
		assert.equal(mock.calls[0].init?.method, 'POST');
		assert.deepEqual(JSON.parse(String(mock.calls[0].init?.body)), {
			raw_value: 'threshold_min',
			canonical_bucket: 'lower_bound'
		});
		assert.equal(res.corrected_count, 3);
		assert.equal(res.entry.status, 'approved');
	} finally {
		mock.restore();
	}
});

test('upsertValueRangeTypeMapEntry surfaces the server error_msg on failure', async () => {
	const mock = installFetchMock(async () =>
		Response.json({ status: false, error_msg: 'canonical_bucket is required (CWB_KB_RTE_212)' }, { status: 400 })
	);
	try {
		await assert.rejects(
			() => upsertValueRangeTypeMapEntry({ raw_value: 'threshold_min', canonical_bucket: '' }),
			/canonical_bucket is required/
		);
	} finally {
		mock.restore();
	}
});

test('isInvalidMapEntry flags anything that is not approved', () => {
	assert.equal(isInvalidMapEntry({ raw_value: 'x', status: 'proposed', occurrence_count: 0 }), true);
	assert.equal(isInvalidMapEntry({ raw_value: 'x', status: 'ambiguous', occurrence_count: 0 }), true);
	assert.equal(isInvalidMapEntry({ raw_value: 'x', status: 'approved', occurrence_count: 0 }), false);
});
