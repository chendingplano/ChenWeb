<script lang="ts">
	// The CDM Editor document list: the browsing half of CdmEditorShell's left
	// pane (the other half is DocumentEditor, swapped in when a document is
	// opened or created). Store selection, "New Document," and opening a
	// document are all owned by the Shell and handed down as props/callbacks
	// -- this component only renders the list for whatever store it is given.
	//
	// This replaced two things in turn: task group 8's bridging list (a bare
	// <h1> + <ul> store picker + unstyled <table>), then a designed
	// single-pane destination with its own store gate and creation form. Both
	// the store gate and the creation form moved up into CdmEditorShell (a
	// compact store dropdown and a "New Document" button that opens the
	// editor directly, deferring persistence until the author confirms Save)
	// so this component's only job is: given a store, list its documents.
	import FilePlusIcon from '@lucide/svelte/icons/file-plus';
	import SearchIcon from '@lucide/svelte/icons/search';
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import type { KnowledgeStoreRecord } from '$lib/services/kbService';
	import { listDocuments, type DocumentSummary } from './cdm-client.js';

	let {
		darkMode = true,
		activeStore,
		stores,
		onChangeStore,
		onNewDocument,
		onOpenDocument,
		refreshKey = 0
	}: {
		darkMode?: boolean;
		activeStore: KnowledgeStoreRecord;
		stores: KnowledgeStoreRecord[];
		onChangeStore: (store: KnowledgeStoreRecord) => void;
		onNewDocument: () => void;
		onOpenDocument: (key: string) => void;
		// Bump to force a reload without changing anything else -- e.g. after
		// a newly-created document's first save, so the list picks it up.
		refreshKey?: number;
	} = $props();

	// ---------- Aesthetic tokens: "archival reading room", the palette
	// inputs-mgmt-view.svelte established for home3's document surfaces ----------
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
	let documents = $state<DocumentSummary[]>([]);
	let loading = $state(false);
	let loadError = $state('');

	let titleFilter = $state('');
	let statusFilter = $state<'all' | 'draft' | 'published'>('all');

	let searchOpen = $state(false);
	let searchDraft = $state('');

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

	// Re-derives from activeStore.tenant_id (switching stores refreshes the
	// list) and refreshKey (an explicit host-forced reload), not a one-time
	// onMount.
	$effect(() => {
		const tenantId = activeStore.tenant_id;
		void refreshKey;
		if (tenantId) loadDocuments(tenantId);
	});

	function openSearch() {
		searchDraft = titleFilter;
		searchOpen = true;
	}

	function applySearch() {
		titleFilter = searchDraft.trim();
		searchOpen = false;
	}

	function clearSearch() {
		searchDraft = '';
		titleFilter = '';
		searchOpen = false;
	}

	function handleStoreSelect(e: Event) {
		const id = Number((e.target as HTMLSelectElement).value);
		const next = stores.find((s) => s.id === id);
		if (next) onChangeStore(next);
	}

	// The row stays a real <a href> (right-click "copy link", ctrl/cmd/middle
	// click "open in new tab" all keep working) but a plain left click is
	// intercepted: opening a document is in-shell state, not a navigation --
	// CdmEditorShell has no per-document route when embedded in /development,
	// and even on /home3/cdm a full navigation would remount the shell and
	// lose the split-pane/preview state for no reason.
	function handleRowClick(e: MouseEvent, key: string) {
		if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) {
			return;
		}
		e.preventDefault();
		onOpenDocument(key);
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
	class="cdm-list"
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
				equations, callouts — and a Typst template decides how it looks.
			</p>
		</div>

		<div class="cdm-masthead-controls">
			<label class="cdm-store-select">
				<span class="cdm-store-select-label">
					<DatabaseIcon size={12} />
					Knowledge store
				</span>
				<div class="cdm-store-select-input">
					<select value={activeStore.id} onchange={handleStoreSelect}>
						{#each stores as store (store.id)}
							<option value={store.id}>{store.ks_name}</option>
						{/each}
					</select>
					<ChevronDownIcon size={14} />
				</div>
			</label>
			<button type="button" class="cdm-btn cdm-btn--primary" onclick={onNewDocument}>
				<FilePlusIcon size={15} />
				New Document
			</button>
		</div>
	</header>

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

			<div class="cdm-search-wrap">
				<button
					type="button"
					class="cdm-btn cdm-btn--ghost cdm-search-btn"
					onclick={() => (searchOpen ? (searchOpen = false) : openSearch())}
				>
					<SearchIcon size={14} />
					Search
					{#if titleFilter}<span class="cdm-search-dot" aria-hidden="true"></span>{/if}
				</button>
				{#if searchOpen}
					<div class="cdm-search-popover">
						<input
							type="text"
							placeholder="Title contains…"
							bind:value={searchDraft}
							onkeydown={(e) => {
								if (e.key === 'Enter') applySearch();
								if (e.key === 'Escape') searchOpen = false;
							}}
						/>
						<div class="cdm-search-popover-actions">
							<button type="button" class="cdm-btn cdm-btn--ghost" onclick={clearSearch}>
								Clear
							</button>
							<button type="button" class="cdm-btn cdm-btn--primary" onclick={applySearch}>
								Search
							</button>
						</div>
					</div>
				{/if}
			</div>
		</div>

		{#if loading}
			<p class="cdm-status-line">Loading documents…</p>
		{:else if loadError}
			<p class="cdm-error">{loadError}</p>
			<button
				type="button"
				class="cdm-btn cdm-btn--ghost"
				onclick={() => activeStore.tenant_id && loadDocuments(activeStore.tenant_id)}
			>
				<RefreshCwIcon size={14} /> Retry
			</button>
		{:else if documents.length === 0}
			<div class="cdm-empty">
				<span class="cdm-diamond cdm-diamond--lg" aria-hidden="true"></span>
				<p>No documents in this store yet.</p>
				<p class="cdm-empty-sub">Click “New Document” above to start writing.</p>
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
						<!-- The whole row is the link, so the "Edit"/"Open" affordance below is
						     a styled span rather than a nested button or second anchor: it would
						     otherwise be either invalid HTML or a second tab stop to the same
						     destination. -->
						<a
							class="cdm-doc-row"
							href={`/home3/cdm/${encodeURIComponent(doc.document_key)}`}
							onclick={(e) => handleRowClick(e, doc.document_key)}
						>
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
							<!-- A published document is frozen (D8), so it opens read-only --
							     "Open" rather than "Edit" says so before the click. -->
							<span class="cdm-doc-action">
								{doc.published ? 'Open' : 'Edit'}
								<ArrowRightIcon size={13} />
							</span>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
</div>

<style>
	.cdm-list {
		box-sizing: border-box;
		padding: 28px 28px 40px;
		color: var(--text);
		font-family: var(--font-sans);
	}

	/* ── Masthead ──────────────────────────────────────────────────────── */
	.cdm-masthead {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 24px;
		flex-wrap: wrap;
		margin: 0 0 24px;
		padding-bottom: 20px;
		border-bottom: 1px solid var(--line);
	}
	.cdm-masthead-copy {
		flex: 1 1 320px;
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
		margin: 12px 0 0;
		font-family: var(--font-serif);
		font-size: clamp(1.4rem, 1.6vw + 0.7rem, 1.9rem);
		font-weight: 600;
		line-height: 1.2;
		letter-spacing: -0.01em;
		color: var(--text);
	}
	.cdm-masthead p {
		margin: 8px 0 0;
		max-width: 56ch;
		font-size: 0.85rem;
		line-height: 1.7;
		color: var(--text-2);
	}

	/* ── Masthead controls: knowledge store + new document ──────────────── */
	.cdm-masthead-controls {
		display: flex;
		align-items: flex-end;
		gap: 10px;
		flex: 0 0 auto;
	}
	.cdm-store-select {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.cdm-store-select-label {
		display: flex;
		align-items: center;
		gap: 5px;
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--text-3);
	}
	.cdm-store-select-label :global(svg) {
		color: var(--brass);
	}
	.cdm-store-select-input {
		position: relative;
		display: flex;
		align-items: center;
	}
	.cdm-store-select-input select {
		appearance: none;
		width: 100%;
		min-width: 180px;
		padding: 8px 30px 8px 12px;
		border: 1px solid var(--line);
		border-radius: 7px;
		background: var(--panel);
		color: var(--text);
		font-family: inherit;
		font-size: 0.86rem;
		cursor: pointer;
	}
	.cdm-store-select-input select:focus-visible {
		outline: 2px solid var(--brass);
		outline-offset: 1px;
	}
	.cdm-store-select-input :global(svg) {
		position: absolute;
		right: 10px;
		color: var(--text-3);
		pointer-events: none;
	}

	/* ── Panels ────────────────────────────────────────────────────────── */
	.cdm-panel {
		padding: 22px 24px;
		border: 1px solid var(--line);
		border-radius: 10px;
		background: var(--panel);
	}

	/* ── Buttons ───────────────────────────────────────────────────────── */
	.cdm-btn {
		display: inline-flex;
		align-items: center;
		gap: 7px;
		padding: 8px 14px;
		border: 1px solid transparent;
		border-radius: 7px;
		font-family: inherit;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
		white-space: nowrap;
		transition:
			opacity 0.15s ease,
			background 0.15s ease,
			border-color 0.15s ease;
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
		border-color: var(--line);
		background: transparent;
		color: var(--text-2);
	}
	.cdm-btn--ghost:hover {
		background: var(--panel-alt);
		border-color: var(--brass);
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

	/* ── Search: a button matching kb-input-record-browser.svelte's own
	   "ghost search-btn" affordance (pill, icon + label, hover accent border),
	   opening a small popover instead of that component's full search dialog
	   -- CDM's list only needs a title filter, not kb.inputs' query surface. */
	.cdm-search-wrap {
		position: relative;
	}
	.cdm-search-btn {
		position: relative;
	}
	.cdm-search-dot {
		display: inline-block;
		width: 6px;
		height: 6px;
		border-radius: 999px;
		background: var(--brass);
	}
	.cdm-search-popover {
		position: absolute;
		top: calc(100% + 8px);
		right: 0;
		z-index: 10;
		width: 260px;
		padding: 12px;
		border: 1px solid var(--line);
		border-radius: 9px;
		background: var(--panel);
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
	}
	.cdm-search-popover input {
		width: 100%;
		box-sizing: border-box;
		padding: 8px 10px;
		border: 1px solid var(--line);
		border-radius: 6px;
		background: var(--panel-alt);
		color: var(--text);
		font-family: inherit;
		font-size: 0.86rem;
	}
	.cdm-search-popover input:focus-visible {
		outline: 2px solid var(--brass);
		outline-offset: 1px;
	}
	.cdm-search-popover-actions {
		display: flex;
		justify-content: flex-end;
		gap: 6px;
		margin-top: 10px;
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
		grid-template-columns: minmax(0, 1fr) auto 3rem minmax(0, 10rem) 8.5rem auto;
		align-items: center;
		gap: 12px;
		padding: 12px 10px;
		border-bottom: 1px solid var(--line-soft);
		color: inherit;
		text-decoration: none;
		transition: background 0.15s ease;
	}
	.cdm-doc-row:hover,
	.cdm-doc-row:focus-visible {
		background: var(--panel-alt);
	}
	.cdm-doc-row:focus-visible {
		outline: 2px solid var(--brass);
		outline-offset: -2px;
	}
	/* The title carries the link colour so the row reads as navigable at rest,
	   not only under the cursor. */
	.cdm-doc-title {
		font-size: 0.92rem;
		font-weight: 600;
		color: var(--brass);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.cdm-doc-row:hover .cdm-doc-title,
	.cdm-doc-row:focus-visible .cdm-doc-title {
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.cdm-doc-action {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		padding: 4px 10px;
		border: 1px solid var(--line);
		border-radius: 6px;
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--text-2);
		white-space: nowrap;
		transition:
			background 0.15s ease,
			border-color 0.15s ease,
			color 0.15s ease;
	}
	.cdm-doc-action :global(svg) {
		transition: transform 0.15s ease;
	}
	.cdm-doc-row:hover .cdm-doc-action,
	.cdm-doc-row:focus-visible .cdm-doc-action {
		border-color: var(--brass);
		background: var(--brass);
		color: #16120a;
	}
	.cdm-doc-row:hover .cdm-doc-action :global(svg) {
		transform: translateX(2px);
	}
	.cdm-doc-version,
	.cdm-doc-key,
	.cdm-doc-time {
		font-family: var(--font-mono);
		font-size: 0.72rem;
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
		font-size: 0.66rem;
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
		margin: 0 0 12px;
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
		.cdm-list {
			padding: 20px 16px 32px;
		}
		.cdm-doc-row {
			grid-template-columns: minmax(0, 1fr) auto auto;
			gap: 6px 12px;
		}
		.cdm-doc-key {
			display: none;
		}
		.cdm-doc-version,
		.cdm-doc-time {
			text-align: left;
		}
		.cdm-masthead-controls {
			width: 100%;
			justify-content: space-between;
		}
	}
</style>
