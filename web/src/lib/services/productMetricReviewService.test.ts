import test from 'node:test';
import assert from 'node:assert/strict';

import {
	createReview,
	getRunResults,
	listAspects,
	listProfiles,
	rerunReview,
	runExportUrl,
	needsReconcileReview,
	setProfileDrawing,
	startProductReviewIntake,
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

test('setProfileDrawing PATCHes the drawing id to the profile', async () => {
	const f = stubFetch({ status: true });
	try {
		const out = await setProfileDrawing(7, 42);
		assert.equal(f.last().url, '/api/v1/kb/product-profiles/7/drawing');
		assert.equal(f.last().method, 'PATCH');
		assert.deepEqual(JSON.parse(f.last().body), { drawing_id: 42 });
		assert.equal(out.status, true);
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

test('listProfiles GETs the profile list endpoint with no query by default', async () => {
	const f = stubFetch({ status: true, profiles: [{ id: 1, name: 'Ventilator' }] });
	try {
		const out = await listProfiles();
		assert.equal(f.last().url, '/api/v1/kb/product-profiles');
		assert.equal(f.last().method, 'GET');
		assert.equal(out.profiles.length, 1);
		assert.equal(out.profiles[0].name, 'Ventilator');
	} finally {
		f.restore();
	}
});

test('listProfiles passes a limit through as a query param', async () => {
	const f = stubFetch({ status: true, profiles: [] });
	try {
		await listProfiles(10);
		assert.equal(f.last().url, '/api/v1/kb/product-profiles?limit=10');
	} finally {
		f.restore();
	}
});

test('listProfiles serializes sort, repeated keywords, and name filters', async () => {
	const f = stubFetch({ status: true, profiles: [], keywords: ['icu', 'respiratory'] });
	try {
		await listProfiles({ sort: 'metrics_desc', keywords: ['icu', 'respiratory'], name: 'vent' });
		const url = f.last().url;
		assert.ok(url.includes('sort=metrics_desc'));
		assert.ok(url.includes('keywords=icu'));
		assert.ok(url.includes('keywords=respiratory'));
		assert.ok(url.includes('name=vent'));
	} finally {
		f.restore();
	}
});

test('listProfiles surfaces a { status: false } envelope as a rejected promise', async () => {
	const f = stubFetch({ status: false, error_msg: 'db unavailable' }, 500);
	try {
		await assert.rejects(() => listProfiles(), /db unavailable/);
	} finally {
		f.restore();
	}
});

test('startProductReviewIntake POSTs the form fields to the intake endpoint', async () => {
	const f = stubFetch({
		status: true,
		duplicate: false,
		profile: { id: 1, name: 'Ventilator' },
		run: { id: 9, run_number: 1, status: 'completed' }
	});
	try {
		const out = await startProductReviewIntake({
			name: 'Ventilator',
			product_description: 'ICU ventilator',
			keywords: ['icu', 'respiratory'],
			notes: 'urgent'
		});
		assert.equal(f.last().url, '/api/v1/kb/product-reviews/intake');
		assert.equal(f.last().method, 'POST');
		assert.deepEqual(JSON.parse(f.last().body), {
			name: 'Ventilator',
			product_description: 'ICU ventilator',
			keywords: ['icu', 'respiratory'],
			notes: 'urgent'
		});
		assert.equal(out.duplicate, false);
		assert.equal(out.run?.id, 9);
	} finally {
		f.restore();
	}
});

test('startProductReviewIntake surfaces a duplicate response without a run', async () => {
	const f = stubFetch({
		status: true,
		duplicate: true,
		profile: { id: 1, name: 'Ventilator' },
		latest_request_id: 40,
		latest_run: { id: 6, run_number: 2, status: 'completed' }
	});
	try {
		const out = await startProductReviewIntake({ name: 'Ventilator' });
		assert.equal(out.duplicate, true);
		assert.equal(out.run, undefined);
		assert.equal(out.latest_request_id, 40);
		assert.equal(out.latest_run?.id, 6);
	} finally {
		f.restore();
	}
});

test('startProductReviewIntake passes resume_profile_id through for the draft re-run path', async () => {
	const f = stubFetch({
		status: true,
		duplicate: false,
		profile: { id: 1, name: 'Ventilator' },
		run: { id: 10, run_number: 1, status: 'completed' }
	});
	try {
		await startProductReviewIntake({ name: 'Ventilator', resume_profile_id: 1 });
		assert.deepEqual(JSON.parse(f.last().body), { name: 'Ventilator', resume_profile_id: 1 });
	} finally {
		f.restore();
	}
});

test('startProductReviewIntake surfaces a failed build as a rejected promise', async () => {
	const f = stubFetch({ status: false, error_msg: 'LLM proposer failed' }, 422);
	try {
		await assert.rejects(
			() => startProductReviewIntake({ name: 'Ventilator' }),
			/LLM proposer failed/
		);
	} finally {
		f.restore();
	}
});

test('needsReconcileReview flags ambiguous / pending_review nodes only', () => {
	const base = { reconcile_status: '' } as ProfileNode;
	assert.equal(needsReconcileReview(base), false);
	assert.equal(needsReconcileReview({ ...base, reconcile_status: 'ambiguous' }), true);
	assert.equal(needsReconcileReview({ ...base, reconcile_status: 'pending_review' }), true);
	assert.equal(needsReconcileReview({ ...base, reconcile_status: 'active' }), false);
});
