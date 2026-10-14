<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { theme } from '$lib/stores/theme.svelte';
	import { Chart } from 'svelte-echarts';
	import type { EChartsOption } from 'echarts';
	import { init, use } from 'echarts/core';
	import { GraphChart, TreeChart } from 'echarts/charts';
	import { TooltipComponent } from 'echarts/components';
	import { CanvasRenderer } from 'echarts/renderers';
	import KbInputRecordBrowser from '$lib/components/home3/kb-input-record-browser.svelte';
	import { buildDocumentHierarchy, flattenHierarchy, buildNavigator, type ViewMode } from './graph-data';
	import {
		listKbMetrics,
		listKbReferences,
		type KbInputRecord,
		type KbMetricRecord,
		type KbReferenceMention
	} from '$lib/services/kbService';

	use([GraphChart, TreeChart, TooltipComponent, CanvasRenderer]);


	const LEFT_WIDTH_KEY = 'reference-graph:left-width';
	const MIDDLE_WIDTH_KEY = 'reference-graph:middle-width';
	const LEFT_MIN = 260;
	const LEFT_MAX = 760;
	const MIDDLE_MIN = 300;
	const MIDDLE_MAX = 820;
	const GRAPH_MIN = 320;
	const DIVIDER_WIDTH = 8;

	let darkMode = $derived(theme.isDark);
	let record = $state<KbInputRecord | null>(null);
	let metrics = $state<KbMetricRecord[]>([]);
	let references = $state<KbReferenceMention[]>([]);
	let selectedReferenceId = $state<number | null>(null);
	let tab = $state<'metrics' | 'references'>('metrics');
	let viewMode = $state<ViewMode>('graph');
	let loading = $state(false);
	let error = $state('');
	let leftWidth = $state(400);
	let middleWidth = $state(500);
	let resizing = $state<'left' | 'middle' | null>(null);
	let chartApi = $state<ReturnType<typeof init> | undefined>(undefined);
	let graphPositions = $state<Record<string, [number, number]>>({});
	let navigator = $state({ x: 0, y: 0, zoom: 1 });
	let navigatorPointer: { x: number; y: number } | null = null;
	let requestId = 0;
	let chartWrap = $state<HTMLDivElement | undefined>(undefined);

	const palette = $derived(darkMode
		? { background: '#161a22', text: '#ede7d3', muted: '#9b947e', border: '#2a3140', node: '#303a4b', root: '#d4a24c', edge: '#9b947e', thumb: '#2a3140' }
		: { background: '#fbf8f0', text: '#1a1711', muted: '#726b5e', border: '#d8cfbb', node: '#f0eadb', root: '#a87518', edge: '#817767', thumb: '#d7cfb8' });
	const gridStyle = $derived(`--left-panel-width:${leftWidth}px;--middle-panel-width:${middleWidth}px`);

	function recordTitle(value: KbInputRecord) {
		return value.title || value.name || value.file_name || m.reference_graph_document({ id: value.id });
	}

	function referenceTitle(reference: KbReferenceMention) {
		return reference.target_input_title || reference.target_title || reference.resource_title || reference.cited_raw || reference.ref_id;
	}

	async function selectRecord(value: KbInputRecord) {
		const request = ++requestId;
		navigator = { x: 0, y: 0, zoom: 1 };
		record = value;
		metrics = [];
		references = [];
		selectedReferenceId = null;
		navigator = { x: 0, y: 0, zoom: 1 };
		error = '';
		loading = true;
		try {
			const [metricResult, referenceResult] = await Promise.all([
				listKbMetrics(value.id),
				listKbReferences(value.id)
			]);
			if (request !== requestId) return;
			metrics = metricResult.results ?? [];
			references = referenceResult.results ?? [];
		} catch (cause) {
			if (request === requestId) error = cause instanceof Error ? cause.message : m.reference_graph_load_failed();
		} finally {
			if (request === requestId) loading = false;
		}
	}

	const hierarchy = $derived(record ? buildDocumentHierarchy(
		{ id: record.id, name: recordTitle(record) },
		metrics.map((metric) => ({ id: metric.id, name: metric.metric_name || metric.metric_name_en || m.reference_graph_untitled_metric(), value: metric.metric_value })),
		references.map((reference) => ({ id: reference.id, name: referenceTitle(reference), value: reference.ref_id })),
		{ metrics: m.home3_knowledge_metrics(), references: m.reference_graph_references() }
	) : null);
	const graphData = $derived(hierarchy ? flattenHierarchy(hierarchy) : { nodes: [], links: [] });

	const graphOption = $derived.by<EChartsOption>(() => {
		if (!hierarchy) return {};
		return {
			backgroundColor: 'transparent',
			animationDuration: 0,
			animationDurationUpdate: 0,
			tooltip: {
				trigger: 'item',
				backgroundColor: palette.background,
				borderColor: palette.border,
				textStyle: { color: palette.text },
				formatter: (item: any) => item.dataType === 'edge'
					? `${item.data.name || ''}`
					: `${item.data.name || ''}`
			},
			series: [{
				id: 'reference-graph-series',
				type: 'graph',
				layout: 'force',
				roam: true,
				draggable: false,
				data: graphData.nodes.map((node) => ({
					...node,
					symbolSize: node.depth === 0 ? 48 : node.depth === 1 ? 38 : 24,
					itemStyle: { color: node.depth < 2 ? palette.root : palette.node },
					label: { color: palette.text }
				})),
				links: graphData.links.map((link) => ({
					...link,
					lineStyle: { color: palette.edge, opacity: 0.72 },
				})),
				force: { repulsion: 800, gravity: 0.1, edgeLength: 100, layoutAnimation: false },
				label: { show: true, position: 'right', formatter: '{b}', fontSize: 11, color: palette.text, width: 180, overflow: 'truncate' },
				edgeLabel: { show: false },
				lineStyle: { color: palette.edge, curveness: 0.12 },
				emphasis: { focus: 'adjacency', lineStyle: { width: 3 } },
				scaleLimit: { min: 0.2, max: 5 }
			}]
		};
	});

	const treeOption = $derived.by<EChartsOption>(() => {
		if (!hierarchy) return {};
		const orient = viewMode === 'left-right' ? 'LR'
			: viewMode === 'right-left' ? 'RL'
			: viewMode === 'top-bottom' ? 'TB' : 'BT';
		const rootLabelPosition = orient === 'LR' ? 'left' : orient === 'RL' ? 'right' : orient === 'TB' ? 'top' : 'bottom';
		const leafLabelPosition = orient === 'LR' ? 'right' : orient === 'RL' ? 'left' : orient === 'TB' ? 'bottom' : 'top';
		const horizontalTree = orient === 'LR' || orient === 'RL';
		return {
			backgroundColor: 'transparent',
			animationDuration: 0,
			animationDurationUpdate: 0,
			tooltip: {
				trigger: 'item',
				backgroundColor: palette.background,
				borderColor: palette.border,
				textStyle: { color: palette.text },
				formatter: (item: any) => item.data.value ? `${item.data.name} · ${item.data.value}` : `${item.data.name || ''}`
			},
			series: [{
				id: 'reference-graph-series',
				type: 'tree',
				data: [{
					...hierarchy,
					itemStyle: { color: palette.root },
					children: hierarchy.children.map((group) => ({
						...group,
						itemStyle: { color: palette.root }
					}))
				}],
				layout: 'orthogonal',
				orient,
				roam: true,
				symbol: 'circle',
				symbolSize: 9,
				itemStyle: { color: palette.node },
				expandAndCollapse: false,
				initialTreeDepth: -1,
				lineStyle: { color: palette.edge, width: 1.2, curveness: 0.25 },
				// Match the ECharts vertical tree: parent labels extend above, leaves below.
				label: { position: rootLabelPosition, rotate: horizontalTree ? 0 : -90, verticalAlign: 'middle', align: rootLabelPosition === 'left' || rootLabelPosition === 'top' ? 'right' : 'left', color: palette.text, width: 190, overflow: 'truncate', fontSize: 11 },
				leaves: { label: { position: leafLabelPosition, rotate: horizontalTree ? 0 : -90, verticalAlign: 'middle', align: leafLabelPosition === 'left' || leafLabelPosition === 'top' ? 'right' : 'left', color: palette.text, width: 220, overflow: 'truncate', fontSize: 10 } },
				left: horizontalTree ? '40%' : '6%',
				right: horizontalTree ? '40%' : '6%',
				top: horizontalTree ? 20 : '40%',
				bottom: horizontalTree ? 20 : '40%',
				emphasis: { focus: 'descendant' }
			}]
		};
	});

	const chartOption = $derived(viewMode === 'graph' ? graphOption : treeOption);
	const navigatorNodes = $derived(hierarchy ? buildNavigator(hierarchy, viewMode, graphPositions) : []);
	const navigatorEdges = $derived(navigatorNodes.filter((node) => node.parentId).map((node) => ({
		id: node.id,
		source: navigatorNodes.find((parent) => parent.id === node.parentId)!,
		target: node
	})));

	const navigatorViewport = $derived.by(() => {
		const width = Math.max(32, 116 / navigator.zoom);
		const height = Math.max(24, 72 / navigator.zoom);
		return {
			x: Math.max(0, Math.min(200 - width, 100 + navigator.x - width / 2)),
			y: Math.max(0, Math.min(130 - height, 65 + navigator.y - height / 2)),
			width,
			height
		};
	});

	function dispatchNavigatorPan(dx: number, dy: number) {
		if (!chartApi || !chartWrap) return;
		const action = viewMode === 'graph' ? 'graphRoam' : 'treeRoam';
		chartApi.dispatchAction({ type: action, seriesId: 'reference-graph-series', dx, dy } as any);
		// Roam actions update the model only; the built-in mouse controller
		// normally moves the rendered view before dispatching them.
		const series = (chartApi.getOption().series as { id: string; center: [number, number]; zoom: number }[])
			.find((series) => series.id === 'reference-graph-series');
		if (series) chartApi.setOption({ series: [{ id: series.id, center: series.center, zoom: series.zoom }] });
	}

	function startNavigatorMove(event: PointerEvent) {
		if (!chartApi || !chartWrap) return;
		event.preventDefault();
		const map = (event.currentTarget as HTMLElement).getBoundingClientRect();
		const target = event.target;
		const movingViewport = target instanceof Element && target.closest('.mini-viewport') !== null;
		navigatorPointer = { x: event.clientX, y: event.clientY };
		if (!movingViewport) {
			const currentCenter = { x: map.left + map.width * (navigatorViewport.x + navigatorViewport.width / 2) / 200, y: map.top + map.height * (navigatorViewport.y + navigatorViewport.height / 2) / 130 };
			const dx = (currentCenter.x - event.clientX) / map.width * chartWrap.clientWidth;
			const dy = (currentCenter.y - event.clientY) / map.height * chartWrap.clientHeight;
			dispatchNavigatorPan(dx, dy);
		}
		const move = (next: PointerEvent) => {
			if (!navigatorPointer) return;
			const deltaX = next.clientX - navigatorPointer.x;
			const deltaY = next.clientY - navigatorPointer.y;
			navigatorPointer = { x: next.clientX, y: next.clientY };
			dispatchNavigatorPan(-deltaX / map.width * chartWrap!.clientWidth, -deltaY / map.height * chartWrap!.clientHeight);
		};
		const end = () => {
			navigatorPointer = null;
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', end);
			window.removeEventListener('pointercancel', end);
		};
		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', end, { once: true });
		window.addEventListener('pointercancel', end, { once: true });
	}

	function handleChartRoam(event: any) {
		if (!chartApi || !chartWrap) return;
		const width = Math.max(chartWrap.clientWidth, 1);
		const height = Math.max(chartWrap.clientHeight, 1);
		navigator = {
			x: Math.max(-70, Math.min(70, navigator.x - (event.dx || 0) / width * 200)),
			y: Math.max(-45, Math.min(45, navigator.y - (event.dy || 0) / height * 130)),
			zoom: Math.max(0.4, Math.min(5, navigator.zoom * (event.zoom || 1)))
		};
	}

	function changeViewMode(value: string) {
		viewMode = value as ViewMode;
		navigator = { x: 0, y: 0, zoom: 1 };
	}

	function configureTreeRoam() {
		if (!chartApi) return;
		const series = (chartApi as any).getModel().getSeriesByIndex(0);
		if (series?.subType !== 'tree') return;
		const view = (chartApi as any).getViewOfSeriesModel(series);
		// ECharts restricts tree roaming to the drawing bounds, which are narrow
		// for horizontal trees. Accept gestures anywhere inside this chart.
		view._controller.setPointerChecker((_event: unknown, x: number, y: number) =>
			x >= 0 && y >= 0 && x <= chartApi!.getWidth() && y <= chartApi!.getHeight());
	}

	function syncGraphPositions() {
		if (!chartApi || viewMode !== 'graph') return;
		// Read the force layout after rendering, including on chart recreation.
		const series = (chartApi as any).getModel().getSeriesByIndex(0);
		if (series?.subType !== 'graph') return;
		const data = series.getData();
		const positions: Record<string, [number, number]> = {};
		for (let index = 0; index < data.count(); index++) {
			const point = data.getItemLayout(index);
			if (point?.every(Number.isFinite)) positions[data.getId(index)] = [point[0], point[1]];
		}
		graphPositions = positions;
	}

	$effect(() => {
		const api = chartApi;
		if (!api) return;
		api.on('graphRoam', handleChartRoam);
		api.on('treeRoam', handleChartRoam);
		api.on('rendered', syncGraphPositions);
		api.on('rendered', configureTreeRoam);
		configureTreeRoam();
		syncGraphPositions();
		return () => {
			api.off('graphRoam', handleChartRoam);
			api.off('treeRoam', handleChartRoam);
			api.off('rendered', syncGraphPositions);
			api.off('rendered', configureTreeRoam);
		};
	});

	function clampWidth(value: number, min: number, max: number) {
		return Math.max(min, Math.min(max, Math.round(value)));
	}

	function startResize(panel: 'left' | 'middle', event: PointerEvent) {
		event.preventDefault();
		const startX = event.clientX;
		const initialLeft = leftWidth;
		const initialMiddle = middleWidth;
		resizing = panel;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		const move = (next: PointerEvent) => {
			const available = window.innerWidth - GRAPH_MIN - DIVIDER_WIDTH * 2;
			if (panel === 'left') {
				const maxLeft = Math.min(LEFT_MAX, available - middleWidth - MIDDLE_MIN);
				leftWidth = clampWidth(initialLeft + next.clientX - startX, LEFT_MIN, maxLeft);
			} else {
				const maxMiddle = Math.min(MIDDLE_MAX, available - leftWidth - LEFT_MIN);
				middleWidth = clampWidth(initialMiddle + next.clientX - startX, MIDDLE_MIN, maxMiddle);
			}
		};
		const end = () => {
			resizing = null;
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			localStorage.setItem(LEFT_WIDTH_KEY, String(leftWidth));
			localStorage.setItem(MIDDLE_WIDTH_KEY, String(middleWidth));
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', end);
			window.removeEventListener('pointercancel', end);
		};
		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', end, { once: true });
		window.addEventListener('pointercancel', end, { once: true });
	}

	function resizeWithKeyboard(event: KeyboardEvent, panel: 'left' | 'middle') {
		if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
		event.preventDefault();
		const delta = event.key === 'ArrowRight' ? 16 : -16;
		if (panel === 'left') leftWidth = clampWidth(leftWidth + delta, LEFT_MIN, Math.min(LEFT_MAX, window.innerWidth - middleWidth - GRAPH_MIN - DIVIDER_WIDTH * 2));
		else middleWidth = clampWidth(middleWidth + delta, MIDDLE_MIN, Math.min(MIDDLE_MAX, window.innerWidth - leftWidth - GRAPH_MIN - DIVIDER_WIDTH * 2));
		localStorage.setItem(LEFT_WIDTH_KEY, String(leftWidth));
		localStorage.setItem(MIDDLE_WIDTH_KEY, String(middleWidth));
	}

	onMount(() => {
		const savedLeft = Number(localStorage.getItem(LEFT_WIDTH_KEY));
		const savedMiddle = Number(localStorage.getItem(MIDDLE_WIDTH_KEY));
		if (Number.isFinite(savedLeft) && savedLeft >= LEFT_MIN && savedLeft <= LEFT_MAX) leftWidth = savedLeft;
		if (Number.isFinite(savedMiddle) && savedMiddle >= MIDDLE_MIN && savedMiddle <= MIDDLE_MAX) middleWidth = savedMiddle;
	});
</script>

<svelte:head><title>{m.reference_graph_title()}</title></svelte:head>

<main class="reference-graph-page select-text" style={`--rg-bg:${palette.background};--rg-panel:${palette.background};--rg-line:${palette.border};--rg-text:${palette.text};--rg-muted:${palette.muted};--rg-thumb:${palette.thumb};${gridStyle}`}>
	<aside class="records"><KbInputRecordBrowser {darkMode} fitPanel instanceKey="reference-graph-records" title={m.references_mgmt_kb_inputs()} subtitle={m.references_mgmt_search_and_select()} emptyTitle={m.references_mgmt_no_records()} emptySubtitle={m.references_mgmt_use_search_to_browse()} autoSelectFirstRecord={false} selectedRecordId={record?.id ?? null} onSelect={selectRecord} onError={(cause) => (error = cause.message)} /></aside>
	<button class="resizer" class:active={resizing === 'left'} type="button" aria-label={m.reference_graph_resize_left()} onpointerdown={(event) => startResize('left', event)} onkeydown={(event) => resizeWithKeyboard(event, 'left')}><span></span></button>
	<section class="middle">
		<nav class="tabs" aria-label={m.reference_graph_lists()}>
			<button class:active={tab === 'metrics'} aria-pressed={tab === 'metrics'} onclick={() => (tab = 'metrics')}>{m.home3_knowledge_metrics()} <span>{metrics.length}</span></button>
			<button class:active={tab === 'references'} aria-pressed={tab === 'references'} onclick={() => (tab = 'references')}>{m.reference_graph_references()} <span>{references.length}</span></button>
		</nav>
		{#if loading}<p class="empty">{m.reference_graph_loading()}</p>
		{:else if error}<p class="empty error">{error}</p>
		{:else if !record}<p class="empty">{m.reference_graph_select_document()}</p>
		{:else if tab === 'metrics'}
			<div class="entry-list">
				{#each metrics as metric (metric.id)}
					<article class="entry"><strong>{metric.metric_name || metric.metric_name_en || m.reference_graph_untitled_metric()}</strong>{#if metric.metric_desc || metric.metric_context || metric.metric_subject}<p>{metric.metric_desc || metric.metric_context || metric.metric_subject}</p>{/if}<small>{metric.metric_id || ''}{metric.metric_value ? ` · ${metric.metric_value}` : ''}</small></article>
				{:else}<p class="empty">{m.reference_graph_no_metrics()}</p>{/each}
			</div>
		{:else}
			<div class="entry-list">
				{#each references as reference (reference.id)}
					<button class="entry reference-entry" class:chosen={selectedReferenceId === reference.id} aria-pressed={selectedReferenceId === reference.id} onclick={() => (selectedReferenceId = reference.id)}>
						<strong>{referenceTitle(reference)}</strong>{#if reference.evidence || reference.cited_raw}<p>{reference.evidence || reference.cited_raw}</p>{/if}
						<small>{reference.ref_id} · {reference.target_type}{reference.target_input_id ? ` · ${m.reference_graph_document({ id: reference.target_input_id })}` : ''}</small>
					</button>
				{:else}<p class="empty">{m.reference_graph_no_references()}</p>{/each}
			</div>
		{/if}
	</section>
	<button class="resizer" class:active={resizing === 'middle'} type="button" aria-label={m.reference_graph_resize_middle()} onpointerdown={(event) => startResize('middle', event)} onkeydown={(event) => resizeWithKeyboard(event, 'middle')}><span></span></button>
	<section class="graph-panel" aria-label={m.reference_graph_title()}>
		<header><div><span class="eyebrow">{m.reference_graph_eyebrow()}</span><h1>{m.reference_graph_title()}</h1></div><div class="graph-controls"><span class="count">{graphData.links.length} {m.reference_graph_edges()}</span><label class="view-mode-label" for="reference-graph-view-mode">{m.reference_graph_view_mode()}</label><select id="reference-graph-view-mode" value={viewMode} onchange={(event) => changeViewMode(event.currentTarget.value)}><option value="graph">{m.reference_graph_graph_mode()}</option><option value="left-right">{m.reference_graph_left_right_tree()}</option><option value="top-bottom">{m.reference_graph_top_bottom_tree()}</option><option value="bottom-top">{m.reference_graph_bottom_top_tree()}</option><option value="right-left">{m.reference_graph_right_left_tree()}</option></select></div></header>
		{#if loading}<div class="graph-empty">{m.reference_graph_loading()}</div>
		{:else if error}<div class="graph-empty error">{error}</div>
		{:else if !record}<div class="graph-empty">{m.reference_graph_select_document()}</div>
		{:else}<div class="chart-wrap" bind:this={chartWrap}>{#key `${record?.id}:${viewMode}`}<Chart bind:chart={chartApi} {init} options={chartOption} style="width:100%;height:100%;" />{/key}
			<button class="mini-map" type="button" aria-label={m.reference_graph_minimap()} onpointerdown={startNavigatorMove} onkeydown={(event) => { const step = event.shiftKey ? 80 : 30; if (event.key === 'ArrowLeft') dispatchNavigatorPan(step, 0); else if (event.key === 'ArrowRight') dispatchNavigatorPan(-step, 0); else if (event.key === 'ArrowUp') dispatchNavigatorPan(0, step); else if (event.key === 'ArrowDown') dispatchNavigatorPan(0, -step); }}>
				<svg viewBox="0 0 200 130" aria-hidden="true"><rect class="mini-backdrop" width="200" height="130" rx="6" />
					{#each navigatorEdges as edge (edge.id)}<line class="mini-edge" x1={edge.source.x} y1={edge.source.y} x2={edge.target.x} y2={edge.target.y} />{/each}
					{#each navigatorNodes as node (node.id)}<circle class:mini-root={node.root} class="mini-node" cx={node.x} cy={node.y} r={node.root ? 4 : 2.5} />{/each}
					<rect class="mini-viewport" x={navigatorViewport.x} y={navigatorViewport.y} width={navigatorViewport.width} height={navigatorViewport.height} rx="3" />
				</svg>
			</button>
		</div>{/if}
	</section>
</main>

<style>
	.reference-graph-page{display:grid;grid-template-columns:var(--left-panel-width) 8px var(--middle-panel-width) 8px minmax(320px,1fr);height:100dvh;min-height:0;overflow:hidden;background:var(--rg-bg);color:var(--rg-text);font:14px/1.45 system-ui,sans-serif;user-select:text}.records,.middle,.graph-panel{min-width:0;min-height:0;background:var(--rg-panel)}.records{overflow:hidden;border-right:1px solid var(--rg-line)}.middle{display:flex;flex-direction:column;overflow:hidden;border-right:1px solid var(--rg-line)}.tabs{display:flex;gap:4px;padding:8px;border-bottom:1px solid var(--rg-line)}.tabs button{flex:1;padding:10px 8px;border:0;border-radius:6px;background:transparent;color:var(--rg-muted);font:inherit;cursor:pointer}.tabs button.active{background:#303a4b;color:var(--rg-text)}.tabs span{margin-left:5px;font-size:11px;color:#d4a24c}.entry-list{overflow:auto;padding:8px;display:flex;flex-direction:column;gap:6px}.entry{display:block;width:100%;padding:11px;text-align:left;border:1px solid transparent;border-radius:6px;background:#252d3a;color:var(--rg-text);font:inherit}.reference-entry{cursor:pointer}.reference-entry:hover,.reference-entry.chosen{border-color:#b08d57}.entry strong{display:block;font-size:13px}.entry p{margin:5px 0;color:var(--rg-muted);font-size:11px}.entry small{color:#b08d57;font-size:10px}.empty{padding:18px;color:var(--rg-muted);font-size:13px}.empty.error,.graph-empty.error{color:#e98d78}.graph-panel{display:flex;flex-direction:column;overflow:hidden}.graph-panel header{display:flex;align-items:center;justify-content:space-between;padding:18px 20px;border-bottom:1px solid var(--rg-line)}.eyebrow{color:#b08d57;font-size:9px;letter-spacing:.15em;text-transform:uppercase}.graph-panel h1{margin:4px 0 0;font-family:Georgia,serif;font-size:22px;font-weight:500}.count{color:var(--rg-muted);font-size:10px}.graph-empty{display:grid;flex:1;place-items:center;color:var(--rg-muted);font-size:12px}.chart-wrap{flex:1;min-height:0;overflow:hidden}.resizer{position:relative;width:8px;height:100%;padding:0;border:0;background:transparent;cursor:col-resize;touch-action:none}.resizer span{position:absolute;top:0;bottom:0;left:3px;width:2px;background:var(--rg-line)}.resizer:hover span,.resizer.active span,.resizer:focus-visible span{left:2px;width:4px;background:#b08d57}.resizer:focus-visible{outline:2px solid #b08d57;outline-offset:-2px}.resizer{user-select:none}.entry-list{scrollbar-width:thin;scrollbar-color:var(--rg-thumb) transparent}.entry-list::-webkit-scrollbar{width:8px;height:8px}.entry-list::-webkit-scrollbar-thumb{background:var(--rg-thumb)}.entry-list::-webkit-scrollbar-track{background:transparent}
	.graph-controls{display:flex;align-items:center;gap:8px}.view-mode-label{color:var(--rg-muted);font-size:10px}.graph-controls select{max-width:170px;padding:6px 26px 6px 8px;border:1px solid var(--rg-line);border-radius:5px;background:var(--rg-panel);color:var(--rg-text);font:inherit;font-size:11px}.chart-wrap{position:relative}.mini-map{position:absolute;right:18px;bottom:18px;width:200px;height:130px;padding:0;overflow:hidden;border:1px solid var(--rg-line);border-radius:7px;background:transparent;color:inherit;font:inherit;box-shadow:0 6px 24px #0008;cursor:grab;touch-action:none;user-select:none}.mini-map:active{cursor:grabbing}.mini-map:focus-visible{outline:2px solid #b08d57}.mini-map svg{display:block;width:100%;height:100%;pointer-events:auto}.mini-backdrop,.mini-edge,.mini-node{pointer-events:none}.mini-backdrop{fill:#11151cdd}.mini-edge{stroke:#8b806c;stroke-opacity:.45;stroke-width:.7}.mini-node{fill:#687287}.mini-root{fill:#d4a24c}.mini-viewport{fill:#d4a24c1a;stroke:#d4a24c;stroke-width:1.5;pointer-events:all;cursor:move}
	@media(max-width:900px){.reference-graph-page{grid-template-columns:minmax(180px,min(var(--left-panel-width),32vw)) 8px minmax(220px,min(var(--middle-panel-width),38vw)) 8px minmax(180px,1fr)}.graph-panel header{padding:14px}.count{display:none}}
</style>
