<script lang="ts">
	import SharedPdfViewer from '$lib/components/home3/shared-pdf-viewer.svelte';
	import type { PdfPageViewport } from '$lib/components/home3/shared-pdf-viewer.svelte';
	import { getRawLines, listKbMetrics, type RawLine } from '$lib/services/kbService';
	import { recordIdFromMetricId, resolveMetricSpans } from './metric-source-spans';
	import type { ExplorerTokens } from './theme';

	let {
		tokens,
		metricId = '',
		evidenceSpans = [],
		evidenceLabel = ''
	}: {
		tokens: ExplorerTokens;
		metricId?: string;
		evidenceSpans?: string[];
		evidenceLabel?: string;
	} = $props();

	type Src = { inputId: number; fileUrl: string; title: string };
	let src = $state<Src | null>(null);
	let err = $state('');
	let page = $state(1);
	let zoom = $state(0.5);
	let spanCount = $state(0);
	let highlightVersion = $state('');
	let linesByPage = $state<Map<number, RawLine[]>>(new Map());

	// MinerU line boxes are normalised to a 1000×1000 space.
	const MINERU_COORD_SIZE = 1000;

	$effect(() => {
		const id = metricId.trim();
		src = null;
		err = '';
		spanCount = 0;
		highlightVersion = '';
		linesByPage = new Map();
		if (!id) return;

		const recordId = recordIdFromMetricId(id);
		if (!recordId) {
			err = `Unrecognised metric id "${id}".`;
			return;
		}

		// The record id is the metric_id prefix, so the PDF can load right away —
		// no wiki round-trip. Page + highlights layer on once raw lines arrive.
		src = {
			inputId: recordId,
			fileUrl: `/api/v1/kb/inputs/${recordId}/file`,
			title: `input ${recordId}`
		};
		page = 1;

		let cancelled = false;
		Promise.all([listKbMetrics(recordId), getRawLines(recordId)])
			.then(([metricsRes, rawRes]) => {
				if (cancelled) return;
				const lines = rawRes?.lines ?? [];
				if (rawRes?.file_name && src) src = { ...src, title: rawRes.file_name };

				const metric = (metricsRes.results ?? []).find((m) => m.metric_id === id) ?? null;
				const spans = resolveMetricSpans(metric?.source_line_spans, lines);
				spanCount = spans.length;

				const byKey = new Map<string, RawLine>();
				for (const ln of lines) byKey.set(`${ln.page_number}:${ln.line_number}`, ln);
				const grouped = new Map<number, RawLine[]>();
				for (const s of spans) {
					const ln = byKey.get(`${s.page_number}:${s.line_number}`);
					if (ln && Array.isArray(ln.coords) && ln.coords.length >= 4) {
						const arr = grouped.get(s.page_number) ?? [];
						arr.push(ln);
						grouped.set(s.page_number, arr);
					}
				}
				linesByPage = grouped;
				if (spans[0]) page = spans[0].page_number > 0 ? spans[0].page_number : 1;
				highlightVersion = `${id}:${spans.length}`;
			})
			.catch((e) => {
				if (!cancelled) err = e instanceof Error ? e.message : String(e);
			});
		return () => {
			cancelled = true;
		};
	});

	function renderHighlights(pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) {
		for (const ln of linesByPage.get(pageNo) ?? []) {
			const c = ln.coords;
			if (!Array.isArray(c) || c.length < 4) continue;
			const left = (Math.min(c[0], c[2]) * viewport.width) / MINERU_COORD_SIZE;
			const top = (Math.min(c[1], c[3]) * viewport.height) / MINERU_COORD_SIZE;
			const width = (Math.abs(c[2] - c[0]) * viewport.width) / MINERU_COORD_SIZE;
			const height = (Math.abs(c[3] - c[1]) * viewport.height) / MINERU_COORD_SIZE;
			if (width < 1 || height < 1) continue;
			const box = document.createElement('div');
			box.className = 'pdf-highlight';
			box.style.left = `${Math.max(0, left - 4)}px`;
			box.style.top = `${Math.max(0, top - 2)}px`;
			box.style.width = `${width + 12}px`;
			box.style.height = `${height + 4}px`;
			box.title = `line ${ln.line_number}`;
			overlay.appendChild(box);
		}
	}
</script>

<div
	class="sp"
	style="--panel:{tokens.panelBg}; --border:{tokens.border}; --text:{tokens.textPrimary}; --text-2:{tokens.textSecondary}; --accent:{tokens.accent};"
>
	<div class="bar">
		<span class="lbl">Source document</span>
		{#if src}<span class="doc">{src.title}</span>{/if}
		<span class="grow"></span>
		{#if src && spanCount}
			<span class="ev">{spanCount} line{spanCount > 1 ? 's' : ''} highlighted</span>
		{:else if evidenceSpans.length}
			<span class="ev"
				>{evidenceSpans.length} evidence span{evidenceSpans.length > 1 ? 's' : ''}{evidenceLabel
					? ` · ${evidenceLabel}`
					: ''}</span
			>
		{/if}
	</div>

	<div class="stage">
		{#if src}
			<SharedPdfViewer
				inputId={src.inputId}
				fileUrl={src.fileUrl}
				bind:page
				bind:zoom
				{highlightVersion}
				renderHighlights={spanCount ? renderHighlights : undefined}
			/>
		{:else if err}
			<div class="empty">{err}</div>
		{:else}
			<div class="empty">
				<p>No metric selected.</p>
				<p class="hint">
					Open this page with <code>?metric_id=&lt;id&gt;</code> — from the metric list or a search result
					— to load its source document here.
				</p>
			</div>
		{/if}
	</div>
</div>

<style>
	.sp {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
		background: var(--panel);
		color: var(--text);
	}
	.bar {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 14px;
		border-bottom: 1px solid var(--border);
		flex: 0 0 auto;
	}
	.bar .lbl {
		font:
			600 10px/1 ui-sans-serif,
			system-ui,
			sans-serif;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: var(--text-2);
	}
	.bar .doc {
		font:
			italic 13px/1 'Fraunces',
			Georgia,
			serif;
		color: var(--text-2);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.bar .grow {
		flex: 1;
	}
	.bar .ev {
		font:
			600 10px/1 ui-sans-serif,
			system-ui,
			sans-serif;
		letter-spacing: 0.04em;
		color: var(--accent);
	}
	.stage {
		flex: 1 1 auto;
		min-height: 0;
		position: relative;
		display: flex;
	}
	.empty {
		margin: auto;
		max-width: 42ch;
		text-align: center;
		color: var(--text-2);
		font:
			400 13px/1.55 ui-sans-serif,
			system-ui,
			sans-serif;
		padding: 24px;
	}
	.empty .hint {
		font-size: 12px;
		color: var(--text-2);
		opacity: 0.85;
		margin-top: 8px;
	}
	:global(.pdf-highlight) {
		position: absolute;
		background: rgba(129, 140, 248, 0.18);
		border: 1px solid rgba(129, 140, 248, 0.9);
		box-shadow: inset 0 0 0 1px rgba(199, 210, 254, 0.25);
		pointer-events: none;
	}
	code {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.92em;
	}
</style>
