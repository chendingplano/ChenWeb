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
	import SummaryCategoryTab from './summary-category-tab.svelte';
	import SummaryGraphTabs from './summary-graph-tabs.svelte';
	import SummaryNodeDialog from './summary-node-dialog.svelte';
	import {
		getSummaryCategoryMock,
		listSummaryGraphMock
	} from '$lib/services/kbService';
	import type {
		SummaryCategoryNode,
		SummaryCategoryTab as SummaryCategoryTabType,
		SummaryPdfTarget,
		SummaryRecordCard
	} from './summary-types';

	use([TreeChart, TooltipComponent, CanvasRenderer]);

	type DialogMode = 'rename' | 'metadata' | 'add' | 'delete' | 'merge' | 'split' | null;

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

	onMount(async () => {
		const response = await listSummaryGraphMock();
		nodes = response.results;
		selectedNodeId = response.results[0]?.id ?? null;
		loading = false;
	});

	let selectedNode = $derived(nodes.find((node) => node.id === selectedNodeId) ?? null);
	let activeTab = $derived(tabs.find((tab) => tab.id === activeTabId) ?? tabs[0]);

	function openDialog(mode: DialogMode, nodeId: string) {
		selectedNodeId = nodeId;
		dialogMode = mode;
		dialogOpen = true;
	}

	async function showSummaries(node: SummaryCategoryNode) {
		const result = openCategorySummaryTab(tabs, node.categoryPath);
		tabs = result.tabs;
		activeTabId = result.activeTabId;
		if (!categorySummaries[node.categoryPath]) {
			const response = await getSummaryCategoryMock(node.categoryPath);
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
			}
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
			nodes = splitNode(nodes, selectedNode.id, Array.isArray(payload.splitLabels) ? (payload.splitLabels as string[]) : []);
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
			symbolSize: isSelected ? 14 : 11,
			itemStyle: {
				color: isSelected ? accent : darkMode ? '#cbd5e1' : '#94a3b8',
				borderColor: isSelected ? warm : accent,
				borderWidth: isSelected ? 3 : 2
			},
			label: {
				color: isSelected ? textMain : textMuted,
				fontWeight: isSelected ? 700 : 500
			},
			children: childrenOf(node).map(buildTreeNode)
		};
	}

	function treeTooltipFormatter(params: any) {
		const data = (params?.data ?? {}) as Record<string, unknown>;
		if (!data.categoryPath) return '';
		const keywords = Array.isArray(data.keywords) ? data.keywords.join(', ') : '';
		return `
			<div style="min-width:220px">
				<div style="font-weight:700; margin-bottom:4px;">${String(data.name ?? '')}</div>
				<div style="font-size:12px; opacity:0.75; margin-bottom:6px;">${String(data.categoryPath ?? '')}</div>
				<div style="font-size:12px; margin-bottom:6px;">${String(data.desc ?? '')}</div>
				<div style="font-size:12px;">Summaries: ${String(data.summaryCount ?? 0)}</div>
				<div style="font-size:12px;">Keywords: ${keywords || '—'}</div>
			</div>
		`;
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
				trigger: 'item',
				triggerOn: 'mousemove',
				backgroundColor: darkMode ? '#0f172a' : '#ffffff',
				borderColor: darkMode ? '#334155' : '#cbd5e1',
				textStyle: {
					color: darkMode ? '#e2e8f0' : '#0f172a'
				},
				formatter: treeTooltipFormatter
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

<div class="graph-shell" style={`--panel:${panelBg}; --panel-alt:${panelAlt}; --border:${border}; --text:${textMain}; --muted:${textMuted}; --accent:${accent}; --warm:${warm};`}>
	<SummaryNodeDialog
		bind:open={dialogOpen}
		mode={dialogMode}
		node={selectedNode}
		availableNodes={nodes}
		onConfirm={applyDialog}
	/>

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

	<SummaryGraphTabs {tabs} {activeTabId} onSelect={(tabId) => (activeTabId = tabId)} onClose={closeTab} />

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
			<div class="graph-stage">
				{#if loading}
					<div class="empty-state">Loading mocked summary categories…</div>
				{:else}
					<Chart
						{init}
						theme={chartTheme}
						options={treeOption}
						style="width:100%; height:100%;"
						onclick={(event: any) => {
							const nodeId = String(event?.data?.id ?? '');
							if (nodeId && nodeId !== 'summary-root') {
								selectedNodeId = nodeId;
							}
						}}
					/>
				{/if}
			</div>

			<div class="inspector">
				<div class="eyebrow">Node Inspector</div>
				{#if selectedNode}
					<h3>{selectedNode.label}</h3>
					<p class="path">{selectedNode.categoryPath}</p>
					<div class="meta-grid">
						<div><span>Confidence</span><strong>{selectedNode.metadata.confidence}</strong></div>
						<div><span>Children</span><strong>{selectedNode.childIds.length}</strong></div>
						<div><span>Summaries</span><strong>{selectedNode.summaryIds.length}</strong></div>
						<div><span>Keywords</span><strong>{selectedNode.metadata.keywords.join(', ') || '—'}</strong></div>
					</div>
					<div class="action-grid action-grid-top">
						<button type="button" onclick={() => (nodes = toggleNodeExpanded(nodes, selectedNode.id))}>
							{selectedNode.expanded ? 'Collapse' : 'Expand'}
						</button>
						<button type="button" class="accent-action" onclick={() => showSummaries(selectedNode)}>
							Show Summaries
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
				{:else}
					<div class="empty-state">Select a category node to inspect and edit it.</div>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.graph-shell {
		position: relative;
		display: flex;
		height: 100%;
		flex-direction: column;
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

	.graph-workspace {
		display: grid;
		min-height: 0;
		flex: 1;
		grid-template-columns: minmax(0, 1.45fr) 360px;
		gap: 1rem;
	}

	.graph-stage,
	.inspector {
		min-height: 0;
		border-radius: 24px;
		border: 1px solid var(--border);
		background: var(--panel);
	}

	.graph-stage {
		overflow: hidden;
		padding: 0.5rem;
	}

	.path,
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
		padding: 1rem;
	}

	.meta-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.75rem;
		margin: 1rem 0;
	}

	.meta-grid div {
		border-radius: 16px;
		background: var(--panel-alt);
		padding: 0.8rem;
	}

	.meta-grid span {
		display: block;
		margin-bottom: 0.2rem;
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--muted);
	}

	.meta-grid strong {
		font-size: 0.92rem;
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
	}
</style>
