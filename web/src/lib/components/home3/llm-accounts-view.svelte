<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';

	import {
		applyLLMAccountsImport,
		addLLMDeposit,
		createLLMAccount,
		importLLMAccountsPreview,
		listDepositAPIKeys,
		listLLMManualRecords,
		setLLMTotalSpending,
		listLLMAccounts,
		updateLLMAccount,
		type ApplyLLMAccountsImportResponse,
		type CreateLLMAccountInput,
		type ImportLLMAccountsPreviewResponse,
		type LLMAccount
	} from './llm-accounts-client';
	import { addModel } from './llm-models-client';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let accounts = $state<LLMAccount[]>([]);
	let depositAPIKeys = $state<string[]>([]);
	let manualRecords = $state<any[]>([]);
	let manualMode = $state<'deposit' | 'set-total-spending'>('deposit');
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
	let showDeposit = $state(false);
	let deposit = $state({ api_key_name: '', currency_code: 'CNY', deposit_amount: 0, balance_amount: 0, captured_at: '', note: '' });

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

	type AddModelDraft = {
		profile_name: string;
		model_name: string;
		model_type: string;
		provider: string;
		base_url: string;
		api_key: string;
		account_name: string;
		host: string;
		thinking_type: string;
		timeout_sec: number;
		max_inflight: number;
		max_requests_per_minute: number;
		max_tokens_per_minute: number;
		token_reserve_per_call: number;
	};
	let showAddModel = $state(false);
	let addingModel = $state(false);
	let addModelDraft = $state<AddModelDraft>({
		profile_name: '',
		model_name: '',
		model_type: 'llm',
		provider: 'deepseek',
		base_url: 'https://api.deepseek.com',
		api_key: '',
		account_name: '',
		host: 'cloud',
		thinking_type: '',
		timeout_sec: 120,
		max_inflight: 16,
		max_requests_per_minute: 500,
		max_tokens_per_minute: 200000,
		token_reserve_per_call: 256
	});

	onMount(() => {
		loadAccounts();
		loadDepositAPIKeys();
		loadManualRecords();
	});

	async function submitDeposit() {
		submitting = true; error = null;
		try { const payload = { ...deposit, balance_amount: manualMode === 'deposit' ? deposit.balance_amount : 0, captured_at: deposit.captured_at ? new Date(deposit.captured_at).toISOString() : '' }; if (manualMode === 'deposit') await addLLMDeposit(payload); else await setLLMTotalSpending(payload); info = manualMode === 'deposit' ? m.llm_accounts_deposit_recorded() : m.llm_accounts_total_spending_recorded(); await loadManualRecords(); showDeposit = false; }
		catch (err) { error = String((err as Error).message ?? err); }
		finally { submitting = false; }
	}

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

	async function loadDepositAPIKeys() {
		try {
			depositAPIKeys = (await listDepositAPIKeys()).api_keys;
		} catch (err) {
			error = String((err as Error).message ?? err);
		}
	}
	async function loadManualRecords() { try { manualRecords = (await listLLMManualRecords()).records; } catch (err) { error = String((err as Error).message ?? err); } }

	async function submitCreate() {
		error = null;
		info = null;
		const accountName = draft.account_name.trim();
		if (!accountName) {
			error = m.llm_accounts_account_name_is_required();
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
			info = m.llm_accounts_llm_account_created();
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
			info = m.llm_accounts_loaded_account_candidates_and_model({ accountsCount: preview.accounts.length, profilesCount: preview.profiles.length });
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
			info = m.llm_accounts_llm_account_updated();
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
			info = m.llm_accounts_imported_accounts_and_profiles_from({ accounts_imported: lastImportResult.accounts_imported, profiles_imported: lastImportResult.profiles_imported });
			await loadAccounts();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			applyingImport = false;
		}
	}

	async function submitAddModel() {
		error = null;
		info = null;
		if (!addModelDraft.profile_name.trim()) {
			error = m.llm_accounts_profile_name_is_required();
			return;
		}
		addingModel = true;
		try {
			const profileName = addModelDraft.profile_name.trim();
			await addModel({
				profile_name: profileName,
				model_name: addModelDraft.model_name.trim(),
				model_type: addModelDraft.model_type.trim(),
				provider: addModelDraft.provider.trim(),
				base_url: addModelDraft.base_url.trim(),
				api_key: addModelDraft.api_key.trim(),
				account_name: addModelDraft.account_name.trim(),
				host: addModelDraft.host.trim(),
				thinking_type: addModelDraft.thinking_type.trim(),
				timeout_sec: addModelDraft.timeout_sec,
				max_inflight: addModelDraft.max_inflight,
				max_requests_per_minute: addModelDraft.max_requests_per_minute,
				max_tokens_per_minute: addModelDraft.max_tokens_per_minute,
				token_reserve_per_call: addModelDraft.token_reserve_per_call
			});
			addModelDraft.profile_name = '';
			addModelDraft.model_name = '';
			addModelDraft.model_type = 'llm';
			addModelDraft.api_key = '';
			showAddModel = false;
			info = m.llm_accounts_model_added_to_models_toml({ profileName });
			await loadAccounts();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			addingModel = false;
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
	const selectBorder = $derived(darkMode ? '#4B5872' : '#9CA3AF');
	const selectColorScheme = $derived(darkMode ? 'dark' : 'light');
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
	style:--select-border={selectBorder}
	style:--select-color-scheme={selectColorScheme}
>
	<header class="toolbar">
		<div>
			<h2>{m.llm_accounts_llm_accounts()}</h2>
			<p class="muted">
				{m.llm_accounts_provider_agnostic_account_registry_for()}
			</p>
		</div>
		<div class="toolbar-actions">
			<button class="ghost" onclick={loadAccounts} disabled={loading}>
				{loading ? m.llm_accounts_refreshing() : m.llm_accounts_refresh()}
			</button>
			<button class="ghost" onclick={loadPreview} disabled={importing}>
				{importing ? m.llm_accounts_inspecting() : preview ? m.llm_accounts_hide_preview() : m.llm_accounts_preview_models_toml()}
			</button>
			<button
				class="alt-btn"
				disabled={showAddModel}
				onclick={() => {
					showAddModel = true;
					showCreate = false;
				}}
			>
				{m.llm_accounts_add_a_model()}
			</button>
			<button
				class="primary"
				disabled={showCreate}
				onclick={() => {
					showCreate = true;
					showAddModel = false;
				}}
			>
				{m.llm_accounts_new_account()}
			</button>
			<button class="alt-btn" onclick={() => { manualMode = 'deposit'; showDeposit = !showDeposit; showCreate = false; showAddModel = false; }}> {showDeposit && manualMode === 'deposit' ? m.llm_accounts_cancel() : m.llm_accounts_add_deposit()} </button>
			<button class="ghost" onclick={() => { manualMode = 'set-total-spending'; showDeposit = true; showCreate = false; showAddModel = false; }}>{m.llm_accounts_set_total_spend()}</button>
		</div>
	</header>

	{#if showDeposit}
		<form class="create-form" onsubmit={(e) => { e.preventDefault(); void submitDeposit(); }}>
			<h3>{manualMode === 'deposit' ? m.llm_accounts_add_deposit() : m.llm_accounts_set_total_spend()}</h3>
			<div class="row two"><label><span>{m.llm_accounts_api_key()}</span><select bind:value={deposit.api_key_name} required><option value="">{m.llm_accounts_select_an_api_key()}</option>{#each depositAPIKeys as apiKey (apiKey)}<option value={apiKey}>{apiKey}</option>{/each}</select></label><label><span>{m.llm_accounts_currency()}</span><select bind:value={deposit.currency_code}><option value="CNY">{m.llm_accounts_cny()}</option><option value="USD">{m.llm_accounts_usd()}</option></select></label></div>
			<div class="row two"><label><span>{manualMode === 'deposit' ? m.llm_accounts_deposit_amount() : m.llm_accounts_total_spending()}</span><input type="number" min="1" step={manualMode === 'deposit' ? '1' : '0.01'} bind:value={deposit.deposit_amount} required /></label>{#if manualMode === 'deposit'}<label><span>{m.llm_accounts_balance_after_deposit()}</span><input type="number" step="0.01" bind:value={deposit.balance_amount} required /></label>{/if}</div>
			<div class="row two"><label><span>{m.llm_accounts_timestamp()}</span><input type="datetime-local" bind:value={deposit.captured_at} /><small class="field-help">{m.llm_accounts_leave_blank_to_use_the()}</small></label><label><span>{m.llm_accounts_note()}</span><input bind:value={deposit.note} placeholder={m.llm_accounts_optional_reference()} /></label></div>
			<div class="row form-foot"><button class="primary" disabled={submitting}>{submitting ? m.llm_accounts_saving() : manualMode === 'deposit' ? m.llm_accounts_save_deposit() : m.llm_accounts_save_total_spend()}</button></div>
		</form>
	{/if}
	<div class="panel"><div class="panel-head"><h3>{m.llm_accounts_manual_records()}</h3></div>{#if manualRecords.length === 0}<div class="empty compact">{m.llm_accounts_no_deposits_or_total_spending()}</div>{:else}<div class="table-wrap"><table><thead><tr><th>{m.llm_accounts_timestamp()}</th><th>{m.llm_accounts_api_key()}</th><th>{m.llm_accounts_type()}</th><th>{m.llm_accounts_currency()}</th><th>{m.llm_accounts_amount()}</th><th>{m.llm_accounts_note()}</th></tr></thead><tbody>{#each manualRecords as record}<tr><td>{fmtDate(record.captured_at)}</td><td>{record.api_key_name}</td><td>{record.entry_kind === 'deposit' ? m.llm_accounts_deposit() : m.llm_accounts_total_spending()}</td><td>{record.currency_code}</td><td>{record.amount}</td><td>{record.note}</td></tr>{/each}</tbody></table></div>{/if}</div>

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">{m.llm_accounts_accounts()}</div>
			<div class="summary-value">{accounts.length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">{m.llm_accounts_reconciliation_enabled()}</div>
			<div class="summary-value">
				{accounts.filter((row) => row.is_reconciliation_enabled).length}
			</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">{m.llm_accounts_providers()}</div>
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
					<span>{m.llm_accounts_account_name()}</span>
					<input bind:value={draft.account_name} required placeholder={m.llm_accounts_deepseek_prod()} />
				</label>
				<label>
					<span>{m.llm_accounts_provider()}</span>
					<input bind:value={draft.provider} required placeholder={m.llm_accounts_deepseek()} />
				</label>
			</div>
			<div class="row two">
				<label>
					<span>{m.llm_accounts_base_url()}</span>
					<input bind:value={draft.base_url} placeholder={m.llm_accounts_https_api_deepseek_com()} />
				</label>
				<label>
					<span>{m.llm_accounts_default_model()}</span>
					<input bind:value={draft.default_model_name} placeholder={m.llm_accounts_deepseek_chat()} />
				</label>
			</div>
			<div class="row two">
				<label>
					<span>{m.llm_accounts_status()}</span>
					<input bind:value={draft.status} placeholder={m.llm_accounts_active()} />
				</label>
				<label>
					<span>{m.llm_accounts_reconciliation_kind()}</span>
					<input bind:value={draft.reconciliation_kind} placeholder={m.llm_accounts_provider_balance()} />
				</label>
			</div>
			<label>
				<span>{m.llm_accounts_api_key()}</span>
				<input bind:value={draft.api_key} type="password" placeholder={m.llm_accounts_stored_server_side()} />
			</label>
			<label class="toggle-row">
				<span>{m.llm_accounts_enable_provider_side_reconciliation()}</span>
				<input type="checkbox" bind:checked={draft.is_reconciliation_enabled} />
			</label>
			<div class="form-foot">
				<button class="ghost" type="button" onclick={() => (showCreate = false)}>{m.llm_accounts_cancel()}</button>
				<button class="primary" type="submit" disabled={submitting || !draft.account_name.trim()}>
					{submitting ? m.llm_accounts_creating() : m.llm_accounts_create_account()}
				</button>
			</div>
		</form>
	{/if}

	{#if showAddModel}
		<form
			class="create-form"
			onsubmit={(e) => {
				e.preventDefault();
				submitAddModel();
			}}
		>
			<div class="add-model-notice">
				{m.llm_accounts_adds_the_model_to()} <strong>{m.llm_accounts_models_toml()}</strong> {m.llm_accounts_and_registers_it_in_the()}
			</div>
			<div class="row two">
				<label>
					<span>{m.llm_accounts_profile_name()} <span class="req">*</span></span>
					<input bind:value={addModelDraft.profile_name} required placeholder={m.llm_accounts_my_new_model()} />
				</label>
				<label>
					<span>{m.llm_accounts_model_name()}</span>
					<input bind:value={addModelDraft.model_name} placeholder={m.llm_accounts_deepseek_chat()} />
				</label>
			</div>
			<label>
				<span>{m.llm_accounts_model_type()}</span>
				<input bind:value={addModelDraft.model_type} placeholder={m.llm_accounts_llm()} />
			</label>
			<div class="row two">
				<label>
					<span>{m.llm_accounts_provider()}</span>
					<input bind:value={addModelDraft.provider} placeholder={m.llm_accounts_deepseek()} />
				</label>
				<label>
					<span>{m.llm_accounts_base_url()}</span>
					<input bind:value={addModelDraft.base_url} placeholder={m.llm_accounts_https_api_deepseek_com()} />
				</label>
			</div>
			<div class="row two">
				<label>
					<span>{m.llm_accounts_account_name_db_label()}</span>
					<input bind:value={addModelDraft.account_name} placeholder={m.llm_accounts_auto_generated_if_blank()} />
				</label>
				<label>
					<span>{m.llm_accounts_host()}</span>
					<input bind:value={addModelDraft.host} placeholder={m.llm_accounts_cloud()} />
				</label>
			</div>
			<label>
				<span>{m.llm_accounts_api_key()}</span>
				<input type="password" bind:value={addModelDraft.api_key} placeholder={m.llm_accounts_sk()} />
			</label>
			<div class="row two">
				<label>
					<span>{m.llm_accounts_thinking_type()}</span>
					<input bind:value={addModelDraft.thinking_type} placeholder={m.llm_accounts_disabled()} />
				</label>
				<label>
					<span>{m.llm_accounts_timeout_sec()}</span>
					<input type="number" bind:value={addModelDraft.timeout_sec} min="0" />
				</label>
			</div>
			<div class="row three">
				<label>
					<span>{m.llm_accounts_max_inflight()}</span>
					<input type="number" bind:value={addModelDraft.max_inflight} min="0" />
				</label>
				<label>
					<span>{m.llm_accounts_max_req_min()}</span>
					<input type="number" bind:value={addModelDraft.max_requests_per_minute} min="0" />
				</label>
				<label>
					<span>{m.llm_accounts_max_tokens_min()}</span>
					<input type="number" bind:value={addModelDraft.max_tokens_per_minute} min="0" />
				</label>
			</div>
			<label>
				<span>{m.llm_accounts_token_reserve_call()}</span>
				<input
					type="number"
					bind:value={addModelDraft.token_reserve_per_call}
					min="0"
					style="max-width:200px;"
				/>
			</label>
			<div class="form-foot">
				<button class="ghost" type="button" onclick={() => (showAddModel = false)}>{m.llm_accounts_cancel()}</button>
				<button
					class="alt-btn"
					type="submit"
					disabled={addingModel || !addModelDraft.profile_name.trim()}
				>
					{addingModel ? m.llm_accounts_adding() : m.llm_accounts_add_model()}
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
				<h3>{m.llm_accounts_registered_accounts()}</h3>
				<p class="muted">
					{m.llm_accounts_daily_reports_roll_up_by()}
				</p>
			</div>
		</div>

		{#if loading}
			<div class="empty">{m.llm_accounts_loading_accounts()}</div>
		{:else if accounts.length === 0}
			<div class="empty">
				{m.llm_accounts_no_llm_accounts_yet_create()}
			</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>{m.llm_accounts_account()}</th>
							<th>{m.llm_accounts_provider()}</th>
							<th>{m.llm_accounts_default_model()}</th>
							<th>{m.llm_accounts_profiles()}</th>
							<th>{m.llm_accounts_reconciliation()}</th>
							<th>{m.llm_accounts_updated()}</th>
							<th>{m.llm_accounts_action()}</th>
						</tr>
					</thead>
					<tbody>
						{#each accounts as account (account.id)}
							<tr>
								<td>
									<div class="cell-primary">{account.account_name}</div>
									<div class="cell-secondary">{account.base_url || m.llm_accounts_no_base_url()}</div>
								</td>
								<td>{account.provider}</td>
								<td>{account.default_model_name || m.llm_accounts_not_set()}</td>
								<td>{account.profile_count}</td>
								<td>{account.is_reconciliation_enabled ? m.llm_accounts_enabled() : m.llm_accounts_disabled_2()}</td>
								<td>{fmtDate(account.updated_at)}</td>
								<td>
									{#if editingAccountID === account.id}
										<div class="row-actions">
											<button class="ghost compact-btn" onclick={cancelEdit} disabled={submitting}
												>{m.llm_accounts_cancel()}</button
											>
										</div>
									{:else}
										<button class="ghost compact-btn" onclick={() => startEdit(account)}
											>{m.llm_accounts_edit()}</button
										>
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
													<span>{m.llm_accounts_account_name()}</span>
													<input bind:value={editDraft.account_name} required />
												</label>
												<label>
													<span>{m.llm_accounts_provider()}</span>
													<input bind:value={editDraft.provider} required />
												</label>
											</div>
											<div class="row two">
												<label>
													<span>{m.llm_accounts_base_url()}</span>
													<input bind:value={editDraft.base_url} />
												</label>
												<label>
													<span>{m.llm_accounts_default_model()}</span>
													<input bind:value={editDraft.default_model_name} />
												</label>
											</div>
											<div class="row two">
												<label>
													<span>{m.llm_accounts_status()}</span>
													<input bind:value={editDraft.status} />
												</label>
												<label>
													<span>{m.llm_accounts_reconciliation_kind()}</span>
													<input bind:value={editDraft.reconciliation_kind} />
												</label>
											</div>
											<label>
												<span>{m.llm_accounts_api_key()}</span>
												<input
													bind:value={editDraft.api_key}
													type="password"
													placeholder={m.llm_accounts_leave_blank_to_keep_current()}
												/>
											</label>
											<label class="toggle-row">
												<span>{m.llm_accounts_enable_provider_side_reconciliation()}</span>
												<input type="checkbox" bind:checked={editDraft.is_reconciliation_enabled} />
											</label>
											<div class="form-foot">
												<button
													class="primary"
													type="submit"
													disabled={submitting || !editDraft.account_name.trim()}
												>
													{submitting ? m.llm_accounts_saving() : m.llm_accounts_save_account()}
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
					<h3>{m.llm_accounts_bootstrap_preview()}</h3>
					<p class="muted">{preview.path}</p>
				</div>
				<button class="ghost compact-btn" onclick={() => (preview = null)}>{m.llm_accounts_close()}</button>
			</div>
			<div class="preview-grid">
				<div class="preview-card">
					<div class="summary-label">{m.llm_accounts_account_candidates()}</div>
					<div class="summary-value">{preview.accounts.length}</div>
				</div>
				<div class="preview-card">
					<div class="summary-label">{m.llm_accounts_model_profiles()}</div>
					<div class="summary-value">{preview.profiles.length}</div>
				</div>
			</div>
			<div class="preview-actions">
				<button class="primary" onclick={applyImport} disabled={applyingImport}>
					{applyingImport ? m.llm_accounts_importing() : m.llm_accounts_import_into_accounts()}
				</button>
				{#if lastImportResult}
					<div class="muted inline-status">
						{m.llm_accounts_last_import_accounts_profiles({ accounts_imported: lastImportResult.accounts_imported, profiles_imported: lastImportResult.profiles_imported })}
					</div>
				{/if}
			</div>
			<div class="preview-columns">
				<div class="preview-list">
					<h4>{m.llm_accounts_accounts()}</h4>
					{#if preview.accounts.length === 0}
						<div class="empty compact">{m.llm_accounts_no_accounts_discovered()}</div>
					{:else}
						<ul>
							{#each preview.accounts as item, index (index)}
								<li>{JSON.stringify(item)}</li>
							{/each}
						</ul>
					{/if}
				</div>
				<div class="preview-list">
					<h4>{m.llm_accounts_profiles()}</h4>
					{#if preview.profiles.length === 0}
						<div class="empty compact">{m.llm_accounts_no_profiles_discovered()}</div>
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
	.toolbar,
	.panel-head,
	.form-foot,
	.toolbar-actions,
	.toggle-row,
	.preview-columns,
	.summary-grid,
	.preview-grid,
	.preview-actions {
		display: flex;
	}
	.toolbar,
	.panel-head {
		justify-content: space-between;
		align-items: flex-end;
		gap: 12px;
	}
	.toolbar-actions,
	.summary-grid,
	.preview-grid {
		gap: 10px;
		flex-wrap: wrap;
	}
	.summary-grid,
	.preview-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
	}
	h2,
	h3,
	h4 {
		margin: 0;
		color: var(--heading);
	}
	h2 {
		font-size: 20px;
	}
	h3 {
		font-size: 16px;
	}
	h4 {
		font-size: 14px;
	}
	.muted {
		color: var(--sub);
		font-size: 12px;
		margin: 4px 0 0;
	}
	.primary,
	.ghost,
	.alt-btn {
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
	.alt-btn {
		background: var(--alt-btn);
		color: white;
		border: none;
	}
	.compact-btn {
		padding: 6px 10px;
		font-size: 12px;
	}
	.primary:disabled,
	.ghost:disabled,
	.alt-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.row.three {
		grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
	}
	.add-model-notice {
		font-size: 12px;
		color: var(--sub);
		background: var(--panel-bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 10px 12px;
		line-height: 1.5;
	}
	.add-model-notice strong {
		color: var(--heading);
	}
	.req {
		color: #f87171;
	}
	.summary-card,
	.preview-card,
	.panel,
	.create-form {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
	}
	.summary-card,
	.preview-card {
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
	.create-form,
	.panel {
		padding: 16px;
	}
	.create-form {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.row-actions {
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
	.field-help {
		font-size: 11px;
		line-height: 1.35;
	}
	input,
	select {
		background: var(--input-bg);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		font-family: inherit;
	}
	select {
		border-color: var(--select-border);
		color-scheme: var(--select-color-scheme);
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
		gap: 8px;
	}
	.error,
	.info {
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
	th,
	td {
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
