<script lang="ts">
	import type { Snippet } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import PdfViewWindow from './pdf-view-window.svelte';
	import type { TableReference } from './pdf-table-geometry';
	import type { PdfPageViewport } from './shared-pdf-viewer.svelte';

	let {
		run,
		scoreCards,
		warnings,
		provenance,
		evidenceActions = [],
		inputId,
		darkMode,
		sourcePage,
		sourcePageVersion,
		renderSourceHighlights,
		onFrameElement,
		reportBody
	}: {
		run: { id: string | number; title: string; status: string; documentTitle: string; createdAt: string; subtitle?: string };
		scoreCards: { label: string; value: string; detail?: string }[];
		warnings: string[];
		provenance?: Snippet;
		evidenceActions?: { id: string; label: string; view(): void; download?(): void }[];
		inputId: number | null;
		darkMode: boolean;
		sourcePage?: number;
		sourcePageVersion?: number;
		renderSourceHighlights?: (pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement, resolvedTableLines: ReadonlySet<number>) => void;
		onFrameElement?: (element: HTMLDivElement | null) => void;
		reportBody: Snippet<[{ selectSource(lines: number[], tableReferences?: TableReference[]): void; selectedSourceLines: number[]; selectedTableReferences: TableReference[] }]>
	} = $props();

	let selectedSourceLines = $state<number[]>([]);
	let selectedTableReferences = $state<TableReference[]>([]);
	let highlightVersion = $state(0);
	let pdfPage = $state(1);
	let pdfZoom = $state(0.5);
	let splitPanelsEl = $state<HTMLDivElement | null>(null);
	let leftPanelRatio = $state(0.5);
	let resizingPanels = $state(false);
	let previousIdentity: { runId: string | number; inputId: number | null } | null = null;

	$effect(() => onFrameElement?.(splitPanelsEl));

	function selectSource(lines: number[], tableReferences: TableReference[] = []) {
		selectedSourceLines = [...lines];
		selectedTableReferences = [...tableReferences];
		highlightVersion++;
	}

	$effect(() => {
		void sourcePageVersion;
		if (sourcePage != null) pdfPage = sourcePage;
	});

	$effect(() => {
		const identity = { runId: run.id, inputId };
		if (previousIdentity && (previousIdentity.runId !== identity.runId || previousIdentity.inputId !== identity.inputId)) {
			selectedSourceLines = [];
			selectedTableReferences = [];
			highlightVersion++;
			pdfPage = 1;
			pdfZoom = 0.5;
		}
		previousIdentity = identity;
	});

	function startPanelResize(event: PointerEvent) {
		if (window.matchMedia('(max-width: 700px)').matches) return;
		event.preventDefault();
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		resizingPanels = true;
	}
	function movePanelResize(event: PointerEvent) {
		if (!resizingPanels || !splitPanelsEl) return;
		const bounds = splitPanelsEl.getBoundingClientRect();
		if (bounds.width > 0) leftPanelRatio = Math.min(0.75, Math.max(0.25, (event.clientX - bounds.left) / bounds.width));
	}
	function endPanelResize() { resizingPanels = false; }
	function resizePanelsWithKeyboard(event: KeyboardEvent) {
		if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
			event.preventDefault();
			leftPanelRatio = Math.min(0.75, Math.max(0.25, leftPanelRatio + (event.key === 'ArrowLeft' ? -0.05 : 0.05)));
		} else if (event.key === 'Home') { event.preventDefault(); leftPanelRatio = 0.25; }
		else if (event.key === 'End') { event.preventDefault(); leftPanelRatio = 0.75; }
	}
</script>

<div class="benchmark select-text" class:light={!darkMode}>
	<div class="split-panels" bind:this={splitPanelsEl} style={`--left-panel-fr: ${leftPanelRatio}fr; --right-panel-fr: ${1 - leftPanelRatio}fr;`}>
		<section class="result-panel" aria-label={run.title}>
			<header>
				<h1>{run.title}{run.status ? ` · ${run.status}` : ''}</h1>
				{#if run.documentTitle || run.createdAt}<p>{run.documentTitle}{run.documentTitle && run.createdAt ? ' · ' : ''}{run.createdAt}</p>{/if}
				{#if run.subtitle}<p>{run.subtitle}</p>{/if}
			</header>
			{#if scoreCards.length}<div class="stats">{#each scoreCards as card}<div><span>{card.label}</span><strong>{card.value}</strong>{#if card.detail}<small>{card.detail}</small>{/if}</div>{/each}</div>{/if}
			{#if warnings.length}<div class="warnings"><h2>{m.msc_warnings()}</h2><ul>{#each warnings as warning}<li>{warning}</li>{/each}</ul></div>{/if}
			{#if provenance}<div class="provenance">{@render provenance()}</div>{/if}
			{#if evidenceActions.length}<div class="actions evidence-actions"><h2>{m.msc_evidence()}</h2>{#each evidenceActions as action (action.id)}<div class="evidence-action"><button onclick={action.view}>{action.label}</button>{#if action.download}<button onclick={action.download}>{m.msc_download()}</button>{/if}</div>{/each}</div>{/if}
			{@render reportBody({ selectSource, selectedSourceLines, selectedTableReferences })}
		</section>
		<button type="button" class="panel-resizer" class:resizing={resizingPanels} role="slider" aria-orientation="horizontal" aria-label={m.msc_resize_panels()} aria-valuemin="25" aria-valuemax="75" aria-valuenow={Math.round(leftPanelRatio * 100)} onpointerdown={startPanelResize} onpointermove={movePanelResize} onpointerup={endPanelResize} onpointercancel={endPanelResize} onkeydown={resizePanelsWithKeyboard}></button>
		<aside class="pdf-panel" aria-label={m.msc_pdf_panel()}>
			<h2>{m.msc_pdf_panel()}</h2>
			{#if inputId != null}
				{#key `${run.id}|${inputId}`}
					<PdfViewWindow inputId={inputId} fileUrl={`/api/v1/kb/inputs/${inputId}/file`} bind:page={pdfPage} bind:zoom={pdfZoom} {highlightVersion} tableReferences={selectedTableReferences} renderHighlights={renderSourceHighlights} enableSelectionDialog={false} {darkMode} />
				{/key}
			{:else}<p class="muted pdf-empty">{m.msc_pdf_empty()}</p>{/if}
		</aside>
	</div>
</div>

<style>
	.benchmark { --bg:#171b26; --card:#1f2333; --border:#2d3348; --text:#e2e8f0; --muted:#94a3b8; --surface:#252a3a; --accent:#818cf8; padding:24px; background:var(--bg); color:var(--text); min-height:100%; user-select:text; }
	.light { --bg:#f2f4f7; --card:#fff; --border:#e4e6eb; --text:#111827; --muted:#6b7280; --surface:#eceef2; --accent:#6366f1; }
	header { margin-bottom:22px; } h1 { font-size:24px; font-weight:650; } h2 { font-size:18px; font-weight:600; } p { margin:10px 0; } header p,.muted,small { color:var(--muted); }
	.split-panels { display:grid; grid-template-columns:minmax(0,var(--left-panel-fr)) 12px minmax(0,var(--right-panel-fr)); align-items:stretch; }
	.result-panel,.pdf-panel { min-width:0; height:min(75vh,900px); min-height:480px; border:1px solid var(--border); border-radius:12px; background:var(--card); padding:16px; box-sizing:border-box; }
	.result-panel { overflow:auto; } .pdf-panel { display:flex; flex-direction:column; gap:10px; overflow:hidden; } .pdf-panel h2 { margin:0; flex:0 0 auto; } .pdf-empty { margin:auto; text-align:center; }
	.panel-resizer { position:relative; width:12px; min-width:12px; margin:0; padding:0; border:0; border-radius:8px; background:transparent; cursor:col-resize; touch-action:none; user-select:none; }
	.panel-resizer::before { position:absolute; inset:0 4px; border-radius:4px; background:var(--border); content:''; } .panel-resizer:hover::before,.panel-resizer:focus-visible::before,.panel-resizer.resizing::before { background:var(--accent); }
	.panel-resizer:focus-visible { outline:2px solid var(--accent); outline-offset:1px; } .stats { display:grid; grid-template-columns:repeat(auto-fit,minmax(130px,1fr)); gap:12px; margin:16px 0; } .stats div { padding:14px; background:var(--surface); border-radius:8px; } .stats span,.stats small { display:block; font-size:12px; color:var(--muted); } .stats strong { display:block; font-size:22px; margin-top:6px; }
	.warnings { color:#fbbf24; } .light .warnings { color:#a16207; } ul { padding-left:20px; } .actions { display:flex; flex-wrap:wrap; gap:10px; align-items:center; } .evidence-actions h2 { flex-basis:100%; } .evidence-action { display:flex; gap:6px; } button { border:1px solid var(--border); border-radius:7px; padding:9px 12px; background:var(--surface); color:var(--text); font-size:13px; cursor:pointer; }
	@media(max-width:700px) { .split-panels { grid-template-columns:minmax(0,1fr); } .panel-resizer { display:none; } .result-panel,.pdf-panel { height:60vh; min-height:360px; } .benchmark { padding:12px; } }
</style>
