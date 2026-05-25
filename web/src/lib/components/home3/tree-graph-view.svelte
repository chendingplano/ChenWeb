<script lang="ts">
	import { onMount } from 'svelte';
	import { Chart } from 'svelte-echarts';
	import type { EChartsOption } from 'echarts';
	import { init, use } from 'echarts/core';
	import { TreeChart } from 'echarts/charts';
	import { CanvasRenderer } from 'echarts/renderers';
	import { TooltipComponent } from 'echarts/components';
	import {
		computeHoverCardPosition,
		isPointInHoverKeepAliveZone,
		toContainerLocalPoint
	} from './summary-graph-hover-position.js';
	import {
		buildPaginatedChildren,
		isVirtualPaginationId,
		nextVirtualId,
		pageVirtualId,
		parseNextVirtualId
	} from './tree-graph-pagination.js';
	import {
		getFixedTreeLayoutHeight,
		getFixedTreeLayoutWidth,
		getPanOffsetToRevealRect,
		getVisibleTreeDepth
	} from './graph-layout-utils.js';
	import SummaryCategoryTabPanel from './summary-category-tab.svelte';
	import SummaryGraphTabs from './summary-graph-tabs.svelte';
	import TopicCategoryTabPanel from './topic-category-tab.svelte';
	import TopicGraphTabs from './topic-graph-tabs.svelte';
	import SummaryNodeDialog from './summary-node-dialog.svelte';
	import ViewToolbar, { type GraphSettings } from './view-toolbar.svelte';
	import {
		getSummaryCategory,
		filterGraphNodes,
		listSummaryGraph,
		getTopicCategory,
		listTopicGraph
	} from '$lib/services/kbService';
	import {
		filterNodesInSelectedLevel,
		getNodesInSelectedLevel
	} from './tree-graph-filter-state.js';
	import type {
		SummaryCategoryNode,
		SummaryCategoryTab,
		SummaryPdfTarget,
		SummaryRecordCard
	} from './summary-types';
	import type { TopicCard, TopicCategoryTab, TopicPdfTarget } from './topic-types';

	use([TreeChart, TooltipComponent, CanvasRenderer]);

	type Mode = 'summary' | 'topic';
	type NodeStyle = 'circle' | 'rect';
	type DialogMode = 'rename' | 'metadata' | 'add' | 'delete' | 'merge' | 'split' | null;
	type LevelFilterDraft = {
		keywords: string;
		startTime: string;
		endTime: string;
		semanticText: string;
		threshold: number;
	};
	type HoverCardPos = { x: number; y: number };
	type HoverAnchor = { x: number; y: number; nodeRadius: number } | null;
	type MiniMapNode = { id: string; x: number; y: number; selected: boolean };
	type MiniMapEdge = { fromX: number; fromY: number; toX: number; toY: number };
	type MiniMapLayout = { nodes: MiniMapNode[]; edges: MiniMapEdge[] };
	type MiniMapDragStart = {
		x: number;
		y: number;
		offsetX: number;
		offsetY: number;
		boundsWidth: number;
		boundsHeight: number;
		scaleX: number;
		scaleY: number;
	} | null;
	type StageDragStart = {
		x: number;
		y: number;
		offsetX: number;
		offsetY: number;
	} | null;
	type RoamContext = {
		group: any;
		bounds: { x: number; y: number; width: number; height: number };
		stageWidth: number;
		stageHeight: number;
		scaleX: number;
		scaleY: number;
		offsetX: number;
		offsetY: number;
	};
	// SummaryCategoryTab and TopicCategoryTab have identical shapes — use a common alias
	type GraphTab = SummaryCategoryTab;

	// Unified node type that normalises both SummaryCategoryNode and TopicCategoryNode
	type GraphCategoryNode = {
		id: string;
		label: string;
		categoryPath: string;
		metadata: {
			desc: string;
			confidence: number;
			keywords: string[];
			create_time: string;
			category_type?: string;
		};
		childIds: string[];
		itemIds: string[];
		hasItemsFile: boolean;
		expanded: boolean;
		isItemNode?: boolean;
	};
	type RenderedNodeHit = {
		node: GraphCategoryNode;
		point: { x: number; y: number };
		distance: number;
		dx: number;
		dy: number;
		source: 'hovered' | 'label' | 'nearest';
	};

	const HOVER_CARD_WIDTH = 428;
	const HOVER_CARD_HEIGHT = 320;
	const HOVER_KEEP_ALIVE_BUFFER = 20;
	const HOVER_GAP_LEFT_X = -30;
	const HOVER_GAP_TOP_Y = 0;
	const HOVER_GAP_BELOW_Y = 70;
	const SVG_FALLBACK_HEIGHT = 720;
	const PARENT_CHILD_DISTANCE = 300;
	const NODE_CLICK_FALLBACK_MAX_DX = 280;
	const NODE_CLICK_FALLBACK_MAX_DY = 26;
	const NODE_CLICK_FALLBACK_ROW_DY = 36;
	const NODE_SELECTION_MARGIN = 8;

	let {
		mode,
		darkMode = true,
		heroEyebrow,
		heroTitle,
		heroDescription,
		rootTabLabel,
		loadErrorLabel,
		itemLabelPlural,
		showItemsLabel,
		showItemNodes = false,
		hideTabStrip = false,
		listGraph,
		getCategoryItems,
		onOpenCategoryTab
	}: {
		mode: Mode;
		darkMode?: boolean;
		heroEyebrow?: string;
		heroTitle?: string;
		heroDescription?: string;
		rootTabLabel?: string;
		loadErrorLabel?: string;
		itemLabelPlural?: string;
		showItemsLabel?: string;
		showItemNodes?: boolean;
		hideTabStrip?: boolean;
		listGraph?: () => Promise<{ results: any[] }>;
		getCategoryItems?: (categoryPath: string) => Promise<{ topics: TopicCard[] }>;
		onOpenCategoryTab?: (categoryPath: string) => void;
	} = $props();

	let nodeStyle = $state<NodeStyle>('rect');

	const GRAPH_SETTINGS_KEY = 'tree-graph-view-settings';
	const defaultGraphSettings: GraphSettings = { defaultExpandDepth: 6, showInfoBlock: false };

	function loadGraphSettings(): GraphSettings {
		try {
			const raw = localStorage.getItem(GRAPH_SETTINGS_KEY);
			if (raw) return { ...defaultGraphSettings, ...JSON.parse(raw) };
		} catch {}
		return { ...defaultGraphSettings };
	}

	let graphSettings = $state<GraphSettings>(loadGraphSettings());

	$effect(() => {
		localStorage.setItem(GRAPH_SETTINGS_KEY, JSON.stringify(graphSettings));
	});

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let panelAlt = $derived(darkMode ? '#0f172a' : '#eef2ff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');
	let accent = $derived(
		mode === 'summary' ? (darkMode ? '#818cf8' : '#4f46e5') : darkMode ? '#22c55e' : '#16a34a'
	);
	let warm = $derived(
		mode === 'summary' ? (darkMode ? '#fbbf24' : '#b45309') : darkMode ? '#4ade80' : '#15803d'
	);
	let lineColor = $derived(
		mode === 'summary'
			? darkMode
				? 'rgba(148, 163, 184, 0.28)'
				: 'rgba(100, 116, 139, 0.24)'
			: darkMode
				? 'rgba(34,197,94,0.28)'
				: 'rgba(22,163,74,0.24)'
	);
	let chartTheme = $derived(darkMode ? ('dark' as const) : ('light' as const));
	let hoverGapRightX = $derived(nodeStyle === 'rect' ? 95 : 150);
	let revealMargin = $derived(nodeStyle === 'rect' ? 112 : 96);
	let rootId = $derived(`${mode}-root`);

	const PAGE_SIZE = 30;
	let revealedPagesByNode = $state<Record<string, number>>({});

	// --- Graph nodes (unified type) ---
	let inspectorWidth = $state(360);
	let inspectorResizing = $state(false);
	let inspectorResizeStartX = 0;
	let inspectorResizeStartWidth = 0;

	function startInspectorResize(e: PointerEvent) {
		e.preventDefault();
		inspectorResizeStartX = e.clientX;
		inspectorResizeStartWidth = inspectorWidth;
		inspectorResizing = true;
		window.addEventListener('pointermove', onInspectorResizeMove);
		window.addEventListener('pointerup', onInspectorResizeEnd, { once: true });
	}
	function onInspectorResizeMove(e: PointerEvent) {
		inspectorWidth = Math.max(
			220,
			Math.min(600, inspectorResizeStartWidth - (e.clientX - inspectorResizeStartX))
		);
	}
	function onInspectorResizeEnd() {
		inspectorResizing = false;
		window.removeEventListener('pointermove', onInspectorResizeMove);
	}

	let nodes = $state<GraphCategoryNode[]>([]);
	let tabs = $state<GraphTab[]>(makeInitialTabs());
	let activeTabId = $state(mode === 'summary' ? 'summary-graph' : 'topic-graph');
	let selectedNodeId = $state<string | null>(null);
	let dialogOpen = $state(false);
	let dialogMode = $state<DialogMode>(null);
	let filterDialogOpen = $state(false);
	let filterDraft = $state<LevelFilterDraft>({
		keywords: '',
		startTime: '',
		endTime: '',
		semanticText: '',
		threshold: 0.6
	});
	let filterApplying = $state(false);
	let filterError = $state('');
	let filterMatchNodeIds = $state<string[]>([]);
	let filterSemanticScores = $state<Record<string, number>>({});
	let activeFilterLevel = $state<number | null>(null);
	let activeFilterSelectedPath = $state<string | null>(null);
	let loading = $state(true);
	let loadError = $state('');
	let errorDialogOpen = $state(false);
	let categoryLoadingByPath = $state<Record<string, boolean>>({});

	// Summary-mode tab content
	let categorySummaries = $state<Record<string, SummaryRecordCard[]>>({});
	let selectedSummaryIdByPath = $state<Record<string, string | null>>({});
	let selectedSummaryTargetByPath = $state<Record<string, SummaryPdfTarget | null>>({});

	// Topic-mode tab content
	let categoryTopics = $state<Record<string, TopicCard[]>>({});
	let selectedTopicIdByPath = $state<Record<string, string | null>>({});
	let selectedTopicTargetByPath = $state<Record<string, TopicPdfTarget | null>>({});

	// Interaction state
	let chartApi = $state<any>(null);
	let graphStageEl = $state<HTMLDivElement | null>(null);
	let hoverCardEl = $state<HTMLDivElement | null>(null);
	let miniMapSvgEl = $state<SVGSVGElement | null>(null);
	let hoveredNodeId = $state<string | null>(null);
	let hoverCardPos = $state<HoverCardPos>({ x: 28, y: 28 });
	let hoverAnchor = $state<HoverAnchor>(null);
	let hoverCardHovering = $state(false);
	let dismissedHoverNodeId = $state<string | null>(null);
	let hoverDismissPending = $state(false);
	let miniViewport = $state({ left: 0.08, top: 0.12, width: 0.36, height: 0.42, scale: 1 });
	let stagePointerDown = $state(false);
	let stageDragStart = $state<StageDragStart>(null);
	let miniMapDragging = $state(false);
	let miniMapDragStart = $state<MiniMapDragStart>(null);
	let miniMapDragMoved = $state(false);
	let graphCanPan = $state(false);
	let pendingRevealNodeId = $state<string | null>(null);
	let hoverHideTimer: ReturnType<typeof setTimeout> | null = null;
	let pendingZrClickFallbackTimer: ReturnType<typeof setTimeout> | null = null;
	let lastEChartsClickStamp = 0;

	onMount(() => {
		const handlePointerUp = () => {
			stopStagePan(graphCanPan ? 'grab' : 'default');
			miniMapDragging = false;
			miniMapDragStart = null;
		};
		window.addEventListener('pointerup', handlePointerUp);
		window.addEventListener('pointercancel', handlePointerUp);
		void loadGraph();
		return () => {
			window.removeEventListener('pointerup', handlePointerUp);
			window.removeEventListener('pointercancel', handlePointerUp);
		};
	});

	let selectedNode = $derived(nodes.find((n) => n.id === selectedNodeId) ?? null);
	let hoveredNode = $derived(nodes.find((n) => n.id === hoveredNodeId) ?? null);
	let activeTab = $derived(tabs.find((t) => t.id === activeTabId) ?? tabs[0]);
	let miniMapLayout = $derived.by((): MiniMapLayout => buildMiniMapLayout());
	let visibleTreeDepth = $derived(getVisibleTreeDepth(nodes));
	let hasPaginatedNodes = $derived(
		visibleRoots().length > PAGE_SIZE || nodes.some((n) => n.childIds.length > PAGE_SIZE)
	);
	let fixedTreeLayoutWidth = $derived(
		getFixedTreeLayoutWidth({
			visibleDepth: visibleTreeDepth + (hasPaginatedNodes ? 1 : 0),
			parentChildDistance: PARENT_CHILD_DISTANCE
		})
	);
	let dialogNode = $derived(selectedNode ? toDialogNode(selectedNode) : null);
	let dialogAvailableNodes = $derived(nodes.map(toDialogNode));
	let selectedLevelNodes = $derived(getNodesInSelectedLevel(nodes, selectedNodeId));
	let filterMatchNodeIdSet = $derived(new Set(filterMatchNodeIds));
	let hasActiveFilter = $derived(activeFilterLevel !== null);
	let totalItemCount = $derived.by(() => {
		const ids = new Set<string>();
		for (const node of nodes) {
			for (const id of node.itemIds ?? []) ids.add(id);
		}
		return ids.size;
	});
	let bestFilterSemanticScore = $derived.by(() => {
		const scores = Object.values(filterSemanticScores);
		return scores.length > 0 ? Math.max(...scores) : null;
	});
	let renderedRootChildren = $derived.by(() =>
		buildPaginatedChildNodes(rootId, visibleRoots().filter(shouldRenderNode))
	);
	let defaultTreeLayoutHeight = $derived(
		(graphStageEl?.clientHeight ?? SVG_FALLBACK_HEIGHT) * 0.96
	);
	let fixedTreeLayoutHeight = $derived(
		getFixedTreeLayoutHeight({
			visiblePageCount: countPaginationPageNodes(renderedRootChildren),
			pageHeight: defaultTreeLayoutHeight,
			minHeight: defaultTreeLayoutHeight
		})
	);

	// ---- Helpers ----

	function makeInitialTabs(): GraphTab[] {
		return mode === 'summary'
			? [{ id: 'summary-graph', label: 'Summary Graph', categoryPath: null, closable: false }]
			: [
					{
						id: 'topic-graph',
						label: rootTabLabel ?? 'Semantic Web',
						categoryPath: null,
						closable: false
					}
				];
	}

	function toDialogNode(node: GraphCategoryNode): SummaryCategoryNode {
		return {
			id: node.id,
			categoryPath: node.categoryPath,
			label: node.label,
			metadata: {
				desc: node.metadata.desc,
				category_type: node.metadata.category_type ?? '',
				confidence: node.metadata.confidence,
				keywords: node.metadata.keywords,
				create_time: node.metadata.create_time
			},
			childIds: node.childIds,
			summaryIds: node.itemIds,
			hasSummariesFile: node.hasItemsFile,
			expanded: node.expanded
		};
	}

	function canToggleNodeExpanded(node: GraphCategoryNode | null | undefined): boolean {
		return Boolean(node && Array.isArray(node.childIds) && node.childIds.length > 0);
	}

	function visibleRoots() {
		return nodes.filter((n) => !n.id.includes('/'));
	}

	function childrenOf(node: GraphCategoryNode): GraphCategoryNode[] {
		return node.childIds
			.map((id) => nodes.find((n) => n.id === id))
			.filter(Boolean) as GraphCategoryNode[];
	}

	function keywordText(keywords: string[] | null | undefined) {
		if (!Array.isArray(keywords) || keywords.length === 0) return '—';
		return keywords.join(', ');
	}

	function getNodeLevel(node: GraphCategoryNode) {
		return node.categoryPath.split('/').filter(Boolean).length - 1;
	}

	function clearLevelFilter() {
		filterMatchNodeIds = [];
		filterSemanticScores = {};
		activeFilterLevel = null;
		activeFilterSelectedPath = null;
	}

	function openFilterDialog() {
		if (!selectedNode) return;
		filterError = '';
		filterDialogOpen = true;
	}

	async function applyLevelFilter() {
		if (!selectedNode) return;
		filterApplying = true;
		loadError = '';
		filterError = '';
		try {
			const levelNodes = getNodesInSelectedLevel(nodes, selectedNode.id) as GraphCategoryNode[];
			let matches = new Set(
				filterNodesInSelectedLevel(nodes, selectedNode.id, {
					keywords: filterDraft.keywords,
					startTime: filterDraft.startTime,
					endTime: filterDraft.endTime
				})
			);
			const semanticText = filterDraft.semanticText.trim();
			const nextScores: Record<string, number> = {};
			if (semanticText) {
				const response = await filterGraphNodes({
					mode,
					candidatePaths: levelNodes.map((node) => node.categoryPath),
					semanticText,
					threshold: filterDraft.threshold
				});
				const semanticIds = new Set<string>();
				for (const match of response.matches) {
					const node = levelNodes.find((n) => n.categoryPath === match.categoryPath);
					if (!node) continue;
					semanticIds.add(node.id);
					nextScores[node.id] = match.score;
				}
				matches = new Set([...matches].filter((id) => semanticIds.has(id)));
			}
			filterMatchNodeIds = [...matches];
			filterSemanticScores = nextScores;
			activeFilterLevel = getNodeLevel(selectedNode);
			activeFilterSelectedPath = selectedNode.categoryPath;
			filterDialogOpen = false;
			if (filterMatchNodeIds[0]) selectedNodeId = filterMatchNodeIds[0];
		} catch (error) {
			filterError = error instanceof Error ? error.message : 'Failed to filter graph nodes';
		} finally {
			filterApplying = false;
		}
	}

	function shouldRenderNode(node: GraphCategoryNode) {
		if (!hasActiveFilter || activeFilterLevel === null) return true;
		const nodeLevel = getNodeLevel(node);
		if (nodeLevel !== activeFilterLevel) return true;
		return filterMatchNodeIdSet.has(node.id);
	}

	function filteredChildrenOf(node: GraphCategoryNode): GraphCategoryNode[] {
		return childrenOf(node).filter(shouldRenderNode);
	}

	// ---- Node state operations ----

	function toggleNodeExpanded(nodeId: string) {
		nodes = nodes.map((n) => (n.id === nodeId ? { ...n, expanded: !n.expanded } : n));
	}

	function renameNodeLocal(nodeId: string, label: string) {
		nodes = nodes.map((n) => (n.id === nodeId ? { ...n, label } : n));
	}

	function addChildNodeLocal(parentId: string, child: GraphCategoryNode) {
		nodes = [
			...nodes.map((n) => (n.id === parentId ? { ...n, childIds: [...n.childIds, child.id] } : n)),
			child
		];
	}

	function deleteNodeLocal(nodeId: string) {
		nodes = nodes
			.filter((n) => n.id !== nodeId)
			.map((n) => ({ ...n, childIds: n.childIds.filter((id) => id !== nodeId) }));
	}

	function mergeNodesLocal(sourceId: string, targetId: string) {
		const source = nodes.find((n) => n.id === sourceId);
		if (!source || sourceId === targetId) return;
		nodes = nodes
			.filter((n) => n.id !== sourceId)
			.map((n) => {
				if (n.id === targetId) {
					return {
						...n,
						childIds: [...new Set([...n.childIds, ...source.childIds])],
						itemIds: [...new Set([...n.itemIds, ...source.itemIds])]
					};
				}
				return { ...n, childIds: n.childIds.filter((id) => id !== sourceId) };
			});
	}

	function splitNodeLocal(nodeId: string, labels: string[]) {
		const target = nodes.find((n) => n.id === nodeId);
		if (!target) return;
		const clean = labels.map((l) => l.trim()).filter(Boolean);
		if (clean.length < 2) return;
		const newNodes = clean.map((label, i) => ({
			id: `${nodeId}/split-${i + 1}`,
			categoryPath: `${target.categoryPath}/${label.toLowerCase().replace(/\s+/g, '-')}`,
			label,
			metadata: {
				desc: `Split from ${target.label}`,
				confidence: target.metadata.confidence,
				keywords: [...target.metadata.keywords],
				create_time: target.metadata.create_time,
				...(target.metadata.category_type !== undefined
					? { category_type: target.metadata.category_type }
					: {})
			},
			childIds: [],
			itemIds: [],
			hasItemsFile: false,
			expanded: false
		}));
		nodes = [
			...nodes.map((n) =>
				n.id === nodeId
					? { ...n, childIds: [...n.childIds, ...newNodes.map((c) => c.id)], expanded: true }
					: n
			),
			...newNodes
		];
	}

	// ---- Data loading ----

	function createItemNode(parent: GraphCategoryNode, topic: TopicCard): GraphCategoryNode {
		const label = String(topic.topicName || topic.id || 'Provision').trim();
		return {
			id: `${parent.id}::item::${topic.id}`,
			label,
			categoryPath: `${parent.categoryPath}/${label}`,
			metadata: {
				desc: topic.topicText,
				confidence: parent.metadata.confidence,
				keywords: topic.topicKeywords ?? [],
				create_time: parent.metadata.create_time
			},
			childIds: [],
			itemIds: [topic.id],
			hasItemsFile: false,
			expanded: false,
			isItemNode: true
		};
	}

	async function expandTopicItemsAsNodes(
		baseNodes: GraphCategoryNode[]
	): Promise<GraphCategoryNode[]> {
		if (!showItemNodes || mode !== 'topic' || !getCategoryItems) return baseNodes;

		const leafCategories = baseNodes.filter(
			(node) => node.childIds.length === 0 && node.itemIds.length > 0
		);
		if (leafCategories.length === 0) return baseNodes;

		const fetched = await Promise.all(
			leafCategories.map(async (node) => ({
				node,
				response: await getCategoryItems(node.categoryPath)
			}))
		);
		const nextCategoryTopics: Record<string, TopicCard[]> = {};
		const itemNodes: GraphCategoryNode[] = [];
		const itemChildIdsByParent = new Map<string, string[]>();

		for (const { node, response } of fetched) {
			const topics = response.topics ?? [];
			nextCategoryTopics[node.categoryPath] = topics;
			if (topics[0]) {
				selectedTopicIdByPath = {
					...selectedTopicIdByPath,
					[node.categoryPath]: topics[0].id
				};
				selectedTopicTargetByPath = {
					...selectedTopicTargetByPath,
					[node.categoryPath]: {
						inputId: topics[0].inputId,
						page: topics[0].page,
						topicId: topics[0].id
					}
				};
			}

			const childNodes = topics.map((topic) => createItemNode(node, topic));
			itemNodes.push(...childNodes);
			itemChildIdsByParent.set(
				node.id,
				childNodes.map((child) => child.id)
			);
		}

		categoryTopics = { ...categoryTopics, ...nextCategoryTopics };
		const categoryNodeIds = new Set(baseNodes.map((node) => node.id));
		return [
			...baseNodes.map((node) => {
				const itemChildIds = itemChildIdsByParent.get(node.id);
				const hasCategoryChildren = node.childIds.some((childId) => categoryNodeIds.has(childId));
				const shouldExpandTopLevelCategory =
					node.categoryPath.split('/').filter(Boolean).length === 1 && hasCategoryChildren;
				if (itemChildIds) {
					return {
						...node,
						childIds: [...new Set([...node.childIds, ...itemChildIds])],
						expanded: shouldExpandTopLevelCategory
					};
				}
				if (shouldExpandTopLevelCategory) return { ...node, expanded: true };
				return node;
			}),
			...itemNodes
		];
	}

	async function loadGraph() {
		loading = true;
		loadError = '';
		try {
			if (mode === 'summary') {
				const response = await listSummaryGraph();
				nodes = response.results.map((n: any) => ({
					id: n.id,
					label: n.label,
					categoryPath: n.categoryPath,
					metadata: { ...n.metadata, keywords: n.metadata?.keywords ?? [] },
					childIds: n.childIds,
					itemIds: n.summaryIds,
					hasItemsFile: n.hasSummariesFile,
					expanded: n.expanded ?? false
				}));
			} else {
				const response = listGraph ? await listGraph() : await listTopicGraph();
				const graphNodes = response.results.map((n: any) => ({
					id: n.id,
					label: n.label,
					categoryPath: n.categoryPath,
					metadata: { ...n.metadata, keywords: n.metadata?.keywords ?? [] },
					childIds: n.childIds,
					itemIds: n.topicIds,
					hasItemsFile: n.hasTopicsFile,
					expanded: n.expanded ?? false
				}));
				nodes = await expandTopicItemsAsNodes(graphNodes);
			}
			selectedNodeId = nodes[0]?.id ?? null;
			clearLevelFilter();
			revealedPagesByNode = {};
		} catch (error) {
			nodes = [];
			selectedNodeId = null;
			loadError =
				error instanceof Error
					? error.message
					: `Failed to load ${mode === 'summary' ? 'summary' : 'topic'} graph`;
			errorDialogOpen = true;
		} finally {
			loading = false;
		}
	}

	function openCategoryTab(categoryPath: string) {
		if (onOpenCategoryTab) {
			onOpenCategoryTab(categoryPath);
			return;
		}
		const prefix = mode === 'summary' ? 'category:' : 'topic-category:';
		const nextId = `${prefix}${categoryPath}`;
		const existing = tabs.find((t) => t.id === nextId);
		if (existing) {
			activeTabId = existing.id;
			return;
		}
		tabs = [...tabs, { id: nextId, label: categoryPath, categoryPath, closable: true }];
		activeTabId = nextId;
	}

	async function showItems(node: GraphCategoryNode) {
		openCategoryTab(node.categoryPath);
		if (mode === 'summary') {
			if (categorySummaries[node.categoryPath] || categoryLoadingByPath[node.categoryPath]) return;
			categoryLoadingByPath = { ...categoryLoadingByPath, [node.categoryPath]: true };
			try {
				const response = await getSummaryCategory(node.categoryPath);
				categorySummaries = { ...categorySummaries, [node.categoryPath]: response.summaries };
				if (response.summaries[0]) {
					selectedSummaryIdByPath = {
						...selectedSummaryIdByPath,
						[node.categoryPath]: response.summaries[0].id
					};
					selectedSummaryTargetByPath = {
						...selectedSummaryTargetByPath,
						[node.categoryPath]: {
							inputId: response.summaries[0].inputId,
							page: response.summaries[0].page,
							summaryId: response.summaries[0].id
						}
					};
				} else {
					selectedSummaryIdByPath = { ...selectedSummaryIdByPath, [node.categoryPath]: null };
					selectedSummaryTargetByPath = {
						...selectedSummaryTargetByPath,
						[node.categoryPath]: null
					};
				}
			} catch (error) {
				loadError =
					error instanceof Error
						? error.message
						: `Failed to load summaries for ${node.categoryPath}`;
				errorDialogOpen = true;
			} finally {
				categoryLoadingByPath = { ...categoryLoadingByPath, [node.categoryPath]: false };
			}
		} else {
			if (categoryTopics[node.categoryPath] || categoryLoadingByPath[node.categoryPath]) return;
			categoryLoadingByPath = { ...categoryLoadingByPath, [node.categoryPath]: true };
			try {
				const response = getCategoryItems
					? await getCategoryItems(node.categoryPath)
					: await getTopicCategory(node.categoryPath);
				categoryTopics = { ...categoryTopics, [node.categoryPath]: response.topics };
				if (response.topics[0]) {
					selectedTopicIdByPath = {
						...selectedTopicIdByPath,
						[node.categoryPath]: response.topics[0].id
					};
					selectedTopicTargetByPath = {
						...selectedTopicTargetByPath,
						[node.categoryPath]: {
							inputId: response.topics[0].inputId,
							page: response.topics[0].page,
							topicId: response.topics[0].id
						}
					};
				} else {
					selectedTopicIdByPath = { ...selectedTopicIdByPath, [node.categoryPath]: null };
					selectedTopicTargetByPath = {
						...selectedTopicTargetByPath,
						[node.categoryPath]: null
					};
				}
			} catch (error) {
				loadError =
					error instanceof Error ? error.message : `Failed to load topics for ${node.categoryPath}`;
				errorDialogOpen = true;
			} finally {
				categoryLoadingByPath = { ...categoryLoadingByPath, [node.categoryPath]: false };
			}
		}
	}

	function closeTab(tabId: string) {
		tabs = tabs.filter((t) => t.id !== tabId);
		const defaultTabId = mode === 'summary' ? 'summary-graph' : 'topic-graph';
		if (activeTabId === tabId) activeTabId = defaultTabId;
	}

	function applyDialog(payload: Record<string, unknown>) {
		if (!selectedNode || !dialogMode) return;
		if (dialogMode === 'rename') {
			const label = String(payload.label ?? '').trim();
			if (!label) return;
			renameNodeLocal(selectedNode.id, label);
		}
		if (dialogMode === 'add') {
			const label = String(payload.label ?? '').trim();
			if (!label) return;
			addChildNodeLocal(selectedNode.id, {
				id: `${selectedNode.id}/${label.toLowerCase().replace(/\s+/g, '-')}`,
				categoryPath: `${selectedNode.categoryPath}/${label.toLowerCase().replace(/\s+/g, '-')}`,
				label,
				metadata: {
					desc: String(payload.desc ?? ''),
					confidence: Number(payload.confidence ?? 0.75),
					keywords: Array.isArray(payload.keywords) ? (payload.keywords as string[]) : [],
					create_time: '20260501-120000',
					...(mode === 'summary' ? { category_type: String(payload.categoryType ?? 'topic') } : {})
				},
				childIds: [],
				itemIds: [],
				hasItemsFile: false,
				expanded: false
			});
		}
		if (dialogMode === 'metadata') {
			nodes = nodes.map((n) =>
				n.id === selectedNode.id
					? {
							...n,
							metadata: {
								...n.metadata,
								desc: String(payload.desc ?? n.metadata.desc),
								confidence: Number(payload.confidence ?? n.metadata.confidence),
								keywords: Array.isArray(payload.keywords)
									? (payload.keywords as string[])
									: n.metadata.keywords,
								...(mode === 'summary'
									? {
											category_type: String(payload.categoryType ?? n.metadata.category_type ?? '')
										}
									: {})
							}
						}
					: n
			);
		}
		if (dialogMode === 'delete') {
			deleteNodeLocal(selectedNode.id);
			selectedNodeId = nodes[0]?.id ?? null;
		}
		if (dialogMode === 'merge') {
			const targetId = String(payload.mergeTargetId ?? '');
			mergeNodesLocal(selectedNode.id, targetId);
			selectedNodeId = targetId || selectedNodeId;
		}
		if (dialogMode === 'split') {
			splitNodeLocal(
				selectedNode.id,
				Array.isArray(payload.splitLabels) ? (payload.splitLabels as string[]) : []
			);
		}
	}

	function openDialog(dialogModeVal: DialogMode, nodeId: string) {
		selectedNodeId = nodeId;
		dialogMode = dialogModeVal;
		dialogOpen = true;
	}

	function runHoverAction(
		action: Exclude<DialogMode, null> | 'show-items' | 'toggle-expand',
		node: GraphCategoryNode
	) {
		selectedNodeId = node.id;
		if (action === 'show-items') {
			showItems(node);
			return;
		}
		if (action === 'toggle-expand') {
			toggleNodeAndMaybeReveal(node.id);
			return;
		}
		openDialog(action, node.id);
	}

	// ---- Hover card ----

	function clearHoverHideTimer() {
		if (hoverHideTimer) {
			clearTimeout(hoverHideTimer);
			hoverHideTimer = null;
		}
	}

	function scheduleHoverHide() {
		clearHoverHideTimer();
		hoverHideTimer = setTimeout(() => {
			if (!hoverCardHovering) {
				hoveredNodeId = null;
				hoverAnchor = null;
				hoverDismissPending = false;
			}
		}, 120);
	}

	function dismissHoverCard() {
		dismissedHoverNodeId = hoveredNodeId;
		hoverDismissPending = true;
		hoverCardHovering = false;
		scheduleHoverHide();
	}

	function shouldKeepHoverAlive(pointerX: number, pointerY: number) {
		if (dismissedHoverNodeId) return false;
		if (!hoverAnchor || !hoveredNodeId) return false;
		return isPointInHoverKeepAliveZone({
			pointX: pointerX,
			pointY: pointerY,
			nodeX: hoverAnchor.x,
			nodeY: hoverAnchor.y,
			nodeRadius: hoverAnchor.nodeRadius,
			cardX: hoverCardPos.x,
			cardY: hoverCardPos.y,
			cardWidth: HOVER_CARD_WIDTH,
			cardHeight: HOVER_CARD_HEIGHT,
			buffer: HOVER_KEEP_ALIVE_BUFFER
		});
	}

	function placeHoverCardNearNode(nodeX: number, nodeY: number, nodeRadius = 8) {
		const stageWidth = graphStageEl?.clientWidth ?? 1200;
		const stageHeight = graphStageEl?.clientHeight ?? 720;
		const cardRect = hoverCardEl?.getBoundingClientRect?.();
		const cardWidth = cardRect?.width || HOVER_CARD_WIDTH;
		const cardHeight = cardRect?.height || HOVER_CARD_HEIGHT;
		hoverAnchor = { x: nodeX, y: nodeY, nodeRadius };
		hoverCardPos = computeHoverCardPosition({
			nodeX,
			nodeY,
			nodeRadius,
			stageWidth,
			stageHeight,
			cardWidth,
			cardHeight,
			gapLeftX: HOVER_GAP_LEFT_X,
			gapRightX: hoverGapRightX,
			gapTopY: HOVER_GAP_TOP_Y,
			gapBelowY: HOVER_GAP_BELOW_Y,
			debug: true
		});
	}

	function normalizeChartPointToStage(
		globalX: number,
		globalY: number,
		referenceX?: number,
		referenceY?: number
	) {
		const stageRect = graphStageEl?.getBoundingClientRect?.();
		if (!stageRect) return { x: globalX, y: globalY };
		const globalAdjustedPoint = toContainerLocalPoint({
			globalX,
			globalY,
			containerLeft: stageRect.left,
			containerTop: stageRect.top
		});
		const directPoint = { x: globalX, y: globalY };
		const hasReference = Number.isFinite(referenceX) && Number.isFinite(referenceY);
		const distanceToReference = (point: { x: number; y: number }) =>
			Math.hypot(point.x - Number(referenceX), point.y - Number(referenceY));
		return hasReference
			? distanceToReference(globalAdjustedPoint) <= distanceToReference(directPoint)
				? globalAdjustedPoint
				: directPoint
			: globalAdjustedPoint;
	}

	function getHoveredNodePixel(
		event: any,
		referenceX?: number,
		referenceY?: number
	): { x: number; y: number } | null {
		const dataIndex = Number(event?.dataIndex);
		if (!chartApi || !Number.isFinite(dataIndex) || dataIndex < 0) return null;
		const seriesModel = chartApi.getModel?.()?.getSeriesByIndex?.(0);
		const seriesData = seriesModel?.getData?.();
		const layout = seriesData?.getItemLayout?.(dataIndex);
		const group = chartApi?._chartsViews?.[0]?.group;
		if (!layout || !group) return null;
		const localX = Number(layout?.x ?? layout?.[0]);
		const localY = Number(layout?.y ?? layout?.[1]);
		if (!Number.isFinite(localX) || !Number.isFinite(localY)) return null;
		if (typeof group.transformCoordToGlobal === 'function') {
			const global = group.transformCoordToGlobal(localX, localY);
			const point = Array.isArray(global) ? global : [global?.x, global?.y];
			const x = Number(point?.[0]);
			const y = Number(point?.[1]);
			if (Number.isFinite(x) && Number.isFinite(y)) {
				return normalizeChartPointToStage(x, y, referenceX, referenceY);
			}
		}
		const scaleX = Number(group?.scaleX ?? 1) || 1;
		const scaleY = Number(group?.scaleY ?? 1) || 1;
		const offsetX = Number(group?.x ?? 0);
		const offsetY = Number(group?.y ?? 0);
		return { x: localX * scaleX + offsetX, y: localY * scaleY + offsetY };
	}

	function updateHoverNode(event: any, source: 'mouseover' | 'mousemove' | 'click' = 'mousemove') {
		const nodeId = String(event?.data?.id ?? '');
		if (!nodeId || nodeId === rootId) return;
		const rawSize = event?.data?.symbolSize;
		const nodeRadius = Array.isArray(rawSize)
			? Number(rawSize[1]) / 2
			: Number.isFinite(Number(rawSize))
				? Number(rawSize) / 2
				: 6;
		const nativeEvent = event?.event?.event ?? event?.event ?? null;
		const nativeX = Number(nativeEvent?.offsetX);
		const nativeY = Number(nativeEvent?.offsetY);
		const nodePixel = getHoveredNodePixel(event, nativeX, nativeY);

		if (nodeId === dismissedHoverNodeId && source !== 'click') {
			if (hoverDismissPending) return;
			if (source === 'mouseover') {
				dismissedHoverNodeId = null;
			} else {
				const anchor =
					nodePixel ??
					(Number.isFinite(nativeX) && Number.isFinite(nativeY)
						? { x: nativeX, y: nativeY }
						: null);
				const pointerIsBackOnNode =
					anchor &&
					Number.isFinite(nativeX) &&
					Number.isFinite(nativeY) &&
					Math.hypot(nativeX - anchor.x, nativeY - anchor.y) <=
						nodeRadius + HOVER_KEEP_ALIVE_BUFFER;
				if (!pointerIsBackOnNode) return;
			}
		}

		dismissedHoverNodeId = null;
		hoverDismissPending = false;
		clearHoverHideTimer();
		hoveredNodeId = nodeId;
		if (nodePixel) {
			placeHoverCardNearNode(nodePixel.x, nodePixel.y, nodeRadius);
			return;
		}
		const fallbackX = Number(nativeEvent?.offsetX ?? 48);
		const fallbackY = Number(nativeEvent?.offsetY ?? 48);
		placeHoverCardNearNode(fallbackX, fallbackY, nodeRadius);
	}

	function handleChartPointerOut() {
		scheduleHoverHide();
	}

	// ---- Click handling ----

	function clearPendingZrClickFallback() {
		if (pendingZrClickFallbackTimer) {
			clearTimeout(pendingZrClickFallbackTimer);
			pendingZrClickFallbackTimer = null;
		}
	}

	function handleChartNodeClick(event: any) {
		lastEChartsClickStamp = Date.now();
		clearPendingZrClickFallback();
		const nodeId = String(event?.data?.id ?? '');
		if (!nodeId || nodeId === rootId) return;
		const nextInfo = parseNextVirtualId(nodeId);
		if (nextInfo) {
			revealedPagesByNode = { ...revealedPagesByNode, [nextInfo.parentId]: nextInfo.nextPage };
			scheduleNodeCenter(pageVirtualId(nextInfo.parentId, nextInfo.nextPage));
			return;
		}
		if (isVirtualPaginationId(nodeId)) return;
		dismissedHoverNodeId = null;
		hoverDismissPending = false;
		selectedNodeId = nodeId;
		hoveredNodeId = nodeId;
		clearHoverHideTimer();
		const clickedNode = nodes.find((n) => n.id === nodeId);
		if (canToggleNodeExpanded(clickedNode)) {
			toggleNodeAndMaybeReveal(nodeId);
		}
		updateHoverNode(event, 'click');
	}

	function showNodeItemsOnDoubleClick(nodeId: string) {
		const node = nodes.find((n) => n.id === nodeId);
		if (!node || !node.hasItemsFile) return;
		selectedNodeId = nodeId;
		hoveredNodeId = nodeId;
		dismissedHoverNodeId = null;
		hoverDismissPending = false;
		clearHoverHideTimer();
		void showItems(node);
	}

	function handleChartNodeDoubleClick(event: any) {
		clearPendingZrClickFallback();
		const nodeId = String(event?.data?.id ?? '');
		if (!nodeId || nodeId === rootId) return;
		if (isVirtualPaginationId(nodeId)) return;
		showNodeItemsOnDoubleClick(nodeId);
		updateHoverNode(event, 'click');
	}

	function getVisibleNodeIds() {
		const visibleIds = new Set<string>();
		const visit = (node: GraphCategoryNode) => {
			visibleIds.add(node.id);
			if (!node.expanded) return;
			for (const child of filteredChildrenOf(node)) visit(child);
		};
		for (const root of visibleRoots().filter(shouldRenderNode)) visit(root);
		return visibleIds;
	}

	function getRenderedNodePoint(nodeId: string): { x: number; y: number } | null {
		const seriesModel = chartApi?.getModel?.()?.getSeriesByIndex?.(0);
		const seriesData = seriesModel?.getData?.();
		const group = chartApi?._chartsViews?.[0]?.group;
		if (!seriesData || !group) return null;
		let point: { x: number; y: number } | null = null;
		seriesData.each?.((dataIndex: number) => {
			if (point) return;
			const option = seriesData.getItemModel?.(dataIndex)?.option;
			if (String(option?.id ?? '') !== nodeId) return;
			const layout = seriesData.getItemLayout?.(dataIndex);
			const localX = Number(layout?.x ?? layout?.[0]);
			const localY = Number(layout?.y ?? layout?.[1]);
			if (!Number.isFinite(localX) || !Number.isFinite(localY)) return;
			if (typeof group.transformCoordToGlobal === 'function') {
				const global = group.transformCoordToGlobal(localX, localY);
				const chartPoint = Array.isArray(global) ? global : [global?.x, global?.y];
				const x = Number(chartPoint?.[0]);
				const y = Number(chartPoint?.[1]);
				if (Number.isFinite(x) && Number.isFinite(y)) {
					point = normalizeChartPointToStage(x, y);
				}
				return;
			}
			const scaleX = Number(group?.scaleX ?? 1) || 1;
			const scaleY = Number(group?.scaleY ?? 1) || 1;
			const offsetX = Number(group?.x ?? 0);
			const offsetY = Number(group?.y ?? 0);
			point = { x: localX * scaleX + offsetX, y: localY * scaleY + offsetY };
		});
		return point;
	}

	function getNativeEventPoint(event: any): { x: number; y: number } | null {
		const nativeEvent = event?.event?.event ?? event?.event ?? event;
		const offsetX = Number(nativeEvent?.offsetX);
		const offsetY = Number(nativeEvent?.offsetY);
		if (Number.isFinite(offsetX) && Number.isFinite(offsetY)) return { x: offsetX, y: offsetY };
		const clientX = Number(nativeEvent?.clientX);
		const clientY = Number(nativeEvent?.clientY);
		const stageRect = graphStageEl?.getBoundingClientRect?.();
		if (stageRect && Number.isFinite(clientX) && Number.isFinite(clientY)) {
			return { x: clientX - stageRect.left, y: clientY - stageRect.top };
		}
		return null;
	}

	function getRenderedHitForNode(
		node: GraphCategoryNode,
		point: { x: number; y: number },
		source: RenderedNodeHit['source']
	): RenderedNodeHit | null {
		const renderedPoint = getRenderedNodePoint(node.id);
		if (!renderedPoint) return null;
		const dx = Math.abs(point.x - renderedPoint.x);
		const dy = Math.abs(point.y - renderedPoint.y);
		return { node, point: renderedPoint, distance: Math.hypot(dx, dy), dx, dy, source };
	}

	function isAcceptableRenderedNodeHit(hit: RenderedNodeHit) {
		if (hit.source === 'label' || hit.source === 'hovered') return true;
		return (
			hit.distance <= 44 ||
			hit.dy <= NODE_CLICK_FALLBACK_ROW_DY ||
			(hit.dy <= NODE_CLICK_FALLBACK_MAX_DY && hit.dx <= NODE_CLICK_FALLBACK_MAX_DX)
		);
	}

	function findRenderedNodeHit(event: any): RenderedNodeHit | null {
		const point = getNativeEventPoint(event);
		if (!point) return null;
		const targetText = String(
			event?.target?.style?.text ?? event?.topTarget?.style?.text ?? ''
		).trim();
		const visibleNodeIds = getVisibleNodeIds();
		const curHoveredNode = hoveredNodeId
			? nodes.find((n) => n.id === hoveredNodeId && visibleNodeIds.has(n.id))
			: null;
		if (curHoveredNode) {
			const hit = getRenderedHitForNode(curHoveredNode, point, 'hovered');
			if (hit && isAcceptableRenderedNodeHit(hit)) return hit;
		}
		const labelMatches = targetText
			? nodes.filter(
					(n) =>
						visibleNodeIds.has(n.id) && (n.label === targetText || n.categoryPath === targetText)
				)
			: [];
		const candidates =
			labelMatches.length > 0 ? labelMatches : nodes.filter((n) => visibleNodeIds.has(n.id));
		let best: RenderedNodeHit | null = null;
		for (const node of candidates) {
			const hit = getRenderedHitForNode(node, point, labelMatches.length > 0 ? 'label' : 'nearest');
			if (!hit) continue;
			if (!best || hit.distance < best.distance) best = hit;
		}
		if (!best) return null;
		return isAcceptableRenderedNodeHit(best) ? best : null;
	}

	function handleZrClickFallback(
		event: any,
		clickStamp: number,
		capturedPoint?: { x: number; y: number } | null
	) {
		if (lastEChartsClickStamp && Math.abs(lastEChartsClickStamp - clickStamp) < 50) return;
		const hit = findRenderedNodeHit(event);
		if (hit) {
			selectedNodeId = hit.node.id;
			hoveredNodeId = hit.node.id;
			dismissedHoverNodeId = null;
			hoverDismissPending = false;
			clearHoverHideTimer();
			if (canToggleNodeExpanded(hit.node)) {
				toggleNodeAndMaybeReveal(hit.node.id);
			}
			return;
		}
		if (!hit && hoveredNodeId && hoverAnchor && capturedPoint) {
			const dist = Math.hypot(capturedPoint.x - hoverAnchor.x, capturedPoint.y - hoverAnchor.y);
			const clickRadius = (hoverAnchor.nodeRadius || 8) + NODE_SELECTION_MARGIN + 22;
			if (dist <= clickRadius) {
				selectedNodeId = hoveredNodeId;
				hoveredNodeId = hoveredNodeId;
				dismissedHoverNodeId = null;
				hoverDismissPending = false;
				clearHoverHideTimer();
				if (canToggleNodeExpanded(hoveredNode)) {
					toggleNodeAndMaybeReveal(hoveredNodeId);
				}
			}
		}
	}

	function handleZrDoubleClickFallback(
		event: any,
		capturedPoint?: { x: number; y: number } | null
	) {
		const hit = findRenderedNodeHit(event);
		if (hit) {
			showNodeItemsOnDoubleClick(hit.node.id);
			return;
		}
		if (!hoveredNodeId || !hoverAnchor || !capturedPoint || !hoveredNode?.hasItemsFile) return;
		const dist = Math.hypot(capturedPoint.x - hoverAnchor.x, capturedPoint.y - hoverAnchor.y);
		const clickRadius = (hoverAnchor.nodeRadius || 8) + 30;
		if (dist <= clickRadius) showNodeItemsOnDoubleClick(hoveredNodeId);
	}

	function scheduleZrClickFallback(event: any) {
		clearPendingZrClickFallback();
		const clickStamp = Date.now();
		const capturedPoint = getNativeEventPoint(event);
		pendingZrClickFallbackTimer = setTimeout(() => {
			pendingZrClickFallbackTimer = null;
			handleZrClickFallback(event, clickStamp, capturedPoint);
		}, 0);
	}

	function scheduleZrDoubleClickFallback(event: any) {
		clearPendingZrClickFallback();
		const capturedPoint = getNativeEventPoint(event);
		setTimeout(() => {
			handleZrDoubleClickFallback(event, capturedPoint);
		}, 0);
	}

	// ---- Mini-map ----

	function buildMiniMapLayout(): MiniMapLayout {
		const mapped: Array<{ id: string; depth: number; row: number; selected: boolean }> = [];
		const edges: Array<{
			fromDepth: number;
			fromRow: number;
			toDepth: number;
			toRow: number;
		}> = [];
		let row = 0;
		function visit(node: GraphCategoryNode, depth: number) {
			const nodeRow = row++;
			mapped.push({ id: node.id, depth, row: nodeRow, selected: node.id === selectedNodeId });
			if (!node.expanded) return nodeRow;
			for (const child of filteredChildrenOf(node)) {
				const childRow = visit(child, depth + 1);
				edges.push({
					fromDepth: depth,
					fromRow: nodeRow,
					toDepth: depth + 1,
					toRow: childRow
				});
			}
			return nodeRow;
		}
		for (const root of visibleRoots().filter(shouldRenderNode)) visit(root, 0);
		const maxDepth = Math.max(1, ...mapped.map((n) => n.depth));
		const maxRow = Math.max(1, row - 1);
		return {
			nodes: mapped.map((n) => ({
				id: n.id,
				x: mapped.length === 1 ? 0.08 : 0.08 + (n.depth / maxDepth) * 0.84,
				y: row <= 1 ? 0.5 : 0.08 + (n.row / maxRow) * 0.84,
				selected: n.selected
			})),
			edges: edges.map((e) => ({
				fromX: 0.08 + (e.fromDepth / maxDepth) * 0.84,
				fromY: row <= 1 ? 0.5 : 0.08 + (e.fromRow / maxRow) * 0.84,
				toX: 0.08 + (e.toDepth / maxDepth) * 0.84,
				toY: row <= 1 ? 0.5 : 0.08 + (e.toRow / maxRow) * 0.84
			}))
		};
	}

	// ---- Roam / pan ----

	function getRoamContext(): RoamContext | null {
		const view = chartApi?._chartsViews?.[0];
		const group = view?.group;
		const bounds = group?.getBoundingRect?.();
		const stageWidth = graphStageEl?.clientWidth ?? 0;
		const stageHeight = graphStageEl?.clientHeight ?? 0;
		const scaleX = Math.abs(Number(group?.scaleX ?? 1)) || 1;
		const scaleY = Math.abs(Number(group?.scaleY ?? 1)) || 1;
		const offsetX = Number(group?.x ?? 0);
		const offsetY = Number(group?.y ?? 0);
		if (
			!bounds ||
			!stageWidth ||
			!stageHeight ||
			!Number.isFinite(bounds.width) ||
			bounds.width <= 0 ||
			!Number.isFinite(bounds.height) ||
			bounds.height <= 0
		) {
			return null;
		}
		return { group, bounds, stageWidth, stageHeight, scaleX, scaleY, offsetX, offsetY };
	}

	function syncMiniViewport() {
		const context = getRoamContext();
		if (!context) {
			miniViewport = { left: 0.08, top: 0.12, width: 0.36, height: 0.42, scale: 1 };
			return;
		}
		const { bounds, stageWidth, stageHeight, scaleX, scaleY, offsetX, offsetY } = context;
		const viewLeft = -offsetX / scaleX;
		const viewTop = -offsetY / scaleY;
		const viewWidth = stageWidth / scaleX;
		const viewHeight = stageHeight / scaleY;
		const left = Math.max(-1, Math.min(2, (viewLeft - bounds.x) / bounds.width));
		const top = Math.max(-1, Math.min(2, (viewTop - bounds.y) / bounds.height));
		const width = Math.max(0.1, Math.min(2.5, viewWidth / bounds.width));
		const height = Math.max(0.12, Math.min(2.5, viewHeight / bounds.height));
		miniViewport = { left, top, width, height, scale: Math.max(scaleX, scaleY) };
	}

	function moveChartToOffset(nextX: number, nextY: number) {
		const context = getRoamContext();
		if (!context) return;
		const { group, offsetX, offsetY } = context;
		group.attr({ x: nextX, y: nextY });
		group.dirty?.();
		chartApi?.dispatchAction?.({
			type: 'treeRoam',
			seriesIndex: 0,
			dx: nextX - offsetX,
			dy: nextY - offsetY
		});
		chartApi?.getZr?.()?.refreshImmediately?.();
		syncMiniViewport();
	}

	function stopStagePan(cursor: 'grab' | 'default' = 'default') {
		stagePointerDown = false;
		stageDragStart = null;
		chartApi?.getZr?.()?.setCursorStyle?.(cursor);
	}

	function startStagePan(event: PointerEvent) {
		const target = event.target as HTMLElement | null;
		if (target?.closest('.hover-card, .mini-map')) return;
		const context = getRoamContext();
		if (!context) return;
		stagePointerDown = true;
		stageDragStart = {
			x: event.clientX,
			y: event.clientY,
			offsetX: context.offsetX,
			offsetY: context.offsetY
		};
		(event.currentTarget as HTMLElement | null)?.setPointerCapture?.(event.pointerId);
		chartApi?.getZr?.()?.setCursorStyle?.('grabbing');
	}

	function dragStagePan(event: PointerEvent) {
		const target = event.target as HTMLElement | null;
		if (target?.closest('.hover-card, .mini-map')) return;
		const primaryDown = (event.buttons & 1) !== 0;
		if (!primaryDown) {
			stopStagePan(graphCanPan ? 'grab' : 'default');
			return;
		}
		if (!stagePointerDown || !stageDragStart) return;
		chartApi?.getZr?.()?.setCursorStyle?.('grabbing');
		moveChartToOffset(
			stageDragStart.offsetX + event.clientX - stageDragStart.x,
			stageDragStart.offsetY + event.clientY - stageDragStart.y
		);
	}

	function revealExpandedChildren(nodeId: string) {
		const context = getRoamContext();
		const node = nodes.find((n) => n.id === nodeId);
		if (!context || !node || !node.expanded || node.childIds.length === 0) return false;
		const childPoints = node.childIds
			.map((id) => getRenderedNodePoint(id))
			.filter(Boolean) as Array<{ x: number; y: number }>;
		if (childPoints.length === 0) return false;
		const xs = childPoints.map((p) => p.x);
		const ys = childPoints.map((p) => p.y);
		const isRect = nodeStyle === 'rect';
		const pan = getPanOffsetToRevealRect({
			rect: {
				left: Math.min(...xs) - (isRect ? 90 : 48),
				right: Math.max(...xs) + (isRect ? 190 : 160),
				top: Math.min(...ys) - (isRect ? 64 : 48),
				bottom: Math.max(...ys) + (isRect ? 64 : 48)
			},
			offsetX: context.offsetX,
			offsetY: context.offsetY,
			stageWidth: context.stageWidth,
			stageHeight: context.stageHeight,
			margin: revealMargin
		});
		if (pan.changed) moveChartToOffset(pan.x, pan.y);
		return true;
	}

	function centerRenderedNodeVertically(nodeId: string) {
		const context = getRoamContext();
		const point = getRenderedNodePoint(nodeId);
		if (!context || !point) return false;
		moveChartToOffset(context.offsetX, context.offsetY + context.stageHeight / 2 - point.y);
		return true;
	}

	function revealPendingExpandedChildren() {
		const nodeId = pendingRevealNodeId;
		if (!nodeId) return;
		pendingRevealNodeId = null;
		revealExpandedChildren(nodeId);
		syncMiniViewport();
	}

	function scheduleExpandedNodeReveal(nodeId: string) {
		pendingRevealNodeId = nodeId;
		requestAnimationFrame(() => requestAnimationFrame(revealPendingExpandedChildren));
		setTimeout(revealPendingExpandedChildren, 380);
	}

	function scheduleNodeCenter(nodeId: string) {
		requestAnimationFrame(() =>
			requestAnimationFrame(() => {
				if (!centerRenderedNodeVertically(nodeId))
					setTimeout(() => centerRenderedNodeVertically(nodeId), 180);
			})
		);
		setTimeout(() => centerRenderedNodeVertically(nodeId), 420);
	}

	function toggleNodeAndMaybeReveal(nodeId: string) {
		const node = nodes.find((n) => n.id === nodeId);
		const shouldReveal = Boolean(node && node.childIds.length > 0 && !node.expanded);
		toggleNodeExpanded(nodeId);
		if (shouldReveal) scheduleExpandedNodeReveal(nodeId);
		else setTimeout(syncMiniViewport, 0);
	}

	function collapseSelectedNode() {
		if (!selectedNode?.expanded) return;
		nodes = nodes.map((n) => (n.id === selectedNode!.id ? { ...n, expanded: false } : n));
		setTimeout(syncMiniViewport, 0);
	}

	function expandSelectedToLevel(depth: number) {
		if (!selectedNode) return;
		const basePath = selectedNode.categoryPath;
		const baseDepth = getNodeLevel(selectedNode);
		nodes = nodes.map((n) => {
			if (!n.categoryPath.startsWith(basePath)) return n;
			const relativeDepth = getNodeLevel(n) - baseDepth;
			if (relativeDepth < depth) return { ...n, expanded: true };
			return n;
		});
		scheduleExpandedNodeReveal(selectedNode.id);
	}

	function applyMiniMapViewport(normalizedCenterX: number, normalizedCenterY: number) {
		const context = getRoamContext();
		if (!context) return;
		const { bounds, scaleX, scaleY } = context;
		const clampedWidth = Math.min(0.92, Math.max(0.1, miniViewport.width));
		const clampedHeight = Math.min(0.92, Math.max(0.12, miniViewport.height));
		const nextLeft = Math.max(0, Math.min(1 - clampedWidth, normalizedCenterX - clampedWidth / 2));
		const nextTop = Math.max(0, Math.min(1 - clampedHeight, normalizedCenterY - clampedHeight / 2));
		moveChartToOffset(
			-(bounds.x + nextLeft * bounds.width) * scaleX,
			-(bounds.y + nextTop * bounds.height) * scaleY
		);
	}

	function updateMiniMapFromPointer(event: PointerEvent) {
		if (!miniMapSvgEl) return;
		const rect = miniMapSvgEl.getBoundingClientRect();
		if (!rect.width || !rect.height) return;
		applyMiniMapViewport(
			Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width)),
			Math.max(0, Math.min(1, (event.clientY - rect.top) / rect.height))
		);
	}

	function getMiniMapPointer(event: PointerEvent) {
		if (!miniMapSvgEl) return null;
		const rect = miniMapSvgEl.getBoundingClientRect();
		if (!rect.width || !rect.height) return null;
		return {
			x: Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width)),
			y: Math.max(0, Math.min(1, (event.clientY - rect.top) / rect.height))
		};
	}

	function startMiniMapDrag(event: PointerEvent) {
		const point = getMiniMapPointer(event);
		const context = getRoamContext();
		if (!point || !context) return;
		miniMapDragStart = {
			x: point.x,
			y: point.y,
			offsetX: context.offsetX,
			offsetY: context.offsetY,
			boundsWidth: context.bounds.width,
			boundsHeight: context.bounds.height,
			scaleX: context.scaleX,
			scaleY: context.scaleY
		};
	}

	function dragMiniMapViewport(event: PointerEvent) {
		const point = getMiniMapPointer(event);
		if (!point || !miniMapDragStart) return;
		const deltaX = point.x - miniMapDragStart.x;
		const deltaY = point.y - miniMapDragStart.y;
		miniMapDragMoved = miniMapDragMoved || Math.hypot(deltaX, deltaY) > 0.002;
		moveChartToOffset(
			miniMapDragStart.offsetX - deltaX * miniMapDragStart.boundsWidth * miniMapDragStart.scaleX,
			miniMapDragStart.offsetY - deltaY * miniMapDragStart.boundsHeight * miniMapDragStart.scaleY
		);
	}

	// ---- ECharts effect ----

	$effect(() => {
		if (chartApi && graphStageEl && !loadError) {
			setTimeout(syncMiniViewport, 0);
			const zr = chartApi?.getZr?.();
			if (!zr) return;
			const setChartCursor = (cursor: 'grab' | 'grabbing' | 'default') =>
				zr.setCursorStyle?.(cursor);
			const handleRoam = (event?: any) => {
				graphCanPan = !event?.target;
				setChartCursor(
					stagePointerDown || miniMapDragging ? 'grabbing' : graphCanPan ? 'grab' : 'default'
				);
				syncMiniViewport();
			};
			const handleZrClick = (event?: any) => scheduleZrClickFallback(event);
			const handleZrDoubleClick = (event?: any) => scheduleZrDoubleClickFallback(event);
			zr.on('mousewheel', handleRoam);
			zr.on('mousemove', handleRoam);
			zr.on('click', handleZrClick);
			zr.on('dblclick', handleZrDoubleClick);
			return () => {
				zr.off('mousewheel', handleRoam);
				zr.off('mousemove', handleRoam);
				zr.off('click', handleZrClick);
				zr.off('dblclick', handleZrDoubleClick);
				clearPendingZrClickFallback();
				setChartCursor('default');
			};
		}
	});

	// ---- Chart data ----

	// Rect symbol: 210×84 visual rect centered within 210×108 layout area (24px gap between siblings).
	// Ghost lines at (0,0) and (210,108) anchor the bounding box so ECharts allocates the full 108px.
	const RECT_NODE_SYMBOL =
		'path://M0,0 L0.001,0 M209.999,108 L210,108 M10,12 H200 Q210,12 210,22 V86 Q210,96 200,96 H10 Q0,96 0,86 V22 Q0,12 10,12 Z';
	const RECT_NODE_SIZE = [210, 108] as const;

	// 160×32 pill symbol — ghost anchors at (0,0)/(160,32) keep bounding box correct
	const PILL_SYMBOL =
		'path://M0,0 L0.001,0 M159.999,32 L160,32 M8,0 H152 Q160,0 160,8 V24 Q160,32 152,32 H8 Q0,32 0,24 V8 Q0,0 8,0 Z';
	const PILL_SIZE = [160, 32] as const;

	function countPaginationPageNodes(treeNodes: Record<string, unknown>[]): number {
		let count = 0;
		const visit = (node: Record<string, unknown>) => {
			if (String(node.id ?? '').startsWith('|pg|')) count += 1;
			const children = Array.isArray(node.children) ? node.children : [];
			for (const child of children) {
				if (child && typeof child === 'object') visit(child as Record<string, unknown>);
			}
		};
		for (const node of treeNodes) visit(node);
		return Math.max(1, count);
	}

	function buildNextVirtualNode(parentId: string, pageNum: number): Record<string, unknown> {
		return {
			id: nextVirtualId(parentId, pageNum),
			name: 'next …',
			collapsed: false,
			symbol: PILL_SYMBOL,
			symbolSize: PILL_SIZE,
			itemStyle: {
				color: darkMode ? 'rgba(251,191,36,0.12)' : 'rgba(180,83,9,0.08)',
				borderColor: warm,
				borderWidth: 1.5
			},
			label: {
				show: true,
				position: 'inside',
				verticalAlign: 'middle',
				align: 'center',
				color: warm,
				fontSize: 11,
				fontWeight: '700'
			},
			children: []
		};
	}

	function buildPageVirtualNode(
		parentId: string,
		pageNum: number,
		totalPages: number,
		pageChildren: Record<string, unknown>[]
	): Record<string, unknown> {
		return {
			id: pageVirtualId(parentId, pageNum),
			name: `page ${pageNum} of ${totalPages}`,
			collapsed: false,
			symbol: PILL_SYMBOL,
			symbolSize: PILL_SIZE,
			itemStyle: {
				color: darkMode ? 'rgba(30,41,59,0.65)' : 'rgba(226,232,240,0.65)',
				borderColor: accent,
				borderWidth: 1
			},
			label: {
				show: true,
				position: 'inside',
				verticalAlign: 'middle',
				align: 'center',
				color: textMuted,
				fontSize: 10,
				fontStyle: 'italic'
			},
			children: pageChildren
		};
	}

	function buildPaginatedChildNodes(
		parentId: string,
		allChildren: GraphCategoryNode[]
	): Record<string, unknown>[] {
		return buildPaginatedChildren({
			parentId,
			allChildren,
			pageSize: PAGE_SIZE,
			revealedPages: revealedPagesByNode[parentId] ?? 1,
			buildChildNode: buildTreeNode,
			buildPageNode: ({ parentId, pageNum, totalPages, pageChildren }) =>
				buildPageVirtualNode(parentId, pageNum, totalPages, pageChildren),
			buildNextNode: ({ parentId, pageNum }) => buildNextVirtualNode(parentId, pageNum)
		});
	}

	function buildTreeNode(node: GraphCategoryNode): Record<string, unknown> {
		const isSelected = node.id === selectedNodeId;
		const isFilterMatch = hasActiveFilter && filterMatchNodeIdSet.has(node.id);
		if (nodeStyle === 'rect') {
			const conf = node.metadata.confidence ? `Conf: ${node.metadata.confidence.toFixed(2)}` : '—';

			// Split keywords across up to 2 lines. L2_MAX: chars for the keywords portion on the conf line.
			// L3_MAX: chars for the second keywords-only line before truncation.
			const allKws = (node.metadata.keywords ?? []).map(
				(k) => k.charAt(0).toUpperCase() + k.slice(1)
			);
			const L2_MAX = 10;
			const L3_MAX = 15;
			let kws1: string;
			let kws2: string | null = null;
			if (allKws.length === 0) {
				kws1 = '—';
			} else {
				const all = allKws.join(', ');
				if (all.length <= L2_MAX) {
					kws1 = all;
				} else {
					const fit: string[] = [];
					const rest: string[] = [];
					let done = false;
					for (const kw of allKws) {
						if (!done) {
							const candidate = fit.length ? fit.join(', ') + ', ' + kw : kw;
							if (candidate.length <= L2_MAX) fit.push(kw);
							else {
								done = true;
								rest.push(kw);
							}
						} else {
							rest.push(kw);
						}
					}
					kws1 = fit.length ? fit.join(', ') : allKws[0].slice(0, L2_MAX - 3) + '...';
					if (rest.length > 0) {
						const raw3 = rest.join(', ');
						kws2 = raw3.length > L3_MAX ? raw3.slice(0, L3_MAX - 3) + '...' : raw3;
					}
				}
			}

			return {
				id: node.id,
				name: node.label,
				value: node.itemIds.length,
				categoryPath: node.categoryPath,
				desc: node.metadata.desc,
				confidence: node.metadata.confidence,
				keywords: node.metadata.keywords,
				topicCount: node.itemIds.length,
				collapsed: !node.expanded,
				symbol: RECT_NODE_SYMBOL,
				symbolSize: RECT_NODE_SIZE,
				itemStyle: {
					color: isSelected
						? darkMode
							? `rgba(${mode === 'summary' ? '79,70,229' : '13,148,136'},0.7)`
							: `rgba(${mode === 'summary' ? '99,102,241' : '22,163,74'},0.15)`
						: darkMode
							? 'rgba(15,23,42,0.85)'
							: 'rgba(241,245,249,0.9)',
					borderColor: isSelected || isFilterMatch ? warm : accent,
					borderWidth: isSelected || isFilterMatch ? 2 : 1,
					shadowBlur: isSelected || isFilterMatch ? 18 : 0,
					shadowColor:
						isSelected || isFilterMatch
							? mode === 'summary'
								? 'rgba(129,140,248,0.5)'
								: 'rgba(34,197,94,0.5)'
							: 'transparent'
				},
				label: {
					show: true,
					position: 'inside',
					color: isSelected ? '#ffffff' : textMain,
					formatter: (params: any) => {
						const raw = String(params.data?.name ?? '');
						const displayName = raw
							.split('_')
							.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
							.join(' ');
						const name = displayName.length > 20 ? displayName.slice(0, 17) + '...' : displayName;
						const kwsLine = kws2
							? `{meta|${conf}  ${kws1}}\n{meta|${kws2}}`
							: `{meta|${conf}  ${kws1}}`;
						return `{label|${name}}\n${kwsLine}`;
					},
					rich: {
						label: {
							fontSize: 12,
							fontWeight: '700',
							lineHeight: 18,
							color: isSelected ? '#ffffff' : textMain
						},
						meta: {
							fontSize: 10,
							lineHeight: 14,
							color: isSelected ? 'rgba(255,255,255,0.7)' : textMuted
						}
					}
				},
				children: buildPaginatedChildNodes(node.id, filteredChildrenOf(node))
			};
		}

		// circle style
		return {
			id: node.id,
			name: node.label,
			value: node.itemIds.length,
			categoryPath: node.categoryPath,
			desc: node.metadata.desc,
			confidence: node.metadata.confidence,
			keywords: node.metadata.keywords,
			summaryCount: node.itemIds.length,
			collapsed: !node.expanded,
			symbolSize: isSelected || isFilterMatch ? 16 : 11,
			itemStyle: {
				color: isSelected || isFilterMatch ? accent : darkMode ? '#cbd5e1' : '#94a3b8',
				borderColor: isSelected || isFilterMatch ? warm : accent,
				borderWidth: isSelected || isFilterMatch ? 4 : 2,
				shadowBlur: isSelected || isFilterMatch ? 18 : 0,
				shadowColor:
					isSelected || isFilterMatch
						? mode === 'summary'
							? 'rgba(129, 140, 248, 0.72)'
							: 'rgba(34, 197, 94, 0.72)'
						: 'transparent'
			},
			label: {
				color: isSelected || isFilterMatch ? textMain : textMuted,
				fontWeight: isSelected || isFilterMatch ? 700 : 500,
				backgroundColor:
					isSelected || isFilterMatch
						? darkMode
							? mode === 'summary'
								? 'rgba(99, 102, 241, 0.16)'
								: 'rgba(22, 163, 74, 0.16)'
							: mode === 'summary'
								? 'rgba(79, 70, 229, 0.12)'
								: 'rgba(22, 163, 74, 0.12)'
						: 'transparent',
				padding: isSelected || isFilterMatch ? [4, 8] : 0,
				borderRadius: isSelected || isFilterMatch ? 999 : 0
			},
			children: buildPaginatedChildNodes(node.id, filteredChildrenOf(node))
		};
	}

	let treeOption = $derived.by((): EChartsOption => {
		const isRect = nodeStyle === 'rect';
		const root = {
			id: rootId,
			name: '',
			symbol: isRect ? 'rect' : 'emptyCircle',
			symbolSize: isRect ? [1, 1] : 1,
			itemStyle: { color: 'transparent', borderColor: 'transparent' },
			label: { show: false },
			lineStyle: { color: lineColor, width: 2, curveness: 0.55 },
			children: renderedRootChildren
		};
		return {
			backgroundColor: 'transparent',
			animationDuration: 300,
			animationDurationUpdate: 350,
			tooltip: { show: false },
			series: [
				{
					type: 'tree',
					data: [root],
					top: '2%',
					left: '2%',
					bottom: '2%',
					width: fixedTreeLayoutWidth,
					height: fixedTreeLayoutHeight,
					layout: 'orthogonal',
					orient: 'LR',
					symbol: isRect ? RECT_NODE_SYMBOL : 'emptyCircle',
					symbolSize: isRect ? RECT_NODE_SIZE : 11,
					edgeShape: 'curve',
					expandAndCollapse: false,
					initialTreeDepth: -1,
					lineStyle: { color: lineColor, width: 2, curveness: 0.55 },
					label: {
						position: isRect ? 'inside' : 'left',
						verticalAlign: 'middle',
						align: isRect ? 'center' : 'right',
						fontSize: isRect ? 12 : 13
					},
					leaves: {
						label: {
							position: isRect ? 'inside' : 'right',
							align: isRect ? 'center' : 'left',
							color: textMain
						}
					},
					emphasis: { focus: 'descendant' },
					roam: false
				} as any
			]
		};
	});
</script>

<div
	class="graph-shell"
	style={`--panel:${panelBg}; --panel-alt:${panelAlt}; --border:${border}; --text:${textMain}; --muted:${textMuted}; --accent:${accent}; --warm:${warm};`}
>
	<SummaryNodeDialog
		bind:open={dialogOpen}
		mode={dialogMode}
		node={dialogNode}
		availableNodes={dialogAvailableNodes}
		onConfirm={applyDialog}
	/>

	{#if errorDialogOpen && loadError}
		<div
			class="error-overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => (errorDialogOpen = false)}
			onkeydown={(event) => {
				if (event.key === 'Escape') errorDialogOpen = false;
			}}
		>
			<div
				class="error-dialog"
				role="dialog"
				aria-modal="true"
				aria-label="Graph Load Error"
				tabindex="0"
				onclick={(event) => event.stopPropagation()}
				onkeydown={(event) => event.stopPropagation()}
			>
				<div class="eyebrow">Load Error</div>
				<h3>
					Could not load {mode === 'summary' ? 'Summary Graph' : (loadErrorLabel ?? 'Semantic Web')}
				</h3>
				<p class="dialog-copy">{loadError}</p>
				<div class="dialog-actions">
					<button type="button" class="secondary-btn" onclick={() => (errorDialogOpen = false)}>
						Close
					</button>
					<button
						type="button"
						class="primary-btn"
						onclick={async () => {
							errorDialogOpen = false;
							await loadGraph();
						}}
					>
						Try Again
					</button>
				</div>
			</div>
		</div>
	{/if}

	{#if filterDialogOpen && selectedNode}
		<div
			class="modal-overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => {
				if (!filterApplying) filterDialogOpen = false;
			}}
			onkeydown={(event) => {
				if (event.key === 'Escape' && !filterApplying) filterDialogOpen = false;
			}}
		>
			<div
				class="filter-dialog"
				role="dialog"
				aria-modal="true"
				aria-label="Filter Nodes in Current Level"
				tabindex="0"
				onclick={(event) => event.stopPropagation()}
				onkeydown={(event) => event.stopPropagation()}
			>
				<form
					onsubmit={(event) => {
						event.preventDefault();
						void applyLevelFilter();
					}}
				>
					<div class="eyebrow">Current Level</div>
					<h3>Filter Nodes</h3>
					<p class="dialog-copy">
						Level {getNodeLevel(selectedNode) + 1} from <strong>{selectedNode.categoryPath}</strong>
						contains {selectedLevelNodes.length} nodes.
					</p>
					{#if filterError}
						<div class="filter-error" role="alert">{filterError}</div>
					{/if}
					<label class="field">
						<span>Keywords</span>
						<input
							type="text"
							bind:value={filterDraft.keywords}
							placeholder="keyword, phrase, another"
						/>
					</label>
					<div class="filter-grid">
						<label class="field">
							<span>Created After</span>
							<input type="text" bind:value={filterDraft.startTime} placeholder="YYYYMMDD-HHMMSS" />
						</label>
						<label class="field">
							<span>Created Before</span>
							<input type="text" bind:value={filterDraft.endTime} placeholder="YYYYMMDD-HHMMSS" />
						</label>
					</div>
					<label class="field">
						<span>Semantic Text</span>
						<textarea
							rows="3"
							bind:value={filterDraft.semanticText}
							placeholder="Text to embed and compare against current-level node vectors"
						></textarea>
					</label>
					<label class="field">
						<span>Semantic Threshold {filterDraft.threshold.toFixed(2)}</span>
						<input type="range" min="0" max="1" step="0.01" bind:value={filterDraft.threshold} />
					</label>
					<div class="dialog-actions">
						<button
							type="button"
							class="secondary-btn"
							disabled={filterApplying}
							onclick={() => (filterDialogOpen = false)}
						>
							Cancel
						</button>
						<button type="submit" class="primary-btn" disabled={filterApplying}>
							{filterApplying ? 'Filtering…' : 'Apply Filter'}
						</button>
					</div>
				</form>
			</div>
		</div>
	{/if}

	<div class="hero">
		<div>
			<div class="eyebrow">
				{heroEyebrow ?? (mode === 'summary' ? 'Document Summaries' : 'Semantic Web')}
			</div>
			<h2>{heroTitle ?? (mode === 'summary' ? 'Summary Graph' : 'Topic Graph')}</h2>
			<p>
				{heroDescription ??
					(mode === 'summary'
						? 'Category-first workspace for browsing, editing, and opening category-path summary tabs.'
						: 'Category-first workspace for browsing topics indexed in the Semantic Web.')}
			</p>
		</div>
		<div class="hero-stats">
			<div><span>Nodes</span><strong>{nodes.length}</strong></div>
			<div><span>Tabs</span><strong>{tabs.length}</strong></div>
			<div>
				<span>{mode === 'summary' ? 'Mode' : 'Topics'}</span>
				<strong>
					{mode === 'summary' ? 'Phase 1 Mock' : totalItemCount}
				</strong>
			</div>
		</div>
	</div>

	<ViewToolbar
		{darkMode}
		{nodeStyle}
		filterDisabled={!selectedNode}
		onFilter={openFilterDialog}
		resetFilterDisabled={!hasActiveFilter}
		onResetFilter={clearLevelFilter}
		collapseSelectedDisabled={!selectedNode?.expanded}
		onCollapseSelected={collapseSelectedNode}
		onExpandToLevel={expandSelectedToLevel}
		onToggleNodeStyle={() => {
			nodeStyle = nodeStyle === 'circle' ? 'rect' : 'circle';
		}}
		settings={graphSettings}
		onSettingsChange={(patch) => {
			graphSettings = { ...graphSettings, ...patch };
		}}
	/>

	{#if hasActiveFilter}
		<div class="filter-status" role="status">
			<span>
				Filtered level {activeFilterLevel !== null ? activeFilterLevel + 1 : '—'} from
				<strong>{activeFilterSelectedPath}</strong>: {filterMatchNodeIds.length} match{filterMatchNodeIds.length ===
				1
					? ''
					: 'es'}
				{#if bestFilterSemanticScore !== null}
					, best semantic score {bestFilterSemanticScore.toFixed(2)}
				{/if}
			</span>
			<button type="button" class="text-btn" onclick={clearLevelFilter}>Clear</button>
		</div>
	{/if}

	<div class="tabbed-window">
		{#if !hideTabStrip}
			<div class="tabbed-window-head">
				{#if mode === 'summary'}
					<SummaryGraphTabs
						{tabs}
						{activeTabId}
						onSelect={(tabId) => (activeTabId = tabId)}
						onClose={closeTab}
					/>
				{:else}
					<TopicGraphTabs
						tabs={tabs as TopicCategoryTab[]}
						{activeTabId}
						onSelect={(tabId) => (activeTabId = tabId)}
						onClose={closeTab}
					/>
				{/if}
			</div>
		{/if}

		<div class="tabbed-window-body">
			{#if activeTab.categoryPath}
				{#if mode === 'summary'}
					<SummaryCategoryTabPanel
						categoryPath={activeTab.categoryPath}
						summaries={categorySummaries[activeTab.categoryPath] ?? []}
						selectedSummaryId={selectedSummaryIdByPath[activeTab.categoryPath] ?? null}
						selectedTarget={selectedSummaryTargetByPath[activeTab.categoryPath] ?? null}
						onSelectSummary={(summary) => {
							const path = activeTab.categoryPath!;
							selectedSummaryIdByPath = { ...selectedSummaryIdByPath, [path]: summary.id };
							selectedSummaryTargetByPath = {
								...selectedSummaryTargetByPath,
								[path]: { inputId: summary.inputId, page: summary.page, summaryId: summary.id }
							};
						}}
					/>
				{:else}
					<TopicCategoryTabPanel
						categoryPath={activeTab.categoryPath}
						topics={categoryTopics[activeTab.categoryPath] ?? []}
						selectedTopicId={selectedTopicIdByPath[activeTab.categoryPath] ?? null}
						selectedTarget={selectedTopicTargetByPath[activeTab.categoryPath] ?? null}
						onSelectTopic={(topic) => {
							const path = activeTab.categoryPath!;
							selectedTopicIdByPath = { ...selectedTopicIdByPath, [path]: topic.id };
							selectedTopicTargetByPath = {
								...selectedTopicTargetByPath,
								[path]: { inputId: topic.inputId, page: topic.page, topicId: topic.id }
							};
						}}
					/>
				{/if}
			{:else}
				<div class="graph-workspace">
					<div
						class="graph-stage"
						style="flex:1; min-width:0;"
						class:can-pan={graphCanPan}
						class:is-panning={stagePointerDown && !miniMapDragging}
						role="presentation"
						bind:this={graphStageEl}
						onpointerenter={() => {
							graphCanPan = true;
							chartApi?.getZr?.()?.setCursorStyle?.('grab');
						}}
						onpointerleave={() => {
							graphCanPan = false;
							stopStagePan('default');
						}}
						onpointerdown={startStagePan}
						onpointermove={dragStagePan}
						onmousemove={(event: MouseEvent) => {
							if (shouldKeepHoverAlive(event.offsetX, event.offsetY)) {
								clearHoverHideTimer();
							}
							if (!stagePointerDown && !miniMapDragging && hoverAnchor && hoveredNodeId) {
								const dist = Math.hypot(
									event.offsetX - hoverAnchor.x,
									event.offsetY - hoverAnchor.y
								);
								if (dist <= hoverAnchor.nodeRadius + NODE_SELECTION_MARGIN) {
									chartApi?.getZr?.()?.setCursorStyle?.('default');
								}
							}
						}}
					>
						{#if loading}
							<div class="empty-state">
								Loading {mode === 'summary' ? 'summary' : 'topic'} categories…
							</div>
						{:else if loadError}
							<div class="empty-state">
								{mode === 'summary' ? 'Summary Graph' : (loadErrorLabel ?? 'Semantic Web')} could not
								be loaded. Open the error dialog for details or try again.
							</div>
						{:else}
							<div class="chart-cursor-host">
								<Chart
									bind:chart={chartApi}
									{init}
									theme={chartTheme}
									options={treeOption}
									style="width:100%; height:100%;"
									onrendered={() => {
										syncMiniViewport();
										revealPendingExpandedChildren();
									}}
									onfinished={() => {
										syncMiniViewport();
										revealPendingExpandedChildren();
									}}
									onmouseover={(event: any) => updateHoverNode(event, 'mouseover')}
									onmousemove={(event: any) => {
										if (event?.data?.id) updateHoverNode(event, 'mousemove');
									}}
									onmouseout={handleChartPointerOut}
									onglobalout={handleChartPointerOut}
									onclick={handleChartNodeClick}
									ondblclick={handleChartNodeDoubleClick}
								/>
							</div>

							{#if hoveredNode && graphSettings.showInfoBlock}
								<div
									bind:this={hoverCardEl}
									class="hover-card"
									role="presentation"
									style={`left:${hoverCardPos.x}px; top:${hoverCardPos.y}px;`}
									onmouseenter={() => {
										hoverCardHovering = true;
										dismissedHoverNodeId = null;
										hoverDismissPending = false;
										clearHoverHideTimer();
									}}
									onmouseleave={dismissHoverCard}
									onclick={(event: MouseEvent) => {
										if ((event.target as Element).closest('button')) return;
										if (hoveredNode && canToggleNodeExpanded(hoveredNode)) {
											toggleNodeAndMaybeReveal(hoveredNode.id);
										}
									}}
								>
									<div class="hover-card-head">
										<div>
											<div class="hover-card-title">Name</div>
											<div class="hover-card-value strong">{hoveredNode.label}</div>
										</div>
										<div
											class:active={hoveredNode.id === selectedNodeId}
											class="hover-selected-pill"
										>
											{hoveredNode.id === selectedNodeId ? 'Selected' : 'Hover'}
										</div>
									</div>
									<div class="hover-card-row">
										<span>Path</span>
										<strong>{hoveredNode.categoryPath}</strong>
									</div>
									<div class="hover-card-row">
										<span>Description</span>
										<strong>{hoveredNode.metadata.desc || '—'}</strong>
									</div>
									<div class="hover-card-row">
										<span>Keywords</span>
										<strong>{keywordText(hoveredNode.metadata.keywords)}</strong>
									</div>
									<div class="hover-card-row compact">
										<div>
											<span>{mode === 'summary' ? 'Summaries' : 'Topics'}</span>
											<strong>{hoveredNode.itemIds.length}</strong>
										</div>
										<div>
											<span>Children</span>
											<strong>{hoveredNode.childIds.length}</strong>
										</div>
									</div>
									<div class="hover-toolbar">
										<button
											type="button"
											class="toolbar-btn primary"
											disabled={!hoveredNode.hasItemsFile}
											title={hoveredNode.hasItemsFile
												? undefined
												: `No ${mode === 'summary' ? 'summaries' : 'topics'}.txt file for this category`}
											onclick={() => runHoverAction('show-items', hoveredNode)}
										>
											{mode === 'summary' ? 'Show Summaries' : (showItemsLabel ?? 'Show Topics')}
										</button>
										<button
											type="button"
											class="toolbar-btn"
											onclick={() => runHoverAction('toggle-expand', hoveredNode)}
										>
											{hoveredNode.expanded ? 'Collapse' : 'Expand'}
										</button>
										<button
											type="button"
											class="toolbar-btn"
											onclick={() => runHoverAction('rename', hoveredNode)}>Rename</button
										>
										<button
											type="button"
											class="toolbar-btn"
											onclick={() => runHoverAction('metadata', hoveredNode)}>Metadata</button
										>
										<button
											type="button"
											class="toolbar-btn"
											onclick={() => runHoverAction('add', hoveredNode)}>Add</button
										>
										<button
											type="button"
											class="toolbar-btn"
											onclick={() => runHoverAction('merge', hoveredNode)}>Merge</button
										>
										<button
											type="button"
											class="toolbar-btn"
											onclick={() => runHoverAction('split', hoveredNode)}>Split</button
										>
										<button
											type="button"
											class="toolbar-btn danger"
											onclick={() => runHoverAction('delete', hoveredNode)}>Delete</button
										>
									</div>
								</div>
							{/if}

							<div class="mini-map">
								<div class="mini-map-head">
									<span>Zoom Window</span>
									<strong>{miniViewport.scale.toFixed(2)}x</strong>
								</div>
								<svg
									bind:this={miniMapSvgEl}
									viewBox="0 0 100 100"
									class="mini-map-svg"
									class:is-dragging={miniMapDragging}
									aria-hidden="true"
									onpointerdown={(event: PointerEvent) => {
										miniMapDragging = true;
										stagePointerDown = false;
										(event.currentTarget as SVGSVGElement).setPointerCapture?.(event.pointerId);
										event.preventDefault();
										event.stopPropagation();
										miniMapDragMoved = false;
										startMiniMapDrag(event);
									}}
									onpointermove={(event: PointerEvent) => {
										if (!miniMapDragging) return;
										event.preventDefault();
										event.stopPropagation();
										dragMiniMapViewport(event);
									}}
									onpointerup={(event: PointerEvent) => {
										miniMapDragging = false;
										miniMapDragStart = null;
										(event.currentTarget as SVGSVGElement).releasePointerCapture?.(event.pointerId);
									}}
									onclick={(event: MouseEvent) => {
										if (miniMapDragMoved) {
											miniMapDragMoved = false;
											return;
										}
										updateMiniMapFromPointer(event as PointerEvent);
									}}
								>
									{#each miniMapLayout.edges as edge}
										<line
											x1={edge.fromX * 100}
											y1={edge.fromY * 100}
											x2={edge.toX * 100}
											y2={edge.toY * 100}
											class="mini-map-edge"
										/>
									{/each}
									{#each miniMapLayout.nodes as mmNode}
										{#if nodeStyle === 'rect'}
											<rect
												x={mmNode.x * 100 - 3}
												y={mmNode.y * 100 - 1.5}
												width="6"
												height="3"
												rx="1"
												class:selected={mmNode.selected}
												class="mini-map-node"
											/>
										{:else}
											<circle
												cx={mmNode.x * 100}
												cy={mmNode.y * 100}
												r={mmNode.selected ? 2.6 : 1.8}
												class:selected={mmNode.selected}
												class="mini-map-node"
											/>
										{/if}
									{/each}
									<rect
										x={miniViewport.left * 100}
										y={miniViewport.top * 100}
										width={miniViewport.width * 100}
										height={miniViewport.height * 100}
										class="mini-map-viewport"
									/>
								</svg>
							</div>
						{/if}
					</div>

					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div
						class="inspector-resize-handle"
						class:is-resizing={inspectorResizing}
						onpointerdown={startInspectorResize}
					></div>

					<div class="inspector" style="width:{inspectorWidth}px; flex-shrink:0;">
						<div class="eyebrow">Node Inspector</div>
						{#if selectedNode}
							<div class="inspector-card">
								<div class="hover-card-head inspector-card-head">
									<div>
										<div class="hover-card-title">Name</div>
										<div class="hover-card-value strong">{selectedNode.label}</div>
									</div>
									<div class="hover-selected-pill active">Selected</div>
								</div>
								<div class="hover-card-row">
									<span>Path</span>
									<strong>{selectedNode.categoryPath}</strong>
								</div>
								<div class="hover-card-row">
									<span>Description</span>
									<strong>{selectedNode.metadata.desc || '—'}</strong>
								</div>
								<div class="hover-card-row">
									<span>Keywords</span>
									<strong>{keywordText(selectedNode.metadata.keywords)}</strong>
								</div>
								<div class="inspector-grid">
									<div class="inspector-stat">
										<span>Confidence</span>
										<strong>{selectedNode.metadata.confidence}</strong>
									</div>
									<div class="inspector-stat">
										<span>Children</span>
										<strong>{selectedNode.childIds.length}</strong>
									</div>
									<div class="inspector-stat">
										<span>{mode === 'summary' ? 'Summaries' : (itemLabelPlural ?? 'Topics')}</span>
										<strong>{selectedNode.itemIds.length}</strong>
									</div>
									{#if mode === 'summary' && selectedNode.metadata.category_type}
										<div class="inspector-stat">
											<span>Category Type</span>
											<strong>{selectedNode.metadata.category_type}</strong>
										</div>
									{/if}
									<div class="inspector-stat inspector-stat-wide">
										<span>Category Path</span>
										<strong>{selectedNode.categoryPath}</strong>
									</div>
									<div class="inspector-stat inspector-stat-wide">
										<span>Create Time</span>
										<strong>{selectedNode.metadata.create_time || '—'}</strong>
									</div>
								</div>
								<div class="action-grid action-grid-top">
									<button type="button" onclick={() => toggleNodeAndMaybeReveal(selectedNode.id)}>
										{selectedNode.expanded ? 'Collapse' : 'Expand'}
									</button>
									<button
										type="button"
										class="accent-action"
										disabled={!selectedNode.hasItemsFile}
										title={selectedNode.hasItemsFile
											? undefined
											: `No ${mode === 'summary' ? 'summaries' : 'topics'}.txt file for this category`}
										onclick={() => showItems(selectedNode)}
									>
										{mode === 'summary' ? 'Show Summaries' : (showItemsLabel ?? 'Show Topics')}
									</button>
								</div>
								<div class="action-grid">
									<button type="button" onclick={() => openDialog('rename', selectedNode.id)}>
										Rename
									</button>
									<button type="button" onclick={() => openDialog('metadata', selectedNode.id)}>
										Edit Metadata
									</button>
									<button type="button" onclick={() => openDialog('add', selectedNode.id)}>
										Add Node
									</button>
									<button type="button" onclick={() => openDialog('merge', selectedNode.id)}>
										Merge
									</button>
									<button type="button" onclick={() => openDialog('split', selectedNode.id)}>
										Split
									</button>
									<button
										type="button"
										class="danger"
										onclick={() => openDialog('delete', selectedNode.id)}>Delete</button
									>
								</div>
							</div>
						{:else}
							<div class="empty-state">Select a category node to inspect and edit it.</div>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.graph-shell {
		position: relative;
		display: flex;
		height: 100%;
		flex-direction: column;
		color: var(--text);
	}

	.error-overlay,
	.modal-overlay {
		position: absolute;
		inset: 0;
		z-index: 24;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1.5rem;
		background: rgba(2, 6, 23, 0.62);
		backdrop-filter: blur(10px);
	}

	.error-dialog {
		width: min(540px, 100%);
		border-radius: 22px;
		border: 1px solid rgba(248, 113, 113, 0.28);
		background: #111827;
		padding: 1.25rem;
		box-shadow: 0 30px 80px rgba(15, 23, 42, 0.5);
	}

	.filter-dialog {
		width: min(620px, 100%);
		border-radius: 22px;
		border: 1px solid rgba(var(--accent-rgb, 129, 140, 248), 0.24);
		background: #111827;
		padding: 1.25rem;
		box-shadow: 0 30px 80px rgba(15, 23, 42, 0.5);
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.38rem;
		margin-top: 0.8rem;
	}

	.field span {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--muted);
	}

	.field input,
	.field textarea {
		width: 100%;
		border-radius: 12px;
		border: 1px solid rgba(148, 163, 184, 0.18);
		background: rgba(15, 23, 42, 0.68);
		color: var(--text);
		padding: 0.68rem 0.75rem;
		font: inherit;
	}

	.field textarea {
		resize: vertical;
		min-height: 88px;
	}

	.field input[type='range'] {
		padding: 0;
		accent-color: var(--accent);
	}

	.filter-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.75rem;
	}

	.filter-error {
		margin-top: 0.8rem;
		border: 1px solid rgba(248, 113, 113, 0.34);
		border-radius: 12px;
		background: rgba(127, 29, 29, 0.28);
		color: #fecaca;
		padding: 0.68rem 0.75rem;
		font-size: 0.82rem;
		line-height: 1.45;
		overflow-wrap: anywhere;
	}

	.dialog-copy {
		margin-top: 0.55rem;
		color: var(--muted);
	}

	.dialog-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.primary-btn,
	.secondary-btn {
		border-radius: 12px;
		padding: 0.72rem 1rem;
		border: 1px solid rgba(148, 163, 184, 0.18);
		cursor: pointer;
	}

	.primary-btn {
		background: var(--accent);
		color: #fff;
	}

	.secondary-btn {
		background: rgba(15, 23, 42, 0.55);
		color: var(--text);
	}

	.primary-btn:disabled,
	.secondary-btn:disabled {
		opacity: 0.48;
		cursor: not-allowed;
	}

	.filter-status {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		margin: -0.25rem 0 0.75rem;
		border: 1px solid rgba(var(--accent-rgb, 129, 140, 248), 0.2);
		border-radius: 14px;
		background: rgba(var(--accent-rgb, 99, 102, 241), 0.12);
		padding: 0.62rem 0.75rem;
		font-size: 0.82rem;
		color: var(--text);
	}

	.filter-status span {
		min-width: 0;
		overflow-wrap: anywhere;
	}

	.filter-status strong {
		margin-inline: 0.25rem;
	}

	.text-btn {
		border: none;
		background: transparent;
		color: var(--accent);
		font-weight: 700;
		cursor: pointer;
	}

	.hero {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
		padding: 1.1rem 1.2rem;
		border-radius: 24px;
		background:
			radial-gradient(
				circle at top left,
				rgba(var(--accent-rgb, 129, 140, 248), 0.18),
				transparent 42%
			),
			linear-gradient(180deg, rgba(15, 23, 42, 0.86), rgba(15, 23, 42, 0.66));
		border: 1px solid var(--border);
	}

	.eyebrow {
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
	}

	h2,
	h3,
	p {
		margin: 0;
	}

	h2 {
		margin-top: 0.3rem;
		font-size: 1.5rem;
	}

	.hero p {
		margin-top: 0.45rem;
		max-width: 52rem;
		color: var(--muted);
	}

	.hero-stats {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.75rem;
	}

	.hero-stats div {
		min-width: 110px;
		border-radius: 16px;
		border: 1px solid rgba(148, 163, 184, 0.12);
		background: rgba(30, 41, 59, 0.36);
		padding: 0.75rem 0.9rem;
	}

	.hero-stats span {
		display: block;
		font-size: 0.72rem;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.hero-stats strong {
		display: block;
		margin-top: 0.2rem;
		font-size: 1rem;
	}

	.tabbed-window {
		display: flex;
		min-height: 0;
		flex: 1;
		flex-direction: column;
	}

	.tabbed-window-head {
		position: relative;
		z-index: 4;
		margin-bottom: -1px;
		padding-inline: 0.55rem;
	}

	.tabbed-window-body {
		display: flex;
		min-height: 0;
		flex: 1;
		flex-direction: column;
		border: 1px solid var(--border);
		border-radius: 0 24px 24px 24px;
		background: linear-gradient(180deg, rgba(15, 23, 42, 0.72), rgba(15, 23, 42, 0.58));
		box-shadow:
			inset 0 1px 0 rgba(255, 255, 255, 0.04),
			0 18px 44px rgba(2, 6, 23, 0.18);
		overflow: hidden;
	}

	.graph-workspace {
		display: flex;
		min-height: 0;
		flex: 1;
	}

	.graph-stage,
	.inspector {
		min-height: 0;
		background: transparent;
	}

	.graph-stage {
		flex: 1;
		min-width: 0;
		position: relative;
		overflow: hidden;
		padding: 0.85rem 0.75rem 0.75rem;
		cursor: default;
	}

	.inspector-resize-handle {
		width: 4px;
		flex-shrink: 0;
		cursor: col-resize;
		background: transparent;
		transition: background 150ms;
		user-select: none;
		touch-action: none;
		border-left: 1px solid var(--border);
		margin-right: -1px;
	}

	.inspector-resize-handle:hover,
	.inspector-resize-handle.is-resizing {
		background: rgba(129, 140, 248, 0.45);
	}

	.graph-stage.can-pan {
		cursor: grab;
	}

	.graph-stage.is-panning {
		cursor: grabbing;
	}

	.chart-cursor-host {
		width: 100%;
		height: 100%;
	}

	.graph-stage.can-pan .chart-cursor-host :global(*) {
		cursor: grab !important;
	}

	.graph-stage.is-panning .chart-cursor-host :global(*) {
		cursor: grabbing !important;
	}

	.hover-card {
		position: absolute;
		z-index: 12;
		width: min(428px, calc(100% - 2rem));
		max-width: calc(100% - 2rem);
		border-radius: 18px;
		border: 1px solid rgba(var(--accent-rgb, 129, 140, 248), 0.2);
		background: rgba(15, 23, 42, 0.94);
		box-shadow: 0 20px 48px rgba(2, 6, 23, 0.38);
		backdrop-filter: blur(12px);
		padding: 0.9rem;
	}

	.hover-card-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
		margin-bottom: 0.7rem;
	}

	.hover-card-head > div:first-child,
	.hover-card-row > strong,
	.hover-card-row.compact > div {
		min-width: 0;
	}

	.hover-card-title,
	.hover-card-row span,
	.mini-map-head span {
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
	}

	.hover-card-value,
	.hover-card-row strong {
		display: block;
		margin-top: 0.2rem;
		font-size: 0.84rem;
		line-height: 1.4;
		max-width: 100%;
		overflow-wrap: anywhere;
		word-break: break-word;
		white-space: normal;
	}

	.hover-card-value.strong {
		font-size: 0.98rem;
		font-weight: 700;
		color: var(--text);
	}

	.hover-selected-pill {
		border-radius: 999px;
		border: 1px solid rgba(148, 163, 184, 0.18);
		padding: 0.3rem 0.6rem;
		font-size: 0.7rem;
		color: var(--muted);
	}

	.hover-selected-pill.active {
		border-color: rgba(var(--accent-rgb, 129, 140, 248), 0.36);
		background: rgba(var(--accent-rgb, 99, 102, 241), 0.18);
		color: #e0e7ff;
	}

	.hover-card-row {
		margin-bottom: 0.65rem;
	}

	.hover-card-row.compact {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.75rem;
	}

	.hover-toolbar {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 0.5rem;
		margin-top: 0.35rem;
	}

	.toolbar-btn {
		border-radius: 10px;
		border: 1px solid rgba(148, 163, 184, 0.18);
		background: rgba(15, 23, 42, 0.7);
		padding: 0.48rem 0.5rem;
		font-size: 0.73rem;
		color: var(--text);
		cursor: pointer;
	}

	.toolbar-btn.primary {
		border-color: rgba(var(--accent-rgb, 129, 140, 248), 0.32);
		background: rgba(var(--accent-rgb, 99, 102, 241), 0.18);
		color: #e0e7ff;
	}

	.toolbar-btn:disabled,
	.accent-action:disabled {
		opacity: 0.38;
		cursor: not-allowed;
	}

	.toolbar-btn.danger {
		border-color: rgba(248, 113, 113, 0.28);
		color: #fca5a5;
	}

	.mini-map {
		position: absolute;
		right: 1rem;
		bottom: 1rem;
		z-index: 10;
		width: 220px;
		border-radius: 18px;
		border: 1px solid rgba(148, 163, 184, 0.18);
		background: rgba(15, 23, 42, 0.92);
		box-shadow: 0 18px 42px rgba(2, 6, 23, 0.28);
		padding: 0.75rem;
		backdrop-filter: blur(10px);
	}

	.mini-map-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 0.5rem;
	}

	.mini-map-head strong {
		font-size: 0.78rem;
		color: var(--text);
	}

	.mini-map-svg {
		display: block;
		width: 100%;
		height: 120px;
		border-radius: 12px;
		cursor: grab;
		touch-action: none;
		background:
			radial-gradient(circle at top left, rgba(129, 140, 248, 0.12), transparent 48%),
			rgba(2, 6, 23, 0.34);
	}

	.mini-map-svg.is-dragging {
		cursor: grabbing;
	}

	.mini-map-edge {
		stroke: rgba(148, 163, 184, 0.32);
		stroke-width: 1.2;
	}

	.mini-map-node {
		fill: rgba(226, 232, 240, 0.86);
	}

	.mini-map-node.selected {
		fill: var(--accent);
	}

	.mini-map-viewport {
		fill: rgba(129, 140, 248, 0.08);
		stroke: rgba(129, 140, 248, 0.92);
		stroke-width: 1.2;
		rx: 4;
	}

	.empty-state {
		display: flex;
		min-height: 220px;
		align-items: center;
		justify-content: center;
		border-radius: 18px;
		border: 1px dashed rgba(148, 163, 184, 0.2);
		background: rgba(15, 23, 42, 0.2);
		color: var(--muted);
		font-size: 0.84rem;
		text-align: center;
		padding: 1rem;
	}

	.action-grid button {
		border-radius: 12px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: transparent;
		padding: 0.55rem 0.8rem;
		color: inherit;
		cursor: pointer;
	}

	.inspector {
		flex-shrink: 0;
		border-left: 1px solid rgba(148, 163, 184, 0.12);
		padding: 1rem;
		background: rgba(2, 6, 23, 0.16);
		overflow: auto;
	}

	.inspector-card {
		border-radius: 22px;
		border: 1px solid rgba(var(--accent-rgb, 129, 140, 248), 0.16);
		background: rgba(15, 23, 42, 0.7);
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
		padding: 1rem;
	}

	.inspector-card-head {
		margin-bottom: 0.95rem;
	}

	.inspector-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.75rem;
		margin: 1rem 0;
	}

	.inspector-stat {
		border-radius: 16px;
		background: var(--panel-alt);
		padding: 0.8rem;
		min-width: 0;
	}

	.inspector-stat-wide {
		grid-column: 1 / -1;
	}

	.inspector-stat span {
		display: block;
		margin-bottom: 0.2rem;
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--muted);
	}

	.inspector-stat strong {
		font-size: 0.92rem;
		display: block;
		max-width: 100%;
		overflow-wrap: anywhere;
		word-break: break-word;
		white-space: normal;
	}

	.action-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.75rem;
	}

	.action-grid-top {
		margin-bottom: 0.75rem;
	}

	.accent-action {
		background: rgba(var(--accent-rgb, 99, 102, 241), 0.14);
		color: #c7d2fe;
		border-color: rgba(var(--accent-rgb, 129, 140, 248), 0.3);
	}

	.action-grid .danger {
		border-color: rgba(248, 113, 113, 0.28);
		color: #fca5a5;
	}

	@media (max-width: 980px) {
		.graph-workspace {
			flex-direction: column;
		}

		.inspector {
			width: 100% !important;
			flex-shrink: 1;
		}

		.inspector-resize-handle {
			display: none;
		}

		.mini-map {
			width: 180px;
		}

		.hover-toolbar {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}

		.inspector-grid {
			grid-template-columns: 1fr;
		}

		.filter-grid {
			grid-template-columns: 1fr;
		}

		.filter-status {
			align-items: flex-start;
			flex-direction: column;
		}
	}
</style>
