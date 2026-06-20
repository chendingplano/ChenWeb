import test from 'node:test';
import assert from 'node:assert/strict';

import {
	createLLMAccount,
	applyLLMAccountsImport,
	importLLMAccountsPreview,
	listLLMAccounts,
	updateLLMAccount,
	type CreateLLMAccountInput
} from './llm-accounts-client.js';

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

test('listLLMAccounts loads account rows from the admin endpoint', async () => {
	const mock = installFetchMock(async () =>
		Response.json({
			accounts: [
				{
					id: 'acct-1',
					account_name: 'DeepSeek Prod',
					provider: 'deepseek',
					base_url: 'https://api.deepseek.com',
					status: 'active',
					is_reconciliation_enabled: true,
					default_model_name: 'deepseek-chat',
					created_at: '2026-06-19T00:00:00Z',
					updated_at: '2026-06-19T01:00:00Z',
					profile_count: 2
				}
			]
		})
	);

	try {
		const response = await listLLMAccounts();

		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/llm/accounts');
		assert.equal(mock.calls[0].init?.credentials, 'same-origin');
		assert.equal(response.accounts.length, 1);
		assert.equal(response.accounts[0].account_name, 'DeepSeek Prod');
	} finally {
		mock.restore();
	}
});

test('createLLMAccount posts the new account payload', async () => {
	const payload: CreateLLMAccountInput = {
		account_name: 'DeepSeek Sandbox',
		provider: 'deepseek',
		base_url: 'https://api.deepseek.com',
		api_key: 'sk-test',
		status: 'active',
		reconciliation_kind: 'provider_balance',
		is_reconciliation_enabled: true,
		default_model_name: 'deepseek-chat'
	};

	const mock = installFetchMock(async () =>
		new Response(
			JSON.stringify({
				id: 'acct-2',
				account_name: 'DeepSeek Sandbox',
				provider: 'deepseek',
				base_url: 'https://api.deepseek.com',
				status: 'active',
				is_reconciliation_enabled: true,
				default_model_name: 'deepseek-chat',
				created_at: '2026-06-19T00:00:00Z',
				updated_at: '2026-06-19T00:00:00Z',
				profile_count: 0
			}),
			{
				status: 201,
				headers: { 'Content-Type': 'application/json' }
			}
		)
	);

	try {
		const account = await createLLMAccount(payload);

		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/llm/accounts');
		assert.equal(mock.calls[0].init?.method, 'POST');
		assert.equal(mock.calls[0].init?.credentials, 'same-origin');
		assert.deepEqual(JSON.parse(String(mock.calls[0].init?.body)), payload);
		assert.equal(account.id, 'acct-2');
	} finally {
		mock.restore();
	}
});

test('createLLMAccount surfaces backend error messages', async () => {
	const mock = installFetchMock(async () =>
		new Response(JSON.stringify({ error_msg: 'provider is required' }), {
			status: 400,
			headers: { 'Content-Type': 'application/json' }
		})
	);

	try {
		await assert.rejects(
			() =>
				createLLMAccount({
					account_name: 'Incomplete',
					provider: '',
					base_url: '',
					api_key: '',
					status: 'active',
					reconciliation_kind: 'provider_balance',
					is_reconciliation_enabled: false,
					default_model_name: ''
				}),
			/provider is required/
		);
	} finally {
		mock.restore();
	}
});

test('updateLLMAccount puts the edited account payload', async () => {
	const payload: CreateLLMAccountInput = {
		account_name: 'DeepSeek Prod',
		provider: 'deepseek',
		base_url: 'https://api.deepseek.com',
		api_key: 'sk-updated',
		status: 'active',
		reconciliation_kind: 'provider_balance',
		is_reconciliation_enabled: true,
		default_model_name: 'deepseek-v4-flash'
	};

	const mock = installFetchMock(async () =>
		new Response(
			JSON.stringify({
				id: 'acct-1',
				account_name: 'DeepSeek Prod',
				provider: 'deepseek',
				base_url: 'https://api.deepseek.com',
				status: 'active',
				is_reconciliation_enabled: true,
				default_model_name: 'deepseek-v4-flash',
				created_at: '2026-06-19T00:00:00Z',
				updated_at: '2026-06-20T00:00:00Z',
				profile_count: 3
			}),
			{
				status: 200,
				headers: { 'Content-Type': 'application/json' }
			}
		)
	);

	try {
		const account = await updateLLMAccount('acct-1', payload);

		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/llm/accounts/acct-1');
		assert.equal(mock.calls[0].init?.method, 'PUT');
		assert.deepEqual(JSON.parse(String(mock.calls[0].init?.body)), payload);
		assert.equal(account.id, 'acct-1');
	} finally {
		mock.restore();
	}
});

test('importLLMAccountsPreview calls the bootstrap preview endpoint', async () => {
	const mock = installFetchMock(async () =>
		Response.json({
			ok: true,
			path: '/tmp/.models.toml',
			accounts: [{ account_name: 'DeepSeek Prod' }],
			profiles: [{ profile_name: 'deepseek-chat' }]
		})
	);

	try {
		const preview = await importLLMAccountsPreview();

		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/llm/accounts/import-models-toml');
		assert.equal(mock.calls[0].init?.method, 'POST');
		assert.equal(preview.ok, true);
		assert.equal(preview.accounts.length, 1);
	} finally {
		mock.restore();
	}
});

test('applyLLMAccountsImport calls the bootstrap apply endpoint', async () => {
	const mock = installFetchMock(async () =>
		Response.json({
			ok: true,
			path: '/tmp/.models.toml',
			accounts_imported: 2,
			profiles_imported: 3
		})
	);

	try {
		const result = await applyLLMAccountsImport();

		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/llm/accounts/import-models-toml/apply');
		assert.equal(mock.calls[0].init?.method, 'POST');
		assert.equal(result.accounts_imported, 2);
		assert.equal(result.profiles_imported, 3);
	} finally {
		mock.restore();
	}
});
