<script lang="ts">
	// Search tab for the content viewer. Reuses the Metrics view's search pieces:
	//   • KbInputRecordBrowser — RECORD ID + Retrieve + the "Find a record" filter
	//     dialog + the kb.inputs results list.
	//   • Global Metric Search — free-text + agent-style filters over all of kb.metrics.
	// Selecting a document lists every one of its metrics (no further filtering).
	// Picking a metric (from either list) calls `onpick` with the canonical
	// metric_id; the view deep-links it (?metric_id=) so the source pane resolves.
	import KbInputRecordBrowser from '$lib/components/home3/kb-input-record-browser.svelte';
	import { listKbMetrics, type KbInputRecord, type KbMetricRecord } from '$lib/services/kbService';
	import { searchKbMetrics, type KbMetricSearchResult } from '$lib/services/kbMetricSearch';
	import {
		KB_METRIC_SEARCH_DEFAULTS,
		buildKbMetricSearchParams,
		createEmptyKbMetricSearchFilters,
		hasKbMetricSearchFilters
	} from '../kb-metric-search-state.js';
	import {
		metricSearchResultChips,
		metricSearchResultSecondaryText
	} from '../kb-metric-search-result.js';
	import { pickableMetricId } from './metric-search-pick';
	import type { ExplorerTokens } from './theme';

	let {
		tokens,
		darkMode = true,
		activeMetricId = '',
		onpick
	}: {
		tokens: ExplorerTokens;
		darkMode?: boolean;
		activeMetricId?: string;
		onpick: (metricId: string) => void;
	} = $props();

	// ---- Selected document → its metrics -------------------------------------
	let selectedRecord = $state<KbInputRecord | null>(null);
	let recordMetrics = $state<KbMetricRecord[]>([]);
	let metricsLoading = $state(false);
	let metricsError = $state('');

	async function handleRecordSelect(record: KbInputRecord) {
		if (selectedRecord?.id === record.id) return;
		selectedRecord = record;
		clearGlobalSearch();
		await loadMetrics(record.id);
	}

	async function loadMetrics(id: number) {
		metricsError = '';
		metricsLoading = true;
		recordMetrics = [];
		try {
			const res = await listKbMetrics(id);
			recordMetrics = res.results ?? [];
		} catch (e) {
			metricsError = e instanceof Error ? e.message : String(e);
		} finally {
			metricsLoading = false;
		}
	}

	// ---- Global metric search ----------------------------------------------
	let gQuery = $state('');
	let gFilters = $state(createEmptyKbMetricSearchFilters());
	let gResults = $state<KbMetricSearchResult[]>([]);
	let gLoading = $state(false);
	let gError = $state('');
	let gHasRun = $state(false);
	let gTotal = $state(0);
	let gPage = $state<number>(KB_METRIC_SEARCH_DEFAULTS.page);

	const gActive = $derived(gQuery.trim().length > 0 || hasKbMetricSearchFilters(gFilters));
	const gTotalPages = $derived(Math.max(1, Math.ceil(gTotal / KB_METRIC_SEARCH_DEFAULTS.pageSize)));
	const showGlobal = $derived(gActive && gHasRun);

	// The pane is a two-step master/detail: browse (record browser + global
	// search form) → detail (the selected document's metrics, or global hits).
	const detailOpen = $derived(showGlobal || selectedRecord != null);

	function backToBrowse() {
		selectedRecord = null;
		recordMetrics = [];
		metricsError = '';
		clearGlobalSearch();
	}

	async function runGlobalSearch(page: number = KB_METRIC_SEARCH_DEFAULTS.page) {
		gError = '';
		gHasRun = true;
		const params = buildKbMetricSearchParams({
			query: gQuery,
			page,
			pageSize: KB_METRIC_SEARCH_DEFAULTS.pageSize,
			filters: gFilters
		});
		if (!params.q && !hasKbMetricSearchFilters(gFilters)) {
			gResults = [];
			gTotal = 0;
			gPage = KB_METRIC_SEARCH_DEFAULTS.page;
			gError = 'Enter a query or set at least one filter before searching.';
			return;
		}
		gLoading = true;
		try {
			const res = await searchKbMetrics(params);
			gResults = res.results ?? [];
			gTotal = res.total ?? 0;
			gPage = res.page ?? page;
		} catch (e) {
			gResults = [];
			gTotal = 0;
			gError = e instanceof Error ? e.message : String(e);
		} finally {
			gLoading = false;
		}
	}

	function clearGlobalSearch() {
		gQuery = '';
		gFilters = createEmptyKbMetricSearchFilters();
		gResults = [];
		gLoading = false;
		gError = '';
		gHasRun = false;
		gTotal = 0;
		gPage = KB_METRIC_SEARCH_DEFAULTS.page;
	}

	// ---- Pick ------------------------------------------------------------------
	function pick(m: { metric_id?: string | null }) {
		const id = pickableMetricId(m);
		if (id) onpick(id);
	}

	function recordTitle(r: KbInputRecord): string {
		return r.title?.trim() || r.name?.trim() || r.file_name?.trim() || `Record #${r.id}`;
	}
	function metricName(m: KbMetricRecord): string {
		return m.metric_name?.trim() || m.metric_subject?.trim() || `Metric #${m.id}`;
	}
	function metricValueText(m: KbMetricRecord): string {
		return [m.metric_value?.trim(), m.metric_unit?.trim()].filter(Boolean).join(' ');
	}
	function confPct(c: number | undefined): string {
		return typeof c === 'number' && Number.isFinite(c) ? `${Math.round(c * 100)}%` : '—';
	}
</script>

<div
	class="ms"
	style="
		--panel:{tokens.panelBg}; --card:{tokens.cardBg}; --border:{tokens.border};
		--border-strong:{tokens.borderStrong}; --text:{tokens.textPrimary}; --text-2:{tokens.textSecondary};
		--text-3:{tokens.textMuted}; --accent:{tokens.accent}; --hover:{tokens.hoverBg}; --warn:{tokens.warn};
		--panel-bg:{tokens.panelBg}; --panel-bg-alt:{tokens.cardBg}; --ink-line:{tokens.border};
		--ink-line-soft:{tokens.border}; --text-primary:{tokens.textPrimary}; --text-secondary:{tokens.textSecondary};
		--brass:{tokens.accent}; --crimson:{tokens.err};
	"
>
	<!-- BROWSE — record browser + the global metric-search form -->
	<div class="browse" class:is-hidden={detailOpen}>
		<div class="rec">
			<KbInputRecordBrowser
				{darkMode}
				instanceKey="moe-search-records"
				title="kb.inputs"
				subtitle="Search or retrieve a document, then pick one of its metrics."
				emptyTitle="No records yet"
				emptySubtitle="Use Search or Retrieve to browse kb.inputs."
				autoSelectFirstRecord={false}
				defaultListWidth={360}
				selectedRecordId={selectedRecord?.id ?? null}
				onSelect={handleRecordSelect}
				onError={(e) => (metricsError = e.message)}
			/>
		</div>

		<details class="gms">
			<summary>Global metric search</summary>
			<div class="gms-body">
				<input
					class="in q"
					type="text"
					placeholder="Search metrics, thresholds, units, keywords…"
					bind:value={gQuery}
					onkeydown={(e) => {
						if (e.key === 'Enter') void runGlobalSearch();
					}}
				/>
				<div class="grid">
					<input
						class="in"
						type="text"
						placeholder="Record ID"
						bind:value={gFilters.inputRecordId}
					/>
					<select class="in" bind:value={gFilters.isExplicitMetric}>
						<option value="">Explicit metric?</option>
						<option value="true">Explicit only</option>
						<option value="false">Implicit only</option>
					</select>
					<input
						class="in"
						type="text"
						placeholder="Value class"
						bind:value={gFilters.valueClass}
					/>
					<input
						class="in"
						type="text"
						placeholder="Value type"
						bind:value={gFilters.valueDataType}
					/>
					<input
						class="in"
						type="text"
						placeholder="Metric unit"
						bind:value={gFilters.metricUnit}
					/>
				</div>
				<div class="row">
					<button class="btn primary" disabled={gLoading} onclick={() => void runGlobalSearch()}>
						{gLoading ? 'Searching…' : 'Search'}
					</button>
					<button class="btn" onclick={clearGlobalSearch}>Clear</button>
				</div>
			</div>
		</details>
	</div>

	<!-- DETAIL — the picked document's metrics, or the global-search hits -->
	{#if detailOpen}
		<div class="detail">
			<div class="detail-head">
				<button class="back" onclick={backToBrowse}>‹ Back</button>
				<span class="ctx">
					{#if showGlobal}
						{gTotal} result{gTotal === 1 ? '' : 's'}{gQuery.trim() ? ` for “${gQuery.trim()}”` : ''}
					{:else if selectedRecord}
						{recordMetrics.length} metric{recordMetrics.length === 1 ? '' : 's'} · {recordTitle(
							selectedRecord
						)}
					{/if}
				</span>
				{#if showGlobal && gTotalPages > 1}
					<span class="pg-wrap">
						{gPage}/{gTotalPages}
						<button
							class="pg"
							disabled={gLoading || gPage <= 1}
							onclick={() => void runGlobalSearch(gPage - 1)}>‹</button
						>
						<button
							class="pg"
							disabled={gLoading || gPage >= gTotalPages}
							onclick={() => void runGlobalSearch(gPage + 1)}>›</button
						>
					</span>
				{/if}
			</div>

			<div class="cards">
				{#if gError}
					<p class="note err">{gError}</p>
				{:else if metricsError && !showGlobal}
					<p class="note err">{metricsError}</p>
				{:else if gLoading || (metricsLoading && !showGlobal)}
					<p class="note">Loading…</p>
				{:else if showGlobal}
					{#if gResults.length === 0}
						<p class="note">No matches. Try broader keywords or relax a filter.</p>
					{:else}
						{#each gResults as r, i (r.id)}
							{@const linkId = pickableMetricId(r)}
							<button
								class="hit"
								class:on={linkId != null && linkId === activeMetricId}
								disabled={linkId == null}
								title={linkId == null
									? 'This hit has no canonical metric id and cannot be opened here'
									: ''}
								onclick={() => pick(r)}
							>
								<div class="hit-top">
									<span class="rank"
										>⌕ {String((gPage - 1) * KB_METRIC_SEARCH_DEFAULTS.pageSize + i + 1).padStart(
											3,
											'0'
										)}</span
									>
									<span class="score" title="Search score">{r.score.toFixed(3)}</span>
								</div>
								<div class="hit-name">{r.primary_label}</div>
								{#if metricSearchResultSecondaryText(r)}<div class="hit-desc">
										{metricSearchResultSecondaryText(r)}
									</div>{/if}
								<div class="hit-foot">
									<span class="tag">record {r.input_record_id}</span>
									{#each metricSearchResultChips(r) as c (`${r.id}-${c}`)}<span class="tag quiet"
											>{c}</span
										>{/each}
									{#if linkId == null}<span class="tag warn">no metric id</span>{/if}
								</div>
							</button>
						{/each}
					{/if}
				{:else if selectedRecord}
					{#if recordMetrics.length === 0}
						<p class="note">This document has no extracted metrics.</p>
					{:else}
						{#each recordMetrics as m, i (m.id)}
							{@const linkId = pickableMetricId(m)}
							<button
								class="hit"
								class:on={linkId != null && linkId === activeMetricId}
								disabled={linkId == null}
								title={linkId == null
									? 'This metric has no canonical metric id and cannot be opened here'
									: ''}
								onclick={() => pick(m)}
							>
								<div class="hit-top">
									<span class="rank">№ {String(i + 1).padStart(3, '0')}</span>
									<span class="score" title="Confidence">{confPct(m.confidence)}</span>
								</div>
								<div class="hit-name">{metricName(m)}</div>
								{#if metricValueText(m)}<div class="hit-desc">{metricValueText(m)}</div>{/if}
								<div class="hit-foot">
									{#if m.value_class}<span class="tag quiet">{m.value_class}</span>{/if}
									{#if m.location_type}<span class="tag quiet">{m.location_type}</span>{/if}
									{#if linkId == null}<span class="tag warn">no metric id</span>{/if}
								</div>
							</button>
						{/each}
					{/if}
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.ms {
		display: flex;
		flex-direction: column;
		min-height: 0;
		height: 100%;
		color: var(--text);
		background: var(--panel);
	}

	/* BROWSE step — record browser (fills) + collapsible global-search form. */
	.browse {
		flex: 1 1 auto;
		min-height: 0;
		display: flex;
		flex-direction: column;
	}
	.browse.is-hidden {
		display: none;
	}
	.rec {
		flex: 1 1 auto;
		min-height: 0;
		display: flex;
		overflow: auto;
		padding: 10px 10px 0;
	}

	/* Global metric search — collapsible so it does not crowd the record list. */
	.gms {
		flex: 0 0 auto;
		border-top: 1px solid var(--border);
		background: var(--panel);
	}
	.gms > summary {
		list-style: none;
		cursor: pointer;
		padding: 8px 14px;
		font:
			600 10px/1 ui-sans-serif,
			system-ui,
			sans-serif;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--text-2);
	}
	.gms > summary::-webkit-details-marker {
		display: none;
	}
	.gms > summary::before {
		content: '▸ ';
		color: var(--text-3);
	}
	.gms[open] > summary::before {
		content: '▾ ';
	}
	.gms-body {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 0 14px 12px;
	}
	.grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 6px;
	}
	.in {
		appearance: none;
		width: 100%;
		box-sizing: border-box;
		border: 1px solid var(--border-strong);
		border-radius: 6px;
		background: var(--card);
		color: var(--text);
		font:
			400 11.5px/1.4 ui-sans-serif,
			system-ui,
			sans-serif;
		padding: 6px 8px;
	}
	.in.q {
		grid-column: 1 / -1;
	}
	.in:focus {
		outline: none;
		border-color: var(--accent);
	}
	.row {
		display: flex;
		gap: 6px;
	}
	.btn {
		appearance: none;
		border: 1px solid var(--border-strong);
		border-radius: 6px;
		background: transparent;
		color: var(--text-2);
		font:
			600 10.5px/1 ui-sans-serif,
			system-ui,
			sans-serif;
		letter-spacing: 0.03em;
		padding: 6px 10px;
		cursor: pointer;
	}
	.btn:hover:not(:disabled) {
		color: var(--text);
		border-color: var(--accent);
	}
	.btn.primary {
		background: var(--accent);
		border-color: var(--accent);
		color: #fff;
	}
	.btn:disabled {
		opacity: 0.5;
		cursor: default;
	}

	/* DETAIL step — the picked document's metrics, or global-search hits. */
	.detail {
		flex: 1 1 auto;
		min-height: 0;
		display: flex;
		flex-direction: column;
	}
	.detail-head {
		flex: 0 0 auto;
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		border-bottom: 1px solid var(--border);
	}
	.back {
		appearance: none;
		flex: 0 0 auto;
		border: 1px solid var(--border-strong);
		border-radius: 6px;
		background: transparent;
		color: var(--text-2);
		font:
			600 10.5px/1 ui-sans-serif,
			system-ui,
			sans-serif;
		padding: 5px 9px;
		cursor: pointer;
	}
	.back:hover {
		color: var(--text);
		border-color: var(--accent);
	}
	.ctx {
		flex: 1 1 auto;
		min-width: 0;
		font:
			400 11px/1.4 ui-sans-serif,
			system-ui,
			sans-serif;
		color: var(--text-2);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.pg-wrap {
		white-space: nowrap;
	}
	.pg {
		appearance: none;
		border: 1px solid var(--border);
		border-radius: 5px;
		background: transparent;
		color: var(--text-2);
		font:
			600 10px/1 ui-sans-serif,
			system-ui,
			sans-serif;
		padding: 3px 7px;
		margin-left: 4px;
		cursor: pointer;
	}
	.pg:disabled {
		opacity: 0.4;
		cursor: default;
	}
	.cards {
		flex: 1 1 auto;
		min-height: 0;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 4px 14px 16px;
	}
	.note {
		margin: 4px 0 0;
		font:
			400 11.5px/1.55 ui-sans-serif,
			system-ui,
			sans-serif;
		color: var(--text-2);
		max-width: 52ch;
	}
	.note.err {
		color: var(--warn);
	}

	.hit {
		appearance: none;
		text-align: left;
		display: flex;
		flex-direction: column;
		gap: 5px;
		border: 1px solid var(--border);
		border-left: 3px solid var(--border-strong);
		border-radius: 6px;
		background: var(--card);
		padding: 9px 11px;
		cursor: pointer;
		color: var(--text);
	}
	.hit:hover:not(:disabled) {
		border-color: var(--accent);
		border-left-color: var(--accent);
		background: var(--hover);
	}
	.hit.on {
		border-color: var(--accent);
		border-left-color: var(--accent);
	}
	.hit:disabled {
		opacity: 0.55;
		cursor: default;
	}
	.hit-top {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}
	.rank {
		font:
			600 9px/1 ui-monospace,
			SFMono-Regular,
			Menlo,
			monospace;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--text-3);
	}
	.score {
		font:
			600 10px/1 ui-monospace,
			SFMono-Regular,
			Menlo,
			monospace;
		color: var(--accent);
	}
	.hit-name {
		font:
			600 13px/1.3 'Fraunces',
			Georgia,
			serif;
	}
	.hit-desc {
		font:
			400 11.5px/1.45 ui-sans-serif,
			system-ui,
			sans-serif;
		color: var(--text-2);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.hit-foot {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		margin-top: 1px;
	}
	.tag {
		font:
			600 9px/1.4 ui-monospace,
			SFMono-Regular,
			Menlo,
			monospace;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		border: 1px solid var(--border);
		border-radius: 999px;
		padding: 2px 7px;
		color: var(--text-2);
	}
	.tag.quiet {
		color: var(--text-3);
	}
	.tag.warn {
		color: var(--warn);
		border-color: var(--warn);
	}
</style>
