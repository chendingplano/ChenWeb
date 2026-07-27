<script lang="ts">
	// The CDM Editor main page: the landing surface reached from the
	// "CDM Editor" workspace app tile (/semos/workspace -> /home3/cdm) and the
	// hub every authoring action starts from -- pick a knowledge store, start a
	// document, find an existing one, open it in the editor.
	//
	// This replaced the bridging list task group 8 shipped with the MVP (a bare
	// <h1> + <ul> store picker + unstyled <table>), which was route wiring
	// rather than a page. What it does has not changed: same three API calls,
	// same knowledgeStoreState scoping. What is new is that it is a designed
	// destination -- theme-aware, with a store selector, title filter, status
	// filter, and designed empty/loading/error states.
	//
	// tenant_id/ks_store_id come from knowledgeStoreState (home3's existing
	// active-knowledge-store singleton, shared with kb-import-view.svelte and
	// document-review-view.svelte), not a new selection mechanism:
	// cdm-client.ts's createDocument/listDocuments already need exactly what
	// that state holds, and every other home3 feature that needs a tenant
	// sources it the same way.
	import { goto } from '$app/navigation';
	import FilePlusIcon from '@lucide/svelte/icons/file-plus';
	import SearchIcon from '@lucide/svelte/icons/search';
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import { listKnowledgeStores, type KnowledgeStoreRecord } from '$lib/services/kbService';
	import { knowledgeStoreState } from '../home3/knowledge-store-state.svelte.js';
	import { listDocuments, createDocument, type DocumentSummary } from './cdm-client.js';
	import { SCHEMA_VERSION, type Document } from './types.js';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// ---------- Aesthetic tokens: "archival reading room", the palette
	// inputs-mgmt-view.svelte established for home3's document surfaces ----------
	let pageBg = $derived(darkMode ? '#0E1116' : '#F5F1E8');
	let panelBg = $derived(darkMode ? '#161A22' : '#FBF8F0');
	let panelBgAlt = $derived(darkMode ? '#1C212C' : '#F0EADB');
	let inkLine = $derived(darkMode ? '#2A3140' : '#D7CFB8');
	let inkLineSoft = $derived(darkMode ? '#1F2530' : '#E5DEC8');
	let textPrimary = $derived(darkMode ? '#EDE7D3' : '#1A1410');
	let textSecondary = $derived(darkMode ? '#B5AE94' : '#5C5345');
	let textMuted = $derived(darkMode ? '#7C7560' : '#8F8472');
	let brass = $derived(darkMode ? '#D4A24C' : '#B8801E');
	let brassFaint = $derived(darkMode ? 'rgba(212,162,76,0.12)' : 'rgba(184,128,30,0.10)');
	let teal = $derived(darkMode ? '#5DAFA8' : '#2D7B73');
	let tealFaint = $derived(darkMode ? 'rgba(93,175,168,0.14)' : 'rgba(45,123,115,0.10)');
	let crimson = $derived(darkMode ? '#C8553D' : '#A23E26');
	let crimsonFaint = $derived(darkMode ? 'rgba(200,85,61,0.18)' : 'rgba(162,62,38,0.12)');

	const fontSerif = "'Cormorant Garamond', 'Playfair Display', Georgia, serif";
	const fontMono = "'JetBrains Mono', 'IBM Plex Mono', monospace";
	const fontSans = "'Inter Tight', system-ui, sans-serif";

	// ---------- State ----------
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

	let titleFilter = $state('');
	let statusFilter = $state<'all' | 'draft' | 'published'>('all');

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

	// Filtering is client-side over the page the API already returned, not a
	// query parameter: ListDocuments has no title or status filter, and adding
	// one is API work this page does not need in order to be usable.
	let visibleDocuments = $derived.by(() => {
		const needle = titleFilter.trim().toLowerCase();
		return documents.filter((doc) => {
			if (statusFilter === 'draft' && doc.published) return false;
			if (statusFilter === 'published' && !doc.published) return false;
			if (needle && !doc.title.toLowerCase().includes(needle)) return false;
			return true;
		});
	});

	let draftCount = $derived(documents.filter((d) => !d.published).length);
	let publishedCount = $derived(documents.length - draftCount);

	const statusTabs: Array<{ id: 'all' | 'draft' | 'published'; label: string }> = [
		{ id: 'all', label: 'All' },
		{ id: 'draft', label: 'Drafts' },
		{ id: 'published', label: 'Published' }
	];

	function tabCount(id: 'all' | 'draft' | 'published'): number {
		if (id === 'draft') return draftCount;
		if (id === 'published') return publishedCount;
		return documents.length;
	}

	function formatTime(iso: string): string {
		try {
			return new Intl.DateTimeFormat('en-US', {
				dateStyle: 'medium',
				timeStyle: 'short'
			}).format(new Date(iso));
		} catch {
			return iso;
		}
	}
</script>

<div
	class="cdm-home"
	style:--bg={pageBg}
	style:--panel={panelBg}
	style:--panel-alt={panelBgAlt}
	style:--line={inkLine}
	style:--line-soft={inkLineSoft}
	style:--text={textPrimary}
	style:--text-2={textSecondary}
	style:--text-3={textMuted}
	style:--brass={brass}
	style:--brass-faint={brassFaint}
	style:--teal={teal}
	style:--teal-faint={tealFaint}
	style:--crimson={crimson}
	style:--crimson-faint={crimsonFaint}
	style:--font-serif={fontSerif}
	style:--font-mono={fontMono}
	style:--font-sans={fontSans}
>
	<!-- ── Masthead ─────────────────────────────────────────────────────── -->
	<header class="cdm-masthead">
		<div class="cdm-masthead-copy">
			<div class="cdm-kicker">
				<span class="cdm-diamond" aria-hidden="true"></span>
				CDM Editor
			</div>
			<h1>Author documents SemOS understands.</h1>
			<p>
				A knowledge editor, not a word processor: you write meaning — headings, tables,
				equations, callouts — and a Typst template decides how it looks. Everything you publish
				here feeds the same pipeline, search, and review tooling as an uploaded document.
			</p>
		</div>

		{#if activeStore}
			<div class="cdm-store-chip">
				<DatabaseIcon size={14} />
				<div>
					<span class="cdm-store-label">Knowledge store</span>
					<strong>{activeStore.ks_name}</strong>
				</div>
				<button type="button" class="cdm-link-btn" onclick={changeStore}>Change</button>
			</div>
		{/if}
	</header>

	{#if !activeStore}
		<!-- ── Store gate: nothing on this page is addressable without one ── -->
		<section class="cdm-panel cdm-gate">
			<h2>Choose a knowledge store</h2>
			<p class="cdm-gate-help">
				Documents are scoped to a knowledge store's tenant. Pick one to see its documents and
				start new ones.
			</p>

			{#if loadingStores}
				<p class="cdm-status-line">Loading knowledge stores…</p>
			{:else if storeError}
				<p class="cdm-error">{storeError}</p>
				<button type="button" class="cdm-btn cdm-btn--ghost" onclick={loadStoreOptions}>
					<RefreshCwIcon size={14} /> Retry
				</button>
			{:else if storeOptions.length === 0}
				<p class="cdm-status-line">
					No knowledge stores available. Create one under Knowledge Base first.
				</p>
			{:else}
				<ul class="cdm-store-options">
					{#each storeOptions as store (store.id)}
						<li>
							<button type="button" class="cdm-store-option" onclick={() => pickStore(store)}>
								<DatabaseIcon size={15} />
								<span class="cdm-store-option-name">{store.ks_name}</span>
								{#if store.ks_desc}
									<span class="cdm-store-option-desc">{store.ks_desc}</span>
								{/if}
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{:else}
		<!-- ── Start a document ────────────────────────────────────────────── -->
		<section class="cdm-panel cdm-create">
			<label class="cdm-field">
				<span class="cdm-field-label">New document</span>
				<input
					type="text"
					placeholder="Working title — you can change it in the editor"
					bind:value={newTitle}
					disabled={creating}
					onkeydown={(e) => e.key === 'Enter' && create()}
				/>
			</label>
			<button
				type="button"
				class="cdm-btn cdm-btn--primary"
				onclick={create}
				disabled={creating || !newTitle.trim()}
			>
				<FilePlusIcon size={15} />
				{creating ? 'Creating…' : 'Create & open'}
			</button>
		</section>
		{#if createError}
			<p class="cdm-error">{createError}</p>
		{/if}

		<!-- ── Documents ───────────────────────────────────────────────────── -->
		<section class="cdm-panel cdm-docs">
			<div class="cdm-docs-toolbar">
				<div class="cdm-tabs" role="tablist" aria-label="Filter by status">
					{#each statusTabs as tab (tab.id)}
						<button
							type="button"
							role="tab"
							aria-selected={statusFilter === tab.id}
							class="cdm-tab"
							class:is-active={statusFilter === tab.id}
							onclick={() => (statusFilter = tab.id)}
						>
							{tab.label}
							<span class="cdm-tab-count">{tabCount(tab.id)}</span>
						</button>
					{/each}
				</div>

				<div class="cdm-search">
					<SearchIcon size={14} />
					<input type="search" placeholder="Filter by title…" bind:value={titleFilter} />
				</div>
			</div>

			{#if loading}
				<p class="cdm-status-line">Loading documents…</p>
			{:else if loadError}
				<p class="cdm-error">{loadError}</p>
				<button
					type="button"
					class="cdm-btn cdm-btn--ghost"
					onclick={() => activeStore?.tenant_id && loadDocuments(activeStore.tenant_id)}
				>
					<RefreshCwIcon size={14} /> Retry
				</button>
			{:else if documents.length === 0}
				<div class="cdm-empty">
					<span class="cdm-diamond cdm-diamond--lg" aria-hidden="true"></span>
					<p>No documents in this store yet.</p>
					<p class="cdm-empty-sub">Give one a title above to start writing.</p>
				</div>
			{:else if visibleDocuments.length === 0}
				<div class="cdm-empty">
					<span class="cdm-diamond cdm-diamond--lg" aria-hidden="true"></span>
					<p>No documents match this filter.</p>
				</div>
			{:else}
				<ul class="cdm-doc-list">
					{#each visibleDocuments as doc (doc.document_key)}
						<li>
							<a class="cdm-doc-row" href={`/home3/cdm/${encodeURIComponent(doc.document_key)}`}>
								<span class="cdm-doc-title">{doc.title}</span>
								<span
									class="cdm-badge"
									class:cdm-badge--published={doc.published}
									class:cdm-badge--draft={!doc.published}
								>
									{doc.published ? 'Published' : 'Draft'}
								</span>
								<span class="cdm-doc-version">v{doc.content_version}</span>
								<span class="cdm-doc-key">{doc.document_key}</span>
								<span class="cdm-doc-time">{formatTime(doc.update_time)}</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
</div>

<style>
	.cdm-home {
		min-height: 100%;
		box-sizing: border-box;
		padding: 40px 32px 64px;
		background: var(--bg);
		color: var(--text);
		font-family: var(--font-sans);
	}

	/* ── Masthead ──────────────────────────────────────────────────────── */
	.cdm-masthead {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 32px;
		flex-wrap: wrap;
		max-width: 1100px;
		margin: 0 auto 32px;
		padding-bottom: 24px;
		border-bottom: 1px solid var(--line);
	}
	.cdm-masthead-copy {
		flex: 1 1 420px;
		min-width: 0;
	}
	.cdm-kicker {
		display: flex;
		align-items: center;
		gap: 9px;
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.22em;
		text-transform: uppercase;
		color: var(--brass);
	}
	.cdm-diamond {
		display: inline-block;
		width: 6px;
		height: 6px;
		rotate: 45deg;
		background: var(--brass);
	}
	.cdm-diamond--lg {
		width: 10px;
		height: 10px;
		opacity: 0.4;
	}
	.cdm-masthead h1 {
		margin: 14px 0 0;
		font-family: var(--font-serif);
		font-size: clamp(1.8rem, 2.4vw + 0.7rem, 2.5rem);
		font-weight: 600;
		line-height: 1.15;
		letter-spacing: -0.01em;
		color: var(--text);
	}
	.cdm-masthead p {
		margin: 12px 0 0;
		max-width: 62ch;
		font-size: 0.9rem;
		line-height: 1.8;
		color: var(--text-2);
	}

	.cdm-store-chip {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 14px;
		border: 1px solid var(--line);
		border-radius: 8px;
		background: var(--panel);
		color: var(--text-2);
	}
	.cdm-store-chip :global(svg) {
		color: var(--brass);
		flex: 0 0 auto;
	}
	.cdm-store-label {
		display: block;
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--text-3);
	}
	.cdm-store-chip strong {
		display: block;
		margin-top: 2px;
		font-size: 0.9rem;
		color: var(--text);
	}

	/* ── Panels ────────────────────────────────────────────────────────── */
	.cdm-panel {
		max-width: 1100px;
		margin: 0 auto 20px;
		padding: 22px 24px;
		border: 1px solid var(--line);
		border-radius: 10px;
		background: var(--panel);
	}

	.cdm-gate h2 {
		margin: 0;
		font-family: var(--font-serif);
		font-size: 1.35rem;
		font-weight: 600;
		color: var(--text);
	}
	.cdm-gate-help {
		margin: 8px 0 18px;
		font-size: 0.86rem;
		line-height: 1.7;
		color: var(--text-2);
	}
	.cdm-store-options {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 8px;
	}
	.cdm-store-option {
		display: flex;
		align-items: baseline;
		gap: 10px;
		width: 100%;
		padding: 12px 14px;
		border: 1px solid var(--line-soft);
		border-radius: 8px;
		background: var(--panel-alt);
		color: var(--text);
		font-family: inherit;
		font-size: 0.92rem;
		text-align: left;
		cursor: pointer;
		transition:
			border-color 0.15s ease,
			background 0.15s ease;
	}
	.cdm-store-option:hover {
		border-color: var(--brass);
		background: var(--brass-faint);
	}
	.cdm-store-option :global(svg) {
		color: var(--brass);
		flex: 0 0 auto;
		align-self: center;
	}
	.cdm-store-option-name {
		font-weight: 600;
	}
	.cdm-store-option-desc {
		font-size: 0.8rem;
		color: var(--text-3);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* ── Create row ────────────────────────────────────────────────────── */
	.cdm-create {
		display: flex;
		align-items: flex-end;
		gap: 12px;
		flex-wrap: wrap;
	}
	.cdm-field {
		flex: 1 1 320px;
		min-width: 0;
	}
	.cdm-field-label {
		display: block;
		margin-bottom: 6px;
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--brass);
	}
	.cdm-field input {
		width: 100%;
		box-sizing: border-box;
		padding: 10px 12px;
		border: 1px solid var(--line);
		border-radius: 7px;
		background: var(--panel-alt);
		color: var(--text);
		font-family: inherit;
		font-size: 0.92rem;
	}
	.cdm-field input::placeholder {
		color: var(--text-3);
	}
	.cdm-field input:focus-visible,
	.cdm-search input:focus-visible {
		outline: 2px solid var(--brass);
		outline-offset: 1px;
	}

	/* ── Buttons ───────────────────────────────────────────────────────── */
	.cdm-btn {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		padding: 10px 16px;
		border: 1px solid transparent;
		border-radius: 7px;
		font-family: inherit;
		font-size: 0.88rem;
		font-weight: 600;
		cursor: pointer;
		transition:
			opacity 0.15s ease,
			background 0.15s ease;
	}
	.cdm-btn:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
	.cdm-btn--primary {
		background: var(--brass);
		color: #16120a;
	}
	.cdm-btn--primary:not(:disabled):hover {
		opacity: 0.88;
	}
	.cdm-btn--ghost {
		margin-top: 10px;
		border-color: var(--line);
		background: transparent;
		color: var(--text-2);
	}
	.cdm-btn--ghost:hover {
		background: var(--panel-alt);
	}
	.cdm-link-btn {
		margin-left: 6px;
		border: none;
		background: none;
		padding: 0;
		font: inherit;
		font-size: 0.78rem;
		color: var(--brass);
		cursor: pointer;
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	/* ── Documents toolbar ─────────────────────────────────────────────── */
	.cdm-docs-toolbar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		flex-wrap: wrap;
		margin-bottom: 16px;
	}
	.cdm-tabs {
		display: flex;
		gap: 4px;
	}
	.cdm-tab {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		padding: 7px 13px;
		border: 1px solid transparent;
		border-radius: 999px;
		background: transparent;
		color: var(--text-2);
		font-family: inherit;
		font-size: 0.83rem;
		font-weight: 600;
		cursor: pointer;
	}
	.cdm-tab:hover {
		background: var(--panel-alt);
	}
	.cdm-tab.is-active {
		border-color: var(--brass);
		background: var(--brass-faint);
		color: var(--brass);
	}
	.cdm-tab-count {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--text-3);
	}
	.cdm-tab.is-active .cdm-tab-count {
		color: var(--brass);
	}

	.cdm-search {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 0 12px;
		border: 1px solid var(--line);
		border-radius: 7px;
		background: var(--panel-alt);
	}
	.cdm-search :global(svg) {
		color: var(--text-3);
		flex: 0 0 auto;
	}
	.cdm-search input {
		width: 210px;
		padding: 9px 0;
		border: none;
		background: none;
		color: var(--text);
		font-family: inherit;
		font-size: 0.88rem;
	}
	.cdm-search input::placeholder {
		color: var(--text-3);
	}

	/* ── Document list ─────────────────────────────────────────────────── */
	.cdm-doc-list {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: 1px solid var(--line-soft);
	}
	.cdm-doc-row {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto 3.5rem minmax(0, 14rem) 11rem;
		align-items: center;
		gap: 14px;
		padding: 13px 10px;
		border-bottom: 1px solid var(--line-soft);
		color: inherit;
		text-decoration: none;
		transition: background 0.15s ease;
	}
	.cdm-doc-row:hover {
		background: var(--panel-alt);
	}
	.cdm-doc-title {
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.cdm-doc-version,
	.cdm-doc-key,
	.cdm-doc-time {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--text-3);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.cdm-doc-time {
		text-align: right;
	}
	.cdm-badge {
		padding: 3px 9px;
		border-radius: 999px;
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.09em;
		text-transform: uppercase;
		white-space: nowrap;
	}
	.cdm-badge--draft {
		background: var(--brass-faint);
		color: var(--brass);
	}
	.cdm-badge--published {
		background: var(--teal-faint);
		color: var(--teal);
	}

	/* ── States ────────────────────────────────────────────────────────── */
	.cdm-status-line {
		margin: 0;
		font-size: 0.88rem;
		color: var(--text-2);
	}
	.cdm-error {
		max-width: 1100px;
		margin: 0 auto 12px;
		padding: 10px 14px;
		border: 1px solid var(--crimson);
		border-radius: 7px;
		background: var(--crimson-faint);
		color: var(--crimson);
		font-size: 0.86rem;
	}
	.cdm-empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		padding: 46px 20px;
		border-top: 1px solid var(--line-soft);
		border-bottom: 1px solid var(--line-soft);
	}
	.cdm-empty p {
		margin: 0;
		font-size: 0.9rem;
		color: var(--text-2);
	}
	.cdm-empty-sub {
		font-size: 0.82rem !important;
		color: var(--text-3) !important;
	}

	/* Below ~820px the metadata columns stop earning their width; the row
	   collapses to title + badge and the rest wraps underneath. */
	@media (max-width: 820px) {
		.cdm-home {
			padding: 28px 18px 48px;
		}
		.cdm-doc-row {
			grid-template-columns: minmax(0, 1fr) auto;
		}
		.cdm-doc-key {
			display: none;
		}
		.cdm-doc-version,
		.cdm-doc-time {
			text-align: left;
		}
		.cdm-search input {
			width: 100%;
		}
	}
</style>
