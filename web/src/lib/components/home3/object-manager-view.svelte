<script lang="ts">
	import { browser } from '$app/environment';
	import { onMount } from 'svelte';
	import { Chart } from 'svelte-echarts';
	import { init, use } from 'echarts/core';
	import type { EChartsType } from 'echarts/core';
	import type { EChartsOption } from 'echarts';
	import { GraphChart, PieChart, BarChart } from 'echarts/charts';
	import {
		TooltipComponent,
		LegendComponent,
		GridComponent,
		TitleComponent
	} from 'echarts/components';
	import { CanvasRenderer } from 'echarts/renderers';
	import {
		searchObjects,
		buildObjectGraph,
		getArtifactObjectStats,
		getObjectNodeStats,
		getObjectConnectivity,
		getObjectPdfLocator,
		mergeObjectNodes,
		type ObjectSearchTable,
		type ObjectNodeSummary,
		type ArtifactObjectSummary,
		type ObjectGraph,
		type ArtifactObjectStats,
		type ObjectNodeStats,
		type ConnectivityRow,
		type PdfLocator
	} from '$lib/services/objectManagerService';

	use([
		GraphChart,
		PieChart,
		BarChart,
		TooltipComponent,
		LegendComponent,
		GridComponent,
		TitleComponent,
		CanvasRenderer
	]);

	let { darkMode = true }: { darkMode?: boolean } = $props();

	const panelBg = $derived(darkMode ? '#161A22' : '#FBF8F0');
	const cardBg = $derived(darkMode ? '#1C212C' : '#F0EADB');
	const borderCol = $derived(darkMode ? '#2A303C' : '#E0D8C4');
	const textCol = $derived(darkMode ? '#E5E7EB' : '#1F2937');
	const mutedCol = $derived(darkMode ? '#9CA3AF' : '#6B7280');
	const scrollThumb = $derived(darkMode ? '#3A4150' : '#CBB994');

	// ---------- Panel widths (draggable splitters) ----------
	const LEFT_W_KEY = 'object-manager:left-width';
	const RIGHT_W_KEY = 'object-manager:right-width';
	const LEFT_MIN = 200;
	const LEFT_MAX = 800;
	const RIGHT_MIN = 240;
	const RIGHT_MAX = 1000;
	// Smallest the Middle panel may shrink to (halved from the previous ~280px
	// effective minimum) so users can grow Left/Right further into it.
	const MIDDLE_MIN = 140;
	let leftWidth = $state(280);
	let rightWidth = $state(360);
	let containerWidth = $state(0);

	$effect(() => {
		if (!browser) return;
		const l = Number(localStorage.getItem(LEFT_W_KEY));
		if (Number.isFinite(l) && l >= LEFT_MIN && l <= LEFT_MAX) leftWidth = l;
		const r = Number(localStorage.getItem(RIGHT_W_KEY));
		if (Number.isFinite(r) && r >= RIGHT_MIN && r <= RIGHT_MAX) rightWidth = r;
	});

	function startPanelResize(side: 'left' | 'right', event: PointerEvent) {
		event.preventDefault();
		const startX = event.clientX;
		const startLeft = leftWidth;
		const startRight = rightWidth;
		document.body.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		const move = (e: PointerEvent) => {
			const dx = e.clientX - startX;
			// Two 6px resizers sit inside the container width.
			const resizers = 12;
			if (side === 'left') {
				let maxLeft = LEFT_MAX;
				if (containerWidth > 0)
					maxLeft = Math.min(maxLeft, containerWidth - rightWidth - MIDDLE_MIN - resizers);
				leftWidth = Math.max(LEFT_MIN, Math.min(maxLeft, Math.round(startLeft + dx)));
			} else {
				let maxRight = RIGHT_MAX;
				if (containerWidth > 0)
					maxRight = Math.min(maxRight, containerWidth - leftWidth - MIDDLE_MIN - resizers);
				rightWidth = Math.max(RIGHT_MIN, Math.min(maxRight, Math.round(startRight - dx)));
			}
		};
		const up = () => {
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
			if (browser) {
				localStorage.setItem(LEFT_W_KEY, String(leftWidth));
				localStorage.setItem(RIGHT_W_KEY, String(rightWidth));
			}
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', up);
			window.removeEventListener('pointercancel', up);
		};
		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', up, { once: true });
		window.addEventListener('pointercancel', up, { once: true });
	}

	// ---------- Left panel: search ----------
	let table = $state<ObjectSearchTable>('object_nodes');
	let searchQuery = $state('');
	let recordIdInput = $state('');
	let rows = $state<(ObjectNodeSummary | ArtifactObjectSummary)[]>([]);
	let searchError = $state('');
	let searching = $state(false);
	let searchDialogOpen = $state(false);

	// Dialog search results + multi-select checkboxes.
	let searchResults = $state<(ObjectNodeSummary | ArtifactObjectSummary)[]>([]);
	let checkedIds = $state<Set<number>>(new Set());

	// Left-panel paging (browse mode) and curated selection mode.
	const PAGE_SIZE = 50;
	let page = $state(1);
	let hasMore = $state(false);
	let loadingList = $state(false);
	let selectionMode = $state(false);

	function isNodeRow(r: ObjectNodeSummary | ArtifactObjectSummary): r is ObjectNodeSummary {
		return (r as ObjectNodeSummary).canonical_name !== undefined;
	}

	// loadPage lists the current table page-by-page (browse mode). On open the
	// page defaults to all rows of kb.object_nodes.
	async function loadPage(p: number) {
		loadingList = true;
		searchError = '';
		try {
			const resp = await searchObjects({ table, page: p, page_size: PAGE_SIZE });
			rows = resp.rows;
			page = p;
			hasMore = resp.rows.length === PAGE_SIZE;
			selectionMode = false;
		} catch (err) {
			searchError = err instanceof Error ? err.message : String(err);
			rows = [];
			hasMore = false;
		} finally {
			loadingList = false;
		}
	}

	function reloadList() {
		void loadPage(1);
	}

	function openSearchDialog() {
		searchResults = [];
		checkedIds = new Set();
		searchDialogOpen = true;
	}

	// runSearch populates the dialog's result list (Search & Select).
	async function runSearch() {
		searching = true;
		searchError = '';
		try {
			const recordId = recordIdInput.trim() ? Number(recordIdInput.trim()) : undefined;
			const resp = await searchObjects({
				table,
				query: searchQuery.trim() || undefined,
				record_id: recordId && recordId > 0 ? recordId : undefined,
				page_size: 200
			});
			searchResults = resp.rows;
			checkedIds = new Set();
		} catch (err) {
			searchError = err instanceof Error ? err.message : String(err);
			searchResults = [];
		} finally {
			searching = false;
		}
	}

	function toggleChecked(id: number) {
		const next = new Set(checkedIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		checkedIds = next;
	}

	// applySelection commits the checked search results as the left-panel list.
	function applySelection() {
		const chosen = searchResults.filter((r) => checkedIds.has(r.id));
		if (chosen.length === 0) return;
		rows = chosen;
		selectionMode = true;
		hasMore = false;
		searchDialogOpen = false;
		void selectRow(chosen[0]);
	}

	// ---------- Selection ----------
	let selectedObjectId = $state('');
	let selectedNode = $state<ObjectNodeSummary | null>(null);
	let selectedArtifact = $state<ArtifactObjectSummary | null>(null);
	let graph = $state<ObjectGraph | null>(null);
	let graphError = $state('');
	let graphLoading = $state(false);

	// Derive a representative Artifact Object Info from the loaded graph — the
	// first artifact_object (mention) node belonging to the selected object.
	function firstArtifactObjectFromGraph(): ArtifactObjectSummary | null {
		const g = graph;
		if (!g) return null;
		const n = g.nodes.find((x) => x.type === 'artifact_object');
		if (!n) return null;
		return {
			id: n.key.startsWith('ao:') ? Number(n.key.slice(3)) : 0,
			artifact_type: n.artifact_type ?? '',
			artifact_id: n.artifact_id ?? '',
			object_name: n.label,
			object_name_en: '',
			object_id: n.object_id ?? '',
			reconcile_status: n.reconcile_status ?? ''
		};
	}

	async function selectRow(r: ObjectNodeSummary | ArtifactObjectSummary) {
		if (isNodeRow(r)) {
			selectedNode = r;
			selectedObjectId = r.object_id;
			const gp = loadGraph({ object_id: r.object_id });
			void loadLocator({ objectId: r.object_id });
			await gp;
			// Populate Artifact Object Info from a representative mention.
			selectedArtifact = firstArtifactObjectFromGraph();
		} else {
			selectedArtifact = r;
			selectedNode = null;
			selectedObjectId = r.object_id;
			await Promise.all([
				loadGraph({ artifact_object_id: r.id }),
				loadLocator({ artifactObjectId: r.id })
			]);
		}
	}

	async function loadGraph(req: { object_id?: string; artifact_object_id?: number }) {
		graphLoading = true;
		graphError = '';
		try {
			const resp = await buildObjectGraph(req);
			graph = resp.graph;
		} catch (err) {
			graphError = err instanceof Error ? err.message : String(err);
			graph = null;
		} finally {
			graphLoading = false;
		}
	}

	// ---------- Relation chart ----------
	let chartApi = $state<EChartsType | undefined>(undefined);

	const nodeCategories = [
		{ name: 'Object', color: '#5eead4' },
		{ name: 'Mention', color: '#fbbf24' },
		{ name: 'Artifact', color: '#93c5fd' }
	];
	function categoryIndex(type: string): number {
		if (type === 'object_node') return 0;
		if (type === 'artifact_object') return 1;
		return 2;
	}

	const graphOption = $derived.by<EChartsOption>(() => {
		const g = graph;
		if (!g) return {};
		return {
			tooltip: { trigger: 'item' },
			legend: [{ data: nodeCategories.map((c) => c.name), textStyle: { color: textCol } }],
			series: [
				{
					type: 'graph',
					layout: 'force',
					roam: true,
					draggable: true,
					label: { show: true, position: 'right', color: textCol, formatter: '{b}' },
					force: { repulsion: 160, edgeLength: 90 },
					categories: nodeCategories.map((c) => ({ name: c.name, itemStyle: { color: c.color } })),
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
					data: g.nodes.map((n) => ({
						id: n.key,
						name: n.label,
						category: categoryIndex(n.type),
						symbolSize: n.type === 'object_node' ? 34 : n.type === 'artifact_object' ? 22 : 16,
						// custom fields carried through to click events
						object_id: n.object_id ?? '',
						ao_id:
							n.type === 'artifact_object' && n.key.startsWith('ao:')
								? Number(n.key.slice(3))
								: 0
					})) as any,
					edges: g.edges.map((e) => ({
						source: e.from,
						target: e.to,
						value: e.type,
						lineStyle: { opacity: e.type === 'similar' ? 0.4 : 0.85, curveness: 0.05 }
					})) as any
				}
			]
		};
	});

	function onChartClick(event: any) {
		const data = event?.data;
		if (!data) return;
		if (data.ao_id && data.ao_id > 0) {
			loadLocator({ artifactObjectId: data.ao_id });
		} else if (data.object_id) {
			selectedObjectId = data.object_id;
			loadLocator({ objectId: data.object_id });
			loadGraph({ object_id: data.object_id });
		}
	}

	// ---------- Statistics ----------
	let artifactStats = $state<ArtifactObjectStats | null>(null);
	let nodeStats = $state<ObjectNodeStats | null>(null);
	let statsError = $state('');

	async function loadStats() {
		statsError = '';
		try {
			const [a, n] = await Promise.all([getArtifactObjectStats(), getObjectNodeStats()]);
			artifactStats = a.stats;
			nodeStats = n.stats;
		} catch (err) {
			statsError = err instanceof Error ? err.message : String(err);
		}
	}

	const artifactStatsOption = $derived.by<EChartsOption>(() => {
		const s = artifactStats;
		if (!s) return {};
		return {
			tooltip: { trigger: 'item' },
			title: { text: `Total ${s.total}`, left: 'center', top: 'center', textStyle: { color: textCol, fontSize: 14 } },
			series: [
				{
					type: 'pie',
					radius: ['45%', '70%'],
					label: { color: textCol },
					data: [
						{ name: 'Provisions', value: s.provisions },
						{ name: 'Metrics', value: s.metrics },
						{ name: 'Inventory Items', value: s.inventory_items },
						{ name: 'Other', value: s.other }
					]
				}
			]
		};
	});

	const nodeStatsOption = $derived.by<EChartsOption>(() => {
		const s = nodeStats;
		if (!s) return {};
		return {
			tooltip: { trigger: 'axis' },
			grid: { left: 80, right: 20, top: 20, bottom: 30 },
			xAxis: { type: 'value', axisLabel: { color: mutedCol } },
			yAxis: {
				type: 'category',
				data: ['Provisions', 'Metrics', 'Inventory Items'],
				axisLabel: { color: mutedCol }
			},
			series: [
				{
					type: 'bar',
					data: [s.provisions, s.metrics, s.inventory_items],
					itemStyle: { color: '#93c5fd' }
				}
			]
		};
	});

	// ---------- Connectivity ----------
	let connectivityTopN = $state(50);
	let connectivity = $state<ConnectivityRow[]>([]);
	let connectivityError = $state('');
	const topNChoices = [20, 50, 100, 200, 300];

	async function loadConnectivity() {
		connectivityError = '';
		try {
			const resp = await getObjectConnectivity(connectivityTopN);
			connectivity = resp.rows;
		} catch (err) {
			connectivityError = err instanceof Error ? err.message : String(err);
		}
	}

	const connectivityOption = $derived.by<EChartsOption>(() => {
		if (connectivity.length === 0) return {};
		const top = connectivity.slice(0, 30);
		// Vertical-column histogram: categories along the x-axis, connection
		// counts (values) on the y-axis.
		return {
			tooltip: { trigger: 'axis' },
			grid: { left: 44, right: 16, top: 16, bottom: 96 },
			xAxis: {
				type: 'category',
				data: top.map((r) => r.canonical_name || r.object_id),
				axisLabel: { color: mutedCol, rotate: 45, interval: 0, width: 90, overflow: 'truncate' }
			},
			yAxis: { type: 'value', minInterval: 1, axisLabel: { color: mutedCol } },
			series: [{ type: 'bar', data: top.map((r) => r.connections), itemStyle: { color: '#5eead4' } }]
		};
	});

	// ---------- PDF locator (Right panel) ----------
	let locator = $state<PdfLocator | null>(null);
	let locatorError = $state('');
	const pdfFileUrl = $derived(
		locator && locator.input_record_id > 0
			? `/api/v1/kb/inputs/${locator.input_record_id}/file#zoom=page-width`
			: ''
	);

	async function loadLocator(params: { artifactObjectId?: number; objectId?: string }) {
		locatorError = '';
		try {
			const resp = await getObjectPdfLocator(params);
			locator = resp.locator;
		} catch (err) {
			locatorError = err instanceof Error ? err.message : String(err);
			locator = null;
		}
	}

	// ---------- Merge ----------
	let mergeSurvivorId = $state('');
	let mergeMessage = $state('');
	async function runMerge() {
		if (!selectedNode || !mergeSurvivorId.trim()) return;
		mergeMessage = '';
		try {
			const resp = await mergeObjectNodes(selectedNode.object_id, mergeSurvivorId.trim());
			mergeMessage = `Merged; repointed ${resp.repointed_mentions} mention(s) to ${resp.survivor_object_id}.`;
			mergeSurvivorId = '';
		} catch (err) {
			mergeMessage = err instanceof Error ? err.message : String(err);
		}
	}

	// ---------- Middle panel: draggable / resizable grid ----------
	type WidgetId =
		| 'relation-chart'
		| 'node-info'
		| 'artifact-info'
		| 'stats-artifact'
		| 'stats-nodes'
		| 'connectivity'
		| 'log';

	interface WidgetLayout {
		id: WidgetId;
		colSpan: number;
		height: number;
	}

	const WIDGET_TITLES: Record<WidgetId, string> = {
		'relation-chart': 'Object Relation Chart',
		'node-info': 'Object Node Info',
		'artifact-info': 'Artifact Object Info',
		'stats-artifact': 'Artifact Object Statistics',
		'stats-nodes': 'Object Nodes Statistics',
		connectivity: 'Object Node Connectivity',
		log: 'Log'
	};

	const DEFAULT_LAYOUT: WidgetLayout[] = [
		{ id: 'relation-chart', colSpan: 4, height: 360 },
		{ id: 'node-info', colSpan: 2, height: 220 },
		{ id: 'artifact-info', colSpan: 2, height: 220 },
		{ id: 'stats-artifact', colSpan: 2, height: 240 },
		{ id: 'stats-nodes', colSpan: 2, height: 240 },
		{ id: 'connectivity', colSpan: 4, height: 300 },
		{ id: 'log', colSpan: 4, height: 160 }
	];

	const LAYOUT_KEY = 'object-manager:layout:v1';

	function loadLayout(): WidgetLayout[] {
		if (!browser) return DEFAULT_LAYOUT.map((w) => ({ ...w }));
		try {
			const raw = localStorage.getItem(LAYOUT_KEY);
			if (!raw) return DEFAULT_LAYOUT.map((w) => ({ ...w }));
			const parsed = JSON.parse(raw) as WidgetLayout[];
			const known = new Set(DEFAULT_LAYOUT.map((w) => w.id));
			const filtered = parsed.filter((w) => known.has(w.id));
			// append any widgets missing from stored layout (e.g. new widgets)
			for (const def of DEFAULT_LAYOUT) {
				if (!filtered.some((w) => w.id === def.id)) filtered.push({ ...def });
			}
			return filtered;
		} catch {
			return DEFAULT_LAYOUT.map((w) => ({ ...w }));
		}
	}

	let layout = $state<WidgetLayout[]>(loadLayout());

	function persistLayout() {
		if (browser) localStorage.setItem(LAYOUT_KEY, JSON.stringify(layout));
	}

	let dragIndex = $state<number | null>(null);

	function onDragStart(index: number) {
		dragIndex = index;
	}
	function onDrop(index: number) {
		if (dragIndex === null || dragIndex === index) {
			dragIndex = null;
			return;
		}
		const next = [...layout];
		const [moved] = next.splice(dragIndex, 1);
		next.splice(index, 0, moved);
		layout = next;
		dragIndex = null;
		persistLayout();
	}
	function setColSpan(index: number, delta: number) {
		const next = [...layout];
		next[index] = { ...next[index], colSpan: Math.min(4, Math.max(1, next[index].colSpan + delta)) };
		layout = next;
		persistLayout();
	}
	// Grid snapping: widths snap to columns (colSpan), heights snap to ROW_UNIT.
	const ROW_UNIT = 20;
	const MIN_ROWS = 6;
	const resizeTimers = new Map<number, ReturnType<typeof setTimeout>>();

	function onResizeHeight(index: number, el: HTMLElement) {
		const existing = resizeTimers.get(index);
		if (existing) clearTimeout(existing);
		// Snap shortly after the user stops dragging, so we don't fight the drag.
		resizeTimers.set(
			index,
			setTimeout(() => {
				const raw = Math.round(el.getBoundingClientRect().height);
				const snapped = Math.max(ROW_UNIT * MIN_ROWS, Math.round(raw / ROW_UNIT) * ROW_UNIT);
				el.style.height = `${snapped}px`;
				if (index < layout.length && layout[index].height !== snapped) {
					const next = [...layout];
					next[index] = { ...next[index], height: snapped };
					layout = next;
					persistLayout();
				}
			}, 140)
		);
	}

	// Svelte action: persist a widget's height when the user drags its resize handle.
	function resizeWatcher(el: HTMLElement, index: number) {
		let idx = index;
		const ro = new ResizeObserver(() => onResizeHeight(idx, el));
		ro.observe(el);
		return {
			update(newIndex: number) {
				idx = newIndex;
			},
			destroy() {
				ro.disconnect();
			}
		};
	}
	function resetLayout() {
		layout = DEFAULT_LAYOUT.map((w) => ({ ...w }));
		persistLayout();
	}

	onMount(() => {
		void loadPage(1); // default: list all kb.object_nodes
		void loadStats();
		void loadConnectivity();
	});
</script>

<div
	class="object-manager"
	bind:clientWidth={containerWidth}
	style="--panel-bg:{panelBg}; --card-bg:{cardBg}; --border:{borderCol}; --text:{textCol}; --muted:{mutedCol}; --scrollbar:{scrollThumb}; --left-w:{leftWidth}px; --right-w:{rightWidth}px;"
>
	<!-- Left Panel -->
	<aside class="left">
		<div class="left-header">
			<select class="table-select" bind:value={table} onchange={reloadList} aria-label="Table">
				<option value="object_nodes">kb.object_nodes</option>
				<option value="artifact_objects">kb.artifact_objects</option>
			</select>
			<button type="button" class="search-btn" onclick={openSearchDialog}>Search…</button>
		</div>
		<div class="left-count">
			{rows.length}
			{selectionMode ? 'selected' : 'shown'}
			{#if selectionMode}
				· <button type="button" class="link-btn" onclick={reloadList}>show all</button>
			{/if}
		</div>
		{#if searchError}<p class="error">{searchError}</p>{/if}
		<ul class="record-list">
			{#each rows as r (r.id)}
				<li>
					<button
						type="button"
						class="record"
						class:selected={isNodeRow(r) ? selectedNode?.id === r.id : selectedArtifact?.id === r.id}
						onclick={() => selectRow(r)}
					>
						{#if isNodeRow(r)}
							<span class="record-title">{r.canonical_name || r.object_id}</span>
							<span class="record-sub">{r.object_type} · {r.reconcile_status}</span>
						{:else}
							<span class="record-title">{r.object_name || r.artifact_id}</span>
							<span class="record-sub">{r.artifact_type} · {r.reconcile_status}</span>
						{/if}
					</button>
				</li>
			{/each}
		</ul>
		{#if !selectionMode}
			<div class="pager">
				<button type="button" onclick={() => loadPage(page - 1)} disabled={page <= 1 || loadingList}>
					Prev
				</button>
				<span class="pager-label">Page {page}</span>
				<button type="button" onclick={() => loadPage(page + 1)} disabled={!hasMore || loadingList}>
					Next
				</button>
			</div>
		{/if}
	</aside>

	<!-- Left | Middle resizer -->
	<div
		class="col-resizer"
		role="separator"
		aria-orientation="vertical"
		tabindex="-1"
		onpointerdown={(e) => startPanelResize('left', e)}
	></div>

	<!-- Middle Panel -->
	<main class="middle">
		<div class="middle-toolbar">
			<span class="crumb">{selectedObjectId ? `object: ${selectedObjectId}` : 'No selection'}</span>
			<button type="button" class="link-btn" onclick={resetLayout}>Reset layout</button>
		</div>
		<div class="grid">
			{#each layout as w, i (w.id)}
				<section
					class="widget"
					style="grid-column: span {w.colSpan}; height: {w.height}px;"
					ondragover={(e) => e.preventDefault()}
					ondrop={() => onDrop(i)}
					use:resizeWatcher={i}
					role="group"
					aria-label={WIDGET_TITLES[w.id]}
				>
					<header
						class="widget-head"
						draggable="true"
						role="button"
						tabindex="-1"
						ondragstart={() => onDragStart(i)}
					>
						<span class="widget-title">{WIDGET_TITLES[w.id]}</span>
						<span class="widget-actions">
							<button type="button" title="Narrower" onclick={() => setColSpan(i, -1)}>−</button>
							<span class="span-badge">{w.colSpan}</span>
							<button type="button" title="Wider" onclick={() => setColSpan(i, 1)}>+</button>
						</span>
					</header>
					<div class="widget-body">
						{#if w.id === 'relation-chart'}
							{#if graphLoading}
								<p class="muted">Loading graph…</p>
							{:else if graphError}
								<p class="error">{graphError}</p>
							{:else if graph && graph.nodes.length > 0}
								{#if graph.truncated}<p class="muted small">Graph truncated at node cap.</p>{/if}
								<Chart bind:chart={chartApi} {init} options={graphOption} style="width:100%;height:100%;" onclick={onChartClick} />
							{:else}
								<p class="muted">Select a record to view its relation graph.</p>
							{/if}
						{:else if w.id === 'node-info'}
							{#if selectedNode}
								<dl class="kv">
									<dt>object_id</dt><dd>{selectedNode.object_id}</dd>
									<dt>canonical_name</dt><dd>{selectedNode.canonical_name}</dd>
									<dt>object_type</dt><dd>{selectedNode.object_type}</dd>
									<dt>reconcile_status</dt><dd>{selectedNode.reconcile_status}</dd>
								</dl>
								<div class="merge-row">
									<input type="text" placeholder="Merge into object_id…" bind:value={mergeSurvivorId} />
									<button type="button" onclick={runMerge} disabled={!mergeSurvivorId.trim()}>Merge</button>
								</div>
								{#if mergeMessage}<p class="muted small">{mergeMessage}</p>{/if}
							{:else}
								<p class="muted">No object node selected.</p>
							{/if}
						{:else if w.id === 'artifact-info'}
							{#if selectedArtifact}
								<dl class="kv">
									<dt>id</dt><dd>{selectedArtifact.id}</dd>
									<dt>artifact</dt><dd>{selectedArtifact.artifact_type} / {selectedArtifact.artifact_id}</dd>
									<dt>object_name</dt><dd>{selectedArtifact.object_name}</dd>
									<dt>object_id</dt><dd>{selectedArtifact.object_id || '(unresolved)'}</dd>
									<dt>reconcile_status</dt><dd>{selectedArtifact.reconcile_status}</dd>
								</dl>
							{:else}
								<p class="muted">No artifact object selected.</p>
							{/if}
						{:else if w.id === 'stats-artifact'}
							{#if statsError}<p class="error">{statsError}</p>{/if}
							{#if artifactStats}
								<Chart {init} options={artifactStatsOption} style="width:100%;height:calc(100% - 24px);" />
								<p class="muted small">Unresolved: {artifactStats.unresolved} / {artifactStats.total}</p>
							{:else}
								<p class="muted">Loading…</p>
							{/if}
						{:else if w.id === 'stats-nodes'}
							{#if nodeStats}
								<Chart {init} options={nodeStatsOption} style="width:100%;height:100%;" />
							{:else}
								<p class="muted">Loading…</p>
							{/if}
						{:else if w.id === 'connectivity'}
							<div class="conn-controls">
								<label>Top N
									<select bind:value={connectivityTopN} onchange={loadConnectivity}>
										{#each topNChoices as n (n)}<option value={n}>{n}</option>{/each}
									</select>
								</label>
							</div>
							{#if connectivityError}<p class="error">{connectivityError}</p>{/if}
							{#if connectivity.length > 0}
								<Chart {init} options={connectivityOption} style="width:100%;height:calc(100% - 32px);" />
							{:else}
								<p class="muted">No connectivity data.</p>
							{/if}
						{:else if w.id === 'log'}
							<p class="muted">Log entries for the selected object appear here (kb.object_audit_log).</p>
						{/if}
					</div>
				</section>
			{/each}
		</div>
	</main>

	<!-- Middle | Right resizer -->
	<div
		class="col-resizer"
		role="separator"
		aria-orientation="vertical"
		tabindex="-1"
		onpointerdown={(e) => startPanelResize('right', e)}
	></div>

	<!-- Right Panel -->
	<aside class="right">
		<div class="right-head">
			<span class="widget-title">PDF</span>
			{#if locator}<span class="muted small">{locator.document}</span>{/if}
		</div>
		{#if locatorError}<p class="error">{locatorError}</p>{/if}
		{#if locator && pdfFileUrl}
			<p class="muted small">Spans: {locator.source_line_spans.join(', ') || '—'}</p>
			<iframe class="pdf-frame" src={pdfFileUrl} title={locator.document}></iframe>
		{:else}
			<p class="muted">Click a chart node or record to open its source PDF.</p>
		{/if}
	</aside>

	<!-- Search dialog (conditions → search → pick), same pattern as Provisions -->
	{#if searchDialogOpen}
		<div
			class="dialog-backdrop"
			role="button"
			tabindex="-1"
			onclick={() => (searchDialogOpen = false)}
			onkeydown={(e) => e.key === 'Escape' && (searchDialogOpen = false)}
		>
			<div
				class="dialog"
				role="dialog"
				aria-modal="true"
				aria-label="Search objects"
				tabindex="-1"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
			>
				<div class="dialog-head">
					<span class="dialog-title">Search &amp; Select</span>
					<button type="button" class="ghost" onclick={() => (searchDialogOpen = false)}>Close</button>
				</div>
				<div class="dialog-conditions">
					<label>
						Table
						<select
							bind:value={table}
							onchange={() => {
								searchResults = [];
								checkedIds = new Set();
							}}
						>
							<option value="object_nodes">kb.object_nodes</option>
							<option value="artifact_objects">kb.artifact_objects</option>
						</select>
					</label>
					<label>
						Name contains
						<input
							type="text"
							placeholder="Search text…"
							bind:value={searchQuery}
							onkeydown={(e) => e.key === 'Enter' && runSearch()}
						/>
					</label>
					<label>
						Record ID
						<input
							type="text"
							placeholder="Exact id…"
							bind:value={recordIdInput}
							onkeydown={(e) => e.key === 'Enter' && runSearch()}
						/>
					</label>
					<button type="button" class="primary" onclick={runSearch} disabled={searching}>
						{searching ? 'Searching…' : 'Search'}
					</button>
				</div>
				{#if searchError}<p class="error">{searchError}</p>{/if}
				<ul class="dialog-results">
					{#each searchResults as r (r.id)}
						<li>
							<label class="result-row" class:checked={checkedIds.has(r.id)}>
								<input
									type="checkbox"
									checked={checkedIds.has(r.id)}
									onchange={() => toggleChecked(r.id)}
								/>
								<span class="result-text">
									{#if isNodeRow(r)}
										<span class="record-title">{r.canonical_name || r.object_id}</span>
										<span class="record-sub">{r.object_type} · {r.reconcile_status}</span>
									{:else}
										<span class="record-title">{r.object_name || r.artifact_id}</span>
										<span class="record-sub">{r.artifact_type} / {r.artifact_id} · {r.reconcile_status}</span>
									{/if}
								</span>
							</label>
						</li>
					{:else}
						<li class="muted small dialog-empty">No results yet. Enter conditions and Search.</li>
					{/each}
				</ul>
				<div class="dialog-footer">
					<span class="muted small">{checkedIds.size} selected</span>
					<button
						type="button"
						class="primary select-btn"
						onclick={applySelection}
						disabled={checkedIds.size === 0}
					>
						Select
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.object-manager {
		display: grid;
		grid-template-columns: var(--left-w) 6px minmax(0, 1fr) 6px var(--right-w);
		gap: 6px;
		height: 100%;
		background: var(--panel-bg);
		color: var(--text);
		padding: 8px;
		box-sizing: border-box;
	}
	.col-resizer {
		align-self: stretch;
		width: 6px;
		border-radius: 3px;
		background: var(--border);
		cursor: col-resize;
		touch-action: none;
	}
	.col-resizer:hover {
		background: #5eead4;
	}
	.left,
	.right {
		display: flex;
		flex-direction: column;
		min-height: 0;
		background: var(--card-bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px;
		overflow: hidden;
	}
	.left-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
	}
	.left-count {
		font-size: 11px;
		color: var(--muted);
		margin: 4px 0 2px;
	}
	.search-btn {
		font-size: 12px;
		padding: 4px 10px;
	}
	.table-select {
		flex: 1;
		min-width: 0;
		background: var(--panel-bg);
		color: var(--text);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 5px 8px;
		font-size: 12px;
	}
	.pager {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		padding-top: 8px;
		margin-top: 4px;
		border-top: 1px solid var(--border);
	}
	.pager button {
		font-size: 12px;
		padding: 4px 10px;
	}
	.pager-label {
		font-size: 12px;
		color: var(--muted);
	}
	.conn-controls select,
	.merge-row input,
	.dialog-conditions select,
	.dialog-conditions input {
		background: var(--panel-bg);
		color: var(--text);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 6px 8px;
		font-size: 13px;
	}
	button {
		background: var(--panel-bg);
		color: var(--text);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 6px 10px;
		font-size: 13px;
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.record-list {
		list-style: none;
		margin: 8px 0 0;
		padding: 0;
		overflow-y: auto;
		min-height: 0;
	}
	.record {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		width: 100%;
		text-align: left;
		gap: 2px;
		margin-bottom: 4px;
	}
	.record.selected {
		border-color: #5eead4;
	}
	.record-title {
		font-size: 13px;
		font-weight: 600;
	}
	.record-sub {
		font-size: 11px;
		color: var(--muted);
	}
	.middle {
		display: flex;
		flex-direction: column;
		min-height: 0;
		overflow: hidden;
	}
	.middle-toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0 4px 8px;
	}
	.crumb {
		font-size: 12px;
		color: var(--muted);
	}
	.link-btn {
		border: none;
		background: none;
		color: #5eead4;
		padding: 0;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 8px;
		overflow-y: auto;
		min-height: 0;
		align-content: start;
	}
	.widget {
		display: flex;
		flex-direction: column;
		background: var(--card-bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		overflow: hidden;
		min-width: 0;
		resize: vertical;
	}
	.widget-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 6px 8px;
		border-bottom: 1px solid var(--border);
		cursor: move;
		user-select: none;
	}
	.widget-title {
		font-size: 12px;
		font-weight: 600;
	}
	.widget-actions {
		display: flex;
		align-items: center;
		gap: 4px;
	}
	.widget-actions button {
		padding: 0 6px;
		line-height: 1.6;
	}
	.span-badge {
		font-size: 11px;
		color: var(--muted);
		min-width: 12px;
		text-align: center;
	}
	.widget-body {
		position: relative;
		flex: 1;
		min-height: 0;
		padding: 8px;
		overflow: auto;
	}
	.kv {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 2px 10px;
		margin: 0;
		font-size: 12px;
	}
	.kv dt {
		color: var(--muted);
	}
	.kv dd {
		margin: 0;
		word-break: break-word;
	}
	.merge-row {
		display: flex;
		gap: 6px;
		margin-top: 8px;
	}
	.merge-row input {
		flex: 1;
	}
	.conn-controls {
		margin-bottom: 6px;
		font-size: 12px;
		color: var(--muted);
	}
	.right-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		margin-bottom: 6px;
	}
	.pdf-frame {
		flex: 1;
		width: 100%;
		border: 1px solid var(--border);
		border-radius: 6px;
		min-height: 0;
	}
	.muted {
		color: var(--muted);
		font-size: 13px;
	}
	.small {
		font-size: 11px;
	}
	.error {
		color: #f87171;
		font-size: 12px;
	}

	/* Scrollbars — match the Provisions page (thin, themed thumb). */
	.right {
		overflow-y: auto;
	}
	.record-list,
	.grid,
	.widget-body,
	.right,
	.dialog,
	.dialog-results {
		scrollbar-width: thin;
		scrollbar-color: var(--scrollbar) transparent;
	}
	.record-list::-webkit-scrollbar,
	.grid::-webkit-scrollbar,
	.widget-body::-webkit-scrollbar,
	.right::-webkit-scrollbar,
	.dialog::-webkit-scrollbar,
	.dialog-results::-webkit-scrollbar {
		width: 8px;
		height: 8px;
	}
	.record-list::-webkit-scrollbar-thumb,
	.grid::-webkit-scrollbar-thumb,
	.widget-body::-webkit-scrollbar-thumb,
	.right::-webkit-scrollbar-thumb,
	.dialog::-webkit-scrollbar-thumb,
	.dialog-results::-webkit-scrollbar-thumb {
		background: var(--scrollbar);
		border-radius: 4px;
	}
	.record-list::-webkit-scrollbar-track,
	.grid::-webkit-scrollbar-track,
	.widget-body::-webkit-scrollbar-track,
	.right::-webkit-scrollbar-track,
	.dialog::-webkit-scrollbar-track,
	.dialog-results::-webkit-scrollbar-track {
		background: transparent;
	}

	/* Search dialog */
	.dialog-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.45);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 50;
	}
	.dialog {
		width: min(560px, 92vw);
		max-height: 80vh;
		overflow-y: auto;
		background: var(--card-bg);
		color: var(--text);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 14px;
		box-shadow: 0 12px 40px rgba(0, 0, 0, 0.4);
	}
	.dialog-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 12px;
	}
	.dialog-title {
		font-size: 14px;
		font-weight: 600;
	}
	.dialog-conditions {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 8px;
		align-items: end;
	}
	.dialog-conditions label {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 12px;
		color: var(--muted);
	}
	.primary {
		background: #5eead4;
		color: #0b1220;
		border-color: #5eead4;
		font-weight: 600;
	}
	.primary:disabled {
		opacity: 0.5;
	}
	.dialog-conditions .primary {
		grid-column: 1 / -1;
	}
	.ghost {
		background: transparent;
	}
	.dialog-results {
		list-style: none;
		margin: 12px 0 0;
		padding: 0;
		max-height: 42vh;
		overflow-y: auto;
	}
	.dialog-results li {
		margin-bottom: 4px;
	}
	.result-row {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		padding: 6px 8px;
		border: 1px solid var(--border);
		border-radius: 6px;
		cursor: pointer;
	}
	.result-row.checked {
		border-color: #5eead4;
	}
	.result-text {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.dialog-empty {
		padding: 16px 4px;
	}
	.dialog-footer {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 12px;
		margin-top: 12px;
		padding-top: 12px;
		border-top: 1px solid var(--border);
	}
	.select-btn {
		padding: 6px 18px;
	}
</style>
