<script lang="ts">
	import { onMount } from 'svelte';
	import {
		getSummaryCategory,
		getTopicCategory,
		getProvisionCategory,
		getMetricCategory,
		getSceneCategory,
		getProductCategory
	} from '$lib/services/kbService';
	import type { ArtifactCategoryItem } from '$lib/services/kbService';

	let {
		categoryPath,
		darkMode = true,
		ksStoreId = null
	}: {
		categoryPath: string;
		darkMode?: boolean;
		ksStoreId?: number | null;
	} = $props();

	// ---- Theming ----
	let panelBg = $derived(darkMode ? '#161c2b' : '#ffffff');
	let border = $derived(darkMode ? '#2b3548' : '#dbe3f0');
	let textMain = $derived(darkMode ? '#e2e8f0' : '#0f172a');
	let textMuted = $derived(darkMode ? '#94a3b8' : '#64748b');

	// ---- Artifact groups ----
	type ArtifactGroupKey = 'summaries' | 'topics' | 'metrics' | 'scenes' | 'provisions' | 'products';

	type ArtifactGroup = {
		key: ArtifactGroupKey;
		label: string;
		color: string;
		items: ArtifactCategoryItem[];
		loaded: boolean;
		loading: boolean;
		error: string;
		expanded: boolean;
	};

	const GROUP_DEFS: Array<{ key: ArtifactGroupKey; label: string; color: string }> = [
		{ key: 'summaries', label: 'Summaries', color: '#6366f1' },
		{ key: 'topics', label: 'Topics', color: '#22c55e' },
		{ key: 'metrics', label: 'Metrics', color: '#f59e0b' },
		{ key: 'scenes', label: 'Scenes', color: '#a855f7' },
		{ key: 'provisions', label: 'Provisions', color: '#14b8a6' },
		{ key: 'products', label: 'Products', color: '#f43f5e' }
	];

	let groups = $state<ArtifactGroup[]>(
		GROUP_DEFS.map((d) => ({ ...d, items: [], loaded: false, loading: false, error: '', expanded: false }))
	);

	let selectedItem = $state<ArtifactCategoryItem | null>(null);
	let selectedGroupKey = $state<ArtifactGroupKey | null>(null);

	// ---- SVG layout constants ----
	const SVG_W = 820;
	const SVG_H = 520;
	const CX = SVG_W / 2;
	const CY = SVG_H / 2;
	const GROUP_R = 210; // distance from center to group circle center
	const CIRCLE_R = 50; // group circle radius
	const ITEM_R = 108;  // distance from group center to item node
	const ITEM_W = 84;
	const ITEM_H = 26;
	const ITEM_RX = 13;
	const MAX_ITEMS_SHOWN = 8;

	function groupAngle(i: number): number {
		return (2 * Math.PI * i) / GROUP_DEFS.length - Math.PI / 2;
	}

	function groupCenter(i: number): { x: number; y: number } {
		const a = groupAngle(i);
		return { x: CX + GROUP_R * Math.cos(a), y: CY + GROUP_R * Math.sin(a) };
	}

	function itemPositions(
		groupIndex: number,
		count: number
	): Array<{ x: number; y: number }> {
		const { x: gx, y: gy } = groupCenter(groupIndex);
		const baseAngle = groupAngle(groupIndex);
		const n = Math.min(count, MAX_ITEMS_SHOWN);
		const spread = n === 1 ? 0 : Math.min(Math.PI * 0.65, (n - 1) * 0.28);
		return Array.from({ length: n }, (_, i) => {
			const a = n === 1 ? baseAngle : baseAngle - spread / 2 + (spread / (n - 1)) * i;
			return { x: gx + ITEM_R * Math.cos(a), y: gy + ITEM_R * Math.sin(a) };
		});
	}

	// ---- Data loading ----
	async function loadGroup(key: ArtifactGroupKey) {
		groups = groups.map((g) => (g.key === key ? { ...g, loading: true, error: '' } : g));
		try {
			let items: ArtifactCategoryItem[] = [];

			if (key === 'summaries') {
				const r = await getSummaryCategory(categoryPath);
				items = r.summaries.map((s) => ({
					id: s.id,
					label: (s.summaryText ?? s.id).slice(0, 48),
					inputId: s.inputId,
					page: s.page
				}));
			} else if (key === 'topics') {
				const r = await getTopicCategory(categoryPath);
				items = r.topics.map((t) => ({
					id: t.id,
					label: (t.topicText ?? t.id).slice(0, 48),
					inputId: t.inputId,
					page: t.page
				}));
			} else if (key === 'metrics') {
				const r = await getMetricCategory(categoryPath, ksStoreId);
				items = r.items;
			} else if (key === 'scenes') {
				const r = await getSceneCategory(categoryPath, ksStoreId);
				items = r.items;
			} else if (key === 'provisions') {
				const r = await getProvisionCategory(categoryPath, ksStoreId);
				items = r.topics.map((t) => ({
					id: t.id,
					label: (t.topicText ?? t.id).slice(0, 48),
					inputId: t.inputId,
					page: t.page
				}));
			} else if (key === 'products') {
				const r = await getProductCategory(categoryPath, ksStoreId);
				items = r.items;
			}

			groups = groups.map((g) =>
				g.key === key ? { ...g, items, loaded: true, loading: false } : g
			);
		} catch (err) {
			groups = groups.map((g) =>
				g.key === key
					? { ...g, loaded: true, loading: false, error: err instanceof Error ? err.message : 'Failed' }
					: g
			);
		}
	}

	onMount(() => {
		for (const def of GROUP_DEFS) {
			void loadGroup(def.key);
		}
	});

	// ---- Interactions ----
	function toggleGroup(key: ArtifactGroupKey) {
		const g = groups.find((g) => g.key === key);
		if (!g) return;
		groups = groups.map((gr) => (gr.key === key ? { ...gr, expanded: !gr.expanded } : gr));
	}

	function selectItem(item: ArtifactCategoryItem, groupKey: ArtifactGroupKey) {
		selectedItem = item;
		selectedGroupKey = groupKey;
	}

	// ---- Panel resizing ----
	let leftWidth = $state(520);
	let resizing = $state(false);
	let resizeStartX = 0;
	let resizeStartWidth = 0;

	function startResize(e: PointerEvent) {
		e.preventDefault();
		resizeStartX = e.clientX;
		resizeStartWidth = leftWidth;
		resizing = true;
		window.addEventListener('pointermove', onResizeMove);
		window.addEventListener('pointerup', onResizeEnd, { once: true });
	}
	function onResizeMove(e: PointerEvent) {
		leftWidth = Math.max(320, Math.min(720, resizeStartWidth + (e.clientX - resizeStartX)));
	}
	function onResizeEnd() {
		resizing = false;
		window.removeEventListener('pointermove', onResizeMove);
	}

	// Selected group's color for info card
	let selectedGroupColor = $derived(
		GROUP_DEFS.find((d) => d.key === selectedGroupKey)?.color ?? '#6366f1'
	);

	// PDF file URL for selected item
	let pdfFileUrl = $derived(
		selectedItem?.inputId
			? `/api/v1/kb/inputs/${selectedItem.inputId}/file#page=${selectedItem.page ?? 1}&zoom=page-width`
			: ''
	);
</script>

<div
	class="panel-shell"
	style="--bg:{panelBg}; --border:{border}; --text:{textMain}; --muted:{textMuted};"
>
	<!-- Header -->
	<div class="panel-head">
		<div>
			<div class="eyebrow">Artifact Wiki</div>
			<h3 title={categoryPath}>{categoryPath}</h3>
		</div>
		<div class="head-pills">
			{#each groups as g}
				{#if g.loaded && g.items.length > 0}
					<span class="count-pill" style="--c:{g.color};">{g.items.length} {g.label.toLowerCase()}</span>
				{/if}
			{/each}
		</div>
	</div>

	<!-- Body: graph + PDF -->
	<div class="panel-body">
		<!-- Left: Artifact Graph -->
		<div class="graph-pane" style="width:{leftWidth}px; flex: 0 0 {leftWidth}px;">
			<!-- Selected instance info card -->
			{#if selectedItem}
				<div class="info-card" style="--c:{selectedGroupColor};">
					<div class="info-eyebrow">{selectedGroupKey}</div>
					<div class="info-title">{selectedItem.label}</div>
					{#if selectedItem.sublabel}
						<div class="info-sub">{selectedItem.sublabel}</div>
					{/if}
					<div class="info-meta">Input #{selectedItem.inputId} · Page {selectedItem.page}</div>
				</div>
			{:else}
				<div class="info-hint">
					Click a group circle to expand it, then click an instance to view its source.
				</div>
			{/if}

			<!-- SVG graph -->
			<svg
				viewBox="0 0 {SVG_W} {SVG_H}"
				class="artifact-svg"
				xmlns="http://www.w3.org/2000/svg"
				role="img"
				aria-label="Artifact graph for {categoryPath}"
			>
				<!-- Center hub -->
				<circle cx={CX} cy={CY} r={14} class="hub-circle" />
				<text x={CX} y={CY + 5} class="hub-label">artifacts</text>

				{#each groups as group, gi}
					{@const gp = groupCenter(gi)}
					<!-- Spoke from center to group -->
					<line x1={CX} y1={CY} x2={gp.x} y2={gp.y} class="spoke" />

					<!-- Group circle (clickable) -->
					<!-- svelte-ignore a11y_click_events_have_key_events -->
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<g onclick={() => toggleGroup(group.key)} style="cursor:pointer;">
						<circle
							cx={gp.x}
							cy={gp.y}
							r={CIRCLE_R}
							fill={group.color}
							fill-opacity={group.expanded ? 0.28 : 0.14}
							stroke={group.color}
							stroke-width={group.expanded ? 2.5 : 1.5}
						/>
						<text x={gp.x} y={gp.y - 10} class="group-name" fill={group.color}>{group.label}</text>
						<text x={gp.x} y={gp.y + 12} class="group-count" fill={group.color}>
							{#if group.loading}…
							{:else if group.error}err
							{:else}{group.items.length}
							{/if}
						</text>
					</g>

					<!-- Instance nodes when expanded -->
					{#if group.expanded && group.items.length > 0}
						{@const positions = itemPositions(gi, group.items.length)}
						{#each group.items.slice(0, MAX_ITEMS_SHOWN) as item, ii}
							{@const ip = positions[ii]}
							{@const isSelected = selectedItem?.id === item.id && selectedGroupKey === group.key}
							<!-- Connector line -->
							<line
								x1={gp.x}
								y1={gp.y}
								x2={ip.x}
								y2={ip.y}
								stroke={group.color}
								stroke-width="1"
								stroke-opacity="0.55"
								stroke-dasharray="3 3"
							/>
							<!-- Item node -->
							<!-- svelte-ignore a11y_click_events_have_key_events -->
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<g onclick={() => selectItem(item, group.key)} style="cursor:pointer;">
								<rect
									x={ip.x - ITEM_W / 2}
									y={ip.y - ITEM_H / 2}
									width={ITEM_W}
									height={ITEM_H}
									rx={ITEM_RX}
									fill={isSelected ? group.color : (darkMode ? 'rgba(15,23,42,0.85)' : 'rgba(241,245,249,0.95)')}
									fill-opacity={isSelected ? 0.9 : 1}
									stroke={group.color}
									stroke-width={isSelected ? 2 : 1}
								/>
								<text
									x={ip.x}
									y={ip.y + 4}
									class="item-label"
									fill={isSelected ? '#ffffff' : group.color}
								>
									{item.label.length > 10 ? item.label.slice(0, 9) + '…' : item.label}
								</text>
							</g>
						{/each}
						{#if group.items.length > MAX_ITEMS_SHOWN}
							<text
								x={gp.x}
								y={gp.y + CIRCLE_R + 16}
								class="overflow-label"
								fill={group.color}
							>+{group.items.length - MAX_ITEMS_SHOWN} more</text>
						{/if}
					{/if}
				{/each}
			</svg>
		</div>

		<!-- Resize handle -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="resize-handle"
			class:resizing
			onpointerdown={startResize}
			role="separator"
			aria-label="Resize panels"
		></div>

		<!-- Right: PDF Display -->
		<div class="pdf-pane">
			<div class="pdf-head">
				<div>
					<div class="eyebrow">PDF Display</div>
					<h4>
						{#if selectedItem}
							Input #{selectedItem.inputId} · {selectedGroupKey}
						{:else}
							Select an artifact
						{/if}
					</h4>
				</div>
				{#if selectedItem}
					<span class="page-pill">page {selectedItem.page}</span>
				{/if}
			</div>

			{#if selectedItem && pdfFileUrl}
				<iframe
					title="PDF viewer for input {selectedItem.inputId}"
					src={pdfFileUrl}
					class="pdf-frame"
				></iframe>
			{:else}
				<div class="pdf-empty">
					<div class="pdf-empty-icon">⬡</div>
					<p>Expand a group, then click an instance to view its source document.</p>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.panel-shell {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--bg);
		color: var(--text);
		padding: 1.25rem 1.5rem 1rem;
		gap: 0.75rem;
	}

	.panel-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		flex-shrink: 0;
	}

	.eyebrow {
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--muted);
	}

	h3 {
		margin: 0.2rem 0 0;
		font-size: 1rem;
		font-weight: 700;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 600px;
	}

	h4 {
		margin: 0.2rem 0 0;
		font-size: 0.9rem;
		font-weight: 600;
	}

	.head-pills {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		justify-content: flex-end;
	}

	.count-pill {
		font-size: 0.7rem;
		font-weight: 700;
		padding: 0.2rem 0.55rem;
		border-radius: 999px;
		background: color-mix(in srgb, var(--c) 15%, transparent);
		color: var(--c);
		border: 1px solid color-mix(in srgb, var(--c) 35%, transparent);
	}

	.panel-body {
		display: flex;
		min-height: 0;
		flex: 1;
		gap: 0;
		border-radius: 16px;
		border: 1px solid var(--border);
		overflow: hidden;
	}

	/* --- Graph pane --- */
	.graph-pane {
		display: flex;
		flex-direction: column;
		min-width: 0;
		overflow: hidden;
		background: var(--bg);
	}

	.info-card {
		flex-shrink: 0;
		margin: 0.75rem 1rem 0;
		padding: 0.65rem 0.9rem;
		border-radius: 10px;
		border-left: 3px solid var(--c);
		background: color-mix(in srgb, var(--c) 10%, transparent);
	}

	.info-eyebrow {
		font-size: 0.68rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.07em;
		color: var(--c);
		margin-bottom: 0.25rem;
	}

	.info-title {
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--text);
		line-height: 1.35;
	}

	.info-sub {
		font-size: 0.75rem;
		color: var(--muted);
		margin-top: 0.15rem;
	}

	.info-meta {
		font-size: 0.72rem;
		color: var(--muted);
		margin-top: 0.3rem;
	}

	.info-hint {
		flex-shrink: 0;
		margin: 0.75rem 1rem 0;
		padding: 0.5rem 0.8rem;
		border-radius: 8px;
		font-size: 0.78rem;
		color: var(--muted);
		border: 1px dashed var(--border);
		text-align: center;
	}

	.artifact-svg {
		flex: 1;
		width: 100%;
		height: 100%;
		min-height: 0;
	}

	/* SVG text styles */
	:global(.hub-circle) {
		fill: rgba(148, 163, 184, 0.18);
		stroke: rgba(148, 163, 184, 0.45);
		stroke-width: 1.5;
	}
	:global(.hub-label) {
		fill: rgba(148, 163, 184, 0.7);
		font-size: 9px;
		text-anchor: middle;
		font-family: inherit;
	}
	:global(.spoke) {
		stroke: rgba(148, 163, 184, 0.18);
		stroke-width: 1;
	}
	:global(.group-name) {
		font-size: 11px;
		font-weight: 700;
		text-anchor: middle;
		font-family: inherit;
	}
	:global(.group-count) {
		font-size: 16px;
		font-weight: 700;
		text-anchor: middle;
		font-family: inherit;
	}
	:global(.item-label) {
		font-size: 9px;
		font-weight: 600;
		text-anchor: middle;
		font-family: inherit;
	}
	:global(.overflow-label) {
		font-size: 9px;
		text-anchor: middle;
		font-family: inherit;
		opacity: 0.75;
	}

	/* --- Resize handle --- */
	.resize-handle {
		width: 5px;
		flex-shrink: 0;
		cursor: col-resize;
		background: var(--border);
		transition: background 120ms;
	}
	.resize-handle:hover,
	.resize-handle.resizing {
		background: rgba(99, 102, 241, 0.5);
	}

	/* --- PDF pane --- */
	.pdf-pane {
		display: flex;
		flex: 1;
		min-width: 0;
		flex-direction: column;
		background: var(--bg);
	}

	.pdf-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.75rem 1rem 0.5rem;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
		gap: 0.5rem;
	}

	.page-pill {
		font-size: 0.72rem;
		font-weight: 700;
		padding: 0.2rem 0.55rem;
		border-radius: 999px;
		background: rgba(99, 102, 241, 0.14);
		color: #818cf8;
	}

	.pdf-frame {
		flex: 1;
		width: 100%;
		border: none;
		min-height: 0;
	}

	.pdf-empty {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.75rem;
		padding: 2rem;
	}

	.pdf-empty-icon {
		font-size: 2.5rem;
		opacity: 0.2;
	}

	.pdf-empty p {
		font-size: 0.82rem;
		color: var(--muted);
		text-align: center;
		max-width: 280px;
		line-height: 1.55;
		margin: 0;
	}
</style>
