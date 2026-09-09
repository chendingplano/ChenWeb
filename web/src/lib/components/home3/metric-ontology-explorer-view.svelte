<script lang="ts">
	// Metric Ontology Explorer — home3/knowledge → Ontology → Metric Ontology Explorer.
	// A graph-first workspace: an orrery canvas, a tabbed content viewer, and a
	// source-document pane, in a resizable three-pane shell.

	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import PanelShell from './metric-ontology-explorer/panel-shell.svelte';
	import OntologyCanvas from './metric-ontology-explorer/ontology-canvas.svelte';
	import ContentViewer from './metric-ontology-explorer/content-viewer.svelte';
	import SourcePane from './metric-ontology-explorer/source-pane.svelte';
	import { CHAIN_NODE_BY_ID, CHAINS } from './metric-ontology-explorer/model';
	import { explorerTokens } from './metric-ontology-explorer/theme';
	import {
		getMetricGraph,
		type MetricGraph,
		type MetricGraphMetric
	} from '$lib/services/metricOntologyExplorerService';

	let { darkMode = true, metricId = '' }: { darkMode?: boolean; metricId?: string } = $props();

	const tokens = $derived(explorerTokens(darkMode));

	let focusId = $state('metric');
	let openChain = $state<string | null>(null);
	let openTabs = $state<string[]>([]);
	let activeTab = $state('entry');

	// One fetch per selected metric; every record tab reads its slice.
	let graph = $state<MetricGraph | null>(null);
	let graphError = $state('');

	$effect(() => {
		const id = metricId.trim();
		graph = null;
		graphError = '';
		if (!id) return;
		let cancelled = false;
		getMetricGraph(id)
			.then((g) => {
				if (!cancelled) graph = g;
			})
			.catch((e) => {
				if (!cancelled) graphError = e instanceof Error ? e.message : String(e);
			});
		return () => {
			cancelled = true;
		};
	});

	const metric = $derived<MetricGraphMetric | null>(graph?.metric ?? null);
	const nodeRows = $derived(graph?.nodes ?? null);
	// Per-satellite badge = row count of the satellite's first chain node.
	const counts = $derived.by<Record<string, number>>(() => {
		const out: Record<string, number> = {};
		if (!nodeRows) return out;
		for (const [sat, chain] of Object.entries(CHAINS)) {
			const first = chain[0]?.id;
			if (first && nodeRows[first]) out[sat] = nodeRows[first].rows.length;
		}
		return out;
	});

	function onfocus(id: string) {
		focusId = id;
		activeTab = 'entry';
	}
	function ontogglechain(satelliteId: string) {
		if (openChain === satelliteId) {
			openChain = null;
		} else {
			openChain = satelliteId;
		}
		openTabs = [];
		activeTab = 'entry';
	}
	function onopenrecord(chainNodeId: string) {
		if (!openTabs.includes(chainNodeId)) openTabs = [...openTabs, chainNodeId];
		activeTab = chainNodeId;
	}
	function onselecttab(id: string) {
		activeTab = id;
	}
	function onclosetab(id: string) {
		openTabs = openTabs.filter((x) => x !== id);
		if (activeTab === id) activeTab = 'entry';
	}
	function onpickmetric(id: string) {
		// Deep-link the picked metric so +page.svelte's `metric_id`-derived prop
		// flows back down and the source pane re-resolves; `section` keeps the
		// explorer selected across a refresh.
		const url = new URL(page.url);
		url.searchParams.set('section', 'kb-metric-ontology-explorer');
		url.searchParams.set('metric_id', id);
		void goto(url, { keepFocus: true, noScroll: true });
	}

	const evidenceSpans = $derived(
		activeTab !== 'entry' ? (CHAIN_NODE_BY_ID[activeTab]?.evidenceSpans ?? []) : []
	);
	const evidenceLabel = $derived(
		activeTab !== 'entry' ? (CHAIN_NODE_BY_ID[activeTab]?.label ?? '') : ''
	);
</script>

<div class="moe-root" style="background:{tokens.pageBg};">
	<PanelShell {tokens}>
		{#snippet canvas()}
			<OntologyCanvas
				{tokens}
				{focusId}
				{openChain}
				{metric}
				{counts}
				{onfocus}
				{ontogglechain}
				{onopenrecord}
			/>
		{/snippet}
		{#snippet content()}
			<ContentViewer
				{tokens}
				{darkMode}
				{focusId}
				{metricId}
				{nodeRows}
				{graphError}
				{openTabs}
				{activeTab}
				{onselecttab}
				{onclosetab}
				{onpickmetric}
			/>
		{/snippet}
		{#snippet source()}
			<SourcePane {tokens} {metricId} {evidenceSpans} {evidenceLabel} />
		{/snippet}
	</PanelShell>
</div>

<style>
	.moe-root {
		height: 100%;
		min-height: 0;
		display: flex;
		flex-direction: column;
	}
</style>
