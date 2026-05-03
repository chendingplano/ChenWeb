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
		createTopicGraphTabs,
		deleteNode,
		mergeNodes,
		openCategoryTopicTab,
		renameNode,
		splitNode,
		toggleNodeExpanded
	} from './topic-graph-state.js';
	import {
		computeHoverCardPosition,
		toContainerLocalPoint
	} from './summary-graph-hover-position.js';
	import TopicCategoryTab from './topic-category-tab.svelte';
	import TopicGraphTabs from './topic-graph-tabs.svelte';
	import SummaryNodeDialog from './summary-node-dialog.svelte';
	import { getTopicCategory, listTopicGraph } from '$lib/services/kbService';
	import type {
		TopicCategoryNode,
		TopicCategoryTab as TopicCategoryTabType,
		TopicPdfTarget,
		TopicCard
	} from './topic-types';

	use([TreeChart, TooltipComponent, CanvasRenderer]);

	type DialogMode = 'rename' | 'metadata' | 'add' | 'delete' | 'merge' | 'split' | null;
	type HoverCardPos = { x: number; y: number };
	type NodeDimensions = { width: number; height: number };
	type MiniMapNode = { id: string; x: number; y: number; selected: boolean };
	type MiniMapEdge = { fromX: number; fromY: number; toX: number; toY: number };
	type MiniMapLayout = { nodes: MiniMapNode[]; edges: MiniMapEdge[] };

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let panelAlt = $derived(darkMode ? '#0f172a' : '#eef2ff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');
	let accent = $derived(darkMode ? '#22c55e' : '#16a34a');
	let warm = $derived(darkMode ? '#4ade80' : '#15803d');
	let chartTheme = $derived(darkMode ? ('dark' as const) : ('light' as const));

	let nodes = $state<TopicCategoryNode[]>([]);
	let tabs = $state<TopicCategoryTabType[]>(createTopicGraphTabs());
	let activeTabId = $state('topic-graph');
	let selectedNodeId = $state<string | null>(null);
	let dialogOpen = $state(false);
	let dialogMode = $state<DialogMode>(null);
	let categoryTopics = $state<Record<string, TopicCard[]>>({});
	let selectedTopicIdByPath = $state<Record<string, string | null>>({});
	let selectedTargetByPath = $state<Record<string, TopicPdfTarget | null>>({});
	let loading = $state(true);
	let loadError = $state('');
	let errorDialogOpen = $state(false);
	let categoryLoadingByPath = $state<Record<string, boolean>>({});
	let chartApi = $state<any>(null);
	let graphStageEl = $state<HTMLDivElement | null>(null);
	let hoverCardEl = $state<HTMLDivElement | null>(null);
	let hoveredNodeId = $state<string | null>(null);
	let hoverCardPos = $state<HoverCardPos>({ x: 28, y: 28 });
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
			if (!hoverCardHovering) hoveredNodeId = null;
		}, 120);
	}

	async function loadGraph() {
		loading = true;
		loadError = '';
		try {
			const response = await listTopicGraph();
			nodes = response.results;
			selectedNodeId = response.results[0]?.id ?? null;
		} catch (error) {
			nodes = [];
			selectedNodeId = null;
			loadError = error instanceof Error ? error.message : 'Failed to load topic graph';
			errorDialogOpen = true;
		} finally {
			loading = false;
		}
	}

	async function showTopics(node: TopicCategoryNode) {
		const result = openCategoryTopicTab(tabs, node.categoryPath);
		tabs = result.tabs;
		activeTabId = result.activeTabId;
		if (categoryTopics[node.categoryPath] || categoryLoadingByPath[node.categoryPath]) {
			return;
		}

		categoryLoadingByPath = { ...categoryLoadingByPath, [node.categoryPath]: true };
		try {
			const response = await getTopicCategory(node.categoryPath);
			categoryTopics = { ...categoryTopics, [node.categoryPath]: response.topics };
			if (response.topics[0]) {
				selectedTopicIdByPath = {
					...selectedTopicIdByPath,
					[node.categoryPath]: response.topics[0].id
				};
				selectedTargetByPath = {
					...selectedTargetByPath,
					[node.categoryPath]: {
						inputId: response.topics[0].inputId,
						page: response.topics[0].page,
						topicId: response.topics[0].id
					}
				};
			} else {
				selectedTopicIdByPath = { ...selectedTopicIdByPath, [node.categoryPath]: null };
				selectedTargetByPath = { ...selectedTargetByPath, [node.categoryPath]: null };
			}
		} catch (error) {
			loadError =
				error instanceof Error
					? error.message
					: `Failed to load topics for ${node.categoryPath}`;
			errorDialogOpen = true;
		} finally {
			categoryLoadingByPath = { ...categoryLoadingByPath, [node.categoryPath]: false };
		}
	}

	function closeTab(tabId: string) {
		tabs = tabs.filter((tab) => tab.id !== tabId);
		if (activeTabId === tabId) activeTabId = 'topic-graph';
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
						desc: String(payload.desc ?? ''),
						confidence: Number(payload.confidence ?? 0.75),
						keywords: Array.isArray(payload.keywords) ? (payload.keywords as string[]) : [],
						create_time: '20260501-120000'
					},
					childIds: [],
					topicIds: [],
					hasTopicsFile: false,
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

	function selectTopic(categoryPath: string, topic: TopicCard) {
		selectedTopicIdByPath = { ...selectedTopicIdByPath, [categoryPath]: topic.id };
		selectedTargetByPath = {
			...selectedTargetByPath,
			[categoryPath]: {
				inputId: topic.inputId,
				page: topic.page,
				topicId: topic.id
			}
		};
	}

	function keywordText(keywords: string[] | null | undefined) {
		if (!Array.isArray(keywords) || keywords.length === 0) return '—';
		return keywords.join(', ');
	}

	function getNodeDimensions(symbolSize: unknown): NodeDimensions {
		if (Array.isArray(symbolSize)) {
			const width = Number(symbolSize[0]);
			const height = Number(symbolSize[1]);
			if (Number.isFinite(width) && Number.isFinite(height)) return { width, height };
		}
		const size = Number(symbolSize);
		const fallback = Number.isFinite(size) ? size : 11;
		return { width: fallback, height: fallback };
	}

	function placeHoverCardNearNode(
		nodeX: number,
		nodeY: number,
		nodeRadius = 8,
		nodeDimensions: NodeDimensions = { width: nodeRadius * 2, height: nodeRadius * 2 }
	) {
		const stageWidth = graphStageEl?.clientWidth ?? 1200;
		const stageHeight = graphStageEl?.clientHeight ?? 720;
		const cardRect = hoverCardEl?.getBoundingClientRect?.();
		const cardWidth = cardRect?.width || 428;
		const cardHeight = cardRect?.height || 320;
		hoverCardPos = computeHoverCardPosition({
			nodeX,
			nodeY,
			nodeRadius,
			nodeWidth: nodeDimensions.width,
			nodeHeight: nodeDimensions.height,
			stageWidth,
			stageHeight,
			cardWidth,
			cardHeight,
			gap: 5,
			debug: true
		});
		console.debug('[topic-hover-placeHoverCardNearNode]', {
			nodeX,
			nodeY,
			nodeRadius,
			nodeDimensions,
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
		console.debug('[topic-hover-coordinate]', {
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

	function getHoveredNodePixel(event: any, referenceX?: number, referenceY?: number): { x: number; y: number } | null {
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
		if (!nodeId || nodeId === 'topic-root') return;
		clearHoverHideTimer();
		hoveredNodeId = nodeId;
		const nodeDimensions = getNodeDimensions(event?.data?.symbolSize);
		const nodeRadius = nodeDimensions.height / 2;
		const nativeEvent = event?.event?.event ?? event?.event ?? null;
		const nativeX = Number(nativeEvent?.offsetX);
		const nativeY = Number(nativeEvent?.offsetY);
		const nodePixel = getHoveredNodePixel(event, nativeX, nativeY);
		if (nodePixel) {
			const reconciledNodePixel =
				Number.isFinite(nativeX) && Number.isFinite(nativeY)
					? {
							x:
								Math.abs(nodePixel.x - nativeX) > nodeDimensions.width
									? nativeX
									: nodePixel.x,
							y:
								Math.abs(nodePixel.y - nativeY) > nodeDimensions.height
									? nativeY
									: nodePixel.y
						}
					: nodePixel;
			console.debug('[topic-hover-anchor]', {
				source: 'chart-layout',
				nodeId,
				nodePixel,
				reconciledNodePixel,
				nodeRadius,
				nodeDimensions,
				referencePointer:
					Number.isFinite(nativeX) && Number.isFinite(nativeY) ? { x: nativeX, y: nativeY } : null,
				rawSymbolSize: event?.data?.symbolSize
			});
			placeHoverCardNearNode(
				reconciledNodePixel.x,
				reconciledNodePixel.y,
				nodeRadius,
				nodeDimensions
			);
			return;
		}
		const fallbackX = Number(nativeEvent?.offsetX ?? 48);
		const fallbackY = Number(nativeEvent?.offsetY ?? 48);
		console.debug('[topic-hover-anchor]', {
			source: 'fallback-native-offset',
			nodeId,
			fallbackX,
			fallbackY,
			nodeRadius,
			nodeDimensions,
			rawSymbolSize: event?.data?.symbolSize
		});
		placeHoverCardNearNode(fallbackX, fallbackY, nodeRadius, nodeDimensions);
	}

	function buildMiniMapLayout(): MiniMapLayout {
		const mappedNodes: Array<{ id: string; depth: number; row: number; selected: boolean }> = [];
		const edges: Array<{ fromDepth: number; fromRow: number; toDepth: number; toRow: number }> = [];
		let row = 0;

		function visit(node: TopicCategoryNode, depth: number) {
			const nodeRow = row++;
			mappedNodes.push({ id: node.id, depth, row: nodeRow, selected: node.id === selectedNodeId });
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
		action: Exclude<DialogMode, null> | 'show-topics' | 'toggle-expand',
		node: TopicCategoryNode
	) {
		selectedNodeId = node.id;
		if (action === 'show-topics') {
			showTopics(node);
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

	function childrenOf(node: TopicCategoryNode) {
		return node.childIds
			.map((childId) => nodes.find((candidate) => candidate.id === childId))
			.filter(Boolean) as TopicCategoryNode[];
	}

	function buildTreeNode(node: TopicCategoryNode): Record<string, unknown> {
		const isSelected = node.id === selectedNodeId;
		const kws = node.metadata.keywords.slice(0, 3).join(', ') || '—';
		const conf = node.metadata.confidence ? node.metadata.confidence.toFixed(2) : '—';
		return {
			id: node.id,
			name: node.label,
			value: node.topicIds.length,
			categoryPath: node.categoryPath,
			desc: node.metadata.desc,
			confidence: node.metadata.confidence,
			keywords: node.metadata.keywords,
			topicCount: node.topicIds.length,
			collapsed: !node.expanded,
			symbol: 'rect',
			symbolSize: [160, 64],
			itemStyle: {
				color: isSelected
					? darkMode ? 'rgba(13,148,136,0.7)' : 'rgba(22,163,74,0.15)'
					: darkMode ? 'rgba(15,23,42,0.85)' : 'rgba(241,245,249,0.9)',
				borderColor: isSelected ? warm : accent,
				borderWidth: isSelected ? 2 : 1,
				borderRadius: 10,
				shadowBlur: isSelected ? 18 : 0,
				shadowColor: isSelected ? 'rgba(34,197,94,0.5)' : 'transparent'
			},
			label: {
				show: true,
				position: 'inside',
				color: isSelected ? '#ffffff' : textMain,
				formatter: (params: any) => {
					const name = String(params.data?.name ?? '');
					return `{label|${name}}\n{meta|${conf}  ${kws}}`;
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
			children: childrenOf(node).map(buildTreeNode)
		};
	}

	let treeOption = $derived.by((): EChartsOption => {
		const root = {
			id: 'topic-root',
			name: '',
			symbol: 'rect',
			symbolSize: [1, 1],
			itemStyle: { color: 'transparent', borderColor: 'transparent' },
			label: { show: false },
			lineStyle: {
				color: darkMode ? 'rgba(34,197,94,0.28)' : 'rgba(22,163,74,0.24)',
				width: 2,
				curveness: 0.55
			},
			children: visibleRoots().map(buildTreeNode)
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
					top: '4%',
					left: '4%',
					bottom: '4%',
					right: '20%',
					layout: 'orthogonal',
					orient: 'LR',
					layerPadding: 100,
					nodePadding: 32,
					symbol: 'rect',
					symbolSize: [160, 64],
					edgeShape: 'curve',
					expandAndCollapse: false,
					initialTreeDepth: -1,
					lineStyle: {
						color: darkMode ? 'rgba(34,197,94,0.28)' : 'rgba(22,163,74,0.24)',
						width: 2,
						curveness: 0.55
					},
					label: {
						position: 'inside',
						verticalAlign: 'middle',
						align: 'center',
						fontSize: 12
					},
					leaves: {
						label: {
							position: 'inside',
							align: 'center',
							color: textMain
						}
					},
					emphasis: { focus: 'descendant' },
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
		node={selectedNode ? { ...selectedNode, summaryIds: (selectedNode as any).topicIds ?? [], metadata: { ...selectedNode.metadata, category_type: '' } } : null}
		availableNodes={nodes.map((n) => ({ ...n, summaryIds: (n as any).topicIds ?? [], metadata: { ...n.metadata, category_type: '' } }))}
		onConfirm={applyDialog}
	/>

	{#if errorDialogOpen && loadError}
		<div
			class="error-overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => (errorDialogOpen = false)}
			onkeydown={(event) => { if (event.key === 'Escape') errorDialogOpen = false; }}
		>
			<div
				class="error-dialog"
				role="dialog"
				aria-modal="true"
				aria-label="Semantic Web Load Error"
				tabindex="0"
				onclick={(event) => event.stopPropagation()}
				onkeydown={(event) => event.stopPropagation()}
			>
				<div class="eyebrow">Load Error</div>
				<h3>Could not load Semantic Web</h3>
				<p class="dialog-copy">{loadError}</p>
				<div class="dialog-actions">
					<button type="button" class="secondary-btn" onclick={() => (errorDialogOpen = false)}>Close</button>
					<button type="button" class="primary-btn" onclick={async () => { errorDialogOpen = false; await loadGraph(); }}>Try Again</button>
				</div>
			</div>
		</div>
	{/if}

	<div class="hero">
		<div>
			<div class="eyebrow">Semantic Web</div>
			<h2>Topic Graph</h2>
			<p>Category-first workspace for browsing topics indexed in the Semantic Web.</p>
		</div>
		<div class="hero-stats">
			<div><span>Nodes</span><strong>{nodes.length}</strong></div>
			<div><span>Tabs</span><strong>{tabs.length}</strong></div>
			<div><span>Topics</span><strong>{nodes.reduce((s, n) => s + n.topicIds.length, 0)}</strong></div>
		</div>
	</div>

	<div class="tabbed-window">
		<div class="tabbed-window-head">
			<TopicGraphTabs
				{tabs}
				{activeTabId}
				onSelect={(tabId) => (activeTabId = tabId)}
				onClose={closeTab}
			/>
		</div>

		<div class="tabbed-window-body">
			{#if activeTab.categoryPath}
				<TopicCategoryTab
					categoryPath={activeTab.categoryPath}
					topics={categoryTopics[activeTab.categoryPath] ?? []}
					selectedTopicId={selectedTopicIdByPath[activeTab.categoryPath] ?? null}
					selectedTarget={selectedTargetByPath[activeTab.categoryPath] ?? null}
					onSelectTopic={(topic) => selectTopic(activeTab.categoryPath!, topic)}
				/>
			{:else}
				<div class="graph-workspace">
					<div class="graph-stage" bind:this={graphStageEl}>
						{#if loading}
							<div class="empty-state">Loading topic categories…</div>
						{:else if loadError}
							<div class="empty-state">
								Semantic Web could not be loaded. Open the error dialog for details or try again.
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
								onmousemove={(event: any) => { if (event?.data?.id) updateHoverNode(event); }}
								onmouseout={() => scheduleHoverHide()}
								onglobalout={() => scheduleHoverHide()}
								onclick={(event: any) => {
									const nodeId = String(event?.data?.id ?? '');
									if (nodeId && nodeId !== 'topic-root') {
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
									onmouseenter={() => { hoverCardHovering = true; clearHoverHideTimer(); }}
									onmouseleave={() => { hoverCardHovering = false; scheduleHoverHide(); }}
								>
									<div class="hover-card-head">
										<div>
											<div class="hover-card-title">Name</div>
											<div class="hover-card-value strong">{hoveredNode.label}</div>
										</div>
										<div class:active={hoveredNode.id === selectedNodeId} class="hover-selected-pill">
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
											<span>Topics</span>
											<strong>{hoveredNode.topicIds.length}</strong>
										</div>
										<div>
											<span>Children</span>
											<strong>{hoveredNode.childIds.length}</strong>
										</div>
									</div>
									<div class="hover-toolbar">
										<button type="button" class="toolbar-btn primary" disabled={!hoveredNode.hasTopicsFile} title={hoveredNode.hasTopicsFile ? undefined : 'No topics.txt file for this category'} onclick={() => runHoverAction('show-topics', hoveredNode)}>Show Topics</button>
										<button type="button" class="toolbar-btn" onclick={() => runHoverAction('toggle-expand', hoveredNode)}>{hoveredNode.expanded ? 'Collapse' : 'Expand'}</button>
										<button type="button" class="toolbar-btn" onclick={() => runHoverAction('rename', hoveredNode)}>Rename</button>
										<button type="button" class="toolbar-btn" onclick={() => runHoverAction('metadata', hoveredNode)}>Metadata</button>
										<button type="button" class="toolbar-btn" onclick={() => runHoverAction('add', hoveredNode)}>Add</button>
										<button type="button" class="toolbar-btn" onclick={() => runHoverAction('merge', hoveredNode)}>Merge</button>
										<button type="button" class="toolbar-btn" onclick={() => runHoverAction('split', hoveredNode)}>Split</button>
										<button type="button" class="toolbar-btn danger" onclick={() => runHoverAction('delete', hoveredNode)}>Delete</button>
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
										<line x1={edge.fromX * 100} y1={edge.fromY * 100} x2={edge.toX * 100} y2={edge.toY * 100} class="mini-map-edge" />
									{/each}
									{#each miniMapLayout.nodes as node}
										<rect
											x={node.x * 100 - 3}
											y={node.y * 100 - 1.5}
											width="6"
											height="3"
											rx="1"
											class:selected={node.selected}
											class="mini-map-node"
										/>
									{/each}
									<rect x={miniViewport.left * 100} y={miniViewport.top * 100} width={miniViewport.width * 100} height={miniViewport.height * 100} class="mini-map-viewport" />
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
										<span>Topics</span>
										<strong>{selectedNode.topicIds.length}</strong>
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
									<button type="button" onclick={() => (nodes = toggleNodeExpanded(nodes, selectedNode.id))}>
										{selectedNode.expanded ? 'Collapse' : 'Expand'}
									</button>
									<button type="button" class="accent-action" disabled={!selectedNode.hasTopicsFile} title={selectedNode.hasTopicsFile ? undefined : 'No topics.txt file for this category'} onclick={() => showTopics(selectedNode)}>
										Show Topics
									</button>
								</div>
								<div class="action-grid">
									<button type="button" onclick={() => openDialog('rename', selectedNode.id)}>Rename</button>
									<button type="button" onclick={() => openDialog('metadata', selectedNode.id)}>Edit Metadata</button>
									<button type="button" onclick={() => openDialog('add', selectedNode.id)}>Add Node</button>
									<button type="button" onclick={() => openDialog('merge', selectedNode.id)}>Merge</button>
									<button type="button" onclick={() => openDialog('split', selectedNode.id)}>Split</button>
									<button type="button" class="danger" onclick={() => openDialog('delete', selectedNode.id)}>Delete</button>
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

	.dialog-copy { margin-top: 0.55rem; color: var(--muted); }

	.dialog-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.primary-btn, .secondary-btn {
		border-radius: 12px;
		padding: 0.72rem 1rem;
		border: 1px solid rgba(148, 163, 184, 0.18);
		cursor: pointer;
	}

	.primary-btn { background: var(--accent); color: #fff; }
	.secondary-btn { background: rgba(15, 23, 42, 0.55); color: var(--text); }

	.hero {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
		padding: 1.1rem 1.2rem;
		border-radius: 24px;
		background:
			radial-gradient(circle at top left, rgba(34, 197, 94, 0.18), transparent 42%),
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

	h2, h3, p { margin: 0; }
	h2 { margin-top: 0.3rem; font-size: 1.5rem; }
	.hero p { margin-top: 0.45rem; max-width: 52rem; color: var(--muted); }

	.hero-stats {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.75rem;
	}

	.hero-stats div {
		min-width: 110px;
		border-radius: 16px;
		border: 1px solid rgba(34, 197, 94, 0.15);
		background: rgba(30, 41, 59, 0.36);
		padding: 0.75rem 0.9rem;
	}

	.hero-stats span { display: block; font-size: 0.72rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.08em; }
	.hero-stats strong { display: block; margin-top: 0.2rem; font-size: 1rem; }

	.tabbed-window { display: flex; min-height: 0; flex: 1; flex-direction: column; }
	.tabbed-window-head { position: relative; z-index: 4; margin-bottom: -1px; padding-inline: 0.55rem; }
	.tabbed-window-body {
		display: flex;
		min-height: 0;
		flex: 1;
		flex-direction: column;
		border: 1px solid var(--border);
		border-radius: 0 24px 24px 24px;
		background: linear-gradient(180deg, rgba(15, 23, 42, 0.72), rgba(15, 23, 42, 0.58));
		box-shadow: inset 0 1px 0 rgba(255,255,255,0.04), 0 18px 44px rgba(2,6,23,0.18);
		overflow: hidden;
	}

	.graph-workspace {
		display: grid;
		min-height: 0;
		flex: 1;
		grid-template-columns: minmax(0, 1.45fr) 360px;
		gap: 0;
	}

	.graph-stage, .inspector { min-height: 0; background: transparent; }
	.graph-stage { position: relative; overflow: hidden; padding: 0.85rem 0.75rem 0.75rem; }

	.hover-card {
		position: absolute;
		z-index: 12;
		width: min(428px, calc(100% - 2rem));
		max-width: calc(100% - 2rem);
		border-radius: 18px;
		border: 1px solid rgba(34, 197, 94, 0.2);
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
	.hover-card-row.compact > div { min-width: 0; }

	.hover-card-title, .hover-card-row span, .mini-map-head span {
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--muted);
	}

	.hover-card-value, .hover-card-row strong {
		display: block;
		margin-top: 0.2rem;
		font-size: 0.84rem;
		line-height: 1.4;
		max-width: 100%;
		overflow-wrap: anywhere;
		word-break: break-word;
		white-space: normal;
	}

	.hover-card-value.strong { font-size: 0.98rem; font-weight: 700; color: var(--text); }

	.hover-selected-pill {
		border-radius: 999px;
		border: 1px solid rgba(148, 163, 184, 0.18);
		padding: 0.3rem 0.6rem;
		font-size: 0.7rem;
		color: var(--muted);
	}

	.hover-selected-pill.active {
		border-color: rgba(34, 197, 94, 0.36);
		background: rgba(13, 148, 136, 0.18);
		color: #a7f3d0;
	}

	.hover-card-row { margin-bottom: 0.65rem; }
	.hover-card-row.compact { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0.75rem; }

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

	.toolbar-btn.primary { border-color: rgba(34, 197, 94, 0.32); background: rgba(13, 148, 136, 0.18); color: #a7f3d0; }
	.toolbar-btn.danger { border-color: rgba(248, 113, 113, 0.28); color: #fca5a5; }

	.toolbar-btn:disabled,
	.accent-action:disabled {
		opacity: 0.38;
		cursor: not-allowed;
	}

	.mini-map {
		position: absolute;
		right: 1rem;
		bottom: 1rem;
		z-index: 10;
		width: 220px;
		border-radius: 18px;
		border: 1px solid rgba(34, 197, 94, 0.18);
		background: rgba(15, 23, 42, 0.92);
		box-shadow: 0 18px 42px rgba(2, 6, 23, 0.28);
		padding: 0.75rem;
		backdrop-filter: blur(10px);
	}

	.mini-map-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 0.5rem; }
	.mini-map-head strong { font-size: 0.78rem; color: var(--text); }

	.mini-map-svg {
		display: block;
		width: 100%;
		height: 120px;
		border-radius: 12px;
		background: radial-gradient(circle at top left, rgba(34, 197, 94, 0.12), transparent 48%), rgba(2, 6, 23, 0.34);
	}

	.mini-map-edge { stroke: rgba(34, 197, 94, 0.32); stroke-width: 1.2; }
	.mini-map-node { fill: rgba(226, 232, 240, 0.86); }
	.mini-map-node.selected { fill: #22c55e; }
	.mini-map-viewport { fill: rgba(34, 197, 94, 0.08); stroke: rgba(34, 197, 94, 0.92); stroke-width: 1.2; rx: 4; }

	.empty-state {
		color: var(--muted);
		font-size: 0.84rem;
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

	.action-grid button {
		border-radius: 12px;
		border: 1px solid rgba(148, 163, 184, 0.14);
		background: transparent;
		padding: 0.55rem 0.8rem;
		color: inherit;
		cursor: pointer;
	}

	.inspector {
		border-left: 1px solid rgba(34, 197, 94, 0.12);
		padding: 1rem;
		background: rgba(2, 6, 23, 0.16);
		overflow: auto;
	}

	.inspector-card {
		border-radius: 22px;
		border: 1px solid rgba(34, 197, 94, 0.16);
		background: rgba(15, 23, 42, 0.7);
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.02);
		padding: 1rem;
	}

	.inspector-card-head { margin-bottom: 0.95rem; }

	.inspector-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0.75rem; margin: 1rem 0; }

	.inspector-stat { border-radius: 16px; background: var(--panel-alt); padding: 0.8rem; min-width: 0; }
	.inspector-stat-wide { grid-column: 1 / -1; }
	.inspector-stat span { display: block; margin-bottom: 0.2rem; font-size: 0.72rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted); }
	.inspector-stat strong { font-size: 0.92rem; display: block; max-width: 100%; overflow-wrap: anywhere; word-break: break-word; white-space: normal; }

	.action-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0.75rem; }
	.action-grid-top { margin-bottom: 0.75rem; }

	.accent-action { background: rgba(13, 148, 136, 0.14); color: #6ee7b7; border-color: rgba(34, 197, 94, 0.3); }
	.action-grid .danger { border-color: rgba(248, 113, 113, 0.28); color: #fca5a5; }

	@media (max-width: 980px) {
		.graph-workspace { grid-template-columns: minmax(0, 1fr); }
		.mini-map { width: 180px; }
		.hover-toolbar { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.inspector-grid { grid-template-columns: 1fr; }
	}
</style>
