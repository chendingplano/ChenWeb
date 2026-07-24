<script lang="ts">
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import { renderDocument } from '$lib/documents/render';
	import type { DocumentTreeNode } from '$lib/documents/types';
	import { userManualSource } from '$lib/content/user-manual/tree';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Layout dimensions ---
	const LEFT_WIDTH_DEFAULT = 260; // default left tree panel width in px
	const LEFT_WIDTH_MIN = 180; // minimum draggable left panel width in px
	const LEFT_WIDTH_MAX = 420; // maximum draggable left panel width in px

	// --- Design tokens (matching the dashboard app shell) ---
	let pageBg = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#6B7488' : '#9CA3AF');
	let hoverBg = $derived(darkMode ? 'rgba(45,51,72,0.6)' : 'rgba(228,230,235,0.7)');

	const tree = userManualSource.listTree();

	let leftWidth = $state(LEFT_WIDTH_DEFAULT);
	let expanded = $state<Record<string, boolean>>({});
	let selectedId = $state<string | null>(null);
	let selectedLabel = $state<string | null>(null);
	let contentHtml = $state('');
	let loadError = $state<string | null>(null);

	function toggleExpanded(id: string) {
		expanded[id] = !expanded[id];
	}

	async function selectLeaf(node: DocumentTreeNode) {
		selectedId = node.id;
		selectedLabel = node.label;
		loadError = null;
		const doc = userManualSource.getDocument(node.id);
		if (!doc) {
			loadError = `No content found for "${node.label}"`;
			contentHtml = '';
			return;
		}
		try {
			contentHtml = await renderDocument(doc);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Failed to render document';
			contentHtml = '';
		}
	}

	// --- Drag resize state (scoped to this component's own divider) ---
	let isDragging = false;
	let dragStartX = 0;
	let dragStartWidth = 0;

	function startDrag(e: MouseEvent) {
		isDragging = true;
		dragStartX = e.clientX;
		dragStartWidth = leftWidth;
		document.addEventListener('mousemove', onMouseMove);
		document.addEventListener('mouseup', onMouseUp);
		e.preventDefault();
	}

	function onMouseMove(e: MouseEvent) {
		if (!isDragging) return;
		const delta = e.clientX - dragStartX;
		leftWidth = Math.max(LEFT_WIDTH_MIN, Math.min(LEFT_WIDTH_MAX, dragStartWidth + delta));
	}

	function onMouseUp() {
		isDragging = false;
		document.removeEventListener('mousemove', onMouseMove);
		document.removeEventListener('mouseup', onMouseUp);
	}
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="flex h-full overflow-hidden select-none" style="background:{pageBg};">
	<!-- Left panel: manual tree -->
	<nav
		class="flex-shrink-0 overflow-y-auto py-2"
		style="width:{leftWidth}px; background:{surface2}; border-right:1px solid {borderColor};"
	>
		{#each tree as node (node.id)}
			<div class="mb-0.5 px-2">
				{#if node.children}
					<button
						onclick={() => toggleExpanded(node.id)}
						class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 transition-colors duration-150"
						style="color:{textSecondary}; background:transparent;"
						onmouseenter={(e) => {
							(e.currentTarget as HTMLElement).style.background = hoverBg;
							(e.currentTarget as HTMLElement).style.color = textPrimary;
						}}
						onmouseleave={(e) => {
							(e.currentTarget as HTMLElement).style.background = 'transparent';
							(e.currentTarget as HTMLElement).style.color = textSecondary;
						}}
					>
						<span class="flex-1 truncate text-left" style="font-size:14px; font-weight:500;"
							>{node.label}</span
						>
						<ChevronDownIcon
							class="flex-shrink-0 transition-transform duration-200"
							style="width:14px; height:14px; transform: rotate({expanded[node.id]
								? '180deg'
								: '0deg'});"
						/>
					</button>
					{#if expanded[node.id]}
						<div class="mt-0.5 mb-1 ml-3" style="border-left:2px solid {borderColor};">
							{#each node.children as child (child.id)}
								<button
									onclick={() => selectLeaf(child)}
									class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 transition-colors duration-150"
									style="
										color: {selectedId === child.id ? accent : textMuted};
										background: {selectedId === child.id ? accentTint : 'transparent'};
										font-size: 13px;
									"
									onmouseenter={(e) => {
										if (selectedId !== child.id) {
											(e.currentTarget as HTMLElement).style.background = hoverBg;
											(e.currentTarget as HTMLElement).style.color = textPrimary;
										}
									}}
									onmouseleave={(e) => {
										if (selectedId !== child.id) {
											(e.currentTarget as HTMLElement).style.background = 'transparent';
											(e.currentTarget as HTMLElement).style.color = textMuted;
										}
									}}
								>
									{child.label}
								</button>
							{/each}
						</div>
					{/if}
				{:else}
					<button
						onclick={() => selectLeaf(node)}
						class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 transition-colors duration-150"
						style="
							color: {selectedId === node.id ? accent : textSecondary};
							background: {selectedId === node.id ? accentTint : 'transparent'};
						"
						onmouseenter={(e) => {
							if (selectedId !== node.id) {
								(e.currentTarget as HTMLElement).style.background = hoverBg;
								(e.currentTarget as HTMLElement).style.color = textPrimary;
							}
						}}
						onmouseleave={(e) => {
							if (selectedId !== node.id) {
								(e.currentTarget as HTMLElement).style.background = 'transparent';
								(e.currentTarget as HTMLElement).style.color = textSecondary;
							}
						}}
					>
						<span class="flex-1 truncate text-left" style="font-size:14px; font-weight:500;"
							>{node.label}</span
						>
					</button>
				{/if}
			</div>
		{/each}
	</nav>

	<!-- Resize divider -->
	<div
		class="group flex flex-shrink-0 cursor-col-resize items-center justify-center"
		style="width:4px; background:{borderColor}; transition:background 150ms;"
		onmousedown={startDrag}
		onmouseenter={(e) => {
			(e.currentTarget as HTMLElement).style.background = accent + '50';
		}}
		onmouseleave={(e) => {
			(e.currentTarget as HTMLElement).style.background = borderColor;
		}}
		title="Drag to resize"
	>
		<div
			class="flex flex-col gap-1 opacity-0 transition-opacity duration-150 group-hover:opacity-100"
		>
			{#each Array(4) as _}
				<div class="h-1 w-1 rounded-full" style="background:{accent};"></div>
			{/each}
		</div>
	</div>

	<!-- Right panel: selected document content -->
	<div class="flex-1 overflow-y-auto p-6">
		{#if loadError}
			<p style="color:#ef4444;">{loadError}</p>
		{:else if selectedId}
			<article class="prose-manual" style="color:{textPrimary};">
				{@html contentHtml}
			</article>
		{:else}
			<p style="color:{textSecondary};">Select a page from the tree to view its content.</p>
		{/if}
	</div>
</div>

<style>
	.prose-manual :global(h1) {
		font-size: 22px;
		font-weight: 600;
		margin-bottom: 12px;
	}
	.prose-manual :global(h2) {
		font-size: 17px;
		font-weight: 600;
		margin-top: 20px;
		margin-bottom: 8px;
	}
	.prose-manual :global(p) {
		margin-bottom: 12px;
		line-height: 1.6;
	}
	.prose-manual :global(ul),
	.prose-manual :global(ol) {
		margin-bottom: 12px;
		padding-left: 20px;
	}
	.prose-manual :global(li) {
		margin-bottom: 4px;
	}
	.prose-manual :global(code) {
		font-size: 0.9em;
		padding: 2px 5px;
		border-radius: 4px;
		background: rgba(129, 140, 248, 0.12);
	}
	.prose-manual :global(pre) {
		padding: 12px;
		border-radius: 8px;
		overflow-x: auto;
		background: rgba(0, 0, 0, 0.25);
	}
	.prose-manual :global(pre code) {
		background: none;
		padding: 0;
	}
</style>
