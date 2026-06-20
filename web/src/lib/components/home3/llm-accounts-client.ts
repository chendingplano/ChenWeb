export type LLMAccount = {
	id: string;
	account_name: string;
	provider: string;
	base_url: string;
	status: string;
	is_reconciliation_enabled: boolean;
	default_model_name: string;
	created_at: string;
	updated_at: string;
	profile_count: number;
};

export type ListLLMAccountsResponse = {
	accounts: LLMAccount[];
};

export type CreateLLMAccountInput = {
	account_name: string;
	provider: string;
	base_url: string;
	api_key: string;
	status: string;
	reconciliation_kind: string;
	is_reconciliation_enabled: boolean;
	default_model_name: string;
};

export type ImportLLMAccountsPreviewResponse = {
	ok: boolean;
	path: string;
	accounts: Array<Record<string, unknown>>;
	profiles: Array<Record<string, unknown>>;
};

export type ApplyLLMAccountsImportResponse = {
	ok: boolean;
	path: string;
	accounts_imported: number;
	profiles_imported: number;
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, {
		credentials: 'same-origin',
		...init
	});
	const text = await res.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
	}
	if (!res.ok) {
		const msg =
			parsed && typeof parsed === 'object' && parsed !== null && 'error_msg' in parsed
				? String((parsed as { error_msg: unknown }).error_msg)
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

export function listLLMAccounts(): Promise<ListLLMAccountsResponse> {
	return req<ListLLMAccountsResponse>('/api/v1/llm/accounts');
}

export function createLLMAccount(input: CreateLLMAccountInput): Promise<LLMAccount> {
	return req<LLMAccount>('/api/v1/llm/accounts', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}

export function importLLMAccountsPreview(): Promise<ImportLLMAccountsPreviewResponse> {
	return req<ImportLLMAccountsPreviewResponse>('/api/v1/llm/accounts/import-models-toml', {
		method: 'POST'
	});
}

export function applyLLMAccountsImport(): Promise<ApplyLLMAccountsImportResponse> {
	return req<ApplyLLMAccountsImportResponse>('/api/v1/llm/accounts/import-models-toml/apply', {
		method: 'POST'
	});
}
