<script lang="ts">
	// The CDM Editor's top-level shell: a persistent two-pane frame (left:
	// browse the document list, or edit one; right: a docked live preview),
	// with a draggable divider between them. This is the ONE CDM Editor
	// design, shared by every host that reaches it:
	//   - /home3/cdm            (routed=true,  no initialDocumentKey)
	//   - /home3/cdm/[key]      (routed=true,  initialDocumentKey={key})
	//   - /development, Tools > CDM Editor (routed=false, embedded)
	//
	// Store selection is never a separate gate screen the author must clear
	// before doing anything (the old DocumentListView's "Choose a knowledge
	// store" step): this component auto-selects a store on mount if none is
	// already active, and the list's own compact dropdown is how it is
	// changed afterward. The only "no store" state left is the true edge
	// case of zero stores existing at all.
	//
	// "New Document" does not call the API. It opens a fresh, empty, purely
	// in-memory Document (document_key: '') straight into the editor pane --
	// DocumentEditor's own Save action is what defers to a confirmation
	// dialog naming the target knowledge store and only then calls
	// createDocument, the first time a new document is saved.
	import { onMount } from 'svelte';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import { listKnowledgeStores, type KnowledgeStoreRecord } from '$lib/services/kbService';
	import { knowledgeStoreState } from '../home3/knowledge-store-state.svelte.js';
	import { getDocument, CdmApiError, type CreateTarget } from './cdm-client.js';
	import { SCHEMA_VERSION, type Document } from './types.js';
	import DocumentListView from './DocumentListView.svelte';
	import DocumentEditor from './DocumentEditor.svelte';

	let {
		darkMode = true,
		initialDocumentKey = undefined,
		// True for the two /home3/cdm routes (a real URL exists to go back
		// to); false for the /development embed (nav-rail selection is local
		// state with no per-item URL, so there is nothing to navigate back
		// to -- "back" there is purely mode = 'list').
		routed = false,
		onBack
	}: {
		darkMode?: boolean;
		initialDocumentKey?: string;
		routed?: boolean;
		// Called in addition to the local list/editor state reset, only
		// meaningful when routed: /home3/cdm/[key] passes
		// () => goto('/home3/cdm') so the URL and the visible pane agree
		// (refreshing while "back" is showing the list must not jump back
		// into the editor because the URL still says .../[key]).
		onBack?: () => void;
	} = $props();

	// ---------- Aesthetic tokens: "archival reading room", the palette
	// inputs-mgmt-view.svelte established for home3's document surfaces ----------
	let panelBg = $derived(darkMode ? '#161A22' : '#FBF8F0');
	let inkLine = $derived(darkMode ? '#2A3140' : '#D7CFB8');
	let textPrimary = $derived(darkMode ? '#EDE7D3' : '#1A1410');
	let textSecondary = $derived(darkMode ? '#B5AE94' : '#5C5345');
	let textMuted = $derived(darkMode ? '#7C7560' : '#8F8472');
	let brass = $derived(darkMode ? '#D4A24C' : '#B8801E');
	let crimson = $derived(darkMode ? '#C8553D' : '#A23E26');
	let crimsonFaint = $derived(darkMode ? 'rgba(200,85,61,0.18)' : 'rgba(162,62,38,0.12)');

	// ---------- Knowledge store ----------
	let stores = $state<KnowledgeStoreRecord[]>([]);
	let storesLoading = $state(false);
	let storesError = $state('');
	let activeStore = $derived(knowledgeStoreState.activeStore);

	async function loadStores() {
		storesLoading = true;
		storesError = '';
		try {
			const res = await listKnowledgeStores();
			stores = res.results ?? [];
			if (!knowledgeStoreState.activeStore && stores.length > 0) {
				knowledgeStoreState.setActiveStore(stores[0]);
			}
		} catch (err) {
			storesError = err instanceof Error ? err.message : 'Failed to load knowledge stores.';
		} finally {
			storesLoading = false;
		}
	}

	onMount(() => {
		loadStores();
	});

	function changeStore(store: KnowledgeStoreRecord) {
		knowledgeStoreState.setActiveStore(store);
	}

	let createTarget: CreateTarget | null = $derived(
		activeStore
			? {
					tenantId: activeStore.tenant_id ?? '',
					ksStoreId: activeStore.id,
					ksName: activeStore.ks_name
				}
			: null
	);

	// ---------- List / editor mode ----------
	let mode = $state<'list' | 'editor'>('list');
	// $state.raw, not $state: this component only ever replaces the whole
	// reference (opening a new document, or a fresh unsaved one), never
	// mutates a field inside it -- DocumentEditor owns it from here on and
	// takes its own private copy via structuredClone. Plain $state deep-
	// proxies the object, and structuredClone cannot clone a Svelte 5
	// reactive proxy ("could not be cloned") -- the same crash
	// DocumentEditorPage.svelte was built to avoid (see ADR
	// 2026072603's 2026/07/27 Change Log) and that this component's own
	// removal reintroduced until this fix.
	let editingDocument = $state.raw<Document | null>(null);
	let editorLoadState = $state<'idle' | 'loading' | 'error'>('idle');
	let editorLoadError = $state('');
	let listRefreshKey = $state(0);

	// Preview dock (right pane), bound into DocumentEditor -- see that
	// component's own doc comment on why these are bindable rather than
	// callback props.
	let previewPages = $state<string[] | null>(null);
	let previewVersion = $state<number | null>(null);
	let previewLoading = $state(false);

	function resetPreview() {
		previewPages = null;
		previewVersion = null;
		previewLoading = false;
	}

	function openNewDocument() {
		editingDocument = {
			document_key: '',
			title: '',
			language: 'en',
			schema_version: SCHEMA_VERSION,
			content_version: 0,
			metadata: {},
			blocks: []
		};
		editorLoadState = 'idle';
		editorLoadError = '';
		resetPreview();
		mode = 'editor';
	}

	async function openExistingDocument(key: string) {
		mode = 'editor';
		editingDocument = null;
		editorLoadState = 'loading';
		editorLoadError = '';
		resetPreview();
		try {
			editingDocument = await getDocument(key);
			editorLoadState = 'idle';
		} catch (err) {
			editorLoadState = 'error';
			editorLoadError = err instanceof CdmApiError ? err.message : String(err);
		}
	}

	function backToList() {
		mode = 'list';
		editingDocument = null;
		editorLoadState = 'idle';
		editorLoadError = '';
		resetPreview();
		listRefreshKey++;
		onBack?.();
	}

	// Fires once per distinct initialDocumentKey (a deep link from
	// /home3/cdm/[key]), not on every render -- tracked against the key
	// actually opened so re-running this effect for an unrelated reason
	// doesn't re-fetch the same document.
	let openedInitialKey = $state<string | null>(null);
	$effect(() => {
		const key = initialDocumentKey;
		if (key && key !== openedInitialKey) {
			openedInitialKey = key;
			openExistingDocument(key);
		}
	});

	function handleCreated() {
		// Stay in the editor after a first save -- the author was just in the
		// middle of writing it. The list picks up the new document via
		// listRefreshKey the next time "All documents" is clicked.
		listRefreshKey++;
	}

	// ---------- Split-pane divider, the same drag mechanic
	// dashboard.svelte's rail/shelf resize already uses ----------
	const LEFT_WIDTH_DEFAULT = 940;
	const LEFT_WIDTH_MIN = 380;
	const LEFT_WIDTH_MAX = 1500;
	let leftWidth = $state(LEFT_WIDTH_DEFAULT);
	let dragStartX = 0;
	let dragStartWidth = 0;

	function startDrag(e: MouseEvent) {
		dragStartX = e.clientX;
		dragStartWidth = leftWidth;
		document.addEventListener('mousemove', onDragMove);
		document.addEventListener('mouseup', onDragEnd);
		e.preventDefault();
	}
	function onDragMove(e: MouseEvent) {
		const delta = e.clientX - dragStartX;
		leftWidth = Math.max(LEFT_WIDTH_MIN, Math.min(LEFT_WIDTH_MAX, dragStartWidth + delta));
	}
	function onDragEnd() {
		document.removeEventListener('mousemove', onDragMove);
		document.removeEventListener('mouseup', onDragEnd);
	}
</script>

<div
	class="cdm-shell"
	style:--cdm-surface={panelBg}
	style:--cdm-muted={textMuted}
	style:--shell-line={inkLine}
	style:--shell-text={textPrimary}
	style:--shell-text-2={textSecondary}
	style:--shell-brass={brass}
	style:--shell-crimson={crimson}
	style:--shell-crimson-faint={crimsonFaint}
>
	{#if storesLoading}
		<p class="cdm-shell-status">Loading knowledge stores…</p>
	{:else if storesError}
		<p class="cdm-shell-error">{storesError}</p>
		<button type="button" class="cdm-shell-retry" onclick={loadStores}>
			<RefreshCwIcon size={14} /> Retry
		</button>
	{:else if !activeStore}
		<p class="cdm-shell-status">
			No knowledge stores available. Create one under Knowledge Base first.
		</p>
	{:else}
		<div class="cdm-shell-split">
			<div class="cdm-shell-left" style:width="{leftWidth}px">
				{#if mode === 'list'}
					<DocumentListView
						{darkMode}
						{activeStore}
						{stores}
						onChangeStore={changeStore}
						onNewDocument={openNewDocument}
						onOpenDocument={openExistingDocument}
						refreshKey={listRefreshKey}
					/>
				{:else if editorLoadState === 'loading'}
					<div class="cdm-shell-editor-status">
						<p>Loading document…</p>
					</div>
				{:else if editorLoadState === 'error'}
					<div class="cdm-shell-editor-status">
						<p class="cdm-shell-error">{editorLoadError}</p>
						<button type="button" class="cdm-shell-retry" onclick={backToList}>
							<ChevronLeftIcon size={14} /> Back to documents
						</button>
					</div>
				{:else if editingDocument}
					<nav class="cdm-shell-crumbs">
						<button type="button" onclick={backToList}>
							<ChevronLeftIcon size={14} /> All documents
						</button>
						{#if editingDocument.document_key}
							<span class="cdm-shell-key">{editingDocument.document_key}</span>
						{/if}
					</nav>
					<DocumentEditor
						initialDocument={editingDocument}
						{createTarget}
						onCreated={handleCreated}
						bind:previewPages
						bind:previewVersion
						bind:previewLoading
					/>
				{/if}
			</div>

			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
			<div class="cdm-shell-divider" onmousedown={startDrag} title="Drag to resize"></div>

			<div class="cdm-shell-right">
				{#if previewLoading}
					<div class="cdm-shell-preview-status">Rendering preview…</div>
				{:else if previewPages && previewPages.length > 0}
					<div class="cdm-shell-preview-header">Preview — version {previewVersion}</div>
					<div class="cdm-shell-preview-pages">
						{#each previewPages as pageSvg, i (i)}
							<div class="cdm-shell-preview-page">{@html pageSvg}</div>
						{/each}
					</div>
				{:else if mode === 'editor'}
					<div class="cdm-shell-preview-status">
						Click “Preview” above to render this document here.
					</div>
				{:else}
					<div class="cdm-shell-preview-status">
						Open or create a document, then click “Preview” to see it rendered here.
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.cdm-shell {
		height: 100%;
		box-sizing: border-box;
		color: var(--shell-text);
		font-family: 'Inter Tight', system-ui, sans-serif;
	}
	.cdm-shell-status {
		padding: 24px 28px;
		color: var(--shell-text-2);
		font-size: 0.9rem;
	}
	.cdm-shell-error {
		margin: 0;
		padding: 10px 14px;
		border: 1px solid var(--shell-crimson);
		border-radius: 7px;
		background: var(--shell-crimson-faint);
		color: var(--shell-crimson);
		font-size: 0.86rem;
	}
	.cdm-shell-retry {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		margin: 10px 28px;
		padding: 7px 13px;
		border: 1px solid var(--shell-line);
		border-radius: 7px;
		background: transparent;
		color: var(--shell-text-2);
		font: inherit;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}

	.cdm-shell-split {
		display: flex;
		height: 100%;
		min-height: 0;
	}

	.cdm-shell-left {
		flex: 0 0 auto;
		min-width: 0;
		overflow-y: auto;
	}

	.cdm-shell-divider {
		flex: 0 0 auto;
		width: 6px;
		cursor: col-resize;
		background: var(--shell-line);
		transition: background 0.15s ease;
	}
	.cdm-shell-divider:hover,
	.cdm-shell-divider:focus-visible {
		background: var(--shell-brass);
		outline: none;
	}

	.cdm-shell-right {
		flex: 1 1 auto;
		min-width: 0;
		overflow-y: auto;
		background: var(--cdm-surface);
	}
	.cdm-shell-preview-status {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		padding: 24px;
		text-align: center;
		color: var(--cdm-muted);
		font-size: 0.88rem;
	}
	.cdm-shell-preview-header {
		position: sticky;
		top: 0;
		padding: 10px 16px;
		border-bottom: 1px solid var(--shell-line);
		background: var(--cdm-surface);
		font-size: 0.82rem;
		color: var(--shell-text-2);
		z-index: 2;
	}
	.cdm-shell-preview-pages {
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		align-items: center;
		width: 100%;
		min-width: 0;
		box-sizing: border-box;
	}
	.cdm-shell-preview-page {
		width: min(100%, 816px);
		min-width: 0;
	}
	.cdm-shell-preview-page :global(svg) {
		display: block;
		width: 100%;
		height: auto;
		max-width: 100%;
		box-shadow: 0 1px 6px rgba(0, 0, 0, 0.25);
	}

	.cdm-shell-editor-status {
		padding: 24px 28px;
		display: flex;
		flex-direction: column;
		gap: 12px;
		align-items: flex-start;
	}

	.cdm-shell-crumbs {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 16px 20px 0;
	}
	.cdm-shell-crumbs button {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		border: none;
		background: none;
		padding: 0;
		font: inherit;
		font-size: 0.82rem;
		font-weight: 600;
		color: var(--shell-brass);
		cursor: pointer;
	}
	.cdm-shell-crumbs button:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}
	.cdm-shell-key {
		font-family: 'JetBrains Mono', ui-monospace, monospace;
		font-size: 0.72rem;
		color: var(--cdm-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* DocumentEditor's own header/content padding assumes it sits in a
	   max-width reading column (its pre-shell hosts); here it fills the left
	   pane instead, so give it the same horizontal breathing room directly. */
	.cdm-shell-left :global(.cdm-document-editor) {
		padding: 12px 20px 32px;
	}
	.cdm-shell-left :global(.cdm-list) {
		height: 100%;
	}

	@media (max-width: 900px) {
		.cdm-shell-split {
			flex-direction: column;
		}
		.cdm-shell-left {
			width: auto !important;
			flex: 1 1 auto;
		}
		.cdm-shell-divider {
			width: auto;
			height: 6px;
			cursor: row-resize;
		}
		.cdm-shell-right {
			min-width: 0;
			min-height: 200px;
		}
	}
</style>
