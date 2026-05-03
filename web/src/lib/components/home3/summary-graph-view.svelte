<script lang="ts">
	import { onMount } from 'svelte';
	import { Chart } from 'svelte-echarts';
	import type { EChartsOption } from 'echarts';
	import { init, use } from 'echarts/core';
	import { TreeChart } from 'echarts/charts';
	import { CanvasRenderer } from 'echarts/renderers';
	import { TooltipComponent } from 'echarts/components';
	import {
		addChildNode,
		createSummaryGraphTabs,
		deleteNode,
		mergeNodes,
		openCategorySummaryTab,
		renameNode,
		splitNode,
		toggleNodeExpanded
	} from './summary-graph-state.js';
	import {
		computeHoverCardPosition,
		isPointInHoverKeepAliveZone,
		toContainerLocalPoint
	} from './summary-graph-hover-position.js';
	import SummaryCategoryTab from './summary-category-tab.svelte';
	import SummaryGraphTabs from './summary-graph-tabs.svelte';
	import SummaryNodeDialog from './summary-node-dialog.svelte';
	import { getSummaryCategory, listSummaryGraph } from '$lib/services/kbService';
	import type {
		SummaryCategoryNode,
		SummaryCategoryTab as SummaryCategoryTabType,
		SummaryPdfTarget,
		SummaryRecordCard
	} from './summary-types';

	use([TreeChart, TooltipComponent, CanvasRenderer]);

	type DialogMode = 'rename' | 'metadata' | 'add' | 'delete' | 'merge' | 'split' | null;
	type HoverCardPos = { x: number; y: number };
	type HoverAnchor = { x: number; y: number; nodeRadius: number } | null;
	type MiniMapNode = { id: string; x: number; y: number; selected: boolean };
	type MiniMapEdge = { fromX: number; fromY: number; toX: number; toY: number };
	type MiniMapLayout = { nodes: MiniMapNode[]; edges: MiniMapEdge[] };

	const HOVER_CARD_WIDTH = 428;
	const HOVER_CARD_HEIGHT = 320;
	const HOVER_KEEP_ALIVE_BUFFER = 20;
	const SUMMARY_HOVER_GAP_LEFT_X = -30;
	const SUMMARY_HOVER_GAP_RIGHT_X = 150;
	const SUMMARY_HOVER_GAP_TOP_Y = 0;
	const SUMMARY_HOVER_GAP_BELOW_Y = 70;

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let panelAlt = $derived(darkMode ? '#0f172a' : '#eef2ff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');
	let accent = $derived(darkMode ? '#818cf8' : '#4f46e5');
	let warm = $derived(darkMode ? '#fbbf24' : '#b45309');
	let chartTheme = $derived(darkMode ? ('dark' as const) : ('light' as const));

	let nodes = $state<SummaryCategoryNode[]>([]);
	let tabs = $state<SummaryCategoryTabType[]>(createSummaryGraphTabs());
	let activeTabId = $state('summary-graph');
	let selectedNodeId = $state<string | null>(null);
	let dialogOpen = $state(false);
	let dialogMode = $state<DialogMode>(null);
	let categorySummaries = $state<Record<string, SummaryRecordCard[]>>({});
	let selectedSummaryIdByPath = $state<Record<string, string | null>>({});
	let selectedTargetByPath = $state<Record<string, SummaryPdfTarget | null>>({});
	let loading = $state(true);
	let loadError = $state('');
	let errorDialogOpen = $state(false);
	let categoryLoadingByPath = $state<Record<string, boolean>>({});
	let chartApi = $state<any>(null);
	let graphStageEl = $state<HTMLDivElement | null>(null);
	let hoverCardEl = $state<HTMLDivElement | null>(null);
	let hoveredNodeId = $state<string | null>(null);
	let hoverCardPos = $state<HoverCardPos>({ x: 28, y: 28 });
	let hoverAnchor = $state<HoverAnchor>(null);
	let hoverCardHovering = $state(false);
	let miniViewport = $state({ left: 0.08, top: 0.12, width: 0.36, height: 0.42, scale: 1 });
	let hoverHideTimer: ReturnType<typeof setTimeout> | null = null;

	onMount(async () => {
		await loadGraph();
	});

	let selectedNode = $derived(nodes.find((node) => node.id === selectedNodeId) ?? null);
	let hoveredNode = $derived(nodes.find((node) => node.id === hoveredNodeId) ?? null);
	let activeTab = $derived(tabs.find((tab) => tab.id === activeTabId) ?? tabs[0]);
	let miniMapLayout = $derived.by((): MiniMapLayout => buildMiniMapLayout());

	function openDialog(mode: DialogMode, nodeId: string) {
		selectedNodeId = nodeId;
		dialogMode = mode;
		dialogOpen = true;
	}

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
			}
		}, 120);
	}

	function shouldKeepHoverAlive(pointerX: number, pointerY: number) {
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

	async function loadGraph() {
		loading = true;
		loadError = '';
		try {
			const response = await listSummaryGraph();
			nodes = response.results;
			selectedNodeId = response.results[0]?.id ?? null;
		} catch (error) {
			nodes = [];
			selectedNodeId = null;
			loadError = error instanceof Error ? error.message : 'Failed to load summary graph';
			errorDialogOpen = true;
		} finally {
			loading = false;
		}
	}

	async function showSummaries(node: SummaryCategoryNode) {
		const result = openCategorySummaryTab(tabs, node.categoryPath);
		tabs = result.tabs;
		activeTabId = result.activeTabId;
		if (categorySummaries[node.categoryPath] || categoryLoadingByPath[node.categoryPath]) {
			return;
		}

		categoryLoadingByPath = { ...categoryLoadingByPath, [node.categoryPath]: true };
		try {
			const response = await getSummaryCategory(node.categoryPath);
			categorySummaries = { ...categorySummaries, [node.categoryPath]: response.summaries };
			if (response.summaries[0]) {
				selectedSummaryIdByPath = {
					...selectedSummaryIdByPath,
					[node.categoryPath]: response.summaries[0].id
				};
				selectedTargetByPath = {
					...selectedTargetByPath,
					[node.categoryPath]: {
						inputId: response.summaries[0].inputId,
						page: response.summaries[0].page,
						summaryId: response.summaries[0].id
					}
				};
			} else {
				selectedSummaryIdByPath = {
					...selectedSummaryIdByPath,
					[node.categoryPath]: null
				};
				selectedTargetByPath = {
					...selectedTargetByPath,
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
	}

	function closeTab(tabId: string) {
		tabs = tabs.filter((tab) => tab.id !== tabId);
		if (activeTabId === tabId) activeTabId = 'summary-graph';
	}

	function applyDialog(payload: Record<string, unknown>) {
		if (!selectedNode || !dialogMode) return;
		if (dialogMode === 'rename' || dialogMode === 'add') {
			const label = String(payload.label ?? '').trim();
			if (!label) return;
			if (dialogMode === 'rename') {
				nodes = renameNode(nodes, selectedNode.id, label);
			} else {
				nodes = addChildNode(nodes, selectedNode.id, {
					id: `${selectedNode.id}/${label.toLowerCase().replace(/\s+/g, '-')}`,
					categoryPath: `${selectedNode.categoryPath}/${label.toLowerCase().replace(/\s+/g, '-')}`,
					label,
					metadata: {
						desc: String(payload.desc ?? 'Mock category created in phase 1.'),
						category_type: String(payload.categoryType ?? 'topic'),
						confidence: Number(payload.confidence ?? 0.75),
						keywords: Array.isArray(payload.keywords) ? (payload.keywords as string[]) : [],
						create_time: '20260501-120000'
					},
					childIds: [],
					summaryIds: [],
					hasSummariesFile: false,
					expanded: false
				});
			}
		}
		if (dialogMode === 'metadata') {
			nodes = nodes.map((node) =>
				node.id === selectedNode.id
					? {
							...node,
							metadata: {
								...node.metadata,
								desc: String(payload.desc ?? node.metadata.desc),
								category_type: String(payload.categoryType ?? node.metadata.category_type),
								confidence: Number(payload.confidence ?? node.metadata.confidence),
								keywords: Array.isArray(payload.keywords)
									? (payload.keywords as string[])
									: node.metadata.keywords
							}
						}
					: node
			);
		}
		if (dialogMode === 'delete') {
			nodes = deleteNode(nodes, selectedNode.id);
			selectedNodeId = nodes[0]?.id ?? null;
		}
		if (dialogMode === 'merge') {
			const targetId = String(payload.mergeTargetId ?? '');
			nodes = mergeNodes(nodes, selectedNode.id, targetId);
			selectedNodeId = targetId || selectedNodeId;
		}
		if (dialogMode === 'split') {
			nodes = splitNode(
				nodes,
				selectedNode.id,
				Array.isArray(payload.splitLabels) ? (payload.splitLabels as string[]) : []
			);
		}
	}

	function selectSummary(categoryPath: string, summary: SummaryRecordCard) {
		selectedSummaryIdByPath = { ...selectedSummaryIdByPath, [categoryPath]: summary.id };
		selectedTargetByPath = {
			...selectedTargetByPath,
			[categoryPath]: {
				inputId: summary.inputId,
				page: summary.page,
				summaryId: summary.id
			}
		};
	}

	function keywordText(keywords: string[] | null | undefined) {
		if (!Array.isArray(keywords) || keywords.length === 0) return '—';
		return keywords.join(', ');
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
			gapLeftX: SUMMARY_HOVER_GAP_LEFT_X,
			gapRightX: SUMMARY_HOVER_GAP_RIGHT_X,
			gapTopY: SUMMARY_HOVER_GAP_TOP_Y,
			gapBelowY: SUMMARY_HOVER_GAP_BELOW_Y,
			debug: true
		});
		console.debug('[summary-hover-placeHoverCardNearNode]', {
			nodeX,
			nodeY,
			nodeRadius,
			stageWidth,
			stageHeight,
			cardWidth,
			cardHeight,
			hoverCardPos
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
		const localPoint = hasReference
			? distanceToReference(globalAdjustedPoint) <= distanceToReference(directPoint)
				? globalAdjustedPoint
				: directPoint
			: globalAdjustedPoint;

		console.debug('[summary-hover-coordinate]', {
			globalX,
			globalY,
			stageRect: {
				left: stageRect.left,
				top: stageRect.top,
				width: stageRect.width,
				height: stageRect.height
			},
			reference: hasReference ? { x: referenceX, y: referenceY } : null,
			candidates: {
				globalAdjustedPoint,
				directPoint
			},
			chosenSource: localPoint === globalAdjustedPoint ? 'global-adjusted' : 'direct-local',
			localPoint
		});
		return localPoint;
	}

	function getHoveredNodePixel(
		event: any,
		referenceX?: number,
		referenceY?: number
	): { x: number; y: number } | null {
		const dataIndex = Number(event?.dataIndex);
		if (!chartApi || !Number.isFinite(dataIndex) || dataIndex < 0) return null;

		const seriesModel = chartApi.getModel?.().getSeriesByIndex?.(0);
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
		return {
			x: localX * scaleX + offsetX,
			y: localY * scaleY + offsetY
		};
	}

	function updateHoverNode(event: any) {
		const nodeId = String(event?.data?.id ?? '');
		if (!nodeId || nodeId === 'summary-root') return;
		clearHoverHideTimer();
		hoveredNodeId = nodeId;
		const rawSize = Number(event?.data?.symbolSize ?? 11);
		const nodeRadius = Number.isFinite(rawSize) ? rawSize / 2 : 6;
		const nativeEvent = event?.event?.event ?? event?.event ?? null;
		const nativeX = Number(nativeEvent?.offsetX);
		const nativeY = Number(nativeEvent?.offsetY);
		const nodePixel = getHoveredNodePixel(event, nativeX, nativeY);
		if (nodePixel) {
			console.debug('[summary-hover-anchor]', {
				source: 'chart-layout',
				nodeId,
				nodePixel,
				nodeRadius,
				referencePointer:
					Number.isFinite(nativeX) && Number.isFinite(nativeY) ? { x: nativeX, y: nativeY } : null
			});
			placeHoverCardNearNode(nodePixel.x, nodePixel.y, nodeRadius);
			return;
		}
		const fallbackX = Number(nativeEvent?.offsetX ?? 48);
		const fallbackY = Number(nativeEvent?.offsetY ?? 48);
		const pointerAnchorRadius = Number.isFinite(rawSize) ? rawSize : nodeRadius;
		console.debug('[summary-hover-anchor]', {
			source: 'fallback-native-offset',
			nodeId,
			fallbackX,
			fallbackY,
			nodeRadius,
			pointerAnchorRadius
		});
		placeHoverCardNearNode(fallbackX, fallbackY, pointerAnchorRadius);
	}

	function buildMiniMapLayout(): MiniMapLayout {
		const mappedNodes: Array<{ id: string; depth: number; row: number; selected: boolean }> = [];
		const edges: Array<{ fromDepth: number; fromRow: number; toDepth: number; toRow: number }> = [];
		let row = 0;

		function visit(node: SummaryCategoryNode, depth: number) {
			const nodeRow = row++;
			mappedNodes.push({
				id: node.id,
				depth,
				row: nodeRow,
				selected: node.id === selectedNodeId
			});
			if (!node.expanded) return nodeRow;
			for (const child of childrenOf(node)) {
				const childRow = visit(child, depth + 1);
				edges.push({ fromDepth: depth, fromRow: nodeRow, toDepth: depth + 1, toRow: childRow });
			}
			return nodeRow;
		}

		for (const root of visibleRoots()) visit(root, 0);

		const maxDepth = Math.max(1, ...mappedNodes.map((node) => node.depth));
		const maxRow = Math.max(1, row - 1);

		return {
			nodes: mappedNodes.map((node) => ({
				id: node.id,
				x: mappedNodes.length === 1 ? 0.08 : 0.08 + (node.depth / maxDepth) * 0.84,
				y: row <= 1 ? 0.5 : 0.08 + (node.row / maxRow) * 0.84,
				selected: node.selected
			})),
			edges: edges.map((edge) => ({
				fromX: 0.08 + (edge.fromDepth / maxDepth) * 0.84,
				fromY: row <= 1 ? 0.5 : 0.08 + (edge.fromRow / maxRow) * 0.84,
				toX: 0.08 + (edge.toDepth / maxDepth) * 0.84,
				toY: row <= 1 ? 0.5 : 0.08 + (edge.toRow / maxRow) * 0.84
			}))
		};
	}

	function syncMiniViewport() {
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
			miniViewport = { left: 0.08, top: 0.12, width: 0.36, height: 0.42, scale: 1 };
			return;
		}

		const viewLeft = -offsetX / scaleX;
		const viewTop = -offsetY / scaleY;
		const viewWidth = stageWidth / scaleX;
		const viewHeight = stageHeight / scaleY;
		const left = Math.max(0, Math.min(1, (viewLeft - bounds.x) / bounds.width));
		const top = Math.max(0, Math.min(1, (viewTop - bounds.y) / bounds.height));
		const width = Math.max(0.1, Math.min(1 - left, viewWidth / bounds.width));
		const height = Math.max(0.12, Math.min(1 - top, viewHeight / bounds.height));

		miniViewport = { left, top, width, height, scale: Math.max(scaleX, scaleY) };
	}

	function runHoverAction(
		action: Exclude<DialogMode, null> | 'show-summaries' | 'toggle-expand',
		node: SummaryCategoryNode
	) {
		selectedNodeId = node.id;
		if (action === 'show-summaries') {
			showSummaries(node);
			return;
		}
		if (action === 'toggle-expand') {
			nodes = toggleNodeExpanded(nodes, node.id);
			setTimeout(syncMiniViewport, 0);
			return;
		}
		openDialog(action, node.id);
	}

	$effect(() => {
		if (chartApi && graphStageEl && !loadError) {
			setTimeout(syncMiniViewport, 0);
		}
	});

	function visibleRoots() {
		return nodes.filter((node) => !node.id.includes('/'));
	}

	function childrenOf(node: SummaryCategoryNode) {
		return node.childIds
			.map((childId) => nodes.find((candidate) => candidate.id === childId))
			.filter(Boolean) as SummaryCategoryNode[];
	}

	function buildTreeNode(node: SummaryCategoryNode): Record<string, unknown> {
		const isSelected = node.id === selectedNodeId;
		return {
			id: node.id,
			name: node.label,
			value: node.summaryIds.length,
			categoryPath: node.categoryPath,
			desc: node.metadata.desc,
			categoryType: node.metadata.category_type,
			confidence: node.metadata.confidence,
			keywords: node.metadata.keywords,
			summaryCount: node.summaryIds.length,
			collapsed: !node.expanded,
			symbolSize: isSelected ? 16 : 11,
			itemStyle: {
				color: isSelected ? accent : darkMode ? '#cbd5e1' : '#94a3b8',
				borderColor: isSelected ? warm : accent,
				borderWidth: isSelected ? 4 : 2,
				shadowBlur: isSelected ? 18 : 0,
				shadowColor: isSelected ? 'rgba(129, 140, 248, 0.72)' : 'transparent'
			},
			label: {
				color: isSelected ? textMain : textMuted,
				fontWeight: isSelected ? 700 : 500,
				backgroundColor: isSelected
					? darkMode
						? 'rgba(99, 102, 241, 0.16)'
						: 'rgba(79, 70, 229, 0.12)'
					: 'transparent',
				padding: isSelected ? [4, 8] : 0,
				borderRadius: isSelected ? 999 : 0
			},
			children: childrenOf(node).map(buildTreeNode)
		};
	}

	let treeOption = $derived.by((): EChartsOption => {
		const root = {
			id: 'summary-root',
			name: '',
			symbolSize: 1,
			itemStyle: {
				color: 'transparent',
				borderColor: 'transparent'
			},
			label: {
				show: false
			},
			lineStyle: {
				color: darkMode ? 'rgba(148, 163, 184, 0.28)' : 'rgba(100, 116, 139, 0.24)',
				width: 2,
				curveness: 0.55
			},
			children: visibleRoots().map(buildTreeNode)
		};

		return {
			backgroundColor: 'transparent',
			animationDuration: 300,
			animationDurationUpdate: 350,
			tooltip: {
				show: false
			},
			series: [
				{
					type: 'tree',
					data: [root],
					top: '2%',
					left: '2%',
					bottom: '2%',
					right: '16%',
					layout: 'orthogonal',
					orient: 'LR',
					layerPadding: 72,
					nodePadding: 26,
					symbol: 'emptyCircle',
					edgeShape: 'curve',
					expandAndCollapse: false,
					initialTreeDepth: -1,
					lineStyle: {
						color: darkMode ? 'rgba(148, 163, 184, 0.28)' : 'rgba(100, 116, 139, 0.24)',
						width: 2,
						curveness: 0.55
					},
					label: {
						position: 'left',
						verticalAlign: 'middle',
						align: 'right',
						fontSize: 13
					},
					leaves: {
						label: {
							position: 'right',
							align: 'left',
							color: textMain
						}
					},
					emphasis: {
						focus: 'descendant'
					},
					roam: true
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
		node={selectedNode}
		availableNodes={nodes}
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
				aria-label="Summary Graph Load Error"
				tabindex="0"
				onclick={(event) => event.stopPropagation()}
				onkeydown={(event) => event.stopPropagation()}
			>
				<div class="eyebrow">Load Error</div>
				<h3>Could not load Summary Graph</h3>
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

	<div class="hero">
		<div>
			<div class="eyebrow">Document Summaries</div>
			<h2>Summary Graph</h2>
			<p>Category-first workspace for browsing, editing, and opening category-path summary tabs.</p>
		</div>
		<div class="hero-stats">
			<div><span>Nodes</span><strong>{nodes.length}</strong></div>
			<div><span>Tabs</span><strong>{tabs.length}</strong></div>
			<div><span>Mode</span><strong>Phase 1 Mock</strong></div>
		</div>
	</div>

	<div class="tabbed-window">
		<div class="tabbed-window-head">
			<SummaryGraphTabs
				{tabs}
				{activeTabId}
				onSelect={(tabId) => (activeTabId = tabId)}
				onClose={closeTab}
			/>
		</div>

		<div class="tabbed-window-body">
			{#if activeTab.categoryPath}
				<SummaryCategoryTab
					categoryPath={activeTab.categoryPath}
					summaries={categorySummaries[activeTab.categoryPath] ?? []}
					selectedSummaryId={selectedSummaryIdByPath[activeTab.categoryPath] ?? null}
					selectedTarget={selectedTargetByPath[activeTab.categoryPath] ?? null}
					onSelectSummary={(summary) => selectSummary(activeTab.categoryPath!, summary)}
				/>
			{:else}
				<div class="graph-workspace">
					<div
						class="graph-stage"
						role="presentation"
						bind:this={graphStageEl}
						onmousemove={(event: MouseEvent) => {
							if (shouldKeepHoverAlive(event.offsetX, event.offsetY)) {
								clearHoverHideTimer();
							}
						}}
					>
						{#if loading}
							<div class="empty-state">Loading summary categories…</div>
						{:else if loadError}
							<div class="empty-state">
								Summary Graph could not be loaded. Open the error dialog for details or try again.
							</div>
						{:else}
							<Chart
								bind:chart={chartApi}
								{init}
								theme={chartTheme}
								options={treeOption}
								style="width:100%; height:100%;"
								onrendered={syncMiniViewport}
								onfinished={syncMiniViewport}
								onmouseover={(event: any) => updateHoverNode(event)}
								onmousemove={(event: any) => {
									if (event?.data?.id) updateHoverNode(event);
								}}
								onmouseout={() => scheduleHoverHide()}
								onglobalout={() => scheduleHoverHide()}
								onclick={(event: any) => {
									const nodeId = String(event?.data?.id ?? '');
									if (nodeId && nodeId !== 'summary-root') {
										selectedNodeId = nodeId;
										hoveredNodeId = nodeId;
										const clickedNode = nodes.find((node) => node.id === nodeId);
										if (clickedNode && clickedNode.childIds.length > 0 && !clickedNode.expanded) {
											nodes = toggleNodeExpanded(nodes, nodeId);
											setTimeout(syncMiniViewport, 0);
										}
										updateHoverNode(event);
									}
								}}
							/>

							{#if hoveredNode}
								<div
									bind:this={hoverCardEl}
									class="hover-card"
									role="presentation"
									style={`left:${hoverCardPos.x}px; top:${hoverCardPos.y}px;`}
									onmouseenter={() => {
										hoverCardHovering = true;
										clearHoverHideTimer();
									}}
									onmouseleave={() => {
										hoverCardHovering = false;
										scheduleHoverHide();
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
											<span>Summaries</span>
											<strong>{hoveredNode.summaryIds.length}</strong>
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
											disabled={!hoveredNode.hasSummariesFile}
											title={hoveredNode.hasSummariesFile
												? undefined
												: 'No summaries.txt file for this category'}
											onclick={() => runHoverAction('show-summaries', hoveredNode)}
										>
											Show Summaries
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
								<svg viewBox="0 0 100 100" class="mini-map-svg" aria-hidden="true">
									{#each miniMapLayout.edges as edge}
										<line
											x1={edge.fromX * 100}
											y1={edge.fromY * 100}
											x2={edge.toX * 100}
											y2={edge.toY * 100}
											class="mini-map-edge"
										/>
									{/each}
									{#each miniMapLayout.nodes as node}
										<circle
											cx={node.x * 100}
											cy={node.y * 100}
											r={node.selected ? 2.6 : 1.8}
											class:selected={node.selected}
											class="mini-map-node"
										/>
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

					<div class="inspector">
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
										<span>Summaries</span>
										<strong>{selectedNode.summaryIds.length}</strong>
									</div>
									<div class="inspector-stat">
										<span>Category Type</span>
										<strong>{selectedNode.metadata.category_type || '—'}</strong>
									</div>
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
									<button
										type="button"
										onclick={() => (nodes = toggleNodeExpanded(nodes, selectedNode.id))}
									>
										{selectedNode.expanded ? 'Collapse' : 'Expand'}
									</button>
									<button
										type="button"
										class="accent-action"
										disabled={!selectedNode.hasSummariesFile}
										title={selectedNode.hasSummariesFile
											? undefined
											: 'No summaries.txt file for this category'}
										onclick={() => showSummaries(selectedNode)}
									>
										Show Summaries
									</button>
								</div>
								<div class="action-grid">
									<button type="button" onclick={() => openDialog('rename', selectedNode.id)}
										>Rename</button
									>
									<button type="button" onclick={() => openDialog('metadata', selectedNode.id)}
										>Edit Metadata</button
									>
									<button type="button" onclick={() => openDialog('add', selectedNode.id)}
										>Add Node</button
									>
									<button type="button" onclick={() => openDialog('merge', selectedNode.id)}
										>Merge</button
									>
									<button type="button" onclick={() => openDialog('split', selectedNode.id)}
										>Split</button
									>
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

	.error-overlay {
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

	.hero {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
		padding: 1.1rem 1.2rem;
		border-radius: 24px;
		background:
			radial-gradient(circle at top left, rgba(129, 140, 248, 0.18), transparent 42%),
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
		display: grid;
		min-height: 0;
		flex: 1;
		grid-template-columns: minmax(0, 1.45fr) 360px;
		gap: 0;
	}

	.graph-stage,
	.inspector {
		min-height: 0;
		background: transparent;
	}

	.graph-stage {
		position: relative;
		overflow: hidden;
		padding: 0.85rem 0.75rem 0.75rem;
	}

	.hover-card {
		position: absolute;
		z-index: 12;
		width: min(428px, calc(100% - 2rem));
		max-width: calc(100% - 2rem);
		border-radius: 18px;
		border: 1px solid rgba(129, 140, 248, 0.2);
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
		border-color: rgba(129, 140, 248, 0.36);
		background: rgba(99, 102, 241, 0.18);
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
		border-color: rgba(129, 140, 248, 0.32);
		background: rgba(99, 102, 241, 0.18);
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
		background:
			radial-gradient(circle at top left, rgba(129, 140, 248, 0.12), transparent 48%),
			rgba(2, 6, 23, 0.34);
	}

	.mini-map-edge {
		stroke: rgba(148, 163, 184, 0.32);
		stroke-width: 1.2;
	}

	.mini-map-node {
		fill: rgba(226, 232, 240, 0.86);
	}

	.mini-map-node.selected {
		fill: #818cf8;
	}

	.mini-map-viewport {
		fill: rgba(129, 140, 248, 0.08);
		stroke: rgba(129, 140, 248, 0.92);
		stroke-width: 1.2;
		rx: 4;
	}

	.empty-state {
		color: var(--muted);
		font-size: 0.84rem;
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
		border-left: 1px solid rgba(148, 163, 184, 0.12);
		padding: 1rem;
		background: rgba(2, 6, 23, 0.16);
		overflow: auto;
	}

	.inspector-card {
		border-radius: 22px;
		border: 1px solid rgba(129, 140, 248, 0.16);
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
		background: rgba(99, 102, 241, 0.14);
		color: #c7d2fe;
		border-color: rgba(129, 140, 248, 0.3);
	}

	.action-grid .danger {
		border-color: rgba(248, 113, 113, 0.28);
		color: #fca5a5;
	}

	.empty-state {
		display: flex;
		min-height: 220px;
		align-items: center;
		justify-content: center;
		border-radius: 18px;
		border: 1px dashed rgba(148, 163, 184, 0.2);
		background: rgba(15, 23, 42, 0.2);
		text-align: center;
		padding: 1rem;
	}

	@media (max-width: 980px) {
		.graph-workspace {
			grid-template-columns: minmax(0, 1fr);
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
	}
</style>
