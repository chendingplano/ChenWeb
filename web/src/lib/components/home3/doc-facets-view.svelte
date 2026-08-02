<script lang="ts">
	import { onMount } from 'svelte';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	type DocFacet = {
		record_id: number;
		ks_store_id: number;
		knowledge_store_binding: string;
		input_doc_type: string;
		source_language: string;
		has_document_number: boolean;
		create_time: string;
		modify_time: string;
	};

	type Draft = Omit<DocFacet, 'record_id' | 'create_time' | 'modify_time'>;

	let records = $state<DocFacet[]>([]);
	let loading = $state(false);
	let submitting = $state(false);
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let searchQuery = $state('');
	let editingId = $state<number | null>(null);
	let deletingId = $state<number | null>(null);
	let editDraft = $state<Draft>({
		ks_store_id: 0,
		knowledge_store_binding: '',
		input_doc_type: '',
		source_language: '',
		has_document_number: false
	});

	onMount(() => {
		loadRecords();
	});

	async function loadRecords() {
		loading = true;
		error = null;
		try {
			const params = searchQuery.trim() ? `?q=${encodeURIComponent(searchQuery.trim())}` : '';
			const res = await fetch(`/api/v1/kb/doc-facets/list${params}`, { credentials: 'same-origin' });
			if (!res.ok) throw new Error(`HTTP ${res.status}`);
			const data = await res.json();
			records = data.results ?? [];
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			loading = false;
		}
	}

	function startEdit(r: DocFacet) {
		editingId = r.record_id;
		editDraft = {
			ks_store_id: r.ks_store_id,
			knowledge_store_binding: r.knowledge_store_binding,
			input_doc_type: r.input_doc_type,
			source_language: r.source_language,
			has_document_number: r.has_document_number
		};
		error = null;
		info = null;
	}

	async function saveEdit() {
		if (editingId === null) return;
		submitting = true;
		error = null;
		info = null;
		try {
			const res = await fetch(`/api/v1/kb/doc-facets/${editingId}`, {
				method: 'PUT',
				credentials: 'same-origin',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(editDraft)
			});
			if (!res.ok) {
				const body = await res.json().catch(() => null);
				throw new Error(body?.error_msg ?? `HTTP ${res.status}`);
			}
			editingId = null;
			info = `Record ${editingId} updated.`;
			await loadRecords();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	async function confirmDelete(recordId: number) {
		submitting = true;
		error = null;
		info = null;
		try {
			const res = await fetch(`/api/v1/kb/doc-facets/${recordId}`, {
				method: 'DELETE',
				credentials: 'same-origin'
			});
			if (!res.ok) {
				const body = await res.json().catch(() => null);
				throw new Error(body?.error_msg ?? `HTTP ${res.status}`);
			}
			deletingId = null;
			info = `Record ${recordId} deleted.`;
			await loadRecords();
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			submitting = false;
		}
	}

	function handleSearch() {
		loadRecords();
	}

	function formatDate(iso: string): string {
		if (!iso) return '—';
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
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
			<h2>Doc Facets</h2>
			<p class="muted">Deterministic routing facets for kb.doc_facets (keyed by record_id)</p>
		</div>
		<div class="toolbar-actions">
			<button class="ghost" onclick={loadRecords} disabled={loading}>
				{loading ? 'Refreshing…' : 'Refresh'}
			</button>
		</div>
	</header>

	<div class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">Total Records</div>
			<div class="summary-value">{records.length}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Doc Types</div>
			<div class="summary-value">{new Set(records.map((r) => r.input_doc_type).filter(Boolean)).size}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Languages</div>
			<div class="summary-value">{new Set(records.map((r) => r.source_language).filter(Boolean)).size}</div>
		</div>
	</div>

	<div class="search-bar">
		<input
			type="text"
			placeholder="Search by record_id, store_id, binding, doc type, or language…"
			bind:value={searchQuery}
			onkeydown={(e) => { if (e.key === 'Enter') handleSearch(); }}
		/>
		<button class="primary" onclick={handleSearch} disabled={loading}>Search</button>
		{#if searchQuery}
			<button class="ghost" onclick={() => { searchQuery = ''; handleSearch(); }}>Clear</button>
		{/if}
	</div>

	{#if error}
		<div class="error" role="alert">{error}</div>
	{:else if info}
		<div class="info" role="status">{info}</div>
	{/if}

	<div class="panel">
		<div class="panel-head">
			<div>
				<h3>Records</h3>
				<p class="muted">Each row maps a kb.inputs record to its routing facets.</p>
			</div>
		</div>

		{#if loading}
			<div class="empty">Loading…</div>
		{:else if records.length === 0}
			<div class="empty">No records found.</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>Record ID</th>
							<th>KS Store ID</th>
							<th>Binding</th>
							<th>Doc Type</th>
							<th>Language</th>
							<th>Has Doc #</th>
							<th>Modified</th>
							<th>Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each records as r (r.record_id)}
							<tr>
								<td><div class="cell-primary">{r.record_id}</div></td>
								<td>{r.ks_store_id}</td>
								<td>{r.knowledge_store_binding || '—'}</td>
								<td>{r.input_doc_type || '—'}</td>
								<td>{r.source_language || '—'}</td>
								<td>{r.has_document_number ? 'Yes' : 'No'}</td>
								<td class="date-cell">{formatDate(r.modify_time)}</td>
								<td>
									<div class="row-actions">
										{#if editingId === r.record_id}
											<button class="ghost compact-btn" onclick={() => (editingId = null)} disabled={submitting}>Cancel</button>
										{:else if deletingId === r.record_id}
											<button class="danger-btn compact-btn" onclick={() => confirmDelete(r.record_id)} disabled={submitting}>
												{submitting ? 'Deleting…' : 'Confirm'}
											</button>
											<button class="ghost compact-btn" onclick={() => (deletingId = null)}>Cancel</button>
										{:else}
											<button class="ghost compact-btn" onclick={() => startEdit(r)}>Edit</button>
											<button class="ghost compact-btn" onclick={() => (deletingId = r.record_id)}>Delete</button>
										{/if}
									</div>
								</td>
							</tr>
							{#if editingId === r.record_id}
								<tr>
									<td colspan="8" class="edit-cell">
										<form
											class="edit-form"
											onsubmit={(e) => {
												e.preventDefault();
												saveEdit();
											}}
										>
											<div class="row three">
												<label>
													<span>KS Store ID</span>
													<input type="number" bind:value={editDraft.ks_store_id} min="0" />
												</label>
												<label>
													<span>Knowledge Store Binding</span>
													<input bind:value={editDraft.knowledge_store_binding} placeholder="absent" />
												</label>
												<label>
													<span>Input Doc Type</span>
													<input bind:value={editDraft.input_doc_type} placeholder="pdf" />
												</label>
											</div>
											<div class="row two">
												<label>
													<span>Source Language</span>
													<input bind:value={editDraft.source_language} placeholder="en" />
												</label>
												<label class="checkbox-label">
													<span>Has Document Number</span>
													<input type="checkbox" bind:checked={editDraft.has_document_number} />
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
	.search-bar {
		display: flex;
		gap: 8px;
		align-items: center;
	}
	.search-bar input {
		flex: 1;
		background: var(--card);
		color: var(--heading);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 12px;
		font-size: 13px;
		font-family: inherit;
	}
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
	.summary-card, .panel {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 10px;
	}
	.summary-card { padding: 14px 16px; }
	.summary-label { font-size: 12px; color: var(--sub); text-transform: uppercase; letter-spacing: 0.04em; }
	.summary-value { margin-top: 6px; font-size: 22px; font-weight: 600; color: var(--heading); }
	.panel { padding: 16px; }
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
	.checkbox-label {
		flex-direction: row;
		align-items: center;
		gap: 8px;
		padding-top: 20px;
	}
	.checkbox-label input[type="checkbox"] {
		width: 18px;
		height: 18px;
	}
	input:not([type="checkbox"]) {
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
	.date-cell { font-size: 12px; white-space: nowrap; }
	.empty { color: var(--sub); font-style: italic; padding: 24px 8px 8px; }
</style>
