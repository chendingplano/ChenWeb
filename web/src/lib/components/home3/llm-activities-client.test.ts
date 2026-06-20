import test from 'node:test';
import assert from 'node:assert/strict';

import {
	getLLMTodaySummary,
	listLLMCurrentBalances,
	listLLMDailyReports,
	listLLMUsageEvents,
	runLLMReconciliationNow
} from './llm-activities-client.js';

type FetchCall = {
	input: string | URL | Request;
	init?: RequestInit;
};

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

test('listLLMDailyReports applies the limit query parameter', async () => {
	const mock = installFetchMock(async () =>
		Response.json({
			reports: [
				{
					account_id: 'acct-1',
					account_name: 'deepseek:api.deepseek.com',
					workspace_day: '2026-06-19T00:00:00Z',
					timezone_name: 'America/Chicago',
					opening_balance: 50,
					closing_balance: 44.5,
					spend_amount: 5.5,
					currency_code: 'USD',
					input_tokens: 1200,
					output_tokens: 3400,
					total_tokens: 4600,
					request_count: 23,
					reconciliation_status: 'reconciled'
				}
			]
		})
	);

	try {
		const response = await listLLMDailyReports(14);

		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/llm/reports/daily?limit=14');
		assert.equal(mock.calls[0].init?.credentials, 'same-origin');
		assert.equal(response.reports[0].spend_amount, 5.5);
	} finally {
		mock.restore();
	}
});

test('listLLMUsageEvents applies the limit query parameter', async () => {
	const mock = installFetchMock(async () =>
		Response.json({
			usage_events: [
				{
					id: 'evt-1',
					account_id: 'acct-1',
					account_name: 'deepseek:api.deepseek.com',
					profile_id: 'prof-1',
					provider: 'deepseek',
					model_name: 'deepseek-chat',
					prompt_name: 'daily-summary',
					request_started_at: '2026-06-19T03:00:00Z',
					input_tokens: 100,
					output_tokens: 50,
					total_tokens: 150,
					latency_ms: 2400,
					error_message: ''
				}
			]
		})
	);

	try {
		const response = await listLLMUsageEvents(25);

		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/llm/usage-events?limit=25');
		assert.equal(mock.calls[0].init?.credentials, 'same-origin');
		assert.equal(response.usage_events[0].prompt_name, 'daily-summary');
	} finally {
		mock.restore();
	}
});

test('listLLMCurrentBalances applies the limit query parameter', async () => {
	const mock = installFetchMock(async () =>
		Response.json({
			balances: [
				{
					account_id: 'acct-1',
					account_name: 'deepseek:api.deepseek.com',
					provider: 'deepseek',
					workspace_day: '2026-06-20T00:00:00Z',
					captured_at: '2026-06-20T15:42:19Z',
					balance_amount: 475.59,
					currency_code: 'CNY'
				}
			]
		})
	);

	try {
		const response = await listLLMCurrentBalances(7);

		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/llm/balances/current?limit=7');
		assert.equal(mock.calls[0].init?.credentials, 'same-origin');
		assert.equal(response.balances[0].balance_amount, 475.59);
	} finally {
		mock.restore();
	}
});

test('getLLMTodaySummary loads the today aggregate endpoint', async () => {
	const mock = installFetchMock(async () =>
		Response.json({
			summary: {
				workspace_day: '2026-06-20',
				timezone_name: 'America/Chicago',
				spend_amount: 12.34,
				currency_code: 'CNY',
				request_count: 7,
				total_tokens: 4567,
				error_count: 2
			}
		})
	);

	try {
		const response = await getLLMTodaySummary();

		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/llm/summary/today');
		assert.equal(mock.calls[0].init?.credentials, 'same-origin');
		assert.equal(response.summary.request_count, 7);
		assert.equal(response.summary.total_tokens, 4567);
	} finally {
		mock.restore();
	}
});

test('listLLMUsageEvents falls back to http status when the backend returns non-json', async () => {
	const mock = installFetchMock(async () => new Response('server unavailable', { status: 503 }));

	try {
		await assert.rejects(() => listLLMUsageEvents(), /HTTP 503/);
	} finally {
		mock.restore();
	}
});

test('runLLMReconciliationNow posts to the manual reconciliation endpoint', async () => {
	const mock = installFetchMock(async () => Response.json({ ok: true }));

	try {
		const response = await runLLMReconciliationNow();

		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/llm/reconciliation/run');
		assert.equal(mock.calls[0].init?.method, 'POST');
		assert.equal(mock.calls[0].init?.credentials, 'same-origin');
		assert.equal(response.ok, true);
	} finally {
		mock.restore();
	}
});
