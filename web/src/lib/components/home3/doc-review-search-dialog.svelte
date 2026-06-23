<script lang="ts">
	import { listRequests } from '$lib/services/docReviewService';
	import type { RequestListFilter, RequestListItem, TierInfo } from '$lib/services/docReviewService';

	let {
		open = $bindable(false),
		tiers = [],
		onSelect = () => {}
	}: {
		open?: boolean;
		tiers?: TierInfo[];
		// Called with the requests the user checked and confirmed via "Select".
		onSelect?: (records: RequestListItem[]) => void;
	} = $props();

	const statusOptions = [
		{ value: 'all', label: 'All' },
		{ value: 'accepted', label: 'Accepted' },
		{ value: 'running', label: 'Running' },
		{ value: 'completed', label: 'Completed' },
		{ value: 'failed', label: 'Failed' },
		{ value: 'stopped', label: 'Stopped' }
	];

	let requestId = $state('');
	let title = $state('');
	let requester = $state('');
	let tier = $state('all');
	let status = $state('all');
	let createStart = $state('');
	let createEnd = $state('');

	let searchLoading = $state(false);
	let searchResults = $state<RequestListItem[]>([]);
	let searchTotal = $state<number | null>(null);
	let searchSelected = $state<Set<number>>(new Set());
	let searchError = $state('');
	let lastOpenState = $state(false);

	const tierLabels = $derived.by(() => {
		const m: Record<string, string> = {};
		for (const t of tiers) m[t.key] = t.label;
		return m;
	});

	// Reset the dialog each time it opens fresh.
	$effect(() => {
		if (open && !lastOpenState) resetSearch();
		lastOpenState = open;
	});

	function currentFilter(): RequestListFilter {
		return { requestId, title, requester, tier, status, createStart, createEnd };
	}

	async function runSearch() {
		searchLoading = true;
		searchError = '';
		searchTotal = null;
		searchSelected = new Set();
		try {
			searchResults = await listRequests(currentFilter());
			searchTotal = searchResults.length;
		} catch (err) {
			searchError = err instanceof Error ? err.message : 'Search failed';
		} finally {
			searchLoading = false;
		}
	}

	function resetSearch() {
		requestId = '';
		title = '';
		requester = '';
		tier = 'all';
		status = 'all';
		createStart = '';
		createEnd = '';
		searchResults = [];
		searchTotal = null;
		searchSelected = new Set();
		searchError = '';
	}

	function closeDialog() {
		open = false;
	}

	function toggleSelection(id: number) {
		const next = new Set(searchSelected);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		searchSelected = next;
	}

	function toggleSelectAll() {
		if (searchSelected.size === searchResults.length && searchResults.length > 0) {
			searchSelected = new Set();
		} else {
			searchSelected = new Set(searchResults.map((r) => r.request_id));
		}
	}

	function pickResult(record: RequestListItem) {
		open = false;
		onSelect([record]);
	}

	function confirmSelection() {
		const selected = searchResults.filter((r) => searchSelected.has(r.request_id));
		if (selected.length > 0) {
			open = false;
			onSelect(selected);
		}
	}

	function tierLabel(key: string): string {
		return tierLabels[key] || key;
	}

	function fmtClock(s?: string): string {
		if (!s) return '—';
		return s.slice(0, 19).replace('T', ' ');
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') void runSearch();
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="dialog-overlay"
		onmousedown={(e) => {
			if (e.target === e.currentTarget) closeDialog();
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') closeDialog();
		}}
	>
		<div
			class="dialog"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			tabindex="0"
		>
			<div class="dialog-head">
				<div class="dialog-eyebrow">kb.doc_review_requests</div>
				<h2 class="dialog-title">Search review requests</h2>
				<p class="dialog-subtitle">
					Filter by request, document, requester, tier, status, and create window.
				</p>
			</div>

			<div class="dialog-scroll">
				<div class="dialog-controls">
					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Identity</div>
							<div class="dialog-section-copy">Match the request, its document, and who submitted it.</div>
						</div>
						<div class="dialog-grid dialog-grid-primary">
							<label class="field dialog-field">
								<span class="field-label">Request ID</span>
								<input type="text" bind:value={requestId} placeholder="42" onkeydown={onKeydown} />
							</label>
							<label class="field dialog-field dialog-field-wide">
								<span class="field-label">Document title contains</span>
								<input type="text" bind:value={title} placeholder="标准名称, spec…" onkeydown={onKeydown} />
							</label>
							<label class="field dialog-field dialog-field-wide">
								<span class="field-label">Requester contains</span>
								<input type="text" bind:value={requester} placeholder="Alex Johnson…" onkeydown={onKeydown} />
							</label>
						</div>
					</div>

					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Classification</div>
							<div class="dialog-section-copy">Narrow by review tier and lifecycle status.</div>
						</div>
						<div class="dialog-grid">
							<label class="field dialog-field">
								<span class="field-label">Tier</span>
								<select bind:value={tier}>
									<option value="all">All</option>
									{#each tiers as t}
										<option value={t.key}>{t.label}</option>
									{/each}
								</select>
							</label>
							<label class="field dialog-field">
								<span class="field-label">Status</span>
								<select bind:value={status}>
									{#each statusOptions as opt}
										<option value={opt.value}>{opt.label}</option>
									{/each}
								</select>
							</label>
						</div>
					</div>

					<div class="dialog-section">
						<div class="dialog-section-head">
							<div class="dialog-section-title">Time Window</div>
							<div class="dialog-section-copy">Search by create timestamp using a local date-time range.</div>
						</div>
						<div class="dialog-grid">
							<label class="field dialog-field"
								><span class="field-label">Created from</span><input
									type="datetime-local"
									bind:value={createStart}
								/></label
							>
							<label class="field dialog-field"
								><span class="field-label">Created to</span><input
									type="datetime-local"
									bind:value={createEnd}
								/></label
							>
						</div>
					</div>

					<div class="dialog-toolbar">
						<div class="dialog-toolbar-copy">
							<div class="dialog-toolbar-title">Search Scope</div>
							<div class="dialog-toolbar-text">
								Leave fields empty to broaden the search. Results are capped to the newest 100 requests.
							</div>
						</div>
						<div class="dialog-toolbar-actions">
							<button class="btn btn-ghost" type="button" onclick={resetSearch}>Reset</button>
							<button
								class="btn btn-primary dialog-search-btn"
								type="button"
								onclick={runSearch}
								disabled={searchLoading}
							>
								{searchLoading ? 'Searching…' : 'Search'}
							</button>
						</div>
					</div>
				</div>

				{#if searchError}
					<div class="dialog-error">{searchError}</div>
				{/if}

				{#if searchTotal !== null && !searchLoading}
					<div class="results-count">
						{#if searchTotal === 0}
							No requests matched.
						{:else}
							<strong>{searchTotal}</strong> {searchTotal === 1 ? 'request' : 'requests'} matched
						{/if}
					</div>
				{/if}

				<div class="dialog-results">
					{#if searchResults.length === 0 && !searchLoading}
						<div class="dialog-empty">
							<div class="empty-glyph">⌕</div>
							<div class="dialog-empty-title">Run a search to see requests.</div>
							<div class="dialog-empty-copy">
								Use any combination of identity, classification, and time filters to narrow the list.
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
									<th>Document</th>
									<th>Tier</th>
									<th>Status</th>
									<th>Requester</th>
									<th>Created</th>
								</tr>
							</thead>
							<tbody>
								{#each searchResults as record (record.request_id)}
									<tr
										class:selected={searchSelected.has(record.request_id)}
										onclick={() => toggleSelection(record.request_id)}
										ondblclick={() => pickResult(record)}
									>
										<td class="col-check">
											<input
												type="checkbox"
												class="row-check"
												checked={searchSelected.has(record.request_id)}
												onclick={(e) => e.stopPropagation()}
												onchange={() => toggleSelection(record.request_id)}
											/>
										</td>
										<td class="mono">#{record.request_id}</td>
										<td class="ellipsis">{record.doc_title || `Document #${record.input_record_id}`}</td>
										<td>{tierLabel(record.tier)}</td>
										<td><span class="status-pill">{record.status}</span></td>
										<td>{record.requester_name || '—'}</td>
										<td class="mono muted">{fmtClock(record.create_time)}</td>
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
					<button class="btn btn-ghost" type="button" onclick={closeDialog}>Cancel</button>
					<button
						class="btn btn-primary dialog-select-btn"
						type="button"
						onclick={confirmSelection}
						disabled={searchSelected.size === 0}
					>
						{searchSelected.size > 0 ? `Select (${searchSelected.size})` : 'Select'}
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
		padding: 0.75rem;
		background: rgba(2, 6, 23, 0.68);
		backdrop-filter: blur(10px);
	}

	.dialog {
		width: min(1200px, 100%);
		max-height: min(94vh, 1100px);
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

	.dialog-grid-primary {
		grid-template-columns: 0.8fr 1.4fr 1.4fr;
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
		grid-column: span 1;
	}

	.dialog-field :global(input),
	.dialog-field :global(select) {
		height: 42px;
		border: 1px solid rgba(255, 255, 255, 0.08);
		background: #2a3140;
		padding: 0 12px;
		color: #f3eedf;
		font-size: 14px;
		border-radius: 8px;
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
		border-radius: 10px;
		padding: 0.7rem 1.1rem;
		border: 1px solid rgba(148, 163, 184, 0.16);
		cursor: pointer;
		font-size: 13px;
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

	.results-count {
		padding: 6px 28px;
		font-family: 'JetBrains Mono', monospace;
		font-size: 11px;
		letter-spacing: 0.08em;
		color: #9ca3af;
		background: #111827;
		border-top: 1px solid #1f2530;
		flex: 0 0 auto;
	}

	.results-count strong {
		color: #d4a24c;
		font-weight: 600;
	}

	.dialog-results {
		flex: 1 1 280px;
		border-top: 1px solid #1f2530;
		border-bottom: 1px solid #1f2530;
		min-height: 300px;
		background: #121720;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
		overflow: auto;
	}

	.dialog-empty {
		padding: 64px 20px;
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
		text-align: left;
		padding: 12px 16px;
		border-bottom: 1px solid rgba(148, 163, 184, 0.12);
		background: #181d27;
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
		accent-color: #d4a24c;
		cursor: pointer;
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

	.status-pill {
		padding: 3px 10px;
		border-radius: 999px;
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		background: rgba(124, 117, 96, 0.16);
		color: #d1d5db;
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

	@media (max-width: 760px) {
		.dialog-grid,
		.dialog-grid-primary {
			grid-template-columns: 1fr;
		}

		.dialog-toolbar,
		.dialog-foot,
		.dialog-section-head {
			flex-direction: column;
			align-items: stretch;
		}
	}
</style>
