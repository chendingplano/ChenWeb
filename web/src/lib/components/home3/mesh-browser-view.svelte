<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount, untrack } from 'svelte';
	import * as echarts from 'echarts/core';
	import type { ECharts } from 'echarts/core';
	import type { EChartsOption } from 'echarts';
	import { GraphChart, TreeChart } from 'echarts/charts';
	import { LegendComponent, TooltipComponent } from 'echarts/components';
	import { LabelLayout } from 'echarts/features';
	import { CanvasRenderer } from 'echarts/renderers';
	import {
		getMeshEntity,
		getMeshNeighborhood,
		getMeshOverview,
		listMeshEntities,
		listMeshTree,
		type MeshEntity,
		type MeshEntityRow,
		type MeshNode,
		type MeshOverview,
		type MeshValue
	} from '$lib/services/meshService';

	echarts.use([
		TreeChart,
		GraphChart,
		TooltipComponent,
		LegendComponent,
		LabelLayout,
		CanvasRenderer
	]);

	// Node shape the ECharts tree series reads: name is shown, id is used on click.
	// Per-node ECharts options: a hidden node has no symbol, label or edge.
	type ChartNode = {
		name: string;
		id: string;
		expandable: boolean;
		symbol?: 'none';
		label?: { show?: boolean; color?: string; fontWeight?: 'bold' };
		lineStyle?: { opacity: number };
		children?: ChartNode[];
	};

	// Relation graph: the shown record, its filed descriptors, and every record either links to.
	// Each record's values hang off one hub node per relation (record → hub → values).
	// Category 0 is the shown record, 1 a filed descriptor; the rest are one per relation.
	type GraphNode = {
		id: string;
		name: string;
		category: number;
		symbolSize: number;
		symbol?: 'roundRect';
		label?: { fontSize: number; fontWeight: 'bold' };
	};
	type GraphLink = { source: string; target: string; value: string };
	type RelationGraph = { nodes: GraphNode[]; links: GraphLink[]; categories: { name: string }[] };

	type Box = { x: number; y: number; w: number; h: number };
	type GraphMini = {
		box: Box;
		view: Box;
		scale: number;
		nodes: { x: number; y: number; color: string }[];
		links: { x1: number; y1: number; x2: number; y2: number }[];
	};

	// The graph series' laid-out node positions and colors are not in the public ECharts API.
	type GraphSeriesData = {
		count(): number;
		getItemLayout(i: number): number[] | undefined;
		getItemVisual(i: number, key: 'style'): { fill?: string } | undefined;
	};
	type GraphSeriesModel = { getData(): GraphSeriesData };
	type ChartInternals = {
		getModel(): { getSeriesByIndex(i: number): GraphSeriesModel | undefined };
		getViewOfSeriesModel(
			model: GraphSeriesModel
		): { group: { x: number; y: number; dirty(): void } };
	};

	// Label colour of the node whose details are shown.
	const SELECTED_LABEL_COLOR = '#F59E0B';

	const HIDDEN_NODE = {
		symbol: 'none',
		label: { show: false },
		lineStyle: { opacity: 0 }
	} as const;

	let { darkMode = false }: { darkMode?: boolean } = $props();

	// Record kinds and relations that get a translated name; anything else shows its raw name.
	const kindLabels: Record<string, () => string> = {
		TopicalDescriptor: () => m.mesh_kind_topical_descriptor(),
		GeographicalDescriptor: () => m.mesh_kind_geographical_descriptor(),
		Qualifier: () => m.mesh_kind_qualifier(),
		Concept: () => m.mesh_kind_concept(),
		Term: () => m.mesh_kind_term(),
		TreeNumber: () => m.mesh_kind_tree_number(),
		PublicationType: () => m.mesh_kind_publication_type(),
		SCR_Chemical: () => m.mesh_kind_scr_chemical(),
		SCR_Organism: () => m.mesh_kind_scr_organism(),
		SCR_Disease: () => m.mesh_kind_scr_disease()
	};

	const predicateLabels: Record<string, () => string> = {
		treeNumber: () => m.mesh_pred_tree_number(),
		parentTreeNumber: () => m.mesh_pred_parent_tree_number(),
		broaderDescriptor: () => m.mesh_pred_broader_descriptor(),
		concept: () => m.mesh_pred_concept(),
		term: () => m.mesh_pred_term(),
		preferredConcept: () => m.mesh_pred_preferred_concept(),
		preferredTerm: () => m.mesh_pred_preferred_term(),
		allowableQualifier: () => m.mesh_pred_allowable_qualifier(),
		pharmacologicalAction: () => m.mesh_pred_pharmacological_action(),
		scopeNote: () => m.mesh_pred_scope_note(),
		dateCreated: () => m.mesh_pred_date_created(),
		dateRevised: () => m.mesh_pred_date_revised(),
		dateEstablished: () => m.mesh_pred_date_established(),
		historyNote: () => m.mesh_pred_history_note(),
		previousIndexing: () => m.mesh_pred_previous_indexing(),
		identifier: () => m.mesh_identifier(),
		lexicalTag: () => m.mesh_pred_lexical_tag()
	};

	// Hub node names for relations; any other relation's hub uses its predicate name.
	const hubLabels: Record<string, () => string> = {
		allowableQualifier: () => m.mesh_hub_allowable_qualifier(),
		broaderDescriptor: () => m.mesh_hub_broader_descriptor(),
		concept: () => m.mesh_hub_concept(),
		preferredTerm: () => m.mesh_hub_preferred_term(),
		previousIndexing: () => m.mesh_hub_previous_indexing(),
		treeNumber: () => m.mesh_hub_tree_number()
	};

	// Relations whose values are plain text, not links, but still get a node each in the graph.
	const TEXT_NODE_PREDICATES = new Set(['previousIndexing']);

	type Mode = 'overview' | 'kind';
	type Resizing = 'tree' | 'detail' | 'split' | null;

	const TREE_WIDTH = { min: 200, max: 480 };
	const DETAIL_WIDTH = { min: 260, max: 600 };
	// Share of the chart area, in percent, taken by the tree canvas when the two canvases sit side by side.
	const TREE_SHARE = { min: 20, max: 80 };

	// Tree chart canvas: three fixed columns (root, children, grandchildren) and one row per leaf.
	const CANVAS_MIN_W = 620;
	// Room for the root's label on the left and the deepest labels on the right, plus one column of
	// this width per level, so a node's label never reaches the next column.
	const CANVAS_MARGIN_W = 280;
	const COLUMN_W = 280;
	const ROW_H = 22;
	const CHART_MIN_H = 360;
	// Miniature of the whole canvas in the corner of the chart.
	const MINI_W = 180;
	const MINI_H = 200;
	// A tall canvas would shrink the miniature to a sliver, so it is stretched to at least this width.
	const MINI_MIN_W = 120;

	// Working area: overview and kind list, or the tree chart when a tree number is selected.
	let mode = $state<Mode>('overview');
	let overview = $state<MeshOverview | null>(null);
	let overviewError = $state('');

	let kind = $state('');
	let query = $state('');
	let items = $state<MeshEntityRow[]>([]);
	let hasMore = $state(false);
	let listLoading = $state(false);
	let listError = $state('');

	// Tree sidebar: top-level categories, with children loaded when a node opens.
	let roots = $state<MeshEntityRow[]>([]);
	let open = $state<Record<string, boolean>>({});
	let children = $state<Record<string, MeshEntityRow[]>>({});

	// Tree chart: the selected tree number is the root; its neighborhood is loaded for the chart.
	let chartRootId = $state('');
	let graph = $state<MeshNode | null>(null);
	let graphError = $state('');
	// Circles clicked to fold their children, by tree number id.
	let folded = $state<Record<string, boolean>>({});
	let chartEl = $state<HTMLDivElement>();
	let chart = $state.raw<ECharts>();
	let viewportEl = $state<HTMLDivElement>();
	let miniEl = $state<HTMLDivElement>();
	let miniUrl = $state('');
	let scrollX = $state(0);
	let scrollY = $state(0);
	let viewW = $state(0);
	let viewH = $state(0);
	let miniDragging = false;

	// Detail panel: the record last clicked.
	let detailId = $state('');
	let entity = $state<MeshEntity | null>(null);
	let entityLoading = $state(false);
	let entityError = $state('');

	// Descriptors filed under the shown tree number: each expands inline to its own properties.
	let filedOpen = $state<Record<string, boolean>>({});
	let filedEntity = $state<Record<string, MeshEntity>>({});
	let filedError = $state<Record<string, string>>({});
	// Filed descriptors being fetched, so the detail panel and the graph do not fetch one twice.
	const filedPending = new Set<string>();

	// Relation graph canvas.
	let relEl = $state<HTMLDivElement>();
	let relChart = $state.raw<ECharts>();
	// Miniature of the whole graph, in the graph's own layout coordinates, with the visible part.
	let graphMini = $state<GraphMini | null>(null);
	let relMiniEl = $state<HTMLDivElement>();
	let relMiniDragging = false;
	let relMiniFrame = 0;

	// Panel widths, set by the dividers between panels.
	let treeWidth = $state(288);
	let detailWidth = $state(360);
	let treeShare = $state(50);
	let splitEl = $state<HTMLDivElement>();
	let resizing = $state<Resizing>(null);
	let resizeStartX = 0;
	let resizeStartWidth = 0;

	let bg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let panelBg = $derived(darkMode ? '#252A3A' : '#FFFFFF');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let hoverBg = $derived(darkMode ? 'rgba(45,51,72,0.6)' : 'rgba(228,230,235,0.7)');

	function kindName(k: string): string {
		return kindLabels[k]?.() ?? k;
	}

	function predicateName(p: string): string {
		return predicateLabels[p]?.() ?? p;
	}

	function errorText(e: unknown): string {
		return e instanceof Error ? e.message : String(e);
	}

	function clamp(value: number, range: { min: number; max: number }): number {
		return Math.max(range.min, Math.min(range.max, value));
	}

	function startResize(e: PointerEvent, which: Exclude<Resizing, null>) {
		e.preventDefault();
		resizing = which;
		resizeStartX = e.clientX;
		resizeStartWidth = which === 'tree' ? treeWidth : which === 'detail' ? detailWidth : treeShare;
		window.addEventListener('pointermove', onResizeMove);
		window.addEventListener('pointerup', endResize, { once: true });
	}

	function onResizeMove(e: PointerEvent) {
		const dx = e.clientX - resizeStartX;
		// The tree divider sits on the right of its panel; the detail divider on the left of its panel.
		if (resizing === 'tree') treeWidth = clamp(resizeStartWidth + dx, TREE_WIDTH);
		if (resizing === 'detail') detailWidth = clamp(resizeStartWidth - dx, DETAIL_WIDTH);
		// The split divider sits between the tree and graph canvases.
		if (resizing === 'split' && splitEl)
			treeShare = clamp(resizeStartWidth + (dx / splitEl.clientWidth) * 100, TREE_SHARE);
	}

	function endResize() {
		resizing = null;
		window.removeEventListener('pointermove', onResizeMove);
	}

	async function loadOverview() {
		try {
			overview = await getMeshOverview();
		} catch (e) {
			overviewError = errorText(e);
		}
	}

	async function loadRoots() {
		try {
			roots = await listMeshTree('');
		} catch {
			roots = [];
		}
	}

	async function loadKind(reset: boolean) {
		listLoading = true;
		listError = '';
		try {
			const page = await listMeshEntities(kind, query, reset ? 0 : items.length);
			items = reset ? page.items : [...items, ...page.items];
			hasMore = page.has_more;
		} catch (e) {
			listError = errorText(e);
		} finally {
			listLoading = false;
		}
	}

	function openKind(k: string) {
		kind = k;
		query = '';
		mode = 'kind';
		loadKind(true);
	}

	function search(value: string) {
		query = value.trim();
		loadKind(true);
	}

	async function loadFiled(id: string) {
		if (filedEntity[id] || filedPending.has(id)) return;
		filedPending.add(id);
		try {
			filedEntity = { ...filedEntity, [id]: await getMeshEntity(id) };
		} catch (e) {
			filedError = { ...filedError, [id]: errorText(e) };
		} finally {
			filedPending.delete(id);
		}
	}

	async function toggleFiled(id: string) {
		filedOpen = { ...filedOpen, [id]: !filedOpen[id] };
		if (filedOpen[id]) await loadFiled(id);
	}

	async function openEntity(id: string) {
		detailId = id;
		entity = null;
		entityError = '';
		entityLoading = true;
		filedOpen = {};
		try {
			const loaded = await getMeshEntity(id);
			if (detailId !== id) return;
			entity = loaded;
			// The first filed descriptor opens by default.
			const first = loaded.filed?.[0]?.id;
			if (first) {
				filedOpen = { [first]: true };
				loadFiled(first);
			}
		} catch (e) {
			if (detailId === id) entityError = errorText(e);
		} finally {
			if (detailId === id) entityLoading = false;
		}
	}

	async function expandNode(id: string) {
		open = { ...open, [id]: true };
		if (!children[id]) {
			try {
				children = { ...children, [id]: await listMeshTree(id) };
			} catch {
				children = { ...children, [id]: [] };
			}
		}
	}

	// Clicking a tree number expands it, opens its detail and makes it the chart's root.
	function selectTreeNode(id: string) {
		if (chartRootId !== id) folded = {};
		chartRootId = id;
		openEntity(id);
		expandNode(id);
	}

	function toggleNode(node: MeshEntityRow) {
		if (open[node.id]) {
			open = { ...open, [node.id]: false };
		} else {
			expandNode(node.id);
		}
	}

	function toggleFolded(id: string) {
		folded = { ...folded, [id]: !folded[id] };
	}

	function closeChart() {
		chartRootId = '';
	}

	function withHidden(node: ChartNode, hidden: boolean): ChartNode {
		return hidden ? { ...node, ...HIDDEN_NODE } : node;
	}

	// Chart data in the shape ECharts' tree series expects. Folding keeps the hidden children in
	// the data, so every other node keeps its position: a hidden node draws no symbol, label or edge.
	function toChartNode(
		n: MeshNode,
		folded: Record<string, boolean>,
		hidden: boolean,
		selected: string
	): ChartNode {
		const count = n.child_count ?? 0;
		const expandable = count > 0;
		const isFolded = !!folded[n.id] && expandable;
		const label = n.label || n.id;
		const name = expandable ? `${label} (${count})` : label;
		const kids: ChartNode[] = (n.children ?? []).map((k) =>
			toChartNode(k, folded, hidden || isFolded, selected)
		);
		if (n.hidden) {
			kids.push(
				withHidden(
					{ name: m.mesh_chart_more({ count: n.hidden }), id: '', expandable: false },
					hidden || isFolded
				)
			);
		}
		const node: ChartNode = { name, id: n.id, expandable, children: kids };
		if (n.id === selected) node.label = { color: SELECTED_LABEL_COLOR, fontWeight: 'bold' };
		return withHidden(node, hidden);
	}

	function findNode(n: MeshNode, id: string): MeshNode | undefined {
		if (n.id === id) return n;
		for (const k of n.children ?? []) {
			const found = findNode(k, id);
			if (found) return found;
		}
		return undefined;
	}

	// Clicking a node folds or unfolds it. The first unfold loads its children.
	async function toggleChartNode(id: string) {
		const node = graph && findNode(graph, id);
		if (!node) return;
		if (node.children?.length) {
			toggleFolded(id);
			return;
		}
		try {
			const loaded = await getMeshNeighborhood(id);
			node.children = loaded.children;
			node.hidden = loaded.hidden;
			folded = { ...folded, [id]: false };
		} catch (e) {
			graphError = errorText(e);
		}
	}

	// One row per leaf, plus one for each "N more" placeholder.
	function leafRows(n: MeshNode): number {
		const kids = n.children ?? [];
		if (kids.length === 0) return 1;
		return kids.reduce((sum, k) => sum + leafRows(k), 0) + (n.hidden ? 1 : 0);
	}

	function depthOf(n: MeshNode): number {
		return (n.children ?? []).reduce((max, k) => Math.max(max, depthOf(k) + 1), 0);
	}

	let canvasW = $derived(
		graph ? Math.max(CANVAS_MIN_W, CANVAS_MARGIN_W + depthOf(graph) * COLUMN_W) : CANVAS_MIN_W
	);
	let chartH = $derived(graph ? Math.max(CHART_MIN_H, leafRows(graph) * ROW_H + 40) : CHART_MIN_H);
	let miniScale = $derived(Math.min(MINI_W / canvasW, MINI_H / chartH));
	let miniW = $derived(Math.max(canvasW * miniScale, MINI_MIN_W));
	let miniH = $derived(chartH * miniScale);
	let miniScaleX = $derived(miniW / canvasW);

	function treeOption(
		root: MeshNode,
		dark: boolean,
		folded: Record<string, boolean>,
		selected: string
	): EChartsOption {
		const text = dark ? '#E2E8F0' : '#111827';
		const line = dark ? '#64748B' : '#9CA3AF';
		return {
			tooltip: { trigger: 'item', triggerOn: 'mousemove' },
			series: [
				{
					type: 'tree',
					data: [toChartNode(root, folded, false, selected)],
					top: 20,
					bottom: 20,
					left: 90,
					right: 190,
					symbolSize: 8,
					// Folding is driven by the click handler (circle only), not ECharts' own toggle.
					expandAndCollapse: false,
					lineStyle: { color: line },
					itemStyle: { color: dark ? '#818CF8' : '#6366F1' },
					// Labels stay on the right of their circle, so a node's label does not move when it
					// gains children.
					label: {
						position: 'right',
						verticalAlign: 'middle',
						align: 'left',
						fontSize: 12,
						color: text
					},
					emphasis: { focus: 'descendant' },
					animationDuration: 550,
					animationDurationUpdate: 750
				}
			]
		} as EChartsOption;
	}

	// The shown record, its filed descriptors, and the records each of them links to, grouped
	// under one hub per relation. A record reached by several relations takes the category of
	// the first one.
	function buildRelationGraph(center: MeshEntity, descriptors: MeshEntity[]): RelationGraph {
		const categories = [{ name: m.mesh_graph_selected() }, { name: m.mesh_graph_descriptor() }];
		const categoryOf: Record<string, number> = {};
		const nodes = new Map<string, GraphNode>();
		const links: GraphLink[] = [];
		const linked = new Set<string>();

		function addNode(id: string, name: string, category: number, symbolSize: number, extra = {}) {
			if (!nodes.has(id)) nodes.set(id, { id, name: name || id, category, symbolSize, ...extra });
		}
		function addLink(source: string, target: string, relation: string) {
			const key = [source, target].sort().join('|');
			if (source === target || linked.has(key)) return;
			linked.add(key);
			links.push({ source, target, value: relation });
		}
		function addRelations(ent: MeshEntity) {
			for (const prop of ent.properties ?? []) {
				const withText = TEXT_NODE_PREDICATES.has(prop.predicate);
				const values = (prop.values ?? []).filter((v) => v.id || (withText && v.text));
				if (values.length === 0) continue;
				const relation = predicateName(prop.predicate);
				if (categoryOf[prop.predicate] === undefined) {
					categoryOf[prop.predicate] = categories.length;
					categories.push({ name: relation });
				}
				const category = categoryOf[prop.predicate];
				const hub = `${ent.id}#${prop.predicate}`;
				const hubName = hubLabels[prop.predicate]?.() ?? relation;
				addNode(hub, hubName, category, 18, { symbol: 'roundRect' });
				addLink(ent.id, hub, relation);
				values.forEach((v, i) => {
					const id = v.id || `${hub}#${i}`;
					addNode(id, v.label || v.id || v.text || '', category, 12);
					addLink(hub, id, relation);
				});
			}
		}

		addNode(center.id, center.label || center.id, 0, 60, {
			label: { fontSize: 15, fontWeight: 'bold' }
		});
		for (const d of descriptors) {
			addNode(d.id, d.label || d.id, 1, 60, { label: { fontSize: 15, fontWeight: 'bold' } });
			addLink(center.id, d.id, m.mesh_filed_under());
		}
		addRelations(center);
		for (const d of descriptors) addRelations(d);
		return { nodes: [...nodes.values()], links, categories };
	}

	let relationGraph = $derived(
		entity
			? buildRelationGraph(
					entity,
					(entity.filed ?? []).map((r) => filedEntity[r.id]).filter((d): d is MeshEntity => !!d)
				)
			: null
	);

	function graphOption(g: RelationGraph, dark: boolean): EChartsOption {
		const text = dark ? '#E2E8F0' : '#111827';
		const line = dark ? '#64748B' : '#9CA3AF';
		return {
			animation: false,
			tooltip: { trigger: 'item' },
			legend: [
				{
					type: 'scroll',
					top: 8,
					left: 12,
					right: 12,
					data: g.categories.map((c) => c.name),
					textStyle: { color: text, fontSize: 11 },
					pageTextStyle: { color: text }
				}
			],
			series: [
				{
					type: 'graph',
					layout: 'force',
					top: 50,
					data: g.nodes,
					links: g.links,
					categories: g.categories,
					// Wheel zooms, drag pans.
					roam: true,
					zoom: 0.8,
					scaleLimit: { min: 0.2, max: 10 },
					draggable: true,
					// Laid out once, with no animation; wide spacing so the labels have room.
					force: {
						repulsion: 600,
						edgeLength: [120, 240],
						gravity: 0.03,
						layoutAnimation: false
					},
					label: { show: true, position: 'right', fontSize: 11, color: text, formatter: '{b}' },
					labelLayout: { hideOverlap: true },
					lineStyle: { color: line, opacity: 0.7, curveness: 0.1 },
					emphasis: { focus: 'adjacency', lineStyle: { width: 3 } }
				}
			]
		} as EChartsOption;
	}

	// The graph needs every filed descriptor of the shown record, not only the one opened in the panel.
	$effect(() => {
		const filed = entity?.filed ?? [];
		untrack(() => filed.forEach((r) => loadFiled(r.id)));
	});

	// Create the relation graph when its container is shown. Clicking a node shows that record.
	$effect(() => {
		const el = relEl;
		if (!el) return;
		const instance = echarts.init(el);
		instance.on('click', (p) => {
			const id = p.dataType === 'node' ? (p.data as GraphNode | undefined)?.id : undefined;
			if (id && id !== detailId) openEntity(id);
		});
		// Redraw the miniature after each render, and after panning or zooming, which do not re-render.
		instance.on('finished', refreshGraphMini);
		instance.on('graphRoam', refreshGraphMini);
		const observer = new ResizeObserver(() => instance.resize());
		observer.observe(el);
		relChart = instance;
		return () => {
			observer.disconnect();
			instance.dispose();
			relChart = undefined;
		};
	});

	$effect(() => {
		if (!relChart) return;
		if (relationGraph) {
			relChart.setOption(graphOption(relationGraph, darkMode), true);
			// Pin every node where the layout put it: a resize re-runs the force layout, which would
			// otherwise move the nodes toward the new center and turn the graph around.
			relChart.setOption({
				series: [{ data: relationGraph.nodes.map((n) => ({ ...n, fixed: true })) }]
			});
		} else {
			relChart.clear();
			graphMini = null;
		}
	});

	function refreshGraphMini() {
		const g = relationGraph;
		const data =
			relChart && g
				? (relChart as unknown as ChartInternals).getModel().getSeriesByIndex(0)?.getData()
				: undefined;
		if (!relChart || !g || !data || data.count() === 0) {
			graphMini = null;
			return;
		}
		const index: Record<string, number> = {};
		g.nodes.forEach((n, i) => (index[n.id] = i));
		const points = g.nodes.map((_, i) => data.getItemLayout(i) ?? [NaN, NaN]);
		const placed = points.filter((p) => !isNaN(p[0]) && !isNaN(p[1]));
		if (placed.length === 0) return;
		const xs = placed.map((p) => p[0]);
		const ys = placed.map((p) => p[1]);
		const minX = Math.min(...xs);
		const minY = Math.min(...ys);
		const pad = Math.max(Math.max(...xs) - minX, Math.max(...ys) - minY, 100) * 0.08;
		const box = {
			x: minX - pad,
			y: minY - pad,
			w: Math.max(...xs) - minX + 2 * pad,
			h: Math.max(...ys) - minY + 2 * pad
		};
		const p0 = relChart.convertFromPixel({ seriesIndex: 0 }, [0, 0]) as number[];
		const p1 = relChart.convertFromPixel({ seriesIndex: 0 }, [
			relChart.getWidth(),
			relChart.getHeight()
		]) as number[];
		graphMini = {
			box,
			view: { x: p0[0], y: p0[1], w: p1[0] - p0[0], h: p1[1] - p0[1] },
			scale: Math.min(MINI_W / box.w, MINI_H / box.h),
			nodes: points.map((p, i) => ({
				x: p[0],
				y: p[1],
				color: data.getItemVisual(i, 'style')?.fill ?? accent
			})),
			links: g.links.map((l) => {
				const a = points[index[l.source]];
				const b = points[index[l.target]];
				return { x1: a[0], y1: a[1], x2: b[0], y2: b[1] };
			})
		};
	}

	// Center the graph on the pointer position inside its miniature, at most once per frame.
	// This pans the way ECharts' own drag does (move the drawn group, then record the pan with
	// graphRoam): changing the series center instead would run the force layout again.
	function moveGraphTo(e: PointerEvent) {
		if (!relMiniEl || !graphMini) return;
		const r = relMiniEl.getBoundingClientRect();
		const fx = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
		const fy = Math.min(1, Math.max(0, (e.clientY - r.top) / r.height));
		const { box } = graphMini;
		const center = [box.x + fx * box.w, box.y + fy * box.h];
		cancelAnimationFrame(relMiniFrame);
		relMiniFrame = requestAnimationFrame(() => {
			const internals = relChart as unknown as ChartInternals | undefined;
			const model = internals?.getModel().getSeriesByIndex(0);
			if (!relChart || !internals || !model) return;
			const [px, py] = relChart.convertToPixel({ seriesIndex: 0 }, center) as number[];
			const dx = relChart.getWidth() / 2 - px;
			const dy = relChart.getHeight() / 2 - py;
			const group = internals.getViewOfSeriesModel(model).group;
			group.x += dx;
			group.y += dy;
			group.dirty();
			relChart.dispatchAction({ type: 'graphRoam', seriesIndex: 0, dx, dy });
		});
	}

	function startGraphMiniDrag(e: PointerEvent) {
		if (!relMiniEl) return;
		relMiniDragging = true;
		relMiniEl.setPointerCapture(e.pointerId);
		moveGraphTo(e);
	}

	function moveGraphMiniDrag(e: PointerEvent) {
		if (relMiniDragging) moveGraphTo(e);
	}

	function endGraphMiniDrag() {
		relMiniDragging = false;
	}

	// Load the neighborhood whenever the chart root changes.
	$effect(() => {
		const id = chartRootId;
		graph = null;
		graphError = '';
		if (!id) return;
		let cancelled = false;
		getMeshNeighborhood(id)
			.then((g) => {
				if (!cancelled) graph = g;
			})
			.catch((e) => {
				if (!cancelled) graphError = errorText(e);
			});
		return () => {
			cancelled = true;
		};
	});

	// Create the chart when its container is shown; dispose it when the chart closes.
	$effect(() => {
		const el = chartEl;
		if (!el) return;
		const instance = echarts.init(el);
		// Every click shows the node's details and folds or unfolds it. The chart keeps its root.
		instance.on('click', (p) => {
			const data = p.data as ChartNode | undefined;
			if (!data?.id) return;
			openEntity(data.id);
			if (data.expandable) toggleChartNode(data.id);
		});
		instance.on('finished', refreshMini);
		const observer = new ResizeObserver(() => instance.resize());
		observer.observe(el);
		chart = instance;
		return () => {
			observer.disconnect();
			instance.dispose();
			chart = undefined;
		};
	});

	// Redraw whenever the loaded tree or the theme changes.
	$effect(() => {
		if (!chart || !graph) return;
		chart.setOption(treeOption(graph, darkMode, folded, detailId), true);
		chart.resize();
	});

	// Track the viewport size so the miniature's visible rectangle stays correct.
	$effect(() => {
		const el = viewportEl;
		if (!el) return;
		const observer = new ResizeObserver(updateView);
		observer.observe(el);
		updateView();
		return () => observer.disconnect();
	});

	function updateView() {
		if (!viewportEl) return;
		scrollX = viewportEl.scrollLeft;
		scrollY = viewportEl.scrollTop;
		viewW = viewportEl.clientWidth;
		viewH = viewportEl.clientHeight;
	}

	// The miniature is a PNG of the chart, redrawn after each chart render.
	function refreshMini() {
		if (!chart) return;
		miniUrl = chart.getDataURL({ type: 'png', pixelRatio: 1, backgroundColor: panelBg });
		updateView();
	}

	// Center the viewport on the pointer position inside the miniature.
	function moveViewportTo(e: PointerEvent) {
		if (!miniEl || !viewportEl) return;
		const r = miniEl.getBoundingClientRect();
		const fx = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
		const fy = Math.min(1, Math.max(0, (e.clientY - r.top) / r.height));
		viewportEl.scrollLeft = fx * canvasW - viewportEl.clientWidth / 2;
		viewportEl.scrollTop = fy * chartH - viewportEl.clientHeight / 2;
	}

	function startMiniDrag(e: PointerEvent) {
		if (!miniEl) return;
		miniDragging = true;
		miniEl.setPointerCapture(e.pointerId);
		moveViewportTo(e);
	}

	function moveMiniDrag(e: PointerEvent) {
		if (miniDragging) moveViewportTo(e);
	}

	function endMiniDrag() {
		miniDragging = false;
	}

	onMount(() => {
		loadOverview();
		loadRoots();
	});
</script>

{#snippet treeNodes(nodes: MeshEntityRow[], depth: number)}
	{#each nodes as node (node.id)}
		<div>
			<div class="flex items-center gap-1" style="padding-left:{depth * 12}px;">
				<button
					type="button"
					class="w-5 shrink-0 text-center text-xs"
					style="color:{textSecondary};"
					aria-label={open[node.id] ? m.mesh_collapse() : m.mesh_expand()}
					onclick={() => toggleNode(node)}>{open[node.id] ? '▾' : '▸'}</button
				>
				<button
					type="button"
					class="min-w-0 flex-1 truncate rounded px-1.5 py-1 text-left text-sm"
					style="color:{chartRootId === node.id ? accent : textPrimary};"
					onclick={() => selectTreeNode(node.id)}
					onmouseenter={(e) => (e.currentTarget.style.background = hoverBg)}
					onmouseleave={(e) => (e.currentTarget.style.background = 'transparent')}
				>
					<span class="font-mono text-xs" style="color:{accent};">{node.id}</span>
					<span>{node.label}</span>
				</button>
			</div>
			{#if open[node.id] && children[node.id]}
				{#if children[node.id].length === 0}
					<div
						class="text-xs"
						style="padding-left:{(depth + 1) * 12 + 20}px; color:{textSecondary};"
					>
						{m.mesh_no_children()}
					</div>
				{:else}
					{@render treeNodes(children[node.id], depth + 1)}
				{/if}
			{/if}
		</div>
	{/each}
{/snippet}

{#snippet propertyList(ent: MeshEntity)}
	<div class="overflow-hidden rounded-lg border" style="border-color:{borderColor};">
		{#each ent.properties as prop (prop.predicate)}
			<div class="border-b px-3 py-2 text-sm last:border-b-0" style="border-color:{borderColor};">
				<div class="text-xs font-medium" title={prop.predicate} style="color:{textSecondary};">
					{predicateName(prop.predicate)}
				</div>
				<div class="mt-0.5 flex min-w-0 flex-wrap gap-x-3 gap-y-1">
					{#each prop.values as v (v.id ?? v.text)}
						{@render valueCell(v)}
					{/each}
				</div>
			</div>
		{/each}
	</div>
{/snippet}

{#snippet valueCell(v: MeshValue)}
	{#if v.id}
		<button
			type="button"
			class="text-left text-sm underline decoration-dotted underline-offset-2"
			style="color:{accent};"
			onclick={() => openEntity(v.id!)}
		>
			{v.label || v.id}
		</button>
	{:else}
		<span class="min-w-0 break-words">{v.text}</span>
		{#if v.lang}
			<span
				class="self-center rounded px-1 text-[10px] uppercase"
				style="background:{hoverBg}; color:{textSecondary};">{v.lang}</span
			>
		{/if}
	{/if}
{/snippet}

<div
	class="mesh-browser flex h-full min-h-0 flex-col select-text md:flex-row"
	style="background:{bg}; color:{textPrimary};"
>
	<aside
		class="mesh-tree flex min-h-0 shrink-0 flex-col overflow-hidden border-b md:border-b-0"
		style="width:{treeWidth}px; background:{panelBg}; border-color:{borderColor};"
	>
		<div
			class="px-3 py-2 text-xs font-semibold tracking-wide uppercase"
			style="color:{textSecondary};"
		>
			{m.mesh_hierarchy()}
		</div>
		<div
			class="min-h-0 flex-1 overflow-y-auto px-2 pb-3"
			style="scrollbar-width:thin; scrollbar-color:{borderColor} transparent;"
		>
			{#if roots.length === 0}
				<div class="px-2 py-1 text-xs" style="color:{textSecondary};">{m.mesh_loading()}</div>
			{:else}
				{@render treeNodes(roots, 0)}
			{/if}
		</div>
	</aside>

	<div
		class="mesh-resize"
		class:is-resizing={resizing === 'tree'}
		role="separator"
		aria-orientation="vertical"
		onpointerdown={(e) => startResize(e, 'tree')}
	></div>

	<section
		class="mesh-main flex min-h-0 min-w-0 flex-1 flex-col p-4 md:p-6 {chartRootId
			? 'overflow-hidden'
			: 'overflow-y-auto'}"
	>
		{#if chartRootId}
			<div class="flex flex-wrap items-baseline gap-3 pb-3">
				<h2 class="text-lg font-semibold">
					{m.mesh_chart_title({ label: graph?.label || chartRootId })}
				</h2>
				<span class="font-mono text-xs" style="color:{textSecondary};">{chartRootId}</span>
				<button type="button" class="ml-auto text-sm" style="color:{accent};" onclick={closeChart}
					>{m.mesh_chart_close()}</button
				>
			</div>
			{#if graphError}
				<div class="text-sm text-red-500">{m.mesh_error({ message: graphError })}</div>
			{:else}
				<div
					bind:this={splitEl}
					class="flex min-h-0 flex-1 flex-col gap-3 xl:flex-row xl:gap-1"
					style="--tree-share:{treeShare}%;"
				>
					<div
						class="flex min-h-[360px] min-w-0 flex-1 flex-col xl:w-[var(--tree-share)] xl:flex-none"
					>
						<div
							class="pb-1 text-xs font-semibold tracking-wide uppercase"
							style="color:{textSecondary};"
						>
							{m.mesh_tree_canvas()}
						</div>
						<div class="relative flex min-h-0 flex-1">
							<div
								bind:this={viewportEl}
								onscroll={updateView}
								class="absolute inset-0 overflow-auto rounded-lg border"
								style="border-color:{borderColor}; background:{panelBg}; scrollbar-width:thin; scrollbar-color:{borderColor} transparent;"
							>
								<div class="relative" style="width:{canvasW}px; height:{chartH}px;">
									<div bind:this={chartEl} class="absolute inset-0"></div>
								</div>
							</div>
							{#if miniUrl}
								<div
									bind:this={miniEl}
									role="presentation"
									aria-label={m.mesh_minimap()}
									class="absolute right-3 bottom-3 cursor-pointer touch-none overflow-hidden rounded border shadow-lg"
									style="width:{miniW}px; height:{miniH}px; border-color:{borderColor}; background:{panelBg};"
									onpointerdown={startMiniDrag}
									onpointermove={moveMiniDrag}
									onpointerup={endMiniDrag}
								>
									<img
										src={miniUrl}
										alt=""
										draggable="false"
										class="absolute inset-0 h-full w-full"
									/>
									<div
										class="pointer-events-none absolute border-2"
										style="left:{scrollX * miniScaleX}px; top:{scrollY *
											miniScale}px; width:{Math.min(viewW * miniScaleX, miniW)}px; height:{Math.min(
											viewH * miniScale,
											miniH
										)}px; border-color:{accent}; background:rgba(129,140,248,0.15);"
									></div>
								</div>
							{/if}
						</div>
					</div>
					<div
						class="mesh-resize hidden xl:block"
						class:is-resizing={resizing === 'split'}
						role="separator"
						aria-orientation="vertical"
						onpointerdown={(e) => startResize(e, 'split')}
					></div>
					<div class="flex min-h-[360px] min-w-0 flex-1 flex-col">
						<div
							class="pb-1 text-xs font-semibold tracking-wide uppercase"
							style="color:{textSecondary};"
						>
							{m.mesh_graph_canvas()}
						</div>
						<div
							class="relative min-h-0 flex-1 overflow-hidden rounded-lg border"
							style="border-color:{borderColor}; background:{panelBg};"
						>
							<div bind:this={relEl} class="absolute inset-0"></div>
							{#if graphMini}
								{@const gm = graphMini}
								<div
									bind:this={relMiniEl}
									role="presentation"
									aria-label={m.mesh_minimap()}
									class="absolute right-3 bottom-3 cursor-pointer touch-none overflow-hidden rounded border shadow-lg"
									style="width:{gm.box.w * gm.scale}px; height:{gm.box.h *
										gm.scale}px; border-color:{borderColor}; background:{panelBg};"
									onpointerdown={startGraphMiniDrag}
									onpointermove={moveGraphMiniDrag}
									onpointerup={endGraphMiniDrag}
								>
									<svg
										class="absolute inset-0 h-full w-full"
										viewBox="{gm.box.x} {gm.box.y} {gm.box.w} {gm.box.h}"
									>
										{#each gm.links as l, i (i)}
											<line
												x1={l.x1}
												y1={l.y1}
												x2={l.x2}
												y2={l.y2}
												stroke={textSecondary}
												stroke-opacity="0.5"
												stroke-width={0.6 / gm.scale}
											/>
										{/each}
										{#each gm.nodes as n, i (i)}
											<circle cx={n.x} cy={n.y} r={1.8 / gm.scale} fill={n.color} />
										{/each}
										<rect
											x={gm.view.x}
											y={gm.view.y}
											width={gm.view.w}
											height={gm.view.h}
											fill="rgba(129,140,248,0.15)"
											stroke={accent}
											stroke-width={2 / gm.scale}
										/>
									</svg>
								</div>
							{/if}
							{#if !relationGraph}
								<div
									class="absolute inset-0 flex items-center justify-center p-6 text-center text-sm"
									style="color:{textSecondary};"
								>
									{entityLoading ? m.mesh_loading() : m.mesh_graph_hint()}
								</div>
							{/if}
						</div>
					</div>
				</div>
			{/if}
		{:else if mode === 'overview'}
			<h2 class="text-xl font-semibold">{m.mesh_title()}</h2>
			<p class="mt-2 max-w-3xl text-sm leading-6" style="color:{textSecondary};">
				{m.mesh_intro()}
			</p>

			{#if overviewError}
				<div class="mt-4 text-sm text-red-500">{m.mesh_error({ message: overviewError })}</div>
			{:else if !overview}
				<div class="mt-4 text-sm" style="color:{textSecondary};">{m.mesh_loading()}</div>
			{:else}
				<div class="mt-4 text-sm" style="color:{textSecondary};">
					{m.mesh_release({ release: overview.release || '—' })} · {m.mesh_total_triples({
						count: overview.source_triples.toLocaleString()
					})}
				</div>
				<div class="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
					{#each overview.kinds as k (k.kind)}
						<button
							type="button"
							class="rounded-lg border p-4 text-left transition"
							style="background:{panelBg}; border-color:{borderColor};"
							onclick={() => openKind(k.kind)}
						>
							<div class="text-sm font-semibold">{kindName(k.kind)}</div>
							<div class="mt-1 font-mono text-xs" style="color:{textSecondary};">{k.kind}</div>
							<div class="mt-2 text-2xl font-semibold" style="color:{accent};">
								{k.count.toLocaleString()}
							</div>
						</button>
					{/each}
				</div>
			{/if}
		{:else}
			<div class="flex flex-wrap items-center gap-3">
				<button
					type="button"
					class="text-sm"
					style="color:{accent};"
					onclick={() => (mode = 'overview')}
				>
					← {m.mesh_title()}
				</button>
				<h2 class="text-xl font-semibold">{kindName(kind)}</h2>
				<span class="font-mono text-xs" style="color:{textSecondary};">{kind}</span>
				<input
					type="search"
					class="ml-auto w-full rounded-md border px-3 py-1.5 text-sm sm:w-72"
					style="background:{panelBg}; border-color:{borderColor}; color:{textPrimary};"
					placeholder={m.mesh_search_placeholder()}
					value={query}
					onkeydown={(e) => e.key === 'Enter' && search(e.currentTarget.value)}
				/>
			</div>

			{#if listError}
				<div class="mt-4 text-sm text-red-500">{m.mesh_error({ message: listError })}</div>
			{:else}
				<ul
					class="mt-4 divide-y rounded-lg border"
					style="border-color:{borderColor}; background:{panelBg};"
				>
					{#each items as row (row.id)}
						<li>
							<button
								type="button"
								class="flex w-full items-baseline gap-3 px-4 py-2.5 text-left text-sm"
								style="color:{detailId === row.id ? accent : textPrimary};"
								onclick={() => openEntity(row.id)}
								onmouseenter={(e) => (e.currentTarget.style.background = hoverBg)}
								onmouseleave={(e) => (e.currentTarget.style.background = 'transparent')}
							>
								<span class="shrink-0 font-mono text-xs" style="color:{accent};">{row.id}</span>
								<span class="min-w-0 truncate">{row.label || row.id}</span>
							</button>
						</li>
					{:else}
						{#if !listLoading}
							<li class="px-4 py-3 text-sm" style="color:{textSecondary};">
								{m.mesh_no_results()}
							</li>
						{/if}
					{/each}
				</ul>
				{#if listLoading}
					<div class="mt-3 text-sm" style="color:{textSecondary};">{m.mesh_loading()}</div>
				{:else if hasMore}
					<button
						type="button"
						class="mt-3 rounded-md border px-3 py-1.5 text-sm"
						style="border-color:{borderColor}; color:{accent};"
						onclick={() => loadKind(false)}>{m.mesh_load_more()}</button
					>
				{/if}
			{/if}
		{/if}
	</section>

	<div
		class="mesh-resize"
		class:is-resizing={resizing === 'detail'}
		role="separator"
		aria-orientation="vertical"
		onpointerdown={(e) => startResize(e, 'detail')}
	></div>

	<aside
		class="mesh-detail flex min-h-0 shrink-0 flex-col overflow-hidden border-t md:border-t-0"
		style="width:{detailWidth}px; background:{panelBg}; border-color:{borderColor};"
	>
		<div
			class="px-4 py-2 text-xs font-semibold tracking-wide uppercase"
			style="color:{textSecondary};"
		>
			{m.mesh_detail_title()}
		</div>
		<div
			class="min-h-0 flex-1 overflow-y-auto px-4 pb-4"
			style="scrollbar-width:thin; scrollbar-color:{borderColor} transparent;"
		>
			{#if !detailId}
				<div class="text-sm" style="color:{textSecondary};">{m.mesh_detail_empty()}</div>
			{:else if entityLoading}
				<div class="text-sm" style="color:{textSecondary};">{m.mesh_loading()}</div>
			{:else if entityError}
				<div class="text-sm text-red-500">{m.mesh_error({ message: entityError })}</div>
			{:else if entity}
				<div class="flex flex-wrap items-baseline gap-2">
					<h2 class="text-lg font-semibold">{entity.label || entity.id}</h2>
					<span class="rounded px-2 py-0.5 text-xs" style="background:{hoverBg}; color:{accent};">
						{kindName(entity.kind)}
					</span>
				</div>
				<div class="mt-1 text-sm" style="color:{textSecondary};">
					{m.mesh_identifier()}: <span class="font-mono">{entity.id}</span>
				</div>

				<div class="mt-4">
					{@render propertyList(entity)}
				</div>

				{#if entity.filed}
					<h3 class="mt-5 text-sm font-semibold">{m.mesh_filed_under()}</h3>
					{#if entity.filed.length === 0}
						<div class="mt-2 text-sm" style="color:{textSecondary};">{m.mesh_no_results()}</div>
					{:else}
						<div class="mt-2 flex flex-col gap-2">
							{#each entity.filed as row (row.id)}
								{@const filed = filedEntity[row.id]}
								<div class="overflow-hidden rounded-lg border" style="border-color:{borderColor};">
									<button
										type="button"
										class="flex w-full items-baseline gap-2 px-3 py-2 text-left text-sm"
										aria-expanded={!!filedOpen[row.id]}
										onclick={() => toggleFiled(row.id)}
									>
										<span class="w-3 shrink-0 text-xs" style="color:{textSecondary};"
											>{filedOpen[row.id] ? '▾' : '▸'}</span
										>
										<span class="shrink-0 font-mono text-xs" style="color:{accent};">{row.id}</span>
										<span class="min-w-0 truncate">{row.label || row.id}</span>
									</button>
									{#if filedOpen[row.id]}
										<div class="border-t px-3 py-2" style="border-color:{borderColor};">
											{#if filed}
												{@render propertyList(filed)}
											{:else if filedError[row.id]}
												<div class="text-sm text-red-500">
													{m.mesh_error({ message: filedError[row.id] })}
												</div>
											{:else}
												<div class="text-sm" style="color:{textSecondary};">{m.mesh_loading()}</div>
											{/if}
										</div>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				{/if}
			{:else}
				<div class="text-sm" style="color:{textSecondary};">{m.mesh_not_found()}</div>
			{/if}
		</div>
	</aside>
</div>

<style>
	.mesh-resize {
		width: 4px;
		flex-shrink: 0;
		cursor: col-resize;
		background: transparent;
		transition: background 150ms;
		user-select: none;
		touch-action: none;
	}

	.mesh-resize:hover,
	.mesh-resize.is-resizing {
		background: rgba(129, 140, 248, 0.45);
	}

	@media (max-width: 767px) {
		.mesh-tree,
		.mesh-detail {
			width: 100% !important;
			max-height: 40vh;
		}

		.mesh-resize {
			display: none;
		}

		.mesh-main {
			min-height: 50vh;
		}
	}
</style>
