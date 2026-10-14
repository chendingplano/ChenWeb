<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import PdfViewWindow from '$lib/components/home3/pdf-view-window.svelte';
	import type { PdfPageViewport } from '$lib/components/home3/shared-pdf-viewer.svelte';
	import type { TableReference } from '$lib/components/home3/pdf-table-geometry';
	import { getRawLines, type RawLine } from '$lib/services/kbService';
	import { resolveReferenceLines, paintReferenceHighlights } from './reference-grounding';

	let { inputId, darkMode, spans = [], tables = [], selectionVersion = 0, documentTitle = '', settingsKey }: {
		inputId: number; darkMode: boolean; spans?: unknown[]; tables?: TableReference[];
		selectionVersion?: number; documentTitle?: string; settingsKey: string;
	} = $props();
	let lines = $state<RawLine[]>([]);
	let page = $state(1);
	let locationsUnavailable = $state(false);
	const selectedLines = $derived(resolveReferenceLines(spans, tables, lines, documentTitle));
	const highlightVersion = $derived(`${selectionVersion}:${JSON.stringify(selectedLines)}:${JSON.stringify(tables)}`);

	$effect(() => {
		const id = inputId;
		let cancelled = false;
		lines = [];
		locationsUnavailable = false;
		getRawLines(id).then(result => {
			if (!cancelled) lines = result.lines ?? [];
		}).catch(error => {
			if (!cancelled) {
				locationsUnavailable = true;
				console.warn('Reference PDF locations unavailable', { inputId: id, error });
			}
		});
		return () => { cancelled = true; };
	});

	$effect(() => {
		selectionVersion;
		page = selectedLines[0]?.page_number ?? 1;
	});

	function renderHighlights(pageNo: number, viewport: PdfPageViewport, overlay: HTMLDivElement) {
		paintReferenceHighlights(selectedLines, tables, pageNo, viewport, overlay);
	}
</script>

<div class="reference-pdf">
	{#if locationsUnavailable}<p class="location-error" role="status">{m.reference_graph_locations_unavailable()}</p>{/if}
	<div class="viewer"><PdfViewWindow {darkMode} {inputId} fileUrl={`/api/v1/kb/inputs/${inputId}/file`} bind:page {highlightVersion} {renderHighlights} tableReferences={tables} showSidebar={false} enableSelectionDialog={false} sidebarSettingsKey={settingsKey} /></div>
</div>

<style>
	.reference-pdf{display:flex;flex-direction:column;height:100%;min-height:0}.viewer{flex:1;min-height:0;overflow:hidden}.location-error{margin:0;padding:8px 14px;color:var(--rg-muted);font-size:11px}
</style>
