<script lang="ts">
	// System Admin -> LLM -> Review Metrics. Search kb.inputs, select a record,
	// and show (or run) an LLM review of its extracted metrics. See
	// openspec/changes/llm-review-metrics.
	import { onDestroy } from 'svelte';
	import SearchIcon from '@lucide/svelte/icons/search';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import {
		getMetricReview,
		groupNonMetrics,
		NON_METRIC_CATEGORY_LABEL,
		searchInputs,
		sortBySeverity,
		startMetricReview,
		type InputRecordSummary,
		type MetricReview,
		type MetricSnapshot
	} from './metric-review-client.js';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// --- Design tokens (match llm-usage-logs-view) ---
	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let success = $derived(darkMode ? '#34D399' : '#059669');
	let warning = $derived(darkMode ? '#FBBF24' : '#B45309');
	let danger = $derived(darkMode ? '#F87171' : '#DC2626');

	const POLL_MS = 4000;

	// --- Search ---
	let query = $state('');
	let searching = $state(false);
	let searchError = $state('');
	let results = $state<InputRecordSummary[]>([]);
	let searched = $state(false);

	async function runSearch() {
		searching = true;
		searchError = '';
		try {
			results = await searchInputs(query);
			searched = true;
		} catch (e) {
			searchError = e instanceof Error ? e.message : String(e);
		} finally {
			searching = false;
		}
	}

	// --- Selected record + review ---
	let selected = $state<InputRecordSummary | null>(null);
	let review = $state<MetricReview | null>(null);
	let loadingReview = $state(false);
	let reviewError = $state('');
	let force = $state(false);
	let starting = $state(false);
	let pollTimer: ReturnType<typeof setTimeout> | null = null;

	function stopPolling() {
		if (pollTimer) clearTimeout(pollTimer);
		pollTimer = null;
	}

	function schedulePoll(recordId: number) {
		stopPolling();
		pollTimer = setTimeout(async () => {
			if (selected?.id !== recordId) return;
			try {
				const r = await getMetricReview(recordId);
				if (selected?.id !== recordId) return;
				review = r;
				if (r?.status === 'running') schedulePoll(recordId);
			} catch (e) {
				reviewError = e instanceof Error ? e.message : String(e);
			}
		}, POLL_MS);
	}

	async function selectRecord(rec: InputRecordSummary) {
		stopPolling();
		selected = rec;
		review = null;
		reviewError = '';
		loadingReview = true;
		try {
			const r = await getMetricReview(rec.id);
			if (selected?.id !== rec.id) return;
			review = r;
			if (r?.status === 'running') schedulePoll(rec.id);
		} catch (e) {
			reviewError = e instanceof Error ? e.message : String(e);
		} finally {
			loadingReview = false;
		}
	}

	async function runReview() {
		if (!selected) return;
		const recordId = selected.id;
		starting = true;
		reviewError = '';
		try {
			const res = await startMetricReview(recordId, force);
			if (selected?.id !== recordId) return;
			review = res.review;
			if (res.review?.status === 'running') schedulePoll(recordId);
		} catch (e) {
			reviewError = e instanceof Error ? e.message : String(e);
		} finally {
			starting = false;
		}
	}

	onDestroy(stopPolling);

	// --- Report helpers ---
	let report = $derived(review?.status === 'done' ? review.report : undefined);
	let snapshotById = $derived(new Map((report?.metrics ?? []).map((m) => [m.metric_id, m])));

	function shortId(id: string): string {
		const prefix = `${selected?.id}_`;
		return id.startsWith(prefix) ? id.slice(prefix.length) : id;
	}

	function metricTitle(m: MetricSnapshot | undefined, id: string): string {
		if (!m) return id;
		const val = [m.value, m.unit].filter(Boolean).join(' ');
		return [m.name, val, m.lines ? `L${m.lines}` : ''].filter(Boolean).join(' · ');
	}

	function severityColor(s: string): string {
		return s === 'high' ? danger : s === 'low' ? textMuted : warning;
	}

	function fmtTime(s?: string): string {
		if (!s) return '';
		const d = new Date(s);
		return Number.isNaN(d.getTime()) ? s : d.toLocaleString();
	}
</script>

{#snippet metricChips(ids: string[])}
	<span class="inline-flex flex-wrap gap-1">
		{#each ids as id (id)}
			{@const m = snapshotById.get(id)}
			<span
				class="rounded px-1.5 py-0.5 text-xs"
				style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				title={metricTitle(m, id)}
			>
				<span style="color:{accent}; font-family:ui-monospace,monospace;">{shortId(id)}</span>
				{#if m?.name}<span style="color:{textSecondary};"> {m.name}</span>{/if}
			</span>
		{/each}
	</span>
{/snippet}

{#snippet severityBadge(s: string)}
	<span
		class="rounded px-1.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide"
		style="color:{severityColor(s)}; border:1px solid {severityColor(s)};">{s}</span
	>
{/snippet}

<div class="p-6 space-y-4 h-full flex flex-col overflow-hidden" style="background:{pageBg};">
	<!-- Header -->
	<div class="rounded-xl p-5 flex-shrink-0" style="background:{cardBg}; border:1px solid {borderColor};">
		<h2 style="font-size:18px; font-weight:600; color:{textPrimary};">Review Metrics</h2>
		<p style="font-size:13px; color:{textSecondary}; margin-top:2px;">
			LLM review of the metrics extracted from a document: missed metrics, rows that should not be metrics, and
			attribute correctness. Reviews are stored; tick <em>Force to Review</em> to run a fresh one.
		</p>
	</div>

	<div class="flex flex-1 min-h-0 gap-4">
		<!-- Left: search -->
		<div
			class="rounded-xl flex flex-col w-80 flex-shrink-0 min-h-0"
			style="background:{cardBg}; border:1px solid {borderColor};"
		>
			<form
				class="p-4 flex gap-2 flex-shrink-0"
				onsubmit={(e) => {
					e.preventDefault();
					runSearch();
				}}
			>
				<input
					bind:value={query}
					placeholder="Record ID or title"
					class="flex-1 min-w-0 rounded-lg px-3 py-2 text-sm outline-none"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				/>
				<button
					type="submit"
					disabled={searching}
					class="rounded-lg px-3 py-2 cursor-pointer"
					style="background:{accent}; color:#fff;"
					aria-label="Search"
				>
					<SearchIcon class="w-4 h-4 {searching ? 'animate-pulse' : ''}" />
				</button>
			</form>
			{#if searchError}
				<p class="px-4 pb-2 text-sm" style="color:{danger};">{searchError}</p>
			{/if}
			<div class="flex-1 min-h-0 overflow-y-auto px-2 pb-2">
				{#if searched && results.length === 0}
					<p class="px-2 text-sm" style="color:{textMuted};">No records found.</p>
				{/if}
				{#each results as rec (rec.id)}
					<button
						class="w-full text-left rounded-lg px-3 py-2 mb-1 cursor-pointer"
						style="background:{selected?.id === rec.id ? surface2 : 'transparent'}; border:1px solid {selected?.id ===
						rec.id
							? accent
							: 'transparent'};"
						onclick={() => selectRecord(rec)}
					>
						<div class="text-xs" style="color:{textMuted};">
							#{rec.id}{rec.doc_no ? ` · ${rec.doc_no}` : ''}
						</div>
						<div class="text-sm truncate" style="color:{textPrimary};">
							{rec.title || rec.file_name || '(untitled)'}
						</div>
					</button>
				{/each}
			</div>
		</div>

		<!-- Right: review -->
		<div
			class="rounded-xl flex-1 min-w-0 min-h-0 overflow-y-auto"
			style="background:{cardBg}; border:1px solid {borderColor};"
		>
			{#if !selected}
				<div class="h-full flex items-center justify-center text-sm" style="color:{textMuted};">
					Search for a document and select it to see its metrics review.
				</div>
			{:else}
				<div class="p-6 space-y-6 max-w-5xl">
					<!-- Record header + actions -->
					<div class="flex flex-wrap items-start justify-between gap-4">
						<div class="min-w-0">
							<div class="text-xs" style="color:{textMuted};">
								Record #{selected.id}{selected.doc_no ? ` · ${selected.doc_no}` : ''}
							</div>
							<h3 class="text-lg font-semibold" style="color:{textPrimary};">
								{selected.title || selected.file_name || '(untitled)'}
							</h3>
						</div>
						<div class="flex items-center gap-3">
							<label class="flex items-center gap-2 text-sm cursor-pointer" style="color:{textSecondary};">
								<input type="checkbox" bind:checked={force} />
								Force to Review
							</label>
							<button
								onclick={runReview}
								disabled={starting || review?.status === 'running'}
								class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium cursor-pointer disabled:opacity-50"
								style="background:{accent}; color:#fff;"
							>
								<SparklesIcon class="w-4 h-4" />
								Review
							</button>
						</div>
					</div>

					<!-- Status line -->
					{#if loadingReview}
						<p class="text-sm" style="color:{textMuted};">Loading review…</p>
					{:else if reviewError}
						<p class="flex items-center gap-2 text-sm" style="color:{danger};">
							<CircleAlertIcon class="w-4 h-4" />{reviewError}
						</p>
					{:else if !review}
						<p class="text-sm" style="color:{textSecondary};">
							This document has not been reviewed yet. Press <strong>Review</strong> to run one.
						</p>
					{:else if review.status === 'running'}
						<p class="flex items-center gap-2 text-sm" style="color:{accent};">
							<RefreshCwIcon class="w-4 h-4 animate-spin" />
							Reviewing {review.metrics_count} metrics… started {fmtTime(review.created_at)}. This can take a few
							minutes.
						</p>
					{:else if review.status === 'failed'}
						<p class="flex items-start gap-2 text-sm" style="color:{danger};">
							<CircleAlertIcon class="w-4 h-4 mt-0.5 flex-shrink-0" />
							<span>Review failed ({fmtTime(review.created_at)}): {review.error_msg}</span>
						</p>
					{:else}
						<p class="text-xs" style="color:{textMuted};">
							Reviewed {fmtTime(review.finished_at || review.created_at)} · {review.model_name} · {review.prompt_name}{review.created_by
								? ` · by ${review.created_by}`
								: ''}
						</p>
					{/if}

					{#if report}
						<!-- Summary -->
						<p class="text-[15px] leading-relaxed" style="color:{textPrimary};">{report.summary}</p>

						<!-- Tally -->
						<div class="grid grid-cols-3 md:grid-cols-6 gap-3">
							{#each [{ label: 'Stored', n: report.tally.stored, c: textPrimary }, { label: 'Kept', n: report.tally.kept, c: success }, { label: 'Not a metric', n: report.tally.not_metric, c: danger }, { label: 'Duplicates', n: report.tally.duplicate, c: warning }, { label: 'Formula inputs', n: report.tally.formula_input, c: warning }, { label: 'Missed', n: report.tally.missed, c: accent }] as t (t.label)}
								<div class="rounded-lg p-3" style="background:{surface2};">
									<div class="text-2xl font-semibold" style="color:{t.c};">{t.n}</div>
									<div class="text-xs" style="color:{textSecondary};">{t.label}</div>
								</div>
							{/each}
						</div>

						<!-- 1. Missed metrics -->
						<section class="space-y-2">
							<h4 class="font-semibold" style="color:{textPrimary};">
								1. Missed metrics <span style="color:{textMuted};">({report.missed_metrics.length})</span>
							</h4>
							{#if report.missed_metrics.length === 0}
								<p class="text-sm" style="color:{textMuted};">None — every metric in the document was found.</p>
							{/if}
							{#each sortBySeverity(report.missed_metrics) as m, i (i)}
								<div class="rounded-lg p-3 space-y-1" style="border:1px solid {borderColor};">
									<div class="flex flex-wrap items-center gap-2">
										{@render severityBadge(m.severity)}
										<span class="font-medium" style="color:{textPrimary};">{m.name}</span>
										{#if m.value}<span style="color:{accent};">{m.value} {m.unit}</span>{/if}
										{#if m.lines}<span class="text-xs" style="color:{textMuted};">L{m.lines}</span>{/if}
									</div>
									<p class="text-sm" style="color:{textSecondary};">{m.reason}</p>
								</div>
							{/each}
						</section>

						<!-- 2. Not metrics -->
						<section class="space-y-3">
							<h4 class="font-semibold" style="color:{textPrimary};">
								2. Rows that should not be metrics
								<span style="color:{textMuted};"
									>({report.tally.not_metric + report.tally.duplicate + report.tally.formula_input})</span
								>
							</h4>
							{#if report.non_metrics.length === 0}
								<p class="text-sm" style="color:{textMuted};">None.</p>
							{/if}
							{#each groupNonMetrics(report.non_metrics) as g (g.category)}
								<div class="space-y-2">
									<div class="text-xs font-semibold uppercase tracking-wide" style="color:{textSecondary};">
										{NON_METRIC_CATEGORY_LABEL[g.category]}
									</div>
									{#each g.entries as e, i (i)}
										<div class="rounded-lg p-3 space-y-1.5" style="border:1px solid {borderColor};">
											<div class="flex flex-wrap items-center gap-2">
												{@render metricChips(e.metric_ids)}
												{#if e.duplicate_of}
													<span class="text-xs" style="color:{textMuted};">duplicate of</span>
													{@render metricChips([e.duplicate_of])}
												{/if}
											</div>
											<p class="text-sm" style="color:{textSecondary};">{e.reason}</p>
										</div>
									{/each}
								</div>
							{/each}
						</section>

						<!-- 3. Attribute issues -->
						<section class="space-y-2">
							<h4 class="font-semibold" style="color:{textPrimary};">
								3. Attribute issues <span style="color:{textMuted};">({report.attribute_issues.length})</span>
							</h4>
							{#if report.attribute_issues.length === 0}
								<p class="text-sm" style="color:{textMuted};">None.</p>
							{/if}
							{#each sortBySeverity(report.attribute_issues) as a, i (i)}
								<div class="rounded-lg p-3 space-y-1.5" style="border:1px solid {borderColor};">
									<div class="flex flex-wrap items-center gap-2">
										{@render severityBadge(a.severity)}
										<code class="text-xs rounded px-1.5 py-0.5" style="background:{surface2}; color:{accent};"
											>{a.field}</code
										>
										{@render metricChips(a.metric_ids)}
									</div>
									{#if a.stored || a.suggested}
										<div class="text-sm flex flex-wrap items-center gap-2">
											<span style="color:{danger}; text-decoration:line-through;">{a.stored || '(empty)'}</span>
											<span style="color:{textMuted};">→</span>
											<span style="color:{success};">{a.suggested || '(empty)'}</span>
										</div>
									{/if}
									<p class="text-sm" style="color:{textSecondary};">{a.reason}</p>
								</div>
							{/each}
						</section>

						<!-- Recommendations -->
						{#if report.recommendations.length > 0}
							<section class="space-y-2">
								<h4 class="font-semibold" style="color:{textPrimary};">Recommendations</h4>
								<ol class="list-decimal pl-5 space-y-1 text-sm" style="color:{textSecondary};">
									{#each report.recommendations as r, i (i)}
										<li>{r}</li>
									{/each}
								</ol>
							</section>
						{/if}
					{/if}
				</div>
			{/if}
		</div>
	</div>
</div>
