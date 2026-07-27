<script lang="ts">
	// Task 8.1: the /home3/cdm document list -- shows existing CDM documents
	// for the active knowledge store and lets an author start a new one.
	//
	// tenant_id/ks_store_id come from knowledgeStoreState (home3's existing
	// active-knowledge-store singleton, shared with kb-import-view.svelte and
	// document-review-view.svelte), not a new selection mechanism: cdm-client.ts's
	// createDocument/listDocuments already need exactly what that state holds,
	// and every other home3 feature that needs a tenant sources it the same
	// way. The inline picker below (loadStoreOptions/pickStore) is copied in
	// shape from document-review-view.svelte's, not extracted into a shared
	// component, matching how that duplication already exists between
	// kb-import-view.svelte and document-review-view.svelte.
	import { goto } from '$app/navigation';
	import { listKnowledgeStores, type KnowledgeStoreRecord } from '$lib/services/kbService';
	import { knowledgeStoreState } from '../home3/knowledge-store-state.svelte.js';
	import { listDocuments, createDocument, type DocumentSummary } from './cdm-client.js';
	import { SCHEMA_VERSION, type Document } from './types.js';

	let activeStore = $derived(knowledgeStoreState.activeStore);

	let storeOptions = $state<KnowledgeStoreRecord[]>([]);
	let loadingStores = $state(false);
	let storeError = $state('');

	let documents = $state<DocumentSummary[]>([]);
	let loading = $state(false);
	let loadError = $state('');

	let newTitle = $state('');
	let creating = $state(false);
	let createError = $state('');

	async function loadStoreOptions() {
		loadingStores = true;
		storeError = '';
		try {
			const res = await listKnowledgeStores();
			storeOptions = res.results ?? [];
		} catch (err) {
			storeError = err instanceof Error ? err.message : 'Failed to load knowledge stores.';
		} finally {
			loadingStores = false;
		}
	}

	function pickStore(store: KnowledgeStoreRecord) {
		knowledgeStoreState.setActiveStore(store);
	}

	function changeStore() {
		knowledgeStoreState.setActiveStore(null);
		if (storeOptions.length === 0) loadStoreOptions();
	}

	async function loadDocuments(tenantId: string) {
		loading = true;
		loadError = '';
		try {
			const res = await listDocuments({ tenantId });
			documents = res.results;
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Failed to load documents.';
		} finally {
			loading = false;
		}
	}

	// Loads whenever the active store (or its tenant_id) changes, including
	// the very first time one is picked; re-derives from `activeStore` rather
	// than a one-time onMount so switching stores refreshes the list too.
	$effect(() => {
		const tenantId = activeStore?.tenant_id;
		if (tenantId) {
			loadDocuments(tenantId);
		} else if (storeOptions.length === 0 && !loadingStores) {
			loadStoreOptions();
		}
	});

	async function create() {
		const tenantId = activeStore?.tenant_id;
		const title = newTitle.trim();
		if (!tenantId || !title) return;
		creating = true;
		createError = '';
		try {
			const doc: Document = {
				document_key: '',
				title,
				language: 'en',
				schema_version: SCHEMA_VERSION,
				content_version: 0,
				metadata: {},
				blocks: []
			};
			const result = await createDocument(doc, { tenantId, ksStoreId: activeStore?.id });
			goto(`/home3/cdm/${encodeURIComponent(result.document_key)}`);
		} catch (err) {
			createError = err instanceof Error ? err.message : 'Failed to create document.';
		} finally {
			creating = false;
		}
	}
</script>

<div class="cdm-doc-list">
	<h1>CDM Documents</h1>

	{#if !activeStore}
		<section class="cdm-store-picker">
			<p>Pick a knowledge store to see its documents.</p>
			{#if loadingStores}
				<p>Loading knowledge stores…</p>
			{:else if storeError}
				<p class="cdm-error">{storeError}</p>
			{:else if storeOptions.length === 0}
				<p>No knowledge stores available.</p>
			{:else}
				<ul class="cdm-store-options">
					{#each storeOptions as store (store.id)}
						<li>
							<button type="button" onclick={() => pickStore(store)}>{store.ks_name}</button>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{:else}
		<div class="cdm-active-store">
			<span>Store: <strong>{activeStore.ks_name}</strong></span>
			<button type="button" onclick={changeStore}>Change store</button>
		</div>

		<section class="cdm-create-row">
			<input
				type="text"
				placeholder="New document title"
				bind:value={newTitle}
				disabled={creating}
			/>
			<button type="button" onclick={create} disabled={creating || !newTitle.trim()}>
				{creating ? 'Creating…' : 'Create document'}
			</button>
		</section>
		{#if createError}
			<p class="cdm-error">{createError}</p>
		{/if}

		{#if loading}
			<p>Loading documents…</p>
		{:else if loadError}
			<p class="cdm-error">{loadError}</p>
		{:else if documents.length === 0}
			<p class="cdm-empty">No documents yet.</p>
		{:else}
			<table class="cdm-doc-table">
				<thead>
					<tr>
						<th>Title</th>
						<th>Version</th>
						<th>Status</th>
						<th>Updated</th>
					</tr>
				</thead>
				<tbody>
					{#each documents as doc (doc.document_key)}
						<tr>
							<td>
								<a href={`/home3/cdm/${encodeURIComponent(doc.document_key)}`}>{doc.title}</a>
							</td>
							<td>v{doc.content_version}</td>
							<td>{doc.published ? 'Published' : 'Draft'}</td>
							<td>{doc.update_time}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	{/if}
</div>

<style>
	.cdm-doc-list {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.cdm-active-store {
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 0.9em;
	}
	.cdm-create-row {
		display: flex;
		gap: 8px;
	}
	.cdm-create-row input {
		flex: 1 1 auto;
		border: 1px solid rgba(127, 127, 127, 0.35);
		border-radius: 6px;
		padding: 6px 10px;
	}
	.cdm-doc-table {
		border-collapse: collapse;
		width: 100%;
	}
	.cdm-doc-table th,
	.cdm-doc-table td {
		border: 1px solid rgba(127, 127, 127, 0.3);
		padding: 6px 10px;
		text-align: left;
	}
	.cdm-store-options {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.cdm-error {
		color: #b91c1c;
	}
	.cdm-empty {
		color: var(--cdm-muted, #6b7280);
		font-style: italic;
	}
</style>
