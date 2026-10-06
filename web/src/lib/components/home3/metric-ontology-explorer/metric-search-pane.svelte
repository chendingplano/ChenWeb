<script lang="ts">
	import { m as msg } from '$lib/paraglide/messages.js';
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
	import StatementKindBadge from '$lib/components/home3/statement-kind-badge.svelte';
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
			gError = msg.metric_search_pane_enter_a_query_or_set();
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
		return (
			r.title?.trim() ||
			r.name?.trim() ||
			r.file_name?.trim() ||
			msg.metric_search_pane_record_2({ id: r.id })
		);
	}
	function metricName(m: KbMetricRecord): string {
		return (
			m.metric_name?.trim() ||
			m.metric_subject?.trim() ||
			msg.metric_search_pane_metric_2({ id: m.id })
		);
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
				title={msg.metric_search_pane_kb_inputs()}
				subtitle={msg.metric_search_pane_search_or_retrieve_a_document()}
				emptyTitle={msg.metric_search_pane_no_records_yet()}
				emptySubtitle={msg.metric_search_pane_use_search_or_retrieve_to()}
				autoSelectFirstRecord={false}
				defaultListWidth={360}
				selectedRecordId={selectedRecord?.id ?? null}
				onSelect={handleRecordSelect}
				onError={(e) => (metricsError = e.message)}
			/>
		</div>

		<details class="gms">
			<summary>{msg.metric_search_pane_global_metric_search()}</summary>
			<div class="gms-body">
				<input
					class="in q"
					type="text"
					placeholder={msg.metric_search_pane_search_metrics_thresholds_units_keywords()}
					bind:value={gQuery}
					onkeydown={(e) => {
						if (e.key === 'Enter') void runGlobalSearch();
					}}
				/>
				<div class="grid">
					<input
						class="in"
						type="text"
						placeholder={msg.metric_search_pane_record_id()}
						bind:value={gFilters.inputRecordId}
					/>
					<select class="in" bind:value={gFilters.isExplicitMetric}>
						<option value="">{msg.metric_search_pane_explicit_metric()}</option>
						<option value="true">{msg.metric_search_pane_explicit_only()}</option>
						<option value="false">{msg.metric_search_pane_implicit_only()}</option>
					</select>
					<input
						class="in"
						type="text"
						placeholder={msg.metric_search_pane_value_class()}
						bind:value={gFilters.valueClass}
					/>
					<input
						class="in"
						type="text"
						placeholder={msg.metric_search_pane_value_type()}
						bind:value={gFilters.valueDataType}
					/>
					<input
						class="in"
						type="text"
						placeholder={msg.metric_search_pane_metric_unit()}
						bind:value={gFilters.metricUnit}
					/>
				</div>
				<div class="row">
					<button class="btn primary" disabled={gLoading} onclick={() => void runGlobalSearch()}>
						{gLoading ? msg.metric_search_pane_searching() : msg.metric_search_pane_search()}
					</button>
					<button class="btn" onclick={clearGlobalSearch}>{msg.metric_search_pane_clear()}</button>
				</div>
			</div>
		</details>
	</div>

	<!-- DETAIL — the picked document's metrics, or the global-search hits -->
	{#if detailOpen}
		<div class="detail">
			<div class="detail-head">
				<button class="back" onclick={backToBrowse}>{msg.metric_search_pane_back()}</button>
				<span class="ctx">
					{#if showGlobal}
						{msg.metric_search_pane_result({
							gTotal,
							plural: gTotal === 1 ? '' : 's',
							value: gQuery.trim() ? msg.metric_search_pane_for({ gQuery: gQuery.trim() }) : ''
						})}
					{:else if selectedRecord}
						{msg.metric_search_pane_metric({
							recordMetricsCount: recordMetrics.length,
							plural: recordMetrics.length === 1 ? '' : 's',
							selectedRecord: recordTitle(selectedRecord)
						})}
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
					<p class="note">{msg.metric_search_pane_loading()}</p>
				{:else if showGlobal}
					{#if gResults.length === 0}
						<p class="note">{msg.metric_search_pane_no_matches_try_broader_keywords()}</p>
					{:else}
						{#each gResults as r, i (r.id)}
							{@const linkId = pickableMetricId(r)}
							<button
								class="hit"
								class:on={linkId != null && linkId === activeMetricId}
								disabled={linkId == null}
								title={linkId == null ? msg.metric_search_pane_this_hit_has_no_canonical() : ''}
								onclick={() => pick(r)}
							>
								<div class="hit-top">
									<span class="rank"
										>⌕ {String((gPage - 1) * KB_METRIC_SEARCH_DEFAULTS.pageSize + i + 1).padStart(
											3,
											'0'
										)}</span
									>
									<span class="score" title={msg.metric_search_pane_search_score()}
										>{r.score.toFixed(3)}</span
									>
								</div>
								<div class="hit-name">{r.primary_label}</div>
								{#if metricSearchResultSecondaryText(r)}<div class="hit-desc">
										{metricSearchResultSecondaryText(r)}
									</div>{/if}
								<div class="hit-foot">
									<span class="tag"
										>{msg.metric_search_pane_record({ input_record_id: r.input_record_id })}</span
									>
									{#each metricSearchResultChips(r) as c (`${r.id}-${c}`)}<span class="tag quiet"
											>{c}</span
										>{/each}
									{#if linkId == null}<span class="tag warn"
											>{msg.metric_search_pane_no_metric_id()}</span
										>{/if}
								</div>
							</button>
						{/each}
					{/if}
				{:else if selectedRecord}
					{#if recordMetrics.length === 0}
						<p class="note">{msg.metric_search_pane_this_document_has_no_extracted()}</p>
					{:else}
						{#each recordMetrics as m, i (m.id)}
							{@const linkId = pickableMetricId(m)}
							<button
								class="hit"
								class:on={linkId != null && linkId === activeMetricId}
								disabled={linkId == null}
								title={linkId == null ? msg.metric_search_pane_this_metric_has_no_canonical() : ''}
								onclick={() => pick(m)}
							>
								<div class="hit-top">
									<span class="rank">№ {String(i + 1).padStart(3, '0')}</span>
									<span class="score" title={msg.metric_search_pane_confidence()}
										>{confPct(m.confidence)}</span
									>
								</div>
								<div class="hit-name">{metricName(m)}</div>
								{#if metricValueText(m)}<div class="hit-desc">{metricValueText(m)}</div>{/if}
								<div class="hit-foot">
									{#if m.value_class}<StatementKindBadge row={m} />{/if}
									{#if m.value_class}<span class="tag quiet">{m.value_class}</span>{/if}
									{#if m.location_type}<span class="tag quiet">{m.location_type}</span>{/if}
									{#if linkId == null}<span class="tag warn"
											>{msg.metric_search_pane_no_metric_id()}</span
										>{/if}
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
		scrollbar-width: thin;
		scrollbar-color: var(--ink-line) transparent;
	}
	.cards::-webkit-scrollbar {
		width: 8px;
	}
	.cards::-webkit-scrollbar-thumb {
		background: var(--ink-line);
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
