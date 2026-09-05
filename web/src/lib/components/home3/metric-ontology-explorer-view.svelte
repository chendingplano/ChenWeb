<script lang="ts">
	// Metric Ontology Explorer — home3/knowledge → Ontology → Metric Ontology Explorer.
	// A graph-first workspace: an orrery canvas, a tabbed content viewer, and a
	// source-document pane, in a resizable three-pane shell.

	import PanelShell from './metric-ontology-explorer/panel-shell.svelte';
	import OntologyCanvas from './metric-ontology-explorer/ontology-canvas.svelte';
	import ContentViewer from './metric-ontology-explorer/content-viewer.svelte';
	import SourcePane from './metric-ontology-explorer/source-pane.svelte';
	import { CHAIN_NODE_BY_ID } from './metric-ontology-explorer/model';
	import { explorerTokens } from './metric-ontology-explorer/theme';

	let { darkMode = true, metricId = '' }: { darkMode?: boolean; metricId?: string } = $props();

	const tokens = $derived(explorerTokens(darkMode));

	let focusId = $state('metric');
	let openChain = $state<string | null>(null);
	let openTabs = $state<string[]>([]);
	let activeTab = $state('entry');

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
			<OntologyCanvas {tokens} {focusId} {openChain} {onfocus} {ontogglechain} {onopenrecord} />
		{/snippet}
		{#snippet content()}
			<ContentViewer {tokens} {focusId} {openTabs} {activeTab} {onselecttab} {onclosetab} />
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
