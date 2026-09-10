import test from 'node:test';
import assert from 'node:assert/strict';

import {
	createReview,
	getRunResults,
	listAspects,
	rerunReview,
	runExportUrl,
	needsReconcileReview,
	type ProfileNode
} from './productMetricReviewService';

type Captured = { url: string; method: string; body: string };

function stubFetch(response: unknown, status = 200): { restore: () => void; last: () => Captured } {
	const original = globalThis.fetch;
	let captured: Captured = { url: '', method: 'GET', body: '' };
	globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
		captured = {
			url: String(input),
			method: init?.method ?? 'GET',
			body: typeof init?.body === 'string' ? init.body : ''
		};
		return new Response(JSON.stringify(response), {
			status,
			headers: { 'Content-Type': 'application/json' }
		});
	}) as typeof fetch;
	return { restore: () => void (globalThis.fetch = original), last: () => captured };
}

test('createReview POSTs the review body to the reviews endpoint', async () => {
	const f = stubFetch({ status: true, run: { id: 5, run_number: 1, status: 'completed' } });
	try {
		const out = await createReview({ profile_id: 7, artifact_types: ['metric'] });
		assert.equal(f.last().url, '/api/v1/kb/product-reviews');
		assert.equal(f.last().method, 'POST');
		assert.deepEqual(JSON.parse(f.last().body), { profile_id: 7, artifact_types: ['metric'] });
		assert.equal(out.run.status, 'completed');
	} finally {
		f.restore();
	}
});

test('getRunResults serializes node/tier/exclude filters into the query string', async () => {
	const f = stubFetch({ status: true, results: [], count: 0 });
	try {
		await getRunResults(91, { node_id: 2, exclude_tier: ['document_scope'], path: 'direct' });
		const url = f.last().url;
		assert.ok(url.startsWith('/api/v1/kb/product-reviews/runs/91/results?'));
		assert.ok(url.includes('node_id=2'));
		assert.ok(url.includes('exclude_tier=document_scope'));
		assert.ok(url.includes('path=direct'));
	} finally {
		f.restore();
	}
});

test('listAspects passes the locale through as ?lang=', async () => {
	const f = stubFetch({ status: true, aspects: [] });
	try {
		await listAspects('zh-cn');
		assert.equal(f.last().url, '/api/v1/kb/product-reviews/aspects?lang=zh-cn');
	} finally {
		f.restore();
	}
});

test('rerunReview POSTs to the request rerun endpoint', async () => {
	const f = stubFetch({ status: true, run: { id: 6, run_number: 2, status: 'completed' } });
	try {
		const out = await rerunReview(40);
		assert.equal(f.last().url, '/api/v1/kb/product-reviews/40/rerun');
		assert.equal(f.last().method, 'POST');
		assert.equal(out.run.run_number, 2);
	} finally {
		f.restore();
	}
});

test('a { status: false } envelope surfaces its error_msg', async () => {
	const f = stubFetch(
		{ status: false, error_code: 'CWB_KB_PMR_030', error_msg: 'profile is still draft' },
		422
	);
	try {
		await assert.rejects(() => createReview({ profile_id: 7 }), /profile is still draft/);
	} finally {
		f.restore();
	}
});

test('runExportUrl builds a plain URL with the same filters (for an <a href>)', () => {
	assert.equal(
		runExportUrl(91, { tier: 'part' }),
		'/api/v1/kb/product-reviews/runs/91/export?tier=part'
	);
	assert.equal(runExportUrl(91), '/api/v1/kb/product-reviews/runs/91/export');
});

test('needsReconcileReview flags ambiguous / pending_review nodes only', () => {
	const base = { reconcile_status: '' } as ProfileNode;
	assert.equal(needsReconcileReview(base), false);
	assert.equal(needsReconcileReview({ ...base, reconcile_status: 'ambiguous' }), true);
	assert.equal(needsReconcileReview({ ...base, reconcile_status: 'pending_review' }), true);
	assert.equal(needsReconcileReview({ ...base, reconcile_status: 'active' }), false);
});
