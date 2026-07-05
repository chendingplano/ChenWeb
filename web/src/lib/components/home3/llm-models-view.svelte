<script lang="ts">
	import { onMount } from 'svelte';
	import {
		getModelsTOML,
		upsertModelTOML,
		deleteModelTOML,
		type LLMModelEntry
	} from './llm-models-client';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let models = $state<LLMModelEntry[]>([]);
	let tomlPath = $state('');
	let loading = $state(false);
	let submitting = $state(false);
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let showCreate = $state(false);
	let editingKey = $state<string | null>(null);
	let deletingKey = $state<string | null>(null);

	type ModelDraft = Omit<LLMModelEntry, 'key'> & { key: string };

	const emptyDraft = (): ModelDraft => ({
		key: '',
		host: 'cloud',
		model_name: '',
		base_url: '',
		timeout_sec: 120,
		thinking_type: '',
		max_inflight: 16,
		max_requests_per_minute: 500,
		max_tokens_per_minute: 200000,
		token_reserve_per_call: 256
	});

	let draft = $state<ModelDraft>(emptyDraft());
	let editDraft = $state<ModelDraft>(emptyDraft());

	onMount(() => {
		loadModels();
	});

	async function loadModels() {
		loading = true;
		error = null;
		try {
			const res = await getModelsTOML();
			models = res.models;
			tomlPath = res.path;
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			loading = false;
		}
	}

	async function submitCreate() {
		error = null;
		info = null;
		if (!draft.key.trim()) {
			error = 'Key is required';
			return;
		}
		submitting = true;
		try {
			const { key, ...entry } = draft;
			await upsertModelTOML(key.trim(), entry);
			draft = emptyDraft();
			showCreate = false;
			info = `Model "${key.trim()}" saved to .models.toml.`;
			await loadModels();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	function startEdit(m: LLMModelEntry) {
		editingKey = m.key;
		editDraft = { ...m };
		error = null;
		info = null;
	}

	async function saveEdit() {
		if (!editingKey) return;
		submitting = true;
		error = null;
		info = null;
		try {
			const { key, ...entry } = editDraft;
			await upsertModelTOML(key, entry);
			editingKey = null;
			info = `Model "${key}" updated.`;
			await loadModels();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	async function confirmDelete(key: string) {
		submitting = true;
		error = null;
		info = null;
		try {
			await deleteModelTOML(key);
			deletingKey = null;
			info = `Model "${key}" removed from .models.toml.`;
			await loadModels();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	const pageBg = $derived(darkMode ? '#0F1320' : '#F7F8FA');
	const card = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const border = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');
	const btn = $derived(darkMode ? '#0F766E' : '#0F766E');
	const danger = '#dc2626';
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
	style:--danger={danger}
	style:--input-bg={inputBg}
	style:--panel-bg={panelBg}
>
	<header class="toolbar">
		<div>
			<h2>LLM Models</h2>
			<p class="muted">{tomlPath ? `Editing ${tomlPath}` : 'Runtime model configuration file (.models.toml)'}</p>
		</div>
		<div class="toolbar-actions">
			<button class="ghost" onclick={loadModels} disabled={loading}>
				{loading ? 'Refreshing…' : 'Refresh'}
			</button>
			<button class="primary" onclick={() => (showCreate = !showCreate)}>
				{showCreate ? 'Cancel' : '+ Add Model'}
			</button>
		</div>
	</header>

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">Models</div>
			<div class="summary-value">{models.length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Providers</div>
			<div class="summary-value">{new Set(models.map((m) => new URL(m.base_url || 'http://x').hostname).filter(Boolean)).size}</div>
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
					<span>Key (TOML section name)</span>
					<input bind:value={draft.key} required placeholder="deepseek-v4-pro" />
				</label>
				<label>
					<span>Model Name</span>
					<input bind:value={draft.model_name} placeholder="deepseek-chat" />
				</label>
			</div>
			<div class="row two">
				<label>
					<span>Base URL</span>
					<input bind:value={draft.base_url} placeholder="https://api.deepseek.com" />
				</label>
				<label>
					<span>Host</span>
					<input bind:value={draft.host} placeholder="cloud" />
				</label>
			</div>
			<label>
				<span>API Key</span>
				<input type="password" bind:value={(draft as any).api_key} placeholder="sk-…" />
			</label>
			<div class="row two">
				<label>
					<span>Thinking Type</span>
					<input bind:value={draft.thinking_type} placeholder="disabled" />
				</label>
				<label>
					<span>Timeout (sec)</span>
					<input type="number" bind:value={draft.timeout_sec} min="0" />
				</label>
			</div>
			<div class="row three">
				<label>
					<span>Max Inflight</span>
					<input type="number" bind:value={draft.max_inflight} min="0" />
				</label>
				<label>
					<span>Max Req/Min</span>
					<input type="number" bind:value={draft.max_requests_per_minute} min="0" />
				</label>
				<label>
					<span>Max Tokens/Min</span>
					<input type="number" bind:value={draft.max_tokens_per_minute} min="0" />
				</label>
			</div>
			<label>
				<span>Token Reserve/Call</span>
				<input type="number" bind:value={draft.token_reserve_per_call} min="0" style="max-width:200px;" />
			</label>
			<div class="form-foot">
				<button class="primary" type="submit" disabled={submitting || !draft.key.trim()}>
					{submitting ? 'Saving…' : 'Save to .models.toml'}
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
				<h3>Current Entries</h3>
				<p class="muted">All model definitions in .models.toml. Changes take effect on next server restart.</p>
			</div>
		</div>

		{#if loading}
			<div class="empty">Loading…</div>
		{:else if models.length === 0}
			<div class="empty">No models in .models.toml yet. Add one above.</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>Key</th>
							<th>Model</th>
							<th>Base URL</th>
							<th>Timeout</th>
							<th>Inflight</th>
							<th>Action</th>
						</tr>
					</thead>
					<tbody>
						{#each models as m (m.key)}
							<tr>
								<td>
									<div class="cell-primary">{m.key}</div>
									{#if m.thinking_type}
										<div class="cell-secondary">thinking: {m.thinking_type}</div>
									{/if}
								</td>
								<td>{m.model_name || '—'}</td>
								<td class="url-cell">{m.base_url || '—'}</td>
								<td>{m.timeout_sec || '—'}</td>
								<td>{m.max_inflight || '—'}</td>
								<td>
									<div class="row-actions">
										{#if editingKey === m.key}
											<button class="ghost compact-btn" onclick={() => (editingKey = null)} disabled={submitting}>Cancel</button>
										{:else if deletingKey === m.key}
											<button class="danger-btn compact-btn" onclick={() => confirmDelete(m.key)} disabled={submitting}>
												{submitting ? 'Deleting…' : 'Confirm'}
											</button>
											<button class="ghost compact-btn" onclick={() => (deletingKey = null)}>Cancel</button>
										{:else}
											<button class="ghost compact-btn" onclick={() => startEdit(m)}>Edit</button>
											<button class="ghost compact-btn" onclick={() => (deletingKey = m.key)}>Delete</button>
										{/if}
									</div>
								</td>
							</tr>
							{#if editingKey === m.key}
								<tr>
									<td colspan="6" class="edit-cell">
										<form
											class="edit-form"
											onsubmit={(e) => {
												e.preventDefault();
												saveEdit();
											}}
										>
											<div class="row two">
												<label>
													<span>Key</span>
													<input bind:value={editDraft.key} required />
												</label>
												<label>
													<span>Model Name</span>
													<input bind:value={editDraft.model_name} />
												</label>
											</div>
											<div class="row two">
												<label>
													<span>Base URL</span>
													<input bind:value={editDraft.base_url} />
												</label>
												<label>
													<span>Host</span>
													<input bind:value={editDraft.host} />
												</label>
											</div>
											<div class="row two">
												<label>
													<span>Thinking Type</span>
													<input bind:value={editDraft.thinking_type} />
												</label>
												<label>
													<span>Timeout (sec)</span>
													<input type="number" bind:value={editDraft.timeout_sec} min="0" />
												</label>
											</div>
											<div class="row three">
												<label>
													<span>Max Inflight</span>
													<input type="number" bind:value={editDraft.max_inflight} min="0" />
												</label>
												<label>
													<span>Max Req/Min</span>
													<input type="number" bind:value={editDraft.max_requests_per_minute} min="0" />
												</label>
												<label>
													<span>Max Tokens/Min</span>
													<input type="number" bind:value={editDraft.max_tokens_per_minute} min="0" />
												</label>
											</div>
											<div class="form-foot">
												<button class="primary" type="submit" disabled={submitting}>
													{submitting ? 'Saving…' : 'Save changes'}
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
	.toolbar, .panel-head, .form-foot, .toolbar-actions, .row-actions {
		display: flex;
	}
	.toolbar, .panel-head {
		justify-content: space-between;
		align-items: flex-end;
		gap: 12px;
	}
	.toolbar-actions, .row-actions {
		gap: 8px;
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
	.primary, .ghost, .danger-btn {
		border-radius: 8px;
		padding: 8px 14px;
		font-size: 13px;
		cursor: pointer;
	}
	.primary { background: var(--btn); color: white; border: none; }
	.ghost { background: transparent; color: var(--heading); border: 1px solid var(--border); }
	.danger-btn { background: var(--danger); color: white; border: none; }
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
	input {
		background: var(--input-bg);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		font-family: inherit;
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
	.url-cell { font-size: 12px; max-width: 240px; word-break: break-all; }
	.empty { color: var(--sub); font-style: italic; padding: 24px 8px 8px; }
</style>
