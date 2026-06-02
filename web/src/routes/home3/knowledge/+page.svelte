<script lang="ts">
	import { page } from '$app/state';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import SearchIcon from '@lucide/svelte/icons/search';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import ListTreeIcon from '@lucide/svelte/icons/list-tree';
	import BarChart3Icon from '@lucide/svelte/icons/bar-chart-3';
	import BoxesIcon from '@lucide/svelte/icons/boxes';
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import BriefcaseBusinessIcon from '@lucide/svelte/icons/briefcase-business';
	import FunctionSquareIcon from '@lucide/svelte/icons/function-square';
	import PackageIcon from '@lucide/svelte/icons/package';
	import QuoteIcon from '@lucide/svelte/icons/quote';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import Table2Icon from '@lucide/svelte/icons/table-2';
	import WorkflowIcon from '@lucide/svelte/icons/workflow';
	import KbImportView from '$lib/components/home3/kb-import-view.svelte';
	import MetricMgmtView from '$lib/components/home3/metric-mgmt-view.svelte';
	import ProvisionMgmtView from '$lib/components/home3/provision-mgmt-view.svelte';
	import InputsMgmtView from '$lib/components/home3/inputs-mgmt-view.svelte';
	import DocStructureView from '$lib/components/home3/doc-structure-view.svelte';
	import ChunkMgmtView from '$lib/components/home3/chunk-mgmt-view.svelte';
	import KnowledgeStoreView from '$lib/components/home3/knowledge-store-view.svelte';
	import ArtifactWikiView from '$lib/components/home3/artifact-wiki-view.svelte';
	import SummaryTreeView from '$lib/components/home3/summary-tree-view.svelte';
	import TopicGraphView from '$lib/components/home3/topic-graph-view.svelte';
	import TopicTreeView from '$lib/components/home3/topic-tree-view.svelte';
	import SceneBlocksView from '$lib/components/home3/scene-blocks-view.svelte';
	import ProductsView from '$lib/components/home3/products-view.svelte';
	import CategoryReviewView from '$lib/components/home3/category-review-view.svelte';
	import InventoryItemsView from '$lib/components/home3/inventory-items-view.svelte';
	import SemanticProjectionsView from '$lib/components/home3/semantic-projections-view.svelte';
	import { knowledgeStoreState } from '$lib/components/home3/knowledge-store-state.svelte';
	import { getProvisionCategory, listProvisionGraph } from '$lib/services/kbService';
	import {
		KNOWLEDGE_UNDER_CONSTRUCTION_SECTIONS,
		isUnderConstructionKnowledgeSection
	} from '$lib/components/home3/knowledge-sections.js';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import PanelLeftIcon from '@lucide/svelte/icons/panel-left';
	import PanelLeftCloseIcon from '@lucide/svelte/icons/panel-left-close';
	import UploadIcon from '@lucide/svelte/icons/upload';
	import LayersIcon from '@lucide/svelte/icons/layers';

	type KbSectionId =
		| 'kb-search'
		| 'kb-import'
		| 'kb-input-details'
		| 'kb-metrics'
		| 'kb-doc-structure'
		| 'kb-scene-blocks'
		| 'kb-products'
		| 'kb-chunks'
		| 'kb-category-review'
		| 'kb-summary-graph'
		| 'kb-summary-tree'
		| 'kb-topic-graph'
		| 'kb-topic-tree'
		| 'kb-provision-graph'
		| 'kb-provision-tree'
		| 'kb-inventory-items'
		| 'kb-semantic-projections'
		| 'kb-doc-wiki'
		| 'kb-references'
		| 'kb-formulas'
		| 'kb-tables'
		| 'kb-quotations'
		| 'kb-case-studies'
		| 'kb-workflow'
		| 'kb-product-parts';

	type KbMenuItem = {
		id: KbSectionId;
		label: string;
		description: string;
		icon: any;
		children?: Array<{ id: KbSectionId; label: string; description: string }>;
	};

	const menuItems: KbMenuItem[] = [
		{
			id: 'kb-search',
			label: 'Knowledge Stores',
			description: 'Explore indexed knowledge',
			icon: SearchIcon
		},
		{
			id: 'kb-import',
			label: 'Injestion',
			description: 'Ingest documents and data',
			icon: UploadIcon,
			children: [
				{
					id: 'kb-import',
					label: 'Upload Files',
					description: 'Upload and import document files'
				}
			]
		},
		{
			id: 'kb-doc-wiki',
			label: 'Wiki',
			description: 'Browse document knowledge',
			icon: BookOpenIcon,
			children: [
				{
					id: 'kb-input-details',
					label: 'Document Metadata',
					description: 'Inspect source inputs'
				},
				{
					id: 'kb-doc-structure',
					label: 'Document Structure',
					description: 'Inspect parsed hierarchy'
				},
				{
					id: 'kb-summary-tree',
					label: 'Document Tree',
					description: 'Document-centric summary browser'
				},
				{
					id: 'kb-summary-graph',
					label: 'Artifact Wiki',
					description: 'Category-first artifact exploration'
				},
				{
					id: 'kb-semantic-projections',
					label: 'Semantic Projections',
					description: 'Retrieval-optimized projection browser'
				},
				{
					id: 'kb-topic-tree',
					label: 'Document Topic Tree',
					description: 'Document-centric topic browser'
				},
				{
					id: 'kb-metrics',
					label: 'Metrics',
					description: 'Manage extracted metrics'
				},
				{
					id: 'kb-scene-blocks',
					label: 'Scene Blocks',
					description: 'Event-driven scene extraction'
				},
				{
					id: 'kb-products',
					label: 'Products',
					description: 'Product and part relation extraction'
				},
				{
					id: 'kb-provision-tree',
					label: 'Provisions',
					description: 'Document-centric provision browser'
				},
				{
					id: 'kb-inventory-items',
					label: 'Inventory Items',
					description: 'Document-centric inventory item browser'
				},
				...KNOWLEDGE_UNDER_CONSTRUCTION_SECTIONS.map((section) => ({
					id: section.id as KbSectionId,
					label: section.label,
					description: section.description
				}))
			]
		},
		{
			id: 'kb-chunks',
			label: 'Document Processing',
			description: 'Process and analyze documents',
			icon: LayersIcon,
			children: [
				{
					id: 'kb-chunks',
					label: 'Document Chunking',
					description: 'Browse chunk output'
				},
				{
					id: 'kb-category-review',
					label: 'Category Review',
					description: 'Curate the inventory category ontology'
				}
			]
		}
	];

	let menuCollapsed = $state(false);
	let menuCollapsedBeforeFocus = false;
	let menuWidth = $state(280);

	// Scene Blocks and Products "Show Selected Item" fold the Menu while an item is on the canvas.
	function handleExtractionFocusMode(focused: boolean) {
		if (focused) {
			menuCollapsedBeforeFocus = menuCollapsed;
			menuCollapsed = true;
		} else {
			menuCollapsed = menuCollapsedBeforeFocus;
		}
	}
	// Keep backward-compat alias used below.
	const handleSceneFocusMode = handleExtractionFocusMode;
	let menuResizing = $state(false);
	let menuResizeStartX = 0;
	let menuResizeStartWidth = 0;

	function startMenuResize(e: PointerEvent) {
		if (menuCollapsed) return;
		e.preventDefault();
		menuResizeStartX = e.clientX;
		menuResizeStartWidth = menuWidth;
		menuResizing = true;
		window.addEventListener('pointermove', onMenuResizeMove);
		window.addEventListener('pointerup', onMenuResizeEnd, { once: true });
	}
	function onMenuResizeMove(e: PointerEvent) {
		menuWidth = Math.max(180, Math.min(480, menuResizeStartWidth + (e.clientX - menuResizeStartX)));
	}
	function onMenuResizeEnd() {
		menuResizing = false;
		window.removeEventListener('pointermove', onMenuResizeMove);
	}

	let darkMode = $derived(page.url.searchParams.get('dark') !== '0');
	let initialSection = $derived(
		(page.url.searchParams.get('section') as KbSectionId | null) ?? 'kb-search'
	);
	let activeSection = $state<KbSectionId>('kb-search');

	$effect(() => {
		const found = menuItems.some(
			(item) =>
				item.id === initialSection ||
				item.children?.some((child) => child.id === initialSection)
		);
		if (found) {
			activeSection = initialSection;
		}
	});

	let injestionOpen = $state(false);
	let documentWikiOpen = $state(false);
	let documentProcessingOpen = $state(false);
	let activeItem = $derived(
		menuItems.find(
			(item) =>
				item.id === activeSection || item.children?.some((child) => child.id === activeSection)
		) ?? menuItems[0]
	);
	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let panelBg = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let contentBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let hoverBg = $derived(darkMode ? 'rgba(45,51,72,0.6)' : 'rgba(228,230,235,0.7)');

	let needsActiveStore = $derived(
		activeSection !== 'kb-search' &&
			activeSection !== 'kb-category-review' &&
			!isUnderConstructionKnowledgeSection(activeSection)
	);

	function selectSection(id: KbSectionId) {
		if (id === 'kb-doc-wiki') {
			activeSection = 'kb-input-details';
			documentWikiOpen = true;
			return;
		}
		activeSection = id;
		if (id === 'kb-import') {
			injestionOpen = true;
		}
		const docProcItem = menuItems.find((item) => item.id === 'kb-chunks');
		if (id === 'kb-chunks' || docProcItem?.children?.some((child) => child.id === id)) {
			documentProcessingOpen = true;
		}
		const docWikiItem = menuItems.find((item) => item.id === 'kb-doc-wiki');
		if (docWikiItem?.children?.some((child) => child.id === id)) {
			documentWikiOpen = true;
		}
	}

	function isCollapsibleParent(item: KbMenuItem) {
		return item.children && item.children.length > 0;
	}

	function isParentOpen(item: KbMenuItem) {
		if (item.id === 'kb-import') return injestionOpen;
		if (item.id === 'kb-doc-wiki') return documentWikiOpen;
		if (item.id === 'kb-chunks') return documentProcessingOpen;
		return false;
	}

	function toggleParentOpen(item: KbMenuItem) {
		if (item.id === 'kb-import') injestionOpen = !injestionOpen;
		else if (item.id === 'kb-doc-wiki') documentWikiOpen = !documentWikiOpen;
		else if (item.id === 'kb-chunks') documentProcessingOpen = !documentProcessingOpen;
	}

	function isChildActive(childId: KbSectionId) {
		return activeSection === childId;
	}

	function getUnderConstructionLabel(sectionId: KbSectionId) {
		return (
			KNOWLEDGE_UNDER_CONSTRUCTION_SECTIONS.find((section) => section.id === sectionId)?.label ??
			activeItem.label
		);
	}
</script>

<div class="kb-page flex overflow-hidden" style="background:{pageBg}; color:{textPrimary};">
	<aside
		class="kb-menu flex flex-col overflow-hidden"
		class:menu-collapsed={menuCollapsed}
		style="
			background:{panelBg};
			border-right:1px solid {borderColor};
			width:{menuCollapsed ? 56 : menuWidth}px;
			flex: 0 0 {menuCollapsed ? 56 : menuWidth}px;
			transition:{menuResizing ? 'none' : 'width 200ms ease, flex-basis 200ms ease'};
		"
	>
		<!-- Header: matches nav-rail style -->
		<div
			class="flex flex-shrink-0 items-center px-2"
			style="height:48px; border-bottom:1px solid {borderColor};"
		>
			{#if !menuCollapsed}
				<div class="flex w-full items-center justify-between px-1">
					<span style="font-size:13px; font-weight:600; color:{accent};">Knowledge</span>
					<button
						type="button"
						onclick={() => (menuCollapsed = true)}
						class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-lg transition-colors duration-150"
						style="border:none; background:transparent; color:{textMuted};"
						onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = accent; }}
						onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
						aria-label="Collapse menu"
						title="Collapse menu"
					>
						<PanelLeftCloseIcon class="h-4 w-4" />
					</button>
				</div>
			{:else}
				<button
					type="button"
					onclick={() => (menuCollapsed = false)}
					class="flex h-8 w-full cursor-pointer items-center justify-center rounded-lg transition-colors duration-150"
					style="border:none; background:transparent; color:{textMuted};"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = accent; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
					aria-label="Expand menu"
					title="Expand menu"
				>
					<PanelLeftIcon class="h-5 w-5" />
				</button>
			{/if}
		</div>

		<!-- Nav: always rendered; items adapt between icon-only and full -->
		<nav
			class="flex-1 overflow-y-auto py-2"
			style="scrollbar-width:thin; scrollbar-color:{borderColor} transparent;"
		>
			{#each menuItems as item (item.id)}
				<div class="mb-0.5 px-2">
					{#if isCollapsibleParent(item)}
						{#if menuCollapsed}
							<!-- Collapsed: icon only, clicking enters the parent section -->
							<button
								type="button"
								onclick={() => selectSection(item.id)}
								class="flex w-full cursor-pointer items-center justify-center rounded-lg transition-colors duration-150"
								style="
									padding:9px 0;
									border:none;
									background:{item.children?.some((child) => isChildActive(child.id)) ? accentTint : 'transparent'};
									color:{item.children?.some((child) => isChildActive(child.id)) ? accent : textSecondary};
								"
								title={item.label}
								aria-label={item.label}
								onmouseenter={(e) => {
									const el = e.currentTarget as HTMLElement;
									if (!item.children?.some((child) => isChildActive(child.id))) el.style.background = hoverBg;
									el.style.color = textPrimary;
								}}
								onmouseleave={(e) => {
									const el = e.currentTarget as HTMLElement;
									if (!item.children?.some((child) => isChildActive(child.id))) el.style.background = 'transparent';
									el.style.color = item.children?.some((child) => isChildActive(child.id)) ? accent : textSecondary;
								}}
							>
								<item.icon class="h-5 w-5 flex-shrink-0" />
							</button>
						{:else}
							<!-- Expanded: full collapsible parent -->
							<div
								class="rounded-lg"
								style="background:{item.children?.some((child) => isChildActive(child.id)) ? accentTint : 'transparent'};"
							>
								<button
									type="button"
									onclick={() => toggleParentOpen(item)}
									class="flex w-full cursor-pointer items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors duration-150"
									style="
										color:{item.children?.some((child) => isChildActive(child.id)) ? accent : textSecondary};
										border:none;
										border-left:2px solid {item.children?.some((child) => isChildActive(child.id)) ? accent : 'transparent'};
									"
								>
									<item.icon class="h-5 w-5 flex-shrink-0" />
									<span class="min-w-0 flex-1">
										<span class="block truncate" style="font-size:14px; font-weight:600;">{item.label}</span>
										<span class="block truncate" style="font-size:12px; color:{textMuted};">{item.description}</span>
									</span>
									<span style="font-size:12px; color:{textMuted};">{isParentOpen(item) ? '−' : '+'}</span>
								</button>
								{#if isParentOpen(item)}
									<div class="pr-2 pb-2 pl-6">
										{#each item.children ?? [] as child (child.id)}
											<button
												type="button"
												onclick={() => selectSection(child.id)}
												class="mt-1 flex w-full cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-left transition-colors duration-150"
												style="
													background:{isChildActive(child.id) ? 'rgba(255,255,255,0.08)' : 'transparent'};
													color:{isChildActive(child.id) ? textPrimary : textSecondary};
													border:none;
												"
											>
												<span style="font-size:12px; color:{isChildActive(child.id) ? accent : textMuted};">•</span>
												<span class="min-w-0">
													<span class="block truncate" style="font-size:13px; font-weight:600;">{child.label}</span>
													<span class="block truncate" style="font-size:11px; color:{textMuted};">{child.description}</span>
												</span>
											</button>
										{/each}
									</div>
								{/if}
							</div>
						{/if}
					{:else}
						<!-- Leaf item -->
						<button
							type="button"
							onclick={() => selectSection(item.id)}
							class="flex w-full cursor-pointer items-center rounded-lg text-left transition-colors duration-150"
							style="
								gap:{menuCollapsed ? '0' : '12px'};
								padding:{menuCollapsed ? '9px 0' : '8px 10px'};
								justify-content:{menuCollapsed ? 'center' : 'flex-start'};
								background:{activeSection === item.id ? accentTint : 'transparent'};
								color:{activeSection === item.id ? accent : textSecondary};
								border:none;
								border-left:{!menuCollapsed && activeSection === item.id ? '2px solid ' + accent : '2px solid transparent'};
							"
							title={menuCollapsed ? item.label : undefined}
							aria-label={item.label}
							onmouseenter={(e) => {
								const el = e.currentTarget as HTMLElement;
								if (activeSection !== item.id) el.style.background = hoverBg;
								el.style.color = textPrimary;
							}}
							onmouseleave={(e) => {
								const el = e.currentTarget as HTMLElement;
								if (activeSection !== item.id) el.style.background = 'transparent';
								el.style.color = activeSection === item.id ? accent : textSecondary;
							}}
						>
							<item.icon class="h-5 w-5 flex-shrink-0" />
							{#if !menuCollapsed}
								<span class="min-w-0 flex-1">
									<span class="block truncate" style="font-size:14px; font-weight:600;">{item.label}</span>
									<span class="block truncate" style="font-size:12px; color:{textMuted};">{item.description}</span>
								</span>
							{/if}
						</button>
					{/if}
				</div>
			{/each}
		</nav>
	</aside>

	{#if !menuCollapsed}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="menu-resize-handle"
			class:is-resizing={menuResizing}
			onpointerdown={startMenuResize}
		></div>
	{/if}

	<main class="flex min-w-0 flex-1 flex-col overflow-hidden" style="background:{contentBg};">
		<div class="min-h-0 flex-1 overflow-hidden">
			{#if needsActiveStore && !knowledgeStoreState.activeStore}
				<div
					class="flex h-full flex-col items-center justify-center p-8"
					style="background:{contentBg};"
				>
					<div
						class="rounded-2xl p-8 text-center"
						style="background:{panelBg}; border:1px solid {borderColor}; max-width:420px; width:100%;"
					>
						<div style="font-size:2.5rem; margin-bottom:1rem; opacity:0.35; color:{textSecondary};">
							◎
						</div>
						<div
							style="font-size:17px; font-weight:700; color:{textPrimary}; margin-bottom:0.5rem;"
						>
							No Knowledge Store Selected
						</div>
						<p
							style="font-size:13px; color:{textSecondary}; line-height:1.6; margin-bottom:1.5rem;"
						>
							This section operates on the active knowledge store. Go to <strong
								style="color:{textPrimary};">Knowledge Stores</strong
							> and click a card to select one before continuing.
						</p>
						<button
							type="button"
							onclick={() => selectSection('kb-search')}
							style="padding:10px 20px; border-radius:8px; background:{accent}; color:white; font-size:14px; font-weight:600; border:none; cursor:pointer;"
						>
							Go to Knowledge Stores
						</button>
					</div>
				</div>
			{:else if activeSection === 'kb-search'}
				<KnowledgeStoreView {darkMode} />
			{:else if activeSection === 'kb-import'}
				<KbImportView {darkMode} />
			{:else if activeSection === 'kb-input-details'}
				<InputsMgmtView {darkMode} />
			{:else if activeSection === 'kb-metrics'}
				<MetricMgmtView {darkMode} onFocusModeChange={handleExtractionFocusMode} />
			{:else if activeSection === 'kb-doc-structure'}
				<DocStructureView {darkMode} />
			{:else if activeSection === 'kb-scene-blocks'}
				<SceneBlocksView
					{darkMode}
					browserInstanceKey="scene-blocks"
					onFocusModeChange={handleExtractionFocusMode}
				/>
			{:else if activeSection === 'kb-products'}
				<ProductsView
					{darkMode}
					browserInstanceKey="products"
					onFocusModeChange={handleExtractionFocusMode}
				/>
			{:else if activeSection === 'kb-chunks'}
				<ChunkMgmtView {darkMode} />
			{:else if activeSection === 'kb-category-review'}
				<CategoryReviewView {darkMode} />
			{:else if activeSection === 'kb-summary-graph'}
				<ArtifactWikiView {darkMode} />
			{:else if activeSection === 'kb-summary-tree'}
				<SummaryTreeView {darkMode} onFocusModeChange={handleExtractionFocusMode} />
			{:else if activeSection === 'kb-topic-graph'}
				<TopicGraphView {darkMode} />
			{:else if activeSection === 'kb-topic-tree'}
				<TopicTreeView {darkMode} browserInstanceKey="topic-tree" />
			{:else if activeSection === 'kb-provision-graph'}
				<TopicGraphView
					{darkMode}
					heroEyebrow="Compliance Provisions"
					heroTitle="Provision Wiki"
					heroDescription="Category-first workspace for browsing compliance provisions indexed from the active knowledge store."
					rootTabLabel="Provision Wiki"
					loadErrorLabel="Provision Wiki"
					itemLabelPlural="Provisions"
					showItemsLabel="Show Provisions"
					showItemNodes={true}
					listGraph={() => listProvisionGraph(knowledgeStoreState.activeStore?.id ?? null)}
					getCategoryItems={(categoryPath) =>
						getProvisionCategory(categoryPath, knowledgeStoreState.activeStore?.id ?? null)
					}
				/>
			{:else if activeSection === 'kb-provision-tree'}
				<ProvisionMgmtView {darkMode} onFocusModeChange={handleExtractionFocusMode} />
			{:else if activeSection === 'kb-inventory-items'}
				<InventoryItemsView {darkMode} onFocusModeChange={handleExtractionFocusMode} />
			{:else if activeSection === 'kb-semantic-projections'}
				<SemanticProjectionsView {darkMode} onFocusModeChange={handleExtractionFocusMode} />
			{:else if isUnderConstructionKnowledgeSection(activeSection)}
				<div
					class="flex h-full flex-col items-center justify-center p-8"
					style="background:{contentBg};"
				>
					<div
						class="w-full rounded-2xl p-8 text-center"
						style="background:{panelBg}; border:1px solid {borderColor}; max-width:440px;"
					>
						<div
							class="mx-auto mb-5 flex h-12 w-12 items-center justify-center rounded-xl"
							style="background:{accentTint}; color:{accent}; border:1px solid {accent}30;"
						>
							<BoxesIcon class="h-6 w-6" />
						</div>
						<div
							style="font-size:17px; font-weight:700; color:{textPrimary}; margin-bottom:0.5rem;"
						>
							{getUnderConstructionLabel(activeSection)}
						</div>
						<p style="font-size:13px; color:{textSecondary}; line-height:1.6; margin:0;">
							Under construction
						</p>
					</div>
				</div>
			{/if}
		</div>
	</main>
</div>

<style>
	.kb-page {
		height: 100vh;
	}

	.kb-menu {
		flex-shrink: 0;
	}

	.menu-resize-handle {
		width: 4px;
		flex-shrink: 0;
		cursor: col-resize;
		margin-left: -1px;
		background: transparent;
		transition: background 150ms;
		user-select: none;
		touch-action: none;
		z-index: 10;
	}

	.menu-resize-handle:hover,
	.menu-resize-handle.is-resizing {
		background: rgba(129, 140, 248, 0.45);
	}

	@media (max-width: 760px) {
		.kb-page {
			flex-direction: column;
		}

		.kb-menu {
			width: 100%;
			flex: 0 0 auto;
			max-height: 44vh;
			border-right: none !important;
		}
	}
</style>
