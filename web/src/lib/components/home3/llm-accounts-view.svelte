<script lang="ts">
	import { onMount } from 'svelte';

	import {
		applyLLMAccountsImport,
		createLLMAccount,
		importLLMAccountsPreview,
		listLLMAccounts,
		updateLLMAccount,
		type ApplyLLMAccountsImportResponse,
		type CreateLLMAccountInput,
		type ImportLLMAccountsPreviewResponse,
		type LLMAccount
	} from './llm-accounts-client';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let accounts = $state<LLMAccount[]>([]);
	let loading = $state(false);
	let submitting = $state(false);
	let importing = $state(false);
	let applyingImport = $state(false);
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let showCreate = $state(false);
	let preview = $state<ImportLLMAccountsPreviewResponse | null>(null);
	let lastImportResult = $state<ApplyLLMAccountsImportResponse | null>(null);
	let editingAccountID = $state<string | null>(null);

	let draft = $state<CreateLLMAccountInput>({
		account_name: '',
		provider: 'deepseek',
		base_url: 'https://api.deepseek.com',
		api_key: '',
		status: 'active',
		reconciliation_kind: 'provider_balance',
		is_reconciliation_enabled: true,
		default_model_name: 'deepseek-chat'
	});
	let editDraft = $state<CreateLLMAccountInput>({
		account_name: '',
		provider: '',
		base_url: '',
		api_key: '',
		status: 'active',
		reconciliation_kind: 'provider_balance',
		is_reconciliation_enabled: false,
		default_model_name: ''
	});

	onMount(() => {
		loadAccounts();
	});

	async function loadAccounts() {
		loading = true;
		error = null;
		try {
			const response = await listLLMAccounts();
			accounts = response.accounts;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			loading = false;
		}
	}

	async function submitCreate() {
		error = null;
		info = null;
		const accountName = draft.account_name.trim();
		if (!accountName) {
			error = 'Account name is required';
			return;
		}
		submitting = true;
		try {
			await createLLMAccount({
				...draft,
				account_name: accountName,
				base_url: draft.base_url.trim(),
				api_key: draft.api_key.trim(),
				default_model_name: draft.default_model_name.trim()
			});
			draft = {
				account_name: '',
				provider: draft.provider,
				base_url: draft.base_url,
				api_key: '',
				status: draft.status,
				reconciliation_kind: draft.reconciliation_kind,
				is_reconciliation_enabled: draft.is_reconciliation_enabled,
				default_model_name: draft.default_model_name
			};
			showCreate = false;
			info = 'LLM account created.';
			await loadAccounts();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	async function loadPreview() {
		if (preview) {
			preview = null;
			lastImportResult = null;
			info = null;
			return;
		}
		importing = true;
		error = null;
		info = null;
		try {
			preview = await importLLMAccountsPreview();
			info = `Loaded ${preview.accounts.length} account candidates and ${preview.profiles.length} model profiles from .models.toml.`;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			importing = false;
		}
	}

	function startEdit(account: LLMAccount) {
		editingAccountID = account.id;
		editDraft = {
			account_name: account.account_name,
			provider: account.provider,
			base_url: account.base_url,
			api_key: '',
			status: account.status,
			reconciliation_kind: 'provider_balance',
			is_reconciliation_enabled: account.is_reconciliation_enabled,
			default_model_name: account.default_model_name
		};
		error = null;
		info = null;
	}

	function cancelEdit() {
		editingAccountID = null;
	}

	async function saveEdit(accountID: string) {
		submitting = true;
		error = null;
		info = null;
		try {
			await updateLLMAccount(accountID, {
				...editDraft,
				account_name: editDraft.account_name.trim(),
				provider: editDraft.provider.trim(),
				base_url: editDraft.base_url.trim(),
				api_key: editDraft.api_key.trim(),
				status: editDraft.status.trim(),
				reconciliation_kind: editDraft.reconciliation_kind.trim(),
				default_model_name: editDraft.default_model_name.trim()
			});
			editingAccountID = null;
			info = 'LLM account updated.';
			await loadAccounts();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	async function applyImport() {
		applyingImport = true;
		error = null;
		info = null;
		try {
			lastImportResult = await applyLLMAccountsImport();
			info = `Imported ${lastImportResult.accounts_imported} accounts and ${lastImportResult.profiles_imported} profiles from .models.toml.`;
			await loadAccounts();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			applyingImport = false;
		}
	}

	function fmtDate(raw: string): string {
		return new Date(raw).toLocaleString();
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#0F766E' : '#0F766E');
	const altBtn = $derived(darkMode ? '#818CF8' : '#6366F1');
	const inputBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const panelBg = $derived(darkMode ? '#151A29' : '#FDFDFD');
</script>

<div
	class="wrap"
	style:--page={pageBg}
	style:--card={card}
	style:--border={border}
	style:--heading={heading}
	style:--sub={sub}
	style:--btn={btn}
	style:--alt-btn={altBtn}
	style:--input-bg={inputBg}
	style:--panel-bg={panelBg}
>
	<header class="toolbar">
		<div>
			<h2>LLM Accounts</h2>
			<p class="muted">Provider-agnostic account registry for reconciliation and runtime usage capture.</p>
		</div>
		<div class="toolbar-actions">
			<button class="ghost" onclick={loadAccounts} disabled={loading}>
				{loading ? 'Refreshing…' : 'Refresh'}
			</button>
			<button class="ghost" onclick={loadPreview} disabled={importing}>
				{importing ? 'Inspecting…' : preview ? 'Hide Preview' : 'Preview .models.toml'}
			</button>
			<button class="primary" onclick={() => (showCreate = !showCreate)}>
				{showCreate ? 'Cancel' : '+ New Account'}
			</button>
		</div>
	</header>

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">Accounts</div>
			<div class="summary-value">{accounts.length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Reconciliation Enabled</div>
			<div class="summary-value">{accounts.filter((row) => row.is_reconciliation_enabled).length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Providers</div>
			<div class="summary-value">{new Set(accounts.map((row) => row.provider)).size}</div>
		</div>
	</div>

	{#if showCreate}
		<form
			class="create-form"
			onsubmit={(e) => {
				e.preventDefault();
				submitCreate();
			}}
		>
			<div class="row two">
				<label>
					<span>Account Name</span>
					<input bind:value={draft.account_name} required placeholder="DeepSeek Prod" />
				</label>
				<label>
					<span>Provider</span>
					<input bind:value={draft.provider} required placeholder="deepseek" />
				</label>
			</div>
			<div class="row two">
				<label>
					<span>Base URL</span>
					<input bind:value={draft.base_url} placeholder="https://api.deepseek.com" />
				</label>
				<label>
					<span>Default Model</span>
					<input bind:value={draft.default_model_name} placeholder="deepseek-chat" />
				</label>
			</div>
			<div class="row two">
				<label>
					<span>Status</span>
					<input bind:value={draft.status} placeholder="active" />
				</label>
				<label>
					<span>Reconciliation Kind</span>
					<input bind:value={draft.reconciliation_kind} placeholder="provider_balance" />
				</label>
			</div>
			<label>
				<span>API Key</span>
				<input bind:value={draft.api_key} type="password" placeholder="Stored server-side" />
			</label>
			<label class="toggle-row">
				<span>Enable provider-side reconciliation</span>
				<input type="checkbox" bind:checked={draft.is_reconciliation_enabled} />
			</label>
			<div class="form-foot">
				<button class="primary" type="submit" disabled={submitting || !draft.account_name.trim()}>
					{submitting ? 'Creating…' : 'Create account'}
				</button>
			</div>
		</form>
	{/if}

	{#if error}
		<div class="error" role="alert">{error}</div>
	{:else if info}
		<div class="info" role="status">{info}</div>
	{/if}

	<div class="panel">
		<div class="panel-head">
			<div>
				<h3>Registered Accounts</h3>
				<p class="muted">Daily reports roll up by account. You can now edit account settings from the table.</p>
			</div>
		</div>

		{#if loading}
			<div class="empty">Loading accounts…</div>
		{:else if accounts.length === 0}
			<div class="empty">No LLM accounts yet. Create one or preview `.models.toml` to prepare the migration.</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>Account</th>
							<th>Provider</th>
							<th>Default Model</th>
							<th>Profiles</th>
							<th>Reconciliation</th>
							<th>Updated</th>
							<th>Action</th>
						</tr>
					</thead>
					<tbody>
						{#each accounts as account (account.id)}
							<tr>
								<td>
									<div class="cell-primary">{account.account_name}</div>
									<div class="cell-secondary">{account.base_url || 'No base URL'}</div>
								</td>
								<td>{account.provider}</td>
								<td>{account.default_model_name || 'Not set'}</td>
								<td>{account.profile_count}</td>
								<td>{account.is_reconciliation_enabled ? 'Enabled' : 'Disabled'}</td>
								<td>{fmtDate(account.updated_at)}</td>
								<td>
									{#if editingAccountID === account.id}
										<div class="row-actions">
											<button class="ghost compact-btn" onclick={cancelEdit} disabled={submitting}>Cancel</button>
										</div>
									{:else}
										<button class="ghost compact-btn" onclick={() => startEdit(account)}>Edit</button>
									{/if}
								</td>
							</tr>
							{#if editingAccountID === account.id}
								<tr>
									<td colspan="7" class="edit-cell">
										<form
											class="edit-form"
											onsubmit={(e) => {
												e.preventDefault();
												saveEdit(account.id);
											}}
										>
											<div class="row two">
												<label>
													<span>Account Name</span>
													<input bind:value={editDraft.account_name} required />
												</label>
												<label>
													<span>Provider</span>
													<input bind:value={editDraft.provider} required />
												</label>
											</div>
											<div class="row two">
												<label>
													<span>Base URL</span>
													<input bind:value={editDraft.base_url} />
												</label>
												<label>
													<span>Default Model</span>
													<input bind:value={editDraft.default_model_name} />
												</label>
											</div>
											<div class="row two">
												<label>
													<span>Status</span>
													<input bind:value={editDraft.status} />
												</label>
												<label>
													<span>Reconciliation Kind</span>
													<input bind:value={editDraft.reconciliation_kind} />
												</label>
											</div>
											<label>
												<span>API Key</span>
												<input
													bind:value={editDraft.api_key}
													type="password"
													placeholder="Leave blank to keep current key"
												/>
											</label>
											<label class="toggle-row">
												<span>Enable provider-side reconciliation</span>
												<input type="checkbox" bind:checked={editDraft.is_reconciliation_enabled} />
											</label>
											<div class="form-foot">
												<button class="primary" type="submit" disabled={submitting || !editDraft.account_name.trim()}>
													{submitting ? 'Saving…' : 'Save account'}
												</button>
											</div>
										</form>
									</td>
								</tr>
							{/if}
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	{#if preview}
		<div class="panel">
			<div class="panel-head">
				<div>
					<h3>Bootstrap Preview</h3>
					<p class="muted">{preview.path}</p>
				</div>
				<button class="ghost compact-btn" onclick={() => (preview = null)}>Close</button>
			</div>
			<div class="preview-grid">
				<div class="preview-card">
					<div class="summary-label">Account Candidates</div>
					<div class="summary-value">{preview.accounts.length}</div>
				</div>
				<div class="preview-card">
					<div class="summary-label">Model Profiles</div>
					<div class="summary-value">{preview.profiles.length}</div>
				</div>
			</div>
			<div class="preview-actions">
				<button class="primary" onclick={applyImport} disabled={applyingImport}>
					{applyingImport ? 'Importing…' : 'Import Into Accounts'}
				</button>
				{#if lastImportResult}
					<div class="muted inline-status">
						Last import: {lastImportResult.accounts_imported} accounts, {lastImportResult.profiles_imported} profiles
					</div>
				{/if}
			</div>
			<div class="preview-columns">
				<div class="preview-list">
					<h4>Accounts</h4>
					{#if preview.accounts.length === 0}
						<div class="empty compact">No accounts discovered.</div>
					{:else}
						<ul>
							{#each preview.accounts as item, index (index)}
								<li>{JSON.stringify(item)}</li>
							{/each}
						</ul>
					{/if}
				</div>
				<div class="preview-list">
					<h4>Profiles</h4>
					{#if preview.profiles.length === 0}
						<div class="empty compact">No profiles discovered.</div>
					{:else}
						<ul>
							{#each preview.profiles as item, index (index)}
								<li>{JSON.stringify(item)}</li>
							{/each}
						</ul>
					{/if}
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.wrap {
		display: flex;
		flex-direction: column;
		gap: 16px;
		background: var(--page);
		min-height: 100%;
		padding: 16px 20px 32px;
	}
	.toolbar, .panel-head, .form-foot, .toolbar-actions, .toggle-row, .preview-columns, .summary-grid, .preview-grid, .preview-actions {
		display: flex;
	}
	.toolbar, .panel-head {
		justify-content: space-between;
		align-items: flex-end;
		gap: 12px;
	}
	.toolbar-actions, .summary-grid, .preview-grid {
		gap: 10px;
		flex-wrap: wrap;
	}
	.summary-grid, .preview-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
	}
	h2, h3, h4 { margin: 0; color: var(--heading); }
	h2 { font-size: 20px; }
	h3 { font-size: 16px; }
	h4 { font-size: 14px; }
	.muted { color: var(--sub); font-size: 12px; margin: 4px 0 0; }
	.primary, .ghost {
		border-radius: 8px;
		padding: 8px 14px;
		font-size: 13px;
		cursor: pointer;
	}
	.primary {
		background: var(--btn);
		color: white;
		border: none;
	}
	.ghost {
		background: transparent;
		color: var(--heading);
		border: 1px solid var(--border);
	}
	.compact-btn {
		padding: 6px 10px;
		font-size: 12px;
	}
	.primary:disabled, .ghost:disabled { opacity: 0.5; cursor: not-allowed; }
	.summary-card, .preview-card, .panel, .create-form {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
	}
	.summary-card, .preview-card {
		padding: 14px 16px;
	}
	.summary-label {
		font-size: 12px;
		color: var(--sub);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.summary-value {
		margin-top: 6px;
		font-size: 22px;
		font-weight: 600;
		color: var(--heading);
	}
	.create-form, .panel {
		padding: 16px;
	}
	.create-form {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.row-actions, .inline-toggle {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.edit-cell {
		padding: 0;
	}
	.edit-form {
		padding: 16px;
		background: var(--panel-bg);
		border-top: 1px solid var(--border);
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.row {
		display: grid;
		gap: 10px;
	}
	.row.two {
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 12px;
		color: var(--sub);
	}
	input {
		background: var(--input-bg);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		font-family: inherit;
	}
	.toggle-row {
		align-items: center;
		justify-content: space-between;
		background: var(--panel-bg);
		border: 1px solid var(--border);
		padding: 10px 12px;
		border-radius: 8px;
	}
	.form-foot {
		justify-content: flex-end;
	}
	.error, .info {
		padding: 10px 12px;
		border-radius: 8px;
		font-size: 13px;
	}
	.error {
		background: rgba(248, 113, 113, 0.12);
		color: #f87171;
	}
	.info {
		background: rgba(15, 118, 110, 0.16);
		color: #5eead4;
	}
	.table-wrap {
		overflow-x: auto;
		margin-top: 12px;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th, td {
		padding: 12px 10px;
		border-top: 1px solid var(--border);
		font-size: 13px;
		color: var(--heading);
		text-align: left;
		vertical-align: top;
	}
	th {
		color: var(--sub);
		font-size: 12px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		border-top: none;
		padding-top: 0;
	}
	.cell-primary {
		font-weight: 600;
	}
	.cell-secondary {
		font-size: 12px;
		color: var(--sub);
		margin-top: 2px;
	}
	.preview-columns {
		gap: 16px;
		align-items: flex-start;
		margin-top: 14px;
		flex-wrap: wrap;
	}
	.preview-actions {
		margin-top: 14px;
		gap: 12px;
		align-items: center;
		flex-wrap: wrap;
	}
	.inline-status {
		margin: 0;
	}
	.preview-list {
		flex: 1 1 320px;
		background: var(--panel-bg);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 12px;
	}
	.preview-list ul {
		margin: 10px 0 0;
		padding-left: 18px;
		color: var(--heading);
		font-size: 12px;
		word-break: break-word;
	}
	.preview-list li + li {
		margin-top: 6px;
	}
	.empty {
		color: var(--sub);
		font-style: italic;
		padding: 24px 8px 8px;
	}
	.empty.compact {
		padding: 12px 0 0;
	}
</style>
