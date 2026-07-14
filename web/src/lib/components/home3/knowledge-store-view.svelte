<script lang="ts">
	import { onMount } from 'svelte';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import PaletteIcon from '@lucide/svelte/icons/palette';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import CircleCheckBigIcon from '@lucide/svelte/icons/circle-check-big';
	import type {
		CreateKnowledgeStorePayload,
		KnowledgeStoreRecord,
		UpdateKnowledgeStorePayload
	} from '$lib/services/kbService';
	import {
		createKnowledgeStore,
		deleteKnowledgeStore,
		getDefaultKnowledgeStore,
		listKnowledgeStores,
		updateKnowledgeStore
	} from '$lib/services/kbService';
	import { knowledgeStoreState } from './knowledge-store-state.svelte';

	let { darkMode = true }: { darkMode: boolean } = $props();

	type DialogMode = 'create' | 'edit' | 'delete' | 'style' | null;
	type CardStylePreset = 'blush' | 'graphite' | 'neon';

	let stores = $state<KnowledgeStoreRecord[]>([]);
	let loading = $state(false);
	let saving = $state(false);
	let error = $state('');
	let dialogMode = $state<DialogMode>(null);
	let dialogError = $state('');
	let editingStore = $state<KnowledgeStoreRecord | null>(null);

	let formName = $state('');
	let formType = $state('');
	let formDescription = $state('');
	let formSyncMode = $state<'auto' | 'manual'>('manual');
	let formStatus = $state<'active' | 'suspended' | 'inactive'>('active');
	let formSources = $state('');
	let formNotes = $state('');
	let cardStylePreset = $state<CardStylePreset>('neon');
	let cardBackgroundColor = $state('#7c4dff');
	let draftCardStylePreset = $state<CardStylePreset>('neon');
	let draftCardBackgroundColor = $state('#7c4dff');
	let browserReady = $state(false);

	let pageBg = $derived(darkMode ? '#111827' : '#f5efe4');
	let shellBg = $derived(darkMode ? '#161f2b' : '#fcf8f2');
	let cardBg = $derived(darkMode ? 'rgba(24, 35, 52, 0.88)' : 'rgba(255, 255, 255, 0.9)');
	let cardBgRaised = $derived(darkMode ? 'rgba(31, 47, 71, 0.96)' : 'rgba(255, 255, 255, 0.98)');
	let borderColor = $derived(darkMode ? '#304663' : '#d7dee8');
	let borderSoft = $derived(darkMode ? '#23364f' : '#e7ecf3');
	let textPrimary = $derived(darkMode ? '#f7f5f1' : '#18212f');
	let textSecondary = $derived(darkMode ? '#c4cfdf' : '#5d697c');
	let textMuted = $derived(darkMode ? '#8ea2bb' : '#8693a8');
	let accent = $derived(darkMode ? '#ff9b54' : '#ff8b38');
	let accentSoft = $derived(darkMode ? 'rgba(255,155,84,0.14)' : 'rgba(255,139,56,0.12)');
	let successSoft = $derived(darkMode ? 'rgba(71,182,153,0.16)' : 'rgba(46,155,126,0.14)');
	let danger = $derived(darkMode ? '#ff7d6b' : '#e85d47');
	let dangerSoft = $derived(darkMode ? 'rgba(255,125,107,0.14)' : 'rgba(232,93,71,0.12)');
	let neonSlab = $derived(darkMode ? 'rgba(7,10,18,0.96)' : 'rgba(255,255,255,0.94)');
	let neonSlabSoft = $derived(darkMode ? 'rgba(7,10,18,0.94)' : 'rgba(247,249,253,0.92)');
	let neonSlabRaised = $derived(darkMode ? 'rgba(7,10,18,0.98)' : 'rgba(255,255,255,0.97)');
	let neonSlabRaisedSoft = $derived(darkMode ? 'rgba(7,10,18,0.95)' : 'rgba(247,249,253,0.95)');
	let neonPillText = $derived(darkMode ? 'white 20%' : 'black 25%');

	const emptyText = 'No active knowledge store selected yet. Click a card to make it the active knowledge store for this page.';
	const CARD_STYLE_STORAGE_KEY = 'knowledge-store-card-style';
	const CARD_COLOR_STORAGE_KEY = 'knowledge-store-card-bg-color';

	const cardStyleOptions: Array<{
		id: CardStylePreset;
		name: string;
		description: string;
		defaultColor: string;
	}> = [
		{
			id: 'blush',
			name: 'Blush Gradient',
			description: 'Soft pastel card with a bright top glow.',
			defaultColor: '#ff8fb1'
		},
		{
			id: 'graphite',
			name: 'Graphite',
			description: 'Minimal dark slab with a subtle industrial rim.',
			defaultColor: '#5d6675'
		},
		{
			id: 'neon',
			name: 'Neon Glow',
			description: 'High-contrast panel with an electric halo edge.',
			defaultColor: '#7c4dff'
		}
	];

	function normalizeSources(value: unknown): string[] {
		if (!Array.isArray(value)) return [];
		return value.map((item) => String(item).trim()).filter(Boolean);
	}

	function formatDate(value?: string): string {
		if (!value) return 'No timestamp';
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return date.toLocaleString();
	}

	function sourceSummary(store: KnowledgeStoreRecord): string {
		const count = normalizeSources(store.ks_sources).length;
		if (count === 0) return 'No sources configured';
		if (count === 1) return '1 source configured';
		return `${count} sources configured`;
	}

	function sourcePreview(store: KnowledgeStoreRecord): string[] {
		return normalizeSources(store.ks_sources).slice(0, 3);
	}

	function statusLabel(store: KnowledgeStoreRecord): string {
		return store.status?.trim() || 'active';
	}

	function syncModeLabel(store: KnowledgeStoreRecord): string {
		return store.ks_sync_mode?.trim() || 'manual';
	}

	function openCreateDialog() {
		dialogMode = 'create';
		dialogError = '';
		editingStore = null;
		formName = '';
		formType = '';
		formDescription = '';
		formSyncMode = 'manual';
		formStatus = 'active';
		formSources = '';
		formNotes = '';
	}

	function openEditDialog(store: KnowledgeStoreRecord) {
		dialogMode = 'edit';
		dialogError = '';
		editingStore = store;
		formName = store.ks_name ?? '';
		formType = store.ks_type ?? '';
		formDescription = store.ks_desc ?? '';
		formSyncMode = (store.ks_sync_mode === 'auto' ? 'auto' : 'manual') as 'auto' | 'manual';
		formStatus = ((store.status as 'active' | 'suspended' | 'inactive') || 'active');
		formSources = normalizeSources(store.ks_sources).join('\n');
		formNotes = store.notes ?? '';
	}

	function openDeleteDialog(store: KnowledgeStoreRecord) {
		dialogMode = 'delete';
		dialogError = '';
		editingStore = store;
	}

	function openStyleDialog() {
		dialogMode = 'style';
		dialogError = '';
		draftCardStylePreset = cardStylePreset;
		draftCardBackgroundColor = cardBackgroundColor;
	}

	function closeDialog() {
		dialogMode = null;
		dialogError = '';
		editingStore = null;
	}

	function persistCardAppearance() {
		if (!browserReady) return;
		localStorage.setItem(CARD_STYLE_STORAGE_KEY, cardStylePreset);
		localStorage.setItem(CARD_COLOR_STORAGE_KEY, cardBackgroundColor);
	}

	function selectDraftCardStylePreset(style: CardStylePreset) {
		draftCardStylePreset = style;
		const option = cardStyleOptions.find((entry) => entry.id === style);
		if (option) draftCardBackgroundColor = option.defaultColor;
	}

	function handleDraftCardColorChange(value: string) {
		draftCardBackgroundColor = value;
	}

	function confirmStyleDialog() {
		cardStylePreset = draftCardStylePreset;
		cardBackgroundColor = draftCardBackgroundColor;
		persistCardAppearance();
		closeDialog();
	}

	function parseSources(raw: string): string[] {
		return raw
			.split(/\r?\n|,/)
			.map((item) => item.trim())
			.filter(Boolean);
	}

	function buildPayload(): CreateKnowledgeStorePayload | UpdateKnowledgeStorePayload {
		return {
			ks_name: formName.trim(),
			ks_type: formType.trim() || null,
			ks_desc: formDescription.trim() || null,
			ks_sync_mode: formSyncMode,
			ks_sources: parseSources(formSources),
			status: formStatus,
			notes: formNotes.trim() || null
		};
	}

	async function loadStores() {
		loading = true;
		error = '';
		try {
			const result = await listKnowledgeStores();
			stores = result.results ?? [];
			knowledgeStoreState.syncActiveStore(stores);
			await applyDefaultStore();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load knowledge stores';
		} finally {
			loading = false;
		}
	}

	// applyDefaultStore preselects the store named by
	// [frontend].default_knowledge_store, but only when nothing is active yet so a
	// user's manual choice is never overridden. When the config is missing or
	// resolves to none/several stores, the backend returns status=false and no
	// store is selected — the user picks one manually.
	async function applyDefaultStore() {
		if (knowledgeStoreState.activeStore) return;
		const result = await getDefaultKnowledgeStore();
		if (!result.status || !result.record) return;
		const match = stores.find((store) => store.id === result.record?.id);
		if (match) knowledgeStoreState.setActiveStore(match);
	}

	function selectStore(store: KnowledgeStoreRecord) {
		knowledgeStoreState.setActiveStore(store);
	}

	async function saveDialog() {
		if (!formName.trim()) {
			dialogError = 'Knowledge store name is required.';
			return;
		}
		saving = true;
		dialogError = '';
		try {
			if (dialogMode === 'create') {
				const created = await createKnowledgeStore(buildPayload() as CreateKnowledgeStorePayload);
				stores = [created.record, ...stores];
			} else if (dialogMode === 'edit' && editingStore) {
				const updated = await updateKnowledgeStore(editingStore.id, buildPayload());
				stores = stores.map((store) => (store.id === editingStore?.id ? updated.record : store));
				if (knowledgeStoreState.activeStoreId === editingStore.id) {
					knowledgeStoreState.setActiveStore(updated.record);
				}
			}
			knowledgeStoreState.syncActiveStore(stores);
			closeDialog();
		} catch (err) {
			dialogError = err instanceof Error ? err.message : 'Failed to save knowledge store';
		} finally {
			saving = false;
		}
	}

	async function confirmDelete() {
		const target = editingStore;
		if (!target) return;
		saving = true;
		dialogError = '';
		try {
			await deleteKnowledgeStore(target.id);
			stores = stores.filter((store) => store.id !== target.id);
			if (knowledgeStoreState.activeStoreId === target.id) {
				knowledgeStoreState.setActiveStore(null);
			}
			closeDialog();
		} catch (err) {
			dialogError = err instanceof Error ? err.message : 'Failed to delete knowledge store';
		} finally {
			saving = false;
		}
	}

	onMount(() => {
		browserReady = true;
		const storedStyle = localStorage.getItem(CARD_STYLE_STORAGE_KEY);
		const storedColor = localStorage.getItem(CARD_COLOR_STORAGE_KEY);
		if (storedStyle === 'blush' || storedStyle === 'graphite' || storedStyle === 'neon') {
			cardStylePreset = storedStyle;
			draftCardStylePreset = storedStyle;
		}
		if (storedColor && /^#[0-9a-fA-F]{6}$/.test(storedColor)) {
			cardBackgroundColor = storedColor;
			draftCardBackgroundColor = storedColor;
		}
		loadStores();
	});
</script>

<div class="knowledge-store-view" style="--page-bg:{pageBg}; --shell-bg:{shellBg}; --card-bg:{cardBg}; --card-raised:{cardBgRaised}; --border:{borderColor}; --border-soft:{borderSoft}; --text-primary:{textPrimary}; --text-secondary:{textSecondary}; --text-muted:{textMuted}; --accent:{accent}; --accent-soft:{accentSoft}; --success-soft:{successSoft}; --danger:{danger}; --danger-soft:{dangerSoft}; --user-card-color:{cardBackgroundColor}; --neon-slab:{neonSlab}; --neon-slab-soft:{neonSlabSoft}; --neon-slab-raised:{neonSlabRaised}; --neon-slab-raised-soft:{neonSlabRaisedSoft}; --neon-pill-text:{neonPillText};">
	<section class="hero-shell">
		<div class="hero-copy">
			<div class="eyebrow">Knowledge Stores</div>
			<h2>Choose a Knowledge Store to Explore</h2>
			<p>
				This page is the control surface for what the rest of the Knowledge System operates on. Cards are selection-first, but they also expose the configuration you need to edit with confidence.
			</p>
			<div class="hero-active-strip">
				<div class="active-banner-icon">
					{#if knowledgeStoreState.activeStore}
						<CircleCheckBigIcon class="h-5 w-5" />
					{:else}
						<SparklesIcon class="h-5 w-5" />
					{/if}
				</div>
				<div class="active-banner-copy">
					<div class="active-banner-label">Active Knowledge Store</div>
					<div class="active-banner-title">
						{knowledgeStoreState.activeStore ? knowledgeStoreState.activeStore.ks_name : 'Nothing selected'}
					</div>
					<div class="active-banner-text">
						{#if knowledgeStoreState.activeStore}
							<span class="mono">{knowledgeStoreState.activeStore.ks_name}</span> is currently selected. Most sections in <span class="mono">/home3/knowledge</span> should assume this store.
						{:else}
							{emptyText}
						{/if}
					</div>
				</div>
			</div>
		</div>
		<div class="hero-actions">
			<button class="ghost-button" type="button" onclick={openStyleDialog}>
				<PaletteIcon class="h-4 w-4" />
				Card Style
			</button>
			<button class="add-button" type="button" onclick={openCreateDialog}>
				<PlusIcon class="h-4 w-4" />
				Add Knowledge Store
			</button>
		</div>
	</section>

	{#if error}
		<div class="notice notice-error">
			<div>
				<div class="notice-title">Could not load knowledge stores</div>
				<div class="notice-copy">{error}</div>
			</div>
			<button class="ghost-button" type="button" onclick={loadStores}>
				<RefreshCwIcon class="h-4 w-4" />
				Try Again
			</button>
		</div>
	{/if}

	{#if loading}
		<div class="state-panel">
			<div class="state-glyph">⋯</div>
			<div class="state-title">Loading knowledge stores</div>
			<div class="state-copy">Pulling the catalog and active-store context now.</div>
		</div>
	{:else if !error && stores.length === 0}
		<div class="state-panel">
			<div class="state-glyph">◎</div>
			<div class="state-title">No knowledge stores yet</div>
			<div class="state-copy">Create the first store to define what this knowledge workspace should contain.</div>
			<button class="add-button" type="button" onclick={openCreateDialog}>
				<PlusIcon class="h-4 w-4" />
				Create First Store
			</button>
		</div>
	{:else}
		<section class="store-grid">
			{#each stores as store (store.id)}
				<div
					class:style-blush={cardStylePreset === 'blush'}
					class:style-graphite={cardStylePreset === 'graphite'}
					class:style-neon={cardStylePreset === 'neon'}
					class:selected={knowledgeStoreState.activeStoreId === store.id}
					class:warning={Boolean(store.error_msg)}
					class="store-card"
					role="button"
					tabindex="0"
					onclick={() => selectStore(store)}
					onkeydown={(event) => {
						if (event.key === 'Enter' || event.key === ' ') {
							event.preventDefault();
							selectStore(store);
						}
					}}
				>
					<div class="store-card-head">
						<div>
							<div class="store-type">{store.ks_type || 'Unclassified store'}</div>
							<h3>{store.ks_name}</h3>
						</div>
						{#if knowledgeStoreState.activeStoreId === store.id}
							<span class="active-pill">Active</span>
						{/if}
					</div>

					<p class="store-description">
						{store.ks_desc?.trim() || 'No description yet. Use Modify to explain the scope and intended contents of this store.'}
					</p>

					<div class="badge-row">
						<span class="badge accent">{syncModeLabel(store)}</span>
						<span class="badge">{statusLabel(store)}</span>
						{#if store.error_msg}
							<span class="badge danger">Needs attention</span>
						{/if}
					</div>

					<div class="source-panel">
						<div class="source-header">
							<div class="source-label">Sources</div>
							<div class="source-summary">{sourceSummary(store)}</div>
						</div>
						{#if sourcePreview(store).length > 0}
							<ul>
								{#each sourcePreview(store) as source}
									<li>{source}</li>
								{/each}
							</ul>
						{:else}
							<div class="source-empty">Add directories, URLs, or tables to give this store a knowledge scope.</div>
						{/if}
					</div>

					<div class="meta-row">
						<div>
							<span class="meta-label">Updated</span>
							<span class="meta-value">{formatDate(store.modify_time || store.create_time)}</span>
						</div>
						<div>
							<span class="meta-label">ID</span>
							<span class="meta-value mono">{store.id}</span>
						</div>
					</div>

					{#if store.error_msg}
						<div class="error-strip">
							<TriangleAlertIcon class="h-4 w-4" />
							<span>{store.error_msg}</span>
						</div>
					{/if}

					<div class="card-actions">
						<button
							type="button"
							class="action-button"
							onclick={(event) => {
								event.stopPropagation();
								openEditDialog(store);
							}}
						>
							<PencilIcon class="h-4 w-4" />
							Modify
						</button>
						<button
							type="button"
							class="action-button danger"
							onclick={(event) => {
								event.stopPropagation();
								openDeleteDialog(store);
							}}
						>
							<Trash2Icon class="h-4 w-4" />
							Delete
						</button>
					</div>
				</div>
			{/each}
		</section>
	{/if}

	{#if dialogMode}
		<div class="dialog-overlay" aria-hidden="true">
			<div
				class="dialog-panel"
				role="dialog"
				aria-modal="true"
				tabindex="0"
				onclick={(event) => event.stopPropagation()}
				onkeydown={(event) => event.stopPropagation()}
			>
				{#if dialogMode === 'delete'}
					<div class="dialog-header">
						<div class="dialog-eyebrow">Delete Knowledge Store</div>
						<h3>Remove <span class="mono">{editingStore?.ks_name}</span>?</h3>
						<p>
							{#if editingStore && knowledgeStoreState.activeStoreId === editingStore.id}
								This store is currently active. Deleting it will clear the active knowledge store for this page.
							{:else}
								This action removes the knowledge store from the management catalog.
							{/if}
						</p>
					</div>
					{#if dialogError}
						<div class="notice notice-error compact">{dialogError}</div>
					{/if}
					<div class="dialog-actions">
						<button class="ghost-button" type="button" onclick={closeDialog}>Cancel</button>
						<button class="danger-button" type="button" onclick={confirmDelete} disabled={saving}>
							{saving ? 'Deleting…' : 'Delete'}
						</button>
					</div>
				{:else if dialogMode === 'style'}
					<div class="dialog-header">
						<div class="dialog-eyebrow">Card Style</div>
						<h3>Pick the card look for your knowledge stores</h3>
						<p>Choose one of the three presets, then tune the background color. Your selection is remembered on this browser.</p>
					</div>

					<div class="style-grid">
						{#each cardStyleOptions as option}
							<button
								type="button"
								class:selected={draftCardStylePreset === option.id}
								class="style-option"
								onclick={() => selectDraftCardStylePreset(option.id)}
							>
								<div class={`style-preview ${option.id}`}>
									<div class="style-preview-inner">
										<div class="style-preview-title">{option.name}</div>
										<div class="style-preview-copy">{option.description}</div>
									</div>
								</div>
								<div class="style-option-name">{option.name}</div>
								<div class="style-option-copy">{option.description}</div>
							</button>
						{/each}
					</div>

					<div class="field field-wide style-color-field">
						<span>Background Color</span>
						<div class="color-row">
							<input
								bind:value={draftCardBackgroundColor}
								type="color"
								oninput={(event) => handleDraftCardColorChange((event.currentTarget as HTMLInputElement).value)}
							/>
							<input
								bind:value={draftCardBackgroundColor}
								type="text"
								pattern="^#[0-9a-fA-F]{6}$"
								onchange={(event) => handleDraftCardColorChange((event.currentTarget as HTMLInputElement).value)}
							/>
						</div>
					</div>

					<div class="dialog-actions">
						<button class="ghost-button" type="button" onclick={closeDialog}>Cancel</button>
						<button class="add-button" type="button" onclick={confirmStyleDialog}>Confirm</button>
					</div>
				{:else}
					<div class="dialog-header">
						<div class="dialog-eyebrow">{dialogMode === 'create' ? 'Create Knowledge Store' : 'Modify Knowledge Store'}</div>
						<h3>{dialogMode === 'create' ? 'Add a new knowledge store' : `Edit ${editingStore?.ks_name}`}</h3>
						<p>Describe what this store contains first, then layer on status and sync behavior as supporting details.</p>
					</div>

					<div class="dialog-grid">
						<label class="field field-wide">
							<span>Name</span>
							<input bind:value={formName} type="text" placeholder="Semiconductor Standards Library" />
						</label>
						<label class="field">
							<span>Type</span>
							<input bind:value={formType} type="text" placeholder="Reference, Research, Spec…" />
						</label>
						<label class="field">
							<span>Sync Mode</span>
							<select bind:value={formSyncMode}>
								<option value="manual">manual</option>
								<option value="auto">auto</option>
							</select>
						</label>
						<label class="field field-wide">
							<span>Description</span>
							<textarea bind:value={formDescription} rows="3" placeholder="What does this store cover, and why would someone choose it as the active store?"></textarea>
						</label>
						<label class="field field-wide">
							<span>Sources</span>
							<textarea bind:value={formSources} rows="5" placeholder="/data/specs&#10;https://example.com/reference&#10;kb.table_name"></textarea>
						</label>
						<label class="field">
							<span>Status</span>
							<select bind:value={formStatus}>
								<option value="active">active</option>
								<option value="suspended">suspended</option>
								<option value="inactive">inactive</option>
							</select>
						</label>
						<label class="field field-wide">
							<span>Notes</span>
							<textarea bind:value={formNotes} rows="3" placeholder="Optional operational notes or handoff context"></textarea>
						</label>
					</div>

					{#if dialogError}
						<div class="notice notice-error compact">{dialogError}</div>
					{/if}

					<div class="dialog-actions">
						<button class="ghost-button" type="button" onclick={closeDialog}>Cancel</button>
						<button class="add-button" type="button" onclick={saveDialog} disabled={saving}>
							{saving ? 'Saving…' : dialogMode === 'create' ? 'Create Store' : 'Save Changes'}
						</button>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	@import url('https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,500;9..144,600&family=Manrope:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500&display=swap');

	.knowledge-store-view {
		height: 100%;
		overflow: auto;
		padding: 1.5rem;
		background: var(--page-bg);
		color: var(--text-primary);
	}

	.hero-shell,
	.state-panel,
	.notice,
	.store-card,
	.dialog-panel {
		border: 1px solid var(--border);
		background: var(--card-bg);
		backdrop-filter: blur(18px);
		box-shadow: 0 24px 60px rgba(4, 10, 24, 0.24);
	}

	.hero-shell {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1.5rem;
		padding: 1.75rem;
		border-radius: 1.75rem;
		margin-bottom: 1rem;
		background: var(--card-bg);
	}

	.hero-actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex: 0 0 auto;
	}

	.hero-copy h2,
	.dialog-header h3,
	.store-card h3 {
		font-family: 'Fraunces', Georgia, serif;
	}

	.hero-copy h2 {
		margin: 0.25rem 0 0.5rem;
		font-size: clamp(1.35rem, 1.85vw, 1.8rem);
		line-height: 1.05;
		max-width: none;
		white-space: nowrap;
	}

	.hero-copy p,
	.active-banner-text,
	.store-description,
	.notice-copy,
	.state-copy,
	.dialog-header p,
	.source-empty,
	.source-panel li {
		font-family: 'Manrope', sans-serif;
		color: var(--text-secondary);
		line-height: 1.6;
	}

	.eyebrow,
	.active-banner-label,
	.dialog-eyebrow,
	.meta-label,
	.source-label,
	.store-type {
		font-family: 'IBM Plex Mono', monospace;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		font-size: 0.72rem;
		color: var(--text-muted);
	}

	.add-button,
	.ghost-button,
	.danger-button,
	.action-button {
		display: inline-flex;
		align-items: center;
		gap: 0.55rem;
		border-radius: 999px;
		border: 1px solid transparent;
		padding: 0.78rem 1.1rem;
		font-family: 'Manrope', sans-serif;
		font-weight: 700;
		cursor: pointer;
		transition:
			transform 140ms ease,
			background 140ms ease,
			border-color 140ms ease,
			color 140ms ease;
	}

	.add-button {
		background: var(--accent);
		color: #fff8f2;
	}

	.ghost-button,
	.action-button {
		background: transparent;
		border-color: var(--border);
		color: var(--text-primary);
	}

	.danger-button,
	.action-button.danger {
		background: var(--danger-soft);
		border-color: color-mix(in srgb, var(--danger) 45%, var(--border));
		color: var(--danger);
	}

	.add-button:hover,
	.ghost-button:hover,
	.danger-button:hover,
	.action-button:hover {
		transform: translateY(-1px);
	}

	.active-banner {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 1.15rem 1.25rem;
		border-radius: 1.25rem;
		margin-bottom: 1.2rem;
	}

	.active-banner-icon {
		display: flex;
		height: 3rem;
		width: 3rem;
		align-items: center;
		justify-content: center;
		border-radius: 1rem;
		background: var(--accent-soft);
		color: var(--accent);
		flex: 0 0 auto;
	}

	.active-banner-title {
		font-family: 'Fraunces', Georgia, serif;
		font-size: 1.08rem;
		margin: 0.18rem 0;
	}

	.hero-active-strip {
		display: flex;
		align-items: center;
		gap: 0.95rem;
		margin-top: 1rem;
		padding: 0.95rem 1rem;
		border-radius: 1.15rem;
		border: 1px solid var(--border-soft);
		background: linear-gradient(180deg, rgba(255, 255, 255, 0.035), rgba(255, 255, 255, 0.01));
		max-width: 48rem;
	}

	.notice {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		border-radius: 1rem;
		padding: 1rem 1.1rem;
		margin-bottom: 1rem;
	}

	.notice-title {
		font-family: 'Manrope', sans-serif;
		font-weight: 700;
		margin-bottom: 0.18rem;
	}

	.notice-error {
		border-color: color-mix(in srgb, var(--danger) 36%, var(--border));
		background: color-mix(in srgb, var(--danger-soft) 70%, var(--card-bg));
	}

	.notice.compact {
		margin-top: 0.5rem;
		margin-bottom: 0;
		padding: 0.85rem 1rem;
	}

	.state-panel {
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
		border-radius: 1.5rem;
		padding: 3.5rem 1.5rem;
		gap: 0.75rem;
	}

	.state-glyph {
		font-family: 'Fraunces', Georgia, serif;
		font-size: 2rem;
		color: var(--accent);
	}

	.state-title {
		font-family: 'Fraunces', Georgia, serif;
		font-size: 1.4rem;
	}

	.store-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(320px, 320px));
		justify-content: start;
		gap: 1rem;
		padding-bottom: 1rem;
	}

	.store-card {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		width: 320px;
		min-height: 380px;
		padding: 1.2rem;
		border-radius: 1.7rem;
		text-align: left;
		cursor: pointer;
		background:
			linear-gradient(180deg, rgba(255, 255, 255, 0.04), rgba(255, 255, 255, 0.015)),
			linear-gradient(135deg, rgba(89, 159, 255, 0.10), transparent 62%),
			var(--card-bg);
		transition:
			transform 160ms ease,
			border-color 160ms ease,
			background 160ms ease,
			box-shadow 160ms ease;
	}

	.store-card.style-blush {
		background:
			linear-gradient(180deg, color-mix(in srgb, var(--user-card-color) 56%, white 34%), rgba(255, 255, 255, 0.02) 42%),
			linear-gradient(135deg, rgba(255, 244, 214, 0.16), transparent 70%),
			var(--card-bg);
	}

	.store-card.style-blush:hover,
	.store-card.style-blush.selected {
		background:
			linear-gradient(180deg, color-mix(in srgb, var(--user-card-color) 62%, white 30%), rgba(255, 255, 255, 0.04) 40%),
			linear-gradient(135deg, rgba(255, 244, 214, 0.2), transparent 68%),
			var(--card-raised);
	}

	.store-card.style-graphite {
		background:
			radial-gradient(circle at top left, color-mix(in srgb, var(--user-card-color) 26%, white 10%), transparent 42%),
			linear-gradient(180deg, rgba(255, 255, 255, 0.03), rgba(255, 255, 255, 0.01)),
			color-mix(in srgb, var(--user-card-color) 20%, var(--card-bg) 80%);
		border-color: color-mix(in srgb, white 18%, var(--border));
	}

	.store-card.style-graphite:hover,
	.store-card.style-graphite.selected {
		background:
			radial-gradient(circle at top left, color-mix(in srgb, var(--user-card-color) 32%, white 12%), transparent 42%),
			linear-gradient(180deg, rgba(255, 255, 255, 0.045), rgba(255, 255, 255, 0.015)),
			color-mix(in srgb, var(--user-card-color) 26%, var(--card-raised) 74%);
	}

	.store-card.style-neon {
		background:
			linear-gradient(180deg, var(--neon-slab), var(--neon-slab-soft)),
			var(--card-bg);
		box-shadow:
			0 0 0 1px color-mix(in srgb, var(--user-card-color) 75%, #39d0ff 25%),
			0 0 34px color-mix(in srgb, var(--user-card-color) 48%, transparent),
			0 26px 60px rgba(4, 10, 24, 0.34);
	}

	.store-card.style-neon:hover,
	.store-card.style-neon.selected {
		background:
			linear-gradient(180deg, var(--neon-slab-raised), var(--neon-slab-raised-soft)),
			var(--card-raised);
		box-shadow:
			0 0 0 1px color-mix(in srgb, var(--user-card-color) 78%, #39d0ff 22%),
			0 0 38px color-mix(in srgb, var(--user-card-color) 54%, transparent),
			0 32px 72px rgba(7, 12, 26, 0.42);
	}

	.store-card.style-neon .active-pill,
	.store-card.style-neon .badge.accent {
		color: color-mix(in srgb, var(--user-card-color) 80%, var(--neon-pill-text));
	}

	.store-card:hover {
		transform: translateY(-4px);
		box-shadow: 0 26px 60px rgba(4, 10, 24, 0.34);
	}

	.store-card.selected {
		border-color: color-mix(in srgb, var(--user-card-color) 60%, #76a7ff 40%);
		box-shadow: 0 32px 72px rgba(7, 12, 26, 0.36);
	}

	.store-card.warning {
		border-color: color-mix(in srgb, var(--danger) 36%, var(--border));
	}

	.store-card-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}

	.store-card h3 {
		margin: 0.22rem 0 0;
		font-size: 1.45rem;
		line-height: 1.1;
	}

	.active-pill,
	.badge {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border-radius: 999px;
		padding: 0.32rem 0.68rem;
		font-family: 'IBM Plex Mono', monospace;
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.active-pill {
		background: var(--success-soft);
		color: #8ce5cc;
	}

	.badge-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
	}

	.badge {
		border: 1px solid var(--border-soft);
		background: color-mix(in srgb, var(--shell-bg) 70%, var(--card-bg));
		color: var(--text-secondary);
	}

	.badge.accent {
		background: var(--accent-soft);
		color: var(--accent);
		border-color: transparent;
	}

	.badge.danger {
		background: var(--danger-soft);
		color: var(--danger);
		border-color: transparent;
	}

	.source-panel {
		border: 1px solid var(--border-soft);
		border-radius: 1.1rem;
		padding: 0.85rem 0.95rem;
		background: linear-gradient(180deg, rgba(12, 21, 35, 0.34), rgba(12, 21, 35, 0.18));
	}

	.source-header,
	.meta-row,
	.dialog-actions,
	.card-actions {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.source-panel ul {
		margin: 0.65rem 0 0;
		padding-left: 1rem;
	}

	.source-panel li {
		margin-bottom: 0.35rem;
		word-break: break-word;
	}

	.source-summary,
	.meta-value,
	.mono {
		font-family: 'IBM Plex Mono', monospace;
		font-size: 0.82rem;
		color: var(--text-secondary);
	}

	.meta-row {
		padding-top: 0.25rem;
		border-top: 1px solid var(--border-soft);
	}

	.meta-row > div {
		display: flex;
		flex-direction: column;
		gap: 0.24rem;
	}

	.error-strip {
		display: flex;
		align-items: flex-start;
		gap: 0.55rem;
		border-radius: 0.95rem;
		padding: 0.75rem 0.85rem;
		background: var(--danger-soft);
		color: var(--danger);
		font-family: 'Manrope', sans-serif;
		font-size: 0.92rem;
	}

	.card-actions {
		margin-top: auto;
		padding-top: 0.15rem;
	}

	.action-button {
		background: rgba(255, 255, 255, 0.02);
	}

	.dialog-overlay {
		position: fixed;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1.2rem;
		background: rgba(15, 23, 42, 0.48);
		backdrop-filter: blur(10px);
		z-index: 30;
	}

	.dialog-panel {
		width: min(760px, 100%);
		max-height: calc(100vh - 2.4rem);
		overflow: auto;
		padding: 1.4rem;
		border-radius: 1.5rem;
	}

	.dialog-header h3 {
		margin: 0.28rem 0 0.45rem;
		font-size: 1.6rem;
	}

	.dialog-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.9rem;
		margin-top: 1rem;
	}

	.style-grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 1rem;
		margin-top: 1rem;
	}

	.style-option {
		border: 1px solid var(--border-soft);
		border-radius: 1.25rem;
		padding: 0.85rem;
		background: rgba(255, 255, 255, 0.02);
		color: var(--text-primary);
		text-align: left;
		cursor: pointer;
	}

	.style-option.selected {
		border-color: var(--accent);
		box-shadow: 0 0 0 1px rgba(255, 155, 84, 0.28);
	}

	.style-preview {
		height: 180px;
		border-radius: 1rem;
		padding: 0.9rem;
		margin-bottom: 0.8rem;
		display: flex;
		align-items: flex-end;
	}

	.style-preview.blush {
		background: linear-gradient(180deg, #ff8fb1, #ffd5b7 68%);
	}

	.style-preview.graphite {
		background: linear-gradient(135deg, #505057, #35353a);
		box-shadow: inset 0 0 0 2px rgba(255, 255, 255, 0.5);
	}

	.style-preview.neon {
		background: linear-gradient(180deg, #070a12, #070a12);
		box-shadow:
			0 0 0 2px #8b5cf6,
			0 0 28px rgba(109, 40, 217, 0.45);
	}

	.style-preview-inner {
		border-radius: 0.75rem;
		padding: 0.8rem;
		width: 100%;
		background: rgba(10, 14, 22, 0.32);
		backdrop-filter: blur(6px);
	}

	.style-preview-title,
	.style-option-name {
		font-family: 'Manrope', sans-serif;
		font-weight: 700;
	}

	.style-preview-copy,
	.style-option-copy {
		font-family: 'Manrope', sans-serif;
		font-size: 0.88rem;
		color: var(--text-secondary);
		line-height: 1.5;
	}

	.style-color-field {
		margin-top: 1rem;
	}

	.color-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.color-row input[type='color'] {
		width: 3rem;
		height: 3rem;
		padding: 0.2rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.42rem;
		font-family: 'Manrope', sans-serif;
	}

	.field span {
		font-size: 0.88rem;
		font-weight: 700;
		color: var(--text-secondary);
	}

	.field input,
	.field select,
	.field textarea {
		width: 100%;
		border-radius: 0.95rem;
		border: 1px solid var(--border);
		background: color-mix(in srgb, var(--shell-bg) 72%, var(--card-bg));
		color: var(--text-primary);
		padding: 0.82rem 0.95rem;
		font: inherit;
	}

	.field textarea {
		resize: vertical;
	}

	.field-wide {
		grid-column: 1 / -1;
	}

	.dialog-actions {
		margin-top: 1rem;
		justify-content: flex-end;
	}

	@media (max-width: 900px) {
		.hero-shell {
			flex-direction: column;
		}

		.hero-copy h2 {
			white-space: normal;
		}

		.style-grid {
			grid-template-columns: 1fr;
		}
	}

	@media (max-width: 720px) {
		.knowledge-store-view {
			padding: 1rem;
		}

		.store-grid {
			grid-template-columns: minmax(0, 1fr);
		}

		.store-card {
			width: 100%;
			min-height: 0;
		}

		.dialog-grid,
		.meta-row,
		.card-actions,
		.notice,
		.dialog-actions {
			grid-template-columns: 1fr;
			flex-direction: column;
			align-items: stretch;
		}

		.hero-active-strip,
		.store-card-head,
		.source-header {
			flex-direction: column;
			align-items: flex-start;
		}
	}
</style>
