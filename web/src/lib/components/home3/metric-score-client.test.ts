import test from 'node:test';
import assert from 'node:assert/strict';
import {
	ScoreAPIError,
	goldRunKey,
	startScore,
	scoreHistory,
	scoreDetail
} from './metric-score-client.js';

test('benchmark client preserves selected gold run and report language, filters history, and exposes server error codes', async () => {
	const original = globalThis.fetch;
	const calls: { url: string; init?: RequestInit }[] = [];
	globalThis.fetch = (async (url: unknown, init?: RequestInit) => {
		calls.push({ url: String(url), init });
		if (String(url).endsWith('/99'))
			return new Response(JSON.stringify({ status: false, error_code: 'NO_GOLD' }), {
				status: 422
			});
		return new Response(JSON.stringify({ status: true, run: { id: 12 }, runs: [], total: 0 }));
	}) as typeof fetch;
	try {
		await startScore(416, 'model-one', 'zh-cn', {
			skill_version: '3.1.0',
			model_name: 'gold-model',
			benchmark_run_id: 'run-one',
			rows: 4
		});
		assert.deepEqual(JSON.parse(String(calls[0].init?.body)), {
			record_id: 416,
			model: 'model-one',
			lang: 'zh-cn',
			gold_version: '3.1.0',
			gold_model: 'gold-model',
			gold_run_id: 'run-one'
		});
		assert.equal(calls[0].init?.credentials, 'same-origin');
		await startScore(416, 'model-one', 'en');
		assert.equal('gold_run_id' in JSON.parse(String(calls[1].init?.body)), false);
		await scoreHistory(416, 20);
		const params = new URL(calls[2].url, 'http://localhost').searchParams;
		assert.equal(params.get('record_id'), '416');
		assert.equal(params.get('offset'), '20');
		assert.equal(params.get('limit'), '20');
		await assert.rejects(
			scoreDetail(99),
			(error: unknown) =>
				error instanceof ScoreAPIError && error.code === 'NO_GOLD' && error.status === 422
		);
	} finally {
		globalThis.fetch = original;
	}
});

test('gold selector distinguishes versions and models sharing a run ID', () => {
	const run = { skill_version: '3.1.0', model_name: 'one', benchmark_run_id: 'shared', rows: 2 };
	assert.notEqual(goldRunKey(run), goldRunKey({ ...run, model_name: 'two' }));
	assert.notEqual(goldRunKey(run), goldRunKey({ ...run, skill_version: '3.2.0' }));
});
