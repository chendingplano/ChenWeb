<script lang="ts">
	import { browser } from '$app/environment';
	import SearchIcon from '@lucide/svelte/icons/search';
	import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';
	import Settings2Icon from '@lucide/svelte/icons/settings-2';
	import {
		getKbInput,
		listKbInputs,
		type KbInputRecord
	} from '$lib/services/kbService';
	import KbInputSearchDialog from './kb-input-search-dialog.svelte';
	import { knowledgeStoreState } from './knowledge-store-state.svelte';
	import {
		KB_INPUT_RECORD_BROWSER_DEFAULT_HIGHLIGHT_COLOR,
		KB_INPUT_RECORD_BROWSER_DEFAULT_LIST_WIDTH,
		KB_INPUT_RECORD_BROWSER_DEFAULT_PAGE_SIZE,
		KB_INPUT_RECORD_BROWSER_HIGHLIGHT_PRESETS,
		KB_INPUT_RECORD_BROWSER_MAX_LIST_WIDTH,
		KB_INPUT_RECORD_BROWSER_MIN_LIST_WIDTH,
		clampKbInputRecordBrowserListWidth,
		clampKbInputRecordBrowserPageSize,
		readKbInputRecordBrowserSettings,
		writeKbInputRecordBrowserSettings
	} from './kb-input-record-browser-settings.js';
	import {
		buildTopicTreeListParams,
		createDefaultRecordBrowserFilters,
		hasActiveRecordBrowserFilters,
		resolveSelectedRecordId
	} from './topic-tree-record-browser.js';

	type RecordBrowserFilters = ReturnType<typeof createDefaultRecordBrowserFilters>;

	type BrowserRecordCard = {
		id: number;
		title: string;
		subtitle?: string;
		meta?: string[];
		status?: string;
		description?: string;
		badges?: string[];
	};

	let {
		darkMode = true,
		instanceKey,
		title = 'kb.inputs',
		subtitle = '',
		emptyTitle = 'No records found.',
		emptySubtitle = 'Use Search or Retrieve to browse kb.inputs.',
		pageSize = KB_INPUT_RECORD_BROWSER_DEFAULT_PAGE_SIZE,
		scopeToActiveStore = false,
		selectedRecordId = null,
		autoSelectFirstRecord = true,
		defaultListWidth = KB_INPUT_RECORD_BROWSER_DEFAULT_LIST_WIDTH,
		renderMode = 'compact',
		mapRecord = defaultMapRecord,
		onSelect = () => {},
		onResultsChange = () => {},
		onError = () => {},
		onFiltersChange = () => {}
	}: {
		darkMode?: boolean;
		instanceKey: string;
		title?: string;
		subtitle?: string;
		emptyTitle?: string;
		emptySubtitle?: string;
		pageSize?: number;
		scopeToActiveStore?: boolean;
		selectedRecordId?: number | null;
		autoSelectFirstRecord?: boolean;
		defaultListWidth?: number;
		renderMode?: 'compact' | 'cards';
		mapRecord?: (record: KbInputRecord) => BrowserRecordCard;
		onSelect?: (record: KbInputRecord) => void;
		onResultsChange?: (payload: { results: KbInputRecord[]; total: number; page: number }) => void;
		onError?: (error: Error) => void;
		onFiltersChange?: (filters: RecordBrowserFilters) => void;
	} = $props();

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let panelAlt = $derived(darkMode ? '#0f172a' : '#eef2ff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');
	let accent = $derived(darkMode ? '#22c55e' : '#16a34a');

	let searchOpen = $state(false);
	let settingsOpen = $state(false);
	let recordIdInput = $state('');
	let results = $state<KbInputRecord[]>([]);
	let filters = $state<RecordBrowserFilters>(createDefaultRecordBrowserFilters());
	let loading = $state(true);
	let loadError = $state('');
	let listPage = $state(1);
	let listTotal = $state(0);
	let listJumpInput = $state('1');
	let listWidth = $state(clampKbInputRecordBrowserListWidth(defaultListWidth));
	let selectedRecordIdInternal = $state<number | null>(selectedRecordId);
	let lastEmittedRecordId = $state<number | null>(null);
	let justEmitted = $state(false);
	let resizing = $state(false);
	let settingsHydrated = $state(false);

	let highlightColor = $state(KB_INPUT_RECORD_BROWSER_DEFAULT_HIGHLIGHT_COLOR);
	let effectivePageSize = $state(clampKbInputRecordBrowserPageSize(pageSize));
	let listTotalPages = $derived(Math.max(1, Math.ceil(listTotal / effectivePageSize)));
	let resetDisabled = $derived(!hasActiveRecordBrowserFilters(filters));

	$effect(() => {
		if (!settingsHydrated && browser) {
			const saved = readKbInputRecordBrowserSettings(localStorage, instanceKey);
			listWidth = clampKbInputRecordBrowserListWidth(saved.listWidth ?? defaultListWidth);
			effectivePageSize = clampKbInputRecordBrowserPageSize(saved.pageSize ?? pageSize);
			if (saved.highlightColor) highlightColor = saved.highlightColor;
			settingsHydrated = true;
		}
	});

	$effect(() => {
		if (
			selectedRecordId != null &&
			selectedRecordId !== selectedRecordIdInternal
		) {
			if (justEmitted) {
				// We just called onSelect() for a different record.
				// The parent's selectedRecordId prop still holds the old
				// value (the async handler hasn't finished yet).  Do NOT
				// sync the stale value back — that causes an oscillation
				// where the browser keeps toggling between two records.
				return;
			}
			selectedRecordIdInternal = selectedRecordId;
		}
	});

	// Clear justEmitted once the parent acknowledges our emit by
	// updating selectedRecordId to match.
	$effect(() => {
		if (justEmitted && selectedRecordId === lastEmittedRecordId) {
			justEmitted = false;
		}
	});

	$effect(() => {
		if (settingsHydrated) {
			void loadRecords(1, selectedRecordIdInternal);
		}
	});

	function saveSettings(next: Partial<{ listWidth: number; pageSize: number; highlightColor: string }>) {
		const mergedWidth = clampKbInputRecordBrowserListWidth(next.listWidth ?? listWidth);
		const mergedPageSize = clampKbInputRecordBrowserPageSize(next.pageSize ?? effectivePageSize);
		const mergedHighlight =
			typeof next.highlightColor === 'string' && /^#[0-9a-fA-F]{6}$/.test(next.highlightColor)
				? next.highlightColor
				: highlightColor;
		listWidth = mergedWidth;
		effectivePageSize = mergedPageSize;
		highlightColor = mergedHighlight;
		if (browser) {
			writeKbInputRecordBrowserSettings(localStorage, instanceKey, {
				listWidth: mergedWidth,
				pageSize: mergedPageSize,
				highlightColor: mergedHighlight
			});
		}
	}

	function applyFilters(nextFilters: RecordBrowserFilters) {
		filters = { ...nextFilters };
		onFiltersChange(filters);
	}

	function formatMaybeDate(value?: string) {
		if (!value) return '—';
		return value.replace('T', ' ').slice(0, 19);
	}

	function statusText(record: KbInputRecord) {
		const matched = [...(record.status ?? [])].reverse().find((item) => item != null);
		if (!matched) return 'pending';
		return (
			matched.proc_status?.trim() ||
			matched['proc-status']?.trim() ||
			matched.status?.trim() ||
			'pending'
		);
	}

	function defaultMapRecord(record: KbInputRecord): BrowserRecordCard {
		return {
			id: record.id,
			title:
				record.title?.trim() ||
				record.name?.trim() ||
				record.file_name?.trim() ||
				`Record #${record.id}`,
			subtitle: record.file_name?.trim() || record.name?.trim() || '—',
			meta: [record.doc_no?.trim() || '—', record.parser_name?.trim() || '—'],
			status: statusText(record),
			description: formatMaybeDate(record.create_time),
			badges: [record.type?.trim() || '—']
		};
	}

	function emitSelectedRecord(record: KbInputRecord | null, force = false) {
		selectedRecordIdInternal = record?.id ?? null;
		if (!record) {
			lastEmittedRecordId = null;
			justEmitted = false;
			return;
		}
		if (!force && lastEmittedRecordId === record.id) return;
		lastEmittedRecordId = record.id;
		justEmitted = true;
		onSelect(record);
	}

	async function loadRecords(page = listPage, preferredRecordId = selectedRecordIdInternal) {
		loading = true;
		loadError = '';
		try {
			const response = await listKbInputs(
				buildTopicTreeListParams({
					page,
					pageSize: effectivePageSize,
					activeStoreId: knowledgeStoreState.activeStore?.id ?? null,
					scopeToActiveStore,
					filters
				})
			);
			results = response.results ?? [];
			listTotal = Math.max(0, response.total ?? 0);
			listPage = Math.max(1, response.page ?? page);
			listJumpInput = String(listPage);
			onResultsChange({ results, total: listTotal, page: listPage });

			const nextRecordId = resolveSelectedRecordId({
				records: results,
				selectedRecordId: preferredRecordId,
				autoSelectFirstRecord
			});
			const nextRecord = results.find((record) => record.id === nextRecordId) ?? null;
			emitSelectedRecord(nextRecord, nextRecordId !== preferredRecordId);
		} catch (error) {
			results = [];
			listTotal = 0;
			listPage = 1;
			listJumpInput = '1';
			loadError =
				error instanceof Error ? error.message : 'Failed to load kb.inputs records';
			selectedRecordIdInternal = null;
			lastEmittedRecordId = null;
			onError(error instanceof Error ? error : new Error(loadError));
		} finally {
			loading = false;
		}
	}

	async function retrieveRecord() {
		const recordId = Number(recordIdInput.trim());
		loadError = '';
		if (recordIdInput.trim() === '') {
			await loadRecords(1, selectedRecordIdInternal);
			return;
		}
		if (!Number.isFinite(recordId) || recordId <= 0) {
			loadError = 'Enter a valid Record ID';
			return;
		}

		loading = true;
		try {
			const response = await getKbInput(recordId);
			results = [response.record];
			listTotal = 1;
			listPage = 1;
			listJumpInput = '1';
			emitSelectedRecord(response.record, true);
			onResultsChange({ results, total: 1, page: 1 });
		} catch (error) {
			results = [];
			listTotal = 0;
			selectedRecordIdInternal = null;
			loadError =
				error instanceof Error ? error.message : 'Failed to retrieve kb.inputs record';
			onError(error instanceof Error ? error : new Error(loadError));
		} finally {
			loading = false;
		}
	}

	async function prevPage() {
		if (listPage <= 1 || loading || recordIdInput.trim() !== '') return;
		await loadRecords(listPage - 1, selectedRecordIdInternal);
	}

	async function nextPage() {
		if (listPage >= listTotalPages || loading || recordIdInput.trim() !== '') return;
		await loadRecords(listPage + 1, selectedRecordIdInternal);
	}

	async function jumpToPage() {
		if (loading || recordIdInput.trim() !== '') return;
		const parsed = Number(listJumpInput.trim());
		if (!Number.isFinite(parsed)) {
			listJumpInput = String(listPage);
			return;
		}
		const target = Math.max(1, Math.min(listTotalPages, Math.trunc(parsed)));
		listJumpInput = String(target);
		if (target === listPage) return;
		await loadRecords(target, selectedRecordIdInternal);
	}

	async function resetFilters() {
		if (resetDisabled) return;
		recordIdInput = '';
		applyFilters(createDefaultRecordBrowserFilters());
		await loadRecords(1, null);
	}

	function chooseRecord(record: KbInputRecord) {
		emitSelectedRecord(record, true);
	}

	function startResize(event: PointerEvent) {
		event.preventDefault();
		const startX = event.clientX;
		const startWidth = listWidth;
		resizing = true;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';

		const handleMove = (moveEvent: PointerEvent) => {
			saveSettings({
				listWidth: clampKbInputRecordBrowserListWidth(startWidth + (moveEvent.clientX - startX))
			});
		};
		const handleUp = () => {
			resizing = false;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			window.removeEventListener('pointermove', handleMove);
			window.removeEventListener('pointerup', handleUp);
			window.removeEventListener('pointercancel', handleUp);
		};

		window.addEventListener('pointermove', handleMove);
		window.addEventListener('pointerup', handleUp, { once: true });
		window.addEventListener('pointercancel', handleUp, { once: true });
	}

	function onResizerKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			saveSettings({ listWidth: listWidth - 16 });
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			saveSettings({ listWidth: listWidth + 16 });
		}
	}
</script>

<div
	class="browser-shell"
	style={`--panel:${panelBg}; --panel-alt:${panelAlt}; --border:${border}; --text:${textMain}; --muted:${textMuted}; --accent:${accent}; --panel-bg:${panelBg}; --panel-bg-alt:${panelAlt}; --ink-line:${border}; --ink-line-soft:rgba(148, 163, 184, 0.16); --text-primary:${textMain}; --text-secondary:${textMuted}; --text-muted:${textMuted}; --highlight:${highlightColor}; width:${listWidth}px;`}
>
	<KbInputSearchDialog
		bind:open={searchOpen}
		initialFilters={filters}
		{scopeToActiveStore}
		onSelect={async (record, nextFilters) => {
			recordIdInput = '';
			applyFilters(nextFilters);
			await loadRecords(1, record.id);
		}}
	/>

	<div class="panel">
		<div class="left-controls">
			<label class="field">
				<span class="field-label">Record ID</span>
				<div class="field-row">
					<input
						type="text"
						inputmode="numeric"
						bind:value={recordIdInput}
						placeholder="e.g. 1042"
						onkeydown={(event) => {
							if (event.key === 'Enter') {
								void retrieveRecord();
							}
						}}
					/>
					<button type="button" class="ghost search-btn" onclick={() => (searchOpen = true)} title="Search records from kb.inputs">
						<SearchIcon class="h-4 w-4" />
						Search
					</button>
				</div>
			</label>

			<div class="action-row">
				<button type="button" class="retrieve-btn" onclick={retrieveRecord} disabled={loading}>
					{#if loading}Loading…{:else}<span class="retrieve-arrow">→</span>Retrieve{/if}
				</button>
				<button type="button" class="ghost reset-btn" onclick={() => void resetFilters()} disabled={resetDisabled}>
					<RotateCcwIcon class="h-4 w-4" />
					Reset
				</button>
				<button type="button" class="ghost settings-btn" onclick={() => (settingsOpen = !settingsOpen)}>
					<Settings2Icon class="h-4 w-4" />
					Settings
				</button>
			</div>

			{#if subtitle}
				<div class="control-copy">{subtitle}</div>
			{/if}

			{#if loadError}
				<div class="error">{loadError}</div>
			{/if}
		</div>

		<div class="left-meta">
			<div class="left-meta-title">{title}</div>
			<div class="left-meta-count">{listTotal} found</div>
		</div>

		<div class="result-list">
			{#if loading}
				<div class="empty-state">Loading records…</div>
			{:else if results.length === 0}
				<div class="empty-state">
					<div class="empty-title">{emptyTitle}</div>
					<div class="empty-copy">{emptySubtitle}</div>
				</div>
			{:else if renderMode === 'cards'}
				{#each results as record (record.id)}
					{@const display = mapRecord(record)}
					<button
						type="button"
						class="record-card"
						class:selected={selectedRecordIdInternal === record.id}
						onclick={() => chooseRecord(record)}
					>
						<div class="record-head">
							<div>
								<div class="record-title">{display.title}</div>
								<div class="record-subtitle">{display.subtitle || '—'}</div>
							</div>
							<div class="record-status">{display.status || '—'}</div>
						</div>
						<div class="record-meta">
							{#each display.meta ?? [] as meta}
								<span>{meta}</span>
							{/each}
						</div>
						<p>{display.description || 'Select this record to continue.'}</p>
					</button>
				{/each}
			{:else}
				{#each results as record, idx (record.id)}
					{@const display = mapRecord(record)}
					<button
						type="button"
						class="record-browser-card"
						class:selected={selectedRecordIdInternal === record.id}
						onclick={() => chooseRecord(record)}
					>
						<div class="card-rule" aria-hidden="true"></div>
						<div class="card-body">
							<div class="card-row-top">
								<div class="card-index">
									№ {String((listPage - 1) * effectivePageSize + idx + 1).padStart(3, '0')}
									<span class="card-id">id {record.id}</span>
								</div>
								<div class="card-conf mono" title="Type">{display.badges?.[0] || '—'}</div>
							</div>
							<div class="card-name">{display.title}</div>
							<div class="card-desc">{display.meta?.[0] ? `Doc No: ${display.meta[0]}` : 'Doc No: —'}</div>
							<div class="card-foot">
								<span class="chip chip-quiet">{display.description || formatMaybeDate(record.create_time)}</span>
							</div>
						</div>
					</button>
				{/each}
			{/if}
		</div>

		{#if listTotal > effectivePageSize}
			<div class="list-pager">
				<button class="pager-btn" onclick={prevPage} disabled={listPage <= 1 || loading || recordIdInput.trim() !== ''}
					>‹ Prev</button
				>
				<div class="pager-meta">
					<span>Page {listPage} / {listTotalPages}</span>
					<span>{results.length} on page</span>
					<div class="pager-jump">
						<label for={`kb-browser-jump-${instanceKey}`}>Go</label>
						<input
							id={`kb-browser-jump-${instanceKey}`}
							type="number"
							min="1"
							max={listTotalPages}
							bind:value={listJumpInput}
							disabled={recordIdInput.trim() !== ''}
							onkeydown={(event) => {
								if (event.key === 'Enter') {
									void jumpToPage();
								}
							}}
						/>
						<button class="pager-btn pager-go" onclick={jumpToPage} disabled={loading || recordIdInput.trim() !== ''}>
							Jump
						</button>
					</div>
				</div>
				<button class="pager-btn" onclick={nextPage} disabled={listPage >= listTotalPages || loading || recordIdInput.trim() !== ''}
					>Next ›</button
				>
			</div>
		{/if}
	</div>

	<button
		type="button"
		class="browser-resizer"
		class:active={resizing}
		aria-label="Resize record browser"
		onpointerdown={startResize}
		onkeydown={onResizerKeydown}
	>
		<span class="browser-resizer-grip" aria-hidden="true"></span>
	</button>

	{#if settingsOpen}
		<div class="settings-popover">
			<div class="settings-title">Record Browser Settings</div>
			<label class="field settings-field">
				<span class="field-label">Page size ({effectivePageSize})</span>
				<input
					type="range"
					min="10"
					max="200"
					step="5"
					value={effectivePageSize}
					oninput={(event) => {
						saveSettings({ pageSize: Number((event.currentTarget as HTMLInputElement).value) });
					}}
					onchange={() => void loadRecords(1, selectedRecordIdInternal)}
				/>
			</label>
			<label class="field settings-field">
				<span class="field-label">List width ({listWidth}px)</span>
				<input
					type="range"
					min={KB_INPUT_RECORD_BROWSER_MIN_LIST_WIDTH}
					max={KB_INPUT_RECORD_BROWSER_MAX_LIST_WIDTH}
					step="4"
					value={listWidth}
					oninput={(event) => {
						saveSettings({ listWidth: Number((event.currentTarget as HTMLInputElement).value) });
					}}
				/>
			</label>
			<div class="field settings-field">
				<span class="field-label">Highlight color</span>
				<div class="color-presets">
					{#each KB_INPUT_RECORD_BROWSER_HIGHLIGHT_PRESETS as preset}
						<button
							type="button"
							class="color-swatch"
							class:active={highlightColor === preset}
							style={`background: ${preset}`}
							title={preset}
							onclick={() => saveSettings({ highlightColor: preset })}
						></button>
					{/each}
				</div>
				<label class="color-custom-row">
					<input
						type="color"
						value={highlightColor}
						oninput={(e) => saveSettings({ highlightColor: (e.currentTarget as HTMLInputElement).value })}
					/>
					<span class="color-custom-label">Custom</span>
					<span class="color-custom-hex mono">{highlightColor}</span>
				</label>
			</div>
			<div class="settings-actions">
				<button type="button" class="ghost settings-close" onclick={() => (settingsOpen = false)}>Close</button>
			</div>
		</div>
	{/if}
</div>

<style>
	.browser-shell {
		position: relative;
		min-width: 0;
	}
	.panel {
		display: flex;
		min-height: 0;
		height: 100%;
		flex-direction: column;
		border-radius: 24px;
		border: 1px solid var(--border);
		background: var(--panel);
		overflow: hidden;
	}
	.left-controls { padding: 1rem; border-bottom: 1px solid rgba(148, 163, 184, 0.08); }
	.field { display: block; }
	.field-label {
		display: block;
		margin-bottom: 0.6rem;
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.field-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 0.75rem; }
	.field-row input {
		width: 100%;
		min-width: 0;
		border-radius: 0;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: rgba(31, 41, 55, 0.9);
		padding: 0.95rem 1rem;
		font-size: 0.95rem;
		color: #f3f4f6;
	}
	.ghost {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.45rem;
		border-radius: 14px;
		border: 1px solid rgba(148, 163, 184, 0.16);
		background: rgba(15, 23, 42, 0.36);
		color: var(--text);
		cursor: pointer;
	}
	.ghost:disabled { opacity: 0.45; cursor: not-allowed; }
	.search-btn { padding-inline: 1rem; }
	.action-row { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; gap: 0.6rem; margin-top: 1rem; }
	.retrieve-btn {
		width: 100%;
		border: none;
		border-radius: 14px;
		background: #22c55e;
		color: #111827;
		padding: 0.95rem 1rem;
		font-size: 1rem;
		font-weight: 700;
		cursor: pointer;
	}
	.retrieve-btn:disabled { opacity: 0.7; cursor: default; }
	.retrieve-arrow { margin-right: 0.45rem; }
	.reset-btn, .settings-btn { padding: 0 0.9rem; }
	.control-copy { margin-top: 0.85rem; font-size: 0.82rem; line-height: 1.5; color: var(--muted); }
	.error {
		margin-top: 0.85rem;
		border-radius: 14px;
		border: 1px solid rgba(248, 113, 113, 0.25);
		background: rgba(127, 29, 29, 0.18);
		padding: 0.75rem 0.9rem;
		color: #fecaca;
		font-size: 0.86rem;
	}
	.left-meta {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		padding: 1rem;
		border-bottom: 1px solid rgba(148, 163, 184, 0.08);
	}
	.left-meta-title { font-size: 0.95rem; font-weight: 700; color: #bbf7d0; }
	.left-meta-count {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: #4ade80;
	}
	.result-list {
		display: flex;
		min-height: 0;
		flex: 1;
		flex-direction: column;
		overflow: auto;
		padding: 0 1rem 1rem;
		gap: 0.8rem;
	}
	.empty-state {
		display: grid;
		place-items: center;
		min-height: 260px;
		text-align: center;
		color: var(--muted);
		padding: 1rem;
	}
	.empty-title { font-weight: 700; color: var(--text); }
	.empty-copy { margin-top: 0.45rem; font-size: 0.85rem; line-height: 1.5; }
	.record-browser-card,
	.record-card {
		text-align: left;
		cursor: pointer;
	}
	.record-browser-card {
		display: grid;
		grid-template-columns: 3px minmax(0, 1fr);
		padding: 0;
		border: 1px solid rgba(148, 163, 184, 0.12);
		border-radius: 18px;
		background: rgba(15, 23, 42, 0.28);
		overflow: hidden;
	}
	.record-browser-card.selected,
	.record-card.selected {
		border-color: color-mix(in srgb, var(--highlight) 65%, transparent);
		background: color-mix(in srgb, var(--highlight) 9%, transparent);
		box-shadow: 0 0 0 1px color-mix(in srgb, var(--highlight) 20%, transparent), 0 12px 30px rgba(2, 6, 23, 0.28);
	}
	.card-rule { background: linear-gradient(180deg, color-mix(in srgb, var(--highlight) 40%, transparent), color-mix(in srgb, var(--highlight) 15%, transparent)); }
	.record-browser-card.selected .card-rule { background: linear-gradient(180deg, var(--highlight), color-mix(in srgb, var(--highlight) 55%, transparent)); }
	.card-body { padding: 0.95rem 1rem 1rem; }
	.card-row-top, .record-head {
		display: flex;
		justify-content: space-between;
		gap: 0.75rem;
		align-items: flex-start;
	}
	.card-index, .record-status, .card-conf {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.7rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.card-id { display: block; margin-top: 0.25rem; opacity: 0.7; }
	.card-name, .record-title { margin-top: 0.65rem; font-weight: 700; color: var(--text); }
	.record-subtitle, .card-desc, .record-meta, .record-card p, .card-foot { color: var(--muted); }
	.card-desc { margin-top: 0.4rem; font-size: 0.84rem; }
	.card-foot { margin-top: 0.75rem; }
	.chip {
		display: inline-flex;
		align-items: center;
		padding: 0.24rem 0.48rem;
		border-radius: 999px;
		font-size: 0.72rem;
		background: rgba(15, 23, 42, 0.35);
	}
	.record-card {
		border-radius: 20px;
		border: 1px solid rgba(148, 163, 184, 0.12);
		background: rgba(15, 23, 42, 0.28);
		padding: 1rem;
	}
	.record-meta {
		display: flex;
		gap: 0.6rem;
		flex-wrap: wrap;
		margin-top: 0.6rem;
		font-size: 0.78rem;
	}
	.record-card p { margin: 0.8rem 0 0; font-size: 0.85rem; line-height: 1.5; }
	.list-pager {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
		padding: 10px 16px 14px;
		border-top: 1px solid var(--ink-line-soft);
		background: linear-gradient(180deg, rgba(0, 0, 0, 0), var(--panel-bg));
	}
	.pager-btn {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 11px;
		padding: 6px 10px;
		border: 1px solid var(--ink-line);
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		cursor: pointer;
	}
	.pager-btn:disabled { opacity: 0.45; cursor: not-allowed; }
	.pager-meta {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 10px;
		letter-spacing: 0.05em;
		color: var(--text-muted);
	}
	.pager-jump { display: inline-flex; align-items: center; gap: 6px; margin-top: 4px; }
	.pager-jump label {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 10px;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.pager-jump input {
		width: 58px;
		padding: 4px 6px;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 11px;
		background: var(--panel-bg-alt);
		color: var(--text-primary);
		border: 1px solid var(--ink-line);
	}
	.pager-go { padding: 4px 8px; font-size: 10px; }
	.browser-resizer {
		position: absolute;
		top: 0;
		right: -8px;
		bottom: 0;
		width: 16px;
		padding: 0;
		border: 0;
		background: transparent;
		cursor: col-resize;
		outline: none;
	}
	.browser-resizer::before {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: 7px;
		width: 1px;
		background: var(--ink-line);
		opacity: 0.8;
	}
	.browser-resizer-grip {
		position: absolute;
		top: 50%;
		left: 50%;
		width: 8px;
		height: 52px;
		transform: translate(-50%, -50%);
		border-radius: 999px;
		background: radial-gradient(circle, var(--text-muted) 22%, transparent 24%) center 6px / 6px 12px repeat-y, var(--panel-bg);
		border: 1px solid var(--ink-line-soft);
		box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.14);
	}
	.browser-resizer.active .browser-resizer-grip,
	.browser-resizer:hover .browser-resizer-grip,
	.browser-resizer:focus-visible .browser-resizer-grip {
		border-color: #22c55e;
	}
	.settings-popover {
		position: absolute;
		top: 1rem;
		right: 1rem;
		z-index: 20;
		width: min(320px, calc(100% - 2rem));
		border-radius: 18px;
		border: 1px solid rgba(148, 163, 184, 0.18);
		background: #111827;
		padding: 1rem;
		box-shadow: 0 30px 80px rgba(15, 23, 42, 0.45);
	}
	.settings-title { font-size: 0.9rem; font-weight: 700; color: #f3f4f6; }
	.settings-field { margin-top: 1rem; }
	.settings-field input[type='range'] { width: 100%; }
	.settings-actions { display: flex; justify-content: flex-end; margin-top: 1rem; }
	.settings-close { padding: 0.65rem 0.9rem; }
	.color-presets {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
		margin-top: 0.5rem;
	}
	.color-swatch {
		width: 24px;
		height: 24px;
		border-radius: 50%;
		border: 2px solid transparent;
		cursor: pointer;
		padding: 0;
		transition: transform 0.1s, border-color 0.1s;
	}
	.color-swatch:hover { transform: scale(1.15); }
	.color-swatch.active {
		border-color: #f3f4f6;
		box-shadow: 0 0 0 1px rgba(0,0,0,0.4);
		transform: scale(1.15);
	}
	.color-custom-row {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		margin-top: 0.65rem;
		cursor: pointer;
	}
	.color-custom-row input[type='color'] {
		width: 28px;
		height: 28px;
		border: none;
		border-radius: 50%;
		padding: 0;
		cursor: pointer;
		background: none;
	}
	.color-custom-label { font-size: 0.8rem; color: var(--muted); }
	.color-custom-hex {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.75rem;
		color: var(--muted);
		opacity: 0.7;
	}
	@media (max-width: 980px) {
		.browser-shell { width: 100% !important; }
		.browser-resizer { display: none; }
	}
</style>
