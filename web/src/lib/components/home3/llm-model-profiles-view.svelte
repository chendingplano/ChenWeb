<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import {
		listModelProfiles,
		createModelProfile,
		updateModelProfile,
		type CreateModelProfileInput,
		type ModelProfile
	} from './llm-model-profiles-client';
	import { listLLMAccounts, type LLMAccount } from './llm-accounts-client';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let profiles = $state<ModelProfile[]>([]);
	let accounts = $state<LLMAccount[]>([]);
	let loading = $state(false);
	let submitting = $state(false);
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let showCreate = $state(false);
	let editingID = $state<string | null>(null);

	let draft = $state<CreateModelProfileInput>({
		account_id: '',
		profile_name: '',
		model_name: '',
		thinking_type: '',
		timeout_sec: 120,
		max_inflight: 16,
		max_requests_per_minute: 500,
		max_tokens_per_minute: 200000,
		token_reserve_per_call: 256,
		is_active: true
	});

	let editDraft = $state<CreateModelProfileInput>({
		account_id: '',
		profile_name: '',
		model_name: '',
		thinking_type: '',
		timeout_sec: 0,
		max_inflight: 0,
		max_requests_per_minute: 0,
		max_tokens_per_minute: 0,
		token_reserve_per_call: 0,
		is_active: true
	});

	onMount(() => {
		loadAll();
	});

	async function loadAll() {
		loading = true;
		error = null;
		try {
			const [profilesRes, accountsRes] = await Promise.all([listModelProfiles(), listLLMAccounts()]);
			profiles = profilesRes.profiles;
			accounts = accountsRes.accounts;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			loading = false;
		}
	}

	async function submitCreate() {
		error = null;
		info = null;
		if (!draft.profile_name.trim()) {
			error = m.llm_model_profiles_profile_name_is_required();
			return;
		}
		if (!draft.account_id) {
			error = m.llm_model_profiles_account_is_required();
			return;
		}
		submitting = true;
		try {
			await createModelProfile({ ...draft, profile_name: draft.profile_name.trim() });
			draft.profile_name = '';
			draft.model_name = '';
			showCreate = false;
			info = m.llm_model_profiles_model_profile_created();
			await loadAll();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	function startEdit(p: ModelProfile) {
		editingID = p.id;
		editDraft = {
			account_id: p.account_id,
			profile_name: p.profile_name,
			model_name: p.model_name,
			thinking_type: p.thinking_type,
			timeout_sec: p.timeout_sec,
			max_inflight: p.max_inflight,
			max_requests_per_minute: p.max_requests_per_minute,
			max_tokens_per_minute: p.max_tokens_per_minute,
			token_reserve_per_call: p.token_reserve_per_call,
			is_active: p.is_active
		};
		error = null;
		info = null;
	}

	async function saveEdit(id: string) {
		submitting = true;
		error = null;
		info = null;
		try {
			await updateModelProfile(id, editDraft);
			editingID = null;
			info = m.llm_model_profiles_profile_updated();
			await loadAll();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
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
	style:--input-bg={inputBg}
	style:--panel-bg={panelBg}
>
	<header class="toolbar">
		<div>
			<h2>{m.llm_model_profiles_model_profiles()}</h2>
			<p class="muted">{m.llm_model_profiles_named_model_configurations_linked_to()}</p>
		</div>
		<div class="toolbar-actions">
			<button class="ghost" onclick={loadAll} disabled={loading}>
				{loading ? m.llm_model_profiles_refreshing() : m.llm_model_profiles_refresh()}
			</button>
			<button class="primary" onclick={() => (showCreate = !showCreate)}>
				{showCreate ? m.llm_model_profiles_cancel() : m.llm_model_profiles_new_profile()}
			</button>
		</div>
	</header>

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">{m.llm_model_profiles_profiles()}</div>
			<div class="summary-value">{profiles.length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">{m.llm_model_profiles_active()}</div>
			<div class="summary-value">{profiles.filter((p) => p.is_active).length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">{m.llm_model_profiles_accounts()}</div>
			<div class="summary-value">{new Set(profiles.map((p) => p.account_id)).size}</div>
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
					<span>{m.llm_model_profiles_profile_name()}</span>
					<input bind:value={draft.profile_name} required placeholder={m.llm_model_profiles_deepseek_v4_pro()} />
				</label>
				<label>
					<span>{m.llm_model_profiles_model_name()}</span>
					<input bind:value={draft.model_name} placeholder={m.llm_model_profiles_deepseek_chat()} />
				</label>
			</div>
			<div class="row two">
				<label>
					<span>{m.llm_model_profiles_account()}</span>
					<select bind:value={draft.account_id}>
						<option value="">{m.llm_model_profiles_select_account()}</option>
						{#each accounts as acct (acct.id)}
							<option value={acct.id}>{acct.account_name} ({acct.provider})</option>
						{/each}
					</select>
				</label>
				<label>
					<span>{m.llm_model_profiles_thinking_type()}</span>
					<input bind:value={draft.thinking_type} placeholder={m.llm_model_profiles_disabled()} />
				</label>
			</div>
			<div class="row three">
				<label>
					<span>{m.llm_model_profiles_timeout_sec()}</span>
					<input type="number" bind:value={draft.timeout_sec} min="0" />
				</label>
				<label>
					<span>{m.llm_model_profiles_max_inflight()}</span>
					<input type="number" bind:value={draft.max_inflight} min="0" />
				</label>
				<label>
					<span>{m.llm_model_profiles_token_reserve()}</span>
					<input type="number" bind:value={draft.token_reserve_per_call} min="0" />
				</label>
			</div>
			<div class="row two">
				<label>
					<span>{m.llm_model_profiles_max_req_min()}</span>
					<input type="number" bind:value={draft.max_requests_per_minute} min="0" />
				</label>
				<label>
					<span>{m.llm_model_profiles_max_tokens_min()}</span>
					<input type="number" bind:value={draft.max_tokens_per_minute} min="0" />
				</label>
			</div>
			<label class="toggle-row">
				<span>{m.llm_model_profiles_active()}</span>
				<input type="checkbox" bind:checked={draft.is_active} />
			</label>
			<div class="form-foot">
				<button class="primary" type="submit" disabled={submitting || !draft.profile_name.trim() || !draft.account_id}>
					{submitting ? m.llm_model_profiles_creating() : m.llm_model_profiles_create_profile()}
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
				<h3>{m.llm_model_profiles_registered_profiles()}</h3>
				<p class="muted">{m.llm_model_profiles_each_profile_maps_a_name()}</p>
			</div>
		</div>

		{#if loading}
			<div class="empty">{m.llm_model_profiles_loading_profiles()}</div>
		{:else if profiles.length === 0}
			<div class="empty">{m.llm_model_profiles_no_model_profiles_yet_create()}</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>{m.llm_model_profiles_profile()}</th>
							<th>{m.llm_model_profiles_account()}</th>
							<th>{m.llm_model_profiles_model()}</th>
							<th>{m.llm_model_profiles_timeout()}</th>
							<th>{m.llm_model_profiles_inflight()}</th>
							<th>{m.llm_model_profiles_active()}</th>
							<th>{m.llm_model_profiles_updated()}</th>
							<th>{m.llm_model_profiles_action()}</th>
						</tr>
					</thead>
					<tbody>
						{#each profiles as profile (profile.id)}
							<tr>
								<td>
									<div class="cell-primary">{profile.profile_name}</div>
									{#if profile.thinking_type}
										<div class="cell-secondary">{m.llm_model_profiles_thinking({ thinking_type: profile.thinking_type })}</div>
									{/if}
								</td>
								<td>{profile.account_name}</td>
								<td>{profile.model_name || '—'}</td>
								<td>{profile.timeout_sec || '—'}</td>
								<td>{profile.max_inflight || '—'}</td>
								<td>{profile.is_active ? m.llm_model_profiles_yes() : m.llm_model_profiles_no()}</td>
								<td>{fmtDate(profile.updated_at)}</td>
								<td>
									{#if editingID === profile.id}
										<button class="ghost compact-btn" onclick={() => (editingID = null)} disabled={submitting}>{m.llm_model_profiles_cancel()}</button>
									{:else}
										<button class="ghost compact-btn" onclick={() => startEdit(profile)}>{m.llm_model_profiles_edit()}</button>
									{/if}
								</td>
							</tr>
							{#if editingID === profile.id}
								<tr>
									<td colspan="8" class="edit-cell">
										<form
											class="edit-form"
											onsubmit={(e) => {
												e.preventDefault();
												saveEdit(profile.id);
											}}
										>
											<div class="row two">
												<label>
													<span>{m.llm_model_profiles_profile_name()}</span>
													<input bind:value={editDraft.profile_name} required />
												</label>
												<label>
													<span>{m.llm_model_profiles_model_name()}</span>
													<input bind:value={editDraft.model_name} />
												</label>
											</div>
											<div class="row two">
												<label>
													<span>{m.llm_model_profiles_account()}</span>
													<select bind:value={editDraft.account_id}>
														{#each accounts as acct (acct.id)}
															<option value={acct.id}>{acct.account_name} ({acct.provider})</option>
														{/each}
													</select>
												</label>
												<label>
													<span>{m.llm_model_profiles_thinking_type()}</span>
													<input bind:value={editDraft.thinking_type} />
												</label>
											</div>
											<div class="row three">
												<label>
													<span>{m.llm_model_profiles_timeout_sec()}</span>
													<input type="number" bind:value={editDraft.timeout_sec} min="0" />
												</label>
												<label>
													<span>{m.llm_model_profiles_max_inflight()}</span>
													<input type="number" bind:value={editDraft.max_inflight} min="0" />
												</label>
												<label>
													<span>{m.llm_model_profiles_token_reserve()}</span>
													<input type="number" bind:value={editDraft.token_reserve_per_call} min="0" />
												</label>
											</div>
											<div class="row two">
												<label>
													<span>{m.llm_model_profiles_max_req_min()}</span>
													<input type="number" bind:value={editDraft.max_requests_per_minute} min="0" />
												</label>
												<label>
													<span>{m.llm_model_profiles_max_tokens_min()}</span>
													<input type="number" bind:value={editDraft.max_tokens_per_minute} min="0" />
												</label>
											</div>
											<label class="toggle-row">
												<span>{m.llm_model_profiles_active()}</span>
												<input type="checkbox" bind:checked={editDraft.is_active} />
											</label>
											<div class="form-foot">
												<button class="primary" type="submit" disabled={submitting}>
													{submitting ? m.llm_model_profiles_saving() : m.llm_model_profiles_save_profile()}
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
	.toolbar, .panel-head, .form-foot, .toolbar-actions, .toggle-row {
		display: flex;
	}
	.toolbar, .panel-head {
		justify-content: space-between;
		align-items: flex-end;
		gap: 12px;
	}
	.toolbar-actions {
		gap: 10px;
		flex-wrap: wrap;
	}
	.summary-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: 10px;
	}
	h2, h3 { margin: 0; color: var(--heading); }
	h2 { font-size: 20px; }
	h3 { font-size: 16px; }
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
	.compact-btn { padding: 6px 10px; font-size: 12px; }
	.primary:disabled, .ghost:disabled { opacity: 0.5; cursor: not-allowed; }
	.summary-card, .panel, .create-form {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
	}
	.summary-card { padding: 14px 16px; }
	.summary-label { font-size: 12px; color: var(--sub); text-transform: uppercase; letter-spacing: 0.04em; }
	.summary-value { margin-top: 6px; font-size: 22px; font-weight: 600; color: var(--heading); }
	.create-form, .panel { padding: 16px; }
	.create-form { display: flex; flex-direction: column; gap: 10px; }
	.edit-cell { padding: 0; }
	.edit-form {
		padding: 16px;
		background: var(--panel-bg);
		border-top: 1px solid var(--border);
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.row { display: grid; gap: 10px; }
	.row.two { grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); }
	.row.three { grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); }
	label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--sub); }
	input, select {
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
	.form-foot { justify-content: flex-end; }
	.error, .info { padding: 10px 12px; border-radius: 8px; font-size: 13px; }
	.error { background: rgba(248, 113, 113, 0.12); color: #f87171; }
	.info { background: rgba(15, 118, 110, 0.16); color: #5eead4; }
	.table-wrap { overflow-x: auto; margin-top: 12px; }
	table { width: 100%; border-collapse: collapse; }
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
	.cell-primary { font-weight: 600; }
	.cell-secondary { font-size: 12px; color: var(--sub); margin-top: 2px; }
	.empty { color: var(--sub); font-style: italic; padding: 24px 8px 8px; }
</style>
