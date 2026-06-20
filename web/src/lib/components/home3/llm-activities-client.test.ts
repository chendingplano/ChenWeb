import test from 'node:test';
import assert from 'node:assert/strict';

import { listLLMDailyReports, listLLMUsageEvents } from './llm-activities-client.js';

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

test('listLLMUsageEvents falls back to http status when the backend returns non-json', async () => {
	const mock = installFetchMock(async () => new Response('server unavailable', { status: 503 }));

	try {
		await assert.rejects(() => listLLMUsageEvents(), /HTTP 503/);
	} finally {
		mock.restore();
	}
});
