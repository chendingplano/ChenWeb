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
	import PdfViewWindow from './pdf-view-window.svelte';

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
		GROUP_DEFS.map((d) => ({ ...d, items: [], loaded: false, loading: false, error: '', expanded: true }))
	);

	let selectedItem = $state<ArtifactCategoryItem | null>(null);
	let selectedGroupKey = $state<ArtifactGroupKey | null>(null);

	// Keep the previous SVG layout helpers available while this panel is being
	// refactored. In dev, stale HMR output can still reference these symbols.
	const SVG_W = 820;
	const SVG_H = 520;
	const CX = SVG_W / 2;
	const CY = SVG_H / 2;
	const GROUP_R = 210;
	const ITEM_R = 108;
	const MAX_ITEMS_SHOWN = 8;

	function truncateLabel(s: string, maxLen = 48): string {
		return s.length > maxLen ? s.slice(0, maxLen - 3) + '...' : s;
	}

	function groupAngle(i: number): number {
		return (2 * Math.PI * i) / GROUP_DEFS.length - Math.PI / 2;
	}

	function groupCenter(i: number): { x: number; y: number } {
		const a = groupAngle(i);
		return { x: CX + GROUP_R * Math.cos(a), y: CY + GROUP_R * Math.sin(a) };
	}

	function itemPositions(groupIndex: number, count: number): Array<{ x: number; y: number }> {
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
					label: s.id,
					inputId: s.inputId,
					page: s.page
				}));
			} else if (key === 'topics') {
				const r = await getTopicCategory(categoryPath);
				items = r.topics.map((t) => ({
					id: t.id,
					label: truncateLabel(t.topicText ?? String(t.id)),
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
					label: truncateLabel(t.topicText ?? String(t.id)),
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
	let leftWidth = $state(420);
	let midWidth = $state(260);
	let activeResize: 'left' | 'mid' | null = $state(null);
	let resizeStartX = 0;
	let resizeStartWidth = 0;

	function startResizeLeft(e: PointerEvent) {
		e.preventDefault();
		resizeStartX = e.clientX;
		resizeStartWidth = leftWidth;
		activeResize = 'left';
		window.addEventListener('pointermove', onResizeMove);
		window.addEventListener('pointerup', onResizeEnd, { once: true });
	}
	function startResizeMid(e: PointerEvent) {
		e.preventDefault();
		resizeStartX = e.clientX;
		resizeStartWidth = midWidth;
		activeResize = 'mid';
		window.addEventListener('pointermove', onResizeMove);
		window.addEventListener('pointerup', onResizeEnd, { once: true });
	}
	function onResizeMove(e: PointerEvent) {
		if (activeResize === 'left') {
			leftWidth = Math.max(260, Math.min(640, resizeStartWidth + (e.clientX - resizeStartX)));
		} else if (activeResize === 'mid') {
			midWidth = Math.max(180, Math.min(480, resizeStartWidth + (e.clientX - resizeStartX)));
		}
	}
	function onResizeEnd() {
		activeResize = null;
		window.removeEventListener('pointermove', onResizeMove);
	}

	// Selected group's color for info card
	let selectedGroupColor = $derived(
		GROUP_DEFS.find((d) => d.key === selectedGroupKey)?.color ?? '#6366f1'
	);

	// PDF viewer state
	let viewerInputId = $derived(selectedItem?.inputId ?? null);
	let viewerFileUrl = $derived(viewerInputId ? `/api/v1/kb/inputs/${viewerInputId}/file` : '');
	let viewerPage = $state(1);
	let viewerZoom = $state(0.5);
	let viewerNumPages = $state(0);

	$effect(() => {
		if (selectedItem?.page) {
			viewerPage = selectedItem.page;
		}
	});
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

	<!-- Body: chart + info + PDF -->
	<div class="panel-body">
		<!-- Left: Artifact Table -->
		<div class="table-pane" style="width:{leftWidth}px; flex: 0 0 {leftWidth}px;">
			<div class="artifact-table">
				{#each groups as group}
					<div class="artifact-section">
						<button
							type="button"
							class="section-header"
							style="--c:{group.color};"
							onclick={() => toggleGroup(group.key)}
						>
							<span class="section-dot"></span>
							<span class="section-name">{group.label}</span>
							<span class="section-badge">
								{#if group.loading}…
								{:else if group.error}!
								{:else}{group.items.length}
								{/if}
							</span>
							<span class="section-chevron" class:open={group.expanded}>›</span>
						</button>

						{#if group.expanded}
							{#if group.loading}
								<div class="section-empty">Loading…</div>
							{:else if group.error}
								<div class="section-error">{group.error}</div>
							{:else if group.items.length === 0}
								<div class="section-empty">No artifacts in this category.</div>
							{:else}
								<div class="section-rows">
									{#each group.items as item}
										{@const isSelected = selectedItem?.id === item.id && selectedGroupKey === group.key}
										<button
											type="button"
											class="artifact-row"
											class:selected={isSelected}
											style="--c:{group.color};"
											onclick={() => selectItem(item, group.key)}
											title={item.label}
										>
											<span class="row-label">{item.label}</span>
											{#if item.sublabel}
												<span class="row-sub">{item.sublabel}</span>
											{/if}
										</button>
									{/each}
								</div>
							{/if}
						{/if}
					</div>
				{/each}
			</div>
		</div>

		<!-- Resize handle: Left ↔ Middle -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="resize-handle"
			class:resizing={activeResize === 'left'}
			onpointerdown={startResizeLeft}
			role="separator"
			aria-label="Resize chart and information panels"
		></div>

		<!-- Middle: Information Panel -->
		<div class="info-pane" style="width:{midWidth}px; flex: 0 0 {midWidth}px;">
			<div class="info-pane-head">
				<div class="eyebrow">Information</div>
				{#if selectedItem}
					<span class="info-group-badge" style="--c:{selectedGroupColor};">{selectedGroupKey}</span>
				{/if}
			</div>
			{#if selectedItem}
				<div class="info-detail" style="--c:{selectedGroupColor};">
					<div class="info-title">{selectedItem.label}</div>
					{#if selectedItem.sublabel}
						<div class="info-sub">{selectedItem.sublabel}</div>
					{/if}
					<div class="info-meta">Input #{selectedItem.inputId} · Page {selectedItem.page}</div>
				</div>
			{:else}
				<div class="info-hint">
					Click an artifact row on the left to view its details.
				</div>
			{/if}
		</div>

		<!-- Resize handle: Middle ↔ Right -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="resize-handle"
			class:resizing={activeResize === 'mid'}
			onpointerdown={startResizeMid}
			role="separator"
			aria-label="Resize information and PDF panels"
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

			{#if viewerInputId && viewerFileUrl}
				<PdfViewWindow
					inputId={viewerInputId}
					fileUrl={viewerFileUrl}
					bind:page={viewerPage}
					bind:zoom={viewerZoom}
					bind:numPages={viewerNumPages}
					highlightVersion={selectedItem ? `${selectedItem.id}:${selectedItem.page}` : 0}
					sidebarSettingsKey="artifact-wiki-pdf-sidebar"
					sidebarTitle="Artifact Info"
					darkMode={darkMode}
				/>
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

	/* --- Table pane --- */
	.table-pane {
		display: flex;
		flex-direction: column;
		min-width: 0;
		overflow: hidden;
		background: var(--bg);
	}

	.artifact-table {
		flex: 1;
		overflow-y: auto;
		padding: 0.5rem 0;
		scrollbar-width: thin;
	}

	.artifact-section {
		border-bottom: 1px solid var(--border);
	}
	.artifact-section:last-child {
		border-bottom: none;
	}

	.section-header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		padding: 0.55rem 0.85rem;
		background: transparent;
		border: none;
		cursor: pointer;
		color: var(--text);
		text-align: left;
	}
	.section-header:hover {
		background: color-mix(in srgb, var(--c) 6%, transparent);
	}

	.section-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--c);
		flex-shrink: 0;
	}

	.section-name {
		font-size: 0.78rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--c);
		flex: 1;
	}

	.section-badge {
		font-size: 0.7rem;
		font-weight: 700;
		padding: 0.1rem 0.45rem;
		border-radius: 999px;
		background: color-mix(in srgb, var(--c) 14%, transparent);
		color: var(--c);
		border: 1px solid color-mix(in srgb, var(--c) 30%, transparent);
		min-width: 1.4rem;
		text-align: center;
	}

	.section-chevron {
		font-size: 1rem;
		color: var(--muted);
		transition: transform 150ms;
		display: inline-block;
		transform: rotate(0deg);
	}
	.section-chevron.open {
		transform: rotate(90deg);
	}

	.section-empty {
		padding: 0.35rem 0.85rem 0.55rem 2.2rem;
		font-size: 0.75rem;
		color: var(--muted);
		font-style: italic;
	}

	.section-error {
		padding: 0.35rem 0.85rem 0.55rem 2.2rem;
		font-size: 0.75rem;
		color: #f43f5e;
	}

	.section-rows {
		display: flex;
		flex-direction: column;
		padding-bottom: 0.35rem;
	}

	.artifact-row {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
		width: 100%;
		padding: 0.32rem 0.85rem 0.32rem 2.2rem;
		background: transparent;
		border: none;
		cursor: pointer;
		text-align: left;
		border-left: 2px solid transparent;
	}
	.artifact-row:hover {
		background: color-mix(in srgb, var(--c) 8%, transparent);
		border-left-color: color-mix(in srgb, var(--c) 40%, transparent);
	}
	.artifact-row.selected {
		background: color-mix(in srgb, var(--c) 14%, transparent);
		border-left-color: var(--c);
	}

	.row-label {
		font-size: 0.78rem;
		font-weight: 500;
		color: var(--text);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		display: block;
		max-width: 100%;
	}
	.artifact-row.selected .row-label {
		color: var(--c);
	}

	.row-sub {
		font-size: 0.7rem;
		color: var(--muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		display: block;
		max-width: 100%;
	}

	/* --- Information pane --- */
	.info-pane {
		display: flex;
		flex-direction: column;
		min-width: 0;
		overflow: hidden;
		background: var(--bg);
		border-left: 1px solid var(--border);
	}

	.info-pane-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 0.65rem 0.9rem 0.45rem;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
	}

	.info-group-badge {
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		padding: 0.15rem 0.5rem;
		border-radius: 999px;
		background: color-mix(in srgb, var(--c) 15%, transparent);
		color: var(--c);
		border: 1px solid color-mix(in srgb, var(--c) 35%, transparent);
	}

	.info-detail {
		padding: 0.85rem 0.9rem;
		border-left: 3px solid var(--c);
		margin: 0.75rem 0.75rem 0;
		border-radius: 0 8px 8px 0;
		background: color-mix(in srgb, var(--c) 8%, transparent);
	}

	.info-title {
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--text);
		line-height: 1.4;
		word-break: break-word;
	}

	.info-sub {
		font-size: 0.75rem;
		color: var(--muted);
		margin-top: 0.2rem;
		word-break: break-word;
	}

	.info-meta {
		font-size: 0.72rem;
		color: var(--muted);
		margin-top: 0.35rem;
	}

	.info-hint {
		margin: 0.85rem 0.75rem 0;
		padding: 0.6rem 0.8rem;
		border-radius: 8px;
		font-size: 0.78rem;
		color: var(--muted);
		border: 1px dashed var(--border);
		text-align: center;
		line-height: 1.5;
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

	.pdf-pane :global(.doc-page-bar) {
		border-bottom: 1px solid var(--border);
	}

	.pdf-pane :global(.pdf-stage) {
		background: var(--bg);
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
