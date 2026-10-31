<script lang="ts">
	// One pane of a page instance, the code of the page-layouter module (devdoc
	// 2026102601 §3.2.2; OpenSpec wpi-pages-own-panes, design D7). A split pane
	// lays its children out in a row or a column. A leaf pane places what it
	// shows inside its margins, with its alignment and scroll bars: its own
	// panel, or the page instance opened in it that the Panel Cache shows. Hidden
	// page instances stay mounted. bare draws only the content: the root of a
	// page opened in a pane, whose host pane does the placing.
	import { m } from '$lib/paraglide/messages.js';
	import PageView from './PageView.svelte';
	import PanelView from './PanelView.svelte';
	import PaneView from './PaneView.svelte';
	import { clampPx, isLeaf, isResizable, inheritPaneColors, paneStyle, placeStyle, splitStyle, surfaceColorStyle } from './layout';
	import { paneInstanceId } from './panel-cache';
	import { getRuntime } from './runtime.svelte';
	import type { PaneSpec, SurfaceColors } from './types';

	let {
		instanceId,
		pane,
		axis = undefined,
		handle = null,
		bare = false,
		host = undefined,
		inheritedColors = {}
	}: { instanceId: string; pane: PaneSpec; axis?: 'width' | 'height'; handle?: 'start' | 'end' | null; bare?: boolean; host?: PaneSpec; inheritedColors?: SurfaceColors } = $props();

	const rt = getRuntime();
	const id = $derived(paneInstanceId(instanceId, pane.pane_id));
	const model = $derived(rt.panes[id]);
	const childAxis = $derived(pane.split === 'columns' ? 'width' : 'height');
	const children = $derived(pane.children ?? []);
	const colors = $derived(bare ? inheritPaneColors(host ?? {}, inheritedColors) : inheritPaneColors(pane, inheritedColors));
	const effectivePane = $derived({ ...(bare ? (host ?? { pane_id: pane.pane_id }) : pane), ...colors });
	const resizable = $derived(!!axis && handle !== null && isResizable(pane, axis));

	let section = $state<HTMLElement>();
	let dragging: number | null = null;

	function startResize(event: PointerEvent) {
		if (event.button !== 0) return;
		dragging = event.pointerId;
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		event.preventDefault();
	}

	function resize(event: PointerEvent) {
		if (dragging !== event.pointerId || !section || !axis) return;
		const r = section.getBoundingClientRect();
		const next = axis === 'width'
			? (handle === 'end' ? event.clientX - r.left : r.right - event.clientX)
			: (handle === 'end' ? event.clientY - r.top : r.bottom - event.clientY);
		rt.paneSizes[id] = clampPx(next, axis === 'width' ? pane.min_width : pane.min_height, axis === 'width' ? pane.max_width : pane.max_height);
	}

	function endResize(event: PointerEvent) {
		if (dragging === event.pointerId) dragging = null;
	}

	// A resizable child gets its handle on the edge facing its next sibling, or
	// its previous one when it is the last.
	function edge(i: number): 'start' | 'end' {
		return i === children.length - 1 ? 'start' : 'end';
	}
</script>

{#snippet split()}
	<div style={splitStyle(pane)} class={pane.split === 'columns' ? 'divide-x' : 'divide-y'}>
		{#each children as child, i (child.pane_id)}
			<PaneView {instanceId} pane={child} inheritedColors={colors} axis={childAxis} handle={edge(i)} />
		{/each}
	</div>
{/snippet}

{#if bare}
	{#if isLeaf(pane)}<PanelView {instanceId} paneId={pane.pane_id} pane={effectivePane} />{:else}{@render split()}{/if}
{:else}
	<section
		bind:this={section}
		data-pane={pane.pane_id}
		data-wpi-surface
		class="relative min-h-0 min-w-0"
		style={`${axis ? paneStyle(pane, axis, rt.paneSizes[id]) : 'flex: 1 1 auto; width: 100%; min-width: 0; min-height: 0'}; ${surfaceColorStyle(colors)}`}
	>
		{#if isLeaf(pane)}
			<div style={placeStyle(pane)}>
				<div style:display={model?.visible ? 'none' : 'contents'}>
					<PanelView {instanceId} paneId={pane.pane_id} pane={effectivePane} />
				</div>
				{#each model?.opened ?? [] as opened (opened)}
					<div style:display={opened === model?.visible ? 'contents' : 'none'}>
						<PageView instanceId={opened} host={effectivePane} />
					</div>
				{/each}
			</div>
		{:else}
			{@render split()}
		{/if}
		{#if resizable}
			<div
				role="separator"
				aria-orientation={axis === 'width' ? 'vertical' : 'horizontal'}
				aria-label={m.wpi_resize_pane()}
				class="hover:bg-border absolute z-10 select-none {axis === 'width'
					? `inset-y-0 w-1.5 cursor-col-resize ${handle === 'end' ? 'right-0' : 'left-0'}`
					: `inset-x-0 h-1.5 cursor-row-resize ${handle === 'end' ? 'bottom-0' : 'top-0'}`}"
				onpointerdown={startResize}
				onpointermove={resize}
				onpointerup={endResize}
				onpointercancel={endResize}
			></div>
		{/if}
	</section>
{/if}

<style>
 [data-wpi-surface] { color-scheme: light; }
 :global(.dark) [data-wpi-surface] { color-scheme: dark; }
</style>
