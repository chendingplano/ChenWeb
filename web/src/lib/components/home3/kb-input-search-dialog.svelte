<script lang="ts">
	import { knowledgeStoreState } from './knowledge-store-state.svelte';
	import { listKbInputs, type KbInputRecord } from '$lib/services/kbService';
	import { createDefaultRecordBrowserFilters } from './topic-tree-record-browser.js';

	type RecordBrowserFilters = ReturnType<typeof createDefaultRecordBrowserFilters>;

	let {
		open = $bindable(false),
		onSelect = () => {},
		initialFilters = createDefaultRecordBrowserFilters(),
		scopeToActiveStore = false,
		darkMode = true
	}: {
		open?: boolean;
		onSelect?: (records: KbInputRecord[], filters: RecordBrowserFilters) => void;
		initialFilters?: RecordBrowserFilters;
		scopeToActiveStore?: boolean;
		darkMode?: boolean;
	} = $props();

	// Theme. This dialog is embedded in four hosts. The record-browser host (all nine
	// /home3 views) supplies the letterpress design tokens (--panel-bg, --ink-line,
	// --brass, …) and they cascade into these fixed overlays, so the dialog adopts its
	// host's palette. The other three hosts (doc-processor dashboard, kb-import,
	// document-review) define no such tokens, so the --sd-* aliases fall back to these
	// theme-aware $derived values, driven by the darkMode prop. Every dark branch below
	// equals the literal it replaced, so those three hosts are byte-identical in dark
	// mode. See KnowledgeStore doc-2026071403 / doc-2026071404 for the pattern.
	let panel = $derived(darkMode ? '#111827' : '#FBF8F0');
	let panelAlt = $derived(darkMode ? '#171c26' : '#F0EADB');
	let surface = $derived(darkMode ? '#1a202b' : '#F0EADB');
	let line = $derived(darkMode ? 'rgba(148, 163, 184, 0.16)' : 'rgba(100, 116, 139, 0.18)');
	let lineSoft = $derived(darkMode ? 'rgba(148, 163, 184, 0.12)' : 'rgba(100, 116, 139, 0.12)');
	let text = $derived(darkMode ? '#f3eedf' : '#1A140A');
	let muted = $derived(darkMode ? '#9ca3af' : '#6B6250');
	let subtle = $derived(darkMode ? '#b5ae94' : '#6B6250');
	let accent = $derived(darkMode ? '#d4a24c' : '#B8801E');
	let accentBorder = $derived(darkMode ? '#e0b768' : '#C9922B');
	let errText = $derived(darkMode ? '#fca5a5' : '#991b1b');
	let scopeEmpty = $derived(darkMode ? '#ff7d6b' : '#A23E26');
	let inputBg = $derived(darkMode ? '#2a3140' : '#FFFFFF');
	let resultsBg = $derived(darkMode ? '#121720' : '#F6F1E6');
	let thBg = $derived(darkMode ? '#181d27' : '#EFE8D8');
	let rowHover = $derived(darkMode ? '#1d2330' : '#EDE6D6');
	let ghostBg = $derived(darkMode ? 'rgba(15, 23, 42, 0.36)' : 'rgba(120, 110, 90, 0.10)');
	let chipBg = $derived(darkMode ? 'rgba(255, 255, 255, 0.04)' : 'rgba(120, 110, 90, 0.10)');
	let disabledBg = $derived(darkMode ? '#4a4f5c' : '#D8D2C4');
	let disabledText = $derived(darkMode ? '#aeb4c0' : '#94897A');
	let disabledLine = $derived(darkMode ? '#636b79' : '#C3BBA8');

	// Aliases prefer the host's token and fall back to the theme-aware value above, so a
	// letterpress host repaints the dialog to match while a token-less host still themes.
	let tokenStyle = $derived(
		`--sd-panel: var(--panel-bg, ${panel});` +
		`--sd-panel-alt: var(--panel-bg-alt, ${panelAlt});` +
		`--sd-surface: var(--panel-bg-alt, ${surface});` +
		`--sd-line: var(--ink-line, ${line});` +
		`--sd-line-soft: var(--ink-line-soft, ${lineSoft});` +
		`--sd-text: var(--text-primary, ${text});` +
		`--sd-muted: var(--text-secondary, ${muted});` +
		`--sd-subtle: var(--text-secondary, ${subtle});` +
		`--sd-accent: var(--brass, ${accent});` +
		`--sd-error-text: var(--crimson, ${errText});` +
		`--sd-scope-empty: var(--crimson, ${scopeEmpty});` +
		`--sd-accent-border: ${accentBorder};` +
		`--sd-input-bg: ${inputBg};` +
		`--sd-results-bg: ${resultsBg};` +
		`--sd-th-bg: ${thBg};` +
		`--sd-row-hover: ${rowHover};` +
		`--sd-ghost-bg: ${ghostBg};` +
		`--sd-chip-bg: ${chipBg};` +
		`--sd-disabled-bg: ${disabledBg};` +
		`--sd-disabled-text: ${disabledText};` +
		`--sd-disabled-line: ${disabledLine};`
	);

	const docTypeOptions = ['all', 'pdf', 'doc', 'excel', 'ppt', 'text', 'markdown'];
	const parserNameOptions = ['', 'mineru', 'opendata', 'paddleocr', 'docling'];
	const procStatusOptions = ['all', 'success', 'fail'];
	const operationOptions = [
		{ value: '', label: 'All' },
		{ value: 'parsed_success', label: 'Parsed' },
		{ value: 'parse_failed', label: 'Parse Failed' },
		{ value: 'parsed_not_started', label: 'Parsed Not Started' },
		{ value: 'all_processors_success', label: 'All Processors Success' },
		{ value: 'failed_processors', label: 'Failed Processors' },
		{ value: 'with_not_started_procs', label: 'With Not Started Processors' },
		{ value: 'with_unfinished_procs', label: 'With Unfinished Processors' }
	];

	let searchLoading = $state(false);
	let searchResults = $state<KbInputRecord[]>([]);
	let searchTotal = $state<number | null>(null);
	let searchSelected = $state<Set<number>>(new Set());
	let searchError = $state('');
	let searchRecordId = $state('');
	let searchTitle = $state('');
	let searchDocNo = $state('');
	let searchFileName = $state('');
	let searchDocType = $state('all');
	let searchParserName = $state('');
	let searchOperation = $state('');
	let searchProcStatus = $state('all');
	let searchCreateStart = $state('');
	let searchCreateEnd = $state('');
	let searchModifyStart = $state('');
	let searchModifyEnd = $state('');
	let lastOpenState = $state(false);

	$effect(() => {
		if (open && !lastOpenState) applyFilters(initialFilters);
		lastOpenState = open;
	});

	function resetSearch() {
		applyFilters(createDefaultRecordBrowserFilters());
	}

	function applyFilters(filters: RecordBrowserFilters) {
		searchSelected = new Set();
		searchResults = [];
		searchTotal = null;
		searchError = '';
		searchRecordId = filters.searchRecordId ?? '';
		searchTitle = filters.searchTitle ?? '';
		searchDocNo = filters.searchDocNo ?? '';
		searchFileName = filters.searchFileName ?? '';
		searchDocType = filters.searchDocType ?? 'all';
		searchParserName = filters.searchParserName ?? '';
		searchOperation = filters.searchOperation ?? '';
		searchProcStatus = filters.searchProcStatus ?? 'all';
		searchCreateStart = filters.searchCreateStart ?? '';
		searchCreateEnd = filters.searchCreateEnd ?? '';
		searchModifyStart = filters.searchModifyStart ?? '';
		searchModifyEnd = filters.searchModifyEnd ?? '';
	}

	function currentFilters(): RecordBrowserFilters {
		return {
			searchRecordId,
			searchTitle,
			searchDocNo,
			searchFileName,
			searchDocType,
			searchParserName,
			searchOperation,
			searchProcStatus,
			searchCreateStart,
			searchCreateEnd,
			searchModifyStart,
			searchModifyEnd
		};
	}

	async function runSearch() {
		searchLoading = true;
		searchError = '';
		searchTotal = null;
		searchSelected = new Set();
		try {
			const res = await listKbInputs({
				recordId: searchRecordId,
				docType: searchDocType,
				parseState: 'all',
				title: searchTitle,
				docNo: searchDocNo,
				fileName: searchFileName,
				parserName: searchParserName,
				pipelineFilter: searchOperation || undefined,
				operation: '',
				procStatus: searchOperation ? '' : (searchProcStatus === 'all' ? '' : searchProcStatus),
				startTime: searchCreateStart,
				endTime: searchCreateEnd,
				modifyStartTime: searchModifyStart,
				modifyEndTime: searchModifyEnd,
				page: 1,
				pageSize: 50,
				ksStoreId: scopeToActiveStore ? (knowledgeStoreState.activeStoreId ?? undefined) : undefined
			});
			searchResults = res.results ?? [];
			searchTotal = res.total ?? searchResults.length;
		} catch (err) {
			searchError = err instanceof Error ? err.message : 'Search failed';
		} finally {
			searchLoading = false;
		}
	}

	function closeSearch() {
		open = false;
	}

	function toggleSearchSelection(id: number) {
		const next = new Set(searchSelected);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		searchSelected = next;
	}

	function toggleSelectAll() {
		if (searchSelected.size === searchResults.length && searchResults.length > 0) {
			searchSelected = new Set();
		} else {
			searchSelected = new Set(searchResults.map((r) => r.id));
		}
	}

	function pickSearchResult(record: KbInputRecord) {
		open = false;
		onSelect([record], currentFilters());
	}

	function confirmSearchSelection() {
		const selected = searchResults.filter((r) => searchSelected.has(r.id));
		if (selected.length > 0) {
			open = false;
			onSelect(selected, currentFilters());
		}
	}

	function searchStatusText(record: KbInputRecord): { operation: string; procStatus: string } {
		const items = record.status ?? [];
		const matched = [...items].reverse().find((item) => item != null);
		if (!matched) return { operation: '—', procStatus: 'pending' };
		return {
			operation: matched.operation?.trim() || '—',
			procStatus:
				matched.proc_status?.trim() ||
				matched['proc-status']?.trim() ||
				matched.status?.trim() ||
				'pending'
		};
	}

	function inputDisplayName(record: KbInputRecord) {
		return record.title?.trim() || record.name?.trim() || record.file_name?.trim() || `Record #${record.id}`;
	}

	function inputDisplayDocNo(record: KbInputRecord) {
		return record.doc_no?.trim() || '';
	}

	// ── Status / detail dialog ────────────────────────────────────────────────
	let statusDialogOpen = $state(false);
	let statusDialogRecord = $state<KbInputRecord | null>(null);
	let statusDialogTitle = $state('');
	let statusDialogItems = $state<KbInputRecord['status']>([]);

	// Reset status dialog when the search dialog closes (statusDialogOpen lives outside
	// the {#if open} block, so it would otherwise persist after the parent closes).
	$effect(() => {
		if (!open) {
			statusDialogOpen = false;
			statusDialogRecord = null;
		}
	});

	type FlatRow = { key: string; value: string | null; depth: number };

	function tryParseJsonLike(v: unknown): unknown {
		if (typeof v !== 'string') return v;
		const t = v.trim();
		if ((t.startsWith('{') && t.endsWith('}')) || (t.startsWith('[') && t.endsWith(']'))) {
			try { return JSON.parse(t); } catch { /* noop */ }
		}
		return v;
	}

	function flattenNestedForDisplay(obj: unknown, depth = 0): FlatRow[] {
		const rows: FlatRow[] = [];
		if (obj === null || obj === undefined || typeof obj !== 'object') return rows;
		const entries: Array<[string, unknown]> = Array.isArray(obj)
			? (obj as unknown[]).map((v, i) => [`[${i}]`, v] as [string, unknown])
			: Object.entries(obj as Record<string, unknown>);
		for (const [k, v] of entries) {
			const parsed = tryParseJsonLike(v);
			if (parsed !== null && parsed !== undefined && typeof parsed === 'object') {
				const isArr = Array.isArray(parsed);
				const len = isArr ? (parsed as unknown[]).length : Object.keys(parsed as object).length;
				if (len === 0) {
					rows.push({ key: k, value: isArr ? '[]' : '{}', depth });
				} else if (isArr && (parsed as unknown[]).every(item => item === null || typeof item !== 'object')) {
					rows.push({ key: k, value: (parsed as unknown[]).map(item => item === null ? 'null' : String(item)).join(', '), depth });
				} else {
					rows.push({ key: k, value: null, depth });
					rows.push(...flattenNestedForDisplay(parsed, depth + 1));
				}
			} else {
				rows.push({ key: k, value: v === null || v === undefined ? '—' : String(v).trim() || '—', depth });
			}
		}
		return rows;
	}

	let statusDialogDocMeta = $derived.by((): FlatRow[] => {
		if (!statusDialogRecord) return [];
		const rec = statusDialogRecord as Record<string, unknown>;
		let meta: unknown = rec.doc_metadata;
		if (typeof meta === 'string') {
			try { meta = JSON.parse(meta); } catch { return []; }
		}
		if (!meta || typeof meta !== 'object' || Array.isArray(meta)) return [];
		return flattenNestedForDisplay(meta);
	});

	let statusDialogRecordRows = $derived.by((): FlatRow[] => {
		if (!statusDialogRecord) return [];
		const filtered: Record<string, unknown> = {};
		for (const [k, v] of Object.entries(statusDialogRecord as Record<string, unknown>)) {
			if (k !== 'status' && k !== 'doc_metadata') filtered[k] = v;
		}
		return flattenNestedForDisplay(filtered);
	});

	function openStatusDialog(record: KbInputRecord) {
		statusDialogRecord = record;
		statusDialogItems = record.status ?? [];
		statusDialogTitle = `Record ID: ${record.id}`;
		statusDialogOpen = true;
	}

	function closeStatusDialog() {
		statusDialogOpen = false;
		statusDialogRecord = null;
	}
</script>

{#if open}
	<div class="dialog-overlay" style={tokenStyle}>
		<div
			class="dialog"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			tabindex="0"
		>
			<div class="dialog-head">
				<div>
					<div class="dialog-eyebrow">kb.inputs</div>
					<h2 class="dialog-title">Find a record</h2>
					<p class="dialog-subtitle">
						Search by record metadata, parser pipeline state, and create or modify windows.
					</p>
					<div class="scope-copy">
						Store scope:
						{#if scopeToActiveStore && knowledgeStoreState.activeStore}
							<span class="scope-store">{knowledgeStoreState.activeStore.ks_name}</span>
							<span class="scope-id">#{knowledgeStoreState.activeStore.id}</span>
						{:else}
							<span class="scope-empty">All stores</span>
						{/if}
					</div>
				</div>
			</div>

			<div class="dialog-scroll">
				<div class="dialog-controls">
					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Identity</div>
							<div class="dialog-section-copy">Match the record itself and its document metadata.</div>
						</div>
						<div class="dialog-grid dialog-grid-primary">
							<label class="field dialog-field">
								<span class="field-label">Record ID</span>
								<input type="text" bind:value={searchRecordId} placeholder="84" onkeydown={(e) => {
									if (e.key === 'Enter') void runSearch();
								}} />
							</label>
							<label class="field dialog-field">
								<span class="field-label">Type</span>
								<select bind:value={searchDocType}>
									{#each docTypeOptions as option}
										<option value={option}>{option}</option>
									{/each}
								</select>
							</label>
							<label class="field dialog-field dialog-field-wide">
								<span class="field-label">Title contains</span>
								<input type="text" bind:value={searchTitle} placeholder="Input title, standard title…" onkeydown={(e) => {
									if (e.key === 'Enter') void runSearch();
								}} />
							</label>
							<label class="field dialog-field">
								<span class="field-label">Doc No contains</span>
								<input type="text" bind:value={searchDocNo} placeholder="GB/T 123…" onkeydown={(e) => {
									if (e.key === 'Enter') void runSearch();
								}} />
							</label>
							<label class="field dialog-field dialog-field-wide">
								<span class="field-label">File name contains</span>
								<input type="text" bind:value={searchFileName} placeholder="report, spec, drawing…" onkeydown={(e) => {
									if (e.key === 'Enter') void runSearch();
								}} />
							</label>
						</div>
					</div>

					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Processing Status</div>
							<div class="dialog-section-copy">
								Filter the pipeline entry by parser, operation, and final `proc_status`.
							</div>
						</div>
						<div class="dialog-grid dialog-grid-time">
							<label class="field dialog-field">
								<span class="field-label">Parser name</span>
								<select bind:value={searchParserName}>
									{#each parserNameOptions as opt}
										<option value={opt}>{opt === '' ? 'All' : opt}</option>
									{/each}
								</select>
							</label>
							<label class="field dialog-field">
								<span class="field-label">Operation</span>
								<select bind:value={searchOperation} onchange={() => { if (searchOperation) searchProcStatus = 'all'; }}>
									{#each operationOptions as opt}
										<option value={opt.value}>{opt.label}</option>
									{/each}
								</select>
							</label>
							{#if !searchOperation}
								<label class="field dialog-field">
									<span class="field-label">Proc status</span>
									<select bind:value={searchProcStatus}>
										{#each procStatusOptions as option}
											<option value={option}>{option}</option>
										{/each}
									</select>
								</label>
							{/if}
						</div>
					</div>

					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Time Windows</div>
							<div class="dialog-section-copy">
								Search by create and modify timestamps using local date-time ranges.
							</div>
						</div>
						<div class="dialog-grid">
							<label class="field dialog-field"><span class="field-label">Create time from</span><input type="datetime-local" bind:value={searchCreateStart} /></label>
							<label class="field dialog-field"><span class="field-label">Create time to</span><input type="datetime-local" bind:value={searchCreateEnd} /></label>
							<label class="field dialog-field"><span class="field-label">Modify time from</span><input type="datetime-local" bind:value={searchModifyStart} /></label>
							<label class="field dialog-field"><span class="field-label">Modify time to</span><input type="datetime-local" bind:value={searchModifyEnd} /></label>
						</div>
					</div>

					<div class="dialog-toolbar">
						<div class="dialog-toolbar-copy">
							<div class="dialog-toolbar-title">Search Scope</div>
							<div class="dialog-toolbar-text">
								Leave fields empty to broaden the search. Results are capped to the newest 50 records.
							</div>
						</div>
						<div class="dialog-toolbar-actions">
							<button class="btn btn-ghost" type="button" onclick={resetSearch}>Reset</button>
							<button class="btn btn-primary dialog-search-btn" type="button" onclick={runSearch} disabled={searchLoading}>
								{searchLoading ? 'Searching…' : 'Search'}
							</button>
						</div>
					</div>
				</div>

				{#if searchError}
					<div class="error dialog-error">{searchError}</div>
				{/if}

				{#if searchTotal !== null && !searchLoading}
					<div class="results-count">
						{#if searchTotal === 0}
							No records matched.
						{:else if searchResults.length < searchTotal}
							Showing <strong>{searchResults.length}</strong> of <strong>{searchTotal}</strong> records
						{:else}
							<strong>{searchTotal}</strong> {searchTotal === 1 ? 'record' : 'records'} matched
						{/if}
					</div>
				{/if}

				<div class="dialog-results">
					{#if searchResults.length === 0 && !searchLoading}
						<div class="dialog-empty">
							<div class="empty-glyph">⌕</div>
							<div class="dialog-empty-title">Run a search to see records.</div>
							<div class="dialog-empty-copy">
								Use any combination of metadata, parser, and time filters to narrow the archive.
							</div>
						</div>
					{:else}
						<table class="result-table">
							<thead>
								<tr>
									<th class="col-check">
										<input
											type="checkbox"
											class="row-check"
											checked={searchSelected.size === searchResults.length && searchResults.length > 0}
											indeterminate={searchSelected.size > 0 && searchSelected.size < searchResults.length}
											onchange={toggleSelectAll}
											title="Select all"
										/>
									</th>
									<th>ID</th>
									<th>Type</th>
									<th>Title / Doc No</th>
									<th>File name</th>
									<th>Status</th>
									<th>Created</th>
									<th>Updated</th>
									<th>Detail</th>
								</tr>
							</thead>
							<tbody>
								{#each searchResults as record (record.id)}
									{@const statusSummary = searchStatusText(record)}
									<tr
										class:selected={searchSelected.has(record.id)}
										onclick={() => toggleSearchSelection(record.id)}
										ondblclick={() => pickSearchResult(record)}
									>
										<td class="col-check">
											<input
												type="checkbox"
												class="row-check"
												checked={searchSelected.has(record.id)}
												onclick={(e) => e.stopPropagation()}
												onchange={() => toggleSearchSelection(record.id)}
											/>
										</td>
										<td class="mono">{record.id}</td>
										<td>{record.type}</td>
										<td>
											<div class="result-primary">{inputDisplayName(record)}</div>
											<div class="result-secondary mono">{inputDisplayDocNo(record) || '—'}</div>
										</td>
										<td class="ellipsis">{record.file_name ?? '—'}</td>
										<td>
											<div class="status-stack">
												<span class="status-chip mono">{statusSummary.operation}</span>
												<span
													class="status-pill"
													class:status-pill-success={statusSummary.procStatus.toLowerCase() === 'success'}
													class:status-pill-fail={statusSummary.procStatus.toLowerCase() === 'fail' ||
														statusSummary.procStatus.toLowerCase() === 'failed'}
												>
													{statusSummary.procStatus}
												</span>
											</div>
										</td>
										<td class="mono muted">{(record.create_time ?? '').slice(0, 19).replace('T', ' ')}</td>
										<td class="mono muted">{(record.modify_time ?? '').slice(0, 19).replace('T', ' ')}</td>
										<td>
											<button
												class="view-btn"
												onclick={(e) => { e.stopPropagation(); openStatusDialog(record); }}
											>
												View
											</button>
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
				</div>
			</div>

			<div class="dialog-foot">
				<div class="dialog-foot-hint">Click to toggle, double-click to pick one. Header checkbox selects all.</div>
				<div class="dialog-foot-buttons">
					<button class="btn btn-ghost" type="button" onclick={closeSearch}>Cancel</button>
					<button class="btn btn-primary dialog-select-btn" type="button" onclick={confirmSearchSelection} disabled={searchSelected.size === 0}>
						{searchSelected.size > 0 ? `Select (${searchSelected.size})` : 'Select'}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

{#if statusDialogOpen}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="view-dialog-overlay"
		style={tokenStyle}
		onmousedown={(e) => { if (e.target === e.currentTarget) closeStatusDialog(); }}
		onkeydown={(e) => { if (e.key === 'Escape') closeStatusDialog(); }}
		role="button"
		tabindex="0"
	>
		<div
			class="view-dialog"
			onmousedown={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			aria-label="Record details"
			tabindex="0"
		>
			<div class="view-dialog-head">
				<h3 class="view-dialog-title">{statusDialogTitle}</h3>
				<button class="btn btn-ghost view-close-btn" onclick={closeStatusDialog}>Close</button>
			</div>
			<div class="view-dialog-body">
				{#if statusDialogRecord}
					<div class="view-section">
						<div class="view-section-label">Record Fields</div>
						<div class="view-rows-box">
							{#each statusDialogRecordRows as row}
								<div class="view-row" style="padding-left:{row.depth * 16}px;">
									<span class="view-key" class:view-key-header={row.value === null}>{row.key}</span>
									{#if row.value !== null}
										<span class="view-val">{row.value}</span>
									{/if}
								</div>
							{/each}
						</div>
					</div>

					<div class="view-section">
						<div class="view-section-label">
							Doc Metadata ({statusDialogDocMeta.filter(r => r.depth === 0).length} {statusDialogDocMeta.filter(r => r.depth === 0).length === 1 ? 'field' : 'fields'})
						</div>
						{#if statusDialogDocMeta.length === 0}
							<div class="view-empty">No doc_metadata available.</div>
						{:else}
							<div class="view-rows-box">
								{#each statusDialogDocMeta as row}
									<div class="view-row" style="padding-left:{row.depth * 16}px;">
										<span class="view-key" class:view-key-header={row.value === null}>{row.key}</span>
										{#if row.value !== null}
											<span class="view-val">{row.value}</span>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>

					<div class="view-section">
						<div class="view-section-label">
							Status ({statusDialogItems.length} {statusDialogItems.length === 1 ? 'entry' : 'entries'})
						</div>
						{#if statusDialogItems.length === 0}
							<div class="view-empty">No status entries.</div>
						{:else}
							<div class="view-entries">
								{#each statusDialogItems as item, idx}
									<div class="view-entry">
										<div class="view-entry-head">Entry #{idx + 1}</div>
										<div class="view-entry-grid">
											{#each Object.entries(item as Record<string, unknown>) as [key, val]}
												<span class="view-key">{key}</span>
												<span class="view-val">
													{val === null || val === undefined ? '—' : typeof val === 'object' ? JSON.stringify(val) : String(val)}
												</span>
											{/each}
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}

<style>
	@import url('https://fonts.googleapis.com/css2?family=Cormorant+Garamond:wght@400;500;600&family=JetBrains+Mono:wght@400;500;600&family=Inter+Tight:wght@400;500;600&display=swap');
	.dialog-overlay {
		position: fixed;
		inset: 0;
		z-index: 25;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 0.75rem;
		background: rgba(2, 6, 23, 0.68);
		backdrop-filter: blur(10px);
	}

	.dialog {
		width: min(1440px, 100%);
		max-height: min(96vh, 1320px);
		display: flex;
		flex-direction: column;
		overflow: hidden;
		border-radius: 24px;
		border: 1px solid var(--sd-line);
		background: var(--sd-panel);
		color: var(--sd-text);
		font-family: 'Inter Tight', system-ui, sans-serif;
	}

	.dialog-head {
		padding: 28px 32px 20px;
		border-bottom: 1px solid var(--sd-line-soft);
	}

	.dialog-eyebrow,
	.field-label,
	.dialog-section-title,
	.dialog-toolbar-title,
	.result-table thead th {
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		font-weight: 600;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--sd-muted);
	}

	.dialog-title {
		margin: 0;
		font-family: 'Cormorant Garamond', Georgia, serif;
		font-size: 28px;
		font-weight: 500;
		color: var(--sd-text);
	}

	.dialog-subtitle {
		margin: 8px 0 0;
		max-width: 640px;
		font-size: 13px;
		line-height: 1.45;
		color: var(--sd-subtle);
	}

	.scope-copy {
		margin-top: 8px;
		font-size: 11px;
		font-family: 'JetBrains Mono', monospace;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--sd-muted);
	}

	.scope-store {
		color: var(--sd-accent);
		font-weight: 600;
	}

	.scope-id {
		opacity: 0.55;
	}

	.scope-empty {
		color: var(--sd-scope-empty);
		text-transform: none;
		letter-spacing: 0;
	}

	.dialog-scroll {
		display: flex;
		flex: 1;
		flex-direction: column;
		min-height: 0;
		overflow: auto;
	}

	.dialog-controls {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 16px 28px 12px;
		background: linear-gradient(180deg, rgba(255, 255, 255, 0.025), rgba(0, 0, 0, 0.03));
		flex: 0 0 auto;
	}

	.dialog-section {
		border: 1px solid rgba(212, 162, 76, 0.16);
		border-radius: 20px;
		padding: 14px;
		background:
			linear-gradient(180deg, rgba(255, 255, 255, 0.04), rgba(255, 255, 255, 0.01)),
			var(--sd-panel-alt);
	}

	.dialog-section-head {
		display: flex;
		justify-content: space-between;
		gap: 16px;
		margin-bottom: 10px;
		align-items: baseline;
	}

	.dialog-section-copy,
	.dialog-toolbar-text {
		font-size: 12px;
		color: var(--sd-subtle);
	}

	.dialog-grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 10px;
	}

	.dialog-grid-time {
		grid-template-columns: repeat(4, minmax(0, 1fr));
	}

	.dialog-grid-primary {
		grid-template-columns: 0.8fr 0.8fr 1.2fr 1fr 1.4fr;
	}

	.dialog-field {
		display: flex;
		flex-direction: column;
		gap: 8px;
		margin: 0;
		padding: 10px 10px 8px;
		border-radius: 16px;
		border: 1px solid var(--sd-line-soft);
		background: var(--sd-surface);
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
	}

	.dialog-field-wide {
		grid-column: span 2;
	}

	.dialog-field :global(input),
	.dialog-field :global(select) {
		height: 42px;
		border: 1px solid var(--sd-line);
		background: var(--sd-input-bg);
		padding: 0 12px;
		color: var(--sd-text);
		font-size: 14px;
	}

	.dialog-field :global(input:focus),
	.dialog-field :global(select:focus) {
		outline: none;
		border-color: var(--sd-accent);
		box-shadow:
			0 0 0 1px rgba(212, 162, 76, 0.28),
			0 0 0 4px rgba(212, 162, 76, 0.08);
	}

	.dialog-toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
		padding: 0 2px;
	}

	.dialog-toolbar-actions,
	.dialog-foot-buttons {
		display: flex;
		gap: 10px;
		align-items: center;
	}

	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		border-radius: 0;
		padding: 0.8rem 1rem;
		border: 1px solid var(--sd-line);
		cursor: pointer;
	}

	.btn-ghost {
		background: var(--sd-ghost-bg);
		color: var(--sd-text);
	}

	.btn-primary,
	.dialog-search-btn,
	.dialog-select-btn {
		background: var(--sd-accent) !important;
		color: #15110a !important;
		border: 1px solid var(--sd-accent-border) !important;
	}

	.dialog-search-btn {
		min-width: 140px;
		box-shadow: 0 8px 20px rgba(212, 162, 76, 0.22);
		opacity: 1 !important;
	}

	.dialog-select-btn {
		min-width: 132px;
		opacity: 1 !important;
	}

	.dialog-select-btn:disabled {
		background: var(--sd-disabled-bg) !important;
		color: var(--sd-disabled-text) !important;
		border: 1px solid var(--sd-disabled-line) !important;
		box-shadow: none !important;
		cursor: not-allowed !important;
	}

	.dialog-error {
		margin: 0 32px 16px;
		color: var(--sd-error-text);
	}

	.results-count {
		padding: 6px 28px;
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--sd-muted);
		background: var(--sd-panel);
		border-top: 1px solid var(--sd-line);
		flex: 0 0 auto;
	}

	.results-count strong {
		color: var(--sd-accent);
		font-weight: 600;
	}

	.dialog-results {
		flex: 1 1 320px;
		border-top: 1px solid var(--sd-line);
		border-bottom: 1px solid var(--sd-line);
		min-height: 460px;
		background: var(--sd-results-bg);
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
		overflow: auto;
	}

	.dialog-empty {
		padding: 72px 20px;
		text-align: center;
		color: var(--sd-muted);
	}

	.empty-glyph {
		font-size: 48px;
		color: var(--sd-accent);
	}

	.dialog-empty-title {
		font-size: 18px;
		color: var(--sd-text);
		margin-top: 10px;
	}

	.dialog-empty-copy {
		max-width: 420px;
		margin: 10px auto 0;
		font-size: 13px;
		line-height: 1.6;
	}

	.result-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}

	.result-table thead th {
		font-family: 'JetBrains Mono', monospace;
		text-align: left;
		padding: 12px 16px;
		border-bottom: 1px solid var(--sd-line-soft);
		background: var(--sd-th-bg);
		position: sticky;
		top: 0;
		z-index: 1;
	}

	.col-check {
		width: 40px;
		padding: 0 8px 0 16px !important;
		text-align: center !important;
	}

	.row-check {
		width: 15px;
		height: 15px;
		accent-color: var(--sd-accent);
		cursor: pointer;
	}

	.result-table tbody tr {
		border-bottom: 1px solid var(--sd-line-soft);
		cursor: pointer;
		transition: background 120ms;
	}

	.result-table tbody tr:hover {
		background: var(--sd-row-hover);
	}

	.result-table tbody tr.selected {
		background: rgba(212, 162, 76, 0.16);
	}

	.result-table tbody tr.selected td {
		color: var(--sd-text);
	}

	.result-table td {
		padding: 11px 16px;
		color: var(--sd-text);
		vertical-align: top;
	}

	.mono {
		font-family: 'JetBrains Mono', monospace;
		font-size: 12px;
	}

	.muted {
		color: var(--sd-muted);
	}

	.ellipsis {
		max-width: 360px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.result-primary {
		font-weight: 600;
		color: var(--sd-text);
	}

	.result-secondary {
		margin-top: 4px;
		color: var(--sd-muted);
	}

	.status-stack {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 6px;
	}

	.status-chip {
		padding: 2px 8px;
		border-radius: 999px;
		border: 1px solid var(--sd-line);
		background: var(--sd-chip-bg);
		color: var(--sd-subtle);
	}

	.status-pill {
		padding: 4px 10px;
		border-radius: 999px;
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		background: rgba(124, 117, 96, 0.16);
		color: var(--sd-muted);
	}

	.status-pill-success {
		background: rgba(93, 175, 168, 0.16);
		color: #5dafa8;
	}

	.status-pill-fail {
		background: rgba(200, 85, 61, 0.18);
		color: #c8553d;
	}

	.dialog-foot {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
		padding: 14px 28px 18px;
		background: var(--sd-panel-alt);
	}

	.dialog-foot-hint {
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		color: var(--sd-muted);
	}

	@media (max-width: 980px) {
		.dialog-grid,
		.dialog-grid-primary,
		.dialog-grid-time {
			grid-template-columns: 1fr;
		}

		.dialog-toolbar,
		.dialog-foot,
		.dialog-section-head {
			flex-direction: column;
			align-items: stretch;
		}

		.dialog-field-wide {
			grid-column: auto;
		}
	}

	.view-btn {
		height: 24px;
		padding: 0 10px;
		border: 1px solid var(--sd-line);
		border-radius: 6px;
		background: var(--sd-chip-bg);
		color: var(--sd-muted);
		font-size: 11px;
		font-family: 'JetBrains Mono', monospace;
		cursor: pointer;
		transition: background 100ms, color 100ms, border-color 100ms;
	}

	.view-btn:hover {
		background: rgba(212, 162, 76, 0.12);
		color: var(--sd-accent);
		border-color: rgba(212, 162, 76, 0.3);
	}

	.view-dialog-overlay {
		position: fixed;
		inset: 0;
		z-index: 50;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 0.75rem;
		background: rgba(2, 6, 23, 0.72);
		backdrop-filter: blur(10px);
	}

	.view-dialog {
		display: flex;
		flex-direction: column;
		width: min(1046px, calc(100vw - 48px));
		max-height: calc(100vh - 48px);
		overflow: hidden;
		resize: both;
		min-width: 480px;
		min-height: 200px;
		border-radius: 20px;
		border: 1px solid var(--sd-line);
		background: var(--sd-panel);
		color: var(--sd-text);
		font-family: 'Inter Tight', system-ui, sans-serif;
	}

	.view-dialog-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 16px 24px;
		border-bottom: 1px solid var(--sd-line-soft);
		flex-shrink: 0;
	}

	.view-dialog-title {
		margin: 0;
		font-size: 15px;
		font-weight: 600;
		color: var(--sd-text);
		font-family: 'JetBrains Mono', monospace;
	}

	.view-close-btn {
		height: 30px;
		padding: 0 12px;
		font-size: 12px;
	}

	.view-dialog-body {
		flex: 1;
		overflow-y: auto;
		min-height: 0;
		padding: 16px 24px;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.view-section-label {
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--sd-muted);
		margin-bottom: 6px;
	}

	.view-rows-box {
		border: 1px solid var(--sd-line-soft);
		border-radius: 12px;
		padding: 8px 12px;
		background: var(--sd-surface);
	}

	.view-row {
		display: flex;
		align-items: baseline;
		min-height: 22px;
		gap: 8px;
		padding: 2px 0;
	}

	.view-key {
		font-size: 12px;
		width: 140px;
		flex-shrink: 0;
		word-break: break-all;
		color: var(--sd-muted);
		font-family: 'JetBrains Mono', monospace;
	}

	.view-key-header {
		color: var(--sd-subtle);
		font-weight: 600;
	}

	.view-val {
		font-size: 12px;
		color: var(--sd-text);
		word-break: break-word;
		flex: 1;
		min-width: 0;
	}

	.view-empty {
		font-size: 12px;
		color: var(--sd-muted);
	}

	.view-entries {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.view-entry {
		border: 1px solid var(--sd-line-soft);
		border-radius: 12px;
		padding: 8px 12px;
		background: var(--sd-surface);
	}

	.view-entry-head {
		font-size: 12px;
		font-weight: 600;
		color: var(--sd-text);
		margin-bottom: 6px;
		font-family: 'JetBrains Mono', monospace;
	}

	.view-entry-grid {
		display: grid;
		grid-template-columns: 160px 1fr;
		gap: 4px;
	}
</style>
