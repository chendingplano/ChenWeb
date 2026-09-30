<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import {
		listDocProcessors,
		createProcessor,
		updateProcessor,
		deleteProcessor,
		DOC_PROCESSOR_TYPES,
		DOC_PROCESSOR_STATUSES,
		type DocProcessor,
		type DocProcessorType,
		type DocProcessorStatus
	} from './doc-processors-client';

	interface Props {
		darkMode?: boolean;
	}

	let { darkMode = true }: Props = $props();

	const pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	const cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	const surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	const borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	const accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	const heading = $derived(darkMode ? '#E2E8F0' : '#111827');
	const sub = $derived(darkMode ? '#94A3B8' : '#6B7280');

	let processors = $state<DocProcessor[]>([]);
	let loading = $state(true);
	let search = $state('');
	let searchInput = $state('');
	let errorMsg = $state('');
	let infoMsg = $state('');

	let showEditor = $state(false);
	let editing = $state(false);
	let editingOriginalName = $state('');
	let saving = $state(false);

	// Editor fields
	let edName = $state('');
	let edDisplay = $state('');
	let edType = $state<DocProcessorType>('configurable');
	let edStatus = $state<DocProcessorStatus>('active');
	let edRequireLLM = $state(false);
	let edDescription = $state('');
	let edNotes = $state('');
	let edRequires = $state('');

	let deleted = $state('');
	let deleting = $state(false);

	function parseRequiresList(raw: string): string[] {
		const seen = new Set<string>();
		const out: string[] = [];
		for (const part of raw.split(',')) {
			const v = part.trim();
			if (v && !seen.has(v)) {
				seen.add(v);
				out.push(v);
			}
		}
		return out;
	}

	function formatTime(iso: string): string {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return iso;
		return d.toLocaleString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	async function loadAll() {
		loading = true;
		errorMsg = '';
		try {
			processors = await listDocProcessors(search);
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : String(err);
		} finally {
			loading = false;
		}
	}

	// Debounced search
	let searchTimer: ReturnType<typeof setTimeout> | undefined;
	function onSearchInput() {
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			search = searchInput.trim();
			loadAll();
		}, 300);
	}

	function openCreate() {
		errorMsg = '';
		editing = false;
		editingOriginalName = '';
		edName = '';
		edDisplay = '';
		edType = 'configurable';
		edStatus = 'active';
		edRequireLLM = false;
		edDescription = '';
		edNotes = '';
		edRequires = '';
		showEditor = true;
	}

	function openEdit(p: DocProcessor) {
		errorMsg = '';
		editing = true;
		editingOriginalName = p.name_as_id;
		edName = p.name_as_id;
		edDisplay = p.display_name;
		edType = p.type;
		edStatus = p.status;
		edRequireLLM = p.require_llm;
		edDescription = p.description ?? '';
		edNotes = p.notes ?? '';
		edRequires = (p.requires ?? []).join(', ');
		showEditor = true;
	}

	function closeEditor() {
		showEditor = false;
	}

	async function submitEditor() {
		errorMsg = '';
		if (!edName.trim() && !editing) {
			errorMsg = m.doc_processors_name_as_id_is_required();
			return;
		}
		if (!edDisplay.trim()) {
			errorMsg = m.doc_processors_display_name_is_required();
			return;
		}
		const requires = parseRequiresList(edRequires);
		saving = true;
		try {
			if (editing) {
				await updateProcessor(editingOriginalName, {
					display_name: edDisplay,
					type: edType,
					status: edStatus,
					require_llm: edRequireLLM,
					description: edDescription ? edDescription : null,
					notes: edNotes ? edNotes : null,
					requires
				});
				infoMsg = m.doc_processors_updated({ editingOriginalName });
			} else {
				await createProcessor({
					name_as_id: edName,
					display_name: edDisplay,
					type: edType,
					status: edStatus,
					require_llm: edRequireLLM,
					description: edDescription ? edDescription : undefined,
					notes: edNotes ? edNotes : undefined,
					requires
				});
				infoMsg = m.doc_processors_created({ edName });
			}
			showEditor = false;
			await loadAll();
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : String(err);
		} finally {
			saving = false;
		}
	}

	async function onDelete(p: DocProcessor) {
		if (!confirm(m.doc_processors_delete_doc_processor_this_cannot({ name_as_id: p.name_as_id }))) return;
		deleting = true;
		errorMsg = '';
		try {
			await deleteProcessor(p.name_as_id);
			deleted = p.name_as_id;
			await loadAll();
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : String(err);
		} finally {
			deleting = false;
		}
	}

	onMount(loadAll);
</script>

<div
	class="page"
	class:dark={darkMode}
	style:--page={pageBg}
	style:--card={cardBg}
	style:--panel-bg={surface2}
	style:--border={borderColor}
	style:--input-bg={surface2}
	style:--heading={heading}
	style:--sub={sub}
	style:--btn={accent}
>
	<div class="header">
		<div class="header-left">
			<h1>{m.doc_processors_doc_processors()}</h1>
			<p class="sub">{m.doc_processors_admin_catalog_of_the_pipeline()}</p>
		</div>
		<button class="btn primary" onclick={openCreate}>{m.doc_processors_new_processor()}</button>
	</div>

	{#if errorMsg}
		<div class="banner error" role="alert">{errorMsg}</div>
	{/if}
	{#if infoMsg}
		<div class="banner info" role="status">{infoMsg}</div>
	{/if}
	{#if deleted}
		<div class="banner info" role="status">{m.doc_processors_deleted({ deleted })}</div>
	{/if}

	<div class="card">
		<div class="toolbar">
			<input
				type="text"
				class="search"
				placeholder={m.doc_processors_search_name_or_display_name()}
				bind:value={searchInput}
				oninput={onSearchInput}
			/>
			<span class="count">{m.doc_processors_processor({ processorsCount: processors.length, plural: processors.length === 1 ? '' : 's' })}</span>
		</div>

		{#if loading}
			<div class="loading">{m.doc_processors_loading()}</div>
		{:else if processors.length === 0}
			<div class="empty">{m.doc_processors_no_doc_processors_found()}</div>
		{:else}
			<div class="table-wrap">
				<table>
					<thead>
						<tr>
							<th>{m.doc_processors_name_id()}</th>
							<th>{m.doc_processors_display_name()}</th>
							<th>{m.doc_processors_type()}</th>
							<th>{m.doc_processors_llm()}</th>
							<th>{m.doc_processors_status()}</th>
							<th>{m.doc_processors_requires()}</th>
							<th>{m.doc_processors_notes()}</th>
							<th>{m.doc_processors_modified()}</th>
							<th class="actions-col">{m.doc_processors_actions()}</th>
						</tr>
					</thead>
					<tbody>
						{#each processors as p (p.name_as_id)}
							<tr>
								<td class="mono">{p.name_as_id}</td>
								<td>
									{p.display_name}
									{#if p.description}
										<div class="cell-desc">{p.description}</div>
									{/if}
								</td>
								<td>
									<span class="badge" class:type-config={p.type === 'configurable'}>{p.type}</span>
								</td>
								<td>
									{#if p.require_llm}
										<span class="badge llm">{m.doc_processors_llm()}</span>
									{:else}
										<span class="muted">—</span>
									{/if}
								</td>
								<td>
									<span
										class="badge status"
										class:st-active={p.status === 'active'}
										class:st-disabled={p.status === 'disabled'}
										class:st-suspended={p.status === 'suspended'}
									>{p.status}</span
									>
								</td>
								<td>
									{#if p.requires && p.requires.length}
										<div class="req-list">
											{#each p.requires as r (r)}
												<span class="badge req">{r}</span>
											{/each}
										</div>
									{:else}
										<span class="muted">—</span>
									{/if}
								</td>
								<td>
									{#if p.notes}
										<div class="cell-notes">{p.notes}</div>
									{:else}
										<span class="muted">—</span>
									{/if}
								</td>
								<td class="muted">{formatTime(p.modify_time)}</td>
								<td class="actions-col">
									<button class="btn small" onclick={() => openEdit(p)}>{m.doc_processors_edit()}</button>
									<button
										class="btn small danger"
										onclick={() => onDelete(p)}
										disabled={deleting}
									>{m.doc_processors_delete()}</button
									>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>

{#if showEditor}
	<div
		class="modal-overlay"
		class:dark={darkMode}
		style:--card={cardBg}
		style:--border={borderColor}
		style:--input-bg={surface2}
		style:--heading={heading}
		style:--sub={sub}
		style:--btn={accent}
	>
		<div class="modal">
			<div class="modal-header">
				<h2>{editing ? m.doc_processors_edit_2({ editingOriginalName }) : m.doc_processors_new_processor_2()}</h2>
				<button class="btn ghost" onclick={closeEditor} aria-label={m.doc_processors_close()}>✕</button>
			</div>
			<form onsubmit={(e) => { e.preventDefault(); submitEditor(); }}>
				<div class="form-grid">
					<label>
						<span>{m.doc_processors_name_id_2({ value: editing ? '' : ' *' })}</span>
						<input
							type="text"
							bind:value={edName}
							disabled={editing}
							placeholder={m.doc_processors_e_g_extract_metrics()}
							class:input-disabled={editing}
						/>
						{#if editing}
							<small class="hint">{m.doc_processors_name_as_id_is_immutable()}</small>
						{/if}
					</label>
					<label>
						<span>{m.doc_processors_display_name_2()}</span>
						<input type="text" bind:value={edDisplay} placeholder={m.doc_processors_e_g_extract_metrics_2()} />
					</label>
					<label>
						<span>{m.doc_processors_type()}</span>
						<select bind:value={edType}>
							{#each DOC_PROCESSOR_TYPES as t}
								<option value={t}>{t}</option>
							{/each}
						</select>
					</label>
					<label>
						<span>{m.doc_processors_status()}</span>
						<select bind:value={edStatus}>
							{#each DOC_PROCESSOR_STATUSES as s}
								<option value={s}>{s}</option>
							{/each}
						</select>
					</label>
					<label class="toggle-label">
						<span>{m.doc_processors_requires_llm()}</span>
						<input type="checkbox" bind:checked={edRequireLLM} />
					</label>
					<label class="wide">
						<span>{m.doc_processors_requires_comma_separated_name_as()}</span>
						<input
							type="text"
							bind:value={edRequires}
							placeholder={m.doc_processors_e_g_chunking_extract_metadata()}
						/>
						<small class="hint">{m.doc_processors_processors_this_processor_depends_on()}</small>
					</label>
					<label class="wide">
						<span>{m.doc_processors_description()}</span>
						<textarea
							bind:value={edDescription}
							placeholder={m.doc_processors_describe_the_processor_and_its()}
							rows="3"
						></textarea>
					</label>
					<label class="wide">
						<span>{m.doc_processors_notes()}</span>
						<textarea
							bind:value={edNotes}
							placeholder={m.doc_processors_free_form_notes()}
							rows="3"
						></textarea>
					</label>
				</div>
				{#if errorMsg}
					<div class="banner error modal-error" role="alert">{errorMsg}</div>
				{/if}
				<div class="modal-footer">
					<button type="button" class="btn ghost" onclick={closeEditor}>{m.doc_processors_cancel()}</button>
					<button type="submit" class="btn primary" disabled={saving}>
						{saving ? m.doc_processors_saving() : editing ? m.doc_processors_save_changes() : m.doc_processors_create()}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		height: 100%;
		overflow-y: auto;
		padding: 1.25rem;
		background: var(--page, #f5f6f8);
		color: var(--heading, #1b1e23);
	}

	.header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}

	.header-left h1 {
		margin: 0;
		font-size: 1.25rem;
		font-weight: 650;
	}

	.sub {
		margin: 0.25rem 0 0;
		font-size: 0.85rem;
		color: var(--sub, #6b7280);
	}

	.banner {
		padding: 0.6rem 0.9rem;
		border-radius: 8px;
		font-size: 0.85rem;
	}

	.banner.error {
		background: rgba(220, 38, 38, 0.1);
		color: #dc2626;
		border: 1px solid rgba(220, 38, 38, 0.25);
	}

	.banner.info {
		background: rgba(16, 185, 129, 0.1);
		color: #059669;
		border: 1px solid rgba(16, 185, 129, 0.25);
	}

	.card {
		background: var(--card, #ffffff);
		border: 1px solid var(--border, #e2e5e9);
		border-radius: 12px;
		padding: 1rem;
	}

	.toolbar {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
	}

	.search {
		flex: 1;
		max-width: 380px;
		padding: 0.5rem 0.75rem;
		border-radius: 8px;
		border: 1px solid var(--border, #e2e5e9);
		background: var(--input-bg, #fbfbfc);
		color: inherit;
		font-size: 0.875rem;
		outline: none;
	}

	.search:focus {
		border-color: var(--btn, #2563eb);
	}

	.count {
		font-size: 0.8rem;
		color: var(--sub, #6b7280);
	}

	.loading,
	.empty {
		padding: 2rem 1rem;
		text-align: center;
		color: var(--sub, #6b7280);
		font-size: 0.9rem;
	}

	.table-wrap {
		overflow-x: auto;
	}

	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
	}

	th {
		text-align: left;
		padding: 0.5rem 0.6rem;
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--sub, #6b7280);
		border-bottom: 1px solid var(--border, #e2e5e9);
		white-space: nowrap;
	}

	td {
		padding: 0.55rem 0.6rem;
		border-bottom: 1px solid var(--border, #e2e5e9);
		vertical-align: top;
	}

	tr:last-child td {
		border-bottom: none;
	}

	.mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.82rem;
		white-space: nowrap;
	}

	.muted {
		color: var(--sub, #6b7280);
		font-size: 0.8rem;
		white-space: nowrap;
	}

	.cell-desc {
		margin-top: 0.15rem;
		font-size: 0.78rem;
		color: var(--sub, #6b7280);
		max-width: 34rem;
	}

	.badge {
		display: inline-block;
		padding: 0.15rem 0.5rem;
		border-radius: 999px;
		font-size: 0.72rem;
		font-weight: 600;
		background: var(--panel-bg, #eef0f3);
		color: var(--sub, #6b7280);
		white-space: nowrap;
	}

	.badge.type-config {
		background: rgba(139, 92, 246, 0.12);
		color: #7c3aed;
	}

	.badge.llm {
		background: rgba(245, 158, 11, 0.14);
		color: #b45309;
	}

	.badge.st-active {
		background: rgba(16, 185, 129, 0.14);
		color: #059669;
	}

	.badge.st-disabled {
		background: rgba(107, 114, 128, 0.14);
		color: #6b7280;
	}

	.badge.st-suspended {
		background: rgba(220, 38, 38, 0.14);
		color: #dc2626;
	}

	.req-list {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem;
	}

	.badge.req {
		background: rgba(59, 130, 246, 0.12);
		color: #2563eb;
	}

	.cell-notes {
		margin-top: 0.15rem;
		font-size: 0.78rem;
		color: var(--sub, #6b7280);
		max-width: 26rem;
	}

	.dark .banner.error {
		color: #f87171;
	}

	.dark .banner.info {
		color: #34d399;
	}

	.dark .badge.type-config {
		color: #a78bfa;
	}

	.dark .badge.llm {
		color: #fbbf24;
	}

	.dark .badge.st-active {
		color: #34d399;
	}

	.dark .badge.st-disabled {
		color: #94a3b8;
	}

	.dark .badge.st-suspended {
		color: #f87171;
	}

	.dark .badge.req {
		color: #93c5fd;
	}

	.dark .btn.danger {
		color: #f87171;
	}

	.dark .btn.danger:hover {
		border-color: #f87171;
		color: #f87171;
	}

	.actions-col {
		white-space: nowrap;
	}

	.btn {
		padding: 0.4rem 0.8rem;
		border-radius: 8px;
		border: 1px solid var(--border, #e2e5e9);
		background: var(--input-bg, #fbfbfc);
		color: var(--heading, #1b1e23);
		font-size: 0.85rem;
		cursor: pointer;
	}

	.btn:hover {
		border-color: var(--btn, #2563eb);
		color: var(--btn, #2563eb);
	}

	.btn.small {
		padding: 0.25rem 0.6rem;
		font-size: 0.78rem;
	}

	.btn.danger {
		color: #dc2626;
	}

	.btn.danger:hover {
		border-color: #dc2626;
		color: #dc2626;
	}

	.btn.primary {
		background: var(--btn, #2563eb);
		border-color: var(--btn, #2563eb);
		color: #fff;
	}

	.btn.primary:hover {
		filter: brightness(0.94);
		color: #fff;
	}

	.btn.ghost {
		background: transparent;
	}

	.btn:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.45);
		display: flex;
		align-items: flex-start;
		justify-content: center;
		z-index: 50;
		padding: 4rem 1rem 1rem;
	}

	.modal {
		width: min(560px, 100%);
		color: var(--heading, #1b1e23);
		background: var(--card, #ffffff);
		border: 1px solid var(--border, #e2e5e9);
		border-radius: 12px;
		box-shadow: 0 12px 32px rgba(0, 0, 0, 0.18);
		padding: 1.25rem;
	}

	.modal-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 1rem;
	}

	.modal-header h2 {
		margin: 0;
		font-size: 1.05rem;
	}

	.form-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.9rem;
	}

	label {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		font-size: 0.82rem;
		color: var(--sub, #6b7280);
	}

	label.wide {
		grid-column: 1 / -1;
	}

	input[type='text'],
	select,
	textarea {
		padding: 0.5rem 0.7rem;
		border-radius: 8px;
		border: 1px solid var(--border, #e2e5e9);
		background: var(--input-bg, #fbfbfc);
		color: inherit;
		font-size: 0.875rem;
		outline: none;
	}

	textarea {
		resize: vertical;
		min-height: 84px;
		line-height: 1.45;
		font-family: inherit;
	}

	input[type='text']:focus,
	select:focus,
	textarea:focus {
		border-color: var(--btn, #2563eb);
	}

	input.input-disabled {
		opacity: 0.6;
	}

	.hint {
		font-size: 0.72rem;
		color: var(--sub, #6b7280);
	}

	.toggle-label {
		flex-direction: row;
		align-items: center;
		gap: 0.5rem;
		padding-top: 1.35rem;
	}

	.toggle-label input[type='checkbox'] {
		width: 1rem;
		height: 1rem;
		accent-color: var(--btn, #2563eb);
	}

	.modal-error {
		margin-top: 0.9rem;
	}

	.modal-footer {
		display: flex;
		justify-content: flex-end;
		gap: 0.6rem;
		margin-top: 1.1rem;
	}
</style>
