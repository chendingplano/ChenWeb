<script lang="ts">
	import { knowledgeStoreState } from './knowledge-store-state.svelte';
	import { listKbInputs, type KbInputRecord } from '$lib/services/kbService';
	import { createDefaultRecordBrowserFilters } from './topic-tree-record-browser.js';

	type RecordBrowserFilters = ReturnType<typeof createDefaultRecordBrowserFilters>;

	let {
		open = $bindable(false),
		onSelect = () => {},
		initialFilters = createDefaultRecordBrowserFilters(),
		scopeToActiveStore = false
	}: {
		open?: boolean;
		onSelect?: (record: KbInputRecord, filters: RecordBrowserFilters) => void;
		initialFilters?: RecordBrowserFilters;
		scopeToActiveStore?: boolean;
	} = $props();

	const docTypeOptions = ['all', 'pdf', 'doc', 'excel', 'ppt', 'text', 'markdown'];
	const procStatusOptions = ['all', 'success', 'fail'];

	let searchLoading = $state(false);
	let searchResults = $state<KbInputRecord[]>([]);
	let searchSelected = $state<number | null>(null);
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
		searchSelected = null;
		searchResults = [];
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
		try {
			const res = await listKbInputs({
				recordId: searchRecordId,
				docType: searchDocType,
				parseState: 'all',
				title: searchTitle,
				docNo: searchDocNo,
				fileName: searchFileName,
				parserName: searchParserName,
				operation: searchOperation,
				procStatus: searchProcStatus === 'all' ? '' : searchProcStatus,
				startTime: searchCreateStart,
				endTime: searchCreateEnd,
				modifyStartTime: searchModifyStart,
				modifyEndTime: searchModifyEnd,
				page: 1,
				pageSize: 50,
				ksStoreId: scopeToActiveStore ? (knowledgeStoreState.activeStoreId ?? undefined) : undefined
			});
			searchResults = res.results ?? [];
		} catch (err) {
			searchError = err instanceof Error ? err.message : 'Search failed';
		} finally {
			searchLoading = false;
		}
	}

	function closeSearch() {
		open = false;
	}

	function pickSearchResult(record: KbInputRecord) {
		open = false;
		onSelect(record, currentFilters());
	}

	function confirmSearchSelection() {
		const record = searchResults.find((x) => x.id === searchSelected);
		if (record) pickSearchResult(record);
	}

	function searchStatusText(record: KbInputRecord): { operation: string; procStatus: string } {
		const items = record.status ?? [];
		const desiredOperation = searchOperation.trim().toLowerCase();
		const matched =
			desiredOperation !== ''
				? [...items]
						.reverse()
						.find((item) => (item?.operation ?? '').trim().toLowerCase() === desiredOperation)
				: [...items].reverse().find((item) => item != null);

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
</script>

{#if open}
	<div class="dialog-overlay" aria-hidden="true">
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
								<input type="text" bind:value={searchParserName} placeholder="mineru, pdf-parser…" onkeydown={(e) => {
									if (e.key === 'Enter') void runSearch();
								}} />
							</label>
							<label class="field dialog-field">
								<span class="field-label">Operation</span>
								<input type="text" bind:value={searchOperation} placeholder="extract_metadata" onkeydown={(e) => {
									if (e.key === 'Enter') void runSearch();
								}} />
							</label>
							<label class="field dialog-field">
								<span class="field-label">Proc status</span>
								<select bind:value={searchProcStatus}>
									{#each procStatusOptions as option}
										<option value={option}>{option}</option>
									{/each}
								</select>
							</label>
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
									<th>ID</th>
									<th>Type</th>
									<th>Title / Doc No</th>
									<th>File name</th>
									<th>Parser</th>
									<th>Status</th>
									<th>Created</th>
									<th>Updated</th>
								</tr>
							</thead>
							<tbody>
								{#each searchResults as record (record.id)}
									{@const statusSummary = searchStatusText(record)}
									<tr
										class:selected={searchSelected === record.id}
										onclick={() => (searchSelected = record.id)}
										ondblclick={() => pickSearchResult(record)}
									>
										<td class="mono">{record.id}</td>
										<td>{record.type}</td>
										<td>
											<div class="result-primary">{inputDisplayName(record)}</div>
											<div class="result-secondary mono">{inputDisplayDocNo(record) || '—'}</div>
										</td>
										<td class="ellipsis">{record.file_name ?? '—'}</td>
										<td class="mono muted">{record.parser_name || '—'}</td>
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
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
				</div>
			</div>

			<div class="dialog-foot">
				<div class="dialog-foot-hint">Double-click a row, or select and press Select.</div>
				<div class="dialog-foot-buttons">
					<button class="btn btn-ghost" type="button" onclick={closeSearch}>Cancel</button>
					<button class="btn btn-primary dialog-select-btn" type="button" onclick={confirmSearchSelection} disabled={searchSelected == null}>
						Select
					</button>
				</div>
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
		padding: 1.5rem;
		background: rgba(2, 6, 23, 0.68);
		backdrop-filter: blur(10px);
	}

	.dialog {
		width: min(1180px, 100%);
		max-height: min(88vh, 980px);
		display: flex;
		flex-direction: column;
		overflow: hidden;
		border-radius: 24px;
		border: 1px solid rgba(148, 163, 184, 0.16);
		background: #111827;
		color: #f3eedf;
		font-family: 'Inter Tight', system-ui, sans-serif;
	}

	.dialog-head {
		padding: 28px 32px 20px;
		border-bottom: 1px solid rgba(148, 163, 184, 0.12);
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
		color: #9ca3af;
	}

	.dialog-title {
		margin: 0;
		font-family: 'Cormorant Garamond', Georgia, serif;
		font-size: 28px;
		font-weight: 500;
		color: #f3eedf;
	}

	.dialog-subtitle {
		margin: 8px 0 0;
		max-width: 640px;
		font-size: 13px;
		line-height: 1.45;
		color: #b5ae94;
	}

	.scope-copy {
		margin-top: 8px;
		font-size: 11px;
		font-family: 'JetBrains Mono', monospace;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: #9ca3af;
	}

	.scope-store {
		color: #d4a24c;
		font-weight: 600;
	}

	.scope-id {
		opacity: 0.55;
	}

	.scope-empty {
		color: #ff7d6b;
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
			#171c26;
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
		color: #b5ae94;
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
		border: 1px solid rgba(255, 255, 255, 0.06);
		background: #1a202b;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
	}

	.dialog-field-wide {
		grid-column: span 2;
	}

	.dialog-field :global(input),
	.dialog-field :global(select) {
		height: 42px;
		border: 1px solid rgba(255, 255, 255, 0.08);
		background: #2a3140;
		padding: 0 12px;
		color: #f3eedf;
		font-size: 14px;
	}

	.dialog-field :global(input:focus),
	.dialog-field :global(select:focus) {
		outline: none;
		border-color: #d4a24c;
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
		border: 1px solid rgba(148, 163, 184, 0.16);
		cursor: pointer;
	}

	.btn-ghost {
		background: rgba(15, 23, 42, 0.36);
		color: #f3eedf;
	}

	.btn-primary,
	.dialog-search-btn,
	.dialog-select-btn {
		background: #d4a24c !important;
		color: #15110a !important;
		border: 1px solid #e0b768 !important;
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
		background: #4a4f5c !important;
		color: #aeb4c0 !important;
		border: 1px solid #636b79 !important;
		box-shadow: none !important;
		cursor: not-allowed !important;
	}

	.dialog-error {
		margin: 0 32px 16px;
		color: #fca5a5;
	}

	.dialog-results {
		flex: 1 1 320px;
		border-top: 1px solid #1f2530;
		border-bottom: 1px solid #1f2530;
		min-height: 360px;
		background: #121720;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
		overflow: auto;
	}

	.dialog-empty {
		padding: 72px 20px;
		text-align: center;
		color: #9ca3af;
	}

	.empty-glyph {
		font-size: 48px;
		color: #d4a24c;
	}

	.dialog-empty-title {
		font-size: 18px;
		color: #f3eedf;
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
		border-bottom: 1px solid rgba(148, 163, 184, 0.12);
		background: #181d27;
		position: sticky;
		top: 0;
		z-index: 1;
	}

	.result-table tbody tr {
		border-bottom: 1px solid rgba(148, 163, 184, 0.08);
		cursor: pointer;
		transition: background 120ms;
	}

	.result-table tbody tr:hover {
		background: #1d2330;
	}

	.result-table tbody tr.selected {
		background: rgba(212, 162, 76, 0.16);
	}

	.result-table tbody tr.selected td {
		color: #f4ddb0;
	}

	.result-table td {
		padding: 11px 16px;
		color: #f3eedf;
		vertical-align: top;
	}

	.mono {
		font-family: 'JetBrains Mono', monospace;
		font-size: 12px;
	}

	.muted {
		color: #9ca3af;
	}

	.ellipsis {
		max-width: 360px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.result-primary {
		font-weight: 600;
		color: #f3eedf;
	}

	.result-secondary {
		margin-top: 4px;
		color: #9ca3af;
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
		border: 1px solid rgba(255, 255, 255, 0.08);
		background: rgba(255, 255, 255, 0.04);
		color: #d7cfbb;
	}

	.status-pill {
		padding: 4px 10px;
		border-radius: 999px;
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		background: rgba(124, 117, 96, 0.16);
		color: #d1d5db;
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
		background: #171c26;
	}

	.dialog-foot-hint {
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		color: #9ca3af;
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
</style>
