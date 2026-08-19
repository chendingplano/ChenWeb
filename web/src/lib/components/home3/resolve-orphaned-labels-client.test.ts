import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildOrphanedLabelsQuery,
	listOrphanedLabels,
	resolveOrphanedLabels
} from './resolve-orphaned-labels-client.js';

type FetchCall = { input: string | URL | Request; init?: RequestInit };

function installFetchMock(handler: (call: FetchCall) => Promise<Response>) {
	const originalFetch = globalThis.fetch;
	const calls: FetchCall[] = [];
	globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
		const call = { input, init };
		calls.push(call);
		return handler(call);
	}) as typeof fetch;
	return { calls, restore: () => { globalThis.fetch = originalFetch; } };
}

test('buildOrphanedLabelsQuery encodes all filters and omits blanks', () => {
	assert.equal(buildOrphanedLabelsQuery({}), '');
	assert.equal(
		buildOrphanedLabelsQuery({ q: '垃圾 标签', lang: 'zh-cn', label_role: 'prefLabel' }),
		'q=%E5%9E%83%E5%9C%BE+%E6%A0%87%E7%AD%BE&lang=zh-cn&label_role=prefLabel'
	);
});

test('listOrphanedLabels loads the filtered orphan rows', async () => {
	const mock = installFetchMock(async () => Response.json({ status: true, results: [], total: 0 }));
	try {
		const result = await listOrphanedLabels({ q: 'orphan' });
		assert.equal(String(mock.calls[0].input), '/api/v1/admin/db/ontology-term-labels/orphans?q=orphan');
		assert.equal(result.total, 0);
	} finally {
		mock.restore();
	}
});

test('resolveOrphanedLabels posts the visible row ids', async () => {
	const mock = installFetchMock(async () => Response.json({ status: true, deleted_count: 2 }));
	try {
		const result = await resolveOrphanedLabels([42, 43], { q: 'orphan', lang: 'en' });
		assert.equal(String(mock.calls[0].input), '/api/v1/admin/db/ontology-term-labels/orphans/resolve');
		assert.equal(mock.calls[0].init?.method, 'POST');
		assert.equal(
			mock.calls[0].init?.body,
			JSON.stringify({ ids: [42, 43], q: 'orphan', lang: 'en', label_role: '' })
		);
		assert.equal(result.deleted_count, 2);
	} finally {
		mock.restore();
	}
});
