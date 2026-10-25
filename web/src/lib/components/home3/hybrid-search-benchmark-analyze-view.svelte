<script lang="ts">
	// Analyze tab of the Hybrid Search Benchmark (openspec hybrid-search-benchmark-analyze-page):
	// the question's log on the left, the clicked metric's document on the right (reusable PDF
	// viewer), and the metric's details in the dashboard's right panel (Metric Detail Visualizer).
	import { onDestroy, onMount, type Snippet } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import PdfViewWindow from './pdf-view-window.svelte';
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';
	import type { TableReference } from './pdf-table-geometry';
	import MetricDetailVisualizer from '$lib/components/shared-ui/metric-detail/metric-detail-visualizer.svelte';
	import HybridSearchBenchmarkTraceLog from './hybrid-search-benchmark-trace-log.svelte';
	import {
		lineNumbersFromSpans,
		recordIdFromMetricId,
		resolveMetricSpans
	} from './metric-ontology-explorer/metric-source-spans';
	import {
		getBenchmarkTrace,
		type HybridBenchmarkResult,
		type TraceQuery
	} from '$lib/services/hybridSearchBenchmarkService';
	import {
		getRawLines,
		listKbMetrics,
		type KbMetricRecord,
		type RawLine
	} from '$lib/services/kbService';

	let {
		darkMode = true,
		resultId,
		onMetricDetailsChange
	}: {
		darkMode?: boolean;
		resultId: number;
		onMetricDetailsChange?: (details: Snippet | null) => void;
	} = $props();

	type RecordDocument = { metrics: KbMetricRecord[]; rawLines: RawLine[] };

	let loading = $state(true);
	let loadError = $state(false);
	let result = $state<HybridBenchmarkResult | null>(null);
	let queries = $state<TraceQuery[]>([]);
	let limit = $state(0);
	let backend = $state('');

	let selectedMetricId = $state<string | null>(null);
	let selectedMetric = $state<KbMetricRecord | null>(null);
	let metricLoading = $state(false);
	let metricMessage = $state('');
	let inputId = $state<number | null>(null);
	let rawLines = $state<RawLine[]>([]);
	let highlightedLines = $state<number[]>([]);
	let tableReferences = $state<TableReference[]>([]);
	let highlightVersion = $state(0);
	let pdfPage = $state(1);
	let pdfZoom = $state(0.5);
	let selectSeq = 0;
	// Each record's metrics and raw lines, loaded once per tab.
	const documents = new Map<number, Promise<RecordDocument>>();

	let splitEl = $state<HTMLDivElement | null>(null);
	let leftRatio = $state(0.5);
	let resizing = $state(false);
	let controller: AbortController | undefined;

	onMount(() => {
		onMetricDetailsChange?.(metricDetails as unknown as Snippet);
		void load();
	});
	onDestroy(() => {
		controller?.abort();
		onMetricDetailsChange?.(null);
	});

	async function load() {
		controller?.abort();
		const current = new AbortController();
		controller = current;
		loading = true;
		loadError = false;
		try {
			const trace = await getBenchmarkTrace(resultId, current.signal);
			if (current.signal.aborted) return;
			result = trace.result;
			queries = trace.queries ?? [];
			limit = trace.limit;
			backend = trace.lexical_backend;
		} catch {
			if (!current.signal.aborted) loadError = true;
		} finally {
			if (!current.signal.aborted) loading = false;
		}
	}

	function loadDocument(recordId: number): Promise<RecordDocument> {
		let doc = documents.get(recordId);
		if (!doc) {
			doc = Promise.all([listKbMetrics(recordId), getRawLines(recordId)]).then(
				([metrics, lines]) => ({
					metrics: metrics.results ?? [],
					rawLines: lines.lines ?? []
				})
			);
			doc.catch(() => documents.delete(recordId));
			documents.set(recordId, doc);
		}
		return doc;
	}

	// Opens the metric's document at the page of its first source line and highlights its lines.
	async function selectMetric(metricId: string) {
		const seq = ++selectSeq;
		selectedMetricId = metricId;
		metricMessage = '';
		const recordId = recordIdFromMetricId(metricId);
		if (recordId === null) {
			selectedMetric = null;
			metricMessage = m.hsba_metric_not_found({ id: metricId });
			return;
		}
		metricLoading = true;
		try {
			const doc = await loadDocument(recordId);
			if (seq !== selectSeq) return;
			const metric = doc.metrics.find((x) => x.metric_id === metricId) ?? null;
			if (!metric) {
				selectedMetric = null;
				metricMessage = m.hsba_metric_not_found({ id: metricId });
				return;
			}
			const first = resolveMetricSpans(metric.source_line_spans, doc.rawLines)[0];
			pdfPage = first?.page_number ?? 1;
			inputId = recordId;
			rawLines = doc.rawLines;
			selectedMetric = metric;
			highlightedLines = lineNumbersFromSpans(metric.source_line_spans);
			tableReferences = metric.source_table_rows ?? [];
			highlightVersion++;
		} catch {
			if (seq === selectSeq) {
				selectedMetric = null;
				metricMessage = m.hsba_metric_load_error({ id: metricId });
			}
		} finally {
			if (seq === selectSeq) metricLoading = false;
		}
	}

	// Draws the metric's source lines; table lines whose rows the viewer located are drawn by it.
	function renderHighlights(
		pageNo: number,
		viewport: PdfPageViewport,
		overlay: HTMLDivElement,
		resolvedTableLines: ReadonlySet<number>
	) {
		for (const line of rawLines) {
			if (resolvedTableLines.has(line.line_number)) continue;
			if (
				line.page_number !== pageNo ||
				!highlightedLines.includes(line.line_number) ||
				!Array.isArray(line.coords) ||
				line.coords.length < 4
			)
				continue;
			const [x1, y1, x2, y2] = line.coords;
			const mark = document.createElement('div');
			mark.className = 'pdf-highlight';
			mark.style.cssText = `position:absolute;left:${(Math.min(x1, x2) * viewport.width) / 1000}px;top:${(Math.min(y1, y2) * viewport.height) / 1000}px;width:${(Math.abs(x2 - x1) * viewport.width) / 1000 + 20}px;height:${Math.max(2, (Math.abs(y2 - y1) * viewport.height) / 1000)}px;background:rgba(129,140,248,.35);pointer-events:none;`;
			overlay.appendChild(mark);
		}
	}

	function startResize(event: PointerEvent) {
		if (window.matchMedia('(max-width: 900px)').matches) return;
		event.preventDefault();
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		resizing = true;
	}
	function moveResize(event: PointerEvent) {
		if (!resizing || !splitEl) return;
		const bounds = splitEl.getBoundingClientRect();
		if (bounds.width > 0)
			leftRatio = Math.min(0.75, Math.max(0.25, (event.clientX - bounds.left) / bounds.width));
	}
	function endResize() {
		resizing = false;
	}
	function resizeWithKeyboard(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
			event.preventDefault();
			leftRatio = Math.min(
				0.75,
				Math.max(0.25, leftRatio + (event.key === 'ArrowLeft' ? -0.05 : 0.05))
			);
		} else if (event.key === 'Home') {
			event.preventDefault();
			leftRatio = 0.25;
		} else if (event.key === 'End') {
			event.preventDefault();
			leftRatio = 0.75;
		}
	}

	// Metric Detail Visualizer theme tokens (it reads the page's CSS variables).
	let detailTokens = $derived(
		darkMode
			? '--panel-bg:#1f2333; --panel-bg-alt:#252a3a; --ink-line:#2d3348; --ink-line-soft:#262b3b; --text-primary:#e2e8f0; --text-secondary:#cbd5e1; --text-muted:#94a3b8; --brass:#a5b4fc;'
			: '--panel-bg:#ffffff; --panel-bg-alt:#f6f7f9; --ink-line:#e4e6eb; --ink-line-soft:#eef0f3; --text-primary:#111827; --text-secondary:#374151; --text-muted:#6b7280; --brass:#4f46e5;'
	);
</script>

{#snippet metricDetails()}
	<div class="hsba-details select-text" style={detailTokens}>
		{#if selectedMetric}
			<MetricDetailVisualizer metric={selectedMetric} {rawLines} />
		{:else}
			<p class="details-empty">{m.hsba_details_empty()}</p>
		{/if}
	</div>
{/snippet}

<div class="hsba-page select-text" class:dark={darkMode}>
	<div
		class="split"
		bind:this={splitEl}
		style={`--left-fr: ${leftRatio}fr; --right-fr: ${1 - leftRatio}fr;`}
	>
		<section class="panel content" aria-label={m.hsba_content_panel()}>
			{#if loading}<p class="hint" role="status">{m.hsba_loading()}</p>
			{:else if loadError}
				<p class="error" role="alert">
					{m.hsba_load_error()}
					<button type="button" onclick={() => void load()}>{m.qg_retry()}</button>
				</p>
			{:else if result}
				<HybridSearchBenchmarkTraceLog
					{result}
					{queries}
					{limit}
					{backend}
					{selectedMetricId}
					onSelectMetric={(id) => void selectMetric(id)}
				/>
			{/if}
		</section>
		<button
			type="button"
			class="resizer"
			class:resizing
			role="slider"
			aria-orientation="horizontal"
			aria-label={m.hsba_resize_panels()}
			aria-valuemin="25"
			aria-valuemax="75"
			aria-valuenow={Math.round(leftRatio * 100)}
			onpointerdown={startResize}
			onpointermove={moveResize}
			onpointerup={endResize}
			onpointercancel={endResize}
			onkeydown={resizeWithKeyboard}
		></button>
		<aside class="panel pdf" aria-label={m.hsba_pdf_panel()}>
			<h2>
				{m.hsba_pdf_panel()}{#if selectedMetric}<span class="pdf-meta"
						>{m.hsba_pdf_metric({
							metric: selectedMetric.metric_id ?? '',
							record: selectedMetric.input_record_id
						})}{#if selectedMetric.input_filename}
							· {selectedMetric.input_filename}{/if}</span
					>{/if}
			</h2>
			{#if metricLoading}<p class="hint" role="status">{m.hsba_metric_loading()}</p>{/if}
			{#if metricMessage}<p class="error" role="alert">{metricMessage}</p>{/if}
			{#if inputId !== null && selectedMetric}
				{#key inputId}
					<PdfViewWindow
						{inputId}
						fileUrl={`/api/v1/kb/inputs/${inputId}/file`}
						bind:page={pdfPage}
						bind:zoom={pdfZoom}
						{highlightVersion}
						{tableReferences}
						{renderHighlights}
						enableSelectionDialog={false}
						{darkMode}
					/>
				{/key}
			{:else if !metricLoading && !metricMessage}
				<p class="hint pdf-empty">{m.hsba_pdf_empty()}</p>
			{/if}
		</aside>
	</div>
</div>

<style>
	.hsba-page {
		color-scheme: light;
		--bg: #f2f4f7;
		--panel: #fff;
		--line: #e4e6eb;
		--text: #111827;
		--muted: #6b7280;
		--accent: #4f46e5;
		--hit: #15803d;
		--picked: #c7d2fe;
		height: 100%;
		box-sizing: border-box;
		padding: 16px;
		background: var(--bg);
		color: var(--text);
		user-select: text;
	}
	.hsba-page.dark {
		color-scheme: dark;
		--bg: #171b26;
		--panel: #1f2333;
		--line: #2d3348;
		--text: #e2e8f0;
		--muted: #94a3b8;
		--accent: #a5b4fc;
		--hit: #4ade80;
		--picked: #3d4566;
	}
	.split {
		height: 100%;
		display: grid;
		grid-template-columns: minmax(0, var(--left-fr)) 12px minmax(0, var(--right-fr));
	}
	.panel {
		min-width: 0;
		min-height: 0;
		border: 1px solid var(--line);
		border-radius: 12px;
		background: var(--panel);
		padding: 14px 16px;
		box-sizing: border-box;
	}
	.content {
		overflow: auto;
	}
	.pdf {
		display: flex;
		flex-direction: column;
		gap: 8px;
		overflow: hidden;
	}
	.pdf h2 {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		display: flex;
		flex-wrap: wrap;
		gap: 4px 10px;
		align-items: baseline;
	}
	.pdf-meta {
		font-size: 12px;
		font-weight: 400;
		color: var(--muted);
	}
	.pdf-empty {
		margin: auto;
		text-align: center;
	}
	.resizer {
		position: relative;
		width: 12px;
		margin: 0;
		padding: 0;
		border: 0;
		background: transparent;
		cursor: col-resize;
		touch-action: none;
		user-select: none;
	}
	.resizer::before {
		position: absolute;
		inset: 0 4px;
		border-radius: 4px;
		background: var(--line);
		content: '';
	}
	.resizer:hover::before,
	.resizer:focus-visible::before,
	.resizer.resizing::before {
		background: var(--accent);
	}
	.resizer:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 1px;
	}
	.hint {
		font-size: 12px;
		color: var(--muted);
	}
	.error {
		color: #ef7373;
		font-size: 12px;
	}
	button:not(.resizer) {
		color: var(--text);
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 6px 12px;
		font-size: 13px;
		cursor: pointer;
		background: var(--bg);
	}
	.hsba-details {
		flex: 1;
		min-width: 0;
		min-height: 0;
		display: flex;
		flex-direction: column;
	}
	.details-empty {
		font-size: 13px;
		color: var(--text-muted);
		margin: 8px 0;
	}
	@media (max-width: 900px) {
		.hsba-page {
			height: auto;
			padding: 12px;
		}
		.split {
			grid-template-columns: minmax(0, 1fr);
			gap: 12px;
		}
		.resizer {
			display: none;
		}
		.content {
			max-height: none;
		}
		.pdf {
			height: 70vh;
			min-height: 360px;
		}
	}
</style>
