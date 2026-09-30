<script lang="ts">
	// System Admin -> LLM -> Review Metrics. Search kb.inputs, select a record,
	// and show (or run) an LLM review of its extracted metrics. See
	// openspec/changes/llm-review-metrics. Reviews are per UI language, can be
	// translated from another language, and exported (metric-review-i18n-export).
	import { onDestroy } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import SearchIcon from '@lucide/svelte/icons/search';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import LanguagesIcon from '@lucide/svelte/icons/languages';
	import PdfViewWindow from './pdf-view-window.svelte';
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';
	import { getRawLines, type RawLine } from '$lib/services/kbService';
	import {
		buildReviewMarkdown,
		buildReviewPrintHtml,
		getMetricReview,
		groupNonMetrics,
		listMetricReviewModels,
		reviewExportFilename,
		reviewLineNumbers,
		searchInputs,
		sortBySeverity,
		startMetricReview,
		translateMetricReview,
		type InputRecordSummary,
		type MetricReview,
		type MetricSnapshot,
		type NonMetricCategory,
		type ReviewExportLabels,
		type ReviewSeverity
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
	let menuWidth = $state(228);
	let reportWidth = $state(540);
	let layoutEl: HTMLDivElement;
	let rawLines = $state<RawLine[]>([]);
	let pdfPage = $state(1);
	let highlightedLines = $state<number[]>([]);
	let selectedFindingKey = $state<string | null>(null);
	let highlightVersion = $state(0);
	let pdfError = $state('');
	let pdfZoom = $state(0.5);
	let pdfPages = $state(0);
	let dragPane: 'menu' | 'report' | null = null;

	function startPaneDrag(event: PointerEvent, pane: 'menu' | 'report') {
		dragPane = pane;
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		event.preventDefault();
	}
	function movePaneDrag(event: PointerEvent) {
		if (!dragPane || !layoutEl) return;
		const width = layoutEl.clientWidth;
		const x = event.clientX - layoutEl.getBoundingClientRect().left;
		if (dragPane === 'menu') menuWidth = Math.max(170, Math.min(width - 560, x));
		else reportWidth = Math.max(300, Math.min(width - menuWidth - 280, x - menuWidth - 20));
	}
	function stopPaneDrag() { dragPane = null; }
	function onPaneKeydown(event: KeyboardEvent, pane: 'menu' | 'report') {
		if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
		event.preventDefault();
		const delta = event.key === 'ArrowLeft' ? -16 : 16;
		if (pane === 'menu') menuWidth = Math.max(170, Math.min(layoutEl.clientWidth - 560, menuWidth + delta));
		else reportWidth = Math.max(300, Math.min(layoutEl.clientWidth - menuWidth - 280, reportWidth + delta));
	}
	function showSource(key: string, spans: string[]) {
		selectedFindingKey = key;
		highlightedLines = reviewLineNumbers(spans);
		const first = rawLines.find((line) => highlightedLines.includes(line.line_number));
		if (first) pdfPage = first.page_number;
		highlightVersion++;
	}
	function sourceForMetricIds(ids: string[]): string[] {
		return ids.flatMap((id) => {
			const snap = snapshotById.get(id);
			return snap?.source_line_spans ?? (snap?.lines ? snap.lines.split(',') : []);
		});
	}
	function renderSourceHighlights(pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) {
		for (const line of rawLines) {
			if (line.page_number !== pageNo || !highlightedLines.includes(line.line_number) || !Array.isArray(line.coords) || line.coords.length < 4) continue;
			const [x1, y1, x2, y2] = line.coords;
			const mark = document.createElement('div');
			mark.className = 'pdf-highlight';
			mark.style.cssText = `position:absolute;left:${Math.min(x1,x2)*viewport.width/1000}px;top:${Math.min(y1,y2)*viewport.height/1000}px;width:${Math.abs(x2-x1)*viewport.width/1000+20}px;height:${Math.max(2,Math.abs(y2-y1)*viewport.height/1000)}px;background:rgba(129,140,248,.35);pointer-events:none;`;
			overlay.appendChild(mark);
		}
	}

	// Paraglide reloads the page on a locale switch, so the locale is fixed here.
	const lang = getLocale();
	const LANG_LABELS: Record<string, string> = { en: 'English', 'zh-cn': '中文' };
	const langLabel = (l: string) => LANG_LABELS[l] ?? l;

	const CATEGORY_LABEL: Record<NonMetricCategory, string> = {
		not_metric: m.mrv_cat_not_metric(),
		duplicate: m.mrv_cat_duplicate(),
		formula_input: m.mrv_cat_formula_input()
	};
	const SEVERITY_LABEL: Record<ReviewSeverity, string> = {
		high: m.mrv_sev_high(),
		medium: m.mrv_sev_medium(),
		low: m.mrv_sev_low()
	};

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
	// LLM models from .models.toml; '' until loaded (the server then uses its default).
	let models = $state<string[]>([]);
	let model = $state('');
	let modelsError = $state('');
	listMetricReviewModels()
		.then((res) => {
			models = res.models;
			model = res.models.includes(res.defaultModel) ? res.defaultModel : (res.models[0] ?? '');
		})
		.catch((e) => (modelsError = e instanceof Error ? e.message : String(e)));
	// Languages with a finished review when the current language has none.
	let otherLangs = $state<string[]>([]);
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
				const { review: r, otherLangs: ol } = await getMetricReview(recordId, lang);
				if (selected?.id !== recordId) return;
				review = r;
				otherLangs = ol;
				if (r?.status === 'running') schedulePoll(recordId);
			} catch (e) {
				reviewError = e instanceof Error ? e.message : String(e);
			}
		}, POLL_MS);
	}

	async function selectRecord(rec: InputRecordSummary) {
		stopPolling();
		selected = rec;
		rawLines = [];
		pdfPage = 1;
		highlightedLines = [];
		selectedFindingKey = null;
		pdfError = '';
		getRawLines(rec.id).then((res) => {
			if (selected?.id !== rec.id) return;
			rawLines = res.lines ?? [];
			const first = rawLines.find((line) => highlightedLines.includes(line.line_number));
			if (first) pdfPage = first.page_number;
			highlightVersion++;
		}).catch((e) => { if (selected?.id === rec.id) pdfError = e instanceof Error ? e.message : String(e); });
		review = null;
		otherLangs = [];
		reviewError = '';
		loadingReview = true;
		try {
			const res = await getMetricReview(rec.id, lang);
			if (selected?.id !== rec.id) return;
			review = res.review;
			otherLangs = res.otherLangs;
			const r = res.review;
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
			const res = await startMetricReview(recordId, force, lang, model);
			if (selected?.id !== recordId) return;
			review = res.review;
			otherLangs = [];
			if (res.review?.status === 'running') schedulePoll(recordId);
		} catch (e) {
			reviewError = e instanceof Error ? e.message : String(e);
		} finally {
			starting = false;
		}
	}

	async function runTranslate() {
		if (!selected) return;
		const recordId = selected.id;
		starting = true;
		reviewError = '';
		try {
			const res = await translateMetricReview(recordId, lang);
			if (selected?.id !== recordId) return;
			review = res.review;
			otherLangs = [];
			if (res.review?.status === 'running') schedulePoll(recordId);
		} catch (e) {
			reviewError = e instanceof Error ? e.message : String(e);
		} finally {
			starting = false;
		}
	}

	onDestroy(stopPolling);

	// --- Export ---
	let exportOpen = $state(false);

	function exportLabels(r: MetricReview): ReviewExportLabels {
		return {
			title: m.mrv_title(),
			record: m.mrv_record(),
			reviewed: m.mrv_reviewed(),
			model: m.mrv_model(),
			prompt: m.mrv_prompt(),
			translatedFrom: r.translated_from_id ? m.mrv_translated_from({ id: r.translated_from_id }) : '',
			tally: {
				stored: m.mrv_tally_stored(),
				kept: m.mrv_tally_kept(),
				not_metric: m.mrv_tally_not_metric(),
				duplicate: m.mrv_tally_duplicate(),
				formula_input: m.mrv_tally_formula_input(),
				missed: m.mrv_tally_missed()
			},
			missed: m.mrv_sec_missed(),
			nonMetrics: m.mrv_sec_non_metrics(),
			attributes: m.mrv_sec_attributes(),
			recommendations: m.mrv_sec_recommendations(),
			none: m.mrv_none(),
			duplicateOf: m.mrv_duplicate_of(),
			empty: m.mrv_empty(),
			category: CATEGORY_LABEL,
			severity: SEVERITY_LABEL
		};
	}

	function currentMarkdown(): string {
		if (!selected || !review || review.status !== 'done') return '';
		return buildReviewMarkdown(selected, review, exportLabels(review), fmtTime);
	}

	function exportMarkdown() {
		exportOpen = false;
		const md = currentMarkdown();
		if (!md || !selected) return;
		const url = URL.createObjectURL(new Blob([md], { type: 'text/markdown;charset=utf-8' }));
		const a = document.createElement('a');
		a.href = url;
		a.download = reviewExportFilename(selected.id, review?.lang ?? lang, 'md');
		a.click();
		URL.revokeObjectURL(url);
	}

	// PDF: a print-styled copy in a new window; the browser's "Save as PDF"
	// renders CJK text correctly without embedding fonts (design D6).
	function exportPdf() {
		exportOpen = false;
		const md = currentMarkdown();
		if (!md || !selected) return;
		const title = reviewExportFilename(selected.id, review?.lang ?? lang, 'pdf').replace(/\.pdf$/, '');
		const w = window.open('', '_blank');
		if (!w) {
			reviewError = m.mrv_export_popup_blocked();
			return;
		}
		w.document.write(buildReviewPrintHtml(md, title, review?.lang ?? lang));
		w.document.close();
		w.focus();
		setTimeout(() => w.print(), 300);
	}

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
			{@const snap = snapshotById.get(id)}
			<span
				class="rounded px-1.5 py-0.5 text-xs"
				style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				title={metricTitle(snap, id)}
			>
				<span style="color:{accent}; font-family:ui-monospace,monospace;">{shortId(id)}</span>
				{#if snap?.name}<span style="color:{textSecondary};"> {snap.name}</span>{/if}
			</span>
		{/each}
	</span>
{/snippet}

<svelte:window onclick={() => (exportOpen = false)} />

{#snippet translateBanner()}
	<div
		class="flex flex-wrap items-center gap-3 rounded-lg p-3 text-sm"
		style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};"
	>
		<LanguagesIcon class="w-4 h-4 flex-shrink-0" style="color:{accent};" />
		<span class="flex-1 min-w-0">
			{m.mrv_translate_prompt({ lang: langLabel(lang), others: otherLangs.map(langLabel).join(', ') })}
		</span>
		<button
			onclick={runTranslate}
			disabled={starting}
			class="rounded-lg px-3 py-1.5 text-sm font-medium cursor-pointer disabled:opacity-50"
			style="background:{accent}; color:#fff;">{m.mrv_translate()}</button
		>
	</div>
{/snippet}

{#snippet severityBadge(s: string)}
	<span
		class="rounded px-1.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide"
		style="color:{severityColor(s)}; border:1px solid {severityColor(s)};"
		>{SEVERITY_LABEL[s as ReviewSeverity] ?? s}</span
	>
{/snippet}

<!-- The dashboard shell sets select-none; opt back in so report text can be copied. -->
<div class="p-6 space-y-4 h-full flex flex-col overflow-hidden select-text" style="background:{pageBg};">
	<!-- Header -->
	<div class="rounded-xl p-5 flex-shrink-0" style="background:{cardBg}; border:1px solid {borderColor};">
		<h2 style="font-size:18px; font-weight:600; color:{textPrimary};">{m.mrv_title()}</h2>
		<p style="font-size:13px; color:{textSecondary}; margin-top:2px;">{m.mrv_intro()}</p>
	</div>

	<div class="flex flex-1 min-h-0 gap-2" bind:this={layoutEl}>
		<!-- Left: search -->
		<div
			class="rounded-xl flex flex-col flex-shrink-0 min-h-0"
			style="width:{menuWidth}px; background:{cardBg}; border:1px solid {borderColor};"
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
					placeholder={m.mrv_search_placeholder()}
					class="flex-1 min-w-0 rounded-lg px-3 py-2 text-sm outline-none"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				/>
				<button
					type="submit"
					disabled={searching}
					class="rounded-lg px-3 py-2 cursor-pointer"
					style="background:{accent}; color:#fff;"
					aria-label={m.mrv_search()}
				>
					<SearchIcon class="w-4 h-4 {searching ? 'animate-pulse' : ''}" />
				</button>
			</form>
			{#if searchError}
				<p class="px-4 pb-2 text-sm" style="color:{danger};">{searchError}</p>
			{/if}
			<div class="flex-1 min-h-0 overflow-y-auto px-2 pb-2">
				{#if searched && results.length === 0}
					<p class="px-2 text-sm" style="color:{textMuted};">{m.mrv_no_records()}</p>
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
							{rec.title || rec.file_name || m.mrv_untitled()}
						</div>
					</button>
				{/each}
			</div>
		</div>
		<button type="button" aria-label={m.mrv_resize_menu()} class="pane-divider" onpointerdown={(e) => startPaneDrag(e, 'menu')} onpointermove={movePaneDrag} onpointerup={stopPaneDrag} onpointercancel={stopPaneDrag} onkeydown={(e) => onPaneKeydown(e, 'menu')}></button>

		<!-- Middle left: review -->
		<div
			class="rounded-xl flex-shrink-0 min-w-0 min-h-0 overflow-y-auto"
			style="width:{reportWidth}px; background:{cardBg}; border:1px solid {borderColor};"
		>
			{#if !selected}
				<div class="h-full flex items-center justify-center text-sm" style="color:{textMuted};">
					{m.mrv_select_hint()}
				</div>
			{:else}
				<div class="p-5 space-y-6">
					<!-- Record header + actions -->
					<div class="flex flex-wrap items-start justify-between gap-4">
						<div class="min-w-0">
							<div class="text-xs" style="color:{textMuted};">
								{m.mrv_record()} #{selected.id}{selected.doc_no ? ` · ${selected.doc_no}` : ''}
							</div>
							<h3 class="text-lg font-semibold" style="color:{textPrimary};">
								{selected.title || selected.file_name || m.mrv_untitled()}
							</h3>
						</div>
						<div class="flex flex-col items-start gap-2">
							<label class="flex items-center gap-2 text-sm" style="color:{textSecondary};">
								{m.mrv_model()}
								<select
									bind:value={model}
									disabled={models.length === 0}
									title={modelsError || m.mrv_model_hint()}
									class="rounded-lg px-2 py-1.5 text-sm cursor-pointer disabled:opacity-50"
									style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
								>
									{#if models.length === 0}
										<option value="">{modelsError ? m.mrv_models_error() : m.mrv_models_loading()}</option>
									{/if}
									{#each models as name (name)}
										<option value={name}>{name}</option>
									{/each}
								</select>
							</label>
							<div class="flex items-center gap-3">
								<label class="flex items-center gap-2 text-sm cursor-pointer whitespace-nowrap" style="color:{textSecondary};">
									<input type="checkbox" bind:checked={force} />
									{m.mrv_force()}
								</label>
								<button
									onclick={runReview}
									disabled={starting || review?.status === 'running'}
									class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium cursor-pointer disabled:opacity-50"
									style="background:{accent}; color:#fff;"
								>
									<SparklesIcon class="w-4 h-4" />
									{m.mrv_review()}
								</button>
								<div class="relative">
									<button
										onclick={(e) => {
											e.stopPropagation();
											exportOpen = !exportOpen;
										}}
										disabled={!report}
										aria-haspopup="menu"
										aria-expanded={exportOpen}
										class="inline-flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium cursor-pointer disabled:opacity-50 disabled:cursor-default"
										style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
									>
										<DownloadIcon class="w-4 h-4" />
										{m.mrv_export()}
										<ChevronDownIcon class="w-4 h-4" />
									</button>
									{#if exportOpen && report}
										<div
											role="menu"
											class="absolute right-0 mt-1 z-20 min-w-44 rounded-lg py-1 shadow-lg"
											style="background:{cardBg}; border:1px solid {borderColor};"
										>
											{#each [{ label: m.mrv_export_md(), run: exportMarkdown }, { label: m.mrv_export_pdf(), run: exportPdf }] as item (item.label)}
												<button
													role="menuitem"
													class="block w-full text-left px-3 py-2 text-sm cursor-pointer export-item"
													style="color:{textPrimary}; --hover-bg:{surface2};"
													onclick={(e) => {
														e.stopPropagation();
														item.run();
													}}>{item.label}</button
												>
											{/each}
										</div>
									{/if}
								</div>
							</div>
						</div>
					</div>

					<!-- Status line -->
					{#if loadingReview}
						<p class="text-sm" style="color:{textMuted};">{m.mrv_loading()}</p>
					{:else if reviewError}
						<p class="flex items-center gap-2 text-sm" style="color:{danger};">
							<CircleAlertIcon class="w-4 h-4" />{reviewError}
						</p>
					{:else if !review && otherLangs.length > 0}
						{@render translateBanner()}
					{:else if !review}
						<p class="text-sm" style="color:{textSecondary};">{m.mrv_not_reviewed()}</p>
					{:else if review.status === 'running'}
						<p class="flex items-center gap-2 text-sm" style="color:{accent};">
							<RefreshCwIcon class="w-4 h-4 animate-spin" />
							{review.translated_from_id
								? m.mrv_translating({ id: review.translated_from_id, time: fmtTime(review.created_at) })
								: m.mrv_running({ count: review.metrics_count, time: fmtTime(review.created_at) })}
						</p>
					{:else if review.status === 'failed'}
						<p class="flex items-start gap-2 text-sm" style="color:{danger};">
							<CircleAlertIcon class="w-4 h-4 mt-0.5 flex-shrink-0" />
							<span
								>{(review.translated_from_id ? m.mrv_translate_failed : m.mrv_failed)({
									time: fmtTime(review.created_at),
									error: review.error_msg ?? ''
								})}</span
							>
						</p>
						{#if otherLangs.length > 0}
							{@render translateBanner()}
						{/if}
					{:else}
						<p class="text-xs" style="color:{textMuted};">
							{m.mrv_reviewed()}
							{fmtTime(review.finished_at || review.created_at)} · {review.model_name} · {review.prompt_name}{review.created_by
								? ` · ${m.mrv_by({ user: review.created_by })}`
								: ''}{review.translated_from_id ? ` · ${m.mrv_translated_from({ id: review.translated_from_id })}` : ''}
						</p>
					{/if}

					{#if report}
						<!-- Summary -->
						<p class="text-[15px] leading-relaxed" style="color:{textPrimary};">{report.summary}</p>

						<!-- Tally -->
						<div class="grid grid-cols-3 md:grid-cols-6 gap-3">
							{#each [{ label: m.mrv_tally_stored(), n: report.tally.stored, c: textPrimary }, { label: m.mrv_tally_kept(), n: report.tally.kept, c: success }, { label: m.mrv_tally_not_metric(), n: report.tally.not_metric, c: danger }, { label: m.mrv_tally_duplicate(), n: report.tally.duplicate, c: warning }, { label: m.mrv_tally_formula_input(), n: report.tally.formula_input, c: warning }, { label: m.mrv_tally_missed(), n: report.tally.missed, c: accent }] as t (t.label)}
								<div class="rounded-lg p-3" style="background:{surface2};">
									<div class="text-2xl font-semibold" style="color:{t.c};">{t.n}</div>
									<div class="text-xs" style="color:{textSecondary};">{t.label}</div>
								</div>
							{/each}
						</div>

						<!-- 1. Missed metrics -->
						<section class="space-y-2">
							<h4 class="font-semibold" style="color:{textPrimary};">
								{m.mrv_sec_missed()} <span style="color:{textMuted};">({report.missed_metrics.length})</span>
							</h4>
							{#if report.missed_metrics.length === 0}
								<p class="text-sm" style="color:{textMuted};">{m.mrv_none_missed()}</p>
							{/if}
							{#each sortBySeverity(report.missed_metrics) as mm, i (i)}
				<div role="button" tabindex="0" aria-pressed={selectedFindingKey === `missed:${i}`} class="rounded-lg p-3 space-y-1 cursor-pointer" style="background:{selectedFindingKey === `missed:${i}` ? surface2 : 'transparent'}; border:1px solid {selectedFindingKey === `missed:${i}` ? accent : borderColor};" onclick={() => showSource(`missed:${i}`, mm.source_line_spans ?? mm.lines.split(','))} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); showSource(`missed:${i}`, mm.source_line_spans ?? mm.lines.split(',')); } }}>
									<div class="flex flex-wrap items-center gap-2">
										{@render severityBadge(mm.severity)}
										<span class="font-medium" style="color:{textPrimary};">{mm.name}</span>
										{#if mm.value}<span style="color:{accent};">{mm.value} {mm.unit}</span>{/if}
										{#if mm.lines}<span class="text-xs" style="color:{textMuted};">L{mm.lines}</span>{/if}
									</div>
									<p class="text-sm" style="color:{textSecondary};">{mm.reason}</p>
								</div>
							{/each}
						</section>

						<!-- 2. Not metrics -->
						<section class="space-y-3">
							<h4 class="font-semibold" style="color:{textPrimary};">
								{m.mrv_sec_non_metrics()}
								<span style="color:{textMuted};"
									>({report.tally.not_metric + report.tally.duplicate + report.tally.formula_input})</span
								>
							</h4>
							{#if report.non_metrics.length === 0}
								<p class="text-sm" style="color:{textMuted};">{m.mrv_none()}</p>
							{/if}
							{#each groupNonMetrics(report.non_metrics) as g (g.category)}
								<div class="space-y-2">
									<div class="text-xs font-semibold uppercase tracking-wide" style="color:{textSecondary};">
										{CATEGORY_LABEL[g.category]}
									</div>
									{#each g.entries as e, i (i)}
						<div role="button" tabindex="0" aria-pressed={selectedFindingKey === `non:${g.category}:${i}`} class="rounded-lg p-3 space-y-1.5 cursor-pointer" style="background:{selectedFindingKey === `non:${g.category}:${i}` ? surface2 : 'transparent'}; border:1px solid {selectedFindingKey === `non:${g.category}:${i}` ? accent : borderColor};" onclick={() => showSource(`non:${g.category}:${i}`, sourceForMetricIds(e.metric_ids))} onkeydown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); showSource(`non:${g.category}:${i}`, sourceForMetricIds(e.metric_ids)); } }}>
											<div class="flex flex-wrap items-center gap-2">
												{@render metricChips(e.metric_ids)}
												{#if e.duplicate_of}
													<span class="text-xs" style="color:{textMuted};">{m.mrv_duplicate_of()}</span>
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
								{m.mrv_sec_attributes()} <span style="color:{textMuted};">({report.attribute_issues.length})</span>
							</h4>
							{#if report.attribute_issues.length === 0}
								<p class="text-sm" style="color:{textMuted};">{m.mrv_none()}</p>
							{/if}
							{#each sortBySeverity(report.attribute_issues) as a, i (i)}
				<div role="button" tabindex="0" aria-pressed={selectedFindingKey === `attribute:${i}`} class="rounded-lg p-3 space-y-1.5 cursor-pointer" style="background:{selectedFindingKey === `attribute:${i}` ? surface2 : 'transparent'}; border:1px solid {selectedFindingKey === `attribute:${i}` ? accent : borderColor};" onclick={() => showSource(`attribute:${i}`, sourceForMetricIds(a.metric_ids))} onkeydown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); showSource(`attribute:${i}`, sourceForMetricIds(a.metric_ids)); } }}>
									<div class="flex flex-wrap items-center gap-2">
										{@render severityBadge(a.severity)}
										<code class="text-xs rounded px-1.5 py-0.5" style="background:{surface2}; color:{accent};"
											>{a.field}</code
										>
										{@render metricChips(a.metric_ids)}
									</div>
									{#if a.stored || a.suggested}
										<div class="text-sm flex flex-wrap items-center gap-2">
											<span style="color:{danger}; text-decoration:line-through;">{a.stored || m.mrv_empty()}</span>
											<span style="color:{textMuted};">→</span>
											<span style="color:{success};">{a.suggested || m.mrv_empty()}</span>
										</div>
									{/if}
									<p class="text-sm" style="color:{textSecondary};">{a.reason}</p>
								</div>
							{/each}
						</section>

						<!-- Recommendations -->
						{#if report.recommendations.length > 0}
							<section class="space-y-2">
								<h4 class="font-semibold" style="color:{textPrimary};">{m.mrv_sec_recommendations()}</h4>
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
		<button type="button" aria-label={m.mrv_resize_report()} class="pane-divider" onpointerdown={(e) => startPaneDrag(e, 'report')} onpointermove={movePaneDrag} onpointerup={stopPaneDrag} onpointercancel={stopPaneDrag} onkeydown={(e) => onPaneKeydown(e, 'report')}></button>
		<div class="rounded-xl flex flex-col flex-1 min-w-0 min-h-0 overflow-hidden" style="background:{cardBg}; border:1px solid {borderColor};">
			{#if selected}
				{#if pdfError}<p class="p-4 text-sm" style="color:{danger};">{pdfError}</p>{/if}
				<PdfViewWindow inputId={selected.id} fileUrl={`/api/v1/kb/inputs/${selected.id}/file`} bind:page={pdfPage} bind:zoom={pdfZoom} bind:numPages={pdfPages} {darkMode} showSidebar={false} enableSelectionDialog={false} {highlightVersion} renderHighlights={renderSourceHighlights} />
			{:else}
				<div class="h-full flex items-center justify-center text-sm" style="color:{textMuted};">{m.mrv_pdf_hint()}</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.pane-divider { width: 10px; flex: none; padding: 0; border: 0; cursor: col-resize; border-radius: 5px; background: transparent; touch-action: none; }
	.pane-divider:hover, .pane-divider:focus-visible { background: rgba(129, 140, 248, .4); outline: none; }
	.export-item:hover {
		background: var(--hover-bg);
	}
</style>
